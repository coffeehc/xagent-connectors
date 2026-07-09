package domain

import connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"

// AuthSession 表示一次微信二维码登录授权会话。
type AuthSession struct {
	// ID 是 connector 内部认证会话 id。
	ID string
	// ConnectorChannelID 是 Connector 分配的用户级 channel id。
	ConnectorChannelID string
	// FlowID 是本次认证使用的 auth flow id。
	FlowID string
	// QRText 是用于生成二维码图片的微信扫码载荷。
	QRText string
	// QRCodeImage 是 xAgent 前端可直接展示的二维码 data URL。
	QRCodeImage string
	// WeChatQRCode 是微信 iLink 返回的二维码会话标识。
	WeChatQRCode string
	// PollingBaseURL 是后续轮询二维码状态使用的微信 API base URL。
	PollingBaseURL string
	// LocalBotTokens 是本次获取二维码时提供给微信后端的本地 bot token 候选。
	LocalBotTokens []string
	// QRRefreshCount 是当前认证会话自动刷新二维码材料的次数。
	QRRefreshCount int
	// QRCodeRefreshRequired 表示微信状态接口明确要求当前二维码换新。
	QRCodeRefreshRequired bool
	// QRCodeDelivered 表示当前二维码材料已经通过 auth.status 返回给 xAgent 用户端。
	QRCodeDelivered bool
	// Status 是当前认证会话状态。
	Status connectorprotocol.ConnectorAuthStatus
	// Message 是给用户展示的认证状态说明。
	Message string
	// ExpiresAt 是认证会话过期时间，单位毫秒时间戳。
	ExpiresAt int64
	// CreatedAt 是认证会话创建时间，单位毫秒时间戳。
	CreatedAt int64
	// ConfirmedAt 是用户完成扫码确认时间，单位毫秒时间戳。
	ConfirmedAt int64
	// Token 是认证成功后绑定的 connector_channel_id。
	Token string
	// Cancelled 表示用户已经取消该二维码认证会话；认证成功后不得再置为 true。
	Cancelled bool
}

// Bot 表示 connector 内部持久化的微信 bot 账号登录态。
type Bot struct {
	// WeChatUserID 是微信 iLink 返回的用户标识。
	WeChatUserID string `json:"wechat_user_id,omitempty"`
	// BotToken 是 connector 本地持有的微信 bot token。
	BotToken string `json:"bot_token"`
	// BotAccountID 是微信 iLink 返回的 bot 账号标识。
	BotAccountID string `json:"bot_account_id"`
	// BaseURL 是该登录态后续调用微信 API 的 base URL。
	BaseURL string `json:"base_url"`
	// DisplayName 是 connection descriptor 中展示的绑定目标名称。
	DisplayName string `json:"display_name"`
	// AccountHint 是脱敏后的微信账号提示。
	AccountHint string `json:"account_hint"`
	// CreatedAt 是登录态创建时间，单位毫秒时间戳。
	CreatedAt int64 `json:"created_at"`
	// GetUpdatesBuf 是微信 getupdates 长轮询的本地游标。
	GetUpdatesBuf string `json:"get_updates_buf,omitempty"`
	// ContextTokens 是按微信用户 ID 缓存的 context_token。
	ContextTokens map[string]string `json:"context_tokens,omitempty"`
	// LastInboundAt 是最近一次收到微信入站消息的时间，单位毫秒时间戳。
	LastInboundAt int64 `json:"last_inbound_at,omitempty"`
}

// ConnectionBinding 表示 connector_channel_id 与微信认证账号之间的本地绑定。
//
// 约束说明：该结构属于认证/connection 层，不代表 channel 状态；channel 本身只由 connector_channel_id
// 和 data plane open/close 状态表达。
type ConnectionBinding struct {
	// ConnectorChannelID 是 xAgent 侧持有的用户级 channel 引用。
	ConnectorChannelID string `json:"-"`
	// BotAccountID 是该 connection 绑定的微信 bot 账号标识。
	BotAccountID string `json:"bot_account_id"`
	// CreatedAt 是 connection 绑定创建时间，单位毫秒时间戳。
	CreatedAt int64 `json:"created_at"`
	// LastDeliveredMessageID 是最后成功写入 data plane 的 pending message ID。
	LastDeliveredMessageID string `json:"last_delivered_message_id,omitempty"`
	// LastDeliveredAt 是最后成功写入 data plane 的时间，单位毫秒时间戳。
	LastDeliveredAt int64 `json:"last_delivered_at,omitempty"`
}

// Connection 表示 connector 内部运行时投影，不作为持久化事实直接落盘。
type Connection struct {
	// Token 是 connector_channel_id，沿用运行时字段名以兼容现有调用链。
	Token string `json:"token"`
	// WeChatUserID 是微信 iLink 返回的用户标识。
	WeChatUserID string `json:"wechat_user_id,omitempty"`
	// BotToken 是 connector 本地持有的微信 bot token。
	BotToken string `json:"bot_token"`
	// BotAccountID 是微信 iLink 返回的 bot 账号标识。
	BotAccountID string `json:"bot_account_id"`
	// BaseURL 是该登录态后续调用微信 API 的 base URL。
	BaseURL string `json:"base_url"`
	// DisplayName 是 connection descriptor 中展示的绑定目标名称。
	DisplayName string `json:"display_name"`
	// AccountHint 是脱敏后的微信账号提示。
	AccountHint string `json:"account_hint"`
	// CreatedAt 是 connection 绑定创建时间，单位毫秒时间戳。
	CreatedAt int64 `json:"created_at"`
	// GetUpdatesBuf 是微信 getupdates 长轮询的本地游标。
	GetUpdatesBuf string `json:"get_updates_buf,omitempty"`
	// ContextTokens 是按微信用户 ID 缓存的 context_token。
	ContextTokens map[string]string `json:"context_tokens,omitempty"`
	// LastInboundAt 是最近一次收到微信入站消息的时间，单位毫秒时间戳。
	LastInboundAt int64 `json:"last_inbound_at,omitempty"`
}

