package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// HistoricalImportRepository owns the narrow Contact-side DM01 writes. It is
// transaction-bound and intentionally has no role, Provider, or event method.
type HistoricalImportRepository struct{}

var _ contactport.HistoricalImportTarget = HistoricalImportRepository{}

func (HistoricalImportRepository) UpsertStaff(ctx context.Context, userID, name string, active bool, createdAt, updatedAt time.Time) (int64, error) {
	if strings.TrimSpace(userID) != userID || userID == "" || strings.TrimSpace(name) != name || name == "" || createdAt.IsZero() || updatedAt.IsZero() || createdAt.After(updatedAt) {
		return 0, ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	return contactdb.New(tx).InsertHistoricalImportStaff(ctx, contactdb.InsertHistoricalImportStaffParams{WecomUserid: userID, Name: name, IsActive: active, CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true}})
}

func (HistoricalImportRepository) CreateCustomer(ctx context.Context, name string, avatarURL *string, gender *int16, ownerStaffID *int64, firstSeenAt, lastSeenAt, createdAt, updatedAt time.Time) (int64, error) {
	if firstSeenAt.IsZero() || lastSeenAt.IsZero() || createdAt.IsZero() || updatedAt.IsZero() || createdAt.After(updatedAt) || firstSeenAt.After(lastSeenAt) {
		return 0, ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	avatar := pgtype.Text{}
	if avatarURL != nil && *avatarURL != "" {
		avatar = pgtype.Text{String: *avatarURL, Valid: true}
	}
	genderValue := pgtype.Int2{}
	if gender != nil {
		genderValue = pgtype.Int2{Int16: *gender, Valid: true}
	}
	owner := pgtype.Int8{}
	if ownerStaffID != nil {
		owner = pgtype.Int8{Int64: *ownerStaffID, Valid: true}
	}
	return contactdb.New(tx).CreateHistoricalImportCustomer(ctx, contactdb.CreateHistoricalImportCustomerParams{Name: name, AvatarUrl: avatar, Gender: genderValue, OwnerStaffID: owner, FirstSeenAt: pgtype.Timestamptz{Time: firstSeenAt, Valid: true}, LastSeenAt: pgtype.Timestamptz{Time: lastSeenAt, Valid: true}, CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true}})
}

