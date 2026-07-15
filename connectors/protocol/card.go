package protocol

// ConnectorProtocol 表示 xAgent 与 Connector Server 之间的传输协议。
type ConnectorProtocol string

const (
	// ConnectorProtocolHTTP 表示首版 HTTP Connector 协议。
	ConnectorProtocolHTTP ConnectorProtocol = "xagent-connector-http"
	// ConnectorProtocolWebSocket 表示 WebSocket Connector 协议。
	ConnectorProtocolWebSocket ConnectorProtocol = "xagent-connector-ws"
	// ConnectorProtocolGRPC 表示 gRPC Connector 协议。
	ConnectorProtocolGRPC ConnectorProtocol = "xagent-connector-grpc"
)

// ConnectorTrustLevel 表示 Connector Card 声明的来源信任级别。
type ConnectorTrustLevel string

const (
	// ConnectorTrustLevelUnknown 表示 connector 未声明或尚未审查信任级别。
	ConnectorTrustLevelUnknown ConnectorTrustLevel = "unknown"
	// ConnectorTrustLevelBuiltin 表示 xAgent 内置 connector。
	ConnectorTrustLevelBuiltin ConnectorTrustLevel = "builtin"
	// ConnectorTrustLevelVerified 表示管理员认可或已验证 connector。
	ConnectorTrustLevelVerified ConnectorTrustLevel = "verified"
	// ConnectorTrustLevelThirdParty 表示第三方 connector。
	ConnectorTrustLevelThirdParty ConnectorTrustLevel = "third_party"
	// ConnectorTrustLevelLocal 表示本地部署 connector。
	ConnectorTrustLevelLocal ConnectorTrustLevel = "local"
)

// ConnectorTargetType 表示 connector 面向的标准目标系统领域。
//
// 语义边界：
// 1. 该枚举用于选择 xAgent profile 级通用规则，例如 IM、邮箱、日历的入站消息投影；
// 2. 具体供应商、部署和展示名不进入该枚举，必须通过 provider 和 label 表达；
// 3. 空值不合法，未知领域应先扩展枚举再接入 profile 规则。
type ConnectorTargetType string

const (
	// ConnectorTargetTypeIM 表示即时通讯类目标系统。
	ConnectorTargetTypeIM ConnectorTargetType = "im"
	// ConnectorTargetTypeEmail 表示邮件类目标系统。
	ConnectorTargetTypeEmail ConnectorTargetType = "email"
	// ConnectorTargetTypeCalendar 表示日历类目标系统。
	ConnectorTargetTypeCalendar ConnectorTargetType = "calendar"
	// ConnectorTargetTypeTicket 表示工单或任务跟踪类目标系统。
	ConnectorTargetTypeTicket ConnectorTargetType = "ticket"
)

// ConnectorUserChannelMode 表示 xAgent 对单个用户创建 Connector channel 的治理模式。
//
// 该枚举只供 xAgent 在 channel.open 前实施用户级数量限制，Connector Server 不得据此
// 拒绝或合并 channel.open 信令。空值由 xAgent 按 single 处理，以兼容尚未声明该字段的旧 Card。
type ConnectorUserChannelMode string

const (
	// ConnectorUserChannelModeSingle 表示 xAgent 对每个用户只允许保留一个当前 Connector Card channel。
	ConnectorUserChannelModeSingle ConnectorUserChannelMode = "single"
	// ConnectorUserChannelModeMultiple 表示 xAgent 允许每个用户创建多个当前 Connector Card channel。
	ConnectorUserChannelModeMultiple ConnectorUserChannelMode = "multiple"
)

