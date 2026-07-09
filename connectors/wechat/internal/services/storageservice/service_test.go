package storageservice

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
)

func TestServicePersistsBotsAndChannelsAcrossRestart(t *testing.T) {
	stateDir := t.TempDir()
	storage := newService(stateDir, protocol.ConnectorCardID)
	now := time.Now().UnixMilli()
	bot := &connectordomain.Bot{
		WeChatUserID:  "wx-user-a",
		BotToken:      "bot-token-a",
		BotAccountID:  "0069-test-bot",
		BaseURL:       protocol.WeChatAPIBaseURL,
		DisplayName:   "微信 0069***.bot",
		AccountHint:   "0069***.bot",
		CreatedAt:     now,
		ContextTokens: map[string]string{"wx-user-a": "ctx-1"},
	}
	channel := &connectordomain.ConnectionBinding{
		ConnectorChannelID: "cch_user_a",
		BotAccountID:       bot.BotAccountID,
		CreatedAt:          now,
	}
	if err := storage.SaveBots(map[string]*connectordomain.Bot{bot.BotAccountID: bot}); err != nil {
		t.Fatalf("SaveBots returned error: %v", err)
	}
	if err := storage.SaveConnectionBindings(map[string]*connectordomain.ConnectionBinding{channel.ConnectorChannelID: channel}); err != nil {
		t.Fatalf("SaveConnectionBindings returned error: %v", err)
	}

	restarted := newService(stateDir, protocol.ConnectorCardID)
	bots, err := restarted.LoadBots()
	if err != nil {
		t.Fatalf("LoadBots returned error: %v", err)
	}
	channels, err := restarted.LoadConnectionBindings()
	if err != nil {
		t.Fatalf("LoadConnectionBindings returned error: %v", err)
	}
	restoredBot := bots[bot.BotAccountID]
	if restoredBot == nil || restoredBot.BotToken != bot.BotToken || restoredBot.ContextTokens["wx-user-a"] != "ctx-1" {
		t.Fatalf("重启后微信 bot 登录态不完整，got=%+v", restoredBot)
	}
	restoredChannel := channels[channel.ConnectorChannelID]
	if restoredChannel == nil || restoredChannel.BotAccountID != bot.BotAccountID {
		t.Fatalf("重启后 connection 绑定异常，got=%+v", restoredChannel)
	}
}

