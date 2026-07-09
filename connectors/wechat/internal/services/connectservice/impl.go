package connectservice

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coffeehc/base/log"
	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/ilinkservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/mediaservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/storageservice"
	qrcode "github.com/skip2/go-qrcode"
	"go.uber.org/zap"
)

const (
	authSessionTTL            = 5 * time.Minute
	maxAuthQRCodeRefreshCount = 3
)

const (
	staleTokenErrCode                = -14
	maxConsecutiveGetUpdatesFailures = 3
	getUpdatesFailureBackoff         = 30 * time.Second
	getUpdatesRetryDelay             = 2 * time.Second
)

// weChatLoginMaterial 表示微信后端返回的一次二维码登录材料。
type weChatLoginMaterial struct {
	qrCode         string
	qrText         string
	qrCodeImage    string
	pollingBaseURL string
	localBotTokens []string
}

type serviceImpl struct {
	mu                     sync.Mutex
	apiKey                 string
	longPollTTL            time.Duration
	inboundTTL             time.Duration
	inboundCleanupInterval time.Duration
	inboundMaxPerChannel   int
	storage                storageservice.Service
	wechat                 ilinkservice.Service
	media                  mediaservice.Service
	pusher                 MessagePusher
	authSessions           map[string]*connectordomain.AuthSession
	bots                   map[string]*connectordomain.Bot
	channels               map[string]*connectordomain.ConnectionBinding
	connections            map[string]*connectordomain.Connection
	monitors               map[string]context.CancelFunc
	typingMu               sync.Mutex
	autoTypingStates       map[string]*autoTypingState
	stopCtx                context.Context
	stop                   context.CancelFunc
}

func newService(config Config, storage storageservice.Service, wechat ilinkservice.Service, media mediaservice.Service) Service {
	return &serviceImpl{
		apiKey:                 config.APIKey,
		longPollTTL:            time.Duration(protocol.WeChatLongPollTimeoutMillis) * time.Millisecond,
		inboundTTL:             time.Duration(protocol.InboundCacheTTLMillis) * time.Millisecond,
		inboundCleanupInterval: time.Duration(protocol.InboundCacheCleanupIntervalMillis) * time.Millisecond,
		inboundMaxPerChannel:   protocol.InboundCacheMaxMessagesPerChannel,
		storage:                storage,
		wechat:                 wechat,
		media:                  media,
	}
}

