package protocol

// ConnectorAuthFlowType 表示 connector 支持的认证交互类型。
type ConnectorAuthFlowType string

const (
	// ConnectorAuthFlowTypeQRLogin 表示二维码登录型认证。
	ConnectorAuthFlowTypeQRLogin ConnectorAuthFlowType = "qr_login"
	// ConnectorAuthFlowTypeForm 表示动态表单提交型认证。
	ConnectorAuthFlowTypeForm ConnectorAuthFlowType = "form"
)

// ConnectorAuthInputType 表示认证表单字段的输入控件类型。
type ConnectorAuthInputType string

const (
	// ConnectorAuthInputTypeText 表示普通文本输入框。
	ConnectorAuthInputTypeText ConnectorAuthInputType = "text"
	// ConnectorAuthInputTypePassword 表示敏感输入框。
	ConnectorAuthInputTypePassword ConnectorAuthInputType = "password"
)

// ConnectorAuthFlowField 表示 Connector Card 中一个动态认证表单字段。
type ConnectorAuthFlowField struct {
	// Name 表示提交字段名，必须在同一 auth flow 内唯一。
	Name string `json:"name"`
	// Label 表示前端展示名称。
	Label string `json:"label"`
	// InputType 表示输入控件类型，例如 text、password。
	InputType ConnectorAuthInputType `json:"input_type"`
	// Required 表示该字段是否必填。
	Required bool `json:"required,omitempty"`
	// Placeholder 表示输入框占位提示，可为空。
	Placeholder string `json:"placeholder,omitempty"`
	// HelpText 表示字段帮助说明，可为空。
	HelpText string `json:"help_text,omitempty"`
	// Secret 表示字段是否属于敏感输入，前端应避免明文展示。
	Secret bool `json:"secret,omitempty"`
	// DefaultValue 表示字段默认值，可为空；敏感字段禁止设置默认值。
	DefaultValue string `json:"default_value,omitempty"`
}

// ConnectorAuthStartRequest 表示 xAgent 发给 connector 的认证启动请求。
type ConnectorAuthStartRequest struct {
	// FlowID 表示本次认证使用的 Connector Card auth flow id。
	FlowID string `json:"flow_id"`
	// Input 表示动态表单认证提交的字段值；非表单认证可为空。
	Input map[string]string `json:"input,omitempty"`
}

// ConnectorAuthStartStatus 表示 connector 用户连接初始化后的当前状态。
type ConnectorAuthStartStatus string

const (
	// ConnectorAuthStartPending 表示用户连接初始化会话已经创建，正在等待用户操作。
	ConnectorAuthStartPending ConnectorAuthStartStatus = "pending"
	// ConnectorAuthStartScanned 表示用户已经扫码但尚未确认。
	ConnectorAuthStartScanned ConnectorAuthStartStatus = "scanned"
	// ConnectorAuthStartAuthenticated 表示当前 Connector channel 已经完成目标系统认证。
	ConnectorAuthStartAuthenticated ConnectorAuthStartStatus = "authenticated"
	// ConnectorAuthStartExpired 表示认证会话已过期。
	ConnectorAuthStartExpired ConnectorAuthStartStatus = "expired"
	// ConnectorAuthStartQRRefreshRequired 表示二维码或认证材料需要刷新。
	ConnectorAuthStartQRRefreshRequired ConnectorAuthStartStatus = "qr_refresh_required"
	// ConnectorAuthStartFailed 表示认证流程启动或首轮认证失败。
	ConnectorAuthStartFailed ConnectorAuthStartStatus = "failed"
)

// ConnectorAuthStatus 表示 connector 用户认证轮询状态。
type ConnectorAuthStatus string

const (
	// ConnectorAuthStatusPending 表示仍在等待扫码或授权确认。
	ConnectorAuthStatusPending ConnectorAuthStatus = "pending"
	// ConnectorAuthStatusScanned 表示二维码已被扫码但尚未确认。
	ConnectorAuthStatusScanned ConnectorAuthStatus = "scanned"
	// ConnectorAuthStatusAuthenticated 表示用户认证已经完成。
	ConnectorAuthStatusAuthenticated ConnectorAuthStatus = "authenticated"
	// ConnectorAuthStatusUnauthenticated 表示 channel 已打开但用户尚未完成认证。
	ConnectorAuthStatusUnauthenticated ConnectorAuthStatus = "unauthenticated"
	// ConnectorAuthStatusExpired 表示认证会话已过期。
	ConnectorAuthStatusExpired ConnectorAuthStatus = "expired"
	// ConnectorAuthStatusQRRefreshRequired 表示二维码或认证材料需要刷新。
	ConnectorAuthStatusQRRefreshRequired ConnectorAuthStatus = "qr_refresh_required"
	// ConnectorAuthStatusFailed 表示认证流程失败。
	ConnectorAuthStatusFailed ConnectorAuthStatus = "failed"
)

// ConnectorAuthCancelStatus 表示 connector auth.cancel 请求的处理结果。
type ConnectorAuthCancelStatus string

