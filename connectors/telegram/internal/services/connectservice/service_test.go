package connectservice

import (
	"context"
	"strings"
	"sync"
	"testing"

	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/telegramservice"
)

type testStorageService struct {
	mu       sync.Mutex
	bots     map[string]*connectordomain.Bot
	bindings map[string]*connectordomain.ConnectionBinding
}

func (service *testStorageService) Start(context.Context) error {
	return nil
}

func (service *testStorageService) Stop(context.Context) error {
	return nil
}

func (service *testStorageService) StateDir() string {
	return "/tmp/xagent-telegram-connector-test"
}

func (service *testStorageService) LoadBots() (map[string]*connectordomain.Bot, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return cloneTestBotMap(service.bots), nil
}

func (service *testStorageService) SaveBots(bots map[string]*connectordomain.Bot) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.bots = cloneTestBotMap(bots)
	return nil
}

func (service *testStorageService) SaveBot(bot *connectordomain.Bot) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.bots == nil {
		service.bots = map[string]*connectordomain.Bot{}
	}
	service.bots[bot.BotID] = cloneBot(bot)
	return nil
}

func (service *testStorageService) LoadConnectionBindings() (map[string]*connectordomain.ConnectionBinding, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return cloneTestBindingMap(service.bindings), nil
}

func (service *testStorageService) SaveConnectionBindings(bindings map[string]*connectordomain.ConnectionBinding) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.bindings = cloneTestBindingMap(bindings)
	return nil
}

func (service *testStorageService) SaveConnectionBinding(binding *connectordomain.ConnectionBinding) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.bindings == nil {
		service.bindings = map[string]*connectordomain.ConnectionBinding{}
	}
	service.bindings[binding.ConnectorChannelID] = cloneBinding(binding)
	return nil
}

func (service *testStorageService) DeleteConnectionBinding(connectorChannelID string) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	delete(service.bindings, connectorChannelID)
	return nil
}

func (service *testStorageService) SaveMediaReference(reference *connectordomain.MediaReference) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	return nil
}

func (service *testStorageService) GetMediaReference(string, int64) (*connectordomain.MediaReference, error) {
	return nil, nil
}

func (service *testStorageService) PruneExpiredMediaReferences(int64) (int, error) {
	return 0, nil
}

type testTelegramService struct {
	getMe    *telegramservice.User
	getChat  *telegramservice.Chat
	messages []telegramservice.SendMessageInput
}

func (service *testTelegramService) Start(context.Context) error {
	return nil
}

func (service *testTelegramService) Stop(context.Context) error {
	return nil
}

func (service *testTelegramService) APIBaseURL() string {
	return "https://api.telegram.org"
}

func (service *testTelegramService) GetMe(context.Context, string) (*telegramservice.User, error) {
	return service.getMe, nil
}

func (service *testTelegramService) GetChat(context.Context, telegramservice.GetChatInput) (*telegramservice.Chat, error) {
	return service.getChat, nil
}

