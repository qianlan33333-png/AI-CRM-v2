// Package store persists only User Ops local facts. Contact customers and
// Media metadata are deliberately read through composition ports instead of
// being joined or referenced by this schema.
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/userops/domain"
	useropsport "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/port"
	useropsdb "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/store/generated"
)

type Repository struct{}

var _ useropsport.Repository = (*Repository)(nil)

func NewRepository() *Repository { return &Repository{} }

func (r *Repository) ReadLocalOverview(ctx context.Context) (useropsport.LocalOverviewRead, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return useropsport.LocalOverviewRead{}, unavailable(err)
	}
	row, err := q.ReadUserOpsLocalOverview(ctx)
	if err != nil {
		return useropsport.LocalOverviewRead{}, unavailable(err)
	}
	return useropsport.LocalOverviewRead{ActiveDNDCount: row.ActiveDndCount, DraftPlanCount: row.DraftPlanCount, PendingReviewPlanCount: row.PendingReviewPlanCount}, nil
}

func (r *Repository) ReadDND(ctx context.Context, customerID domain.CustomerID) (*domain.DoNotDisturb, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || !customerID.Valid() {
		return nil, unavailable(err)
	}
	row, err := q.ReadUserOpsDND(ctx, int64(customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, unavailable(err)
	}
	value, err := dnd(row)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *Repository) ListActiveDND(ctx context.Context, customerIDs []domain.CustomerID) ([]domain.DoNotDisturb, error) {
	return r.activeDND(ctx, customerIDs, false)
}

func (r *Repository) LockActiveDND(ctx context.Context, customerIDs []domain.CustomerID) ([]domain.DoNotDisturb, error) {
	return r.activeDND(ctx, customerIDs, true)
}

func (r *Repository) activeDND(ctx context.Context, customerIDs []domain.CustomerID, lock bool) ([]domain.DoNotDisturb, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return nil, unavailable(err)
	}
	ids := customerIDsInt64(customerIDs)
	var rows []useropsdb.UserOpsDnd
	if lock {
		rows, err = q.LockUserOpsActiveDND(ctx, ids)
	} else {
		rows, err = q.ListUserOpsActiveDND(ctx, ids)
	}
	if err != nil {
		return nil, unavailable(err)
	}
	items := make([]domain.DoNotDisturb, len(rows))
	for i, row := range rows {
		if items[i], err = dnd(row); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (r *Repository) UpsertDND(ctx context.Context, input useropsport.UpsertDNDInput) (useropsport.DNDMutation, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return useropsport.DNDMutation{}, unavailable(err)
	}
	payload := dndSetPayload(input)
	receipt, fresh, err := reserve(ctx, q, "dnd_set", input.ActorID, input.IdempotencyKey, payload)
	if err != nil {
		return useropsport.DNDMutation{}, err
	}
	if !fresh {
		return replayDNDSet(receipt, payload)
	}
	var row useropsdb.UserOpsDnd
	if input.ExpectedVersion == nil {
		row, err = q.InsertUserOpsDND(ctx, useropsdb.InsertUserOpsDNDParams{CustomerID: int64(input.CustomerID), Reason: input.Reason})
	} else {
		row, err = q.UpdateUserOpsDND(ctx, useropsdb.UpdateUserOpsDNDParams{CustomerID: int64(input.CustomerID), Reason: input.Reason, ExpectedVersion: *input.ExpectedVersion})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return useropsport.DNDMutation{}, useropsport.ErrConflict
	}
	if err != nil {
		return useropsport.DNDMutation{}, unavailable(err)
	}
	value, err := dnd(row)
	if err != nil {
		return useropsport.DNDMutation{}, err
	}
	snapshot, _ := json.Marshal(value)
	if err = complete(ctx, q, receipt.ID, snapshot); err != nil {
		return useropsport.DNDMutation{}, err
	}
	return useropsport.DNDMutation{}, nil
}

func (r *Repository) ClearDND(ctx context.Context, input useropsport.ClearDNDInput) (useropsport.DNDMutation, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return useropsport.DNDMutation{}, unavailable(err)
	}
	payload := dndClearPayload(input)
	receipt, fresh, err := reserve(ctx, q, "dnd_clear", input.ActorID, input.IdempotencyKey, payload)
	if err != nil {
		return useropsport.DNDMutation{}, err
	}
	if !fresh {
		return replayDNDClear(receipt, payload)
	}
	id, err := q.DeleteUserOpsDND(ctx, useropsdb.DeleteUserOpsDNDParams{CustomerID: int64(input.CustomerID), ExpectedVersion: input.ExpectedVersion})
	if errors.Is(err, pgx.ErrNoRows) || id != int64(input.CustomerID) {
		return useropsport.DNDMutation{}, useropsport.ErrConflict
	}
	if err != nil {
		return useropsport.DNDMutation{}, unavailable(err)
	}
	snapshot := []byte(`{"cleared":true}`)
	if err = complete(ctx, q, receipt.ID, snapshot); err != nil {
		return useropsport.DNDMutation{}, err
	}
	return useropsport.DNDMutation{Cleared: true}, nil
}

