package connectservice

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coffeehc/base/log"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/feishuservice"
	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/storageservice"
	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	qrcode "github.com/skip2/go-qrcode"
	"go.uber.org/zap"
)

const (
	maximumImageSize       = 10 << 20
	referenceTTL           = 24 * time.Hour
	replyReferenceTTL      = 7 * 24 * time.Hour
	pendingDiagnosticDelay = 30 * time.Second
)

type serviceImpl struct {
	mu           sync.Mutex
	apiKey       string
	storage      storageservice.Service
	feishu       feishuservice.Service
	pusher       MessagePusher
	apps         map[string]*connectordomain.AppBinding
	connections  map[string]*connectordomain.Connection
	auth         map[string]*connectordomain.AuthSession
	authCancel   map[string]context.CancelFunc
	reply        map[string]*connectordomain.ReplyReference
	media        map[string]*connectordomain.MediaReference
	streams      map[string]feishuservice.Stream
	streamCancel map[string]context.CancelFunc
	seen         map[string]int64
	inbound      chan inboundEnvelope
	stop         context.CancelFunc
}

type inboundEnvelope struct {
	ChannelID string
	AppID     string
	AppSecret string
	Message   feishuservice.InboundMessage
}

func newService(config Config, storage storageservice.Service, feishu feishuservice.Service) Service {
	return &serviceImpl{apiKey: config.APIKey, storage: storage, feishu: feishu}
}

// Start 恢复应用绑定并启动长连接。
func (impl *serviceImpl) Start(context.Context) error {
	apps, err := impl.storage.LoadApps()
	if err != nil {
		return err
	}
	reply, media, err := impl.storage.LoadReferences()
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	pruneReferences(reply, media, now)
	ctx, cancel := context.WithCancel(context.Background())
	impl.mu.Lock()
	impl.apps = apps
	impl.reply = reply
	impl.media = media
	impl.connections = map[string]*connectordomain.Connection{}
	impl.auth = map[string]*connectordomain.AuthSession{}
	impl.authCancel = map[string]context.CancelFunc{}
	impl.streams = map[string]feishuservice.Stream{}
	impl.streamCancel = map[string]context.CancelFunc{}
	impl.seen = map[string]int64{}
	impl.inbound = make(chan inboundEnvelope, 128)
	impl.stop = cancel
	for channelID, app := range apps {
		impl.connections[channelID] = connectionFromApp(app)
		impl.startStreamLocked(ctx, app)
	}
	impl.mu.Unlock()
	go impl.runInboundWorker(ctx)
	return impl.storage.SaveReferences(reply, media)
}

// Stop 停止全部长连接和认证会话。
func (impl *serviceImpl) Stop(context.Context) error {
	impl.mu.Lock()
	if impl.stop != nil {
		impl.stop()
	}
	for _, cancel := range impl.authCancel {
		cancel()
	}
	for _, stream := range impl.streams {
		stream.Close()
	}
	impl.mu.Unlock()
	return nil
}

// BindMessagePusher 绑定入站消息推送端口。
func (impl *serviceImpl) BindMessagePusher(pusher MessagePusher) {
	impl.mu.Lock()
	impl.pusher = pusher
	impl.mu.Unlock()
}

// APIKey 返回 connector system API key。
func (impl *serviceImpl) APIKey() string { return impl.apiKey }

// ConnectorID 返回 Connector Card ID。
func (impl *serviceImpl) ConnectorID() string { return protocol.ConnectorCardID }

// StateDir 返回本地状态目录。
func (impl *serviceImpl) StateDir() string { return impl.storage.StateDir() }

// StartAuth 创建扫码创建飞书应用的认证会话。
func (impl *serviceImpl) StartAuth(_ context.Context, connectorChannelID string, request connectorprotocol.ConnectorAuthStartRequest) (*connectorprotocol.ConnectorAuthStartResult, error) {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	flowID := strings.TrimSpace(request.FlowID)
	if flowID == "" {
		flowID = protocol.FeishuQRCreateFlowID
	}
	if connectorChannelID == "" {
		return nil, fmt.Errorf("connector_channel_id required")
	}
	if flowID != protocol.FeishuQRCreateFlowID {
		return nil, fmt.Errorf("unknown_flow")
	}
	if connection := impl.ConnectionByChannel(connectorChannelID); connection != nil {
		return &connectorprotocol.ConnectorAuthStartResult{ConnectorChannelID: connectorChannelID, FlowID: flowID, Status: connectorprotocol.ConnectorAuthStartAuthenticated, Message: "飞书应用已连接", ConnectionDescriptor: impl.BuildConnectionDescriptor(connection)}, nil
	}
	now := time.Now()
	session := &connectordomain.AuthSession{ID: "auth_" + randomToken(18), ConnectorChannelID: connectorChannelID, FlowID: flowID, Status: string(connectorprotocol.ConnectorAuthStatusPending), Message: "正在生成飞书扫码创建应用二维码。", ExpiresAt: now.Add(10 * time.Minute).UnixMilli(), PollIntervalMillis: protocol.AuthPollIntervalMillis, CreatedAt: now.UnixMilli(), Attempt: 1}
	ctx, cancel := context.WithCancel(context.Background())
	impl.mu.Lock()
	for id, existing := range impl.auth {
		if existing.ConnectorChannelID == connectorChannelID && existing.Status == string(connectorprotocol.ConnectorAuthStatusPending) {
			if oldCancel := impl.authCancel[id]; oldCancel != nil {
				oldCancel()
			}
			existing.Status = string(connectorprotocol.ConnectorAuthStatusFailed)
		}
	}
	impl.auth[session.ID] = session
	impl.authCancel[session.ID] = cancel
	impl.mu.Unlock()
	go impl.registerApp(ctx, session.ID, session.Attempt)
	return &connectorprotocol.ConnectorAuthStartResult{ConnectorChannelID: connectorChannelID, FlowID: flowID, AuthSessionID: session.ID, Status: connectorprotocol.ConnectorAuthStartPending, ExpiresAt: session.ExpiresAt, PollIntervalMillis: protocol.AuthPollIntervalMillis, Message: session.Message}, nil
}

