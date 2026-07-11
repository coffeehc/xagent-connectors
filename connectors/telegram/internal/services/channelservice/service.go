package channelservice

import (
	"context"

	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	"github.com/gofiber/fiber/v3"
)

// Service 定义 xAgent 与 Telegram connector 的 WebSocket data plane 能力。
//
// 线程安全性：实现必须允许多个 WebSocket channel 并发读写。
// 错误语义：channel 未打开、写入失败或 packet 不合法时返回 error 或写回 error packet。
type Service interface {
	// Start 完成 data plane channel 服务启动检查。
	Start(ctx context.Context) error
	// Stop 停止 data plane channel 服务。
	Stop(ctx context.Context) error
	// HandleDataPlane 处理 WebSocket data plane 连接。
	HandleDataPlane(c fiber.Ctx) error
	// PushMessage 将 connector 已收到的目标系统消息推送到 xAgent data plane。
	PushMessage(ctx context.Context, input MessagePushInput) error
	// PushConnectionDescriptor 将 connector 侧 connection 状态变化推送到 xAgent data plane。
	PushConnectionDescriptor(ctx context.Context, input DescriptorPushInput) error
}

// MessagePushInput 表示 connector 主动推送给 xAgent 的入站消息。
type MessagePushInput struct {
	// ConnectorChannelID 是入站消息所属的用户 channel。
	ConnectorChannelID string
	// Payload 是 message.push 的业务负载，由具体 connector 按协议填充。
	Payload map[string]any
}

// DescriptorPushInput 表示 connector 主动推送给 xAgent 的 connection descriptor 更新。
type DescriptorPushInput struct {
	// ConnectorChannelID 是状态变化所属的用户 channel。
	ConnectorChannelID string
	// Descriptor 是 connector 当前计算出的用户态 connection descriptor。
	Descriptor *connectorprotocol.ConnectionDescriptor
}
