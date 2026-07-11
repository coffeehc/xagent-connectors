package connectservice

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/ilinkservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/mediaservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
)

type testStorageService struct {
	connections map[string]*connectordomain.Connection
	bots        map[string]*connectordomain.Bot
	channels    map[string]*connectordomain.ConnectionBinding
	pending     []*connectordomain.PendingInboundMessage
	references  []*connectordomain.MediaReference
}

func (service *testStorageService) Start(context.Context) error {
	return nil
}

func (service *testStorageService) Stop(context.Context) error {
	return nil
}

func (service *testStorageService) StateDir() string {
	return tTempStateDir
}

func (service *testStorageService) LoadBots() (map[string]*connectordomain.Bot, error) {
	if service.bots == nil {
		service.bots = map[string]*connectordomain.Bot{}
		for _, connection := range service.connections {
			bot := botFromConnection(connection)
			if bot != nil {
				service.bots[bot.BotAccountID] = bot
			}
		}
	}
	return service.bots, nil
}

func (service *testStorageService) SaveBots(bots map[string]*connectordomain.Bot) error {
	service.bots = bots
	return nil
}

func (service *testStorageService) SaveBot(bot *connectordomain.Bot) error {
	if service.bots == nil {
		service.bots = map[string]*connectordomain.Bot{}
	}
	service.bots[bot.BotAccountID] = bot
	return nil
}

func (service *testStorageService) LoadConnectionBindings() (map[string]*connectordomain.ConnectionBinding, error) {
	if service.channels == nil {
		service.channels = map[string]*connectordomain.ConnectionBinding{}
		for _, connection := range service.connections {
			if connection == nil || connection.Token == "" || connection.BotAccountID == "" {
				continue
			}
			service.channels[connection.Token] = &connectordomain.ConnectionBinding{
				ConnectorChannelID: connection.Token,
				BotAccountID:       connection.BotAccountID,
				CreatedAt:          connection.CreatedAt,
			}
		}
	}
	return service.channels, nil
}

func (service *testStorageService) SaveConnectionBindings(channels map[string]*connectordomain.ConnectionBinding) error {
	service.channels = channels
	return nil
}

func (service *testStorageService) SaveConnectionBinding(channel *connectordomain.ConnectionBinding) error {
	if service.channels == nil {
		service.channels = map[string]*connectordomain.ConnectionBinding{}
	}
	service.channels[channel.ConnectorChannelID] = channel
	return nil
}

func (service *testStorageService) DeleteConnectionBinding(connectorChannelID string) error {
	delete(service.channels, connectorChannelID)
	return nil
}

func (service *testStorageService) LoadLegacyConnections() (map[string]*connectordomain.Connection, error) {
	return map[string]*connectordomain.Connection{}, nil
}

func (service *testStorageService) SaveConnection(connection *connectordomain.Connection) error {
	bot := botFromConnection(connection)
	if bot == nil {
		return nil
	}
	return service.SaveBot(bot)
}

func (service *testStorageService) SaveConnections(connections map[string]*connectordomain.Connection) error {
	service.connections = connections
	service.bots = nil
	service.channels = nil
	return nil
}

func (service *testStorageService) EnqueuePendingInboundMessage(message *connectordomain.PendingInboundMessage, _ int) error {
	service.pending = append(service.pending, message)
	return nil
}

func (service *testStorageService) ListPendingInboundMessages(string, int64, int) ([]*connectordomain.PendingInboundMessage, error) {
	return service.pending, nil
}

func (service *testStorageService) MarkPendingInboundDelivered(connectorChannelID string, messageID string, deliveredAt int64) error {
	next := service.pending[:0]
	for _, message := range service.pending {
		if message.ID == messageID {
			continue
		}
		next = append(next, message)
	}
	service.pending = next
	if service.channels == nil {
		service.channels = map[string]*connectordomain.ConnectionBinding{}
	}
	channel := service.channels[connectorChannelID]
	if channel == nil {
		channel = &connectordomain.ConnectionBinding{ConnectorChannelID: connectorChannelID, BotAccountID: "bot-test"}
		service.channels[connectorChannelID] = channel
	}
	channel.LastDeliveredMessageID = messageID
	channel.LastDeliveredAt = deliveredAt
	return nil
}

func (service *testStorageService) MarkPendingInboundFailed(string, string, int64, string) error {
	return nil
}

func (service *testStorageService) PruneExpiredPendingInboundMessages(int64) (int, error) {
	return 0, nil
}

func (service *testStorageService) LoadInboundChannelCursor(string) (*connectordomain.InboundChannelCursor, error) {
	return nil, nil
}

func (service *testStorageService) SaveMediaReference(reference *connectordomain.MediaReference) error {
	for index, item := range service.references {
		if item != nil && item.Ref == reference.Ref {
			service.references[index] = reference
			return nil
		}
	}
	service.references = append(service.references, reference)
	return nil
}

func (service *testStorageService) GetMediaReference(ref string, nowMillis int64) (*connectordomain.MediaReference, error) {
	for _, item := range service.references {
		if item == nil || item.Ref != ref {
			continue
		}
		if item.ExpiresAt > 0 && nowMillis >= item.ExpiresAt {
			return nil, nil
		}
		return item, nil
	}
	return nil, nil
}

func (service *testStorageService) PruneExpiredMediaReferences(nowMillis int64) (int, error) {
	next := service.references[:0]
	for _, item := range service.references {
		if item == nil || item.ExpiresAt > 0 && nowMillis >= item.ExpiresAt {
			continue
		}
		next = append(next, item)
	}
	removed := len(service.references) - len(next)
	service.references = next
	return removed, nil
}

type testWeChatService struct {
	mu                sync.Mutex
	sent              []ilinkservice.SendTextMessageInput
	messages          []ilinkservice.SendMessageInput
	uploadRequests    []ilinkservice.GetUploadURLInput
	configReads       []ilinkservice.GetConfigInput
	typingEvents      []ilinkservice.SendTypingInput
	qrResponses       []*ilinkservice.QRCodeResponse
	qrStatusResponses []*ilinkservice.QRCodeStatusResponse
	qrStatusInputs    []ilinkservice.QRCodeStatusInput
	qrFetchStarted    chan struct{}
	qrFetchRelease    <-chan struct{}
	getUpdates        []*ilinkservice.GetUpdatesResponse
}

type testMediaService struct {
	uploads              []mediaservice.UploadEncryptedBufferInput
	fileUploads          []mediaservice.UploadEncryptedFileInput
	references           []*connectordomain.MediaReference
	inboundRegistrations []mediaservice.RegisterInboundMediaInput
}

type testMessagePusher struct {
	mu          sync.Mutex
	delivered   []map[string]any
	descriptors []*connectorprotocol.ConnectionDescriptor
}

func (pusher *testMessagePusher) PushMessage(_ context.Context, _ string, payload map[string]any) error {
	pusher.mu.Lock()
	defer pusher.mu.Unlock()
	pusher.delivered = append(pusher.delivered, payload)
	return nil
}

func (pusher *testMessagePusher) PushConnectionDescriptor(_ context.Context, _ string, descriptor *connectorprotocol.ConnectionDescriptor) error {
	pusher.mu.Lock()
	defer pusher.mu.Unlock()
	pusher.descriptors = append(pusher.descriptors, descriptor)
	return nil
}

func (pusher *testMessagePusher) descriptorSnapshot() []*connectorprotocol.ConnectionDescriptor {
	pusher.mu.Lock()
	defer pusher.mu.Unlock()
	return append([]*connectorprotocol.ConnectionDescriptor(nil), pusher.descriptors...)
}

func (service *testWeChatService) Start(context.Context) error {
	return nil
}

func (service *testWeChatService) Stop(context.Context) error {
	return nil
}

func (service *testWeChatService) APIBaseURL() string {
	return protocol.WeChatAPIBaseURL
}

func (service *testWeChatService) BotType() string {
	return protocol.WeChatBotType
}

func (service *testWeChatService) FetchQRCode(context.Context, []string) (*ilinkservice.QRCodeResponse, error) {
	if service.qrFetchStarted != nil {
		select {
		case service.qrFetchStarted <- struct{}{}:
		default:
		}
	}
	if service.qrFetchRelease != nil {
		<-service.qrFetchRelease
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.qrResponses) == 0 {
		return &ilinkservice.QRCodeResponse{QRCode: "qr-default", QRCodeImageContent: "https://weixin.example/qr-default"}, nil
	}
	response := service.qrResponses[0]
	service.qrResponses = service.qrResponses[1:]
	return response, nil
}

func (service *testWeChatService) FetchQRCodeStatus(_ context.Context, input ilinkservice.QRCodeStatusInput) (*ilinkservice.QRCodeStatusResponse, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.qrStatusInputs = append(service.qrStatusInputs, input)
	if len(service.qrStatusResponses) == 0 {
		return &ilinkservice.QRCodeStatusResponse{Status: "wait"}, nil
	}
	response := service.qrStatusResponses[0]
	service.qrStatusResponses = service.qrStatusResponses[1:]
	return response, nil
}

