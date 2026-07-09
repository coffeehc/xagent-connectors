package main

import (
	"context"

	baselog "github.com/coffeehc/base/log"
	"github.com/coffeehc/boot/configuration"
	"github.com/coffeehc/boot/engine"
	connectorinternal "github.com/coffeehc/xagent-connectors/connectors/wechat/internal"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/configservice"
)

// main 启动微信 Connector Server 进程。
func main() {
	configservice.InitConfig()
	configuration.SetRunModel(configuration.Model_product)
	if configuration.Version == "" {
		configuration.Version = "0.0.1"
	}
	baselog.InitLogger(true)
	engine.StartEngine(context.TODO(), connectorinternal.GetServiceInfo(), connectorinternal.Start)
}
