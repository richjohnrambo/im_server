package types

import "testing"

func TestTopicKeyRoutingKeyIsTenantScoped(t *testing.T) {
	a := TopicKey{TenantID: 1, Topic: "sys"}
	b := TopicKey{TenantID: 2, Topic: "sys"}
	if !a.IsValid() || !b.IsValid() {
		t.Fatal("valid topic keys reported as invalid")
	}
	if a.RoutingKey() == b.RoutingKey() {
		t.Fatalf("routing key is not tenant scoped: %q", a.RoutingKey())
	}
	if got := a.RoutingKey(); got != "1:sys" {
		t.Fatalf("unexpected routing key: %q", got)
	}
}

func TestTenantKeysRejectMissingScope(t *testing.T) {
	if (TopicKey{Topic: "sys"}).IsValid() {
		t.Fatal("topic key without tenant is valid")
	}
	if (UserKey{TenantID: 1}).IsValid() {
		t.Fatal("user key without user is valid")
	}
}
