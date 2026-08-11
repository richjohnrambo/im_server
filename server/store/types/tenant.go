package types

import "time"

// TenantID is the database identifier of a tenant.
type TenantID int64

// ZeroTenantID represents an unbound tenant context.
const ZeroTenantID TenantID = 0

// IsZero reports whether the tenant ID is uninitialized.
func (id TenantID) IsZero() bool {
	return id == ZeroTenantID
}

// IsValid reports whether the tenant ID can be used for tenant-scoped access.
func (id TenantID) IsValid() bool {
	return id > ZeroTenantID
}

// TenantState is the lifecycle state of a tenant.
type TenantState int16

const (
	TenantStateProvisioning TenantState = iota
	TenantStateActive
	TenantStateSuspended
	TenantStateDeleting
	TenantStateDeleted
)

// Tenant is the tenant registry record used to establish a session context.
type Tenant struct {
	ID         TenantID    `db:"id" json:"-"`
	Code       string      `db:"code" json:"code"`
	Name       string      `db:"name" json:"name"`
	TenantDesc *string     `db:"tenant_desc" json:"tenant_desc,omitempty"`
	State      TenantState `db:"state" json:"-"`
	CreatedAt  time.Time   `db:"created_at" json:"-"`
	CreatedBy  int64       `db:"created_by" json:"-"`
	UpdatedAt  time.Time   `db:"updated_at" json:"-"`
	UpdatedBy  int64       `db:"updated_by" json:"-"`
}

// IsActive reports whether the tenant can accept new sessions.
func (t *Tenant) IsActive() bool {
	return t != nil && t.State == TenantStateActive
}
