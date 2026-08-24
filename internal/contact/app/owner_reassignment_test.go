package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

func TestParseOwnerReassignmentCSVStrictAndFormulaSafe(t *testing.T) {
	now := time.Date(2026, 8, 24, 9, 0, 0, 123000000, time.UTC)
	data := []byte("customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\r\n41,7," + now.Format(time.RFC3339Nano) + ",9\r\n41,7," + now.Format(time.RFC3339Nano) + ",9\r\n")
	rows, issues, err := parseOwnerReassignmentCSV(data)
	if err != nil || len(rows) != 1 || rows[0].CustomerID != 41 || len(issues) != 1 || issues[0].Line != 3 {
		t.Fatalf("rows=%+v issues=%+v err=%v", rows, issues, err)
	}
	_, sameOwnerIssues, err := parseOwnerReassignmentCSV([]byte("customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\n42,7," + now.Format(time.RFC3339Nano) + ",7\n43,7," + now.Format(time.RFC3339Nano) + ",9\n"))
	if err != nil || len(sameOwnerIssues) != 1 || sameOwnerIssues[0].Code != "invalid_row" {
		t.Fatalf("same owner issues=%+v err=%v", sameOwnerIssues, err)
	}
	for _, bad := range [][]byte{
		[]byte("customer_id,target_owner_staff_id,expected_updated_at,expected_owner_staff_id\n"),
		[]byte("customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\n=41,7," + now.Format(time.RFC3339Nano) + ",9\n"),
	} {
		if _, _, err := parseOwnerReassignmentCSV(bad); !errors.Is(err, ErrOwnerReassignmentInvalid) {
			t.Fatalf("bad csv error=%v", err)
		}
	}
}

func TestOwnerReassignmentExecuteLocksTargetsThenCustomersAndCompletesReceipt(t *testing.T) {
	now := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	store := &ownerReassignmentStoreFake{customers: map[int64]OwnerReassignmentRow{
		41: {CustomerID: 41, ExpectedOwnerStaffID: 7, ExpectedUpdatedAt: now},
		99: {CustomerID: 99, ExpectedOwnerStaffID: 8, ExpectedUpdatedAt: now},
	}}
	eventLog := 0
	service := NewOwnerReassignmentService(ownerReassignmentUOW{}, store, ownerReassignmentEvents{count: &eventLog})
	service.now = func() time.Time { return now }
	csv := []byte("customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\n99,8," + now.Format(time.RFC3339Nano) + ",4\n41,7," + now.Format(time.RFC3339Nano) + ",3\n")
	preview, err := service.CreatePreview(context.Background(), 11, csv, "owner-preview-key-01")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Execute(context.Background(), 11, preview.ID, preview.Hash, OwnerReassignmentConfirmation, "owner-reassign-key-01")
	if err != nil || !result.Executed || !store.completed || !store.marked || store.customerEvents != 2 || eventLog != 2 {
		t.Fatalf("result=%+v store=%+v err=%v", result, store, err)
	}
	raw, err := json.Marshal(result.Result[0])
	if err != nil || string(raw) == "" || string(raw) == "{}" || !containsJSONField(raw, "previous_owner_staff_id") || containsJSONField(raw, "expected_updated_at") {
		t.Fatalf("result json=%s err=%v", raw, err)
	}
	if got := store.locks; len(got) != 4 || got[0] != "staff:3" || got[1] != "staff:4" || got[2] != "customer:41" || got[3] != "customer:99" {
		t.Fatalf("locks=%v", got)
	}
	if _, err = service.Execute(context.Background(), 11, preview.ID, preview.Hash, OwnerReassignmentConfirmation, "owner-reassign-key-02"); !errors.Is(err, ErrOwnerReassignmentConflict) {
		t.Fatalf("single use err=%v", err)
	}
}

type ownerReassignmentUOW struct{}

