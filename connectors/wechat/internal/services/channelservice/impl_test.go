package channelservice

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/connectservice"
	connectordomain "github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
)

type testConnectService struct {
	cancelAuthSessionID string
	cancelAuthChannelID string
	descriptor          *connectorprotocol.ConnectionDescriptor
	connections         map[string]*connectordomain.Connection
	authStatusStarted   chan struct{}
	authStatusRelease   chan struct{}
	authStatusResult    *connectorprotocol.ConnectorAuthStatusResult
}

func (service *testConnectService) Start(context.Context) error {
	return nil
}

func (service *testConnectService) Stop(context.Context) error {
	return nil
}

func (service *testConnectService) APIKey() string {
	return ""
}

func (service *testConnectService) ConnectorID() string {
	return protocol.ConnectorCardID
}

func (service *testConnectService) StateDir() string {
	return ""
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

func (service *testConnectService) AuthStatus(_ context.Context, connectorChannelID string, authSessionID string, _ bool) (*connectorprotocol.ConnectorAuthStatusResult, bool) {
	if service.authStatusStarted != nil {
		select {
		case service.authStatusStarted <- struct{}{}:
		default:
		}
	}
	if service.authStatusRelease != nil {
		<-service.authStatusRelease
	}
	if service.authStatusResult != nil {
		return service.authStatusResult, true
	}
	if authSessionID != "" {
		return &connectorprotocol.ConnectorAuthStatusResult{
			ConnectorChannelID: connectorChannelID,
			AuthSessionID:      authSessionID,
			Status:             connectorprotocol.ConnectorAuthStatusPending,
			Message:            "等待扫码确认",
		}, true
	}
	return nil, false
}

func (service *testConnectService) CancelAuth(_ context.Context, connectorChannelID string, authSessionID string) *connectorprotocol.ConnectorAuthCancelResult {
	service.cancelAuthChannelID = connectorChannelID
	service.cancelAuthSessionID = authSessionID
	return &connectorprotocol.ConnectorAuthCancelResult{
		ConnectorChannelID: connectorChannelID,
		AuthSessionID:      authSessionID,
		Status:             connectorprotocol.ConnectorAuthCancelStatusCanceled,
		AuthStatus:         connectorprotocol.ConnectorAuthStatusFailed,
		Message:            "认证已取消",
	}
}

func (service *testConnectService) ConnectionByChannel(connectorChannelID string) *connectordomain.Connection {
	return service.connections[connectorChannelID]
}

func (service *testConnectService) LogoutByChannel(string) bool {
	return false
}

func (service *testConnectService) InvokeTool(context.Context, connectservice.ToolInvokeInput) (*connectservice.ToolInvokeResult, error) {
	return nil, nil
}

func (service *testConnectService) UploadMedia(context.Context, connectservice.UploadMediaInput) (*connectservice.UploadMediaResult, error) {
	return nil, nil
}

func (service *testConnectService) ReadConnectorSkill(context.Context) (*connectservice.ConnectorSkillContent, error) {
	return nil, nil
}

func (service *testConnectService) BuildConnectorCard() *connectorprotocol.ConnectorCard {
	return nil
}

func (service *testConnectService) BuildChannelDescriptor(string) *connectorprotocol.ConnectionDescriptor {
	return service.descriptor
}

func (service *testConnectService) BuildConnectionDescriptor(*connectordomain.Connection) *connectorprotocol.ConnectionDescriptor {
	return nil
}

func TestBuildChannelDescriptorInjectsAcceptedConnectorID(t *testing.T) {
	connect := &testConnectService{
		descriptor: &connectorprotocol.ConnectionDescriptor{
			Schema: "xagent.connection/v1",
			Connection: connectorprotocol.ConnectionDescriptorInfo{
				ConnectorCardID:    protocol.ConnectorCardID,
				ConnectorChannelID: "cch_user_a",
				TargetType:         connectorprotocol.ConnectorTargetTypeIM,
				Profile:            "xagent.im.v1",
				Status:             connectorprotocol.ConnectionStatusCreated,
			},
			Target: connectorprotocol.ConnectionTargetDescriptor{Provider: "wechat", Label: "微信", DisplayName: "未绑定微信"},
		},
	}
	impl := newService(connect).(*serviceImpl)
	impl.connectorID = "conn_wechat_runtime"

	descriptor := impl.buildChannelDescriptor("cch_user_a")
	if descriptor == nil {
		t.Fatalf("buildChannelDescriptor returned nil")
	}
	if descriptor.Connection.ConnectorID != "conn_wechat_runtime" {
		t.Fatalf("connection_descriptor 必须带 data plane connector_id，got=%q", descriptor.Connection.ConnectorID)
	}
}

func TestPushMessageWritesMessagePushPacket(t *testing.T) {
	impl := newService(&testConnectService{
		descriptor: &connectorprotocol.ConnectionDescriptor{
			Schema: "xagent.connection/v1",
			Connection: connectorprotocol.ConnectionDescriptorInfo{
				ConnectorCardID:    protocol.ConnectorCardID,
				ConnectorChannelID: "cch_user_a",
				Status:             connectorprotocol.ConnectionStatusCreated,
			},
		},
		connections: map[string]*connectordomain.Connection{
			"cch_user_a": {Token: "cch_user_a"},
		},
	}).(*serviceImpl)
	conn := openTestDataPlane(t, impl)
	defer conn.Close()
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:    connectorprotocol.PacketSchema,
		PacketID:  "pkt_hello",
		RequestID: "req_hello",
		Type:      "connector.hello",
		Time:      time.Now().UnixMilli(),
		Payload: map[string]any{
			"connector_card_id": protocol.ConnectorCardID,
		},
	})
	helloAck := mustReadWirePacket(t, conn)
	if helloAck.Type != "connector.hello.ack" {
		t.Fatalf("expected connector.hello.ack, got=%s", helloAck.Type)
	}
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_open",
		RequestID:          "req_open",
		ConnectorChannelID: "cch_user_a",
		Type:               "channel.open",
		Time:               time.Now().UnixMilli(),
	})
	openAck := mustReadWirePacket(t, conn)
	if openAck.Type != "channel.open.ack" || openAck.ConnectorChannelID != "cch_user_a" {
		t.Fatalf("expected channel.open.ack for cch_user_a, got=%+v", openAck)
	}

	if err := impl.PushMessage(context.Background(), MessagePushInput{
		ConnectorChannelID: "cch_user_a",
		Payload: map[string]any{
			"message_id":  "msg_1",
			"sender_name": "Coffee",
			"text":        "hello",
		},
	}); err != nil {
		t.Fatalf("PushMessage returned error: %v", err)
	}
	push := mustReadWirePacket(t, conn)
	if push.Type != "message.push" || push.ConnectorChannelID != "cch_user_a" {
		t.Fatalf("expected message.push for cch_user_a, got=%+v", push)
	}
	if push.Payload["message_id"] != "msg_1" || push.Payload["text"] != "hello" {
		t.Fatalf("message.push payload mismatch: %+v", push.Payload)
	}
}

