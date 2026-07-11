package endpointservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/channelservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/connectservice"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/mediaservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
	"github.com/gofiber/fiber/v3"
)

type testConnectService struct {
	uploadInput *connectservice.UploadMediaInput
}

func (service *testConnectService) Start(context.Context) error {
	return nil
}

func (service *testConnectService) Stop(context.Context) error {
	return nil
}

func (service *testConnectService) APIKey() string {
	return protocol.DefaultAPIKey
}

func (service *testConnectService) ConnectorID() string {
	return protocol.ConnectorCardID
}

func (service *testConnectService) StateDir() string {
	return "/tmp/xagent-wechat-connector-test"
}

func (service *testConnectService) WeChatAPIBaseURL() string {
	return protocol.WeChatAPIBaseURL
}

func (service *testConnectService) WeChatBotType() string {
	return protocol.WeChatBotType
}

func (service *testConnectService) BindMessagePusher(connectservice.MessagePusher) {}

func (service *testConnectService) FlushInboundMessages(context.Context, string) {}

func (service *testConnectService) StartAuth(context.Context, string, string) (*connectorprotocol.ConnectorAuthStartResult, error) {
	return nil, nil
}

func (service *testConnectService) AuthStatus(context.Context, string, string, bool) (*connectorprotocol.ConnectorAuthStatusResult, bool) {
	return nil, false
}

func (service *testConnectService) CancelAuth(context.Context, string, string) *connectorprotocol.ConnectorAuthCancelResult {
	return nil
}

func (service *testConnectService) ConnectionByChannel(string) *connectordomain.Connection {
	return nil
}

func (service *testConnectService) LogoutByChannel(string) bool {
	return false
}

func (service *testConnectService) InvokeTool(context.Context, connectservice.ToolInvokeInput) (*connectservice.ToolInvokeResult, error) {
	return nil, nil
}

func (service *testConnectService) UploadMedia(_ context.Context, input connectservice.UploadMediaInput) (*connectservice.UploadMediaResult, error) {
	service.uploadInput = &input
	return &connectservice.UploadMediaResult{
		MediaRef:  "media_ref_1",
		MediaType: "video",
		Filename:  input.Filename,
		ByteSize:  input.Size,
		ExpiresAt: 1000,
	}, nil
}

func (service *testConnectService) ReadConnectorSkill(context.Context) (*connectservice.ConnectorSkillContent, error) {
	return &connectservice.ConnectorSkillContent{
		SkillID:     protocol.ConnectorSkillIMReplyID,
		ContentType: "text/markdown; charset=utf-8",
		Content:     "# IM Connector Reply\n",
		SHA256:      "test-sha256",
	}, nil
}

func (service *testConnectService) BuildConnectorCard() *connectorprotocol.ConnectorCard {
	return &connectorprotocol.ConnectorCard{
		Schema: "xagent.connector/v1",
		Connector: connectorprotocol.ConnectorCardInfo{
			Version: "0.0.1",
		},
	}
}

func (service *testConnectService) BuildChannelDescriptor(string) *connectorprotocol.ConnectionDescriptor {
	return &connectorprotocol.ConnectionDescriptor{Schema: "xagent.connection/v1"}
}

func (service *testConnectService) BuildConnectionDescriptor(*connectordomain.Connection) *connectorprotocol.ConnectionDescriptor {
	return nil
}

type testChannelService struct{}

func (service *testChannelService) Start(context.Context) error {
	return nil
}

func (service *testChannelService) Stop(context.Context) error {
	return nil
}

func (service *testChannelService) HandleDataPlane(fiber.Ctx) error {
	return nil
}

func (service *testChannelService) PushMessage(context.Context, channelservice.MessagePushInput) error {
	return nil
}

func (service *testChannelService) PushConnectionDescriptor(context.Context, channelservice.DescriptorPushInput) error {
	return nil
}

type testMediaService struct {
	stream *mediaservice.OpenMediaStreamResult
}

type failingReadCloser struct{}

