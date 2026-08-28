package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

var signupTagHistoryTestTime = time.Date(2026, 8, 28, 10, 11, 12, 123456789, time.FixedZone("CST", 8*60*60))

func TestSignupTagHistoryImportsImmutableRuleAndReplays(t *testing.T) {
	store, journal := newSignupTagHistoryStore(), newSignupTagHistoryJournal()
	service := NewSignupTagHistoryService(store, journal)
	fact := signupTagHistoryFact()
	source := hex.EncodeToString(fact.SourceKeyDigest[:])
	first, err := service.ImportRule(context.Background(), source, fact.SourcePayloadDigest, fact)
	if err != nil || first.Replayed || first.TargetID != 1 || first.TargetDigest == ([32]byte{}) || store.creates != 1 || journal.records != 1 {
		t.Fatalf("first=%+v creates=%d records=%d err=%v", first, store.creates, journal.records, err)
	}
	stored := store.records[first.TargetID]
	if stored.TagSourceID != "v1-tag-7" || stored.TagName != " \n报名标签\t " || stored.SignupStatus != "approved" || stored.OriginalActive || !stored.UpdatedAt.Equal(signupTagHistoryTestTime.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("stored=%+v", stored)
	}
	second, err := service.ImportRule(context.Background(), source, fact.SourcePayloadDigest, fact)
	if err != nil || !second.Replayed || second.TargetID != first.TargetID || second.TargetDigest != first.TargetDigest || store.creates != 1 || store.gets != 1 || journal.records != 1 {
		t.Fatalf("second=%+v creates=%d gets=%d records=%d err=%v", second, store.creates, store.gets, journal.records, err)
	}
}

func TestSignupTagHistoryReplayRejectsTargetAndReceiptDrift(t *testing.T) {
	store, journal := newSignupTagHistoryStore(), newSignupTagHistoryJournal()
	service := NewSignupTagHistoryService(store, journal)
	fact := signupTagHistoryFact()
	source := hex.EncodeToString(fact.SourceKeyDigest[:])
	receipt, err := service.ImportRule(context.Background(), source, fact.SourcePayloadDigest, fact)
	if err != nil {
		t.Fatal(err)
	}
	changed := store.records[receipt.TargetID]
	changed.SignupStatus = "drift"
	store.records[receipt.TargetID] = changed
	if _, err = service.ImportRule(context.Background(), source, fact.SourcePayloadDigest, fact); !errors.Is(err, contactport.ErrSignupTagHistoryConflict) {
		t.Fatalf("target drift error=%v", err)
	}
	store.records[receipt.TargetID] = withHistoricalSignupTagRuleID(fact, receipt.TargetID)
	journal.receipts[source] = contactport.SignupTagHistoryReceipt{SourceIdentifier: source, PayloadDigest: sha256.Sum256([]byte("changed")), TargetID: receipt.TargetID, TargetDigest: receipt.TargetDigest}
	if _, err = service.ImportRule(context.Background(), source, fact.SourcePayloadDigest, fact); !errors.Is(err, contactport.ErrSignupTagHistoryConflict) {
		t.Fatalf("receipt drift error=%v", err)
	}
}

