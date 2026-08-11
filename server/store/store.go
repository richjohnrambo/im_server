// Package store provides methods for registering and accessing database adapters.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tinode/chat/server/auth"
	adapter "github.com/tinode/chat/server/db"
	"github.com/tinode/chat/server/logs"
	"github.com/tinode/chat/server/media"
	"github.com/tinode/chat/server/store/types"
	"github.com/tinode/chat/server/validate"
)

var adp adapter.Adapter
var availableAdapters = make(map[string]adapter.Adapter)
var mediaHandler media.Handler
var activeTenantIDs sync.Map

// Unique ID generator
var uGen types.UidGenerator

type configType struct {
	// 16-byte key for XTEA. Used to initialize types.UidGenerator.
	UidKey []byte `json:"uid_key"`
	// Maximum number of results to return from adapter.
	MaxResults int `json:"max_results"`
	// DB adapter name to use. Should be one of those specified in `Adapters`.
	UseAdapter string `json:"use_adapter"`
	// Configurations for individual adapters.
	Adapters map[string]json.RawMessage `json:"adapters"`
}

func openAdapter(workerId int, jsonconf json.RawMessage) error {
	var config configType
	if err := json.Unmarshal(jsonconf, &config); err != nil {
		return errors.New("store: failed to parse config: " + err.Error() + "(" + string(jsonconf) + ")")
	}

	if adp == nil {
		if len(config.UseAdapter) > 0 {
			// Adapter name specified explicitly.
			if ad, ok := availableAdapters[config.UseAdapter]; ok {
				adp = ad
			} else {
				return errors.New("store: " + config.UseAdapter + " adapter is not available in this binary")
			}
		} else if len(availableAdapters) == 1 {
			// Default to the only entry in availableAdapters.
			for _, v := range availableAdapters {
				adp = v
			}
		} else {
			return errors.New("store: db adapter is not specified. Please set `store_config.use_adapter` in `tinode.conf`")
		}
	}

	if adp.IsOpen() {
		return errors.New("store: connection is already opened")
	}
	tenantBusiness, ok := adp.(adapter.TenantBusinessAdapter)
	if !ok {
		return errors.New("store: " + adp.GetName() + " adapter does not support tenant-scoped business access")
	}
	if !tenantBusiness.TenantBusinessReady() {
		return errors.New("store: " + adp.GetName() + " adapter tenant-scoped business access is not ready")
	}
	if _, ok := adp.(adapter.TenantAdapter); !ok {
		return errors.New("store: " + adp.GetName() + " adapter does not support tenant resolution")
	}
	if _, ok := adp.(adapter.TenantAuthAdapter); !ok {
		return errors.New("store: " + adp.GetName() + " adapter does not support tenant-scoped authentication")
	}

	// Initialize snowflake.
	if workerId < 0 || workerId > 1023 {
		return errors.New("store: invalid worker ID")
	}

	if err := uGen.Init(uint(workerId), config.UidKey); err != nil {
		return errors.New("store: failed to init snowflake: " + err.Error())
	}

	if err := adp.SetMaxResults(config.MaxResults); err != nil {
		return err
	}

	var adapterConfig json.RawMessage
	if config.Adapters != nil {
		adapterConfig = config.Adapters[adp.GetName()]
	}

	return adp.Open(adapterConfig)
}

// PersistentStorageInterface defines methods used for interation with persistent storage.
type PersistentStorageInterface interface {
	Open(workerId int, jsonconf json.RawMessage) error
	Close() error
	IsOpen() bool
	GetAdapter() adapter.Adapter
	GetAdapterName() string
	GetAdapterVersion() int
	GetDbVersion() int
	InitDb(jsonconf json.RawMessage, reset bool) error
	UpgradeDb(jsonconf json.RawMessage) error
	GetUid() types.Uid
	GetUidString() string
	DbStats() func() any
	GetAuthNames() []string
	GetAuthHandler(name string) auth.AuthHandler
	GetLogicalAuthHandler(name string) auth.AuthHandler
	GetValidator(name string) validate.Validator
	GetMediaHandler() media.Handler
	UseMediaHandler(name, config string) error
}

// Store is the main object for interacting with persistent storage.
var Store PersistentStorageInterface

type storeObj struct{}

// TenantsPersistenceInterface defines tenant registry lookups used before authentication.
type TenantsPersistenceInterface interface {
	GetByCode(code string) (*types.Tenant, error)
}

type tenantsMapper struct{}

// Tenants is the tenant registry access object.
var Tenants TenantsPersistenceInterface

// GetByCode resolves a tenant by its public enterprise code.
func (tenantsMapper) GetByCode(code string) (*types.Tenant, error) {
	tenantAdp, ok := adp.(adapter.TenantAdapter)
	if !ok {
		return nil, types.ErrUnsupported
	}
	return tenantAdp.TenantGetByCode(code)
}

func tenantAuthAdapter(tenantID types.TenantID) (adapter.TenantAuthAdapter, error) {
	if !tenantID.IsValid() {
		return nil, types.ErrMalformed
	}
	tenantAdp, ok := adp.(adapter.TenantAuthAdapter)
	if !ok {
		return nil, types.ErrUnsupported
	}
	return tenantAdp, nil
}

func tenantBusinessAdapter(tenantID types.TenantID) (adapter.TenantBusinessAdapter, error) {
	if !tenantID.IsValid() {
		return nil, types.ErrMalformed
	}
	tenantAdp, ok := adp.(adapter.TenantBusinessAdapter)
	if !ok {
		return nil, types.ErrUnsupported
	}
	if !tenantAdp.TenantBusinessReady() {
		return nil, types.ErrUnsupported
	}
	activeTenantIDs.Store(tenantID, struct{}{})
	return tenantAdp, nil
}

// KnownTenantIDs returns tenants which have used the business Store since this
// process started. It is used only for tenant-scoped maintenance work.
func KnownTenantIDs() []types.TenantID {
	var result []types.TenantID
	activeTenantIDs.Range(func(key, _ any) bool {
		result = append(result, key.(types.TenantID))
		return true
	})
	return result
}

// TenantStore is a validated tenant scope. Domain mappers still receive the
// tenant explicitly so it remains visible at every persistence boundary.
type TenantStore struct {
	TenantID types.TenantID
}

