package main

import "testing"

func TestPbClientHiTenantRoundTrip(t *testing.T) {
	original := &ClientComMessage{Hi: &MsgClientHi{
		Id:      "hi-1",
		Version: "1",
		Tenant:  "acme",
	}}

	decoded := pbCliDeserialize(pbCliSerialize(original))
	if decoded.Hi == nil {
		t.Fatal("decoded hi message is nil")
	}
	if decoded.Hi.Tenant != original.Hi.Tenant {
		t.Fatalf("tenant: expected %q, got %q", original.Hi.Tenant, decoded.Hi.Tenant)
	}
}