func (service *testWeChatService) GetUpdates(context.Context, ilinkservice.GetUpdatesInput) (*ilinkservice.GetUpdatesResponse, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.getUpdates) > 0 {
		response := service.getUpdates[0]
		service.getUpdates = service.getUpdates[1:]
		return response, nil
	}
	return &ilinkservice.GetUpdatesResponse{}, nil
}

func (service *testWeChatService) SendTextMessage(_ context.Context, input ilinkservice.SendTextMessageInput) (*ilinkservice.SendTextMessageResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.sent = append(service.sent, input)
	return &ilinkservice.SendTextMessageResult{MessageID: "msg_test_1"}, nil
}

func (service *testWeChatService) SendMessage(_ context.Context, input ilinkservice.SendMessageInput) (*ilinkservice.SendMessageResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.messages = append(service.messages, input)
	messageID := "msg_test_1"
	if input.Message != nil && input.Message.ClientID != "" {
		messageID = input.Message.ClientID
	}
	return &ilinkservice.SendMessageResult{MessageID: messageID}, nil
}

func (service *testWeChatService) GetUploadURL(_ context.Context, input ilinkservice.GetUploadURLInput) (*ilinkservice.GetUploadURLResponse, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.uploadRequests = append(service.uploadRequests, input)
	return &ilinkservice.GetUploadURLResponse{UploadFullURL: "https://cdn.example/upload", UploadParam: "upload-param-1"}, nil
}

func (service *testWeChatService) GetConfig(_ context.Context, input ilinkservice.GetConfigInput) (*ilinkservice.GetConfigResponse, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.configReads = append(service.configReads, input)
	return &ilinkservice.GetConfigResponse{TypingTicket: "typing-ticket-1"}, nil
}

func (service *testWeChatService) SendTyping(_ context.Context, input ilinkservice.SendTypingInput) (*ilinkservice.SendTypingResponse, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.typingEvents = append(service.typingEvents, input)
	return &ilinkservice.SendTypingResponse{}, nil
}

func (service *testWeChatService) typingEventSnapshot() []ilinkservice.SendTypingInput {
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]ilinkservice.SendTypingInput(nil), service.typingEvents...)
}

func (service *testWeChatService) configReadSnapshot() []ilinkservice.GetConfigInput {
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]ilinkservice.GetConfigInput(nil), service.configReads...)
}

func (service *testWeChatService) NotifyStart(context.Context, ilinkservice.NotifyInput) (*ilinkservice.NotifyResponse, error) {
	return &ilinkservice.NotifyResponse{}, nil
}

func (service *testWeChatService) NotifyStop(context.Context, ilinkservice.NotifyInput) (*ilinkservice.NotifyResponse, error) {
	return &ilinkservice.NotifyResponse{}, nil
}

func (service *testMediaService) Start(context.Context) error {
	return nil
}

func (service *testMediaService) Stop(context.Context) error {
	return nil
}

func (service *testMediaService) CDNBaseURL() string {
	return protocol.WeChatCDNBaseURL
}

func (service *testMediaService) BuildUploadURL(input mediaservice.BuildUploadURLInput) (string, error) {
	return input.UploadFullURL, nil
}

func (service *testMediaService) BuildDownloadURL(string) (string, error) {
	return "", nil
}

func (service *testMediaService) EncryptAESECB(plaintext []byte, _ []byte) ([]byte, error) {
	return plaintext, nil
}

func (service *testMediaService) DecryptAESECB(ciphertext []byte, _ []byte) ([]byte, error) {
	return ciphertext, nil
}

func (service *testMediaService) AESECBPaddedSize(plaintextSize int64) int64 {
	return (plaintextSize/16 + 1) * 16
}

func (service *testMediaService) UploadEncryptedBuffer(_ context.Context, input mediaservice.UploadEncryptedBufferInput) (*mediaservice.UploadEncryptedBufferResult, error) {
	service.uploads = append(service.uploads, input)
	return &mediaservice.UploadEncryptedBufferResult{DownloadParam: "download-param-" + input.FileKey}, nil
}

func (service *testMediaService) UploadEncryptedFile(_ context.Context, input mediaservice.UploadEncryptedFileInput) (*mediaservice.UploadEncryptedBufferResult, error) {
	if _, err := os.Stat(input.FilePath); err != nil {
		return nil, err
	}
	service.fileUploads = append(service.fileUploads, input)
	return &mediaservice.UploadEncryptedBufferResult{DownloadParam: "download-param-" + input.FileKey}, nil
}

func (service *testMediaService) DownloadAndDecryptBuffer(context.Context, mediaservice.DownloadAndDecryptBufferInput) ([]byte, error) {
	return nil, nil
}

func (service *testMediaService) RegisterOutboundMedia(_ context.Context, input mediaservice.RegisterOutboundMediaInput) (*connectordomain.MediaReference, error) {
	now := time.Now().UnixMilli()
	reference := &connectordomain.MediaReference{
		Ref:                fmt.Sprintf("media_ref_%d", len(service.references)+1),
		Direction:          "outbound",
		ConnectorChannelID: input.ConnectorChannelID,
		PeerRef:            input.PeerRef,
		MediaType:          input.MediaType,
		ILinkMediaType:     input.ILinkMediaType,
		Filename:           input.Filename,
		ContentType:        input.ContentType,
		RawSize:            input.RawSize,
		RawMD5:             input.RawMD5,
		CipherSize:         input.CipherSize,
		DownloadParam:      input.DownloadParam,
		AESKeyBase64:       input.AESKeyBase64,
		CreatedAt:          now,
		ExpiresAt:          now + protocol.DefaultMediaReferenceTTLMillis,
	}
	service.references = append(service.references, reference)
	return reference, nil
}

func (service *testMediaService) RegisterInboundMedia(_ context.Context, input mediaservice.RegisterInboundMediaInput) (*connectordomain.MediaReference, error) {
	now := time.Now().UnixMilli()
	reference := &connectordomain.MediaReference{
		Ref:                fmt.Sprintf("media_ref_%d", len(service.references)+1),
		Direction:          "inbound",
		ConnectorChannelID: input.ConnectorChannelID,
		PeerRef:            input.PeerRef,
		WeChatMessageID:    input.WeChatMessageID,
		MediaType:          input.MediaType,
		Filename:           input.Filename,
		ContentType:        input.ContentType,
		RawSize:            input.RawSize,
		RawMD5:             input.RawMD5,
		CipherSize:         input.CipherSize,
		DownloadParam:      input.DownloadParam,
		FullURL:            input.FullURL,
		AESKeyBase64:       input.AESKeyBase64,
		CreatedAt:          now,
		ExpiresAt:          now + protocol.DefaultMediaReferenceTTLMillis,
	}
	service.inboundRegistrations = append(service.inboundRegistrations, input)
	service.references = append(service.references, reference)
	return reference, nil
}

func (service *testMediaService) GetMediaReference(_ context.Context, mediaRef string, nowMillis int64) (*connectordomain.MediaReference, error) {
	for _, item := range service.references {
		if item == nil || item.Ref != mediaRef {
			continue
		}
		if item.ExpiresAt > 0 && nowMillis >= item.ExpiresAt {
			return nil, nil
		}
		return item, nil
	}
	return nil, nil
}

func (service *testMediaService) PruneExpiredMediaReferences(_ context.Context, nowMillis int64) (int, error) {
	next := service.references[:0]
	for _, item := range service.references {
		if item == nil || item.ExpiresAt > 0 && nowMillis >= item.ExpiresAt {
			continue
		}
		next = append(next, item)
	}
	removed := len(service.references) - len(next)
	service.references = next
	return removed, nil
}

func (service *testMediaService) OpenMediaStream(context.Context, mediaservice.OpenMediaStreamInput) (*mediaservice.OpenMediaStreamResult, error) {
	return nil, nil
}

const tTempStateDir = "/tmp/xagent-wechat-connector-test"

