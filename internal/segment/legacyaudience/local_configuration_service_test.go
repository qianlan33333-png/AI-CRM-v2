package legacyaudience

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type localConfigurationWorld struct {
	base             *memoryWorld
	bindings         map[int64]AutomationBinding
	senders          map[int64][]PackageSender
	configs          map[int64]ConfigurationVersion
	receipts         map[string]Receipt
	entries          []contactport.StaffDirectoryEntry
	operationMembers []OperationMember
	agents           map[int64]AutomationAgent
	nextID           int64
}

func newLocalConfigurationWorld() *localConfigurationWorld {
	return &localConfigurationWorld{
		base: newMemoryWorld(), bindings: make(map[int64]AutomationBinding), senders: make(map[int64][]PackageSender),
		receipts: make(map[string]Receipt), configs: make(map[int64]ConfigurationVersion), nextID: 1,
		entries: []contactport.StaffDirectoryEntry{{WeComUserID: "beta", DisplayName: "Beta"}, {WeComUserID: "alpha", DisplayName: "Alpha"}},
		agents:  map[int64]AutomationAgent{7: {ID: 7, Status: "active"}, 8: {ID: 8, Status: "paused"}, 9: {ID: 9, Status: "archived"}},
	}
}

func (world *localConfigurationWorld) Within(ctx context.Context, callback func(context.Context) error) error {
	return world.base.Within(ctx, callback)
}
func (world *localConfigurationWorld) GetPackageMetadata(ctx context.Context, id int64) (PackageMetadata, error) {
	return world.base.GetPackageMetadata(ctx, id)
}
func (world *localConfigurationWorld) LockPackage(ctx context.Context, id int64) (PackageWriteModel, error) {
	return world.base.LockPackage(ctx, id)
}
func (world *localConfigurationWorld) GetAutomationBinding(ctx context.Context, id int64) (*AutomationBinding, error) {
	if err := requireTransaction(ctx); err != nil {
		return nil, err
	}
	binding, found := world.bindings[id]
	if !found {
		return nil, nil
	}
	copy := binding
	return &copy, nil
}
func (world *localConfigurationWorld) SaveAutomationBinding(ctx context.Context, value AutomationBinding, actor, expectedVersion int64, now time.Time) (AutomationBinding, error) {
	if err := requireTransaction(ctx); err != nil {
		return AutomationBinding{}, err
	}
	if existing, found := world.bindings[value.PackageID]; found {
		if existing.Version != expectedVersion {
			return AutomationBinding{}, ErrVersionConflict
		}
		value.CreatedBy, value.CreatedAt = existing.CreatedBy, existing.CreatedAt
		value.Version = existing.Version + 1
	} else {
		if expectedVersion != 0 {
			return AutomationBinding{}, ErrVersionConflict
		}
		value.CreatedBy, value.CreatedAt = actor, now
		value.Version = 1
	}
	value.UpdatedBy, value.UpdatedAt = actor, now
	world.bindings[value.PackageID] = value
	return value, nil
}
func (world *localConfigurationWorld) DeleteAutomationBinding(ctx context.Context, id, expectedVersion int64) (bool, error) {
	if err := requireTransaction(ctx); err != nil {
		return false, err
	}
	binding, found := world.bindings[id]
	if !found {
		return false, nil
	}
	if binding.Version != expectedVersion {
		return false, ErrVersionConflict
	}
	delete(world.bindings, id)
	return true, nil
}
func (world *localConfigurationWorld) ListPackageSenders(ctx context.Context, id int64) ([]PackageSender, error) {
	if ctx == nil {
		return nil, ErrUnavailable
	}
	return clonePackageSenders(world.senders[id]), nil
}
func (world *localConfigurationWorld) ReplacePackageSenders(ctx context.Context, id int64, items []PackageSender, _ int64, _ time.Time) ([]PackageSender, bool, error) {
	if err := requireTransaction(ctx); err != nil {
		return nil, false, err
	}
	current := clonePackageSenders(world.senders[id])
	if samePackageSenders(current, items) {
		return current, false, nil
	}
	world.senders[id] = clonePackageSenders(items)
	return clonePackageSenders(items), true, nil
}
func (world *localConfigurationWorld) LockEligibleStaffByWeComUserID(ctx context.Context, id string) (contactport.StaffDirectoryEntry, error) {
	if err := requireTransaction(ctx); err != nil {
		return contactport.StaffDirectoryEntry{}, err
	}
	for _, entry := range world.entries {
		if entry.WeComUserID == id {
			return entry, nil
		}
	}
	return contactport.StaffDirectoryEntry{}, contactport.ErrStaffReferenceNotFound
}
func (world *localConfigurationWorld) ReserveConfigurationReceipt(ctx context.Context, wanted ReceiptReservation) (Receipt, bool, error) {
	if err := requireTransaction(ctx); err != nil {
		return Receipt{}, false, err
	}
	key := fmt.Sprintf("%s:%d:%x", wanted.Operation, wanted.ActorID, wanted.KeyDigest)
	if receipt, found := world.receipts[key]; found {
		return receipt, false, nil
	}
	receipt := Receipt{ID: world.nextID, Operation: wanted.Operation, ActorID: wanted.ActorID, KeyDigest: wanted.KeyDigest, PayloadDigest: wanted.PayloadDigest, State: "in_progress"}
	world.nextID++
	world.receipts[key] = receipt
	return receipt, true, nil
}
func (world *localConfigurationWorld) CompleteConfigurationReceipt(ctx context.Context, id int64, result json.RawMessage, _ time.Time) (Receipt, error) {
	if err := requireTransaction(ctx); err != nil {
		return Receipt{}, err
	}
	for key, receipt := range world.receipts {
		if receipt.ID == id && receipt.State == "in_progress" {
			receipt.State, receipt.ResultJSON = "completed", append(json.RawMessage(nil), result...)
			world.receipts[key] = receipt
			return receipt, nil
		}
	}
	return Receipt{}, ErrConflict
}
func (world *localConfigurationWorld) GetCurrentConfiguration(_ context.Context, packageID int64) (*ConfigurationVersion, error) {
	value, found := world.configs[packageID]
	if !found {
		return nil, nil
	}
	return cloneConfigurationVersion(&value), nil
}
func (world *localConfigurationWorld) GetConfigurationVersion(_ context.Context, packageID, version int64) (*ConfigurationVersion, error) {
	value, found := world.configs[packageID]
	if !found || value.Version != version {
		return nil, nil
	}
	return cloneConfigurationVersion(&value), nil
}
func (world *localConfigurationWorld) InsertConfigurationVersion(ctx context.Context, value ConfigurationVersion) (ConfigurationVersion, error) {
	if err := requireTransaction(ctx); err != nil {
		return ConfigurationVersion{}, err
	}
	if current, found := world.configs[value.PackageID]; found && current.Version+1 != value.Version {
		return ConfigurationVersion{}, ErrVersionConflict
	}
	world.configs[value.PackageID] = *cloneConfigurationVersion(&value)
	return *cloneConfigurationVersion(&value), nil
}
func (world *localConfigurationWorld) ListOperationMembers(context.Context) ([]OperationMember, error) {
	return append([]OperationMember(nil), world.operationMembers...), nil
}
func (world *localConfigurationWorld) ReplaceOperationMembers(ctx context.Context, items []OperationMember, _ time.Time) ([]OperationMember, error) {
	if err := requireTransaction(ctx); err != nil {
		return nil, err
	}
	world.operationMembers = append([]OperationMember(nil), items...)
	return append([]OperationMember(nil), world.operationMembers...), nil
}
func (world *localConfigurationWorld) GetAutomationAgent(ctx context.Context, id int64) (AutomationAgent, error) {
	if err := requireTransaction(ctx); err != nil {
		return AutomationAgent{}, err
	}
	agent, found := world.agents[id]
	if !found {
		return AutomationAgent{}, ErrNotFound
	}
	return agent, nil
}
func (world *localConfigurationWorld) ListEligibleStaff(context.Context) ([]contactport.StaffDirectoryEntry, error) {
	result := make([]contactport.StaffDirectoryEntry, len(world.entries))
	copy(result, world.entries)
	return result, nil
}
func (world *localConfigurationWorld) Append(ctx context.Context, event LocalEvent) error {
	return world.base.Append(ctx, event)
}
func (world *localConfigurationWorld) Preview(ctx context.Context, definition segmentport.Definition, reference time.Time) (segmentport.DefinitionEvaluation, error) {
	if err := requireTransaction(ctx); err != nil {
		return segmentport.DefinitionEvaluation{}, err
	}
	return segmentport.DefinitionEvaluation{MemberCount: 2, MemberDigest: sha256.Sum256([]byte(definition)), EvaluatedAt: reference}, nil
}
func (world *localConfigurationWorld) Materialize(ctx context.Context, id segmentport.SegmentID, definition segmentport.Definition, reference time.Time) (segmentport.DefinitionEvaluation, error) {
	if int64(id) != 101 {
		return segmentport.DefinitionEvaluation{}, ErrNotFound
	}
	return world.Preview(ctx, definition, reference)
}

