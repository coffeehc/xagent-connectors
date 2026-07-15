package channelservice

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coffeehc/base/log"
	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/connectservice"
	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type serviceImpl struct {
	mu          sync.Mutex
	connect     connectservice.Service
	connectorID string
	channels    map[string]*channelConnection
	upgrader    websocket.FastHTTPUpgrader
}

type channelConnection struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func newService(connect connectservice.Service) Service {
	return &serviceImpl{
		connect:  connect,
		channels: map[string]*channelConnection{},
		upgrader: websocket.FastHTTPUpgrader{
			Subprotocols: []string{connectorprotocol.DataPlaneSubprotocol},
			CheckOrigin:  func(*fasthttp.RequestCtx) bool { return true },
		},
	}
}

// Start 完成 data plane channel 服务启动检查。
func (impl *serviceImpl) Start(context.Context) error {
	return nil
}

// Stop 停止 data plane channel 服务。
func (impl *serviceImpl) Stop(context.Context) error {
	return nil
}

// PushMessage 将 connector 已收到的目标系统消息推送到 xAgent data plane。
func (impl *serviceImpl) PushMessage(_ context.Context, input MessagePushInput) error {
	connectorChannelID := strings.TrimSpace(input.ConnectorChannelID)
	if connectorChannelID == "" {
		return fmt.Errorf("connector_channel_id 不能为空")
	}
	channel := impl.channelConnection(connectorChannelID)
	if channel == nil {
		return fmt.Errorf("connector channel 未打开: %s", connectorChannelID)
	}
	payload := input.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	return channel.writePacket(&connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		ConnectorChannelID: connectorChannelID,
		Type:               "message.push",
		Time:               time.Now().UnixMilli(),
		Payload:            payload,
	})
}

// PushConnectionDescriptor 将 connector 侧 connection 状态变化推送到 xAgent data plane。
func (impl *serviceImpl) PushConnectionDescriptor(_ context.Context, input DescriptorPushInput) error {
	connectorChannelID := strings.TrimSpace(input.ConnectorChannelID)
	if connectorChannelID == "" {
		return fmt.Errorf("connector_channel_id 不能为空")
	}
	if input.Descriptor == nil {
		return fmt.Errorf("connection_descriptor 不能为空")
	}
	channel := impl.channelConnection(connectorChannelID)
	if channel == nil {
		return fmt.Errorf("connector channel 未打开: %s", connectorChannelID)
	}
	descriptor := *input.Descriptor
	descriptor.Connection.ConnectorChannelID = connectorChannelID
	descriptorPtr := impl.applyConnectorIDToDescriptor(&descriptor)
	return channel.writePacket(&connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		ConnectorChannelID: connectorChannelID,
		Type:               "connection.descriptor.push",
		Time:               time.Now().UnixMilli(),
		Payload: map[string]any{
			"connection_descriptor": descriptorPtr,
		},
	})
}

