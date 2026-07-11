package storageservice

import (
	"context"
	"sync"

	"github.com/coffeehc/boot/plugin"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/configservice"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/protocol"
	"github.com/spf13/viper"
)

var service Service
var serviceMutex = new(sync.RWMutex)
var serviceName = "storageService"

// GetService 返回 Telegram connector 本地 storage 服务实例。
func GetService() Service {
	if service == nil {
		panic("storageService 未初始化")
	}
	return service
}

// EnablePlugin 启用 Telegram connector 本地 storage 服务插件。
func EnablePlugin(ctx context.Context) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	if service != nil {
		return
	}
	configservice.InitConfig()
	service = newService(viper.GetString(configservice.KeyStateDir), protocol.ConnectorCardID)
	plugin.RegisterPlugin(serviceName, service)
}