func (impl *serviceImpl) registerApp(ctx context.Context, sessionID string, attempt int64) {
	credential, err := impl.feishu.RegisterApp(ctx, func(info feishuservice.QRCodeInfo) {
		png, encodeErr := qrcode.Encode(info.URL, qrcode.Medium, 384)
		if encodeErr != nil {
			return
		}
		impl.mu.Lock()
		if session := impl.auth[sessionID]; session != nil && session.Attempt == attempt {
			session.QRCodeText = info.URL
			session.QRCodeImage = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
			session.ExpiresAt = time.Now().Add(time.Duration(info.ExpiresInSeconds) * time.Second).UnixMilli()
			session.Message = "请使用飞书扫码并确认创建 xAgent助手。"
		}
		impl.mu.Unlock()
	}, func(status feishuservice.RegistrationStatus) {
		impl.applyRegistrationStatus(sessionID, attempt, status)
	})
	impl.mu.Lock()
	session := impl.auth[sessionID]
	if session == nil || session.Attempt != attempt {
		impl.mu.Unlock()
		return
	}
	if err != nil {
		if ctx.Err() != nil {
			session.Status = string(connectorprotocol.ConnectorAuthStatusFailed)
			session.Message = "认证已取消。"
		} else {
			impl.applyRegistrationErrorLocked(session, err)
		}
		status := session.Status
		remoteStatus := session.RemoteStatus
		remoteErrorCode := session.RemoteErrorCode
		message := session.Message
		connectorChannelID := session.ConnectorChannelID
		delete(impl.authCancel, sessionID)
		impl.mu.Unlock()
		fields := []zap.Field{
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("auth_session_id", sessionID),
			zap.Int64("attempt", attempt),
			zap.String("auth_status", status),
			zap.String("remote_status", remoteStatus),
			zap.String("remote_error_code", remoteErrorCode),
			zap.String("message", message),
			zap.Error(err),
		}
		if ctx.Err() != nil {
			log.Debug("飞书应用注册已取消", fields...)
		} else {
			log.Error("飞书应用注册失败", fields...)
		}
		return
	}
	app := &connectordomain.AppBinding{ConnectorChannelID: session.ConnectorChannelID, AppID: credential.AppID, AppSecret: credential.AppSecret, TenantOpenID: credential.UserOpenID, CreatedAt: time.Now().UnixMilli()}
	impl.apps[app.ConnectorChannelID] = app
	impl.connections[app.ConnectorChannelID] = connectionFromApp(app)
	session.Status = string(connectorprotocol.ConnectorAuthStatusAuthenticated)
	session.Message = "飞书应用已创建并连接。"
	delete(impl.authCancel, sessionID)
	apps := cloneApps(impl.apps)
	impl.startStreamLocked(context.Background(), app)
	connection := impl.connections[app.ConnectorChannelID]
	pusher := impl.pusher
	impl.mu.Unlock()
	if saveErr := impl.storage.SaveApps(apps); saveErr != nil {
		log.Debug("保存飞书应用凭据失败", zap.Error(saveErr))
	}
	if pusher != nil {
		_ = pusher.PushConnectionDescriptor(context.Background(), app.ConnectorChannelID, impl.BuildConnectionDescriptor(connection))
	}
}

func (impl *serviceImpl) applyRegistrationStatus(sessionID string, attempt int64, status feishuservice.RegistrationStatus) {
	impl.mu.Lock()
	session := impl.auth[sessionID]
	if session == nil || session.Attempt != attempt {
		impl.mu.Unlock()
		return
	}
	remoteStatus := strings.TrimSpace(status.Status)
	statusChanged := session.RemoteStatus != remoteStatus
	intervalChanged := remoteStatus == "slow_down" && status.IntervalSeconds > 0 && session.PollIntervalMillis != int64(status.IntervalSeconds)*int64(time.Second/time.Millisecond)
	session.RemoteStatus = remoteStatus
	session.RemoteStatusAt = time.Now().UnixMilli()
	switch session.RemoteStatus {
	case "polling":
		session.Message = "等待扫码或飞书返回应用创建结果。"
	case "slow_down":
		if status.IntervalSeconds > 0 {
			session.PollIntervalMillis = int64(status.IntervalSeconds) * int64(time.Second/time.Millisecond)
		}
		session.Message = "飞书要求降低查询频率，正在继续等待创建结果。"
	case "domain_switched":
		session.Message = "检测到 Lark 国际版，当前 connector 只支持国内飞书。"
	}
	connectorChannelID := session.ConnectorChannelID
	message := session.Message
	impl.mu.Unlock()
	if !statusChanged && !intervalChanged {
		return
	}
	log.Debug("飞书应用注册状态变化",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("auth_session_id", sessionID),
		zap.Int64("attempt", attempt),
		zap.String("remote_status", status.Status),
		zap.Int("remote_interval_seconds", status.IntervalSeconds),
		zap.String("message", message),
	)
}