// Start 完成 connector 连接编排服务启动检查。
func (impl *serviceImpl) Start(context.Context) error {
	bots, err := impl.storage.LoadBots()
	if err != nil {
		log.Debug("加载微信 bot 登录态失败",
			zap.String("state_dir", impl.storage.StateDir()),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return err
	}
	connectionBindings, err := impl.storage.LoadConnectionBindings()
	if err != nil {
		log.Debug("加载微信 connection 绑定失败",
			zap.String("state_dir", impl.storage.StateDir()),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return err
	}
	legacyConnections, err := impl.storage.LoadLegacyConnections()
	if err != nil {
		return err
	}
	changed := impl.mergeLegacyConnections(bots, connectionBindings, legacyConnections)
	if impl.pruneDuplicateBotChannels(connectionBindings) {
		changed = true
	}
	if impl.pruneUnboundBots(bots, connectionBindings) {
		changed = true
	}
	if changed {
		if err = impl.storage.SaveBots(bots); err != nil {
			return err
		}
		if err = impl.storage.SaveConnectionBindings(connectionBindings); err != nil {
			return err
		}
	}
	connections := impl.buildConnections(bots, connectionBindings)
	botCount := len(bots)
	connectionBindingCount := len(connectionBindings)
	impl.mu.Lock()
	impl.authSessions = map[string]*connectordomain.AuthSession{}
	impl.bots = bots
	impl.channels = connectionBindings
	impl.connections = connections
	impl.monitors = map[string]context.CancelFunc{}
	impl.stopCtx, impl.stop = context.WithCancel(context.Background())
	for _, connection := range connections {
		impl.startConnectionMonitorLocked(connection)
	}
	impl.mu.Unlock()
	now := time.Now().UnixMilli()
	impl.pruneExpiredPendingInboundMessages(now)
	impl.pruneExpiredMediaReferences(now)
	go impl.cleanupPendingInboundLoop()
	log.Debug("加载微信 connector 登录态完成",
		zap.String("state_dir", impl.storage.StateDir()),
		zap.Int("bot_count", botCount),
		zap.Int("connection_binding_count", connectionBindingCount),
		zap.String("result", "succeeded"),
	)
	return nil
}

// Stop 停止 connector 连接编排服务。
func (impl *serviceImpl) Stop(context.Context) error {
	impl.mu.Lock()
	if impl.stop != nil {
		impl.stop()
	}
	monitors := impl.monitors
	impl.monitors = map[string]context.CancelFunc{}
	impl.mu.Unlock()
	for _, cancel := range monitors {
		if cancel != nil {
			cancel()
		}
	}
	impl.stopAutoTypingTimers()
	return nil
}

// BindMessagePusher 绑定 connector 入站消息推送端口。
func (impl *serviceImpl) BindMessagePusher(pusher MessagePusher) {
	impl.mu.Lock()
	impl.pusher = pusher
	impl.mu.Unlock()
}

// APIKey 返回 connector server 的 system API key。
func (impl *serviceImpl) APIKey() string {
	return impl.apiKey
}

// ConnectorID 返回 Connector Card id。
func (impl *serviceImpl) ConnectorID() string {
	return protocol.ConnectorCardID
}

// StateDir 返回本地登录态目录。
func (impl *serviceImpl) StateDir() string {
	return impl.storage.StateDir()
}

// WeChatAPIBaseURL 返回微信 iLink API endpoint。
func (impl *serviceImpl) WeChatAPIBaseURL() string {
	return impl.wechat.APIBaseURL()
}

// WeChatBotType 返回微信 iLink bot_type。
func (impl *serviceImpl) WeChatBotType() string {
	return impl.wechat.BotType()
}

// StartAuth 创建微信二维码登录认证会话，并异步加载二维码材料。
func (impl *serviceImpl) StartAuth(ctx context.Context, connectorChannelID string, flowID string) (*connectorprotocol.ConnectorAuthStartResult, error) {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" {
		return nil, fmt.Errorf("connector_channel_id required")
	}
	if strings.TrimSpace(flowID) == "" {
		flowID = protocol.WeChatQRLoginFlowID
	}
	if flowID != protocol.WeChatQRLoginFlowID {
		return nil, fmt.Errorf("unknown_flow")
	}
	log.Debug("创建微信二维码认证会话",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("flow_id", flowID),
		zap.String("result", "started"),
	)
	impl.mu.Lock()
	if connection := impl.connections[connectorChannelID]; connection != nil {
		impl.mu.Unlock()
		return &connectorprotocol.ConnectorAuthStartResult{
			FlowID:               flowID,
			Status:               connectorprotocol.ConnectorAuthStartAuthenticated,
			PollIntervalMillis:   protocol.AuthPollIntervalMillis,
			Message:              "微信登录已确认",
			ConnectionDescriptor: impl.BuildConnectionDescriptor(connection),
		}, nil
	}
	impl.mu.Unlock()
	now := time.Now().UnixMilli()
	authSession := &connectordomain.AuthSession{
		ID:                 "auth_" + randomToken(12),
		ConnectorChannelID: connectorChannelID,
		FlowID:             flowID,
		Status:             connectorprotocol.ConnectorAuthStatusPending,
		Message:            "正在获取微信二维码。",
		CreatedAt:          now,
		ExpiresAt:          now + authSessionTTL.Milliseconds(),
	}
	impl.mu.Lock()
	impl.cancelPendingAuthSessionsLocked(connectorChannelID)
	impl.authSessions[authSession.ID] = authSession
	impl.mu.Unlock()
	result := &connectorprotocol.ConnectorAuthStartResult{
		FlowID:             authSession.FlowID,
		AuthSessionID:      authSession.ID,
		Status:             connectorprotocol.ConnectorAuthStartPending,
		ExpiresAt:          authSession.ExpiresAt,
		PollIntervalMillis: authSessionPollIntervalMillis(authSession),
		Message:            authSession.Message,
	}
	go impl.loadWeChatAuthMaterial(context.Background(), authSession.ID, connectorChannelID, "创建微信二维码认证会话失败: ")
	log.Debug("创建微信二维码认证会话完成",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("flow_id", flowID),
		zap.String("auth_session_id", authSession.ID),
		zap.Int64("expires_at", authSession.ExpiresAt),
		zap.String("result", "pending"),
	)
	return result, nil
}

// AuthStatus 刷新并返回微信二维码登录认证状态。
func (impl *serviceImpl) AuthStatus(ctx context.Context, connectorChannelID string, authSessionID string, refresh bool) (*connectorprotocol.ConnectorAuthStatusResult, bool) {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	authSessionID = strings.TrimSpace(authSessionID)
	if authSessionID == "" {
		if connectorChannelID == "" {
			return nil, false
		}
		impl.mu.Lock()
		connection := impl.connections[connectorChannelID]
		connectionDescriptor := impl.BuildConnectionDescriptor(connection)
		impl.mu.Unlock()
		result := &connectorprotocol.ConnectorAuthStatusResult{
			ConnectorChannelID: connectorChannelID,
			Status:             connectorprotocol.ConnectorAuthStatusUnauthenticated,
			Message:            "微信尚未认证",
			PollIntervalMillis: protocol.AuthPollIntervalMillis,
		}
		if connectionDescriptor != nil {
			result.Status = connectorprotocol.ConnectorAuthStatusAuthenticated
			result.Message = "微信登录已确认"
			result.ConnectionDescriptor = connectionDescriptor
		}
		return result, true
	}
	if refresh {
		impl.refreshWeChatAuthMaterial(ctx, authSessionID, connectorChannelID, true)
	} else if !impl.shouldReturnQRCodeBeforePolling(authSessionID, connectorChannelID) {
		impl.refreshRealWeChatAuthStatus(ctx, authSessionID)
		impl.refreshRequiredWeChatAuthMaterial(ctx, authSessionID, connectorChannelID)
	}
	impl.mu.Lock()
	authSession := impl.authSessions[authSessionID]
	if authSession != nil && strings.TrimSpace(authSession.ConnectorChannelID) != connectorChannelID {
		impl.mu.Unlock()
		log.Debug("查询微信二维码认证状态失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("auth_session_id", authSessionID),
			zap.String("result", "channel_mismatch"),
		)
		return nil, false
	}
	if authSession != nil && time.Now().UnixMilli() > authSession.ExpiresAt && authSession.Status != connectorprotocol.ConnectorAuthStatusAuthenticated {
		previousStatus := authSession.Status
		authSession.Status = connectorprotocol.ConnectorAuthStatusExpired
		authSession.Message = "认证会话已过期，请重新发起认证。"
		if previousStatus != connectorprotocol.ConnectorAuthStatusExpired {
			log.Debug("微信二维码认证会话已过期",
				zap.String("connector_channel_id", authSession.ConnectorChannelID),
				zap.String("auth_session_id", authSession.ID),
				zap.String("auth_status", string(authSession.Status)),
				zap.String("result", "expired"),
			)
		}
	}
	var connection *connectordomain.Connection
	if authSession != nil && authSession.Token != "" {
		connection = impl.connections[authSession.Token]
	}
	if authSession == nil {
		impl.mu.Unlock()
		log.Debug("查询微信二维码认证状态失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("auth_session_id", authSessionID),
			zap.String("result", "auth_session_not_found"),
		)
		return nil, false
	}
	result := &connectorprotocol.ConnectorAuthStatusResult{
		FlowID:             authSession.FlowID,
		AuthSessionID:      authSession.ID,
		Status:             authSession.Status,
		Message:            statusMessage(authSession),
		QRCodeText:         authSession.QRText,
		QRCodeImage:        authSession.QRCodeImage,
		ExpiresAt:          authSession.ExpiresAt,
		PollIntervalMillis: authSessionPollIntervalMillis(authSession),
	}
	if authSession.Status == connectorprotocol.ConnectorAuthStatusAuthenticated {
		result.Message = "微信登录已确认"
		result.ConnectionDescriptor = impl.BuildConnectionDescriptor(connection)
	}
	if shouldMarkQRCodeDelivered(authSession) {
		authSession.QRCodeDelivered = true
	}
	impl.mu.Unlock()
	return result, true
}

// CancelAuth 取消未完成的微信二维码登录认证会话；已经完成认证时安全忽略。
func (impl *serviceImpl) CancelAuth(_ context.Context, connectorChannelID string, authSessionID string) *connectorprotocol.ConnectorAuthCancelResult {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	authSessionID = strings.TrimSpace(authSessionID)
	result := &connectorprotocol.ConnectorAuthCancelResult{
		ConnectorChannelID: connectorChannelID,
		AuthSessionID:      authSessionID,
		Status:             connectorprotocol.ConnectorAuthCancelStatusNotFound,
		AuthStatus:         connectorprotocol.ConnectorAuthStatusUnauthenticated,
		Message:            "未找到需要取消的认证会话",
	}
	if connectorChannelID == "" {
		result.Message = "connector_channel_id required"
		return result
	}
	impl.mu.Lock()
	if connection := impl.connections[connectorChannelID]; connection != nil {
		result.Status = connectorprotocol.ConnectorAuthCancelStatusIgnored
		result.AuthStatus = connectorprotocol.ConnectorAuthStatusAuthenticated
		result.Message = "认证已经完成，取消请求已忽略"
		result.ConnectionDescriptor = impl.BuildConnectionDescriptor(connection)
		impl.mu.Unlock()
		return result
	}
	if authSessionID == "" {
		result.Message = "auth_session_id required"
		impl.mu.Unlock()
		return result
	}
	authSession := impl.authSessions[authSessionID]
	impl.cancelAuthSessionLocked(result, authSession)
	impl.mu.Unlock()
	return result
}

func (impl *serviceImpl) cancelAuthSessionLocked(result *connectorprotocol.ConnectorAuthCancelResult, authSession *connectordomain.AuthSession) {
	if result == nil || authSession == nil {
		return
	}
	result.AuthSessionID = authSession.ID
	if authSession.Status == connectorprotocol.ConnectorAuthStatusAuthenticated {
		result.Status = connectorprotocol.ConnectorAuthCancelStatusIgnored
		result.AuthStatus = connectorprotocol.ConnectorAuthStatusAuthenticated
		result.Message = "认证已经完成，取消请求已忽略"
		if connection := impl.connections[authSession.Token]; connection != nil {
			result.ConnectionDescriptor = impl.BuildConnectionDescriptor(connection)
		}
		return
	}
	authSession.Cancelled = true
	authSession.Status = connectorprotocol.ConnectorAuthStatusFailed
	authSession.Message = "认证已取消"
	delete(impl.authSessions, authSession.ID)
	result.Status = connectorprotocol.ConnectorAuthCancelStatusCanceled
	result.AuthStatus = connectorprotocol.ConnectorAuthStatusFailed
	result.Message = "认证已取消"
}

func (impl *serviceImpl) cancelPendingAuthSessionsLocked(connectorChannelID string) {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" || impl.connections[connectorChannelID] != nil {
		return
	}
	for sessionID, authSession := range impl.authSessions {
		if authSession == nil || strings.TrimSpace(authSession.ConnectorChannelID) != connectorChannelID || authSession.Status == connectorprotocol.ConnectorAuthStatusAuthenticated {
			continue
		}
		authSession.Cancelled = true
		authSession.Status = connectorprotocol.ConnectorAuthStatusFailed
		authSession.Message = "认证已取消"
		delete(impl.authSessions, sessionID)
	}
}

