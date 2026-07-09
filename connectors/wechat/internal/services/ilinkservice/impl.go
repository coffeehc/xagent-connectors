package ilinkservice

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coffeehc/base/log"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
	"go.uber.org/zap"
)

const (
	requestTTL            = 8 * time.Second
	statusTTL             = 5 * time.Second
	configTTL             = 10 * time.Second
	getUpdatesEndpoint    = "ilink/bot/getupdates"
	getUploadURLEndpoint  = "ilink/bot/getuploadurl"
	sendMessageEndpoint   = "ilink/bot/sendmessage"
	getConfigEndpoint     = "ilink/bot/getconfig"
	sendTypingEndpoint    = "ilink/bot/sendtyping"
	notifyStartEndpoint   = "ilink/bot/msg/notifystart"
	notifyStopEndpoint    = "ilink/bot/msg/notifystop"
	defaultChannelVersion = protocol.DefaultVersion
	defaultBotAgent       = "xAgent/0.1"
)

type serviceImpl struct {
	apiBaseURL string
	botType    string
	httpClient *http.Client
}

func newService(apiBaseURL string, botType string, httpClient *http.Client) Service {
	if apiBaseURL == "" {
		apiBaseURL = protocol.WeChatAPIBaseURL
	}
	if botType == "" {
		botType = protocol.WeChatBotType
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &serviceImpl{
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
		botType:    botType,
		httpClient: httpClient,
	}
}

// Start 完成微信 iLink API 服务启动检查。
func (impl *serviceImpl) Start(context.Context) error {
	return nil
}

// Stop 停止微信 iLink API 服务。
func (impl *serviceImpl) Stop(context.Context) error {
	return nil
}

// APIBaseURL 返回当前微信 iLink API endpoint。
func (impl *serviceImpl) APIBaseURL() string {
	return impl.apiBaseURL
}

// BotType 返回当前微信 iLink bot_type。
func (impl *serviceImpl) BotType() string {
	return impl.botType
}

// FetchQRCode 获取真实微信二维码载荷。
func (impl *serviceImpl) FetchQRCode(ctx context.Context, localBotTokens []string) (*QRCodeResponse, error) {
	requestURL, err := buildWeChatURL(impl.apiBaseURL, "ilink/bot/get_bot_qrcode?bot_type="+url.QueryEscape(impl.botType))
	if err != nil {
		return nil, err
	}
	log.Debug("调用微信二维码 API",
		zap.String("action", "get_bot_qrcode"),
		zap.String("bot_type", impl.botType),
		zap.Int("local_token_count", len(localBotTokens)),
		zap.String("result", "started"),
	)
	output := &QRCodeResponse{}
	input := map[string]any{"local_token_list": localBotTokens}
	if err = impl.doRequest(ctx, http.MethodPost, requestURL, input, output, requestTTL); err != nil {
		log.Debug("调用微信二维码 API 失败",
			zap.String("action", "get_bot_qrcode"),
			zap.String("bot_type", impl.botType),
			zap.Int("local_token_count", len(localBotTokens)),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return nil, fmt.Errorf("获取微信二维码失败: %w", err)
	}
	log.Debug("调用微信二维码 API 完成",
		zap.String("action", "get_bot_qrcode"),
		zap.String("bot_type", impl.botType),
		zap.Bool("has_qrcode", strings.TrimSpace(output.QRCode) != ""),
		zap.Bool("has_qrcode_image_content", strings.TrimSpace(output.QRCodeImageContent) != ""),
		zap.String("result", "succeeded"),
	)
	return output, nil
}

// FetchQRCodeStatus 轮询真实微信二维码状态。
func (impl *serviceImpl) FetchQRCodeStatus(ctx context.Context, input QRCodeStatusInput) (*QRCodeStatusResponse, error) {
	qrcode := strings.TrimSpace(input.QRCode)
	if strings.TrimSpace(qrcode) == "" {
		return nil, fmt.Errorf("微信二维码会话缺少 qrcode")
	}
	endpoint := "ilink/bot/get_qrcode_status?qrcode=" + url.QueryEscape(qrcode)
	if verifyCode := strings.TrimSpace(input.VerifyCode); verifyCode != "" {
		endpoint += "&verify_code=" + url.QueryEscape(verifyCode)
	}
	requestURL, err := buildWeChatURL(input.BaseURL, endpoint)
	if err != nil {
		return nil, err
	}
	output := &QRCodeStatusResponse{}
	if err = impl.doRequest(ctx, http.MethodGet, requestURL, nil, output, statusTTL); err != nil {
		return nil, err
	}
	return output, nil
}

// GetUpdates 长轮询读取微信主动发来的消息。
func (impl *serviceImpl) GetUpdates(ctx context.Context, input GetUpdatesInput) (*GetUpdatesResponse, error) {
	timeout := time.Duration(input.LongPollTimeoutMillis) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(protocol.WeChatLongPollTimeoutMillis) * time.Millisecond
	}
	requestURL, err := buildWeChatURL(botRequestBaseURL(input.BaseURL, impl.apiBaseURL), getUpdatesEndpoint)
	if err != nil {
		return nil, err
	}
	output := &GetUpdatesResponse{}
	body := map[string]any{
		"get_updates_buf": strings.TrimSpace(input.GetUpdatesBuf),
		"base_info":       buildBaseInfo(),
	}
	if err = impl.doBotRequest(ctx, http.MethodPost, requestURL, input.BotToken, body, output, timeout); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return &GetUpdatesResponse{Ret: 0, GetUpdatesBuf: input.GetUpdatesBuf}, nil
		}
		return nil, err
	}
	return output, nil
}

