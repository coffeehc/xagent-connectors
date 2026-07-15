package feishuservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/scene/registration"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

const (
	appName                    = "xAgent助手"
	appDescription             = "由 xAgent 提供的飞书智能助手"
	maximumDownloadedImageSize = 10 << 20
)

type serviceImpl struct{}
type streamImpl struct{ client *larkws.Client }

func newService() Service { return &serviceImpl{} }

// Start 完成 SDK service 启动检查。
func (impl *serviceImpl) Start(context.Context) error { return nil }

// Stop 停止 SDK service。
func (impl *serviceImpl) Stop(context.Context) error { return nil }

// RegisterApp 通过官方一键创建应用流程创建国内飞书应用。
func (impl *serviceImpl) RegisterApp(ctx context.Context, onQRCode func(QRCodeInfo), onStatusChange func(RegistrationStatus)) (*AppCredential, error) {
	minimal := false
	result, err := registration.RegisterApp(ctx, &registration.Options{
		Source: "xagent-feishu-connector", CreateOnly: true,
		AppPreset: &registration.AppPreset{Name: appName, Desc: appDescription},
		Addons: &registration.AppAddons{
			Preset: &minimal,
			Scopes: registration.AppAddonsScopes{Tenant: []string{"im:message.p2p_msg:readonly", "im:message.group_at_msg:readonly", "im:message:send_as_bot", "im:resource"}},
			Events: registration.AppAddonsEvents{Items: registration.AppAddonsEventItems{Tenant: []string{"im.message.receive_v1"}}},
		},
		OnQRCode: func(info *registration.QRCodeInfo) {
			if info != nil && onQRCode != nil {
				onQRCode(QRCodeInfo{URL: info.URL, ExpiresInSeconds: info.ExpireIn})
			}
		},
		OnStatusChange: func(info *registration.StatusChangeInfo) {
			if info != nil && onStatusChange != nil {
				onStatusChange(RegistrationStatus{Status: info.Status, IntervalSeconds: info.Interval})
			}
		},
	})
	if err != nil {
		return nil, normalizeRegistrationError(err)
	}
	if result.UserInfo != nil && strings.EqualFold(strings.TrimSpace(result.UserInfo.TenantBrand), "lark") {
		return nil, fmt.Errorf("暂不支持 Lark 国际版")
	}
	credential := &AppCredential{AppID: result.ClientID, AppSecret: result.ClientSecret}
	if result.UserInfo != nil {
		credential.UserOpenID = result.UserInfo.OpenID
	}
	return credential, nil
}

func normalizeRegistrationError(err error) error {
	if err == nil {
		return nil
	}
	var accessDenied *registration.AccessDeniedError
	if errors.As(err, &accessDenied) {
		return &RegistrationError{Kind: RegistrationErrorAccessDenied, Code: accessDenied.Code, Description: accessDenied.Description}
	}
	var expired *registration.ExpiredError
	if errors.As(err, &expired) {
		return &RegistrationError{Kind: RegistrationErrorExpired, Code: expired.Code, Description: expired.Description}
	}
	var remote *registration.RegisterAppError
	if errors.As(err, &remote) {
		return &RegistrationError{Kind: RegistrationErrorRemote, Code: remote.Code, Description: remote.Description}
	}
	return &RegistrationError{Kind: RegistrationErrorTransport, Code: "registration_failed", Description: err.Error()}
}

// NewStream 创建指定应用的消息长连接。
func (impl *serviceImpl) NewStream(appID string, appSecret string, handler MessageHandler) Stream {
	eventDispatcher := dispatcher.NewEventDispatcher("", "").OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
		if event == nil || event.Event == nil || event.Event.Message == nil {
			return nil
		}
		message := event.Event.Message
		normalized := InboundMessage{MessageID: value(message.MessageId), ChatID: value(message.ChatId), ThreadID: value(message.ThreadId), ChatType: value(message.ChatType), MessageType: value(message.MessageType), Content: value(message.Content), CreateTime: value(message.CreateTime)}
		if event.Event.Sender != nil {
			normalized.SenderType = value(event.Event.Sender.SenderType)
			if event.Event.Sender.SenderId != nil {
				normalized.SenderOpenID = value(event.Event.Sender.SenderId.OpenId)
			}
		}
		if handler == nil {
			return nil
		}
		return handler(ctx, normalized)
	})
	return &streamImpl{client: larkws.NewClient(appID, appSecret, larkws.WithEventHandler(eventDispatcher))}
}

// Start 启动并阻塞运行长连接。
func (impl *streamImpl) Start(ctx context.Context) error { return impl.client.Start(ctx) }

