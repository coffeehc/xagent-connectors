package protocol

// WirePacket 表示 xAgent 与 connector data plane 之间传输的 packet。
type WirePacket struct {
	// Schema 是 packet 协议 schema，首版必须是 xagent.connector.packet/v1。
	Schema string `json:"schema"`
	// PacketID 是当前 packet 的唯一标识。
	PacketID string `json:"packet_id"`
	// RequestID 是请求和响应配对使用的 id，可为空。
	RequestID string `json:"request_id,omitempty"`
	// ReplyTo 是当前 packet 回复的 packet_id，可为空。
	ReplyTo string `json:"reply_to,omitempty"`
	// ConnectorChannelID 是 connector 分配的逻辑用户 channel id，可为空。
	ConnectorChannelID string `json:"connector_channel_id,omitempty"`
	// Type 是 packet 类型，例如 channel.open、channel.open.ack、tool.invoke。
	Type string `json:"type"`
	// Time 是 packet 创建时间，单位毫秒时间戳；未设置时为空。
	Time int64 `json:"time,omitempty"`
	// Payload 是具体 packet 类型的业务负载，可为空。
	Payload map[string]any `json:"payload,omitempty"`
	// Error 是错误 packet 的错误信息，可为空。
	Error *WireError `json:"error,omitempty"`
}

// WireError 表示 data plane packet 错误。
type WireError struct {
	// Code 是稳定错误码。
	Code string `json:"code"`
	// Message 是可读错误信息，可为空。
	Message string `json:"message,omitempty"`
}
