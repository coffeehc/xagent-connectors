package mediaservice

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coffeehc/base/log"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/storageservice"
	"go.uber.org/zap"
)

const (
	cdnRequestTTL   = 30 * time.Second
	cdnMaxBodyBytes = 100 * 1024 * 1024
	cdnMaxRetries   = 3
	cdnUploadBuffer = 1024 * 1024
)

type serviceImpl struct {
	cdnBaseURL string
	httpClient *http.Client
	storage    storageservice.Service
}

func newService(cdnBaseURL string, httpClient *http.Client, storage storageservice.Service) Service {
	if strings.TrimSpace(cdnBaseURL) == "" {
		cdnBaseURL = protocol.WeChatCDNBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &serviceImpl{
		cdnBaseURL: strings.TrimRight(cdnBaseURL, "/"),
		httpClient: httpClient,
		storage:    storage,
	}
}

// Start 完成媒体服务启动检查。
func (impl *serviceImpl) Start(context.Context) error {
	return nil
}

// Stop 停止媒体服务。
func (impl *serviceImpl) Stop(context.Context) error {
	return nil
}

// CDNBaseURL 返回当前微信 CDN base URL。
func (impl *serviceImpl) CDNBaseURL() string {
	return impl.cdnBaseURL
}

// BuildUploadURL 根据 upload_param 与 filekey 构建微信 CDN 上传 URL。
func (impl *serviceImpl) BuildUploadURL(input BuildUploadURLInput) (string, error) {
	if uploadFullURL := strings.TrimSpace(input.UploadFullURL); uploadFullURL != "" {
		return uploadFullURL, nil
	}
	uploadParam := strings.TrimSpace(input.UploadParam)
	fileKey := strings.TrimSpace(input.FileKey)
	if uploadParam == "" || fileKey == "" {
		return "", fmt.Errorf("upload_param 和 filekey 不能为空")
	}
	cdnURL, err := url.Parse(impl.cdnBaseURL + "/upload")
	if err != nil {
		return "", err
	}
	values := cdnURL.Query()
	values.Set("encrypted_query_param", uploadParam)
	values.Set("filekey", fileKey)
	cdnURL.RawQuery = values.Encode()
	return cdnURL.String(), nil
}

// BuildDownloadURL 根据 encrypt_query_param 构建微信 CDN 下载 URL。
func (impl *serviceImpl) BuildDownloadURL(encryptQueryParam string) (string, error) {
	encryptQueryParam = strings.TrimSpace(encryptQueryParam)
	if encryptQueryParam == "" {
		return "", fmt.Errorf("encrypt_query_param 不能为空")
	}
	cdnURL, err := url.Parse(impl.cdnBaseURL + "/download")
	if err != nil {
		return "", err
	}
	values := cdnURL.Query()
	values.Set("encrypted_query_param", encryptQueryParam)
	cdnURL.RawQuery = values.Encode()
	return cdnURL.String(), nil
}

// EncryptAESECB 使用 AES-128-ECB + PKCS7 padding 加密明文。
func (impl *serviceImpl) EncryptAESECB(plaintext []byte, key []byte) ([]byte, error) {
	if len(key) != aes.BlockSize {
		return nil, fmt.Errorf("AES-128 key 长度必须为 16")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	for offset := 0; offset < len(padded); offset += aes.BlockSize {
		block.Encrypt(ciphertext[offset:offset+aes.BlockSize], padded[offset:offset+aes.BlockSize])
	}
	return ciphertext, nil
}

// DecryptAESECB 使用 AES-128-ECB + PKCS7 padding 解密密文。
func (impl *serviceImpl) DecryptAESECB(ciphertext []byte, key []byte) ([]byte, error) {
	if len(key) != aes.BlockSize {
		return nil, fmt.Errorf("AES-128 key 长度必须为 16")
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("AES-ECB 密文长度非法")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	for offset := 0; offset < len(ciphertext); offset += aes.BlockSize {
		block.Decrypt(plaintext[offset:offset+aes.BlockSize], ciphertext[offset:offset+aes.BlockSize])
	}
	return pkcs7Unpad(plaintext, aes.BlockSize)
}

// AESECBPaddedSize 返回 AES-128-ECB + PKCS7 padding 后的密文大小。
func (impl *serviceImpl) AESECBPaddedSize(plaintextSize int64) int64 {
	if plaintextSize < 0 {
		return 0
	}
	return ((plaintextSize / aes.BlockSize) + 1) * aes.BlockSize
}

// UploadEncryptedBuffer 上传明文 buffer 到微信 CDN，返回下载加密参数。
func (impl *serviceImpl) UploadEncryptedBuffer(ctx context.Context, input UploadEncryptedBufferInput) (*UploadEncryptedBufferResult, error) {
	uploadURL, err := impl.BuildUploadURL(BuildUploadURLInput{
		UploadFullURL: input.UploadFullURL,
		UploadParam:   input.UploadParam,
		FileKey:       input.FileKey,
	})
	if err != nil {
		return nil, err
	}
	ciphertext, err := impl.EncryptAESECB(input.Plaintext, input.AESKey)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 1; attempt <= cdnMaxRetries; attempt++ {
		downloadParam, err := impl.uploadCiphertext(ctx, uploadURL, ciphertext)
		if err == nil {
			return &UploadEncryptedBufferResult{DownloadParam: downloadParam}, nil
		}
		lastErr = err
		if isCDNClientError(err) {
			break
		}
		log.Debug("上传微信 CDN 失败，准备重试",
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", cdnMaxRetries),
			zap.String("result", "failed"),
			zap.Error(err),
		)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("上传微信 CDN 失败")
	}
	return nil, lastErr
}

// UploadEncryptedFile 从本地文件流式加密并上传到微信 CDN，返回下载加密参数。
func (impl *serviceImpl) UploadEncryptedFile(ctx context.Context, input UploadEncryptedFileInput) (*UploadEncryptedBufferResult, error) {
	uploadURL, err := impl.BuildUploadURL(BuildUploadURLInput{
		UploadFullURL: input.UploadFullURL,
		UploadParam:   input.UploadParam,
		FileKey:       input.FileKey,
	})
	if err != nil {
		return nil, err
	}
	if len(input.AESKey) != aes.BlockSize {
		return nil, fmt.Errorf("AES-128 key 长度必须为 16")
	}
	var lastErr error
	for attempt := 1; attempt <= cdnMaxRetries; attempt++ {
		downloadParam, err := impl.uploadEncryptedFile(ctx, uploadURL, strings.TrimSpace(input.FilePath), input.AESKey, impl.AESECBPaddedSize(input.PlaintextSize))
		if err == nil {
			return &UploadEncryptedBufferResult{DownloadParam: downloadParam}, nil
		}
		lastErr = err
		if isCDNClientError(err) {
			break
		}
		log.Debug("上传微信 CDN 文件失败，准备重试",
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", cdnMaxRetries),
			zap.String("result", "failed"),
			zap.Error(err),
		)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("上传微信 CDN 文件失败")
	}
	return nil, lastErr
}

// DownloadAndDecryptBuffer 下载微信 CDN 媒体并按需解密。
func (impl *serviceImpl) DownloadAndDecryptBuffer(ctx context.Context, input DownloadAndDecryptBufferInput) ([]byte, error) {
	downloadURL := strings.TrimSpace(input.FullURL)
	if downloadURL == "" {
		var err error
		downloadURL, err = impl.BuildDownloadURL(input.EncryptQueryParam)
		if err != nil {
			return nil, err
		}
	}
	requestCtx, cancel := context.WithTimeout(ctx, cdnRequestTTL)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := impl.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, cdnMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > cdnMaxBodyBytes {
		return nil, fmt.Errorf("微信 CDN 响应超过大小限制")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("微信 CDN 下载失败: HTTP %d %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	if len(input.AESKey) == 0 {
		return payload, nil
	}
	return impl.DecryptAESECB(payload, input.AESKey)
}

func (impl *serviceImpl) downloadReferenceBuffer(ctx context.Context, reference *connectordomain.MediaReference) ([]byte, error) {
	var aesKey []byte
	if strings.TrimSpace(reference.AESKeyBase64) != "" {
		var err error
		aesKey, err = DecodeAESKey(reference.AESKeyBase64)
		if err != nil {
			return nil, err
		}
	}
	return impl.DownloadAndDecryptBuffer(ctx, DownloadAndDecryptBufferInput{
		EncryptQueryParam: reference.DownloadParam,
		FullURL:           reference.FullURL,
		AESKey:            aesKey,
	})
}

func (impl *serviceImpl) cacheInboundReferencePayload(reference *connectordomain.MediaReference, payload []byte) error {
	if reference == nil || strings.TrimSpace(reference.Ref) == "" {
		return fmt.Errorf("media reference incomplete")
	}
	cacheDir := filepath.Join(impl.storage.StateDir(), protocol.MediaCacheDirname)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return fmt.Errorf("创建微信媒体缓存目录失败: %w", err)
	}
	filePath := filepath.Join(cacheDir, storageservice.SanitizeIDPart(reference.Ref))
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o600); err != nil {
		return fmt.Errorf("写入微信媒体缓存失败: %w", err)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("保存微信媒体缓存失败: %w", err)
	}
	_ = os.Chmod(filePath, 0o600)
	reference.LocalPath = filePath
	reference.RawSize = int64(len(payload))
	return nil
}

// RegisterOutboundMedia 保存 xAgent 上传到微信 CDN 后得到的短期媒体 key。
func (impl *serviceImpl) RegisterOutboundMedia(_ context.Context, input RegisterOutboundMediaInput) (*connectordomain.MediaReference, error) {
	now := time.Now().UnixMilli()
	reference := &connectordomain.MediaReference{
		Ref:                "media_" + randomMediaToken(16),
		Direction:          "outbound",
		ConnectorChannelID: strings.TrimSpace(input.ConnectorChannelID),
		PeerRef:            strings.TrimSpace(input.PeerRef),
		MediaType:          strings.TrimSpace(input.MediaType),
		ILinkMediaType:     input.ILinkMediaType,
		Filename:           strings.TrimSpace(input.Filename),
		ContentType:        strings.TrimSpace(input.ContentType),
		RawSize:            input.RawSize,
		RawMD5:             strings.TrimSpace(input.RawMD5),
		CipherSize:         input.CipherSize,
		DownloadParam:      strings.TrimSpace(input.DownloadParam),
		AESKeyBase64:       strings.TrimSpace(input.AESKeyBase64),
		CreatedAt:          now,
		ExpiresAt:          now + protocol.DefaultMediaReferenceTTLMillis,
	}
	if err := validateMediaReference(reference); err != nil {
		return nil, err
	}
	if err := impl.storage.SaveMediaReference(reference); err != nil {
		return nil, err
	}
	return reference, nil
}

// RegisterInboundMedia 保存微信入站消息携带的 CDN 媒体材料并返回短期媒体 key。
func (impl *serviceImpl) RegisterInboundMedia(ctx context.Context, input RegisterInboundMediaInput) (*connectordomain.MediaReference, error) {
	now := time.Now().UnixMilli()
	reference := &connectordomain.MediaReference{
		Ref:                "media_" + randomMediaToken(16),
		Direction:          "inbound",
		ConnectorChannelID: strings.TrimSpace(input.ConnectorChannelID),
		PeerRef:            strings.TrimSpace(input.PeerRef),
		WeChatMessageID:    strings.TrimSpace(input.WeChatMessageID),
		MediaType:          strings.TrimSpace(input.MediaType),
		Filename:           strings.TrimSpace(input.Filename),
		ContentType:        strings.TrimSpace(input.ContentType),
		RawSize:            input.RawSize,
		RawMD5:             strings.TrimSpace(input.RawMD5),
		CipherSize:         input.CipherSize,
		DownloadParam:      strings.TrimSpace(input.DownloadParam),
		FullURL:            strings.TrimSpace(input.FullURL),
		AESKeyBase64:       strings.TrimSpace(input.AESKeyBase64),
		CreatedAt:          now,
		ExpiresAt:          now + protocol.DefaultMediaReferenceTTLMillis,
	}
	if err := validateMediaReference(reference); err != nil {
		return nil, err
	}
	payload, err := impl.downloadReferenceBuffer(ctx, reference)
	if err != nil {
		return nil, err
	}
	if err := impl.cacheInboundReferencePayload(reference, payload); err != nil {
		return nil, err
	}
	if err := impl.storage.SaveMediaReference(reference); err != nil {
		return nil, err
	}
	return reference, nil
}

// GetMediaReference 按 key 读取未过期媒体映射。
func (impl *serviceImpl) GetMediaReference(_ context.Context, mediaRef string, nowMillis int64) (*connectordomain.MediaReference, error) {
	return impl.storage.GetMediaReference(mediaRef, nowMillis)
}

// PruneExpiredMediaReferences 删除已过期媒体映射。
func (impl *serviceImpl) PruneExpiredMediaReferences(_ context.Context, nowMillis int64) (int, error) {
	return impl.storage.PruneExpiredMediaReferences(nowMillis)
}

// OpenMediaStream 根据媒体 key 打开微信 CDN 解密流。
func (impl *serviceImpl) OpenMediaStream(ctx context.Context, input OpenMediaStreamInput) (*OpenMediaStreamResult, error) {
	reference, err := impl.storage.GetMediaReference(input.MediaRef, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	if reference == nil {
		return nil, fmt.Errorf("media_ref not found or expired")
	}
	if strings.TrimSpace(input.ConnectorChannelID) != "" && reference.ConnectorChannelID != strings.TrimSpace(input.ConnectorChannelID) {
		return nil, fmt.Errorf("media_ref channel mismatch")
	}
	if localPath := strings.TrimSpace(reference.LocalPath); localPath != "" {
		file, err := os.Open(localPath)
		if err != nil {
			return nil, err
		}
		size := reference.RawSize
		if fileInfo, statErr := file.Stat(); statErr == nil {
			size = fileInfo.Size()
		}
		return &OpenMediaStreamResult{
			Reference:   reference,
			Reader:      file,
			ContentType: mediaReferenceContentType(reference),
			Filename:    mediaReferenceFilename(reference),
			Size:        size,
		}, nil
	}
	payload, err := impl.downloadReferenceBuffer(ctx, reference)
	if err != nil {
		return nil, err
	}
	return &OpenMediaStreamResult{
		Reference:   reference,
		Reader:      io.NopCloser(bytes.NewReader(payload)),
		ContentType: mediaReferenceContentType(reference),
		Filename:    mediaReferenceFilename(reference),
		Size:        int64(len(payload)),
	}, nil
}

func validateMediaReference(reference *connectordomain.MediaReference) error {
	if reference == nil || reference.Ref == "" || reference.ConnectorChannelID == "" || reference.MediaType == "" {
		return fmt.Errorf("media reference incomplete")
	}
	if reference.DownloadParam == "" && reference.FullURL == "" {
		return fmt.Errorf("media reference missing download material")
	}
	return nil
}

func mediaReferenceContentType(reference *connectordomain.MediaReference) string {
	if reference == nil {
		return "application/octet-stream"
	}
	if contentType := strings.TrimSpace(reference.ContentType); contentType != "" {
		return contentType
	}
	switch reference.MediaType {
	case "image":
		return "image/jpeg"
	case "video":
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}

func mediaReferenceFilename(reference *connectordomain.MediaReference) string {
	if reference == nil || strings.TrimSpace(reference.Filename) == "" {
		return "media.bin"
	}
	return strings.TrimSpace(reference.Filename)
}

func (impl *serviceImpl) uploadCiphertext(ctx context.Context, uploadURL string, ciphertext []byte) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cdnRequestTTL)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, uploadURL, bytes.NewReader(ciphertext))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	response, err := impl.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	return cdnDownloadParamFromResponse(response)
}

func (impl *serviceImpl) uploadEncryptedFile(ctx context.Context, uploadURL string, filePath string, aesKey []byte, ciphertextSize int64) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	reader, writer := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		errCh <- streamAESECBEncrypt(file, writer, aesKey)
	}()
	requestCtx, cancel := context.WithTimeout(ctx, cdnRequestTTL)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, uploadURL, reader)
	if err != nil {
		_ = reader.Close()
		<-errCh
		return "", err
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	if ciphertextSize > 0 {
		request.ContentLength = ciphertextSize
	}
	response, err := impl.httpClient.Do(request)
	if err != nil {
		_ = reader.Close()
		if streamErr := <-errCh; streamErr != nil && !errors.Is(streamErr, io.ErrClosedPipe) {
			return "", streamErr
		}
		return "", err
	}
	defer response.Body.Close()
	if streamErr := <-errCh; streamErr != nil {
		return "", streamErr
	}
	return cdnDownloadParamFromResponse(response)
}

