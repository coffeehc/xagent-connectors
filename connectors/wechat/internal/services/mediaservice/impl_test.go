package mediaservice

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
)

type testStorageService struct {
	stateDir   string
	references []*connectordomain.MediaReference
}

func (service *testStorageService) Start(context.Context) error {
	return nil
}

func (service *testStorageService) Stop(context.Context) error {
	return nil
}

func (service *testStorageService) StateDir() string {
	if service.stateDir != "" {
		return service.stateDir
	}
	return tTempStateDir
}

func (service *testStorageService) LoadBots() (map[string]*connectordomain.Bot, error) {
	return nil, nil
}

func (service *testStorageService) SaveBots(map[string]*connectordomain.Bot) error {
	return nil
}

func (service *testStorageService) SaveBot(*connectordomain.Bot) error {
	return nil
}

func (service *testStorageService) LoadConnectionBindings() (map[string]*connectordomain.ConnectionBinding, error) {
	return nil, nil
}

func (service *testStorageService) SaveConnectionBindings(map[string]*connectordomain.ConnectionBinding) error {
	return nil
}

func (service *testStorageService) SaveConnectionBinding(*connectordomain.ConnectionBinding) error {
	return nil
}

func (service *testStorageService) DeleteConnectionBinding(string) error {
	return nil
}

func (service *testStorageService) LoadLegacyConnections() (map[string]*connectordomain.Connection, error) {
	return nil, nil
}

func (service *testStorageService) SaveConnection(*connectordomain.Connection) error {
	return nil
}

func (service *testStorageService) SaveConnections(map[string]*connectordomain.Connection) error {
	return nil
}

func (service *testStorageService) EnqueuePendingInboundMessage(*connectordomain.PendingInboundMessage, int) error {
	return nil
}

func (service *testStorageService) ListPendingInboundMessages(string, int64, int) ([]*connectordomain.PendingInboundMessage, error) {
	return nil, nil
}

func (service *testStorageService) MarkPendingInboundDelivered(string, string, int64) error {
	return nil
}

func (service *testStorageService) MarkPendingInboundFailed(string, string, int64, string) error {
	return nil
}

func (service *testStorageService) PruneExpiredPendingInboundMessages(int64) (int, error) {
	return 0, nil
}

func (service *testStorageService) LoadInboundChannelCursor(string) (*connectordomain.InboundChannelCursor, error) {
	return nil, nil
}

func (service *testStorageService) SaveMediaReference(reference *connectordomain.MediaReference) error {
	for index, item := range service.references {
		if item != nil && item.Ref == reference.Ref {
			service.references[index] = reference
			return nil
		}
	}
	service.references = append(service.references, reference)
	return nil
}

func (service *testStorageService) GetMediaReference(ref string, nowMillis int64) (*connectordomain.MediaReference, error) {
	for _, item := range service.references {
		if item == nil || item.Ref != ref {
			continue
		}
		if item.ExpiresAt > 0 && nowMillis >= item.ExpiresAt {
			return nil, nil
		}
		return item, nil
	}
	return nil, nil
}

func (service *testStorageService) PruneExpiredMediaReferences(nowMillis int64) (int, error) {
	next := service.references[:0]
	for _, item := range service.references {
		if item == nil || item.ExpiresAt > 0 && nowMillis >= item.ExpiresAt {
			continue
		}
		next = append(next, item)
	}
	removed := len(service.references) - len(next)
	service.references = next
	return removed, nil
}

const tTempStateDir = "/tmp/xagent-wechat-media-test"

func TestAESECBEncryptDecryptAndPaddedSize(t *testing.T) {
	service := newService("https://cdn.example/c2c", nil, nil)
	key := []byte("1234567890abcdef")
	plaintext := []byte("hello wechat media")
	ciphertext, err := service.EncryptAESECB(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptAESECB returned error: %v", err)
	}
	if int64(len(ciphertext)) != service.AESECBPaddedSize(int64(len(plaintext))) {
		t.Fatalf("ciphertext size mismatch: got=%d", len(ciphertext))
	}
	decrypted, err := service.DecryptAESECB(ciphertext, key)
	if err != nil {
		t.Fatalf("DecryptAESECB returned error: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypt mismatch: got=%q", decrypted)
	}
}

