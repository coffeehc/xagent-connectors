package connectservice

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coffeehc/base/log"
	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/telegramservice"
	"go.uber.org/zap"
)

func (impl *serviceImpl) startBotMonitorLocked(bot *connectordomain.Bot) {
	if bot == nil || strings.TrimSpace(bot.BotID) == "" || strings.TrimSpace(bot.BotToken) == "" {
		return
	}
	impl.ensureRuntimeStateLocked()
	if impl.monitors[bot.BotID] != nil {
		return
	}
	parent := impl.stopCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	impl.monitors[bot.BotID] = cancel
	snapshot := cloneBot(bot)
	go impl.monitorBot(ctx, snapshot)
	log.Debug("Telegram 入站消息监听已启动",
		zap.String("bot_id", bot.BotID),
		zap.Int64("update_offset", bot.UpdateOffset),
		zap.Int("bound_channel_count", impl.boundChannelCountLocked(bot.BotID)),
		zap.String("result", "started"),
	)
}

func (impl *serviceImpl) restartBotMonitorLocked(bot *connectordomain.Bot) {
	if bot == nil {
		return
	}
	impl.stopBotMonitorLocked(bot.BotID)
	if impl.botHasBoundChannelLocked(bot.BotID) {
		impl.startBotMonitorLocked(bot)
	}
}

func (impl *serviceImpl) stopBotMonitorLocked(botID string) {
	botID = strings.TrimSpace(botID)
	if botID == "" || impl.monitors == nil {
		return
	}
	cancel := impl.monitors[botID]
	delete(impl.monitors, botID)
	if cancel != nil {
		cancel()
	}
}

func (impl *serviceImpl) botHasBoundChannelLocked(botID string) bool {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return false
	}
	for _, channel := range impl.channels {
		if channel != nil && channel.BotID == botID {
			return true
		}
	}
	return false
}

func (impl *serviceImpl) boundChannelCountLocked(botID string) int {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return 0
	}
	count := 0
	for _, channel := range impl.channels {
		if channel != nil && channel.BotID == botID {
			count++
		}
	}
	return count
}

