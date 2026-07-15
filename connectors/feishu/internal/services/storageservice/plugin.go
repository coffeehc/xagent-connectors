package storageservice

import (
	"context"
	"sync"

	"github.com/coffeehc/boot/plugin"
	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/configservice"
	"github.com/spf13/viper"
)

var service Service
var serviceMutex sync.RWMutex

// GetService 返回飞书 connector storage service。
func GetService() Service {
	if service == nil {
		panic("storageService 未初始化")
	}
	return service
}

// EnablePlugin 启用飞书 connector storage service。
func EnablePlugin(ctx context.Context) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	if service != nil {
		return
	}
	configservice.InitConfig()
	service = newService(viper.GetString(configservice.KeyStateDir))
	plugin.RegisterPlugin("storageService", service)
}