// HandleDataPlane 处理 WebSocket data plane 连接。
func (impl *serviceImpl) HandleDataPlane(c fiber.Ctx) error {
	return impl.upgrader.Upgrade(c.RequestCtx(), func(conn *websocket.Conn) {
		channelConn := &channelConnection{conn: conn}
		startedAt := time.Now()
		remoteAddr := ""
		if conn.RemoteAddr() != nil {
			remoteAddr = conn.RemoteAddr().String()
		}
		log.Debug("Telegram connector data plane 连接已建立",
			zap.String("remote_addr", remoteAddr),
			zap.String("result", "connected"),
		)
		defer func() {
			impl.removeChannelConnection(channelConn)
			_ = conn.Close()
			log.Debug("Telegram connector data plane 连接已关闭",
				zap.String("remote_addr", remoteAddr),
				zap.Duration("duration", time.Since(startedAt)),
				zap.String("result", "closed"),
			)
		}()
		helloAccepted := false
		for {
			messageType, payload, readErr := conn.ReadMessage()
			if readErr != nil {
				log.Debug("Telegram connector data plane 读取结束",
					zap.String("remote_addr", remoteAddr),
					zap.String("result", "read_closed"),
					zap.Error(readErr),
				)
				return
			}
			if messageType != websocket.TextMessage {
				continue
			}
			packet := &connectorprotocol.WirePacket{}
			if decodeErr := json.Unmarshal(payload, packet); decodeErr != nil {
				_ = writeError(channelConn, "", "", "invalid_packet", decodeErr.Error())
				continue
			}
			if packet.Type == "connector.hello" {
				helloAccepted = impl.handleConnectorHello(channelConn, packet)
				continue
			}
			if !helloAccepted {
				_ = writeError(channelConn, packet.RequestID, packet.ConnectorChannelID, "hello_required", "connector.hello must be accepted before user packets")
				continue
			}
			switch packet.Type {
			case "channel.open":
				impl.handleChannelOpen(channelConn, packet)
			case "auth.start":
				impl.handleAuthStart(channelConn, packet)
			case "auth.cancel":
				impl.handleAuthCancel(channelConn, packet)
			case "auth.status":
				impl.handleAuthStatus(channelConn, packet)
			case "auth.logout":
				impl.handleAuthLogout(channelConn, packet)
			case "connection.descriptor.get":
				impl.handleDescriptorGet(channelConn, packet)
			case "tool.invoke":
				impl.handleToolInvoke(channelConn, packet)
			case "channel.close":
				impl.handleChannelClose(channelConn, packet)
			case "ping":
				_ = writePacket(channelConn, &connectorprotocol.WirePacket{
					Schema:             connectorprotocol.PacketSchema,
					PacketID:           "pkt_" + randomToken(10),
					RequestID:          packet.RequestID,
					ConnectorChannelID: packet.ConnectorChannelID,
					Type:               "pong",
					Time:               time.Now().UnixMilli(),
				})
			default:
				_ = writeError(channelConn, packet.RequestID, packet.ConnectorChannelID, "unsupported_packet", "unsupported packet type: "+packet.Type)
			}
		}
	})
}

func (impl *serviceImpl) handleConnectorHello(conn *channelConnection, packet *connectorprotocol.WirePacket) bool {
	connectorCardID := stringFromAny(packet.Payload["connector_card_id"])
	if connectorCardID == "" || connectorCardID != impl.connect.ConnectorID() {
		_ = writeError(conn, packet.RequestID, "", "connector_card_id_mismatch", "connector_card_id does not match this connector")
		return false
	}
	connectorID := stringFromAny(packet.Payload["connector_id"])
	impl.mu.Lock()
	if impl.connectorID == "" {
		if connectorID != "" {
			impl.connectorID = connectorID
		} else {
			impl.connectorID = "conn_" + sanitizeIDPart(connectorCardID) + "_" + randomToken(8)
		}
	} else if connectorID != "" && connectorID != impl.connectorID {
		previousConnectorID := impl.connectorID
		impl.connectorID = "conn_" + sanitizeIDPart(connectorCardID) + "_" + randomToken(8)
		log.Debug("connector hello 检测到后端身份变化，已重新分配 connector_id",
			zap.String("connector_card_id", connectorCardID),
			zap.String("cached_connector_id", connectorID),
			zap.String("previous_connector_id", previousConnectorID),
			zap.String("assigned_connector_id", impl.connectorID),
		)
	}
	connectorID = impl.connectorID
	impl.mu.Unlock()
	_ = writePacket(conn, &connectorprotocol.WirePacket{
		Schema:    connectorprotocol.PacketSchema,
		PacketID:  "pkt_" + randomToken(10),
		RequestID: packet.RequestID,
		ReplyTo:   packet.PacketID,
		Type:      "connector.hello.ack",
		Time:      time.Now().UnixMilli(),
		Payload: map[string]any{
			"connector_card_id": connectorCardID,
			"connector_id":      connectorID,
		},
	})
	return true
}

