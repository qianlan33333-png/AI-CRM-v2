package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

type Repository struct{}

var _ memberport.Store = (*Repository)(nil)

func NewRepository() *Repository { return &Repository{} }

func (*Repository) ServiceProductExists(ctx context.Context, productID int64) (bool, error) {
	q, err := queries(ctx)
	if err != nil || productID < 1 {
		return false, invalidOrUnavailable(err, productID)
	}
	exists, err := q.ServicePeriodProductExists(ctx, productID)
	return exists, classify(err)
}

func (*Repository) LockServiceProductForMemberAdd(ctx context.Context, productID int64) (bool, error) {
	q, err := queries(ctx)
	if err != nil || productID < 1 {
		return false, invalidOrUnavailable(err, productID)
	}
	accepted, err := q.LockServiceProductForMemberAdd(ctx, productID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return accepted, classify(err)
}

func (*Repository) CustomerExists(ctx context.Context, customerID int64) (bool, error) {
	q, err := queries(ctx)
	if err != nil || customerID < 1 {
		return false, invalidOrUnavailable(err, customerID)
	}
	exists, err := q.ServicePeriodMemberCustomerExists(ctx, customerID)
	return exists, classify(err)
}

func (*Repository) Get(ctx context.Context, productID int64, memberRef string) (memberdomain.Member, error) {
	if productID < 1 || !memberdomain.ValidMemberRef(memberRef) {
		return memberdomain.Member{}, memberport.ErrInvalidInput
	}
	q, err := queries(ctx)
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	row, err := q.GetServicePeriodMember(ctx, productdb.GetServicePeriodMemberParams{ProductID: productID, MemberRef: memberRef})
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	return mapMember(row.MemberRef, row.ServiceProductID, row.CustomerID, row.State, row.Source, row.StartsAt,
		row.ExpiresAt, row.ExpiredAt, row.RemovedAt, row.Remark, row.Alliance, row.Version, row.CreatedAt, row.UpdatedAt)
}

func (*Repository) GetForUpdate(ctx context.Context, productID int64, memberRef string) (memberdomain.Member, error) {
	if productID < 1 || !memberdomain.ValidMemberRef(memberRef) {
		return memberdomain.Member{}, memberport.ErrInvalidInput
	}
	q, err := queries(ctx)
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	row, err := q.LockServicePeriodMember(ctx, productdb.LockServicePeriodMemberParams{ProductID: productID, MemberRef: memberRef})
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	return mapMember(row.MemberRef, row.ServiceProductID, row.CustomerID, row.State, row.Source, row.StartsAt,
		row.ExpiresAt, row.ExpiredAt, row.RemovedAt, row.Remark, row.Alliance, row.Version, row.CreatedAt, row.UpdatedAt)
}

func (*Repository) Create(ctx context.Context, record memberport.CreateRecord) (memberdomain.Member, error) {
	q, err := queries(ctx)
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	row, err := q.CreateServicePeriodMember(ctx, productdb.CreateServicePeriodMemberParams{
		MemberRef: record.MemberRef, ProductID: record.ServiceProductID, CustomerID: record.CustomerID,
		Source: string(record.Source), StartsAt: timestamp(record.StartsAt), ExpiresAt: optionalTimestamp(record.ExpiresAt),
		Remark: optionalText(record.Remark), Alliance: optionalText(record.Alliance), CreatedAt: timestamp(record.CreatedAt),
	})
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	return mapMember(row.MemberRef, row.ServiceProductID, row.CustomerID, row.State, row.Source, row.StartsAt,
		row.ExpiresAt, row.ExpiredAt, row.RemovedAt, row.Remark, row.Alliance, row.Version, row.CreatedAt, row.UpdatedAt)
}

func (*Repository) Transition(ctx context.Context, record memberport.TransitionRecord) (memberdomain.Member, error) {
	q, err := queries(ctx)
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	row, err := q.TransitionServicePeriodMember(ctx, productdb.TransitionServicePeriodMemberParams{
		ProductID: record.ServiceProductID, MemberRef: record.MemberRef, ExpectedVersion: record.ExpectedVersion,
		TargetState: string(record.Target), TransitionedAt: timestamp(record.TransitionedAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return memberdomain.Member{}, memberport.ErrConflict
	}
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	return mapMember(row.MemberRef, row.ServiceProductID, row.CustomerID, row.State, row.Source, row.StartsAt,
		row.ExpiresAt, row.ExpiredAt, row.RemovedAt, row.Remark, row.Alliance, row.Version, row.CreatedAt, row.UpdatedAt)
}

func (*Repository) UpdateFields(ctx context.Context, record memberport.UpdateFieldsRecord) (memberdomain.Member, error) {
	q, err := queries(ctx)
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	row, err := q.UpdateServicePeriodMemberFields(ctx, productdb.UpdateServicePeriodMemberFieldsParams{
		ProductID: record.ServiceProductID, MemberRef: record.MemberRef, ExpectedVersion: record.ExpectedVersion,
		Remark: optionalText(record.Remark), Alliance: optionalText(record.Alliance), UpdatedAt: timestamp(record.UpdatedAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return memberdomain.Member{}, memberport.ErrConflict
	}
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	return mapMember(row.MemberRef, row.ServiceProductID, row.CustomerID, row.State, row.Source, row.StartsAt,
		row.ExpiresAt, row.ExpiredAt, row.RemovedAt, row.Remark, row.Alliance, row.Version, row.CreatedAt, row.UpdatedAt)
}

func (*Repository) List(ctx context.Context, query memberport.StoreListQuery) ([]memberdomain.Member, error) {
	q, err := queries(ctx)
	if err != nil {
		return nil, classify(err)
	}
	params := productdb.ListServicePeriodMembersParams{ProductID: query.ServiceProductID, RowLimit: int32(query.Limit)}
	if query.State != nil {
		params.State = pgtype.Text{String: string(*query.State), Valid: true}
	}
	if query.Source != nil {
		params.Source = pgtype.Text{String: string(*query.Source), Valid: true}
	}
	if query.After != nil {
		params.AfterUpdatedAt = timestamp(query.After.UpdatedAt)
		params.AfterMemberRef = pgtype.Text{String: query.After.MemberRef, Valid: true}
	}
	rows, err := q.ListServicePeriodMembers(ctx, params)
	if err != nil {
		return nil, classify(err)
	}
	result := make([]memberdomain.Member, len(rows))
	for index, row := range rows {
		result[index], err = mapMember(row.MemberRef, row.ServiceProductID, row.CustomerID, row.State, row.Source, row.StartsAt,
			row.ExpiresAt, row.ExpiredAt, row.RemovedAt, row.Remark, row.Alliance, row.Version, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (*Repository) ListCustomer(ctx context.Context, query memberport.CustomerListQuery) ([]memberdomain.Member, error) {
	q, err := queries(ctx)
	if err != nil || query.CustomerID < 1 || query.Limit < 1 || query.Offset < 0 {
		return nil, classify(err)
	}
	rows, err := q.ListServicePeriodMembersByCustomer(ctx, productdb.ListServicePeriodMembersByCustomerParams{
		CustomerID: query.CustomerID, RowLimit: int32(query.Limit), RowOffset: int32(query.Offset),
	})
	if err != nil {
		return nil, classify(err)
	}
	result := make([]memberdomain.Member, len(rows))
	for index, row := range rows {
		result[index], err = mapMember(row.MemberRef, row.ServiceProductID, row.CustomerID, row.State, row.Source, row.StartsAt,
			row.ExpiresAt, row.ExpiredAt, row.RemovedAt, row.Remark, row.Alliance, row.Version, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (*Repository) ReserveReceipt(ctx context.Context, reservation memberport.ReceiptReservation) (memberport.Receipt, bool, error) {
	q, err := queries(ctx)
	if err != nil {
		return memberport.Receipt{}, false, classify(err)
	}
	row, err := q.ReserveServicePeriodMemberReceipt(ctx, productdb.ReserveServicePeriodMemberReceiptParams{
		Operation: reservation.Operation, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:],
		PayloadDigest: reservation.PayloadDigest[:], CreatedAt: timestamp(reservation.CreatedAt),
	})
	if err == nil {
		return mapReceipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot, row.CreatedAt, reservation), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return memberport.Receipt{}, false, classify(err)
	}
	existing, err := q.GetServicePeriodMemberReceipt(ctx, productdb.GetServicePeriodMemberReceiptParams{
		Operation: reservation.Operation, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:],
	})
	if err != nil {
		return memberport.Receipt{}, false, classify(err)
	}
	return mapReceipt(existing.ID, existing.Operation, existing.ActorScope, existing.KeyDigest, existing.PayloadDigest,
		existing.State, existing.ResultSnapshot, existing.CreatedAt, reservation), false, nil
}

func (*Repository) CompleteReceipt(ctx context.Context, receiptID int64, snapshot json.RawMessage, completedAt time.Time) (memberport.Receipt, error) {
	q, err := queries(ctx)
	if err != nil {
		return memberport.Receipt{}, classify(err)
	}
	row, err := q.CompleteServicePeriodMemberReceipt(ctx, productdb.CompleteServicePeriodMemberReceiptParams{
		ReceiptID: receiptID, ResultSnapshot: snapshot, CompletedAt: timestamp(completedAt),
	})
	if err != nil {
		return memberport.Receipt{}, classify(err)
	}
	reservation := memberport.ReceiptReservation{Operation: row.Operation, ActorScope: row.ActorScope}
	return mapReceipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot, row.CreatedAt, reservation), nil
}

func queries(ctx context.Context) (*productdb.Queries, error) {
	if ctx == nil {
		return nil, memberport.ErrUnavailable
	}
	transaction, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return productdb.New(transaction), nil
}

func mapMember(memberRef string, productID, customerID int64, state, source string, startsAt, expiresAt, expiredAt, removedAt pgtype.Timestamptz, remark, alliance pgtype.Text, version int64, createdAt, updatedAt pgtype.Timestamptz) (memberdomain.Member, error) {
	if !startsAt.Valid || !createdAt.Valid || !updatedAt.Valid {
		return memberdomain.Member{}, memberport.ErrUnavailable
	}
	return memberdomain.Member{
		MemberRef: memberRef, ServiceProductID: productID, CustomerID: customerID,
		State: memberdomain.State(state), Source: memberdomain.Source(source), StartsAt: startsAt.Time.UTC(),
		ExpiresAt: optionalTime(expiresAt), ExpiredAt: optionalTime(expiredAt), RemovedAt: optionalTime(removedAt),
		Remark: optionalString(remark), Alliance: optionalString(alliance), Version: version,
		CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC(),
	}, nil
}

func mapReceipt(id int64, operation, actorScope string, keyDigest, payloadDigest []byte, state string, snapshot []byte, createdAt pgtype.Timestamptz, reservation memberport.ReceiptReservation) memberport.Receipt {
	receipt := memberport.Receipt{ID: id, ReceiptReservation: reservation, State: state, ResultSnapshot: append(json.RawMessage(nil), snapshot...)}
	receipt.Operation, receipt.ActorScope = operation, actorScope
	if len(keyDigest) == 32 {
		copy(receipt.KeyDigest[:], keyDigest)
	}
	if len(payloadDigest) == 32 {
		copy(receipt.PayloadDigest[:], payloadDigest)
	}
	if createdAt.Valid {
		receipt.CreatedAt = createdAt.Time.UTC()
	}
	return receipt
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: !value.IsZero()}
}

func optionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*value)
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func invalidOrUnavailable(err error, id int64) error {
	if id < 1 {
		return memberport.ErrInvalidInput
	}
	return classify(err)
}

func classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return memberport.ErrNotFound
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23503":
			return memberport.ErrNotFound
		case "23505":
			return memberport.ErrConflict
		}
	}
	return errors.Join(memberport.ErrUnavailable, err)
}
