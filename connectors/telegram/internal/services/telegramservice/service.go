package telegramservice

import "context"

// Service 定义 Telegram Bot API 调用能力。
//
// 线程安全性：实现不保存请求级状态，可并发调用。
// 错误语义：Telegram Bot API 返回 ok=false、网络失败或响应无法解析时返回 error。
type Service interface {
	// Start 完成 Telegram Bot API 服务启动检查。
	Start(ctx context.Context) error
	// Stop 停止 Telegram Bot API 服务。
	Stop(ctx context.Context) error
	// APIBaseURL 返回当前 Telegram Bot API endpoint。
	APIBaseURL() string
	// GetMe 使用 bot token 读取当前 bot 基本信息。
	GetMe(ctx context.Context, botToken string) (*User, error)
	// GetChat 验证并读取 bot 可访问的 Telegram chat。
	GetChat(ctx context.Context, input GetChatInput) (*Chat, error)
	// GetUpdates 通过长轮询读取指定 bot 的 update 流。
	GetUpdates(ctx context.Context, input GetUpdatesInput) (*GetUpdatesResult, error)
	// SendMessage 通过指定 bot 向指定 chat 发送文本消息。
	SendMessage(ctx context.Context, input SendMessageInput) (*Message, error)
	// SendMedia 通过指定 bot 向指定 chat 发送图片、视频或文件。
	SendMedia(ctx context.Context, input SendMediaInput) (*Message, error)
	// GetFile 读取 Telegram file_id 对应的下载路径。
	GetFile(ctx context.Context, input GetFileInput) (*File, error)
	// DownloadFile 下载 Telegram file_path 对应的文件内容。
	DownloadFile(ctx context.Context, input DownloadFileInput) (*DownloadFileResult, error)
}

// User 表示 Telegram Bot API User 对象中 connector 需要的字段。
type User struct {
	// ID 是 Telegram 用户或 bot id。
	ID int64 `json:"id"`
	// IsBot 表示该用户是否是 bot。
	IsBot bool `json:"is_bot"`
	// FirstName 是 Telegram 返回的 first_name，可为空。
	FirstName string `json:"first_name,omitempty"`
	// Username 是 Telegram username，可为空。
	Username string `json:"username,omitempty"`
}

// Chat 表示 Telegram Bot API Chat 对象中 connector 需要的字段。
type Chat struct {
	// ID 是 Telegram chat_id。
	ID int64 `json:"id"`
	// Type 是 Telegram chat type，例如 private、group、supergroup 或 channel。
	Type string `json:"type"`
	// Title 是群组或频道标题，可为空。
	Title string `json:"title,omitempty"`
	// Username 是 chat username，可为空。
	Username string `json:"username,omitempty"`
	// FirstName 是私聊对象 first_name，可为空。
	FirstName string `json:"first_name,omitempty"`
	// LastName 是私聊对象 last_name，可为空。
	LastName string `json:"last_name,omitempty"`
}

// Message 表示 Telegram Bot API Message 对象中 connector 需要的字段。
type Message struct {
	// MessageID 是 Telegram message_id。
	MessageID int64 `json:"message_id"`
	// From 是发送方用户，可为空。
	From *User `json:"from,omitempty"`
	// Chat 是消息所属 chat。
	Chat *Chat `json:"chat,omitempty"`
	// Date 是 Telegram 消息时间，单位秒。
	Date int64 `json:"date,omitempty"`
	// Text 是文本消息内容，可为空。
	Text string `json:"text,omitempty"`
	// Caption 是媒体消息说明，可为空。
	Caption string `json:"caption,omitempty"`
	// Photo 是图片消息的尺寸集合，可为空。
	Photo []*PhotoSize `json:"photo,omitempty"`
	// Document 是普通文件消息，可为空。
	Document *Document `json:"document,omitempty"`
	// Video 是视频消息，可为空。
	Video *Video `json:"video,omitempty"`
}

// PhotoSize 表示 Telegram 图片不同尺寸对象。
type PhotoSize struct {
	// FileID 是 Telegram file_id。
	FileID string `json:"file_id"`
	// FileUniqueID 是 Telegram file_unique_id。
	FileUniqueID string `json:"file_unique_id,omitempty"`
	// Width 是图片宽度。
	Width int `json:"width"`
	// Height 是图片高度。
	Height int `json:"height"`
	// FileSize 是文件大小，单位字节；未知时为 0。
	FileSize int64 `json:"file_size,omitempty"`
}

