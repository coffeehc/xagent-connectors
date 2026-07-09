package ilinkservice

import "context"

// Service 定义微信 iLink API 调用能力。
type Service interface {
	// Start 完成微信 iLink API 服务启动检查。
	Start(ctx context.Context) error
	// Stop 停止微信 iLink API 服务。
	Stop(ctx context.Context) error
	// APIBaseURL 返回当前微信 iLink API endpoint。
	APIBaseURL() string
	// BotType 返回当前微信 iLink bot_type。
	BotType() string
	// FetchQRCode 获取真实微信二维码载荷。
	FetchQRCode(ctx context.Context, localBotTokens []string) (*QRCodeResponse, error)
	// FetchQRCodeStatus 轮询真实微信二维码状态。
	FetchQRCodeStatus(ctx context.Context, input QRCodeStatusInput) (*QRCodeStatusResponse, error)
	// GetUpdates 长轮询读取微信主动发来的消息。
	GetUpdates(ctx context.Context, input GetUpdatesInput) (*GetUpdatesResponse, error)
	// SendMessage 通过当前登录态发送官方 WeixinMessage 结构。
	SendMessage(ctx context.Context, input SendMessageInput) (*SendMessageResult, error)
	// SendTextMessage 通过当前登录态向指定微信上下文发送文本消息。
	SendTextMessage(ctx context.Context, input SendTextMessageInput) (*SendTextMessageResult, error)
	// GetUploadURL 获取微信 CDN 上传 URL。
	GetUploadURL(ctx context.Context, input GetUploadURLInput) (*GetUploadURLResponse, error)
	// GetConfig 获取指定微信用户会话配置。
	GetConfig(ctx context.Context, input GetConfigInput) (*GetConfigResponse, error)
	// SendTyping 发送微信输入状态。
	SendTyping(ctx context.Context, input SendTypingInput) (*SendTypingResponse, error)
	// NotifyStart 通知微信 iLink 当前 connector 开始拉取消息。
	NotifyStart(ctx context.Context, input NotifyInput) (*NotifyResponse, error)
	// NotifyStop 通知微信 iLink 当前 connector 停止拉取消息。
	NotifyStop(ctx context.Context, input NotifyInput) (*NotifyResponse, error)
}

const (
	// UploadMediaTypeImage 表示图片上传。
	UploadMediaTypeImage = 1
	// UploadMediaTypeVideo 表示视频上传。
	UploadMediaTypeVideo = 2
	// UploadMediaTypeFile 表示文件上传。
	UploadMediaTypeFile = 3
	// UploadMediaTypeVoice 表示语音上传。
	UploadMediaTypeVoice = 4
)

const (
	// MessageTypeNone 表示未指定消息方向。
	MessageTypeNone = 0
	// MessageTypeUser 表示用户发送的消息。
	MessageTypeUser = 1
	// MessageTypeBot 表示 bot 发送的消息。
	MessageTypeBot = 2
)

const (
	// MessageItemTypeNone 表示未指定消息条目类型。
	MessageItemTypeNone = 0
	// MessageItemTypeText 表示文本条目。
	MessageItemTypeText = 1
	// MessageItemTypeImage 表示图片条目。
	MessageItemTypeImage = 2
	// MessageItemTypeVoice 表示语音条目。
	MessageItemTypeVoice = 3
	// MessageItemTypeFile 表示文件条目。
	MessageItemTypeFile = 4
	// MessageItemTypeVideo 表示视频条目。
	MessageItemTypeVideo = 5
	// MessageItemTypeToolCallStart 表示工具调用开始条目。
	MessageItemTypeToolCallStart = 11
	// MessageItemTypeToolCallResult 表示工具调用结果条目。
	MessageItemTypeToolCallResult = 12
)

const (
	// MessageStateNew 表示消息新建状态。
	MessageStateNew = 0
	// MessageStateGenerating 表示消息生成中状态。
	MessageStateGenerating = 1
	// MessageStateFinish 表示消息完成状态。
	MessageStateFinish = 2
)

const (
	// TypingStatusTyping 表示正在输入。
	TypingStatusTyping = 1
	// TypingStatusCancel 表示取消正在输入。
	TypingStatusCancel = 2
)

// BaseInfo 表示微信 iLink 请求公共元信息。
type BaseInfo struct {
	// ChannelVersion 是 connector 声明的通道版本。
	ChannelVersion string `json:"channel_version,omitempty"`
	// BotAgent 是 connector 声明的 bot agent 标识。
	BotAgent string `json:"bot_agent,omitempty"`
}

