package configservice

import (
	"strings"
	"sync"

	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	// KeyAddr 是微信 Connector Server HTTP 监听地址配置键。
	KeyAddr = "wechat_connector.addr"
	// KeyAPIKey 是 connector system API key 配置键。
	KeyAPIKey = "wechat_connector.api_key"
	// KeyStateDir 是 connector 本地登录态持久化目录配置键。
	KeyStateDir = "wechat_connector.state_dir"
)

var initConfigOnce sync.Once

// InitConfig 注册微信 Connector Server 的默认配置和命令行参数。
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

	bindStringFlag("addr", KeyAddr, protocol.DefaultAddr, "HTTP listen address")
	bindStringFlag("api-key", KeyAPIKey, protocol.DefaultAPIKey, "API key required as Authorization: Bearer <api-key>; set empty to disable")
	bindStringFlag("state-dir", KeyStateDir, "", "directory used to persist connector login state; defaults to ~/.xagent/connectors/im-wechat")
}

func bindStringFlag(flagName string, configKey string, defaultValue string, usage string) {
	pflag.String(flagName, defaultValue, usage)
	_ = viper.BindPFlag(configKey, pflag.Lookup(flagName))
}
