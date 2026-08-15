package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

type boardTestTxKey struct{}

type boardTestUOW struct{ calls int }

func (u *boardTestUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	u.calls++
	return fn(context.WithValue(ctx, boardTestTxKey{}, true))
}

type boardTestEvents struct{ rows []eventport.Event }

func (e *boardTestEvents) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	if in, _ := ctx.Value(boardTestTxKey{}).(bool); !in {
		return 0, errors.New("order event escaped its transaction")
	}
	e.rows = append(e.rows, event)
	return eventport.EventID(len(e.rows)), nil
}

type boardTestStore struct {
	record       orderport.Record
	records      []orderport.Record
	receipts     map[string]BoardReceipt
	effects      map[int64]orderport.ExternalEffect
	refunds      []orderport.Refund
	exports      map[string]orderport.ExportJob
	nextEffectID int64
	nextRefundID int64
}

func newBoardTestStore(now time.Time) *boardTestStore {
	record := validOrderRecord()
	record.CreatedAt, record.UpdatedAt, record.Status = now, now, "paid"
	return &boardTestStore{record: record, records: []orderport.Record{record}, receipts: map[string]BoardReceipt{}, effects: map[int64]orderport.ExternalEffect{}, exports: map[string]orderport.ExportJob{}, nextEffectID: 1, nextRefundID: 1}
}

