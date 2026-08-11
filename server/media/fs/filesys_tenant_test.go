package fs

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tinode/chat/server/store/types"
)

func TestTenantFileLocation(t *testing.T) {
	got, err := tenantFileLocation("/tmp/uploads", types.TenantID(42), "abc123", time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("tenantFileLocation failed: %v", err)
	}
	want := filepath.Join("/tmp/uploads", "tenants", "42", "2026", "08", "abc123")
	if got != want {
		t.Fatalf("tenantFileLocation = %q, want %q", got, want)
	}
}

func TestTenantFileLocationRejectsZeroTenant(t *testing.T) {
	if _, err := tenantFileLocation("/tmp/uploads", types.ZeroTenantID, "abc123", time.Time{}); err != types.ErrMalformed {
		t.Fatalf("tenantFileLocation err = %v, want malformed", err)
	}
}
