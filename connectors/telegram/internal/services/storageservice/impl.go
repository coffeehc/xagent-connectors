package storageservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/protocol"
)

type serviceImpl struct {
	stateDir       string
	mu             sync.Mutex
	bots           map[string]*connectordomain.Bot
	botsLoaded     bool
	channels       map[string]*connectordomain.ConnectionBinding
	channelsLoaded bool
	media          map[string]*connectordomain.MediaReference
	mediaLoaded    bool
}

type botStateFile map[string]*connectordomain.Bot

type channelStateFile map[string]*connectordomain.ConnectionBinding

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
	_, err := impl.PruneExpiredMediaReferences(0)
	return err
}

// Stop 停止本地登录态 storage 服务。
func (impl *serviceImpl) Stop(context.Context) error {
	return nil
}

// DefaultStateDir 返回 connector 默认持久化目录。
func DefaultStateDir(connectorID string) string {
	if envValue := strings.TrimSpace(os.Getenv("XAGENT_TELEGRAM_CONNECTOR_STATE_DIR")); envValue != "" {
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

func (impl *serviceImpl) mediaReferenceFilePath() string {
	return filepath.Join(impl.stateDir, protocol.MediaReferenceFilename)
}

// MediaCacheDir 返回出站 Telegram 媒体本地缓存目录。
func MediaCacheDir(stateDir string) string {
	return filepath.Join(stateDir, protocol.MediaCacheDirname)
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

// LoadBots 从内存读取 Telegram bot 登录态；首次调用从 bots.json 恢复。
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
		return nil, fmt.Errorf("创建 Telegram connector state dir 失败: %w", err)
	}
	raw, err := os.ReadFile(impl.botFilePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return bots, nil
		}
		return nil, fmt.Errorf("读取 Telegram bot 登录态失败: %w", err)
	}
	state := botStateFile{}
	if err = json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("解析 Telegram bot 登录态失败: %w", err)
	}
	for botID, bot := range state {
		if bot == nil || strings.TrimSpace(botID) == "" || strings.TrimSpace(bot.BotID) == "" || strings.TrimSpace(bot.BotToken) == "" {
			continue
		}
		bots[bot.BotID] = bot
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

// SaveBots 更新内存中的完整 Telegram bot 登录态，并刷新 bots.json 恢复快照。
func (impl *serviceImpl) SaveBots(bots map[string]*connectordomain.Bot) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	impl.bots = cloneBotMap(bots)
	impl.botsLoaded = true
	return impl.saveBotsLocked(impl.bots)
}

func (impl *serviceImpl) saveBotsLocked(bots map[string]*connectordomain.Bot) error {
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return fmt.Errorf("创建 Telegram connector state dir 失败: %w", err)
	}
	state := botStateFile{}
	for botID, bot := range bots {
		if bot == nil || strings.TrimSpace(botID) == "" || strings.TrimSpace(bot.BotID) == "" {
			continue
		}
		state[bot.BotID] = cloneBot(bot)
	}
	return writeJSONFile(impl.botFilePath(), state)
}

// SaveBot 更新内存中的单个 Telegram bot 登录态，并刷新 bots.json 恢复快照。
func (impl *serviceImpl) SaveBot(bot *connectordomain.Bot) error {
	if bot == nil || strings.TrimSpace(bot.BotID) == "" {
		return fmt.Errorf("Telegram bot 登录态不能为空")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if err := impl.ensureBotsLoadedLocked(); err != nil {
		return err
	}
	impl.bots[bot.BotID] = cloneBot(bot)
	return impl.saveBotsLocked(impl.bots)
}

// LoadConnectionBindings 从内存读取 connector connection 绑定；首次调用从 channels.json 恢复。
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
		return nil, fmt.Errorf("创建 Telegram connector state dir 失败: %w", err)
	}
	raw, err := os.ReadFile(impl.channelFilePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return bindings, nil
		}
		return nil, fmt.Errorf("读取 Telegram connection 绑定失败: %w", err)
	}
	state := channelStateFile{}
	if err = json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("解析 Telegram connection 绑定失败: %w", err)
	}
	for connectorChannelID, binding := range state {
		if binding == nil || strings.TrimSpace(connectorChannelID) == "" || strings.TrimSpace(binding.BotID) == "" || strings.TrimSpace(binding.ChatID) == "" {
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

// SaveConnectionBindings 更新内存中的完整 connector connection 绑定，并刷新 channels.json 恢复快照。
func (impl *serviceImpl) SaveConnectionBindings(bindings map[string]*connectordomain.ConnectionBinding) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	impl.channels = cloneChannelMap(bindings)
	impl.channelsLoaded = true
	return impl.saveChannelsLocked(impl.channels)
}

func (impl *serviceImpl) saveChannelsLocked(channels map[string]*connectordomain.ConnectionBinding) error {
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return fmt.Errorf("创建 Telegram connector state dir 失败: %w", err)
	}
	state := channelStateFile{}
	for connectorChannelID, channel := range channels {
		if channel == nil || strings.TrimSpace(connectorChannelID) == "" || strings.TrimSpace(channel.BotID) == "" || strings.TrimSpace(channel.ChatID) == "" {
			continue
		}
		cloned := cloneChannel(channel)
		cloned.ConnectorChannelID = connectorChannelID
		state[connectorChannelID] = cloned
	}
	return writeJSONFile(impl.channelFilePath(), state)
}