// QRCodeResponse 表示微信二维码 API 响应。
type QRCodeResponse struct {
	// QRCode 是微信 iLink 返回的二维码会话标识。
	QRCode string `json:"qrcode"`
	// QRCodeImageContent 是微信客户端可识别的二维码内容。
	QRCodeImageContent string `json:"qrcode_img_content"`
}

// QRCodeStatusInput 表示微信二维码状态查询请求。
type QRCodeStatusInput struct {
	// BaseURL 是本次轮询使用的微信 iLink base URL。
	BaseURL string
	// QRCode 是微信 iLink 返回的二维码会话标识。
	QRCode string
	// VerifyCode 是微信要求用户输入数字验证码后携带的校验码。
	VerifyCode string
}

// QRCodeStatusResponse 表示微信二维码状态 API 响应。
type QRCodeStatusResponse struct {
	// Status 是微信二维码登录状态。
	Status string `json:"status"`
	// BotToken 是用户确认后微信返回的 bot token。
	BotToken string `json:"bot_token"`
	// IlinkBotID 是微信返回的 bot 账号标识。
	IlinkBotID string `json:"ilink_bot_id"`
	// BaseURL 是该登录态后续调用微信 API 使用的 base URL。
	BaseURL string `json:"baseurl"`
	// IlinkUserID 是微信返回的用户标识。
	IlinkUserID string `json:"ilink_user_id"`
	// RedirectHost 是微信要求切换轮询节点时返回的 host。
	RedirectHost string `json:"redirect_host"`
}

// GetUpdatesInput 表示微信 getupdates 长轮询请求。
type GetUpdatesInput struct {
	// BaseURL 是当前登录态后续调用微信 API 的 base URL。
	BaseURL string
	// BotToken 是 connector 本地持有的微信 bot token。
	BotToken string
	// GetUpdatesBuf 是本地持久化的微信增量游标。
	GetUpdatesBuf string
	// LongPollTimeoutMillis 是本次长轮询 HTTP 超时，0 表示使用默认值。
	LongPollTimeoutMillis int
}

// GetUpdatesResponse 表示微信 getupdates 长轮询响应。
type GetUpdatesResponse struct {
	// Ret 是微信 iLink 返回的通用结果码。
	Ret int `json:"ret,omitempty"`
	// ErrCode 是微信 iLink 返回的错误码。
	ErrCode int `json:"errcode,omitempty"`
	// ErrMsg 是微信 iLink 返回的错误说明。
	ErrMsg string `json:"errmsg,omitempty"`
	// Messages 是微信主动发来的消息列表。
	Messages []*WeixinMessage `json:"msgs,omitempty"`
	// SyncBuf 是兼容旧协议的增量游标字段。
	SyncBuf string `json:"sync_buf,omitempty"`
	// GetUpdatesBuf 是后续 getupdates 请求需要携带的完整游标。
	GetUpdatesBuf string `json:"get_updates_buf,omitempty"`
	// LongpollingTimeoutMillis 是服务端建议的下一次长轮询超时。
	LongpollingTimeoutMillis int `json:"longpolling_timeout_ms,omitempty"`
}

// TextItem 表示微信文本消息条目。
type TextItem struct {
	// Text 是文本内容。
	Text string `json:"text,omitempty"`
}

// CDNMedia 表示微信 CDN 媒体引用。
type CDNMedia struct {
	// EncryptQueryParam 是 CDN 下载加密参数。
	EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
	// AESKey 是 base64 编码的 AES key。
	AESKey string `json:"aes_key,omitempty"`
	// EncryptType 是微信 CDN 加密类型。
	EncryptType int `json:"encrypt_type,omitempty"`
	// FullURL 是服务端直接返回的完整下载 URL。
	FullURL string `json:"full_url,omitempty"`
}

// ImageItem 表示微信图片消息条目。
type ImageItem struct {
	// Media 是原图 CDN 引用。
	Media *CDNMedia `json:"media,omitempty"`
	// ThumbMedia 是缩略图 CDN 引用。
	ThumbMedia *CDNMedia `json:"thumb_media,omitempty"`
	// AESKey 是 hex 编码的原始 AES key。
	AESKey string `json:"aeskey,omitempty"`
	// URL 是兼容字段中的图片 URL。
	URL string `json:"url,omitempty"`
	// MidSize 是中图密文大小。
	MidSize int64 `json:"mid_size,omitempty"`
	// ThumbSize 是缩略图大小。
	ThumbSize int64 `json:"thumb_size,omitempty"`
	// ThumbHeight 是缩略图高度。
	ThumbHeight int `json:"thumb_height,omitempty"`
	// ThumbWidth 是缩略图宽度。
	ThumbWidth int `json:"thumb_width,omitempty"`
	// HDSize 是高清图大小。
	HDSize int64 `json:"hd_size,omitempty"`
}

