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

func TestDeferredIdentityDigestsBindPrivateFacts(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 123456000, time.UTC)
	person := deferredPerson(at)
	person.ID = 1
	before, err := HistoricalDeferredPersonDigest(person)
	if err != nil {
		t.Fatal(err)
	}
	person.PrivateDigest = deferredDigest(91)
	after, _ := HistoricalDeferredPersonDigest(person)
	if before == after {
		t.Fatal("person private digest omitted")
	}

	conflict := deferredConflict(at)
	conflict.ID = 1
	before, err = HistoricalDeferredIdentityConflictDigest(conflict)
	if err != nil {
		t.Fatal(err)
	}
	conflict.ResolutionNoteDigest = deferredDigest(92)
	after, _ = HistoricalDeferredIdentityConflictDigest(conflict)
	if before == after {
		t.Fatal("conflict private digest omitted")
	}

	missing := missingRoot(at)
	missing.ID = 1
	before, err = HistoricalMissingRootIdentityDigest(missing)
	if err != nil {
		t.Fatal(err)
	}
	missing.DM01SourceKeyDigest = deferredDigest(93)
	after, _ = HistoricalMissingRootIdentityDigest(missing)
	if before == after {
		t.Fatal("DM01 HMAC omitted")
	}
	encoded, err := json.Marshal(missing)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "dm01_run_id") || strings.Contains(string(encoded), "private_digest") || strings.Contains(string(encoded), "redacted_roots") {
		t.Fatalf("private evidence leaked: %s", encoded)
	}
}

func TestDeferredIdentityWriterNormalizesReplaysAndClosesDrift(t *testing.T) {
	at := time.Date(2026, 8, 29, 9, 2, 3, 123456789, time.FixedZone("+8", 8*3600))
	store := &deferredIdentityFakeStore{}
	journal := &deferredIdentityFakeJournal{receipts: map[string]contact.DeferredIdentityHistoryReceipt{}}
	writer, err := NewDeferredIdentityHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}

	person := deferredPerson(at)
	source := hex.EncodeToString(person.SourceKeyDigest[:])
	first, err := writer.ImportHistoricalDeferredPerson(context.Background(), source, person)
	if err != nil || first.Kind != deferredPersonKind || first.Replayed || first.TargetID != 1 {
		t.Fatalf("person first=%+v err=%v", first, err)
	}
	if store.person.CreatedAt.Location() != time.UTC || store.person.CreatedAt.Nanosecond() != 123456000 || !store.person.UpdatedAt.Before(store.person.CreatedAt) {
		t.Fatal("person timestamp was not normalized without changing ordering")
	}
	if replay, err := writer.ImportHistoricalDeferredPerson(context.Background(), source, person); err != nil || !replay.Replayed {
		t.Fatalf("person replay=%+v err=%v", replay, err)
	}
	store.person.PrivateDigest = deferredDigest(94)
	if _, err := writer.ImportHistoricalDeferredPerson(context.Background(), source, person); !errors.Is(err, contact.ErrDeferredIdentityHistoryConflict) {
		t.Fatalf("stored person private drift=%v", err)
	}
	store.person.PrivateDigest = person.PrivateDigest
	person.MobileDigest = deferredDigest(94)
	if _, err := writer.ImportHistoricalDeferredPerson(context.Background(), source, person); !errors.Is(err, contact.ErrDeferredIdentityHistoryConflict) {
		t.Fatalf("input person private drift=%v", err)
	}

	conflict := deferredConflict(at)
	if _, err := writer.ImportHistoricalDeferredIdentityConflict(context.Background(), hex.EncodeToString(conflict.SourceKeyDigest[:]), conflict); err != nil {
		t.Fatalf("conflict import=%v", err)
	}
	if store.conflict.ResolvedAt == nil || store.conflict.ResolvedAt.Location() != time.UTC || store.conflict.ResolvedAt.Nanosecond() != 123456000 {
		t.Fatal("conflict nullable timestamp was not preserved")
	}

	missing := missingRoot(at)
	if receipt, err := writer.ImportHistoricalMissingRootIdentity(context.Background(), hex.EncodeToString(missing.SourceKeyDigest[:]), missing); err != nil || receipt.Kind != missingRootKind {
		t.Fatalf("missing root receipt=%+v err=%v", receipt, err)
	}
	if store.missing.Type != nil || store.missing.GenderDigest != nil || len(store.missing.RedactedRoots) != 2 {
		t.Fatal("missing-root nullable fields or roots changed")
	}
}

