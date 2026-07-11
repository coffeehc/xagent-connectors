package main

import (
	"context"

	"github.com/coffeehc/boot/configuration"
	"github.com/coffeehc/boot/engine"
	connectorinternal "github.com/coffeehc/xagent-connectors/connectors/telegram/internal"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/configservice"
)

// main 启动 Telegram Connector Server 进程。
func main() {
	configservice.InitConfig()
	configuration.SetRunModel(configuration.Model_dev)
	if configuration.Version == "" {
		configuration.Version = "0.0.1"
	}
	engine.StartEngine(context.TODO(), connectorinternal.GetServiceInfo(), connectorinternal.Start)
}
