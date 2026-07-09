package internal

import (
	"github.com/coffeehc/boot/configuration"
)

const defaultServiceVersion = "0.0.1"

// GetServiceInfo 返回微信 Connector Server 服务元信息。
func GetServiceInfo() configuration.ServiceInfo {
	version := configuration.Version
	if version == "" {
		version = defaultServiceVersion
	}
	return configuration.ServiceInfo{
		ServiceName: "xagent-wechat-connector",
		Version:     version,
		Descriptor:  "xAgent WeChat Connector Server",
	}
}