// ConnectionByChannel 根据 connector_channel_id 返回 connection。
func (impl *serviceImpl) ConnectionByChannel(connectorChannelID string) *connectordomain.Connection {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	return impl.connections[strings.TrimSpace(connectorChannelID)]
}

// LogoutByChannel 根据 connector_channel_id 清理 connector 内登录态。
func (impl *serviceImpl) LogoutByChannel(connectorChannelID string) bool {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" {
		return false
	}
	impl.mu.Lock()
	connection := impl.connections[connectorChannelID]
	if connection == nil {
		impl.mu.Unlock()
		log.Debug("微信 connector 登出跳过",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("result", "connection_not_found"),
		)
		return false
	}
	delete(impl.connections, connectorChannelID)
	delete(impl.channels, connectorChannelID)
	impl.stopConnectionMonitorLocked(connectorChannelID)
	impl.pruneUnboundBots(impl.bots, impl.channels)
	err := impl.saveConnectorIdentityStateLocked()
	impl.mu.Unlock()
	impl.clearAutoTypingStatesForChannel(connectorChannelID)
	if err != nil {
		log.Debug("保存微信 connector 登出状态失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return false
	}
	log.Debug("微信 connector 登出状态已保存",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("result", "succeeded"),
	)
	return true
}

// BuildChannelDescriptor 构建指定 channel 的当前 Connection Descriptor。
func (impl *serviceImpl) BuildChannelDescriptor(connectorChannelID string) *connectorprotocol.ConnectionDescriptor {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	connection := impl.ConnectionByChannel(connectorChannelID)
	if connection != nil {
		return impl.BuildConnectionDescriptor(connection)
	}
	return impl.buildBaseChannelDescriptor(connectorChannelID, connectorprotocol.ConnectionStatusCreated)
}

func (impl *serviceImpl) buildBaseChannelDescriptor(connectorChannelID string, status connectorprotocol.ConnectionStatus) *connectorprotocol.ConnectionDescriptor {
	return &connectorprotocol.ConnectionDescriptor{
		Schema: "xagent.connection/v1",
		Connection: connectorprotocol.ConnectionDescriptorInfo{
			ConnectorCardID:    protocol.ConnectorCardID,
			ConnectorChannelID: connectorChannelID,
			TargetType:         connectorprotocol.ConnectorTargetTypeIM,
			Profile:            "xagent.im.v1",
			Status:             status,
		},
		Target: connectorprotocol.ConnectionTargetDescriptor{
			Provider:    "wechat",
			Label:       "微信",
			DisplayName: "未绑定微信",
		},
	}
}

// BuildConnectionDescriptor 构建用户绑定后的 Connection Descriptor。
func (impl *serviceImpl) BuildConnectionDescriptor(connection *connectordomain.Connection) *connectorprotocol.ConnectionDescriptor {
	if connection == nil {
		return nil
	}
	descriptor := impl.buildBaseChannelDescriptor(connection.Token, connectorprotocol.ConnectionStatusConnected)
	descriptor.Target.DisplayName = connection.DisplayName
	descriptor.Target.AccountHint = connection.AccountHint
	descriptor.Tools = []connectorprotocol.ConnectionToolState{
		{ToolID: toolIDWeChatMessageSend, Status: connectorprotocol.ConnectionToolStatusAvailable, TargetPermissionState: connectorprotocol.ConnectionTargetPermissionGranted},
		{ToolID: toolIDWeChatMessageSendMedia, Status: connectorprotocol.ConnectionToolStatusAvailable, TargetPermissionState: connectorprotocol.ConnectionTargetPermissionGranted},
	}
	return descriptor
}

