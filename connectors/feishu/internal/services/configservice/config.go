package configservice

import (
	"strings"
	"sync"

	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/protocol"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	// KeyAddr 是 Feishu Connector HTTP 监听地址配置键。
	KeyAddr = "feishu_connector.addr"
	// KeyAPIKey 是 connector system API key 配置键。
	KeyAPIKey = "feishu_connector.api_key"
	// KeyStateDir 是 connector 本地状态目录配置键。
	KeyStateDir = "feishu_connector.state_dir"
)

var initConfigOnce sync.Once

// InitConfig 注册 Feishu Connector 默认配置和命令行参数。
func InitConfig() { initConfigOnce.Do(initConfig) }

// EffectiveAddr 返回归一化后的 HTTP 监听地址。
func EffectiveAddr() string {
	if value := strings.TrimSpace(viper.GetString(KeyAddr)); value != "" {
		return value
	}
	return protocol.DefaultAddr
}

func initConfig() {
	viper.SetDefault(KeyAddr, protocol.DefaultAddr)
	viper.SetDefault(KeyAPIKey, protocol.DefaultAPIKey)
	viper.SetDefault(KeyStateDir, "")
	bindStringFlag("addr", KeyAddr, protocol.DefaultAddr, "HTTP listen address")
	bindStringFlag("api-key", KeyAPIKey, protocol.DefaultAPIKey, "API key required as Authorization: Bearer <api-key>; set empty to disable")
	bindStringFlag("state-dir", KeyStateDir, "", "directory used to persist connector state; defaults to ~/.xagent/connectors/im.feishu")
}

func bindStringFlag(name string, key string, value string, usage string) {
	pflag.String(name, value, usage)
	_ = viper.BindPFlag(key, pflag.Lookup(name))
}