func (impl *serviceImpl) monitorBot(ctx context.Context, bot *connectordomain.Bot) {
	defer impl.removeBotMonitor(bot.BotID)
	consecutiveFailures := 0
	offset := bot.UpdateOffset
	for ctx.Err() == nil {
		result, err := impl.telegram.GetUpdates(ctx, telegramservice.GetUpdatesInput{
			BotToken:       bot.BotToken,
			Offset:         offset,
			TimeoutSeconds: protocol.TelegramLongPollTimeoutSeconds,
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			consecutiveFailures++
			log.Debug("拉取 Telegram 入站消息失败",
				zap.String("bot_id", bot.BotID),
				zap.Int("consecutive_failure_count", consecutiveFailures),
				zap.String("result", "failed"),
				zap.Error(err),
			)
			impl.sleepAfterGetUpdatesFailure(ctx, consecutiveFailures)
			continue
		}
		consecutiveFailures = 0
		if result == nil {
			log.Debug("Telegram getUpdates 返回空响应",
				zap.String("bot_id", bot.BotID),
				zap.Int64("offset", offset),
				zap.String("result", "empty_response"),
			)
			continue
		}
		if len(result.Updates) > 0 {
			log.Debug("Telegram getUpdates 收到入站更新",
				zap.String("bot_id", bot.BotID),
				zap.Int("update_count", len(result.Updates)),
				zap.Int64("offset", offset),
				zap.Int64("next_offset", result.NextOffset),
				zap.String("result", "received"),
			)
		}
		for _, update := range result.Updates {
			impl.dispatchTelegramUpdate(ctx, bot.BotID, update)
		}
		if result.NextOffset > offset {
			offset = result.NextOffset
			impl.saveBotOffset(bot.BotID, offset)
		}
	}
}

func (impl *serviceImpl) dispatchTelegramUpdate(ctx context.Context, botID string, update *telegramservice.Update) {
	if update == nil || update.Message == nil || update.Message.Chat == nil {
		updateID := int64(0)
		if update != nil {
			updateID = update.UpdateID
		}
		log.Debug("忽略 Telegram update：缺少 message chat",
			zap.String("bot_id", botID),
			zap.Int64("update_id", updateID),
			zap.String("result", "ignored"),
		)
		return
	}
	chatID := strconv.FormatInt(update.Message.Chat.ID, 10)
	connection := impl.connectionByBotChat(botID, chatID)
	if connection == nil {
		log.Debug("忽略 Telegram 入站消息：chat 未绑定",
			zap.String("bot_id", botID),
			zap.String("chat_id", chatID),
			zap.String("chat_type", update.Message.Chat.Type),
			zap.String("chat_title", telegramChatDisplayName(update.Message.Chat)),
			zap.Int64("update_id", update.UpdateID),
			zap.String("result", "unbound_chat"),
		)
		return
	}
	text := telegramMessageText(update.Message)
	mediaRefs := impl.mediaRefsFromMessage(connection, update.Message)
	if text == "" && len(mediaRefs) == 0 {
		log.Debug("忽略 Telegram 入站消息：当前仅支持文本、图片、视频和文件",
			zap.String("connector_channel_id", connection.Token),
			zap.String("bot_id", botID),
			zap.String("chat_id", chatID),
			zap.Int64("update_id", update.UpdateID),
			zap.String("result", "non_text_message"),
		)
		return
	}
	senderName := telegramUserDisplayName(update.Message.From)
	visibleText := buildTelegramInboundVisibleText(text, telegramInboundMessageType(update.Message), senderName)
	payload := map[string]any{
		"provider":           "telegram",
		"profile":            "xagent.im.v1",
		"event_kind":         "im.message.received",
		"message_id":         fmt.Sprintf("%d", update.Message.MessageID),
		"chat_id":            chatID,
		"chat_type":          connection.ChatType,
		"text":               visibleText,
		"content":            visibleText,
		"raw_text":           text,
		"sender_id":          telegramSenderID(update.Message.From),
		"sender_name":        senderName,
		"display_name":       senderName,
		"received_at":        time.Now().UnixMilli(),
		"create_time_ms":     update.Message.Date * 1000,
		"reply_token":        chatID,
		"connector":          protocol.ConnectorCardID,
		"activation_message": visibleText,
		"reply": map[string]any{
			"required": true,
			"tool_id":  toolIDTelegramMessageSend,
		},
		"skill": map[string]any{
			"skill_id":          protocol.ConnectorSkillIMReplyID,
			"required_tool_ids": []string{toolIDTelegramMessageSend, toolIDTelegramMessageSendMedia},
		},
	}
	if len(mediaRefs) > 0 {
		payload["media"] = mediaRefs
	}
	if connection != nil {
		payload["account_hint"] = connection.AccountHint
	}
	impl.mu.Lock()
	pusher := impl.pusher
	impl.mu.Unlock()
	if pusher == nil {
		log.Debug("推送 Telegram 入站消息失败：data plane pusher 未绑定",
			zap.String("connector_channel_id", connection.Token),
			zap.String("bot_id", botID),
			zap.String("chat_id", chatID),
			zap.String("result", "pusher_not_bound"),
		)
		return
	}
	if err := pusher.PushMessage(ctx, connection.Token, payload); err != nil {
		log.Debug("推送 Telegram 入站消息失败",
			zap.String("connector_channel_id", connection.Token),
			zap.String("bot_id", botID),
			zap.String("chat_id", chatID),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return
	}
	impl.markBotInbound(botID, time.Now().UnixMilli())
	log.Debug("推送 Telegram 入站消息完成",
		zap.String("connector_channel_id", connection.Token),
		zap.String("bot_id", botID),
		zap.String("chat_id", chatID),
		zap.String("message_id", fmt.Sprintf("%d", update.Message.MessageID)),
		zap.Int("text_length", len(text)),
		zap.String("result", "delivered"),
	)
}

func (impl *serviceImpl) mediaRefsFromMessage(connection *connectordomain.Connection, message *telegramservice.Message) []map[string]any {
	if connection == nil || message == nil {
		return nil
	}
	now := time.Now().UnixMilli()
	refs := []map[string]any{}
	if len(message.Photo) > 0 {
		photo := largestPhoto(message.Photo)
		if photo != nil {
			refs = append(refs, impl.registerInboundMedia(connection, now, connectorMediaTypeImage, photo.FileID, "", "", photo.FileSize))
		}
	}
	if message.Document != nil {
		refs = append(refs, impl.registerInboundMedia(connection, now, connectorMediaTypeFile, message.Document.FileID, message.Document.FileName, message.Document.MimeType, message.Document.FileSize))
	}
	if message.Video != nil {
		refs = append(refs, impl.registerInboundMedia(connection, now, connectorMediaTypeVideo, message.Video.FileID, message.Video.FileName, message.Video.MimeType, message.Video.FileSize))
	}
	return refs
}

func (impl *serviceImpl) registerInboundMedia(connection *connectordomain.Connection, nowMillis int64, mediaType string, fileID string, filename string, contentType string, byteSize int64) map[string]any {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil
	}
	mediaRef := "tgmedia_" + randomToken(12)
	if strings.TrimSpace(filename) == "" {
		filename = mediaRef + defaultMediaExtension(mediaType)
	}
	reference := &connectordomain.MediaReference{
		Ref:                mediaRef,
		Direction:          "inbound",
		ConnectorChannelID: connection.Token,
		BotID:              connection.BotID,
		BotToken:           connection.BotToken,
		ChatID:             connection.ChatID,
		FileID:             fileID,
		MediaType:          mediaType,
		Filename:           filename,
		ContentType:        strings.TrimSpace(contentType),
		ByteSize:           byteSize,
		CreatedAt:          nowMillis,
		ExpiresAt:          nowMillis + protocol.DefaultMediaReferenceTTLMillis,
	}
	if err := impl.storage.SaveMediaReference(reference); err != nil {
		log.Debug("注册 Telegram 入站媒体失败",
			zap.String("connector_channel_id", connection.Token),
			zap.String("media_type", mediaType),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return nil
	}
	return map[string]any{
		"media_ref":    reference.Ref,
		"download_url": mediaDownloadURI(reference.Ref),
		"media_type":   reference.MediaType,
		"filename":     reference.Filename,
		"content_type": reference.ContentType,
		"byte_size":    reference.ByteSize,
		"expires_at":   reference.ExpiresAt,
	}
}

func largestPhoto(items []*telegramservice.PhotoSize) *telegramservice.PhotoSize {
	var selected *telegramservice.PhotoSize
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.FileID) == "" {
			continue
		}
		if selected == nil || item.FileSize > selected.FileSize || item.Width*item.Height > selected.Width*selected.Height {
			selected = item
		}
	}
	return selected
}