func (impl *serviceImpl) handleChannelOpen(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	requestedChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	connectorChannelID := requestedChannelID
	if connectorChannelID == "" || !impl.isKnownChannel(connectorChannelID) {
		connectorChannelID = randomChannelToken(12)
	}
	impl.mu.Lock()
	impl.channels[connectorChannelID] = conn
	impl.mu.Unlock()
	_ = writePacket(conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		RequestID:          packet.RequestID,
		ReplyTo:            packet.PacketID,
		ConnectorChannelID: connectorChannelID,
		Type:               "channel.open.ack",
		Time:               time.Now().UnixMilli(),
		Payload: map[string]any{
			"connector_channel_id":  connectorChannelID,
			"connection_descriptor": impl.buildChannelDescriptor(connectorChannelID),
		},
	})
}

func (impl *serviceImpl) handleAuthStart(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	if !impl.hasChannel(connectorChannelID) {
		_ = writeError(conn, packet.RequestID, connectorChannelID, "channel_not_open", "channel.open must complete before auth.start")
		return
	}
	go impl.completeAuthStart(conn, packet)
}

func (impl *serviceImpl) completeAuthStart(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	request := connectorprotocol.ConnectorAuthStartRequest{}
	if err := decodePacketPayload(packet.Payload, &request); err != nil {
		_ = writeAckError(conn, packet, "auth.start.ack", "invalid_auth_start", err.Error())
		return
	}
	result, err := impl.connect.StartAuth(context.Background(), connectorChannelID, request)
	if err != nil {
		_ = writeAckError(conn, packet, "auth.start.ack", "auth_start_failed", err.Error())
		return
	}
	_ = writePacket(conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		RequestID:          packet.RequestID,
		ReplyTo:            packet.PacketID,
		ConnectorChannelID: connectorChannelID,
		Type:               "auth.start.ack",
		Time:               time.Now().UnixMilli(),
		Payload:            impl.authStartPayload(result),
	})
}

func (impl *serviceImpl) handleAuthCancel(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	if !impl.hasChannel(connectorChannelID) {
		_ = writeAckError(conn, packet, "auth.cancel.ack", "channel_not_open", "channel.open must complete before auth.cancel")
		return
	}
	authSessionID := stringFromAny(packet.Payload["auth_session_id"])
	result := impl.connect.CancelAuth(context.Background(), connectorChannelID, authSessionID)
	_ = writePacket(conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		RequestID:          packet.RequestID,
		ReplyTo:            packet.PacketID,
		ConnectorChannelID: connectorChannelID,
		Type:               "auth.cancel.ack",
		Time:               time.Now().UnixMilli(),
		Payload:            impl.authCancelPayload(result),
	})
}

func (impl *serviceImpl) handleAuthStatus(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	if !impl.hasChannel(connectorChannelID) {
		_ = writeAckError(conn, packet, "auth.status.ack", "channel_not_open", "channel.open must complete before auth.status")
		return
	}
	authSessionID := stringFromAny(packet.Payload["auth_session_id"])
	refresh := boolFromAny(packet.Payload["refresh"])
	result, ok := impl.connect.AuthStatus(context.Background(), connectorChannelID, authSessionID, refresh)
	if !ok {
		_ = writeAckError(conn, packet, "auth.status.ack", "auth_session_not_found", "auth session not found")
		return
	}
	_ = writePacket(conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		RequestID:          packet.RequestID,
		ReplyTo:            packet.PacketID,
		ConnectorChannelID: connectorChannelID,
		Type:               "auth.status.ack",
		Time:               time.Now().UnixMilli(),
		Payload:            impl.authStatusPayload(result),
	})
}

func (impl *serviceImpl) handleDescriptorGet(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	if !impl.hasChannel(connectorChannelID) {
		_ = writeError(conn, packet.RequestID, packet.ConnectorChannelID, "connection_not_found", "channel is not bound to a valid connection")
		return
	}
	_ = writePacket(conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		RequestID:          packet.RequestID,
		ReplyTo:            packet.PacketID,
		ConnectorChannelID: connectorChannelID,
		Type:               "connection.descriptor.get.ack",
		Time:               time.Now().UnixMilli(),
		Payload: map[string]any{
			"connection_descriptor": impl.buildChannelDescriptor(connectorChannelID),
		},
	})
}