// SaveConnectionBinding 更新内存中的单个 connector connection 绑定，并刷新 channels.json 恢复快照。
func (impl *serviceImpl) SaveConnectionBinding(binding *connectordomain.ConnectionBinding) error {
	if binding == nil || strings.TrimSpace(binding.ConnectorChannelID) == "" {
		return fmt.Errorf("Telegram connection 绑定不能为空")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if err := impl.ensureChannelsLoadedLocked(); err != nil {
		return err
	}
	impl.channels[binding.ConnectorChannelID] = cloneChannel(binding)
	return impl.saveChannelsLocked(impl.channels)
}

// DeleteConnectionBinding 从内存删除指定 connector connection 绑定，并刷新 channels.json 恢复快照。
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

func cloneBotMap(input map[string]*connectordomain.Bot) map[string]*connectordomain.Bot {
	output := map[string]*connectordomain.Bot{}
	for botID, bot := range input {
		if bot == nil {
			continue
		}
		output[botID] = cloneBot(bot)
	}
	return output
}

func cloneBot(bot *connectordomain.Bot) *connectordomain.Bot {
	if bot == nil {
		return nil
	}
	cloned := *bot
	return &cloned
}

func cloneChannelMap(input map[string]*connectordomain.ConnectionBinding) map[string]*connectordomain.ConnectionBinding {
	output := map[string]*connectordomain.ConnectionBinding{}
	for connectorChannelID, channel := range input {
		if channel == nil {
			continue
		}
		output[connectorChannelID] = cloneChannel(channel)
	}
	return output
}

func cloneChannel(channel *connectordomain.ConnectionBinding) *connectordomain.ConnectionBinding {
	if channel == nil {
		return nil
	}
	cloned := *channel
	return &cloned
}

// SaveMediaReference 保存 connector 短期媒体 key 映射。
func (impl *serviceImpl) SaveMediaReference(reference *connectordomain.MediaReference) error {
	if reference == nil || strings.TrimSpace(reference.Ref) == "" {
		return fmt.Errorf("Telegram media_ref 不能为空")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if err := impl.ensureMediaLoadedLocked(); err != nil {
		return err
	}
	impl.media[reference.Ref] = cloneMediaReference(reference)
	return impl.saveMediaReferencesLocked(impl.media)
}

// GetMediaReference 按 key 读取未过期媒体映射。
func (impl *serviceImpl) GetMediaReference(ref string, nowMillis int64) (*connectordomain.MediaReference, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("media_ref 不能为空")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if err := impl.ensureMediaLoadedLocked(); err != nil {
		return nil, err
	}
	reference := impl.media[ref]
	if reference == nil {
		return nil, nil
	}
	if reference.ExpiresAt > 0 && nowMillis > 0 && nowMillis >= reference.ExpiresAt {
		delete(impl.media, ref)
		_ = impl.saveMediaReferencesLocked(impl.media)
		return nil, nil
	}
	return cloneMediaReference(reference), nil
}

// PruneExpiredMediaReferences 删除已过期媒体映射。
func (impl *serviceImpl) PruneExpiredMediaReferences(nowMillis int64) (int, error) {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if err := impl.ensureMediaLoadedLocked(); err != nil {
		return 0, err
	}
	removed := 0
	for ref, reference := range impl.media {
		if reference == nil || reference.ExpiresAt > 0 && nowMillis > 0 && nowMillis >= reference.ExpiresAt {
			delete(impl.media, ref)
			removed++
		}
	}
	if removed == 0 {
		return 0, nil
	}
	return removed, impl.saveMediaReferencesLocked(impl.media)
}

func (impl *serviceImpl) ensureMediaLoadedLocked() error {
	if impl.mediaLoaded {
		return nil
	}
	media, err := impl.loadMediaReferencesFileLocked()
	if err != nil {
		return err
	}
	impl.media = media
	impl.mediaLoaded = true
	return nil
}

func (impl *serviceImpl) loadMediaReferencesFileLocked() (map[string]*connectordomain.MediaReference, error) {
	media := map[string]*connectordomain.MediaReference{}
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建 Telegram connector state dir 失败: %w", err)
	}
	raw, err := os.ReadFile(impl.mediaReferenceFilePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return media, nil
		}
		return nil, fmt.Errorf("读取 Telegram media_ref 失败: %w", err)
	}
	state := mediaReferenceStateFile{}
	if err = json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("解析 Telegram media_ref 失败: %w", err)
	}
	for _, reference := range state.Items {
		if reference == nil || strings.TrimSpace(reference.Ref) == "" {
			continue
		}
		media[reference.Ref] = reference
	}
	return media, nil
}

func (impl *serviceImpl) saveMediaReferencesLocked(media map[string]*connectordomain.MediaReference) error {
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return fmt.Errorf("创建 Telegram connector state dir 失败: %w", err)
	}
	state := mediaReferenceStateFile{}
	for _, reference := range media {
		if reference == nil || strings.TrimSpace(reference.Ref) == "" {
			continue
		}
		state.Items = append(state.Items, cloneMediaReference(reference))
	}
	return writeJSONFile(impl.mediaReferenceFilePath(), state)
}

func cloneMediaReference(reference *connectordomain.MediaReference) *connectordomain.MediaReference {
	if reference == nil {
		return nil
	}
	cloned := *reference
	return &cloned
}
