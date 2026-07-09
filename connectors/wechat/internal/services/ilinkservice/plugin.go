package ilinkservice

import (
	"context"
	"sync"

	"github.com/coffeehc/boot/plugin"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
)

var service Service
var serviceMutex = new(sync.RWMutex)
var serviceName = "ilinkService"

// GetService 返回微信 iLink API 服务实例。
func GetService() Service {
	if service == nil {
		panic("ilinkService 未初始化")
	}
	return service
}

// EnablePlugin 启用微信 iLink API 服务插件。
func EnablePlugin(ctx context.Context) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	if service != nil {
		return
	}
	service = newService(
		protocol.WeChatAPIBaseURL,
		protocol.WeChatBotType,
		nil,
	)
	plugin.RegisterPlugin(serviceName, service)
}
