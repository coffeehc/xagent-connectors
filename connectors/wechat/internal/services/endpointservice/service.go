package endpointservice

import "context"

// Service 定义微信 connector HTTP endpoint 服务生命周期能力。
type Service interface {
	// Start 启动微信 connector HTTP endpoint 服务。
	Start(ctx context.Context) error
	// Stop 停止 connector HTTP endpoint 服务。
	Stop(ctx context.Context) error
}