// ForTenant validates and returns a tenant-scoped Store handle.
func ForTenant(tenantID types.TenantID) (*TenantStore, error) {
	if _, err := tenantBusinessAdapter(tenantID); err != nil {
		return nil, err
	}
	return &TenantStore{TenantID: tenantID}, nil
}

// Open initializes the persistence system. Adapter holds a connection pool for a database instance.
//
//		name - name of the adapter rquested in the config file
//	  jsonconf - configuration string
func (storeObj) Open(workerId int, jsonconf json.RawMessage) error {
	if err := openAdapter(workerId, jsonconf); err != nil {
		return err
	}

	return adp.CheckDbVersion()
}

// Close terminates connection to persistent storage.
func (storeObj) Close() error {
	if adp.IsOpen() {
		return adp.Close()
	}

	return nil
}

// IsOpen checks if persistent storage connection has been initialized.
func (storeObj) IsOpen() bool {
	if adp != nil {
		return adp.IsOpen()
	}

	return false
}

// GetAdapter returns the currently configured adapter.
func (storeObj) GetAdapter() adapter.Adapter {
	return adp
}

// GetAdapterName returns the name of the current adater.
func (storeObj) GetAdapterName() string {
	if adp != nil {
		return adp.GetName()
	}

	return ""
}

// GetAdapterVersion returns version of the current adater.
func (storeObj) GetAdapterVersion() int {
	if adp != nil {
		return adp.Version()
	}

	return -1
}

// GetDbVersion returns version of the underlying database.
func (storeObj) GetDbVersion() int {
	if adp != nil {
		vers, _ := adp.GetDbVersion()
		return vers
	}

	return -1
}

// InitDb creates and configures a new database instance. If 'reset' is true it will first
// attempt to drop an existing database. If jsconf is nil it will assume that the adapter is
// already open. If it's non-nil and the adapter is not open, it will use the config string
// to open the adapter first.
func (s storeObj) InitDb(jsonconf json.RawMessage, reset bool) error {
	if !s.IsOpen() {
		if err := openAdapter(1, jsonconf); err != nil {
			return err
		}
	}
	return adp.CreateDb(reset)
}

// UpgradeDb performes an upgrade of the database to the current adapter version.
// If jsconf is nil it will assume that the adapter is already open. If it's non-nil and the
// adapter is not open, it will use the config string to open the adapter first.
func (s storeObj) UpgradeDb(jsonconf json.RawMessage) error {
	if !s.IsOpen() {
		if err := openAdapter(1, jsonconf); err != nil {
			return err
		}
	}
	return adp.UpgradeDb()
}

// RegisterAdapter makes a persistence adapter available.
// If Register is called twice or if the adapter is nil, it panics.
func RegisterAdapter(a adapter.Adapter) {
	if a == nil {
		panic("store: Register adapter is nil")
	}

	adapterName := a.GetName()
	if _, ok := availableAdapters[adapterName]; ok {
		panic("store: adapter '" + adapterName + "' is already registered")
	}
	availableAdapters[adapterName] = a
}

// GetUid generates a unique ID suitable for use as a primary key.
func (storeObj) GetUid() types.Uid {
	return uGen.Get()
}

// GetUidString generate unique ID as a string.
func (storeObj) GetUidString() string {
	return uGen.GetStr()
}

// DecodeUid takes an XTEA encrypted Uid and decrypts it into an int64.
// This is needed for sql compatibility. Tte original int64 values
// are generated by snowflake which ensures that the top bit is unset.
func DecodeUid(uid types.Uid) int64 {
	if uid.IsZero() {
		return 0
	}
	return uGen.DecodeUid(uid)
}

// EncodeUid applies XTEA encryption to an int64 value. It's the inverse of DecodeUid.
func EncodeUid(id int64) types.Uid {
	if id == 0 {
		return types.ZeroUid
	}
	return uGen.EncodeInt64(id)
}

// DbStats returns a callback returning db connection stats object.
func (s storeObj) DbStats() func() any {
	if !s.IsOpen() {
		return nil
	}
	return adp.Stats
}

// UsersPersistenceInterface is an interface which defines methods for persistent storage of user records.
type UsersPersistenceInterface interface {
	Create(tenantID types.TenantID, user *types.User, private any) (*types.User, error)
	GetAuthRecord(tenantID types.TenantID, user types.Uid, scheme string) (string, auth.Level, []byte, time.Time, error)
	GetAuthUniqueRecord(tenantID types.TenantID, scheme, unique string) (types.Uid, auth.Level, []byte, time.Time, error)
	AddAuthRecord(tenantID types.TenantID, uid types.Uid, authLvl auth.Level, scheme, unique string, secret []byte, expires time.Time) error
	UpdateAuthRecord(tenantID types.TenantID, uid types.Uid, authLvl auth.Level, scheme, unique string, secret []byte, expires time.Time) error
	DelAuthRecords(tenantID types.TenantID, uid types.Uid, scheme string) error
	Get(tenantID types.TenantID, uid types.Uid) (*types.User, error)
	GetAll(tenantID types.TenantID, uid ...types.Uid) ([]types.User, error)
	GetByCred(tenantID types.TenantID, method, value string) (types.Uid, error)
	Delete(tenantID types.TenantID, id types.Uid, hard bool) error
	UpdateLastSeen(tenantID types.TenantID, uid types.Uid, userAgent string, when time.Time) error
	Update(tenantID types.TenantID, uid types.Uid, update map[string]any) error
	UpdateTags(tenantID types.TenantID, uid types.Uid, add, remove, reset []string) ([]string, error)
	UpdateState(tenantID types.TenantID, uid types.Uid, state types.ObjState) error
	GetSubs(tenantID types.TenantID, id types.Uid) ([]types.Subscription, error)
	FindSubs(tenantID types.TenantID, caller types.Uid, prefPrefix string, required [][]string, optional []string, activeOnly bool) ([]types.Subscription, error)
	FindOne(tenantID types.TenantID, tag string) (string, error)
	GetTopics(tenantID types.TenantID, id types.Uid, opts *types.QueryOpt) ([]types.Subscription, error)
	GetTopicsAny(tenantID types.TenantID, id types.Uid, opts *types.QueryOpt) ([]types.Subscription, error)
	GetOwnTopics(tenantID types.TenantID, id types.Uid) ([]string, error)
	GetChannels(tenantID types.TenantID, id types.Uid) ([]string, error)
	UpsertCred(tenantID types.TenantID, cred *types.Credential) (bool, error)
	ConfirmCred(tenantID types.TenantID, id types.Uid, method string) error
	FailCred(tenantID types.TenantID, id types.Uid, method string) error
	GetActiveCred(tenantID types.TenantID, id types.Uid, method string) (*types.Credential, error)
	GetAllCreds(tenantID types.TenantID, id types.Uid, method string, validatedOnly bool) ([]types.Credential, error)
	DelCred(tenantID types.TenantID, id types.Uid, method, value string) error
	GetUnreadCount(tenantID types.TenantID, ids ...types.Uid) (map[types.Uid]int, error)
	GetUnvalidated(tenantID types.TenantID, lastUpdatedBefore time.Time, limit int) ([]types.Uid, error)
}