func (service *testTelegramService) GetUpdates(ctx context.Context, _ telegramservice.GetUpdatesInput) (*telegramservice.GetUpdatesResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (service *testTelegramService) SendMessage(_ context.Context, input telegramservice.SendMessageInput) (*telegramservice.Message, error) {
	service.messages = append(service.messages, input)
	return &telegramservice.Message{MessageID: 77}, nil
}

func (service *testTelegramService) SendMedia(context.Context, telegramservice.SendMediaInput) (*telegramservice.Message, error) {
	return &telegramservice.Message{MessageID: 78}, nil
}

func (service *testTelegramService) GetFile(context.Context, telegramservice.GetFileInput) (*telegramservice.File, error) {
	return &telegramservice.File{FilePath: "documents/file.bin"}, nil
}

func (service *testTelegramService) DownloadFile(context.Context, telegramservice.DownloadFileInput) (*telegramservice.DownloadFileResult, error) {
	return &telegramservice.DownloadFileResult{ContentType: "application/octet-stream", Body: []byte("file")}, nil
}

type testMessagePusher struct {
	messages []testPushedMessage
}

type testPushedMessage struct {
	connectorChannelID string
	payload            map[string]any
}

func (pusher *testMessagePusher) PushMessage(_ context.Context, connectorChannelID string, payload map[string]any) error {
	pusher.messages = append(pusher.messages, testPushedMessage{
		connectorChannelID: connectorChannelID,
		payload:            payload,
	})
	return nil
}

func (pusher *testMessagePusher) PushConnectionDescriptor(context.Context, string, *connectorprotocol.ConnectionDescriptor) error {
	return nil
}

func TestBuildConnectorCardDeclaresFormAuth(t *testing.T) {
	service := newService(Config{APIKey: "test-key"}, &testStorageService{}, &testTelegramService{})
	card := service.BuildConnectorCard()
	if card == nil {
		t.Fatalf("BuildConnectorCard returned nil")
	}
	if card.ConnectorCardID != protocol.ConnectorCardID {
		t.Fatalf("unexpected connector_card_id: %s", card.ConnectorCardID)
	}
	if card.Supports.UserChannelMode != connectorprotocol.ConnectorUserChannelModeSingle {
		t.Fatalf("Telegram Connector Card must declare single user channel mode, got=%s", card.Supports.UserChannelMode)
	}
	if len(card.Tools) != 2 {
		t.Fatalf("unexpected tool count: %d", len(card.Tools))
	}
	if card.Tools[0].ToolID != toolIDTelegramMessageSend || card.Tools[1].ToolID != toolIDTelegramMessageSendMedia {
		t.Fatalf("unexpected tools: %+v", card.Tools)
	}
	if err := connectorprotocol.ValidateConnectorCardToolInputSchemas(card); err != nil {
		t.Fatalf("connector card tool input schemas invalid: %v", err)
	}
	if len(card.Tools[0].RelatedSkillIDs) != 1 || card.Tools[0].RelatedSkillIDs[0] != protocol.ConnectorSkillIMReplyID {
		t.Fatalf("text tool related skill ids mismatch: %+v", card.Tools[0].RelatedSkillIDs)
	}
	if len(card.Tools[1].RelatedSkillIDs) != 1 || card.Tools[1].RelatedSkillIDs[0] != protocol.ConnectorSkillIMReplyID {
		t.Fatalf("media tool related skill ids mismatch: %+v", card.Tools[1].RelatedSkillIDs)
	}
	if !strings.Contains(card.Tools[0].Description, "纯文本消息") || !strings.Contains(card.Tools[0].Description, toolIDTelegramMessageSendMedia) {
		t.Fatalf("text tool description should direct media elsewhere: %s", card.Tools[0].Description)
	}
	if !strings.Contains(card.Tools[0].Description, protocol.ConnectorSkillIMReplyID) {
		t.Fatalf("text tool description should point to connector skill: %s", card.Tools[0].Description)
	}
	textSchemaDescription, _ := card.Tools[0].InputSchema["description"].(string)
	if !strings.Contains(textSchemaDescription, protocol.ConnectorSkillIMReplyID) || !strings.Contains(textSchemaDescription, toolIDTelegramMessageSendMedia) {
		t.Fatalf("text tool input schema should direct attachments to skill and media tool: %s", textSchemaDescription)
	}
	textProperties, ok := card.Tools[0].InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("text tool properties schema mismatch: %+v", card.Tools[0].InputSchema["properties"])
	}
	textProperty, ok := textProperties["text"].(map[string]any)
	textPropertyDescription, _ := textProperty["description"].(string)
	if !ok || !strings.Contains(textPropertyDescription, protocol.ConnectorSkillIMReplyID) {
		t.Fatalf("text field schema should mention connector skill: %+v", textProperty)
	}
	assertConnectorChannelIDInputSchema(t, card.Tools[0])
	textRequired := schemaRequiredStrings(t, card.Tools[0].InputSchema)
	if !containsTestString(textRequired, "text") || !containsTestString(textRequired, connectorprotocol.ConnectorToolParamConnectorChannelID) {
		t.Fatalf("text tool required schema mismatch: %+v", textRequired)
	}
	if !strings.Contains(card.Tools[1].Description, "POST /media/uploads") || !strings.Contains(card.Tools[1].Description, "X-XAgent-Connector-Channel-ID") || !strings.Contains(card.Tools[1].Description, "Telegram file_id") {
		t.Fatalf("media tool description should explain upload workflow and forbidden args: %s", card.Tools[1].Description)
	}
	assertConnectorChannelIDInputSchema(t, card.Tools[1])
	required := schemaRequiredStrings(t, card.Tools[1].InputSchema)
	if !containsTestString(required, "media_ref") || !containsTestString(required, connectorprotocol.ConnectorToolParamConnectorChannelID) {
		t.Fatalf("media tool required schema mismatch: %+v", required)
	}
	properties, ok := card.Tools[1].InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("media tool properties schema mismatch: %+v", card.Tools[1].InputSchema["properties"])
	}
	mediaRefProperty, ok := properties["media_ref"].(map[string]any)
	mediaRefDescription, _ := mediaRefProperty["description"].(string)
	if !ok || !strings.Contains(mediaRefDescription, "payload.media[].media_ref") {
		t.Fatalf("media_ref schema should mention inbound media reuse: %+v", mediaRefProperty)
	}
	if len(card.AuthFlows) != 1 {
		t.Fatalf("unexpected auth flow count: %d", len(card.AuthFlows))
	}
	flow := card.AuthFlows[0]
	if flow.Type != connectorprotocol.ConnectorAuthFlowTypeForm {
		t.Fatalf("unexpected auth flow type: %s", flow.Type)
	}
	if len(flow.Fields) != 2 {
		t.Fatalf("unexpected field count: %d", len(flow.Fields))
	}
	if flow.Fields[0].Name != "bot_token" || flow.Fields[0].InputType != connectorprotocol.ConnectorAuthInputTypePassword || !flow.Fields[0].Secret {
		t.Fatalf("bot_token field mismatch: %+v", flow.Fields[0])
	}
	if flow.Fields[1].Name != "chat_id" || flow.Fields[1].InputType != connectorprotocol.ConnectorAuthInputTypeText {
		t.Fatalf("chat_id field mismatch: %+v", flow.Fields[1])
	}
}