func TestBuildConnectorCardDeclaresStaticCapabilities(t *testing.T) {
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{}}, &testWeChatService{}, &testMediaService{})

	card := service.BuildConnectorCard()
	if card == nil {
		t.Fatalf("BuildConnectorCard returned nil")
	}
	if card.Schema != "xagent.connector/v1" || card.ConnectorCardID != "im.wechat" {
		t.Fatalf("Connector Card 基础身份异常: %+v", card)
	}
	if len(card.Tools) != 2 {
		t.Fatalf("Connector Card 应声明 2 个工具，got=%d tools=%+v", len(card.Tools), card.Tools)
	}
	if card.Tools[0].ToolID != "wechat_message_send" || card.Tools[1].ToolID != "wechat_message_send_media" {
		t.Fatalf("Connector Card 工具声明异常: %+v", card.Tools)
	}
	if err := connectorprotocol.ValidateConnectorCardToolInputSchemas(card); err != nil {
		t.Fatalf("Connector Card 工具 schema 协议校验失败: %v", err)
	}
	if len(card.Tools[0].RelatedSkillIDs) != 1 || card.Tools[0].RelatedSkillIDs[0] != protocol.ConnectorSkillIMReplyID {
		t.Fatalf("文本工具 related_skill_ids 异常: %+v", card.Tools[0].RelatedSkillIDs)
	}
	if len(card.Tools[1].RelatedSkillIDs) != 1 || card.Tools[1].RelatedSkillIDs[0] != protocol.ConnectorSkillIMReplyID {
		t.Fatalf("媒体工具 related_skill_ids 异常: %+v", card.Tools[1].RelatedSkillIDs)
	}
	textSchemaDescription, _ := card.Tools[0].InputSchema["description"].(string)
	if !strings.Contains(textSchemaDescription, protocol.ConnectorSkillIMReplyID) || !strings.Contains(textSchemaDescription, "wechat_message_send_media") {
		t.Fatalf("文本工具 schema 应指向 skill 和媒体工具: %s", textSchemaDescription)
	}
	assertConnectorChannelIDInputSchema(t, card.Tools[0])
	textRequired := schemaRequiredStrings(t, card.Tools[0].InputSchema)
	if !containsTestString(textRequired, "text") || !containsTestString(textRequired, connectorprotocol.ConnectorToolParamConnectorChannelID) {
		t.Fatalf("文本工具 required schema 异常: %+v", textRequired)
	}
	assertConnectorChannelIDInputSchema(t, card.Tools[1])
	mediaRequired := schemaRequiredStrings(t, card.Tools[1].InputSchema)
	if !containsTestString(mediaRequired, "media_ref") || !containsTestString(mediaRequired, connectorprotocol.ConnectorToolParamConnectorChannelID) {
		t.Fatalf("媒体工具 required schema 异常: %+v", mediaRequired)
	}
	skillContent, err := service.ReadConnectorSkill(context.Background())
	if err != nil {
		t.Fatalf("ReadConnectorSkill failed: %v", err)
	}
	if skillContent == nil || skillContent.SHA256 == "" || skillContent.SkillID != protocol.ConnectorSkillIMReplyID {
		t.Fatalf("主 Skill 内容异常: %+v", skillContent)
	}
	if !strings.Contains(skillContent.Content, "This skill owns the WeChat attachment sending workflow") || !strings.Contains(skillContent.Content, "wechat_message_send is the wrong tool") {
		t.Fatalf("主 Skill 应声明附件流程 owner 和工具选择规则")
	}
}

func assertConnectorChannelIDInputSchema(t *testing.T, tool connectorprotocol.ConnectorToolDescriptor) {
	t.Helper()
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("工具 %s properties schema 异常: %+v", tool.ToolID, tool.InputSchema["properties"])
	}
	channelProperty, ok := properties[connectorprotocol.ConnectorToolParamConnectorChannelID].(map[string]any)
	if !ok {
		t.Fatalf("工具 %s 缺少 connector_channel_id property: %+v", tool.ToolID, properties)
	}
	if channelProperty["type"] != "string" {
		t.Fatalf("工具 %s connector_channel_id 必须是 string: %+v", tool.ToolID, channelProperty)
	}
	required := schemaRequiredStrings(t, tool.InputSchema)
	if !containsTestString(required, connectorprotocol.ConnectorToolParamConnectorChannelID) {
		t.Fatalf("工具 %s connector_channel_id 必须在 required 中: %+v", tool.ToolID, required)
	}
}

func schemaRequiredStrings(t *testing.T, schema map[string]any) []string {
	t.Helper()
	switch values := schema["required"].(type) {
	case []string:
		return append([]string{}, values...)
	case []any:
		output := []string{}
		for _, value := range values {
			item, ok := value.(string)
			if !ok {
				t.Fatalf("required item 应为 string: %+v", values)
			}
			output = append(output, item)
		}
		return output
	default:
		t.Fatalf("required schema 异常: %+v", schema["required"])
		return nil
	}
}

func containsTestString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestBuildConnectionDescriptor(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:        "uct_wechat_test",
		WeChatUserID: "wx-user-a",
		BotToken:     "bot-token-a",
		BotAccountID: "0069-test-bot",
		BaseURL:      protocol.WeChatAPIBaseURL,
		DisplayName:  "微信 0069***.bot",
		AccountHint:  "0069***.bot",
	}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}, &testWeChatService{}, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	descriptor := service.BuildConnectionDescriptor(connection)
	if descriptor == nil || descriptor.Target.AccountHint != connection.AccountHint {
		t.Fatalf("connection descriptor 不可用: %+v", descriptor)
	}
	if len(descriptor.Tools) != 2 {
		t.Fatalf("connection descriptor 应声明 2 个可用工具，got=%d tools=%+v", len(descriptor.Tools), descriptor.Tools)
	}
}

