package membergrid

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type managementEventAppender struct {
	err error
}

func (appender *managementEventAppender) Append(_ context.Context, _ eventport.Event) (eventport.EventID, error) {
	if appender.err != nil {
		return 0, appender.err
	}
	return 1, nil
}

type managementMemoryStore struct {
	mu sync.Mutex

	products       map[int64]bool
	activeStaff    map[int64]bool
	views          map[int64]SavedView
	collaborators  map[int64]Collaborator
	externalShares map[int64]ExternalShare
	receipts       map[string]MutationReceipt

	nextViewID         int64
	nextCollaboratorID int64
	nextReceiptID      int64
	failComplete       bool
	failCreateView     error
}

type managementMemorySnapshot struct {
	views              map[int64]SavedView
	collaborators      map[int64]Collaborator
	externalShares     map[int64]ExternalShare
	receipts           map[string]MutationReceipt
	nextViewID         int64
	nextCollaboratorID int64
	nextReceiptID      int64
}

type managementRollbackUOW struct {
	store *managementMemoryStore
	calls int
}

func (unit *managementRollbackUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	unit.calls++
	before := unit.store.snapshot()
	err := callback(ctx)
	if err != nil {
		unit.store.restore(before)
	}
	return err
}

func newManagementMemoryStore() *managementMemoryStore {
	return &managementMemoryStore{
		products: make(map[int64]bool), activeStaff: make(map[int64]bool), views: make(map[int64]SavedView),
		collaborators: make(map[int64]Collaborator), externalShares: make(map[int64]ExternalShare), receipts: make(map[string]MutationReceipt),
		nextViewID: 1, nextCollaboratorID: 1, nextReceiptID: 1,
	}
}

func newManagementTestService(t *testing.T, store *managementMemoryStore) (*ManagementService, *managementRollbackUOW) {
	t.Helper()
	unit := &managementRollbackUOW{store: store}
	service, err := NewManagementService(unit, store, &managementEventAppender{})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	var tick int64
	service.now = func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Minute)
	}
	return service, unit
}

func (store *managementMemoryStore) snapshot() managementMemorySnapshot {
	store.mu.Lock()
	defer store.mu.Unlock()
	return managementMemorySnapshot{
		views: cloneViewMap(store.views), collaborators: cloneCollaboratorMap(store.collaborators),
		externalShares: cloneExternalShareMap(store.externalShares),
		receipts:       cloneReceiptMap(store.receipts), nextViewID: store.nextViewID,
		nextCollaboratorID: store.nextCollaboratorID, nextReceiptID: store.nextReceiptID,
	}
}

func (store *managementMemoryStore) restore(snapshot managementMemorySnapshot) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.views = cloneViewMap(snapshot.views)
	store.collaborators = cloneCollaboratorMap(snapshot.collaborators)
	store.externalShares = cloneExternalShareMap(snapshot.externalShares)
	store.receipts = cloneReceiptMap(snapshot.receipts)
	store.nextViewID = snapshot.nextViewID
	store.nextCollaboratorID = snapshot.nextCollaboratorID
	store.nextReceiptID = snapshot.nextReceiptID
}

func (store *managementMemoryStore) ProductExists(_ context.Context, productID int64) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.products[productID], nil
}

func (store *managementMemoryStore) ActiveStaffExists(_ context.Context, staffID int64) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.activeStaff[staffID], nil
}