func newLocalConfigurationService(t *testing.T, world *localConfigurationWorld) *LocalConfigurationService {
	t.Helper()
	service, err := NewLocalConfigurationService(world, world, world, world, world, world, world)
	if err != nil {
		t.Fatalf("NewLocalConfigurationService: %v", err)
	}
	service.now = func() time.Time { return time.Date(2026, 8, 22, 3, 4, 5, 0, time.UTC) }
	return service
}

func TestLocalConfigurationServiceBindsOnlyNonArchivedAutomationAndReplaysReceipt(t *testing.T) {
	world := newLocalConfigurationWorld()
	service := newLocalConfigurationService(t, world)
	input := PutAutomationBindingInput{PackageID: 101, AutomationAgentID: 7, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-put-key-0001"}

	first, err := service.PutAutomationBinding(context.Background(), input)
	if err != nil || first.Binding == nil || first.Binding.AutomationAgentID != 7 || !first.LocalProjection || first.RealExternalCallExecuted {
		t.Fatalf("PutAutomationBinding response=%+v err=%v", first, err)
	}
	second, err := service.PutAutomationBinding(context.Background(), input)
	if err != nil || !reflect.DeepEqual(first, second) || len(world.base.events) != 1 {
		t.Fatalf("binding replay response=%+v err=%v events=%d", second, err, len(world.base.events))
	}
	paused, err := service.PutAutomationBinding(context.Background(), PutAutomationBindingInput{PackageID: 101, AutomationAgentID: 8, ExpectedVersion: 1, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-paused-key-01"})
	if err != nil || paused.Binding == nil || paused.Binding.AutomationAgentID != 8 {
		t.Fatalf("paused agent response=%+v err=%v", paused, err)
	}
	if _, err = service.PutAutomationBinding(context.Background(), PutAutomationBindingInput{PackageID: 101, AutomationAgentID: 9, ExpectedVersion: 2, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-archived-key-1"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("archived agent error=%v, want conflict", err)
	}
	if _, err = service.PutAutomationBinding(context.Background(), PutAutomationBindingInput{PackageID: 101, AutomationAgentID: 99, ExpectedVersion: 2, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-missing-key-1"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing agent error=%v, want not found", err)
	}
	deleted, err := service.DeleteAutomationBinding(context.Background(), DeleteAutomationBindingInput{PackageID: 101, ExpectedVersion: 2, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-delete-key-1"})
	if err != nil || !deleted.Deleted {
		t.Fatalf("DeleteAutomationBinding response=%+v err=%v", deleted, err)
	}
	repeated, err := service.DeleteAutomationBinding(context.Background(), DeleteAutomationBindingInput{PackageID: 101, ExpectedVersion: 0, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-delete-key-2"})
	if err != nil || repeated.Deleted {
		t.Fatalf("idempotent unbind response=%+v err=%v", repeated, err)
	}
}

func TestLocalConfigurationServiceRejectsStaleAutomationBindingCAS(t *testing.T) {
	world := newLocalConfigurationWorld()
	service := newLocalConfigurationService(t, world)
	if _, err := service.PutAutomationBinding(context.Background(), PutAutomationBindingInput{
		PackageID: 101, AutomationAgentID: 7, ExpectedVersion: 0, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-cas-first-key",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PutAutomationBinding(context.Background(), PutAutomationBindingInput{
		PackageID: 101, AutomationAgentID: 8, ExpectedVersion: 0, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-cas-stale-key",
	}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale binding CAS error=%v, want version conflict", err)
	}
	if stored := world.bindings[101]; stored.AutomationAgentID != 7 || stored.Version != 1 {
		t.Fatalf("stale write changed binding=%+v", stored)
	}
}

func TestLocalConfigurationServiceReplacesOnlyEligibleOrderedSenders(t *testing.T) {
	world := newLocalConfigurationWorld()
	service := newLocalConfigurationService(t, world)
	items := []PackageSender{{SenderUserID: "alpha", SortOrder: 1, IsEnabled: true}, {SenderUserID: "beta", SortOrder: 2, IsEnabled: false}}
	input := ReplaceSendersInput{PackageID: 101, Items: items, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "senders-put-key-0001"}

	first, err := service.ReplaceSenders(context.Background(), input)
	if err != nil || !reflect.DeepEqual(first.Items, items) || !first.LocalProjection || first.RealExternalCallExecuted {
		t.Fatalf("ReplaceSenders response=%+v err=%v", first, err)
	}
	second, err := service.ReplaceSenders(context.Background(), input)
	if err != nil || !reflect.DeepEqual(first, second) || len(world.base.events) != 1 {
		t.Fatalf("sender replay response=%+v err=%v events=%d", second, err, len(world.base.events))
	}
	if _, err = service.ReplaceSenders(context.Background(), ReplaceSendersInput{
		PackageID: 101, Items: []PackageSender{{SenderUserID: "outside", SortOrder: 1, IsEnabled: true}},
		Actor: Actor{AdminUserID: 9}, IdempotencyKey: "senders-outside-key"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("non-local sender error=%v, want conflict", err)
	}
	if _, err = service.ReplaceSenders(context.Background(), ReplaceSendersInput{
		PackageID: 101, Items: []PackageSender{{SenderUserID: "beta", SortOrder: 2, IsEnabled: true}},
		Actor: Actor{AdminUserID: 9}, IdempotencyKey: "senders-order-key-01"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("non-contiguous sender order error=%v, want invalid", err)
	}
	world.senders[101] = []PackageSender{{SenderUserID: "retired", SortOrder: 1, IsEnabled: true}}
	if _, err = service.GetSenders(context.Background(), 101); !errors.Is(err, ErrConflict) {
		t.Fatalf("inactive stored sender error=%v, want conflict", err)
	}
	world.operationMembers = []OperationMember{{SenderUserID: "alpha", DisplayName: "Alpha"}}
	members, err := service.ListOperationMembers(context.Background(), 1)
	if err != nil || !reflect.DeepEqual(members.Items, []OperationMember{{SenderUserID: "alpha", DisplayName: "Alpha"}}) || members.Scope != OperationMemberScope || members.ProviderReadExecuted {
		t.Fatalf("ListOperationMembers response=%+v err=%v", members, err)
	}
}

func TestLocalConfigurationServiceRejectsConflictingIdempotencyPayload(t *testing.T) {
	world := newLocalConfigurationWorld()
	service := newLocalConfigurationService(t, world)
	key := "senders-same-key-0001"
	if _, err := service.ReplaceSenders(context.Background(), ReplaceSendersInput{PackageID: 101, Items: []PackageSender{{SenderUserID: "alpha", SortOrder: 1, IsEnabled: true}}, Actor: Actor{AdminUserID: 9}, IdempotencyKey: key}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReplaceSenders(context.Background(), ReplaceSendersInput{PackageID: 101, Items: []PackageSender{{SenderUserID: "beta", SortOrder: 1, IsEnabled: true}}, Actor: Actor{AdminUserID: 9}, IdempotencyKey: key}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("payload mismatch error=%v, want idempotency conflict", err)
	}
	if len(world.receipts) != 1 {
		t.Fatalf("receipt count=%d", len(world.receipts))
	}
}

func TestLocalConfigurationServiceVersionsPreviewsAndMaterializesTypedSnapshot(t *testing.T) {
	world := newLocalConfigurationWorld()
	paused := world.base.packages[101]
	paused.Metadata.Lifecycle = PackagePaused
	world.base.packages[101] = paused
	service := newLocalConfigurationService(t, world)
	input := PutConfigurationInput{
		PackageID: 101, ExpectedVersion: 0, ExpectedPackageVersion: 1,
		Actor: Actor{AdminUserID: 9}, IdempotencyKey: "configuration-version-key-1",
	}
	first, err := service.PutConfiguration(context.Background(), input)
	if err != nil || first.Configuration == nil || first.Configuration.Version != 1 || first.Configuration.SchemaVersion != ConfigurationSchemaVersion ||
		first.Configuration.PackageVersion != 1 || first.Configuration.DefinitionDigest == "" || !first.LocalProjection || first.RealExternalCallExecuted {
		t.Fatalf("PutConfiguration response=%+v err=%v", first, err)
	}
	replayed, err := service.PutConfiguration(context.Background(), input)
	if err != nil || !reflect.DeepEqual(first, replayed) {
		t.Fatalf("configuration replay=%+v err=%v", replayed, err)
	}
	if _, err = service.PutConfiguration(context.Background(), PutConfigurationInput{
		PackageID: 101, ExpectedVersion: 0, ExpectedPackageVersion: 2, Actor: Actor{AdminUserID: 9}, IdempotencyKey: input.IdempotencyKey,
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed configuration payload error=%v, want idempotency conflict", err)
	}
	reference := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	preview, err := service.PreviewConfiguration(context.Background(), PreviewConfigurationInput{PackageID: 101, ConfigurationVersion: 1, EvaluatedAt: reference})
	if err != nil || preview.Materialized || preview.MemberCount != 2 || preview.EvaluatedAt != reference || preview.MemberDigest == "" {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	materializeInput := MaterializeConfigurationInput{PackageID: 101, ConfigurationVersion: 1, ExpectedPackageVersion: 1,
		Actor: Actor{AdminUserID: 9}, IdempotencyKey: "configuration-materialize-key-1"}
	materialized, err := service.MaterializeConfiguration(context.Background(), materializeInput)
	if err != nil || !materialized.Materialized || materialized.DefinitionDigest != first.Configuration.DefinitionDigest {
		t.Fatalf("materialized=%+v err=%v", materialized, err)
	}
	repeated, err := service.MaterializeConfiguration(context.Background(), materializeInput)
	if err != nil || !reflect.DeepEqual(materialized, repeated) {
		t.Fatalf("materialize replay=%+v err=%v", repeated, err)
	}
	otherActor := materializeInput
	otherActor.Actor.AdminUserID = 10
	if _, err = service.MaterializeConfiguration(context.Background(), otherActor); err != nil {
		t.Fatalf("same key for a different actor must have an independent receipt/event: %v", err)
	}
	if len(world.receipts) != 3 || len(world.base.events) != 3 {
		t.Fatalf("actor-bound config receipts=%d events=%d, want 3 each", len(world.receipts), len(world.base.events))
	}
}

func TestLocalConfigurationServiceRejectsActivePackageConfigurationActions(t *testing.T) {
	world := newLocalConfigurationWorld()
	paused := world.base.packages[101]
	paused.Metadata.Lifecycle = PackagePaused
	world.base.packages[101] = paused
	service := newLocalConfigurationService(t, world)
	if _, err := service.PutConfiguration(context.Background(), PutConfigurationInput{
		PackageID: 101, ExpectedVersion: 0, ExpectedPackageVersion: 1,
		Actor: Actor{AdminUserID: 9}, IdempotencyKey: "configuration-paused-before-active",
	}); err != nil {
		t.Fatalf("initial paused configuration: %v", err)
	}
	active := world.base.packages[101]
	active.Metadata.Lifecycle = PackageActive
	world.base.packages[101] = active

	if _, err := service.PutConfiguration(context.Background(), PutConfigurationInput{
		PackageID: 101, ExpectedVersion: 1, ExpectedPackageVersion: 1,
		Actor: Actor{AdminUserID: 9}, IdempotencyKey: "configuration-active-put",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("active put error=%v, want conflict", err)
	}
	if _, err := service.PreviewConfiguration(context.Background(), PreviewConfigurationInput{PackageID: 101, ConfigurationVersion: 1}); !errors.Is(err, ErrConflict) {
		t.Fatalf("active preview error=%v, want conflict", err)
	}
	if _, err := service.MaterializeConfiguration(context.Background(), MaterializeConfigurationInput{
		PackageID: 101, ConfigurationVersion: 1, ExpectedPackageVersion: 1,
		Actor: Actor{AdminUserID: 9}, IdempotencyKey: "configuration-active-materialize",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("active materialize error=%v, want conflict", err)
	}
	if current := world.configs[101]; current.Version != 1 || len(world.base.events) != 1 {
		t.Fatalf("active configuration changed snapshot=%+v events=%d", current, len(world.base.events))
	}
}