func TestReadConnectorSkillDocumentsAttachmentToolSelection(t *testing.T) {
	service := newService(Config{APIKey: "test-key"}, &testStorageService{}, &testTelegramService{})
	content, err := service.ReadConnectorSkill(context.Background())
	if err != nil {
		t.Fatalf("ReadConnectorSkill failed: %v", err)
	}
	if !strings.Contains(content.Content, "do not call telegram_message_send") {
		t.Fatalf("skill should forbid text tool for attachment replies")
	}
	if !strings.Contains(content.Content, "POST /media/uploads") || !strings.Contains(content.Content, "X-XAgent-Connector-Channel-ID") {
		t.Fatalf("skill should document media upload endpoint and channel header")
	}
	if !strings.Contains(content.Content, "telegram_message_send is the wrong tool") {
		t.Fatalf("skill should declare attachment tool selection guard")
	}
	if !strings.Contains(content.Content, "This skill owns the Telegram attachment sending workflow") {
		t.Fatalf("skill should declare attachment workflow ownership")
	}
}

func assertConnectorChannelIDInputSchema(t *testing.T, tool connectorprotocol.ConnectorToolDescriptor) {
	t.Helper()
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool %s properties schema mismatch: %+v", tool.ToolID, tool.InputSchema["properties"])
	}
	channelProperty, ok := properties[connectorprotocol.ConnectorToolParamConnectorChannelID].(map[string]any)
	if !ok {
		t.Fatalf("tool %s missing connector_channel_id property: %+v", tool.ToolID, properties)
	}
	if channelProperty["type"] != "string" {
		t.Fatalf("tool %s connector_channel_id must be string: %+v", tool.ToolID, channelProperty)
	}
	required := schemaRequiredStrings(t, tool.InputSchema)
	if !containsTestString(required, connectorprotocol.ConnectorToolParamConnectorChannelID) {
		t.Fatalf("tool %s connector_channel_id must be required: %+v", tool.ToolID, required)
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
				t.Fatalf("required item should be string: %+v", values)
			}
			output = append(output, item)
		}
		return output
	default:
		t.Fatalf("required schema mismatch: %+v", schema["required"])
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