func TestStartPrunesUnboundBotsAndKeepsSingleChannelPerBot(t *testing.T) {
	storage := &testStorageService{
		bots: map[string]*connectordomain.Bot{
			"bot-a": {
				WeChatUserID: "wx_user_a",
				BotToken:     "bot-token-a",
				BotAccountID: "bot-a",
				BaseURL:      protocol.WeChatAPIBaseURL,
				DisplayName:  "微信 bot-a",
				AccountHint:  "bot-a",
				CreatedAt:    100,
			},
			"bot-expired": {
				WeChatUserID: "wx_expired",
				BotToken:     "bot-token-expired",
				BotAccountID: "bot-expired",
				BaseURL:      protocol.WeChatAPIBaseURL,
				DisplayName:  "微信 expired",
				AccountHint:  "expired",
				CreatedAt:    100,
			},
		},
		channels: map[string]*connectordomain.ConnectionBinding{
			"old_channel": {ConnectorChannelID: "old_channel", BotAccountID: "bot-a", CreatedAt: 100},
			"new_channel": {ConnectorChannelID: "new_channel", BotAccountID: "bot-a", CreatedAt: 200},
		},
	}
	service := newService(Config{
		APIKey: "test-key",
	}, storage, &testWeChatService{}, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if storage.bots["bot-expired"] != nil {
		t.Fatalf("未绑定 channel 的 bot 应删除，got=%+v", storage.bots["bot-expired"])
	}
	if len(storage.channels) != 1 || storage.channels["new_channel"] == nil {
		t.Fatalf("同一 bot 应只保留最新 channel，got=%+v", storage.channels)
	}
	if service.ConnectionByChannel("old_channel") != nil || service.ConnectionByChannel("new_channel") == nil {
		t.Fatalf("runtime connection 投影异常")
	}
}

func TestAuthStatusRefreshesQRCodeWhenWeChatExpires(t *testing.T) {
	wechat := &testWeChatService{
		qrResponses: []*ilinkservice.QRCodeResponse{
			{QRCode: "qr-start", QRCodeImageContent: "https://weixin.example/qr-start"},
			{QRCode: "qr-refresh", QRCodeImageContent: "https://weixin.example/qr-refresh"},
		},
		qrStatusResponses: []*ilinkservice.QRCodeStatusResponse{
			{Status: "expired"},
		},
	}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{}, wechat, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	startResult, err := service.StartAuth(context.Background(), "cch_user_a", protocol.WeChatQRLoginFlowID)
	if err != nil {
		t.Fatalf("StartAuth returned error: %v", err)
	}
	waitForAuthSessionQRCode(t, service, startResult.AuthSessionID)
	statusResult, ok := service.AuthStatus(context.Background(), "cch_user_a", startResult.AuthSessionID, false)
	if !ok {
		t.Fatalf("AuthStatus returned !ok")
	}
	if statusResult.Status != connectorprotocol.ConnectorAuthStatusPending || statusResult.QRCodeText != "https://weixin.example/qr-start" {
		t.Fatalf("首次 auth.status 应先把二维码返回给前端，got=%+v", statusResult)
	}
	if len(wechat.qrStatusInputs) != 0 {
		t.Fatalf("首次返回二维码前不应先阻塞查询微信状态，got=%+v", wechat.qrStatusInputs)
	}
	statusResult, ok = service.AuthStatus(context.Background(), "cch_user_a", startResult.AuthSessionID, false)
	if !ok {
		t.Fatalf("AuthStatus returned !ok")
	}
	if statusResult.Status != connectorprotocol.ConnectorAuthStatusPending || statusResult.QRCodeText != "https://weixin.example/qr-refresh" {
		t.Fatalf("二维码过期后应自动刷新并继续 pending，got=%+v", statusResult)
	}
	if len(wechat.qrStatusInputs) != 1 || wechat.qrStatusInputs[0].QRCode != "qr-start" {
		t.Fatalf("应使用初始二维码查询微信状态，got=%+v", wechat.qrStatusInputs)
	}
}

func TestAuthStatusDoesNotRefreshQRCodeWhileWeChatStillWaits(t *testing.T) {
	wechat := &testWeChatService{
		qrResponses: []*ilinkservice.QRCodeResponse{
			{QRCode: "qr-start", QRCodeImageContent: "https://weixin.example/qr-start"},
			{QRCode: "qr-refresh", QRCodeImageContent: "https://weixin.example/qr-refresh"},
		},
		qrStatusResponses: []*ilinkservice.QRCodeStatusResponse{
			{Status: "wait"},
			{Status: "wait"},
		},
	}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{}, wechat, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	startResult, err := service.StartAuth(context.Background(), "cch_user_a", protocol.WeChatQRLoginFlowID)
	if err != nil {
		t.Fatalf("StartAuth returned error: %v", err)
	}
	waitForAuthSessionQRCode(t, service, startResult.AuthSessionID)
	firstStatus, ok := service.AuthStatus(context.Background(), "cch_user_a", startResult.AuthSessionID, false)
	if !ok {
		t.Fatalf("first AuthStatus returned !ok")
	}
	if firstStatus.Status != connectorprotocol.ConnectorAuthStatusPending || firstStatus.QRCodeText != "https://weixin.example/qr-start" {
		t.Fatalf("首次 auth.status 应返回已加载二维码，got=%+v", firstStatus)
	}
	secondStatus, ok := service.AuthStatus(context.Background(), "cch_user_a", startResult.AuthSessionID, false)
	if !ok {
		t.Fatalf("second AuthStatus returned !ok")
	}
	thirdStatus, ok := service.AuthStatus(context.Background(), "cch_user_a", startResult.AuthSessionID, false)
	if !ok {
		t.Fatalf("third AuthStatus returned !ok")
	}
	if secondStatus.QRCodeText != "https://weixin.example/qr-start" || thirdStatus.QRCodeText != "https://weixin.example/qr-start" {
		t.Fatalf("微信仍等待时不应刷新二维码，second=%+v third=%+v", secondStatus, thirdStatus)
	}
	if len(wechat.qrStatusInputs) != 2 {
		t.Fatalf("二维码返回后每次 status 只应查询二维码状态，got=%+v", wechat.qrStatusInputs)
	}
	wechat.mu.Lock()
	remainingQRResponses := len(wechat.qrResponses)
	wechat.mu.Unlock()
	if remainingQRResponses != 1 {
		t.Fatalf("微信未提示 expired 时不应获取新二维码，remaining=%d", remainingQRResponses)
	}
}

func TestAuthStatusDoesNotRefreshQRCodeWhenLocalSessionExpires(t *testing.T) {
	wechat := &testWeChatService{
		qrResponses: []*ilinkservice.QRCodeResponse{
			{QRCode: "qr-start", QRCodeImageContent: "https://weixin.example/qr-start"},
			{QRCode: "qr-refresh", QRCodeImageContent: "https://weixin.example/qr-refresh"},
		},
	}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{}, wechat, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	startResult, err := service.StartAuth(context.Background(), "cch_user_a", protocol.WeChatQRLoginFlowID)
	if err != nil {
		t.Fatalf("StartAuth returned error: %v", err)
	}
	waitForAuthSessionQRCode(t, service, startResult.AuthSessionID)
	impl := service.(*serviceImpl)
	impl.mu.Lock()
	impl.authSessions[startResult.AuthSessionID].ExpiresAt = time.Now().Add(-time.Second).UnixMilli()
	impl.mu.Unlock()
	statusResult, ok := service.AuthStatus(context.Background(), "cch_user_a", startResult.AuthSessionID, false)
	if !ok {
		t.Fatalf("AuthStatus returned !ok")
	}
	if statusResult.Status != connectorprotocol.ConnectorAuthStatusExpired || statusResult.QRCodeText != "https://weixin.example/qr-start" {
		t.Fatalf("本地认证会话过期不应自动刷新二维码，got=%+v", statusResult)
	}
	if len(wechat.qrStatusInputs) != 0 {
		t.Fatalf("本地 TTL 已过期时不应继续查询旧二维码状态，got=%+v", wechat.qrStatusInputs)
	}
	wechat.mu.Lock()
	remainingQRResponses := len(wechat.qrResponses)
	wechat.mu.Unlock()
	if remainingQRResponses != 1 {
		t.Fatalf("本地认证会话过期不应获取新二维码，remaining=%d", remainingQRResponses)
	}
}

func TestStartAuthReturnsBeforeQRCodeMaterialIsReady(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	wechat := &testWeChatService{
		qrFetchStarted: started,
		qrFetchRelease: release,
	}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{}, wechat, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	startedAt := time.Now()
	startResult, err := service.StartAuth(context.Background(), "cch_user_a", protocol.WeChatQRLoginFlowID)
	if err != nil {
		t.Fatalf("StartAuth returned error: %v", err)
	}
	if time.Since(startedAt) > 100*time.Millisecond {
		t.Fatalf("StartAuth 不应等待微信二维码接口完成")
	}
	if startResult.AuthSessionID == "" || startResult.Status != connectorprotocol.ConnectorAuthStartPending || startResult.QRCodeText != "" {
		t.Fatalf("StartAuth 应先返回 pending session，不直接等待二维码，got=%+v", startResult)
	}
	if startResult.PollIntervalMillis != protocol.AuthMaterialPollIntervalMillis {
		t.Fatalf("二维码材料未就绪时应使用短轮询间隔，got=%d", startResult.PollIntervalMillis)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("StartAuth 应异步启动二维码材料加载")
	}
	statusBeforeQRCode, ok := service.AuthStatus(context.Background(), "cch_user_a", startResult.AuthSessionID, false)
	if !ok {
		t.Fatalf("AuthStatus returned !ok before QR material")
	}
	if statusBeforeQRCode.QRCodeImage != "" || statusBeforeQRCode.PollIntervalMillis != protocol.AuthMaterialPollIntervalMillis {
		t.Fatalf("二维码材料加载中应短轮询且不返回空二维码图片，got=%+v", statusBeforeQRCode)
	}
	close(release)
	released = true
	waitForAuthSessionQRCode(t, service, startResult.AuthSessionID)
	statusResult, ok := service.AuthStatus(context.Background(), "cch_user_a", startResult.AuthSessionID, false)
	if !ok {
		t.Fatalf("AuthStatus returned !ok")
	}
	if statusResult.QRCodeImage == "" || statusResult.QRCodeText == "" || statusResult.Message == "正在获取微信二维码。" {
		t.Fatalf("二维码材料加载完成后 auth.status 应返回二维码，got=%+v", statusResult)
	}
	if statusResult.PollIntervalMillis != protocol.AuthPollIntervalMillis {
		t.Fatalf("二维码已返回后应恢复扫码状态轮询间隔，got=%d", statusResult.PollIntervalMillis)
	}
	if len(wechat.qrStatusInputs) != 0 {
		t.Fatalf("首次返回二维码前不应阻塞查询微信状态，got=%+v", wechat.qrStatusInputs)
	}
}

func TestCancelAuthRemovesPendingSession(t *testing.T) {
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{}, &testWeChatService{}, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	startResult, err := service.StartAuth(context.Background(), "cch_user_a", protocol.WeChatQRLoginFlowID)
	if err != nil {
		t.Fatalf("StartAuth returned error: %v", err)
	}
	cancelResult := service.CancelAuth(context.Background(), "cch_user_a", startResult.AuthSessionID)
	if cancelResult.Status != connectorprotocol.ConnectorAuthCancelStatusCanceled {
		t.Fatalf("pending auth session 应取消，got=%+v", cancelResult)
	}
	impl := service.(*serviceImpl)
	impl.mu.Lock()
	_, exists := impl.authSessions[startResult.AuthSessionID]
	impl.mu.Unlock()
	if exists {
		t.Fatalf("取消后不应继续保留 pending auth session")
	}
}

func TestCancelAuthWithoutSessionDoesNotCancelPendingSession(t *testing.T) {
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{}, &testWeChatService{}, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	startResult, err := service.StartAuth(context.Background(), "cch_user_a", protocol.WeChatQRLoginFlowID)
	if err != nil {
		t.Fatalf("StartAuth returned error: %v", err)
	}
	cancelResult := service.CancelAuth(context.Background(), "cch_user_a", "")
	if cancelResult.Status != connectorprotocol.ConnectorAuthCancelStatusNotFound {
		t.Fatalf("空 auth_session_id 不应取消任意 pending session，got=%+v", cancelResult)
	}
	impl := service.(*serviceImpl)
	impl.mu.Lock()
	authSession := impl.authSessions[startResult.AuthSessionID]
	impl.mu.Unlock()
	if authSession == nil || authSession.Status != connectorprotocol.ConnectorAuthStatusPending {
		t.Fatalf("空 auth_session_id 取消后 pending session 应保留，got=%+v", authSession)
	}
}

func TestCancelAuthIgnoresAuthenticatedSession(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:        "cch_user_a",
		BotToken:     "bot-token-a",
		BotAccountID: "bot-a",
		BaseURL:      protocol.WeChatAPIBaseURL,
		DisplayName:  "微信 bot-a",
		AccountHint:  "bot-a",
	}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{}, &testWeChatService{}, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	startResult, err := service.StartAuth(context.Background(), connection.Token, protocol.WeChatQRLoginFlowID)
	if err != nil {
		t.Fatalf("StartAuth returned error: %v", err)
	}
	impl := service.(*serviceImpl)
	impl.mu.Lock()
	impl.connections[connection.Token] = connection
	impl.authSessions[startResult.AuthSessionID].Status = connectorprotocol.ConnectorAuthStatusAuthenticated
	impl.authSessions[startResult.AuthSessionID].Token = connection.Token
	impl.mu.Unlock()
	cancelResult := service.CancelAuth(context.Background(), connection.Token, startResult.AuthSessionID)
	if cancelResult.Status != connectorprotocol.ConnectorAuthCancelStatusIgnored || cancelResult.AuthStatus != connectorprotocol.ConnectorAuthStatusAuthenticated || cancelResult.ConnectionDescriptor == nil {
		t.Fatalf("authenticated auth session 应忽略 cancel，got=%+v", cancelResult)
	}
	if service.ConnectionByChannel(connection.Token) == nil {
		t.Fatalf("cancel ignored 时不能清理真实 connection")
	}
}

