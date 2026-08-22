package legacyaudience

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type transactionKey struct{}

type memoryWorld struct {
	groups          map[int64]Group
	packages        map[int64]PackageWriteModel
	segments        map[int64]segmentport.Segment
	memberSnapshots map[int64][]int64
	receipts        map[string]Receipt
	events          []LocalEvent
	nextGroupID     int64
	nextPackageID   int64
	nextReceiptID   int64
	failEvent       bool
	commits         int
	rollbacks       int
	segmentReads    int
}

type worldSnapshot struct {
	groups          map[int64]Group
	packages        map[int64]PackageWriteModel
	segments        map[int64]segmentport.Segment
	memberSnapshots map[int64][]int64
	receipts        map[string]Receipt
	events          []LocalEvent
	nextGroupID     int64
	nextPackageID   int64
	nextReceiptID   int64
}

func newMemoryWorld() *memoryWorld {
	now := time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC)
	definition := segmentport.Definition(`{"field":"stage_id","op":"eq","value":1}`)
	world := &memoryWorld{
		groups: map[int64]Group{
			1: {ID: 1, Name: "默认分组", SortOrder: 0, Version: 1, CreatedBy: 1, CreatedAt: now, UpdatedAt: now},
			2: {ID: 2, Name: "空分组", SortOrder: 10, Version: 1, CreatedBy: 1, CreatedAt: now, UpdatedAt: now},
		},
		packages:        make(map[int64]PackageWriteModel),
		segments:        make(map[int64]segmentport.Segment),
		memberSnapshots: make(map[int64][]int64),
		receipts:        make(map[string]Receipt),
		nextGroupID:     3,
		nextPackageID:   103,
		nextReceiptID:   1,
	}
	world.addPackage(PackageWriteModel{
		SegmentID: 101, Name: "高价值客户", Definition: definition, RefreshMode: segmentport.RefreshModeManual,
		SegmentLifecycle: segmentport.LifecycleStatusActive,
		Metadata: PackageMetadata{
			SegmentID: 101, GroupID: int64Pointer(1), Lifecycle: PackageActive, Version: 1,
			CreatedBy: 1, UpdatedBy: 1, CreatedAt: now, UpdatedAt: now,
		},
	}, 3, []int64{11, 12, 13})
	world.addPackage(PackageWriteModel{
		SegmentID: 102, Name: "近期客户", Definition: definition, RefreshMode: segmentport.RefreshModeManual,
		SegmentLifecycle: segmentport.LifecycleStatusActive,
		Metadata: PackageMetadata{
			SegmentID: 102, Lifecycle: PackagePaused, Version: 1,
			CreatedBy: 1, UpdatedBy: 1, CreatedAt: now, UpdatedAt: now,
		},
	}, 7, []int64{21, 22, 23, 24, 25, 26, 27})
	return world
}

func (world *memoryWorld) addPackage(model PackageWriteModel, memberCount int64, members []int64) {
	model = cloneWriteModel(model)
	world.packages[model.SegmentID] = model
	world.segments[model.SegmentID] = segmentport.Segment{
		ID: segmentport.SegmentID(model.SegmentID), Name: model.Name, Definition: cloneDefinition(model.Definition),
		RefreshMode: model.RefreshMode, RefreshCron: cloneString(model.RefreshCron), MemberCount: memberCount,
		RefreshStatus: segmentport.RefreshStatusIdle, LifecycleStatus: model.SegmentLifecycle,
		CreatedAt: model.Metadata.CreatedAt, UpdatedAt: model.Metadata.UpdatedAt,
	}
	world.memberSnapshots[model.SegmentID] = append([]int64(nil), members...)
}

func (world *memoryWorld) Within(ctx context.Context, callback func(context.Context) error) error {
	if ctx == nil || callback == nil {
		return ErrUnavailable
	}
	snapshot := world.snapshot()
	err := callback(context.WithValue(ctx, transactionKey{}, true))
	if err != nil {
		world.restore(snapshot)
		world.rollbacks++
		return err
	}
	world.commits++
	return nil
}

