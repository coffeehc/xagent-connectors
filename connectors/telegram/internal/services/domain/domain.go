package domain

// Bot 表示 connector 内部持久化的 Telegram bot 账号事实。
type Bot struct {
	// BotID 是 Telegram getMe 返回的 bot id，按字符串保存以避免 JSON 精度损失。
	BotID string `json:"bot_id"`
	// BotToken 是用户提交并由 connector 本地持有的 Telegram bot token。
	BotToken string `json:"bot_token"`
	// Username 是 Telegram bot username，可为空。
	Username string `json:"username,omitempty"`
	// DisplayName 是 Telegram bot 展示名，可为空。
	DisplayName string `json:"display_name,omitempty"`
	// CreatedAt 是 bot 首次绑定时间，单位毫秒时间戳。
	CreatedAt int64 `json:"created_at"`
	// UpdateOffset 是该 bot getUpdates 长轮询的下一次 offset。
	UpdateOffset int64 `json:"update_offset,omitempty"`
	// LastInboundAt 是最近一次收到已绑定 chat 入站消息的时间，单位毫秒时间戳。
	LastInboundAt int64 `json:"last_inbound_at,omitempty"`
}

// ConnectionBinding 表示 connector_channel_id 与 Telegram chat 的本地绑定。
type ConnectionBinding struct {
	// ConnectorChannelID 是 xAgent 侧持有的用户级 channel 引用。
	ConnectorChannelID string `json:"-"`
	// BotID 是该 channel 归属的 Telegram bot id。
	BotID string `json:"bot_id"`
	// ChatID 是该 channel 映射的 Telegram chat_id。
	ChatID string `json:"chat_id"`
	// ChatType 是 Telegram chat type，例如 private、group、supergroup 或 channel，可为空。
	ChatType string `json:"chat_type,omitempty"`
	// ChatTitle 是 Telegram chat 展示名，可为空。
	ChatTitle string `json:"chat_title,omitempty"`
	// CreatedAt 是 connection 绑定创建时间，单位毫秒时间戳。
	CreatedAt int64 `json:"created_at"`
}

// Connection 表示 connector 内部运行时投影，不作为持久化事实直接落盘。
type Connection struct {
	// Token 是 connector_channel_id，沿用运行时字段名以兼容现有 data plane 语义。
	Token string `json:"token"`
	// BotID 是该 connection 归属的 Telegram bot id。
	BotID string `json:"bot_id"`
	// BotToken 是 connector 本地持有的 Telegram bot token。
	BotToken string `json:"bot_token"`
	// BotUsername 是 Telegram bot username，可为空。
	BotUsername string `json:"bot_username,omitempty"`
	// BotDisplayName 是 Telegram bot 展示名，可为空。
	BotDisplayName string `json:"bot_display_name,omitempty"`
	// ChatID 是该 connection 映射的 Telegram chat_id。
	ChatID string `json:"chat_id"`
	// ChatType 是 Telegram chat type，可为空。
	ChatType string `json:"chat_type,omitempty"`
	// ChatTitle 是 Telegram chat 展示名，可为空。
	ChatTitle string `json:"chat_title,omitempty"`
	// DisplayName 是 connection descriptor 中展示的绑定目标名称。
	DisplayName string `json:"display_name"`
	// AccountHint 是脱敏后的 Telegram 账号提示。
	AccountHint string `json:"account_hint"`
	// CreatedAt 是 connection 绑定创建时间，单位毫秒时间戳。
	CreatedAt int64 `json:"created_at"`
}

// MediaReference 表示 connector-server 管理的 Telegram 媒体引用。
type MediaReference struct {
	// Ref 是返回给 xAgent 的 connector 内部媒体 key。
	Ref string `json:"ref"`
	// Direction 表示媒体方向：outbound 表示 xAgent 上传后发给 Telegram，inbound 表示 Telegram 传入后由 xAgent 拉取。
	Direction string `json:"direction"`
	// ConnectorChannelID 是媒体所属的 connector channel。
	ConnectorChannelID string `json:"connector_channel_id"`
	// BotID 是该媒体所属的 Telegram bot id。
	BotID string `json:"bot_id"`
	// BotToken 是读取 Telegram 入站文件时需要的 bot token；仅保存在 connector 本地。
	BotToken string `json:"bot_token,omitempty"`
	// ChatID 是该媒体所属的 Telegram chat_id。
	ChatID string `json:"chat_id"`
	// FileID 是 Telegram 可复用或可下载的 file_id；出站本地上传媒体可为空。
	FileID string `json:"file_id,omitempty"`
	// MediaType 是 connector 归一化后的媒体类型：image、video 或 file。
	MediaType string `json:"media_type"`
	// Filename 是文件展示名，可为空。
	Filename string `json:"filename,omitempty"`
	// ContentType 是媒体 MIME 类型，可为空。
	ContentType string `json:"content_type,omitempty"`
	// ByteSize 是文件大小，单位字节；未知时可为 0。
	ByteSize int64 `json:"byte_size,omitempty"`
	// LocalPath 是出站上传文件在 connector 本机缓存的路径。
	LocalPath string `json:"local_path,omitempty"`
	// CreatedAt 是媒体引用创建时间，单位毫秒时间戳。
	CreatedAt int64 `json:"created_at"`
	// ExpiresAt 是媒体引用过期时间，单位毫秒时间戳。
	ExpiresAt int64 `json:"expires_at"`
}