func waitForAuthSessionQRCode(t *testing.T, service Service, authSessionID string) {
	t.Helper()
	impl := service.(*serviceImpl)
	deadline := time.Now().Add(time.Second)
	for {
		impl.mu.Lock()
		authSession := impl.authSessions[authSessionID]
		qrText := ""
		status := connectorprotocol.ConnectorAuthStatus("")
		message := ""
		if authSession != nil {
			qrText = authSession.QRText
			status = authSession.Status
			message = authSession.Message
		}
		impl.mu.Unlock()
		if qrText != "" {
			return
		}
		if status == connectorprotocol.ConnectorAuthStatusFailed {
			t.Fatalf("等待二维码材料失败: %s", message)
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待二维码材料超时")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestInvokeToolSendUsesExplicitRecipientAndCachedContextToken(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:         "cch_user_a",
		WeChatUserID:  "wx-user-a",
		BotToken:      "bot-token-a",
		BotAccountID:  "0069-test-bot",
		BaseURL:       protocol.WeChatAPIBaseURL,
		DisplayName:   "微信 0069***.bot",
		AccountHint:   "0069***.bot",
		ContextTokens: map[string]string{"wx_user_1": "ctx_msg_1"},
	}
	wechat := &testWeChatService{}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}, wechat, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	result, err := service.InvokeTool(context.Background(), ToolInvokeInput{
		ConnectorChannelID: connection.Token,
		ToolID:             "wechat_message_send",
		Arguments:          map[string]any{"recipient_ref": "wx_user_1", "text": "你好"},
	})
	if err != nil {
		t.Fatalf("wechat_message_send returned error: %v", err)
	}
	if result.Result["message_id"] != "msg_test_1" || result.Result["recipient_ref"] != "wx_user_1" {
		t.Fatalf("发送结果缺少 message_id: %+v", result.Result)
	}
	if len(wechat.sent) != 1 || wechat.sent[0].ContactID != "wx_user_1" || wechat.sent[0].ContextToken != "ctx_msg_1" || wechat.sent[0].Text != "你好" {
		t.Fatalf("发送请求参数异常: %+v", wechat.sent)
	}
}

func TestInvokeToolSendCancelsAutoTypingBeforeReply(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:         "cch_user_a",
		WeChatUserID:  "wx_user_1",
		BotToken:      "bot-token-a",
		BotAccountID:  "0069-test-bot",
		BaseURL:       protocol.WeChatAPIBaseURL,
		DisplayName:   "微信 0069***.bot",
		AccountHint:   "0069***.bot",
		ContextTokens: map[string]string{"wx_user_1": "ctx_msg_1"},
	}
	wechat := &testWeChatService{}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}, wechat, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() {
		_ = service.Stop(context.Background())
	}()

	service.(*serviceImpl).startAutoTypingForDeliveredMessage(context.Background(), connection.Token, map[string]any{
		"sender_id":     "wx_user_1",
		"context_token": "ctx_msg_1",
	})
	_, err := service.InvokeTool(context.Background(), ToolInvokeInput{
		ConnectorChannelID: connection.Token,
		ToolID:             toolIDWeChatMessageSend,
		Arguments:          map[string]any{"recipient_ref": "wx_user_1", "text": "你好"},
	})
	if err != nil {
		t.Fatalf("wechat_message_send returned error: %v", err)
	}

	events := wechat.typingEventSnapshot()
	if len(events) != 2 || events[0].Status != ilinkservice.TypingStatusTyping || events[1].Status != ilinkservice.TypingStatusCancel {
		t.Fatalf("回复前应先取消自动输入状态，got=%+v", events)
	}
	if len(wechat.sent) != 1 || wechat.sent[0].Text != "你好" {
		t.Fatalf("回复消息未发送: %+v", wechat.sent)
	}
}

func TestInvokeToolSendUsesChannelDefaultRecipient(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:         "cch_user_a",
		WeChatUserID:  "wx_user_2",
		BotToken:      "bot-token-a",
		BotAccountID:  "0069-test-bot",
		BaseURL:       protocol.WeChatAPIBaseURL,
		DisplayName:   "微信 0069***.bot",
		AccountHint:   "0069***.bot",
		ContextTokens: map[string]string{"wx_user_2": "ctx_msg_2"},
	}
	wechat := &testWeChatService{}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}, wechat, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	result, err := service.InvokeTool(context.Background(), ToolInvokeInput{
		ConnectorChannelID: connection.Token,
		ToolID:             "wechat_message_send",
		Arguments:          map[string]any{"text": "任务已完成"},
	})
	if err != nil {
		t.Fatalf("wechat_message_send returned error: %v", err)
	}
	if result.Result["recipient_ref"] != "wx_user_2" || result.Result["message_id"] != "msg_test_1" {
		t.Fatalf("主动发送结果异常: %+v", result.Result)
	}
	if len(wechat.sent) != 1 || wechat.sent[0].ContactID != "wx_user_2" || wechat.sent[0].ContextToken != "ctx_msg_2" || wechat.sent[0].Text != "任务已完成" {
		t.Fatalf("主动发送请求参数异常: %+v", wechat.sent)
	}
}

func TestUploadMediaStoresMediaRef(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:        "cch_user_a",
		WeChatUserID: "wx_user_2",
		BotToken:     "bot-token-a",
		BotAccountID: "0069-test-bot",
		BaseURL:      protocol.WeChatAPIBaseURL,
		DisplayName:  "微信 0069***.bot",
		AccountHint:  "0069***.bot",
	}
	storage := &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}
	wechat := &testWeChatService{}
	media := &testMediaService{}
	service := newService(Config{
		APIKey: "test-key",
	}, storage, wechat, media)
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	payload := "video-bytes"
	result, err := service.UploadMedia(context.Background(), UploadMediaInput{
		ConnectorChannelID: connection.Token,
		Filename:           "done.mp4",
		ContentType:        "video/mp4",
		Source:             strings.NewReader(payload),
		Size:               int64(len(payload)),
	})
	if err != nil {
		t.Fatalf("UploadMedia returned error: %v", err)
	}
	if result.MediaRef == "" || result.MediaType != connectorMediaTypeVideo || result.ByteSize != int64(len(payload)) {
		t.Fatalf("UploadMedia result 异常: %+v", result)
	}
	if len(wechat.uploadRequests) != 1 {
		t.Fatalf("应请求一次微信 getUploadURL，got=%+v", wechat.uploadRequests)
	}
	uploadRequest := wechat.uploadRequests[0]
	if uploadRequest.MediaType != ilinkservice.UploadMediaTypeVideo || uploadRequest.ToUserID != "wx_user_2" || uploadRequest.RawSize != int64(len(payload)) || !uploadRequest.NoNeedThumb {
		t.Fatalf("getUploadURL 参数异常: %+v", uploadRequest)
	}
	if uploadRequest.RawFileMD5 == "" || uploadRequest.AESKey == "" || uploadRequest.FileSize == 0 {
		t.Fatalf("getUploadURL 缺少媒体元数据: %+v", uploadRequest)
	}
	if len(media.fileUploads) != 1 {
		t.Fatalf("应执行一次文件上传，got=%+v", media.fileUploads)
	}
	if len(media.references) != 1 || media.references[0].Ref != result.MediaRef || media.references[0].DownloadParam == "" || media.references[0].AESKeyBase64 == "" {
		t.Fatalf("media_ref 注册异常: %+v", media.references)
	}
}