// VoiceItem 表示微信语音消息条目。
type VoiceItem struct {
	// Media 是语音 CDN 引用。
	Media *CDNMedia `json:"media,omitempty"`
	// EncodeType 是语音编码类型。
	EncodeType int `json:"encode_type,omitempty"`
	// BitsPerSample 是采样位深。
	BitsPerSample int `json:"bits_per_sample,omitempty"`
	// SampleRate 是采样率。
	SampleRate int `json:"sample_rate,omitempty"`
	// Playtime 是语音长度，单位毫秒。
	Playtime int `json:"playtime,omitempty"`
	// Text 是语音转文字内容。
	Text string `json:"text,omitempty"`
}

// FileItem 表示微信文件消息条目。
type FileItem struct {
	// Media 是文件 CDN 引用。
	Media *CDNMedia `json:"media,omitempty"`
	// FileName 是文件名。
	FileName string `json:"file_name,omitempty"`
	// MD5 是文件 MD5。
	MD5 string `json:"md5,omitempty"`
	// Len 是文件明文长度。
	Len string `json:"len,omitempty"`
}

// VideoItem 表示微信视频消息条目。
type VideoItem struct {
	// Media 是视频 CDN 引用。
	Media *CDNMedia `json:"media,omitempty"`
	// VideoSize 是视频密文大小。
	VideoSize int64 `json:"video_size,omitempty"`
	// PlayLength 是视频播放时长。
	PlayLength int `json:"play_length,omitempty"`
	// VideoMD5 是视频 MD5。
	VideoMD5 string `json:"video_md5,omitempty"`
	// ThumbMedia 是视频缩略图 CDN 引用。
	ThumbMedia *CDNMedia `json:"thumb_media,omitempty"`
	// ThumbSize 是缩略图大小。
	ThumbSize int64 `json:"thumb_size,omitempty"`
	// ThumbHeight 是缩略图高度。
	ThumbHeight int `json:"thumb_height,omitempty"`
	// ThumbWidth 是缩略图宽度。
	ThumbWidth int `json:"thumb_width,omitempty"`
}

// RefMessage 表示微信引用消息摘要。
type RefMessage struct {
	// MessageItem 是被引用的消息条目。
	MessageItem *MessageItem `json:"message_item,omitempty"`
	// Title 是被引用消息摘要。
	Title string `json:"title,omitempty"`
}

