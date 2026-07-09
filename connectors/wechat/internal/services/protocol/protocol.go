package protocol

const (
	// DefaultAddr 是微信 Connector Server 默认监听地址。
	DefaultAddr = "127.0.0.1:19090"
	// DefaultAPIKey 是本地测试默认 API key。
	DefaultAPIKey = "test-api"
	// ConnectorCardID 是微信 Connector Card 的固定协议 ID。
	ConnectorCardID = "im.wechat"
	// ConnectorName 是微信 Connector Card 的固定展示名。
	ConnectorName = "WeChat Connector"
	// ILinkChannelVersion 是微信 iLink base_info 使用的内部通道版本，不是 Connector Card 或 GitHub Release 版本。
	ILinkChannelVersion = "1.0.6"
	// WeChatQRLoginFlowID 是微信二维码登录流程 id。
	WeChatQRLoginFlowID = "wechat_qr_login"
	// WeChatAPIBaseURL 是固定的微信 iLink API endpoint。
	WeChatAPIBaseURL = "https://ilinkai.weixin.qq.com"
	// WeChatCDNBaseURL 是固定的微信 CDN API endpoint。
	WeChatCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
	// WeChatBotType 是固定的微信 iLink bot_type。
	WeChatBotType = "3"
	// WeChatLongPollTimeoutMillis 是 getupdates 固定长轮询超时。
	WeChatLongPollTimeoutMillis = 35000
	// BotStateFilename 是本地微信 bot 登录态文件名。
	BotStateFilename = "bots.json"
	// ChannelStateFilename 是历史沿用的 connection binding 快照文件名。
	ChannelStateFilename = "channels.json"
	// LegacyConnectorStateFilename 是旧版本地 connection 登录态文件名，仅用于迁移读取。
	LegacyConnectorStateFilename = "connections.json"
	// PendingInboundFilename 是本地待投递入站消息队列文件名。
	PendingInboundFilename = "pending_inbound.json"
	// MediaReferenceFilename 是本地微信 CDN 媒体 key 映射文件名。
	MediaReferenceFilename = "media_references.json"
	// MediaCacheDirname 是入站微信媒体本地缓存目录名。
	MediaCacheDirname = "media_cache"
	// LegacyUploadedMediaFilename 是旧版本出站媒体引用文件名，仅用于启动迁移读取。
	LegacyUploadedMediaFilename = "uploaded_media.json"
	// DefaultMediaReferenceTTLMillis 是媒体 key 映射默认过期时间。
	DefaultMediaReferenceTTLMillis = 24 * 60 * 60 * 1000
	// InboundCacheTTLMillis 是入站消息本地缓存固定过期时间。
	InboundCacheTTLMillis = 60 * 60 * 1000
	// InboundCacheCleanupIntervalMillis 是入站消息缓存固定清理周期。
	InboundCacheCleanupIntervalMillis = 30 * 60 * 1000
	// InboundCacheMaxMessagesPerChannel 是每个 channel 固定最多缓存消息数。
	InboundCacheMaxMessagesPerChannel = 1000
	// ILinkAppID 是调用微信 iLink API 使用的应用标识。
	ILinkAppID = "bot"
	// ILinkAppClientVersion 是调用微信 iLink API 使用的客户端版本。
	ILinkAppClientVersion = "132102"
	// AuthPollIntervalMillis 是 xAgent 轮询 auth.status 的建议间隔。
	AuthPollIntervalMillis = 2000
	// AuthMaterialPollIntervalMillis 是二维码材料尚未返回前的短轮询间隔。
	AuthMaterialPollIntervalMillis = 500
	// ConnectorSkillIMReplyID 是微信 IM 回复 Skill 的稳定 ID。
	ConnectorSkillIMReplyID = "im-connector-reply"
)
