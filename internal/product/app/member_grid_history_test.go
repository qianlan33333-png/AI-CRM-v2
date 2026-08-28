package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestMemberGridHistoryWriterCreatesAndReplaysVerifiedTargets(t *testing.T) {
	store := &fakeMemberGridHistoryStore{}
	journal := &fakeMemberGridHistoryJournal{}
	writer := NewMemberGridHistoryWriter(store, journal)
	key, payload := memberGridHistoryTestDigests()
	source := hex.EncodeToString(key[:])

	viewReceipt, err := writer.WriteMemberView(context.Background(), source, payload, memberGridHistoryTestView(key, payload))
	if err != nil || viewReceipt.Replayed || viewReceipt.TargetID != 1 || store.viewCreates != 1 || journal.records != 1 {
		t.Fatalf("view first receipt=%#v err=%v creates=%d records=%d", viewReceipt, err, store.viewCreates, journal.records)
	}
	usageReceipt, err := writer.WriteMemberUsage(context.Background(), source, payload, memberGridHistoryTestUsage(key, payload))
	if err != nil || usageReceipt.Replayed || usageReceipt.TargetID != 1 || store.usageCreates != 1 || journal.records != 2 {
		t.Fatalf("usage first receipt=%#v err=%v creates=%d records=%d", usageReceipt, err, store.usageCreates, journal.records)
	}
	if stored := store.views[viewReceipt.TargetID]; stored.Position != -2 || stored.SchemaVersion != -3 || stored.Version != -4 || stored.CreatedAt.Location() != time.UTC || stored.CreatedAt.Nanosecond()%int(time.Microsecond) != 0 {
		t.Fatalf("view was not preserved and normalized: %#v", stored)
	}
	if stored := store.usageRows[usageReceipt.TargetID]; stored.OpenCount7D != 0 || stored.LastOpenAt == nil || stored.LastOpenAt.Location() != time.UTC || stored.RefreshedAt.Nanosecond()%int(time.Microsecond) != 0 {
		t.Fatalf("usage was not preserved and normalized: %#v", stored)
	}

	viewReplay, err := writer.WriteMemberView(context.Background(), source, payload, memberGridHistoryTestView(key, payload))
	if err != nil || !viewReplay.Replayed || viewReplay.TargetDigest != viewReceipt.TargetDigest || store.viewCreates != 1 {
		t.Fatalf("view replay=%#v err=%v creates=%d", viewReplay, err, store.viewCreates)
	}
	usageReplay, err := writer.WriteMemberUsage(context.Background(), source, payload, memberGridHistoryTestUsage(key, payload))
	if err != nil || !usageReplay.Replayed || usageReplay.TargetDigest != usageReceipt.TargetDigest || store.usageCreates != 1 {
		t.Fatalf("usage replay=%#v err=%v creates=%d", usageReplay, err, store.usageCreates)
	}
}

func TestMemberGridHistoryWriterRejectsReceiptAndActualTargetDrift(t *testing.T) {
	store := &fakeMemberGridHistoryStore{}
	journal := &fakeMemberGridHistoryJournal{}
	writer := NewMemberGridHistoryWriter(store, journal)
	key, payload := memberGridHistoryTestDigests()
	source := hex.EncodeToString(key[:])
	if _, err := writer.WriteMemberView(context.Background(), source, payload, memberGridHistoryTestView(key, payload)); err != nil {
		t.Fatal(err)
	}
	journal.receipts[memberGridHistoryJournalKey(productport.MemberGridHistoryView, source)] = productport.MemberGridHistoryReceipt{
		Kind: productport.MemberGridHistoryView, SourceIdentifier: source, PayloadDigest: payload, TargetID: 1, TargetDigest: [sha256.Size]byte{9},
	}
	if _, err := writer.WriteMemberView(context.Background(), source, payload, memberGridHistoryTestView(key, payload)); !errors.Is(err, productport.ErrMemberGridHistoryConflict) {
		t.Fatalf("receipt drift err=%v", err)
	}

	journal = &fakeMemberGridHistoryJournal{}
	store = &fakeMemberGridHistoryStore{}
	writer = NewMemberGridHistoryWriter(store, journal)
	if _, err := writer.WriteMemberUsage(context.Background(), source, payload, memberGridHistoryTestUsage(key, payload)); err != nil {
		t.Fatal(err)
	}
	stored := store.usageRows[1]
	stored.OpenCount7D++
	store.usageRows[1] = stored
	if _, err := writer.WriteMemberUsage(context.Background(), source, payload, memberGridHistoryTestUsage(key, payload)); !errors.Is(err, productport.ErrMemberGridHistoryConflict) {
		t.Fatalf("actual target drift err=%v", err)
	}
}

