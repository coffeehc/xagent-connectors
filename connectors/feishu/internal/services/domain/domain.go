package domain

// AppBinding 表示 connector channel 创建并持有的飞书应用凭据。
type AppBinding struct {
	// ConnectorChannelID 是 xAgent 用户级 connector channel ID。
	ConnectorChannelID string `json:"connector_channel_id"`
	// AppID 是飞书应用 ID，仅保存在 connector 内部。
	AppID string `json:"app_id"`
	// AppSecret 是飞书应用密钥，仅保存在 connector 内部。
	AppSecret string `json:"app_secret"`
	// TenantOpenID 是扫码创建应用的飞书用户 open_id，可为空。
	TenantOpenID string `json:"tenant_open_id,omitempty"`
	// DefaultChatID 是默认智能体单聊的飞书 chat_id，可为空。
	DefaultChatID string `json:"default_chat_id,omitempty"`
	// DefaultSenderOpenID 是默认智能体单聊用户的 open_id，可为空。
	DefaultSenderOpenID string `json:"default_sender_open_id,omitempty"`
	// DefaultChatBoundAt 是默认智能体单聊最近绑定时间，Unix 毫秒。
	DefaultChatBoundAt int64 `json:"default_chat_bound_at,omitempty"`
	// CreatedAt 是应用绑定创建时间，Unix 毫秒。
	CreatedAt int64 `json:"created_at"`
	// LastInboundAt 是最近入站消息时间，Unix 毫秒。
	LastInboundAt int64 `json:"last_inbound_at,omitempty"`
}

// Connection 表示飞书应用绑定的运行时 connection 投影。
type Connection struct {
	// Token 是 connector_channel_id。
	Token string `json:"token"`
	// AppID 是飞书应用 ID。
	AppID string `json:"app_id"`
	// DisplayName 是用户看到的连接名称。
	DisplayName string `json:"display_name"`
	// AccountHint 是脱敏后的应用提示。
	AccountHint string `json:"account_hint"`
	// CreatedAt 是绑定创建时间，Unix 毫秒。
	CreatedAt int64 `json:"created_at"`
}

// AuthSession 表示一键创建飞书应用的扫码认证会话。
type AuthSession struct {
	// ID 是认证会话 ID。
	ID string
	// ConnectorChannelID 是所属 connector channel。
	ConnectorChannelID string
	// FlowID 是认证流程 ID。
	FlowID string
	// Status 是 connector 协议认证状态字符串。
	Status string
	// Message 是当前状态说明。
	Message string
	// RemoteStatus 是飞书 SDK 最近一次报告的注册状态。
	RemoteStatus string
	// RemoteErrorCode 是飞书最近一次终态错误码。
	RemoteErrorCode string
	// QRCodeText 是二维码原始 URL。
	QRCodeText string
	// QRCodeImage 是 PNG data URL。
	QRCodeImage string
	// ExpiresAt 是过期时间，Unix 毫秒。
	ExpiresAt int64
	// PollIntervalMillis 是建议前端轮询间隔，单位毫秒。
	PollIntervalMillis int64
	// CreatedAt 是认证会话创建时间，Unix 毫秒。
	CreatedAt int64
	// RemoteStatusAt 是最近一次收到飞书状态变化的时间，Unix 毫秒。
	RemoteStatusAt int64
	// Attempt 是当前二维码注册尝试编号，用于隔离刷新前后的异步结果。
	Attempt int64
}

// ReplyReference 表示一次飞书入站消息的内部回复引用。
type ReplyReference struct {
	// Ref 是返回给 xAgent 的不透明引用。
	Ref string `json:"ref"`
	// ConnectorChannelID 是所属 connector channel。
	ConnectorChannelID string `json:"connector_channel_id"`
	// AppID 是所属飞书应用 ID。
	AppID string `json:"app_id"`
	// MessageID 是待回复飞书消息 ID。
	MessageID string `json:"message_id"`
	// ChatID 是消息所在会话 ID。
	ChatID string `json:"chat_id"`
	// ThreadID 是消息所属话题 ID，可为空。
	ThreadID string `json:"thread_id,omitempty"`
	// ExpiresAt 是引用过期时间，Unix 毫秒。
	ExpiresAt int64 `json:"expires_at"`
}

// MediaReference 表示 connector 管理的飞书图片引用。
type MediaReference struct {
	// Ref 是返回给 xAgent 的不透明引用。
	Ref string `json:"ref"`
	// Direction 是 inbound 或 outbound。
	Direction string `json:"direction"`
	// ConnectorChannelID 是所属 connector channel。
	ConnectorChannelID string `json:"connector_channel_id"`
	// AppID 是所属飞书应用 ID。
	AppID string `json:"app_id"`
	// ImageKey 是飞书图片 key，仅出站引用使用。
	ImageKey string `json:"image_key,omitempty"`
	// LocalPath 是入站图片本地缓存路径。
	LocalPath string `json:"local_path,omitempty"`
	// Filename 是图片文件名。
	Filename string `json:"filename,omitempty"`
	// ContentType 是图片 MIME 类型。
	ContentType string `json:"content_type,omitempty"`
	// ByteSize 是图片大小。
	ByteSize int64 `json:"byte_size"`
	// ExpiresAt 是引用过期时间，Unix 毫秒。
	ExpiresAt int64 `json:"expires_at"`
}