func (impl *serviceImpl) applyRegistrationErrorLocked(session *connectordomain.AuthSession, err error) {
	var registrationError *feishuservice.RegistrationError
	if !errors.As(err, &registrationError) {
		session.Status = string(connectorprotocol.ConnectorAuthStatusFailed)
		session.RemoteErrorCode = "registration_failed"
		session.Message = "飞书应用创建失败：" + err.Error()
		return
	}
	session.RemoteErrorCode = registrationError.Code
	description := strings.TrimSpace(registrationError.Description)
	if description == "" {
		description = "飞书未返回具体原因"
	}
	if registrationError.Kind == feishuservice.RegistrationErrorExpired {
		session.Status = string(connectorprotocol.ConnectorAuthStatusQRRefreshRequired)
		session.Message = "飞书二维码已过期，请刷新后重新扫码。"
		return
	}
	session.Status = string(connectorprotocol.ConnectorAuthStatusFailed)
	session.Message = fmt.Sprintf("飞书应用创建失败（%s）：%s", registrationError.Code, description)
}

// AuthStatus 返回认证状态。
func (impl *serviceImpl) AuthStatus(_ context.Context, connectorChannelID string, authSessionID string, refresh bool) (*connectorprotocol.ConnectorAuthStatusResult, bool) {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	authSessionID = strings.TrimSpace(authSessionID)
	if authSessionID == "" {
		connection := impl.ConnectionByChannel(connectorChannelID)
		status := connectorprotocol.ConnectorAuthStatusUnauthenticated
		message := "飞书应用尚未连接"
		var descriptor *connectorprotocol.ConnectionDescriptor
		if connection != nil {
			status = connectorprotocol.ConnectorAuthStatusAuthenticated
			message = "飞书应用已连接"
			descriptor = impl.BuildConnectionDescriptor(connection)
		}
		return &connectorprotocol.ConnectorAuthStatusResult{ConnectorChannelID: connectorChannelID, FlowID: protocol.FeishuQRCreateFlowID, Status: status, Message: message, ConnectionDescriptor: descriptor}, true
	}
	if refresh {
		impl.refreshAuthSession(connectorChannelID, authSessionID)
	}
	impl.mu.Lock()
	session := impl.auth[authSessionID]
	if session == nil || session.ConnectorChannelID != connectorChannelID {
		impl.mu.Unlock()
		return nil, false
	}
	if time.Now().UnixMilli() > session.ExpiresAt && session.Status == string(connectorprotocol.ConnectorAuthStatusPending) {
		session.Status = string(connectorprotocol.ConnectorAuthStatusExpired)
		session.Message = "飞书二维码已过期，请刷新后重新扫码。"
	}
	snapshot := *session
	connection := impl.connections[connectorChannelID]
	impl.mu.Unlock()
	status := connectorprotocol.ConnectorAuthStatus(snapshot.Status)
	message := snapshot.Message
	if status == connectorprotocol.ConnectorAuthStatusPending && snapshot.RemoteStatus == "polling" && time.Since(time.UnixMilli(snapshot.CreatedAt)) >= pendingDiagnosticDelay {
		message = "飞书尚未返回应用创建结果；如果飞书页面已经提示失败，请取消后重试。"
	}
	pollIntervalMillis := snapshot.PollIntervalMillis
	if pollIntervalMillis <= 0 {
		pollIntervalMillis = protocol.AuthPollIntervalMillis
	}
	result := &connectorprotocol.ConnectorAuthStatusResult{ConnectorChannelID: connectorChannelID, FlowID: snapshot.FlowID, AuthSessionID: snapshot.ID, Status: status, Message: message, QRCodeText: snapshot.QRCodeText, QRCodeImage: snapshot.QRCodeImage, ExpiresAt: snapshot.ExpiresAt, PollIntervalMillis: pollIntervalMillis}
	if status == connectorprotocol.ConnectorAuthStatusAuthenticated {
		result.ConnectionDescriptor = impl.BuildConnectionDescriptor(connection)
	}
	return result, true
}

func (impl *serviceImpl) refreshAuthSession(connectorChannelID string, authSessionID string) {
	now := time.Now()
	impl.mu.Lock()
	session := impl.auth[authSessionID]
	if session == nil || session.ConnectorChannelID != connectorChannelID || session.Status == string(connectorprotocol.ConnectorAuthStatusAuthenticated) {
		impl.mu.Unlock()
		return
	}
	if cancel := impl.authCancel[authSessionID]; cancel != nil {
		cancel()
	}
	session.Attempt++
	session.Status = string(connectorprotocol.ConnectorAuthStatusPending)
	session.Message = "正在刷新飞书扫码创建应用二维码。"
	session.RemoteStatus = ""
	session.RemoteErrorCode = ""
	session.QRCodeText = ""
	session.QRCodeImage = ""
	session.ExpiresAt = now.Add(10 * time.Minute).UnixMilli()
	session.PollIntervalMillis = protocol.AuthPollIntervalMillis
	session.CreatedAt = now.UnixMilli()
	session.RemoteStatusAt = 0
	attempt := session.Attempt
	ctx, cancel := context.WithCancel(context.Background())
	impl.authCancel[authSessionID] = cancel
	impl.mu.Unlock()
	go impl.registerApp(ctx, authSessionID, attempt)
}

