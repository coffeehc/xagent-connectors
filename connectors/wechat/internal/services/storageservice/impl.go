package storageservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
)

type serviceImpl struct {
	stateDir       string
	mu             sync.Mutex
	bots           map[string]*connectordomain.Bot
	botsLoaded     bool
	channels       map[string]*connectordomain.ConnectionBinding
	channelsLoaded bool
}

type botStateFile map[string]*connectordomain.Bot

type channelStateFile map[string]*connectordomain.ConnectionBinding

type connectorStateFile struct {
	Connections []*connectordomain.Connection `json:"connections"`
}

type pendingInboundStateFile struct {
	Messages []*connectordomain.PendingInboundMessage `json:"messages"`
	Cursors  []*connectordomain.InboundChannelCursor  `json:"cursors,omitempty"`
}

type mediaReferenceStateFile struct {
	Items []*connectordomain.MediaReference `json:"items"`
}

// newService 创建本地登录态 storage service。
func newService(stateDir string, connectorID string) Service {
	if stateDir == "" {
		stateDir = DefaultStateDir(connectorID)
	}
	return &serviceImpl{stateDir: stateDir}
}

// Start 完成本地登录态 storage 服务启动检查。
func (impl *serviceImpl) Start(context.Context) error {
	if _, err := impl.LoadBots(); err != nil {
		return err
	}
	if _, err := impl.LoadConnectionBindings(); err != nil {
		return err
	}
	_, err := impl.LoadLegacyConnections()
	return err
}

// Stop 停止本地登录态 storage 服务。
func (impl *serviceImpl) Stop(context.Context) error {
	return nil
}

// DefaultStateDir 返回 connector 默认持久化目录。
func DefaultStateDir(connectorID string) string {
	if envValue := strings.TrimSpace(os.Getenv("XAGENT_WECHAT_CONNECTOR_STATE_DIR")); envValue != "" {
		return envValue
	}
	connectorID = SanitizeIDPart(connectorID)
	if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
		return filepath.Join(homeDir, ".xagent", "connectors", connectorID)
	}
	return filepath.Join(os.TempDir(), "xagent", "connectors", connectorID)
}

// SanitizeIDPart 将 connector id 转成可用于路径或 descriptor id 的片段。
func SanitizeIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '.', char == '-', char == '_':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	result := builder.String()
	if result == "" {
		return "unknown"
	}
	return result
}

// StateDir 返回当前 storage 使用的本地目录。
func (impl *serviceImpl) StateDir() string {
	return impl.stateDir
}

func (impl *serviceImpl) botFilePath() string {
	return filepath.Join(impl.stateDir, protocol.BotStateFilename)
}

func (impl *serviceImpl) channelFilePath() string {
	return filepath.Join(impl.stateDir, protocol.ChannelStateFilename)
}

func (impl *serviceImpl) legacyStateFilePath() string {
	return filepath.Join(impl.stateDir, protocol.LegacyConnectorStateFilename)
}

func (impl *serviceImpl) pendingInboundFilePath() string {
	return filepath.Join(impl.stateDir, protocol.PendingInboundFilename)
}

func (impl *serviceImpl) mediaReferenceFilePath() string {
	return filepath.Join(impl.stateDir, protocol.MediaReferenceFilename)
}

func (impl *serviceImpl) legacyUploadedMediaFilePath() string {
	return filepath.Join(impl.stateDir, protocol.LegacyUploadedMediaFilename)
}

func writeJSONFile(filePath string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := filePath + ".tmp"
	if err = os.WriteFile(tmpPath, payload, 0o600); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Chmod(filePath, 0o600)
	return nil
}

// LoadBots 从内存读取微信 bot 登录态；首次调用从 bots.json 恢复。
func (impl *serviceImpl) LoadBots() (map[string]*connectordomain.Bot, error) {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if err := impl.ensureBotsLoadedLocked(); err != nil {
		return nil, err
	}
	return cloneBotMap(impl.bots), nil
}