// ConnectorCard 表示 Connector Server 暴露给 xAgent 的未绑定前自描述卡片。
//
// 约束说明：
// 1. ConnectorCard 只描述 connector 自身、支持的 target/profile、工具能力和登录 UI；
// 2. 它不描述某个用户已经绑定的目标系统，也不描述绑定后的 tool 可用性；
// 3. Connector 标准 endpoint 由协议固定，接入来源 Base URL 由 xAgent catalog 保存，不进入 ConnectorCard；
// 4. 绑定后的实例状态必须进入 ConnectionDescriptor。
type ConnectorCard struct {
	// Schema 表示 Connector Card 协议版本，首版必须是 xagent.connector/v1。
	Schema string `json:"schema"`
	// ConnectorCardID 表示 Connector Card 声明的静态能力 ID。
	//
	// 约束说明：
	// 1. 该 ID 由 Connector Card 声明并保持稳定；
	// 2. xAgent 用它做发现、展示、配置选择和工具命名空间；
	// 3. 它不是 Connector 在 data plane 中分配给当前 xAgent 系统的 connector_id。
	ConnectorCardID string `json:"connector_card_id"`
	// Connector 表示 connector 基本身份。
	Connector ConnectorCardInfo `json:"connector"`
	// Supports 表示 connector 支持的 target type 与 profile。
	Supports ConnectorCardSupports `json:"supports"`
	// Tools 表示 Connector Card 声明的稳定工具集合。
	Tools []ConnectorToolDescriptor `json:"tools,omitempty"`
	// AuthFlows 表示 connector 支持的用户激活认证流程。
	AuthFlows []ConnectorAuthFlowDescriptor `json:"auth_flows,omitempty"`
	// UI 表示 xAgent 可据此生成未绑定前登录或绑定页面。
	UI *ConnectorCardUI `json:"ui,omitempty"`
	// Security 表示 connector 的认证和数据安全声明。
	Security *ConnectorSecurityDescriptor `json:"security,omitempty"`
}

// ConnectorCardInfo 表示 connector 基本身份。
type ConnectorCardInfo struct {
	// Name 表示 connector 展示名称，不能为空。
	Name string `json:"name"`
	// Version 表示 connector 自身版本，不能为空。
	Version string `json:"version"`
	// Vendor 表示 connector 提供方，可为空。
	Vendor string `json:"vendor,omitempty"`
	// Description 表示 connector 能力说明，可为空。
	Description string `json:"description,omitempty"`
}

// ConnectorCardSupports 表示 connector 支持的目标类型、profile 和 xAgent 用户 channel 治理声明。
type ConnectorCardSupports struct {
	// UserChannelMode 表示 xAgent 对单个用户创建当前 Card channel 的治理模式；为空时由 xAgent 按 single 处理。
	// Connector Server 只声明该策略，不负责执行 channel 数量限制。
	UserChannelMode ConnectorUserChannelMode `json:"user_channel_mode,omitempty"`
	// TargetTypes 表示 connector 支持的 target type 集合。
	TargetTypes []ConnectorTargetType `json:"target_types,omitempty"`
	// Targets 表示 connector 支持的具体目标系统集合，保留 provider 与 label 的自由扩展空间。
	//
	// 语义边界：
	// 1. TargetTypes 是规范化枚举，Targets 是每个枚举下的具体系统声明；
	// 2. provider 是机器可读目标系统标识，例如 wechat、gmail、outlook_calendar；
	// 3. label 是给用户和 LLM 使用的人类可读来源名，例如微信、Gmail、日历。
	Targets []ConnectorTargetDescriptor `json:"targets,omitempty"`
	// Profiles 表示 connector 实现的标准 capability profile 集合。
	Profiles []string `json:"profiles,omitempty"`
}

// ConnectorTargetDescriptor 表示 Connector Card 声明的一个具体目标系统。
type ConnectorTargetDescriptor struct {
	// TargetType 表示该目标系统所属的标准领域枚举。
	TargetType ConnectorTargetType `json:"target_type"`
	// Provider 表示具体目标系统或供应商标识，允许 connector 自由命名。
	Provider string `json:"provider"`
	// Label 表示给用户和 LLM 展示的目标系统来源名。
	Label string `json:"label"`
	// Description 表示目标系统补充说明，可为空。
	Description string `json:"description,omitempty"`
}