// CancelAuth 取消认证会话。
func (impl *serviceImpl) CancelAuth(_ context.Context, connectorChannelID string, authSessionID string) *connectorprotocol.ConnectorAuthCancelResult {
	result := &connectorprotocol.ConnectorAuthCancelResult{ConnectorChannelID: connectorChannelID, AuthSessionID: authSessionID, Status: connectorprotocol.ConnectorAuthCancelStatusNotFound, AuthStatus: connectorprotocol.ConnectorAuthStatusUnauthenticated, Message: "未找到认证会话"}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	session := impl.auth[authSessionID]
	if session == nil || session.ConnectorChannelID != connectorChannelID {
		return result
	}
	if session.Status == string(connectorprotocol.ConnectorAuthStatusAuthenticated) {
		result.Status = connectorprotocol.ConnectorAuthCancelStatusIgnored
		result.AuthStatus = connectorprotocol.ConnectorAuthStatusAuthenticated
		result.Message = "飞书应用已连接"
		result.ConnectionDescriptor = impl.BuildConnectionDescriptor(impl.connections[connectorChannelID])
		return result
	}
	if cancel := impl.authCancel[authSessionID]; cancel != nil {
		cancel()
	}
	session.Status = string(connectorprotocol.ConnectorAuthStatusFailed)
	session.Message = "认证已取消。"
	result.Status = connectorprotocol.ConnectorAuthCancelStatusCanceled
	result.AuthStatus = connectorprotocol.ConnectorAuthStatusUnauthenticated
	result.Message = session.Message
	return result
}

// ConnectionByChannel 返回 channel connection。
func (impl *serviceImpl) ConnectionByChannel(channelID string) *connectordomain.Connection {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	connection := impl.connections[strings.TrimSpace(channelID)]
	if connection == nil {
		return nil
	}
	copy := *connection
	return &copy
}

// LogoutByChannel 删除 connector 内应用凭据并停止长连接。
func (impl *serviceImpl) LogoutByChannel(channelID string) bool {
	impl.mu.Lock()
	if impl.apps[channelID] == nil {
		impl.mu.Unlock()
		return false
	}
	delete(impl.apps, channelID)
	delete(impl.connections, channelID)
	if cancel := impl.streamCancel[channelID]; cancel != nil {
		cancel()
	}
	if stream := impl.streams[channelID]; stream != nil {
		stream.Close()
	}
	delete(impl.streamCancel, channelID)
	delete(impl.streams, channelID)
	apps := cloneApps(impl.apps)
	impl.mu.Unlock()
	_ = impl.storage.SaveApps(apps)
	return true
}

func (impl *serviceImpl) startStreamLocked(parent context.Context, app *connectordomain.AppBinding) {
	if app == nil || impl.streams[app.ConnectorChannelID] != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	stream := impl.feishu.NewStream(app.AppID, app.AppSecret, func(_ context.Context, message feishuservice.InboundMessage) error {
		select {
		case impl.inbound <- inboundEnvelope{ChannelID: app.ConnectorChannelID, AppID: app.AppID, AppSecret: app.AppSecret, Message: message}:
		default:
			return fmt.Errorf("飞书入站队列已满")
		}
		return nil
	})
	impl.streams[app.ConnectorChannelID] = stream
	impl.streamCancel[app.ConnectorChannelID] = cancel
	go func(channelID string) {
		if err := stream.Start(ctx); err != nil && ctx.Err() == nil {
			log.Debug("飞书长连接退出", zap.String("connector_channel_id", channelID), zap.Error(err))
		}
	}(app.ConnectorChannelID)
}

func (impl *serviceImpl) runInboundWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case envelope := <-impl.inbound:
			impl.processInbound(ctx, envelope)
		}
	}
}

