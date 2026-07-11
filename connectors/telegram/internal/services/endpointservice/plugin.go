package endpointservice

import (
	"context"
	"sync"

	"github.com/coffeehc/boot/plugin"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/channelservice"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/configservice"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/connectservice"
)

var service Service
var serviceMutex = new(sync.RWMutex)
var serviceName = "endpointService"

// GetService 返回 Telegram connector HTTP endpoint 服务实例。
func GetService() Service {
	if service == nil {
		panic("endpointService 未初始化")
	}
	return service
}

// EnablePlugin 启用 Telegram connector HTTP endpoint 服务插件。
func EnablePlugin(ctx context.Context) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	if service != nil {
		return
	}
	configservice.InitConfig()
	connectservice.EnablePlugin(ctx)
	channelservice.EnablePlugin(ctx)
	service = newService(connectservice.GetService(), channelservice.GetService())
	plugin.RegisterPlugin(serviceName, service)
}
