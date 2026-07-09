package connectservice

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coffeehc/base/log"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/ilinkservice"
	"go.uber.org/zap"
)

const (
	toolIDWeChatMessageSend      = "wechat_message_send"
	toolIDWeChatMessageSendMedia = "wechat_message_send_media"
	toolIDWeChatTyping           = "wechat_typing"
)

const (
	weChatCDNEncryptType = 1
)

// InvokeTool 在已认证 channel 上执行 connector 声明的微信工具。
func (impl *serviceImpl) InvokeTool(ctx context.Context, input ToolInvokeInput) (*ToolInvokeResult, error) {
	connectorChannelID := strings.TrimSpace(input.ConnectorChannelID)
	if connectorChannelID == "" {
		return nil, fmt.Errorf("connector_channel_id required")
	}
	toolID := strings.TrimSpace(input.ToolID)
	if toolID == "" {
		return nil, fmt.Errorf("tool_id required")
	}
	connection := impl.ConnectionByChannel(connectorChannelID)
	if connection == nil {
		return nil, fmt.Errorf("channel is not bound to an authenticated target user")
	}
	log.Debug("执行微信 connector 工具",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("tool_id", toolID),
		zap.String("result", "started"),
	)
	switch toolID {
	case toolIDWeChatMessageSend:
		return impl.invokeMessageSend(ctx, connection, input.Arguments)
	case toolIDWeChatMessageSendMedia:
		return impl.invokeMessageSendMedia(ctx, connection, input.Arguments)
	case toolIDWeChatTyping:
		return impl.invokeTyping(ctx, connection, input.Arguments)
	default:
		return nil, fmt.Errorf("unknown tool_id: %s", toolID)
	}
}

