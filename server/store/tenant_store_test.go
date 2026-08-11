package store

import (
	"testing"

	"github.com/tinode/chat/server/store/types"
)

func TestForTenantRejectsZero(t *testing.T) {
	if scoped, err := ForTenant(types.ZeroTenantID); err != types.ErrMalformed || scoped != nil {
		t.Fatalf("ForTenant(0) = (%v, %v), want (nil, malformed)", scoped, err)
	}
}