func cdnDownloadParamFromResponse(response *http.Response) (string, error) {
	if response.StatusCode >= http.StatusBadRequest && response.StatusCode < http.StatusInternalServerError {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return "", &cdnHTTPError{StatusCode: response.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("微信 CDN 上传失败: HTTP %d %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	downloadParam := strings.TrimSpace(response.Header.Get("x-encrypted-param"))
	if downloadParam == "" {
		return "", fmt.Errorf("微信 CDN 上传响应缺少 x-encrypted-param")
	}
	return downloadParam, nil
}

func streamAESECBEncrypt(reader io.Reader, writer *io.PipeWriter, key []byte) error {
	defer writer.Close()
	block, err := aes.NewCipher(key)
	if err != nil {
		return writer.CloseWithError(err)
	}
	buffered := bufio.NewReaderSize(reader, cdnUploadBuffer)
	carry := make([]byte, 0, aes.BlockSize)
	buffer := make([]byte, cdnUploadBuffer)
	for {
		n, readErr := buffered.Read(buffer)
		if n > 0 {
			chunk := append(carry, buffer[:n]...)
			plainBlocks := len(chunk) / aes.BlockSize
			if readErr == io.EOF {
				plainBlocks = 0
			}
			if plainBlocks > 0 {
				encryptSize := plainBlocks * aes.BlockSize
				if _, err = writeEncryptedBlocks(writer, block, chunk[:encryptSize]); err != nil {
					return writer.CloseWithError(err)
				}
				carry = append(carry[:0], chunk[encryptSize:]...)
			} else {
				carry = append(carry[:0], chunk...)
			}
		}
		if readErr == io.EOF {
			padded := pkcs7Pad(carry, aes.BlockSize)
			_, err = writeEncryptedBlocks(writer, block, padded)
			if err != nil {
				return writer.CloseWithError(err)
			}
			return nil
		}
		if readErr != nil {
			return writer.CloseWithError(readErr)
		}
	}
}

func writeEncryptedBlocks(writer io.Writer, block cipher.Block, plaintext []byte) (int, error) {
	ciphertext := make([]byte, len(plaintext))
	for offset := 0; offset < len(plaintext); offset += aes.BlockSize {
		block.Encrypt(ciphertext[offset:offset+aes.BlockSize], plaintext[offset:offset+aes.BlockSize])
	}
	return writer.Write(ciphertext)
}

func pkcs7Pad(payload []byte, blockSize int) []byte {
	padding := blockSize - len(payload)%blockSize
	padded := make([]byte, len(payload)+padding)
	copy(padded, payload)
	for i := len(payload); i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	return padded
}

func pkcs7Unpad(payload []byte, blockSize int) ([]byte, error) {
	if len(payload) == 0 || len(payload)%blockSize != 0 {
		return nil, fmt.Errorf("PKCS7 payload 长度非法")
	}
	padding := int(payload[len(payload)-1])
	if padding == 0 || padding > blockSize || padding > len(payload) {
		return nil, fmt.Errorf("PKCS7 padding 非法")
	}
	for _, value := range payload[len(payload)-padding:] {
		if int(value) != padding {
			return nil, fmt.Errorf("PKCS7 padding 非法")
		}
	}
	return payload[:len(payload)-padding], nil
}

func isCDNClientError(err error) bool {
	var httpErr *cdnHTTPError
	return err != nil && errors.As(err, &httpErr) && httpErr.StatusCode >= http.StatusBadRequest && httpErr.StatusCode < http.StatusInternalServerError
}

type cdnHTTPError struct {
	StatusCode int
	Body       string
}

func (err *cdnHTTPError) Error() string {
	if err.Body == "" {
		return fmt.Sprintf("微信 CDN HTTP %d", err.StatusCode)
	}
	return fmt.Sprintf("微信 CDN HTTP %d: %s", err.StatusCode, err.Body)
}

// DecodeAESKey 解析微信协议中常见的 base64 或 hex AES key。
func DecodeAESKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("AES key 不能为空")
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) == aes.BlockSize {
		return decoded, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) == aes.BlockSize*2 {
		if key, hexErr := decodeHexAESKey(string(decoded)); hexErr == nil {
			return key, nil
		}
	}
	if len(value) == aes.BlockSize*2 {
		return decodeHexAESKey(value)
	}
	return nil, fmt.Errorf("AES key 格式非法")
}

func decodeHexAESKey(value string) ([]byte, error) {
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	if len(decoded) == aes.BlockSize {
		return decoded, nil
	}
	return nil, fmt.Errorf("AES key 格式非法")
}

func randomMediaToken(byteCount int) string {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}