func TestStartAuthBindsTelegramBotAndChat(t *testing.T) {
	storage := &testStorageService{
		bots:     map[string]*connectordomain.Bot{},
		bindings: map[string]*connectordomain.ConnectionBinding{},
	}
	telegram := &testTelegramService{
		getMe: &telegramservice.User{
			ID:        123,
			IsBot:     true,
			FirstName: "xAgent Bot",
			Username:  "xagent_bot",
		},
		getChat: &telegramservice.Chat{
			ID:        987,
			Type:      "private",
			FirstName: "Coffee",
		},
	}
	service := newService(Config{APIKey: "test-key"}, storage, telegram)
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop(context.Background())

	result, err := service.StartAuth(context.Background(), "cch_a", connectorprotocol.ConnectorAuthStartRequest{
		FlowID: protocol.TelegramBotBindingFlowID,
		Input: map[string]string{
			"bot_token": "123:secret",
			"chat_id":   "987",
		},
	})
	if err != nil {
		t.Fatalf("StartAuth failed: %v", err)
	}
	if result.Status != connectorprotocol.ConnectorAuthStartAuthenticated {
		t.Fatalf("unexpected auth status: %s", result.Status)
	}
	if result.ConnectionDescriptor == nil || result.ConnectionDescriptor.Target.Provider != "telegram" {
		t.Fatalf("connection descriptor mismatch: %+v", result.ConnectionDescriptor)
	}
	if storage.bots["123"] == nil || storage.bots["123"].BotToken != "123:secret" {
		t.Fatalf("bot was not saved: %+v", storage.bots["123"])
	}
	if storage.bindings["cch_a"] == nil || storage.bindings["cch_a"].ChatID != "987" {
		t.Fatalf("binding was not saved: %+v", storage.bindings["cch_a"])
	}
	connection := service.ConnectionByChannel("cch_a")
	if connection == nil || connection.BotID != "123" || connection.ChatID != "987" {
		t.Fatalf("runtime connection mismatch: %+v", connection)
	}
}

func TestStartAuthNormalizesChatIDFromGetChat(t *testing.T) {
	storage := &testStorageService{
		bots:     map[string]*connectordomain.Bot{},
		bindings: map[string]*connectordomain.ConnectionBinding{},
	}
	telegram := &testTelegramService{
		getMe: &telegramservice.User{
			ID:        123,
			IsBot:     true,
			FirstName: "xAgent Bot",
			Username:  "xagent_bot",
		},
		getChat: &telegramservice.Chat{
			ID:       -100987,
			Type:     "supergroup",
			Title:    "Ops",
			Username: "ops_chat",
		},
	}
	service := newService(Config{APIKey: "test-key"}, storage, telegram)
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop(context.Background())

	_, err := service.StartAuth(context.Background(), "cch_group", connectorprotocol.ConnectorAuthStartRequest{
		FlowID: protocol.TelegramBotBindingFlowID,
		Input: map[string]string{
			"bot_token": "123:secret",
			"chat_id":   "@ops_chat",
		},
	})
	if err != nil {
		t.Fatalf("StartAuth failed: %v", err)
	}
	if storage.bindings["cch_group"] == nil || storage.bindings["cch_group"].ChatID != "-100987" {
		t.Fatalf("binding should store normalized chat id: %+v", storage.bindings["cch_group"])
	}
}