func TestMemberGridHistoryWriterFailsClosedForInvalidOrUnavailableInput(t *testing.T) {
	store := &fakeMemberGridHistoryStore{}
	journal := &fakeMemberGridHistoryJournal{}
	writer := NewMemberGridHistoryWriter(store, journal)
	key, payload := memberGridHistoryTestDigests()
	source := hex.EncodeToString(key[:])

	invalidView := memberGridHistoryTestView(key, payload)
	invalidView.Name = "bad\x00text"
	viewWithReversedTimes := memberGridHistoryTestView(key, payload)
	viewWithReversedTimes.UpdatedAt = viewWithReversedTimes.CreatedAt.Add(-time.Microsecond)
	invalidUsage := memberGridHistoryTestUsage(key, payload)
	invalidUsage.RecoveryEntryDigest = [sha256.Size]byte{}
	for _, call := range []func() error{
		func() error {
			_, err := writer.WriteMemberView(context.Background(), stringsToUpper(source), payload, memberGridHistoryTestView(key, payload))
			return err
		},
		func() error {
			_, err := writer.WriteMemberView(context.Background(), source, [sha256.Size]byte{}, memberGridHistoryTestView(key, payload))
			return err
		},
		func() error {
			_, err := writer.WriteMemberView(context.Background(), source, payload, invalidView)
			return err
		},
		func() error {
			_, err := writer.WriteMemberView(context.Background(), source, payload, viewWithReversedTimes)
			return err
		},
		func() error {
			_, err := writer.WriteMemberUsage(context.Background(), source, payload, invalidUsage)
			return err
		},
		func() error {
			value := memberGridHistoryTestUsage(key, payload)
			zero := int64(0)
			value.CustomerID = &zero
			_, err := writer.WriteMemberUsage(context.Background(), source, payload, value)
			return err
		},
		func() error {
			value := memberGridHistoryTestUsage(key, payload)
			negative := int64(-1)
			value.LearningPlanCurrent = &negative
			_, err := writer.WriteMemberUsage(context.Background(), source, payload, value)
			return err
		},
	} {
		if err := call(); !errors.Is(err, productport.ErrMemberGridHistoryInvalid) {
			t.Fatalf("invalid err=%v", err)
		}
	}
	if store.viewCreates != 0 || store.usageCreates != 0 || journal.loads != 0 || journal.records != 0 {
		t.Fatalf("invalid input reached dependencies: %#v %#v", store, journal)
	}

	var typedNilStore *fakeMemberGridHistoryStore
	if _, err := NewMemberGridHistoryWriter(typedNilStore, journal).WriteMemberView(context.Background(), source, payload, memberGridHistoryTestView(key, payload)); !errors.Is(err, productport.ErrMemberGridHistoryUnavailable) {
		t.Fatalf("typed nil err=%v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := writer.WriteMemberUsage(canceled, source, payload, memberGridHistoryTestUsage(key, payload)); !errors.Is(err, productport.ErrMemberGridHistoryUnavailable) {
		t.Fatalf("canceled err=%v", err)
	}
}

func TestHistoricalMemberGridDigestsCoverEveryTypedField(t *testing.T) {
	key, payload := memberGridHistoryTestDigests()
	view := memberGridHistoryTestView(key, payload)
	view.ID = 1
	view = normalizeHistoricalMemberView(view)
	usage := memberGridHistoryTestUsage(key, payload)
	usage.ID = 1
	usage = normalizeHistoricalMemberUsage(usage)

	viewDigest, err := HistoricalMemberViewDigest(view)
	if err != nil {
		t.Fatal(err)
	}
	usageDigest, err := HistoricalMemberUsageDigest(usage)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*productport.HistoricalMemberView){
		func(value *productport.HistoricalMemberView) { value.SourceViewID++ },
		func(value *productport.HistoricalMemberView) { value.SourceServiceProductID++ },
		func(value *productport.HistoricalMemberView) { *value.ProductID++ },
		func(value *productport.HistoricalMemberView) { value.Name += "x" },
		func(value *productport.HistoricalMemberView) { value.Position++ },
		func(value *productport.HistoricalMemberView) { value.IsDefault = !value.IsDefault },
		func(value *productport.HistoricalMemberView) { value.SchemaVersion++ },
		func(value *productport.HistoricalMemberView) { value.ConfigDigest[0]++ },
		func(value *productport.HistoricalMemberView) { value.Version++ },
		func(value *productport.HistoricalMemberView) { value.CreatedAt = value.CreatedAt.Add(time.Microsecond) },
		func(value *productport.HistoricalMemberView) { value.UpdatedAt = value.UpdatedAt.Add(time.Microsecond) },
		func(value *productport.HistoricalMemberView) { value.SourceKeyDigest[0]++ },
		func(value *productport.HistoricalMemberView) { value.SourcePayloadDigest[0]++ },
	} {
		changed := view
		changed.ProductID = copyMemberGridInt64(view.ProductID)
		mutate(&changed)
		digest, digestErr := HistoricalMemberViewDigest(changed)
		if digestErr != nil || digest == viewDigest {
			t.Fatalf("view field not represented digest=%x err=%v", digest, digestErr)
		}
	}
	for _, mutate := range []func(*productport.HistoricalMemberUsage){
		func(value *productport.HistoricalMemberUsage) { *value.CustomerID++ },
		func(value *productport.HistoricalMemberUsage) { value.FormallyLoggedIn = !value.FormallyLoggedIn },
		func(value *productport.HistoricalMemberUsage) { value.HasTokenUsage = !value.HasTokenUsage },
		func(value *productport.HistoricalMemberUsage) { value.LearningPlanID += "x" },
		func(value *productport.HistoricalMemberUsage) { *value.LearningPlanCurrent++ },
		func(value *productport.HistoricalMemberUsage) { *value.LearningPlanTotal++ },
		func(value *productport.HistoricalMemberUsage) { value.OpenCount7D++ },
		func(value *productport.HistoricalMemberUsage) {
			*value.LastOpenAt = value.LastOpenAt.Add(time.Microsecond)
		},
		func(value *productport.HistoricalMemberUsage) {
			value.RefreshedAt = value.RefreshedAt.Add(time.Microsecond)
		},
		func(value *productport.HistoricalMemberUsage) { value.SourceKeyDigest[0]++ },
		func(value *productport.HistoricalMemberUsage) { value.SourcePayloadDigest[0]++ },
		func(value *productport.HistoricalMemberUsage) { value.RecoveryEntryDigest[0]++ },
	} {
		changed := usage
		changed.CustomerID = copyMemberGridInt64(usage.CustomerID)
		changed.LearningPlanCurrent = copyMemberGridInt64(usage.LearningPlanCurrent)
		changed.LearningPlanTotal = copyMemberGridInt64(usage.LearningPlanTotal)
		changed.LastOpenAt = copyMemberGridTime(usage.LastOpenAt)
		mutate(&changed)
		digest, digestErr := HistoricalMemberUsageDigest(changed)
		if digestErr != nil || digest == usageDigest {
			t.Fatalf("usage field not represented digest=%x err=%v", digest, digestErr)
		}
	}
}

