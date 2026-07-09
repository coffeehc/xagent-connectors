package mediaservice

import (
	"context"
	"sync"

	"github.com/coffeehc/boot/plugin"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/storageservice"
)

var service Service
var serviceMutex = new(sync.RWMutex)
var serviceName = "mediaService"

// GetService 返回微信 CDN 媒体服务实例。
func GetService() Service {
	if service == nil {
		panic("mediaService 未初始化")
	}
	return service
}

// EnablePlugin 启用微信 CDN 媒体服务插件。
func EnablePlugin(ctx context.Context) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	if service != nil {
		return
	}
	storageservice.EnablePlugin(ctx)
	service = newService(protocol.WeChatCDNBaseURL, nil, storageservice.GetService())
	plugin.RegisterPlugin(serviceName, service)
}