func TestInvokeToolSendMediaUsesMediaReference(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:         "cch_user_a",
		WeChatUserID:  "wx-user-a",
		BotToken:      "bot-token-a",
		BotAccountID:  "0069-test-bot",
		BaseURL:       protocol.WeChatAPIBaseURL,
		DisplayName:   "微信 0069***.bot",
		AccountHint:   "0069***.bot",
		ContextTokens: map[string]string{"wx_user_2": "ctx_msg_2"},
	}
	storage := &testStorageService{
		connections: map[string]*connectordomain.Connection{connection.Token: connection},
	}
	media := &testMediaService{
		references: []*connectordomain.MediaReference{{
			Ref:                "media_ref_1",
			Direction:          "outbound",
			ConnectorChannelID: connection.Token,
			PeerRef:            "wx_user_2",
			MediaType:          connectorMediaTypeVideo,
			ILinkMediaType:     ilinkservice.UploadMediaTypeVideo,
			Filename:           "done.mp4",
			ContentType:        "video/mp4",
			RawSize:            11,
			RawMD5:             "raw-md5",
			CipherSize:         16,
			DownloadParam:      "download-param-1",
			AESKeyBase64:       "base64-key",
			CreatedAt:          time.Now().UnixMilli(),
			ExpiresAt:          time.Now().Add(time.Hour).UnixMilli(),
		}},
	}
	wechat := &testWeChatService{}
	service := newService(Config{
		APIKey: "test-key",
	}, storage, wechat, media)
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	result, err := service.InvokeTool(context.Background(), ToolInvokeInput{
		ConnectorChannelID: connection.Token,
		ToolID:             "wechat_message_send_media",
		Arguments:          map[string]any{"media_ref": "media_ref_1", "text": "看视频"},
	})
	if err != nil {
		t.Fatalf("wechat_message_send_media returned error: %v", err)
	}
	if result.Result["media_ref"] != "media_ref_1" || result.Result["recipient_ref"] != "wx_user_2" {
		t.Fatalf("发送媒体结果异常: %+v", result.Result)
	}
	if len(wechat.sent) != 1 || wechat.sent[0].ContactID != "wx_user_2" || wechat.sent[0].ContextToken != "ctx_msg_2" || wechat.sent[0].Text != "看视频" {
		t.Fatalf("媒体说明文本发送异常: %+v", wechat.sent)
	}
	if len(wechat.messages) != 1 || wechat.messages[0].Message == nil || len(wechat.messages[0].Message.ItemList) != 1 {
		t.Fatalf("媒体 sendMessage 请求异常: %+v", wechat.messages)
	}
	message := wechat.messages[0].Message
	item := message.ItemList[0]
	if message.ToUserID != "wx_user_2" || message.ContextToken != "ctx_msg_2" || item.Type != ilinkservice.MessageItemTypeVideo || item.VideoItem == nil {
		t.Fatalf("媒体消息结构异常: %+v", message)
	}
	if item.VideoItem.VideoSize != 16 || item.VideoItem.Media == nil || item.VideoItem.Media.EncryptQueryParam != "download-param-1" || item.VideoItem.Media.AESKey != "base64-key" {
		t.Fatalf("媒体 CDN 引用异常: %+v", item.VideoItem)
	}
}

func TestUploadAudioMediaUsesFileMediaType(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:        "cch_user_a",
		WeChatUserID: "wx_user_2",
		BotToken:     "bot-token-a",
		BotAccountID: "0069-test-bot",
		BaseURL:      protocol.WeChatAPIBaseURL,
		DisplayName:  "微信 0069***.bot",
		AccountHint:  "0069***.bot",
	}
	storage := &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}
	wechat := &testWeChatService{}
	media := &testMediaService{}
	service := newService(Config{
		APIKey: "test-key",
	}, storage, wechat, media)
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	payload := "audio-bytes"
	result, err := service.UploadMedia(context.Background(), UploadMediaInput{
		ConnectorChannelID: connection.Token,
		Filename:           "meeting.mp3",
		ContentType:        "audio/mpeg",
		Source:             strings.NewReader(payload),
		Size:               int64(len(payload)),
	})
	if err != nil {
		t.Fatalf("UploadMedia returned error: %v", err)
	}
	if result.MediaType != connectorMediaTypeFile {
		t.Fatalf("普通 audio 上传应作为 file 媒体处理，got=%+v", result)
	}
	if len(wechat.uploadRequests) != 1 || wechat.uploadRequests[0].MediaType != ilinkservice.UploadMediaTypeFile {
		t.Fatalf("普通 audio 上传应使用 iLink file media_type，got=%+v", wechat.uploadRequests)
	}
	if len(media.references) != 1 || media.references[0].MediaType != connectorMediaTypeFile || media.references[0].ContentType != "audio/mpeg" {
		t.Fatalf("普通 audio media_ref 异常: %+v", media.references)
	}
}

func TestInvokeToolTypingUsesGetConfigAndSendTyping(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:         "cch_user_a",
		WeChatUserID:  "wx-user-a",
		BotToken:      "bot-token-a",
		BotAccountID:  "0069-test-bot",
		BaseURL:       protocol.WeChatAPIBaseURL,
		DisplayName:   "微信 0069***.bot",
		AccountHint:   "0069***.bot",
		ContextTokens: map[string]string{"wx_user_1": "ctx_msg_1"},
	}
	wechat := &testWeChatService{}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}, wechat, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	result, err := service.InvokeTool(context.Background(), ToolInvokeInput{
		ConnectorChannelID: connection.Token,
		ToolID:             "wechat_typing",
		Arguments:          map[string]any{"reply_token": "wx_user_1", "typing": false},
	})
	if err != nil {
		t.Fatalf("wechat_typing returned error: %v", err)
	}
	if result.Result["typing"] != false {
		t.Fatalf("typing 结果异常: %+v", result.Result)
	}
	if len(wechat.configReads) != 1 || wechat.configReads[0].ILinkUserID != "wx_user_1" || wechat.configReads[0].ContextToken != "ctx_msg_1" {
		t.Fatalf("GetConfig 参数异常: %+v", wechat.configReads)
	}
	if len(wechat.typingEvents) != 1 || wechat.typingEvents[0].ILinkUserID != "wx_user_1" || wechat.typingEvents[0].TypingTicket != "typing-ticket-1" || wechat.typingEvents[0].Status != ilinkservice.TypingStatusCancel {
		t.Fatalf("SendTyping 参数异常: %+v", wechat.typingEvents)
	}
}

func TestBuildInboundMessagePayloadUsesMessagePushFields(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:       "cch_user_a",
		AccountHint: "0069***.bot",
	}
	payload := buildInboundMessagePayload(connection, &ilinkservice.WeixinMessage{
		MessageID:        1001,
		FromUserID:       "wx_user_1",
		DisplayName:      "张三",
		ToUserID:         "bot_1",
		ContextToken:     "ctx_msg_1",
		CreateTimeMillis: 12345,
		MessageType:      ilinkservice.MessageTypeUser,
		MessageState:     ilinkservice.MessageStateFinish,
		ItemList: []*ilinkservice.MessageItem{{
			Type:     ilinkservice.MessageItemTypeText,
			TextItem: &ilinkservice.TextItem{Text: "你好"},
		}},
	})
	if payload["message_id"] != "1001" || payload["sender_id"] != "wx_user_1" || payload["sender_name"] != "张三" || payload["display_name"] != "张三" || payload["raw_text"] != "你好" {
		t.Fatalf("message.push payload 异常: %+v", payload)
	}
	text, ok := payload["text"].(string)
	if !ok {
		t.Fatalf("message.push text 类型异常: %+v", payload["text"])
	}
	for _, expected := range []string{
		"来自微信的用户消息：",
		"发送方：张三",
		"消息类型：文本",
		"用户文本：你好",
		"wechat_message_send 工具",
		"参数 text 是回复内容",
		"im-connector-reply skill",
		"wechat_message_send_media 工具",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("message.push text 缺少 %q: %s", expected, text)
		}
	}
	if text != payload["content"] || text != payload["activation_message"] {
		t.Fatalf("message.push content/activation_message 应与可见正文一致: %+v", payload)
	}
	reply, ok := payload["reply"].(map[string]any)
	if !ok || reply["tool_id"] != "wechat_message_send" {
		t.Fatalf("reply 契约异常: %+v", payload["reply"])
	}
	skill, ok := payload["skill"].(map[string]any)
	if !ok {
		t.Fatalf("skill 契约异常: %+v", payload["skill"])
	}
	requiredToolIDs, ok := skill["required_tool_ids"].([]string)
	if !ok || len(requiredToolIDs) != 2 || requiredToolIDs[0] != "wechat_message_send" || requiredToolIDs[1] != "wechat_message_send_media" {
		t.Fatalf("skill required_tool_ids 异常: %+v", skill["required_tool_ids"])
	}
	if payload["raw_message"] == nil {
		t.Fatalf("payload 应保留 raw_message")
	}
}

func TestBuildInboundMessagePayloadDoesNotExposeSenderIDWithoutDisplayName(t *testing.T) {
	senderID := "o9cq80zlBiUObBAiHNIVI2VUEwB0@im.wechat"
	payload := buildInboundMessagePayload(&connectordomain.Connection{Token: "cch_user_a"}, &ilinkservice.WeixinMessage{
		MessageID:  1001,
		FromUserID: senderID,
		ItemList: []*ilinkservice.MessageItem{{
			Type:     ilinkservice.MessageItemTypeText,
			TextItem: &ilinkservice.TextItem{Text: "你好"},
		}},
	})

	if payload["sender_name"] != "" || payload["display_name"] != "" {
		t.Fatalf("缺少 display_name 时不应使用 sender id 作为展示名: %+v", payload)
	}
	text, ok := payload["text"].(string)
	if !ok {
		t.Fatalf("message.push text 类型异常: %+v", payload["text"])
	}
	if strings.Contains(text, senderID) || strings.Contains(text, "发送方：") {
		t.Fatalf("缺少 display_name 时正文不应泄漏 sender id: %s", text)
	}
}

