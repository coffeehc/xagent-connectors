package connectservice

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coffeehc/base/log"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/telegramservice"
	"go.uber.org/zap"
)

const (
	toolIDTelegramMessageSend      = "telegram_message_send"
	toolIDTelegramMessageSendMedia = "telegram_message_send_media"
)

const (
	connectorMediaTypeImage = "image"
	connectorMediaTypeVideo = "video"
	connectorMediaTypeFile  = "file"
)

// InvokeTool 在已认证 channel 上执行 connector 声明的 Telegram 工具。
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
		return nil, fmt.Errorf("channel is not bound to an authenticated Telegram chat")
	}
	switch toolID {
	case toolIDTelegramMessageSend:
		return impl.invokeMessageSend(ctx, connection, input.Arguments)
	case toolIDTelegramMessageSendMedia:
		return impl.invokeMessageSendMedia(ctx, connection, input.Arguments)
	default:
		return nil, fmt.Errorf("unknown tool_id: %s", toolID)
	}
}

func (impl *serviceImpl) invokeMessageSend(ctx context.Context, connection *connectordomain.Connection, arguments map[string]any) (*ToolInvokeResult, error) {
	text := stringArgument(arguments, "text", "message", "content")
	if text == "" {
		return nil, fmt.Errorf("message text required")
	}
	message, err := impl.telegram.SendMessage(ctx, telegramservice.SendMessageInput{
		BotToken: connection.BotToken,
		ChatID:   connection.ChatID,
		Text:     text,
	})
	if err != nil {
		log.Debug("发送 Telegram IM 消息失败",
			zap.String("connector_channel_id", connection.Token),
			zap.String("chat_id", connection.ChatID),
			zap.Int("text_length", len(text)),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return nil, err
	}
	messageID := int64(0)
	if message != nil {
		messageID = message.MessageID
	}
	return &ToolInvokeResult{
		ToolID: toolIDTelegramMessageSend,
		Result: map[string]any{
			"status":     "sent",
			"message_id": messageID,
			"sent_at":    time.Now().UnixMilli(),
			"chat_id":    connection.ChatID,
		},
	}, nil
}

func (impl *serviceImpl) invokeMessageSendMedia(ctx context.Context, connection *connectordomain.Connection, arguments map[string]any) (*ToolInvokeResult, error) {
	mediaRef := stringArgument(arguments, "media_ref", "ref")
	if mediaRef == "" {
		return nil, fmt.Errorf("media_ref required")
	}
	reference, err := impl.storage.GetMediaReference(mediaRef, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	if reference == nil {
		return nil, fmt.Errorf("media_ref not found or expired")
	}
	if reference.ConnectorChannelID != connection.Token {
		return nil, fmt.Errorf("media_ref channel mismatch")
	}
	caption := stringArgument(arguments, "text", "caption")
	message, err := impl.telegram.SendMedia(ctx, telegramservice.SendMediaInput{
		BotToken:  connection.BotToken,
		ChatID:    connection.ChatID,
		MediaType: reference.MediaType,
		FileID:    reference.FileID,
		LocalPath: reference.LocalPath,
		Filename:  reference.Filename,
		Caption:   caption,
	})
	if err != nil {
		log.Debug("发送 Telegram IM 媒体失败",
			zap.String("connector_channel_id", connection.Token),
			zap.String("chat_id", connection.ChatID),
			zap.String("media_ref", mediaRef),
			zap.String("media_type", reference.MediaType),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return nil, err
	}
	messageID := int64(0)
	if message != nil {
		messageID = message.MessageID
	}
	return &ToolInvokeResult{
		ToolID: toolIDTelegramMessageSendMedia,
		Result: map[string]any{
			"status":     "sent",
			"message_id": messageID,
			"sent_at":    time.Now().UnixMilli(),
			"chat_id":    connection.ChatID,
			"media_ref":  mediaRef,
			"media_type": reference.MediaType,
			"filename":   reference.Filename,
			"byte_size":  reference.ByteSize,
		},
	}, nil
}

// UploadMedia 上传待发送媒体到 connector 本地缓存并返回 media_ref。
func (impl *serviceImpl) UploadMedia(_ context.Context, input UploadMediaInput) (*UploadMediaResult, error) {
	connectorChannelID := strings.TrimSpace(input.ConnectorChannelID)
	if connectorChannelID == "" {
		return nil, fmt.Errorf("connector_channel_id required")
	}
	connection := impl.ConnectionByChannel(connectorChannelID)
	if connection == nil {
		return nil, fmt.Errorf("channel is not bound to an authenticated Telegram chat")
	}
	if input.Source == nil {
		return nil, fmt.Errorf("file source required")
	}
	filename := strings.TrimSpace(input.Filename)
	if filename == "" {
		filename = "telegram-upload.bin"
	}
	mediaType := inferMediaType(filename, input.ContentType)
	now := time.Now().UnixMilli()
	mediaRef := "tgmedia_" + randomToken(12)
	cacheDir := filepath.Join(impl.storage.StateDir(), protocol.MediaCacheDirname, connection.Token)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, err
	}
	localPath := filepath.Join(cacheDir, mediaRef+"_"+sanitizeFilename(filename))
	file, err := os.OpenFile(localPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	byteSize, copyErr := io.Copy(file, input.Source)
	closeErr := file.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if input.Size > 0 {
		byteSize = input.Size
	}
	reference := &connectordomain.MediaReference{
		Ref:                mediaRef,
		Direction:          "outbound",
		ConnectorChannelID: connection.Token,
		BotID:              connection.BotID,
		ChatID:             connection.ChatID,
		MediaType:          mediaType,
		Filename:           filename,
		ContentType:        strings.TrimSpace(input.ContentType),
		ByteSize:           byteSize,
		LocalPath:          localPath,
		CreatedAt:          now,
		ExpiresAt:          now + protocol.DefaultMediaReferenceTTLMillis,
	}
	if err = impl.storage.SaveMediaReference(reference); err != nil {
		return nil, err
	}
	return &UploadMediaResult{
		MediaRef:  reference.Ref,
		MediaType: reference.MediaType,
		Filename:  reference.Filename,
		ByteSize:  reference.ByteSize,
		ExpiresAt: reference.ExpiresAt,
	}, nil
}

// OpenMedia 打开入站或出站媒体引用内容。
func (impl *serviceImpl) OpenMedia(ctx context.Context, mediaRef string) (*OpenMediaResult, error) {
	reference, err := impl.storage.GetMediaReference(mediaRef, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	if reference == nil {
		return nil, fmt.Errorf("media_ref not found or expired")
	}
	if strings.TrimSpace(reference.LocalPath) != "" {
		payload, readErr := os.ReadFile(reference.LocalPath)
		if readErr != nil {
			return nil, readErr
		}
		return &OpenMediaResult{ContentType: reference.ContentType, Filename: reference.Filename, Body: payload}, nil
	}
	file, err := impl.telegram.GetFile(ctx, telegramservice.GetFileInput{
		BotToken: reference.BotToken,
		FileID:   reference.FileID,
	})
	if err != nil {
		return nil, err
	}
	if file == nil || strings.TrimSpace(file.FilePath) == "" {
		return nil, fmt.Errorf("Telegram file_path unavailable")
	}
	download, err := impl.telegram.DownloadFile(ctx, telegramservice.DownloadFileInput{
		BotToken: reference.BotToken,
		FilePath: file.FilePath,
	})
	if err != nil {
		return nil, err
	}
	contentType := reference.ContentType
	if contentType == "" && download != nil {
		contentType = download.ContentType
	}
	body := []byte(nil)
	if download != nil {
		body = download.Body
	}
	return &OpenMediaResult{ContentType: contentType, Filename: reference.Filename, Body: body}, nil
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

func inferMediaType(filename string, contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	filename = strings.ToLower(strings.TrimSpace(filename))
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return connectorMediaTypeImage
	case strings.HasPrefix(contentType, "video/"):
		return connectorMediaTypeVideo
	case strings.HasSuffix(filename, ".jpg"), strings.HasSuffix(filename, ".jpeg"), strings.HasSuffix(filename, ".png"), strings.HasSuffix(filename, ".gif"), strings.HasSuffix(filename, ".webp"):
		return connectorMediaTypeImage
	case strings.HasSuffix(filename, ".mp4"), strings.HasSuffix(filename, ".mov"), strings.HasSuffix(filename, ".webm"):
		return connectorMediaTypeVideo
	default:
		return connectorMediaTypeFile
	}
}

func sanitizeFilename(filename string) string {
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "." || filename == string(filepath.Separator) || filename == "" {
		return "file"
	}
	return strings.NewReplacer("/", "_", "\\", "_", "\x00", "_").Replace(filename)
}