func (impl *serviceImpl) loadBotsFileLocked() (map[string]*connectordomain.Bot, error) {
	bots := map[string]*connectordomain.Bot{}
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建 connector state dir 失败: %w", err)
	}
	raw, err := os.ReadFile(impl.botFilePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return bots, nil
		}
		return nil, fmt.Errorf("读取微信 bot 登录态失败: %w", err)
	}
	state := botStateFile{}
	if err = json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("解析微信 bot 登录态失败: %w", err)
	}
	for botAccountID, bot := range state {
		if bot == nil || strings.TrimSpace(botAccountID) == "" || strings.TrimSpace(bot.BotAccountID) == "" || strings.TrimSpace(bot.BotToken) == "" {
			continue
		}
		if bot.ContextTokens == nil {
			bot.ContextTokens = map[string]string{}
		}
		bots[bot.BotAccountID] = bot
	}
	return bots, nil
}

func (impl *serviceImpl) ensureBotsLoadedLocked() error {
	if impl.botsLoaded {
		return nil
	}
	bots, err := impl.loadBotsFileLocked()
	if err != nil {
		return err
	}
	impl.bots = bots
	impl.botsLoaded = true
	return nil
}

// SaveBots 更新内存中的完整微信 bot 登录态，并刷新 bots.json 恢复快照。
func (impl *serviceImpl) SaveBots(bots map[string]*connectordomain.Bot) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	impl.bots = cloneBotMap(bots)
	impl.botsLoaded = true
	return impl.saveBotsLocked(impl.bots)
}

func (impl *serviceImpl) saveBotsLocked(bots map[string]*connectordomain.Bot) error {
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return fmt.Errorf("创建 connector state dir 失败: %w", err)
	}
	state := botStateFile{}
	for botAccountID, bot := range bots {
		if bot == nil || strings.TrimSpace(botAccountID) == "" || strings.TrimSpace(bot.BotAccountID) == "" {
			continue
		}
		state[bot.BotAccountID] = cloneBot(bot)
	}
	return writeJSONFile(impl.botFilePath(), state)
}

// SaveBot 更新内存中的单个微信 bot 登录态，并刷新 bots.json 恢复快照。
func (impl *serviceImpl) SaveBot(bot *connectordomain.Bot) error {
	if bot == nil || strings.TrimSpace(bot.BotAccountID) == "" {
		return fmt.Errorf("微信 bot 登录态不能为空")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if err := impl.ensureBotsLoadedLocked(); err != nil {
		return err
	}
	impl.bots[bot.BotAccountID] = cloneBot(bot)
	return impl.saveBotsLocked(impl.bots)
}

// LoadConnectionBindings 从内存读取 connector connection 绑定；首次调用从兼容的 channels.json 恢复。
func (impl *serviceImpl) LoadConnectionBindings() (map[string]*connectordomain.ConnectionBinding, error) {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if err := impl.ensureChannelsLoadedLocked(); err != nil {
		return nil, err
	}
	return cloneChannelMap(impl.channels), nil
}

func (impl *serviceImpl) loadChannelsFileLocked() (map[string]*connectordomain.ConnectionBinding, error) {
	bindings := map[string]*connectordomain.ConnectionBinding{}
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建 connector state dir 失败: %w", err)
	}
	raw, err := os.ReadFile(impl.channelFilePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return bindings, nil
		}
		return nil, fmt.Errorf("读取 connector connection 绑定失败: %w", err)
	}
	state := channelStateFile{}
	if err = json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("解析 connector connection 绑定失败: %w", err)
	}
	for connectorChannelID, binding := range state {
		if binding == nil || strings.TrimSpace(connectorChannelID) == "" || strings.TrimSpace(binding.BotAccountID) == "" {
			continue
		}
		if strings.TrimSpace(binding.ConnectorChannelID) == "" {
			binding.ConnectorChannelID = connectorChannelID
		}
		bindings[binding.ConnectorChannelID] = binding
	}
	return bindings, nil
}

func (impl *serviceImpl) ensureChannelsLoadedLocked() error {
	if impl.channelsLoaded {
		return nil
	}
	bindings, err := impl.loadChannelsFileLocked()
	if err != nil {
		return err
	}
	impl.channels = bindings
	impl.channelsLoaded = true
	return nil
}