// usersMapper is a concrete type which implements UsersPersistenceInterface.
type usersMapper struct{}

// Users is a singleton ancor object exporting UsersPersistenceInterface methods.
var Users UsersPersistenceInterface

// Create inserts User object into a database, updates creation time and assigns UID
func (usersMapper) Create(tenantID types.TenantID, user *types.User, private any) (*types.User, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	if user == nil || user.TenantID != tenantID {
		return nil, types.ErrMalformed
	}

	user.SetUid(Store.GetUid())
	user.InitTimes()

	err = tenantAdp.TenantUserCreate(tenantID, user)
	if err != nil {
		return nil, err
	}

	// Create user's subscription to 'me' && 'fnd'. These topics are ephemeral, the topic object need not to be
	// inserted.
	err = Subs.Create(tenantID,
		&types.Subscription{
			ObjHeader: types.ObjHeader{CreatedAt: user.CreatedAt},
			TenantID:  user.TenantID,
			User:      user.Id,
			Topic:     user.Uid().UserId(),
			ModeWant:  types.ModeCMeFnd,
			ModeGiven: types.ModeCMeFnd,
			Private:   private,
		},
		&types.Subscription{
			ObjHeader: types.ObjHeader{CreatedAt: user.CreatedAt},
			TenantID:  user.TenantID,
			User:      user.Id,
			Topic:     user.Uid().FndName(),
			ModeWant:  types.ModeCMeFnd,
			ModeGiven: types.ModeCMeFnd,
			Private:   nil,
		})
	if err != nil {
		// Best effort to delete incomplete user record. Orphaned user records are not a problem.
		// They just take up space.
		tenantAdp.TenantUserDelete(tenantID, user.Uid(), true)
		return nil, err
	}

	return user, nil
}

// GetAuthRecord takes a user ID and a authentication scheme name, fetches unique scheme-dependent identifier and
// authentication secret.
func (usersMapper) GetAuthRecord(tenantID types.TenantID, user types.Uid, scheme string) (string, auth.Level, []byte, time.Time, error) {
	tenantAdp, err := tenantAuthAdapter(tenantID)
	if err != nil {
		return "", auth.LevelNone, nil, time.Time{}, err
	}
	unique, authLvl, secret, expires, err := tenantAdp.TenantAuthGetRecord(tenantID, user, scheme)
	if err == nil {
		parts := strings.Split(unique, ":")
		if len(parts) > 1 {
			unique = parts[1]
		} else {
			err = types.ErrInternal
		}
	}

	return unique, authLvl, secret, expires, err
}

// GetAuthUniqueRecord takes a unique identifier and a authentication scheme name, fetches user ID and
// authentication secret.
func (usersMapper) GetAuthUniqueRecord(tenantID types.TenantID, scheme, unique string) (types.Uid, auth.Level, []byte, time.Time, error) {
	tenantAdp, err := tenantAuthAdapter(tenantID)
	if err != nil {
		return types.ZeroUid, auth.LevelNone, nil, time.Time{}, err
	}
	return tenantAdp.TenantAuthGetUniqueRecord(tenantID, scheme+":"+unique)
}

// AddAuthRecord creates a new authentication record for the given user.
func (usersMapper) AddAuthRecord(tenantID types.TenantID, uid types.Uid, authLvl auth.Level, scheme, unique string, secret []byte,
	expires time.Time) error {
	tenantAdp, err := tenantAuthAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantAuthAddRecord(tenantID, uid, scheme, scheme+":"+unique, authLvl, secret, expires)
}

// UpdateAuthRecord updates authentication record with a new secret and expiration time.
func (usersMapper) UpdateAuthRecord(tenantID types.TenantID, uid types.Uid, authLvl auth.Level, scheme, unique string,
	secret []byte, expires time.Time) error {
	tenantAdp, err := tenantAuthAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantAuthUpdRecord(tenantID, uid, scheme, scheme+":"+unique, authLvl, secret, expires)
}

// DelAuthRecords deletes user's auth records of the given scheme.
func (usersMapper) DelAuthRecords(tenantID types.TenantID, uid types.Uid, scheme string) error {
	tenantAdp, err := tenantAuthAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantAuthDelScheme(tenantID, uid, scheme)
}

// Get returns a user object for the given user ID or nil if the user is not found.
func (usersMapper) Get(tenantID types.TenantID, uid types.Uid) (*types.User, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	user, err := tenantAdp.TenantUserGet(tenantID, uid)
	if err == nil && user != nil && user.TenantID != tenantID {
		return nil, types.ErrNotFound
	}
	return user, err
}

// GetAll returns a slice of user objects for the given user IDs.
func (usersMapper) GetAll(tenantID types.TenantID, uid ...types.Uid) ([]types.User, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	users, err := tenantAdp.TenantUserGetAll(tenantID, uid...)
	if err != nil {
		return nil, err
	}
	for i := range users {
		if users[i].TenantID != tenantID {
			return nil, types.ErrNotFound
		}
	}
	return users, nil
}

// GetByCred returns user ID for the given validated credential.
func (usersMapper) GetByCred(tenantID types.TenantID, method, value string) (types.Uid, error) {
	tenantAdp, err := tenantAuthAdapter(tenantID)
	if err != nil {
		return types.ZeroUid, err
	}
	return tenantAdp.TenantUserGetByCred(tenantID, method, value)
}

// Delete deletes user records.
func (usersMapper) Delete(tenantID types.TenantID, id types.Uid, hard bool) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantUserDelete(tenantID, id, hard)
}