func (HistoricalImportRepository) CreateAndMapCustomer(ctx context.Context, runID int64, sourceKeyHMAC, payloadHMAC []byte, name string, avatarURL *string, gender *int16, ownerStaffID *int64, firstSeenAt, lastSeenAt, createdAt, updatedAt time.Time) (int64, error) {
	if runID < 1 || len(sourceKeyHMAC) != 32 || len(payloadHMAC) != 32 {
		return 0, ErrInvalidHistoricalImport
	}
	customerID, err := (HistoricalImportRepository{}).CreateCustomer(ctx, name, avatarURL, gender, ownerStaffID, firstSeenAt, lastSeenAt, createdAt, updatedAt)
	if err != nil {
		return 0, err
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	if err := contactdb.New(tx).InsertHistoricalImportCustomerMapping(ctx, contactdb.InsertHistoricalImportCustomerMappingParams{SourceKeyHmac: sourceKeyHMAC, CustomerID: customerID, RunID: runID, PayloadHmac: payloadHMAC}); err != nil {
		return 0, err
	}
	return customerID, nil
}

func (HistoricalImportRepository) MapScopedIdentity(ctx context.Context, runID, identityID int64, sourceKeyHMAC, payloadHMAC []byte) error {
	if runID < 1 || identityID < 1 || len(sourceKeyHMAC) != 32 || len(payloadHMAC) != 32 {
		return ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	return contactdb.New(tx).InsertHistoricalImportIdentityMapping(ctx, contactdb.InsertHistoricalImportIdentityMappingParams{SourceKeyHmac: sourceKeyHMAC, IdentityID: identityID, RunID: runID, PayloadHmac: payloadHMAC})
}

var ErrInvalidHistoricalImport = historicalImportError("invalid DM01 historical import")
var ErrHistoricalImportTargetDrift = historicalImportError("DM01 historical import target drift")

type historicalImportError string

func (e historicalImportError) Error() string { return string(e) }

func (HistoricalImportRepository) LockMatchingExistingStaff(ctx context.Context, userID, name string, active bool, createdAt, updatedAt time.Time) (int64, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	row, err := contactdb.New(tx).LockHistoricalImportStaffForMatch(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if row.Name != name || row.IsActive != active || !row.CreatedAt.Valid || !row.UpdatedAt.Valid || !row.CreatedAt.Time.Equal(createdAt) || !row.UpdatedAt.Time.Equal(updatedAt) {
		return 0, ErrHistoricalImportTargetDrift
	}
	return row.ID, nil
}

func (HistoricalImportRepository) LockHistoricalImportSource(ctx context.Context, source contactport.HistoricalImportSource, sourceKeyHMAC []byte) error {
	sourceTable, ok := historicalImportSourceTable(source)
	if !ok || len(sourceKeyHMAC) != 32 {
		return ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	return contactdb.New(tx).LockHistoricalImportSource(ctx, contactdb.LockHistoricalImportSourceParams{SourceTable: sourceTable, SourceKeyHmac: sourceKeyHMAC})
}

func (HistoricalImportRepository) FindHistoricalImportRowReceipt(ctx context.Context, runID int64, source contactport.HistoricalImportSource, sourceKeyHMAC []byte) (contactport.HistoricalImportRowReceipt, bool, error) {
	sourceTable, ok := historicalImportSourceTable(source)
	if !ok || runID < 1 || len(sourceKeyHMAC) != 32 {
		return contactport.HistoricalImportRowReceipt{}, false, ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactport.HistoricalImportRowReceipt{}, false, err
	}
	row, err := contactdb.New(tx).FindHistoricalImportRowReceipt(ctx, contactdb.FindHistoricalImportRowReceiptParams{RunID: runID, SourceTable: sourceTable, SourceKeyHmac: sourceKeyHMAC})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.HistoricalImportRowReceipt{}, false, nil
	}
	if err != nil {
		return contactport.HistoricalImportRowReceipt{}, false, err
	}
	receipt := contactport.HistoricalImportRowReceipt{PayloadHMAC: row.PayloadHmac, FieldDigest: row.FieldDigest}
	switch row.Disposition {
	case "imported":
		receipt.Disposition = contactport.HistoricalImportImported
	case "quarantined":
		receipt.Disposition = contactport.HistoricalImportQuarantined
	default:
		return contactport.HistoricalImportRowReceipt{}, false, ErrHistoricalImportTargetDrift
	}
	return receipt, true, nil
}

func (HistoricalImportRepository) LockHistoricalImportLineage(ctx context.Context, source contactport.HistoricalImportSource, sourceKeyHMAC []byte) (contactport.HistoricalImportLineage, bool, error) {
	sourceTable, ok := historicalImportSourceTable(source)
	if !ok || len(sourceKeyHMAC) != 32 {
		return contactport.HistoricalImportLineage{}, false, ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactport.HistoricalImportLineage{}, false, err
	}
	row, err := contactdb.New(tx).LockHistoricalImportLineage(ctx, contactdb.LockHistoricalImportLineageParams{SourceTable: sourceTable, SourceKeyHmac: sourceKeyHMAC})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.HistoricalImportLineage{}, false, nil
	}
	if err != nil {
		return contactport.HistoricalImportLineage{}, false, err
	}
	targetID := int64(0)
	switch source {
	case contactport.HistoricalImportOwnerRoleMap:
		if row.StaffID.Valid {
			targetID = row.StaffID.Int64
		}
	case contactport.HistoricalImportCustomerIdentity:
		if row.CustomerID.Valid {
			targetID = row.CustomerID.Int64
		}
	case contactport.HistoricalImportExternalIdentity:
		if row.IdentityID.Valid {
			targetID = row.IdentityID.Int64
		}
	}
	if targetID < 1 || len(row.PayloadHmac) != 32 {
		return contactport.HistoricalImportLineage{}, false, ErrHistoricalImportTargetDrift
	}
	return contactport.HistoricalImportLineage{TargetID: targetID, PayloadHMAC: row.PayloadHmac}, true, nil
}

func (repository HistoricalImportRepository) EnsureHistoricalImportStaff(ctx context.Context, fact contactport.HistoricalImportStaffFact) (int64, error) {
	id, err := repository.UpsertStaff(ctx, fact.WeComUserID, fact.Name, fact.Active, fact.CreatedAt, fact.UpdatedAt)
	if err == nil {
		if id < 1 {
			return 0, ErrHistoricalImportTargetDrift
		}
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	return repository.LockMatchingExistingStaff(ctx, fact.WeComUserID, fact.Name, fact.Active, fact.CreatedAt, fact.UpdatedAt)
}

func (repository HistoricalImportRepository) CreateHistoricalImportCustomer(ctx context.Context, fact contactport.HistoricalImportCustomerFact) (int64, error) {
	return repository.CreateCustomer(ctx, fact.Name, fact.AvatarURL, fact.Gender, fact.OwnerStaffID, fact.FirstSeenAt, fact.LastSeenAt, fact.CreatedAt, fact.UpdatedAt)
}

func (repository HistoricalImportRepository) ValidateHistoricalImportStaff(ctx context.Context, staffID int64, fact contactport.HistoricalImportStaffFact) error {
	matchedID, err := repository.LockMatchingExistingStaff(ctx, fact.WeComUserID, fact.Name, fact.Active, fact.CreatedAt, fact.UpdatedAt)
	if err != nil {
		return err
	}
	if matchedID != staffID {
		return ErrHistoricalImportTargetDrift
	}
	return nil
}

func (HistoricalImportRepository) ValidateHistoricalImportCustomer(ctx context.Context, customerID int64, fact contactport.HistoricalImportCustomerFact) error {
	if customerID < 1 {
		return ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	row, err := contactdb.New(tx).LockHistoricalImportCustomerForMatch(ctx, customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrHistoricalImportTargetDrift
	}
	if err != nil {
		return err
	}
	if row.Name != fact.Name || !sameOptionalString(row.AvatarUrl, fact.AvatarURL) || !sameOptionalInt16(row.Gender, fact.Gender) || !sameOptionalInt64(row.OwnerStaffID, fact.OwnerStaffID) || !sameTime(row.AddedAt, fact.FirstSeenAt) || !sameTime(row.LastInteractAt, fact.LastSeenAt) || !sameTime(row.CreatedAt, fact.CreatedAt) || !sameTime(row.UpdatedAt, fact.UpdatedAt) {
		return ErrHistoricalImportTargetDrift
	}
	return nil
}

func (HistoricalImportRepository) IsHistoricalImportActiveStaff(ctx context.Context, staffID int64) (bool, error) {
	if staffID < 1 {
		return false, ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return false, err
	}
	active, err := contactdb.New(tx).IsHistoricalImportActiveStaff(ctx, staffID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return active, err
}

func (HistoricalImportRepository) ValidateHistoricalImportCustomerRoot(ctx context.Context, customerID int64) error {
	if customerID < 1 {
		return ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	found, err := contactdb.New(tx).LockHistoricalImportCustomerRoot(ctx, customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrHistoricalImportTargetDrift
	}
	if err != nil {
		return err
	}
	if !found {
		return ErrHistoricalImportTargetDrift
	}
	return nil
}

func (HistoricalImportRepository) AppendHistoricalImportLineage(ctx context.Context, runID int64, source contactport.HistoricalImportSource, fact contactport.HistoricalImportSourceFact, targetID int64) error {
	sourceTable, ok := historicalImportSourceTable(source)
	if !ok || runID < 1 || targetID < 1 || !validHistoricalSourceFact(fact) {
		return ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	var staffID, customerID, identityID pgtype.Int8
	switch source {
	case contactport.HistoricalImportOwnerRoleMap:
		staffID = pgtype.Int8{Int64: targetID, Valid: true}
	case contactport.HistoricalImportCustomerIdentity:
		customerID = pgtype.Int8{Int64: targetID, Valid: true}
	case contactport.HistoricalImportExternalIdentity:
		identityID = pgtype.Int8{Int64: targetID, Valid: true}
	}
	rows, err := contactdb.New(tx).AppendHistoricalImportLineage(ctx, contactdb.AppendHistoricalImportLineageParams{SourceTable: sourceTable, SourceKeyHmac: fact.SourceKeyHMAC, StaffID: staffID, CustomerID: customerID, IdentityID: identityID, RunID: runID, PayloadHmac: fact.PayloadHMAC})
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrHistoricalImportTargetDrift
	}
	return nil
}

func (HistoricalImportRepository) AppendHistoricalImportQuarantine(ctx context.Context, quarantine contactport.HistoricalImportQuarantine) error {
	sourceTable, ok := historicalImportSourceTable(quarantine.Source)
	if !ok || quarantine.RunID < 1 || !validHistoricalSourceFact(quarantine.SourceFact) || strings.TrimSpace(quarantine.ReasonCode) != quarantine.ReasonCode || quarantine.ReasonCode == "" {
		return ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	rows, err := contactdb.New(tx).AppendHistoricalImportQuarantine(ctx, contactdb.AppendHistoricalImportQuarantineParams{RunID: quarantine.RunID, SourceTable: sourceTable, SourceKeyHmac: quarantine.SourceFact.SourceKeyHMAC, ReasonCode: quarantine.ReasonCode, PayloadHmac: quarantine.SourceFact.PayloadHMAC, FieldDigest: quarantine.SourceFact.FieldDigest})
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrHistoricalImportTargetDrift
	}
	return nil
}

func (HistoricalImportRepository) AppendHistoricalImportRowReceipt(ctx context.Context, runID int64, source contactport.HistoricalImportSource, fact contactport.HistoricalImportSourceFact, disposition contactport.HistoricalImportDisposition) error {
	sourceTable, ok := historicalImportSourceTable(source)
	if !ok || runID < 1 || !validHistoricalSourceFact(fact) {
		return ErrInvalidHistoricalImport
	}
	dispositionText := ""
	switch disposition {
	case contactport.HistoricalImportImported:
		dispositionText = "imported"
	case contactport.HistoricalImportQuarantined:
		dispositionText = "quarantined"
	default:
		return ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	rows, err := contactdb.New(tx).AppendHistoricalImportRowReceipt(ctx, contactdb.AppendHistoricalImportRowReceiptParams{RunID: runID, SourceTable: sourceTable, SourceKeyHmac: fact.SourceKeyHMAC, PayloadHmac: fact.PayloadHMAC, FieldDigest: fact.FieldDigest, Disposition: dispositionText})
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrHistoricalImportTargetDrift
	}
	return nil
}

func historicalImportSourceTable(source contactport.HistoricalImportSource) (string, bool) {
	switch source {
	case contactport.HistoricalImportOwnerRoleMap:
		return "owner_role_map", true
	case contactport.HistoricalImportCustomerIdentity:
		return "crm_user_identity", true
	case contactport.HistoricalImportExternalIdentity:
		return "wecom_external_contact_identity_map", true
	default:
		return "", false
	}
}

func validHistoricalSourceFact(fact contactport.HistoricalImportSourceFact) bool {
	return len(fact.SourceKeyHMAC) == 32 && len(fact.PayloadHMAC) == 32 && len(fact.FieldDigest) == 32
}

func sameOptionalString(value pgtype.Text, expected *string) bool {
	return value.Valid == (expected != nil) && (!value.Valid || value.String == *expected)
}

func sameOptionalInt16(value pgtype.Int2, expected *int16) bool {
	return value.Valid == (expected != nil) && (!value.Valid || value.Int16 == *expected)
}

func sameOptionalInt64(value pgtype.Int8, expected *int64) bool {
	return value.Valid == (expected != nil) && (!value.Valid || value.Int64 == *expected)
}

func sameTime(value pgtype.Timestamptz, expected time.Time) bool {
	return value.Valid && value.Time.Equal(expected)
}