// ConnectorToolDescriptor 表示 Connector Card 声明的单个工具能力。
type ConnectorToolDescriptor struct {
	// ToolID 表示 connector 内稳定工具 ID。
	ToolID string `json:"tool_id"`
	// Profile 表示该工具所属的标准 profile，可为空。
	Profile string `json:"profile,omitempty"`
	// Title 表示工具展示标题，可为空。
	Title string `json:"title,omitempty"`
	// Description 表示工具说明，可为空。
	Description string `json:"description,omitempty"`
	// RelatedSkillIDs 表示使用或选择该工具时建议加载的 Connector Skill ID 集合。
	//
	// 语义边界：该字段表达工具与工作流说明的关联，不代表调用工具前必须完成额外认证；
	// xAgent 可在用户意图涉及复杂流程、附件、格式转换或多工具编排时加载这些 Skill。
	RelatedSkillIDs []string `json:"related_skill_ids,omitempty"`
	// InputSchema 表示工具输入 JSON Schema，可为空。
	InputSchema map[string]any `json:"input_schema,omitempty"`
	// OutputSchema 表示工具输出 JSON Schema，可为空。
	OutputSchema map[string]any `json:"output_schema,omitempty"`
}

// ConnectorAuthFlowDescriptor 表示 connector 支持的单个用户激活认证流程。
type ConnectorAuthFlowDescriptor struct {
	// ID 表示 auth flow 稳定标识。
	ID string `json:"id"`
	// TargetType 表示该 auth flow 绑定的 target type，可为空。
	TargetType ConnectorTargetType `json:"target_type,omitempty"`
	// Type 表示认证流程类型，例如 qr_login 或 form。
	Type ConnectorAuthFlowType `json:"type"`
	// Title 表示认证流程展示标题，可为空。
	Title string `json:"title,omitempty"`
	// Description 表示认证流程说明，可为空。
	Description string `json:"description,omitempty"`
	// Fields 表示动态表单认证字段，仅 type=form 时使用。
	Fields []ConnectorAuthFlowField `json:"fields,omitempty"`
	// ConfigSchema 表示配置 JSON Schema，可为空。
	ConfigSchema map[string]any `json:"config_schema,omitempty"`
	// UISchema 表示前端渲染提示，可为空。
	UISchema map[string]any `json:"ui_schema,omitempty"`
}

// ConnectorCardUI 表示未绑定前 xAgent 生成登录页面所需的 UI 描述。
type ConnectorCardUI struct {
	// LoginFlow 表示默认登录或绑定流程的 UI 生成描述；为空表示 connector 暂不声明生成式登录页。
	LoginFlow *ConnectorLoginFlowDescriptor `json:"login_flow,omitempty"`
}

// ConnectorLoginFlowDescriptor 表示 xAgent 可生成的 connector 登录流程。
type ConnectorLoginFlowDescriptor struct {
	// FlowID 表示该 UI 流程绑定的 auth flow id，可为空。
	FlowID string `json:"flow_id,omitempty"`
	// Steps 表示登录流程步骤，按声明顺序执行或渲染。
	Steps []ConnectorLoginFlowStep `json:"steps,omitempty"`
}

// ConnectorLoginFlowStep 表示单个登录流程 UI 步骤。
type ConnectorLoginFlowStep struct {
	// Type 表示步骤类型，例如 qr_code、polling 或 form。
	Type string `json:"type"`
	// RequestType 表示该步骤发起的 packet type，例如 auth.start 或 auth.status。
	RequestType string `json:"request_type,omitempty"`
	// ResponseType 表示该步骤期望收到的 ack packet type，例如 auth.start.ack 或 auth.status.ack。
	ResponseType string `json:"response_type,omitempty"`
}

// ConnectorSecurityDescriptor 表示 connector 安全声明。
type ConnectorSecurityDescriptor struct {
	// TrustLevel 表示 connector 声明的信任级别，可为空。
	TrustLevel ConnectorTrustLevel `json:"trust_level,omitempty"`
	// APIKeyRequired 表示 Connector Server 管理接口是否要求 xAgent 提供 API key。
	APIKeyRequired bool `json:"api_key_required,omitempty"`
	// RequiresSignature 表示 Connector Card 是否要求签名验证。
	RequiresSignature bool `json:"requires_signature,omitempty"`
	// DataClasses 表示 connector 可能触达的数据类别，可为空。
	DataClasses []string `json:"data_classes,omitempty"`
}