// SaveConnectionBindings 更新内存中的完整 connector connection 绑定，并刷新兼容的 channels.json 恢复快照。
func (impl *serviceImpl) SaveConnectionBindings(bindings map[string]*connectordomain.ConnectionBinding) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	impl.channels = cloneChannelMap(bindings)
	impl.channelsLoaded = true
	return impl.saveChannelsLocked(impl.channels)
}

func (impl *serviceImpl) saveChannelsLocked(channels map[string]*connectordomain.ConnectionBinding) error {
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return fmt.Errorf("创建 connector state dir 失败: %w", err)
	}
	state := channelStateFile{}
	for connectorChannelID, channel := range channels {
		if channel == nil || strings.TrimSpace(connectorChannelID) == "" || strings.TrimSpace(channel.BotAccountID) == "" {
			continue
		}
		cloned := cloneChannel(channel)
		cloned.ConnectorChannelID = connectorChannelID
		state[connectorChannelID] = cloned
	}
	return writeJSONFile(impl.channelFilePath(), state)
}

// SaveConnectionBinding 更新内存中的单个 connector connection 绑定，并刷新兼容的 channels.json 恢复快照。
func (impl *serviceImpl) SaveConnectionBinding(binding *connectordomain.ConnectionBinding) error {
	if binding == nil || strings.TrimSpace(binding.ConnectorChannelID) == "" {
		return fmt.Errorf("connector connection 绑定不能为空")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if err := impl.ensureChannelsLoadedLocked(); err != nil {
		return err
	}
	impl.channels[binding.ConnectorChannelID] = cloneChannel(binding)
	return impl.saveChannelsLocked(impl.channels)
}

// DeleteConnectionBinding 从内存删除指定 connector connection 绑定，并刷新兼容的 channels.json 恢复快照。
func (impl *serviceImpl) DeleteConnectionBinding(connectorChannelID string) error {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" {
		return fmt.Errorf("connector_channel_id 不能为空")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if err := impl.ensureChannelsLoadedLocked(); err != nil {
		return err
	}
	delete(impl.channels, connectorChannelID)
	return impl.saveChannelsLocked(impl.channels)
}

// LoadLegacyConnections 从旧 connections.json 读取历史状态，仅用于启动迁移。
func (impl *serviceImpl) LoadLegacyConnections() (map[string]*connectordomain.Connection, error) {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	connections := map[string]*connectordomain.Connection{}
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建 connector state dir 失败: %w", err)
	}
	raw, err := os.ReadFile(impl.legacyStateFilePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return connections, nil
		}
		return nil, fmt.Errorf("读取旧 connector 登录态失败: %w", err)
	}
	state := connectorStateFile{}
	if err = json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("解析旧 connector 登录态失败: %w", err)
	}
	for _, connection := range state.Connections {
		if connection == nil || strings.TrimSpace(connection.Token) == "" || strings.TrimSpace(connection.BotAccountID) == "" || strings.TrimSpace(connection.BotToken) == "" {
			continue
		}
		if connection.ContextTokens == nil {
			connection.ContextTokens = map[string]string{}
		}
		connections[connection.Token] = connection
	}
	return connections, nil
}

