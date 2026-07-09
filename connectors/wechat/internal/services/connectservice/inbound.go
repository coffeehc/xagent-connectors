package connectservice

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coffeehc/base/log"
	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/ilinkservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/mediaservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
	"go.uber.org/zap"
)

func (impl *serviceImpl) startConnectionMonitorLocked(connection *connectordomain.Connection) {
	if connection == nil || strings.TrimSpace(connection.Token) == "" {
		return
	}
	if impl.monitors == nil {
		impl.monitors = map[string]context.CancelFunc{}
	}
	if impl.monitors[connection.Token] != nil {
		return
	}
	parent := impl.stopCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	impl.monitors[connection.Token] = cancel
	snapshot := cloneConnection(connection)
	go impl.monitorConnection(ctx, snapshot)
	log.Debug("微信入站消息监听已启动",
		zap.String("connector_channel_id", connection.Token),
		zap.String("account_hint", connection.AccountHint),
		zap.String("result", "started"),
	)
}

func (impl *serviceImpl) stopConnectionMonitorLocked(connectorChannelID string) {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" || impl.monitors == nil {
		return
	}
	cancel := impl.monitors[connectorChannelID]
	delete(impl.monitors, connectorChannelID)
	if cancel != nil {
		cancel()
	}
}

func (impl *serviceImpl) monitorConnection(ctx context.Context, connection *connectordomain.Connection) {
	defer impl.removeConnectionMonitor(connection.Token)
	if _, err := impl.wechat.NotifyStart(ctx, ilinkservice.NotifyInput{
		BaseURL:  connection.BaseURL,
		BotToken: connection.BotToken,
	}); err != nil && ctx.Err() == nil {
		log.Debug("通知微信入站消息监听启动失败",
			zap.String("connector_channel_id", connection.Token),
			zap.String("result", "failed"),
			zap.Error(err),
		)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := impl.wechat.NotifyStop(stopCtx, ilinkservice.NotifyInput{
			BaseURL:  connection.BaseURL,
			BotToken: connection.BotToken,
		}); err != nil {
			log.Debug("通知微信入站消息监听停止失败",
				zap.String("connector_channel_id", connection.Token),
				zap.String("result", "failed"),
				zap.Error(err),
			)
		}
	}()

	consecutiveFailures := 0
	nextTimeoutMillis := int(impl.longPollTTL / time.Millisecond)
	if nextTimeoutMillis <= 0 {
		nextTimeoutMillis = 35000
	}
	for ctx.Err() == nil {
		resp, err := impl.wechat.GetUpdates(ctx, ilinkservice.GetUpdatesInput{
			BaseURL:               connection.BaseURL,
			BotToken:              connection.BotToken,
			GetUpdatesBuf:         connection.GetUpdatesBuf,
			LongPollTimeoutMillis: nextTimeoutMillis,
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			consecutiveFailures++
			log.Debug("拉取微信入站消息失败",
				zap.String("connector_channel_id", connection.Token),
				zap.Int("consecutive_failure_count", consecutiveFailures),
				zap.String("result", "failed"),
				zap.Error(err),
			)
			impl.sleepAfterGetUpdatesFailure(ctx, consecutiveFailures)
			continue
		}
		if resp == nil {
			continue
		}
		if resp.LongpollingTimeoutMillis > 0 {
			nextTimeoutMillis = resp.LongpollingTimeoutMillis
		}
		if isGetUpdatesAPIError(resp) {
			if resp.ErrCode == staleTokenErrCode || resp.Ret == staleTokenErrCode {
				reason := "微信 bot token 已失效，可能已被新的机器人绑定接管。"
				descriptor := impl.clearConnectionAuth(connection.Token, reason)
				impl.pushConnectionDescriptor(ctx, connection.Token, descriptor)
				log.Debug("微信目标账号绑定已清理",
					zap.String("connector_channel_id", connection.Token),
					zap.Int("ret", resp.Ret),
					zap.Int("errcode", resp.ErrCode),
					zap.String("connection_status", string(connectorprotocol.ConnectionStatusCreated)),
					zap.String("result", "cleared"),
				)
				return
			}
			consecutiveFailures++
			log.Debug("微信入站消息响应异常",
				zap.String("connector_channel_id", connection.Token),
				zap.Int("ret", resp.Ret),
				zap.Int("errcode", resp.ErrCode),
				zap.String("errmsg", resp.ErrMsg),
				zap.Int("consecutive_failure_count", consecutiveFailures),
				zap.String("result", "failed"),
			)
			impl.sleepAfterGetUpdatesFailure(ctx, consecutiveFailures)
			continue
		}
		consecutiveFailures = 0
		queuedCount := 0
		for _, message := range resp.Messages {
			if impl.enqueueInboundWeixinMessage(ctx, connection, message) {
				queuedCount++
			}
		}
		if strings.TrimSpace(resp.GetUpdatesBuf) != "" && resp.GetUpdatesBuf != connection.GetUpdatesBuf {
			if len(resp.Messages) == 0 || queuedCount == len(resp.Messages) {
				connection.GetUpdatesBuf = resp.GetUpdatesBuf
				impl.persistConnectionState(connection)
			} else {
				log.Debug("暂缓推进微信入站游标：存在未成功落盘消息",
					zap.String("connector_channel_id", connection.Token),
					zap.Int("message_count", len(resp.Messages)),
					zap.Int("queued_count", queuedCount),
					zap.String("result", "deferred"),
				)
			}
		}
	}
}

// clearConnectionAuth 清理失效微信 bot token 对应的目标账号绑定。
func (impl *serviceImpl) clearConnectionAuth(connectorChannelID string, reason string) *connectorprotocol.ConnectionDescriptor {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" {
		return nil
	}
	reason = strings.TrimSpace(reason)
	impl.mu.Lock()
	connection := impl.connections[connectorChannelID]
	if connection == nil {
		impl.mu.Unlock()
		return impl.buildBaseChannelDescriptor(connectorChannelID, connectorprotocol.ConnectionStatusCreated)
	}
	accountHint := connection.AccountHint
	delete(impl.connections, connectorChannelID)
	delete(impl.channels, connectorChannelID)
	impl.pruneUnboundBots(impl.bots, impl.channels)
	err := impl.saveConnectorIdentityStateLocked()
	impl.mu.Unlock()
	impl.clearAutoTypingStatesForChannel(connectorChannelID)
	if err != nil {
		log.Debug("保存微信目标账号绑定清理状态失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("account_hint", accountHint),
			zap.String("result", "failed"),
			zap.Error(err),
		)
	}
	log.Debug("微信目标账号绑定已从本地状态清理",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("account_hint", accountHint),
		zap.String("reason", reason),
		zap.String("result", "succeeded"),
	)
	return impl.buildBaseChannelDescriptor(connectorChannelID, connectorprotocol.ConnectionStatusCreated)
}

