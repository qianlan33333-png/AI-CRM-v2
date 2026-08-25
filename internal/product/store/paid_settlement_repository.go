package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

type PaidSettlementRepository struct{}

var _ productapp.PaidSettlementStore = (*PaidSettlementRepository)(nil)

func NewPaidSettlementRepository() *PaidSettlementRepository { return &PaidSettlementRepository{} }

func (*PaidSettlementRepository) LockPaidProduct(ctx context.Context, id productport.ID, version int64, kind productport.PaidSettlementProductKind) (productapp.PaidProductSnapshot, error) {
	q, err := queries(ctx)
	if err != nil {
		return productapp.PaidProductSnapshot{}, paidStoreError(err)
	}
	row, err := q.LockProductForPaidSettlement(ctx, productdb.LockProductForPaidSettlementParams{ProductID: int64(id), ProductVersion: version, ProductKind: string(kind)})
	if err != nil {
		return productapp.PaidProductSnapshot{}, paidStoreError(err)
	}
	return productapp.PaidProductSnapshot{ID: row.ID, Version: row.Version, Kind: productport.PaidSettlementProductKind(row.ProductKind)}, nil
}

func (*PaidSettlementRepository) CreatePaidEntitlement(ctx context.Context, command productport.PaidSettlementCommand) (productapp.PaidEntitlementRecord, error) {
	q, err := queries(ctx)
	if err != nil {
		return productapp.PaidEntitlementRecord{}, paidStoreError(err)
	}
	row, err := q.CreatePaidOrderEntitlement(ctx, productdb.CreatePaidOrderEntitlementParams{ProductID: int64(command.ProductID), OrderID: command.OrderID, CustomerID: command.CustomerID, GrantedAt: stamp(command.OccurredAt), SettlementReceiptDigest: command.SettlementReceiptDigest[:]})
	if err != nil {
		return productapp.PaidEntitlementRecord{}, paidStoreError(err)
	}
	return paidEntitlement(row.ID, row.ProductID, row.OrderID, row.CustomerID, row.State, row.Version, command.SettlementReceiptDigest, row.GrantedAt, row.RevokedAt), nil
}

func (*PaidSettlementRepository) LockPaidEntitlement(ctx context.Context, orderID int64) (productapp.PaidEntitlementRecord, error) {
	q, err := queries(ctx)
	if err != nil {
		return productapp.PaidEntitlementRecord{}, paidStoreError(err)
	}
	row, err := q.LockPaidOrderEntitlement(ctx, orderID)
	if err != nil {
		return productapp.PaidEntitlementRecord{}, paidStoreError(err)
	}
	return paidEntitlement(row.ID, row.ProductID, row.OrderID, row.CustomerID, row.State, row.Version, digest32(row.SettlementReceiptDigest), row.GrantedAt, row.RevokedAt), nil
}

func (*PaidSettlementRepository) RevokePaidEntitlement(ctx context.Context, orderID, version int64, digest [32]byte, at time.Time) (productapp.PaidEntitlementRecord, error) {
	q, err := queries(ctx)
	if err != nil {
		return productapp.PaidEntitlementRecord{}, paidStoreError(err)
	}
	row, err := q.RevokePaidOrderEntitlement(ctx, productdb.RevokePaidOrderEntitlementParams{OrderID: orderID, ExpectedVersion: version, SettlementReceiptDigest: digest[:], RevokedAt: stamp(at)})
	if err != nil {
		return productapp.PaidEntitlementRecord{}, paidStoreError(err)
	}
	return paidEntitlement(row.ID, row.ProductID, row.OrderID, row.CustomerID, row.State, row.Version, digest, row.GrantedAt, row.RevokedAt), nil
}

func (*PaidSettlementRepository) CreatePaidMember(ctx context.Context, memberRef string, command productport.PaidSettlementCommand, entitlementID productport.EntitlementID) (productapp.PaidMemberRecord, error) {
	q, err := queries(ctx)
	if err != nil {
		return productapp.PaidMemberRecord{}, paidStoreError(err)
	}
	row, err := q.CreatePaidOrderServicePeriodMember(ctx, productdb.CreatePaidOrderServicePeriodMemberParams{MemberRef: memberRef, ProductID: int64(command.ProductID), CustomerID: command.CustomerID, StartsAt: stamp(command.OccurredAt), OrderID: pgtype.Int8{Int64: command.OrderID, Valid: true}, EntitlementID: pgtype.Int8{Int64: int64(entitlementID), Valid: true}, SettlementReceiptDigest: command.SettlementReceiptDigest[:]})
	if err != nil {
		return productapp.PaidMemberRecord{}, paidStoreError(err)
	}
	return paidMember(row.MemberRef, row.ServiceProductID, row.CustomerID, row.State, row.Version, row.CreatedAt, row.UpdatedAt), nil
}

func (*PaidSettlementRepository) LockPaidMember(ctx context.Context, orderID int64) (productapp.PaidMemberRecord, error) {
	q, err := queries(ctx)
	if err != nil {
		return productapp.PaidMemberRecord{}, paidStoreError(err)
	}
	row, err := q.LockPaidOrderServicePeriodMember(ctx, pgtype.Int8{Int64: orderID, Valid: true})
	if err != nil {
		return productapp.PaidMemberRecord{}, paidStoreError(err)
	}
	return paidMember(row.MemberRef, row.ServiceProductID, row.CustomerID, row.State, row.Version, row.CreatedAt, row.UpdatedAt), nil
}

func (*PaidSettlementRepository) RemovePaidMember(ctx context.Context, orderID, version int64, digest [32]byte, at time.Time) (productapp.PaidMemberRecord, error) {
	q, err := queries(ctx)
	if err != nil {
		return productapp.PaidMemberRecord{}, paidStoreError(err)
	}
	row, err := q.RemovePaidOrderServicePeriodMember(ctx, productdb.RemovePaidOrderServicePeriodMemberParams{OrderID: pgtype.Int8{Int64: orderID, Valid: true}, ExpectedVersion: version, SettlementReceiptDigest: digest[:], RemovedAt: stamp(at)})
	if err != nil {
		return productapp.PaidMemberRecord{}, paidStoreError(err)
	}
	return paidMember(row.MemberRef, row.ServiceProductID, row.CustomerID, row.State, row.Version, row.CreatedAt, row.UpdatedAt), nil
}

func paidEntitlement(id, productID, orderID, customerID int64, state string, version int64, digest [32]byte, granted, revoked pgtype.Timestamptz) productapp.PaidEntitlementRecord {
	return productapp.PaidEntitlementRecord{ID: productport.EntitlementID(id), ProductID: productport.ID(productID), OrderID: orderID, CustomerID: customerID, State: state, Version: version, Digest: digest, GrantedAt: granted.Time.UTC(), RevokedAt: optionalPaidTime(revoked)}
}

func paidMember(ref string, productID, customerID int64, state string, version int64, created, updated pgtype.Timestamptz) productapp.PaidMemberRecord {
	return productapp.PaidMemberRecord{MemberRef: ref, ProductID: productID, CustomerID: customerID, State: state, Version: version, CreatedAt: created.Time.UTC(), UpdatedAt: updated.Time.UTC()}
}

func digest32(value []byte) [32]byte {
	var result [32]byte
	copy(result[:], value)
	return result
}

func optionalPaidTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func paidStoreError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return productapp.ErrPaidSettlementRowNotFound()
	}
	return errors.Join(productport.ErrPaidSettlementUnavailable, err)
}