func TestStartAuthRejectsBotSelfChatID(t *testing.T) {
	storage := &testStorageService{
		bots:     map[string]*connectordomain.Bot{},
		bindings: map[string]*connectordomain.ConnectionBinding{},
	}
	telegram := &testTelegramService{
		getMe: &telegramservice.User{
			ID:        123,
			IsBot:     true,
			FirstName: "xAgent Bot",
			Username:  "xagent_bot",
		},
		getChat: &telegramservice.Chat{
			ID:        123,
			Type:      "private",
			FirstName: "xAgent Bot",
		},
	}
	service := newService(Config{APIKey: "test-key"}, storage, telegram)
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop(context.Background())

	_, err := service.StartAuth(context.Background(), "cch_bot", connectorprotocol.ConnectorAuthStartRequest{
		FlowID: protocol.TelegramBotBindingFlowID,
		Input: map[string]string{
			"bot_token": "123:secret",
			"chat_id":   "123",
		},
	})
	if err == nil {
		t.Fatalf("StartAuth should reject bot self chat id")
	}
}

func TestDispatchTelegramUpdateBuildsVisibleReplyPrompt(t *testing.T) {
	storage := &testStorageService{
		bots:     map[string]*connectordomain.Bot{},
		bindings: map[string]*connectordomain.ConnectionBinding{},
	}
	telegram := &testTelegramService{
		getMe: &telegramservice.User{
			ID:        123,
			IsBot:     true,
			FirstName: "xAgent Bot",
			Username:  "xagent_bot",
		},
		getChat: &telegramservice.Chat{
			ID:        987,
			Type:      "private",
			FirstName: "Coffee",
		},
	}
	service := newService(Config{APIKey: "test-key"}, storage, telegram)
	impl := service.(*serviceImpl)
	pusher := &testMessagePusher{}
	impl.BindMessagePusher(pusher)
	if err := impl.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer impl.Stop(context.Background())
	_, err := impl.StartAuth(context.Background(), "cch_a", connectorprotocol.ConnectorAuthStartRequest{
		FlowID: protocol.TelegramBotBindingFlowID,
		Input: map[string]string{
			"bot_token": "123:secret",
			"chat_id":   "987",
		},
	})
	if err != nil {
		t.Fatalf("StartAuth failed: %v", err)
	}

	impl.dispatchTelegramUpdate(context.Background(), "123", &telegramservice.Update{
		UpdateID: 10,
		Message: &telegramservice.Message{
			MessageID: 55,
			From: &telegramservice.User{
				ID:        321,
				FirstName: "Coffee",
			},
			Chat: &telegramservice.Chat{
				ID:   987,
				Type: "private",
			},
			Text: "？",
		},
	})
	if len(pusher.messages) != 1 {
		t.Fatalf("expected one pushed message, got %d", len(pusher.messages))
	}
	if pusher.messages[0].connectorChannelID != "cch_a" {
		t.Fatalf("unexpected connector channel: %s", pusher.messages[0].connectorChannelID)
	}
	payload := pusher.messages[0].payload
	visibleText, _ := payload["text"].(string)
	if !strings.Contains(visibleText, "来自 Telegram 的用户消息：") {
		t.Fatalf("visible text missing Telegram source: %s", visibleText)
	}
	if !strings.Contains(visibleText, "用户文本：？") {
		t.Fatalf("visible text missing user text: %s", visibleText)
	}
	if !strings.Contains(visibleText, "文本回复使用 telegram_message_send 工具") {
		t.Fatalf("visible text missing reply tool instruction: %s", visibleText)
	}
	if !strings.Contains(visibleText, "telegram_message_send_media 工具") || !strings.Contains(visibleText, "media_ref") {
		t.Fatalf("visible text missing media reply instruction: %s", visibleText)
	}
	if payload["raw_text"] != "？" || payload["content"] != visibleText || payload["activation_message"] != visibleText {
		t.Fatalf("payload text fields mismatch: %+v", payload)
	}
	reply, _ := payload["reply"].(map[string]any)
	if reply["tool_id"] != toolIDTelegramMessageSend {
		t.Fatalf("reply tool mismatch: %+v", reply)
	}
}

