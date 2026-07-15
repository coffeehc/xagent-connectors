package connectservice

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/feishuservice"
	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/protocol"
	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
)

type testStorage struct {
	mu    sync.Mutex
	apps  map[string]*connectordomain.AppBinding
	reply map[string]*connectordomain.ReplyReference
	media map[string]*connectordomain.MediaReference
	dir   string
}

func (storage *testStorage) Start(context.Context) error { return nil }
func (storage *testStorage) Stop(context.Context) error  { return nil }
func (storage *testStorage) StateDir() string            { return storage.dir }
func (storage *testStorage) MediaCacheDir() string       { return storage.dir }
func (storage *testStorage) LoadApps() (map[string]*connectordomain.AppBinding, error) {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	return cloneApps(storage.apps), nil
}
func (storage *testStorage) SaveApps(apps map[string]*connectordomain.AppBinding) error {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	storage.apps = cloneApps(apps)
	return nil
}
func (storage *testStorage) LoadReferences() (map[string]*connectordomain.ReplyReference, map[string]*connectordomain.MediaReference, error) {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	return cloneReply(storage.reply), cloneMedia(storage.media), nil
}
func (storage *testStorage) SaveReferences(reply map[string]*connectordomain.ReplyReference, media map[string]*connectordomain.MediaReference) error {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	storage.reply = cloneReply(reply)
	storage.media = cloneMedia(media)
	return nil
}

func (storage *testStorage) appSecret(channelID string) string {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	if app := storage.apps[channelID]; app != nil {
		return app.AppSecret
	}
	return ""
}

type testStream struct{}

func (stream *testStream) Start(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
func (stream *testStream) Close()                          {}

type testFeishu struct {
	mu              sync.Mutex
	sentChatID      string
	sentText        string
	sentImage       string
	repliedText     string
	repliedImage    string
	registrationErr error
}

func (service *testFeishu) Start(context.Context) error { return nil }
func (service *testFeishu) Stop(context.Context) error  { return nil }
func (service *testFeishu) RegisterApp(_ context.Context, onQRCode func(feishuservice.QRCodeInfo), onStatusChange func(feishuservice.RegistrationStatus)) (*feishuservice.AppCredential, error) {
	onQRCode(feishuservice.QRCodeInfo{URL: "https://accounts.feishu.cn/qr", ExpiresInSeconds: 600})
	onStatusChange(feishuservice.RegistrationStatus{Status: "polling"})
	if service.registrationErr != nil {
		return nil, service.registrationErr
	}
	return &feishuservice.AppCredential{AppID: "cli_test", AppSecret: "secret", UserOpenID: "ou_test"}, nil
}
func (service *testFeishu) NewStream(string, string, feishuservice.MessageHandler) feishuservice.Stream {
	return &testStream{}
}
func (service *testFeishu) UploadImage(context.Context, string, string, io.Reader) (string, error) {
	return "img_test", nil
}
func (service *testFeishu) DownloadMessageImage(context.Context, string, string, string, string) ([]byte, string, error) {
	return nil, "", nil
}
func (service *testFeishu) SendText(_ context.Context, _, _, chatID string, text string) (string, error) {
	service.mu.Lock()
	service.sentChatID = chatID
	service.sentText = text
	service.mu.Unlock()
	return "om_sent_text", nil
}
func (service *testFeishu) SendImage(_ context.Context, _, _, chatID string, imageKey string) (string, error) {
	service.mu.Lock()
	service.sentChatID = chatID
	service.sentImage = imageKey
	service.mu.Unlock()
	return "om_sent_image", nil
}
func (service *testFeishu) ReplyText(_ context.Context, _, _, _ string, text string) (string, error) {
	service.mu.Lock()
	service.repliedText = text
	service.mu.Unlock()
	return "om_text", nil
}
func (service *testFeishu) ReplyImage(_ context.Context, _, _, _, imageKey string) (string, error) {
	service.mu.Lock()
	service.repliedImage = imageKey
	service.mu.Unlock()
	return "om_image", nil
}

func TestQRAuthPersistsCreatedAppAndReturnsConnection(t *testing.T) {
	storage := &testStorage{apps: map[string]*connectordomain.AppBinding{}, reply: map[string]*connectordomain.ReplyReference{}, media: map[string]*connectordomain.MediaReference{}, dir: t.TempDir()}
	service := newService(Config{}, storage, &testFeishu{}).(*serviceImpl)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop(context.Background()) })
	start, err := service.StartAuth(context.Background(), "cch_test", connectorprotocol.ConnectorAuthStartRequest{FlowID: protocol.FeishuQRCreateFlowID})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, ok := service.AuthStatus(context.Background(), "cch_test", start.AuthSessionID, false)
		if ok && status.Status == connectorprotocol.ConnectorAuthStatusAuthenticated {
			if status.ConnectionDescriptor == nil {
				t.Fatal("missing connection descriptor")
			}
			if storage.appSecret("cch_test") != "secret" {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("auth did not become authenticated")
}

func TestRegistrationErrorIsExposedThroughAuthStatus(t *testing.T) {
	storage := &testStorage{apps: map[string]*connectordomain.AppBinding{}, reply: map[string]*connectordomain.ReplyReference{}, media: map[string]*connectordomain.MediaReference{}, dir: t.TempDir()}
	feishu := &testFeishu{registrationErr: &feishuservice.RegistrationError{Kind: feishuservice.RegistrationErrorAccessDenied, Code: "access_denied", Description: "tenant policy denied"}}
	service := newService(Config{}, storage, feishu).(*serviceImpl)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop(context.Background()) })
	start, err := service.StartAuth(context.Background(), "cch_test", connectorprotocol.ConnectorAuthStartRequest{FlowID: protocol.FeishuQRCreateFlowID})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, ok := service.AuthStatus(context.Background(), "cch_test", start.AuthSessionID, false)
		if ok && status.Status == connectorprotocol.ConnectorAuthStatusFailed {
			if !strings.Contains(status.Message, "access_denied") || !strings.Contains(status.Message, "tenant policy denied") {
				t.Fatalf("unexpected failure message: %s", status.Message)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("registration failure was not exposed")
}

func TestExpiredRegistrationRequiresQRCodeRefresh(t *testing.T) {
	storage := &testStorage{apps: map[string]*connectordomain.AppBinding{}, reply: map[string]*connectordomain.ReplyReference{}, media: map[string]*connectordomain.MediaReference{}, dir: t.TempDir()}
	feishu := &testFeishu{registrationErr: &feishuservice.RegistrationError{Kind: feishuservice.RegistrationErrorExpired, Code: "expired_token", Description: "registration expired"}}
	service := newService(Config{}, storage, feishu).(*serviceImpl)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop(context.Background()) })
	start, err := service.StartAuth(context.Background(), "cch_test", connectorprotocol.ConnectorAuthStartRequest{FlowID: protocol.FeishuQRCreateFlowID})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, ok := service.AuthStatus(context.Background(), "cch_test", start.AuthSessionID, false)
		if ok && status.Status == connectorprotocol.ConnectorAuthStatusQRRefreshRequired {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expired registration did not require QR refresh")
}

