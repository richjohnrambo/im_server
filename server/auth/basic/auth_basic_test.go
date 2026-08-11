package basic

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/tinode/chat/server/auth"
	"github.com/tinode/chat/server/store"
	"github.com/tinode/chat/server/store/mock_store"
	"github.com/tinode/chat/server/store/types"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthenticateScopesLoginByTenant(t *testing.T) {
	ctrl := gomock.NewController(t)
	users := mock_store.NewMockUsersPersistenceInterface(ctrl)
	oldUsers := store.Users
	store.Users = users
	t.Cleanup(func() {
		store.Users = oldUsers
		ctrl.Finish()
	})

	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	users.EXPECT().GetAuthUniqueRecord(types.TenantID(7), "basic", "alice").
		Return(types.Uid(70), auth.LevelAuth, hash, time.Time{}, nil)
	users.EXPECT().GetAuthUniqueRecord(types.TenantID(8), "basic", "alice").
		Return(types.Uid(80), auth.LevelAuth, hash, time.Time{}, nil)

	a := &authenticator{name: "basic", minLoginLength: defaultMinLoginLength,
		minPasswordLength: defaultMinPasswordLength}
	recA, _, err := a.Authenticate(auth.AuthContext{TenantID: 7}, []byte("alice:password"))
	if err != nil {
		t.Fatal(err)
	}
	recB, _, err := a.Authenticate(auth.AuthContext{TenantID: 8}, []byte("alice:password"))
	if err != nil {
		t.Fatal(err)
	}
	if recA.TenantID != 7 || recA.Uid != 70 || recB.TenantID != 8 || recB.Uid != 80 {
		t.Fatalf("unexpected records: A=%+v B=%+v", recA, recB)
	}
}

func TestAuthenticateRejectsMissingTenant(t *testing.T) {
	a := &authenticator{name: "basic", minLoginLength: defaultMinLoginLength,
		minPasswordLength: defaultMinPasswordLength}
	if _, _, err := a.Authenticate(auth.AuthContext{}, []byte("alice:password")); err != types.ErrMalformed {
		t.Fatalf("got %v, want %v", err, types.ErrMalformed)
	}
}
