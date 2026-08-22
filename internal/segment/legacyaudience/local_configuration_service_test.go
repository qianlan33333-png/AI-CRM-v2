package legacyaudience

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type localConfigurationWorld struct {
	base     *memoryWorld
	bindings map[int64]AutomationBinding
	senders  map[int64][]PackageSender
	receipts map[string]Receipt
	entries  []contactport.StaffDirectoryEntry
	agents   map[int64]AutomationAgent
	nextID   int64
}

func newLocalConfigurationWorld() *localConfigurationWorld {
	return &localConfigurationWorld{
		base: newMemoryWorld(), bindings: make(map[int64]AutomationBinding), senders: make(map[int64][]PackageSender),
		receipts: make(map[string]Receipt), nextID: 1,
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
func (world *localConfigurationWorld) SaveAutomationBinding(ctx context.Context, value AutomationBinding, actor int64, now time.Time) (AutomationBinding, error) {
	if err := requireTransaction(ctx); err != nil {
		return AutomationBinding{}, err
	}
	if existing, found := world.bindings[value.PackageID]; found {
		value.CreatedBy, value.CreatedAt = existing.CreatedBy, existing.CreatedAt
	} else {
		value.CreatedBy, value.CreatedAt = actor, now
	}
	value.UpdatedBy, value.UpdatedAt = actor, now
	world.bindings[value.PackageID] = value
	return value, nil
}
func (world *localConfigurationWorld) DeleteAutomationBinding(ctx context.Context, id int64) (bool, error) {
	if err := requireTransaction(ctx); err != nil {
		return false, err
	}
	if _, found := world.bindings[id]; !found {
		return false, nil
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
func (world *localConfigurationWorld) ListEligibleSenderUserIDs(_ context.Context, ids []string) ([]string, error) {
	return world.eligibleSenderUserIDs(ids), nil
}
func (world *localConfigurationWorld) LockEligibleSenderUserIDs(ctx context.Context, ids []string) ([]string, error) {
	if err := requireTransaction(ctx); err != nil {
		return nil, err
	}
	return world.eligibleSenderUserIDs(ids), nil
}
func (world *localConfigurationWorld) eligibleSenderUserIDs(ids []string) []string {
	eligible := make(map[string]struct{}, len(world.entries))
	for _, entry := range world.entries {
		eligible[entry.WeComUserID] = struct{}{}
	}
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := eligible[id]; ok {
			result = append(result, id)
		}
	}
	return result
}
func (world *localConfigurationWorld) ReserveConfigurationReceipt(ctx context.Context, wanted ReceiptReservation) (Receipt, bool, error) {
	if err := requireTransaction(ctx); err != nil {
		return Receipt{}, false, err
	}
	key := string(wanted.Operation) + ":" + string(wanted.KeyDigest[:])
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

func newLocalConfigurationService(t *testing.T, world *localConfigurationWorld) *LocalConfigurationService {
	t.Helper()
	service, err := NewLocalConfigurationService(world, world, world, world, world)
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
	paused, err := service.PutAutomationBinding(context.Background(), PutAutomationBindingInput{PackageID: 101, AutomationAgentID: 8, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-paused-key-01"})
	if err != nil || paused.Binding == nil || paused.Binding.AutomationAgentID != 8 {
		t.Fatalf("paused agent response=%+v err=%v", paused, err)
	}
	if _, err = service.PutAutomationBinding(context.Background(), PutAutomationBindingInput{PackageID: 101, AutomationAgentID: 9, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-archived-key-1"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("archived agent error=%v, want conflict", err)
	}
	if _, err = service.PutAutomationBinding(context.Background(), PutAutomationBindingInput{PackageID: 101, AutomationAgentID: 99, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-missing-key-1"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing agent error=%v, want not found", err)
	}
	deleted, err := service.DeleteAutomationBinding(context.Background(), DeleteAutomationBindingInput{PackageID: 101, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-delete-key-1"})
	if err != nil || !deleted.Deleted {
		t.Fatalf("DeleteAutomationBinding response=%+v err=%v", deleted, err)
	}
	repeated, err := service.DeleteAutomationBinding(context.Background(), DeleteAutomationBindingInput{PackageID: 101, Actor: Actor{AdminUserID: 9}, IdempotencyKey: "binding-delete-key-2"})
	if err != nil || repeated.Deleted {
		t.Fatalf("idempotent unbind response=%+v err=%v", repeated, err)
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
	members, err := service.ListOperationMembers(context.Background(), 1)
	if err != nil || !reflect.DeepEqual(members.Items, []OperationMember{{SenderUserID: "alpha", DisplayName: "Alpha"}}) || members.Scope != OperationMemberScope {
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
