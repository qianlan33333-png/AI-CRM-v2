package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestReferenceHistoryDigestsBindPrivateFacts(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 123456000, time.UTC)
	binding := referenceBinding(at)
	binding.ID = 1
	before, err := HistoricalExternalContactBindingDigest(binding)
	if err != nil {
		t.Fatal(err)
	}
	binding.LastOwnerUserIDDigest = referenceDigest(90)
	after, _ := HistoricalExternalContactBindingDigest(binding)
	if before == after {
		t.Fatal("binding private digest omitted")
	}

	directory := referenceDirectory(at)
	directory.ID = 1
	before, err = HistoricalWeComDirectoryMemberDigest(directory)
	if err != nil {
		t.Fatal(err)
	}
	directory.RawPayloadDigest = referenceDigest(91)
	after, _ = HistoricalWeComDirectoryMemberDigest(directory)
	if before == after {
		t.Fatal("directory private digest omitted")
	}
	encoded, err := json.Marshal(directory)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "raw_payload_digest") || strings.Contains(string(encoded), "wecom_user_id_digest") {
		t.Fatalf("private source evidence leaked: %s", encoded)
	}
}

func TestReferenceHistoryWriterNormalizesReplaysAndClosesDrift(t *testing.T) {
	at := time.Date(2026, 8, 29, 9, 2, 3, 123456789, time.FixedZone("+8", 8*3600))
	store := &referenceHistoryFakeStore{}
	journal := &referenceHistoryFakeJournal{receipts: map[string]contact.ReferenceHistoryReceipt{}}
	writer, err := NewReferenceHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}

	binding := referenceBinding(at)
	source := hex.EncodeToString(binding.SourceKeyDigest[:])
	first, err := writer.ImportHistoricalExternalContactBinding(context.Background(), source, binding)
	if err != nil || first.Kind != externalContactBindingKind || first.Replayed || first.TargetID != 1 {
		t.Fatalf("binding first=%+v err=%v", first, err)
	}
	if store.binding.CreatedAt.Location() != time.UTC || store.binding.CreatedAt.Nanosecond() != 123456000 || !store.binding.UpdatedAt.Before(store.binding.CreatedAt) {
		t.Fatal("binding timestamps were not normalized without changing ordering")
	}
	if replay, err := writer.ImportHistoricalExternalContactBinding(context.Background(), source, binding); err != nil || !replay.Replayed {
		t.Fatalf("binding replay=%+v err=%v", replay, err)
	}
	store.binding.ExternalUserIDDigest = referenceDigest(92)
	if _, err := writer.ImportHistoricalExternalContactBinding(context.Background(), source, binding); !errors.Is(err, contact.ErrReferenceHistoryConflict) {
		t.Fatalf("stored binding private drift=%v", err)
	}
	store.binding.ExternalUserIDDigest = binding.ExternalUserIDDigest

	directory := referenceDirectory(at)
	if receipt, err := writer.ImportHistoricalWeComDirectoryMember(context.Background(), hex.EncodeToString(directory.SourceKeyDigest[:]), directory); err != nil || receipt.Kind != wecomDirectoryMemberKind {
		t.Fatalf("directory receipt=%+v err=%v", receipt, err)
	}
	if store.directory.WeComStatus != nil || store.directory.MatchedStaffID != nil || store.directory.SyncedAt.Location() != time.UTC || store.directory.SyncedAt.Nanosecond() != 123456000 {
		t.Fatal("directory nullable fields or timestamp changed")
	}
	directory.MobileDigest = referenceDigest(93)
	if _, err := writer.ImportHistoricalWeComDirectoryMember(context.Background(), hex.EncodeToString(directory.SourceKeyDigest[:]), directory); !errors.Is(err, contact.ErrReferenceHistoryConflict) {
		t.Fatalf("input directory private drift=%v", err)
	}
}

func TestReferenceHistoryWriterRejectsInvalidBeforeWrite(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	store := &referenceHistoryFakeStore{}
	journal := &referenceHistoryFakeJournal{receipts: map[string]contact.ReferenceHistoryReceipt{}}
	writer, err := NewReferenceHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	binding := referenceBinding(at)
	binding.ID = 7
	if _, err := writer.ImportHistoricalExternalContactBinding(context.Background(), hex.EncodeToString(binding.SourceKeyDigest[:]), binding); !errors.Is(err, contact.ErrReferenceHistoryInvalid) {
		t.Fatalf("target injection err=%v", err)
	}
	binding = referenceBinding(at)
	binding.IdentityAssurance = "verified"
	if _, err := writer.ImportHistoricalExternalContactBinding(context.Background(), hex.EncodeToString(binding.SourceKeyDigest[:]), binding); !errors.Is(err, contact.ErrReferenceHistoryInvalid) {
		t.Fatalf("unresolved assurance err=%v", err)
	}
	directory := referenceDirectory(at)
	directory.CorpAttribution, directory.MatchedStaffID = "unattributable", referenceInt64(1)
	if _, err := writer.ImportHistoricalWeComDirectoryMember(context.Background(), hex.EncodeToString(directory.SourceKeyDigest[:]), directory); !errors.Is(err, contact.ErrReferenceHistoryInvalid) {
		t.Fatalf("unattributable staff err=%v", err)
	}
	if store.creates != 0 || journal.records != 0 {
		t.Fatalf("invalid input wrote store=%d journal=%d", store.creates, journal.records)
	}
	var nilStore *referenceHistoryFakeStore
	if _, err := NewReferenceHistoryWriter(nilStore, journal); !errors.Is(err, contact.ErrReferenceHistoryUnavailable) {
		t.Fatalf("typed nil store=%v", err)
	}
}

