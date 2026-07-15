package protocol

const (
	// ConnectorCardID 是飞书 Connector Card 的稳定 ID。
	ConnectorCardID = "im.feishu"
	// ConnectorName 是飞书 connector 的展示名称。
	ConnectorName = "Feishu Connector"
	// ConnectorSkillIMReplyID 是回复飞书消息的主 Skill ID。
	ConnectorSkillIMReplyID = "im-connector-reply"
	// FeishuQRCreateFlowID 是扫码创建飞书应用的认证流程 ID。
	FeishuQRCreateFlowID = "feishu_qr_create_app"
	// FeishuMessageSendToolID 是向默认单聊发送文本的工具 ID。
	FeishuMessageSendToolID = "feishu_message_send"
	// FeishuMessageSendImageToolID 是向默认单聊发送图片的工具 ID。
	FeishuMessageSendImageToolID = "feishu_message_send_image"
	// FeishuMessageReplyToolID 是文本回复工具 ID。
	FeishuMessageReplyToolID = "feishu_message_reply"
	// FeishuMessageReplyImageToolID 是图片回复工具 ID。
	FeishuMessageReplyImageToolID = "feishu_message_reply_image"
	// DefaultAddr 是 connector 默认监听地址。
	DefaultAddr = "127.0.0.1:19092"
	// DefaultAPIKey 是 connector 默认 API key。
	DefaultAPIKey = "test-api"
	// AuthPollIntervalMillis 是认证状态建议轮询间隔。
	AuthPollIntervalMillis int64 = 1000
	// AppStateFilename 是应用凭据状态文件名。
	AppStateFilename = "apps.json"
	// ReferenceStateFilename 是引用状态文件名。
	ReferenceStateFilename = "references.json"
	// MediaCacheDirname 是入站图片缓存目录名。
	MediaCacheDirname = "media"
)