// ToolCallStartItem 表示工具调用开始条目。
type ToolCallStartItem struct {
	// ToolName 是工具名。
	ToolName string `json:"tool_name,omitempty"`
	// ToolCallID 是工具调用 ID。
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ToolCallResultItem 表示工具调用结果条目。
type ToolCallResultItem struct {
	// ToolName 是工具名。
	ToolName string `json:"tool_name,omitempty"`
	// ToolCallID 是工具调用 ID。
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Status 是工具调用状态。
	Status string `json:"status,omitempty"`
}

// MessageItem 表示微信消息中的单个内容条目。
type MessageItem struct {
	// Type 是消息条目类型。
	Type int `json:"type,omitempty"`
	// CreateTimeMillis 是条目创建时间，单位毫秒。
	CreateTimeMillis int64 `json:"create_time_ms,omitempty"`
	// UpdateTimeMillis 是条目更新时间，单位毫秒。
	UpdateTimeMillis int64 `json:"update_time_ms,omitempty"`
	// IsCompleted 表示条目是否已完成。
	IsCompleted bool `json:"is_completed,omitempty"`
	// MsgID 是条目消息 ID。
	MsgID string `json:"msg_id,omitempty"`
	// RefMessage 是引用消息摘要。
	RefMessage *RefMessage `json:"ref_msg,omitempty"`
	// TextItem 是文本条目内容。
	TextItem *TextItem `json:"text_item,omitempty"`
	// ImageItem 是图片条目内容。
	ImageItem *ImageItem `json:"image_item,omitempty"`
	// VoiceItem 是语音条目内容。
	VoiceItem *VoiceItem `json:"voice_item,omitempty"`
	// FileItem 是文件条目内容。
	FileItem *FileItem `json:"file_item,omitempty"`
	// VideoItem 是视频条目内容。
	VideoItem *VideoItem `json:"video_item,omitempty"`
	// ToolCallStartItem 是工具调用开始条目内容。
	ToolCallStartItem *ToolCallStartItem `json:"tool_call_start_item,omitempty"`
	// ToolCallResultItem 是工具调用结果条目内容。
	ToolCallResultItem *ToolCallResultItem `json:"tool_call_result_item,omitempty"`
}

// WeixinMessage 表示微信 iLink 统一消息结构。
type WeixinMessage struct {
	// Seq 是微信消息序列号。
	Seq int64 `json:"seq,omitempty"`
	// MessageID 是微信消息 ID。
	MessageID int64 `json:"message_id,omitempty"`
	// FromUserID 是发送方微信用户 ID。
	FromUserID string `json:"from_user_id,omitempty"`
	// DisplayName 是发送方在微信侧的展示名，可为空。
	DisplayName string `json:"display_name,omitempty"`
	// ToUserID 是接收方微信用户 ID。
	ToUserID string `json:"to_user_id,omitempty"`
	// ClientID 是客户端生成的发送消息 ID。
	ClientID string `json:"client_id,omitempty"`
	// CreateTimeMillis 是消息创建时间，单位毫秒。
	CreateTimeMillis int64 `json:"create_time_ms,omitempty"`
	// UpdateTimeMillis 是消息更新时间，单位毫秒。
	UpdateTimeMillis int64 `json:"update_time_ms,omitempty"`
	// DeleteTimeMillis 是消息删除时间，单位毫秒。
	DeleteTimeMillis int64 `json:"delete_time_ms,omitempty"`
	// SessionID 是微信会话 ID。
	SessionID string `json:"session_id,omitempty"`
	// GroupID 是微信群 ID。
	GroupID string `json:"group_id,omitempty"`
	// MessageType 是消息方向类型。
	MessageType int `json:"message_type,omitempty"`
	// MessageState 是消息状态。
	MessageState int `json:"message_state,omitempty"`
	// ItemList 是消息内容条目列表。
	ItemList []*MessageItem `json:"item_list,omitempty"`
	// ContextToken 是后续回复该会话需要携带的上下文 token。
	ContextToken string `json:"context_token,omitempty"`
	// RunID 是上游生成消息时携带的运行 ID。
	RunID string `json:"run_id,omitempty"`
}

// SendTextMessageInput 表示微信文本消息发送请求。
type SendTextMessageInput struct {
	// BaseURL 是当前登录态后续调用微信 API 的 base URL。
	BaseURL string
	// BotToken 是 connector 本地持有的微信 bot token。
	BotToken string
	// ContactID 是目标微信用户 ID，可为空。
	ContactID string
	// ContextToken 是 getupdates 入站消息返回的会话上下文 token。
	ContextToken string
	// Text 是待发送文本。
	Text string
}

// SendMessageInput 表示官方微信消息发送请求。
type SendMessageInput struct {
	// BaseURL 是当前登录态后续调用微信 API 的 base URL。
	BaseURL string
	// BotToken 是 connector 本地持有的微信 bot token。
	BotToken string
	// Message 是要发送的官方 WeixinMessage 结构。
	Message *WeixinMessage
}

// SendTextMessageResult 表示微信文本消息发送结果。
type SendTextMessageResult struct {
	// MessageID 是微信 API 返回的消息标识，可为空。
	MessageID string `json:"message_id,omitempty"`
}

// SendMessageResult 表示官方微信消息发送结果。
type SendMessageResult struct {
	// Ret 是微信 iLink 返回的通用结果码。
	Ret int `json:"ret,omitempty"`
	// ErrMsg 是微信 iLink 返回的错误说明。
	ErrMsg string `json:"errmsg,omitempty"`
	// MessageID 是 connector 侧生成的 client_id。
	MessageID string `json:"message_id,omitempty"`
}

// GetUploadURLInput 表示微信 CDN 上传 URL 请求。
type GetUploadURLInput struct {
	// BaseURL 是当前登录态后续调用微信 API 的 base URL。
	BaseURL string
	// BotToken 是 connector 本地持有的微信 bot token。
	BotToken string
	// FileKey 是本次上传文件 key。
	FileKey string `json:"filekey,omitempty"`
	// MediaType 是上传媒体类型。
	MediaType int `json:"media_type,omitempty"`
	// ToUserID 是目标微信用户 ID。
	ToUserID string `json:"to_user_id,omitempty"`
	// RawSize 是明文文件大小。
	RawSize int64 `json:"rawsize,omitempty"`
	// RawFileMD5 是明文文件 MD5。
	RawFileMD5 string `json:"rawfilemd5,omitempty"`
	// FileSize 是密文文件大小。
	FileSize int64 `json:"filesize,omitempty"`
	// ThumbRawSize 是缩略图明文大小。
	ThumbRawSize int64 `json:"thumb_rawsize,omitempty"`
	// ThumbRawFileMD5 是缩略图明文 MD5。
	ThumbRawFileMD5 string `json:"thumb_rawfilemd5,omitempty"`
	// ThumbFileSize 是缩略图密文大小。
	ThumbFileSize int64 `json:"thumb_filesize,omitempty"`
	// NoNeedThumb 表示不需要缩略图上传 URL。
	NoNeedThumb bool `json:"no_need_thumb,omitempty"`
	// AESKey 是 hex 编码的 AES key。
	AESKey string `json:"aeskey,omitempty"`
}

// GetUploadURLResponse 表示微信 CDN 上传 URL 响应。
type GetUploadURLResponse struct {
	// UploadParam 是原始文件上传加密参数。
	UploadParam string `json:"upload_param,omitempty"`
	// ThumbUploadParam 是缩略图上传加密参数。
	ThumbUploadParam string `json:"thumb_upload_param,omitempty"`
	// UploadFullURL 是服务端直接返回的完整上传 URL。
	UploadFullURL string `json:"upload_full_url,omitempty"`
}

// GetConfigInput 表示微信会话配置请求。
type GetConfigInput struct {
	// BaseURL 是当前登录态后续调用微信 API 的 base URL。
	BaseURL string
	// BotToken 是 connector 本地持有的微信 bot token。
	BotToken string
	// ILinkUserID 是目标微信用户 ID。
	ILinkUserID string
	// ContextToken 是当前会话 context token。
	ContextToken string
}

// GetConfigResponse 表示微信会话配置响应。
type GetConfigResponse struct {
	// Ret 是微信 iLink 返回的通用结果码。
	Ret int `json:"ret,omitempty"`
	// ErrMsg 是微信 iLink 返回的错误说明。
	ErrMsg string `json:"errmsg,omitempty"`
	// TypingTicket 是发送输入状态需要携带的 ticket。
	TypingTicket string `json:"typing_ticket,omitempty"`
}

// SendTypingInput 表示微信输入状态请求。
type SendTypingInput struct {
	// BaseURL 是当前登录态后续调用微信 API 的 base URL。
	BaseURL string
	// BotToken 是 connector 本地持有的微信 bot token。
	BotToken string
	// ILinkUserID 是目标微信用户 ID。
	ILinkUserID string `json:"ilink_user_id,omitempty"`
	// TypingTicket 是 getconfig 返回的输入状态 ticket。
	TypingTicket string `json:"typing_ticket,omitempty"`
	// Status 是输入状态。
	Status int `json:"status,omitempty"`
}

// SendTypingResponse 表示微信输入状态响应。
type SendTypingResponse struct {
	// Ret 是微信 iLink 返回的通用结果码。
	Ret int `json:"ret,omitempty"`
	// ErrMsg 是微信 iLink 返回的错误说明。
	ErrMsg string `json:"errmsg,omitempty"`
}

// NotifyInput 表示微信 monitor 启停通知请求。
type NotifyInput struct {
	// BaseURL 是当前登录态后续调用微信 API 的 base URL。
	BaseURL string
	// BotToken 是 connector 本地持有的微信 bot token。
	BotToken string
}

// NotifyResponse 表示微信 monitor 启停通知响应。
type NotifyResponse struct {
	// Ret 是微信 iLink 返回的通用结果码。
	Ret int `json:"ret,omitempty"`
	// ErrMsg 是微信 iLink 返回的错误说明。
	ErrMsg string `json:"errmsg,omitempty"`
}

// HTTPError 表示微信 API 返回的非 2xx 状态。
type HTTPError struct {
	// StatusCode 是微信 API 返回的 HTTP 状态码。
	StatusCode int
	// Body 是微信 API 返回的错误响应体。
	Body string
}