// UpdateLastSeen updates LastSeen and UserAgent.
func (usersMapper) UpdateLastSeen(tenantID types.TenantID, uid types.Uid, userAgent string, when time.Time) error {
	return Users.Update(tenantID, uid, map[string]any{"LastSeen": when, "UserAgent": userAgent})
}

// Update is a general-purpose update of user data.
func (usersMapper) Update(tenantID types.TenantID, uid types.Uid, update map[string]any) error {
	if _, ok := update["UpdatedAt"]; !ok {
		update["UpdatedAt"] = types.TimeNow()
	}
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantUserUpdate(tenantID, uid, update)
}

// UpdateTags either adds, removes, or resets tags to the given slices.
func (usersMapper) UpdateTags(tenantID types.TenantID, uid types.Uid, add, remove, reset []string) ([]string, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantUserUpdateTags(tenantID, uid, add, remove, reset)
}

// UpdateState changes user's state and state of some topics associated with the user.
func (usersMapper) UpdateState(tenantID types.TenantID, uid types.Uid, state types.ObjState) error {
	update := map[string]any{
		"State":   state,
		"StateAt": types.TimeNow()}
	return Users.Update(tenantID, uid, update)
}

// GetSubs loads *all* subscriptions for the given user.
// Does not load Public/Trusted or Private, does not load deleted subscriptions.
func (usersMapper) GetSubs(tenantID types.TenantID, id types.Uid) ([]types.Subscription, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantSubsForUser(tenantID, id)
}

// FindSubs find a list of users and topics for the given tags. Results are formatted as subscriptions.
// `required` specifies an AND of ORs for required terms:
// at least one element of every sublist in `required` must be present in the object's tags list.
// `optional` specifies a list of optional terms.
func (usersMapper) FindSubs(tenantID types.TenantID, caller types.Uid, prefPrefix string, required [][]string, optional []string, activeOnly bool) ([]types.Subscription, error) {
	if len(required) == 0 && len(optional) == 0 {
		// No tags specified, return empty list.
		return nil, nil
	}
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantFind(tenantID, caller.UserId(), prefPrefix, required, optional, activeOnly)
}

// Find returns topics and/or users which match the given tag, with optional partial matching.
func (usersMapper) FindOne(tenantID types.TenantID, tag string) (string, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return "", err
	}
	return tenantAdp.TenantFindOne(tenantID, tag)
}

// GetTopics load a list of user's subscriptions with Public+Trusted fields copied to subscription
func (usersMapper) GetTopics(tenantID types.TenantID, id types.Uid, opts *types.QueryOpt) ([]types.Subscription, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantTopicsForUser(tenantID, id, false, opts)
}

// GetTopicsAny load a list of user's subscriptions with Public+Trusted fields copied to subscription.
// Deleted topics are returned too.
func (usersMapper) GetTopicsAny(tenantID types.TenantID, id types.Uid, opts *types.QueryOpt) ([]types.Subscription, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantTopicsForUser(tenantID, id, true, opts)
}

// GetOwnTopics returns a slice of group topic names where the user is the owner.
func (usersMapper) GetOwnTopics(tenantID types.TenantID, id types.Uid) ([]string, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantOwnTopics(tenantID, id)
}

// GetChannels returns a slice of group topic names where the user is a channel reader.
func (usersMapper) GetChannels(tenantID types.TenantID, id types.Uid) ([]string, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantChannelsForUser(tenantID, id)
}

// UpsertCred adds or updates a credential validation request. Return true if the record was inserted, false if updated.
func (usersMapper) UpsertCred(tenantID types.TenantID, cred *types.Credential) (bool, error) {
	cred.InitTimes()
	tenantAdp, err := tenantAuthAdapter(tenantID)
	if err != nil {
		return false, err
	}
	return tenantAdp.TenantCredUpsert(tenantID, cred)
}

// ConfirmCred marks credential method as confirmed.
func (usersMapper) ConfirmCred(tenantID types.TenantID, id types.Uid, method string) error {
	tenantAdp, err := tenantAuthAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantCredConfirm(tenantID, id, method)
}

// FailCred increments fail count for the given credential method.
func (usersMapper) FailCred(tenantID types.TenantID, id types.Uid, method string) error {
	tenantAdp, err := tenantAuthAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantCredFail(tenantID, id, method)
}

// GetActiveCred gets a the currently active credential for the given user and method.
func (usersMapper) GetActiveCred(tenantID types.TenantID, id types.Uid, method string) (*types.Credential, error) {
	tenantAdp, err := tenantAuthAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantCredGetActive(tenantID, id, method)
}

// GetAllCreds returns credentials of the given user, all or validated only.
func (usersMapper) GetAllCreds(tenantID types.TenantID, id types.Uid, method string, validatedOnly bool) ([]types.Credential, error) {
	tenantAdp, err := tenantAuthAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantCredGetAll(tenantID, id, method, validatedOnly)
}

// DelCred deletes user's credentials. If method is "", all credentials are deleted.
func (usersMapper) DelCred(tenantID types.TenantID, id types.Uid, method, value string) error {
	tenantAdp, err := tenantAuthAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantCredDel(tenantID, id, method, value)
}

// GetUnreadCount returs users' total count of unread messages in all topics with the R permissions.
func (usersMapper) GetUnreadCount(tenantID types.TenantID, ids ...types.Uid) (map[types.Uid]int, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantUserUnreadCount(tenantID, ids...)
}

// GetUnvalidated returns a list of stale user ids which have unvalidated credentials,
// their auth levels and a comma-separated list of these credential names.
func (usersMapper) GetUnvalidated(tenantID types.TenantID, lastUpdatedBefore time.Time, limit int) ([]types.Uid, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantUserGetUnvalidated(tenantID, lastUpdatedBefore, limit)
}

// TopicsPersistenceInterface is an interface which defines methods for persistent storage of topics.
type TopicsPersistenceInterface interface {
	Create(tenantID types.TenantID, topic *types.Topic, owner types.Uid, private any) error
	CreateP2P(tenantID types.TenantID, initiator, invited *types.Subscription) error
	Get(tenantID types.TenantID, topic string) (*types.Topic, error)
	GetUsers(tenantID types.TenantID, topic string, opts *types.QueryOpt) ([]types.Subscription, error)
	GetUsersAny(tenantID types.TenantID, topic string, opts *types.QueryOpt) ([]types.Subscription, error)
	GetSubs(tenantID types.TenantID, topic string, opts *types.QueryOpt) ([]types.Subscription, error)
	GetSubsAny(tenantID types.TenantID, topic string, opts *types.QueryOpt) ([]types.Subscription, error)
	Update(tenantID types.TenantID, topic string, update map[string]any) error
	UpdateSubCnt(tenantID types.TenantID, topic string) error
	OwnerChange(tenantID types.TenantID, topic string, newOwner types.Uid) error
	Delete(tenantID types.TenantID, topic string, isChan, hard bool) error
}

