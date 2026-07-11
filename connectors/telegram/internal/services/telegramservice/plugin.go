package telegramservice

import (
	"context"
	"net/http"
	"sync"

	"github.com/coffeehc/boot/plugin"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/configservice"
	"github.com/spf13/viper"
)

var service Service
var serviceMutex = new(sync.RWMutex)
var serviceName = "telegramService"

// GetService 返回 Telegram Bot API 服务实例。
func GetService() Service {
	if service == nil {
		panic("telegramService 未初始化")
	}
	return service
}

// EnablePlugin 启用 Telegram Bot API 服务插件。
func EnablePlugin(ctx context.Context) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	if service != nil {
		return
	}
	configservice.InitConfig()
	service = newService(viper.GetString(configservice.KeyTelegramAPIBaseURL), http.DefaultClient)
	plugin.RegisterPlugin(serviceName, service)
}
