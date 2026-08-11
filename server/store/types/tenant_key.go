package types

import "strconv"

// TopicKey uniquely identifies a topic inside the server without changing its
// externally visible or persisted name.
type TopicKey struct {
	TenantID TenantID
	Topic    string
}

// IsValid reports whether the key can be used for tenant-scoped routing.
func (k TopicKey) IsValid() bool {
	return k.TenantID.IsValid() && k.Topic != ""
}

// RoutingKey returns the internal key used by the cluster and multiplexing
// layers. It must never be persisted as a topic name or returned to a client.
func (k TopicKey) RoutingKey() string {
	if !k.IsValid() {
		return ""
	}
	return strconv.FormatInt(int64(k.TenantID), 10) + ":" + k.Topic
}

// UserKey uniquely identifies a user within a tenant-scoped operation.
type UserKey struct {
	TenantID TenantID
	User     Uid
}

// IsValid reports whether the key contains both a tenant and a user.
func (k UserKey) IsValid() bool {
	return k.TenantID.IsValid() && !k.User.IsZero()
}

// RoutingKey returns an internal string representation of the user key.
func (k UserKey) RoutingKey() string {
	if !k.IsValid() {
		return ""
	}
	return strconv.FormatInt(int64(k.TenantID), 10) + ":" + k.User.UserId()
}
