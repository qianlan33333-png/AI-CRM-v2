package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

func TestC01ChannelCreateReplayUpdateListAndGet(t *testing.T) {
	service, store, events := channelTestService()
	created, err := service.CreateChannel(context.Background(), CreateChannelCommand{Actor: 7, IdempotencyKey: "channel-create-key-0001", ChannelName: "公开课", LegacyProjection: json.RawMessage(`{"owner_staff_id":"staff-7","welcome_message":"欢迎"}`)})
	if err != nil || created.ID != 1 || created.ChannelCode == "" || created.Status != "active" || len(events.items) != 1 || events.items[0].Type != eventport.EvChannelCreated {
		t.Fatalf("create=%#v events=%#v err=%v", created, events.items, err)
	}
	replay, err := service.CreateChannel(context.Background(), CreateChannelCommand{Actor: 7, IdempotencyKey: "channel-create-key-0001", ChannelName: "公开课", LegacyProjection: json.RawMessage(`{"owner_staff_id":"staff-7","welcome_message":"欢迎"}`)})
	if err != nil || replay.ID != created.ID || store.creates != 1 || len(events.items) != 1 {
		t.Fatalf("replay=%#v creates=%d events=%d err=%v", replay, store.creates, len(events.items), err)
	}
	updated, err := service.UpdateChannel(context.Background(), UpdateChannelCommand{Actor: 7, ChannelID: created.ID, IdempotencyKey: "channel-update-key-0001", Patch: json.RawMessage(`{"status":"archived"}`)})
	if err != nil || updated.Status != "archived" || len(events.items) != 2 || events.items[1].Type != eventport.EvChannelUpdated {
		t.Fatalf("update=%#v events=%#v err=%v", updated, events.items, err)
	}
	rows, err := service.ListChannels(context.Background(), 100, "archived", true)
	if err != nil || len(rows) != 1 || rows[0].ID != created.ID {
		t.Fatalf("list=%#v err=%v", rows, err)
	}
	got, err := service.GetChannel(context.Background(), created.ID)
	if err != nil || got.Status != "archived" {
		t.Fatalf("get=%#v err=%v", got, err)
	}
}