func (impl *serviceImpl) pushConnectionDescriptor(ctx context.Context, connectorChannelID string, descriptor *connectorprotocol.ConnectionDescriptor) {
	if descriptor == nil {
		return
	}
	pusher := impl.currentMessagePusher()
	if pusher == nil {
		return
	}
	if err := pusher.PushConnectionDescriptor(ctx, connectorChannelID, descriptor); err != nil {
		log.Debug("推送微信 connection descriptor 失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("connection_status", string(descriptor.Connection.Status)),
			zap.String("result", "failed"),
			zap.Error(err),
		)
	}
}

func (impl *serviceImpl) enqueueInboundWeixinMessage(ctx context.Context, connection *connectordomain.Connection, message *ilinkservice.WeixinMessage) bool {
	if message == nil {
		return true
	}
	if message.ContextToken != "" && message.FromUserID != "" {
		if connection.ContextTokens == nil {
			connection.ContextTokens = map[string]string{}
		}
		connection.ContextTokens[message.FromUserID] = message.ContextToken
	}
	connection.LastInboundAt = time.Now().UnixMilli()
	impl.persistConnectionState(connection)

	payload := impl.buildInboundMessagePayload(ctx, connection, message)
	now := time.Now().UnixMilli()
	pending := &connectordomain.PendingInboundMessage{
		ID:                 pendingInboundMessageID(connection.Token, message),
		ConnectorChannelID: connection.Token,
		WeChatMessageID:    fmt.Sprint(message.MessageID),
		WeChatSeq:          message.Seq,
		ClientID:           message.ClientID,
		Payload:            payload,
		ReceivedAt:         now,
		ExpiresAt:          now + impl.inboundTTL.Milliseconds(),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := impl.storage.EnqueuePendingInboundMessage(pending, impl.inboundMaxPerChannel); err != nil {
		log.Debug("缓存微信入站消息失败",
			zap.String("connector_channel_id", connection.Token),
			zap.String("message_id", fmt.Sprint(message.MessageID)),
			zap.String("from_user_id", message.FromUserID),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return false
	}
	log.Debug("缓存微信入站消息完成",
		zap.String("connector_channel_id", connection.Token),
		zap.String("message_id", fmt.Sprint(message.MessageID)),
		zap.String("from_user_id", message.FromUserID),
		zap.Int("text_length", len(textFromMessage(message))),
		zap.String("result", "queued"),
	)
	impl.FlushInboundMessages(ctx, connection.Token)
	return true
}

func (impl *serviceImpl) currentMessagePusher() MessagePusher {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	return impl.pusher
}

// FlushInboundMessages 投递指定 channel 本地缓存的入站消息。
func (impl *serviceImpl) FlushInboundMessages(ctx context.Context, connectorChannelID string) {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" {
		return
	}
	pusher := impl.currentMessagePusher()
	if pusher == nil {
		return
	}
	now := time.Now().UnixMilli()
	messages, err := impl.storage.ListPendingInboundMessages(connectorChannelID, now, 0)
	if err != nil {
		log.Debug("读取微信入站消息缓存失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return
	}
	for _, message := range messages {
		if message == nil {
			continue
		}
		if err = pusher.PushMessage(ctx, connectorChannelID, message.Payload); err != nil {
			_ = impl.storage.MarkPendingInboundFailed(connectorChannelID, message.ID, time.Now().UnixMilli(), err.Error())
			if isConnectorChannelNotOpenError(err) {
				log.Debug("等待 connector channel 打开后投递微信入站消息",
					zap.String("connector_channel_id", connectorChannelID),
					zap.String("pending_message_id", message.ID),
					zap.Int("attempt_count", message.AttemptCount+1),
					zap.String("result", "waiting_channel"),
					zap.Error(err),
				)
			} else {
				log.Debug("投递缓存微信入站消息失败",
					zap.String("connector_channel_id", connectorChannelID),
					zap.String("pending_message_id", message.ID),
					zap.Int("attempt_count", message.AttemptCount+1),
					zap.String("result", "failed"),
					zap.Error(err),
				)
			}
			return
		}
		deliveredAt := time.Now().UnixMilli()
		if err = impl.storage.MarkPendingInboundDelivered(connectorChannelID, message.ID, deliveredAt); err != nil {
			log.Debug("更新微信入站消息投递游标失败",
				zap.String("connector_channel_id", connectorChannelID),
				zap.String("pending_message_id", message.ID),
				zap.String("result", "failed"),
				zap.Error(err),
			)
			return
		}
		log.Debug("投递缓存微信入站消息完成",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("pending_message_id", message.ID),
			zap.String("result", "delivered"),
		)
		impl.startAutoTypingForDeliveredMessage(ctx, connectorChannelID, message.Payload)
	}
}

func isConnectorChannelNotOpenError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "connector channel 未打开")
}