func TestBuildInboundMessagePayloadRegistersInboundImageMedia(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:       "cch_user_a",
		AccountHint: "0069***.bot",
	}
	media := &testMediaService{}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}, &testWeChatService{}, media)
	payload := service.(*serviceImpl).buildInboundMessagePayload(context.Background(), connection, &ilinkservice.WeixinMessage{
		MessageID:    1001,
		FromUserID:   "wx_user_1",
		ToUserID:     "bot_1",
		ContextToken: "ctx_msg_1",
		ItemList: []*ilinkservice.MessageItem{{
			Type: ilinkservice.MessageItemTypeImage,
			ImageItem: &ilinkservice.ImageItem{
				Media: &ilinkservice.CDNMedia{
					EncryptQueryParam: "download-param-1",
					AESKey:            "base64-key",
					FullURL:           "https://cdn.example/image",
				},
				MidSize: 128,
			},
		}},
	})

	if len(media.inboundRegistrations) != 1 {
		t.Fatalf("图片入站应注册一次 media_ref，got=%+v", media.inboundRegistrations)
	}
	registration := media.inboundRegistrations[0]
	if registration.ConnectorChannelID != connection.Token || registration.PeerRef != "wx_user_1" || registration.MediaType != "image" {
		t.Fatalf("图片 media_ref 注册参数异常: %+v", registration)
	}
	if registration.ContentType != "image/jpeg" || registration.FullURL != "https://cdn.example/image" || registration.AESKeyBase64 != "base64-key" {
		t.Fatalf("图片 CDN 材料注册异常: %+v", registration)
	}
	mediaItems, ok := payload["media"].([]map[string]any)
	if !ok || len(mediaItems) != 1 {
		t.Fatalf("图片 payload 应包含 media 数组，got=%+v", payload["media"])
	}
	item := mediaItems[0]
	if item["type"] != "image" || item["media_ref"] != "media_ref_1" || item["mime_type"] != "image/jpeg" {
		t.Fatalf("图片 media payload 元数据异常: %+v", item)
	}
	if item["download_url"] != connectorprotocol.ConnectorMediaRefPathPrefix+"/media_ref_1" {
		t.Fatalf("图片 media download_url 异常: %+v", item["download_url"])
	}
	if item["byte_size"] != int64(0) {
		t.Fatalf("图片 media byte_size 应保持未知大小，got=%+v", item["byte_size"])
	}
}

func TestBuildInboundMessagePayloadRegistersQuotedImageMediaWhenCurrentMessageHasNoMedia(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:       "cch_user_a",
		AccountHint: "0069***.bot",
	}
	media := &testMediaService{}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}, &testWeChatService{}, media)
	payload := service.(*serviceImpl).buildInboundMessagePayload(context.Background(), connection, &ilinkservice.WeixinMessage{
		MessageID:  1002,
		FromUserID: "wx_user_1",
		ItemList: []*ilinkservice.MessageItem{{
			Type:     ilinkservice.MessageItemTypeText,
			TextItem: &ilinkservice.TextItem{Text: "帮我看看这张图"},
			RefMessage: &ilinkservice.RefMessage{
				Title: "图片消息",
				MessageItem: &ilinkservice.MessageItem{
					Type: ilinkservice.MessageItemTypeImage,
					ImageItem: &ilinkservice.ImageItem{
						Media: &ilinkservice.CDNMedia{
							EncryptQueryParam: "quoted-download-param",
							AESKey:            "quoted-base64-key",
							FullURL:           "https://cdn.example/quoted-image",
						},
						MidSize: 256,
					},
				},
			},
		}},
	})

	if len(media.inboundRegistrations) != 1 {
		t.Fatalf("当前消息无媒体时应注册引用图片 media_ref，got=%+v", media.inboundRegistrations)
	}
	registration := media.inboundRegistrations[0]
	if registration.DownloadParam != "quoted-download-param" || registration.FullURL != "https://cdn.example/quoted-image" || registration.AESKeyBase64 != "quoted-base64-key" {
		t.Fatalf("引用图片 CDN 材料注册异常: %+v", registration)
	}
	mediaItems, ok := payload["media"].([]map[string]any)
	if !ok || len(mediaItems) != 1 || mediaItems[0]["type"] != "image" {
		t.Fatalf("引用图片 payload 应包含 image media，got=%+v", payload["media"])
	}
	text, ok := payload["text"].(string)
	if !ok || !strings.Contains(text, "[引用: 图片消息]") || !strings.Contains(text, "用户文本：") {
		t.Fatalf("引用文本应保留在消息正文中，got=%+v", payload["text"])
	}
}

func TestBuildInboundMessagePayloadPrefersCurrentMediaOverQuotedMedia(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:       "cch_user_a",
		AccountHint: "0069***.bot",
	}
	media := &testMediaService{}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}, &testWeChatService{}, media)
	service.(*serviceImpl).buildInboundMessagePayload(context.Background(), connection, &ilinkservice.WeixinMessage{
		MessageID:  1003,
		FromUserID: "wx_user_1",
		ItemList: []*ilinkservice.MessageItem{
			{
				Type: ilinkservice.MessageItemTypeImage,
				ImageItem: &ilinkservice.ImageItem{
					Media: &ilinkservice.CDNMedia{
						EncryptQueryParam: "current-download-param",
						FullURL:           "https://cdn.example/current-image",
					},
				},
			},
			{
				Type:     ilinkservice.MessageItemTypeText,
				TextItem: &ilinkservice.TextItem{Text: "当前也有图"},
				RefMessage: &ilinkservice.RefMessage{
					MessageItem: &ilinkservice.MessageItem{
						Type: ilinkservice.MessageItemTypeImage,
						ImageItem: &ilinkservice.ImageItem{
							Media: &ilinkservice.CDNMedia{
								EncryptQueryParam: "quoted-download-param",
								FullURL:           "https://cdn.example/quoted-image",
							},
						},
					},
				},
			},
		},
	})

	if len(media.inboundRegistrations) != 1 {
		t.Fatalf("当前消息已有媒体时不应再注册引用媒体，got=%+v", media.inboundRegistrations)
	}
	if media.inboundRegistrations[0].DownloadParam != "current-download-param" {
		t.Fatalf("应优先注册当前消息媒体，got=%+v", media.inboundRegistrations[0])
	}
}

func TestBuildInboundMessagePayloadPrefersImageItemAESKey(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:       "cch_user_a",
		AccountHint: "0069***.bot",
	}
	media := &testMediaService{}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}, &testWeChatService{}, media)
	service.(*serviceImpl).buildInboundMessagePayload(context.Background(), connection, &ilinkservice.WeixinMessage{
		MessageID:  1001,
		FromUserID: "wx_user_1",
		ToUserID:   "bot_1",
		ItemList: []*ilinkservice.MessageItem{{
			Type: ilinkservice.MessageItemTypeImage,
			ImageItem: &ilinkservice.ImageItem{
				Media: &ilinkservice.CDNMedia{
					EncryptQueryParam: "download-param-1",
					AESKey:            "media-base64-key",
					FullURL:           "https://cdn.example/image",
				},
				AESKey:  "31323334353637383930616263646566",
				MidSize: 128,
			},
		}},
	})

	if len(media.inboundRegistrations) != 1 {
		t.Fatalf("图片入站应注册一次 media_ref，got=%+v", media.inboundRegistrations)
	}
	expected := base64.StdEncoding.EncodeToString([]byte("1234567890abcdef"))
	if media.inboundRegistrations[0].AESKeyBase64 != expected {
		t.Fatalf("图片应优先使用 image_item.aeskey，got=%q want=%q", media.inboundRegistrations[0].AESKeyBase64, expected)
	}
}

func TestBuildInboundMessagePayloadUsesVoiceTranscriptWithoutMediaRef(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:       "cch_user_a",
		AccountHint: "0069***.bot",
	}
	media := &testMediaService{}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}, &testWeChatService{}, media)
	payload := service.(*serviceImpl).buildInboundMessagePayload(context.Background(), connection, &ilinkservice.WeixinMessage{
		MessageID:  1002,
		FromUserID: "wx_user_1",
		ItemList: []*ilinkservice.MessageItem{{
			Type: ilinkservice.MessageItemTypeVoice,
			VoiceItem: &ilinkservice.VoiceItem{
				Text: "帮我查一下今天的日程",
				Media: &ilinkservice.CDNMedia{
					EncryptQueryParam: "voice-download-param",
					AESKey:            "voice-base64-key",
					FullURL:           "https://cdn.example/voice",
				},
			},
		}},
	})

	text, ok := payload["text"].(string)
	if !ok || !strings.Contains(text, "消息类型：语音") || !strings.Contains(text, "用户文本：帮我查一下今天的日程") || payload["content"] != text || payload["raw_text"] != "帮我查一下今天的日程" {
		t.Fatalf("语音入站应使用微信转写文本，got=%+v", payload)
	}
	if _, exists := payload["media"]; exists {
		t.Fatalf("微信 voice 不应作为普通音频附件传给 xAgent，got=%+v", payload["media"])
	}
	if len(media.inboundRegistrations) != 0 {
		t.Fatalf("微信 voice 不应注册 media_ref，got=%+v", media.inboundRegistrations)
	}
}

