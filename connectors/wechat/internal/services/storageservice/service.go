package storageservice

import (
	"context"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
)

// Service 定义 connector 本地登录态文件读写能力。
type Service interface {
	// Start 完成本地登录态 storage 服务启动检查。
	Start(ctx context.Context) error
	// Stop 停止本地登录态 storage 服务。
	Stop(ctx context.Context) error
	// StateDir 返回当前 storage 使用的本地目录。
	StateDir() string
	// LoadBots 从内存读取微信 bot 登录态；首次调用从 bots.json 恢复，返回值按 bot_account_id 索引。
	LoadBots() (map[string]*connectordomain.Bot, error)
	// SaveBots 更新内存中的完整微信 bot 登录态，并刷新 bots.json 恢复快照。
	SaveBots(bots map[string]*connectordomain.Bot) error
	// SaveBot 更新内存中的单个微信 bot 登录态，并刷新 bots.json 恢复快照。
	SaveBot(bot *connectordomain.Bot) error
	// LoadConnectionBindings 从内存读取 connector connection 绑定；首次调用从兼容的 channels.json 恢复，返回值按 connector_channel_id 索引。
	LoadConnectionBindings() (map[string]*connectordomain.ConnectionBinding, error)
	// SaveConnectionBindings 更新内存中的完整 connector connection 绑定，并刷新兼容的 channels.json 恢复快照。
	SaveConnectionBindings(bindings map[string]*connectordomain.ConnectionBinding) error
	// SaveConnectionBinding 更新内存中的单个 connector connection 绑定，并刷新兼容的 channels.json 恢复快照。
	SaveConnectionBinding(binding *connectordomain.ConnectionBinding) error
	// DeleteConnectionBinding 从内存删除指定 connector connection 绑定，并刷新兼容的 channels.json 恢复快照。
	DeleteConnectionBinding(connectorChannelID string) error
	// LoadLegacyConnections 从旧 connections.json 读取历史状态，仅用于启动迁移。
	LoadLegacyConnections() (map[string]*connectordomain.Connection, error)
	// EnqueuePendingInboundMessage 将微信入站消息写入本地待投递队列。
	EnqueuePendingInboundMessage(message *connectordomain.PendingInboundMessage, maxMessagesPerChannel int) error
	// ListPendingInboundMessages 返回指定 channel 尚未投递、未越过 channel 游标且未过期的入站消息。
	ListPendingInboundMessages(connectorChannelID string, nowMillis int64, limit int) ([]*connectordomain.PendingInboundMessage, error)
	// MarkPendingInboundDelivered 标记指定入站消息已成功投递并推进 channel 投递游标。
	MarkPendingInboundDelivered(connectorChannelID string, messageID string, deliveredAt int64) error
	// MarkPendingInboundFailed 记录指定入站消息最近一次投递失败。
	MarkPendingInboundFailed(connectorChannelID string, messageID string, failedAt int64, reason string) error
	// PruneExpiredPendingInboundMessages 删除已过期或已被 channel 游标覆盖的入站消息。
	PruneExpiredPendingInboundMessages(nowMillis int64) (int, error)
	// LoadInboundChannelCursor 读取指定 channel 的本地投递游标。
	LoadInboundChannelCursor(connectorChannelID string) (*connectordomain.InboundChannelCursor, error)
	// SaveMediaReference 保存 connector 短期媒体 key 映射。
	SaveMediaReference(reference *connectordomain.MediaReference) error
	// GetMediaReference 按 key 读取未过期媒体映射。
	GetMediaReference(ref string, nowMillis int64) (*connectordomain.MediaReference, error)
	// PruneExpiredMediaReferences 删除已过期媒体映射。
	PruneExpiredMediaReferences(nowMillis int64) (int, error)
}