func (world *memoryWorld) snapshot() worldSnapshot {
	return worldSnapshot{
		groups: cloneGroups(world.groups), packages: clonePackages(world.packages), segments: cloneSegments(world.segments),
		memberSnapshots: cloneMembers(world.memberSnapshots), receipts: cloneReceipts(world.receipts),
		events: append([]LocalEvent(nil), world.events...), nextGroupID: world.nextGroupID,
		nextPackageID: world.nextPackageID, nextReceiptID: world.nextReceiptID,
	}
}

func (world *memoryWorld) restore(snapshot worldSnapshot) {
	world.groups, world.packages, world.segments = snapshot.groups, snapshot.packages, snapshot.segments
	world.memberSnapshots, world.receipts, world.events = snapshot.memberSnapshots, snapshot.receipts, snapshot.events
	world.nextGroupID, world.nextPackageID, world.nextReceiptID = snapshot.nextGroupID, snapshot.nextPackageID, snapshot.nextReceiptID
}

func cloneGroups(source map[int64]Group) map[int64]Group {
	copy := make(map[int64]Group, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func clonePackages(source map[int64]PackageWriteModel) map[int64]PackageWriteModel {
	copy := make(map[int64]PackageWriteModel, len(source))
	for key, value := range source {
		copy[key] = cloneWriteModel(value)
	}
	return copy
}

func cloneSegments(source map[int64]segmentport.Segment) map[int64]segmentport.Segment {
	copy := make(map[int64]segmentport.Segment, len(source))
	for key, value := range source {
		value.Definition = cloneDefinition(value.Definition)
		value.RefreshCron = cloneString(value.RefreshCron)
		value.RefreshedAt = cloneTime(value.RefreshedAt)
		copy[key] = value
	}
	return copy
}

func cloneMembers(source map[int64][]int64) map[int64][]int64 {
	copy := make(map[int64][]int64, len(source))
	for key, value := range source {
		copy[key] = append([]int64(nil), value...)
	}
	return copy
}

func cloneReceipts(source map[string]Receipt) map[string]Receipt {
	copy := make(map[string]Receipt, len(source))
	for key, value := range source {
		value.ResultJSON = append(json.RawMessage(nil), value.ResultJSON...)
		copy[key] = value
	}
	return copy
}

func requireTransaction(ctx context.Context) error {
	if active, _ := ctx.Value(transactionKey{}).(bool); !active {
		return ErrUnavailable
	}
	return nil
}

func (world *memoryWorld) ListGroups(context.Context) ([]Group, error) {
	groups := make([]Group, 0, len(world.groups))
	for _, group := range world.groups {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(left, right int) bool {
		if groups[left].SortOrder == groups[right].SortOrder {
			return groups[left].ID < groups[right].ID
		}
		return groups[left].SortOrder < groups[right].SortOrder
	})
	return groups, nil
}

func (world *memoryWorld) LockGroup(ctx context.Context, groupID int64) (Group, error) {
	if err := requireTransaction(ctx); err != nil {
		return Group{}, err
	}
	group, exists := world.groups[groupID]
	if !exists {
		return Group{}, ErrNotFound
	}
	return group, nil
}

func (world *memoryWorld) InsertGroup(ctx context.Context, name string, sortOrder int32, actorID int64, now time.Time) (Group, error) {
	if err := requireTransaction(ctx); err != nil {
		return Group{}, err
	}
	for _, group := range world.groups {
		if strings.EqualFold(group.Name, name) {
			return Group{}, ErrConflict
		}
	}
	group := Group{ID: world.nextGroupID, Name: name, SortOrder: sortOrder, Version: 1, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now}
	world.nextGroupID++
	world.groups[group.ID] = group
	return group, nil
}

func (world *memoryWorld) UpdateGroup(ctx context.Context, current Group, name string, sortOrder int32, _ int64, now time.Time) (Group, error) {
	if err := requireTransaction(ctx); err != nil {
		return Group{}, err
	}
	stored, exists := world.groups[current.ID]
	if !exists {
		return Group{}, ErrNotFound
	}
	if stored.Version != current.Version {
		return Group{}, ErrVersionConflict
	}
	for id, group := range world.groups {
		if id != current.ID && strings.EqualFold(group.Name, name) {
			return Group{}, ErrConflict
		}
	}
	stored.Name, stored.SortOrder, stored.Version, stored.UpdatedAt = name, sortOrder, stored.Version+1, now
	world.groups[stored.ID] = stored
	return stored, nil
}

func (world *memoryWorld) CountPackagesInGroup(ctx context.Context, groupID int64) (int64, error) {
	if err := requireTransaction(ctx); err != nil {
		return 0, err
	}
	var count int64
	for _, model := range world.packages {
		if model.Metadata.GroupID != nil && *model.Metadata.GroupID == groupID {
			count++
		}
	}
	return count, nil
}

func (world *memoryWorld) DeleteGroup(ctx context.Context, groupID int64, expectedVersion int64) error {
	if err := requireTransaction(ctx); err != nil {
		return err
	}
	group, exists := world.groups[groupID]
	if !exists {
		return ErrNotFound
	}
	if group.Version != expectedVersion {
		return ErrVersionConflict
	}
	delete(world.groups, groupID)
	return nil
}

func (world *memoryWorld) ListPackageMetadata(_ context.Context, groupID *int64, limit, offset int) ([]PackageMetadata, int64, error) {
	ids := make([]int, 0, len(world.packages))
	for id, model := range world.packages {
		if groupID == nil || model.Metadata.GroupID != nil && *model.Metadata.GroupID == *groupID {
			ids = append(ids, int(id))
		}
	}
	sort.Ints(ids)
	total := int64(len(ids))
	if offset >= len(ids) {
		return []PackageMetadata{}, total, nil
	}
	end := offset + limit
	if end > len(ids) {
		end = len(ids)
	}
	items := make([]PackageMetadata, 0, end-offset)
	for _, id := range ids[offset:end] {
		metadata := world.packages[int64(id)].Metadata
		metadata.GroupID = cloneInt64(metadata.GroupID)
		items = append(items, metadata)
	}
	return items, total, nil
}

func (world *memoryWorld) GetPackageMetadata(_ context.Context, packageID int64) (PackageMetadata, error) {
	model, exists := world.packages[packageID]
	if !exists {
		return PackageMetadata{}, ErrNotFound
	}
	metadata := model.Metadata
	metadata.GroupID = cloneInt64(metadata.GroupID)
	return metadata, nil
}

func (world *memoryWorld) LockPackage(ctx context.Context, packageID int64) (PackageWriteModel, error) {
	if err := requireTransaction(ctx); err != nil {
		return PackageWriteModel{}, err
	}
	model, exists := world.packages[packageID]
	if !exists {
		return PackageWriteModel{}, ErrNotFound
	}
	return cloneWriteModel(model), nil
}

func (world *memoryWorld) SavePackage(ctx context.Context, current, next PackageWriteModel, expectedVersion int64, _ int64, now time.Time) (PackageWriteModel, error) {
	if err := requireTransaction(ctx); err != nil {
		return PackageWriteModel{}, err
	}
	stored, exists := world.packages[current.SegmentID]
	if !exists {
		return PackageWriteModel{}, ErrNotFound
	}
	if stored.Metadata.Version != expectedVersion || stored.Metadata.Version != current.Metadata.Version {
		return PackageWriteModel{}, ErrVersionConflict
	}
	next = cloneWriteModel(next)
	next.Metadata.UpdatedAt = now
	world.packages[next.SegmentID] = next
	segment := world.segments[next.SegmentID]
	segment.Name = next.Name
	segment.Definition = cloneDefinition(next.Definition)
	segment.RefreshMode = next.RefreshMode
	segment.RefreshCron = cloneString(next.RefreshCron)
	segment.LifecycleStatus = next.SegmentLifecycle
	segment.UpdatedAt = now
	world.segments[next.SegmentID] = segment
	return cloneWriteModel(next), nil
}

func (world *memoryWorld) LockCopyNameNamespace(ctx context.Context, _ string) error {
	return requireTransaction(ctx)
}

func (world *memoryWorld) PackageNameExists(ctx context.Context, name string) (bool, error) {
	if err := requireTransaction(ctx); err != nil {
		return false, err
	}
	for _, segment := range world.segments {
		if strings.EqualFold(segment.Name, name) {
			return true, nil
		}
	}
	return false, nil
}

func (world *memoryWorld) InsertPackageCopy(ctx context.Context, source PackageWriteModel, name string, actorID int64, now time.Time) (PackageWriteModel, error) {
	if err := requireTransaction(ctx); err != nil {
		return PackageWriteModel{}, err
	}
	if source.Metadata.Lifecycle == PackageArchived {
		return PackageWriteModel{}, ErrArchived
	}
	id := world.nextPackageID
	world.nextPackageID++
	copy := PackageWriteModel{
		SegmentID: id, Name: name, Definition: cloneDefinition(source.Definition), RefreshMode: source.RefreshMode,
		RefreshCron: cloneString(source.RefreshCron), SegmentLifecycle: segmentport.LifecycleStatusActive,
		Metadata: PackageMetadata{
			SegmentID: id, GroupID: cloneInt64(source.Metadata.GroupID), Lifecycle: PackagePaused, Version: 1,
			CreatedBy: actorID, UpdatedBy: actorID, CreatedAt: now, UpdatedAt: now,
		},
	}
	world.addPackage(copy, 0, nil)
	return cloneWriteModel(copy), nil
}

func receiptMapKey(operation ReceiptOperation, actorID int64, digest [32]byte) string {
	return string(operation) + ":" + strconvInt64(actorID) + ":" + hex.EncodeToString(digest[:])
}

func strconvInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}

func (world *memoryWorld) ReserveReceipt(ctx context.Context, wanted ReceiptReservation) (Receipt, bool, error) {
	if err := requireTransaction(ctx); err != nil {
		return Receipt{}, false, err
	}
	key := receiptMapKey(wanted.Operation, wanted.ActorID, wanted.KeyDigest)
	if receipt, exists := world.receipts[key]; exists {
		return receipt, false, nil
	}
	receipt := Receipt{
		ID: world.nextReceiptID, Operation: wanted.Operation, ActorID: wanted.ActorID,
		KeyDigest: wanted.KeyDigest, PayloadDigest: wanted.PayloadDigest, State: "in_progress",
	}
	world.nextReceiptID++
	world.receipts[key] = receipt
	return receipt, true, nil
}

func (world *memoryWorld) CompleteReceipt(ctx context.Context, receiptID int64, result json.RawMessage, _ time.Time) (Receipt, error) {
	if err := requireTransaction(ctx); err != nil {
		return Receipt{}, err
	}
	for key, receipt := range world.receipts {
		if receipt.ID != receiptID {
			continue
		}
		if receipt.State != "in_progress" {
			return Receipt{}, ErrConflict
		}
		receipt.State = "completed"
		receipt.ResultJSON = append(json.RawMessage(nil), result...)
		world.receipts[key] = receipt
		return receipt, nil
	}
	return Receipt{}, ErrNotFound
}

func (world *memoryWorld) Get(_ context.Context, id segmentport.SegmentID) (segmentport.Segment, error) {
	world.segmentReads++
	segment, exists := world.segments[int64(id)]
	if !exists {
		return segmentport.Segment{}, ErrNotFound
	}
	segment.Definition = cloneDefinition(segment.Definition)
	segment.RefreshCron = cloneString(segment.RefreshCron)
	segment.RefreshedAt = cloneTime(segment.RefreshedAt)
	return segment, nil
}

func (world *memoryWorld) Append(ctx context.Context, event LocalEvent) error {
	if err := requireTransaction(ctx); err != nil {
		return err
	}
	if world.failEvent {
		return errors.New("event append failed")
	}
	world.events = append(world.events, event)
	return nil
}

func newTestService(t *testing.T, world *memoryWorld) (*Service, time.Time) {
	t.Helper()
	service, err := NewService(world, world, world, world)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	now := time.Date(2026, 8, 22, 2, 3, 4, 0, time.UTC)
	service.now = func() time.Time { return now }
	return service, now
}

func int64Pointer(value int64) *int64    { return &value }
func stringPointer(value string) *string { return &value }

func TestServiceListsPackagesWithStablePaginationAndSegmentFacts(t *testing.T) {
	world := newMemoryWorld()
	service, _ := newTestService(t, world)

	groups, err := service.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups.Items) != 2 || groups.Items[0].ID != 1 || !groups.LocalProjection || groups.RealExternalCallExecuted {
		t.Fatalf("unexpected groups response: %+v", groups)
	}

	page, err := service.ListPackages(context.Background(), ListPackagesInput{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("ListPackages: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != 102 || page.Items[0].MemberCount != 7 {
		t.Fatalf("unexpected page: %+v", page)
	}
	if world.segmentReads != 1 {
		t.Fatalf("member/refresh facts must come from Segment reader, got %d reads", world.segmentReads)
	}

	empty, err := service.ListPackages(context.Background(), ListPackagesInput{Limit: 10, Offset: 100})
	if err != nil || len(empty.Items) != 0 || empty.Total != 2 {
		t.Fatalf("empty page: response=%+v err=%v", empty, err)
	}
}

func TestServiceGroupCRUDIdempotencyAndDeleteProtection(t *testing.T) {
	world := newMemoryWorld()
	service, _ := newTestService(t, world)
	actor := Actor{AdminUserID: 9}
	key := "group-create-key-0001"
	create := CreateGroupInput{Name: "  新分组  ", SortOrder: 5, ExpectedVersion: 0, Actor: actor, IdempotencyKey: key}

	first, err := service.CreateGroup(context.Background(), create)
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	second, err := service.CreateGroup(context.Background(), create)
	if err != nil {
		t.Fatalf("CreateGroup replay: %v", err)
	}
	if !reflect.DeepEqual(first, second) || len(world.groups) != 3 || len(world.events) != 1 {
		t.Fatalf("replay changed result/state: first=%+v second=%+v groups=%d events=%d", first, second, len(world.groups), len(world.events))
	}
	conflict := create
	conflict.Name = "不同负载"
	if _, err = service.CreateGroup(context.Background(), conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("payload conflict error = %v", err)
	}

	newName := "重命名分组"
	sortOrder := int32(6)
	updated, err := service.UpdateGroup(context.Background(), UpdateGroupInput{
		GroupID: first.Group.ID, Name: &newName, SortOrder: &sortOrder, ExpectedVersion: 1,
		Actor: actor, IdempotencyKey: "group-update-key-0001",
	})
	if err != nil || updated.Group.Version != 2 || updated.Group.Name != newName {
		t.Fatalf("UpdateGroup response=%+v err=%v", updated, err)
	}
	if _, err = service.UpdateGroup(context.Background(), UpdateGroupInput{
		GroupID: first.Group.ID, Name: &newName, ExpectedVersion: 1,
		Actor: actor, IdempotencyKey: "group-update-stale-001",
	}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale update error = %v", err)
	}

	if _, err = service.DeleteGroup(context.Background(), DeleteGroupInput{
		GroupID: 1, ExpectedVersion: 1, Actor: actor, IdempotencyKey: "group-delete-used-0001",
	}); !errors.Is(err, ErrGroupNotEmpty) {
		t.Fatalf("non-empty delete error = %v", err)
	}
	if _, exists := world.groups[1]; !exists {
		t.Fatal("non-empty group was deleted")
	}

	deleted, err := service.DeleteGroup(context.Background(), DeleteGroupInput{
		GroupID: 2, ExpectedVersion: 1, Actor: actor, IdempotencyKey: "group-delete-empty-001",
	})
	if err != nil || !deleted.Deleted || deleted.GroupID != 2 {
		t.Fatalf("DeleteGroup response=%+v err=%v", deleted, err)
	}
	if _, exists := world.groups[2]; exists {
		t.Fatal("empty group was not deleted")
	}
}

func TestServicePackageUpdateCopyDoesNotCopyMembersAndLifecycle(t *testing.T) {
	world := newMemoryWorld()
	service, _ := newTestService(t, world)
	actor := Actor{AdminUserID: 9}
	name := "高价值客户 2026"
	mode := segmentport.RefreshModeScheduled
	cron := "0 9 * * 1"
	definition := segmentport.Definition(`{"field":"tag_id","op":"has_any","value":[3,2]}`)

	updated, err := service.UpdatePackage(context.Background(), UpdatePackageInput{
		PackageID: 101, Name: &name, Definition: &definition, RefreshMode: &mode,
		RefreshCron: OptionalString{Set: true, Value: &cron}, GroupID: OptionalInt64{Set: true, Value: int64Pointer(2)},
		ExpectedVersion: 1, Actor: actor, IdempotencyKey: "package-update-key-01",
	})
	if err != nil {
		t.Fatalf("UpdatePackage: %v", err)
	}
	if updated.Package.Version != 2 || updated.Package.RefreshMode != mode || updated.Package.RefreshCron == nil || *updated.Package.RefreshCron != cron {
		t.Fatalf("unexpected update: %+v", updated)
	}
	if got := string(world.packages[101].Definition); got != `{"field":"tag_id","op":"has_any","value":[2,3]}` {
		t.Fatalf("definition not canonical: %s", got)
	}
	if world.segments[101].MemberCount != 3 || len(world.memberSnapshots[101]) != 3 {
		t.Fatal("package update modified member facts")
	}

	copied, err := service.CopyPackage(context.Background(), PackageCommand{
		PackageID: 101, ExpectedVersion: 2, Actor: actor, IdempotencyKey: "package-copy-key-0001",
	})
	if err != nil {
		t.Fatalf("CopyPackage: %v", err)
	}
	copyID := copied.Package.ID
	if copyID == 101 || copied.Package.Name != name+" 副本" || copied.Package.Lifecycle != PackagePaused ||
		copied.Package.Version != 1 || copied.Package.MemberCount == nil || *copied.Package.MemberCount != 0 {
		t.Fatalf("unexpected copy: %+v", copied)
	}
	if world.segments[copyID].MemberCount != 0 || len(world.memberSnapshots[copyID]) != 0 {
		t.Fatalf("copy inherited member facts: segment=%+v members=%v", world.segments[copyID], world.memberSnapshots[copyID])
	}
	if !reflect.DeepEqual(world.packages[copyID].Definition, world.packages[101].Definition) {
		t.Fatal("copy did not preserve definition")
	}

	secondCopy, err := service.CopyPackage(context.Background(), PackageCommand{
		PackageID: 101, ExpectedVersion: 2, Actor: actor, IdempotencyKey: "package-copy-key-0002",
	})
	if err != nil || secondCopy.Package.Name != name+" 副本 (2)" {
		t.Fatalf("deterministic collision name: response=%+v err=%v", secondCopy, err)
	}

	paused, err := service.PausePackage(context.Background(), PackageCommand{
		PackageID: 101, ExpectedVersion: 2, Actor: actor, IdempotencyKey: "package-pause-key-001",
	})
	if err != nil || paused.Package.Lifecycle != PackagePaused || paused.Package.Version != 3 {
		t.Fatalf("PausePackage response=%+v err=%v", paused, err)
	}
	active, err := service.ActivatePackage(context.Background(), PackageCommand{
		PackageID: 101, ExpectedVersion: 3, Actor: actor, IdempotencyKey: "package-active-key-01",
	})
	if err != nil || active.Package.Lifecycle != PackageActive || active.Package.Version != 4 {
		t.Fatalf("ActivatePackage response=%+v err=%v", active, err)
	}
	archived, err := service.ArchivePackage(context.Background(), PackageCommand{
		PackageID: 101, ExpectedVersion: 4, Actor: actor, IdempotencyKey: "package-archive-key-1",
	})
	if err != nil || archived.Lifecycle != PackageArchived || archived.Version != 5 || !archived.Archived {
		t.Fatalf("ArchivePackage response=%+v err=%v", archived, err)
	}
	repeated, err := service.ArchivePackage(context.Background(), PackageCommand{
		PackageID: 101, ExpectedVersion: 4, Actor: actor, IdempotencyKey: "package-archive-key-2",
	})
	if err != nil || repeated.Version != 5 || !reflect.DeepEqual(archived, repeated) {
		t.Fatalf("repeat archive response=%+v err=%v", repeated, err)
	}
	if _, err = service.ActivatePackage(context.Background(), PackageCommand{
		PackageID: 101, ExpectedVersion: 5, Actor: actor, IdempotencyKey: "archived-activate-key",
	}); !errors.Is(err, ErrArchived) {
		t.Fatalf("activate archived error = %v", err)
	}
	if world.segments[101].LifecycleStatus != segmentport.LifecycleStatusArchived {
		t.Fatal("segment lifecycle was not archived in the same model")
	}
}

func TestServiceCASAndTransactionRollbackOnEventFailure(t *testing.T) {
	world := newMemoryWorld()
	service, _ := newTestService(t, world)
	actor := Actor{AdminUserID: 9}

	if _, err := service.PausePackage(context.Background(), PackageCommand{
		PackageID: 101, ExpectedVersion: 99, Actor: actor, IdempotencyKey: "stale-pause-key-0001",
	}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("CAS error = %v", err)
	}
	if world.packages[101].Metadata.Version != 1 || len(world.receipts) != 0 {
		t.Fatal("CAS failure committed state or receipt")
	}

	world.failEvent = true
	command := PackageCommand{PackageID: 101, ExpectedVersion: 1, Actor: actor, IdempotencyKey: "rollback-pause-key-01"}
	if _, err := service.PausePackage(context.Background(), command); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("event failure error = %v", err)
	}
	if world.packages[101].Metadata.Lifecycle != PackageActive || world.packages[101].Metadata.Version != 1 || len(world.receipts) != 0 || len(world.events) != 0 {
		t.Fatalf("transaction did not roll back: package=%+v receipts=%d events=%d", world.packages[101], len(world.receipts), len(world.events))
	}
	if world.rollbacks < 2 {
		t.Fatalf("expected CAS and event rollbacks, got %d", world.rollbacks)
	}

	world.failEvent = false
	response, err := service.PausePackage(context.Background(), command)
	if err != nil || response.Package.Version != 2 || len(world.receipts) != 1 || len(world.events) != 1 {
		t.Fatalf("retry after rollback response=%+v err=%v receipts=%d events=%d", response, err, len(world.receipts), len(world.events))
	}
}

func TestServiceIdempotencyDigestUsesPayloadNotKeyOnly(t *testing.T) {
	world := newMemoryWorld()
	service, _ := newTestService(t, world)
	actor := Actor{AdminUserID: 9}
	key := "shared-update-key-001"
	name := "名称一"
	first, err := service.UpdatePackage(context.Background(), UpdatePackageInput{
		PackageID: 101, Name: &name, ExpectedVersion: 1, Actor: actor, IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("first update: %v", err)
	}
	replay, err := service.UpdatePackage(context.Background(), UpdatePackageInput{
		PackageID: 101, Name: &name, ExpectedVersion: 1, Actor: actor, IdempotencyKey: key,
	})
	if err != nil || !reflect.DeepEqual(first, replay) {
		t.Fatalf("replay response=%+v err=%v", replay, err)
	}
	other := "名称二"
	if _, err = service.UpdatePackage(context.Background(), UpdatePackageInput{
		PackageID: 101, Name: &other, ExpectedVersion: 1, Actor: actor, IdempotencyKey: key,
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("payload conflict error = %v", err)
	}
}

func TestReceiptMapKeyIsStable(t *testing.T) {
	digest := sha256.Sum256([]byte("key"))
	if left, right := receiptMapKey(OperationGroupCreate, 9, digest), receiptMapKey(OperationGroupCreate, 9, digest); left != right {
		t.Fatalf("unstable receipt key: %q != %q", left, right)
	}
}
