package token

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tinode/chat/server/auth"
	"github.com/tinode/chat/server/store/types"
)

func newTestAuthenticator(t *testing.T) *authenticator {
	t.Helper()
	conf, err := json.Marshal(map[string]any{
		"key":        []byte("0123456789abcdef0123456789abcdef"),
		"serial_num": 3,
		"expire_in":  3600,
	})
	if err != nil {
		t.Fatal(err)
	}
	ta := &authenticator{}
	if err = ta.Init(conf, "token-test"); err != nil {
		t.Fatal(err)
	}
	return ta
}

func TestTokenV2TenantRoundTrip(t *testing.T) {
	ta := newTestAuthenticator(t)
	rec := &auth.Rec{
		TenantID:  7,
		Uid:       42,
		AuthLevel: auth.LevelAuth,
		Lifetime:  auth.Duration(time.Hour),
		Features:  auth.FeatureValidated,
	}
	token, _, err := ta.GenSecret(rec)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(token), 59; got != want {
		t.Fatalf("token length: got %d, want %d", got, want)
	}

	got, challenge, err := ta.Authenticate(auth.AuthContext{TenantID: 7}, token)
	if err != nil {
		t.Fatal(err)
	}
	if challenge != nil {
		t.Fatal("unexpected challenge")
	}
	if got.TenantID != rec.TenantID || got.Uid != rec.Uid || got.AuthLevel != rec.AuthLevel || got.Features != rec.Features {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, rec)
	}

	// HTTP authentication may recover the tenant from the signed token.
	got, _, err = ta.Authenticate(auth.AuthContext{}, token)
	if err != nil || got.TenantID != rec.TenantID {
		t.Fatalf("tenant recovery failed: rec=%+v err=%v", got, err)
	}
}

func TestTokenV2RejectsWrongOrTamperedTenant(t *testing.T) {
	ta := newTestAuthenticator(t)
	token, _, err := ta.GenSecret(&auth.Rec{
		TenantID: 7, Uid: 42, AuthLevel: auth.LevelAuth, Lifetime: auth.Duration(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err = ta.Authenticate(auth.AuthContext{TenantID: 8}, token); err != types.ErrFailed {
		t.Fatalf("wrong tenant: got %v, want %v", err, types.ErrFailed)
	}

	tampered := append([]byte(nil), token...)
	tampered[1] ^= 1 // First byte of tenant_id in the little-endian payload.
	if _, _, err = ta.Authenticate(auth.AuthContext{}, tampered); err != types.ErrFailed {
		t.Fatalf("tampered tenant: got %v, want %v", err, types.ErrFailed)
	}
}

func TestTokenV2RejectsLegacyAndMissingIdentity(t *testing.T) {
	ta := newTestAuthenticator(t)
	if _, _, err := ta.Authenticate(auth.AuthContext{}, make([]byte, 50)); err != types.ErrMalformed {
		t.Fatalf("legacy token: got %v, want %v", err, types.ErrMalformed)
	}
	if _, _, err := ta.GenSecret(&auth.Rec{Uid: 42, AuthLevel: auth.LevelAuth}); err != types.ErrMalformed {
		t.Fatalf("missing tenant: got %v, want %v", err, types.ErrMalformed)
	}
	if _, _, err := ta.GenSecret(&auth.Rec{TenantID: 7, AuthLevel: auth.LevelAuth}); err != types.ErrMalformed {
		t.Fatalf("missing uid: got %v, want %v", err, types.ErrMalformed)
	}
}
