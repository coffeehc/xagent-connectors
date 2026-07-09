package main

import (
	"context"
	"fmt"
	"os"

	baselog "github.com/coffeehc/base/log"
	"github.com/coffeehc/boot/configuration"
	"github.com/coffeehc/boot/engine"
	connectorinternal "github.com/coffeehc/xagent-connectors/connectors/wechat/internal"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/configservice"
)

// main 启动微信 Connector Server 进程。
func main() {
	if err := bootstrapStartupLogger(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "初始化日志配置失败: %v\n", err)
		os.Exit(1)
	}
	configservice.InitConfig()
	configuration.SetRunModel(RunMode)
	if configuration.Version == "" {
		configuration.Version = "0.0.1"
	}
	baselog.InitLogger(true)
	engine.StartEngine(context.TODO(), connectorinternal.GetServiceInfo(), connectorinternal.Start)
}