func (r *Repository) CreateLocalPlan(ctx context.Context, input useropsport.CreateLocalPlanInput, targets []domain.CustomerID, targetDigest string, content domain.ContentSnapshot) (useropsport.PlanMutation, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return useropsport.PlanMutation{}, unavailable(err)
	}
	payload := planPayload(input, content)
	receipt, fresh, err := reserve(ctx, q, "local_plan_create", input.ActorID, input.IdempotencyKey, payload)
	if err != nil {
		return useropsport.PlanMutation{}, err
	}
	if !fresh {
		return replayPlan(receipt, payload)
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return useropsport.PlanMutation{}, unavailable(err)
	}
	contentDigest, err := decodeDigest(content.ContentDigest)
	if err != nil {
		return useropsport.PlanMutation{}, unavailable(err)
	}
	targetHash, err := decodeDigest(targetDigest)
	if err != nil {
		return useropsport.PlanMutation{}, unavailable(err)
	}
	id, err := q.InsertUserOpsLocalPlan(ctx, useropsdb.InsertUserOpsLocalPlanParams{State: string(input.State), ContentSnapshot: encoded, ContentDigest: contentDigest, TargetDigest: targetHash, TargetCount: int32(len(targets)), CreatedBy: input.ActorID})
	if err != nil || id < 1 {
		return useropsport.PlanMutation{}, unavailable(err)
	}
	status := domain.SendTechnicalStateDraft
	if input.State == domain.LocalPlanPendingReview {
		status = domain.SendTechnicalStatePendingReview
	}
	for _, target := range targets {
		if err = q.InsertUserOpsLocalPlanTarget(ctx, useropsdb.InsertUserOpsLocalPlanTargetParams{PlanID: id, CustomerID: int64(target)}); err != nil {
			return useropsport.PlanMutation{}, unavailable(err)
		}
		if err = q.InsertUserOpsSendRecord(ctx, useropsdb.InsertUserOpsSendRecordParams{PlanID: id, CustomerID: int64(target), TechnicalStatus: string(status)}); err != nil {
			return useropsport.PlanMutation{}, unavailable(err)
		}
	}
	plan, err := readPlan(ctx, q, domain.PlanID(id))
	if err != nil {
		return useropsport.PlanMutation{}, err
	}
	snapshot, _ := json.Marshal(plan)
	if err = complete(ctx, q, receipt.ID, snapshot); err != nil {
		return useropsport.PlanMutation{}, err
	}
	return useropsport.PlanMutation{PlanID: domain.PlanID(id)}, nil
}

func (r *Repository) ReplayLocalPlan(ctx context.Context, input useropsport.CreateLocalPlanInput, content domain.ContentSnapshot) (useropsport.PlanMutation, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return useropsport.PlanMutation{}, unavailable(err)
	}
	payload := planPayload(input, content)
	keyDigest, payloadDigest := sha256.Sum256([]byte(input.IdempotencyKey)), sha256.Sum256(payload)
	row, err := q.ReadUserOpsReceipt(ctx, useropsdb.ReadUserOpsReceiptParams{Operation: "local_plan_create", ActorScope: "user_ops:actor:" + strconv.FormatInt(input.ActorID, 10), KeyDigest: keyDigest[:]})
	if errors.Is(err, pgx.ErrNoRows) {
		return useropsport.PlanMutation{}, nil
	}
	if err != nil {
		return useropsport.PlanMutation{}, unavailable(err)
	}
	if string(row.PayloadDigest) != string(payloadDigest[:]) || row.State != "completed" || !json.Valid(row.ResultSnapshot) {
		return useropsport.PlanMutation{}, useropsport.ErrConflict
	}
	return replayPlan(receiptRow{ID: row.ID, PayloadDigest: row.PayloadDigest, State: row.State, ResultSnapshot: row.ResultSnapshot}, payload)
}

