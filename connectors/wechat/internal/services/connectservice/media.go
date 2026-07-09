package connectservice

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/coffeehc/base/log"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/ilinkservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/mediaservice"
	"go.uber.org/zap"
)

const (
	weChatMediaAESKeySize = 16
)

const (
	connectorMediaTypeImage = "image"
	connectorMediaTypeVideo = "video"
	connectorMediaTypeFile  = "file"
)

// UploadMedia 上传待发送媒体到微信 CDN 并返回 connector 内部 media_ref。
func (impl *serviceImpl) UploadMedia(ctx context.Context, input UploadMediaInput) (*UploadMediaResult, error) {
	connectorChannelID := strings.TrimSpace(input.ConnectorChannelID)
	if connectorChannelID == "" {
		return nil, fmt.Errorf("connector_channel_id required")
	}
	recipientRef := strings.TrimSpace(input.RecipientRef)
	if input.Source == nil {
		return nil, fmt.Errorf("file required")
	}
	connection := impl.ConnectionByChannel(connectorChannelID)
	if connection == nil {
		return nil, fmt.Errorf("channel is not bound to an authenticated target user")
	}
	if recipientRef == "" {
		recipientRef = defaultRecipientRef(connection)
	}
	if recipientRef == "" {
		return nil, fmt.Errorf("recipient_ref unavailable for channel")
	}
	filename := sanitizeUploadFilename(input.Filename)
	contentType := normalizeUploadContentType(input.ContentType, filename)
	mediaType, ilinkMediaType := classifyUploadMedia(contentType, filename)
	tempFile, rawSize, rawMD5, err := writeUploadTempFile(input.Source)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempFile)
	uploaded, err := impl.uploadWeChatMediaFile(ctx, connection, recipientRef, ilinkMediaType, tempFile, rawSize, rawMD5)
	if err != nil {
		log.Debug("上传微信 IM 媒体失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("recipient_ref", recipientRef),
			zap.String("filename", filename),
			zap.String("media_type", mediaType),
			zap.Int64("byte_size", rawSize),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return nil, err
	}
	media, err := impl.media.RegisterOutboundMedia(ctx, mediaservice.RegisterOutboundMediaInput{
		ConnectorChannelID: connectorChannelID,
		PeerRef:            recipientRef,
		MediaType:          mediaType,
		ILinkMediaType:     ilinkMediaType,
		Filename:           filename,
		ContentType:        contentType,
		RawSize:            rawSize,
		RawMD5:             rawMD5,
		CipherSize:         uploaded.cipherSize,
		DownloadParam:      uploaded.downloadParam,
		AESKeyBase64:       uploaded.aesKeyBase64,
	})
	if err != nil {
		return nil, err
	}
	log.Debug("上传微信 IM 媒体完成",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("recipient_ref", recipientRef),
		zap.String("media_ref", media.Ref),
		zap.String("filename", filename),
		zap.String("media_type", mediaType),
		zap.Int64("byte_size", rawSize),
		zap.Int64("expires_at", media.ExpiresAt),
		zap.String("result", "uploaded"),
	)
	return &UploadMediaResult{
		MediaRef:  media.Ref,
		MediaType: media.MediaType,
		Filename:  media.Filename,
		ByteSize:  media.RawSize,
		ExpiresAt: media.ExpiresAt,
	}, nil
}

type weChatMediaUploadResult struct {
	cipherSize    int64
	downloadParam string
	aesKeyBase64  string
}