const (
	// ConnectorAuthCancelStatusCanceled 表示未完成认证会话已经被取消。
	ConnectorAuthCancelStatusCanceled ConnectorAuthCancelStatus = "canceled"
	// ConnectorAuthCancelStatusIgnored 表示认证已经完成，cancel 被安全丢弃。
	ConnectorAuthCancelStatusIgnored ConnectorAuthCancelStatus = "ignored"
	// ConnectorAuthCancelStatusNotFound 表示没有找到需要取消的认证会话。
	ConnectorAuthCancelStatusNotFound ConnectorAuthCancelStatus = "not_found"
)

// ConnectorAuthStartResult 表示 connector auth.start 返回给 xAgent 的用户连接初始化结果。
type ConnectorAuthStartResult struct {
	// ConnectorChannelID 表示本次认证所属用户级 Connector channel，可为空。
	ConnectorChannelID string `json:"connector_channel_id,omitempty"`
	// FlowID 表示本次认证使用的 Connector Card auth flow id。
	FlowID string `json:"flow_id"`
	// AuthSessionID 表示 connector 返回的认证会话 ID。
	AuthSessionID string `json:"auth_session_id"`
	// Status 表示用户连接初始化后的状态。
	Status ConnectorAuthStartStatus `json:"status"`
	// QRCodeText 表示二维码原始内容，前端可据此生成二维码或直接展示，可为空。
	QRCodeText string `json:"qr_code_text,omitempty"`
	// QRCodeImage 表示 connector 已生成的二维码图片 URL 或 data URL，可为空。
	QRCodeImage string `json:"qr_code_image,omitempty"`
	// ExpiresAt 表示认证会话过期时间，Unix 毫秒；未设置时为空。
	ExpiresAt int64 `json:"expires_at,omitempty"`
	// PollIntervalMillis 表示前端建议轮询间隔，毫秒；未设置时为空。
	PollIntervalMillis int64 `json:"poll_interval_millis,omitempty"`
	// Message 表示 connector 返回的人类可读状态说明，可为空。
	Message string `json:"message,omitempty"`
	// ConnectionDescriptor 表示 connector 已有登录态时直接返回的绑定实例描述，可为空。
	ConnectionDescriptor *ConnectionDescriptor `json:"connection_descriptor,omitempty"`
}

// ConnectorAuthStatusResult 表示 connector auth.status 返回给 xAgent 的轮询结果。
type ConnectorAuthStatusResult struct {
	// ConnectorChannelID 表示本次认证所属用户级 Connector channel，可为空。
	ConnectorChannelID string `json:"connector_channel_id,omitempty"`
	// FlowID 表示本次认证使用的 Connector Card auth flow id。
	FlowID string `json:"flow_id"`
	// AuthSessionID 表示 connector 返回的认证会话 ID。
	AuthSessionID string `json:"auth_session_id"`
	// Status 表示当前认证状态。
	Status ConnectorAuthStatus `json:"status"`
	// Message 表示 connector 返回的人类可读状态说明，可为空。
	Message string `json:"message,omitempty"`
	// QRCodeText 表示刷新后的二维码原始内容，前端可据此生成二维码或直接展示，可为空。
	QRCodeText string `json:"qr_code_text,omitempty"`
	// QRCodeImage 表示刷新后的二维码图片 URL 或 data URL，可为空。
	QRCodeImage string `json:"qr_code_image,omitempty"`
	// ConnectionDescriptor 表示认证成功后当前用户绑定的目标实例描述，可为空。
	ConnectionDescriptor *ConnectionDescriptor `json:"connection_descriptor,omitempty"`
	// ExpiresAt 表示认证会话过期时间，Unix 毫秒；未设置时为空。
	ExpiresAt int64 `json:"expires_at,omitempty"`
	// PollIntervalMillis 表示前端建议轮询间隔，毫秒；未设置时为空。
	PollIntervalMillis int64 `json:"poll_interval_millis,omitempty"`
}

// ConnectorAuthCancelResult 表示 connector auth.cancel 返回给 xAgent 的取消结果。
type ConnectorAuthCancelResult struct {
	// ConnectorChannelID 表示本次取消请求所属用户级 Connector channel，可为空。
	ConnectorChannelID string `json:"connector_channel_id,omitempty"`
	// AuthSessionID 表示被取消或被忽略的 connector auth session id，可为空。
	AuthSessionID string `json:"auth_session_id,omitempty"`
	// Status 表示 cancel 请求处理结果。
	Status ConnectorAuthCancelStatus `json:"status"`
	// AuthStatus 表示 cancel 后认证会话或 channel 的认证状态，可为空。
	AuthStatus ConnectorAuthStatus `json:"auth_status,omitempty"`
	// Message 表示 connector 返回的人类可读状态说明，可为空。
	Message string `json:"message,omitempty"`
	// ConnectionDescriptor 表示 cancel 被忽略且认证已完成时的当前绑定实例描述，可为空。
	ConnectionDescriptor *ConnectionDescriptor `json:"connection_descriptor,omitempty"`
}