func (s *boardTestStore) ListBoardOrders(_ context.Context, filter orderport.BoardFilter) ([]orderport.Record, error) {
	start := int(filter.Offset)
	if start >= len(s.records) {
		return []orderport.Record{}, nil
	}
	end := start + int(filter.Limit)
	if end > len(s.records) {
		end = len(s.records)
	}
	return append([]orderport.Record(nil), s.records[start:end]...), nil
}
func (s *boardTestStore) CountBoardOrders(_ context.Context, _ orderport.BoardFilter) (int64, error) {
	return int64(len(s.records)), nil
}
func (s *boardTestStore) GetBoardOrder(_ context.Context, provider, reference string) (orderport.Record, error) {
	if (provider == "auto" || provider == s.record.Provider) && (reference == s.record.MerchantOrderNo || reference == s.record.PlatformTransactionNo || reference == fmt.Sprint(s.record.ID)) {
		return s.record, nil
	}
	return orderport.Record{}, ErrNotFound
}
func (s *boardTestStore) LockBoardOrder(ctx context.Context, provider, reference string) (orderport.Record, error) {
	if in, _ := ctx.Value(boardTestTxKey{}).(bool); !in {
		return orderport.Record{}, errors.New("order lock escaped transaction")
	}
	return s.GetBoardOrder(ctx, provider, reference)
}
func (s *boardTestStore) CountActiveRefundAmount(_ context.Context, orderID orderport.ID) (int64, error) {
	var total int64
	for _, refund := range s.refunds {
		if refund.OrderID == orderID && (refund.Status == "pending_external_gate" || refund.Status == "outcome_unknown" || refund.Status == "completed") {
			total += refund.RefundAmountTotal
		}
	}
	return total, nil
}
func boardTestReceiptKey(x BoardReservation) string {
	return x.Operation + "\x00" + x.ActorScope + "\x00" + fmt.Sprintf("%x", x.KeyDigest)
}
func (s *boardTestStore) ReserveBoardReceipt(ctx context.Context, x BoardReservation) (BoardReceipt, bool, error) {
	if in, _ := ctx.Value(boardTestTxKey{}).(bool); !in {
		return BoardReceipt{}, false, errors.New("receipt escaped transaction")
	}
	key := boardTestReceiptKey(x)
	if row, found := s.receipts[key]; found {
		return row, false, nil
	}
	row := BoardReceipt{ID: int64(len(s.receipts) + 1), Operation: x.Operation, ActorScope: x.ActorScope, KeyDigest: x.KeyDigest, PayloadDigest: x.PayloadDigest, State: "in_progress"}
	s.receipts[key] = row
	return row, true, nil
}
func (s *boardTestStore) CompleteBoardReceipt(ctx context.Context, id int64, snapshot json.RawMessage, _ time.Time) (BoardReceipt, error) {
	if in, _ := ctx.Value(boardTestTxKey{}).(bool); !in {
		return BoardReceipt{}, errors.New("receipt completion escaped transaction")
	}
	for key, row := range s.receipts {
		if row.ID == id && row.State == "in_progress" {
			row.State, row.ResultSnapshot = "completed", append(json.RawMessage(nil), snapshot...)
			s.receipts[key] = row
			return row, nil
		}
	}
	return BoardReceipt{}, ErrBoardUnavailable
}
func (s *boardTestStore) CreateExportJob(_ context.Context, job orderport.ExportJob) (orderport.ExportJob, error) {
	job.Status, job.DownloadURL, job.ContentType, job.FileName = "completed", "/api/admin/exports/"+job.JobID, "text/csv", job.JobID+".csv"
	s.exports[job.JobID] = job
	return job, nil
}
func (s *boardTestStore) GetExportJob(_ context.Context, jobID string) (orderport.ExportJob, error) {
	job, found := s.exports[jobID]
	if !found {
		return orderport.ExportJob{}, ErrNotFound
	}
	return job, nil
}
func (s *boardTestStore) CreateExternalEffect(ctx context.Context, effect orderport.ExternalEffect) (orderport.ExternalEffect, error) {
	if in, _ := ctx.Value(boardTestTxKey{}).(bool); !in {
		return orderport.ExternalEffect{}, errors.New("external-effect intent escaped transaction")
	}
	effect.ID, effect.AutoRetryAllowed = s.nextEffectID, false
	s.nextEffectID++
	s.effects[effect.ID] = effect
	return effect, nil
}
func (s *boardTestStore) GetExternalEffect(_ context.Context, id int64, _ bool) (orderport.ExternalEffect, error) {
	effect, found := s.effects[id]
	if !found {
		return orderport.ExternalEffect{}, ErrNotFound
	}
	return effect, nil
}
func (s *boardTestStore) ListExternalEffects(_ context.Context, orderID orderport.ID) ([]orderport.ExternalEffect, int64, error) {
	items := []orderport.ExternalEffect{}
	for _, effect := range s.effects {
		if effect.OrderID == orderID {
			items = append(items, effect)
		}
	}
	return items, int64(len(items)), nil
}
func (s *boardTestStore) RequestExternalEffectReview(ctx context.Context, id int64, at time.Time) (orderport.ExternalEffect, error) {
	if in, _ := ctx.Value(boardTestTxKey{}).(bool); !in {
		return orderport.ExternalEffect{}, errors.New("effect review escaped transaction")
	}
	effect, err := s.GetExternalEffect(ctx, id, true)
	if err != nil {
		return orderport.ExternalEffect{}, err
	}
	effect.ManualReviewRequested, effect.UpdatedAt = at, at
	s.effects[id] = effect
	return effect, nil
}
func (s *boardTestStore) CreateRefund(ctx context.Context, refund orderport.Refund) (orderport.Refund, error) {
	if in, _ := ctx.Value(boardTestTxKey{}).(bool); !in {
		return orderport.Refund{}, errors.New("refund intent escaped transaction")
	}
	refund.ID = s.nextRefundID
	s.nextRefundID++
	s.refunds = append(s.refunds, refund)
	return refund, nil
}
func (s *boardTestStore) ListRefunds(_ context.Context, _ orderport.RefundFilter) ([]orderport.Refund, int64, error) {
	return append([]orderport.Refund(nil), s.refunds...), int64(len(s.refunds)), nil
}

func boardTestService(now time.Time, store *boardTestStore, events *boardTestEvents) (*BoardService, *boardTestUOW) {
	uow := &boardTestUOW{}
	service := NewBoardService(uow, store, events)
	service.now = func() time.Time { return now }
	return service, uow
}

