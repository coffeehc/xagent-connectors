package configservice

import (
	"strings"
	"sync"

	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/protocol"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	// KeyAddr 是 Telegram Connector Server HTTP 监听地址配置键。
	KeyAddr = "telegram_connector.addr"
	// KeyAPIKey 是 connector system API key 配置键。
	KeyAPIKey = "telegram_connector.api_key"
	// KeyStateDir 是 connector 本地登录态持久化目录配置键。
	KeyStateDir = "telegram_connector.state_dir"
	// KeyTelegramAPIBaseURL 是 Telegram Bot API base URL 配置键。
	KeyTelegramAPIBaseURL = "telegram_connector.api_base_url"
)

var initConfigOnce sync.Once

// InitConfig 注册 Telegram Connector Server 的默认配置和命令行参数。
func InitConfig() {
	initConfigOnce.Do(initConfig)
}

// EffectiveAddr 返回归一化后的 HTTP 监听地址。
func EffectiveAddr() string {
	addr := strings.TrimSpace(viper.GetString(KeyAddr))
	if addr == "" {
		return protocol.DefaultAddr
	}
	return addr
}

func initConfig() {
	viper.SetDefault(KeyAddr, protocol.DefaultAddr)
	viper.SetDefault(KeyAPIKey, protocol.DefaultAPIKey)
	viper.SetDefault(KeyStateDir, "")
	viper.SetDefault(KeyTelegramAPIBaseURL, protocol.TelegramAPIBaseURL)

	bindStringFlag("addr", KeyAddr, protocol.DefaultAddr, "HTTP listen address")
	bindStringFlag("api-key", KeyAPIKey, protocol.DefaultAPIKey, "API key required as Authorization: Bearer <api-key>; set empty to disable")
	bindStringFlag("state-dir", KeyStateDir, "", "directory used to persist connector login state; defaults to ~/.xagent/connectors/im-telegram")
	bindStringFlag("telegram-api-base-url", KeyTelegramAPIBaseURL, protocol.TelegramAPIBaseURL, "Telegram Bot API base URL")
}

func bindStringFlag(flagName string, configKey string, defaultValue string, usage string) {
	pflag.String(flagName, defaultValue, usage)
	_ = viper.BindPFlag(configKey, pflag.Lookup(flagName))
}