func TestReferenceHistoryPreservesOptionalReferenceShapes(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	binding := referenceBinding(at)
	binding.ID = 1
	binding.PersonHistoryID, binding.IdentityID, binding.IdentityAssurance = referenceInt64(2), referenceInt64(3), "declared"
	if _, err := HistoricalExternalContactBindingDigest(binding); err != nil {
		t.Fatalf("declared references rejected: %v", err)
	}
	binding.IdentityID = nil
	if _, err := HistoricalExternalContactBindingDigest(binding); !errors.Is(err, contact.ErrReferenceHistoryInvalid) {
		t.Fatalf("declared null identity err=%v", err)
	}

	directory := referenceDirectory(at)
	directory.ID, directory.CorpAttribution = 1, "matched"
	if _, err := HistoricalWeComDirectoryMemberDigest(directory); err != nil {
		t.Fatalf("matched nil staff rejected: %v", err)
	}
	directory.MatchedStaffID = referenceInt64(4)
	if _, err := HistoricalWeComDirectoryMemberDigest(directory); err != nil {
		t.Fatalf("matched staff rejected: %v", err)
	}
}

type referenceHistoryFakeStore struct {
	binding   contact.HistoricalExternalContactBinding
	directory contact.HistoricalWeComDirectoryMember
	creates   int
}

func (s *referenceHistoryFakeStore) CreateHistoricalExternalContactBinding(_ context.Context, value contact.HistoricalExternalContactBinding) (contact.HistoricalExternalContactBinding, error) {
	s.creates++
	value.ID = 1
	s.binding = value
	return value, nil
}
func (s *referenceHistoryFakeStore) GetHistoricalExternalContactBinding(_ context.Context, id int64) (contact.HistoricalExternalContactBinding, error) {
	if id != s.binding.ID {
		return contact.HistoricalExternalContactBinding{}, contact.ErrReferenceHistoryUnavailable
	}
	return s.binding, nil
}
func (s *referenceHistoryFakeStore) CreateHistoricalWeComDirectoryMember(_ context.Context, value contact.HistoricalWeComDirectoryMember) (contact.HistoricalWeComDirectoryMember, error) {
	s.creates++
	value.ID = 2
	s.directory = value
	return value, nil
}
func (s *referenceHistoryFakeStore) GetHistoricalWeComDirectoryMember(_ context.Context, id int64) (contact.HistoricalWeComDirectoryMember, error) {
	if id != s.directory.ID {
		return contact.HistoricalWeComDirectoryMember{}, contact.ErrReferenceHistoryUnavailable
	}
	return s.directory, nil
}

type referenceHistoryFakeJournal struct {
	receipts map[string]contact.ReferenceHistoryReceipt
	records  int
}

func (j *referenceHistoryFakeJournal) LoadReferenceHistory(_ context.Context, kind, source string) (contact.ReferenceHistoryReceipt, bool, error) {
	value, ok := j.receipts[kind+":"+source]
	return value, ok, nil
}
func (j *referenceHistoryFakeJournal) RecordReferenceHistory(_ context.Context, value contact.ReferenceHistoryReceipt) error {
	j.records++
	j.receipts[value.Kind+":"+value.SourceIdentifier] = value
	return nil
}

func referenceDigest(value byte) [32]byte { return sha256.Sum256([]byte{value}) }
func referenceInt64(value int64) *int64   { return &value }

func referenceBinding(at time.Time) contact.HistoricalExternalContactBinding {
	return contact.HistoricalExternalContactBinding{SourceKeyDigest: referenceDigest(1), SourcePayloadDigest: referenceDigest(2), SourceFieldDigest: referenceDigest(3), ExternalUserIDDigest: referenceDigest(4), SourcePersonID: -8, IdentityAssurance: "unresolved", FirstBoundByUserIDDigest: referenceDigest(5), FirstOwnerUserIDDigest: referenceDigest(6), LastOwnerUserIDDigest: referenceDigest(7), CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}

func referenceDirectory(at time.Time) contact.HistoricalWeComDirectoryMember {
	return contact.HistoricalWeComDirectoryMember{SourceKeyDigest: referenceDigest(10), SourcePayloadDigest: referenceDigest(11), SourceFieldDigest: referenceDigest(12), SourceID: -9, WeComCorpIDDigest: referenceDigest(13), CorpIDDigest: referenceDigest(14), WeComUserIDDigest: referenceDigest(15), CorpAttribution: "unattributable", DisplayName: "", DepartmentIDsDigest: referenceDigest(16), DepartmentName: "", Position: "", IsActive: false, SyncedAt: at, RawPayloadDigest: referenceDigest(17), MobileDigest: referenceDigest(18), AvatarURLDigest: referenceDigest(19), UpdatedByDigest: referenceDigest(20), FirstSeenAt: at, LastSyncedAt: at.Add(-time.Second), CreatedAt: at, UpdatedAt: at.Add(-2 * time.Second)}
}