func (impl *serviceImpl) processInbound(ctx context.Context, envelope inboundEnvelope) {
	message := envelope.Message
	if message.MessageID == "" || message.SenderType != "user" || message.MessageType != "text" && message.MessageType != "image" {
		return
	}
	var replyRef string
	var appsToSave map[string]*connectordomain.AppBinding
	impl.mu.Lock()
	if _, exists := impl.seen[message.MessageID]; exists {
		impl.mu.Unlock()
		return
	}
	impl.seen[message.MessageID] = time.Now().UnixMilli()
	if len(impl.seen) > 4096 {
		impl.seen = map[string]int64{message.MessageID: time.Now().UnixMilli()}
	}
	if isDefaultFeishuConversation(message.ChatType) {
		if app := impl.apps[envelope.ChannelID]; app != nil && app.AppID == envelope.AppID && strings.TrimSpace(message.ChatID) != "" {
			app.DefaultChatID = strings.TrimSpace(message.ChatID)
			app.DefaultSenderOpenID = strings.TrimSpace(message.SenderOpenID)
			app.DefaultChatBoundAt = time.Now().UnixMilli()
			appsToSave = cloneApps(impl.apps)
		}
	} else {
		reference := &connectordomain.ReplyReference{Ref: "reply_" + randomToken(24), ConnectorChannelID: envelope.ChannelID, AppID: envelope.AppID, MessageID: message.MessageID, ChatID: message.ChatID, ThreadID: message.ThreadID, ExpiresAt: time.Now().Add(replyReferenceTTL).UnixMilli()}
		impl.reply[reference.Ref] = reference
		replyRef = reference.Ref
	}
	pusher := impl.pusher
	impl.mu.Unlock()
	if appsToSave != nil {
		if err := impl.storage.SaveApps(appsToSave); err != nil {
			log.Debug("保存飞书默认单聊失败", zap.Error(err))
		}
	}
	rawText := ""
	var mediaItems []map[string]any
	if message.MessageType == "text" {
		var content struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(message.Content), &content) != nil || strings.TrimSpace(content.Text) == "" {
			return
		}
		rawText = strings.TrimSpace(content.Text)
	} else {
		var content struct {
			ImageKey string `json:"image_key"`
		}
		if json.Unmarshal([]byte(message.Content), &content) != nil || content.ImageKey == "" {
			return
		}
		mediaRef, err := impl.cacheInboundImage(ctx, envelope, message.MessageID, content.ImageKey)
		if err != nil {
			log.Debug("缓存飞书入站图片失败", zap.Error(err))
			return
		}
		mediaItems = []map[string]any{{"type": "image", "media_ref": mediaRef.Ref, "filename": mediaRef.Filename, "mime_type": mediaRef.ContentType, "byte_size": mediaRef.ByteSize, "download_url": "/media/refs/" + mediaRef.Ref, "expires_at": mediaRef.ExpiresAt}}
	}
	payload := buildFeishuInboundPayload(message, replyRef, rawText, mediaItems)
	impl.persistReferences()
	if pusher != nil {
		_ = pusher.PushMessage(ctx, envelope.ChannelID, payload)
	}
}

func buildFeishuInboundPayload(message feishuservice.InboundMessage, replyRef string, rawText string, mediaItems []map[string]any) map[string]any {
	rawText = strings.TrimSpace(rawText)
	senderName := strings.TrimSpace(message.SenderOpenID)
	visibleText := buildFeishuInboundVisibleText(message, replyRef, rawText, senderName)
	textToolID := protocol.FeishuMessageReplyToolID
	imageToolID := protocol.FeishuMessageReplyImageToolID
	if isDefaultFeishuConversation(message.ChatType) {
		textToolID = protocol.FeishuMessageSendToolID
		imageToolID = protocol.FeishuMessageSendImageToolID
	}
	payload := map[string]any{
		"provider":            "feishu",
		"profile":             "xagent.im.v1",
		"event_kind":          "im.message.received",
		"message_id":          message.MessageID,
		"chat_id":             message.ChatID,
		"chat_type":           message.ChatType,
		"from":                message.SenderOpenID,
		"sender_id":           message.SenderOpenID,
		"sender_name":         senderName,
		"display_name":        senderName,
		"text":                visibleText,
		"content":             visibleText,
		"raw_text":            rawText,
		"message_type":        message.MessageType,
		"received_at":         time.Now().UnixMilli(),
		"create_time_ms":      feishuCreateTimeMillis(message.CreateTime),
		"reply_tool_id":       textToolID,
		"reply_image_tool_id": imageToolID,
		"related_skill_ids":   []string{protocol.ConnectorSkillIMReplyID},
		"reply": map[string]any{
			"required": true,
			"tool_id":  textToolID,
		},
		"skill": map[string]any{
			"skill_id":          protocol.ConnectorSkillIMReplyID,
			"required_tool_ids": []string{textToolID, imageToolID},
		},
		"activation_message": visibleText,
	}
	if strings.TrimSpace(replyRef) != "" {
		payload["reply_ref"] = replyRef
		payload["reply"].(map[string]any)["reply_ref"] = replyRef
	}
	if len(mediaItems) > 0 {
		payload["media"] = mediaItems
	}
	return payload
}

