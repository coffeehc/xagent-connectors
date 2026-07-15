package feishuservice

import (
	"errors"
	"testing"

	"github.com/larksuite/oapi-sdk-go/v3/scene/registration"
)

func TestNormalizeRegistrationErrorPreservesRemoteReason(t *testing.T) {
	err := normalizeRegistrationError(&registration.AccessDeniedError{RegisterAppError: &registration.RegisterAppError{Code: "access_denied", Description: "tenant policy denied"}})
	var normalized *RegistrationError
	if !errors.As(err, &normalized) {
		t.Fatalf("unexpected error type: %T", err)
	}
	if normalized.Kind != RegistrationErrorAccessDenied || normalized.Code != "access_denied" || normalized.Description != "tenant policy denied" {
		t.Fatalf("unexpected normalized error: %#v", normalized)
	}
}
