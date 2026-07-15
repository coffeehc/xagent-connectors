package storageservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/protocol"
)

type serviceImpl struct {
	stateDir string
	mu       sync.Mutex
}

type referenceState struct {
	Reply map[string]*connectordomain.ReplyReference `json:"reply"`
	Media map[string]*connectordomain.MediaReference `json:"media"`
}

func newService(stateDir string) Service {
	if strings.TrimSpace(stateDir) == "" {
		stateDir = defaultStateDir(protocol.ConnectorCardID)
	}
	return &serviceImpl{stateDir: stateDir}
}

// Start 创建状态和媒体目录。
func (impl *serviceImpl) Start(context.Context) error {
	if err := os.MkdirAll(impl.stateDir, 0o700); err != nil {
		return err
	}
	return os.MkdirAll(impl.MediaCacheDir(), 0o700)
}

// Stop 停止 storage service。
func (impl *serviceImpl) Stop(context.Context) error { return nil }

// StateDir 返回状态目录。
func (impl *serviceImpl) StateDir() string { return impl.stateDir }

// MediaCacheDir 返回媒体缓存目录。
func (impl *serviceImpl) MediaCacheDir() string {
	return filepath.Join(impl.stateDir, protocol.MediaCacheDirname)
}

// LoadApps 加载全部应用绑定。
func (impl *serviceImpl) LoadApps() (map[string]*connectordomain.AppBinding, error) {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	result := map[string]*connectordomain.AppBinding{}
	if err := readJSON(filepath.Join(impl.stateDir, protocol.AppStateFilename), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SaveApps 保存全部应用绑定。
func (impl *serviceImpl) SaveApps(apps map[string]*connectordomain.AppBinding) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	return writeJSON(filepath.Join(impl.stateDir, protocol.AppStateFilename), apps)
}

// LoadReferences 加载回复和媒体引用。
func (impl *serviceImpl) LoadReferences() (map[string]*connectordomain.ReplyReference, map[string]*connectordomain.MediaReference, error) {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	state := referenceState{Reply: map[string]*connectordomain.ReplyReference{}, Media: map[string]*connectordomain.MediaReference{}}
	if err := readJSON(filepath.Join(impl.stateDir, protocol.ReferenceStateFilename), &state); err != nil {
		return nil, nil, err
	}
	if state.Reply == nil {
		state.Reply = map[string]*connectordomain.ReplyReference{}
	}
	if state.Media == nil {
		state.Media = map[string]*connectordomain.MediaReference{}
	}
	return state.Reply, state.Media, nil
}

// SaveReferences 保存回复和媒体引用。
func (impl *serviceImpl) SaveReferences(reply map[string]*connectordomain.ReplyReference, media map[string]*connectordomain.MediaReference) error {
	impl.mu.Lock()
	defer impl.mu.Unlock()
	return writeJSON(filepath.Join(impl.stateDir, protocol.ReferenceStateFilename), referenceState{Reply: reply, Media: media})
}

func defaultStateDir(connectorID string) string {
	if value := strings.TrimSpace(os.Getenv("XAGENT_FEISHU_CONNECTOR_STATE_DIR")); value != "" {
		return value
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".xagent", "connectors", sanitize(connectorID))
	}
	return filepath.Join(os.TempDir(), "xagent", "connectors", sanitize(connectorID))
}

func sanitize(value string) string {
	var result strings.Builder
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '.' || char == '-' || char == '_' {
			result.WriteRune(char)
		} else {
			result.WriteByte('_')
		}
	}
	if result.Len() == 0 {
		return "im.feishu"
	}
	return result.String()
}

func readJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取飞书 connector 状态失败: %w", err)
	}
	if err = json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("解析飞书 connector 状态失败: %w", err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err = os.WriteFile(temporary, raw, 0o600); err != nil {
		return err
	}
	if err = os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return os.Chmod(path, 0o600)
}
