package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	groupopsdb "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/store/generated"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func (repository *Repository) LockDirectoryGroupOwner(ctx context.Context, target string) (int64, error) {
	q, err := queries(ctx)
	if repository == nil || err != nil || target == "" {
		return 0, unavailable(err)
	}
	owner, err := q.LockGroupOpsDirectoryGroupOwner(ctx, target)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, groupopsapp.ErrNotFound
	}
	if err != nil || owner < 1 {
		return 0, unavailable(err)
	}
	return owner, nil
}

type DispatchJobInserter struct {
	client *platformjobqueue.InsertOnlyClient
}

type MaterialContinuationJobInserter struct {
	client *platformjobqueue.InsertOnlyClient
}

func NewMaterialContinuationJobInserter(pool *pgxpool.Pool) (*MaterialContinuationJobInserter, error) {
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, errors.Join(groupopsapp.ErrUnavailable, err)
	}
	return &MaterialContinuationJobInserter{client: client}, nil
}

func (inserter *MaterialContinuationJobInserter) Insert(ctx context.Context, args groupopsapp.GroupOpsMaterialContinuationJobArgs, generation int64, scheduledAt time.Time) (eer.RiverJobLink, error) {
	if inserter == nil || inserter.client == nil || !validRuntimeDigest(args.ExecutionKeyDigest) || generation < 1 || scheduledAt.IsZero() {
		return eer.RiverJobLink{}, groupopsapp.ErrInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return eer.RiverJobLink{}, groupopsapp.ErrUnavailable
	}
	jobID, err := inserter.client.InsertTxScheduled(ctx, tx, args, string(platformjobqueue.QueueOutbound), scheduledAt)
	if err != nil || jobID < 1 {
		return eer.RiverJobLink{}, errors.Join(groupopsapp.ErrUnavailable, err)
	}
	return eer.RiverJobLink{JobID: jobID, Generation: generation, Queue: string(platformjobqueue.QueueOutbound), ArgsDigest: materialContinuationArgsDigest(args), ScheduledAt: scheduledAt.UTC()}, nil
}

func NewDispatchJobInserter(pool *pgxpool.Pool) (*DispatchJobInserter, error) {
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, errors.Join(groupopsapp.ErrUnavailable, err)
	}
	return &DispatchJobInserter{client: client}, nil
}

func (inserter *DispatchJobInserter) Insert(ctx context.Context, args groupopsapp.GroupOpsDispatchJobArgs, generation int64, scheduledAt time.Time) (eer.RiverJobLink, error) {
	if inserter == nil || inserter.client == nil || ctx == nil || args.EffectID == "" || generation < 1 || scheduledAt.IsZero() {
		return eer.RiverJobLink{}, groupopsapp.ErrInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return eer.RiverJobLink{}, groupopsapp.ErrUnavailable
	}
	jobID, err := inserter.client.InsertTxScheduled(ctx, tx, args, string(platformjobqueue.QueueOutbound), scheduledAt)
	if err != nil || jobID < 1 {
		return eer.RiverJobLink{}, errors.Join(groupopsapp.ErrUnavailable, err)
	}
	return eer.RiverJobLink{JobID: jobID, Generation: generation, Queue: string(platformjobqueue.QueueOutbound), ArgsDigest: dispatchArgsDigest(args), ScheduledAt: scheduledAt.UTC()}, nil
}