func (r *Repository) ReadLocalPlan(ctx context.Context, id domain.PlanID) (domain.LocalPlan, error) {
	q, err := queries(ctx)
	if r == nil || err != nil || !id.Valid() {
		return domain.LocalPlan{}, unavailable(err)
	}
	return readPlan(ctx, q, id)
}

func (r *Repository) ListSendRecords(ctx context.Context, input useropsport.SendRecordQuery) (useropsport.SendRecordPageRead, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return useropsport.SendRecordPageRead{}, unavailable(err)
	}
	before := int64(^uint64(0) >> 1)
	if input.Cursor != "" {
		before, err = strconv.ParseInt(input.Cursor, 10, 64)
		if err != nil || before < 1 {
			return useropsport.SendRecordPageRead{}, useropsport.ErrInvalid
		}
	}
	rows, err := q.ListUserOpsSendRecords(ctx, useropsdb.ListUserOpsSendRecordsParams{PlanID: int64(input.PlanID), BeforeID: before, RowLimit: input.Limit + 1})
	if err != nil {
		return useropsport.SendRecordPageRead{}, unavailable(err)
	}
	total, err := q.CountUserOpsSendRecords(ctx, int64(input.PlanID))
	if err != nil {
		return useropsport.SendRecordPageRead{}, unavailable(err)
	}
	page := useropsport.SendRecordPageRead{Total: total}
	if len(rows) > int(input.Limit) {
		next := strconv.FormatInt(rows[input.Limit-1].ID, 10)
		page.NextCursor = &next
		rows = rows[:input.Limit]
	}
	page.Items = make([]domain.SendRecord, len(rows))
	for i, row := range rows {
		if page.Items[i], err = sendRecord(row); err != nil {
			return useropsport.SendRecordPageRead{}, err
		}
	}
	return page, nil
}

type receiptRow struct {
	ID             int64
	PayloadDigest  []byte
	State          string
	ResultSnapshot []byte
}

func reserve(ctx context.Context, q *useropsdb.Queries, operation string, actorID int64, key string, payload []byte) (receiptRow, bool, error) {
	keyDigest := sha256.Sum256([]byte(key))
	payloadDigest := sha256.Sum256(payload)
	actor := "user_ops:actor:" + strconv.FormatInt(actorID, 10)
	row, err := q.ReserveUserOpsReceipt(ctx, useropsdb.ReserveUserOpsReceiptParams{Operation: operation, ActorScope: actor, KeyDigest: keyDigest[:], PayloadDigest: payloadDigest[:]})
	if err == nil {
		return receiptRow{ID: row.ID, PayloadDigest: row.PayloadDigest, State: row.State, ResultSnapshot: row.ResultSnapshot}, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return receiptRow{}, false, unavailable(err)
	}
	stored, err := q.ReadUserOpsReceipt(ctx, useropsdb.ReadUserOpsReceiptParams{Operation: operation, ActorScope: actor, KeyDigest: keyDigest[:]})
	if err != nil {
		return receiptRow{}, false, unavailable(err)
	}
	if string(stored.PayloadDigest) != string(payloadDigest[:]) || stored.State != "completed" || !json.Valid(stored.ResultSnapshot) {
		return receiptRow{}, false, useropsport.ErrConflict
	}
	return receiptRow{ID: stored.ID, PayloadDigest: stored.PayloadDigest, State: stored.State, ResultSnapshot: stored.ResultSnapshot}, false, nil
}

func complete(ctx context.Context, q *useropsdb.Queries, id int64, snapshot []byte) error {
	if !json.Valid(snapshot) {
		return useropsport.ErrUnavailable
	}
	_, err := q.CompleteUserOpsReceipt(ctx, useropsdb.CompleteUserOpsReceiptParams{ID: id, ResultSnapshot: snapshot})
	if errors.Is(err, pgx.ErrNoRows) {
		return useropsport.ErrConflict
	}
	if err != nil {
		return unavailable(err)
	}
	return nil
}

