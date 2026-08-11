package main

import (
	"sync"
	"testing"

	"github.com/tinode/chat/server/store/types"
)

func TestHubKeepsSameTopicNameSeparateByTenant(t *testing.T) {
	h := &Hub{topics: &sync.Map{}}
	aKey := types.TopicKey{TenantID: 1, Topic: "sys"}
	bKey := types.TopicKey{TenantID: 2, Topic: "sys"}
	a := &Topic{tenantID: 1, name: "sys"}
	b := &Topic{tenantID: 2, name: "sys"}

	h.topicPut(aKey, a)
	h.topicPut(bKey, b)

	if got := h.topicGet(aKey); got != a {
		t.Fatalf("tenant 1 returned wrong topic: %p", got)
	}
	if got := h.topicGet(bKey); got != b {
		t.Fatalf("tenant 2 returned wrong topic: %p", got)
	}
	if h.numTopics != 2 {
		t.Fatalf("expected 2 topics, got %d", h.numTopics)
	}
}

func TestServerMessageKeepsTenantAndRejectsCrossTenantDelivery(t *testing.T) {
	original := &ServerComMessage{TenantID: 2, RcptTo: "sys"}
	if copied := original.copy(); copied.TenantID != original.TenantID {
		t.Fatalf("copy lost tenant: got %d, want %d", copied.TenantID, original.TenantID)
	}

	sess := &Session{tenantID: 1, send: make(chan any, 1)}
	if sess.queueOut(original) {
		t.Fatal("cross-tenant message was accepted")
	}
	select {
	case <-sess.send:
		t.Fatal("cross-tenant message reached the session queue")
	default:
	}
}

func TestTopicAssignsTenantToServerMessage(t *testing.T) {
	topic := &Topic{tenantID: 7, cat: types.TopicCatGrp, xoriginal: "grpTest"}
	msg := &ServerComMessage{Data: &MsgServerData{Topic: "grpTest"}}
	topic.prepareBroadcastableMessage(msg, types.ZeroUid, false)
	if msg.TenantID != topic.tenantID {
		t.Fatalf("message tenant = %d, want %d", msg.TenantID, topic.tenantID)
	}
}