// EnqueuePendingInboundMessage 将微信入站消息写入本地待投递队列。
func (impl *serviceImpl) EnqueuePendingInboundMessage(message *connectordomain.PendingInboundMessage, maxMessagesPerChannel int) error {
	if message == nil || strings.TrimSpace(message.ID) == "" || strings.TrimSpace(message.ConnectorChannelID) == "" {
		return fmt.Errorf("pending inbound message 不完整")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	state, err := impl.loadPendingInboundStateLocked()
	if err != nil {
		return err
	}
	now := message.UpdatedAt
	if now == 0 {
		now = message.CreatedAt
	}
	if now == 0 {
		now = message.ReceivedAt
	}
	if now > 0 {
		if err := impl.ensureChannelsLoadedLocked(); err != nil {
			return err
		}
		state.Messages = prunePendingMessages(state.Messages, impl.channels, now)
	}
	replaced := false
	for index, existing := range state.Messages {
		if existing != nil && existing.ID == message.ID {
			state.Messages[index] = message
			replaced = true
			break
		}
	}
	if !replaced {
		state.Messages = append(state.Messages, message)
	}
	state.Messages = trimPendingMessagesForChannel(state.Messages, message.ConnectorChannelID, maxMessagesPerChannel)
	return impl.savePendingInboundStateLocked(state)
}

// ListPendingInboundMessages 返回指定 channel 尚未投递、未越过 channel 游标且未过期的入站消息。
func (impl *serviceImpl) ListPendingInboundMessages(connectorChannelID string, nowMillis int64, limit int) ([]*connectordomain.PendingInboundMessage, error) {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" {
		return nil, fmt.Errorf("connector_channel_id 不能为空")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	state, err := impl.loadPendingInboundStateLocked()
	if err != nil {
		return nil, err
	}
	if err = impl.ensureChannelsLoadedLocked(); err != nil {
		return nil, err
	}
	channel := impl.channels[connectorChannelID]
	pending := make([]*connectordomain.PendingInboundMessage, 0)
	for _, message := range state.Messages {
		if message == nil || message.ConnectorChannelID != connectorChannelID || message.DeliveredAt > 0 {
			continue
		}
		if pendingInboundMessageShouldPrune(message, channel, nowMillis) {
			continue
		}
		pending = append(pending, message)
	}
	sort.SliceStable(pending, func(left, right int) bool {
		if pending[left].ReceivedAt == pending[right].ReceivedAt {
			return pending[left].ID < pending[right].ID
		}
		return pending[left].ReceivedAt < pending[right].ReceivedAt
	})
	if limit > 0 && len(pending) > limit {
		pending = pending[:limit]
	}
	return pending, nil
}

// MarkPendingInboundDelivered 标记指定入站消息已成功投递并推进 channel 投递游标。
func (impl *serviceImpl) MarkPendingInboundDelivered(connectorChannelID string, messageID string, deliveredAt int64) error {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	messageID = strings.TrimSpace(messageID)
	if connectorChannelID == "" || messageID == "" {
		return fmt.Errorf("connector_channel_id 和 message_id 不能为空")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	state, err := impl.loadPendingInboundStateLocked()
	if err != nil {
		return err
	}
	nextMessages := state.Messages[:0]
	found := false
	for _, message := range state.Messages {
		if message == nil {
			continue
		}
		if message.ConnectorChannelID == connectorChannelID && message.ID == messageID {
			found = true
			continue
		}
		nextMessages = append(nextMessages, message)
	}
	if !found {
		return fmt.Errorf("pending inbound message not found: %s", messageID)
	}
	state.Messages = nextMessages
	if err = impl.ensureChannelsLoadedLocked(); err != nil {
		return err
	}
	channel := impl.channels[connectorChannelID]
	if channel == nil {
		return fmt.Errorf("connector channel not found: %s", connectorChannelID)
	}
	channel.LastDeliveredMessageID = messageID
	channel.LastDeliveredAt = deliveredAt
	if err = impl.savePendingInboundStateLocked(state); err != nil {
		return err
	}
	return impl.saveChannelsLocked(impl.channels)
}

// MarkPendingInboundFailed 记录指定入站消息最近一次投递失败。
func (impl *serviceImpl) MarkPendingInboundFailed(connectorChannelID string, messageID string, failedAt int64, reason string) error {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	messageID = strings.TrimSpace(messageID)
	if connectorChannelID == "" || messageID == "" {
		return fmt.Errorf("connector_channel_id 和 message_id 不能为空")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	state, err := impl.loadPendingInboundStateLocked()
	if err != nil {
		return err
	}
	for _, message := range state.Messages {
		if message == nil || message.ConnectorChannelID != connectorChannelID || message.ID != messageID {
			continue
		}
		message.AttemptCount++
		message.LastError = trimErrorReason(reason)
		message.UpdatedAt = failedAt
		return impl.savePendingInboundStateLocked(state)
	}
	return fmt.Errorf("pending inbound message not found: %s", messageID)
}

// PruneExpiredPendingInboundMessages 删除已过期或已被 channel 游标覆盖的入站消息。
func (impl *serviceImpl) PruneExpiredPendingInboundMessages(nowMillis int64) (int, error) {
	if nowMillis <= 0 {
		return 0, nil
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	state, err := impl.loadPendingInboundStateLocked()
	if err != nil {
		return 0, err
	}
	if err = impl.ensureChannelsLoadedLocked(); err != nil {
		return 0, err
	}
	before := len(state.Messages)
	state.Messages = prunePendingMessages(state.Messages, impl.channels, nowMillis)
	removed := before - len(state.Messages)
	if removed == 0 {
		return 0, nil
	}
	return removed, impl.savePendingInboundStateLocked(state)
}

// LoadInboundChannelCursor 读取指定 channel 的本地投递游标。
func (impl *serviceImpl) LoadInboundChannelCursor(connectorChannelID string) (*connectordomain.InboundChannelCursor, error) {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" {
		return nil, fmt.Errorf("connector_channel_id 不能为空")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	state, err := impl.loadPendingInboundStateLocked()
	if err != nil {
		return nil, err
	}
	if err = impl.migratePendingCursorsToChannelsLocked(state); err != nil {
		return nil, err
	}
	if err = impl.ensureChannelsLoadedLocked(); err != nil {
		return nil, err
	}
	if channel := impl.channels[connectorChannelID]; channel != nil {
		return &connectordomain.InboundChannelCursor{
			ConnectorChannelID:     connectorChannelID,
			LastDeliveredMessageID: channel.LastDeliveredMessageID,
			LastDeliveredAt:        channel.LastDeliveredAt,
		}, nil
	}
	return &connectordomain.InboundChannelCursor{ConnectorChannelID: connectorChannelID}, nil
}

// SaveMediaReference 保存 connector 短期媒体 key 映射。
func (impl *serviceImpl) SaveMediaReference(reference *connectordomain.MediaReference) error {
	if reference == nil || strings.TrimSpace(reference.Ref) == "" || strings.TrimSpace(reference.ConnectorChannelID) == "" {
		return fmt.Errorf("media reference 不完整")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	state, err := impl.loadMediaReferenceStateLocked()
	if err != nil {
		return err
	}
	if reference.CreatedAt > 0 {
		state.Items = pruneExpiredMediaReferences(state.Items, reference.CreatedAt)
	}
	replaced := false
	for index, existing := range state.Items {
		if existing != nil && existing.Ref == reference.Ref {
			state.Items[index] = reference
			replaced = true
			break
		}
	}
	if !replaced {
		state.Items = append(state.Items, reference)
	}
	return impl.saveMediaReferenceStateLocked(state)
}

// GetMediaReference 按 key 读取未过期媒体映射。
func (impl *serviceImpl) GetMediaReference(ref string, nowMillis int64) (*connectordomain.MediaReference, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("media_ref 不能为空")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	state, err := impl.loadMediaReferenceStateLocked()
	if err != nil {
		return nil, err
	}
	for _, item := range state.Items {
		if item == nil || item.Ref != ref {
			continue
		}
		if nowMillis > 0 && item.ExpiresAt > 0 && nowMillis >= item.ExpiresAt {
			return nil, nil
		}
		cloned := *item
		return &cloned, nil
	}
	return nil, nil
}

// PruneExpiredMediaReferences 删除已过期媒体映射。
func (impl *serviceImpl) PruneExpiredMediaReferences(nowMillis int64) (int, error) {
	if nowMillis <= 0 {
		return 0, nil
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	state, err := impl.loadMediaReferenceStateLocked()
	if err != nil {
		return 0, err
	}
	nextItems := state.Items[:0]
	expiredItems := []*connectordomain.MediaReference{}
	for _, item := range state.Items {
		if item == nil {
			continue
		}
		if item.ExpiresAt > 0 && nowMillis >= item.ExpiresAt {
			expiredItems = append(expiredItems, item)
			continue
		}
		nextItems = append(nextItems, item)
	}
	if len(expiredItems) == 0 {
		return 0, nil
	}
	state.Items = nextItems
	if err = impl.saveMediaReferenceStateLocked(state); err != nil {
		return 0, err
	}
	for _, item := range expiredItems {
		removeMediaReferenceLocalFile(item)
	}
	return len(expiredItems), nil
}

func (impl *serviceImpl) loadPendingInboundStateLocked() (*pendingInboundStateFile, error) {
	state := &pendingInboundStateFile{}
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建 connector state dir 失败: %w", err)
	}
	raw, err := os.ReadFile(impl.pendingInboundFilePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return nil, fmt.Errorf("读取 connector 入站缓存失败: %w", err)
	}
	if err = json.Unmarshal(raw, state); err != nil {
		return nil, fmt.Errorf("解析 connector 入站缓存失败: %w", err)
	}
	return state, nil
}

func (impl *serviceImpl) migratePendingCursorsToChannelsLocked(state *pendingInboundStateFile) error {
	if state == nil || len(state.Cursors) == 0 {
		return nil
	}
	if err := impl.ensureChannelsLoadedLocked(); err != nil {
		return err
	}
	changed := false
	for _, cursor := range state.Cursors {
		if cursor == nil || strings.TrimSpace(cursor.ConnectorChannelID) == "" {
			continue
		}
		channel := impl.channels[cursor.ConnectorChannelID]
		if channel == nil {
			continue
		}
		if cursor.LastDeliveredMessageID != "" || cursor.LastDeliveredAt > 0 {
			channel.LastDeliveredMessageID = cursor.LastDeliveredMessageID
			channel.LastDeliveredAt = cursor.LastDeliveredAt
			changed = true
		}
	}
	state.Cursors = nil
	if changed {
		if err := impl.saveChannelsLocked(impl.channels); err != nil {
			return err
		}
	}
	return impl.savePendingInboundStateLocked(state)
}

func (impl *serviceImpl) loadMediaReferenceStateLocked() (*mediaReferenceStateFile, error) {
	state := &mediaReferenceStateFile{}
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建 connector state dir 失败: %w", err)
	}
	raw, err := os.ReadFile(impl.mediaReferenceFilePath())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("读取 connector 媒体 key 映射失败: %w", err)
		}
		raw, err = os.ReadFile(impl.legacyUploadedMediaFilePath())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return state, nil
			}
			return nil, fmt.Errorf("读取 connector 旧媒体缓存失败: %w", err)
		}
	}
	if err = json.Unmarshal(raw, state); err != nil {
		return nil, fmt.Errorf("解析 connector 媒体 key 映射失败: %w", err)
	}
	return state, nil
}

