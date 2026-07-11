package telegramservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coffeehc/base/log"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/protocol"
	"go.uber.org/zap"
)

const requestTTL = 10 * time.Second

type serviceImpl struct {
	apiBaseURL string
	httpClient *http.Client
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Description string          `json:"description,omitempty"`
}

func newService(apiBaseURL string, httpClient *http.Client) Service {
	if apiBaseURL == "" {
		apiBaseURL = protocol.TelegramAPIBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &serviceImpl{
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
		httpClient: httpClient,
	}
}

// Start 完成 Telegram Bot API 服务启动检查。
func (impl *serviceImpl) Start(context.Context) error {
	return nil
}

// Stop 停止 Telegram Bot API 服务。
func (impl *serviceImpl) Stop(context.Context) error {
	return nil
}

// APIBaseURL 返回当前 Telegram Bot API endpoint。
func (impl *serviceImpl) APIBaseURL() string {
	return impl.apiBaseURL
}

// GetMe 使用 bot token 读取当前 bot 基本信息。
func (impl *serviceImpl) GetMe(ctx context.Context, botToken string) (*User, error) {
	output := &User{}
	if err := impl.doBotRequest(ctx, botToken, "getMe", nil, output, requestTTL); err != nil {
		return nil, fmt.Errorf("读取 Telegram bot 信息失败: %w", err)
	}
	if !output.IsBot {
		return nil, fmt.Errorf("Telegram token 不属于 bot")
	}
	return output, nil
}

// GetChat 验证并读取 bot 可访问的 Telegram chat。
func (impl *serviceImpl) GetChat(ctx context.Context, input GetChatInput) (*Chat, error) {
	chatID := strings.TrimSpace(input.ChatID)
	if chatID == "" {
		return nil, fmt.Errorf("chat_id 不能为空")
	}
	output := &Chat{}
	body := map[string]any{"chat_id": chatID}
	if err := impl.doBotRequest(ctx, input.BotToken, "getChat", body, output, requestTTL); err != nil {
		return nil, fmt.Errorf("验证 Telegram chat 失败: %w", err)
	}
	return output, nil
}

// GetUpdates 通过长轮询读取指定 bot 的 update 流。
func (impl *serviceImpl) GetUpdates(ctx context.Context, input GetUpdatesInput) (*GetUpdatesResult, error) {
	timeoutSeconds := input.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = protocol.TelegramLongPollTimeoutSeconds
	}
	body := map[string]any{
		"timeout":         timeoutSeconds,
		"allowed_updates": []string{"message"},
	}
	if input.Offset > 0 {
		body["offset"] = input.Offset
	}
	var updates []*Update
	timeout := time.Duration(timeoutSeconds+5) * time.Second
	if err := impl.doBotRequest(ctx, input.BotToken, "getUpdates", body, &updates, timeout); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return &GetUpdatesResult{NextOffset: input.Offset}, nil
		}
		return nil, err
	}
	nextOffset := input.Offset
	for _, update := range updates {
		if update != nil && update.UpdateID >= nextOffset {
			nextOffset = update.UpdateID + 1
		}
	}
	return &GetUpdatesResult{Updates: updates, NextOffset: nextOffset}, nil
}

// SendMessage 通过指定 bot 向指定 chat 发送文本消息。
func (impl *serviceImpl) SendMessage(ctx context.Context, input SendMessageInput) (*Message, error) {
	chatID := strings.TrimSpace(input.ChatID)
	if chatID == "" {
		return nil, fmt.Errorf("chat_id 不能为空")
	}
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return nil, fmt.Errorf("消息内容不能为空")
	}
	output := &Message{}
	body := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if err := impl.doBotRequest(ctx, input.BotToken, "sendMessage", body, output, requestTTL); err != nil {
		return nil, fmt.Errorf("发送 Telegram 消息失败: %w", err)
	}
	return output, nil
}

// SendMedia 通过指定 bot 向指定 chat 发送图片、视频或文件。
func (impl *serviceImpl) SendMedia(ctx context.Context, input SendMediaInput) (*Message, error) {
	chatID := strings.TrimSpace(input.ChatID)
	if chatID == "" {
		return nil, fmt.Errorf("chat_id 不能为空")
	}
	mediaType := strings.TrimSpace(input.MediaType)
	method, fieldName, err := telegramMediaMethod(mediaType)
	if err != nil {
		return nil, err
	}
	output := &Message{}
	fileID := strings.TrimSpace(input.FileID)
	if fileID != "" {
		body := map[string]any{
			"chat_id": chatID,
			fieldName: fileID,
		}
		if caption := strings.TrimSpace(input.Caption); caption != "" {
			body["caption"] = caption
		}
		if err = impl.doBotRequest(ctx, input.BotToken, method, body, output, requestTTL); err != nil {
			return nil, fmt.Errorf("发送 Telegram 媒体失败: %w", err)
		}
		return output, nil
	}
	if err = impl.doBotMultipartRequest(ctx, input.BotToken, method, map[string]string{
		"chat_id": chatID,
		"caption": strings.TrimSpace(input.Caption),
	}, fieldName, input.LocalPath, input.Filename, output, requestTTL); err != nil {
		return nil, fmt.Errorf("上传并发送 Telegram 媒体失败: %w", err)
	}
	return output, nil
}