func TestPendingInboundMessagesPersistAndAdvanceCursor(t *testing.T) {
	stateDir := t.TempDir()
	storage := newService(stateDir, protocol.ConnectorCardID)
	channel := &connectordomain.ConnectionBinding{
		ConnectorChannelID: "cch_user_a",
		BotAccountID:       "0069-test-bot",
		CreatedAt:          50,
	}
	if err := storage.SaveConnectionBindings(map[string]*connectordomain.ConnectionBinding{channel.ConnectorChannelID: channel}); err != nil {
		t.Fatalf("SaveConnectionBindings returned error: %v", err)
	}
	message := &connectordomain.PendingInboundMessage{
		ID:                 "cch_user_a_mid_1",
		ConnectorChannelID: "cch_user_a",
		WeChatMessageID:    "1",
		Payload:            map[string]any{"text": "hello"},
		ReceivedAt:         100,
		ExpiresAt:          1000,
		CreatedAt:          100,
		UpdatedAt:          100,
	}
	if err := storage.EnqueuePendingInboundMessage(message, 100); err != nil {
		t.Fatalf("EnqueuePendingInboundMessage returned error: %v", err)
	}

	restarted := newService(stateDir, protocol.ConnectorCardID)
	messages, err := restarted.ListPendingInboundMessages("cch_user_a", 200, 0)
	if err != nil {
		t.Fatalf("ListPendingInboundMessages returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != message.ID {
		t.Fatalf("重启后 pending 消息恢复异常: %+v", messages)
	}
	if err = restarted.MarkPendingInboundDelivered("cch_user_a", message.ID, 300); err != nil {
		t.Fatalf("MarkPendingInboundDelivered returned error: %v", err)
	}
	cursor, err := restarted.LoadInboundChannelCursor("cch_user_a")
	if err != nil {
		t.Fatalf("LoadInboundChannelCursor returned error: %v", err)
	}
	if cursor.LastDeliveredMessageID != message.ID || cursor.LastDeliveredAt != 300 {
		t.Fatalf("投递游标异常: %+v", cursor)
	}
	channels, err := restarted.LoadConnectionBindings()
	if err != nil {
		t.Fatalf("LoadConnectionBindings returned error after delivered: %v", err)
	}
	if channels["cch_user_a"].LastDeliveredMessageID != message.ID || channels["cch_user_a"].LastDeliveredAt != 300 {
		t.Fatalf("channel 文件中的投递游标异常: %+v", channels["cch_user_a"])
	}
	rawChannels, err := os.ReadFile(filepath.Join(stateDir, protocol.ChannelStateFilename))
	if err != nil {
		t.Fatalf("read channels.json failed: %v", err)
	}
	channelState := map[string]map[string]any{}
	if err = json.Unmarshal(rawChannels, &channelState); err != nil {
		t.Fatalf("decode channels.json failed: %v", err)
	}
	channelRecord := channelState["cch_user_a"]
	if _, exists := channelRecord["connector_channel_id"]; exists {
		t.Fatalf("channels.json 不应在 value 内重复 connector_channel_id: %s", string(rawChannels))
	}
	if channelRecord["last_delivered_message_id"] != message.ID || channelRecord["last_delivered_at"] != float64(300) {
		t.Fatalf("channels.json 应保存投递游标: %s", string(rawChannels))
	}
	messages, err = restarted.ListPendingInboundMessages("cch_user_a", 400, 0)
	if err != nil {
		t.Fatalf("ListPendingInboundMessages returned error after delivered: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("投递成功后 pending 消息应删除: %+v", messages)
	}
}

func TestMediaReferencePersistsAndPrunesExpired(t *testing.T) {
	stateDir := t.TempDir()
	storage := newService(stateDir, protocol.ConnectorCardID)
	now := int64(1000)
	media := &connectordomain.MediaReference{
		Ref:                "media_ref_1",
		Direction:          "outbound",
		ConnectorChannelID: "cch_user_a",
		PeerRef:            "wx_user_2",
		MediaType:          "video",
		ILinkMediaType:     2,
		Filename:           "done.mp4",
		RawSize:            11,
		RawMD5:             "raw-md5",
		CipherSize:         16,
		DownloadParam:      "download-param-1",
		AESKeyBase64:       "base64-key",
		CreatedAt:          now,
		ExpiresAt:          now + 100,
	}
	cachePath := filepath.Join(stateDir, protocol.MediaCacheDirname, "media_ref_1")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		t.Fatalf("create media cache dir failed: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("cached-media"), 0o600); err != nil {
		t.Fatalf("write media cache failed: %v", err)
	}
	media.LocalPath = cachePath
	if err := storage.SaveMediaReference(media); err != nil {
		t.Fatalf("SaveMediaReference returned error: %v", err)
	}
	restarted := newService(stateDir, protocol.ConnectorCardID)
	loaded, err := restarted.GetMediaReference("media_ref_1", now+50)
	if err != nil {
		t.Fatalf("GetMediaReference returned error: %v", err)
	}
	if loaded == nil || loaded.Ref != media.Ref || loaded.DownloadParam != media.DownloadParam {
		t.Fatalf("重启后媒体引用读取异常: %+v", loaded)
	}
	removed, err := restarted.PruneExpiredMediaReferences(now + 101)
	if err != nil {
		t.Fatalf("PruneExpiredMediaReferences returned error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("应清理 1 条过期媒体引用，got=%d", removed)
	}
	loaded, err = restarted.GetMediaReference("media_ref_1", now+101)
	if err != nil {
		t.Fatalf("GetMediaReference after prune returned error: %v", err)
	}
	if loaded != nil {
		t.Fatalf("过期媒体引用应已清理，got=%+v", loaded)
	}
	if _, err = os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("过期媒体引用应同步删除本地缓存，stat err=%v", err)
	}
}

func TestPruneExpiredPendingInboundMessages(t *testing.T) {
	storage := newService(t.TempDir(), protocol.ConnectorCardID)
	for _, message := range []*connectordomain.PendingInboundMessage{
		{
			ID:                 "expired",
			ConnectorChannelID: "cch_user_a",
			Payload:            map[string]any{"text": "old"},
			ReceivedAt:         100,
			ExpiresAt:          250,
			CreatedAt:          100,
			UpdatedAt:          200,
		},
		{
			ID:                 "before_cursor",
			ConnectorChannelID: "cch_user_a",
			Payload:            map[string]any{"text": "delivered"},
			ReceivedAt:         140,
			ExpiresAt:          1000,
			CreatedAt:          140,
			UpdatedAt:          140,
		},
		{
			ID:                 "cursor",
			ConnectorChannelID: "cch_user_a",
			Payload:            map[string]any{"text": "cursor"},
			ReceivedAt:         150,
			ExpiresAt:          1000,
			CreatedAt:          150,
			UpdatedAt:          150,
		},
		{
			ID:                 "live",
			ConnectorChannelID: "cch_user_a",
			Payload:            map[string]any{"text": "new"},
			ReceivedAt:         200,
			ExpiresAt:          1000,
			CreatedAt:          200,
			UpdatedAt:          200,
		},
	} {
		if err := storage.EnqueuePendingInboundMessage(message, 100); err != nil {
			t.Fatalf("EnqueuePendingInboundMessage returned error: %v", err)
		}
	}
	if err := storage.SaveConnectionBindings(map[string]*connectordomain.ConnectionBinding{
		"cch_user_a": {
			ConnectorChannelID:     "cch_user_a",
			BotAccountID:           "0069-test-bot",
			CreatedAt:              1,
			LastDeliveredMessageID: "cursor",
			LastDeliveredAt:        150,
		},
	}); err != nil {
		t.Fatalf("SaveConnectionBindings returned error: %v", err)
	}
	removed, err := storage.PruneExpiredPendingInboundMessages(300)
	if err != nil {
		t.Fatalf("PruneExpiredPendingInboundMessages returned error: %v", err)
	}
	if removed != 3 {
		t.Fatalf("过期清理数量异常: %d", removed)
	}
	messages, err := storage.ListPendingInboundMessages("cch_user_a", 300, 0)
	if err != nil {
		t.Fatalf("ListPendingInboundMessages returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != "live" {
		t.Fatalf("过期清理后消息异常: %+v", messages)
	}
}

func TestPrunePendingInboundMessagesHardExpiresAfterOneHour(t *testing.T) {
	storage := newService(t.TempDir(), protocol.ConnectorCardID)
	now := int64(protocol.InboundCacheTTLMillis + 100)
	live := &connectordomain.PendingInboundMessage{
		ID:                 "live",
		ConnectorChannelID: "cch_user_a",
		Payload:            map[string]any{"text": "new"},
		ReceivedAt:         now - 1000,
		ExpiresAt:          now + 1000,
		CreatedAt:          now - 1000,
		UpdatedAt:          now - 1000,
	}
	stale := &connectordomain.PendingInboundMessage{
		ID:                 "stale",
		ConnectorChannelID: "cch_user_a",
		Payload:            map[string]any{"text": "old"},
		ReceivedAt:         now - protocol.InboundCacheTTLMillis,
		ExpiresAt:          now + 1000,
		CreatedAt:          now - protocol.InboundCacheTTLMillis,
		UpdatedAt:          now - protocol.InboundCacheTTLMillis,
	}
	if err := storage.EnqueuePendingInboundMessage(live, 100); err != nil {
		t.Fatalf("EnqueuePendingInboundMessage live returned error: %v", err)
	}
	if err := storage.EnqueuePendingInboundMessage(stale, 100); err != nil {
		t.Fatalf("EnqueuePendingInboundMessage stale returned error: %v", err)
	}
	removed, err := storage.PruneExpiredPendingInboundMessages(now)
	if err != nil {
		t.Fatalf("PruneExpiredPendingInboundMessages returned error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("超过 1 小时的消息应被硬清理，got=%d", removed)
	}
	messages, err := storage.ListPendingInboundMessages("cch_user_a", now, 0)
	if err != nil {
		t.Fatalf("ListPendingInboundMessages returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != "live" {
		t.Fatalf("硬过期清理后消息异常: %+v", messages)
	}
}
