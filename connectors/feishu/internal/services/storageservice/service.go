package storageservice

import (
	"context"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/domain"
)

// Service 定义飞书 connector 本地状态持久化能力。
type Service interface {
	// Start 检查并加载状态目录。
	Start(ctx context.Context) error
	// Stop 停止 storage service。
	Stop(ctx context.Context) error
	// StateDir 返回状态目录。
	StateDir() string
	// MediaCacheDir 返回媒体缓存目录。
	MediaCacheDir() string
	// LoadApps 加载全部应用绑定。
	LoadApps() (map[string]*connectordomain.AppBinding, error)
	// SaveApps 保存全部应用绑定。
	SaveApps(apps map[string]*connectordomain.AppBinding) error
	// LoadReferences 加载回复和媒体引用。
	LoadReferences() (map[string]*connectordomain.ReplyReference, map[string]*connectordomain.MediaReference, error)
	// SaveReferences 保存回复和媒体引用。
	SaveReferences(reply map[string]*connectordomain.ReplyReference, media map[string]*connectordomain.MediaReference) error
}

// DefaultStateDir 返回 connector 默认状态目录。
func DefaultStateDir(connectorID string) string { return defaultStateDir(connectorID) }