func (impl *serviceImpl) handleToolInvoke(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	if !impl.hasChannel(connectorChannelID) || impl.connect.ConnectionByChannel(connectorChannelID) == nil {
		_ = writeAckError(conn, packet, "tool.invoke.ack", "connection_not_authenticated", "channel is not bound to an authenticated Telegram chat")
		return
	}
	toolID := stringFromAny(packet.Payload["tool_id"])
	result, err := impl.connect.InvokeTool(context.Background(), connectservice.ToolInvokeInput{
		ConnectorChannelID: connectorChannelID,
		ToolID:             toolID,
		Arguments:          mapFromAny(packet.Payload["arguments"]),
	})
	if err != nil {
		_ = writeAckError(conn, packet, "tool.invoke.ack", "tool_invoke_failed", err.Error())
		return
	}
	_ = writePacket(conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		RequestID:          packet.RequestID,
		ReplyTo:            packet.PacketID,
		ConnectorChannelID: connectorChannelID,
		Type:               "tool.invoke.ack",
		Time:               time.Now().UnixMilli(),
		Payload: map[string]any{
			"tool_id": result.ToolID,
			"result":  result.Result,
		},
	})
}

func (impl *serviceImpl) handleAuthLogout(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	if !impl.hasChannel(connectorChannelID) {
		_ = writeError(conn, packet.RequestID, packet.ConnectorChannelID, "connection_not_found", "channel is not bound to a valid connection")
		return
	}
	_ = impl.connect.LogoutByChannel(connectorChannelID)
	_ = writePacket(conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		RequestID:          packet.RequestID,
		ReplyTo:            packet.PacketID,
		ConnectorChannelID: connectorChannelID,
		Type:               "auth.logout.ack",
		Time:               time.Now().UnixMilli(),
		Payload: map[string]any{
			"status":                "ok",
			"connection_descriptor": impl.buildChannelDescriptor(connectorChannelID),
		},
	})
}

func (impl *serviceImpl) buildChannelDescriptor(connectorChannelID string) *connectorprotocol.ConnectionDescriptor {
	descriptor := impl.connect.BuildChannelDescriptor(connectorChannelID)
	if descriptor != nil {
		descriptor.Connection.ConnectorChannelID = strings.TrimSpace(connectorChannelID)
	}
	return impl.applyConnectorIDToDescriptor(descriptor)
}

func (impl *serviceImpl) applyConnectorIDToDescriptor(descriptor *connectorprotocol.ConnectionDescriptor) *connectorprotocol.ConnectionDescriptor {
	if descriptor == nil {
		return nil
	}
	impl.mu.Lock()
	connectorID := impl.connectorID
	impl.mu.Unlock()
	descriptor.Connection.ConnectorID = connectorID
	return descriptor
}

func (impl *serviceImpl) handleChannelClose(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	impl.mu.Lock()
	delete(impl.channels, connectorChannelID)
	impl.mu.Unlock()
	_ = writePacket(conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		RequestID:          packet.RequestID,
		ReplyTo:            packet.PacketID,
		ConnectorChannelID: packet.ConnectorChannelID,
		Type:               "channel.close.ack",
		Time:               time.Now().UnixMilli(),
		Payload:            map[string]any{"status": "ok"},
	})
}

func (impl *serviceImpl) hasChannel(connectorChannelID string) bool {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	_, ok := impl.channels[strings.TrimSpace(connectorChannelID)]
	return ok
}

func (impl *serviceImpl) isKnownChannel(connectorChannelID string) bool {
	connectorChannelID = strings.TrimSpace(connectorChannelID)
	if connectorChannelID == "" {
		return false
	}
	if impl.connect.ConnectionByChannel(connectorChannelID) != nil {
		return true
	}
	return impl.hasChannel(connectorChannelID)
}