func TestPushConnectionDescriptorWritesDescriptorPushPacket(t *testing.T) {
	impl := newService(&testConnectService{
		descriptor: &connectorprotocol.ConnectionDescriptor{
			Schema: "xagent.connection/v1",
			Connection: connectorprotocol.ConnectionDescriptorInfo{
				ConnectorCardID:    protocol.ConnectorCardID,
				ConnectorChannelID: "cch_user_a",
				Status:             connectorprotocol.ConnectionStatusConnected,
			},
		},
		connections: map[string]*connectordomain.Connection{
			"cch_user_a": {Token: "cch_user_a"},
		},
	}).(*serviceImpl)
	conn := openTestDataPlane(t, impl)
	defer conn.Close()
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:    connectorprotocol.PacketSchema,
		PacketID:  "pkt_hello",
		RequestID: "req_hello",
		Type:      "connector.hello",
		Time:      time.Now().UnixMilli(),
		Payload: map[string]any{
			"connector_card_id": protocol.ConnectorCardID,
		},
	})
	_ = mustReadWirePacket(t, conn)
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_open",
		RequestID:          "req_open",
		ConnectorChannelID: "cch_user_a",
		Type:               "channel.open",
		Time:               time.Now().UnixMilli(),
	})
	_ = mustReadWirePacket(t, conn)

	if err := impl.PushConnectionDescriptor(context.Background(), DescriptorPushInput{
		ConnectorChannelID: "cch_user_a",
		Descriptor: &connectorprotocol.ConnectionDescriptor{
			Schema: "xagent.connection/v1",
			Connection: connectorprotocol.ConnectionDescriptorInfo{
				ConnectorCardID:    protocol.ConnectorCardID,
				ConnectorChannelID: "cch_user_a",
				Status:             connectorprotocol.ConnectionStatusExpired,
			},
			Target: connectorprotocol.ConnectionTargetDescriptor{Provider: "wechat", Label: "微信", DisplayName: "微信 0069***.bot"},
		},
	}); err != nil {
		t.Fatalf("PushConnectionDescriptor returned error: %v", err)
	}
	push := mustReadWirePacket(t, conn)
	if push.Type != "connection.descriptor.push" || push.ConnectorChannelID != "cch_user_a" {
		t.Fatalf("expected connection.descriptor.push for cch_user_a, got=%+v", push)
	}
	rawDescriptor := push.Payload["connection_descriptor"]
	encoded, _ := json.Marshal(rawDescriptor)
	descriptor := &connectorprotocol.ConnectionDescriptor{}
	if err := json.Unmarshal(encoded, descriptor); err != nil {
		t.Fatalf("descriptor payload unmarshal failed: %v", err)
	}
	if descriptor.Connection.Status != connectorprotocol.ConnectionStatusExpired || descriptor.Connection.ConnectorID == "" {
		t.Fatalf("descriptor push payload mismatch: %+v", descriptor.Connection)
	}
}