type fakeMemberGridHistoryStore struct {
	views                     map[int64]productport.HistoricalMemberView
	usageRows                 map[int64]productport.HistoricalMemberUsage
	viewCreates, usageCreates int
	err                       error
}

func (store *fakeMemberGridHistoryStore) CreateHistoricalMemberView(ctx context.Context, value productport.HistoricalMemberView) (productport.HistoricalMemberView, error) {
	if ctx == nil || ctx.Err() != nil || store.err != nil {
		return productport.HistoricalMemberView{}, store.err
	}
	if store.views == nil {
		store.views = map[int64]productport.HistoricalMemberView{}
	}
	store.viewCreates++
	value.ID = int64(store.viewCreates)
	store.views[value.ID] = value
	return value, nil
}

func (store *fakeMemberGridHistoryStore) GetHistoricalMemberView(ctx context.Context, id int64) (productport.HistoricalMemberView, error) {
	if ctx == nil || ctx.Err() != nil || store.err != nil {
		return productport.HistoricalMemberView{}, store.err
	}
	return store.views[id], nil
}

func (store *fakeMemberGridHistoryStore) CreateHistoricalMemberUsage(ctx context.Context, value productport.HistoricalMemberUsage) (productport.HistoricalMemberUsage, error) {
	if ctx == nil || ctx.Err() != nil || store.err != nil {
		return productport.HistoricalMemberUsage{}, store.err
	}
	if store.usageRows == nil {
		store.usageRows = map[int64]productport.HistoricalMemberUsage{}
	}
	store.usageCreates++
	value.ID = int64(store.usageCreates)
	store.usageRows[value.ID] = value
	return value, nil
}