func (impl *serviceImpl) channelConnection(connectorChannelID string) *channelConnection {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	return impl.channels[strings.TrimSpace(connectorChannelID)]
}

func (impl *serviceImpl) removeChannelConnection(conn *channelConnection) {
	if conn == nil {
		return
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	for connectorChannelID, channel := range impl.channels {
		if channel == conn {
			delete(impl.channels, connectorChannelID)
		}
	}
}

func (conn *channelConnection) writePacket(packet *connectorprotocol.WirePacket) error {
	if conn == nil || conn.conn == nil {
		return fmt.Errorf("connector channel 未连接")
	}
	payload, err := json.Marshal(packet)
	if err != nil {
		return err
	}
	conn.writeMu.Lock()
	defer conn.writeMu.Unlock()
	return conn.conn.WriteMessage(websocket.TextMessage, payload)
}

func writePacket(conn *channelConnection, packet *connectorprotocol.WirePacket) error {
	return conn.writePacket(packet)
}

func writeError(conn *channelConnection, requestID string, connectorChannelID string, code string, message string) error {
	return writePacket(conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		RequestID:          requestID,
		ConnectorChannelID: connectorChannelID,
		Type:               "error",
		Time:               time.Now().UnixMilli(),
		Error: &connectorprotocol.WireError{
			Code:    code,
			Message: message,
		},
	})
}

func writeAckError(conn *channelConnection, packet *connectorprotocol.WirePacket, ackType string, code string, message string) error {
	return writePacket(conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		RequestID:          packet.RequestID,
		ReplyTo:            packet.PacketID,
		ConnectorChannelID: packet.ConnectorChannelID,
		Type:               ackType,
		Time:               time.Now().UnixMilli(),
		Error: &connectorprotocol.WireError{
			Code:    code,
			Message: message,
		},
	})
}

func (impl *serviceImpl) authStartPayload(result *connectorprotocol.ConnectorAuthStartResult) map[string]any {
	payload := map[string]any{
		"connector_channel_id": result.ConnectorChannelID,
		"flow_id":              result.FlowID,
		"auth_session_id":      result.AuthSessionID,
		"status":               result.Status,
		"message":              result.Message,
	}
	if result.ConnectionDescriptor != nil {
		payload["connection_descriptor"] = impl.applyConnectorIDToDescriptor(result.ConnectionDescriptor)
	}
	return payload
}

func (impl *serviceImpl) authStatusPayload(result *connectorprotocol.ConnectorAuthStatusResult) map[string]any {
	payload := map[string]any{
		"connector_channel_id": result.ConnectorChannelID,
		"flow_id":              result.FlowID,
		"auth_session_id":      result.AuthSessionID,
		"status":               result.Status,
		"message":              result.Message,
	}
	if result.ConnectionDescriptor != nil {
		payload["connection_descriptor"] = impl.applyConnectorIDToDescriptor(result.ConnectionDescriptor)
	}
	if result.PollIntervalMillis > 0 {
		payload["poll_interval_millis"] = result.PollIntervalMillis
	}
	return payload
}

func (impl *serviceImpl) authCancelPayload(result *connectorprotocol.ConnectorAuthCancelResult) map[string]any {
	payload := map[string]any{
		"connector_channel_id": result.ConnectorChannelID,
		"auth_session_id":      result.AuthSessionID,
		"status":               result.Status,
		"auth_status":          result.AuthStatus,
		"message":              result.Message,
	}
	if result.ConnectionDescriptor != nil {
		payload["connection_descriptor"] = impl.applyConnectorIDToDescriptor(result.ConnectionDescriptor)
	}
	return payload
}

func decodePacketPayload(payload map[string]any, target any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func mapFromAny(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func randomChannelToken(byteCount int) string {
	return "cch_" + randomToken(byteCount)
}

func randomToken(byteCount int) string {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}

func sanitizeIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '.', char == '-', char == '_':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	result := builder.String()
	if result == "" {
		return "unknown"
	}
	return result
}