func (impl *serviceImpl) savePendingInboundStateLocked(state *pendingInboundStateFile) error {
	if state == nil {
		state = &pendingInboundStateFile{}
	}
	state.Cursors = nil
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return fmt.Errorf("创建 connector state dir 失败: %w", err)
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	filePath := impl.pendingInboundFilePath()
	tmpPath := filePath + ".tmp"
	if err = os.WriteFile(tmpPath, payload, 0o600); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Chmod(filePath, 0o600)
	return nil
}

func (impl *serviceImpl) saveMediaReferenceStateLocked(state *mediaReferenceStateFile) error {
	if state == nil {
		state = &mediaReferenceStateFile{}
	}
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return fmt.Errorf("创建 connector state dir 失败: %w", err)
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	filePath := impl.mediaReferenceFilePath()
	tmpPath := filePath + ".tmp"
	if err = os.WriteFile(tmpPath, payload, 0o600); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Chmod(filePath, 0o600)
	return nil
}

func prunePendingMessages(messages []*connectordomain.PendingInboundMessage, channels map[string]*connectordomain.ConnectionBinding, nowMillis int64) []*connectordomain.PendingInboundMessage {
	next := messages[:0]
	for _, message := range messages {
		if message == nil {
			continue
		}
		if pendingInboundMessageShouldPrune(message, channels[message.ConnectorChannelID], nowMillis) {
			continue
		}
		next = append(next, message)
	}
	return next
}