func TestChannelOpenReassignsInvalidConnectorChannelID(t *testing.T) {
	connect := &testConnectService{
		descriptor: &connectorprotocol.ConnectionDescriptor{
			Schema: "xagent.connection/v1",
			Connection: connectorprotocol.ConnectionDescriptorInfo{
				ConnectorCardID: protocol.ConnectorCardID,
				Status:          connectorprotocol.ConnectionStatusCreated,
			},
		},
	}
	impl := newService(connect).(*serviceImpl)
	conn := openTestDataPlane(t, impl)
	defer conn.Close()
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:    connectorprotocol.PacketSchema,
		PacketID:  "pkt_hello",
		RequestID: "req_hello",
		Type:      "connector.hello",
		Time:      time.Now().UnixMilli(),
		Payload: map[string]any{
			"connector_card_id": protocol.ConnectorCardID,
		},
	})
	_ = mustReadWirePacket(t, conn)
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_open",
		RequestID:          "req_open",
		ConnectorChannelID: "invalid_channel",
		Type:               "channel.open",
		Time:               time.Now().UnixMilli(),
	})
	ack := mustReadWirePacket(t, conn)
	if ack.ConnectorChannelID == "invalid_channel" || len(ack.ConnectorChannelID) != 12 {
		t.Fatalf("无效 channelId 应换发 12 位 token，got=%q", ack.ConnectorChannelID)
	}
}

