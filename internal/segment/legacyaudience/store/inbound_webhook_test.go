package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	legacyaudience "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudience"
)

type queryResult[T any] struct {
	record T
	err    error
}

type scriptedQueries struct {
	packageExists  bool
	receipts       []queryResult[InboundWebhookReceiptRecord]
	replays        []queryResult[InboundWebhookTransportReplayRecord]
	createdReceipt CreateInboundWebhookReceiptParams
	createdReplay  CreateInboundWebhookTransportReplayParams
	receiptGets    int
	replayGets     int
}

func (queries *scriptedQueries) AIAudienceInboundPackageExists(context.Context, int64) (bool, error) {
	return queries.packageExists, nil
}

func (queries *scriptedQueries) CreateAIAudienceInboundWebhookReceipt(_ context.Context, params CreateInboundWebhookReceiptParams) (InboundWebhookReceiptRecord, error) {
	queries.createdReceipt = params
	result := queries.receipts[0]
	queries.receipts = queries.receipts[1:]
	return result.record, result.err
}

func (queries *scriptedQueries) GetAIAudienceInboundWebhookReceipt(context.Context, int64, []byte) (InboundWebhookReceiptRecord, error) {
	queries.receiptGets++
	result := queries.receipts[0]
	queries.receipts = queries.receipts[1:]
	return result.record, result.err
}

func (queries *scriptedQueries) CreateAIAudienceInboundWebhookTransportReplay(_ context.Context, params CreateInboundWebhookTransportReplayParams) (InboundWebhookTransportReplayRecord, error) {
	queries.createdReplay = params
	result := queries.replays[0]
	queries.replays = queries.replays[1:]
	return result.record, result.err
}

func (queries *scriptedQueries) GetAIAudienceInboundWebhookTransportReplay(context.Context, string, []byte) (InboundWebhookTransportReplayRecord, error) {
	queries.replayGets++
	result := queries.replays[0]
	queries.replays = queries.replays[1:]
	return result.record, result.err
}

func TestInboundWebhookRepositoryReservesReceiptAndTransportReplay(t *testing.T) {
	reservation := inboundReservation()
	queries := &scriptedQueries{packageExists: true}
	queries.receipts = append(queries.receipts, receiptResult(7, reservation, nil))
	queries.replays = append(queries.replays, replayResult(7, reservation.PayloadDigest, nil))
	repository := repositoryForTest(queries)
	exists, err := repository.PackageExistsForInbound(context.Background(), reservation.PackageID)
	if err != nil || !exists {
		t.Fatalf("exists=%t err=%v", exists, err)
	}
	receipt, created, err := repository.ReserveInboundWebhook(context.Background(), reservation)
	if err != nil || !created || receipt.ID != 7 {
		t.Fatalf("receipt=%+v created=%t err=%v", receipt, created, err)
	}
	if queries.createdReceipt.PackageID != reservation.PackageID || queries.createdReceipt.CallbackStatus != reservation.CallbackStatus ||
		queries.createdReplay.ReceiptID != receipt.ID || queries.receiptGets != 0 || queries.replayGets != 0 {
		t.Fatalf("receipt params=%+v replay params=%+v gets=%d/%d", queries.createdReceipt, queries.createdReplay, queries.receiptGets, queries.replayGets)
	}
}

func TestInboundWebhookRepositoryReplaysBusinessEventWithNewTransportEvent(t *testing.T) {
	reservation := inboundReservation()
	queries := &scriptedQueries{}
	queries.receipts = append(queries.receipts, receiptResult(0, reservation, pgx.ErrNoRows), receiptResult(7, reservation, nil))
	queries.replays = append(queries.replays, replayResult(7, reservation.PayloadDigest, nil))
	receipt, created, err := repositoryForTest(queries).ReserveInboundWebhook(context.Background(), reservation)
	if err != nil || created || receipt.ID != 7 || queries.receiptGets != 1 {
		t.Fatalf("receipt=%+v created=%t gets=%d err=%v", receipt, created, queries.receiptGets, err)
	}
}

func TestInboundWebhookRepositoryRejectsTransportReplayBoundToAnotherReceipt(t *testing.T) {
	reservation := inboundReservation()
	queries := &scriptedQueries{}
	queries.receipts = append(queries.receipts, receiptResult(7, reservation, nil))
	queries.replays = append(queries.replays, replayResult(0, reservation.PayloadDigest, pgx.ErrNoRows), replayResult(8, reservation.PayloadDigest, nil))
	if _, _, err := repositoryForTest(queries).ReserveInboundWebhook(context.Background(), reservation); !errors.Is(err, legacyaudience.ErrIdempotencyConflict) {
		t.Fatalf("err=%v", err)
	}
	if queries.replayGets != 1 {
		t.Fatalf("transport replay gets=%d", queries.replayGets)
	}
}

func repositoryForTest(queries InboundWebhookQueries) *InboundWebhookRepository {
	return &InboundWebhookRepository{queries: func(context.Context) (InboundWebhookQueries, error) { return queries, nil }}
}

func inboundReservation() legacyaudience.InboundWebhookReservation {
	return legacyaudience.InboundWebhookReservation{
		PackageID: 42, ClientID: legacyaudience.AIAudienceWebhookClientID,
		TransportEventIDDigest: [32]byte{1}, ExternalEventIDDigest: [32]byte{2}, PayloadDigest: [32]byte{3},
		CallbackStatus: "done", Message: json.RawMessage(`{"text":"ok"}`), Action: json.RawMessage(`{}`),
		CreatedAt: time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC),
	}
}

func receiptResult(id int64, reservation legacyaudience.InboundWebhookReservation, err error) queryResult[InboundWebhookReceiptRecord] {
	return queryResult[InboundWebhookReceiptRecord]{record: InboundWebhookReceiptRecord{
		ID: id, PackageID: reservation.PackageID, State: legacyaudience.InboundWebhookReceived,
		ExternalEventIDDigest: append([]byte(nil), reservation.ExternalEventIDDigest[:]...),
		PayloadDigest:         append([]byte(nil), reservation.PayloadDigest[:]...), CreatedAt: reservation.CreatedAt,
	}, err: err}
}

func replayResult(receiptID int64, digest [32]byte, err error) queryResult[InboundWebhookTransportReplayRecord] {
	return queryResult[InboundWebhookTransportReplayRecord]{
		record: InboundWebhookTransportReplayRecord{ReceiptID: receiptID, PayloadDigest: append([]byte(nil), digest[:]...)}, err: err,
	}
}
