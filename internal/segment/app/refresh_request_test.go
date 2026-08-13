package app

import (
	"context"
	"errors"
	"testing"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestRefreshRequestServicePersistsOneAcceptedFactAndOneJob(t *testing.T) {
	t.Parallel()

	store := &refreshRequestStoreStub{receipt: RefreshReceipt{ID: 11, SegmentID: 7, State: RefreshReceiptReserved}}
	enqueuer := &refreshRequestEnqueuerStub{jobID: 99}
	service := NewRefreshRequestService(immediateUOW{}, store, enqueuer)
	command := segmentport.RefreshCommand{SegmentID: 7, Actor: "admin:42", IdempotencyKey: "same-command-key"}

	accepted, err := service.RequestRefresh(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.ID != 7 || enqueuer.calls != 1 || store.acceptCalls != 1 {
		t.Fatalf("accepted/calls = %#v/%d/%d, want segment 7 and one enqueue/accept", accepted, enqueuer.calls, store.acceptCalls)
	}
	if got := enqueuer.args; got.SegmentID != 7 || got.ReceiptID != 11 {
		t.Fatalf("enqueued args = %#v, want segment 7 receipt 11", got)
	}

	store.receipt = RefreshReceipt{ID: 11, SegmentID: 7, State: RefreshReceiptAccepted, RiverJobID: ptr(int64(99))}
	accepted, err = service.RequestRefresh(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.ID != 7 || enqueuer.calls != 1 || store.acceptCalls != 1 {
		t.Fatalf("replay accepted/calls = %#v/%d/%d, want original fact without another job", accepted, enqueuer.calls, store.acceptCalls)
	}
}

func TestRefreshRequestServiceRejectsConflictingOrUnacceptedCommands(t *testing.T) {
	t.Parallel()

	command := segmentport.RefreshCommand{SegmentID: 7, Actor: "admin:42", IdempotencyKey: "same-command-key"}
	for _, test := range []struct {
		name  string
		store *refreshRequestStoreStub
		queue *refreshRequestEnqueuerStub
		want  error
	}{
		{
			name:  "different command conflicts",
			store: &refreshRequestStoreStub{receipt: RefreshReceipt{ID: 11, SegmentID: 8, State: RefreshReceiptAccepted, RiverJobID: ptr(int64(99))}},
			queue: &refreshRequestEnqueuerStub{jobID: 99}, want: ErrRefreshCommandConflict,
		},
		{
			name:  "enqueue failure rolls back acceptance",
			store: &refreshRequestStoreStub{receipt: RefreshReceipt{ID: 11, SegmentID: 7, State: RefreshReceiptReserved}},
			queue: &refreshRequestEnqueuerStub{err: errors.New("queue unavailable")}, want: ErrRefreshRequestFailed,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewRefreshRequestService(immediateUOW{}, test.store, test.queue).RequestRefresh(context.Background(), command)
			if !errors.Is(err, test.want) {
				t.Fatalf("RequestRefresh() error = %v, want %v", err, test.want)
			}
			if test.store.acceptCalls != 0 || (errors.Is(test.want, ErrRefreshCommandConflict) && test.queue.calls != 0) {
				t.Fatalf("accept/enqueue calls = %d/%d, want no completed side effect", test.store.acceptCalls, test.queue.calls)
			}
		})
	}
}

type immediateUOW struct{}

func (immediateUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type refreshRequestStoreStub struct {
	receipt     RefreshReceipt
	reserveErr  error
	acceptErr   error
	acceptCalls int
}

func (stub *refreshRequestStoreStub) ReserveRefreshReceipt(context.Context, segmentport.Actor, string, segmentport.SegmentID) (RefreshReceipt, error) {
	return stub.receipt, stub.reserveErr
}

func (stub *refreshRequestStoreStub) AcceptRefreshReceipt(_ context.Context, receiptID, jobID int64) (RefreshReceipt, error) {
	stub.acceptCalls++
	if stub.acceptErr != nil {
		return RefreshReceipt{}, stub.acceptErr
	}
	return RefreshReceipt{ID: receiptID, SegmentID: stub.receipt.SegmentID, State: RefreshReceiptAccepted, RiverJobID: ptr(jobID)}, nil
}

type refreshRequestEnqueuerStub struct {
	args  RefreshRequestArgs
	jobID int64
	err   error
	calls int
}

func (stub *refreshRequestEnqueuerStub) EnqueueRefreshRequest(_ context.Context, args RefreshRequestArgs) (int64, error) {
	stub.calls++
	stub.args = args
	return stub.jobID, stub.err
}

func ptr[T any](value T) *T { return &value }