func TestReplyImageRequiresReferencesFromSameChannel(t *testing.T) {
	now := time.Now().Add(time.Hour).UnixMilli()
	storage := &testStorage{apps: map[string]*connectordomain.AppBinding{"cch_a": {ConnectorChannelID: "cch_a", AppID: "cli_a", AppSecret: "secret", CreatedAt: 1}}, reply: map[string]*connectordomain.ReplyReference{"reply_a": {Ref: "reply_a", ConnectorChannelID: "cch_a", AppID: "cli_a", MessageID: "om_source", ExpiresAt: now}}, media: map[string]*connectordomain.MediaReference{"media_a": {Ref: "media_a", Direction: "outbound", ConnectorChannelID: "cch_a", AppID: "cli_a", ImageKey: "img_a", ExpiresAt: now}}, dir: t.TempDir()}
	feishu := &testFeishu{}
	service := newService(Config{}, storage, feishu).(*serviceImpl)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop(context.Background()) })
	result, err := service.InvokeTool(context.Background(), ToolInvokeInput{ConnectorChannelID: "cch_a", ToolID: protocol.FeishuMessageReplyImageToolID, Arguments: map[string]any{"reply_ref": "reply_a", "media_ref": "media_a", "text": "说明"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Result["message_id"] != "om_image" || feishu.repliedImage != "img_a" || feishu.repliedText != "说明" {
		t.Fatalf("unexpected reply result: %#v", result)
	}
	_, err = service.InvokeTool(context.Background(), ToolInvokeInput{ConnectorChannelID: "cch_other", ToolID: protocol.FeishuMessageReplyImageToolID, Arguments: map[string]any{"reply_ref": "reply_a", "media_ref": "media_a"}})
	if err == nil {
		t.Fatal("expected cross-channel reference rejection")
	}
}

func TestSendTextUsesBoundDefaultP2PConversation(t *testing.T) {
	storage := &testStorage{apps: map[string]*connectordomain.AppBinding{"cch_a": {ConnectorChannelID: "cch_a", AppID: "cli_a", AppSecret: "secret", DefaultChatID: "oc_p2p", CreatedAt: 1}}, reply: map[string]*connectordomain.ReplyReference{}, media: map[string]*connectordomain.MediaReference{}, dir: t.TempDir()}
	feishu := &testFeishu{}
	service := newService(Config{}, storage, feishu).(*serviceImpl)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop(context.Background()) })
	result, err := service.InvokeTool(context.Background(), ToolInvokeInput{ConnectorChannelID: "cch_a", ToolID: protocol.FeishuMessageSendToolID, Arguments: map[string]any{"text": "邮件提醒"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Result["message_id"] != "om_sent_text" || feishu.sentChatID != "oc_p2p" || feishu.sentText != "邮件提醒" {
		t.Fatalf("unexpected send result: %#v", result)
	}
}

func TestP2PInboundBindsDefaultConversationWithoutReplyReference(t *testing.T) {
	storage := &testStorage{apps: map[string]*connectordomain.AppBinding{"cch_a": {ConnectorChannelID: "cch_a", AppID: "cli_a", AppSecret: "secret", CreatedAt: 1}}, reply: map[string]*connectordomain.ReplyReference{}, media: map[string]*connectordomain.MediaReference{}, dir: t.TempDir()}
	service := newService(Config{}, storage, &testFeishu{}).(*serviceImpl)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop(context.Background()) })
	service.processInbound(context.Background(), inboundEnvelope{ChannelID: "cch_a", AppID: "cli_a", AppSecret: "secret", Message: feishuservice.InboundMessage{MessageID: "om_p2p", ChatID: "oc_p2p", ChatType: "p2p", MessageType: "text", Content: `{"text":"hi"}`, SenderOpenID: "ou_user", SenderType: "user"}})
	storage.mu.Lock()
	app := storage.apps["cch_a"]
	replyCount := len(storage.reply)
	storage.mu.Unlock()
	if app == nil || app.DefaultChatID != "oc_p2p" || app.DefaultSenderOpenID != "ou_user" {
		t.Fatalf("default p2p conversation was not persisted: %#v", app)
	}
	if replyCount != 0 {
		t.Fatalf("p2p message unexpectedly created reply references: %d", replyCount)
	}
}

func TestBuildFeishuInboundPayloadUsesIMRecommendedFormat(t *testing.T) {
	message := feishuservice.InboundMessage{
		MessageID:    "om_test",
		ChatID:       "oc_test",
		ChatType:     "p2p",
		MessageType:  "text",
		SenderOpenID: "ou_test",
		CreateTime:   "1784123456789",
	}
	payload := buildFeishuInboundPayload(message, "", "hi", nil)
	if payload["provider"] != "feishu" || payload["profile"] != "xagent.im.v1" || payload["event_kind"] != "im.message.received" {
		t.Fatalf("missing IM identity fields: %#v", payload)
	}
	visibleText, _ := payload["text"].(string)
	for _, expected := range []string{"来自飞书的用户消息：", "发送方：ou_test", "会话类型：单聊", "消息类型：文本", "用户文本：hi", protocol.FeishuMessageSendToolID, protocol.ConnectorSkillIMReplyID} {
		if !strings.Contains(visibleText, expected) {
			t.Fatalf("visible text missing %q: %s", expected, visibleText)
		}
	}
	if payload["content"] != visibleText || payload["activation_message"] != visibleText || payload["raw_text"] != "hi" {
		t.Fatalf("text projections are inconsistent: %#v", payload)
	}
	reply, ok := payload["reply"].(map[string]any)
	if !ok || reply["tool_id"] != protocol.FeishuMessageSendToolID {
		t.Fatalf("unexpected reply metadata: %#v", payload["reply"])
	}
	if _, exists := payload["reply_ref"]; exists || strings.Contains(visibleText, "reply_ref") {
		t.Fatalf("p2p payload must not expose reply_ref: %#v", payload)
	}
	if payload["create_time_ms"] != int64(1784123456789) {
		t.Fatalf("unexpected create_time_ms: %#v", payload["create_time_ms"])
	}
}

func TestBuildFeishuInboundPayloadUsesMediaArray(t *testing.T) {
	message := feishuservice.InboundMessage{MessageID: "om_image", ChatType: "group", MessageType: "image", SenderOpenID: "ou_image"}
	mediaItems := []map[string]any{{"type": "image", "media_ref": "media_test", "mime_type": "image/png", "download_url": "/media/refs/media_test"}}
	payload := buildFeishuInboundPayload(message, "reply_image", "", mediaItems)
	media, ok := payload["media"].([]map[string]any)
	if !ok || len(media) != 1 || media[0]["media_ref"] != "media_test" {
		t.Fatalf("unexpected media payload: %#v", payload["media"])
	}
	visibleText, _ := payload["text"].(string)
	if !strings.Contains(visibleText, "会话类型：群聊") || !strings.Contains(visibleText, "消息类型：图片") || !strings.Contains(visibleText, "用户文本：无") {
		t.Fatalf("unexpected image visible text: %s", visibleText)
	}
	reply, ok := payload["reply"].(map[string]any)
	if !ok || reply["tool_id"] != protocol.FeishuMessageReplyToolID || reply["reply_ref"] != "reply_image" {
		t.Fatalf("group payload must use reply mode: %#v", payload["reply"])
	}
}