func TestCH01ChannelReplaysLegacyAssigneeReceiptWithoutSecondWriteOrEvent(t *testing.T) {
	service, store, events := channelTestService()
	command := CreateChannelCommand{
		Actor: 7, IdempotencyKey: "channel-legacy-replay-key-0001", ChannelName: "公开课",
		LegacyProjection: json.RawMessage(`{"owner_staff_id":"staff-7"}`),
	}
	created, err := service.CreateChannel(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	var receiptKey string
	var legacySnapshot json.RawMessage
	for key, receipt := range store.receipts {
		var snapshot map[string]json.RawMessage
		if err = json.Unmarshal(receipt.ResultSnapshot, &snapshot); err != nil {
			t.Fatal(err)
		}
		snapshot["assignees"] = json.RawMessage(`[{"wecom_userid":"staff-7","display_name":"成员 7"}]`)
		legacySnapshot, err = json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		receipt.ResultSnapshot = append(json.RawMessage(nil), legacySnapshot...)
		store.receipts[key] = receipt
		receiptKey = key
	}
	if receiptKey == "" {
		t.Fatal("completed receipt is missing")
	}

	replay, err := service.CreateChannel(context.Background(), command)
	if err != nil || replay.ID != created.ID || len(replay.Assignees) != 1 || replay.Assignees[0].WeComUserID != "staff-7" || replay.Assignees[0].Status != "active" || replay.Assignees[0].Priority != 1 {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	if store.creates != 1 || store.updates != 0 || store.completes != 1 || len(events.items) != 1 || string(store.receipts[receiptKey].ResultSnapshot) != string(legacySnapshot) {
		t.Fatalf("creates/updates/completes/events/snapshot=%d/%d/%d/%d/%s", store.creates, store.updates, store.completes, len(events.items), store.receipts[receiptKey].ResultSnapshot)
	}
}

func TestC01ChannelBoundariesAndActorScopedReceipt(t *testing.T) {
	service, store, events := channelTestService()
	base := CreateChannelCommand{Actor: 7, IdempotencyKey: "channel-shared-key-0001", ChannelCode: "course", ChannelName: "课程"}
	if _, err := service.CreateChannel(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	conflict := base
	conflict.ChannelName = "冲突"
	if _, err := service.CreateChannel(context.Background(), conflict); !errors.Is(err, ErrChannelConflict) {
		t.Fatalf("payload conflict=%v", err)
	}
	second := base
	second.Actor = 8
	second.ChannelCode = "course-2"
	if _, err := service.CreateChannel(context.Background(), second); err != nil {
		t.Fatalf("actor isolation=%v", err)
	}
	for _, command := range []CreateChannelCommand{
		{Actor: 7, IdempotencyKey: "short", ChannelName: "x"},
		{Actor: 7, IdempotencyKey: "channel-invalid-key-0001", ChannelName: "x", Status: "deleted"},
		{Actor: 7, IdempotencyKey: "channel-invalid-key-0002", ChannelName: "x", LegacyProjection: json.RawMessage(`{"tenant` + `_id":"forbidden"}`)},
	} {
		before := store.creates
		if _, err := service.CreateChannel(context.Background(), command); !errors.Is(err, ErrInvalidChannel) {
			t.Fatalf("command=%#v err=%v", command, err)
		}
		if store.creates != before {
			t.Fatal("invalid input leaked a channel write")
		}
	}
	if len(events.items) != 2 {
		t.Fatalf("events=%#v", events.items)
	}
}

func TestChannelAttachmentReferencesUseTransactionBoundReader(t *testing.T) {
	store := &channelStore{receipts: map[string]ChannelReceipt{}}
	events := &channelEvents{}
	attachments := &channelTestAttachments{available: map[int64]bool{19: true}}
	service := NewChannelServiceWithMediaReferences(channelUOW{}, store, channelTestImages{}, attachments, events)
	service.now = func() time.Time { return time.Date(2026, 8, 23, 11, 0, 0, 0, time.UTC) }
	created, err := service.CreateChannel(context.Background(), CreateChannelCommand{Actor: 7, IdempotencyKey: "channel-attachment-key-0001", ChannelName: "附件欢迎语", LegacyProjection: json.RawMessage(`{"welcome_attachment_library_ids":[19]}`)})
	if err != nil || created.ID != 1 || attachments.calls != 1 || len(events.items) != 1 {
		t.Fatalf("created=%+v err=%v calls=%d events=%+v", created, err, attachments.calls, events.items)
	}

	missingStore := &channelStore{receipts: map[string]ChannelReceipt{}}
	missingEvents := &channelEvents{}
	missing := &channelTestAttachments{available: map[int64]bool{}}
	missingService := NewChannelServiceWithMediaReferences(channelUOW{}, missingStore, channelTestImages{}, missing, missingEvents)
	missingService.now = service.now
	if _, err := missingService.CreateChannel(context.Background(), CreateChannelCommand{Actor: 7, IdempotencyKey: "channel-attachment-key-0002", ChannelName: "附件欢迎语", LegacyProjection: json.RawMessage(`{"welcome_attachment_library_ids":[19]}`)}); !errors.Is(err, ErrInvalidChannel) || missing.calls != 1 || missingStore.creates != 0 || len(missingEvents.items) != 0 {
		t.Fatalf("missing err=%v calls=%d creates=%d events=%+v", err, missing.calls, missingStore.creates, missingEvents.items)
	}
}

func TestChannelMaterialReferencesRequireLocallyEligibleReaders(t *testing.T) {
	for _, test := range []struct {
		name, projection string
		materials        channelTestMaterials
		want             error
	}{
		{"enabled", `{"welcome_attachment_library_ids":[19],"welcome_miniprogram_library_ids":[20],"welcome_group_invite_library_ids":[21]}`, channelTestMaterials{attachments: map[int64]bool{19: true}, miniprograms: map[int64]bool{20: true}, groupInvites: map[int64]bool{21: true}}, nil},
		{"disabled attachment", `{"welcome_attachment_library_ids":[19]}`, channelTestMaterials{attachments: map[int64]bool{19: false}}, ErrInvalidChannel},
		{"disabled miniprogram", `{"welcome_miniprogram_library_ids":[20]}`, channelTestMaterials{miniprograms: map[int64]bool{20: false}}, ErrInvalidChannel},
		{"archived group", `{"welcome_group_invite_library_ids":[21]}`, channelTestMaterials{groupInvites: map[int64]bool{21: false}}, ErrInvalidChannel},
		{"reader unavailable", `{"welcome_attachment_library_ids":[19]}`, channelTestMaterials{attachments: map[int64]bool{19: true}, err: errors.New("reader failed")}, ErrChannelUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &channelStore{receipts: map[string]ChannelReceipt{}}
			service := NewChannelServiceWithReferences(channelUOW{}, store, channelTestImages{}, &test.materials, &test.materials, &test.materials, nil, nil, &channelEvents{})
			service.now = func() time.Time { return time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC) }
			_, err := service.CreateChannel(context.Background(), CreateChannelCommand{Actor: 7, IdempotencyKey: "channel-material-key-0001", ChannelName: "素材", LegacyProjection: json.RawMessage(test.projection)})
			if !errors.Is(err, test.want) || test.want == nil && err != nil || test.want != nil && store.creates != 0 {
				t.Fatalf("err=%v creates=%d", err, store.creates)
			}
		})
	}
}

type channelUOW struct{}

func (channelUOW) Within(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type channelEvents struct{ items []eventport.Event }

type channelTestImages struct{}

func (channelTestImages) ImageExists(context.Context, int64) (bool, error) { return true, nil }

type channelTestAttachments struct {
	available map[int64]bool
	calls     int
}

func (reader *channelTestAttachments) AttachmentExists(_ context.Context, id int64) (bool, error) {
	reader.calls++
	return reader.available[id], nil
}

func (reader *channelTestAttachments) ChannelAttachmentEligible(_ context.Context, id int64) (bool, error) {
	reader.calls++
	return reader.available[id], nil
}

var _ mediaport.ImageMetadataReader = channelTestImages{}
var _ mediaport.AttachmentMetadataReader = (*channelTestAttachments)(nil)
var _ mediaport.ChannelAttachmentReferenceReader = (*channelTestAttachments)(nil)

type channelTestMaterials struct {
	attachments, miniprograms, groupInvites map[int64]bool
	err                                     error
}

func (reader *channelTestMaterials) ChannelAttachmentEligible(_ context.Context, id int64) (bool, error) {
	return reader.attachments[id], reader.err
}
func (reader *channelTestMaterials) ChannelMiniProgramEligible(_ context.Context, id int64) (bool, error) {
	return reader.miniprograms[id], reader.err
}
func (reader *channelTestMaterials) ChannelGroupInviteEligible(_ context.Context, id int64) (bool, error) {
	return reader.groupInvites[id], reader.err
}

var _ mediaport.ChannelAttachmentReferenceReader = (*channelTestMaterials)(nil)
var _ mediaport.ChannelMiniProgramReferenceReader = (*channelTestMaterials)(nil)
var _ mediaport.ChannelGroupInviteReferenceReader = (*channelTestMaterials)(nil)

func (events *channelEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	events.items = append(events.items, event)
	return eventport.EventID(len(events.items)), nil
}

type channelStore struct {
	items     []Channel
	receipts  map[string]ChannelReceipt
	creates   int
	updates   int
	completes int
}

func (store *channelStore) ListChannels(_ context.Context, limit int32, status string, includeArchived bool) ([]Channel, error) {
	result := []Channel{}
	for i := len(store.items) - 1; i >= 0; i-- {
		item := store.items[i]
		if status != "" && item.Status != status || !includeArchived && item.Status == "archived" {
			continue
		}
		result = append(result, item)
		if len(result) == int(limit) {
			break
		}
	}
	return result, nil
}
func (store *channelStore) GetChannel(_ context.Context, id int64) (Channel, error) {
	for _, item := range store.items {
		if item.ID == id {
			return item, nil
		}
	}
	return Channel{}, ErrChannelNotFound
}
func (store *channelStore) CreateChannel(_ context.Context, c CreateChannelCommand, now time.Time) (Channel, error) {
	store.creates++
	item := Channel{ID: int64(len(store.items) + 1), ChannelCode: c.ChannelCode, ChannelName: c.ChannelName, Status: c.Status, LegacyProjection: c.LegacyProjection, CreatedBy: c.Actor, UpdatedBy: c.Actor, CreatedAt: now, UpdatedAt: now}
	store.items = append(store.items, item)
	return item, nil
}
func (store *channelStore) UpdateChannel(_ context.Context, current Channel, actor int64, now time.Time) (Channel, error) {
	store.updates++
	for i := range store.items {
		if store.items[i].ID == current.ID {
			current.CreatedAt = store.items[i].CreatedAt
			current.CreatedBy = store.items[i].CreatedBy
			current.UpdatedBy = actor
			current.UpdatedAt = now
			store.items[i] = current
			return current, nil
		}
	}
	return Channel{}, ErrChannelNotFound
}
func (store *channelStore) ReserveChannel(_ context.Context, x ChannelReservation) (ChannelReceipt, bool, error) {
	key := x.Operation + ":" + x.ActorScope + ":" + fmt.Sprintf("%x", x.KeyDigest)
	if old, ok := store.receipts[key]; ok {
		return old, false, nil
	}
	r := ChannelReceipt{ID: int64(len(store.receipts) + 1), Operation: x.Operation, ActorScope: x.ActorScope, KeyDigest: x.KeyDigest, PayloadDigest: x.PayloadDigest, State: "in_progress"}
	store.receipts[key] = r
	return r, true, nil
}
func (store *channelStore) CompleteChannel(_ context.Context, id int64, snapshot json.RawMessage, _ time.Time) (ChannelReceipt, error) {
	store.completes++
	for key, r := range store.receipts {
		if r.ID == id {
			r.State = "completed"
			r.ResultSnapshot = append([]byte(nil), snapshot...)
			store.receipts[key] = r
			return r, nil
		}
	}
	return ChannelReceipt{}, ErrChannelUnavailable
}
func channelTestService() (*ChannelService, *channelStore, *channelEvents) {
	store := &channelStore{receipts: map[string]ChannelReceipt{}}
	events := &channelEvents{}
	service := NewChannelServiceWithLocalReferences(
		channelUOW{}, store, nil, channelTagReader{},
		channelStaffDirectory{entries: []contactport.StaffDirectoryEntry{{WeComUserID: "staff-7", DisplayName: "成员 7", UpdatedAt: time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)}}},
		events,
	)
	service.now = func() time.Time { return time.Date(2026, 8, 15, 11, 0, 0, 0, time.UTC) }
	return service, store, events
}

type channelTagReader struct{ records []contactport.TagReference }

func (reader channelTagReader) LockActiveTag(_ context.Context, id int64) (contactport.TagReference, error) {
	for _, record := range reader.records {
		if record.ID == id {
			return record, nil
		}
	}
	return contactport.TagReference{}, contactport.ErrTagReferenceNotFound
}

type channelStaffDirectory struct {
	entries []contactport.StaffDirectoryEntry
}

func (directory channelStaffDirectory) LockEligibleStaffByWeComUserID(_ context.Context, weComUserID string) (contactport.StaffDirectoryEntry, error) {
	for _, entry := range directory.entries {
		if entry.WeComUserID == weComUserID {
			return entry, nil
		}
	}
	return contactport.StaffDirectoryEntry{}, contactport.ErrStaffReferenceNotFound
}

func TestC01ChannelValidatesEntryTagAndProjectsEligibleOwner(t *testing.T) {
	groupName := "新客"
	service, store, _ := channelTestService()
	service.tags = channelTagReader{records: []contactport.TagReference{{ID: 8, Name: "已报名", GroupName: &groupName}}}
	created, err := service.CreateChannel(context.Background(), CreateChannelCommand{
		Actor: 7, IdempotencyKey: "channel-reference-key-0001", ChannelName: "公开课",
		LegacyProjection: json.RawMessage(`{"owner_staff_id":"staff-7","entry_tag_id":"8","entry_tag_name":"已报名","entry_tag_group_name":"新客"}`),
	})
	if err != nil || len(created.Assignees) != 1 || created.Assignees[0].WeComUserID != "staff-7" || created.Assignees[0].DisplayName != "成员 7" {
		t.Fatalf("created=%+v error=%v", created, err)
	}
	for index, projection := range []string{
		`{"owner_staff_id":"archived","entry_tag_id":"8","entry_tag_name":"已报名","entry_tag_group_name":"新客"}`,
		`{"owner_staff_id":"staff-7","entry_tag_id":"08","entry_tag_name":"已报名","entry_tag_group_name":"新客"}`,
		`{"owner_staff_id":"staff-7","entry_tag_id":"8","entry_tag_name":"错误","entry_tag_group_name":"新客"}`,
	} {
		before := store.creates
		if _, err := service.CreateChannel(context.Background(), CreateChannelCommand{Actor: 7, IdempotencyKey: fmt.Sprintf("channel-reference-invalid-%04d", index), ChannelName: "公开课", LegacyProjection: json.RawMessage(projection)}); !errors.Is(err, ErrInvalidChannel) {
			t.Fatalf("projection=%s error=%v", projection, err)
		}
		if store.creates != before {
			t.Fatalf("invalid projection wrote channel: %s", projection)
		}
	}
}

func TestCH01ChannelAssigneesUseVerifiedActiveStaffAndAssignmentRules(t *testing.T) {
	store := &channelStore{receipts: map[string]ChannelReceipt{}}
	events := &channelEvents{}
	staff := channelStaffDirectory{entries: []contactport.StaffDirectoryEntry{
		{WeComUserID: "staff-7", DisplayName: "成员 7", UpdatedAt: legacyChannelTestTime},
		{WeComUserID: "staff-8", DisplayName: "成员 8", UpdatedAt: legacyChannelTestTime},
	}}
	service := NewChannelServiceWithLocalReferences(channelUOW{}, store, nil, nil, staff, events)
	service.now = func() time.Time { return legacyChannelTestTime }
	created, err := service.CreateChannel(context.Background(), CreateChannelCommand{
		Actor: 7, IdempotencyKey: "channel-assignees-key-0001", ChannelName: "公开课",
		LegacyProjection: json.RawMessage(`{"assignment_mode":"multi_staff","assignment_strategy":"ratio","assignees":[{"staff_id":"staff-7","ratio_percent":40},{"staff_id":"staff-8","ratio_percent":60}]}`),
	})
	if err != nil || len(created.Assignees) != 2 || created.Assignees[0].WeComUserID != "staff-7" || created.Assignees[1].WeComUserID != "staff-8" || created.Assignees[0].RatioPercent == nil || *created.Assignees[0].RatioPercent != 40 || created.Assignees[1].RatioPercent == nil || *created.Assignees[1].RatioPercent != 60 {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	var projection map[string]json.RawMessage
	if json.Unmarshal(created.LegacyProjection, &projection) != nil {
		t.Fatal("invalid normalized projection")
	}
	var owner string
	if json.Unmarshal(projection["owner_staff_id"], &owner) != nil || owner != "staff-7" {
		t.Fatalf("owner=%q projection=%s", owner, created.LegacyProjection)
	}
	for index, raw := range []string{
		`{"assignment_mode":"multi_staff","assignment_strategy":"ratio","assignees":[{"staff_id":"staff-7","ratio_percent":99}]}`,
		`{"assignment_mode":"single_owner","assignment_strategy":"ratio","assignees":[{"staff_id":"staff-7","ratio_percent":50},{"staff_id":"staff-8","ratio_percent":50}]}`,
		`{"assignment_mode":"multi_staff","assignment_strategy":"cap_switch","assignees":[{"staff_id":"staff-7"}]}`,
		`{"assignment_mode":"multi_staff","assignment_strategy":"ratio","assignees":[{"staff_id":"staff-7","ratio_percent":100},{"staff_id":"staff-7","ratio_percent":0}]}`,
		`{"assignment_mode":"multi_staff","assignment_strategy":"ratio","assignees":[{"staff_id":"staff-9","ratio_percent":100}]}`,
	} {
		before := store.creates
		_, err := service.CreateChannel(context.Background(), CreateChannelCommand{Actor: 7, IdempotencyKey: fmt.Sprintf("channel-assignees-invalid-%04d", index), ChannelName: "公开课", LegacyProjection: json.RawMessage(raw)})
		if !errors.Is(err, ErrInvalidChannel) || store.creates != before {
			t.Fatalf("projection=%s err=%v creates=%d", raw, err, store.creates)
		}
	}
}

var legacyChannelTestTime = time.Date(2026, time.August, 25, 10, 0, 0, 0, time.UTC)
