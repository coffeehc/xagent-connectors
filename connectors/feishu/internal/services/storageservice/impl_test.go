package storageservice

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/domain"
	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/protocol"
)

func TestStoragePersistsCredentialsWithOwnerOnlyPermissions(t *testing.T) {
	stateDir := t.TempDir()
	service := newService(stateDir)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	apps := map[string]*connectordomain.AppBinding{"cch_test": {ConnectorChannelID: "cch_test", AppID: "cli_test", AppSecret: "secret", DefaultChatID: "oc_p2p", DefaultSenderOpenID: "ou_user", DefaultChatBoundAt: 2, CreatedAt: 1}}
	if err := service.SaveApps(apps); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(stateDir, protocol.AppStateFilename))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credential file mode = %o, want 600", info.Mode().Perm())
	}
	loaded, err := service.LoadApps()
	if err != nil {
		t.Fatal(err)
	}
	if loaded["cch_test"] == nil || loaded["cch_test"].AppSecret != "secret" || loaded["cch_test"].DefaultChatID != "oc_p2p" || loaded["cch_test"].DefaultSenderOpenID != "ou_user" || loaded["cch_test"].DefaultChatBoundAt != 2 {
		t.Fatalf("unexpected loaded apps: %#v", loaded)
	}
}
