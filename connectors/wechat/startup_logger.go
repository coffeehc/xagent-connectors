package main

import (
	"os"
	"path/filepath"

	baselog "github.com/coffeehc/base/log"
	"github.com/coffeehc/boot/configuration"
	"github.com/spf13/viper"
)

var RunMode = "dev"

const (
	startupLogsDirName = "logs"
	startupLogFileName = "xagent-wechat-connector.log"
)

var startupUserHomeDir = os.UserHomeDir

// bootstrapStartupLogger 在 boot 初始化前写入固定日志配置。
//
// WHY：
// 1. boot/configuration 会自行处理默认 config.yml 和命令行 config 参数，这里不能复制一套配置定位逻辑；
// 2. logger 不再依赖 config.yml，启动入口只按当前 viper/default root_dir 决定日志目录；
// 3. boot 尚未加载配置时，也要能把早期日志稳定写到默认 root_dir/logs 下。
func bootstrapStartupLogger() error {
	loggerConfig := buildStartupLoggerConfig()
	logDir := filepath.Dir(loggerConfig.FileConfig.FileName)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	viper.Set("logger", loggerConfig)
	return nil
}

// buildStartupLoggerConfig 返回当前进程固定使用的日志配置。
func buildStartupLoggerConfig() *baselog.Config {
	rootDir := resolveStartupRootDir()
	logFilePath := filepath.Join(rootDir, startupLogsDirName, startupLogFileName)
	if RunMode == "dev" {
		return &baselog.Config{
			Level: "debug",
			FileConfig: baselog.FileLogConfig{
				FileName:   filepath.Clean(logFilePath),
				Enable:     true,
				Maxsize:    100,
				MaxBackups: 5,
				MaxAge:     7,
				Compress:   false,
			},
			EnableConsole: true,
			EnableColor:   true,
			EnableSampler: true,
		}
	}
	RunMode = configuration.Model_product
	return &baselog.Config{
		Level: "error",
		FileConfig: baselog.FileLogConfig{
			FileName:   filepath.Clean(logFilePath),
			Enable:     true,
			Maxsize:    100,
			MaxBackups: 5,
			MaxAge:     7,
			Compress:   false,
		},
		EnableConsole: false,
		EnableColor:   false,
		EnableSampler: false,
	}
}

// resolveStartupRootDir 返回启动期日志应写入的 root_dir。
func resolveStartupRootDir() string {
	rootDir := viper.GetString("root_dir")
	if rootDir == "" {
		return resolveStartupDefaultRootDir()
	}
	if filepath.IsAbs(rootDir) {
		return filepath.Clean(rootDir)
	}
	return filepath.Clean(rootDir)
}

// resolveStartupDefaultRootDir 返回启动期默认 XAgent 根目录。
func resolveStartupDefaultRootDir() string {
	homeDir, err := startupUserHomeDir()
	if err == nil && homeDir != "" {
		return filepath.Join(homeDir, ".xagent")
	}
	return filepath.Clean(".xagent")
}
