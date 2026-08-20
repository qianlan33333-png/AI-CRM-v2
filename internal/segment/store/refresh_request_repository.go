package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

// RefreshRequestRepository owns the Segment receipt table and only inserts a
// River command inside the transaction supplied by the application service.
type RefreshRequestRepository struct {
	client *platformjobqueue.InsertOnlyClient
}

var (
	_ segmentapp.RefreshRequestStore    = (*RefreshRequestRepository)(nil)
	_ segmentapp.RefreshRequestEnqueuer = (*RefreshRequestRepository)(nil)
)

func NewRefreshRequestRepository(pool *pgxpool.Pool) (*RefreshRequestRepository, error) {
	if pool == nil {
		return nil, segmentapp.ErrRefreshRequestFailed
	}
	// API processes only insert the already-frozen S-5A command. Its worker is
	// intentionally outside this slice, so River must operate in insert-only
	// mode until that worker is registered by the later execution slice.
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, err
	}
	return &RefreshRequestRepository{client: client}, nil
}

// EnsureRefreshable closes the archive race at command acceptance: archived
// definitions retain their snapshot but may never receive another refresh job.
func (repository *RefreshRequestRepository) EnsureRefreshable(ctx context.Context, segmentID segmentport.SegmentID) error {
	if repository == nil || segmentID <= 0 {
		return segmentapp.ErrInvalidRefreshRequest
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT lifecycle_status = 'active' FROM public.segments WHERE id = $1 FOR UPDATE`, int64(segmentID)).Scan(&active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return segmentapp.ErrSegmentNotFound
		}
		return err
	}
	if !active {
		return segmentapp.ErrSegmentNotFound
	}
	return nil
}

func (repository *RefreshRequestRepository) ReserveRefreshReceipt(
	ctx context.Context,
	actor segmentport.Actor,
	key string,
	segmentID segmentport.SegmentID,
) (segmentapp.RefreshReceipt, error) {
	if repository == nil || segmentID <= 0 {
		return segmentapp.RefreshReceipt{}, segmentapp.ErrInvalidRefreshRequest
	}
	queries, err := refreshRequestQueries(ctx)
	if err != nil {
		return segmentapp.RefreshReceipt{}, err
	}
	receipt, err := queries.ReserveSegmentRefreshReceipt(ctx, segmentdb.ReserveSegmentRefreshReceiptParams{
		IdempotencyScope: string(actor), IdempotencyKey: key, SegmentID: int64(segmentID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return segmentapp.RefreshReceipt{}, segmentapp.ErrSegmentNotFound
	}
	if err != nil {
		return segmentapp.RefreshReceipt{}, err
	}
	return refreshReceipt(receipt.ID, receipt.SegmentID, receipt.State, receipt.RiverJobID)
}

func (repository *RefreshRequestRepository) AcceptRefreshReceipt(
	ctx context.Context,
	receiptID int64,
	jobID int64,
) (segmentapp.RefreshReceipt, error) {
	if repository == nil || receiptID <= 0 || jobID <= 0 {
		return segmentapp.RefreshReceipt{}, segmentapp.ErrRefreshRequestFailed
	}
	queries, err := refreshRequestQueries(ctx)
	if err != nil {
		return segmentapp.RefreshReceipt{}, err
	}
	receipt, err := queries.AcceptSegmentRefreshReceipt(ctx, segmentdb.AcceptSegmentRefreshReceiptParams{ID: receiptID, RiverJobID: jobID})
	if err != nil {
		return segmentapp.RefreshReceipt{}, err
	}
	return refreshReceipt(receipt.ID, receipt.SegmentID, receipt.State, receipt.RiverJobID)
}

func (repository *RefreshRequestRepository) EnqueueRefreshRequest(
	ctx context.Context,
	args segmentapp.RefreshRequestArgs,
) (int64, error) {
	if repository == nil || repository.client == nil || args.SegmentID <= 0 || args.ReceiptID <= 0 {
		return 0, segmentapp.ErrRefreshRequestFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	jobID, err := repository.client.InsertTx(ctx, tx, args, "heavy")
	if err != nil || jobID <= 0 {
		return 0, errors.Join(segmentapp.ErrRefreshRequestFailed, err)
	}
	return jobID, nil
}

func refreshRequestQueries(ctx context.Context) (*segmentdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return segmentdb.New(tx), nil
}

func refreshReceipt(id, segmentID int64, state string, jobID pgtype.Int8) (segmentapp.RefreshReceipt, error) {
	if id <= 0 || segmentID <= 0 {
		return segmentapp.RefreshReceipt{}, segmentapp.ErrRefreshRequestFailed
	}
	result := segmentapp.RefreshReceipt{ID: id, SegmentID: segmentport.SegmentID(segmentID), State: segmentapp.RefreshReceiptState(state)}
	if jobID.Valid {
		copied := jobID.Int64
		result.RiverJobID = &copied
	}
	return result, nil
}