// topicsMapper is a concrete type implementing TopicsPersistenceInterface.
type topicsMapper struct{}

// Topics is a singleton ancor object exporting TopicsPersistenceInterface methods.
var Topics TopicsPersistenceInterface

// Create creates a topic and owner's subscription to it.
func (topicsMapper) Create(tenantID types.TenantID, topic *types.Topic, owner types.Uid, private any) error {
	if topic == nil || topic.TenantID != tenantID {
		return types.ErrMalformed
	}
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}

	topic.InitTimes()
	topic.TouchedAt = topic.CreatedAt
	topic.Owner = owner.String()

	err = tenantAdp.TenantTopicCreate(tenantID, topic)
	if err != nil {
		return err
	}

	if !owner.IsZero() {
		err = Subs.Create(tenantID, &types.Subscription{
			ObjHeader: types.ObjHeader{CreatedAt: topic.CreatedAt},
			TenantID:  tenantID,
			User:      owner.String(),
			Topic:     topic.Id,
			ModeGiven: types.ModeCFull,
			ModeWant:  topic.GetAccess(owner),
			Private:   private})
	}

	return err
}

// CreateP2P creates a P2P topic by generating two user's subsciptions to each other.
func (topicsMapper) CreateP2P(tenantID types.TenantID, initiator, invited *types.Subscription) error {
	if initiator == nil || invited == nil || initiator.TenantID != tenantID || invited.TenantID != tenantID {
		return types.ErrMalformed
	}
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	initiator.InitTimes()
	initiator.SetTouchedAt(initiator.CreatedAt)
	invited.InitTimes()
	invited.SetTouchedAt(invited.CreatedAt)

	return tenantAdp.TenantTopicCreateP2P(tenantID, initiator, invited)
}

// Get a single topic with a list of relevant users de-normalized into it
func (topicsMapper) Get(tenantID types.TenantID, topic string) (*types.Topic, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	result, err := tenantAdp.TenantTopicGet(tenantID, topic)
	if err == nil && result != nil && result.TenantID != tenantID {
		return nil, types.ErrNotFound
	}
	return result, err
}

// GetUsers loads subscriptions for topic plus loads user.Public+Trusted.
// Deleted subscriptions are not loaded.
func (topicsMapper) GetUsers(tenantID types.TenantID, topic string, opts *types.QueryOpt) ([]types.Subscription, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantUsersForTopic(tenantID, topic, false, opts)
}

// GetUsersAny loads subscriptions for topic plus loads user.Public+Trusted. It's the same as GetUsers,
// except it loads deleted subscriptions too.
func (topicsMapper) GetUsersAny(tenantID types.TenantID, topic string, opts *types.QueryOpt) ([]types.Subscription, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantUsersForTopic(tenantID, topic, true, opts)
}

// GetSubs loads a list of subscriptions to the given topic, user.Public+Trusted and deleted
// subscriptions are not loaded. Suspended subscriptions are loaded.
func (topicsMapper) GetSubs(tenantID types.TenantID, topic string, opts *types.QueryOpt) ([]types.Subscription, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantSubsForTopic(tenantID, topic, false, opts)
}

// GetSubsAny loads a list of subscriptions to the given topic including deleted subscription.
// user.Public/Trusted are not loaded
func (topicsMapper) GetSubsAny(tenantID types.TenantID, topic string, opts *types.QueryOpt) ([]types.Subscription, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantSubsForTopic(tenantID, topic, true, opts)
}

// UpdateSubCnt refreshes subscriber count value denormalized in topic.
func (topicsMapper) UpdateSubCnt(tenantID types.TenantID, topic string) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantTopicUpdateSubCnt(tenantID, topic)
}

// Update is a generic topic update.
func (topicsMapper) Update(tenantID types.TenantID, topic string, update map[string]any) error {
	if _, ok := update["UpdatedAt"]; !ok {
		update["UpdatedAt"] = types.TimeNow()
	}
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantTopicUpdate(tenantID, topic, update)
}

// OwnerChange replaces the old topic owner with the new owner.
func (topicsMapper) OwnerChange(tenantID types.TenantID, topic string, newOwner types.Uid) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantTopicOwnerChange(tenantID, topic, newOwner)
}

// Delete deletes topic, messages, attachments, and subscriptions.
func (topicsMapper) Delete(tenantID types.TenantID, topic string, isChan, hard bool) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantTopicDelete(tenantID, topic, isChan, hard)
}

// SubsPersistenceInterface is an interface which defines methods for persistent storage of subscriptions.
type SubsPersistenceInterface interface {
	Create(tenantID types.TenantID, subs ...*types.Subscription) error
	Get(tenantID types.TenantID, topic string, user types.Uid, keepDeleted bool) (*types.Subscription, error)
	Update(tenantID types.TenantID, topic string, user types.Uid, update map[string]any) error
	Delete(tenantID types.TenantID, topic string, user types.Uid) error
}

// subsMapper is a concrete type implementing SubsPersistenceInterface.
type subsMapper struct{}

// Subs is a singleton ancor object exporting SubsPersistenceInterface.
var Subs SubsPersistenceInterface

// Create creates multiple subscriptions.
func (subsMapper) Create(tenantID types.TenantID, subs ...*types.Subscription) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		// Nothing to do.
		return nil
	}

	topic := subs[0].Topic
	if types.IsEphemeralTopic(topic) {
		// Ephemeral topics are not persisted in 'topics' table, don't try to update them.
		// Mixing ephemeral and real topics is not permitted.
		topic = ""
	}

	for _, sub := range subs {
		if sub == nil || sub.TenantID != tenantID {
			return types.ErrMalformed
		}
		sub.InitTimes()
		if topic != "" && sub.Topic != topic {
			return fmt.Errorf("all subscriptions must be for the same topic, got %s vs %s", sub.Topic, topic)
		}
	}

	return tenantAdp.TenantTopicShare(tenantID, topic, subs)
}

