package protocol

import "testing"

func TestDefaultAPIKeyMatchesLocalConnectorPassword(t *testing.T) {
	if DefaultAPIKey != "test-api" {
		t.Fatalf("本地测试默认 API key 应为 test-api，got=%q", DefaultAPIKey)
	}
}
