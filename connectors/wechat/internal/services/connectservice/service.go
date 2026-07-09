package connectservice

import (
	"context"
	"io"

	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
)

// Service 定义 connector 连接编排能力。
type Service interface {
	// Start 完成 connector 连接编排服务启动检查。
	Start(ctx context.Context) error
	// Stop 停止 connector 连接编排服务。
	Stop(ctx context.Context) error
	// APIKey 返回 connector server 的 system API key。
	APIKey() string
	// ConnectorID 返回 Connector Card id。
	ConnectorID() string
	// StateDir 返回本地登录态目录。
	StateDir() string
	// WeChatAPIBaseURL 返回微信 iLink API endpoint。
	WeChatAPIBaseURL() string
	// WeChatBotType 返回微信 iLink bot_type。
	WeChatBotType() string
	// BindMessagePusher 绑定 connector 入站消息推送端口。
	BindMessagePusher(pusher MessagePusher)
	// FlushInboundMessages 投递指定 channel 本地缓存的入站消息。
	FlushInboundMessages(ctx context.Context, connectorChannelID string)
	// StartAuth 创建微信二维码登录认证会话。
	StartAuth(ctx context.Context, connectorChannelID string, flowID string) (*connectorprotocol.ConnectorAuthStartResult, error)
	// AuthStatus 刷新并返回微信二维码登录认证状态。
	AuthStatus(ctx context.Context, connectorChannelID string, authSessionID string, refresh bool) (*connectorprotocol.ConnectorAuthStatusResult, bool)
	// CancelAuth 取消未完成的微信二维码登录认证会话；若认证已经完成则安全忽略。
	CancelAuth(ctx context.Context, connectorChannelID string, authSessionID string) *connectorprotocol.ConnectorAuthCancelResult
	// ConnectionByChannel 根据 connector_channel_id 返回 connection。
	ConnectionByChannel(connectorChannelID string) *connectordomain.Connection
	// LogoutByChannel 根据 connector_channel_id 清理 connector 内登录态。
	LogoutByChannel(connectorChannelID string) bool
	// InvokeTool 在已认证 channel 上执行 connector 声明的微信工具。
	InvokeTool(ctx context.Context, input ToolInvokeInput) (*ToolInvokeResult, error)
	// UploadMedia 上传待发送媒体到微信 CDN 并返回 connector 内部 media_ref。
	UploadMedia(ctx context.Context, input UploadMediaInput) (*UploadMediaResult, error)
	// ReadConnectorSkill 读取 connector v1 标准主 Skill 内容。
	ReadConnectorSkill(ctx context.Context) (*ConnectorSkillContent, error)
	// BuildConnectorCard 构建未绑定前公开读取的 Connector Card。
	BuildConnectorCard() *connectorprotocol.ConnectorCard
	// BuildChannelDescriptor 构建指定 channel 的当前 Connection Descriptor。
	BuildChannelDescriptor(connectorChannelID string) *connectorprotocol.ConnectionDescriptor
	// BuildConnectionDescriptor 构建用户绑定后的 Connection Descriptor。
	BuildConnectionDescriptor(connection *connectordomain.Connection) *connectorprotocol.ConnectionDescriptor
}

// ToolInvokeInput 表示微信 connector 工具调用请求。
type ToolInvokeInput struct {
	// ConnectorChannelID 是当前工具调用所属的用户级 channel id。
	ConnectorChannelID string
	// ToolID 是 Connector Card 声明的工具 ID。
	ToolID string
	// Arguments 是工具业务参数，不允许包含系统 API key 或目标系统 token。
	Arguments map[string]any
}

// ToolInvokeResult 表示微信 connector 工具执行结果。
type ToolInvokeResult struct {
	// ToolID 是本次执行的工具 ID。
	ToolID string `json:"tool_id"`
	// Result 是工具返回给 xAgent runtime 的业务结果。
	Result map[string]any `json:"result,omitempty"`
}

// UploadMediaInput 表示微信 connector 媒体上传请求。
type UploadMediaInput struct {
	// ConnectorChannelID 是当前上传所属的用户级 channel id。
	ConnectorChannelID string
	// RecipientRef 是可选的微信 to_user_id；为空时使用当前认证 connection 的默认接收人。
	RecipientRef string
	// Filename 是上传文件名，用于推断媒体类型和文件消息展示。
	Filename string
	// ContentType 是 HTTP 上传携带的 MIME 类型，可为空。
	ContentType string
	// Source 是上传内容流。
	Source io.Reader
	// Size 是明文文件大小，单位字节；未知时可为 0。
	Size int64
}

// UploadMediaResult 表示微信 connector 媒体上传结果。
type UploadMediaResult struct {
	// MediaRef 是后续 wechat_message_send_media 工具使用的 connector 内部媒体引用。
	MediaRef string `json:"media_ref"`
	// MediaType 是 connector 归一化后的媒体类型：image、video 或 file。
	MediaType string `json:"media_type"`
	// Filename 是发送文件类消息时展示给微信用户的文件名。
	Filename string `json:"filename,omitempty"`
	// ByteSize 是明文文件大小，单位字节。
	ByteSize int64 `json:"byte_size"`
	// ExpiresAt 是 media_ref 过期时间，单位毫秒时间戳。
	ExpiresAt int64 `json:"expires_at"`
}

// Config 是 connect service 的启动配置。
type Config struct {
	// APIKey 是 xAgent 调用 connector 控制面和数据面的 system API key。
	APIKey string
}

// MessagePusher 定义 connect service 推送入站消息到 data plane 的窄接口。
type MessagePusher interface {
	// PushMessage 将 connector 已收到的目标系统消息推送到 xAgent data plane。
	PushMessage(ctx context.Context, connectorChannelID string, payload map[string]any) error
	// PushConnectionDescriptor 将 connector 侧 connection 状态变化推送到 xAgent data plane。
	PushConnectionDescriptor(ctx context.Context, connectorChannelID string, descriptor *connectorprotocol.ConnectionDescriptor) error
}