func (impl *serviceImpl) invokeMessageSend(ctx context.Context, connection *connectordomain.Connection, arguments map[string]any) (*ToolInvokeResult, error) {
	text := stringArgument(arguments, "text", "message", "content")
	if text == "" {
		return nil, fmt.Errorf("message text required")
	}
	recipientRef := stringArgument(arguments, "recipient_ref", "to_user_id", "to", "receiver_id", "contact_id")
	if recipientRef == "" {
		recipientRef = defaultRecipientRef(connection)
	}
	contextToken := stringArgument(arguments, "context_token")
	if contextToken == "" && connection.ContextTokens != nil {
		contextToken = strings.TrimSpace(connection.ContextTokens[recipientRef])
	}
	impl.cancelAutoTypingBeforeOutbound(ctx, connection, recipientRef, contextToken)
	sendResult, contextToken, err := impl.sendWeChatTextMessage(ctx, connection, recipientRef, text, contextToken)
	if err != nil {
		log.Debug("主动发送微信 IM 消息失败",
			zap.String("connector_channel_id", connection.Token),
			zap.String("recipient_ref", recipientRef),
			zap.Bool("has_context_token", contextToken != ""),
			zap.Int("text_length", len(text)),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return nil, err
	}
	log.Debug("主动发送微信 IM 消息完成",
		zap.String("connector_channel_id", connection.Token),
		zap.String("recipient_ref", recipientRef),
		zap.Bool("has_context_token", contextToken != ""),
		zap.String("message_id", sendResult.MessageID),
		zap.String("result", "sent"),
	)
	return &ToolInvokeResult{
		ToolID: toolIDWeChatMessageSend,
		Result: map[string]any{
			"status":        "sent",
			"message_id":    sendResult.MessageID,
			"sent_at":       time.Now().UnixMilli(),
			"recipient_ref": recipientRef,
		},
	}, nil
}

func (impl *serviceImpl) invokeMessageSendMedia(ctx context.Context, connection *connectordomain.Connection, arguments map[string]any) (*ToolInvokeResult, error) {
	mediaRef := stringArgument(arguments, "media_ref", "ref")
	if mediaRef == "" {
		return nil, fmt.Errorf("media_ref required")
	}
	reference, err := impl.media.GetMediaReference(ctx, mediaRef, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	if reference == nil {
		return nil, fmt.Errorf("media_ref not found or expired")
	}
	if reference.ConnectorChannelID != connection.Token {
		return nil, fmt.Errorf("media_ref channel mismatch")
	}
	recipientRef := stringArgument(arguments, "recipient_ref", "to_user_id", "to", "receiver_id", "contact_id")
	if recipientRef != "" && recipientRef != reference.PeerRef {
		return nil, fmt.Errorf("media_ref recipient mismatch")
	}
	recipientRef = reference.PeerRef
	if recipientRef == "" {
		recipientRef = defaultRecipientRef(connection)
	}
	contextToken := stringArgument(arguments, "context_token")
	if contextToken == "" && connection.ContextTokens != nil {
		contextToken = strings.TrimSpace(connection.ContextTokens[recipientRef])
	}
	impl.cancelAutoTypingBeforeOutbound(ctx, connection, recipientRef, contextToken)
	mediaItem, err := mediaReferenceMessageItem(reference)
	if err != nil {
		return nil, err
	}
	caption := stringArgument(arguments, "text", "caption")
	messageID, captionMessageID, err := impl.sendWeChatMediaMessage(ctx, connection, recipientRef, contextToken, caption, mediaItem)
	if err != nil {
		log.Debug("发送微信 IM 媒体失败",
			zap.String("connector_channel_id", connection.Token),
			zap.String("recipient_ref", recipientRef),
			zap.String("media_ref", mediaRef),
			zap.String("media_type", reference.MediaType),
			zap.Bool("has_context_token", contextToken != ""),
			zap.Int64("byte_size", reference.RawSize),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return nil, err
	}
	log.Debug("发送微信 IM 媒体完成",
		zap.String("connector_channel_id", connection.Token),
		zap.String("recipient_ref", recipientRef),
		zap.String("media_ref", mediaRef),
		zap.String("media_type", reference.MediaType),
		zap.Bool("has_context_token", contextToken != ""),
		zap.String("message_id", messageID),
		zap.Int64("byte_size", reference.RawSize),
		zap.String("result", "sent"),
	)
	return &ToolInvokeResult{
		ToolID: toolIDWeChatMessageSendMedia,
		Result: map[string]any{
			"status":             "sent",
			"message_id":         messageID,
			"caption_message_id": captionMessageID,
			"sent_at":            time.Now().UnixMilli(),
			"recipient_ref":      recipientRef,
			"media_ref":          mediaRef,
			"media_type":         reference.MediaType,
			"filename":           reference.Filename,
			"byte_size":          reference.RawSize,
		},
	}, nil
}

func (impl *serviceImpl) invokeTyping(ctx context.Context, connection *connectordomain.Connection, arguments map[string]any) (*ToolInvokeResult, error) {
	replyToken := stringArgument(arguments, "reply_token", "to", "receiver_id")
	if replyToken == "" {
		replyToken = defaultRecipientRef(connection)
	}
	contextToken := stringArgument(arguments, "context_token")
	if contextToken == "" && connection.ContextTokens != nil {
		contextToken = strings.TrimSpace(connection.ContextTokens[replyToken])
	}
	status := typingStatusFromArguments(arguments)
	err := impl.sendWeChatTypingState(ctx, connection, replyToken, contextToken, status)
	if err != nil {
		log.Debug("发送微信输入状态失败",
			zap.String("connector_channel_id", connection.Token),
			zap.String("reply_token", replyToken),
			zap.Int("status", status),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return nil, err
	}
	log.Debug("发送微信输入状态完成",
		zap.String("connector_channel_id", connection.Token),
		zap.String("reply_token", replyToken),
		zap.Int("status", status),
		zap.String("result", "sent"),
	)
	return &ToolInvokeResult{
		ToolID: toolIDWeChatTyping,
		Result: map[string]any{
			"status":    "sent",
			"typing":    status == ilinkservice.TypingStatusTyping,
			"sent_at":   time.Now().UnixMilli(),
			"ret":       0,
			"reply_ref": replyToken,
		},
	}, nil
}

func (impl *serviceImpl) sendWeChatTextMessage(ctx context.Context, connection *connectordomain.Connection, recipientRef string, text string, contextToken string) (*ilinkservice.SendTextMessageResult, string, error) {
	recipientRef = strings.TrimSpace(recipientRef)
	if recipientRef == "" {
		return nil, "", fmt.Errorf("recipient_ref unavailable for channel")
	}
	contextToken = strings.TrimSpace(contextToken)
	if contextToken == "" && connection.ContextTokens != nil {
		contextToken = strings.TrimSpace(connection.ContextTokens[recipientRef])
	}
	result, err := impl.wechat.SendTextMessage(ctx, ilinkservice.SendTextMessageInput{
		BaseURL:      connection.BaseURL,
		BotToken:     connection.BotToken,
		ContactID:    recipientRef,
		ContextToken: contextToken,
		Text:         text,
	})
	return result, contextToken, err
}

func defaultRecipientRef(connection *connectordomain.Connection) string {
	if connection == nil {
		return ""
	}
	return strings.TrimSpace(connection.WeChatUserID)
}

func mediaReferenceMessageItem(reference *connectordomain.MediaReference) (*ilinkservice.MessageItem, error) {
	if reference == nil {
		return nil, fmt.Errorf("media reference required")
	}
	media := &ilinkservice.CDNMedia{
		EncryptQueryParam: reference.DownloadParam,
		AESKey:            reference.AESKeyBase64,
		EncryptType:       weChatCDNEncryptType,
	}
	switch reference.MediaType {
	case connectorMediaTypeImage:
		return &ilinkservice.MessageItem{
			Type: ilinkservice.MessageItemTypeImage,
			ImageItem: &ilinkservice.ImageItem{
				Media:   media,
				MidSize: reference.CipherSize,
			},
		}, nil
	case connectorMediaTypeVideo:
		return &ilinkservice.MessageItem{
			Type: ilinkservice.MessageItemTypeVideo,
			VideoItem: &ilinkservice.VideoItem{
				Media:     media,
				VideoSize: reference.CipherSize,
			},
		}, nil
	case connectorMediaTypeFile:
		return &ilinkservice.MessageItem{
			Type: ilinkservice.MessageItemTypeFile,
			FileItem: &ilinkservice.FileItem{
				Media:    media,
				FileName: reference.Filename,
				MD5:      reference.RawMD5,
				Len:      strconv.FormatInt(reference.RawSize, 10),
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported media_type: %s", reference.MediaType)
	}
}

// sendWeChatMediaMessage 按官方插件行为发送可选文本说明，再发送单条媒体消息。
func (impl *serviceImpl) sendWeChatMediaMessage(ctx context.Context, connection *connectordomain.Connection, recipientRef string, contextToken string, caption string, mediaItem *ilinkservice.MessageItem) (string, string, error) {
	captionMessageID := ""
	if caption != "" {
		sendTextResult, err := impl.wechat.SendTextMessage(ctx, ilinkservice.SendTextMessageInput{
			BaseURL:      connection.BaseURL,
			BotToken:     connection.BotToken,
			ContactID:    recipientRef,
			ContextToken: contextToken,
			Text:         caption,
		})
		if err != nil {
			return "", "", err
		}
		if sendTextResult != nil {
			captionMessageID = sendTextResult.MessageID
		}
	}
	clientID := "msg_" + randomToken(16)
	sendResult, err := impl.wechat.SendMessage(ctx, ilinkservice.SendMessageInput{
		BaseURL:  connection.BaseURL,
		BotToken: connection.BotToken,
		Message: &ilinkservice.WeixinMessage{
			ToUserID:     recipientRef,
			ClientID:     clientID,
			MessageType:  ilinkservice.MessageTypeBot,
			MessageState: ilinkservice.MessageStateFinish,
			ItemList:     []*ilinkservice.MessageItem{mediaItem},
			ContextToken: contextToken,
		},
	})
	if err != nil {
		return "", captionMessageID, err
	}
	messageID := clientID
	if sendResult != nil && strings.TrimSpace(sendResult.MessageID) != "" {
		messageID = strings.TrimSpace(sendResult.MessageID)
	}
	return messageID, captionMessageID, nil
}

func stringArgument(arguments map[string]any, keys ...string) string {
	for _, key := range keys {
		value := arguments[key]
		switch typed := value.(type) {
		case string:
			if text := strings.TrimSpace(typed); text != "" {
				return text
			}
		case fmt.Stringer:
			if text := strings.TrimSpace(typed.String()); text != "" {
				return text
			}
		}
	}
	return ""
}

func boolArgument(arguments map[string]any, key string, fallback bool) bool {
	value, ok := arguments[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "typing", "start":
			return true
		case "false", "0", "no", "cancel", "stop":
			return false
		}
	}
	return fallback
}

func typingStatusFromArguments(arguments map[string]any) int {
	if boolArgument(arguments, "typing", true) {
		return ilinkservice.TypingStatusTyping
	}
	return ilinkservice.TypingStatusCancel
}