func TestBuildInboundMessagePayloadUsesVoiceNoTranscriptNotice(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:       "cch_user_a",
		AccountHint: "0069***.bot",
	}
	media := &testMediaService{}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}, &testWeChatService{}, media)
	payload := service.(*serviceImpl).buildInboundMessagePayload(context.Background(), connection, &ilinkservice.WeixinMessage{
		MessageID:  1003,
		FromUserID: "wx_user_1",
		ItemList: []*ilinkservice.MessageItem{{
			Type: ilinkservice.MessageItemTypeVoice,
			VoiceItem: &ilinkservice.VoiceItem{
				Media: &ilinkservice.CDNMedia{
					EncryptQueryParam: "voice-download-param",
					AESKey:            "voice-base64-key",
					FullURL:           "https://cdn.example/voice",
				},
			},
		}},
	})

	text, ok := payload["text"].(string)
	if !ok || !strings.Contains(text, "消息类型：语音") || !strings.Contains(text, "用户文本："+inboundVoiceNoTranscriptText) || payload["content"] != text || payload["raw_text"] != inboundVoiceNoTranscriptText {
		t.Fatalf("未转写语音应转成明确文本事件，got=%+v", payload)
	}
	if _, exists := payload["media"]; exists {
		t.Fatalf("未转写微信 voice 不应作为普通音频附件传给 xAgent，got=%+v", payload["media"])
	}
	if len(media.inboundRegistrations) != 0 {
		t.Fatalf("未转写微信 voice 不应注册 media_ref，got=%+v", media.inboundRegistrations)
	}
}

func TestFlushInboundMessagesDeliversPendingAndAdvancesCursor(t *testing.T) {
	storage := &testStorageService{
		connections: map[string]*connectordomain.Connection{},
		pending: []*connectordomain.PendingInboundMessage{{
			ID:                 "cch_user_a_mid_1",
			ConnectorChannelID: "cch_user_a",
			Payload:            map[string]any{"text": "hello"},
			ReceivedAt:         100,
			ExpiresAt:          time.Now().Add(time.Hour).UnixMilli(),
		}},
	}
	service := newService(Config{
		APIKey: "test-key",
	}, storage, &testWeChatService{}, &testMediaService{})
	pusher := &testMessagePusher{}
	service.BindMessagePusher(pusher)

	service.FlushInboundMessages(context.Background(), "cch_user_a")

	if len(pusher.delivered) != 1 || pusher.delivered[0]["text"] != "hello" {
		t.Fatalf("pending 消息投递异常: %+v", pusher.delivered)
	}
	if len(storage.pending) != 0 {
		t.Fatalf("投递成功后 pending 应删除: %+v", storage.pending)
	}
	if storage.channels["cch_user_a"] == nil || storage.channels["cch_user_a"].LastDeliveredMessageID != "cch_user_a_mid_1" {
		t.Fatalf("投递游标异常: %+v", storage.channels["cch_user_a"])
	}
}

func TestFlushInboundMessagesStartsAutoTypingAfterDelivered(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:         "cch_user_a",
		WeChatUserID:  "wx_user_1",
		BotToken:      "bot-token-a",
		BotAccountID:  "0069-test-bot",
		BaseURL:       protocol.WeChatAPIBaseURL,
		DisplayName:   "微信 0069***.bot",
		AccountHint:   "0069***.bot",
		ContextTokens: map[string]string{"wx_user_1": "ctx_msg_1"},
	}
	storage := &testStorageService{
		connections: map[string]*connectordomain.Connection{connection.Token: connection},
		pending: []*connectordomain.PendingInboundMessage{{
			ID:                 "cch_user_a_mid_1",
			ConnectorChannelID: "cch_user_a",
			Payload: map[string]any{
				"sender_id":     "wx_user_1",
				"context_token": "ctx_msg_1",
				"text":          "hello",
			},
			ReceivedAt: 100,
			ExpiresAt:  time.Now().Add(time.Hour).UnixMilli(),
		}},
	}
	wechat := &testWeChatService{}
	service := newService(Config{
		APIKey: "test-key",
	}, storage, wechat, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() {
		_ = service.Stop(context.Background())
	}()
	pusher := &testMessagePusher{}
	service.BindMessagePusher(pusher)

	service.FlushInboundMessages(context.Background(), connection.Token)

	events := wechat.typingEventSnapshot()
	if len(events) != 1 || events[0].ILinkUserID != "wx_user_1" || events[0].TypingTicket != "typing-ticket-1" || events[0].Status != ilinkservice.TypingStatusTyping {
		t.Fatalf("投递成功后应发送输入中状态，got=%+v", events)
	}
	configReads := wechat.configReadSnapshot()
	if len(configReads) != 1 || configReads[0].ILinkUserID != "wx_user_1" || configReads[0].ContextToken != "ctx_msg_1" {
		t.Fatalf("输入状态配置读取参数异常: %+v", configReads)
	}
}

func TestMonitorClearsConnectionBindingOnStaleBotToken(t *testing.T) {
	connection := &connectordomain.Connection{
		Token:        "cch_user_a",
		WeChatUserID: "wx_user_1",
		BotToken:     "bot-token-a",
		BotAccountID: "0069-test-bot",
		BaseURL:      protocol.WeChatAPIBaseURL,
		DisplayName:  "微信 0069***.bot",
		AccountHint:  "0069***.bot",
	}
	storage := &testStorageService{
		connections: map[string]*connectordomain.Connection{connection.Token: connection},
	}
	wechat := &testWeChatService{
		getUpdates: []*ilinkservice.GetUpdatesResponse{{
			Ret:     staleTokenErrCode,
			ErrCode: staleTokenErrCode,
			ErrMsg:  "stale token",
		}},
	}
	service := newService(Config{
		APIKey: "test-key",
	}, storage, wechat, &testMediaService{})
	pusher := &testMessagePusher{}
	service.BindMessagePusher(pusher)
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() {
		_ = service.Stop(context.Background())
	}()

	deadline := time.After(time.Second)
	for {
		descriptors := pusher.descriptorSnapshot()
		if len(descriptors) > 0 {
			descriptor := descriptors[0]
			if descriptor.Connection.Status != connectorprotocol.ConnectionStatusCreated {
				t.Fatalf("stale token 应推送未绑定 descriptor，got=%+v", descriptor.Connection)
			}
			if descriptor.Target.DisplayName != "未绑定微信" {
				t.Fatalf("stale token 后 descriptor 应清空目标账号展示，got=%+v", descriptor.Target)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("等待未绑定 descriptor push 超时")
		case <-time.After(time.Millisecond):
		}
	}

	if connection := service.ConnectionByChannel("cch_user_a"); connection != nil {
		t.Fatalf("stale token 后 connector 本地 connection 应清空，got=%+v", connection)
	}
	if storage.channels["cch_user_a"] != nil {
		t.Fatalf("stale token 后 connector 本地 connection 绑定应清空，got=%+v", storage.channels["cch_user_a"])
	}
	if storage.bots["0069-test-bot"] != nil {
		t.Fatalf("stale token 后孤儿 bot 应清空，got=%+v", storage.bots["0069-test-bot"])
	}
}

func TestAutoTypingTimeoutCancelsState(t *testing.T) {
	previousTimeout := autoTypingTimeout
	autoTypingTimeout = 10 * time.Millisecond
	defer func() {
		autoTypingTimeout = previousTimeout
	}()
	connection := &connectordomain.Connection{
		Token:         "cch_user_a",
		WeChatUserID:  "wx_user_1",
		BotToken:      "bot-token-a",
		BotAccountID:  "0069-test-bot",
		BaseURL:       protocol.WeChatAPIBaseURL,
		DisplayName:   "微信 0069***.bot",
		AccountHint:   "0069***.bot",
		ContextTokens: map[string]string{"wx_user_1": "ctx_msg_1"},
	}
	wechat := &testWeChatService{}
	service := newService(Config{
		APIKey: "test-key",
	}, &testStorageService{connections: map[string]*connectordomain.Connection{connection.Token: connection}}, wechat, &testMediaService{})
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() {
		_ = service.Stop(context.Background())
	}()

	service.(*serviceImpl).startAutoTypingForDeliveredMessage(context.Background(), connection.Token, map[string]any{
		"sender_id":     "wx_user_1",
		"context_token": "ctx_msg_1",
	})
	deadline := time.Now().Add(time.Second)
	var events []ilinkservice.SendTypingInput
	for time.Now().Before(deadline) {
		events = wechat.typingEventSnapshot()
		if len(events) >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(events) != 2 || events[0].Status != ilinkservice.TypingStatusTyping || events[1].Status != ilinkservice.TypingStatusCancel {
		t.Fatalf("自动输入状态超时后应取消，got=%+v", events)
	}
}