func pendingInboundMessageShouldPrune(message *connectordomain.PendingInboundMessage, channel *connectordomain.ConnectionBinding, nowMillis int64) bool {
	if message == nil {
		return true
	}
	if nowMillis > 0 {
		if message.ExpiresAt > 0 && nowMillis >= message.ExpiresAt {
			return true
		}
		if message.ReceivedAt > 0 && nowMillis-message.ReceivedAt >= protocol.InboundCacheTTLMillis {
			return true
		}
	}
	if channel == nil {
		return false
	}
	if channel.LastDeliveredMessageID != "" && message.ID == channel.LastDeliveredMessageID {
		return true
	}
	if channel.LastDeliveredAt > 0 && message.ReceivedAt > 0 && message.ReceivedAt <= channel.LastDeliveredAt {
		return true
	}
	return false
}

func pruneExpiredMediaReferences(items []*connectordomain.MediaReference, nowMillis int64) []*connectordomain.MediaReference {
	if nowMillis <= 0 {
		return items
	}
	next := items[:0]
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.ExpiresAt > 0 && nowMillis >= item.ExpiresAt {
			continue
		}
		next = append(next, item)
	}
	return next
}

func removeMediaReferenceLocalFile(item *connectordomain.MediaReference) {
	if item == nil {
		return
	}
	localPath := strings.TrimSpace(item.LocalPath)
	if localPath == "" {
		return
	}
	_ = os.Remove(localPath)
}

