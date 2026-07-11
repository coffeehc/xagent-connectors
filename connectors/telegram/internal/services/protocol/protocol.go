package protocol

const (
	// DefaultAddr 是 Telegram Connector Server 默认监听地址。
	DefaultAddr = "127.0.0.1:19091"
	// DefaultAPIKey 是本地测试默认 API key。
	DefaultAPIKey = "test-api"
	// ConnectorCardID 是 Telegram Connector Card 的固定协议 ID。
	ConnectorCardID = "im.telegram"
	// ConnectorName 是 Telegram Connector Card 的固定展示名。
	ConnectorName = "Telegram Connector"
	// TelegramBotBindingFlowID 是 Telegram bot token 与 chat_id 表单绑定流程 id。
	TelegramBotBindingFlowID = "telegram_bot_binding"
	// TelegramAPIBaseURL 是固定的 Telegram Bot API endpoint。
	TelegramAPIBaseURL = "https://api.telegram.org"
	// TelegramLongPollTimeoutSeconds 是 getUpdates 固定长轮询超时。
	TelegramLongPollTimeoutSeconds = 35
	// BotStateFilename 是本地 Telegram bot 登录态文件名。
	BotStateFilename = "bots.json"
	// ChannelStateFilename 是 connection binding 快照文件名。
	ChannelStateFilename = "channels.json"
	// MediaReferenceFilename 是本地 Telegram 媒体 key 映射文件名。
	MediaReferenceFilename = "media_references.json"
	// MediaCacheDirname 是出站 Telegram 媒体本地缓存目录名。
	MediaCacheDirname = "media_cache"
	// DefaultMediaReferenceTTLMillis 是媒体 key 映射默认过期时间。
	DefaultMediaReferenceTTLMillis = 24 * 60 * 60 * 1000
	// AuthPollIntervalMillis 是 xAgent 轮询 auth.status 的建议间隔。
	AuthPollIntervalMillis = 2000
	// ConnectorSkillIMReplyID 是 Telegram IM 回复 Skill 的稳定 ID。
	ConnectorSkillIMReplyID = "im-connector-reply"
)