func replayDNDSet(row receiptRow, _ []byte) (useropsport.DNDMutation, error) {
	var d domain.DoNotDisturb
	if json.Unmarshal(row.ResultSnapshot, &d) != nil {
		return useropsport.DNDMutation{}, useropsport.ErrUnavailable
	}
	return useropsport.DNDMutation{Mutation: useropsport.Mutation{Replayed: true}, DND: &d}, nil
}
func replayDNDClear(row receiptRow, _ []byte) (useropsport.DNDMutation, error) {
	var v struct {
		Cleared bool `json:"cleared"`
	}
	if json.Unmarshal(row.ResultSnapshot, &v) != nil || !v.Cleared {
		return useropsport.DNDMutation{}, useropsport.ErrUnavailable
	}
	return useropsport.DNDMutation{Mutation: useropsport.Mutation{Replayed: true}, Cleared: true}, nil
}
func replayPlan(row receiptRow, _ []byte) (useropsport.PlanMutation, error) {
	var p domain.LocalPlan
	if json.Unmarshal(row.ResultSnapshot, &p) != nil || !p.ID.Valid() {
		return useropsport.PlanMutation{}, useropsport.ErrUnavailable
	}
	return useropsport.PlanMutation{Mutation: useropsport.Mutation{Replayed: true}, Plan: &p}, nil
}

func readPlan(ctx context.Context, q *useropsdb.Queries, id domain.PlanID) (domain.LocalPlan, error) {
	row, err := q.ReadUserOpsLocalPlan(ctx, int64(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LocalPlan{}, useropsport.ErrNotFound
	}
	if err != nil {
		return domain.LocalPlan{}, unavailable(err)
	}
	var content domain.ContentSnapshot
	if json.Unmarshal(row.ContentSnapshot, &content) != nil || !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return domain.LocalPlan{}, useropsport.ErrUnavailable
	}
	return domain.LocalPlan{ID: id, State: domain.LocalPlanState(row.State), Content: content, TargetDigest: hex.EncodeToString(row.TargetDigest), TargetCount: row.TargetCount, Version: row.Version, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}, nil
}
func dnd(row useropsdb.UserOpsDnd) (domain.DoNotDisturb, error) {
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return domain.DoNotDisturb{}, useropsport.ErrUnavailable
	}
	return domain.DoNotDisturb{CustomerID: domain.CustomerID(row.CustomerID), Reason: row.Reason, Version: row.Version, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}, nil
}
func sendRecord(row useropsdb.UserOpsSendRecord) (domain.SendRecord, error) {
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return domain.SendRecord{}, useropsport.ErrUnavailable
	}
	return domain.SendRecord{ID: domain.SendRecordID(row.ID), PlanID: domain.PlanID(row.PlanID), CustomerID: domain.CustomerID(row.CustomerID), TechnicalStatus: domain.SendTechnicalState(row.TechnicalStatus), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}, nil
}
func queries(ctx context.Context) (*useropsdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return useropsdb.New(tx), nil
}
func unavailable(err error) error {
	if err == nil {
		return useropsport.ErrUnavailable
	}
	return errors.Join(useropsport.ErrUnavailable, err)
}
func customerIDsInt64(values []domain.CustomerID) []int64 {
	out := make([]int64, len(values))
	for i, value := range values {
		out[i] = int64(value)
	}
	return out
}
func decodeDigest(value string) ([]byte, error) {
	result, err := hex.DecodeString(value)
	if err != nil || len(result) != sha256.Size {
		return nil, useropsport.ErrInvalid
	}
	return result, nil
}
func dndSetPayload(v useropsport.UpsertDNDInput) []byte {
	b, _ := json.Marshal(struct {
		CustomerID      domain.CustomerID `json:"customer_id"`
		Reason          string            `json:"reason"`
		ExpectedVersion *int64            `json:"expected_version,omitempty"`
	}{v.CustomerID, v.Reason, v.ExpectedVersion})
	return b
}
func dndClearPayload(v useropsport.ClearDNDInput) []byte {
	b, _ := json.Marshal(struct {
		CustomerID      domain.CustomerID `json:"customer_id"`
		ExpectedVersion int64             `json:"expected_version"`
	}{v.CustomerID, v.ExpectedVersion})
	return b
}
func planPayload(v useropsport.CreateLocalPlanInput, content domain.ContentSnapshot) []byte {
	b, _ := json.Marshal(struct {
		State                domain.LocalPlanState  `json:"state"`
		CustomerIDs          []domain.CustomerID    `json:"customer_ids"`
		ExpectedTargetDigest string                 `json:"expected_target_digest"`
		Content              domain.ContentSnapshot `json:"content"`
	}{v.State, v.CustomerIDs, v.ExpectedTargetDigest, content})
	return b
}
