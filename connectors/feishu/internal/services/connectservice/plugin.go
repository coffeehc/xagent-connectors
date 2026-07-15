package connectservice

import (
	"context"
	"strings"
	"sync"

	"github.com/coffeehc/boot/plugin"
	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/configservice"
	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/feishuservice"
	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/storageservice"
	"github.com/spf13/viper"
)

var service Service
var serviceMutex sync.RWMutex

// GetService 返回飞书连接编排 service。
func GetService() Service {
	if service == nil {
		panic("connectService 未初始化")
	}
	return service
}

// EnablePlugin 启用飞书连接编排 service。
func EnablePlugin(ctx context.Context) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	if service != nil {
		return
	}
	configservice.InitConfig()
	storageservice.EnablePlugin(ctx)
	feishuservice.EnablePlugin(ctx)
	service = newService(Config{APIKey: strings.TrimSpace(viper.GetString(configservice.KeyAPIKey))}, storageservice.GetService(), feishuservice.GetService())
	plugin.RegisterPlugin("connectService", service)
}
