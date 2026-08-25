package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

type memoryContextKey struct{}

type memoryState struct {
	links             map[radarport.LinkID]radarport.Link
	idempotency       map[string]radarport.IdempotencyRecord
	events            []eventport.Event
	trackingEvents    []radarport.Event
	trackingByKey     map[string]int
	trackingDigests   map[string][32]byte
	nextLinkID        int64
	nextIdempotencyID int64
}

type memoryDB struct {
	mu           sync.Mutex
	state        memoryState
	failEvent    bool
	failComplete bool
}

type memoryUnitOfWork struct{ db *memoryDB }
type memoryRepository struct{ db *memoryDB }
type memoryAppender struct{ db *memoryDB }

func newMemoryDB() *memoryDB {
	return &memoryDB{state: memoryState{
		links:           make(map[radarport.LinkID]radarport.Link),
		idempotency:     make(map[string]radarport.IdempotencyRecord),
		trackingByKey:   make(map[string]int),
		trackingDigests: make(map[string][32]byte),
	}}
}

func (uow *memoryUnitOfWork) Within(ctx context.Context, callback func(context.Context) error) error {
	if uow == nil || uow.db == nil || callback == nil {
		return radarport.ErrUnavailable
	}
	uow.db.mu.Lock()
	defer uow.db.mu.Unlock()
	working := cloneMemoryState(uow.db.state)
	if err := callback(context.WithValue(ctx, memoryContextKey{}, &working)); err != nil {
		return err
	}
	uow.db.state = working
	return nil
}

func (repository *memoryRepository) List(ctx context.Context, input radarport.ListInput) ([]radarport.Link, int64, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return nil, 0, err
	}
	items := make([]radarport.Link, 0, len(state.links))
	for _, link := range state.links {
		if input.Status != radarport.StatusFilterAll && string(link.Status) != string(input.Status) {
			continue
		}
		items = append(items, cloneLink(link))
	}
	sort.Slice(items, func(left, right int) bool {
		switch input.Sort {
		case radarport.SortCreatedDesc:
			if !items[left].CreatedAt.Equal(items[right].CreatedAt) {
				return items[left].CreatedAt.After(items[right].CreatedAt)
			}
			return items[left].LinkID > items[right].LinkID
		case radarport.SortNameAsc:
			if items[left].Name != items[right].Name {
				return items[left].Name < items[right].Name
			}
			return items[left].LinkID < items[right].LinkID
		default:
			if !items[left].UpdatedAt.Equal(items[right].UpdatedAt) {
				return items[left].UpdatedAt.After(items[right].UpdatedAt)
			}
			return items[left].LinkID > items[right].LinkID
		}
	})
	total := int64(len(items))
	start := int(input.Offset)
	if start > len(items) {
		start = len(items)
	}
	end := start + int(input.Limit)
	if end > len(items) {
		end = len(items)
	}
	return append([]radarport.Link(nil), items[start:end]...), total, nil
}

func (repository *memoryRepository) Get(ctx context.Context, id radarport.LinkID) (radarport.Link, error) {
	return repository.get(ctx, id)
}

func (repository *memoryRepository) GetForUpdate(ctx context.Context, id radarport.LinkID) (radarport.Link, error) {
	return repository.get(ctx, id)
}

func (repository *memoryRepository) get(ctx context.Context, id radarport.LinkID) (radarport.Link, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return radarport.Link{}, err
	}
	link, exists := state.links[id]
	if !exists {
		return radarport.Link{}, radarport.ErrNotFound
	}
	return cloneLink(link), nil
}

func (repository *memoryRepository) Create(ctx context.Context, record radarport.CreateRecord, now time.Time) (radarport.Link, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return radarport.Link{}, err
	}
	for _, existing := range state.links {
		if existing.PublicCode == record.PublicCode {
			return radarport.Link{}, radarport.ErrPublicCodeCollision
		}
	}
	state.nextLinkID++
	link := radarport.Link{
		LinkID:         radarport.LinkID(state.nextLinkID),
		PublicCode:     record.PublicCode,
		Name:           record.Name,
		Title:          record.Title,
		DestinationURL: record.DestinationURL,
		CoverImageID:   cloneID(record.CoverImageID),
		AttachmentID:   cloneID(record.AttachmentID),
		Status:         record.Status,
		Version:        1,
		CreatedBy:      record.ActorID,
		UpdatedBy:      record.ActorID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	state.links[link.LinkID] = cloneLink(link)
	return link, nil
}