func TestSignupTagHistoryRejectsInvalidInputsAndDependencies(t *testing.T) {
	fact := signupTagHistoryFact()
	source := hex.EncodeToString(fact.SourceKeyDigest[:])
	for _, invalid := range []struct {
		name    string
		source  string
		payload [32]byte
		fact    contactport.HistoricalSignupTagRule
	}{
		{name: "upper source", source: stringsToUpper(source), payload: fact.SourcePayloadDigest, fact: fact},
		{name: "payload mismatch", source: source, payload: sha256.Sum256([]byte("other")), fact: fact},
		{name: "zero source", source: source, payload: fact.SourcePayloadDigest, fact: contactport.HistoricalSignupTagRule{}},
		{name: "nul tag", source: source, payload: fact.SourcePayloadDigest, fact: withSignupTagHistoryTagName(fact, "bad\x00")},
		{name: "zero time", source: source, payload: fact.SourcePayloadDigest, fact: withSignupTagHistoryTime(fact, time.Time{})},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			if _, err := NewSignupTagHistoryService(newSignupTagHistoryStore(), newSignupTagHistoryJournal()).ImportRule(context.Background(), invalid.source, invalid.payload, invalid.fact); !errors.Is(err, contactport.ErrSignupTagHistoryInvalid) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	if _, err := NewSignupTagHistoryService(nil, newSignupTagHistoryJournal()).ImportRule(context.Background(), source, fact.SourcePayloadDigest, fact); !errors.Is(err, contactport.ErrSignupTagHistoryUnavailable) {
		t.Fatalf("nil store error=%v", err)
	}
	var nilStore *signupTagHistoryStore
	if _, err := NewSignupTagHistoryService(nilStore, newSignupTagHistoryJournal()).ImportRule(context.Background(), source, fact.SourcePayloadDigest, fact); !errors.Is(err, contactport.ErrSignupTagHistoryUnavailable) {
		t.Fatalf("typed nil store error=%v", err)
	}
	if _, err := NewSignupTagHistoryService(newSignupTagHistoryStore(), nil).ImportRule(context.Background(), source, fact.SourcePayloadDigest, fact); !errors.Is(err, contactport.ErrSignupTagHistoryUnavailable) {
		t.Fatalf("nil journal error=%v", err)
	}
}

func TestHistoricalSignupTagRuleDigestCoversStoredFields(t *testing.T) {
	fact := withHistoricalSignupTagRuleID(signupTagHistoryFact(), 9)
	baseline, err := HistoricalSignupTagRuleDigest(fact)
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range []contactport.HistoricalSignupTagRule{
		withSignupTagHistoryTagName(fact, "other"),
		withSignupTagHistoryStatus(fact, "other"),
		withSignupTagHistoryActive(fact, !fact.OriginalActive),
		withSignupTagHistoryTime(fact, fact.UpdatedAt.Add(time.Microsecond)),
	} {
		digest, digestErr := HistoricalSignupTagRuleDigest(changed)
		if digestErr != nil || digest == baseline {
			t.Fatalf("digest=%x baseline=%x err=%v", digest, baseline, digestErr)
		}
	}
}

func signupTagHistoryFact() contactport.HistoricalSignupTagRule {
	key, payload := sha256.Sum256([]byte("signup-tag-source")), sha256.Sum256([]byte("signup-tag-payload"))
	return contactport.HistoricalSignupTagRule{SourceKeyDigest: key, SourcePayloadDigest: payload, TagSourceID: "v1-tag-7", TagName: " \n报名标签\t ", SignupStatus: "approved", OriginalActive: false, UpdatedAt: signupTagHistoryTestTime}
}

func withSignupTagHistoryTagName(value contactport.HistoricalSignupTagRule, tagName string) contactport.HistoricalSignupTagRule {
	value.TagName = tagName
	return value
}
func withSignupTagHistoryStatus(value contactport.HistoricalSignupTagRule, status string) contactport.HistoricalSignupTagRule {
	value.SignupStatus = status
	return value
}
func withSignupTagHistoryActive(value contactport.HistoricalSignupTagRule, active bool) contactport.HistoricalSignupTagRule {
	value.OriginalActive = active
	return value
}
func withSignupTagHistoryTime(value contactport.HistoricalSignupTagRule, updatedAt time.Time) contactport.HistoricalSignupTagRule {
	value.UpdatedAt = updatedAt
	return value
}
func stringsToUpper(value string) string {
	result := []byte(value)
	for index, character := range result {
		if character >= 'a' && character <= 'f' {
			result[index] = character - ('a' - 'A')
		}
	}
	return string(result)
}

type signupTagHistoryStore struct {
	records               map[int64]contactport.HistoricalSignupTagRule
	nextID, creates, gets int64
	createErr, getErr     error
}

func newSignupTagHistoryStore() *signupTagHistoryStore {
	return &signupTagHistoryStore{records: map[int64]contactport.HistoricalSignupTagRule{}, nextID: 1}
}
func (store *signupTagHistoryStore) CreateHistoricalSignupTagRule(_ context.Context, value contactport.HistoricalSignupTagRule) (contactport.HistoricalSignupTagRule, error) {
	if store.createErr != nil {
		return contactport.HistoricalSignupTagRule{}, store.createErr
	}
	value.ID, store.nextID, store.creates = store.nextID, store.nextID+1, store.creates+1
	store.records[value.ID] = value
	return value, nil
}
func (store *signupTagHistoryStore) GetHistoricalSignupTagRule(_ context.Context, id int64) (contactport.HistoricalSignupTagRule, error) {
	store.gets++
	if store.getErr != nil {
		return contactport.HistoricalSignupTagRule{}, store.getErr
	}
	value, found := store.records[id]
	if !found {
		return contactport.HistoricalSignupTagRule{}, errors.New("missing history")
	}
	return value, nil
}

type signupTagHistoryJournal struct {
	receipts           map[string]contactport.SignupTagHistoryReceipt
	loadErr, recordErr error
	records            int64
}

func newSignupTagHistoryJournal() *signupTagHistoryJournal {
	return &signupTagHistoryJournal{receipts: map[string]contactport.SignupTagHistoryReceipt{}}
}
func (journal *signupTagHistoryJournal) LoadSignupTagHistory(_ context.Context, source string) (contactport.SignupTagHistoryReceipt, bool, error) {
	if journal.loadErr != nil {
		return contactport.SignupTagHistoryReceipt{}, false, journal.loadErr
	}
	receipt, found := journal.receipts[source]
	return receipt, found, nil
}
func (journal *signupTagHistoryJournal) RecordSignupTagHistory(_ context.Context, receipt contactport.SignupTagHistoryReceipt) error {
	if journal.recordErr != nil {
		return journal.recordErr
	}
	if _, found := journal.receipts[receipt.SourceIdentifier]; found {
		return contactport.ErrSignupTagHistoryConflict
	}
	journal.receipts[receipt.SourceIdentifier], journal.records = receipt, journal.records+1
	return nil
}
