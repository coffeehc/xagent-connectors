package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/coffeehc/boot/configuration"
	"github.com/spf13/viper"
)

func TestResolveStartupRootDirUsesCurrentViperRootDir(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	rootDir := filepath.Join(t.TempDir(), "runtime")
	viper.Set("root_dir", rootDir)

	got := resolveStartupRootDir()

	if got != rootDir {
		t.Fatalf("期望使用 viper 当前 root_dir: got=%s want=%s", got, rootDir)
	}
}

func TestBuildStartupLoggerConfigUsesResolvedRootDir(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	rootDir := filepath.Join(t.TempDir(), "runtime")
	viper.Set("root_dir", rootDir)

	loggerConfig := buildStartupLoggerConfig()

	expectedLogFile := filepath.Join(rootDir, startupLogsDirName, startupLogFileName)
	if loggerConfig.FileConfig.FileName != expectedLogFile {
		t.Fatalf("期望日志文件为 %s，实际 %s", expectedLogFile, loggerConfig.FileConfig.FileName)
	}
}

func TestBuildStartupLoggerConfigUsesProductLogPolicy(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	previousRunMode := RunMode
	RunMode = "prod"
	t.Cleanup(func() {
		RunMode = previousRunMode
	})

	loggerConfig := buildStartupLoggerConfig()

	if RunMode != configuration.Model_product {
		t.Fatalf("期望 prod 归一化为 product: %s", RunMode)
	}
	if loggerConfig.Level != "error" || loggerConfig.EnableConsole || loggerConfig.EnableSampler {
		t.Fatalf("生产日志配置异常: %+v", loggerConfig)
	}
}

func TestResolveStartupRootDirFallsBackToHomeXAgent(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	homeDir := t.TempDir()
	previousUserHomeDir := startupUserHomeDir
	startupUserHomeDir = func() (string, error) {
		return homeDir, nil
	}
	t.Cleanup(func() {
		startupUserHomeDir = previousUserHomeDir
	})

	rootDir := resolveStartupRootDir()

	expected := filepath.Join(homeDir, ".xagent")
	if rootDir != expected {
		t.Fatalf("期望回退 root_dir 为 %s，实际 %s", expected, rootDir)
	}
}

func TestResolveStartupRootDirFallsBackToDotXAgentWhenHomeMissing(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	previousUserHomeDir := startupUserHomeDir
	startupUserHomeDir = func() (string, error) {
		return "", os.ErrNotExist
	}
	t.Cleanup(func() {
		startupUserHomeDir = previousUserHomeDir
	})

	rootDir := resolveStartupRootDir()

	expected := filepath.Clean(".xagent")
	if rootDir != expected {
		t.Fatalf("期望 home 缺失时回退 root_dir 为 %s，实际 %s", expected, rootDir)
	}
}
