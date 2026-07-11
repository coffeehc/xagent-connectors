package endpointservice

import "context"

// Service 定义 Telegram Connector Server HTTP endpoint 能力。
//
// 线程安全性：HTTP server 由底层 httpx 管理，Start/Stop 按插件生命周期串行调用。
// 错误语义：启动监听失败、关闭失败或依赖服务异常时返回 error。
type Service interface {
	// Start 启动 Telegram Connector Server HTTP endpoint 服务。
	Start(ctx context.Context) error
	// Stop 停止 Telegram Connector Server HTTP endpoint 服务。
	Stop(ctx context.Context) error
}
