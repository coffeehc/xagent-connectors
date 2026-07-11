package connectservice

import (
	"context"
	"io"

	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/domain"
)

// Service 定义 Telegram connector 连接编排能力。
//
// 线程安全性：实现必须允许 data plane、长轮询 worker 和 HTTP endpoint 并发调用。
// 错误语义：外部 API、持久化和输入校验失败返回 error；未绑定 channel 查询返回 nil。
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
	// TelegramAPIBaseURL 返回 Telegram Bot API endpoint。
	TelegramAPIBaseURL() string
	// BindMessagePusher 绑定 connector 入站消息推送端口。
	BindMessagePusher(pusher MessagePusher)
	// StartAuth 提交 Telegram bot token 与 chat_id 表单并完成 channel 绑定。
	StartAuth(ctx context.Context, connectorChannelID string, request connectorprotocol.ConnectorAuthStartRequest) (*connectorprotocol.ConnectorAuthStartResult, error)
	// AuthStatus 返回 Telegram 表单绑定认证状态。
	AuthStatus(ctx context.Context, connectorChannelID string, authSessionID string, refresh bool) (*connectorprotocol.ConnectorAuthStatusResult, bool)
	// CancelAuth 取消未完成认证会话；Telegram 表单认证为同步提交，通常返回 not_found。
	CancelAuth(ctx context.Context, connectorChannelID string, authSessionID string) *connectorprotocol.ConnectorAuthCancelResult
	// ConnectionByChannel 根据 connector_channel_id 返回 connection。
	ConnectionByChannel(connectorChannelID string) *connectordomain.Connection
	// LogoutByChannel 根据 connector_channel_id 清理 connector 内绑定。
	LogoutByChannel(connectorChannelID string) bool
	// InvokeTool 在已认证 channel 上执行 connector 声明的 Telegram 工具。
	InvokeTool(ctx context.Context, input ToolInvokeInput) (*ToolInvokeResult, error)
	// UploadMedia 上传待发送媒体到 connector 本地缓存并返回 media_ref。
	UploadMedia(ctx context.Context, input UploadMediaInput) (*UploadMediaResult, error)
	// OpenMedia 打开入站或出站媒体引用内容。
	OpenMedia(ctx context.Context, mediaRef string) (*OpenMediaResult, error)
	// ReadConnectorSkill 读取 connector v1 标准主 Skill 内容。
	ReadConnectorSkill(ctx context.Context) (*ConnectorSkillContent, error)
	// BuildConnectorCard 构建未绑定前公开读取的 Connector Card。
	BuildConnectorCard() *connectorprotocol.ConnectorCard
	// BuildChannelDescriptor 构建指定 channel 的当前 Connection Descriptor。
	BuildChannelDescriptor(connectorChannelID string) *connectorprotocol.ConnectionDescriptor
	// BuildConnectionDescriptor 构建用户绑定后的 Connection Descriptor。
	BuildConnectionDescriptor(connection *connectordomain.Connection) *connectorprotocol.ConnectionDescriptor
}

// ToolInvokeInput 表示 Telegram connector 工具调用请求。
type ToolInvokeInput struct {
	// ConnectorChannelID 是当前工具调用所属的用户级 channel id。
	ConnectorChannelID string
	// ToolID 是 Connector Card 声明的工具 ID。
	ToolID string
	// Arguments 是工具业务参数，不允许包含系统 API key 或目标系统 token。
	Arguments map[string]any
}

// ToolInvokeResult 表示 Telegram connector 工具执行结果。
type ToolInvokeResult struct {
	// ToolID 是本次执行的工具 ID。
	ToolID string `json:"tool_id"`
	// Result 是工具返回给 xAgent runtime 的业务结果。
	Result map[string]any `json:"result,omitempty"`
}

// UploadMediaInput 表示 Telegram connector 媒体上传请求。
type UploadMediaInput struct {
	// ConnectorChannelID 是当前上传所属的用户级 channel id。
	ConnectorChannelID string
	// Filename 是上传文件名，用于推断媒体类型和文件消息展示。
	Filename string
	// ContentType 是 HTTP 上传携带的 MIME 类型，可为空。
	ContentType string
	// Source 是上传内容流。
	Source io.Reader
	// Size 是文件大小，单位字节；未知时可为 0。
	Size int64
}

// UploadMediaResult 表示 Telegram connector 媒体上传结果。
type UploadMediaResult struct {
	// MediaRef 是后续 telegram_message_send_media 工具使用的 connector 内部媒体引用。
	MediaRef string `json:"media_ref"`
	// MediaType 是 connector 归一化后的媒体类型：image、video 或 file。
	MediaType string `json:"media_type"`
	// Filename 是发送文件类消息时展示给 Telegram 用户的文件名。
	Filename string `json:"filename,omitempty"`
	// ByteSize 是文件大小，单位字节。
	ByteSize int64 `json:"byte_size"`
	// ExpiresAt 是 media_ref 过期时间，单位毫秒时间戳。
	ExpiresAt int64 `json:"expires_at"`
}

// OpenMediaResult 表示 Telegram connector 媒体下载结果。
type OpenMediaResult struct {
	// ContentType 是媒体 MIME 类型，可为空。
	ContentType string
	// Filename 是文件展示名，可为空。
	Filename string
	// Body 是完整文件内容。
	Body []byte
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

// ConnectorSkillContent 表示 connector 主 Skill 读取结果。
type ConnectorSkillContent struct {
	// SkillID 是 connector 主 Skill 的稳定 ID。
	SkillID string
	// ContentType 是 HTTP 响应 Content-Type。
	ContentType string
	// Content 是 Skill Markdown 内容。
	Content string
	// SHA256 是 Skill Markdown 内容的 sha256 hex。
	SHA256 string
}