func dispatchArgsDigest(args groupopsapp.GroupOpsDispatchJobArgs) eer.Digest {
	if args.EffectID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("group-ops.dispatch.job.v1\x00" + args.EffectID))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func materialContinuationArgsDigest(args groupopsapp.GroupOpsMaterialContinuationJobArgs) eer.Digest {
	if !validRuntimeDigest(args.ExecutionKeyDigest) {
		return ""
	}
	sum := sha256.Sum256([]byte("group-ops.material-continuation.job.v1\x00" + args.ExecutionKeyDigest))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func validRuntimeDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

// DispatchExecutionReader reads the EER effect and its Group Ops execution
// together. An expired attempted lease is returned for recovery only; it is
// never a signal to replay the Provider call.
type DispatchExecutionReader struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

var _ groupopsport.DispatchExecutionReader = (*DispatchExecutionReader)(nil)

func NewDispatchExecutionReader(pool *pgxpool.Pool) (*DispatchExecutionReader, error) {
	if pool == nil {
		return nil, groupopsapp.ErrUnavailable
	}
	return &DispatchExecutionReader{pool: pool, now: time.Now}, nil
}
func (reader *DispatchExecutionReader) LoadDispatchExecution(ctx context.Context, effect string) (groupopsport.DispatchExecution, error) {
	if reader == nil || reader.pool == nil || ctx == nil {
		return groupopsport.DispatchExecution{}, groupopsapp.ErrUnavailable
	}
	id, err := parseExternalEffectID(effect)
	if err != nil {
		return groupopsport.DispatchExecution{}, groupopsapp.ErrInvalid
	}
	q := groupopsdb.New(reader.pool)
	row, err := q.GetGroupOpsExecutionByExternalEffectID(ctx, id)
	if err != nil {
		return groupopsport.DispatchExecution{}, unavailable(err)
	}
	value, err := execution(row)
	if err != nil {
		return groupopsport.DispatchExecution{}, err
	}
	var materialSource []byte
	if intent, intentErr := q.GetAcceptedGroupOpsExecutionIntent(ctx, pgtype.Int8{Int64: value.ID, Valid: true}); intentErr == nil {
		materialSource = append([]byte(nil), intent.MaterialSourceSnapshot...)
	} else if !errors.Is(intentErr, pgx.ErrNoRows) {
		return groupopsport.DispatchExecution{}, unavailable(intentErr)
	}
	effectRow, err := q.GetGroupOpsExternalEffect(ctx, id)
	if err != nil || effectRow.Owner != string(eer.OwnerGroupOps) || effectRow.Kind != string(eer.KindGroupOpsBroadcast) {
		return groupopsport.DispatchExecution{}, unavailable(err)
	}
	if value.State != groupopsport.ExecutionAccepted {
		return toDispatchExecution(row, value, materialSource, nil)
	}
	var recovery *groupopsport.AttemptRecoveryLease
	if effectRow.State == string(eer.StateAttempted) && effectRow.LeaseExpiresAt.Valid && !effectRow.LeaseExpiresAt.Time.After(reader.now().UTC()) && effectRow.Generation > 0 && effectRow.LeaseFence > 0 {
		recovery = &groupopsport.AttemptRecoveryLease{Generation: effectRow.Generation, Fence: effectRow.LeaseFence, ExpiresAt: effectRow.LeaseExpiresAt.Time.UTC()}
	}
	return toDispatchExecution(row, value, materialSource, recovery)
}
func toDispatchExecution(row groupopsdb.GroupOpsExecution, value groupopsport.Execution, materialSource []byte, recovery *groupopsport.AttemptRecoveryLease) (groupopsport.DispatchExecution, error) {
	if !row.SenderUseridSnapshot.Valid || row.SenderUseridSnapshot.String == "" {
		return groupopsport.DispatchExecution{}, groupopsapp.ErrUnavailable
	}
	return groupopsport.DispatchExecution{ExecutionID: value.ID, ExternalEffectID: value.ExternalEffectID, State: value.State, DeliveryProven: value.DeliveryProven, AttemptRecovery: recovery, TargetReference: value.TargetReference, SenderUserID: row.SenderUseridSnapshot.String, ContentSnapshot: append([]byte(nil), row.ContentSnapshot...), ContentDigest: value.ContentDigest, MaterialSnapshot: append([]byte(nil), row.MaterialSnapshot...), MaterialDigest: value.MaterialDigest, MaterialSourceSnapshot: materialSource}, nil
}