// Document 表示 Telegram 普通文件对象。
type Document struct {
	// FileID 是 Telegram file_id。
	FileID string `json:"file_id"`
	// FileUniqueID 是 Telegram file_unique_id。
	FileUniqueID string `json:"file_unique_id,omitempty"`
	// FileName 是文件名，可为空。
	FileName string `json:"file_name,omitempty"`
	// MimeType 是 MIME 类型，可为空。
	MimeType string `json:"mime_type,omitempty"`
	// FileSize 是文件大小，单位字节；未知时为 0。
	FileSize int64 `json:"file_size,omitempty"`
}

// Video 表示 Telegram 视频文件对象。
type Video struct {
	// FileID 是 Telegram file_id。
	FileID string `json:"file_id"`
	// FileUniqueID 是 Telegram file_unique_id。
	FileUniqueID string `json:"file_unique_id,omitempty"`
	// FileName 是文件名，可为空。
	FileName string `json:"file_name,omitempty"`
	// MimeType 是 MIME 类型，可为空。
	MimeType string `json:"mime_type,omitempty"`
	// FileSize 是文件大小，单位字节；未知时为 0。
	FileSize int64 `json:"file_size,omitempty"`
	// Width 是视频宽度。
	Width int `json:"width,omitempty"`
	// Height 是视频高度。
	Height int `json:"height,omitempty"`
	// Duration 是视频时长，单位秒。
	Duration int `json:"duration,omitempty"`
}

// File 表示 Telegram getFile 返回的文件路径对象。
type File struct {
	// FileID 是 Telegram file_id。
	FileID string `json:"file_id"`
	// FileUniqueID 是 Telegram file_unique_id。
	FileUniqueID string `json:"file_unique_id,omitempty"`
	// FileSize 是文件大小，单位字节；未知时为 0。
	FileSize int64 `json:"file_size,omitempty"`
	// FilePath 是 Telegram 文件下载路径。
	FilePath string `json:"file_path,omitempty"`
}

// Update 表示 Telegram Bot API Update 对象中 connector 需要的字段。
type Update struct {
	// UpdateID 是 Telegram update_id，用于推进 getUpdates offset。
	UpdateID int64 `json:"update_id"`
	// Message 是普通入站消息，可为空。
	Message *Message `json:"message,omitempty"`
}

// GetChatInput 表示 Telegram getChat 请求。
type GetChatInput struct {
	// BotToken 是用户提交并由 connector 本地持有的 Telegram bot token。
	BotToken string
	// ChatID 是待验证的 Telegram chat_id。
	ChatID string
}

// GetUpdatesInput 表示 Telegram getUpdates 长轮询请求。
type GetUpdatesInput struct {
	// BotToken 是用户提交并由 connector 本地持有的 Telegram bot token。
	BotToken string
	// Offset 是本次长轮询使用的 Telegram update offset。
	Offset int64
	// TimeoutSeconds 是 Telegram 长轮询超时秒数。
	TimeoutSeconds int
}

// GetUpdatesResult 表示 Telegram getUpdates 长轮询结果。
type GetUpdatesResult struct {
	// Updates 是 Telegram 返回的 update 列表。
	Updates []*Update
	// NextOffset 是下一次 getUpdates 应使用的 offset。
	NextOffset int64
}

// SendMessageInput 表示 Telegram sendMessage 请求。
type SendMessageInput struct {
	// BotToken 是用户提交并由 connector 本地持有的 Telegram bot token。
	BotToken string
	// ChatID 是目标 Telegram chat_id。
	ChatID string
	// Text 是待发送文本。
	Text string
}

// SendMediaInput 表示 Telegram 媒体发送请求。
type SendMediaInput struct {
	// BotToken 是用户提交并由 connector 本地持有的 Telegram bot token。
	BotToken string
	// ChatID 是目标 Telegram chat_id。
	ChatID string
	// MediaType 是 connector 归一化后的媒体类型：image、video 或 file。
	MediaType string
	// FileID 是 Telegram 已有 file_id，可为空。
	FileID string
	// LocalPath 是 connector 本地待上传文件路径；FileID 为空时必填。
	LocalPath string
	// Filename 是上传文件名，可为空。
	Filename string
	// Caption 是媒体说明，可为空。
	Caption string
}

// GetFileInput 表示 Telegram getFile 请求。
type GetFileInput struct {
	// BotToken 是用户提交并由 connector 本地持有的 Telegram bot token。
	BotToken string
	// FileID 是 Telegram file_id。
	FileID string
}

// DownloadFileInput 表示 Telegram 文件下载请求。
type DownloadFileInput struct {
	// BotToken 是用户提交并由 connector 本地持有的 Telegram bot token。
	BotToken string
	// FilePath 是 getFile 返回的 Telegram 文件路径。
	FilePath string
}

// DownloadFileResult 表示 Telegram 文件下载结果。
type DownloadFileResult struct {
	// ContentType 是 HTTP 下载响应 Content-Type，可为空。
	ContentType string
	// Body 是完整文件内容。
	Body []byte
}