func (store *fakeMemberGridHistoryStore) GetHistoricalMemberUsage(ctx context.Context, id int64) (productport.HistoricalMemberUsage, error) {
	if ctx == nil || ctx.Err() != nil || store.err != nil {
		return productport.HistoricalMemberUsage{}, store.err
	}
	return store.usageRows[id], nil
}

type fakeMemberGridHistoryJournal struct {
	receipts       map[string]productport.MemberGridHistoryReceipt
	loads, records int
	err            error
}

func (journal *fakeMemberGridHistoryJournal) LoadMemberGridHistory(ctx context.Context, kind, source string) (productport.MemberGridHistoryReceipt, bool, error) {
	if ctx == nil || ctx.Err() != nil || journal.err != nil {
		return productport.MemberGridHistoryReceipt{}, false, journal.err
	}
	journal.loads++
	if journal.receipts == nil {
		return productport.MemberGridHistoryReceipt{}, false, nil
	}
	receipt, found := journal.receipts[memberGridHistoryJournalKey(kind, source)]
	return receipt, found, nil
}

func (journal *fakeMemberGridHistoryJournal) RecordMemberGridHistory(ctx context.Context, receipt productport.MemberGridHistoryReceipt) error {
	if ctx == nil || ctx.Err() != nil || journal.err != nil {
		return journal.err
	}
	if journal.receipts == nil {
		journal.receipts = map[string]productport.MemberGridHistoryReceipt{}
	}
	journal.records++
	journal.receipts[memberGridHistoryJournalKey(receipt.Kind, receipt.SourceIdentifier)] = receipt
	return nil
}

func memberGridHistoryJournalKey(kind, source string) string { return kind + "\x00" + source }

func memberGridHistoryTestDigests() ([sha256.Size]byte, [sha256.Size]byte) {
	return sha256.Sum256([]byte("source-key")), sha256.Sum256([]byte("payload"))
}

func memberGridHistoryTestView(key, payload [sha256.Size]byte) productport.HistoricalMemberView {
	productID := int64(101)
	return productport.HistoricalMemberView{
		SourceKeyDigest: key, SourceViewID: 11, SourceServiceProductID: 13, ProductID: &productID, Name: "  old view\n",
		Position: -2, IsDefault: true, SchemaVersion: -3, ConfigDigest: sha256.Sum256([]byte("config")), Version: -4,
		CreatedAt: time.Date(2026, 8, 28, 12, 1, 2, 123456789, time.FixedZone("legacy", 8*3600)),
		UpdatedAt: time.Date(2026, 8, 28, 12, 1, 3, 987654321, time.FixedZone("legacy", 8*3600)), SourcePayloadDigest: payload,
	}
}

func memberGridHistoryTestUsage(key, payload [sha256.Size]byte) productport.HistoricalMemberUsage {
	customerID, current, total := int64(31), int64(0), int64(2)
	lastOpenAt := time.Date(2026, 8, 28, 12, 1, 4, 123456789, time.FixedZone("legacy", 8*3600))
	return productport.HistoricalMemberUsage{
		SourceKeyDigest: key, CustomerID: &customerID, FormallyLoggedIn: true, HasTokenUsage: true, LearningPlanID: "  source plan\n",
		LearningPlanCurrent: &current, LearningPlanTotal: &total, OpenCount7D: 0, LastOpenAt: &lastOpenAt,
		RefreshedAt: time.Date(2026, 8, 28, 12, 1, 5, 987654321, time.FixedZone("legacy", 8*3600)), SourcePayloadDigest: payload,
		RecoveryEntryDigest: sha256.Sum256([]byte("recovery-entry")),
	}
}

func stringsToUpper(value string) string {
	result := []byte(value)
	for index := range result {
		if result[index] >= 'a' && result[index] <= 'f' {
			result[index] -= 'a' - 'A'
			return string(result)
		}
	}
	return "A" + value[1:]
}

func copyMemberGridInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func copyMemberGridTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

var _ productport.MemberGridHistoryStore = (*fakeMemberGridHistoryStore)(nil)
var _ productport.MemberGridHistoryJournal = (*fakeMemberGridHistoryJournal)(nil)
