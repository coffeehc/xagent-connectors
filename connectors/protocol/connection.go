package protocol

// ConnectionStatus 表示当前用户某条 connection 的状态。
type ConnectionStatus string

const (
	// ConnectionStatusCreated 表示 connection 已创建但尚未开始认证。
	ConnectionStatusCreated ConnectionStatus = "created"
	// ConnectionStatusAuthenticating 表示 connection 正在认证或绑定。
	ConnectionStatusAuthenticating ConnectionStatus = "authenticating"
	// ConnectionStatusConnected 表示 connection 已绑定且当前在线可用。
	ConnectionStatusConnected ConnectionStatus = "connected"
	// ConnectionStatusDegraded 表示 connection 已绑定但部分能力降级。
	ConnectionStatusDegraded ConnectionStatus = "degraded"
	// ConnectionStatusOffline 表示 connection 绑定仍存在但目标或 connector 当前离线。
	ConnectionStatusOffline ConnectionStatus = "offline"
	// ConnectionStatusExpired 表示 connection 认证材料已经过期。
	ConnectionStatusExpired ConnectionStatus = "expired"
	// ConnectionStatusRevoked 表示 connection 已被用户或目标系统撤销。
	ConnectionStatusRevoked ConnectionStatus = "revoked"
	// ConnectionStatusError 表示 connection 处于不可自动分类的错误态。
	ConnectionStatusError ConnectionStatus = "error"
)

// ConnectionToolStatus 表示绑定后某个 connection tool 的当前可用性。
type ConnectionToolStatus string

const (
	// ConnectionToolStatusAvailable 表示 tool 当前可调用。
	ConnectionToolStatusAvailable ConnectionToolStatus = "available"
	// ConnectionToolStatusUnavailable 表示 tool 当前不可用但原因未进一步分类。
	ConnectionToolStatusUnavailable ConnectionToolStatus = "unavailable"
	// ConnectionToolStatusDeniedByTarget 表示 tool 被目标系统权限拒绝。
	ConnectionToolStatusDeniedByTarget ConnectionToolStatus = "denied_by_target"
	// ConnectionToolStatusRequiresReauth 表示 tool 需要用户重新认证后才能使用。
	ConnectionToolStatusRequiresReauth ConnectionToolStatus = "requires_reauth"
	// ConnectionToolStatusNotSupported 表示该 connection 不支持该 profile tool。
	ConnectionToolStatusNotSupported ConnectionToolStatus = "not_supported"
)

// ConnectionTargetPermissionState 表示目标系统对某个 connection tool 的权限判断。
type ConnectionTargetPermissionState string

const (
	// ConnectionTargetPermissionUnknown 表示目标系统权限状态未知。
	ConnectionTargetPermissionUnknown ConnectionTargetPermissionState = "unknown"
	// ConnectionTargetPermissionGranted 表示目标系统已允许该 tool。
	ConnectionTargetPermissionGranted ConnectionTargetPermissionState = "granted"
	// ConnectionTargetPermissionDenied 表示目标系统已拒绝该 tool。
	ConnectionTargetPermissionDenied ConnectionTargetPermissionState = "denied"
	// ConnectionTargetPermissionRequiresReauth 表示目标系统要求重新认证后才能判断或使用。
	ConnectionTargetPermissionRequiresReauth ConnectionTargetPermissionState = "requires_reauth"
)

// ConnectionDescriptor 表示某个用户绑定后的具体 connection 实例描述。
//
// 约束说明：
// 1. ConnectionDescriptor 只描述当前用户可见的绑定实例；
// 2. tools 是绑定后按目标权限计算出的当前 connector 侧状态；
// 3. 不得用它反向表达 connector 全局能力。
type ConnectionDescriptor struct {
	// Schema 表示 connection descriptor 协议版本，首版必须是 xagent.connection/v1。
	Schema string `json:"schema"`
	// Connection 表示 connection 实例身份和状态。
	Connection ConnectionDescriptorInfo `json:"connection"`
	// Target 表示该 connection 绑定的目标系统账号或实例。
	Target ConnectionTargetDescriptor `json:"target"`
	// Tools 表示当前 connection 已知的 tool 状态，可为空。
	Tools []ConnectionToolState `json:"tools,omitempty"`
}

// ConnectionDescriptorInfo 表示 connection 实例身份和状态。
type ConnectionDescriptorInfo struct {
	// ConnectorCardID 表示该 connection 来源的 Connector Card 静态能力 ID。
	ConnectorCardID string `json:"connector_card_id"`
	// ConnectorID 表示 Connector 分配给当前 xAgent 系统的 ID，可为空。
	ConnectorID string `json:"connector_id,omitempty"`
	// ConnectorChannelID 表示 Connector 分配的用户级持久 channel ID。
	ConnectorChannelID string `json:"connector_channel_id"`
	// TargetType 表示该 connection 绑定的 target type。
	TargetType ConnectorTargetType `json:"target_type"`
	// Profile 表示该 connection 当前采用的标准 profile。
	Profile string `json:"profile"`
	// Status 表示该 connection 当前状态。
	Status ConnectionStatus `json:"status"`
}

// ConnectionTargetDescriptor 表示用户绑定后的目标系统账号或实例。
type ConnectionTargetDescriptor struct {
	// Provider 表示目标系统提供方，例如 wechat。
	Provider string `json:"provider"`
	// Label 表示目标系统给用户和 LLM 使用的来源名，例如微信、Gmail 或日历，可为空。
	Label string `json:"label,omitempty"`
	// DisplayName 表示面向用户展示的目标账号或实例名称。
	DisplayName string `json:"display_name"`
	// AccountHint 表示目标账号脱敏提示，例如 wxid_***，可为空。
	AccountHint string `json:"account_hint,omitempty"`
}

// ConnectionToolState 表示绑定后某个 profile tool 的 concrete tool 与权限状态。
type ConnectionToolState struct {
	// ToolID 表示 Connector Card 声明的稳定 tool id。
	ToolID string `json:"tool_id"`
	// Status 表示该 tool 当前可用性。
	Status ConnectionToolStatus `json:"status"`
	// TargetPermissionState 表示目标系统对该 tool 的权限状态，可为空。
	TargetPermissionState ConnectionTargetPermissionState `json:"target_permission_state,omitempty"`
	// Reason 表示不可用或受限时的简短原因，可为空。
	Reason string `json:"reason,omitempty"`
}
