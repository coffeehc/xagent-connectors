package channelservice

import (
	"testing"

	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
)

func TestAuthStatusPayloadIncludesQRCodeMaterial(t *testing.T) {
	impl := &serviceImpl{}
	payload := impl.authStatusPayload(&connectorprotocol.ConnectorAuthStatusResult{
		ConnectorChannelID: "cch_test",
		FlowID:             "feishu_qr_create_app",
		AuthSessionID:      "auth_test",
		Status:             connectorprotocol.ConnectorAuthStatusPending,
		QRCodeText:         "https://accounts.feishu.cn/qr",
		QRCodeImage:        "data:image/png;base64,test",
		ExpiresAt:          123456,
		PollIntervalMillis: 1000,
	})
	if payload["qr_code_text"] != "https://accounts.feishu.cn/qr" {
		t.Fatalf("qr_code_text = %#v", payload["qr_code_text"])
	}
	if payload["qr_code_image"] != "data:image/png;base64,test" {
		t.Fatalf("qr_code_image = %#v", payload["qr_code_image"])
	}
	if payload["expires_at"] != float64(123456) || payload["poll_interval_millis"] != float64(1000) {
		t.Fatalf("missing QR timing fields: %#v", payload)
	}
}