func TestDispatchTelegramUpdateBuildsMediaRefsForPhotoDocumentAndVideo(t *testing.T) {
	storage := &recordingStorageService{
		testStorageService: testStorageService{
			bots:     map[string]*connectordomain.Bot{},
			bindings: map[string]*connectordomain.ConnectionBinding{},
		},
	}
	telegram := &testTelegramService{
		getMe: &telegramservice.User{
			ID:        123,
			IsBot:     true,
			FirstName: "xAgent Bot",
			Username:  "xagent_bot",
		},
		getChat: &telegramservice.Chat{
			ID:        987,
			Type:      "private",
			FirstName: "Coffee",
		},
	}
	service := newService(Config{APIKey: "test-key"}, storage, telegram)
	impl := service.(*serviceImpl)
	pusher := &testMessagePusher{}
	impl.BindMessagePusher(pusher)
	if err := impl.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer impl.Stop(context.Background())
	_, err := impl.StartAuth(context.Background(), "cch_a", connectorprotocol.ConnectorAuthStartRequest{
		FlowID: protocol.TelegramBotBindingFlowID,
		Input: map[string]string{
			"bot_token": "123:secret",
			"chat_id":   "987",
		},
	})
	if err != nil {
		t.Fatalf("StartAuth failed: %v", err)
	}

	impl.dispatchTelegramUpdate(context.Background(), "123", &telegramservice.Update{
		UpdateID: 10,
		Message: &telegramservice.Message{
			MessageID: 55,
			From:      &telegramservice.User{ID: 321, FirstName: "Coffee"},
			Chat:      &telegramservice.Chat{ID: 987, Type: "private"},
			Photo: []*telegramservice.PhotoSize{
				{FileID: "photo-small", Width: 10, Height: 10, FileSize: 100},
				{FileID: "photo-large", Width: 100, Height: 100, FileSize: 1000},
			},
			Document: &telegramservice.Document{FileID: "doc-file", FileName: "a.pdf", MimeType: "application/pdf", FileSize: 2048},
			Video:    &telegramservice.Video{FileID: "video-file", FileName: "a.mp4", MimeType: "video/mp4", FileSize: 4096},
		},
	})
	if len(pusher.messages) != 1 {
		t.Fatalf("expected one pushed message, got %d", len(pusher.messages))
	}
	payload := pusher.messages[0].payload
	media, ok := payload["media"].([]map[string]any)
	if !ok {
		t.Fatalf("media payload missing: %+v", payload["media"])
	}
	if len(media) != 3 {
		t.Fatalf("unexpected media count: %d", len(media))
	}
	if storage.mediaTypes() != "image,file,video" {
		t.Fatalf("unexpected stored media types: %s", storage.mediaTypes())
	}
	visibleText, _ := payload["text"].(string)
	if !strings.Contains(visibleText, "消息类型：图片+文件+视频") {
		t.Fatalf("visible text missing media types: %s", visibleText)
	}
}

type recordingStorageService struct {
	testStorageService
	media []*connectordomain.MediaReference
}

func (service *recordingStorageService) SaveMediaReference(reference *connectordomain.MediaReference) error {
	service.media = append(service.media, cloneMediaReference(reference))
	return nil
}

func (service *recordingStorageService) mediaTypes() string {
	values := []string{}
	for _, reference := range service.media {
		values = append(values, reference.MediaType)
	}
	return strings.Join(values, ",")
}

func cloneMediaReference(reference *connectordomain.MediaReference) *connectordomain.MediaReference {
	if reference == nil {
		return nil
	}
	cloned := *reference
	return &cloned
}

func cloneTestBotMap(input map[string]*connectordomain.Bot) map[string]*connectordomain.Bot {
	output := map[string]*connectordomain.Bot{}
	for key, value := range input {
		output[key] = cloneBot(value)
	}
	return output
}

func cloneTestBindingMap(input map[string]*connectordomain.ConnectionBinding) map[string]*connectordomain.ConnectionBinding {
	output := map[string]*connectordomain.ConnectionBinding{}
	for key, value := range input {
		output[key] = cloneBinding(value)
	}
	return output
}