func buildFeishuInboundVisibleText(message feishuservice.InboundMessage, replyRef string, rawText string, senderName string) string {
	userText := strings.TrimSpace(rawText)
	if userText == "" {
		userText = "无"
	}
	if strings.TrimSpace(senderName) == "" {
		senderName = "未知用户"
	}
	var builder strings.Builder
	builder.WriteString("来自飞书的用户消息：\n")
	builder.WriteString("发送方：")
	builder.WriteString(senderName)
	builder.WriteString("\n会话类型：")
	builder.WriteString(feishuChatTypeLabel(message.ChatType))
	builder.WriteString("\n消息类型：")
	builder.WriteString(feishuMessageTypeLabel(message.MessageType))
	builder.WriteString("\n用户文本：")
	builder.WriteString(userText)
	if isDefaultFeishuConversation(message.ChatType) {
		builder.WriteString("\n\n先处理飞书消息。文本回复使用 ")
		builder.WriteString(protocol.FeishuMessageSendToolID)
		builder.WriteString(" 工具，参数 text 是回复内容；如果需要发送图片，先加载 ")
		builder.WriteString(protocol.ConnectorSkillIMReplyID)
		builder.WriteString(" skill，并使用 ")
		builder.WriteString(protocol.FeishuMessageSendImageToolID)
		builder.WriteString(" 工具。")
	} else {
		builder.WriteString("\n\n先处理飞书消息。文本回复使用 ")
		builder.WriteString(protocol.FeishuMessageReplyToolID)
		builder.WriteString(" 工具，参数 reply_ref 必须使用 ")
		builder.WriteString(replyRef)
		builder.WriteString("，参数 text 是回复内容；如果需要回复图片，先加载 ")
		builder.WriteString(protocol.ConnectorSkillIMReplyID)
		builder.WriteString(" skill，并使用 ")
		builder.WriteString(protocol.FeishuMessageReplyImageToolID)
		builder.WriteString(" 工具，参数 reply_ref 仍使用 ")
		builder.WriteString(replyRef)
		builder.WriteString("。")
	}
	return builder.String()
}

func isDefaultFeishuConversation(chatType string) bool {
	return strings.TrimSpace(chatType) == "p2p"
}

func feishuChatTypeLabel(chatType string) string {
	switch strings.TrimSpace(chatType) {
	case "p2p":
		return "单聊"
	case "group":
		return "群聊"
	case "topic_group":
		return "话题群"
	default:
		if chatType = strings.TrimSpace(chatType); chatType != "" {
			return chatType
		}
		return "未知"
	}
}

func feishuMessageTypeLabel(messageType string) string {
	switch strings.TrimSpace(messageType) {
	case "text":
		return "文本"
	case "image":
		return "图片"
	default:
		if messageType = strings.TrimSpace(messageType); messageType != "" {
			return messageType
		}
		return "未知"
	}
}

func feishuCreateTimeMillis(value string) int64 {
	timestamp, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return timestamp
}

func (impl *serviceImpl) cacheInboundImage(ctx context.Context, envelope inboundEnvelope, messageID string, imageKey string) (*connectordomain.MediaReference, error) {
	raw, filename, err := impl.feishu.DownloadMessageImage(ctx, envelope.AppID, envelope.AppSecret, messageID, imageKey)
	if err != nil {
		return nil, err
	}
	if len(raw) > maximumImageSize {
		return nil, fmt.Errorf("飞书入站图片超过 10MB")
	}
	if filename == "" {
		filename = "feishu-image"
	}
	ref := "media_" + randomToken(24)
	path := filepath.Join(impl.storage.MediaCacheDir(), ref)
	if err = os.WriteFile(path, raw, 0o600); err != nil {
		return nil, err
	}
	media := &connectordomain.MediaReference{Ref: ref, Direction: "inbound", ConnectorChannelID: envelope.ChannelID, AppID: envelope.AppID, LocalPath: path, Filename: filename, ContentType: http.DetectContentType(raw), ByteSize: int64(len(raw)), ExpiresAt: time.Now().Add(referenceTTL).UnixMilli()}
	impl.mu.Lock()
	impl.media[ref] = media
	impl.mu.Unlock()
	return media, nil
}

// UploadMedia 上传图片到飞书并创建 media_ref。
func (impl *serviceImpl) UploadMedia(ctx context.Context, input UploadMediaInput) (*UploadMediaResult, error) {
	app := impl.appByChannel(input.ConnectorChannelID)
	if app == nil {
		return nil, fmt.Errorf("connector channel not found")
	}
	if input.Source == nil {
		return nil, fmt.Errorf("file required")
	}
	if input.Size > maximumImageSize {
		return nil, fmt.Errorf("image exceeds 10MB")
	}
	raw, err := io.ReadAll(io.LimitReader(input.Source, maximumImageSize+1))
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("image empty")
	}
	if len(raw) > maximumImageSize {
		return nil, fmt.Errorf("image exceeds 10MB")
	}
	contentType := http.DetectContentType(raw)
	if !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("only image uploads are supported")
	}
	imageKey, err := impl.feishu.UploadImage(ctx, app.AppID, app.AppSecret, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	media := &connectordomain.MediaReference{Ref: "media_" + randomToken(24), Direction: "outbound", ConnectorChannelID: app.ConnectorChannelID, AppID: app.AppID, ImageKey: imageKey, Filename: input.Filename, ContentType: contentType, ByteSize: int64(len(raw)), ExpiresAt: time.Now().Add(referenceTTL).UnixMilli()}
	impl.mu.Lock()
	impl.media[media.Ref] = media
	impl.mu.Unlock()
	impl.persistReferences()
	return &UploadMediaResult{MediaRef: media.Ref, MediaType: "image", Filename: media.Filename, ByteSize: media.ByteSize, ExpiresAt: media.ExpiresAt}, nil
}

// OpenMedia 打开已缓存的入站图片。
func (impl *serviceImpl) OpenMedia(_ context.Context, mediaRef string) (*OpenMediaResult, error) {
	impl.mu.Lock()
	media := impl.media[strings.TrimSpace(mediaRef)]
	impl.mu.Unlock()
	if media == nil || media.Direction != "inbound" {
		return nil, fmt.Errorf("media_ref not found")
	}
	if time.Now().UnixMilli() > media.ExpiresAt {
		return nil, fmt.Errorf("media_ref expired")
	}
	raw, err := os.ReadFile(media.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("media_ref not found: %w", err)
	}
	return &OpenMediaResult{ContentType: media.ContentType, Filename: media.Filename, Body: raw}, nil
}