func (ownerReassignmentUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type ownerReassignmentEvents struct{ count *int }

func (s ownerReassignmentEvents) Append(_ context.Context, e eventport.Event) (eventport.EventID, error) {
	if e.Type != eventport.EvCustomerUpdated {
		return 0, errors.New("unexpected event")
	}
	if s.count != nil {
		*s.count++
	}
	return 1, nil
}

type ownerReassignmentStoreFake struct {
	preview             OwnerReassignmentPreview
	customers           map[int64]OwnerReassignmentRow
	locks               []string
	receipt             OwnerReassignmentReceipt
	marked, completed   bool
	reserveCalls        int
	customerEvents      int
	previewKey          []byte
	previewPayload      []byte
	forceReservedReplay bool
}

func (s *ownerReassignmentStoreFake) CreateOwnerReassignmentPreview(_ context.Context, p OwnerReassignmentPreview, _ int64, digest, key []byte, _ time.Time) (OwnerReassignmentPreview, bool, error) {
	if string(s.previewKey) == string(key) {
		if string(s.previewPayload) != string(digest) {
			return OwnerReassignmentPreview{}, false, ErrOwnerReassignmentConflict
		}
		return s.preview, false, nil
	}
	s.preview = p
	s.previewKey = append([]byte(nil), key...)
	s.previewPayload = append([]byte(nil), digest...)
	return p, true, nil
}
func (s *ownerReassignmentStoreFake) ReadOwnerReassignmentPreview(_ context.Context, _ string, _ int64) (OwnerReassignmentPreview, error) {
	return s.preview, nil
}
func (s *ownerReassignmentStoreFake) ReserveOwnerReassignmentReceipt(_ context.Context, _ int64, _ []byte, p []byte, _ time.Time) (OwnerReassignmentReceipt, bool, error) {
	s.reserveCalls++
	if s.forceReservedReplay {
		return OwnerReassignmentReceipt{ID: 1, PayloadDigest: append([]byte(nil), p...)}, false, nil
	}
	if s.reserveCalls == 1 {
		s.receipt = OwnerReassignmentReceipt{ID: 1, PayloadDigest: append([]byte(nil), p...)}
		return s.receipt, true, nil
	}
	return OwnerReassignmentReceipt{ID: int64(s.reserveCalls), PayloadDigest: append([]byte(nil), p...)}, true, nil
}
func (s *ownerReassignmentStoreFake) LockOwnerReassignmentPreview(_ context.Context, _ string, _ int64, hash []byte, now time.Time) (OwnerReassignmentPreview, error) {
	if s.preview.Executed {
		return s.preview, ErrOwnerReassignmentConflict
	}
	want := decodeOwnerReassignmentHash(s.preview.Hash)
	if string(hash) != string(want) || now.After(s.preview.ExpiresAt) {
		return s.preview, ErrOwnerReassignmentConflict
	}
	return s.preview, nil
}
func (s *ownerReassignmentStoreFake) LockActiveOwnerReassignmentStaff(_ context.Context, id int64) error {
	s.locks = append(s.locks, "staff:"+strconv.FormatInt(id, 10))
	return nil
}
func (s *ownerReassignmentStoreFake) LockOwnerReassignmentCustomer(_ context.Context, id int64) (OwnerReassignmentRow, error) {
	s.locks = append(s.locks, "customer:"+strconv.FormatInt(id, 10))
	return s.customers[id], nil
}
func (s *ownerReassignmentStoreFake) UpdateOwnerReassignmentCustomer(_ context.Context, id, target int64) (time.Time, error) {
	row := s.customers[id]
	row.TargetOwnerStaffID = target
	s.customers[id] = row
	return row.ExpectedUpdatedAt.Add(time.Second), nil
}
func (s *ownerReassignmentStoreFake) AppendOwnerReassignmentCustomerEvent(context.Context, int64, []byte, int64, time.Time) error {
	s.customerEvents++
	return nil
}
func (s *ownerReassignmentStoreFake) MarkOwnerReassignmentPreviewExecuted(_ context.Context, _ string, result []OwnerReassignmentResultRow, _ time.Time) error {
	s.preview.Executed = true
	s.preview.Result = result
	s.marked = true
	return nil
}
func (s *ownerReassignmentStoreFake) CompleteOwnerReassignmentReceipt(_ context.Context, _ int64, result []OwnerReassignmentResultRow, _ time.Time) error {
	s.receipt.Completed = true
	s.receipt.Result = result
	s.completed = true
	return nil
}

func containsJSONField(raw []byte, field string) bool {
	var value map[string]any
	return json.Unmarshal(raw, &value) == nil && value[field] != nil
}

func TestOwnerReassignmentPreviewIdempotencyReplaysOrConflicts(t *testing.T) {
	now := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	store := &ownerReassignmentStoreFake{}
	service := NewOwnerReassignmentService(ownerReassignmentUOW{}, store, ownerReassignmentEvents{})
	service.now = func() time.Time { return now }
	data := []byte("customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\n41,7," + now.Format(time.RFC3339Nano) + ",9\n")
	first, err := service.CreatePreview(context.Background(), 11, data, "owner-preview-key-01")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.CreatePreview(context.Background(), 11, data, "owner-preview-key-01")
	if err != nil || replay.ID != first.ID || replay.Hash != first.Hash {
		t.Fatalf("replay=%+v first=%+v err=%v", replay, first, err)
	}
	if _, err = service.CreatePreview(context.Background(), 11, []byte("customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\n41,7,"+now.Format(time.RFC3339Nano)+",10\n"), "owner-preview-key-01"); !errors.Is(err, ErrOwnerReassignmentConflict) {
		t.Fatalf("key conflict=%v", err)
	}
}

func TestOwnerReassignmentExecuteDoesNotReplayReservedReceipt(t *testing.T) {
	now := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	store := &ownerReassignmentStoreFake{forceReservedReplay: true}
	service := NewOwnerReassignmentService(ownerReassignmentUOW{}, store, ownerReassignmentEvents{})
	service.now = func() time.Time { return now }
	preview, err := service.CreatePreview(context.Background(), 11, []byte("customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\n41,7,"+now.Format(time.RFC3339Nano)+",9\n"), "owner-preview-key-01")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Execute(context.Background(), 11, preview.ID, preview.Hash, OwnerReassignmentConfirmation, "owner-execute-key-01"); !errors.Is(err, ErrOwnerReassignmentConflict) {
		t.Fatalf("reserved receipt replay err=%v", err)
	}
}

func TestOwnerReassignmentPreviewExpiresOnlyBeforeExecution(t *testing.T) {
	now := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	store := &ownerReassignmentStoreFake{preview: OwnerReassignmentPreview{
		ID: "cor_abcdefghijklmnopqrstuv", ExpiresAt: now.Add(-time.Second),
	}}
	service := NewOwnerReassignmentService(ownerReassignmentUOW{}, store, ownerReassignmentEvents{})
	service.now = func() time.Time { return now }
	if _, err := service.Preview(context.Background(), 11, store.preview.ID); !errors.Is(err, ErrOwnerReassignmentExpired) {
		t.Fatalf("unexecuted expired preview err=%v", err)
	}
	store.preview.Executed = true
	store.preview.Result = []OwnerReassignmentResultRow{{CustomerID: 41, PreviousOwnerStaffID: 7, TargetOwnerStaffID: 9, UpdatedAt: now}}
	got, err := service.Preview(context.Background(), 11, store.preview.ID)
	if err != nil || !got.Executed || len(got.Result) != 1 {
		t.Fatalf("executed expired preview=%+v err=%v", got, err)
	}
}
