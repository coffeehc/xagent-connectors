package connectservice

import (
	"context"
	"strings"
	"sync"

	"github.com/coffeehc/boot/plugin"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/configservice"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/storageservice"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/telegramservice"
	"github.com/spf13/viper"
)

var service Service
var serviceMutex = new(sync.RWMutex)
var serviceName = "connectService"

// GetService 返回 Telegram connector 连接编排服务实例。
func GetService() Service {
	if service == nil {
		panic("connectService 未初始化")
	}
	return service
}

// EnablePlugin 启用 Telegram connector 连接编排服务插件。
func EnablePlugin(ctx context.Context) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	if service != nil {
		return
	}
	configservice.InitConfig()
	storageservice.EnablePlugin(ctx)
	telegramservice.EnablePlugin(ctx)
	service = newService(Config{
		APIKey: strings.TrimSpace(viper.GetString(configservice.KeyAPIKey)),
	}, storageservice.GetService(), telegramservice.GetService())
	plugin.RegisterPlugin(serviceName, service)
}