func TestDeferredIdentityWriterRejectsInvalidBeforeWrite(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	store := &deferredIdentityFakeStore{}
	journal := &deferredIdentityFakeJournal{receipts: map[string]contact.DeferredIdentityHistoryReceipt{}}
	writer, err := NewDeferredIdentityHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	person := deferredPerson(at)
	person.ID = 8
	if _, err := writer.ImportHistoricalDeferredPerson(context.Background(), hex.EncodeToString(person.SourceKeyDigest[:]), person); !errors.Is(err, contact.ErrDeferredIdentityHistoryInvalid) {
		t.Fatalf("target injection err=%v", err)
	}
	missing := missingRoot(at)
	missing.QuarantineReason = "other"
	if _, err := writer.ImportHistoricalMissingRootIdentity(context.Background(), hex.EncodeToString(missing.SourceKeyDigest[:]), missing); !errors.Is(err, contact.ErrDeferredIdentityHistoryInvalid) {
		t.Fatalf("missing-root reason err=%v", err)
	}
	missing = missingRoot(at)
	missing.DM01RunID = 0
	if _, err := writer.ImportHistoricalMissingRootIdentity(context.Background(), hex.EncodeToString(missing.SourceKeyDigest[:]), missing); !errors.Is(err, contact.ErrDeferredIdentityHistoryInvalid) {
		t.Fatalf("missing-root run err=%v", err)
	}
	if store.creates != 0 || journal.records != 0 {
		t.Fatalf("invalid input wrote store=%d journal=%d", store.creates, journal.records)
	}
	var nilStore *deferredIdentityFakeStore
	if _, err := NewDeferredIdentityHistoryWriter(nilStore, journal); !errors.Is(err, contact.ErrDeferredIdentityHistoryUnavailable) {
		t.Fatalf("typed nil store=%v", err)
	}
}

type deferredIdentityFakeStore struct {
	person   contact.HistoricalDeferredPerson
	conflict contact.HistoricalDeferredIdentityConflict
	missing  contact.HistoricalMissingRootIdentity
	creates  int
}

func (s *deferredIdentityFakeStore) CreateHistoricalDeferredPerson(_ context.Context, value contact.HistoricalDeferredPerson) (contact.HistoricalDeferredPerson, error) {
	s.creates++
	value.ID = 1
	s.person = value
	return value, nil
}
func (s *deferredIdentityFakeStore) GetHistoricalDeferredPerson(_ context.Context, id int64) (contact.HistoricalDeferredPerson, error) {
	if id != s.person.ID {
		return contact.HistoricalDeferredPerson{}, contact.ErrDeferredIdentityHistoryUnavailable
	}
	return s.person, nil
}
func (s *deferredIdentityFakeStore) CreateHistoricalDeferredIdentityConflict(_ context.Context, value contact.HistoricalDeferredIdentityConflict) (contact.HistoricalDeferredIdentityConflict, error) {
	s.creates++
	value.ID = 2
	s.conflict = value
	return value, nil
}
func (s *deferredIdentityFakeStore) GetHistoricalDeferredIdentityConflict(_ context.Context, id int64) (contact.HistoricalDeferredIdentityConflict, error) {
	if id != s.conflict.ID {
		return contact.HistoricalDeferredIdentityConflict{}, contact.ErrDeferredIdentityHistoryUnavailable
	}
	return s.conflict, nil
}
func (s *deferredIdentityFakeStore) CreateHistoricalMissingRootIdentity(_ context.Context, value contact.HistoricalMissingRootIdentity) (contact.HistoricalMissingRootIdentity, error) {
	s.creates++
	value.ID = 3
	s.missing = value
	return value, nil
}
func (s *deferredIdentityFakeStore) GetHistoricalMissingRootIdentity(_ context.Context, id int64) (contact.HistoricalMissingRootIdentity, error) {
	if id != s.missing.ID {
		return contact.HistoricalMissingRootIdentity{}, contact.ErrDeferredIdentityHistoryUnavailable
	}
	return s.missing, nil
}

