package connectservice

import (
	"context"
	"io"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/domain"
	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
)

// Service 定义飞书 connector 连接编排能力。
type Service interface {
	// Start 恢复应用绑定并启动长连接。
	Start(ctx context.Context) error
	// Stop 停止全部长连接和认证会话。
	Stop(ctx context.Context) error
	// APIKey 返回 connector system API key。
	APIKey() string
	// ConnectorID 返回 Connector Card ID。
	ConnectorID() string
	// StateDir 返回本地状态目录。
	StateDir() string
	// BindMessagePusher 绑定入站消息推送端口。
	BindMessagePusher(pusher MessagePusher)
	// StartAuth 创建扫码创建飞书应用的认证会话。
	StartAuth(ctx context.Context, connectorChannelID string, request connectorprotocol.ConnectorAuthStartRequest) (*connectorprotocol.ConnectorAuthStartResult, error)
	// AuthStatus 返回认证状态。
	AuthStatus(ctx context.Context, connectorChannelID string, authSessionID string, refresh bool) (*connectorprotocol.ConnectorAuthStatusResult, bool)
	// CancelAuth 取消认证会话。
	CancelAuth(ctx context.Context, connectorChannelID string, authSessionID string) *connectorprotocol.ConnectorAuthCancelResult
	// ConnectionByChannel 返回 channel connection。
	ConnectionByChannel(connectorChannelID string) *connectordomain.Connection
	// LogoutByChannel 删除 connector 内应用凭据并停止长连接。
	LogoutByChannel(connectorChannelID string) bool
	// InvokeTool 执行飞书发送或回复工具。
	InvokeTool(ctx context.Context, input ToolInvokeInput) (*ToolInvokeResult, error)
	// UploadMedia 上传图片到飞书并创建 media_ref。
	UploadMedia(ctx context.Context, input UploadMediaInput) (*UploadMediaResult, error)
	// OpenMedia 打开已缓存的入站图片。
	OpenMedia(ctx context.Context, mediaRef string) (*OpenMediaResult, error)
	// ReadConnectorSkill 读取 connector 主 Skill。
	ReadConnectorSkill(ctx context.Context) (*ConnectorSkillContent, error)
	// BuildConnectorCard 构建 Connector Card。
	BuildConnectorCard() *connectorprotocol.ConnectorCard
	// BuildChannelDescriptor 构建 channel descriptor。
	BuildChannelDescriptor(connectorChannelID string) *connectorprotocol.ConnectionDescriptor
	// BuildConnectionDescriptor 构建已认证 connection descriptor。
	BuildConnectionDescriptor(connection *connectordomain.Connection) *connectorprotocol.ConnectionDescriptor
}

// ToolInvokeInput 表示飞书工具调用请求。
type ToolInvokeInput struct {
	// ConnectorChannelID 是当前工具调用所属的用户级 channel ID。
	ConnectorChannelID string
	// ToolID 是 Connector Card 声明的工具 ID。
	ToolID string
	// Arguments 是工具业务参数。
	Arguments map[string]any
}

// ToolInvokeResult 表示飞书工具执行结果。
type ToolInvokeResult struct {
	// ToolID 是本次执行的工具 ID。
	ToolID string `json:"tool_id"`
	// Result 是返回给 xAgent runtime 的业务结果。
	Result map[string]any `json:"result,omitempty"`
}

// UploadMediaInput 表示图片上传请求。
type UploadMediaInput struct {
	// ConnectorChannelID 是当前上传所属的用户级 channel ID。
	ConnectorChannelID string
	// Filename 是上传文件名。
	Filename string
	// ContentType 是 HTTP 上传携带的 MIME 类型。
	ContentType string
	// Source 是上传内容流。
	Source io.Reader
	// Size 是图片大小，单位字节。
	Size int64
}

// UploadMediaResult 表示图片上传结果。
type UploadMediaResult struct {
	// MediaRef 是 connector 内部图片引用。
	MediaRef string `json:"media_ref"`
	// MediaType 固定为 image。
	MediaType string `json:"media_type"`
	// Filename 是原始文件名。
	Filename string `json:"filename,omitempty"`
	// ByteSize 是图片大小，单位字节。
	ByteSize int64 `json:"byte_size"`
	// ExpiresAt 是引用过期时间，Unix 毫秒。
	ExpiresAt int64 `json:"expires_at"`
}

// OpenMediaResult 表示媒体下载结果。
type OpenMediaResult struct {
	// ContentType 是图片 MIME 类型。
	ContentType string
	// Filename 是图片文件名。
	Filename string
	// Body 是图片完整内容。
	Body []byte
}

// Config 是 connect service 启动配置。
type Config struct {
	// APIKey 是 xAgent 调用 connector 控制面和数据面的 system API key。
	APIKey string
}

// MessagePusher 定义入站消息推送窄接口。
type MessagePusher interface {
	// PushMessage 将飞书消息推送到 xAgent data plane。
	PushMessage(ctx context.Context, connectorChannelID string, payload map[string]any) error
	// PushConnectionDescriptor 推送 connection 状态变化。
	PushConnectionDescriptor(ctx context.Context, connectorChannelID string, descriptor *connectorprotocol.ConnectionDescriptor) error
}

// ConnectorSkillContent 表示 connector 主 Skill 内容。
type ConnectorSkillContent struct {
	// SkillID 是 connector 主 Skill 的稳定 ID。
	SkillID string
	// ContentType 是 HTTP 响应 Content-Type。
	ContentType string
	// Content 是 Skill Markdown 内容。
	Content string
	// SHA256 是 Skill Markdown 的 sha256 hex。
	SHA256 string
}