// SendTextMessage 通过当前登录态向指定微信上下文发送文本消息。
func (impl *serviceImpl) SendTextMessage(ctx context.Context, input SendTextMessageInput) (*SendTextMessageResult, error) {
	contactID := strings.TrimSpace(input.ContactID)
	contextToken := strings.TrimSpace(input.ContextToken)
	if contactID == "" && contextToken == "" {
		return nil, fmt.Errorf("contact_id 或 context_token 不能为空")
	}
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return nil, fmt.Errorf("消息内容不能为空")
	}
	log.Debug("调用微信消息发送 API",
		zap.String("action", "sendmessage"),
		zap.Bool("has_contact_id", contactID != ""),
		zap.Bool("has_context_token", contextToken != ""),
		zap.Int("text_length", len(text)),
		zap.String("result", "started"),
	)
	clientID := "msg_" + randomToken(12)
	toUserID := contactID
	if toUserID == "" {
		toUserID = contextToken
	}
	result, err := impl.SendMessage(ctx, SendMessageInput{
		BaseURL:  input.BaseURL,
		BotToken: input.BotToken,
		Message: &WeixinMessage{
			ToUserID:     toUserID,
			ClientID:     clientID,
			MessageType:  MessageTypeBot,
			MessageState: MessageStateFinish,
			ItemList: []*MessageItem{{
				Type:     MessageItemTypeText,
				TextItem: &TextItem{Text: text},
			}},
			ContextToken: contextToken,
		},
	})
	if err != nil {
		log.Debug("调用微信消息发送 API 失败",
			zap.String("action", "sendmessage"),
			zap.Bool("has_contact_id", contactID != ""),
			zap.Bool("has_context_token", contextToken != ""),
			zap.Int("text_length", len(text)),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return nil, fmt.Errorf("发送微信消息失败: %w", err)
	}
	log.Debug("调用微信消息发送 API 完成",
		zap.String("action", "sendmessage"),
		zap.Bool("has_contact_id", contactID != ""),
		zap.Bool("has_context_token", contextToken != ""),
		zap.String("message_id", result.MessageID),
		zap.String("result", "succeeded"),
	)
	return &SendTextMessageResult{MessageID: result.MessageID}, nil
}