func TestBuildUploadAndDownloadURL(t *testing.T) {
	service := newService("https://cdn.example/c2c", nil, nil)
	uploadURL, err := service.BuildUploadURL(BuildUploadURLInput{
		UploadParam: "a+b/c",
		FileKey:     "file-key",
	})
	if err != nil {
		t.Fatalf("BuildUploadURL returned error: %v", err)
	}
	if uploadURL != "https://cdn.example/c2c/upload?encrypted_query_param=a%2Bb%2Fc&filekey=file-key" {
		t.Fatalf("upload URL mismatch: %s", uploadURL)
	}
	downloadURL, err := service.BuildDownloadURL("a+b/c")
	if err != nil {
		t.Fatalf("BuildDownloadURL returned error: %v", err)
	}
	if downloadURL != "https://cdn.example/c2c/download?encrypted_query_param=a%2Bb%2Fc" {
		t.Fatalf("download URL mismatch: %s", downloadURL)
	}
}

func TestUploadEncryptedBufferReturnsDownloadParam(t *testing.T) {
	var uploaded []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", request.Method)
		}
		if request.Header.Get("Content-Type") != "application/octet-stream" {
			t.Fatalf("content-type 异常: %s", request.Header.Get("Content-Type"))
		}
		var err error
		uploaded, err = io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		writer.Header().Set("x-encrypted-param", "download-param")
	}))
	defer server.Close()

	service := newService("https://cdn.example/c2c", server.Client(), nil)
	result, err := service.UploadEncryptedBuffer(context.Background(), UploadEncryptedBufferInput{
		Plaintext:     []byte("hello"),
		UploadFullURL: server.URL,
		FileKey:       "file-key",
		AESKey:        []byte("1234567890abcdef"),
	})
	if err != nil {
		t.Fatalf("UploadEncryptedBuffer returned error: %v", err)
	}
	if result.DownloadParam != "download-param" {
		t.Fatalf("download param mismatch: %+v", result)
	}
	if len(uploaded) != 16 {
		t.Fatalf("上传体应为 AES padding 后大小，got=%d", len(uploaded))
	}
}

func TestUploadEncryptedFileReturnsDownloadParam(t *testing.T) {
	var uploaded []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", request.Method)
		}
		var err error
		uploaded, err = io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		writer.Header().Set("x-encrypted-param", "download-param")
	}))
	defer server.Close()

	tempFile, err := os.CreateTemp(t.TempDir(), "upload-*")
	if err != nil {
		t.Fatalf("CreateTemp returned error: %v", err)
	}
	if _, err = tempFile.Write([]byte("hello")); err != nil {
		t.Fatalf("write temp file failed: %v", err)
	}
	if err = tempFile.Close(); err != nil {
		t.Fatalf("close temp file failed: %v", err)
	}
	service := newService("https://cdn.example/c2c", server.Client(), nil)
	result, err := service.UploadEncryptedFile(context.Background(), UploadEncryptedFileInput{
		FilePath:      tempFile.Name(),
		UploadFullURL: server.URL,
		FileKey:       "file-key",
		AESKey:        []byte("1234567890abcdef"),
		PlaintextSize: 5,
	})
	if err != nil {
		t.Fatalf("UploadEncryptedFile returned error: %v", err)
	}
	if result.DownloadParam != "download-param" {
		t.Fatalf("download param mismatch: %+v", result)
	}
	if len(uploaded) != 16 {
		t.Fatalf("上传体应为 AES padding 后大小，got=%d", len(uploaded))
	}
}