func TestChannelOpenKeepsAllocatedUnauthenticatedChannelID(t *testing.T) {
	connect := &testConnectService{
		descriptor: &connectorprotocol.ConnectionDescriptor{
			Schema: "xagent.connection/v1",
			Connection: connectorprotocol.ConnectionDescriptorInfo{
				ConnectorCardID: protocol.ConnectorCardID,
				Status:          connectorprotocol.ConnectionStatusCreated,
			},
		},
	}
	impl := newService(connect).(*serviceImpl)
	conn := openTestDataPlane(t, impl)
	defer conn.Close()
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:    connectorprotocol.PacketSchema,
		PacketID:  "pkt_hello",
		RequestID: "req_hello",
		Type:      "connector.hello",
		Time:      time.Now().UnixMilli(),
		Payload: map[string]any{
			"connector_card_id": protocol.ConnectorCardID,
		},
	})
	_ = mustReadWirePacket(t, conn)
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:    connectorprotocol.PacketSchema,
		PacketID:  "pkt_open_1",
		RequestID: "req_open_1",
		Type:      "channel.open",
		Time:      time.Now().UnixMilli(),
	})
	firstAck := mustReadWirePacket(t, conn)
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_open_2",
		RequestID:          "req_open_2",
		ConnectorChannelID: firstAck.ConnectorChannelID,
		Type:               "channel.open",
		Time:               time.Now().UnixMilli(),
	})
	secondAck := mustReadWirePacket(t, conn)
	if secondAck.ConnectorChannelID != firstAck.ConnectorChannelID {
		t.Fatalf("已分配但未认证 channelId 不应再次换发，first=%q second=%q", firstAck.ConnectorChannelID, secondAck.ConnectorChannelID)
	}
}

func TestAuthCancelWritesAckAndDoesNotRequireLogout(t *testing.T) {
	connect := &testConnectService{
		descriptor: &connectorprotocol.ConnectionDescriptor{
			Schema: "xagent.connection/v1",
			Connection: connectorprotocol.ConnectionDescriptorInfo{
				ConnectorCardID:    protocol.ConnectorCardID,
				ConnectorChannelID: "cch_user_a",
				Status:             connectorprotocol.ConnectionStatusCreated,
			},
		},
	}
	impl := newService(connect).(*serviceImpl)
	conn := openTestDataPlane(t, impl)
	defer conn.Close()
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:    connectorprotocol.PacketSchema,
		PacketID:  "pkt_hello",
		RequestID: "req_hello",
		Type:      "connector.hello",
		Time:      time.Now().UnixMilli(),
		Payload: map[string]any{
			"connector_card_id": protocol.ConnectorCardID,
		},
	})
	_ = mustReadWirePacket(t, conn)
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_open",
		RequestID:          "req_open",
		ConnectorChannelID: "cch_user_a",
		Type:               "channel.open",
		Time:               time.Now().UnixMilli(),
	})
	openAck := mustReadWirePacket(t, conn)
	connectorChannelID := openAck.ConnectorChannelID
	if connectorChannelID == "" {
		t.Fatalf("channel.open.ack missing connector_channel_id: %+v", openAck)
	}
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_cancel",
		RequestID:          "req_cancel",
		ConnectorChannelID: connectorChannelID,
		Type:               "auth.cancel",
		Time:               time.Now().UnixMilli(),
		Payload: map[string]any{
			"auth_session_id": "auth_session_a",
		},
	})
	cancelAck := mustReadWirePacket(t, conn)
	if cancelAck.Type != "auth.cancel.ack" || cancelAck.RequestID != "req_cancel" {
		t.Fatalf("expected auth.cancel.ack, got=%+v", cancelAck)
	}
	if connect.cancelAuthChannelID != connectorChannelID || connect.cancelAuthSessionID != "auth_session_a" {
		t.Fatalf("CancelAuth input mismatch: channel=%q session=%q", connect.cancelAuthChannelID, connect.cancelAuthSessionID)
	}
	if cancelAck.Payload["status"] != string(connectorprotocol.ConnectorAuthCancelStatusCanceled) {
		t.Fatalf("cancel ack payload mismatch: %+v", cancelAck.Payload)
	}
}

