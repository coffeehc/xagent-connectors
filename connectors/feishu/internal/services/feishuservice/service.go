package feishuservice

import (
	"context"
	"io"
)

// QRCodeInfo 表示飞书一键创建应用返回的二维码材料。
type QRCodeInfo struct {
	// URL 是飞书扫码确认页面地址。
	URL string
	// ExpiresInSeconds 是二维码有效秒数。
	ExpiresInSeconds int
}

// AppCredential 表示扫码创建完成后的飞书应用凭据。
type AppCredential struct {
	// AppID 是创建完成的飞书应用 ID。
	AppID string
	// AppSecret 是创建完成的飞书应用密钥。
	AppSecret string
	// UserOpenID 是扫码用户的 open_id，可为空。
	UserOpenID string
}

// RegistrationStatus 表示飞书应用注册轮询的远端状态变化。
type RegistrationStatus struct {
	// Status 是飞书 SDK 返回的状态，例如 polling 或 slow_down。
	Status string
	// IntervalSeconds 是 slow_down 后建议的轮询间隔秒数，可为 0。
	IntervalSeconds int
}

// RegistrationErrorKind 表示飞书应用注册失败的归一化类别。
type RegistrationErrorKind string

const (
	// RegistrationErrorAccessDenied 表示飞书拒绝创建或用户拒绝授权。
	RegistrationErrorAccessDenied RegistrationErrorKind = "access_denied"
	// RegistrationErrorExpired 表示二维码或 registration device code 已过期。
	RegistrationErrorExpired RegistrationErrorKind = "expired"
	// RegistrationErrorRemote 表示飞书返回其他明确业务错误。
	RegistrationErrorRemote RegistrationErrorKind = "remote"
	// RegistrationErrorTransport 表示请求、网络或响应解析失败。
	RegistrationErrorTransport RegistrationErrorKind = "transport"
)

// RegistrationError 表示 connector 可安全记录和展示的飞书注册错误。
type RegistrationError struct {
	// Kind 是归一化错误类别。
	Kind RegistrationErrorKind
	// Code 是飞书返回的错误码；非业务错误时为 registration_failed。
	Code string
	// Description 是飞书返回或 connector 归一化后的错误说明。
	Description string
}

// Error 返回包含飞书错误码和说明的错误文本。
func (err *RegistrationError) Error() string {
	if err == nil {
		return ""
	}
	if err.Description == "" {
		return err.Code
	}
	return err.Code + ": " + err.Description
}

// InboundMessage 表示长连接收到的归一化飞书消息。
type InboundMessage struct {
	// MessageID 是飞书消息 ID。
	MessageID string
	// ChatID 是消息所在会话 ID。
	ChatID string
	// ThreadID 是消息所属话题 ID。
	ThreadID string
	// ChatType 是 p2p、group 或 topic_group。
	ChatType string
	// MessageType 是 text 或 image。
	MessageType string
	// Content 是飞书消息 JSON content。
	Content string
	// SenderOpenID 是发送者 open_id。
	SenderOpenID string
	// SenderType 是发送者类型。
	SenderType string
	// CreateTime 是飞书消息时间戳字符串。
	CreateTime string
}

// MessageHandler 处理长连接收到的飞书消息。
type MessageHandler func(ctx context.Context, message InboundMessage) error

// Stream 表示一个飞书应用长连接。
type Stream interface {
	// Start 启动并阻塞运行长连接。
	Start(ctx context.Context) error
	// Close 关闭长连接。
	Close()
}

// Service 定义飞书开放平台 SDK 适配能力。
type Service interface {
	// Start 完成 SDK service 启动检查。
	Start(ctx context.Context) error
	// Stop 停止 SDK service。
	Stop(ctx context.Context) error
	// RegisterApp 通过官方一键创建应用流程创建飞书应用。
	RegisterApp(ctx context.Context, onQRCode func(QRCodeInfo), onStatusChange func(RegistrationStatus)) (*AppCredential, error)
	// NewStream 创建指定应用的消息长连接。
	NewStream(appID string, appSecret string, handler MessageHandler) Stream
	// UploadImage 上传消息图片并返回 image_key。
	UploadImage(ctx context.Context, appID string, appSecret string, source io.Reader) (string, error)
	// DownloadMessageImage 下载指定消息中的图片。
	DownloadMessageImage(ctx context.Context, appID string, appSecret string, messageID string, imageKey string) ([]byte, string, error)
	// SendText 向指定飞书会话发送文本。
	SendText(ctx context.Context, appID string, appSecret string, chatID string, text string) (string, error)
	// SendImage 向指定飞书会话发送图片。
	SendImage(ctx context.Context, appID string, appSecret string, chatID string, imageKey string) (string, error)
	// ReplyText 回复指定飞书消息。
	ReplyText(ctx context.Context, appID string, appSecret string, messageID string, text string) (string, error)
	// ReplyImage 回复指定飞书消息图片。
	ReplyImage(ctx context.Context, appID string, appSecret string, messageID string, imageKey string) (string, error)
}