func TestRegisterMediaReferencePersistsMapping(t *testing.T) {
	storage := &testStorageService{}
	service := newService("https://cdn.example/c2c", nil, storage)
	reference, err := service.RegisterOutboundMedia(context.Background(), RegisterOutboundMediaInput{
		ConnectorChannelID: "cch_user_a",
		PeerRef:            "wx_user_2",
		MediaType:          "video",
		ILinkMediaType:     2,
		Filename:           "done.mp4",
		ContentType:        "video/mp4",
		RawSize:            11,
		RawMD5:             "raw-md5",
		CipherSize:         16,
		DownloadParam:      "download-param-1",
		AESKeyBase64:       base64.StdEncoding.EncodeToString([]byte("1234567890abcdef")),
	})
	if err != nil {
		t.Fatalf("RegisterOutboundMedia returned error: %v", err)
	}
	if reference.Ref == "" || reference.Direction != "outbound" || reference.PeerRef != "wx_user_2" {
		t.Fatalf("media reference 异常: %+v", reference)
	}
	loaded, err := service.GetMediaReference(context.Background(), reference.Ref, reference.CreatedAt)
	if err != nil {
		t.Fatalf("GetMediaReference returned error: %v", err)
	}
	if loaded == nil || loaded.Ref != reference.Ref || loaded.DownloadParam != "download-param-1" {
		t.Fatalf("media reference 读取异常: %+v", loaded)
	}
}

func TestRegisterInboundMediaDownloadsAndCachesReference(t *testing.T) {
	key := []byte("1234567890abcdef")
	plaintext := []byte("hello inbound media stream")
	cryptoService := newService("https://cdn.example/c2c", nil, nil)
	ciphertext, err := cryptoService.EncryptAESECB(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptAESECB returned error: %v", err)
	}
	hitCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		hitCount++
		if request.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", request.Method)
		}
		_, _ = writer.Write(ciphertext)
	}))
	defer server.Close()

	storage := &testStorageService{stateDir: t.TempDir()}
	service := newService("https://cdn.example/c2c", server.Client(), storage)
	reference, err := service.RegisterInboundMedia(context.Background(), RegisterInboundMediaInput{
		ConnectorChannelID: "cch_user_a",
		PeerRef:            "wx_user_1",
		WeChatMessageID:    "1001",
		MediaType:          "image",
		Filename:           "photo.jpg",
		ContentType:        "image/jpeg",
		RawSize:            int64(len(plaintext)),
		CipherSize:         int64(len(ciphertext)),
		FullURL:            server.URL,
		AESKeyBase64:       base64.StdEncoding.EncodeToString(key),
	})
	if err != nil {
		t.Fatalf("RegisterInboundMedia returned error: %v", err)
	}
	if hitCount != 1 {
		t.Fatalf("入站媒体注册时应立即下载一次，got=%d", hitCount)
	}
	if reference.LocalPath == "" {
		t.Fatalf("入站媒体应写入本地缓存路径: %+v", reference)
	}
	cached, err := os.ReadFile(reference.LocalPath)
	if err != nil {
		t.Fatalf("读取媒体缓存失败: %v", err)
	}
	if !bytes.Equal(cached, plaintext) {
		t.Fatalf("媒体缓存内容异常: got=%q", cached)
	}
	server.Close()
	result, err := service.OpenMediaStream(context.Background(), OpenMediaStreamInput{
		MediaRef:           reference.Ref,
		ConnectorChannelID: "cch_user_a",
	})
	if err != nil {
		t.Fatalf("OpenMediaStream returned error: %v", err)
	}
	defer result.Reader.Close()
	body, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("read media stream failed: %v", err)
	}
	if !bytes.Equal(body, plaintext) {
		t.Fatalf("media stream 解密异常: got=%q", body)
	}
	if result.ContentType != "image/jpeg" || result.Filename != "photo.jpg" || result.Size != int64(len(plaintext)) {
		t.Fatalf("media stream metadata 异常: %+v", result)
	}
	if hitCount != 1 {
		t.Fatalf("OpenMediaStream 不应再次访问 CDN，got=%d", hitCount)
	}
}