func (store *managementMemoryStore) ListSavedViews(_ context.Context, productID int64) ([]SavedView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]SavedView, 0)
	for _, view := range store.views {
		if view.ServiceProductID == productID {
			result = append(result, cloneSavedView(view))
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result, nil
}

func (store *managementMemoryStore) GetSavedViewForUpdate(_ context.Context, productID, viewID int64) (SavedView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	view, ok := store.views[viewID]
	if !ok || view.ServiceProductID != productID {
		return SavedView{}, ErrNotFound
	}
	return cloneSavedView(view), nil
}

func (store *managementMemoryStore) CreateSavedView(_ context.Context, record CreateSavedViewRecord) (SavedView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.failCreateView != nil {
		return SavedView{}, store.failCreateView
	}
	view := SavedView{
		ID: store.nextViewID, ServiceProductID: record.ServiceProductID, Name: record.Name, State: record.State,
		Sort: record.Sort, Columns: cloneColumnsSelection(record.Columns), SourceViewID: cloneOptionalID(record.SourceViewID),
		Version: 1, CreatedBy: record.CreatedBy, CreatedAt: record.CreatedAt.UTC(), UpdatedAt: record.CreatedAt.UTC(),
	}
	store.nextViewID++
	store.views[view.ID] = cloneSavedView(view)
	return cloneSavedView(view), nil
}

func (store *managementMemoryStore) UpdateSavedView(_ context.Context, record UpdateSavedViewRecord) (SavedView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	view, ok := store.views[record.ViewID]
	if !ok || view.ServiceProductID != record.ServiceProductID {
		return SavedView{}, ErrNotFound
	}
	if view.Version != record.ExpectedVersion {
		return SavedView{}, ErrConflict
	}
	view.Name = record.Name
	view.State = record.State
	view.Sort = record.Sort
	view.Columns = cloneColumnsSelection(record.Columns)
	view.Version++
	view.UpdatedAt = record.UpdatedAt.UTC()
	store.views[view.ID] = cloneSavedView(view)
	return cloneSavedView(view), nil
}

func (store *managementMemoryStore) DeleteSavedView(_ context.Context, productID, viewID, expectedVersion int64) (SavedView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	view, ok := store.views[viewID]
	if !ok || view.ServiceProductID != productID {
		return SavedView{}, ErrNotFound
	}
	if view.Version != expectedVersion {
		return SavedView{}, ErrConflict
	}
	delete(store.views, viewID)
	return cloneSavedView(view), nil
}

func (store *managementMemoryStore) ListCollaborators(_ context.Context, productID int64) ([]Collaborator, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]Collaborator, 0)
	for _, collaborator := range store.collaborators {
		if collaborator.ServiceProductID == productID {
			result = append(result, cloneCollaborator(collaborator))
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result, nil
}

func (store *managementMemoryStore) GetCollaboratorForUpdate(_ context.Context, productID, collaboratorID int64) (Collaborator, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	collaborator, ok := store.collaborators[collaboratorID]
	if !ok || collaborator.ServiceProductID != productID {
		return Collaborator{}, ErrNotFound
	}
	return cloneCollaborator(collaborator), nil
}

func (store *managementMemoryStore) CreateCollaborator(_ context.Context, record CreateCollaboratorRecord) (Collaborator, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, collaborator := range store.collaborators {
		if collaborator.ServiceProductID == record.ServiceProductID && collaborator.StaffID == record.StaffID {
			return Collaborator{}, ErrConflict
		}
	}
	collaborator := Collaborator{
		ID: store.nextCollaboratorID, ServiceProductID: record.ServiceProductID, StaffID: record.StaffID,
		Permission: record.Permission, Version: 1, InvitedBy: record.InvitedBy,
		CreatedAt: record.CreatedAt.UTC(), UpdatedAt: record.CreatedAt.UTC(),
	}
	store.nextCollaboratorID++
	store.collaborators[collaborator.ID] = cloneCollaborator(collaborator)
	return cloneCollaborator(collaborator), nil
}

func (store *managementMemoryStore) UpdateCollaborator(_ context.Context, record UpdateCollaboratorRecord) (Collaborator, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	collaborator, ok := store.collaborators[record.CollaboratorID]
	if !ok || collaborator.ServiceProductID != record.ServiceProductID {
		return Collaborator{}, ErrNotFound
	}
	if collaborator.Version != record.ExpectedVersion {
		return Collaborator{}, ErrConflict
	}
	collaborator.Permission = record.Permission
	collaborator.Version++
	collaborator.UpdatedAt = record.UpdatedAt.UTC()
	store.collaborators[collaborator.ID] = cloneCollaborator(collaborator)
	return cloneCollaborator(collaborator), nil
}

func (store *managementMemoryStore) DeleteCollaborator(_ context.Context, productID, collaboratorID, expectedVersion int64) (Collaborator, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	collaborator, ok := store.collaborators[collaboratorID]
	if !ok || collaborator.ServiceProductID != productID {
		return Collaborator{}, ErrNotFound
	}
	if collaborator.Version != expectedVersion {
		return Collaborator{}, ErrConflict
	}
	delete(store.collaborators, collaboratorID)
	return cloneCollaborator(collaborator), nil
}

func (store *managementMemoryStore) CurrentExternalShare(_ context.Context, productID int64) (ExternalShare, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if share, ok := store.externalShares[productID]; ok {
		return cloneExternalShare(share), nil
	}
	return ExternalShare{ServiceProductID: productID, Version: 0}, nil
}

func (store *managementMemoryStore) SetExternalShare(_ context.Context, record SetExternalShareRecord) (ExternalShare, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	current, ok := store.externalShares[record.ServiceProductID]
	if !ok {
		current = ExternalShare{ServiceProductID: record.ServiceProductID, Version: 0}
	}
	if current.Version != record.ExpectedVersion {
		return ExternalShare{}, ErrConflict
	}
	next := ExternalShare{
		ServiceProductID: record.ServiceProductID, ShareID: record.ShareID,
		Enabled: record.Enabled, Version: current.Version + 1,
	}
	store.externalShares[record.ServiceProductID] = cloneExternalShare(next)
	return cloneExternalShare(next), nil
}

func (store *managementMemoryStore) LookupEnabledExternalShare(_ context.Context, shareID string) (ExternalShare, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, share := range store.externalShares {
		if share.Enabled && share.ShareID == shareID {
			return cloneExternalShare(share), nil
		}
	}
	return ExternalShare{}, ErrNotFound
}

func (store *managementMemoryStore) ReserveMutationReceipt(_ context.Context, reservation MutationReceiptReservation) (MutationReceipt, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	key := receiptMemoryKey(reservation.Operation, reservation.ActorScope, reservation.KeyDigest)
	if receipt, ok := store.receipts[key]; ok {
		return cloneReceipt(receipt), false, nil
	}
	receipt := MutationReceipt{
		ID: store.nextReceiptID, Operation: reservation.Operation, ActorScope: reservation.ActorScope,
		KeyDigest: reservation.KeyDigest, PayloadDigest: reservation.PayloadDigest, State: "in_progress",
	}
	store.nextReceiptID++
	store.receipts[key] = cloneReceipt(receipt)
	return cloneReceipt(receipt), true, nil
}

func (store *managementMemoryStore) CompleteMutationReceipt(_ context.Context, receiptID int64, snapshot json.RawMessage, _ time.Time) (MutationReceipt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.failComplete {
		return MutationReceipt{}, errors.New("complete receipt failed")
	}
	for key, receipt := range store.receipts {
		if receipt.ID != receiptID || receipt.State != "in_progress" {
			continue
		}
		receipt.State = "completed"
		receipt.ResultSnapshot = append(json.RawMessage(nil), snapshot...)
		store.receipts[key] = cloneReceipt(receipt)
		return cloneReceipt(receipt), nil
	}
	return MutationReceipt{}, ErrConflict
}

func TestManagementSavedViewLifecycleCloneScopeCASAndIdempotency(t *testing.T) {
	store := newManagementMemoryStore()
	store.products[10] = true
	store.products[20] = true
	service, _ := newManagementTestService(t, store)

	create := CreateSavedViewCommand{
		ServiceProductID: 10, ExpectedVersion: 0, Name: "活跃会员", State: StateActive,
		Sort: ViewSortGrantedAtDesc, Columns: []string{"entitlement_id", "state", "display_name"},
		ActorID: 99, IdempotencyKey: "view-create-key-0001",
	}
	first, err := service.CreateSavedView(context.Background(), create)
	if err != nil {
		t.Fatal(err)
	}
	if !first.OK || first.View.ID != 1 || first.View.Version != 1 || first.View.SourceViewID != nil {
		t.Fatalf("created=%+v", first)
	}

	replayed, err := service.CreateSavedView(context.Background(), create)
	if err != nil || !reflect.DeepEqual(replayed, first) {
		t.Fatalf("replay=%+v err=%v want=%+v", replayed, err, first)
	}
	create.Name = "不同负载"
	if _, err = service.CreateSavedView(context.Background(), create); !errors.Is(err, ErrConflict) {
		t.Fatalf("same key/different payload error=%v", err)
	}

	sourceID := first.View.ID
	cloneResponse, err := service.CreateSavedView(context.Background(), CreateSavedViewCommand{
		ServiceProductID: 10, ExpectedVersion: 0, Name: "活跃会员副本", SourceViewID: &sourceID,
		ActorID: 99, IdempotencyKey: "view-clone-key-00001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cloneResponse.View.SourceViewID == nil || *cloneResponse.View.SourceViewID != sourceID ||
		cloneResponse.View.State != first.View.State || cloneResponse.View.Sort != first.View.Sort ||
		!reflect.DeepEqual(cloneResponse.View.Columns, first.View.Columns) {
		t.Fatalf("clone=%+v source=%+v", cloneResponse, first)
	}

	if _, err = service.CreateSavedView(context.Background(), CreateSavedViewCommand{
		ServiceProductID: 20, ExpectedVersion: 0, Name: "跨商品副本", SourceViewID: &sourceID,
		ActorID: 99, IdempotencyKey: "view-cross-clone-001",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-product clone error=%v", err)
	}

	if _, err = service.UpdateSavedView(context.Background(), UpdateSavedViewCommand{
		ServiceProductID: 20, ViewID: first.View.ID, ExpectedVersion: 1, Name: "跨商品更新",
		State: StateAll, Sort: ViewSortGrantedAtDesc, Columns: []string{"state"}, ActorID: 99,
		IdempotencyKey: "view-cross-update-01",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-product update error=%v", err)
	}
	if _, err = service.DeleteSavedView(context.Background(), DeleteSavedViewCommand{
		ServiceProductID: 20, ViewID: first.View.ID, ExpectedVersion: 1, ActorID: 99,
		IdempotencyKey: "view-cross-delete-01",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-product delete error=%v", err)
	}

	updated, err := service.UpdateSavedView(context.Background(), UpdateSavedViewCommand{
		ServiceProductID: 10, ViewID: first.View.ID, ExpectedVersion: 1, Name: "全部会员",
		State: StateAll, Sort: ViewSortGrantedAtDesc, Columns: []string{"state", "granted_at"}, ActorID: 99,
		IdempotencyKey: "view-update-key-0001",
	})
	if err != nil || updated.View.Version != 2 || updated.View.Name != "全部会员" {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	updateCommand := UpdateSavedViewCommand{
		ServiceProductID: 10, ViewID: first.View.ID, ExpectedVersion: 1, Name: "全部会员",
		State: StateAll, Sort: ViewSortGrantedAtDesc, Columns: []string{"state", "granted_at"}, ActorID: 99,
		IdempotencyKey: "view-update-key-0001",
	}
	replayedUpdate, err := service.UpdateSavedView(context.Background(), updateCommand)
	if err != nil || !reflect.DeepEqual(replayedUpdate, updated) {
		t.Fatalf("update replay=%+v err=%v want=%+v", replayedUpdate, err, updated)
	}
	updateCommand.Name = "同键不同更新负载"
	if _, err = service.UpdateSavedView(context.Background(), updateCommand); !errors.Is(err, ErrConflict) {
		t.Fatalf("update same-key conflict error=%v", err)
	}
	if _, err = service.UpdateSavedView(context.Background(), UpdateSavedViewCommand{
		ServiceProductID: 10, ViewID: first.View.ID, ExpectedVersion: 1, Name: "过期版本",
		State: StateAll, Sort: ViewSortGrantedAtDesc, Columns: []string{"state"}, ActorID: 99,
		IdempotencyKey: "view-stale-update-001",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update error=%v", err)
	}

	deleteCommand := DeleteSavedViewCommand{
		ServiceProductID: 10, ViewID: first.View.ID, ExpectedVersion: 2, ActorID: 99,
		IdempotencyKey: "view-delete-key-0001",
	}
	deleted, err := service.DeleteSavedView(context.Background(), deleteCommand)
	if err != nil || !deleted.OK || !deleted.Deleted || deleted.View.Version != 2 {
		t.Fatalf("deleted=%+v err=%v", deleted, err)
	}
	replayedDelete, err := service.DeleteSavedView(context.Background(), deleteCommand)
	if err != nil || !reflect.DeepEqual(replayedDelete, deleted) {
		t.Fatalf("delete replay=%+v err=%v want=%+v", replayedDelete, err, deleted)
	}
	if _, err = service.DeleteSavedView(context.Background(), DeleteSavedViewCommand{ViewID: 0}); !errors.Is(err, ErrBuiltInView) {
		t.Fatalf("built-in delete error=%v", err)
	}
	if _, err = service.UpdateSavedView(context.Background(), UpdateSavedViewCommand{ViewID: 0}); !errors.Is(err, ErrBuiltInView) {
		t.Fatalf("built-in update error=%v", err)
	}
}

func TestManagementRejectsUnsafeColumnsAndArbitraryConfiguration(t *testing.T) {
	for name, columns := range map[string][]string{
		"customer id":       {"customer_id"},
		"unionid":           {"unionid"},
		"raw mobile":        {"mobile"},
		"external identity": {"external_userid"},
		"opaque":            {"opaque"},
		"sql expression":    {"state DESC; DROP TABLE customers"},
		"duplicate":         {"state", "state"},
		"empty":             {},
	} {
		t.Run(name, func(t *testing.T) {
			store := newManagementMemoryStore()
			store.products[1] = true
			service, _ := newManagementTestService(t, store)
			_, err := service.CreateSavedView(context.Background(), CreateSavedViewCommand{
				ServiceProductID: 1, ExpectedVersion: 0, Name: "安全视图", State: StateAll,
				Sort: ViewSortGrantedAtDesc, Columns: columns, ActorID: 1, IdempotencyKey: strings.Repeat("u", 20),
			})
			if !errors.Is(err, ErrInvalidManagementInput) {
				t.Fatalf("columns=%v error=%v", columns, err)
			}
			if len(store.views) != 0 || len(store.receipts) != 0 {
				t.Fatalf("invalid input mutated state: views=%d receipts=%d", len(store.views), len(store.receipts))
			}
		})
	}
}

func TestManagementSavedViewsRejectCanonicalOnlyStates(t *testing.T) {
	store := newManagementMemoryStore()
	store.products[1] = true
	service, _ := newManagementTestService(t, store)
	for _, state := range []StateFilter{StateExpired, StateRemoved} {
		if _, err := service.CreateSavedView(context.Background(), CreateSavedViewCommand{
			ServiceProductID: 1, ExpectedVersion: 0, Name: "旧视图", State: state,
			Sort: ViewSortGrantedAtDesc, Columns: []string{"state"}, ActorID: 1, IdempotencyKey: "legacy-state-create-" + string(state),
		}); !errors.Is(err, ErrInvalidManagementInput) {
			t.Fatalf("create state=%q error=%v", state, err)
		}
	}
	created, err := service.CreateSavedView(context.Background(), CreateSavedViewCommand{
		ServiceProductID: 1, ExpectedVersion: 0, Name: "旧视图", State: StateActive,
		Sort: ViewSortGrantedAtDesc, Columns: []string{"state"}, ActorID: 1, IdempotencyKey: "legacy-state-create-active",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []StateFilter{StateExpired, StateRemoved} {
		if _, err = service.UpdateSavedView(context.Background(), UpdateSavedViewCommand{
			ServiceProductID: 1, ViewID: created.View.ID, ExpectedVersion: created.View.Version, Name: "旧视图", State: state,
			Sort: ViewSortGrantedAtDesc, Columns: []string{"state"}, ActorID: 1, IdempotencyKey: "legacy-state-update-" + string(state),
		}); !errors.Is(err, ErrInvalidManagementInput) {
			t.Fatalf("update state=%q error=%v", state, err)
		}
	}
}

func TestManagementCollaboratorLifecycleActiveUniqueScopeAndShareSettings(t *testing.T) {
	store := newManagementMemoryStore()
	store.products[1] = true
	store.products[2] = true
	store.activeStaff[7] = true
	store.activeStaff[8] = false
	service, _ := newManagementTestService(t, store)

	if _, err := service.CreateCollaborator(context.Background(), CreateCollaboratorCommand{
		ServiceProductID: 1, ExpectedVersion: 0, StaffID: 8, Permission: CollaboratorPermissionView,
		ActorID: 3, IdempotencyKey: "inactive-staff-key-01",
	}); !errors.Is(err, ErrInactiveStaff) {
		t.Fatalf("inactive staff error=%v", err)
	}

	createCommand := CreateCollaboratorCommand{
		ServiceProductID: 1, ExpectedVersion: 0, StaffID: 7, Permission: CollaboratorPermissionView,
		ActorID: 3, IdempotencyKey: "collaborator-create-01",
	}
	created, err := service.CreateCollaborator(context.Background(), createCommand)
	if err != nil || !created.OK || created.Collaborator.Version != 1 ||
		!created.EditPermissionIsLocalMetadataOnly || created.GrantsCentralProductsPermission {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	replayed, err := service.CreateCollaborator(context.Background(), createCommand)
	if err != nil || !reflect.DeepEqual(replayed, created) {
		t.Fatalf("collaborator replay=%+v err=%v want=%+v", replayed, err, created)
	}
	differentPayload := createCommand
	differentPayload.Permission = CollaboratorPermissionEdit
	if _, err = service.CreateCollaborator(context.Background(), differentPayload); !errors.Is(err, ErrConflict) {
		t.Fatalf("collaborator same-key conflict error=%v", err)
	}
	if _, err = service.CreateCollaborator(context.Background(), CreateCollaboratorCommand{
		ServiceProductID: 1, ExpectedVersion: 0, StaffID: 7, Permission: CollaboratorPermissionEdit,
		ActorID: 3, IdempotencyKey: strings.Repeat("c", 20),
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("unique collaborator error=%v", err)
	}
	if _, err = service.UpdateCollaborator(context.Background(), UpdateCollaboratorCommand{
		ServiceProductID: 2, CollaboratorID: created.Collaborator.ID, ExpectedVersion: 1,
		Permission: CollaboratorPermissionEdit, ActorID: 3, IdempotencyKey: "collaborator-cross-001",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-product collaborator update error=%v", err)
	}
	if _, err = service.DeleteCollaborator(context.Background(), DeleteCollaboratorCommand{
		ServiceProductID: 2, CollaboratorID: created.Collaborator.ID, ExpectedVersion: 1,
		ActorID: 3, IdempotencyKey: "collaborator-cross-del-01",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-product collaborator delete error=%v", err)
	}

	updated, err := service.UpdateCollaborator(context.Background(), UpdateCollaboratorCommand{
		ServiceProductID: 1, CollaboratorID: created.Collaborator.ID, ExpectedVersion: 1,
		Permission: CollaboratorPermissionEdit, ActorID: 3, IdempotencyKey: "collaborator-update-01",
	})
	if err != nil || updated.Collaborator.Version != 2 || updated.Collaborator.Permission != CollaboratorPermissionEdit ||
		updated.GrantsCentralProductsPermission {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	updateCommand := UpdateCollaboratorCommand{
		ServiceProductID: 1, CollaboratorID: created.Collaborator.ID, ExpectedVersion: 1,
		Permission: CollaboratorPermissionEdit, ActorID: 3, IdempotencyKey: "collaborator-update-01",
	}
	replayedUpdate, err := service.UpdateCollaborator(context.Background(), updateCommand)
	if err != nil || !reflect.DeepEqual(replayedUpdate, updated) {
		t.Fatalf("collaborator update replay=%+v err=%v want=%+v", replayedUpdate, err, updated)
	}
	updateCommand.Permission = CollaboratorPermissionView
	if _, err = service.UpdateCollaborator(context.Background(), updateCommand); !errors.Is(err, ErrConflict) {
		t.Fatalf("collaborator update same-key conflict error=%v", err)
	}
	if _, err = service.UpdateCollaborator(context.Background(), UpdateCollaboratorCommand{
		ServiceProductID: 1, CollaboratorID: created.Collaborator.ID, ExpectedVersion: 1,
		Permission: CollaboratorPermissionView, ActorID: 3, IdempotencyKey: "collaborator-stale-001",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale collaborator update error=%v", err)
	}

	settings, err := service.ShareSettings(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if settings.ExternalShareSupported || settings.ExternalShareEnabled || settings.RealExternalCallExecuted ||
		!settings.CollaboratorEditIsLocalMetadataOnly || settings.CollaboratorEditGrantsCentralPermission ||
		len(settings.Collaborators) != 1 || settings.Collaborators[0].Permission != CollaboratorPermissionEdit {
		t.Fatalf("settings=%+v", settings)
	}

	store.activeStaff[7] = false
	if _, err = service.UpdateCollaborator(context.Background(), UpdateCollaboratorCommand{
		ServiceProductID: 1, CollaboratorID: created.Collaborator.ID, ExpectedVersion: 2,
		Permission: CollaboratorPermissionView, ActorID: 3, IdempotencyKey: "inactive-update-key-01",
	}); !errors.Is(err, ErrInactiveStaff) {
		t.Fatalf("inactive update error=%v", err)
	}
	deleteCommand := DeleteCollaboratorCommand{
		ServiceProductID: 1, CollaboratorID: created.Collaborator.ID, ExpectedVersion: 2,
		ActorID: 3, IdempotencyKey: "collaborator-delete-01",
	}
	deleted, err := service.DeleteCollaborator(context.Background(), deleteCommand)
	if err != nil || !deleted.Deleted || !deleted.EditPermissionIsLocalMetadataOnly || deleted.GrantsCentralProductsPermission {
		t.Fatalf("deleted=%+v err=%v", deleted, err)
	}
	replayedDelete, err := service.DeleteCollaborator(context.Background(), deleteCommand)
	if err != nil || !reflect.DeepEqual(replayedDelete, deleted) {
		t.Fatalf("collaborator delete replay=%+v err=%v want=%+v", replayedDelete, err, deleted)
	}
}

func TestManagementTransactionRollbackIncludesMutationAndReceipt(t *testing.T) {
	store := newManagementMemoryStore()
	store.products[1] = true
	store.failComplete = true
	service, unit := newManagementTestService(t, store)
	command := CreateSavedViewCommand{
		ServiceProductID: 1, ExpectedVersion: 0, Name: "回滚视图", State: StateAll,
		Sort: ViewSortGrantedAtDesc, Columns: []string{"state"}, ActorID: 1,
		IdempotencyKey: "rollback-view-key-001",
	}
	if _, err := service.CreateSavedView(context.Background(), command); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("completion failure error=%v", err)
	}
	if len(store.views) != 0 || len(store.receipts) != 0 || store.nextViewID != 1 || store.nextReceiptID != 1 {
		t.Fatalf("transaction not rolled back: views=%d receipts=%d next_view=%d next_receipt=%d",
			len(store.views), len(store.receipts), store.nextViewID, store.nextReceiptID)
	}
	store.failComplete = false
	created, err := service.CreateSavedView(context.Background(), command)
	if err != nil || created.View.ID != 1 || len(store.receipts) != 1 {
		t.Fatalf("retry after rollback=%+v err=%v receipts=%d", created, err, len(store.receipts))
	}
	if unit.calls != 2 {
		t.Fatalf("uow calls=%d", unit.calls)
	}
}

func TestManagementEventAppendFailureRollsBackBusinessFactAndReceipt(t *testing.T) {
	store := newManagementMemoryStore()
	store.products[1] = true
	unit := &managementRollbackUOW{store: store}
	service, err := NewManagementService(unit, store, &managementEventAppender{err: errors.New("event append unavailable")})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 8, 22, 3, 0, 0, 0, time.UTC) }

	_, err = service.CreateSavedView(context.Background(), CreateSavedViewCommand{
		ServiceProductID: 1, ExpectedVersion: 0, Name: "事件回滚视图", State: StateAll,
		Sort: ViewSortGrantedAtDesc, Columns: []string{"state"}, ActorID: 1,
		IdempotencyKey: strings.Repeat("e", 20),
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("event append failure error=%v", err)
	}
	if len(store.views) != 0 || len(store.receipts) != 0 || store.nextViewID != 1 || store.nextReceiptID != 1 {
		t.Fatalf("event append failure did not roll back: views=%d receipts=%d next_view=%d next_receipt=%d",
			len(store.views), len(store.receipts), store.nextViewID, store.nextReceiptID)
	}
}

func TestManagementCollaboratorTransactionRollbackIncludesReceipt(t *testing.T) {
	store := newManagementMemoryStore()
	store.products[1] = true
	store.activeStaff[7] = true
	store.failComplete = true
	service, _ := newManagementTestService(t, store)
	command := CreateCollaboratorCommand{
		ServiceProductID: 1, ExpectedVersion: 0, StaffID: 7, Permission: CollaboratorPermissionEdit,
		ActorID: 3, IdempotencyKey: "collab-rollback-key-001",
	}
	if _, err := service.CreateCollaborator(context.Background(), command); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("completion failure error=%v", err)
	}
	if len(store.collaborators) != 0 || len(store.receipts) != 0 || store.nextCollaboratorID != 1 || store.nextReceiptID != 1 {
		t.Fatalf("transaction not rolled back: collaborators=%d receipts=%d next_collaborator=%d next_receipt=%d",
			len(store.collaborators), len(store.receipts), store.nextCollaboratorID, store.nextReceiptID)
	}
}

func TestShareSettingsMissingProductAndStableEmptyCollections(t *testing.T) {
	store := newManagementMemoryStore()
	service, _ := newManagementTestService(t, store)
	if _, err := service.ShareSettings(context.Background(), 404); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing product error=%v", err)
	}
	store.products[1] = true
	settings, err := service.ShareSettings(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if settings.SavedViews == nil || settings.Collaborators == nil || len(settings.SavedViews) != 0 || len(settings.Collaborators) != 0 {
		t.Fatalf("empty collections=%+v", settings)
	}
}

func TestManagementReplaySnapshotIsClosedAndCommandScoped(t *testing.T) {
	store := newManagementMemoryStore()
	store.products[1] = true
	service, _ := newManagementTestService(t, store)
	command := CreateSavedViewCommand{
		ServiceProductID: 1, ExpectedVersion: 0, Name: "回放范围视图", State: StateAll,
		Sort: ViewSortGrantedAtDesc, Columns: []string{"state"}, ActorID: 9,
		IdempotencyKey: "snapshot-scope-key-001",
	}
	created, err := service.CreateSavedView(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	reservationKey := receiptMemoryKey(
		mutationOperationCreate,
		"membergrid:"+snapshotViewCreated+":actor:9",
		sha256.Sum256([]byte(command.IdempotencyKey)),
	)
	receipt := store.receipts[reservationKey]

	var snapshot map[string]any
	if err = json.Unmarshal(receipt.ResultSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot["unexpected"] = "opaque"
	receipt.ResultSnapshot, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	store.receipts[reservationKey] = receipt
	if _, err = service.CreateSavedView(context.Background(), command); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unknown replay field error=%v", err)
	}

	closed := mutationSnapshot{Kind: snapshotViewCreated, Status: 201, View: &created.View}
	closed.View.ServiceProductID = 2
	receipt.ResultSnapshot, err = json.Marshal(closed)
	if err != nil {
		t.Fatal(err)
	}
	store.receipts[reservationKey] = receipt
	if _, err = service.CreateSavedView(context.Background(), command); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cross-product replay snapshot error=%v", err)
	}
}

func cloneViewMap(source map[int64]SavedView) map[int64]SavedView {
	result := make(map[int64]SavedView, len(source))
	for key, value := range source {
		result[key] = cloneSavedView(value)
	}
	return result
}

func cloneCollaboratorMap(source map[int64]Collaborator) map[int64]Collaborator {
	result := make(map[int64]Collaborator, len(source))
	for key, value := range source {
		result[key] = cloneCollaborator(value)
	}
	return result
}

func cloneExternalShareMap(source map[int64]ExternalShare) map[int64]ExternalShare {
	result := make(map[int64]ExternalShare, len(source))
	for key, value := range source {
		result[key] = cloneExternalShare(value)
	}
	return result
}

func cloneReceiptMap(source map[string]MutationReceipt) map[string]MutationReceipt {
	result := make(map[string]MutationReceipt, len(source))
	for key, value := range source {
		result[key] = cloneReceipt(value)
	}
	return result
}

func cloneReceipt(receipt MutationReceipt) MutationReceipt {
	receipt.ResultSnapshot = append(json.RawMessage(nil), receipt.ResultSnapshot...)
	return receipt
}

func receiptMemoryKey(operation, actorScope string, digest [32]byte) string {
	return operation + "|" + actorScope + "|" + hex.EncodeToString(digest[:])
}