// PendingInboundMessage 表示 connector-server 已从微信接管但尚未成功投递到 xAgent 的入站消息。
type PendingInboundMessage struct {
	// ID 是 connector-server 生成的本地稳定消息 ID。
	ID string `json:"id"`
	// ConnectorChannelID 是入站消息所属的 connector channel。
	ConnectorChannelID string `json:"connector_channel_id"`
	// WeChatMessageID 是微信消息 ID。
	WeChatMessageID string `json:"wechat_message_id,omitempty"`
	// WeChatSeq 是微信消息序列号。
	WeChatSeq int64 `json:"wechat_seq,omitempty"`
	// ClientID 是微信协议中的 client_id。
	ClientID string `json:"client_id,omitempty"`
	// Payload 是待投递给 xAgent data plane 的 message.push payload。
	Payload map[string]any `json:"payload"`
	// ReceivedAt 是 connector-server 收到消息并写入本地队列的时间，单位毫秒时间戳。
	ReceivedAt int64 `json:"received_at"`
	// ExpiresAt 是消息缓存过期时间，单位毫秒时间戳。
	ExpiresAt int64 `json:"expires_at"`
	// DeliveredAt 是最近一次成功写入 data plane 的时间，单位毫秒时间戳。
	DeliveredAt int64 `json:"delivered_at,omitempty"`
	// AttemptCount 是尝试投递次数。
	AttemptCount int `json:"attempt_count,omitempty"`
	// LastError 是最近一次投递失败原因。
	LastError string `json:"last_error,omitempty"`
	// CreatedAt 是本地记录创建时间，单位毫秒时间戳。
	CreatedAt int64 `json:"created_at"`
	// UpdatedAt 是本地记录更新时间，单位毫秒时间戳。
	UpdatedAt int64 `json:"updated_at"`
}

// InboundChannelCursor 表示 connector-server 管理的每个 channel 本地投递游标。
type InboundChannelCursor struct {
	// ConnectorChannelID 是游标所属的 connector channel。
	ConnectorChannelID string `json:"connector_channel_id"`
	// LastDeliveredMessageID 是最后成功写入 data plane 的 pending message ID。
	LastDeliveredMessageID string `json:"last_delivered_message_id,omitempty"`
	// LastDeliveredAt 是最后成功写入 data plane 的时间，单位毫秒时间戳。
	LastDeliveredAt int64 `json:"last_delivered_at,omitempty"`
}

// MediaReference 表示 connector-server 管理的短期媒体引用。
type MediaReference struct {
	// Ref 是返回给 xAgent 的 connector 内部媒体 key。
	Ref string `json:"ref"`
	// Direction 表示媒体方向：outbound 表示 xAgent 上传后发给微信，inbound 表示微信传入后由 xAgent 拉取。
	Direction string `json:"direction"`
	// ConnectorChannelID 是媒体所属的 connector channel。
	ConnectorChannelID string `json:"connector_channel_id"`
	// PeerRef 是媒体绑定的微信对端用户引用。
	PeerRef string `json:"peer_ref"`
	// WeChatMessageID 是入站媒体来源微信消息 ID；出站媒体可为空。
	WeChatMessageID string `json:"wechat_message_id,omitempty"`
	// MediaType 是 connector 归一化后的媒体类型：image、video 或 file；微信 voice 消息由 connector 转成文本事件，不进入媒体引用。
	MediaType string `json:"media_type"`
	// ILinkMediaType 是微信 iLink 上传媒体类型枚举值。
	ILinkMediaType int `json:"ilink_media_type"`
	// Filename 是文件展示名，可为空。
	Filename string `json:"filename,omitempty"`
	// ContentType 是媒体 MIME 类型，可为空。
	ContentType string `json:"content_type,omitempty"`
	// RawSize 是明文文件大小，单位字节。
	RawSize int64 `json:"raw_size"`
	// RawMD5 是明文文件 MD5。
	RawMD5 string `json:"raw_md5"`
	// CipherSize 是微信 CDN 密文大小，单位字节。
	CipherSize int64 `json:"cipher_size"`
	// DownloadParam 是微信 CDN 返回的下载加密参数。
	DownloadParam string `json:"download_param"`
	// FullURL 是微信服务端直接返回的完整下载 URL。
	FullURL string `json:"full_url,omitempty"`
	// AESKeyBase64 是微信协议中的 base64 AES key。
	AESKeyBase64 string `json:"aes_key_base64"`
	// LocalPath 是入站媒体下载解密后在 connector 本机缓存的文件路径。
	LocalPath string `json:"local_path,omitempty"`
	// CreatedAt 是媒体引用创建时间，单位毫秒时间戳。
	CreatedAt int64 `json:"created_at"`
	// ExpiresAt 是媒体引用过期时间，单位毫秒时间戳。
	ExpiresAt int64 `json:"expires_at"`
}