// fetchRealWeChatLoginMaterial 从微信后端获取新的二维码登录材料，但不直接修改认证会话状态。
func (impl *serviceImpl) fetchRealWeChatLoginMaterial(ctx context.Context, connectorChannelID string, authSessionID string) (*weChatLoginMaterial, error) {
	localBotTokens := impl.localBotTokenList()
	log.Debug("请求微信二维码",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("auth_session_id", authSessionID),
		zap.Int("local_token_count", len(localBotTokens)),
		zap.String("result", "started"),
	)
	qrResponse, err := impl.wechat.FetchQRCode(ctx, localBotTokens)
	if err != nil {
		log.Debug("请求微信二维码失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("auth_session_id", authSessionID),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return nil, err
	}
	if strings.TrimSpace(qrResponse.QRCode) == "" || strings.TrimSpace(qrResponse.QRCodeImageContent) == "" {
		log.Debug("请求微信二维码失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("auth_session_id", authSessionID),
			zap.Bool("has_qrcode", strings.TrimSpace(qrResponse.QRCode) != ""),
			zap.Bool("has_qrcode_image_content", strings.TrimSpace(qrResponse.QRCodeImageContent) != ""),
			zap.String("result", "invalid_response"),
		)
		return nil, fmt.Errorf("微信未返回可用二维码数据")
	}
	log.Debug("请求微信二维码完成",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("auth_session_id", authSessionID),
		zap.Bool("has_qrcode_image", true),
		zap.String("result", "succeeded"),
	)
	qrText := strings.TrimSpace(qrResponse.QRCodeImageContent)
	return &weChatLoginMaterial{
		qrCode:         strings.TrimSpace(qrResponse.QRCode),
		qrText:         qrText,
		qrCodeImage:    buildQRCodeDataURL(qrText),
		pollingBaseURL: impl.wechat.APIBaseURL(),
		localBotTokens: localBotTokens,
	}, nil
}

// applyWeChatLoginMaterial 把已确认可用的二维码材料写入当前认证会话。
func (impl *serviceImpl) applyWeChatLoginMaterial(authSession *connectordomain.AuthSession, material *weChatLoginMaterial) {
	authSession.WeChatQRCode = material.qrCode
	authSession.QRText = material.qrText
	authSession.QRCodeImage = material.qrCodeImage
	authSession.PollingBaseURL = material.pollingBaseURL
	authSession.LocalBotTokens = material.localBotTokens
	authSession.QRCodeRefreshRequired = false
	authSession.QRCodeDelivered = false
	authSession.Message = "请用手机微信扫描二维码，并在手机上确认授权。"
}

func (impl *serviceImpl) shouldReturnQRCodeBeforePolling(sessionID string, connectorChannelID string) bool {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	authSession := impl.authSessions[sessionID]
	if authSession == nil || strings.TrimSpace(authSession.ConnectorChannelID) != connectorChannelID {
		return false
	}
	if authSession.QRCodeDelivered || strings.TrimSpace(authSession.QRCodeImage) == "" || strings.TrimSpace(authSession.WeChatQRCode) == "" {
		return false
	}
	if time.Now().UnixMilli() > authSession.ExpiresAt {
		return false
	}
	switch authSession.Status {
	case connectorprotocol.ConnectorAuthStatusAuthenticated, connectorprotocol.ConnectorAuthStatusExpired, connectorprotocol.ConnectorAuthStatusFailed:
		return false
	default:
		return true
	}
}

func shouldMarkQRCodeDelivered(authSession *connectordomain.AuthSession) bool {
	if authSession == nil || strings.TrimSpace(authSession.QRCodeImage) == "" || strings.TrimSpace(authSession.WeChatQRCode) == "" {
		return false
	}
	switch authSession.Status {
	case connectorprotocol.ConnectorAuthStatusPending, connectorprotocol.ConnectorAuthStatusScanned:
		return true
	default:
		return false
	}
}

func authSessionPollIntervalMillis(authSession *connectordomain.AuthSession) int64 {
	if authSession == nil {
		return protocol.AuthPollIntervalMillis
	}
	if strings.TrimSpace(authSession.QRCodeImage) == "" || strings.TrimSpace(authSession.WeChatQRCode) == "" {
		return protocol.AuthMaterialPollIntervalMillis
	}
	return protocol.AuthPollIntervalMillis
}

// loadWeChatAuthMaterial 为已创建的认证会话异步补齐二维码材料。
func (impl *serviceImpl) loadWeChatAuthMaterial(ctx context.Context, sessionID string, connectorChannelID string, failureMessagePrefix string) {
	impl.mu.Lock()
	authSession := impl.authSessions[sessionID]
	if authSession == nil || strings.TrimSpace(authSession.ConnectorChannelID) != connectorChannelID || authSession.Cancelled || authSession.Status == connectorprotocol.ConnectorAuthStatusAuthenticated {
		impl.mu.Unlock()
		return
	}
	authSessionID := authSession.ID
	authSessionConnectorChannelID := authSession.ConnectorChannelID
	impl.mu.Unlock()
	material, err := impl.fetchRealWeChatLoginMaterial(ctx, authSessionConnectorChannelID, authSessionID)
	if err != nil {
		impl.mu.Lock()
		current := impl.authSessions[sessionID]
		if current != nil && strings.TrimSpace(current.ConnectorChannelID) == connectorChannelID && !current.Cancelled && current.Status != connectorprotocol.ConnectorAuthStatusAuthenticated {
			current.Status = connectorprotocol.ConnectorAuthStatusFailed
			current.Message = failureMessagePrefix + err.Error()
		}
		impl.mu.Unlock()
		log.Debug("加载微信二维码认证材料失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("auth_session_id", sessionID),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return
	}
	impl.mu.Lock()
	authSession = impl.authSessions[sessionID]
	if authSession != nil && strings.TrimSpace(authSession.ConnectorChannelID) == connectorChannelID && !authSession.Cancelled && authSession.Status != connectorprotocol.ConnectorAuthStatusAuthenticated && authSession.Status != connectorprotocol.ConnectorAuthStatusFailed {
		impl.applyWeChatLoginMaterial(authSession, material)
	}
	if authSession == nil {
		impl.mu.Unlock()
		return
	}
	logConnectorChannelID := authSession.ConnectorChannelID
	logAuthSessionID := authSession.ID
	logExpiresAt := authSession.ExpiresAt
	impl.mu.Unlock()
	log.Debug("加载微信二维码认证材料完成",
		zap.String("connector_channel_id", logConnectorChannelID),
		zap.String("auth_session_id", logAuthSessionID),
		zap.Int64("expires_at", logExpiresAt),
		zap.String("result", "pending"),
	)
}

// refreshWeChatAuthMaterial 在同一个 auth_session_id 下刷新二维码材料。
func (impl *serviceImpl) refreshWeChatAuthMaterial(ctx context.Context, sessionID string, connectorChannelID string, resetRefreshCount bool) {
	if sessionID == "" {
		return
	}
	impl.mu.Lock()
	authSession := impl.authSessions[sessionID]
	if authSession == nil || strings.TrimSpace(authSession.ConnectorChannelID) != connectorChannelID || authSession.Status == connectorprotocol.ConnectorAuthStatusAuthenticated {
		impl.mu.Unlock()
		return
	}
	now := time.Now().UnixMilli()
	authSession.Status = connectorprotocol.ConnectorAuthStatusPending
	authSession.Message = "正在刷新微信二维码。"
	authSession.ExpiresAt = now + authSessionTTL.Milliseconds()
	authSession.ConfirmedAt = 0
	authSession.Token = ""
	authSession.QRCodeRefreshRequired = false
	if resetRefreshCount {
		authSession.QRRefreshCount = 0
	}
	impl.mu.Unlock()
	impl.loadWeChatAuthMaterial(ctx, sessionID, connectorChannelID, "刷新微信二维码失败: ")
}

// refreshRequiredWeChatAuthMaterial 在微信状态接口明确要求换码后获取新二维码。
func (impl *serviceImpl) refreshRequiredWeChatAuthMaterial(ctx context.Context, sessionID string, connectorChannelID string) {
	impl.mu.Lock()
	authSession := impl.authSessions[sessionID]
	if authSession == nil || strings.TrimSpace(authSession.ConnectorChannelID) != connectorChannelID || !authSession.QRCodeRefreshRequired {
		impl.mu.Unlock()
		return
	}
	if authSession.QRRefreshCount >= maxAuthQRCodeRefreshCount {
		authSession.Status = connectorprotocol.ConnectorAuthStatusFailed
		authSession.Message = "二维码多次失效，连接流程已停止。请重新发起认证。"
		log.Debug("微信二维码自动刷新次数已耗尽",
			zap.String("connector_channel_id", authSession.ConnectorChannelID),
			zap.String("auth_session_id", authSession.ID),
			zap.Int("qr_refresh_count", authSession.QRRefreshCount),
			zap.String("auth_status", string(authSession.Status)),
			zap.String("result", "stopped"),
		)
		impl.mu.Unlock()
		return
	}
	authSession.QRRefreshCount++
	authSession.QRCodeRefreshRequired = false
	authSession.Status = connectorprotocol.ConnectorAuthStatusPending
	authSession.Message = "正在刷新微信二维码。"
	impl.mu.Unlock()
	impl.refreshWeChatAuthMaterial(ctx, sessionID, connectorChannelID, false)
}

func (impl *serviceImpl) refreshRealWeChatAuthStatus(ctx context.Context, sessionID string) {
	if sessionID == "" {
		return
	}
	impl.mu.Lock()
	authSession := impl.authSessions[sessionID]
	if authSession == nil || authSession.Status == connectorprotocol.ConnectorAuthStatusAuthenticated || authSession.Status == connectorprotocol.ConnectorAuthStatusExpired || authSession.Status == connectorprotocol.ConnectorAuthStatusFailed {
		impl.mu.Unlock()
		return
	}
	if time.Now().UnixMilli() > authSession.ExpiresAt {
		authSession.Status = connectorprotocol.ConnectorAuthStatusExpired
		authSession.Message = "认证会话已过期，请重新发起认证。"
		log.Debug("微信二维码认证会话已过期",
			zap.String("connector_channel_id", authSession.ConnectorChannelID),
			zap.String("auth_session_id", authSession.ID),
			zap.String("auth_status", string(authSession.Status)),
			zap.String("result", "expired"),
		)
		impl.mu.Unlock()
		return
	}
	wechatQRCode := authSession.WeChatQRCode
	if strings.TrimSpace(wechatQRCode) == "" {
		authSession.Message = "正在获取微信二维码。"
		impl.mu.Unlock()
		return
	}
	pollingBaseURL := authSession.PollingBaseURL
	if pollingBaseURL == "" {
		pollingBaseURL = protocol.WeChatAPIBaseURL
	}
	impl.mu.Unlock()

	status, err := impl.wechat.FetchQRCodeStatus(ctx, ilinkservice.QRCodeStatusInput{
		BaseURL: pollingBaseURL,
		QRCode:  wechatQRCode,
	})

	impl.mu.Lock()
	defer impl.mu.Unlock()
	authSession = impl.authSessions[sessionID]
	if authSession == nil || authSession.Status == connectorprotocol.ConnectorAuthStatusAuthenticated || authSession.Status == connectorprotocol.ConnectorAuthStatusExpired || authSession.Status == connectorprotocol.ConnectorAuthStatusFailed {
		return
	}
	if err != nil {
		var httpErr *ilinkservice.HTTPError
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &httpErr) && httpErr.StatusCode >= http.StatusInternalServerError) {
			authSession.Message = "等待扫码确认"
			return
		}
		authSession.Status = connectorprotocol.ConnectorAuthStatusFailed
		authSession.Message = "轮询微信二维码状态失败: " + err.Error()
		log.Debug("刷新微信二维码认证状态失败",
			zap.String("connector_channel_id", authSession.ConnectorChannelID),
			zap.String("auth_session_id", authSession.ID),
			zap.String("auth_status", string(authSession.Status)),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return
	}
	previousStatus := authSession.Status
	previousPollingBaseURL := authSession.PollingBaseURL
	impl.applyWeChatQRCodeStatus(authSession, status)
	if previousStatus != authSession.Status || previousPollingBaseURL != authSession.PollingBaseURL {
		log.Debug("微信二维码认证状态已更新",
			zap.String("connector_channel_id", authSession.ConnectorChannelID),
			zap.String("auth_session_id", authSession.ID),
			zap.String("wechat_status", strings.TrimSpace(status.Status)),
			zap.String("previous_auth_status", string(previousStatus)),
			zap.String("auth_status", string(authSession.Status)),
			zap.Bool("polling_base_url_changed", previousPollingBaseURL != authSession.PollingBaseURL),
			zap.String("result", "updated"),
		)
	}
}

func (impl *serviceImpl) applyWeChatQRCodeStatus(authSession *connectordomain.AuthSession, status *ilinkservice.QRCodeStatusResponse) {
	switch strings.TrimSpace(status.Status) {
	case "wait":
		authSession.Status = connectorprotocol.ConnectorAuthStatusPending
		authSession.Message = "等待扫码确认"
	case "scaned":
		authSession.Status = connectorprotocol.ConnectorAuthStatusScanned
		authSession.Message = "已扫码，等待手机确认。"
	case "scaned_but_redirect":
		authSession.Status = connectorprotocol.ConnectorAuthStatusScanned
		if status.RedirectHost != "" {
			authSession.PollingBaseURL = "https://" + strings.TrimPrefix(status.RedirectHost, "https://")
			authSession.Message = "已扫码，正在切换微信验证节点。"
			return
		}
		authSession.Message = "已扫码，等待手机确认。"
	case "expired":
		authSession.Status = connectorprotocol.ConnectorAuthStatusQRRefreshRequired
		authSession.QRCodeRefreshRequired = true
		authSession.Message = "二维码已过期，正在获取新的二维码。"
	case "need_verifycode":
		authSession.Status = connectorprotocol.ConnectorAuthStatusFailed
		authSession.Message = "微信要求输入手机端数字验证码；当前 xAgent 测试连接器暂不支持该交互，请重新发起认证。"
	case "verify_code_blocked":
		authSession.Status = connectorprotocol.ConnectorAuthStatusFailed
		authSession.Message = "微信验证码校验被阻止，请稍后重试。"
	case "binded_redirect":
		if impl.reuseExistingWeChatConnection(authSession, status) {
			return
		}
		authSession.Status = connectorprotocol.ConnectorAuthStatusFailed
		authSession.Message = "该微信账号已绑定到当前微信后端，但 connector 无法定位本地登录态。"
	case "confirmed":
		impl.confirmRealWeChatAuthSession(authSession, status)
	default:
		authSession.Status = connectorprotocol.ConnectorAuthStatusPending
		authSession.Message = "等待扫码确认"
	}
}

func (impl *serviceImpl) confirmRealWeChatAuthSession(authSession *connectordomain.AuthSession, status *ilinkservice.QRCodeStatusResponse) {
	botToken := strings.TrimSpace(status.BotToken)
	botAccountID := strings.TrimSpace(status.IlinkBotID)
	if botToken == "" || botAccountID == "" {
		authSession.Status = connectorprotocol.ConnectorAuthStatusFailed
		authSession.Message = "微信已确认，但未返回完整 bot token 或账号 ID。"
		log.Debug("确认微信二维码认证失败",
			zap.String("connector_channel_id", authSession.ConnectorChannelID),
			zap.String("auth_session_id", authSession.ID),
			zap.Bool("has_bot_token", botToken != ""),
			zap.Bool("has_bot_account_id", botAccountID != ""),
			zap.String("auth_status", string(authSession.Status)),
			zap.String("result", "invalid_response"),
		)
		return
	}
	now := time.Now().UnixMilli()
	accountHint := maskAccountHint(botAccountID)
	baseURL := strings.TrimSpace(status.BaseURL)
	if baseURL == "" {
		baseURL = authSession.PollingBaseURL
	}
	if baseURL == "" {
		baseURL = protocol.WeChatAPIBaseURL
	}
	bot := &connectordomain.Bot{
		WeChatUserID: strings.TrimSpace(status.IlinkUserID),
		BotToken:     botToken,
		BotAccountID: botAccountID,
		BaseURL:      baseURL,
		DisplayName:  "微信 " + accountHint,
		AccountHint:  accountHint,
		CreatedAt:    now,
	}
	impl.mergeExistingBotRuntimeStateLocked(bot)
	channel := &connectordomain.ConnectionBinding{
		ConnectorChannelID: authSession.ConnectorChannelID,
		BotAccountID:       botAccountID,
		CreatedAt:          now,
	}
	impl.ensureRuntimeStateLocked()
	impl.removeBotChannelsLocked(botAccountID, authSession.ConnectorChannelID)
	impl.bots[bot.BotAccountID] = bot
	impl.channels[channel.ConnectorChannelID] = channel
	connection := connectionFromBotChannel(bot, channel)
	impl.connections[connection.Token] = connection
	if err := impl.saveConnectorIdentityStateLocked(); err != nil {
		delete(impl.connections, connection.Token)
		delete(impl.channels, channel.ConnectorChannelID)
		authSession.Status = connectorprotocol.ConnectorAuthStatusFailed
		authSession.Message = "保存 connector 登录态失败: " + err.Error()
		log.Debug("保存微信 connector 登录态失败",
			zap.String("connector_channel_id", authSession.ConnectorChannelID),
			zap.String("auth_session_id", authSession.ID),
			zap.String("account_hint", accountHint),
			zap.String("auth_status", string(authSession.Status)),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return
	}
	impl.startConnectionMonitorLocked(connection)
	authSession.Status = connectorprotocol.ConnectorAuthStatusAuthenticated
	authSession.Message = "微信登录已确认"
	authSession.ConfirmedAt = now
	authSession.Token = connection.Token
	authSession.QRRefreshCount = 0
	authSession.QRCodeRefreshRequired = false
	log.Debug("微信二维码认证确认完成",
		zap.String("connector_channel_id", authSession.ConnectorChannelID),
		zap.String("auth_session_id", authSession.ID),
		zap.String("account_hint", accountHint),
		zap.String("auth_status", string(authSession.Status)),
		zap.String("result", "authenticated"),
	)
}

func (impl *serviceImpl) reuseExistingWeChatConnection(authSession *connectordomain.AuthSession, status *ilinkservice.QRCodeStatusResponse) bool {
	botAccountID := strings.TrimSpace(status.IlinkBotID)
	userID := strings.TrimSpace(status.IlinkUserID)
	botToken := strings.TrimSpace(status.BotToken)
	for _, bot := range impl.bots {
		if bot == nil {
			continue
		}
		if botAccountID != "" && bot.BotAccountID == botAccountID {
			return impl.bindExistingBotToChannel(authSession, bot)
		}
		if userID != "" && bot.WeChatUserID == userID {
			return impl.bindExistingBotToChannel(authSession, bot)
		}
		if botToken != "" && bot.BotToken == botToken {
			return impl.bindExistingBotToChannel(authSession, bot)
		}
	}
	var candidate *connectordomain.Bot
	candidateBotToken := ""
	tokenCandidates := map[string]struct{}{}
	for _, token := range authSession.LocalBotTokens {
		token = strings.TrimSpace(token)
		if token != "" {
			tokenCandidates[token] = struct{}{}
		}
	}
	if len(tokenCandidates) == 0 {
		return false
	}
	for _, bot := range impl.bots {
		if bot == nil || bot.BotToken == "" {
			continue
		}
		if _, ok := tokenCandidates[bot.BotToken]; !ok {
			continue
		}
		if candidate != nil && candidateBotToken != bot.BotToken {
			authSession.Status = connectorprotocol.ConnectorAuthStatusFailed
			authSession.Message = "该微信账号已绑定到当前微信后端，但 connector 无法唯一定位本地登录态。"
			return true
		}
		candidate = bot
		candidateBotToken = bot.BotToken
	}
	if candidate == nil {
		return false
	}
	return impl.bindExistingBotToChannel(authSession, candidate)
}

func (impl *serviceImpl) bindExistingBotToChannel(authSession *connectordomain.AuthSession, bot *connectordomain.Bot) bool {
	if bot == nil || strings.TrimSpace(bot.BotAccountID) == "" {
		return false
	}
	now := time.Now().UnixMilli()
	channel := &connectordomain.ConnectionBinding{
		ConnectorChannelID: authSession.ConnectorChannelID,
		BotAccountID:       bot.BotAccountID,
		CreatedAt:          now,
	}
	impl.ensureRuntimeStateLocked()
	impl.removeBotChannelsLocked(bot.BotAccountID, authSession.ConnectorChannelID)
	impl.channels[channel.ConnectorChannelID] = channel
	connection := connectionFromBotChannel(bot, channel)
	impl.connections[connection.Token] = connection
	if err := impl.saveConnectorIdentityStateLocked(); err != nil {
		delete(impl.connections, connection.Token)
		delete(impl.channels, channel.ConnectorChannelID)
		authSession.Status = connectorprotocol.ConnectorAuthStatusFailed
		authSession.Message = "保存 connector 登录态失败: " + err.Error()
		log.Debug("复用微信 connector 登录态失败",
			zap.String("connector_channel_id", authSession.ConnectorChannelID),
			zap.String("auth_session_id", authSession.ID),
			zap.String("account_hint", bot.AccountHint),
			zap.String("auth_status", string(authSession.Status)),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return true
	}
	impl.startConnectionMonitorLocked(connection)
	authSession.Status = connectorprotocol.ConnectorAuthStatusAuthenticated
	authSession.Message = "微信账号已绑定，已复用 connector 本地登录态。"
	authSession.ConfirmedAt = now
	authSession.Token = connection.Token
	authSession.QRRefreshCount = 0
	authSession.QRCodeRefreshRequired = false
	log.Debug("已复用微信 connector 登录态",
		zap.String("connector_channel_id", authSession.ConnectorChannelID),
		zap.String("auth_session_id", authSession.ID),
		zap.String("account_hint", bot.AccountHint),
		zap.String("auth_status", string(authSession.Status)),
		zap.String("result", "authenticated"),
	)
	return true
}

func (impl *serviceImpl) localBotTokenList() []string {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	tokens := make([]string, 0, 10)
	seen := map[string]struct{}{}
	for _, bot := range impl.bots {
		if bot == nil || bot.BotToken == "" {
			continue
		}
		if _, ok := seen[bot.BotToken]; ok {
			continue
		}
		tokens = append(tokens, bot.BotToken)
		seen[bot.BotToken] = struct{}{}
		if len(tokens) >= 10 {
			break
		}
	}
	sort.Strings(tokens)
	return tokens
}

func (impl *serviceImpl) mergeLegacyConnections(bots map[string]*connectordomain.Bot, channels map[string]*connectordomain.ConnectionBinding, legacyConnections map[string]*connectordomain.Connection) bool {
	changed := false
	for connectorChannelID, connection := range legacyConnections {
		if connection == nil {
			continue
		}
		if strings.TrimSpace(connection.Token) != "" {
			connectorChannelID = connection.Token
		}
		connectorChannelID = strings.TrimSpace(connectorChannelID)
		if connectorChannelID == "" {
			continue
		}
		bot := botFromConnection(connection)
		if bot == nil {
			continue
		}
		existing := bots[bot.BotAccountID]
		if existing == nil || preferBotCandidate(existing, bot) {
			mergeBotContext(bot, existing)
			bots[bot.BotAccountID] = bot
			changed = true
		} else if mergeBotContext(existing, bot) {
			changed = true
		}
		if channels[connectorChannelID] == nil || channels[connectorChannelID].BotAccountID != bot.BotAccountID {
			channels[connectorChannelID] = &connectordomain.ConnectionBinding{
				ConnectorChannelID: connectorChannelID,
				BotAccountID:       bot.BotAccountID,
				CreatedAt:          connection.CreatedAt,
			}
			changed = true
		}
	}
	return changed
}

func (impl *serviceImpl) pruneDuplicateBotChannels(channels map[string]*connectordomain.ConnectionBinding) bool {
	changed := false
	selected := map[string]*connectordomain.ConnectionBinding{}
	for connectorChannelID, channel := range channels {
		if channel == nil || strings.TrimSpace(connectorChannelID) == "" || strings.TrimSpace(channel.BotAccountID) == "" {
			delete(channels, connectorChannelID)
			changed = true
			continue
		}
		channel.ConnectorChannelID = connectorChannelID
		current := selected[channel.BotAccountID]
		if current == nil || preferChannelCandidate(current, channel) {
			selected[channel.BotAccountID] = channel
		}
	}
	for connectorChannelID, channel := range channels {
		if channel == nil {
			continue
		}
		if selected[channel.BotAccountID] != channel {
			delete(channels, connectorChannelID)
			changed = true
		}
	}
	return changed
}

func (impl *serviceImpl) pruneUnboundBots(bots map[string]*connectordomain.Bot, channels map[string]*connectordomain.ConnectionBinding) bool {
	boundBotIDs := map[string]struct{}{}
	for _, channel := range channels {
		if channel == nil || strings.TrimSpace(channel.BotAccountID) == "" {
			continue
		}
		boundBotIDs[channel.BotAccountID] = struct{}{}
	}
	changed := false
	for botAccountID := range bots {
		if _, ok := boundBotIDs[botAccountID]; ok {
			continue
		}
		delete(bots, botAccountID)
		changed = true
	}
	return changed
}

func (impl *serviceImpl) buildConnections(bots map[string]*connectordomain.Bot, channels map[string]*connectordomain.ConnectionBinding) map[string]*connectordomain.Connection {
	connections := map[string]*connectordomain.Connection{}
	for connectorChannelID, channel := range channels {
		if channel == nil {
			continue
		}
		bot := bots[channel.BotAccountID]
		if bot == nil {
			continue
		}
		channel.ConnectorChannelID = connectorChannelID
		connection := connectionFromBotChannel(bot, channel)
		if connection != nil {
			connections[connection.Token] = connection
		}
	}
	return connections
}

func (impl *serviceImpl) ensureRuntimeStateLocked() {
	if impl.authSessions == nil {
		impl.authSessions = map[string]*connectordomain.AuthSession{}
	}
	if impl.bots == nil {
		impl.bots = map[string]*connectordomain.Bot{}
	}
	if impl.channels == nil {
		impl.channels = map[string]*connectordomain.ConnectionBinding{}
	}
	if impl.connections == nil {
		impl.connections = map[string]*connectordomain.Connection{}
	}
	if impl.monitors == nil {
		impl.monitors = map[string]context.CancelFunc{}
	}
}

func (impl *serviceImpl) removeBotChannelsLocked(botAccountID string, exceptConnectorChannelID string) {
	botAccountID = strings.TrimSpace(botAccountID)
	exceptConnectorChannelID = strings.TrimSpace(exceptConnectorChannelID)
	if botAccountID == "" {
		return
	}
	for connectorChannelID, channel := range impl.channels {
		if channel == nil || channel.BotAccountID != botAccountID || connectorChannelID == exceptConnectorChannelID {
			continue
		}
		delete(impl.channels, connectorChannelID)
		delete(impl.connections, connectorChannelID)
		impl.stopConnectionMonitorLocked(connectorChannelID)
	}
}

func (impl *serviceImpl) mergeExistingBotRuntimeStateLocked(bot *connectordomain.Bot) {
	if bot == nil || strings.TrimSpace(bot.BotAccountID) == "" {
		return
	}
	existing := impl.bots[bot.BotAccountID]
	if existing == nil {
		return
	}
	if bot.ContextTokens == nil {
		bot.ContextTokens = map[string]string{}
	}
	mergeBotContext(bot, existing)
	if bot.GetUpdatesBuf == "" {
		bot.GetUpdatesBuf = existing.GetUpdatesBuf
	}
	if bot.LastInboundAt == 0 {
		bot.LastInboundAt = existing.LastInboundAt
	}
}

func (impl *serviceImpl) saveConnectorIdentityStateLocked() error {
	if err := impl.storage.SaveBots(impl.bots); err != nil {
		return err
	}
	return impl.storage.SaveConnectionBindings(impl.channels)
}

func botFromConnection(connection *connectordomain.Connection) *connectordomain.Bot {
	if connection == nil || strings.TrimSpace(connection.BotAccountID) == "" || strings.TrimSpace(connection.BotToken) == "" {
		return nil
	}
	bot := &connectordomain.Bot{
		WeChatUserID:  strings.TrimSpace(connection.WeChatUserID),
		BotToken:      strings.TrimSpace(connection.BotToken),
		BotAccountID:  strings.TrimSpace(connection.BotAccountID),
		BaseURL:       strings.TrimSpace(connection.BaseURL),
		DisplayName:   strings.TrimSpace(connection.DisplayName),
		AccountHint:   strings.TrimSpace(connection.AccountHint),
		CreatedAt:     connection.CreatedAt,
		GetUpdatesBuf: strings.TrimSpace(connection.GetUpdatesBuf),
		ContextTokens: map[string]string{},
		LastInboundAt: connection.LastInboundAt,
	}
	if bot.BaseURL == "" {
		bot.BaseURL = protocol.WeChatAPIBaseURL
	}
	if bot.AccountHint == "" {
		bot.AccountHint = maskAccountHint(bot.BotAccountID)
	}
	if bot.DisplayName == "" {
		bot.DisplayName = "微信 " + bot.AccountHint
	}
	mergeContextTokens(bot.ContextTokens, connection.ContextTokens)
	return bot
}

func connectionFromBotChannel(bot *connectordomain.Bot, channel *connectordomain.ConnectionBinding) *connectordomain.Connection {
	if bot == nil || channel == nil || strings.TrimSpace(channel.ConnectorChannelID) == "" || strings.TrimSpace(bot.BotAccountID) == "" {
		return nil
	}
	createdAt := channel.CreatedAt
	if createdAt == 0 {
		createdAt = bot.CreatedAt
	}
	connection := &connectordomain.Connection{
		Token:         strings.TrimSpace(channel.ConnectorChannelID),
		WeChatUserID:  bot.WeChatUserID,
		BotToken:      bot.BotToken,
		BotAccountID:  bot.BotAccountID,
		BaseURL:       bot.BaseURL,
		DisplayName:   bot.DisplayName,
		AccountHint:   bot.AccountHint,
		CreatedAt:     createdAt,
		GetUpdatesBuf: bot.GetUpdatesBuf,
		ContextTokens: map[string]string{},
		LastInboundAt: bot.LastInboundAt,
	}
	if connection.BaseURL == "" {
		connection.BaseURL = protocol.WeChatAPIBaseURL
	}
	if connection.AccountHint == "" {
		connection.AccountHint = maskAccountHint(connection.BotAccountID)
	}
	if connection.DisplayName == "" {
		connection.DisplayName = "微信 " + connection.AccountHint
	}
	mergeContextTokens(connection.ContextTokens, bot.ContextTokens)
	return connection
}

func preferBotCandidate(current *connectordomain.Bot, candidate *connectordomain.Bot) bool {
	if current == nil {
		return true
	}
	if candidate == nil {
		return false
	}
	currentUpdatedAt := current.LastInboundAt
	if currentUpdatedAt == 0 {
		currentUpdatedAt = current.CreatedAt
	}
	candidateUpdatedAt := candidate.LastInboundAt
	if candidateUpdatedAt == 0 {
		candidateUpdatedAt = candidate.CreatedAt
	}
	return candidateUpdatedAt > currentUpdatedAt
}

func preferChannelCandidate(current *connectordomain.ConnectionBinding, candidate *connectordomain.ConnectionBinding) bool {
	if current == nil {
		return true
	}
	if candidate == nil {
		return false
	}
	if candidate.CreatedAt != current.CreatedAt {
		return candidate.CreatedAt > current.CreatedAt
	}
	return candidate.ConnectorChannelID > current.ConnectorChannelID
}

func mergeBotContext(target *connectordomain.Bot, source *connectordomain.Bot) bool {
	if target == nil || source == nil {
		return false
	}
	if target.ContextTokens == nil {
		target.ContextTokens = map[string]string{}
	}
	changed := mergeContextTokens(target.ContextTokens, source.ContextTokens)
	if target.WeChatUserID == "" && source.WeChatUserID != "" {
		target.WeChatUserID = source.WeChatUserID
		changed = true
	}
	if target.GetUpdatesBuf == "" && source.GetUpdatesBuf != "" {
		target.GetUpdatesBuf = source.GetUpdatesBuf
		changed = true
	}
	if source.LastInboundAt > target.LastInboundAt {
		target.LastInboundAt = source.LastInboundAt
		if source.GetUpdatesBuf != "" {
			target.GetUpdatesBuf = source.GetUpdatesBuf
		}
		changed = true
	}
	return changed
}

func mergeContextTokens(target map[string]string, source map[string]string) bool {
	if target == nil || source == nil {
		return false
	}
	changed := false
	for key, value := range source {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || target[key] == value {
			continue
		}
		target[key] = value
		changed = true
	}
	return changed
}

func randomToken(byteCount int) string {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}

func buildQRCodeDataURL(value string) string {
	pngBytes, err := qrcode.Encode(value, qrcode.Medium, 256)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
}

func maskAccountHint(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return "wechat_***"
	}
	if len(accountID) <= 8 {
		return accountID + "***"
	}
	return accountID[:4] + "***" + accountID[len(accountID)-4:]
}

func statusMessage(session *connectordomain.AuthSession) string {
	if strings.TrimSpace(session.Message) != "" {
		return session.Message
	}
	switch session.Status {
	case connectorprotocol.ConnectorAuthStatusScanned:
		return "已扫码，等待手机确认。"
	case connectorprotocol.ConnectorAuthStatusAuthenticated:
		return "微信登录已确认"
	case connectorprotocol.ConnectorAuthStatusExpired:
		return "认证会话已过期，请重新发起认证。"
	case connectorprotocol.ConnectorAuthStatusQRRefreshRequired:
		return "二维码已过期，正在获取新的二维码。"
	case connectorprotocol.ConnectorAuthStatusFailed:
		return "微信登录失败，请重新发起认证。"
	default:
		return "等待扫码确认"
	}
}