// InvokeTool 执行飞书发送或回复工具。
func (impl *serviceImpl) InvokeTool(ctx context.Context, input ToolInvokeInput) (*ToolInvokeResult, error) {
	app := impl.appByChannel(input.ConnectorChannelID)
	if app == nil {
		return nil, fmt.Errorf("connection not found")
	}
	result := map[string]any{}
	switch input.ToolID {
	case protocol.FeishuMessageSendToolID:
		if strings.TrimSpace(app.DefaultChatID) == "" {
			return nil, fmt.Errorf("飞书默认单聊尚未绑定，请先在飞书中向 xAgent助手发送一条消息")
		}
		text := stringArgument(input.Arguments, "text")
		if text == "" {
			return nil, fmt.Errorf("text required")
		}
		messageID, err := impl.feishu.SendText(ctx, app.AppID, app.AppSecret, app.DefaultChatID, text)
		if err != nil {
			return nil, err
		}
		result["message_id"] = messageID
		result["message_type"] = "text"
	case protocol.FeishuMessageSendImageToolID:
		if strings.TrimSpace(app.DefaultChatID) == "" {
			return nil, fmt.Errorf("飞书默认单聊尚未绑定，请先在飞书中向 xAgent助手发送一条消息")
		}
		media, err := impl.outboundMedia(input.ConnectorChannelID, app.AppID, stringArgument(input.Arguments, "media_ref"))
		if err != nil {
			return nil, err
		}
		messageID, err := impl.feishu.SendImage(ctx, app.AppID, app.AppSecret, app.DefaultChatID, media.ImageKey)
		if err != nil {
			return nil, err
		}
		result["message_id"] = messageID
		result["message_type"] = "image"
		if text := stringArgument(input.Arguments, "text"); text != "" {
			textID, sendErr := impl.feishu.SendText(ctx, app.AppID, app.AppSecret, app.DefaultChatID, text)
			if sendErr != nil {
				return nil, sendErr
			}
			result["text_message_id"] = textID
		}
	case protocol.FeishuMessageReplyToolID:
		replyRef, err := impl.replyReference(input.ConnectorChannelID, app.AppID, stringArgument(input.Arguments, "reply_ref"))
		if err != nil {
			return nil, err
		}
		result["reply_ref"] = replyRef.Ref
		text := stringArgument(input.Arguments, "text")
		if text == "" {
			return nil, fmt.Errorf("text required")
		}
		messageID, err := impl.feishu.ReplyText(ctx, app.AppID, app.AppSecret, replyRef.MessageID, text)
		if err != nil {
			return nil, err
		}
		result["message_id"] = messageID
		result["message_type"] = "text"
	case protocol.FeishuMessageReplyImageToolID:
		replyRef, err := impl.replyReference(input.ConnectorChannelID, app.AppID, stringArgument(input.Arguments, "reply_ref"))
		if err != nil {
			return nil, err
		}
		media, err := impl.outboundMedia(input.ConnectorChannelID, app.AppID, stringArgument(input.Arguments, "media_ref"))
		if err != nil {
			return nil, err
		}
		result["reply_ref"] = replyRef.Ref
		messageID, err := impl.feishu.ReplyImage(ctx, app.AppID, app.AppSecret, replyRef.MessageID, media.ImageKey)
		if err != nil {
			return nil, err
		}
		result["message_id"] = messageID
		result["message_type"] = "image"
		if text := stringArgument(input.Arguments, "text"); text != "" {
			textID, replyErr := impl.feishu.ReplyText(ctx, app.AppID, app.AppSecret, replyRef.MessageID, text)
			if replyErr != nil {
				return nil, replyErr
			}
			result["text_message_id"] = textID
		}
	default:
		return nil, fmt.Errorf("unsupported tool: %s", input.ToolID)
	}
	return &ToolInvokeResult{ToolID: input.ToolID, Result: result}, nil
}

func (impl *serviceImpl) replyReference(channelID string, appID string, reference string) (*connectordomain.ReplyReference, error) {
	impl.mu.Lock()
	replyRef := impl.reply[strings.TrimSpace(reference)]
	impl.mu.Unlock()
	if replyRef == nil || time.Now().UnixMilli() > replyRef.ExpiresAt {
		return nil, fmt.Errorf("reply_ref not found or expired")
	}
	if replyRef.ConnectorChannelID != channelID || replyRef.AppID != appID {
		return nil, fmt.Errorf("reply_ref channel mismatch")
	}
	return replyRef, nil
}

func (impl *serviceImpl) outboundMedia(channelID string, appID string, reference string) (*connectordomain.MediaReference, error) {
	impl.mu.Lock()
	media := impl.media[strings.TrimSpace(reference)]
	impl.mu.Unlock()
	if media == nil || media.Direction != "outbound" || time.Now().UnixMilli() > media.ExpiresAt {
		return nil, fmt.Errorf("media_ref not found or expired")
	}
	if media.ConnectorChannelID != channelID || media.AppID != appID {
		return nil, fmt.Errorf("media_ref channel mismatch")
	}
	return media, nil
}