func (reader failingReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func (reader failingReadCloser) Close() error {
	return nil
}

func (service *testMediaService) Start(context.Context) error {
	return nil
}

func (service *testMediaService) Stop(context.Context) error {
	return nil
}

func (service *testMediaService) CDNBaseURL() string {
	return protocol.WeChatCDNBaseURL
}

func (service *testMediaService) BuildUploadURL(input mediaservice.BuildUploadURLInput) (string, error) {
	return input.UploadFullURL, nil
}

func (service *testMediaService) BuildDownloadURL(string) (string, error) {
	return "", nil
}

func (service *testMediaService) EncryptAESECB(plaintext []byte, _ []byte) ([]byte, error) {
	return plaintext, nil
}

func (service *testMediaService) DecryptAESECB(ciphertext []byte, _ []byte) ([]byte, error) {
	return ciphertext, nil
}

func (service *testMediaService) AESECBPaddedSize(plaintextSize int64) int64 {
	return plaintextSize
}

func (service *testMediaService) UploadEncryptedBuffer(context.Context, mediaservice.UploadEncryptedBufferInput) (*mediaservice.UploadEncryptedBufferResult, error) {
	return nil, nil
}

func (service *testMediaService) UploadEncryptedFile(context.Context, mediaservice.UploadEncryptedFileInput) (*mediaservice.UploadEncryptedBufferResult, error) {
	return nil, nil
}

func (service *testMediaService) DownloadAndDecryptBuffer(context.Context, mediaservice.DownloadAndDecryptBufferInput) ([]byte, error) {
	return nil, nil
}

func (service *testMediaService) RegisterOutboundMedia(context.Context, mediaservice.RegisterOutboundMediaInput) (*connectordomain.MediaReference, error) {
	return nil, nil
}

func (service *testMediaService) RegisterInboundMedia(context.Context, mediaservice.RegisterInboundMediaInput) (*connectordomain.MediaReference, error) {
	return nil, nil
}

func (service *testMediaService) GetMediaReference(context.Context, string, int64) (*connectordomain.MediaReference, error) {
	return nil, nil
}

func (service *testMediaService) PruneExpiredMediaReferences(context.Context, int64) (int, error) {
	return 0, nil
}

func (service *testMediaService) OpenMediaStream(context.Context, mediaservice.OpenMediaStreamInput) (*mediaservice.OpenMediaStreamResult, error) {
	if service.stream == nil {
		return &mediaservice.OpenMediaStreamResult{
			Reader:      io.NopCloser(bytes.NewReader(nil)),
			ContentType: "application/octet-stream",
		}, nil
	}
	return service.stream, nil
}

func TestHealthRequiresAPIKey(t *testing.T) {
	endpointService := newService(&testConnectService{}, &testChannelService{}, &testMediaService{})
	app := fiber.New()
	endpointService.registerRoutes(app)

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/health", http.NoBody))
	if err != nil {
		t.Fatalf("health request returned error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未携带 API key 的 health 应被拒绝，got=%d", response.StatusCode)
	}

	request := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
	request.Header.Set("Authorization", "Bearer "+protocol.DefaultAPIKey)
	response, err = app.Test(request)
	if err != nil {
		t.Fatalf("authorized health request returned error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("携带 API key 的 health 应成功，got=%d", response.StatusCode)
	}
	var payload map[string]string
	if err = json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("health response JSON decode failed: %v", err)
	}
	if payload["connector_card_id"] != protocol.ConnectorCardID || payload["connector_card_version"] != "0.0.1" {
		t.Fatalf("health 应返回 Connector Card 标识和版本，got=%+v", payload)
	}
}

func TestConnectorSkillIsPublicAndReturnsMarkdown(t *testing.T) {
	endpointService := newService(&testConnectService{}, &testChannelService{}, &testMediaService{})
	app := fiber.New()
	endpointService.registerRoutes(app)

	response, err := app.Test(httptest.NewRequest(http.MethodGet, connectorprotocol.ConnectorSkillPath, http.NoBody))
	if err != nil {
		t.Fatalf("connector skill request returned error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("connector skill 应公开读取，got=%d", response.StatusCode)
	}
	if response.Header.Get("Content-Type") != "text/markdown; charset=utf-8" {
		t.Fatalf("connector skill content-type 异常: %s", response.Header.Get("Content-Type"))
	}
	if response.Header.Get("X-XAgent-Skill-ID") != protocol.ConnectorSkillIMReplyID {
		t.Fatalf("connector skill id header 异常: %s", response.Header.Get("X-XAgent-Skill-ID"))
	}
	if response.Header.Get("X-XAgent-Skill-SHA256") != "test-sha256" {
		t.Fatalf("connector skill sha header 异常: %s", response.Header.Get("X-XAgent-Skill-SHA256"))
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read connector skill body failed: %v", err)
	}
	if string(body) != "# IM Connector Reply\n" {
		t.Fatalf("connector skill body 异常: %q", string(body))
	}
}

func TestMediaUploadRequiresAPIKeyAndPassesMultipartFile(t *testing.T) {
	connect := &testConnectService{}
	endpointService := newService(connect, &testChannelService{}, &testMediaService{})
	app := fiber.New()
	endpointService.registerRoutes(app)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("recipient_ref", "wx_user_2"); err != nil {
		t.Fatalf("write field failed: %v", err)
	}
	part, err := writer.CreateFormFile("file", "done.mp4")
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err = part.Write([]byte("video-bytes")); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("close multipart failed: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, connectorprotocol.ConnectorMediaUploadPath, body)
	request.Header.Set("Authorization", "Bearer "+protocol.DefaultAPIKey)
	request.Header.Set("X-XAgent-Connector-Channel-ID", "cch_user_a")
	request.Header.Set("Content-Type", writer.FormDataContentType())

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("media upload request returned error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("media upload 应成功，got=%d", response.StatusCode)
	}
	if connect.uploadInput == nil || connect.uploadInput.ConnectorChannelID != "cch_user_a" || connect.uploadInput.RecipientRef != "wx_user_2" || connect.uploadInput.Filename != "done.mp4" {
		t.Fatalf("UploadMedia 入参异常: %+v", connect.uploadInput)
	}
	var payload map[string]any
	if err = json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("media upload response JSON decode failed: %v", err)
	}
	if payload["media_ref"] != "media_ref_1" || payload["media_type"] != "video" {
		t.Fatalf("media upload response 异常: %+v", payload)
	}
}

func TestMediaDownloadUsesTemporaryMediaRefWithoutAPIKey(t *testing.T) {
	media := &testMediaService{
		stream: &mediaservice.OpenMediaStreamResult{
			Reader:      io.NopCloser(bytes.NewReader([]byte("image-bytes"))),
			ContentType: "image/jpeg",
			Filename:    "photo.jpg",
			Size:        int64(len("image-bytes")),
		},
	}
	endpointService := newService(&testConnectService{}, &testChannelService{}, media)
	app := fiber.New()
	endpointService.registerRoutes(app)

	request := httptest.NewRequest(http.MethodGet, connectorprotocol.ConnectorMediaRefPathPrefix+"/media_ref_1", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("media download request returned error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("media download 应成功，got=%d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read media download body failed: %v", err)
	}
	if string(body) != "image-bytes" {
		t.Fatalf("media download body 异常: %q", body)
	}
	if response.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("media download content-type 异常: %s", response.Header.Get("Content-Type"))
	}
	if response.Header.Get("Content-Disposition") != "inline; filename=\"photo.jpg\"" {
		t.Fatalf("media download disposition 异常: %s", response.Header.Get("Content-Disposition"))
	}
}

func TestMediaDownloadReturnsJSONWhenStreamReadFails(t *testing.T) {
	media := &testMediaService{
		stream: &mediaservice.OpenMediaStreamResult{
			Reader:      failingReadCloser{},
			ContentType: "image/jpeg",
			Filename:    "photo.jpg",
		},
	}
	endpointService := newService(&testConnectService{}, &testChannelService{}, media)
	app := fiber.New()
	endpointService.registerRoutes(app)

	request := httptest.NewRequest(http.MethodGet, connectorprotocol.ConnectorMediaRefPathPrefix+"/media_ref_1", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("media download request returned error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("media download 读流失败应返回 500，got=%d", response.StatusCode)
	}
	var payload map[string]any
	if err = json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("media download error response JSON decode failed: %v", err)
	}
	if payload["error"] != "media_download_failed" {
		t.Fatalf("media download error response 异常: %+v", payload)
	}
}