// SendMessage 通过当前登录态发送官方 WeixinMessage 结构。
func (impl *serviceImpl) SendMessage(ctx context.Context, input SendMessageInput) (*SendMessageResult, error) {
	if input.Message == nil {
		return nil, fmt.Errorf("微信消息不能为空")
	}
	if strings.TrimSpace(input.Message.ClientID) == "" {
		input.Message.ClientID = "msg_" + randomToken(12)
	}
	requestURL, err := buildWeChatURL(botRequestBaseURL(input.BaseURL, impl.apiBaseURL), sendMessageEndpoint)
	if err != nil {
		return nil, err
	}
	output := &SendMessageResult{}
	body := map[string]any{
		"msg":       input.Message,
		"base_info": buildBaseInfo(),
	}
	if err = impl.doBotRequest(ctx, http.MethodPost, requestURL, input.BotToken, body, output, requestTTL); err != nil {
		return nil, err
	}
	if output.Ret != 0 {
		return nil, fmt.Errorf("sendmessage ret=%d errmsg=%s", output.Ret, output.ErrMsg)
	}
	output.MessageID = input.Message.ClientID
	return output, nil
}

// GetUploadURL 获取微信 CDN 上传 URL。
func (impl *serviceImpl) GetUploadURL(ctx context.Context, input GetUploadURLInput) (*GetUploadURLResponse, error) {
	requestURL, err := buildWeChatURL(botRequestBaseURL(input.BaseURL, impl.apiBaseURL), getUploadURLEndpoint)
	if err != nil {
		return nil, err
	}
	output := &GetUploadURLResponse{}
	body := map[string]any{
		"filekey":          strings.TrimSpace(input.FileKey),
		"media_type":       input.MediaType,
		"to_user_id":       strings.TrimSpace(input.ToUserID),
		"rawsize":          input.RawSize,
		"rawfilemd5":       strings.TrimSpace(input.RawFileMD5),
		"filesize":         input.FileSize,
		"thumb_rawsize":    input.ThumbRawSize,
		"thumb_rawfilemd5": strings.TrimSpace(input.ThumbRawFileMD5),
		"thumb_filesize":   input.ThumbFileSize,
		"no_need_thumb":    input.NoNeedThumb,
		"aeskey":           strings.TrimSpace(input.AESKey),
		"base_info":        buildBaseInfo(),
	}
	if err = impl.doBotRequest(ctx, http.MethodPost, requestURL, input.BotToken, body, output, requestTTL); err != nil {
		return nil, err
	}
	return output, nil
}

// GetConfig 获取指定微信用户会话配置。
func (impl *serviceImpl) GetConfig(ctx context.Context, input GetConfigInput) (*GetConfigResponse, error) {
	requestURL, err := buildWeChatURL(botRequestBaseURL(input.BaseURL, impl.apiBaseURL), getConfigEndpoint)
	if err != nil {
		return nil, err
	}
	output := &GetConfigResponse{}
	body := map[string]any{
		"ilink_user_id": strings.TrimSpace(input.ILinkUserID),
		"context_token": strings.TrimSpace(input.ContextToken),
		"base_info":     buildBaseInfo(),
	}
	if err = impl.doBotRequest(ctx, http.MethodPost, requestURL, input.BotToken, body, output, configTTL); err != nil {
		return nil, err
	}
	return output, nil
}

// SendTyping 发送微信输入状态。
func (impl *serviceImpl) SendTyping(ctx context.Context, input SendTypingInput) (*SendTypingResponse, error) {
	requestURL, err := buildWeChatURL(botRequestBaseURL(input.BaseURL, impl.apiBaseURL), sendTypingEndpoint)
	if err != nil {
		return nil, err
	}
	status := input.Status
	if status == 0 {
		status = TypingStatusTyping
	}
	output := &SendTypingResponse{}
	body := map[string]any{
		"ilink_user_id": strings.TrimSpace(input.ILinkUserID),
		"typing_ticket": strings.TrimSpace(input.TypingTicket),
		"status":        status,
		"base_info":     buildBaseInfo(),
	}
	if err = impl.doBotRequest(ctx, http.MethodPost, requestURL, input.BotToken, body, output, configTTL); err != nil {
		return nil, err
	}
	return output, nil
}

// NotifyStart 通知微信 iLink 当前 connector 开始拉取消息。
func (impl *serviceImpl) NotifyStart(ctx context.Context, input NotifyInput) (*NotifyResponse, error) {
	return impl.notify(ctx, notifyStartEndpoint, input)
}

