package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const groupOpsStaffSourceTable = "owner_role_map"

// groupOpsStaffResolver only exposes a staff ID when the immutable DM01
// lineage and the currently locked staff fact still agree. It never treats a
// V1 userid as a V2 ID.
type groupOpsStaffResolver struct {
	contacts contactport.HistoricalImportTarget
	runID    int64
	key      []byte
}

func newGroupOpsStaffResolver(ctx context.Context, uow *platformstore.UnitOfWork, runID int64, key []byte) (*groupOpsStaffResolver, error) {
	if ctx == nil || uow == nil || runID < 1 || len(key) < sha256.Size {
		return nil, v1domain.ErrInvalidScope
	}
	contacts := contactstore.HistoricalImportRepository{}
	if err := uow.Within(ctx, func(tx context.Context) error {
		return validateGroupOpsStaffRun(tx, contacts, runID)
	}); err != nil {
		return nil, err
	}
	return &groupOpsStaffResolver{contacts: contacts, runID: runID, key: append([]byte(nil), key...)}, nil
}

func validateGroupOpsStaffRun(ctx context.Context, runs contactport.HistoricalImportRunReader, runID int64) error {
	if ctx == nil || runs == nil || runID < 1 {
		return v1domain.ErrInvalidScope
	}
	mode, state, err := runs.ReadHistoricalImportRun(ctx, runID)
	if err != nil {
		return err
	}
	if mode != "full" || state != "imported" {
		return v1domain.ErrConflict
	}
	return nil
}

// ResolveGroupOpsStaff is caller-bound: Group Ops invokes it inside the same
// UoW as the history write. Missing DM01 lineage remains an absent reference;
// a nonterminal or drifted lineage aborts the caller transaction.
func (resolver *groupOpsStaffResolver) ResolveGroupOpsStaff(ctx context.Context, sourceUserID string) (*int64, error) {
	if resolver == nil || resolver.contacts == nil || resolver.runID < 1 || len(resolver.key) < sha256.Size {
		return nil, v1domain.ErrInvalidScope
	}
	if sourceUserID == "" {
		return nil, nil
	}
	sourceKey, err := contactmigration.SourceKeyHMAC(resolver.key, groupOpsStaffSourceTable, sourceUserID)
	if err != nil {
		return nil, v1domain.ErrConflict
	}
	const source = contactport.HistoricalImportOwnerRoleMap
	if err = resolver.contacts.LockHistoricalImportSource(ctx, source, sourceKey); err != nil {
		return nil, err
	}
	lineage, found, err := resolver.contacts.LockHistoricalImportLineage(ctx, source, sourceKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	if lineage.TargetID < 1 || lineage.LastRunID != resolver.runID || !validGroupOpsStaffDigest(lineage.PayloadHMAC) || !validGroupOpsStaffDigest(lineage.FieldDigest) {
		return nil, v1domain.ErrConflict
	}
	receipt, found, err := resolver.contacts.FindHistoricalImportRowReceipt(ctx, resolver.runID, source, sourceKey)
	if err != nil {
		return nil, err
	}
	if !found || receipt.Disposition != contactport.HistoricalImportImported ||
		!sameGroupOpsStaffDigest(receipt.PayloadHMAC, lineage.PayloadHMAC) || !sameGroupOpsStaffDigest(receipt.FieldDigest, lineage.FieldDigest) {
		return nil, v1domain.ErrConflict
	}
	actual, err := resolver.contacts.LockHistoricalImportStaffTarget(ctx, lineage.TargetID)
	if err != nil {
		return nil, err
	}
	actualDigest, err := groupOpsStaffTargetDigest(resolver.key, actual)
	if err != nil || !sameGroupOpsStaffDigest(actualDigest, lineage.FieldDigest) || !sameGroupOpsStaffDigest(actualDigest, receipt.FieldDigest) {
		return nil, v1domain.ErrConflict
	}
	return &lineage.TargetID, nil
}

func groupOpsStaffTargetDigest(key []byte, fact contactport.HistoricalImportStaffFact) ([]byte, error) {
	payload, err := json.Marshal(fact)
	if err != nil {
		return nil, err
	}
	return contactmigration.SourceFieldsHMAC(key, groupOpsStaffSourceTable, payload)
}

func validGroupOpsStaffDigest(value []byte) bool { return len(value) == sha256.Size }

func sameGroupOpsStaffDigest(left, right []byte) bool {
	return validGroupOpsStaffDigest(left) && validGroupOpsStaffDigest(right) && hmac.Equal(left, right)
}