func TestAuthStatusDoesNotBlockAuthCancel(t *testing.T) {
	statusStarted := make(chan struct{}, 1)
	statusRelease := make(chan struct{})
	connect := &testConnectService{
		authStatusStarted: statusStarted,
		authStatusRelease: statusRelease,
		descriptor: &connectorprotocol.ConnectionDescriptor{
			Schema: "xagent.connection/v1",
			Connection: connectorprotocol.ConnectionDescriptorInfo{
				ConnectorCardID:    protocol.ConnectorCardID,
				ConnectorChannelID: "cch_user_a",
				Status:             connectorprotocol.ConnectionStatusCreated,
			},
		},
	}
	impl := newService(connect).(*serviceImpl)
	conn := openTestDataPlane(t, impl)
	defer conn.Close()
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:    connectorprotocol.PacketSchema,
		PacketID:  "pkt_hello",
		RequestID: "req_hello",
		Type:      "connector.hello",
		Time:      time.Now().UnixMilli(),
		Payload: map[string]any{
			"connector_card_id": protocol.ConnectorCardID,
		},
	})
	_ = mustReadWirePacket(t, conn)
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_open",
		RequestID:          "req_open",
		ConnectorChannelID: "cch_user_a",
		Type:               "channel.open",
		Time:               time.Now().UnixMilli(),
	})
	openAck := mustReadWirePacket(t, conn)
	connectorChannelID := openAck.ConnectorChannelID
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_status",
		RequestID:          "req_status",
		ConnectorChannelID: connectorChannelID,
		Type:               "auth.status",
		Time:               time.Now().UnixMilli(),
		Payload: map[string]any{
			"auth_session_id": "auth_session_a",
		},
	})
	select {
	case <-statusStarted:
	case <-time.After(time.Second):
		t.Fatalf("auth.status 未进入 connect service")
	}
	mustWriteWirePacket(t, conn, &connectorprotocol.WirePacket{
		Schema:             connectorprotocol.PacketSchema,
		PacketID:           "pkt_cancel",
		RequestID:          "req_cancel",
		ConnectorChannelID: connectorChannelID,
		Type:               "auth.cancel",
		Time:               time.Now().UnixMilli(),
		Payload: map[string]any{
			"auth_session_id": "auth_session_a",
		},
	})
	cancelAck := mustReadWirePacket(t, conn)
	if cancelAck.Type != "auth.cancel.ack" || cancelAck.RequestID != "req_cancel" {
		t.Fatalf("auth.cancel 不应被未完成的 auth.status 阻塞，got=%+v", cancelAck)
	}
	close(statusRelease)
	statusAck := mustReadWirePacket(t, conn)
	if statusAck.Type != "auth.status.ack" || statusAck.RequestID != "req_status" {
		t.Fatalf("auth.status 释放后应返回自己的 ack，got=%+v", statusAck)
	}
}

func openTestDataPlane(t *testing.T, impl *serviceImpl) *websocket.Conn {
	t.Helper()
	app := fiber.New()
	app.Get(connectorprotocol.ConnectorDataPlanePath, impl.HandleDataPlane)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	}()
	t.Cleanup(func() {
		_ = app.Shutdown()
		select {
		case err := <-done:
			if err != nil && !strings.Contains(err.Error(), "closed") {
				t.Fatalf("fiber listener returned error: %v", err)
			}
		case <-time.After(time.Second):
		}
	})

	conn, response, err := websocket.DefaultDialer.Dial("ws://"+listener.Addr().String()+connectorprotocol.ConnectorDataPlanePath, http.Header{
		"Sec-Websocket-Protocol": []string{connectorprotocol.DataPlaneSubprotocol},
	})
	if err != nil {
		if response != nil {
			t.Fatalf("dial failed: %v status=%d", err, response.StatusCode)
		}
		t.Fatalf("dial failed: %v", err)
	}
	return conn
}

func mustWriteWirePacket(t *testing.T, conn *websocket.Conn, packet *connectorprotocol.WirePacket) {
	t.Helper()
	payload, err := json.Marshal(packet)
	if err != nil {
		t.Fatalf("marshal packet failed: %v", err)
	}
	if err = conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("write packet failed: %v", err)
	}
}

func mustReadWirePacket(t *testing.T, conn *websocket.Conn) *connectorprotocol.WirePacket {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline failed: %v", err)
	}
	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read packet failed: %v", err)
	}
	if messageType != websocket.TextMessage {
		t.Fatalf("expected text message, got=%d", messageType)
	}
	packet := &connectorprotocol.WirePacket{}
	if err = json.Unmarshal(payload, packet); err != nil {
		t.Fatalf("decode packet failed: %v payload=%s", err, string(payload))
	}
	return packet
}
