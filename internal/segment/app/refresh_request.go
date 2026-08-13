package app

import (
	"context"
	"errors"
	"reflect"
	"strings"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const RefreshRequestJobKind = "segment_refresh_requested"

var (
	ErrInvalidRefreshRequest  = errors.New("invalid segment refresh request")
	ErrRefreshCommandConflict = errors.New("segment refresh idempotency command conflict")
	ErrRefreshRequestFailed   = errors.New("segment refresh request failed")
)

// RefreshRequestArgs is a durable command for a later Segment worker. S-5A
// only records and queues it; it deliberately does not invoke RefreshOnce.
type RefreshRequestArgs struct {
	SegmentID segmentport.SegmentID `json:"segment_id"`
	ReceiptID int64                 `json:"receipt_id"`
}

func (RefreshRequestArgs) Kind() string { return RefreshRequestJobKind }

type RefreshReceiptState string

const (
	RefreshReceiptReserved RefreshReceiptState = "reserved"
	RefreshReceiptAccepted RefreshReceiptState = "accepted"
)

type RefreshReceipt struct {
	ID         int64
	SegmentID  segmentport.SegmentID
	State      RefreshReceiptState
	RiverJobID *int64
}

type RefreshRequestStore interface {
	ReserveRefreshReceipt(context.Context, segmentport.Actor, string, segmentport.SegmentID) (RefreshReceipt, error)
	AcceptRefreshReceipt(context.Context, int64, int64) (RefreshReceipt, error)
}

type RefreshRequestEnqueuer interface {
	EnqueueRefreshRequest(context.Context, RefreshRequestArgs) (int64, error)
}

// RefreshRequestService closes only the accepted-command boundary: receipt and
// River job share one UoW, so an accepted response never outlives either fact.
type RefreshRequestService struct {
	uow      platformport.UnitOfWork
	store    RefreshRequestStore
	enqueuer RefreshRequestEnqueuer
}

func NewRefreshRequestService(
	uow platformport.UnitOfWork,
	store RefreshRequestStore,
	enqueuer RefreshRequestEnqueuer,
) *RefreshRequestService {
	return &RefreshRequestService{uow: uow, store: store, enqueuer: enqueuer}
}

func (service *RefreshRequestService) RequestRefresh(
	ctx context.Context,
	command segmentport.RefreshCommand,
) (segmentport.Segment, error) {
	if !validRefreshCommand(command) {
		return segmentport.Segment{}, ErrInvalidRefreshRequest
	}
	if service == nil || isNilRefreshDependency(service.uow) || isNilRefreshDependency(service.store) ||
		isNilRefreshDependency(service.enqueuer) || ctx == nil {
		return segmentport.Segment{}, ErrRefreshRequestFailed
	}

	var accepted segmentport.Segment
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, err := service.store.ReserveRefreshReceipt(txCtx, command.Actor, command.IdempotencyKey, command.SegmentID)
		if err != nil {
			return err
		}
		if receipt.ID <= 0 || receipt.SegmentID <= 0 {
			return ErrRefreshRequestFailed
		}
		if receipt.SegmentID != command.SegmentID {
			return ErrRefreshCommandConflict
		}
		switch receipt.State {
		case RefreshReceiptAccepted:
			if receipt.RiverJobID == nil || *receipt.RiverJobID <= 0 {
				return ErrRefreshRequestFailed
			}
		case RefreshReceiptReserved:
			jobID, enqueueErr := service.enqueuer.EnqueueRefreshRequest(txCtx, RefreshRequestArgs{
				SegmentID: command.SegmentID,
				ReceiptID: commandReceiptID(receipt),
			})
			if enqueueErr != nil || jobID <= 0 {
				return errors.Join(ErrRefreshRequestFailed, enqueueErr)
			}
			acceptedReceipt, acceptErr := service.store.AcceptRefreshReceipt(txCtx, receipt.ID, jobID)
			if acceptErr != nil || acceptedReceipt.ID != receipt.ID || acceptedReceipt.SegmentID != command.SegmentID ||
				acceptedReceipt.State != RefreshReceiptAccepted || acceptedReceipt.RiverJobID == nil || *acceptedReceipt.RiverJobID != jobID {
				return errors.Join(ErrRefreshRequestFailed, acceptErr)
			}
		default:
			return ErrRefreshRequestFailed
		}
		accepted = segmentport.Segment{ID: command.SegmentID}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrSegmentNotFound) || errors.Is(err, ErrRefreshCommandConflict) || errors.Is(err, ErrInvalidRefreshRequest) {
			return segmentport.Segment{}, err
		}
		return segmentport.Segment{}, errors.Join(ErrRefreshRequestFailed, err)
	}
	return accepted, nil
}

func validRefreshCommand(command segmentport.RefreshCommand) bool {
	return command.SegmentID > 0 && validRefreshText(string(command.Actor), 1, 200) &&
		validRefreshText(command.IdempotencyKey, 16, 128)
}

func validRefreshText(value string, minimum, maximum int) bool {
	return len(value) >= minimum && len(value) <= maximum && strings.TrimSpace(value) == value && value != ""
}

func commandReceiptID(receipt RefreshReceipt) int64 { return receipt.ID }

func isNilRefreshDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Interface ||
		reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}