// Get subscription given topic and user ID.
func (subsMapper) Get(tenantID types.TenantID, topic string, user types.Uid, keepDeleted bool) (*types.Subscription, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	result, err := tenantAdp.TenantSubscriptionGet(tenantID, topic, user, keepDeleted)
	if err == nil && result != nil && result.TenantID != tenantID {
		return nil, types.ErrNotFound
	}
	return result, err
}

// Update values of topic's subscriptions.
func (subsMapper) Update(tenantID types.TenantID, topic string, user types.Uid, update map[string]any) error {
	update["UpdatedAt"] = types.TimeNow()
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantSubsUpdate(tenantID, topic, user, update)
}

// Delete deletes a subscription.
// To delete channel subscription the channel name must be explicitly specified.
func (subsMapper) Delete(tenantID types.TenantID, topic string, user types.Uid) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantSubsDelete(tenantID, topic, user)
}

// MessagesPersistenceInterface is an interface which defines methods for persistent storage of messages.
type MessagesPersistenceInterface interface {
	Save(tenantID types.TenantID, msg *types.Message, attachmentURLs []string, readBySender bool) (error, bool)
	DeleteList(tenantID types.TenantID, topic string, delID int, forUser types.Uid, msgDelAge time.Duration, ranges []types.Range) error
	GetAll(tenantID types.TenantID, topic string, forUser types.Uid, opt *types.QueryOpt) ([]types.Message, error)
	GetDeleted(tenantID types.TenantID, topic string, forUser types.Uid, opt *types.QueryOpt) ([]types.Range, int, error)
}

// messagesMapper is a concrete type implementing MessagesPersistenceInterface.
type messagesMapper struct{}

// Messages is a singleton ancor object for exporting MessagesPersistenceInterface.
var Messages MessagesPersistenceInterface

// Save message
func (messagesMapper) Save(tenantID types.TenantID, msg *types.Message, attachmentURLs []string, readBySender bool) (error, bool) {
	if msg == nil || msg.TenantID != tenantID {
		return types.ErrMalformed, false
	}
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err, false
	}
	msg.InitTimes()
	msg.SetUid(Store.GetUid())
	// Increment topic's or user's SeqId
	err = tenantAdp.TenantTopicUpdateOnMessage(tenantID, msg.Topic, msg)
	if err != nil {
		return err, false
	}

	err = tenantAdp.TenantMessageSave(tenantID, msg)
	if err != nil {
		return err, false
	}

	markedReadBySender := false
	// Mark message as read by the sender.
	if readBySender {
		// Make sure From is valid, otherwise we will reset values for all subscribers.
		fromUid := types.ParseUid(msg.From)
		if !fromUid.IsZero() {
			// Ignore the error here. It's not a big deal if it fails.
			if subErr := tenantAdp.TenantSubsUpdate(tenantID, msg.Topic, fromUid,
				map[string]any{
					"RecvSeqId": msg.SeqId,
					"ReadSeqId": msg.SeqId}); subErr != nil {
				logs.Warn.Printf("topic[%s]: failed to mark message (seq: %d) read by sender - err: %+v", msg.Topic, msg.SeqId, subErr)
			} else {
				markedReadBySender = true
			}
		}
	}

	if len(attachmentURLs) > 0 {
		var attachments []string
		for _, url := range attachmentURLs {
			// Convert attachment URLs to file IDs.
			if fid := mediaHandler.GetIdFromUrl(url); !fid.IsZero() {
				attachments = append(attachments, fid.String())
			}
		}
		if len(attachments) > 0 {
			return tenantAdp.TenantFileLinkAttachments(tenantID, "", types.ZeroUid, msg.Uid(), attachments), markedReadBySender
		}
	}

	return nil, markedReadBySender
}

// DeleteList deletes multiple messages defined by a list of ranges.
func (messagesMapper) DeleteList(tenantID types.TenantID, topic string, delID int, forUser types.Uid, msgDelAge time.Duration, ranges []types.Range) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	var toDel *types.DelMessage
	if delID > 0 {
		toDel = &types.DelMessage{
			TenantID:    tenantID,
			Topic:       topic,
			DelId:       delID,
			DeletedFor:  forUser.String(),
			SeqIdRanges: ranges}
		toDel.SetUid(Store.GetUid())
		toDel.InitTimes()
		if msgDelAge > 0 {
			toDel.SetNewerThan(toDel.CreatedAt.Add(-msgDelAge))
		}
	}

	err = tenantAdp.TenantMessageDeleteList(tenantID, topic, toDel)
	if err != nil {
		return err
	}

	// TODO: move to adapter.
	if delID > 0 {
		// Record ID of the delete transaction
		err = tenantAdp.TenantTopicUpdate(tenantID, topic, map[string]any{"DelId": delID})
		if err != nil {
			return err
		}

		// Soft-deleting will update one subscription, hard-deleting will ipdate all.
		// Soft- or hard- is defined by the forUser being defined.
		err = tenantAdp.TenantSubsUpdate(tenantID, topic, forUser, map[string]any{"DelId": delID})
		if err != nil {
			return err
		}
	}

	return err
}

// GetAll returns multiple messages.
func (messagesMapper) GetAll(tenantID types.TenantID, topic string, forUser types.Uid, opt *types.QueryOpt) ([]types.Message, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantMessageGetAll(tenantID, topic, forUser, opt)
}

// GetDeleted returns the ranges of deleted messages and the largest DelId reported in the list.
func (messagesMapper) GetDeleted(tenantID types.TenantID, topic string, forUser types.Uid, opt *types.QueryOpt) ([]types.Range, int, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, 0, err
	}
	dmsgs, err := tenantAdp.TenantMessageGetDeleted(tenantID, topic, forUser, opt)
	if err != nil {
		return nil, 0, err
	}

	var ranges []types.Range
	var maxID int
	// Flatten out the ranges
	for i := range dmsgs {
		dm := &dmsgs[i]
		if dm.DelId > maxID {
			maxID = dm.DelId
		}
		ranges = append(ranges, dm.SeqIdRanges...)
	}
	sort.Sort(types.RangeSorter(ranges))
	ranges = types.RangeSorter(ranges).Normalize()

	return ranges, maxID, nil
}

// Registered authentication handlers.
var authHandlers map[string]auth.AuthHandler

// Logical auth handler names
var authHandlerNames map[string]string