// NotifyStop 通知微信 iLink 当前 connector 停止拉取消息。
func (impl *serviceImpl) NotifyStop(ctx context.Context, input NotifyInput) (*NotifyResponse, error) {
	return impl.notify(ctx, notifyStopEndpoint, input)
}

func (impl *serviceImpl) notify(ctx context.Context, endpoint string, input NotifyInput) (*NotifyResponse, error) {
	requestURL, err := buildWeChatURL(botRequestBaseURL(input.BaseURL, impl.apiBaseURL), endpoint)
	if err != nil {
		return nil, err
	}
	output := &NotifyResponse{}
	body := map[string]any{"base_info": buildBaseInfo()}
	if err = impl.doBotRequest(ctx, http.MethodPost, requestURL, input.BotToken, body, output, configTTL); err != nil {
		return nil, err
	}
	return output, nil
}

func (impl *serviceImpl) doRequest(ctx context.Context, method string, requestURL string, body any, target any, timeout time.Duration) error {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(requestCtx, method, requestURL, reader)
	if err != nil {
		return err
	}
	request.Header.Set("iLink-App-Id", protocol.ILinkAppID)
	request.Header.Set("iLink-App-ClientVersion", protocol.ILinkAppClientVersion)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("AuthorizationType", "ilink_bot_token")
		request.Header.Set("X-WECHAT-UIN", randomWeChatUIN())
	}
	response, err := impl.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &HTTPError{StatusCode: response.StatusCode, Body: strings.TrimSpace(string(responseBody))}
	}
	if target == nil {
		return nil
	}
	if err = json.Unmarshal(responseBody, target); err != nil {
		return fmt.Errorf("解析微信 API 响应失败: %w", err)
	}
	return nil
}

func (impl *serviceImpl) doBotRequest(ctx context.Context, method string, requestURL string, botToken string, body any, target any, timeout time.Duration) error {
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return fmt.Errorf("微信 bot token 不能为空")
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(requestCtx, method, requestURL, reader)
	if err != nil {
		return err
	}
	request.Header.Set("iLink-App-Id", protocol.ILinkAppID)
	request.Header.Set("iLink-App-ClientVersion", protocol.ILinkAppClientVersion)
	request.Header.Set("AuthorizationType", "ilink_bot_token")
	request.Header.Set("Authorization", "Bearer "+botToken)
	request.Header.Set("X-WECHAT-UIN", randomWeChatUIN())
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := impl.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &HTTPError{StatusCode: response.StatusCode, Body: strings.TrimSpace(string(responseBody))}
	}
	if target == nil {
		return nil
	}
	if len(responseBody) == 0 {
		return nil
	}
	if err = json.Unmarshal(responseBody, target); err != nil {
		return fmt.Errorf("解析微信 API 响应失败: %w", err)
	}
	return nil
}

func buildBaseInfo() *BaseInfo {
	return &BaseInfo{
		ChannelVersion: defaultChannelVersion,
		BotAgent:       defaultBotAgent,
	}
}

func (err *HTTPError) Error() string {
	if err.Body == "" {
		return fmt.Sprintf("WeChat API HTTP %d", err.StatusCode)
	}
	return fmt.Sprintf("WeChat API HTTP %d: %s", err.StatusCode, err.Body)
}

func buildWeChatURL(baseURL string, endpoint string) (string, error) {
	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return "", err
	}
	relative, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(relative).String(), nil
}

func botRequestBaseURL(connectionBaseURL string, defaultBaseURL string) string {
	connectionBaseURL = strings.TrimSpace(connectionBaseURL)
	if connectionBaseURL != "" {
		return connectionBaseURL
	}
	return defaultBaseURL
}

func randomToken(byteCount int) string {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}

func randomWeChatUIN() string {
	buffer := make([]byte, 4)
	if _, err := rand.Read(buffer); err != nil {
		return base64.StdEncoding.EncodeToString([]byte(strconv.FormatInt(time.Now().UnixNano()%math.MaxUint32, 10)))
	}
	value := binary.BigEndian.Uint32(buffer)
	return base64.StdEncoding.EncodeToString([]byte(strconv.FormatUint(uint64(value), 10)))
}