func TestOpenMediaStreamDecodesBase64WrappedHexAESKey(t *testing.T) {
	key := []byte("1234567890abcdef")
	plaintext := []byte("hello inbound media stream")
	cryptoService := newService("https://cdn.example/c2c", nil, nil)
	ciphertext, err := cryptoService.EncryptAESECB(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptAESECB returned error: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write(ciphertext)
	}))
	defer server.Close()

	storage := &testStorageService{}
	service := newService("https://cdn.example/c2c", server.Client(), storage)
	reference, err := service.RegisterInboundMedia(context.Background(), RegisterInboundMediaInput{
		ConnectorChannelID: "cch_user_a",
		PeerRef:            "wx_user_1",
		WeChatMessageID:    "1001",
		MediaType:          "image",
		Filename:           "photo.jpg",
		ContentType:        "image/jpeg",
		RawSize:            int64(len(plaintext)),
		CipherSize:         int64(len(ciphertext)),
		FullURL:            server.URL,
		AESKeyBase64:       base64.StdEncoding.EncodeToString([]byte("31323334353637383930616263646566")),
	})
	if err != nil {
		t.Fatalf("RegisterInboundMedia returned error: %v", err)
	}
	result, err := service.OpenMediaStream(context.Background(), OpenMediaStreamInput{
		MediaRef:           reference.Ref,
		ConnectorChannelID: "cch_user_a",
	})
	if err != nil {
		t.Fatalf("OpenMediaStream returned error: %v", err)
	}
	defer result.Reader.Close()
	body, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("read media stream failed: %v", err)
	}
	if !bytes.Equal(body, plaintext) {
		t.Fatalf("media stream 解密异常: got=%q", body)
	}
}

func TestRegisterInboundMediaReturnsDecryptErrorBeforeSavingReference(t *testing.T) {
	key := []byte("1234567890abcdef")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("invalid-ciphertext"))
	}))
	defer server.Close()

	storage := &testStorageService{}
	service := newService("https://cdn.example/c2c", server.Client(), storage)
	reference, err := service.RegisterInboundMedia(context.Background(), RegisterInboundMediaInput{
		ConnectorChannelID: "cch_user_a",
		PeerRef:            "wx_user_1",
		WeChatMessageID:    "1001",
		MediaType:          "image",
		Filename:           "photo.jpg",
		ContentType:        "image/jpeg",
		FullURL:            server.URL,
		AESKeyBase64:       base64.StdEncoding.EncodeToString(key),
	})
	if err == nil {
		t.Fatalf("RegisterInboundMedia 应在保存 media_ref 前暴露解密失败")
	}
	if reference != nil || len(storage.references) != 0 {
		t.Fatalf("解密失败不应保存 media_ref，reference=%+v saved=%+v", reference, storage.references)
	}
}

func TestOpenMediaStreamPassesThroughPlainReference(t *testing.T) {
	plaintext := []byte("plain media stream")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write(plaintext)
	}))
	defer server.Close()

	storage := &testStorageService{}
	service := newService("https://cdn.example/c2c", server.Client(), storage)
	reference, err := service.RegisterInboundMedia(context.Background(), RegisterInboundMediaInput{
		ConnectorChannelID: "cch_user_a",
		PeerRef:            "wx_user_1",
		WeChatMessageID:    "1001",
		MediaType:          "file",
		Filename:           "plain.bin",
		RawSize:            int64(len(plaintext)),
		FullURL:            server.URL,
	})
	if err != nil {
		t.Fatalf("RegisterInboundMedia returned error: %v", err)
	}
	result, err := service.OpenMediaStream(context.Background(), OpenMediaStreamInput{
		MediaRef:           reference.Ref,
		ConnectorChannelID: "cch_user_a",
	})
	if err != nil {
		t.Fatalf("OpenMediaStream returned error: %v", err)
	}
	defer result.Reader.Close()
	body, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("read media stream failed: %v", err)
	}
	if !bytes.Equal(body, plaintext) {
		t.Fatalf("media stream passthrough 异常: got=%q", body)
	}
}