func (repository *memoryRepository) Update(ctx context.Context, record radarport.UpdateRecord, now time.Time) (radarport.Link, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return radarport.Link{}, err
	}
	link, exists := state.links[record.LinkID]
	if !exists {
		return radarport.Link{}, radarport.ErrNotFound
	}
	if link.Version != record.ExpectedVersion {
		return radarport.Link{}, radarport.ErrConflict
	}
	link.Name = record.Name
	link.Title = record.Title
	link.DestinationURL = record.DestinationURL
	link.CoverImageID = cloneID(record.CoverImageID)
	link.AttachmentID = cloneID(record.AttachmentID)
	link.Version++
	link.UpdatedBy = record.ActorID
	link.UpdatedAt = now
	state.links[link.LinkID] = cloneLink(link)
	return link, nil
}

func (repository *memoryRepository) SetStatus(ctx context.Context, record radarport.StatusRecord, now time.Time) (radarport.Link, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return radarport.Link{}, err
	}
	link, exists := state.links[record.LinkID]
	if !exists {
		return radarport.Link{}, radarport.ErrNotFound
	}
	if link.Version != record.ExpectedVersion {
		return radarport.Link{}, radarport.ErrConflict
	}
	link.Status = record.Target
	link.Version++
	link.UpdatedBy = record.ActorID
	link.UpdatedAt = now
	state.links[link.LinkID] = cloneLink(link)
	return link, nil
}

func (repository *memoryRepository) GetEnabledByCode(ctx context.Context, code string) (radarport.Link, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return radarport.Link{}, err
	}
	for _, link := range state.links {
		if link.PublicCode == code && link.Status == radarport.StatusEnabled {
			return cloneLink(link), nil
		}
	}
	return radarport.Link{}, radarport.ErrNotFound
}

func (repository *memoryRepository) InsertEvent(ctx context.Context, record radarport.InsertEventRecord) (radarport.Event, bool, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return radarport.Event{}, false, err
	}
	key := trackingKey(record.LinkID, record.KeyDigest)
	if len(record.KeyDigest) != 0 {
		if index, exists := state.trackingByKey[key]; exists {
			return state.trackingEvents[index], false, nil
		}
	}
	event := radarport.Event{
		EventID: int64(len(state.trackingEvents) + 1), ReceiptID: record.ReceiptID,
		LinkID: record.LinkID, Stage: record.Stage, Page: clonePage(record.Page),
		Source: record.Source, CreatedAt: record.CreatedAt,
	}
	state.trackingEvents = append(state.trackingEvents, event)
	if len(record.KeyDigest) != 0 {
		state.trackingByKey[key] = len(state.trackingEvents) - 1
		state.trackingDigests[key] = record.PayloadDigest
	}
	return event, true, nil
}

func (repository *memoryRepository) GetEventByKey(ctx context.Context, linkID radarport.LinkID, keyDigest []byte) (radarport.Event, [32]byte, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return radarport.Event{}, [32]byte{}, err
	}
	key := trackingKey(linkID, keyDigest)
	index, exists := state.trackingByKey[key]
	if !exists {
		return radarport.Event{}, [32]byte{}, radarport.ErrNotFound
	}
	return state.trackingEvents[index], state.trackingDigests[key], nil
}

func (repository *memoryRepository) ListEvents(ctx context.Context, input radarport.EventListInput) ([]radarport.Event, int64, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return nil, 0, err
	}
	items := make([]radarport.Event, 0, len(state.trackingEvents))
	for _, event := range state.trackingEvents {
		if event.LinkID != input.LinkID || input.Stage != nil && event.Stage != *input.Stage {
			continue
		}
		if input.Start != nil && event.CreatedAt.Before(*input.Start) || input.End != nil && event.CreatedAt.After(*input.End) {
			continue
		}
		items = append(items, event)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].EventID > items[j].EventID })
	total := int64(len(items))
	start := min(int(input.Offset), len(items))
	end := min(start+int(input.Limit), len(items))
	return append([]radarport.Event(nil), items[start:end]...), total, nil
}

func (repository *memoryRepository) EventStats(ctx context.Context, linkID radarport.LinkID) (radarport.EventStatsRecord, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return radarport.EventStatsRecord{}, err
	}
	var result radarport.EventStatsRecord
	for _, event := range state.trackingEvents {
		if event.LinkID != linkID {
			continue
		}
		result.TotalEvents++
		result.LastEventAt = latestTime(result.LastEventAt, event.CreatedAt)
		switch event.Stage {
		case radarport.EventStageLanding:
			result.TotalLandings++
			result.TodayLandings++
			result.LastClickedAt = latestTime(result.LastClickedAt, event.CreatedAt)
		case radarport.EventStageRedirect:
			result.Redirects++
		case radarport.EventStageViewerOpen:
			result.ViewerOpens++
			result.LastViewedAt = latestTime(result.LastViewedAt, event.CreatedAt)
		case radarport.EventStageImageLoaded:
			result.ImageLoaded++
		case radarport.EventStagePDFOpened:
			result.PDFOpened++
		}
	}
	return result, nil
}

