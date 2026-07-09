package protocol

import "testing"

func TestProtocolConstants(t *testing.T) {
	if ConnectorCardSchema != "xagent.connector/v1" {
		t.Fatalf("unexpected connector card schema: %s", ConnectorCardSchema)
	}
	if ConnectionDescriptorSchema != "xagent.connection/v1" {
		t.Fatalf("unexpected connection descriptor schema: %s", ConnectionDescriptorSchema)
	}
	if PacketSchema != "xagent.connector.packet/v1" {
		t.Fatalf("unexpected packet schema: %s", PacketSchema)
	}
	if DataPlaneSubprotocol != "xagent.connector.packet.v1" {
		t.Fatalf("unexpected data plane subprotocol: %s", DataPlaneSubprotocol)
	}
}
