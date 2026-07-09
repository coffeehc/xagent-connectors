package mediaservice

import (
	"context"
	"io"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
)

// Service 定义微信 CDN 媒体加解密和上传下载能力。
type Service interface {
	// Start 完成媒体服务启动检查。
	Start(ctx context.Context) error
	// Stop 停止媒体服务。
	Stop(ctx context.Context) error
	// CDNBaseURL 返回当前微信 CDN base URL。
	CDNBaseURL() string
	// BuildUploadURL 根据 upload_param 与 filekey 构建微信 CDN 上传 URL。
	BuildUploadURL(input BuildUploadURLInput) (string, error)
	// BuildDownloadURL 根据 encrypt_query_param 构建微信 CDN 下载 URL。
	BuildDownloadURL(encryptQueryParam string) (string, error)
	// EncryptAESECB 使用 AES-128-ECB + PKCS7 padding 加密明文。
	EncryptAESECB(plaintext []byte, key []byte) ([]byte, error)
	// DecryptAESECB 使用 AES-128-ECB + PKCS7 padding 解密密文。
	DecryptAESECB(ciphertext []byte, key []byte) ([]byte, error)
	// AESECBPaddedSize 返回 AES-128-ECB + PKCS7 padding 后的密文大小。
	AESECBPaddedSize(plaintextSize int64) int64
	// UploadEncryptedBuffer 上传明文 buffer 到微信 CDN，返回下载加密参数。
	UploadEncryptedBuffer(ctx context.Context, input UploadEncryptedBufferInput) (*UploadEncryptedBufferResult, error)
	// UploadEncryptedFile 从本地文件流式加密并上传到微信 CDN，返回下载加密参数。
	UploadEncryptedFile(ctx context.Context, input UploadEncryptedFileInput) (*UploadEncryptedBufferResult, error)
	// DownloadAndDecryptBuffer 下载微信 CDN 媒体并按需解密。
	DownloadAndDecryptBuffer(ctx context.Context, input DownloadAndDecryptBufferInput) ([]byte, error)
	// RegisterOutboundMedia 保存 xAgent 上传到微信 CDN 后得到的短期媒体 key。
	RegisterOutboundMedia(ctx context.Context, input RegisterOutboundMediaInput) (*connectordomain.MediaReference, error)
	// RegisterInboundMedia 下载并缓存微信入站媒体，保存短期媒体 key 映射。
	RegisterInboundMedia(ctx context.Context, input RegisterInboundMediaInput) (*connectordomain.MediaReference, error)
	// GetMediaReference 按 key 读取未过期媒体映射。
	GetMediaReference(ctx context.Context, mediaRef string, nowMillis int64) (*connectordomain.MediaReference, error)
	// PruneExpiredMediaReferences 删除已过期媒体映射及本地缓存。
	PruneExpiredMediaReferences(ctx context.Context, nowMillis int64) (int, error)
	// OpenMediaStream 根据媒体 key 打开本地缓存或微信 CDN 解密流。
	OpenMediaStream(ctx context.Context, input OpenMediaStreamInput) (*OpenMediaStreamResult, error)
}

// BuildUploadURLInput 表示微信 CDN 上传 URL 构建参数。
type BuildUploadURLInput struct {
	// UploadFullURL 是微信服务端直接返回的完整上传 URL。
	UploadFullURL string
	// UploadParam 是微信服务端返回的上传加密参数。
	UploadParam string
	// FileKey 是本次上传文件 key。
	FileKey string
}

// UploadEncryptedBufferInput 表示微信 CDN 上传请求。
type UploadEncryptedBufferInput struct {
	// Plaintext 是要上传的明文内容。
	Plaintext []byte
	// UploadFullURL 是微信服务端直接返回的完整上传 URL。
	UploadFullURL string
	// UploadParam 是微信服务端返回的上传加密参数。
	UploadParam string
	// FileKey 是本次上传文件 key。
	FileKey string
	// AESKey 是 AES-128-ECB 原始 key，长度必须为 16。
	AESKey []byte
}