// RegisterAuthScheme registers an authentication scheme handler.
// The 'name' must be the hardcoded name, NOT the logical name.
func RegisterAuthScheme(name string, handler auth.AuthHandler) {
	if name == "" {
		panic("RegisterAuthScheme: empty auth scheme name")
	}
	if handler == nil {
		panic("RegisterAuthScheme: scheme handler is nil")
	}

	name = strings.ToLower(name)
	if authHandlers == nil {
		authHandlers = make(map[string]auth.AuthHandler)
	}
	if _, dup := authHandlers[name]; dup {
		panic("RegisterAuthScheme: called twice for scheme " + name)
	}
	authHandlers[name] = handler
}

// GetAuthNames returns all addressable auth handler names, logical and hardcoded
// excluding those which are disabled like "basic:".
func (s storeObj) GetAuthNames() []string {
	if len(authHandlers) == 0 {
		return nil
	}

	allNames := make(map[string]struct{})
	for name := range authHandlers {
		allNames[name] = struct{}{}
	}
	for name := range authHandlerNames {
		allNames[name] = struct{}{}
	}

	var names []string
	for name := range allNames {
		if s.GetLogicalAuthHandler(name) != nil {
			names = append(names, name)
		}
	}

	return names

}

// GetAuthHandler returns an auth handler by actual hardcoded name irrspectful of logical naming.
func (storeObj) GetAuthHandler(name string) auth.AuthHandler {
	return authHandlers[strings.ToLower(name)]
}

// GetLogicalAuthHandler returns an auth handler by logical name. If there is no handler by that
// logical name it tries to find one by the hardcoded name.
func (storeObj) GetLogicalAuthHandler(name string) auth.AuthHandler {
	name = strings.ToLower(name)
	if len(authHandlerNames) != 0 {
		if lname, ok := authHandlerNames[name]; ok {
			return authHandlers[lname]
		}
	}
	return authHandlers[name]
}

// InitAuthLogicalNames initializes authentication mapping "logical handler name":"actual handler name".
// Logical name must not be empty, actual name could be an empty string.
func InitAuthLogicalNames(config json.RawMessage) error {
	if config == nil || string(config) == "null" {
		return nil
	}
	var mapping []string
	if err := json.Unmarshal(config, &mapping); err != nil {
		return errors.New("store: failed to parse logical auth names: " + err.Error() + "(" + string(config) + ")")
	}
	if len(mapping) == 0 {
		return nil
	}

	if authHandlerNames == nil {
		authHandlerNames = make(map[string]string)
	}
	for _, pair := range mapping {
		if parts := strings.Split(pair, ":"); len(parts) == 2 {
			if parts[0] == "" {
				return errors.New("store: empty logical auth name '" + pair + "'")
			}
			parts[0] = strings.ToLower(parts[0])
			if _, ok := authHandlerNames[parts[0]]; ok {
				return errors.New("store: duplicate mapping for logical auth name '" + pair + "'")
			}
			parts[1] = strings.ToLower(parts[1])
			if parts[1] != "" {
				if _, ok := authHandlers[parts[1]]; !ok {
					return errors.New("store: unknown handler for logical auth name '" + pair + "'")
				}
			}
			if parts[0] == parts[1] {
				// Skip useless identity mapping.
				continue
			}
			authHandlerNames[parts[0]] = parts[1]
		} else {
			return errors.New("store: invalid logical auth mapping '" + pair + "'")
		}
	}
	return nil
}

// Registered authentication handlers.
var validators map[string]validate.Validator

// RegisterValidator registers validation scheme.
func RegisterValidator(name string, v validate.Validator) {
	name = strings.ToLower(name)
	if validators == nil {
		validators = make(map[string]validate.Validator)
	}

	if v == nil {
		panic("RegisterValidator: validator is nil")
	}
	if _, dup := validators[name]; dup {
		panic("RegisterValidator: called twice for validator " + name)
	}
	validators[name] = v
}

// GetValidator returns registered validator by name.
func (storeObj) GetValidator(name string) validate.Validator {
	return validators[strings.ToLower(name)]
}

// DevicePersistenceInterface is an interface which defines methods used for handling device IDs.
// Mostly used to generate push notifications.
type DevicePersistenceInterface interface {
	Update(tenantID types.TenantID, uid types.Uid, oldDeviceID string, dev *types.DeviceDef) error
	GetAll(tenantID types.TenantID, uid ...types.Uid) (map[types.Uid][]types.DeviceDef, int, error)
	Delete(tenantID types.TenantID, uid types.Uid, deviceID string) error
}

// deviceMapper is a concrete type implementing DevicePersistenceInterface.
type deviceMapper struct{}

// Devices is a singleton instance of DevicePersistenceInterface to map methods to.
var Devices DevicePersistenceInterface

// Update updates a device record.
func (deviceMapper) Update(tenantID types.TenantID, uid types.Uid, oldDeviceID string, dev *types.DeviceDef) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	// If the old device Id is specified and it's different from the new ID, delete the old id
	if oldDeviceID != "" && (dev == nil || dev.DeviceId != oldDeviceID) {
		if err := tenantAdp.TenantDeviceDelete(tenantID, uid, oldDeviceID); err != nil {
			return err
		}
	}

	// Insert or update the new DeviceId if one is given.
	if dev != nil && dev.DeviceId != "" {
		if dev.TenantID.IsZero() {
			dev.TenantID = tenantID
		}
		if dev.TenantID != tenantID {
			return types.ErrMalformed
		}
		return tenantAdp.TenantDeviceUpsert(tenantID, uid, dev)
	}
	return nil
}

// GetAll returns all known device IDs for a given list of user IDs.
// The second return parameter is the count of found device IDs.
func (deviceMapper) GetAll(tenantID types.TenantID, uid ...types.Uid) (map[types.Uid][]types.DeviceDef, int, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, 0, err
	}
	return tenantAdp.TenantDeviceGetAll(tenantID, uid...)
}

// Delete deletes device record for a given user.
func (deviceMapper) Delete(tenantID types.TenantID, uid types.Uid, deviceID string) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantDeviceDelete(tenantID, uid, deviceID)
}

// Registered media/file handlers.
var fileHandlers map[string]media.Handler

