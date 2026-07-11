package storageservice

import (
	"context"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/domain"
)

// Service 定义 Telegram connector 本地登录态文件读写能力。
//
// 线程安全性：实现必须允许多个 service goroutine 并发读取和写入。
// 错误语义：持久化目录、JSON 编解码和原子写入失败时返回 error；未命中读取返回空 map 或 nil。
type Service interface {
	// Start 完成本地登录态 storage 服务启动检查。
	Start(ctx context.Context) error
	// Stop 停止本地登录态 storage 服务。
	Stop(ctx context.Context) error
	// StateDir 返回当前 storage 使用的本地目录。
	StateDir() string
	// LoadBots 从内存读取 Telegram bot 登录态；首次调用从 bots.json 恢复，返回值按 bot_id 索引。
	LoadBots() (map[string]*connectordomain.Bot, error)
	// SaveBots 更新内存中的完整 Telegram bot 登录态，并刷新 bots.json 恢复快照。
	SaveBots(bots map[string]*connectordomain.Bot) error
	// SaveBot 更新内存中的单个 Telegram bot 登录态，并刷新 bots.json 恢复快照。
	SaveBot(bot *connectordomain.Bot) error
	// LoadConnectionBindings 从内存读取 connector connection 绑定，返回值按 connector_channel_id 索引。
	LoadConnectionBindings() (map[string]*connectordomain.ConnectionBinding, error)
	// SaveConnectionBindings 更新内存中的完整 connector connection 绑定，并刷新 channels.json 恢复快照。
	SaveConnectionBindings(bindings map[string]*connectordomain.ConnectionBinding) error
	// SaveConnectionBinding 更新内存中的单个 connector connection 绑定，并刷新 channels.json 恢复快照。
	SaveConnectionBinding(binding *connectordomain.ConnectionBinding) error
	// DeleteConnectionBinding 从内存删除指定 connector connection 绑定，并刷新 channels.json 恢复快照。
	DeleteConnectionBinding(connectorChannelID string) error
	// SaveMediaReference 保存 connector 短期媒体 key 映射。
	SaveMediaReference(reference *connectordomain.MediaReference) error
	// GetMediaReference 按 key 读取未过期媒体映射。
	GetMediaReference(ref string, nowMillis int64) (*connectordomain.MediaReference, error)
	// PruneExpiredMediaReferences 删除已过期媒体映射。
	PruneExpiredMediaReferences(nowMillis int64) (int, error)
}
