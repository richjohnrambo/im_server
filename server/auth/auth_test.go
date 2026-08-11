package auth

import (
	"testing"

	"github.com/tinode/chat/server/store/types"
)

func TestBindRecord(t *testing.T) {
	ctx := AuthContext{TenantID: 7, RemoteAddr: "127.0.0.1"}
	rec := &Rec{}
	if err := BindRecord(ctx, rec); err != nil {
		t.Fatalf("BindRecord failed: %v", err)
	}
	if rec.TenantID != ctx.TenantID {
		t.Fatalf("unexpected tenant: got %d, want %d", rec.TenantID, ctx.TenantID)
	}

	if err := BindRecord(AuthContext{}, &Rec{}); err != types.ErrMalformed {
		t.Fatalf("zero tenant: got %v, want %v", err, types.ErrMalformed)
	}
	if err := BindRecord(ctx, &Rec{TenantID: 8}); err != types.ErrFailed {
		t.Fatalf("tenant mismatch: got %v, want %v", err, types.ErrFailed)
	}
}
