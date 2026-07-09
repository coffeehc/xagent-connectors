package connectservice

import (
	"context"
	"strings"
	"sync"

	"github.com/coffeehc/boot/plugin"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/configservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/ilinkservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/mediaservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/storageservice"
	"github.com/spf13/viper"
)

var service Service
var serviceMutex = new(sync.RWMutex)
var serviceName = "connectService"

// GetService 返回微信 connector 连接编排服务实例。
func GetService() Service {
	if service == nil {
		panic("connectService 未初始化")
	}
	return service
}

// EnablePlugin 启用微信 connector 连接编排服务插件。
func EnablePlugin(ctx context.Context) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	if service != nil {
		return
	}
	configservice.InitConfig()
	storageservice.EnablePlugin(ctx)
	ilinkservice.EnablePlugin(ctx)
	mediaservice.EnablePlugin(ctx)
	service = newService(Config{
		APIKey: strings.TrimSpace(viper.GetString(configservice.KeyAPIKey)),
	}, storageservice.GetService(), ilinkservice.GetService(), mediaservice.GetService())
	plugin.RegisterPlugin(serviceName, service)
}
