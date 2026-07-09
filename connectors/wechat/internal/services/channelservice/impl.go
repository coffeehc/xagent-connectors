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
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/connectservice"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
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
		log.Debug("connector data plane 连接已建立",
			zap.String("remote_addr", remoteAddr),
			zap.String("result", "connected"),
		)
		defer func() {
			impl.removeChannelConnection(channelConn)
			_ = conn.Close()
			log.Debug("connector data plane 连接已关闭",
				zap.String("remote_addr", remoteAddr),
				zap.Duration("duration", time.Since(startedAt)),
				zap.String("result", "closed"),
			)
		}()
		helloAccepted := false
		for {
			messageType, payload, readErr := conn.ReadMessage()
			if readErr != nil {
				log.Debug("connector data plane 读取结束",
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
				log.Debug("解析 connector data plane packet 失败",
					zap.String("remote_addr", remoteAddr),
					zap.String("result", "invalid_packet"),
					zap.Error(decodeErr),
				)
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
		log.Debug("拒绝 connector hello",
			zap.String("connector_card_id", connectorCardID),
			zap.String("expected_connector_card_id", impl.connect.ConnectorID()),
			zap.String("result", "connector_card_id_mismatch"),
		)
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
	}
	if connectorID != "" && connectorID != impl.connectorID {
		impl.mu.Unlock()
		log.Debug("拒绝 connector hello",
			zap.String("connector_card_id", connectorCardID),
			zap.String("connector_id", connectorID),
			zap.String("expected_connector_id", impl.connectorID),
			zap.String("result", "connector_id_mismatch"),
		)
		_ = writeError(conn, packet.RequestID, "", "connector_id_mismatch", "connector_id does not match this connector")
		return false
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
	log.Debug("connector hello 已接受",
		zap.String("connector_card_id", connectorCardID),
		zap.String("connector_id", connectorID),
		zap.String("result", "accepted"),
	)
	return true
}

func (impl *serviceImpl) handleChannelOpen(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	requestedChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	connectorChannelID := requestedChannelID
	reassigned := false
	if connectorChannelID == "" || !impl.isKnownChannel(connectorChannelID) {
		connectorChannelID = randomChannelToken(12)
		reassigned = requestedChannelID != ""
	}
	impl.mu.Lock()
	impl.channels[connectorChannelID] = conn
	impl.mu.Unlock()
	log.Debug("connector channel 已打开",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("requested_connector_channel_id", requestedChannelID),
		zap.Bool("reassigned", reassigned),
		zap.String("result", "opened"),
	)
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
	go impl.connect.FlushInboundMessages(context.Background(), connectorChannelID)
}

func (impl *serviceImpl) handleAuthStart(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	if !impl.hasChannel(connectorChannelID) {
		log.Debug("拒绝 connector 认证启动请求",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("result", "channel_not_open"),
		)
		_ = writeError(conn, packet.RequestID, connectorChannelID, "channel_not_open", "channel.open must complete before auth.start")
		return
	}
	go impl.completeAuthStart(conn, packet)
}

func (impl *serviceImpl) completeAuthStart(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	flowID := stringFromAny(packet.Payload["flow_id"])
	log.Debug("收到 connector 认证启动请求",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("flow_id", flowID),
		zap.String("result", "started"),
	)
	result, err := impl.connect.StartAuth(context.Background(), connectorChannelID, flowID)
	if err != nil {
		log.Debug("connector 认证启动失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("flow_id", flowID),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		_ = writeAckError(conn, packet, "auth.start.ack", "auth_start_failed", err.Error())
		return
	}
	log.Debug("connector 认证启动完成",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("auth_session_id", result.AuthSessionID),
		zap.String("auth_status", string(result.Status)),
		zap.String("result", "pending"),
	)
	_ = writePacket(conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		RequestID:          packet.RequestID,
		ReplyTo:            packet.PacketID,
		ConnectorChannelID: connectorChannelID,
		Type:               "auth.start.ack",
		Time:               time.Now().UnixMilli(),
		Payload:            impl.authStatusPayload(result),
	})
}

func (impl *serviceImpl) handleAuthCancel(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	if !impl.hasChannel(connectorChannelID) {
		log.Debug("拒绝 connector 认证取消请求",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("result", "channel_not_open"),
		)
		_ = writeAckError(conn, packet, "auth.cancel.ack", "channel_not_open", "channel.open must complete before auth.cancel")
		return
	}
	authSessionID := stringFromAny(packet.Payload["auth_session_id"])
	result := impl.connect.CancelAuth(context.Background(), connectorChannelID, authSessionID)
	log.Debug("connector 认证取消请求已处理",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("auth_session_id", authSessionID),
		zap.String("cancel_status", string(result.Status)),
		zap.String("auth_status", string(result.AuthStatus)),
		zap.String("result", "handled"),
	)
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
		log.Debug("查询 connector 认证状态失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("result", "channel_not_open"),
		)
		_ = writeAckError(conn, packet, "auth.status.ack", "channel_not_open", "channel.open must complete before auth.status")
		return
	}
	go impl.completeAuthStatus(conn, packet)
}

func (impl *serviceImpl) completeAuthStatus(conn *channelConnection, packet *connectorprotocol.WirePacket) {
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	authSessionID := stringFromAny(packet.Payload["auth_session_id"])
	refresh := boolFromAny(packet.Payload["refresh"])
	result, ok := impl.connect.AuthStatus(context.Background(), connectorChannelID, authSessionID, refresh)
	if !ok {
		log.Debug("查询 connector 认证状态失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("auth_session_id", authSessionID),
			zap.String("result", "auth_session_not_found"),
		)
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
		log.Debug("查询 connector connection descriptor 失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("result", "connection_not_found"),
		)
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
	if !impl.hasChannel(connectorChannelID) {
		log.Debug("拒绝 connector 工具调用",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("result", "connection_not_found"),
		)
		_ = writeError(conn, packet.RequestID, packet.ConnectorChannelID, "connection_not_found", "channel is not bound to a valid connection")
		return
	}
	if impl.connect.ConnectionByChannel(connectorChannelID) == nil {
		log.Debug("拒绝 connector 工具调用",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("result", "connection_not_authenticated"),
		)
		_ = writeAckError(conn, packet, "tool.invoke.ack", "connection_not_authenticated", "channel is not bound to an authenticated target user")
		return
	}
	toolID := stringFromAny(packet.Payload["tool_id"])
	log.Debug("收到 connector 工具调用",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("tool_id", toolID),
		zap.String("result", "started"),
	)
	result, err := impl.connect.InvokeTool(context.Background(), connectservice.ToolInvokeInput{
		ConnectorChannelID: connectorChannelID,
		ToolID:             toolID,
		Arguments:          mapFromAny(packet.Payload["arguments"]),
	})
	if err != nil {
		log.Debug("connector 工具调用失败",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("tool_id", toolID),
			zap.String("result", "failed"),
			zap.Error(err),
		)
		_ = writeAckError(conn, packet, "tool.invoke.ack", "tool_invoke_failed", err.Error())
		return
	}
	log.Debug("connector 工具调用完成",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("tool_id", result.ToolID),
		zap.String("result", "succeeded"),
	)
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
		log.Debug("拒绝 connector 登出请求",
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("result", "connection_not_found"),
		)
		_ = writeError(conn, packet.RequestID, packet.ConnectorChannelID, "connection_not_found", "channel is not bound to a valid connection")
		return
	}
	loggedOut := impl.connect.LogoutByChannel(connectorChannelID)
	log.Debug("connector 登出请求已处理",
		zap.String("connector_channel_id", connectorChannelID),
		zap.Bool("logged_out", loggedOut),
		zap.String("result", "handled"),
	)
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
	log.Debug("connector channel 已关闭",
		zap.String("connector_channel_id", connectorChannelID),
		zap.String("result", "closed"),
	)
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

func (impl *serviceImpl) connectionByPacket(packet *connectorprotocol.WirePacket) *connectordomain.Connection {
	if packet == nil {
		return nil
	}
	connectorChannelID := strings.TrimSpace(packet.ConnectorChannelID)
	if !impl.hasChannel(connectorChannelID) {
		return nil
	}
	return impl.connect.ConnectionByChannel(connectorChannelID)
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

func writeAckError(conn *channelConnection, request *connectorprotocol.WirePacket, packetType string, code string, message string) error {
	return writePacket(conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_" + randomToken(10),
		RequestID:          request.RequestID,
		ReplyTo:            request.PacketID,
		ConnectorChannelID: strings.TrimSpace(request.ConnectorChannelID),
		Type:               packetType,
		Time:               time.Now().UnixMilli(),
		Error: &connectorprotocol.WireError{
			Code:    code,
			Message: message,
		},
	})
}

func randomToken(byteCount int) string {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}

func randomChannelToken(length int) string {
	if length <= 0 {
		length = 12
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		fallback := randomToken(length)
		if len(fallback) >= length {
			return fallback[:length]
		}
		return fallback
	}
	for index, value := range buffer {
		buffer[index] = alphabet[int(value)%len(alphabet)]
	}
	return string(buffer)
}

func stringFromAny(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func boolFromAny(value any) bool {
	enabled, ok := value.(bool)
	return ok && enabled
}

func (impl *serviceImpl) authStatusPayload(result any) map[string]any {
	impl.applyConnectorIDToAuthResult(result)
	encoded, err := json.Marshal(result)
	if err != nil {
		return map[string]any{"status": "failed", "message": err.Error()}
	}
	payload := map[string]any{}
	if err = json.Unmarshal(encoded, &payload); err != nil {
		return map[string]any{"status": "failed", "message": err.Error()}
	}
	return payload
}

func (impl *serviceImpl) authCancelPayload(result *connectorprotocol.ConnectorAuthCancelResult) map[string]any {
	return impl.authStatusPayload(result)
}

func (impl *serviceImpl) applyConnectorIDToAuthResult(result any) {
	switch typedResult := result.(type) {
	case *connectorprotocol.ConnectorAuthStartResult:
		typedResult.ConnectionDescriptor = impl.applyConnectorIDToDescriptor(typedResult.ConnectionDescriptor)
	case *connectorprotocol.ConnectorAuthStatusResult:
		typedResult.ConnectionDescriptor = impl.applyConnectorIDToDescriptor(typedResult.ConnectionDescriptor)
	case *connectorprotocol.ConnectorAuthCancelResult:
		typedResult.ConnectionDescriptor = impl.applyConnectorIDToDescriptor(typedResult.ConnectionDescriptor)
	}
}

func mapFromAny(value any) map[string]any {
	if value == nil {
		return nil
	}
	if values, ok := value.(map[string]any); ok {
		return values
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	values := map[string]any{}
	if err = json.Unmarshal(encoded, &values); err != nil {
		return nil
	}
	return values
}

func sanitizeIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	builder := strings.Builder{}
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			builder.WriteRune(ch)
			continue
		}
		builder.WriteByte('_')
	}
	return strings.Trim(builder.String(), "_")
}
