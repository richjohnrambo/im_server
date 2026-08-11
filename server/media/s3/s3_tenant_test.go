package s3

import (
	"testing"
	"time"

	"github.com/tinode/chat/server/store/types"
)

func TestTenantObjectKey(t *testing.T) {
	got, err := tenantObjectKey(types.TenantID(42), "abc123", time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("tenantObjectKey failed: %v", err)
	}
	if want := "tenants/42/2026/08/abc123"; got != want {
		t.Fatalf("tenantObjectKey = %q, want %q", got, want)
	}
}

func TestTenantObjectKeyRejectsZeroTenant(t *testing.T) {
	if _, err := tenantObjectKey(types.ZeroTenantID, "abc123", time.Time{}); err != types.ErrMalformed {
		t.Fatalf("tenantObjectKey err = %v, want malformed", err)
	}
}
