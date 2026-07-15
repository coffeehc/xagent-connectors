package feishuservice

import (
	"context"
	"sync"

	"github.com/coffeehc/boot/plugin"
)

var service Service
var serviceMutex sync.RWMutex

// GetService 返回飞书 SDK service。
func GetService() Service {
	if service == nil {
		panic("feishuService 未初始化")
	}
	return service
}

// EnablePlugin 启用飞书 SDK service。
func EnablePlugin(context.Context) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	if service != nil {
		return
	}
	service = newService()
	plugin.RegisterPlugin("feishuService", service)
}
