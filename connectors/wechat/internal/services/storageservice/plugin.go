package storageservice

import (
	"context"
	"strings"
	"sync"

	"github.com/coffeehc/boot/plugin"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/configservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
	"github.com/spf13/viper"
)

var service Service
var serviceMutex = new(sync.RWMutex)
var serviceName = "storageService"

// GetService 返回微信 connector 本地登录态 storage 服务实例。
func GetService() Service {
	if service == nil {
		panic("storageService 未初始化")
	}
	return service
}

// EnablePlugin 启用微信 connector 本地登录态 storage 服务插件。
func EnablePlugin(ctx context.Context) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	if service != nil {
		return
	}
	configservice.InitConfig()
	stateDir := strings.TrimSpace(viper.GetString(configservice.KeyStateDir))
	service = newService(stateDir, protocol.ConnectorCardID)
	plugin.RegisterPlugin(serviceName, service)
}