func TestOrderBoardRefundIntentUsesOneReceiptAndEventTransaction(t *testing.T) {
	now := time.Date(2026, 8, 15, 17, 0, 0, 0, time.UTC)
	store, events := newBoardTestStore(now), &boardTestEvents{}
	service, uow := boardTestService(now, store, events)
	command := orderport.RefundCommand{Provider: "wechat", OrderReference: "M-11", RefundAmountTotal: 1990, Reason: "重复支付", TransactionIDConfirmation: "WX-11", Checked: true, Actor: 9, IdempotencyKey: "aaaaaaaaaaaaaaaa"}
	first, err := service.RequestRefund(context.Background(), command)
	if err != nil || first.ID < 1 || first.OrderID != 11 || first.Status != "pending_external_gate" || first.AutoRetryAllowed || len(events.rows) != 1 || events.rows[0].Type != eventport.EvOrderRefundRequested || uow.calls != 1 {
		t.Fatalf("first=%#v err=%v events=%#v uow=%d", first, err, events.rows, uow.calls)
	}
	replay, err := service.RequestRefund(context.Background(), command)
	if err != nil || replay != first || len(store.refunds) != 1 || len(store.effects) != 1 || len(events.rows) != 1 {
		t.Fatalf("replay=%#v err=%v refunds=%d effects=%d events=%d", replay, err, len(store.refunds), len(store.effects), len(events.rows))
	}
	command.Reason = "不同的退款理由"
	if _, err = service.RequestRefund(context.Background(), command); !errors.Is(err, ErrBoardConflict) || len(store.refunds) != 1 || len(events.rows) != 1 {
		t.Fatalf("same key different payload err=%v refunds=%d events=%d", err, len(store.refunds), len(events.rows))
	}
}

func TestOrderBoardOutcomeUnknownNeverEntersRetryPath(t *testing.T) {
	now := time.Date(2026, 8, 15, 17, 0, 0, 0, time.UTC)
	store, events := newBoardTestStore(now), &boardTestEvents{}
	store.effects[7] = orderport.ExternalEffect{ID: 7, OrderID: 11, Provider: "wechat", EffectKind: "refund", State: "outcome_unknown", AutoRetryAllowed: false, CreatedAt: now, UpdatedAt: now}
	service, _ := boardTestService(now, store, events)
	if _, err := service.RequestExternalEffectRetry(context.Background(), 7, 9, "bbbbbbbbbbbbbbbb"); !errors.Is(err, ErrBoardConflict) {
		t.Fatalf("outcome_unknown retry error=%v", err)
	}
	if !store.effects[7].ManualReviewRequested.IsZero() || len(events.rows) != 0 {
		t.Fatalf("outcome_unknown was changed or emitted an event: effect=%#v events=%#v", store.effects[7], events.rows)
	}
}

func TestOrderBoardReceiptJSONEqualityIsSemantic(t *testing.T) {
	if !jsonEquivalent([]byte(`{"nested":{"amount":1.0},"items":[2e1]}`), []byte(`{"items":[20],"nested":{"amount":1}}`)) {
		t.Fatal("numeric-equivalent JSONB receipts must replay")
	}
	if jsonEquivalent([]byte(`{"amount":1}`), []byte(`{"amount":2}`)) {
		t.Fatal("different receipt result must not replay")
	}
}

func TestOrderBoardExportCarriesEveryPage(t *testing.T) {
	now := time.Date(2026, 8, 15, 17, 0, 0, 0, time.UTC)
	store, events := newBoardTestStore(now), &boardTestEvents{}
	store.records = make([]orderport.Record, MaximumLimit+1)
	for i := range store.records {
		store.records[i] = store.record
		store.records[i].MerchantOrderNo = fmt.Sprintf("M-%03d", i+1)
		store.records[i].PlatformTransactionNo = fmt.Sprintf("WX-%03d", i+1)
	}
	service, _ := boardTestService(now, store, events)
	job, err := service.CreateExport(context.Background(), orderport.ExportCommand{Resource: "orders", Format: "csv", Actor: 9, IdempotencyKey: "cccccccccccccccc"})
	if err != nil || strings.Count(job.ContentText, "\n") != int(MaximumLimit)+2 || len(events.rows) != 1 {
		t.Fatalf("job=%#v err=%v csv_lines=%d events=%d", job, err, strings.Count(job.ContentText, "\n"), len(events.rows))
	}
}