// uploadWeChatMediaFile 执行官方 iLink 媒体上传链路，返回 sendMessage 需要的 CDN 引用材料。
func (impl *serviceImpl) uploadWeChatMediaFile(ctx context.Context, connection *connectordomain.Connection, recipientRef string, mediaType int, filePath string, rawSize int64, rawMD5 string) (*weChatMediaUploadResult, error) {
	cipherSize := impl.media.AESECBPaddedSize(rawSize)
	fileKeyBytes, err := randomBytes(weChatMediaAESKeySize)
	if err != nil {
		return nil, err
	}
	aesKey, err := randomBytes(weChatMediaAESKeySize)
	if err != nil {
		return nil, err
	}
	fileKey := hex.EncodeToString(fileKeyBytes)
	uploadURL, err := impl.wechat.GetUploadURL(ctx, ilinkservice.GetUploadURLInput{
		BaseURL:     connection.BaseURL,
		BotToken:    connection.BotToken,
		FileKey:     fileKey,
		MediaType:   mediaType,
		ToUserID:    recipientRef,
		RawSize:     rawSize,
		RawFileMD5:  rawMD5,
		FileSize:    cipherSize,
		NoNeedThumb: true,
		AESKey:      hex.EncodeToString(aesKey),
	})
	if err != nil {
		return nil, err
	}
	if uploadURL == nil || strings.TrimSpace(uploadURL.UploadFullURL) == "" && strings.TrimSpace(uploadURL.UploadParam) == "" {
		return nil, fmt.Errorf("getUploadURL returned no upload URL")
	}
	uploadResult, err := impl.media.UploadEncryptedFile(ctx, mediaservice.UploadEncryptedFileInput{
		FilePath:      filePath,
		UploadFullURL: strings.TrimSpace(uploadURL.UploadFullURL),
		UploadParam:   uploadURL.UploadParam,
		FileKey:       fileKey,
		AESKey:        aesKey,
		PlaintextSize: rawSize,
	})
	if err != nil {
		return nil, err
	}
	if uploadResult == nil || strings.TrimSpace(uploadResult.DownloadParam) == "" {
		return nil, fmt.Errorf("CDN upload returned no download param")
	}
	return &weChatMediaUploadResult{
		cipherSize:    cipherSize,
		downloadParam: strings.TrimSpace(uploadResult.DownloadParam),
		aesKeyBase64:  base64.StdEncoding.EncodeToString(aesKey),
	}, nil
}

func writeUploadTempFile(source io.Reader) (string, int64, string, error) {
	tempFile, err := os.CreateTemp("", "xagent-wechat-upload-*")
	if err != nil {
		return "", 0, "", err
	}
	tempPath := tempFile.Name()
	hash := md5.New()
	size, copyErr := io.Copy(io.MultiWriter(tempFile, hash), source)
	closeErr := tempFile.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tempPath)
		if copyErr != nil {
			return "", 0, "", copyErr
		}
		return "", 0, "", closeErr
	}
	if size == 0 {
		_ = os.Remove(tempPath)
		return "", 0, "", fmt.Errorf("file must not be empty")
	}
	return tempPath, size, hex.EncodeToString(hash.Sum(nil)), nil
}

func sanitizeUploadFilename(filename string) string {
	filename = strings.TrimSpace(filepath.Base(filename))
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		return "upload.bin"
	}
	return filename
}

func normalizeUploadContentType(contentType string, filename string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType != "" {
		if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && strings.TrimSpace(mediaType) != "" {
			return strings.ToLower(strings.TrimSpace(mediaType))
		}
		return strings.ToLower(contentType)
	}
	if ext := strings.ToLower(filepath.Ext(filename)); ext != "" {
		if inferred := mime.TypeByExtension(ext); inferred != "" {
			if mediaType, _, err := mime.ParseMediaType(inferred); err == nil && strings.TrimSpace(mediaType) != "" {
				return strings.ToLower(strings.TrimSpace(mediaType))
			}
		}
	}
	return "application/octet-stream"
}

func classifyUploadMedia(contentType string, filename string) (string, int) {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return connectorMediaTypeImage, ilinkservice.UploadMediaTypeImage
	case strings.HasPrefix(contentType, "video/"):
		return connectorMediaTypeVideo, ilinkservice.UploadMediaTypeVideo
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".heic":
		return connectorMediaTypeImage, ilinkservice.UploadMediaTypeImage
	case ".mp4", ".mov", ".m4v", ".webm", ".avi", ".mkv":
		return connectorMediaTypeVideo, ilinkservice.UploadMediaTypeVideo
	default:
		return connectorMediaTypeFile, ilinkservice.UploadMediaTypeFile
	}
}

func randomBytes(size int) ([]byte, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return nil, err
	}
	return buffer, nil
}