func (repository *memoryRepository) ListEnabledForSidebar(ctx context.Context, limit, offset int32) ([]radarport.SidebarLink, int64, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return nil, 0, err
	}
	items := make([]radarport.SidebarLink, 0, len(state.links))
	for _, link := range state.links {
		if link.Status != radarport.StatusEnabled {
			continue
		}
		items = append(items, radarport.SidebarLink{LinkID: link.LinkID, Title: link.Title, TargetType: "url", TypeLabel: "Radar", URL: link.PublicCode, UpdatedAt: link.UpdatedAt})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].LinkID > items[j].LinkID })
	total := int64(len(items))
	start := min(int(offset), len(items))
	end := min(start+int(limit), len(items))
	return items[start:end], total, nil
}

func (repository *memoryRepository) ReserveIdempotency(ctx context.Context, reservation radarport.ReserveIdempotencyRecord) (radarport.IdempotencyRecord, bool, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return radarport.IdempotencyRecord{}, false, err
	}
	key := idempotencyMapKey(reservation.ActorID, reservation.KeyDigest)
	if record, exists := state.idempotency[key]; exists {
		return cloneIdempotency(record), false, nil
	}
	state.nextIdempotencyID++
	record := radarport.IdempotencyRecord{
		RecordID:      state.nextIdempotencyID,
		ActorID:       reservation.ActorID,
		KeyDigest:     reservation.KeyDigest,
		Operation:     reservation.Operation,
		PayloadDigest: reservation.PayloadDigest,
		State:         radarport.IdempotencyReserved,
		CreatedAt:     reservation.CreatedAt,
	}
	state.idempotency[key] = record
	return record, true, nil
}

func (repository *memoryRepository) CompleteIdempotency(ctx context.Context, recordID int64, result radarport.Link, now time.Time) (radarport.IdempotencyRecord, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return radarport.IdempotencyRecord{}, err
	}
	if repository.db.failComplete {
		return radarport.IdempotencyRecord{}, errors.New("injected idempotency completion failure")
	}
	for key, record := range state.idempotency {
		if record.RecordID != recordID {
			continue
		}
		if record.State != radarport.IdempotencyReserved {
			return radarport.IdempotencyRecord{}, radarport.ErrIdempotencyStateInvalid
		}
		record.State = radarport.IdempotencyCompleted
		clonedResult := cloneLink(result)
		record.Result = &clonedResult
		completed := now
		record.CompletedAt = &completed
		state.idempotency[key] = cloneIdempotency(record)
		return record, nil
	}
	return radarport.IdempotencyRecord{}, radarport.ErrIdempotencyStateInvalid
}

func (appender *memoryAppender) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	state, err := memoryStateFromContext(ctx)
	if err != nil {
		return 0, err
	}
	if appender.db.failEvent {
		return 0, errors.New("injected event failure")
	}
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	state.events = append(state.events, event)
	return eventport.EventID(len(state.events)), nil
}

