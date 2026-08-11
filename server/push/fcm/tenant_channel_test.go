package fcm

import (
	"testing"

	"github.com/tinode/chat/server/store/types"
)

func TestTenantChannelName(t *testing.T) {
	got := TenantChannelName(types.TenantID(42), "chnAbc")
	if got != "tenant-42-chnAbc" {
		t.Fatalf("TenantChannelName = %q", got)
	}
}

func TestTenantChannelNameInvalidTenantFallsBack(t *testing.T) {
	got := TenantChannelName(types.ZeroTenantID, "chnAbc")
	if got != "chnAbc" {
		t.Fatalf("TenantChannelName zero tenant = %q", got)
	}
}