// Close 关闭长连接。
func (impl *streamImpl) Close() { impl.client.Close() }

// UploadImage 上传消息图片并返回 image_key。
func (impl *serviceImpl) UploadImage(ctx context.Context, appID string, appSecret string, source io.Reader) (string, error) {
	client := lark.NewClient(appID, appSecret)
	req := larkim.NewCreateImageReqBuilder().Body(larkim.NewCreateImageReqBodyBuilder().ImageType("message").Image(source).Build()).Build()
	resp, err := client.Im.V1.Image.Create(ctx, req)
	if err != nil {
		return "", err
	}
	if !resp.Success() || resp.Data == nil || resp.Data.ImageKey == nil {
		return "", fmt.Errorf("飞书上传图片失败: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return strings.TrimSpace(*resp.Data.ImageKey), nil
}

// DownloadMessageImage 下载指定消息中的图片。
func (impl *serviceImpl) DownloadMessageImage(ctx context.Context, appID string, appSecret string, messageID string, imageKey string) ([]byte, string, error) {
	client := lark.NewClient(appID, appSecret)
	req := larkim.NewGetMessageResourceReqBuilder().MessageId(messageID).FileKey(imageKey).Type("image").Build()
	resp, err := client.Im.V1.MessageResource.Get(ctx, req)
	if err != nil {
		return nil, "", err
	}
	if !resp.Success() || resp.File == nil {
		return nil, "", fmt.Errorf("飞书下载图片失败: code=%d msg=%s", resp.Code, resp.Msg)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.File, maximumDownloadedImageSize+1))
	if len(raw) > maximumDownloadedImageSize {
		return nil, "", fmt.Errorf("飞书入站图片超过 10MB")
	}
	return raw, resp.FileName, err
}

// SendText 向指定飞书会话发送文本。
func (impl *serviceImpl) SendText(ctx context.Context, appID string, appSecret string, chatID string, text string) (string, error) {
	content, _ := json.Marshal(map[string]string{"text": text})
	return impl.send(ctx, appID, appSecret, chatID, "text", string(content))
}

// SendImage 向指定飞书会话发送图片。
func (impl *serviceImpl) SendImage(ctx context.Context, appID string, appSecret string, chatID string, imageKey string) (string, error) {
	content, _ := json.Marshal(map[string]string{"image_key": imageKey})
	return impl.send(ctx, appID, appSecret, chatID, "image", string(content))
}

func (impl *serviceImpl) send(ctx context.Context, appID string, appSecret string, chatID string, messageType string, content string) (string, error) {
	client := lark.NewClient(appID, appSecret)
	req := larkim.NewCreateMessageReqBuilder().ReceiveIdType("chat_id").Body(larkim.NewCreateMessageReqBodyBuilder().ReceiveId(chatID).MsgType(messageType).Content(content).Build()).Build()
	resp, err := client.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return "", err
	}
	if !resp.Success() || resp.Data == nil || resp.Data.MessageId == nil {
		return "", fmt.Errorf("飞书发送消息失败: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return *resp.Data.MessageId, nil
}

// ReplyText 回复指定飞书消息。
func (impl *serviceImpl) ReplyText(ctx context.Context, appID string, appSecret string, messageID string, text string) (string, error) {
	content, _ := json.Marshal(map[string]string{"text": text})
	return impl.reply(ctx, appID, appSecret, messageID, "text", string(content))
}

// ReplyImage 回复指定飞书消息图片。
func (impl *serviceImpl) ReplyImage(ctx context.Context, appID string, appSecret string, messageID string, imageKey string) (string, error) {
	content, _ := json.Marshal(map[string]string{"image_key": imageKey})
	return impl.reply(ctx, appID, appSecret, messageID, "image", string(content))
}

func (impl *serviceImpl) reply(ctx context.Context, appID string, appSecret string, messageID string, messageType string, content string) (string, error) {
	client := lark.NewClient(appID, appSecret)
	req := larkim.NewReplyMessageReqBuilder().MessageId(messageID).Body(larkim.NewReplyMessageReqBodyBuilder().MsgType(messageType).Content(content).Build()).Build()
	resp, err := client.Im.V1.Message.Reply(ctx, req)
	if err != nil {
		return "", err
	}
	if !resp.Success() || resp.Data == nil || resp.Data.MessageId == nil {
		return "", fmt.Errorf("飞书回复消息失败: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return *resp.Data.MessageId, nil
}

func value(pointer *string) string {
	if pointer == nil {
		return ""
	}
	return strings.TrimSpace(*pointer)
}