// BuildChannelDescriptor 构建 channel descriptor。
func (impl *serviceImpl) BuildChannelDescriptor(channelID string) *connectorprotocol.ConnectionDescriptor {
	if connection := impl.ConnectionByChannel(channelID); connection != nil {
		return impl.BuildConnectionDescriptor(connection)
	}
	return &connectorprotocol.ConnectionDescriptor{Schema: "xagent.connection/v1", Connection: connectorprotocol.ConnectionDescriptorInfo{ConnectorCardID: protocol.ConnectorCardID, ConnectorChannelID: channelID, TargetType: connectorprotocol.ConnectorTargetTypeIM, Profile: "xagent.im.v1", Status: connectorprotocol.ConnectionStatusCreated}, Target: connectorprotocol.ConnectionTargetDescriptor{Provider: "feishu", Label: "飞书", DisplayName: "尚未连接飞书"}}
}

// BuildConnectionDescriptor 构建已认证 connection descriptor。
func (impl *serviceImpl) BuildConnectionDescriptor(connection *connectordomain.Connection) *connectorprotocol.ConnectionDescriptor {
	if connection == nil {
		return nil
	}
	return &connectorprotocol.ConnectionDescriptor{Schema: "xagent.connection/v1", Connection: connectorprotocol.ConnectionDescriptorInfo{ConnectorCardID: protocol.ConnectorCardID, ConnectorChannelID: connection.Token, TargetType: connectorprotocol.ConnectorTargetTypeIM, Profile: "xagent.im.v1", Status: connectorprotocol.ConnectionStatusConnected}, Target: connectorprotocol.ConnectionTargetDescriptor{Provider: "feishu", Label: "飞书", DisplayName: connection.DisplayName, AccountHint: connection.AccountHint}, Tools: []connectorprotocol.ConnectionToolState{{ToolID: protocol.FeishuMessageSendToolID, Status: connectorprotocol.ConnectionToolStatusAvailable, TargetPermissionState: connectorprotocol.ConnectionTargetPermissionGranted}, {ToolID: protocol.FeishuMessageSendImageToolID, Status: connectorprotocol.ConnectionToolStatusAvailable, TargetPermissionState: connectorprotocol.ConnectionTargetPermissionGranted}, {ToolID: protocol.FeishuMessageReplyToolID, Status: connectorprotocol.ConnectionToolStatusAvailable, TargetPermissionState: connectorprotocol.ConnectionTargetPermissionGranted}, {ToolID: protocol.FeishuMessageReplyImageToolID, Status: connectorprotocol.ConnectionToolStatusAvailable, TargetPermissionState: connectorprotocol.ConnectionTargetPermissionGranted}}}
}

func (impl *serviceImpl) appByChannel(channelID string) *connectordomain.AppBinding {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	app := impl.apps[strings.TrimSpace(channelID)]
	if app == nil {
		return nil
	}
	copy := *app
	return &copy
}
func (impl *serviceImpl) persistReferences() {
	impl.mu.Lock()
	reply := cloneReply(impl.reply)
	media := cloneMedia(impl.media)
	impl.mu.Unlock()
	if err := impl.storage.SaveReferences(reply, media); err != nil {
		log.Debug("保存飞书引用失败", zap.Error(err))
	}
}
func connectionFromApp(app *connectordomain.AppBinding) *connectordomain.Connection {
	return &connectordomain.Connection{Token: app.ConnectorChannelID, AppID: app.AppID, DisplayName: "xAgent助手", AccountHint: mask(app.AppID), CreatedAt: app.CreatedAt}
}
func mask(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:4] + "***" + value[len(value)-4:]
}
func stringArgument(arguments map[string]any, key string) string {
	if arguments == nil {
		return ""
	}
	value, _ := arguments[key].(string)
	return strings.TrimSpace(value)
}
func randomToken(size int) string {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
func cloneApps(source map[string]*connectordomain.AppBinding) map[string]*connectordomain.AppBinding {
	result := map[string]*connectordomain.AppBinding{}
	for key, value := range source {
		if value != nil {
			copy := *value
			result[key] = &copy
		}
	}
	return result
}
func cloneReply(source map[string]*connectordomain.ReplyReference) map[string]*connectordomain.ReplyReference {
	result := map[string]*connectordomain.ReplyReference{}
	for key, value := range source {
		if value != nil {
			copy := *value
			result[key] = &copy
		}
	}
	return result
}
func cloneMedia(source map[string]*connectordomain.MediaReference) map[string]*connectordomain.MediaReference {
	result := map[string]*connectordomain.MediaReference{}
	for key, value := range source {
		if value != nil {
			copy := *value
			result[key] = &copy
		}
	}
	return result
}
func pruneReferences(reply map[string]*connectordomain.ReplyReference, media map[string]*connectordomain.MediaReference, now int64) {
	for key, value := range reply {
		if value == nil || value.ExpiresAt <= now {
			delete(reply, key)
		}
	}
	for key, value := range media {
		if value == nil || value.ExpiresAt <= now {
			if value != nil && value.LocalPath != "" {
				_ = os.Remove(value.LocalPath)
			}
			delete(media, key)
		}
	}
}