func telegramInboundMessageType(message *telegramservice.Message) string {
	if message == nil {
		return "未知"
	}
	types := []string{}
	if telegramMessageText(message) != "" {
		types = append(types, "文本")
	}
	if len(message.Photo) > 0 {
		types = append(types, "图片")
	}
	if message.Document != nil {
		types = append(types, "文件")
	}
	if message.Video != nil {
		types = append(types, "视频")
	}
	if len(types) == 0 {
		return "未知"
	}
	return strings.Join(types, "+")
}

// buildTelegramInboundVisibleText 添加 Telegram 目标系统内部来源。
//
// WHY：Connector 内部最清楚 Telegram 发信人和 chat 语义；xAgent 的连接层只会在外层再补
// connector/card 来源，不应回头解析 Telegram 字段来猜 sender。
func buildTelegramInboundVisibleText(rawText string, messageType string, senderName string) string {
	rawText = strings.TrimSpace(rawText)
	userText := rawText
	if userText == "" {
		userText = "无"
	}
	var builder strings.Builder
	builder.WriteString("来自 Telegram 的用户消息：\n")
	if senderName != "" {
		builder.WriteString("发送方：")
		builder.WriteString(senderName)
		builder.WriteString("\n")
	}
	if strings.TrimSpace(messageType) == "" {
		messageType = "文本"
	}
	builder.WriteString("消息类型：")
	builder.WriteString(messageType)
	builder.WriteString("\n")
	builder.WriteString("用户文本：")
	builder.WriteString(userText)
	builder.WriteString("\n\n")
	builder.WriteString("先处理 Telegram 消息。文本回复使用 ")
	builder.WriteString(toolIDTelegramMessageSend)
	builder.WriteString(" 工具，参数 text 是回复内容；如果需要回复图片、视频或文件，先上传文件取得 media_ref，再使用 ")
	builder.WriteString(toolIDTelegramMessageSendMedia)
	builder.WriteString(" 工具发送。")
	return builder.String()
}

func defaultMediaExtension(mediaType string) string {
	switch mediaType {
	case connectorMediaTypeImage:
		return ".jpg"
	case connectorMediaTypeVideo:
		return ".mp4"
	default:
		return ".bin"
	}
}

func mediaDownloadURI(mediaRef string) string {
	return connectorprotocol.ConnectorMediaRefPathPrefix + "/" + mediaRef
}

func (impl *serviceImpl) connectionByBotChat(botID string, chatID string) *connectordomain.Connection {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	for _, connection := range impl.connections {
		if connection != nil && connection.BotID == botID && connection.ChatID == chatID {
			return cloneConnection(connection)
		}
	}
	return nil
}

func (impl *serviceImpl) saveBotOffset(botID string, offset int64) {
	impl.mu.Lock()
	bot := cloneBot(impl.bots[botID])
	if bot != nil {
		bot.UpdateOffset = offset
		impl.bots[botID] = cloneBot(bot)
	}
	impl.mu.Unlock()
	if bot != nil {
		if err := impl.storage.SaveBot(bot); err != nil {
			log.Debug("保存 Telegram update offset 失败",
				zap.String("bot_id", botID),
				zap.Int64("offset", offset),
				zap.String("result", "failed"),
				zap.Error(err),
			)
		}
	}
}

func (impl *serviceImpl) markBotInbound(botID string, inboundAt int64) {
	impl.mu.Lock()
	bot := cloneBot(impl.bots[botID])
	if bot != nil {
		bot.LastInboundAt = inboundAt
		impl.bots[botID] = cloneBot(bot)
	}
	impl.mu.Unlock()
	if bot != nil {
		_ = impl.storage.SaveBot(bot)
	}
}

func (impl *serviceImpl) removeBotMonitor(botID string) {
	impl.mu.Lock()
	if impl.monitors != nil {
		delete(impl.monitors, strings.TrimSpace(botID))
	}
	impl.mu.Unlock()
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

func telegramSenderID(user *telegramservice.User) string {
	if user == nil {
		return ""
	}
	return strconv.FormatInt(user.ID, 10)
}

func telegramMessageText(message *telegramservice.Message) string {
	if message == nil {
		return ""
	}
	if text := strings.TrimSpace(message.Text); text != "" {
		return text
	}
	return strings.TrimSpace(message.Caption)
}