// UploadEncryptedFileInput 表示微信 CDN 本地文件上传请求。
type UploadEncryptedFileInput struct {
	// FilePath 是 connector-server 本机可读取的明文文件绝对路径。
	FilePath string
	// UploadFullURL 是微信服务端直接返回的完整上传 URL。
	UploadFullURL string
	// UploadParam 是微信服务端返回的上传加密参数。
	UploadParam string
	// FileKey 是本次上传文件 key。
	FileKey string
	// AESKey 是 AES-128-ECB 原始 key，长度必须为 16。
	AESKey []byte
	// PlaintextSize 是明文文件大小，单位字节。
	PlaintextSize int64
}

// UploadEncryptedBufferResult 表示微信 CDN 上传结果。
type UploadEncryptedBufferResult struct {
	// DownloadParam 是 CDN 响应头 x-encrypted-param。
	DownloadParam string
}

// DownloadAndDecryptBufferInput 表示微信 CDN 下载解密请求。
type DownloadAndDecryptBufferInput struct {
	// EncryptQueryParam 是微信 CDN 下载加密参数。
	EncryptQueryParam string
	// FullURL 是微信服务端直接返回的完整下载 URL。
	FullURL string
	// AESKey 是 AES-128-ECB 原始 key；为空时返回下载原文。
	AESKey []byte
}

// RegisterOutboundMediaInput 表示出站媒体 key 注册请求。
type RegisterOutboundMediaInput struct {
	// ConnectorChannelID 是媒体所属 connector channel。
	ConnectorChannelID string
	// PeerRef 是微信接收人引用。
	PeerRef string
	// MediaType 是 connector 归一化后的媒体类型：image、video 或 file。
	MediaType string
	// ILinkMediaType 是微信 iLink 上传媒体类型。
	ILinkMediaType int
	// Filename 是文件展示名，可为空。
	Filename string
	// ContentType 是媒体 MIME 类型，可为空。
	ContentType string
	// RawSize 是明文大小，单位字节。
	RawSize int64
	// RawMD5 是明文 MD5。
	RawMD5 string
	// CipherSize 是微信 CDN 密文大小，单位字节。
	CipherSize int64
	// DownloadParam 是微信 CDN 下载加密参数。
	DownloadParam string
	// AESKeyBase64 是微信协议中的 base64 AES key。
	AESKeyBase64 string
}

// RegisterInboundMediaInput 表示入站媒体下载缓存请求。
type RegisterInboundMediaInput struct {
	// ConnectorChannelID 是媒体所属 connector channel。
	ConnectorChannelID string
	// PeerRef 是微信发送方引用。
	PeerRef string
	// WeChatMessageID 是来源微信消息 ID。
	WeChatMessageID string
	// MediaType 是 connector 归一化后的媒体类型：image、voice、video 或 file。
	MediaType string
	// Filename 是文件展示名，可为空。
	Filename string
	// ContentType 是媒体 MIME 类型，可为空。
	ContentType string
	// RawSize 是明文大小，单位字节；未知时可为 0。
	RawSize int64
	// RawMD5 是明文 MD5，可为空。
	RawMD5 string
	// CipherSize 是微信 CDN 密文大小，单位字节；未知时可为 0。
	CipherSize int64
	// DownloadParam 是微信 CDN 下载加密参数。
	DownloadParam string
	// FullURL 是微信服务端直接返回的完整下载 URL。
	FullURL string
	// AESKeyBase64 是微信协议中的 base64 AES key。
	AESKeyBase64 string
}

// OpenMediaStreamInput 表示媒体 key 下载请求。
type OpenMediaStreamInput struct {
	// MediaRef 是 connector 生成的短期媒体 key。
	MediaRef string
	// ConnectorChannelID 是调用方所属 connector channel，用于防止跨 channel 读取。
	ConnectorChannelID string
}

// OpenMediaStreamResult 表示媒体流打开结果。
type OpenMediaStreamResult struct {
	// Reference 是本次下载命中的媒体映射。
	Reference *connectordomain.MediaReference
	// Reader 是已按需解密的媒体内容流，调用方负责关闭。
	Reader io.ReadCloser
	// ContentType 是响应 MIME 类型。
	ContentType string
	// Filename 是响应下载文件名。
	Filename string
	// Size 是明文大小，未知时为 0。
	Size int64
}
