package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

type SidebarProfileRepository struct{}

func NewSidebarProfileRepository() *SidebarProfileRepository { return &SidebarProfileRepository{} }

func (repository *SidebarProfileRepository) ReadSidebarProfile(ctx context.Context, customerID, ownerStaffID int64) (contactapp.SidebarProfileRecord, error) {
	if repository == nil || customerID < 1 || ownerStaffID < 0 {
		return contactapp.SidebarProfileRecord{}, contactport.ErrSidebarProfileInvalid
	}
	queries, err := customerMutationQueriesFromContext(ctx)
	if err != nil {
		return contactapp.SidebarProfileRecord{}, contactport.ErrSidebarProfileUnavailable
	}
	row, err := queries.GetSidebarCustomerProfile(ctx, contactdb.GetSidebarCustomerProfileParams{CustomerID: customerID, OwnerStaffID: ownerStaffID})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.SidebarProfileRecord{}, contactport.ErrSidebarProfileNotFound
	}
	if err != nil {
		return contactapp.SidebarProfileRecord{}, contactport.ErrSidebarProfileUnavailable
	}
	return sidebarProfileRecord(row.ID, row.Name, row.OwnerStaffID, row.Extra, row.UpdatedAt)
}

func (repository *SidebarProfileRepository) UpdateSidebarProfile(ctx context.Context, record contactapp.SidebarProfileRecord, expectedUpdatedAt time.Time) (contactapp.SidebarProfileRecord, error) {
	if repository == nil || record.CustomerID < 1 || record.OwnerStaffID < 1 || !json.Valid(record.Extra) || record.UpdatedAt.IsZero() || expectedUpdatedAt.IsZero() {
		return contactapp.SidebarProfileRecord{}, contactport.ErrSidebarProfileInvalid
	}
	queries, err := customerMutationQueriesFromContext(ctx)
	if err != nil {
		return contactapp.SidebarProfileRecord{}, contactport.ErrSidebarProfileUnavailable
	}
	row, err := queries.UpdateSidebarCustomerProfile(ctx, contactdb.UpdateSidebarCustomerProfileParams{
		Profile: record.Extra, UpdatedAt: sidebarProfileTimestamp(record.UpdatedAt), CustomerID: record.CustomerID,
		OwnerStaffID: record.OwnerStaffID, ExpectedUpdatedAt: sidebarProfileTimestamp(expectedUpdatedAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.SidebarProfileRecord{}, contactport.ErrSidebarProfileConflict
	}
	if err != nil {
		return contactapp.SidebarProfileRecord{}, contactport.ErrSidebarProfileUnavailable
	}
	return sidebarProfileRecord(row.ID, row.Name, row.OwnerStaffID, row.Extra, row.UpdatedAt)
}

func (repository *SidebarProfileRepository) ReserveSidebarProfileReceipt(ctx context.Context, reservation contactapp.SidebarProfileReceiptReservation) (contactapp.SidebarProfileReceipt, bool, error) {
	queries, err := customerMutationQueriesFromContext(ctx)
	if repository == nil || err != nil || reservation.ActorScope == "" || reservation.CreatedAt.IsZero() {
		return contactapp.SidebarProfileReceipt{}, false, contactport.ErrSidebarProfileUnavailable
	}
	row, err := queries.ReserveSidebarCustomerProfileReceipt(ctx, contactdb.ReserveSidebarCustomerProfileReceiptParams{
		ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:], CreatedAt: sidebarProfileTimestamp(reservation.CreatedAt),
	})
	if err == nil {
		return sidebarProfileReceipt(row.ID, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return contactapp.SidebarProfileReceipt{}, false, contactport.ErrSidebarProfileUnavailable
	}
	existing, err := queries.GetSidebarCustomerProfileReceipt(ctx, contactdb.GetSidebarCustomerProfileReceiptParams{ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:]})
	if err != nil {
		return contactapp.SidebarProfileReceipt{}, false, contactport.ErrSidebarProfileUnavailable
	}
	return sidebarProfileReceipt(existing.ID, existing.ActorScope, existing.KeyDigest, existing.PayloadDigest, existing.State, existing.ResultSnapshot), false, nil
}

func (repository *SidebarProfileRepository) CompleteSidebarProfileReceipt(ctx context.Context, receiptID int64, snapshot json.RawMessage, completedAt time.Time) (contactapp.SidebarProfileReceipt, error) {
	queries, err := customerMutationQueriesFromContext(ctx)
	if repository == nil || err != nil || receiptID < 1 || !json.Valid(snapshot) || completedAt.IsZero() {
		return contactapp.SidebarProfileReceipt{}, contactport.ErrSidebarProfileUnavailable
	}
	row, err := queries.CompleteSidebarCustomerProfileReceipt(ctx, contactdb.CompleteSidebarCustomerProfileReceiptParams{
		ResultSnapshot: snapshot, CompletedAt: sidebarProfileTimestamp(completedAt), ID: receiptID,
	})
	if err != nil {
		return contactapp.SidebarProfileReceipt{}, contactport.ErrSidebarProfileUnavailable
	}
	return sidebarProfileReceipt(row.ID, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}

func sidebarProfileRecord(id int64, name string, owner int64, extra []byte, updatedAt pgtype.Timestamptz) (contactapp.SidebarProfileRecord, error) {
	if id < 1 || name == "" || owner < 1 || !json.Valid(extra) || !updatedAt.Valid || updatedAt.Time.IsZero() {
		return contactapp.SidebarProfileRecord{}, contactport.ErrSidebarProfileUnavailable
	}
	return contactapp.SidebarProfileRecord{CustomerID: id, OwnerStaffID: owner, Name: name, Extra: append([]byte(nil), extra...), UpdatedAt: updatedAt.Time.UTC()}, nil
}

func sidebarProfileReceipt(id int64, actor string, key, payload []byte, state string, snapshot []byte) contactapp.SidebarProfileReceipt {
	var result contactapp.SidebarProfileReceipt
	result.ID, result.ActorScope, result.State = id, actor, state
	copy(result.KeyDigest[:], key)
	copy(result.PayloadDigest[:], payload)
	result.ResultSnapshot = append([]byte(nil), snapshot...)
	return result
}

func sidebarProfileTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: !value.IsZero()}
}

var _ contactapp.SidebarProfileStore = (*SidebarProfileRepository)(nil)
