package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	groupopsdb "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/store/generated"
)

// GroupMessageReceiptStore is deliberately separate from the request-facing
// Repository: a Provider response crosses the external boundary before it can
// be projected through EER, so its evidence is stored immediately and never
// reconstructed from an in-memory adapter result.
type GroupMessageReceiptStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

var _ groupopsport.GroupMessageReceiptWriter = (*GroupMessageReceiptStore)(nil)
var _ groupopsport.GroupMessageReceiptReader = (*GroupMessageReceiptStore)(nil)

func NewGroupMessageReceiptStore(pool *pgxpool.Pool) (*GroupMessageReceiptStore, error) {
	if pool == nil {
		return nil, groupopsapp.ErrUnavailable
	}
	return &GroupMessageReceiptStore{pool: pool, now: time.Now}, nil
}

func (store *GroupMessageReceiptStore) RecordGroupMessageTask(ctx context.Context, value groupopsport.GroupMessageReceipt) error {
	if store == nil || store.pool == nil || ctx == nil || !validGroupMessageReceipt(value) {
		return groupopsapp.ErrInvalid
	}
	effectID, err := parseExternalEffectID(value.ExternalEffectID)
	if err != nil {
		return groupopsapp.ErrInvalid
	}
	now := timestamp(store.now().UTC())
	q := groupopsdb.New(store.pool)
	row, err := q.InsertGroupOpsWeComGroupMessageReceipt(ctx, groupopsdb.InsertGroupOpsWeComGroupMessageReceiptParams{
		ExternalEffectID: effectID, ExecutionID: value.ExecutionID, Msgid: value.MessageID, SenderUserid: value.SenderUserID,
		ChatID: value.ChatID, Userid: value.UserID, TaskEvidenceDigest: value.TaskEvidenceDigest, CreatedAt: now, UpdatedAt: now,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		row, err = q.GetGroupOpsWeComGroupMessageReceipt(ctx, effectID)
	}
	if err != nil || !sameGroupMessageReceipt(row, value) {
		return unavailable(err)
	}
	return nil
}

func (store *GroupMessageReceiptStore) FindGroupMessageReceipt(ctx context.Context, request groupopsport.ReconciliationEvidence) (groupopsport.GroupMessageReceipt, bool, error) {
	if store == nil || store.pool == nil || ctx == nil || request.ExecutionID < 1 {
		return groupopsport.GroupMessageReceipt{}, false, groupopsapp.ErrInvalid
	}
	effectID, err := parseExternalEffectID(request.ExternalEffectID)
	if err != nil {
		return groupopsport.GroupMessageReceipt{}, false, groupopsapp.ErrInvalid
	}
	row, err := groupopsdb.New(store.pool).GetGroupOpsWeComGroupMessageReceipt(ctx, effectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return groupopsport.GroupMessageReceipt{}, false, nil
	}
	if err != nil || row.ExecutionID != request.ExecutionID || row.TaskEvidenceDigest != request.EvidenceDigest {
		return groupopsport.GroupMessageReceipt{}, false, unavailable(err)
	}
	return groupMessageReceipt(row), true, nil
}

func (store *GroupMessageReceiptStore) RecordGroupMessageDelivery(ctx context.Context, value groupopsport.GroupMessageReceipt, evidence string) error {
	if store == nil || store.pool == nil || ctx == nil || !validGroupMessageReceipt(value) || !validGroupMessageDigest(evidence) {
		return groupopsapp.ErrInvalid
	}
	effectID, err := parseExternalEffectID(value.ExternalEffectID)
	if err != nil {
		return groupopsapp.ErrInvalid
	}
	row, err := groupopsdb.New(store.pool).RecordGroupOpsWeComGroupMessageDelivery(ctx, groupopsdb.RecordGroupOpsWeComGroupMessageDeliveryParams{
		DeliveryEvidenceDigest: pgtype.Text{String: evidence, Valid: true}, UpdatedAt: timestamp(store.now().UTC()), ExternalEffectID: effectID,
		Msgid: value.MessageID, SenderUserid: value.SenderUserID, ChatID: value.ChatID, Userid: value.UserID, TaskEvidenceDigest: value.TaskEvidenceDigest,
	})
	if err != nil || !row.SendStatus.Valid || row.SendStatus.Int32 != 1 || !row.DeliveryEvidenceDigest.Valid || row.DeliveryEvidenceDigest.String != evidence {
		return unavailable(err)
	}
	return nil
}

func groupMessageReceipt(row groupopsdb.GroupOpsWecomGroupMessageReceipt) groupopsport.GroupMessageReceipt {
	value := groupopsport.GroupMessageReceipt{ExecutionID: row.ExecutionID, ExternalEffectID: "eer_" + strconv.FormatInt(row.ExternalEffectID, 10), MessageID: row.Msgid, SenderUserID: row.SenderUserid, ChatID: row.ChatID, UserID: row.Userid, TaskEvidenceDigest: row.TaskEvidenceDigest}
	if row.SendStatus.Valid {
		status := int(row.SendStatus.Int32)
		value.DeliveryStatus = &status
	}
	if row.DeliveryEvidenceDigest.Valid {
		value.DeliveryEvidenceDigest = row.DeliveryEvidenceDigest.String
	}
	return value
}

func sameGroupMessageReceipt(row groupopsdb.GroupOpsWecomGroupMessageReceipt, value groupopsport.GroupMessageReceipt) bool {
	return row.ExecutionID == value.ExecutionID && row.Msgid == value.MessageID && row.SenderUserid == value.SenderUserID && row.ChatID == value.ChatID && row.Userid == value.UserID && row.TaskEvidenceDigest == value.TaskEvidenceDigest
}

func validGroupMessageReceipt(value groupopsport.GroupMessageReceipt) bool {
	return value.ExecutionID > 0 && value.MessageID != "" && value.SenderUserID != "" && value.ChatID != "" && value.UserID != "" && validGroupMessageDigest(value.TaskEvidenceDigest)
}

func validGroupMessageDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