// GetFile 读取 Telegram file_id 对应的下载路径。
func (impl *serviceImpl) GetFile(ctx context.Context, input GetFileInput) (*File, error) {
	fileID := strings.TrimSpace(input.FileID)
	if fileID == "" {
		return nil, fmt.Errorf("file_id 不能为空")
	}
	output := &File{}
	if err := impl.doBotRequest(ctx, input.BotToken, "getFile", map[string]any{"file_id": fileID}, output, requestTTL); err != nil {
		return nil, fmt.Errorf("读取 Telegram 文件路径失败: %w", err)
	}
	return output, nil
}

// DownloadFile 下载 Telegram file_path 对应的文件内容。
func (impl *serviceImpl) DownloadFile(ctx context.Context, input DownloadFileInput) (*DownloadFileResult, error) {
	botToken := strings.TrimSpace(input.BotToken)
	filePath := strings.TrimLeft(strings.TrimSpace(input.FilePath), "/")
	if botToken == "" || filePath == "" {
		return nil, fmt.Errorf("bot_token 和 file_path 不能为空")
	}
	requestCtx, cancel := context.WithTimeout(ctx, requestTTL)
	defer cancel()
	requestURL := impl.apiBaseURL + "/file/bot" + botToken + "/" + filePath
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := impl.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Telegram 文件下载 HTTP %d", response.StatusCode)
	}
	return &DownloadFileResult{
		ContentType: response.Header.Get("Content-Type"),
		Body:        payload,
	}, nil
}

func (impl *serviceImpl) doBotRequest(ctx context.Context, botToken string, method string, body any, target any, timeout time.Duration) error {
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return fmt.Errorf("bot_token 不能为空")
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
	requestURL := impl.apiBaseURL + "/bot" + botToken + "/" + strings.TrimSpace(method)
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, requestURL, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := impl.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Telegram API HTTP %d", response.StatusCode)
	}
	apiOutput := &apiResponse{}
	if err = json.Unmarshal(payload, apiOutput); err != nil {
		return err
	}
	if !apiOutput.OK {
		return fmt.Errorf("Telegram API error %d: %s", apiOutput.ErrorCode, apiOutput.Description)
	}
	if target == nil || len(apiOutput.Result) == 0 {
		return nil
	}
	if err = json.Unmarshal(apiOutput.Result, target); err != nil {
		log.Debug("解析 Telegram API 响应失败",
			zap.String("method", method),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return err
	}
	return nil
}

func (impl *serviceImpl) doBotMultipartRequest(ctx context.Context, botToken string, method string, fields map[string]string, fileField string, localPath string, filename string, target any, timeout time.Duration) error {
	botToken = strings.TrimSpace(botToken)
	localPath = strings.TrimSpace(localPath)
	if botToken == "" || localPath == "" {
		return fmt.Errorf("bot_token 和本地文件路径不能为空")
	}
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err = writer.WriteField(key, value); err != nil {
			return err
		}
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = filepath.Base(localPath)
	}
	part, err := writer.CreateFormFile(fileField, filename)
	if err != nil {
		return err
	}
	if _, err = io.Copy(part, file); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}

	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	requestURL := impl.apiBaseURL + "/bot" + botToken + "/" + strings.TrimSpace(method)
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, requestURL, &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := impl.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Telegram API HTTP %d", response.StatusCode)
	}
	apiOutput := &apiResponse{}
	if err = json.Unmarshal(payload, apiOutput); err != nil {
		return err
	}
	if !apiOutput.OK {
		return fmt.Errorf("Telegram API error %d: %s", apiOutput.ErrorCode, apiOutput.Description)
	}
	if target == nil || len(apiOutput.Result) == 0 {
		return nil
	}
	return json.Unmarshal(apiOutput.Result, target)
}

func telegramMediaMethod(mediaType string) (string, string, error) {
	switch strings.TrimSpace(mediaType) {
	case "image":
		return "sendPhoto", "photo", nil
	case "video":
		return "sendVideo", "video", nil
	case "file":
		return "sendDocument", "document", nil
	default:
		return "", "", fmt.Errorf("unsupported media_type: %s", mediaType)
	}
}
