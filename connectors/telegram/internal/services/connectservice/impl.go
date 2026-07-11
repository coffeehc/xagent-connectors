package connectservice

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coffeehc/base/log"
	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/storageservice"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/telegramservice"
	"go.uber.org/zap"
)

const (
	maxConsecutiveGetUpdatesFailures = 3
	getUpdatesRetryDelay             = 2 * time.Second
	getUpdatesFailureBackoff         = 30 * time.Second
)

type serviceImpl struct {
	mu          sync.Mutex
	apiKey      string
	storage     storageservice.Service
	telegram    telegramservice.Service
	pusher      MessagePusher
	bots        map[string]*connectordomain.Bot
	channels    map[string]*connectordomain.ConnectionBinding
	connections map[string]*connectordomain.Connection
	monitors    map[string]context.CancelFunc
	stopCtx     context.Context
	stop        context.CancelFunc
}

func newService(config Config, storage storageservice.Service, telegram telegramservice.Service) Service {
	return &serviceImpl{
		apiKey:   config.APIKey,
		storage:  storage,
		telegram: telegram,
	}
}

// Start 完成 connector 连接编排服务启动检查。
func (impl *serviceImpl) Start(context.Context) error {
	bots, err := impl.storage.LoadBots()
	if err != nil {
		log.Debug("加载 Telegram bot 登录态失败",
			zap.String("state_dir", impl.storage.StateDir()),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return err
	}
	channels, err := impl.storage.LoadConnectionBindings()
	if err != nil {
		log.Debug("加载 Telegram connection 绑定失败",
			zap.String("state_dir", impl.storage.StateDir()),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return err
	}
	if impl.pruneUnboundBots(bots, channels) {
		if err = impl.storage.SaveBots(bots); err != nil {
			return err
		}
	}
	connections := buildConnections(bots, channels)
	impl.mu.Lock()
	impl.bots = bots
	impl.channels = channels
	impl.connections = connections
	impl.monitors = map[string]context.CancelFunc{}
	impl.stopCtx, impl.stop = context.WithCancel(context.Background())
	for botID, bot := range bots {
		if impl.botHasBoundChannelLocked(botID) {
			impl.startBotMonitorLocked(bot)
		}
	}
	impl.mu.Unlock()
	log.Debug("加载 Telegram connector 登录态完成",
		zap.String("state_dir", impl.storage.StateDir()),
		zap.Int("bot_count", len(bots)),
		zap.Int("connection_binding_count", len(channels)),
		zap.String("result", "succeeded"),
	)
	return nil
}

// Stop 停止 connector 连接编排服务。
func (impl *serviceImpl) Stop(context.Context) error {
	impl.mu.Lock()
	if impl.stop != nil {
		impl.stop()
	}
	monitors := impl.monitors
	impl.monitors = map[string]context.CancelFunc{}
	impl.mu.Unlock()
	for _, cancel := range monitors {
		if cancel != nil {
			cancel()
		}
	}
	return nil
}

// BindMessagePusher 绑定 connector 入站消息推送端口。
func (impl *serviceImpl) BindMessagePusher(pusher MessagePusher) {
	impl.mu.Lock()
	impl.pusher = pusher
	impl.mu.Unlock()
}

// APIKey 返回 connector server 的 system API key。
func (impl *serviceImpl) APIKey() string {
	return impl.apiKey
}

// ConnectorID 返回 Connector Card id。
func (impl *serviceImpl) ConnectorID() string {
	return protocol.ConnectorCardID
}

// StateDir 返回本地登录态目录。
func (impl *serviceImpl) StateDir() string {
	return impl.storage.StateDir()
}

// TelegramAPIBaseURL 返回 Telegram Bot API endpoint。
func (impl *serviceImpl) TelegramAPIBaseURL() string {
	return impl.telegram.APIBaseURL()
}

// StartAuth 提交 Telegram bot token 与 chat_id 表单并完成 channel 绑定。
func (impl *serviceImpl) StartAuth(ctx context.Context, connectorChannelID string, request connectorprotocol.ConnectorAuthStartRequest) (*connectorprotocol.ConnectorAuthStartResult, error) {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" {
		return nil, fmt.Errorf("connector_channel_id required")
	}
	flowID := strings.TrimSpace(request.FlowID)
	if flowID == "" {
		flowID = protocol.TelegramBotBindingFlowID
	}
	if flowID != protocol.TelegramBotBindingFlowID {
		return nil, fmt.Errorf("unknown_flow")
	}
	botToken := strings.TrimSpace(request.Input["bot_token"])
	chatID := strings.TrimSpace(request.Input["chat_id"])
	if botToken == "" || chatID == "" {
		if connection := impl.ConnectionByChannel(connectorChannelID); connection != nil {
			return impl.authenticatedStartResult(flowID, connection), nil
		}
		return nil, fmt.Errorf("bot_token 和 chat_id 不能为空")
	}
	botUser, err := impl.telegram.GetMe(ctx, botToken)
	if err != nil {
		return nil, err
	}
	chat, err := impl.telegram.GetChat(ctx, telegramservice.GetChatInput{
		BotToken: botToken,
		ChatID:   chatID,
	})
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	botID := strconv.FormatInt(botUser.ID, 10)
	normalizedChatID := strconv.FormatInt(chat.ID, 10)
	if normalizedChatID == botID {
		return nil, fmt.Errorf("chat_id 不能是 bot 自己的 id，请填写用户或群聊的 chat_id")
	}
	bot := &connectordomain.Bot{
		BotID:       botID,
		BotToken:    botToken,
		Username:    strings.TrimSpace(botUser.Username),
		DisplayName: telegramUserDisplayName(botUser),
		CreatedAt:   now,
	}
	binding := &connectordomain.ConnectionBinding{
		ConnectorChannelID: connectorChannelID,
		BotID:              botID,
		ChatID:             normalizedChatID,
		ChatType:           strings.TrimSpace(chat.Type),
		ChatTitle:          telegramChatDisplayName(chat),
		CreatedAt:          now,
	}
	impl.mu.Lock()
	if previous := impl.bots[botID]; previous != nil {
		bot.CreatedAt = previous.CreatedAt
		bot.UpdateOffset = previous.UpdateOffset
		bot.LastInboundAt = previous.LastInboundAt
	}
	if previous := impl.channels[connectorChannelID]; previous != nil && previous.CreatedAt > 0 {
		binding.CreatedAt = previous.CreatedAt
	}
	impl.mu.Unlock()
	if err = impl.storage.SaveBot(bot); err != nil {
		return nil, err
	}
	if err = impl.storage.SaveConnectionBinding(binding); err != nil {
		return nil, err
	}
	connection := connectionFromBotChannel(bot, binding)
	impl.mu.Lock()
	impl.ensureRuntimeStateLocked()
	impl.bots[bot.BotID] = cloneBot(bot)
	impl.channels[connectorChannelID] = cloneBinding(binding)
	impl.connections[connectorChannelID] = connection
	impl.restartBotMonitorLocked(bot)
	impl.mu.Unlock()
	log.Debug("Telegram connector 绑定完成",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("bot_id", bot.BotID),
		zap.String("input_chat_id", chatID),
		zap.String("chat_id", binding.ChatID),
		zap.String("chat_type", binding.ChatType),
		zap.String("chat_title", binding.ChatTitle),
		zap.String("result", "authenticated"),
	)
	return impl.authenticatedStartResult(flowID, connection), nil
}

func (impl *serviceImpl) authenticatedStartResult(flowID string, connection *connectordomain.Connection) *connectorprotocol.ConnectorAuthStartResult {
	return &connectorprotocol.ConnectorAuthStartResult{
		ConnectorChannelID:   connection.Token,
		FlowID:               flowID,
		AuthSessionID:        "auth_" + randomToken(12),
		Status:               connectorprotocol.ConnectorAuthStartAuthenticated,
		PollIntervalMillis:   protocol.AuthPollIntervalMillis,
		Message:              "Telegram bot 与 chat 已绑定",
		ConnectionDescriptor: impl.BuildConnectionDescriptor(connection),
	}
}

// AuthStatus 返回 Telegram 表单绑定认证状态。
func (impl *serviceImpl) AuthStatus(_ context.Context, connectorChannelID string, authSessionID string, _ bool) (*connectorprotocol.ConnectorAuthStatusResult, bool) {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	connection := impl.ConnectionByChannel(connectorChannelID)
	if connection == nil {
		return &connectorprotocol.ConnectorAuthStatusResult{
			ConnectorChannelID: connectorChannelID,
			FlowID:             protocol.TelegramBotBindingFlowID,
			AuthSessionID:      strings.TrimSpace(authSessionID),
			Status:             connectorprotocol.ConnectorAuthStatusUnauthenticated,
			Message:            "Telegram channel 尚未绑定",
		}, true
	}
	return &connectorprotocol.ConnectorAuthStatusResult{
		ConnectorChannelID:   connectorChannelID,
		FlowID:               protocol.TelegramBotBindingFlowID,
		AuthSessionID:        strings.TrimSpace(authSessionID),
		Status:               connectorprotocol.ConnectorAuthStatusAuthenticated,
		Message:              "Telegram bot 与 chat 已绑定",
		ConnectionDescriptor: impl.BuildConnectionDescriptor(connection),
		PollIntervalMillis:   protocol.AuthPollIntervalMillis,
	}, true
}

// CancelAuth 取消未完成认证会话；Telegram 表单认证为同步提交，通常返回 not_found。
func (impl *serviceImpl) CancelAuth(_ context.Context, connectorChannelID string, authSessionID string) *connectorprotocol.ConnectorAuthCancelResult {
	return &connectorprotocol.ConnectorAuthCancelResult{
		ConnectorChannelID: strings.TrimSpace(connectorChannelID),
		AuthSessionID:      strings.TrimSpace(authSessionID),
		Status:             connectorprotocol.ConnectorAuthCancelStatusNotFound,
		AuthStatus:         connectorprotocol.ConnectorAuthStatusUnauthenticated,
		Message:            "Telegram 表单认证没有待取消会话",
	}
}

// ConnectionByChannel 根据 connector_channel_id 返回 connection。
func (impl *serviceImpl) ConnectionByChannel(connectorChannelID string) *connectordomain.Connection {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	return cloneConnection(impl.connections[strings.TrimSpace(connectorChannelID)])
}

// LogoutByChannel 根据 connector_channel_id 清理 connector 内绑定。
func (impl *serviceImpl) LogoutByChannel(connectorChannelID string) bool {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" {
		return false
	}
	impl.mu.Lock()
	binding := impl.channels[connectorChannelID]
	if binding == nil {
		impl.mu.Unlock()
		return false
	}
	delete(impl.channels, connectorChannelID)
	delete(impl.connections, connectorChannelID)
	botID := binding.BotID
	if !impl.botHasBoundChannelLocked(botID) {
		impl.stopBotMonitorLocked(botID)
	}
	err := impl.storage.DeleteConnectionBinding(connectorChannelID)
	impl.mu.Unlock()
	if err != nil {
		log.Debug("保存 Telegram connector 登出状态失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return false
	}
	return true
}

// BuildChannelDescriptor 构建指定 channel 的当前 Connection Descriptor。
func (impl *serviceImpl) BuildChannelDescriptor(connectorChannelID string) *connectorprotocol.ConnectionDescriptor {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	connection := impl.ConnectionByChannel(connectorChannelID)
	if connection != nil {
		return impl.BuildConnectionDescriptor(connection)
	}
	return impl.buildBaseChannelDescriptor(connectorChannelID, connectorprotocol.ConnectionStatusCreated)
}

func (impl *serviceImpl) buildBaseChannelDescriptor(connectorChannelID string, status connectorprotocol.ConnectionStatus) *connectorprotocol.ConnectionDescriptor {
	return &connectorprotocol.ConnectionDescriptor{
		Schema: connectorprotocol.ConnectionDescriptorSchema,
		Connection: connectorprotocol.ConnectionDescriptorInfo{
			ConnectorCardID:    protocol.ConnectorCardID,
			ConnectorChannelID: connectorChannelID,
			TargetType:         connectorprotocol.ConnectorTargetTypeIM,
			Profile:            "xagent.im.v1",
			Status:             status,
		},
		Target: connectorprotocol.ConnectionTargetDescriptor{
			Provider:    "telegram",
			Label:       "Telegram",
			DisplayName: "未绑定 Telegram",
		},
	}
}

// BuildConnectionDescriptor 构建用户绑定后的 Connection Descriptor。
func (impl *serviceImpl) BuildConnectionDescriptor(connection *connectordomain.Connection) *connectorprotocol.ConnectionDescriptor {
	if connection == nil {
		return nil
	}
	descriptor := impl.buildBaseChannelDescriptor(connection.Token, connectorprotocol.ConnectionStatusConnected)
	descriptor.Target.DisplayName = connection.DisplayName
	descriptor.Target.AccountHint = connection.AccountHint
	descriptor.Tools = []connectorprotocol.ConnectionToolState{
		{ToolID: toolIDTelegramMessageSend, Status: connectorprotocol.ConnectionToolStatusAvailable, TargetPermissionState: connectorprotocol.ConnectionTargetPermissionGranted},
		{ToolID: toolIDTelegramMessageSendMedia, Status: connectorprotocol.ConnectionToolStatusAvailable, TargetPermissionState: connectorprotocol.ConnectionTargetPermissionGranted},
	}
	return descriptor
}

func (impl *serviceImpl) pruneUnboundBots(bots map[string]*connectordomain.Bot, channels map[string]*connectordomain.ConnectionBinding) bool {
	used := map[string]bool{}
	for _, channel := range channels {
		if channel != nil && strings.TrimSpace(channel.BotID) != "" {
			used[channel.BotID] = true
		}
	}
	changed := false
	for botID := range bots {
		if !used[botID] {
			delete(bots, botID)
			changed = true
		}
	}
	return changed
}

func (impl *serviceImpl) ensureRuntimeStateLocked() {
	if impl.bots == nil {
		impl.bots = map[string]*connectordomain.Bot{}
	}
	if impl.channels == nil {
		impl.channels = map[string]*connectordomain.ConnectionBinding{}
	}
	if impl.connections == nil {
		impl.connections = map[string]*connectordomain.Connection{}
	}
	if impl.monitors == nil {
		impl.monitors = map[string]context.CancelFunc{}
	}
}

func buildConnections(bots map[string]*connectordomain.Bot, channels map[string]*connectordomain.ConnectionBinding) map[string]*connectordomain.Connection {
	connections := map[string]*connectordomain.Connection{}
	for connectorChannelID, channel := range channels {
		if channel == nil {
			continue
		}
		bot := bots[channel.BotID]
		if bot == nil {
			continue
		}
		channel.ConnectorChannelID = connectorChannelID
		connection := connectionFromBotChannel(bot, channel)
		if connection != nil {
			connections[connection.Token] = connection
		}
	}
	return connections
}

func connectionFromBotChannel(bot *connectordomain.Bot, channel *connectordomain.ConnectionBinding) *connectordomain.Connection {
	if bot == nil || channel == nil {
		return nil
	}
	displayName := "Telegram"
	if channel.ChatTitle != "" {
		displayName = "Telegram " + channel.ChatTitle
	}
	if bot.Username != "" {
		displayName += " via @" + bot.Username
	}
	accountHint := "chat " + maskID(channel.ChatID)
	if bot.Username != "" {
		accountHint = "@" + bot.Username + " / " + accountHint
	}
	return &connectordomain.Connection{
		Token:          strings.TrimSpace(channel.ConnectorChannelID),
		BotID:          strings.TrimSpace(bot.BotID),
		BotToken:       strings.TrimSpace(bot.BotToken),
		BotUsername:    strings.TrimSpace(bot.Username),
		BotDisplayName: strings.TrimSpace(bot.DisplayName),
		ChatID:         strings.TrimSpace(channel.ChatID),
		ChatType:       strings.TrimSpace(channel.ChatType),
		ChatTitle:      strings.TrimSpace(channel.ChatTitle),
		DisplayName:    displayName,
		AccountHint:    accountHint,
		CreatedAt:      channel.CreatedAt,
	}
}

func telegramUserDisplayName(user *telegramservice.User) string {
	if user == nil {
		return ""
	}
	if strings.TrimSpace(user.FirstName) != "" {
		return strings.TrimSpace(user.FirstName)
	}
	if strings.TrimSpace(user.Username) != "" {
		return "@" + strings.TrimSpace(user.Username)
	}
	return strconv.FormatInt(user.ID, 10)
}

func telegramChatDisplayName(chat *telegramservice.Chat) string {
	if chat == nil {
		return ""
	}
	if strings.TrimSpace(chat.Title) != "" {
		return strings.TrimSpace(chat.Title)
	}
	parts := []string{}
	if strings.TrimSpace(chat.FirstName) != "" {
		parts = append(parts, strings.TrimSpace(chat.FirstName))
	}
	if strings.TrimSpace(chat.LastName) != "" {
		parts = append(parts, strings.TrimSpace(chat.LastName))
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	if strings.TrimSpace(chat.Username) != "" {
		return "@" + strings.TrimSpace(chat.Username)
	}
	return strconv.FormatInt(chat.ID, 10)
}

func cloneBot(bot *connectordomain.Bot) *connectordomain.Bot {
	if bot == nil {
		return nil
	}
	cloned := *bot
	return &cloned
}

func cloneBinding(binding *connectordomain.ConnectionBinding) *connectordomain.ConnectionBinding {
	if binding == nil {
		return nil
	}
	cloned := *binding
	return &cloned
}

func cloneConnection(connection *connectordomain.Connection) *connectordomain.Connection {
	if connection == nil {
		return nil
	}
	cloned := *connection
	return &cloned
}

func randomToken(byteCount int) string {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}

func maskID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "***"
	}
	if len(value) <= 6 {
		return value + "***"
	}
	return value[:3] + "***" + value[len(value)-3:]
}