func (impl *serviceImpl) cleanupPendingInboundLoop() {
	interval := impl.inboundCleanupInterval
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	timer := time.NewTicker(interval)
	defer timer.Stop()
	stopCtx := impl.stopCtx
	if stopCtx == nil {
		return
	}
	for {
		select {
		case <-stopCtx.Done():
			return
		case <-timer.C:
			now := time.Now().UnixMilli()
			impl.pruneExpiredPendingInboundMessages(now)
			impl.pruneExpiredMediaReferences(now)
		}
	}
}

func (impl *serviceImpl) pruneExpiredPendingInboundMessages(nowMillis int64) {
	removed, err := impl.storage.PruneExpiredPendingInboundMessages(nowMillis)
	if err != nil {
		log.Debug("清理过期微信入站消息缓存失败",
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return
	}
	if removed > 0 {
		log.Debug("清理过期微信入站消息缓存完成",
			zap.Int("removed_count", removed),
			zap.String("result", "succeeded"),
		)
	}
}

func (impl *serviceImpl) pruneExpiredMediaReferences(nowMillis int64) {
	removed, err := impl.media.PruneExpiredMediaReferences(context.Background(), nowMillis)
	if err != nil {
		log.Debug("清理过期微信媒体引用失败",
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return
	}
	if removed > 0 {
		log.Debug("清理过期微信媒体引用完成",
			zap.Int("removed_count", removed),
			zap.String("result", "succeeded"),
		)
	}
}

func (impl *serviceImpl) persistConnectionState(connection *connectordomain.Connection) {
	if connection == nil || strings.TrimSpace(connection.Token) == "" {
		return
	}
	impl.mu.Lock()
	current := impl.connections[connection.Token]
	if current == nil {
		impl.mu.Unlock()
		return
	}
	bot := impl.bots[current.BotAccountID]
	if bot == nil {
		impl.mu.Unlock()
		return
	}
	current.GetUpdatesBuf = connection.GetUpdatesBuf
	current.ContextTokens = connection.ContextTokens
	current.LastInboundAt = connection.LastInboundAt
	bot.GetUpdatesBuf = connection.GetUpdatesBuf
	bot.ContextTokens = map[string]string{}
	mergeContextTokens(bot.ContextTokens, connection.ContextTokens)
	bot.LastInboundAt = connection.LastInboundAt
	botSnapshot := *bot
	if bot.ContextTokens != nil {
		botSnapshot.ContextTokens = map[string]string{}
		mergeContextTokens(botSnapshot.ContextTokens, bot.ContextTokens)
	}
	for _, item := range impl.connections {
		if item == nil || item.BotAccountID != bot.BotAccountID {
			continue
		}
		item.GetUpdatesBuf = bot.GetUpdatesBuf
		item.ContextTokens = bot.ContextTokens
		item.LastInboundAt = bot.LastInboundAt
	}
	impl.mu.Unlock()
	if err := impl.storage.SaveBot(&botSnapshot); err != nil {
		log.Debug("保存微信入站游标失败",
			zap.String("connector_channel_id", connection.Token),
			zap.String("result", "failed"),
			zap.Error(err),
		)
	}
}

func (impl *serviceImpl) removeConnectionMonitor(connectorChannelID string) {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if impl.monitors != nil {
		delete(impl.monitors, connectorChannelID)
	}
	log.Debug("微信入站消息监听已停止",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("result", "stopped"),
	)
}

func (impl *serviceImpl) sleepAfterGetUpdatesFailure(ctx context.Context, consecutiveFailures int) {
	delay := getUpdatesRetryDelay
	if consecutiveFailures >= maxConsecutiveGetUpdatesFailures {
		delay = getUpdatesFailureBackoff
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func isGetUpdatesAPIError(resp *ilinkservice.GetUpdatesResponse) bool {
	if resp == nil {
		return false
	}
	return (resp.Ret != 0) || (resp.ErrCode != 0)
}

func (impl *serviceImpl) buildInboundMessagePayload(ctx context.Context, connection *connectordomain.Connection, message *ilinkservice.WeixinMessage) map[string]any {
	payload := buildInboundMessagePayload(connection, message)
	mediaRefs := impl.mediaRefsFromMessage(ctx, connection, message)
	if len(mediaRefs) > 0 {
		payload["media"] = mediaRefs
	}
	return payload
}

func buildInboundMessagePayload(connection *connectordomain.Connection, message *ilinkservice.WeixinMessage) map[string]any {
	rawText := textFromMessage(message)
	senderName := weChatInboundSenderName(message)
	text := buildWeChatInboundVisibleText(message, rawText, senderName)
	messageID := fmt.Sprint(message.MessageID)
	if messageID == "0" {
		messageID = strings.TrimSpace(message.ClientID)
	}
	payload := map[string]any{
		"provider":       "wechat",
		"profile":        "xagent.im.v1",
		"event_kind":     "im.message.received",
		"message_id":     messageID,
		"from":           message.FromUserID,
		"sender_id":      message.FromUserID,
		"sender_name":    senderName,
		"display_name":   senderName,
		"to":             message.ToUserID,
		"text":           text,
		"content":        text,
		"raw_text":       rawText,
		"context_token":  message.ContextToken,
		"create_time_ms": message.CreateTimeMillis,
		"message_type":   message.MessageType,
		"message_state":  message.MessageState,
		"session_id":     message.SessionID,
		"group_id":       message.GroupID,
		"raw_message":    message,
		"reply": map[string]any{
			"required": true,
			"tool_id":  toolIDWeChatMessageSend,
		},
		"skill": map[string]any{
			"skill_id":          protocol.ConnectorSkillIMReplyID,
			"required_tool_ids": []string{toolIDWeChatMessageSend},
		},
		"activation_message": text,
	}
	if connection != nil {
		payload["account_hint"] = connection.AccountHint
	}
	return payload
}

// buildWeChatInboundVisibleText 添加微信目标系统内部来源。
//
// WHY：Connector 内部最清楚外部系统里的发信人、群和 bot/account 语义；xAgent 的
// connectManager 只会在外层再补 connector/card 来源，不应回头解析微信字段来猜 sender。
func buildWeChatInboundVisibleText(message *ilinkservice.WeixinMessage, rawText string, senderName string) string {
	rawText = strings.TrimSpace(rawText)
	messageType := weChatInboundMessageType(message)
	userText := rawText
	if userText == "" {
		userText = "无"
	}
	var builder strings.Builder
	builder.WriteString("来自微信的用户消息：\n")
	if senderName != "" {
		builder.WriteString("发送方：")
		builder.WriteString(senderName)
		builder.WriteString("\n")
	}
	builder.WriteString("消息类型：")
	builder.WriteString(messageType)
	builder.WriteString("\n")
	builder.WriteString("用户文本：")
	builder.WriteString(userText)
	builder.WriteString("\n\n")
	builder.WriteString("先处理微信消息，然后使用 ")
	builder.WriteString(toolIDWeChatMessageSend)
	builder.WriteString(" 工具将处理结果回复给我，参数 text 是回复内容。")
	return builder.String()
}

func pendingInboundMessageID(connectorChannelID string, message *ilinkservice.WeixinMessage) string {
	parts := []string{sanitizePendingIDPart(connectorChannelID)}
	if message != nil {
		if message.MessageID != 0 {
			parts = append(parts, fmt.Sprintf("mid_%d", message.MessageID))
		}
		if message.Seq != 0 {
			parts = append(parts, fmt.Sprintf("seq_%d", message.Seq))
		}
		if strings.TrimSpace(message.ClientID) != "" {
			parts = append(parts, sanitizePendingIDPart(message.ClientID))
		}
	}
	if len(parts) == 1 {
		parts = append(parts, fmt.Sprintf("ts_%d", time.Now().UnixNano()))
	}
	return strings.Join(parts, "_")
}

func sanitizePendingIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' {
			builder.WriteRune(char)
			continue
		}
		builder.WriteByte('_')
	}
	return builder.String()
}

const inboundVoiceNoTranscriptText = "收到一条微信语音消息，但微信 iLink 未返回语音转文字内容。"

func weChatInboundSenderName(message *ilinkservice.WeixinMessage) string {
	if message == nil {
		return ""
	}
	if displayName := strings.TrimSpace(message.DisplayName); displayName != "" {
		return displayName
	}
	return ""
}

func weChatInboundMessageType(message *ilinkservice.WeixinMessage) string {
	if message == nil {
		return "未知"
	}
	hasText := false
	types := []string{}
	for _, item := range message.ItemList {
		if item == nil {
			continue
		}
		switch item.Type {
		case ilinkservice.MessageItemTypeText:
			hasText = true
		case ilinkservice.MessageItemTypeImage:
			types = append(types, "图片")
		case ilinkservice.MessageItemTypeVoice:
			types = append(types, "语音")
		case ilinkservice.MessageItemTypeFile:
			types = append(types, "文件")
		case ilinkservice.MessageItemTypeVideo:
			types = append(types, "视频")
		}
	}
	if len(types) == 0 {
		if hasText {
			return "文本"
		}
		return "未知"
	}
	if hasText {
		types = append([]string{"文本"}, types...)
	}
	return strings.Join(types, "、")
}

func textFromMessage(message *ilinkservice.WeixinMessage) string {
	if message == nil {
		return ""
	}
	for _, item := range message.ItemList {
		if item == nil {
			continue
		}
		if item.Type == ilinkservice.MessageItemTypeText && item.TextItem != nil {
			text := strings.TrimSpace(item.TextItem.Text)
			if text == "" {
				continue
			}
			if item.RefMessage == nil {
				return text
			}
			refText := textFromMessage(&ilinkservice.WeixinMessage{ItemList: []*ilinkservice.MessageItem{item.RefMessage.MessageItem}})
			if item.RefMessage.Title != "" {
				refText = strings.TrimSpace(item.RefMessage.Title + " " + refText)
			}
			if refText == "" {
				return text
			}
			return "[引用: " + refText + "]\n" + text
		}
		if item.Type == ilinkservice.MessageItemTypeVoice && item.VoiceItem != nil {
			if text := strings.TrimSpace(item.VoiceItem.Text); text != "" {
				return text
			}
			return inboundVoiceNoTranscriptText
		}
	}
	return ""
}

func (impl *serviceImpl) mediaRefsFromMessage(ctx context.Context, connection *connectordomain.Connection, message *ilinkservice.WeixinMessage) []map[string]any {
	if connection == nil || message == nil {
		return nil
	}
	refs := impl.mediaRefsFromMessageItems(ctx, connection, message, message.ItemList)
	if len(refs) > 0 {
		return refs
	}
	quotedItems := make([]*ilinkservice.MessageItem, 0, len(message.ItemList))
	for _, item := range message.ItemList {
		if item == nil || item.RefMessage == nil || item.RefMessage.MessageItem == nil {
			continue
		}
		quotedItems = append(quotedItems, item.RefMessage.MessageItem)
	}
	return impl.mediaRefsFromMessageItems(ctx, connection, message, quotedItems)
}

func (impl *serviceImpl) mediaRefsFromMessageItems(ctx context.Context, connection *connectordomain.Connection, message *ilinkservice.WeixinMessage, items []*ilinkservice.MessageItem) []map[string]any {
	refs := []map[string]any{}
	for _, item := range items {
		if item == nil {
			continue
		}
		switch item.Type {
		case ilinkservice.MessageItemTypeImage:
			if item.ImageItem != nil {
				if ref := impl.registerInboundMedia(ctx, connection, message, "image", inboundImageMedia(item.ImageItem)); ref != nil {
					refs = append(refs, ref)
				}
			}
		case ilinkservice.MessageItemTypeFile:
			if item.FileItem != nil {
				if ref := impl.registerInboundMedia(ctx, connection, message, "file", inboundFileMedia(item.FileItem)); ref != nil {
					refs = append(refs, ref)
				}
			}
		case ilinkservice.MessageItemTypeVideo:
			if item.VideoItem != nil {
				if ref := impl.registerInboundMedia(ctx, connection, message, "video", inboundVideoMedia(item.VideoItem)); ref != nil {
					refs = append(refs, ref)
				}
			}
		}
	}
	return refs
}

type inboundMediaMaterial struct {
	filename      string
	contentType   string
	rawSize       int64
	rawMD5        string
	cipherSize    int64
	downloadParam string
	fullURL       string
	aesKeyBase64  string
}

func (impl *serviceImpl) registerInboundMedia(ctx context.Context, connection *connectordomain.Connection, message *ilinkservice.WeixinMessage, mediaType string, material inboundMediaMaterial) map[string]any {
	if material.downloadParam == "" && material.fullURL == "" {
		return nil
	}
	reference, err := impl.media.RegisterInboundMedia(ctx, mediaservice.RegisterInboundMediaInput{
		ConnectorChannelID: connection.Token,
		PeerRef:            message.FromUserID,
		WeChatMessageID:    fmt.Sprint(message.MessageID),
		MediaType:          mediaType,
		Filename:           material.filename,
		ContentType:        material.contentType,
		RawSize:            material.rawSize,
		RawMD5:             material.rawMD5,
		CipherSize:         material.cipherSize,
		DownloadParam:      material.downloadParam,
		FullURL:            material.fullURL,
		AESKeyBase64:       material.aesKeyBase64,
	})
	if err != nil {
		log.Debug("注册微信入站媒体引用失败",
			zap.String("connector_channel_id", connection.Token),
			zap.String("from_user_id", message.FromUserID),
			zap.String("media_type", mediaType),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return nil
	}
	return map[string]any{
		"type":         reference.MediaType,
		"media_ref":    reference.Ref,
		"download_url": mediaDownloadURI(reference.Ref),
		"filename":     reference.Filename,
		"mime_type":    reference.ContentType,
		"byte_size":    reference.RawSize,
		"expires_at":   reference.ExpiresAt,
	}
}

func inboundImageMedia(item *ilinkservice.ImageItem) inboundMediaMaterial {
	media := preferredCDNMedia(item.Media, item.ThumbMedia)
	return inboundMediaMaterial{
		filename:      "wechat-image.jpg",
		contentType:   "image/jpeg",
		cipherSize:    firstPositiveInt64(item.MidSize, item.HDSize, item.ThumbSize),
		downloadParam: strings.TrimSpace(media.EncryptQueryParam),
		fullURL:       strings.TrimSpace(media.FullURL),
		aesKeyBase64:  inboundImageAESKey(item, media),
	}
}

func inboundImageAESKey(item *ilinkservice.ImageItem, media *ilinkservice.CDNMedia) string {
	if item != nil {
		if rawHexKey := strings.TrimSpace(item.AESKey); rawHexKey != "" {
			if rawKey, err := hex.DecodeString(rawHexKey); err == nil {
				return base64.StdEncoding.EncodeToString(rawKey)
			}
			return rawHexKey
		}
	}
	if media == nil {
		return ""
	}
	return strings.TrimSpace(media.AESKey)
}

func inboundFileMedia(item *ilinkservice.FileItem) inboundMediaMaterial {
	media := preferredCDNMedia(item.Media, nil)
	return inboundMediaMaterial{
		filename:      strings.TrimSpace(item.FileName),
		contentType:   "application/octet-stream",
		rawSize:       parseInt64(item.Len),
		rawMD5:        strings.TrimSpace(item.MD5),
		downloadParam: strings.TrimSpace(media.EncryptQueryParam),
		fullURL:       strings.TrimSpace(media.FullURL),
		aesKeyBase64:  strings.TrimSpace(media.AESKey),
	}
}

func inboundVideoMedia(item *ilinkservice.VideoItem) inboundMediaMaterial {
	media := preferredCDNMedia(item.Media, item.ThumbMedia)
	return inboundMediaMaterial{
		filename:      "wechat-video.mp4",
		contentType:   "video/mp4",
		rawMD5:        strings.TrimSpace(item.VideoMD5),
		cipherSize:    item.VideoSize,
		downloadParam: strings.TrimSpace(media.EncryptQueryParam),
		fullURL:       strings.TrimSpace(media.FullURL),
		aesKeyBase64:  strings.TrimSpace(media.AESKey),
	}
}

func preferredCDNMedia(primary *ilinkservice.CDNMedia, fallback *ilinkservice.CDNMedia) *ilinkservice.CDNMedia {
	if primary != nil && (strings.TrimSpace(primary.EncryptQueryParam) != "" || strings.TrimSpace(primary.FullURL) != "") {
		return primary
	}
	if fallback != nil {
		return fallback
	}
	return &ilinkservice.CDNMedia{}
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func parseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}

func mediaDownloadURI(mediaRef string) string {
	return connectorprotocol.ConnectorMediaRefPathPrefix + "/" + mediaRef
}

func cloneConnection(connection *connectordomain.Connection) *connectordomain.Connection {
	if connection == nil {
		return nil
	}
	cloned := *connection
	if connection.ContextTokens != nil {
		cloned.ContextTokens = make(map[string]string, len(connection.ContextTokens))
		for key, value := range connection.ContextTokens {
			cloned.ContextTokens[key] = value
		}
	}
	return &cloned
}
