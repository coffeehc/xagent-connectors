package internal

import (
	"context"

	"github.com/coffeehc/boot/engine"
	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/endpointservice"
	"github.com/spf13/cobra"
)

// Start 启动 Feishu Connector Server 的插件编排入口。
func Start(ctx context.Context, _ *cobra.Command, _ []string) (engine.ServiceCloseCallback, error) {
	endpointservice.EnablePlugin(ctx)
	return func() {}, nil
}
