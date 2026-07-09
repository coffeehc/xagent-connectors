package protocol

const (
	// ConnectorCardSchema 是 Connector Card JSON 文档的协议 schema。
	ConnectorCardSchema = "xagent.connector/v1"
	// ConnectionDescriptorSchema 是绑定后 connection descriptor 的协议 schema。
	ConnectionDescriptorSchema = "xagent.connection/v1"
	// PacketSchema 是 xAgent 与 Connector data plane 传输包的协议 schema。
	PacketSchema = "xagent.connector.packet/v1"
	// DataPlaneSubprotocol 是 WebSocket data plane 使用的子协议名称。
	DataPlaneSubprotocol = "xagent.connector.packet.v1"
	// ProtocolVersion 是当前 xAgent Connector 公共协议版本。
	ProtocolVersion = "1.0"
)

const (
	// ConnectorCardPath 是 connector v1 固定 Connector Card endpoint。
	ConnectorCardPath = "/connector-card.json"
	// ConnectorSkillPath 是 connector v1 固定主 Skill endpoint。
	ConnectorSkillPath = "/skill.md"
	// ConnectorHealthPath 是 connector v1 固定系统级健康检查 endpoint。
	ConnectorHealthPath = "/health"
	// ConnectorDataPlanePath 是 connector v1 固定 WebSocket data plane endpoint。
	ConnectorDataPlanePath = "/ws"
	// ConnectorMediaUploadPath 是 connector 媒体上传 HTTP endpoint。
	ConnectorMediaUploadPath = "/media/uploads"
	// ConnectorMediaRefPathPrefix 是 connector 媒体引用下载 HTTP endpoint 前缀。
	ConnectorMediaRefPathPrefix = "/media/refs"
)