func TestServiceLifecycleCASAndIdempotency(t *testing.T) {
	service, db := newServiceFixture(t)
	ctx := context.Background()
	coverID := int64(7)
	create := radarport.CreateCommand{
		ExpectedVersion: 0,
		Name:            "Launch guide",
		Title:           "Read the launch guide",
		DestinationURL:  "https://docs.example.com/launch",
		CoverImageID:    &coverID,
		ActorID:         41,
		IdempotencyKey:  "radar-create-key-0001",
	}
	created, err := service.Create(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	if created.Link.LinkID != 1 || created.Link.Status != radarport.StatusDraft || created.Link.Version != 1 || !created.LocalProjection || created.RealExternalCallExecuted {
		t.Fatalf("created=%+v", created)
	}
	got, err := service.Get(ctx, created.Link.LinkID)
	if err != nil || !reflect.DeepEqual(created, got) {
		t.Fatalf("get=%+v err=%v", got, err)
	}

	replayed, err := service.Create(ctx, create)
	if err != nil || !reflect.DeepEqual(created, replayed) {
		t.Fatalf("replay=%+v err=%v", replayed, err)
	}
	conflictingCreate := create
	conflictingCreate.Title = "Different"
	if _, err = service.Create(ctx, conflictingCreate); !errors.Is(err, radarport.ErrIdempotencyConflict) {
		t.Fatalf("create conflict err=%v", err)
	}

	updated, err := service.Update(ctx, radarport.UpdateCommand{
		LinkID:          created.Link.LinkID,
		ExpectedVersion: 1,
		Title:           radarport.OptionalString{Set: true, Value: "Updated title"},
		CoverImageID:    radarport.OptionalNullableID{Set: true, Value: nil},
		ActorID:         42,
		IdempotencyKey:  "radar-update-key-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Link.Version != 2 || updated.Link.Title != "Updated title" || updated.Link.CoverImageID != nil || updated.Link.PublicCode != created.Link.PublicCode {
		t.Fatalf("updated=%+v", updated)
	}
	replayedUpdate, err := service.Update(ctx, radarport.UpdateCommand{
		LinkID:          created.Link.LinkID,
		ExpectedVersion: 1,
		Title:           radarport.OptionalString{Set: true, Value: "Updated title"},
		CoverImageID:    radarport.OptionalNullableID{Set: true, Value: nil},
		ActorID:         42,
		IdempotencyKey:  "radar-update-key-0001",
	})
	if err != nil || !reflect.DeepEqual(updated, replayedUpdate) {
		t.Fatalf("update replay=%+v err=%v", replayedUpdate, err)
	}
	if _, err = service.Update(ctx, radarport.UpdateCommand{
		LinkID:          created.Link.LinkID,
		ExpectedVersion: 1,
		Name:            radarport.OptionalString{Set: true, Value: "Stale"},
		ActorID:         42,
		IdempotencyKey:  "radar-update-key-0002",
	}); !errors.Is(err, radarport.ErrConflict) {
		t.Fatalf("CAS err=%v", err)
	}

	enabled, err := service.SetStatus(ctx, radarport.SetStatusCommand{
		LinkID:          created.Link.LinkID,
		ExpectedVersion: 2,
		Target:          radarport.StatusEnabled,
		ActorID:         42,
		IdempotencyKey:  "radar-enable-key-0001",
	})
	if err != nil || enabled.Link.Status != radarport.StatusEnabled || enabled.Link.Version != 3 {
		t.Fatalf("enabled=%+v err=%v", enabled, err)
	}
	replayedEnable, err := service.SetStatus(ctx, radarport.SetStatusCommand{
		LinkID:          created.Link.LinkID,
		ExpectedVersion: 2,
		Target:          radarport.StatusEnabled,
		ActorID:         42,
		IdempotencyKey:  "radar-enable-key-0001",
	})
	if err != nil || !reflect.DeepEqual(enabled, replayedEnable) {
		t.Fatalf("replayed enable=%+v err=%v", replayedEnable, err)
	}
	idempotentEnable, err := service.SetStatus(ctx, radarport.SetStatusCommand{
		LinkID:          created.Link.LinkID,
		ExpectedVersion: 3,
		Target:          radarport.StatusEnabled,
		ActorID:         42,
		IdempotencyKey:  "radar-enable-key-0002",
	})
	if err != nil || !reflect.DeepEqual(enabled, idempotentEnable) {
		t.Fatalf("same-state enable=%+v err=%v", idempotentEnable, err)
	}
	if _, err = service.SetStatus(ctx, radarport.SetStatusCommand{
		LinkID:          created.Link.LinkID,
		ExpectedVersion: 2,
		Target:          radarport.StatusEnabled,
		ActorID:         42,
		IdempotencyKey:  "radar-enable-key-stale",
	}); !errors.Is(err, radarport.ErrConflict) {
		t.Fatalf("same-state stale CAS err=%v", err)
	}
	disabled, err := service.SetStatus(ctx, radarport.SetStatusCommand{
		LinkID:          created.Link.LinkID,
		ExpectedVersion: 3,
		Target:          radarport.StatusDisabled,
		ActorID:         43,
		IdempotencyKey:  "radar-disable-" + strings.Repeat("x", 16),
	})
	if err != nil || disabled.Link.Status != radarport.StatusDisabled || disabled.Link.Version != 4 {
		t.Fatalf("disabled=%+v err=%v", disabled, err)
	}

	share, err := service.Share(ctx, created.Link.LinkID)
	if err != nil {
		t.Fatal(err)
	}
	if share.Available || share.SharePath != "/r/"+share.PublicCode || share.QRPayload != share.SharePath || !share.LocalProjection || !share.PublicRouteReady || share.RealExternalCallExecuted {
		t.Fatalf("share=%+v", share)
	}
	options := service.Options(ctx)
	if !reflect.DeepEqual(options.Statuses, []radarport.Status{radarport.StatusDraft, radarport.StatusEnabled, radarport.StatusDisabled}) || !options.PublicRouteReady || options.RealExternalCallExecuted || !options.LocalProjection {
		t.Fatalf("options=%+v", options)
	}

	state := db.snapshot()
	if len(state.links) != 1 || len(state.events) != 4 {
		t.Fatalf("links/events=%d/%d", len(state.links), len(state.events))
	}
	if len(state.idempotency) != 5 { // create, update, enable x2, disable; failed CAS record rolled back.
		t.Fatalf("idempotency_records=%d", len(state.idempotency))
	}
}

func TestServiceFailsClosedForUnavailableOrMissingCoverImage(t *testing.T) {
	service, db := newServiceFixture(t)
	coverID := int64(19)
	service.images = nil
	_, err := service.Create(context.Background(), radarport.CreateCommand{
		ExpectedVersion: 0,
		Name:            "No image reader",
		Title:           "No image reader",
		DestinationURL:  "https://example.com/no-reader",
		CoverImageID:    &coverID,
		ActorID:         7,
		IdempotencyKey:  "radar-image-reader-0001",
	})
	if !errors.Is(err, radarport.ErrUnavailable) || len(db.snapshot().links) != 0 || len(db.snapshot().idempotency) != 0 {
		t.Fatalf("unavailable err=%v state=%+v", err, db.snapshot())
	}

	images := &memoryImageReader{exists: false}
	service.images = images
	_, err = service.Create(context.Background(), radarport.CreateCommand{
		ExpectedVersion: 0,
		Name:            "Missing image",
		Title:           "Missing image",
		DestinationURL:  "https://example.com/missing",
		CoverImageID:    &coverID,
		ActorID:         7,
		IdempotencyKey:  "radar-image-reader-0002",
	})
	if !errors.Is(err, radarport.ErrInvalidArgument) || images.calls != 1 || len(db.snapshot().links) != 0 || len(db.snapshot().idempotency) != 0 {
		t.Fatalf("missing err=%v calls=%d state=%+v", err, images.calls, db.snapshot())
	}
}

func TestServiceFailsClosedForUnavailableOrMissingAttachment(t *testing.T) {
	service, db := newServiceFixture(t)
	attachmentID := int64(29)
	_, err := service.Create(context.Background(), radarport.CreateCommand{
		ExpectedVersion: 0,
		Name:            "No attachment reader",
		Title:           "No attachment reader",
		DestinationURL:  "https://example.com/no-attachment-reader",
		AttachmentID:    &attachmentID,
		ActorID:         7,
		IdempotencyKey:  "radar-attachment-reader-01",
	})
	if !errors.Is(err, radarport.ErrUnavailable) || len(db.snapshot().links) != 0 || len(db.snapshot().idempotency) != 0 {
		t.Fatalf("unavailable err=%v state=%+v", err, db.snapshot())
	}

	attachments := &memoryAttachmentReader{exists: false}
	service.attachments = attachments
	_, err = service.Create(context.Background(), radarport.CreateCommand{
		ExpectedVersion: 0,
		Name:            "Missing attachment",
		Title:           "Missing attachment",
		DestinationURL:  "https://example.com/missing-attachment",
		AttachmentID:    &attachmentID,
		ActorID:         7,
		IdempotencyKey:  "radar-attachment-reader-02",
	})
	if !errors.Is(err, radarport.ErrInvalidArgument) || attachments.calls != 1 || len(db.snapshot().links) != 0 || len(db.snapshot().idempotency) != 0 {
		t.Fatalf("missing err=%v calls=%d state=%+v", err, attachments.calls, db.snapshot())
	}
}

func TestServiceStateConflictAndTransactionRollback(t *testing.T) {
	service, db := newServiceFixture(t)
	ctx := context.Background()
	created, err := service.Create(ctx, radarport.CreateCommand{
		ExpectedVersion: 0,
		Name:            "Draft",
		Title:           "Draft title",
		DestinationURL:  "https://example.com/draft",
		ActorID:         1,
		IdempotencyKey:  "radar-create-draft01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetStatus(ctx, radarport.SetStatusCommand{
		LinkID:          created.Link.LinkID,
		ExpectedVersion: 1,
		Target:          radarport.StatusDisabled,
		ActorID:         1,
		IdempotencyKey:  "radar-disable-draft1",
	}); !errors.Is(err, radarport.ErrStateConflict) {
		t.Fatalf("state conflict err=%v", err)
	}
	before := db.snapshot()
	if len(before.links) != 1 || len(before.idempotency) != 1 || len(before.events) != 1 {
		t.Fatalf("before=%+v", before)
	}

	db.mu.Lock()
	db.failEvent = true
	db.mu.Unlock()
	if _, err = service.Update(ctx, radarport.UpdateCommand{
		LinkID:          created.Link.LinkID,
		ExpectedVersion: 1,
		Name:            radarport.OptionalString{Set: true, Value: "Must roll back"},
		ActorID:         2,
		IdempotencyKey:  "radar-rollback-event1",
	}); !errors.Is(err, radarport.ErrUnavailable) {
		t.Fatalf("event rollback err=%v", err)
	}
	afterEventFailure := db.snapshot()
	if !reflect.DeepEqual(before, afterEventFailure) {
		t.Fatalf("event failure committed state\nbefore=%+v\nafter=%+v", before, afterEventFailure)
	}

	db.mu.Lock()
	db.failEvent = false
	db.failComplete = true
	db.mu.Unlock()
	if _, err = service.Update(ctx, radarport.UpdateCommand{
		LinkID:          created.Link.LinkID,
		ExpectedVersion: 1,
		Name:            radarport.OptionalString{Set: true, Value: "Must also roll back"},
		ActorID:         2,
		IdempotencyKey:  "radar-rollback-idem001",
	}); !errors.Is(err, radarport.ErrUnavailable) {
		t.Fatalf("idempotency rollback err=%v", err)
	}
	afterIdempotencyFailure := db.snapshot()
	if !reflect.DeepEqual(before, afterIdempotencyFailure) {
		t.Fatalf("idempotency completion failure committed state\nbefore=%+v\nafter=%+v", before, afterIdempotencyFailure)
	}
}

func TestServiceListStablePaginationAndClosedFilter(t *testing.T) {
	service, _ := newServiceFixture(t)
	ctx := context.Background()
	for index, name := range []string{"Charlie", "Alpha", "Bravo"} {
		result, err := service.Create(ctx, radarport.CreateCommand{
			ExpectedVersion: 0,
			Name:            name,
			Title:           name + " title",
			DestinationURL:  "https://example.com/" + strconv.Itoa(index),
			ActorID:         1,
			IdempotencyKey:  "radar-list-create-" + strconv.Itoa(index) + "000",
		})
		if err != nil {
			t.Fatal(err)
		}
		if index == 1 {
			if _, err = service.SetStatus(ctx, radarport.SetStatusCommand{
				LinkID:          result.Link.LinkID,
				ExpectedVersion: 1,
				Target:          radarport.StatusEnabled,
				ActorID:         1,
				IdempotencyKey:  "radar-list-enable-0001",
			}); err != nil {
				t.Fatal(err)
			}
		}
	}

	first, err := service.List(ctx, radarport.ListInput{Status: radarport.StatusFilterAll, Sort: radarport.SortNameAsc, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 3 || len(first.Items) != 2 || first.Items[0].Name != "Alpha" || first.Items[1].Name != "Bravo" || !first.HasMore {
		t.Fatalf("first=%+v", first)
	}
	second, err := service.List(ctx, radarport.ListInput{Status: radarport.StatusFilterAll, Sort: radarport.SortNameAsc, Limit: 2, Offset: 2})
	if err != nil || len(second.Items) != 1 || second.Items[0].Name != "Charlie" || second.HasMore {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	enabled, err := service.List(ctx, radarport.ListInput{Status: radarport.StatusFilterEnabled, Sort: radarport.SortUpdatedDesc, Limit: 100})
	if err != nil || enabled.Total != 1 || len(enabled.Items) != 1 || enabled.Items[0].Name != "Alpha" {
		t.Fatalf("enabled=%+v err=%v", enabled, err)
	}
	if _, err = service.List(ctx, radarport.ListInput{Status: "unknown", Limit: 20}); !errors.Is(err, radarport.ErrInvalidArgument) {
		t.Fatalf("closed filter err=%v", err)
	}
}

func TestServiceRetriesPublicCodeCollisionAndKeepsNoOpUpdateStable(t *testing.T) {
	service, db := newServiceFixture(t)
	ctx := context.Background()
	first, err := service.Create(ctx, radarport.CreateCommand{
		ExpectedVersion: 0,
		Name:            "First",
		Title:           "First title",
		DestinationURL:  "https://example.com/first",
		ActorID:         1,
		IdempotencyKey:  "radar-collision-first1",
	})
	if err != nil {
		t.Fatal(err)
	}
	codes := []string{first.Link.PublicCode, "rd_ZZZZZZZZZZZZZZZZZZZZZZ"}
	var index int
	service.generatePublicCode = func() (string, error) {
		code := codes[index]
		index++
		return code, nil
	}
	second, err := service.Create(ctx, radarport.CreateCommand{
		ExpectedVersion: 0,
		Name:            "Second",
		Title:           "Second title",
		DestinationURL:  "https://example.com/second",
		ActorID:         1,
		IdempotencyKey:  "radar-collision-second",
	})
	if err != nil || second.Link.PublicCode != codes[1] || second.Link.LinkID == first.Link.LinkID || index != 2 {
		t.Fatalf("second=%+v attempts=%d err=%v", second, index, err)
	}

	noOp, err := service.Update(ctx, radarport.UpdateCommand{
		LinkID:          first.Link.LinkID,
		ExpectedVersion: first.Link.Version,
		Title:           radarport.OptionalString{Set: true, Value: first.Link.Title},
		ActorID:         2,
		IdempotencyKey:  "radar-noop-update-001",
	})
	if err != nil || noOp.Link.Version != first.Link.Version || noOp.Link.UpdatedBy != first.Link.UpdatedBy || !noOp.Link.UpdatedAt.Equal(first.Link.UpdatedAt) {
		t.Fatalf("no-op=%+v err=%v", noOp, err)
	}
	state := db.snapshot()
	if len(state.links) != 2 || len(state.events) != 2 || len(state.idempotency) != 3 {
		t.Fatalf("state links/events/idempotency=%d/%d/%d", len(state.links), len(state.events), len(state.idempotency))
	}
}

func TestServiceLocalTrackingClosedLoop(t *testing.T) {
	service, db := newServiceFixture(t)
	ctx := context.Background()
	created, err := service.Create(ctx, radarport.CreateCommand{
		ExpectedVersion: 0, Name: "Tracked", Title: "Tracked title",
		DestinationURL: "https://example.com/tracked", ActorID: 7,
		IdempotencyKey: "radar-tracking-create-001",
	})
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := service.SetStatus(ctx, radarport.SetStatusCommand{
		LinkID: created.Link.LinkID, ExpectedVersion: created.Link.Version,
		Target: radarport.StatusEnabled, ActorID: 7, IdempotencyKey: "radar-tracking-enable-001",
	})
	if err != nil {
		t.Fatal(err)
	}
	share, err := service.Share(ctx, enabled.Link.LinkID)
	if err != nil || !share.Available || !share.PublicRouteReady || share.SharePath != "/r/"+enabled.Link.PublicCode || share.QRPayload != share.SharePath {
		t.Fatalf("share=%+v err=%v", share, err)
	}

	redirect, err := service.ResolvePublicRedirect(ctx, enabled.Link.PublicCode)
	if err != nil {
		t.Fatal(err)
	}
	if redirect.DestinationURL != enabled.Link.DestinationURL || !redirect.Receipt.LocalReceipt || redirect.Receipt.IdentityAttributed || redirect.Receipt.RealExternalCallExecuted {
		t.Fatalf("redirect=%+v", redirect)
	}
	page := int32(3)
	command := radarport.RecordEventCommand{
		PublicCode: enabled.Link.PublicCode, Stage: radarport.EventStagePDFPageLoaded,
		Page: &page, Extra: map[string]any{"variant": "mobile"}, IdempotencyKey: "radar-public-event-001",
	}
	first, err := service.RecordPublicEvent(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.RecordPublicEvent(ctx, command)
	if err != nil || !replay.Replayed || replay.ReceiptID != first.ReceiptID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	command.Extra["variant"] = "desktop"
	if _, err = service.RecordPublicEvent(ctx, command); !errors.Is(err, radarport.ErrIdempotencyConflict) {
		t.Fatalf("idempotency conflict err=%v", err)
	}

	pageResult, err := service.ListEvents(ctx, radarport.EventListInput{LinkID: enabled.Link.LinkID, Limit: 10})
	if err != nil || pageResult.Total != 3 || len(pageResult.Items) != 3 || pageResult.IdentityAttributed || pageResult.RealExternalCallExecuted {
		t.Fatalf("events=%+v err=%v", pageResult, err)
	}
	stats, err := service.EventStats(ctx, enabled.Link.LinkID)
	if err != nil || stats.TotalEvents != 3 || stats.TotalLandings != 1 || stats.Redirects != 1 || stats.AuthorizedUsers != 0 || stats.IdentityAttributed || stats.RealExternalCallExecuted {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	sidebar, err := service.SidebarLinks(ctx, 10, 0, "https://crm.example.com/")
	if err != nil || sidebar.Total != 1 || len(sidebar.Items) != 1 || sidebar.Items[0].URL != "https://crm.example.com/r/"+enabled.Link.PublicCode || !sidebar.LocalProjection {
		t.Fatalf("sidebar=%+v err=%v", sidebar, err)
	}
	state := db.snapshot()
	if len(state.trackingEvents) != 3 || len(state.trackingByKey) != 1 {
		t.Fatalf("tracking state=%+v", state)
	}
}

func newServiceFixture(t *testing.T) (*Service, *memoryDB) {
	t.Helper()
	db := newMemoryDB()
	service, err := NewServiceWithImageReferences(&memoryUnitOfWork{db: db}, &memoryRepository{db: db}, &memoryImageReader{exists: true}, &memoryAppender{db: db})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 22, 1, 2, 3, 0, time.UTC)
	var clockStep int64
	service.now = func() time.Time {
		clockStep++
		return base.Add(time.Duration(clockStep) * time.Second)
	}
	codes := []string{
		"rd_AAAAAAAAAAAAAAAAAAAAAA",
		"rd_BBBBBBBBBBBBBBBBBBBBBB",
		"rd_CCCCCCCCCCCCCCCCCCCCCC",
		"rd_DDDDDDDDDDDDDDDDDDDDDD",
		"rd_EEEEEEEEEEEEEEEEEEEEEE",
	}
	var codeIndex int
	service.generatePublicCode = func() (string, error) {
		if codeIndex >= len(codes) {
			return "", errors.New("no test codes")
		}
		code := codes[codeIndex]
		codeIndex++
		return code, nil
	}
	var receiptIndex int
	service.generateReceiptID = func() (string, error) {
		receiptIndex++
		return fmt.Sprintf("rre_%032x", receiptIndex), nil
	}
	return service, db
}

type memoryImageReader struct {
	exists bool
	err    error
	calls  int
}

func (reader *memoryImageReader) ImageExists(context.Context, int64) (bool, error) {
	reader.calls++
	return reader.exists, reader.err
}

type memoryAttachmentReader struct {
	exists bool
	err    error
	calls  int
}

func (reader *memoryAttachmentReader) AttachmentExists(context.Context, int64) (bool, error) {
	reader.calls++
	return reader.exists, reader.err
}

func memoryStateFromContext(ctx context.Context) (*memoryState, error) {
	state, ok := ctx.Value(memoryContextKey{}).(*memoryState)
	if !ok || state == nil {
		return nil, radarport.ErrUnavailable
	}
	return state, nil
}

func (db *memoryDB) snapshot() memoryState {
	db.mu.Lock()
	defer db.mu.Unlock()
	return cloneMemoryState(db.state)
}

func cloneMemoryState(source memoryState) memoryState {
	cloned := memoryState{
		links:             make(map[radarport.LinkID]radarport.Link, len(source.links)),
		idempotency:       make(map[string]radarport.IdempotencyRecord, len(source.idempotency)),
		events:            make([]eventport.Event, len(source.events)),
		trackingEvents:    make([]radarport.Event, len(source.trackingEvents)),
		trackingByKey:     make(map[string]int, len(source.trackingByKey)),
		trackingDigests:   make(map[string][32]byte, len(source.trackingDigests)),
		nextLinkID:        source.nextLinkID,
		nextIdempotencyID: source.nextIdempotencyID,
	}
	for key, link := range source.links {
		cloned.links[key] = cloneLink(link)
	}
	for key, record := range source.idempotency {
		cloned.idempotency[key] = cloneIdempotency(record)
	}
	for index, event := range source.events {
		event.Payload = append(json.RawMessage(nil), event.Payload...)
		cloned.events[index] = event
	}
	copy(cloned.trackingEvents, source.trackingEvents)
	for key, index := range source.trackingByKey {
		cloned.trackingByKey[key] = index
	}
	for key, digest := range source.trackingDigests {
		cloned.trackingDigests[key] = digest
	}
	return cloned
}

func trackingKey(linkID radarport.LinkID, digest []byte) string {
	return fmt.Sprintf("%d:%x", linkID, digest)
}

func clonePage(page *int32) *int32 {
	if page == nil {
		return nil
	}
	cloned := *page
	return &cloned
}

func latestTime(existing *time.Time, candidate time.Time) *time.Time {
	if existing != nil && !candidate.After(*existing) {
		return existing
	}
	cloned := candidate
	return &cloned
}

func cloneIdempotency(record radarport.IdempotencyRecord) radarport.IdempotencyRecord {
	if record.Result != nil {
		result := cloneLink(*record.Result)
		record.Result = &result
	}
	if record.CompletedAt != nil {
		completed := *record.CompletedAt
		record.CompletedAt = &completed
	}
	return record
}

func idempotencyMapKey(actorID int64, digest [32]byte) string {
	return strconv.FormatInt(actorID, 10) + ":" + hex.EncodeToString(digest[:])
}