func trimPendingMessagesForChannel(messages []*connectordomain.PendingInboundMessage, connectorChannelID string, maxMessagesPerChannel int) []*connectordomain.PendingInboundMessage {
	if maxMessagesPerChannel <= 0 {
		maxMessagesPerChannel = protocol.InboundCacheMaxMessagesPerChannel
	}
	channelMessages := make([]*connectordomain.PendingInboundMessage, 0)
	for _, message := range messages {
		if message != nil && message.ConnectorChannelID == connectorChannelID {
			channelMessages = append(channelMessages, message)
		}
	}
	if len(channelMessages) <= maxMessagesPerChannel {
		return messages
	}
	sort.SliceStable(channelMessages, func(left, right int) bool {
		if channelMessages[left].ReceivedAt == channelMessages[right].ReceivedAt {
			return channelMessages[left].ID < channelMessages[right].ID
		}
		return channelMessages[left].ReceivedAt < channelMessages[right].ReceivedAt
	})
	drop := map[string]struct{}{}
	for _, message := range channelMessages[:len(channelMessages)-maxMessagesPerChannel] {
		drop[message.ID] = struct{}{}
	}
	next := messages[:0]
	for _, message := range messages {
		if message == nil {
			continue
		}
		if message.ConnectorChannelID == connectorChannelID {
			if _, ok := drop[message.ID]; ok {
				continue
			}
		}
		next = append(next, message)
	}
	return next
}

func trimErrorReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > 512 {
		return reason[:512]
	}
	return reason
}

func cloneBotMap(bots map[string]*connectordomain.Bot) map[string]*connectordomain.Bot {
	cloned := map[string]*connectordomain.Bot{}
	for key, bot := range bots {
		if bot == nil {
			continue
		}
		cloned[key] = cloneBot(bot)
	}
	return cloned
}

func cloneBot(bot *connectordomain.Bot) *connectordomain.Bot {
	if bot == nil {
		return nil
	}
	cloned := *bot
	if bot.ContextTokens != nil {
		cloned.ContextTokens = map[string]string{}
		for key, value := range bot.ContextTokens {
			cloned.ContextTokens[key] = value
		}
	}
	return &cloned
}

func cloneChannelMap(channels map[string]*connectordomain.ConnectionBinding) map[string]*connectordomain.ConnectionBinding {
	cloned := map[string]*connectordomain.ConnectionBinding{}
	for key, channel := range channels {
		if channel == nil {
			continue
		}
		cloned[key] = cloneChannel(channel)
	}
	return cloned
}

func cloneChannel(channel *connectordomain.ConnectionBinding) *connectordomain.ConnectionBinding {
	if channel == nil {
		return nil
	}
	cloned := *channel
	return &cloned
}
