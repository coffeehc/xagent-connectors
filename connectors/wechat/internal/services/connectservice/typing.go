package connectservice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/coffeehc/base/log"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/ilinkservice"
	"go.uber.org/zap"
)

const defaultAutoTypingTimeout = 3 * time.Minute

var autoTypingTimeout = defaultAutoTypingTimeout

type autoTypingState struct {
	connection   *connectordomain.Connection
	recipientRef string
	contextToken string
	timer        *time.Timer
}

// startAutoTypingForDeliveredMessage 在入站消息成功交给 xAgent channel 后向微信用户展示输入中状态。
//
// WHY：typing 是 connector 对目标 IM 的即时反馈能力，xAgent 只消费 message.push，不应该理解
// 微信 sendtyping ticket、context_token 或取消时机。
func (impl *serviceImpl) startAutoTypingForDeliveredMessage(ctx context.Context, connectorChannelID string, payload map[string]any) {
	connection := cloneConnection(impl.ConnectionByChannel(connectorChannelID))
	if connection == nil {
		return
	}
	recipientRef := typingPayloadString(payload, "sender_id", "from", "reply_token", "contact_id")
	if recipientRef == "" {
		return
	}
	contextToken := typingPayloadString(payload, "context_token")
	if err := impl.sendWeChatTypingState(ctx, connection, recipientRef, contextToken, ilinkservice.TypingStatusTyping); err != nil {
		log.Debug("启动微信自动输入状态失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("recipient_ref", recipientRef),
			zap.Bool("has_context_token", contextToken != ""),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return
	}
	impl.storeAutoTypingState(connection, recipientRef, contextToken)
	log.Debug("启动微信自动输入状态完成",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("recipient_ref", recipientRef),
		zap.Bool("has_context_token", contextToken != ""),
		zap.Duration("timeout", currentAutoTypingTimeout()),
		zap.String("result", "started"),
	)
}

// cancelAutoTypingBeforeOutbound 在 connector 发回微信消息前取消对应用户的输入中状态。
func (impl *serviceImpl) cancelAutoTypingBeforeOutbound(ctx context.Context, connection *connectordomain.Connection, recipientRef string, contextToken string) {
	recipientRef = strings.TrimSpace(recipientRef)
	if connection == nil || recipientRef == "" {
		return
	}
	state := impl.takeAutoTypingState(connection.Token, recipientRef)
	if state == nil {
		return
	}
	cancelConnection := cloneConnection(connection)
	if cancelConnection == nil {
		cancelConnection = state.connection
	}
	if strings.TrimSpace(contextToken) == "" {
		contextToken = state.contextToken
	}
	if err := impl.sendWeChatTypingState(ctx, cancelConnection, recipientRef, contextToken, ilinkservice.TypingStatusCancel); err != nil {
		log.Debug("取消微信自动输入状态失败",
			zap.String("connector_channel_id", connection.Token),
			zap.String("recipient_ref", recipientRef),
			zap.Bool("has_context_token", strings.TrimSpace(contextToken) != ""),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return
	}
	log.Debug("取消微信自动输入状态完成",
		zap.String("connector_channel_id", connection.Token),
		zap.String("recipient_ref", recipientRef),
		zap.String("result", "cancelled"),
	)
}

func (impl *serviceImpl) sendWeChatTypingState(ctx context.Context, connection *connectordomain.Connection, recipientRef string, contextToken string, status int) error {
	if connection == nil {
		return fmt.Errorf("connection required")
	}
	recipientRef = strings.TrimSpace(recipientRef)
	if recipientRef == "" {
		return fmt.Errorf("recipient_ref required")
	}
	config, err := impl.wechat.GetConfig(ctx, ilinkservice.GetConfigInput{
		BaseURL:      connection.BaseURL,
		BotToken:     connection.BotToken,
		ILinkUserID:  recipientRef,
		ContextToken: strings.TrimSpace(contextToken),
	})
	if err != nil {
		return err
	}
	if config == nil || strings.TrimSpace(config.TypingTicket) == "" {
		return fmt.Errorf("typing_ticket unavailable")
	}
	response, err := impl.wechat.SendTyping(ctx, ilinkservice.SendTypingInput{
		BaseURL:      connection.BaseURL,
		BotToken:     connection.BotToken,
		ILinkUserID:  recipientRef,
		TypingTicket: config.TypingTicket,
		Status:       status,
	})
	if err != nil {
		return err
	}
	if response != nil && response.Ret != 0 {
		return fmt.Errorf("sendtyping ret=%d errmsg=%s", response.Ret, response.ErrMsg)
	}
	return nil
}

func (impl *serviceImpl) storeAutoTypingState(connection *connectordomain.Connection, recipientRef string, contextToken string) {
	connection = cloneConnection(connection)
	recipientRef = strings.TrimSpace(recipientRef)
	if connection == nil || strings.TrimSpace(connection.Token) == "" || recipientRef == "" {
		return
	}
	state := &autoTypingState{
		connection:   connection,
		recipientRef: recipientRef,
		contextToken: strings.TrimSpace(contextToken),
	}
	key := autoTypingStateKey(connection.Token, recipientRef)
	state.timer = time.AfterFunc(currentAutoTypingTimeout(), func() {
		impl.expireAutoTypingState(key, state)
	})
	impl.typingMu.Lock()
	if impl.autoTypingStates == nil {
		impl.autoTypingStates = map[string]*autoTypingState{}
	}
	if previous := impl.autoTypingStates[key]; previous != nil && previous.timer != nil {
		previous.timer.Stop()
	}
	impl.autoTypingStates[key] = state
	impl.typingMu.Unlock()
}

func (impl *serviceImpl) takeAutoTypingState(connectorChannelID string, recipientRef string) *autoTypingState {
	key := autoTypingStateKey(connectorChannelID, recipientRef)
	impl.typingMu.Lock()
	state := impl.autoTypingStates[key]
	if state != nil {
		delete(impl.autoTypingStates, key)
	}
	impl.typingMu.Unlock()
	if state != nil && state.timer != nil {
		state.timer.Stop()
	}
	return state
}

func (impl *serviceImpl) expireAutoTypingState(key string, expected *autoTypingState) {
	impl.typingMu.Lock()
	current := impl.autoTypingStates[key]
	if current != expected {
		impl.typingMu.Unlock()
		return
	}
	delete(impl.autoTypingStates, key)
	impl.typingMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := impl.sendWeChatTypingState(ctx, expected.connection, expected.recipientRef, expected.contextToken, ilinkservice.TypingStatusCancel); err != nil {
		log.Debug("微信自动输入状态超时取消失败",
			zap.String("connector_channel_id", expected.connection.Token),
			zap.String("recipient_ref", expected.recipientRef),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		return
	}
	log.Debug("微信自动输入状态超时取消完成",
		zap.String("connector_channel_id", expected.connection.Token),
		zap.String("recipient_ref", expected.recipientRef),
		zap.String("result", "timeout_cancelled"),
	)
}

func (impl *serviceImpl) stopAutoTypingTimers() {
	impl.typingMu.Lock()
	states := impl.autoTypingStates
	impl.autoTypingStates = nil
	impl.typingMu.Unlock()
	for _, state := range states {
		if state != nil && state.timer != nil {
			state.timer.Stop()
		}
	}
}

func (impl *serviceImpl) clearAutoTypingStatesForChannel(connectorChannelID string) {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" {
		return
	}
	impl.typingMu.Lock()
	for key, state := range impl.autoTypingStates {
		if state == nil || state.connection == nil || strings.TrimSpace(state.connection.Token) != connectorChannelID {
			continue
		}
		if state.timer != nil {
			state.timer.Stop()
		}
		delete(impl.autoTypingStates, key)
	}
	impl.typingMu.Unlock()
}

func currentAutoTypingTimeout() time.Duration {
	if autoTypingTimeout <= 0 {
		return defaultAutoTypingTimeout
	}
	return autoTypingTimeout
}

func autoTypingStateKey(connectorChannelID string, recipientRef string) string {
	return strings.TrimSpace(connectorChannelID) + "\x00" + strings.TrimSpace(recipientRef)
}

func typingPayloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value := payload[key]
		switch typed := value.(type) {
		case string:
			if text := strings.TrimSpace(typed); text != "" {
				return text
			}
		case fmt.Stringer:
			if text := strings.TrimSpace(typed.String()); text != "" {
				return text
			}
		}
	}
	return ""
}
