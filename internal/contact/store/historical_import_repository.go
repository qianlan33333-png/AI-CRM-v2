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
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || encode($2::bytea, 'hex'), 0))`, sourceTable, sourceKeyHMAC)
	return err
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
	var receipt contactport.HistoricalImportRowReceipt
	var disposition string
	err = tx.QueryRow(ctx, `SELECT payload_hmac, field_digest, disposition FROM legacy_contact_identity_import_row_receipts WHERE run_id=$1 AND source_table=$2 AND source_key_hmac=$3 FOR UPDATE`, runID, sourceTable, sourceKeyHMAC).Scan(&receipt.PayloadHMAC, &receipt.FieldDigest, &disposition)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.HistoricalImportRowReceipt{}, false, nil
	}
	if err != nil {
		return contactport.HistoricalImportRowReceipt{}, false, err
	}
	switch disposition {
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
	var staffID, customerID, identityID pgtype.Int8
	var payloadHMAC []byte
	err = tx.QueryRow(ctx, `SELECT staff_id, customer_id, identity_id, payload_hmac FROM legacy_contact_identity_source_mappings WHERE source_table=$1 AND source_key_hmac=$2 FOR UPDATE`, sourceTable, sourceKeyHMAC).Scan(&staffID, &customerID, &identityID, &payloadHMAC)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.HistoricalImportLineage{}, false, nil
	}
	if err != nil {
		return contactport.HistoricalImportLineage{}, false, err
	}
	targetID := int64(0)
	switch source {
	case contactport.HistoricalImportOwnerRoleMap:
		if staffID.Valid {
			targetID = staffID.Int64
		}
	case contactport.HistoricalImportCustomerIdentity:
		if customerID.Valid {
			targetID = customerID.Int64
		}
	case contactport.HistoricalImportExternalIdentity:
		if identityID.Valid {
			targetID = identityID.Int64
		}
	}
	if targetID < 1 || len(payloadHMAC) != 32 {
		return contactport.HistoricalImportLineage{}, false, ErrHistoricalImportTargetDrift
	}
	return contactport.HistoricalImportLineage{TargetID: targetID, PayloadHMAC: payloadHMAC}, true, nil
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
	var name string
	var avatar pgtype.Text
	var gender pgtype.Int2
	var owner pgtype.Int8
	var firstSeen, lastSeen, createdAt, updatedAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `SELECT name, avatar_url, gender, owner_staff_id, added_at, last_interact_at, created_at, updated_at FROM customers WHERE id=$1 AND NOT is_deleted FOR UPDATE`, customerID).Scan(&name, &avatar, &gender, &owner, &firstSeen, &lastSeen, &createdAt, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrHistoricalImportTargetDrift
	}
	if err != nil {
		return err
	}
	if name != fact.Name || !sameOptionalString(avatar, fact.AvatarURL) || !sameOptionalInt16(gender, fact.Gender) || !sameOptionalInt64(owner, fact.OwnerStaffID) || !sameTime(firstSeen, fact.FirstSeenAt) || !sameTime(lastSeen, fact.LastSeenAt) || !sameTime(createdAt, fact.CreatedAt) || !sameTime(updatedAt, fact.UpdatedAt) {
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
	var active bool
	err = tx.QueryRow(ctx, `SELECT is_active FROM staff WHERE id=$1 FOR SHARE`, staffID).Scan(&active)
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
	var found bool
	err = tx.QueryRow(ctx, `SELECT TRUE FROM customers WHERE id=$1 AND NOT is_deleted FOR SHARE`, customerID).Scan(&found)
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
	var staffID, customerID, identityID any
	switch source {
	case contactport.HistoricalImportOwnerRoleMap:
		staffID = targetID
	case contactport.HistoricalImportCustomerIdentity:
		customerID = targetID
	case contactport.HistoricalImportExternalIdentity:
		identityID = targetID
	}
	tag, err := tx.Exec(ctx, `INSERT INTO legacy_contact_identity_source_mappings(source_table,source_key_hmac,staff_id,customer_id,identity_id,first_run_id,last_run_id,payload_hmac) VALUES($1,$2,$3,$4,$5,$6,$6,$7)`, sourceTable, fact.SourceKeyHMAC, staffID, customerID, identityID, runID, fact.PayloadHMAC)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
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
	tag, err := tx.Exec(ctx, `INSERT INTO legacy_contact_identity_import_quarantines(run_id,source_table,source_key_hmac,reason_code,payload_hmac,field_digest) VALUES($1,$2,$3,$4,$5,$6)`, quarantine.RunID, sourceTable, quarantine.SourceFact.SourceKeyHMAC, quarantine.ReasonCode, quarantine.SourceFact.PayloadHMAC, quarantine.SourceFact.FieldDigest)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
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
	tag, err := tx.Exec(ctx, `INSERT INTO legacy_contact_identity_import_row_receipts(run_id,source_table,source_key_hmac,payload_hmac,field_digest,disposition) VALUES($1,$2,$3,$4,$5,$6)`, runID, sourceTable, fact.SourceKeyHMAC, fact.PayloadHMAC, fact.FieldDigest, dispositionText)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
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
