package app

import (
	"context"
	"errors"
	"testing"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestAudienceActivityWriterReadbackReplayAndPrivateDigest(t *testing.T) {
	store := newAudienceActivityStore()
	store.packages[2] = segmentport.AudienceActivityPackageReference{ID: 2}
	store.versions[3] = segmentport.AudienceActivityVersionReference{ID: 3, PackageHistoryID: 2}
	journal := &audienceActivityJournal{values: map[string]segmentport.AudienceActivityHistoryReceipt{}}
	writer := NewAudienceActivityHistoryWriter(store, journal)
	value := activityRun(2, activityID(3))
	first, err := writer.WriteRun(context.Background(), "run-source", digest(1), value)
	if err != nil || first.Replayed || first.TargetID != 1 || len(store.runs) != 1 {
		t.Fatalf("first write failed: receipt=%+v err=%v", first, err)
	}
	second, err := writer.WriteRun(context.Background(), "run-source", digest(1), value)
	if err != nil || !second.Replayed || second.TargetID != first.TargetID || len(store.runs) != 1 {
		t.Fatalf("replay did not read back target: receipt=%+v err=%v", second, err)
	}

	changed := value
	changed.PrivateDigest = digest(9)
	firstDigest, err := HistoricalAudienceActivityRunDigest(withAudienceActivityRunID(value, first.TargetID))
	if err != nil {
		t.Fatal(err)
	}
	changedDigest, err := HistoricalAudienceActivityRunDigest(withAudienceActivityRunID(changed, first.TargetID))
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest == changedDigest {
		t.Fatal("private digest did not participate in target digest")
	}
}

func TestAudienceActivityWriterRejectsCrossPackageParents(t *testing.T) {
	store := newAudienceActivityStore()
	store.packages[2] = segmentport.AudienceActivityPackageReference{ID: 2}
	store.packages[8] = segmentport.AudienceActivityPackageReference{ID: 8}
	store.versions[3] = segmentport.AudienceActivityVersionReference{ID: 3, PackageHistoryID: 8}
	store.members[5] = segmentport.AudienceActivityMemberReference{ID: 5, PackageHistoryID: 8}
	store.runs[7] = activityRun(8, nil)
	store.runs[7] = withAudienceActivityRunID(store.runs[7], 7)
	writer := NewAudienceActivityHistoryWriter(store, &audienceActivityJournal{values: map[string]segmentport.AudienceActivityHistoryReceipt{}})
	if _, err := writer.WriteRun(context.Background(), "bad-version", digest(1), activityRun(2, activityID(3))); !errors.Is(err, segmentport.ErrAudienceActivityHistoryConflict) {
		t.Fatalf("cross-package version error=%v", err)
	}
	if _, err := writer.WriteMemberEvent(context.Background(), "bad-run", digest(2), activityEvent(2, activityID(7), nil)); !errors.Is(err, segmentport.ErrAudienceActivityHistoryConflict) {
		t.Fatalf("cross-package run error=%v", err)
	}
	if _, err := writer.WriteMemberEvent(context.Background(), "bad-member", digest(3), activityEvent(2, nil, activityID(5))); !errors.Is(err, segmentport.ErrAudienceActivityHistoryConflict) {
		t.Fatalf("cross-package member error=%v", err)
	}
	if len(store.events) != 0 || len(store.runs) != 1 {
		t.Fatalf("cross-package validation wrote facts: %+v", store)
	}
}

func TestAudienceActivityWriterPreservesNegativeCountsAndSourceTime(t *testing.T) {
	store := newAudienceActivityStore()
	store.packages[2] = segmentport.AudienceActivityPackageReference{ID: 2}
	writer := NewAudienceActivityHistoryWriter(store, &audienceActivityJournal{values: map[string]segmentport.AudienceActivityHistoryReceipt{}})
	value := activityRun(2, nil)
	value.ReturnedCount = -7
	value.DurationMS = -9
	stamp := time.Date(2026, 8, 29, 8, 30, 0, 123456789, time.FixedZone("source", 8*60*60))
	value.RefreshStartedAt, value.CreatedAt = stamp, stamp
	if _, err := writer.WriteRun(context.Background(), "negative", digest(4), value); err != nil {
		t.Fatal(err)
	}
	stored := store.runs[1]
	if stored.ReturnedCount != -7 || stored.DurationMS != -9 || stored.RefreshStartedAt.Nanosecond() != 123456000 || stored.RefreshStartedAt.Location() != time.UTC {
		t.Fatalf("historical fact changed: %+v", stored)
	}
}

func activityRun(packageID int64, versionID *int64) segmentport.HistoricalAudienceActivityRun {
	stamp := time.Date(2026, 8, 29, 8, 30, 0, 0, time.UTC)
	return segmentport.HistoricalAudienceActivityRun{SourceID: 10, PackageHistoryID: packageID, VersionHistoryID: versionID, RunType: "incremental", OriginalStatus: "succeeded", RefreshStartedAt: stamp, ReturnedCount: 1, EnteredCount: 2, UpdatedCount: 3, ExitedCount: 4, MemberEventCount: 5, DurationMS: 6, CreatedAt: stamp, PrivateDigest: digest(8)}
}

func activityEvent(packageID int64, runID, memberID *int64) segmentport.HistoricalAudienceActivityMemberEvent {
	stamp := time.Date(2026, 8, 29, 8, 30, 0, 0, time.UTC)
	return segmentport.HistoricalAudienceActivityMemberEvent{SourceID: 11, PackageHistoryID: packageID, RunHistoryID: runID, MemberHistoryID: memberID, EventType: "entered", IdentityKind: "unionid", OccurredAt: stamp, CreatedAt: stamp, PrivateDigest: digest(8)}
}

func digest(value byte) [32]byte    { var result [32]byte; result[0] = value; return result }
func activityID(value int64) *int64 { return &value }

type audienceActivityStore struct {
	runs     map[int64]segmentport.HistoricalAudienceActivityRun
	events   map[int64]segmentport.HistoricalAudienceActivityMemberEvent
	packages map[int64]segmentport.AudienceActivityPackageReference
	versions map[int64]segmentport.AudienceActivityVersionReference
	members  map[int64]segmentport.AudienceActivityMemberReference
}

func newAudienceActivityStore() *audienceActivityStore {
	return &audienceActivityStore{runs: map[int64]segmentport.HistoricalAudienceActivityRun{}, events: map[int64]segmentport.HistoricalAudienceActivityMemberEvent{}, packages: map[int64]segmentport.AudienceActivityPackageReference{}, versions: map[int64]segmentport.AudienceActivityVersionReference{}, members: map[int64]segmentport.AudienceActivityMemberReference{}}
}

func (s *audienceActivityStore) CreateHistoricalAudienceActivityRun(_ context.Context, value segmentport.HistoricalAudienceActivityRun) (segmentport.HistoricalAudienceActivityRun, error) {
	value.ID = int64(len(s.runs) + 1)
	s.runs[value.ID] = value
	return value, nil
}
func (s *audienceActivityStore) GetHistoricalAudienceActivityRun(_ context.Context, id int64) (segmentport.HistoricalAudienceActivityRun, error) {
	value, found := s.runs[id]
	if !found {
		return segmentport.HistoricalAudienceActivityRun{}, errors.New("not found")
	}
	return value, nil
}
func (s *audienceActivityStore) CreateHistoricalAudienceActivityMemberEvent(_ context.Context, value segmentport.HistoricalAudienceActivityMemberEvent) (segmentport.HistoricalAudienceActivityMemberEvent, error) {
	value.ID = int64(len(s.events) + 1)
	s.events[value.ID] = value
	return value, nil
}
func (s *audienceActivityStore) GetHistoricalAudienceActivityMemberEvent(_ context.Context, id int64) (segmentport.HistoricalAudienceActivityMemberEvent, error) {
	value, found := s.events[id]
	if !found {
		return segmentport.HistoricalAudienceActivityMemberEvent{}, errors.New("not found")
	}
	return value, nil
}
func (s *audienceActivityStore) GetHistoricalAudienceActivityPackage(_ context.Context, id int64) (segmentport.AudienceActivityPackageReference, error) {
	value, found := s.packages[id]
	if !found {
		return segmentport.AudienceActivityPackageReference{}, errors.New("not found")
	}
	return value, nil
}
func (s *audienceActivityStore) GetHistoricalAudienceActivityVersion(_ context.Context, id int64) (segmentport.AudienceActivityVersionReference, error) {
	value, found := s.versions[id]
	if !found {
		return segmentport.AudienceActivityVersionReference{}, errors.New("not found")
	}
	return value, nil
}
func (s *audienceActivityStore) GetHistoricalAudienceActivityMember(_ context.Context, id int64) (segmentport.AudienceActivityMemberReference, error) {
	value, found := s.members[id]
	if !found {
		return segmentport.AudienceActivityMemberReference{}, errors.New("not found")
	}
	return value, nil
}

type audienceActivityJournal struct {
	values map[string]segmentport.AudienceActivityHistoryReceipt
}

func (j *audienceActivityJournal) LoadAudienceActivityHistory(_ context.Context, kind, source string) (segmentport.AudienceActivityHistoryReceipt, bool, error) {
	value, found := j.values[kind+":"+source]
	return value, found, nil
}
func (j *audienceActivityJournal) RecordAudienceActivityHistory(_ context.Context, kind string, value segmentport.AudienceActivityHistoryReceipt) error {
	j.values[kind+":"+value.SourceIdentifier] = value
	return nil
}