type deferredIdentityFakeJournal struct {
	receipts map[string]contact.DeferredIdentityHistoryReceipt
	records  int
}

func (j *deferredIdentityFakeJournal) LoadDeferredIdentityHistory(_ context.Context, kind, source string) (contact.DeferredIdentityHistoryReceipt, bool, error) {
	value, ok := j.receipts[kind+":"+source]
	return value, ok, nil
}
func (j *deferredIdentityFakeJournal) RecordDeferredIdentityHistory(_ context.Context, value contact.DeferredIdentityHistoryReceipt) error {
	j.records++
	j.receipts[value.Kind+":"+value.SourceIdentifier] = value
	return nil
}

func deferredDigest(value byte) [32]byte { return sha256.Sum256([]byte{value}) }

func deferredPerson(at time.Time) contact.HistoricalDeferredPerson {
	return contact.HistoricalDeferredPerson{SourceID: 0, SourceKeyDigest: deferredDigest(1), SourcePayloadDigest: deferredDigest(2), SourceFieldDigest: deferredDigest(3), MobileDigest: deferredDigest(4), ThirdPartyUserIDDigest: deferredDigest(5), PrivateDigest: deferredDigest(6), RedactedRoots: []string{"mobile", "third_party_user_id"}, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}

func deferredConflict(at time.Time) contact.HistoricalDeferredIdentityConflict {
	resolved := at.Add(-2 * time.Second)
	return contact.HistoricalDeferredIdentityConflict{SourceID: -4, SourceKeyDigest: deferredDigest(10), SourcePayloadDigest: deferredDigest(11), SourceFieldDigest: deferredDigest(12), ConflictType: "", SourceType: "", Status: "", ResolutionStatus: "", UnionIDDigest: deferredDigest(13), CandidateUnionIDDigest: deferredDigest(14), ExternalUserIDDigest: deferredDigest(15), OpenIDDigest: deferredDigest(16), MobileDigest: deferredDigest(17), LegacySourceKeyDigest: deferredDigest(18), PayloadJSONDigest: deferredDigest(19), SourcePayloadJSONDigest: deferredDigest(20), ResolutionNoteDigest: deferredDigest(21), PrivateDigest: deferredDigest(22), RedactedRoots: []string{"payload_json"}, CreatedAt: at, UpdatedAt: at.Add(-time.Second), ResolvedAt: &resolved}
}

func missingRoot(at time.Time) contact.HistoricalMissingRootIdentity {
	return contact.HistoricalMissingRootIdentity{SourceID: -9, SourceKeyDigest: deferredDigest(30), SourcePayloadDigest: deferredDigest(31), SourceFieldDigest: deferredDigest(32), DM01RunID: 2, DM01SourceKeyDigest: deferredDigest(33), DM01SourceHMACKeyVersion: "v1-domain-a1", QuarantineReason: "missing_customer_root", Type: nil, Status: "", CorpIDDigest: deferredDigest(34), ExternalUserIDDigest: deferredDigest(35), UnionIDDigest: deferredDigest(36), OpenIDDigest: deferredDigest(37), FollowUserIDDigest: deferredDigest(38), NameDigest: deferredDigest(39), AvatarDigest: deferredDigest(40), GenderDigest: nil, RawProfileDigest: deferredDigest(41), PrivateDigest: deferredDigest(42), RedactedRoots: []string{"raw_profile", "avatar"}, FirstSeenAt: at, LastSeenAt: at.Add(-time.Second), CreatedAt: at, UpdatedAt: at.Add(-2 * time.Second)}
}
