package channelservice

import (
	"context"
	"sync"

	"github.com/coffeehc/boot/plugin"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/connectservice"
)

var service Service
var serviceMutex = new(sync.RWMutex)
var serviceName = "channelService"

// GetService 返回 Telegram connector data plane channel 服务实例。
func GetService() Service {
	if service == nil {
		panic("channelService 未初始化")
	}
	return service
}

// EnablePlugin 启用 Telegram connector data plane channel 服务插件。
func EnablePlugin(ctx context.Context) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	if service != nil {
		return
	}
	connectservice.EnablePlugin(ctx)
	service = newService(connectservice.GetService())
	plugin.RegisterPlugin(serviceName, service)
}