// RegisterMediaHandler saves reference to a media handler (file upload-download handler).
func RegisterMediaHandler(name string, mh media.Handler) {
	if fileHandlers == nil {
		fileHandlers = make(map[string]media.Handler)
	}

	if mh == nil {
		panic("RegisterMediaHandler: handler is nil")
	}
	if _, dup := fileHandlers[name]; dup {
		panic("RegisterMediaHandler: called twice for handler " + name)
	}
	fileHandlers[name] = mh
}

// GetMediaHandler returns default media handler.
func (storeObj) GetMediaHandler() media.Handler {
	return mediaHandler
}

// UseMediaHandler sets specified media handler as default.
func (storeObj) UseMediaHandler(name, config string) error {
	mediaHandler = fileHandlers[name]
	if mediaHandler == nil {
		panic("UseMediaHandler: unknown handler '" + name + "'")
	}
	return mediaHandler.Init(config)
}

// FilePersistenceInterface is an interface wchich defines methods used for file handling (records or uploaded files).
type FilePersistenceInterface interface {
	// StartUpload records that the given user initiated a file upload
	StartUpload(tenantID types.TenantID, fd *types.FileDef) error
	// FinishUpload marks started upload as successfully finished.
	FinishUpload(tenantID types.TenantID, fd *types.FileDef, success bool, size int64) (*types.FileDef, error)
	// Get fetches a file record for a unique file id.
	Get(tenantID types.TenantID, fid string) (*types.FileDef, error)
	// DeleteUnused removes unused attachments.
	DeleteUnused(tenantID types.TenantID, olderThan time.Time, limit int) error
	// LinkAttachments connects earlier uploaded attachments to a message or topic to prevent it
	// from being garbage collected.
	LinkAttachments(tenantID types.TenantID, topic string, msgId types.Uid, attachments []string) error
}

// fileMapper is concrete type which implements FilePersistenceInterface.
type fileMapper struct{}

// Files is a sigleton instance of FilePersistenceInterface to be used for handling file uploads.
var Files FilePersistenceInterface

// StartUpload records that the given user initiated a file upload
func (fileMapper) StartUpload(tenantID types.TenantID, fd *types.FileDef) error {
	if fd == nil || fd.TenantID != tenantID {
		return types.ErrMalformed
	}
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	fd.Status = types.UploadStarted
	return tenantAdp.TenantFileStartUpload(tenantID, fd)
}

// FinishUpload marks started upload as successfully finished or failed.
func (fileMapper) FinishUpload(tenantID types.TenantID, fd *types.FileDef, success bool, size int64) (*types.FileDef, error) {
	if fd == nil || fd.TenantID != tenantID {
		return nil, types.ErrMalformed
	}
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	return tenantAdp.TenantFileFinishUpload(tenantID, fd, success, size)
}

// Get fetches a file record for a unique file id.
func (fileMapper) Get(tenantID types.TenantID, fid string) (*types.FileDef, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return nil, err
	}
	fd, err := tenantAdp.TenantFileGet(tenantID, fid)
	if err == nil && fd != nil && fd.TenantID != tenantID {
		return nil, types.ErrNotFound
	}
	return fd, err
}

// DeleteUnused removes unused attachments and avatars.
func (fileMapper) DeleteUnused(tenantID types.TenantID, olderThan time.Time, limit int) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	toDel, err := tenantAdp.TenantFileDeleteUnused(tenantID, olderThan, limit)
	if err != nil {
		return err
	}
	if len(toDel) > 0 {
		logs.Warn.Println("deleting media", toDel)
		return Store.GetMediaHandler().Delete(toDel)
	}
	return nil
}

// LinkAttachments connects earlier uploaded attachments to a message or topic to prevent it
// from being garbage collected.
func (fileMapper) LinkAttachments(tenantID types.TenantID, topic string, msgId types.Uid, attachments []string) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	// Convert attachment URLs to file IDs.
	var fids []string
	for _, url := range attachments {
		if fid := mediaHandler.GetIdFromUrl(url); !fid.IsZero() {
			fids = append(fids, fid.String())
		}
	}

	if len(fids) > 0 {
		userId := types.ZeroUid
		if types.GetTopicCat(topic) == types.TopicCatMe {
			userId = types.ParseUserId(topic)
			topic = ""
		}
		return tenantAdp.TenantFileLinkAttachments(tenantID, topic, userId, msgId, fids)
	}
	return nil
}

// PersistentCacheInterface is an interface which defines methods used for accessing persistent key-value cache.
type PersistentCacheInterface interface {
	// Get reads a persistent cache entry.
	Get(tenantID types.TenantID, key string) (string, error)
	// Upsert creates or updates a persistent cache entry.
	Upsert(tenantID types.TenantID, key string, value string, failOnDuplicate bool) error
	// Delete deletes a single persistent cache entry.
	Delete(tenantID types.TenantID, key string) error
	// Expire expires older entries with the specified key prefix.
	Expire(tenantID types.TenantID, keyPrefix string, olderThan time.Time) error
}

// pcacheMapper is concrete type which implements PersistentCacheInterface.
type pcacheMapper struct{}

var PCache PersistentCacheInterface

// Get reads a persistent cache entry.
func (pcacheMapper) Get(tenantID types.TenantID, key string) (string, error) {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return "", err
	}
	return tenantAdp.TenantPCacheGet(tenantID, key)
}

// Upsert creates or updates a persistent cache entry.
func (pcacheMapper) Upsert(tenantID types.TenantID, key string, value string, failOnDuplicate bool) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantPCacheUpsert(tenantID, key, value, failOnDuplicate)
}

// Delete deletes a single persistent cache entry.
func (pcacheMapper) Delete(tenantID types.TenantID, key string) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantPCacheDelete(tenantID, key)
}

// Expire expires older entries with the specified key prefix.
func (pcacheMapper) Expire(tenantID types.TenantID, keyPrefix string, olderThan time.Time) error {
	tenantAdp, err := tenantBusinessAdapter(tenantID)
	if err != nil {
		return err
	}
	return tenantAdp.TenantPCacheExpire(tenantID, keyPrefix, olderThan)
}

func SetTestUidGenerator(g types.UidGenerator) {
	uGen = g
}

func init() {
	Store = storeObj{}
	Tenants = tenantsMapper{}
	Users = usersMapper{}
	Topics = topicsMapper{}
	Subs = subsMapper{}
	Messages = messagesMapper{}
	Devices = deviceMapper{}
	Files = fileMapper{}
	PCache = pcacheMapper{}
}
