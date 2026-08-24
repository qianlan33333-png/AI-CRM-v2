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

type LeaseFence struct {
	RunID      int64
	Generation int64
	TokenHMAC  []byte
}

func (HistoricalImportRepository) AssertLease(ctx context.Context, fence LeaseFence) error {
	if fence.RunID < 1 || fence.Generation < 1 || len(fence.TokenHMAC) != 32 {
		return ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	_, err = contactdb.New(tx).AssertHistoricalImportLease(ctx, contactdb.AssertHistoricalImportLeaseParams{RunID: fence.RunID, ExpectedGeneration: fence.Generation, TokenHmac: fence.TokenHMAC})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrHistoricalImportTargetDrift
	}
	return err
}

var _ contactport.HistoricalImportTarget = HistoricalImportRepository{}
var _ contactport.NonActiveTarget = HistoricalImportRepository{}

func (repository HistoricalImportRepository) AssertNonActiveLease(ctx context.Context, fence contactport.NonActiveLeaseFence) error {
	return repository.AssertLease(ctx, LeaseFence{RunID: fence.RunID, Generation: fence.Generation, TokenHMAC: fence.TokenHMAC})
}

func (HistoricalImportRepository) LockNonActiveSource(ctx context.Context, source contactport.NonActiveSource, sourceKeyHMAC []byte) error {
	table, ok := nonActiveSourceTable(source)
	if !ok || len(sourceKeyHMAC) != 32 {
		return ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	return contactdb.New(tx).LockHistoricalImportSource(ctx, contactdb.LockHistoricalImportSourceParams{SourceTable: table, SourceKeyHmac: sourceKeyHMAC})
}

func (HistoricalImportRepository) FindNonActiveReceipt(ctx context.Context, runID int64, source contactport.NonActiveSource, key []byte) (contactport.NonActiveRowReceipt, bool, error) {
	table, ok := nonActiveSourceTable(source)
	if !ok || runID < 1 || len(key) != 32 {
		return contactport.NonActiveRowReceipt{}, false, ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactport.NonActiveRowReceipt{}, false, err
	}
	row, err := contactdb.New(tx).FindHistoricalImportRowReceipt(ctx, contactdb.FindHistoricalImportRowReceiptParams{RunID: runID, SourceTable: table, SourceKeyHmac: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.NonActiveRowReceipt{}, false, nil
	}
	if err != nil {
		return contactport.NonActiveRowReceipt{}, false, err
	}
	disposition, ok := parseNonActiveDisposition(row.Disposition)
	if !ok {
		return contactport.NonActiveRowReceipt{}, false, ErrHistoricalImportTargetDrift
	}
	return contactport.NonActiveRowReceipt{PayloadHMAC: row.PayloadHmac, FieldDigest: row.FieldDigest, Disposition: disposition}, true, nil
}

func (HistoricalImportRepository) FindNonActiveArchive(ctx context.Context, runID int64, source contactport.NonActiveSource, key []byte) (contactport.NonActiveArchive, bool, error) {
	table, ok := nonActiveSourceTable(source)
	if !ok || runID < 1 || len(key) != 32 {
		return contactport.NonActiveArchive{}, false, ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactport.NonActiveArchive{}, false, err
	}
	row, err := contactdb.New(tx).FindHistoricalImportArchive(ctx, contactdb.FindHistoricalImportArchiveParams{RunID: runID, SourceTable: table, SourceKeyHmac: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.NonActiveArchive{}, false, nil
	}
	if err != nil {
		return contactport.NonActiveArchive{}, false, err
	}
	fact := contactport.HistoricalImportSourceFact{SourceKeyHMAC: key, PayloadHMAC: row.PayloadHmac, FieldDigest: row.FieldDigest}
	return contactport.NonActiveArchive{RunID: runID, Source: source, SourceFact: fact, Nonce: row.ArchiveNonce, Ciphertext: row.ArchiveCiphertext, KeyVersion: row.ArchiveKeyVersion}, true, nil
}

func (HistoricalImportRepository) FindNonActiveQuarantine(ctx context.Context, runID int64, source contactport.NonActiveSource, key []byte) (contactport.NonActiveQuarantine, bool, error) {
	table, ok := nonActiveSourceTable(source)
	reason := nonActiveQuarantineReason(source)
	if !ok || reason == "" || runID < 1 || len(key) != 32 {
		return contactport.NonActiveQuarantine{}, false, ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactport.NonActiveQuarantine{}, false, err
	}
	row, err := contactdb.New(tx).FindHistoricalImportQuarantine(ctx, contactdb.FindHistoricalImportQuarantineParams{RunID: runID, SourceTable: table, SourceKeyHmac: key, ReasonCode: reason})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.NonActiveQuarantine{}, false, nil
	}
	if err != nil {
		return contactport.NonActiveQuarantine{}, false, err
	}
	fact := contactport.HistoricalImportSourceFact{SourceKeyHMAC: key, PayloadHMAC: row.PayloadHmac, FieldDigest: row.FieldDigest}
	return contactport.NonActiveQuarantine{RunID: runID, Source: source, SourceFact: fact, ReasonCode: row.ReasonCode}, true, nil
}

func (HistoricalImportRepository) AppendNonActiveArchive(ctx context.Context, archive contactport.NonActiveArchive) error {
	table, ok := nonActiveSourceTable(archive.Source)
	if !ok || archive.RunID < 1 || !validHistoricalSourceFact(archive.SourceFact) || len(archive.Nonce) != 12 || len(archive.Ciphertext) <= 16 || archive.KeyVersion < 1 {
		return ErrInvalidHistoricalImport
	}
	if archive.Source != contactport.NonActiveMergeAudit && archive.Source != contactport.NonActiveResolutionQueue {
		return ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	rows, err := contactdb.New(tx).AppendHistoricalImportArchive(ctx, contactdb.AppendHistoricalImportArchiveParams{RunID: archive.RunID, SourceTable: table, SourceKeyHmac: archive.SourceFact.SourceKeyHMAC, PayloadHmac: archive.SourceFact.PayloadHMAC, FieldDigest: archive.SourceFact.FieldDigest, ArchiveNonce: archive.Nonce, ArchiveCiphertext: archive.Ciphertext, ArchiveKeyVersion: archive.KeyVersion})
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrHistoricalImportTargetDrift
	}
	return nil
}

func (HistoricalImportRepository) AppendNonActiveQuarantine(ctx context.Context, quarantine contactport.NonActiveQuarantine) error {
	table, ok := nonActiveSourceTable(quarantine.Source)
	want := nonActiveQuarantineReason(quarantine.Source)
	if !ok || want == "" || quarantine.ReasonCode != want || quarantine.RunID < 1 || !validHistoricalSourceFact(quarantine.SourceFact) {
		return ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	rows, err := contactdb.New(tx).AppendHistoricalImportQuarantine(ctx, contactdb.AppendHistoricalImportQuarantineParams{RunID: quarantine.RunID, SourceTable: table, SourceKeyHmac: quarantine.SourceFact.SourceKeyHMAC, ReasonCode: want, PayloadHmac: quarantine.SourceFact.PayloadHMAC, FieldDigest: quarantine.SourceFact.FieldDigest})
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrHistoricalImportTargetDrift
	}
	return nil
}

func (HistoricalImportRepository) AppendNonActiveReceipt(ctx context.Context, fence contactport.NonActiveLeaseFence, source contactport.NonActiveSource, fact contactport.HistoricalImportSourceFact, disposition contactport.NonActiveDisposition) error {
	table, ok := nonActiveSourceTable(source)
	text, okDisposition := nonActiveDispositionText(disposition)
	if !ok || !okDisposition || disposition != expectedNonActiveDisposition(source) || fence.RunID < 1 || fence.Generation < 1 || len(fence.TokenHMAC) != 32 || !validHistoricalSourceFact(fact) {
		return ErrInvalidHistoricalImport
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	rows, err := contactdb.New(tx).AppendHistoricalImportRowReceiptFenced(ctx, contactdb.AppendHistoricalImportRowReceiptFencedParams{RunID: fence.RunID, ExpectedGeneration: fence.Generation, TokenHmac: fence.TokenHMAC, SourceTable: table, SourceKeyHmac: fact.SourceKeyHMAC, PayloadHmac: fact.PayloadHMAC, FieldDigest: fact.FieldDigest, Disposition: text})
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrHistoricalImportTargetDrift
	}
	return nil
}

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

func nonActiveSourceTable(source contactport.NonActiveSource) (string, bool) {
	switch source {
	case contactport.NonActiveMergeAudit:
		return "crm_user_identity_merge_audit", true
	case contactport.NonActiveResolutionQueue:
		return "crm_user_identity_resolution_queue", true
	case contactport.NonActiveContacts:
		return "contacts", true
	case contactport.NonActiveIdentityConflicts:
		return "crm_user_identity_conflicts", true
	case contactport.NonActivePeople:
		return "people", true
	case contactport.NonActiveFollowUsers:
		return "wecom_external_contact_follow_users", true
	case contactport.NonActiveDirectoryMembers:
		return "admin_wecom_directory_members", true
	case contactport.NonActiveExternalBindings:
		return "external_contact_bindings", true
	default:
		return "", false
	}
}

func nonActiveQuarantineReason(source contactport.NonActiveSource) string {
	switch source {
	case contactport.NonActiveIdentityConflicts, contactport.NonActivePeople:
		return "target_schema_deferred"
	case contactport.NonActiveFollowUsers:
		return "multiple_follow_users_deferred"
	default:
		return ""
	}
}

func nonActiveDispositionText(disposition contactport.NonActiveDisposition) (string, bool) {
	switch disposition {
	case contactport.NonActiveArchived:
		return "archived", true
	case contactport.NonActiveSkipped:
		return "skipped", true
	case contactport.NonActiveQuarantined:
		return "quarantined", true
	default:
		return "", false
	}
}

func expectedNonActiveDisposition(source contactport.NonActiveSource) contactport.NonActiveDisposition {
	switch source {
	case contactport.NonActiveMergeAudit, contactport.NonActiveResolutionQueue:
		return contactport.NonActiveArchived
	case contactport.NonActiveIdentityConflicts, contactport.NonActivePeople, contactport.NonActiveFollowUsers:
		return contactport.NonActiveQuarantined
	case contactport.NonActiveContacts, contactport.NonActiveDirectoryMembers, contactport.NonActiveExternalBindings:
		return contactport.NonActiveSkipped
	default:
		return 0
	}
}

func parseNonActiveDisposition(value string) (contactport.NonActiveDisposition, bool) {
	switch value {
	case "archived":
		return contactport.NonActiveArchived, true
	case "skipped":
		return contactport.NonActiveSkipped, true
	case "quarantined":
		return contactport.NonActiveQuarantined, true
	default:
		return 0, false
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
