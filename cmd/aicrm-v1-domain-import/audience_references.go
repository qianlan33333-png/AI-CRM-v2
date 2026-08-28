package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const audienceStaffSourceTable = "owner_role_map"

// audienceHistoryReferences resolves only sealed DM01 lineage while the
// importer holds its caller transaction. It cannot infer a customer or staff
// ID from a V1 identifier and it never calls a Provider.
type audienceHistoryReferences struct {
	customer *channelCustomerResolver
	contacts contactport.HistoricalImportTarget
	run      int64
	key      []byte
}

var _ v1domain.AudienceHistoryResolver = (*audienceHistoryReferences)(nil)

func newAudienceHistoryReferences(ctx context.Context, uow *platformstore.UnitOfWork, dm01RunID int64, key []byte) (*audienceHistoryReferences, error) {
	customer, err := newChannelCustomerResolver(ctx, uow, dm01RunID, key)
	if err != nil {
		return nil, err
	}
	return &audienceHistoryReferences{customer: customer, contacts: customer.contacts, run: customer.run, key: append([]byte(nil), customer.key...)}, nil
}

func (r *audienceHistoryReferences) ResolveAudienceHistoryCustomer(ctx context.Context, unionID string) (*int64, error) {
	if r == nil || r.customer == nil {
		return nil, v1domain.ErrInvalidScope
	}
	return r.customer.ResolveHistoricalChannelCustomer(ctx, unionID)
}

func (r *audienceHistoryReferences) ResolveAudienceHistoryStaff(ctx context.Context, sourceUserID string) (*int64, error) {
	if r == nil || r.contacts == nil || r.run < 1 || len(r.key) < sha256.Size {
		return nil, v1domain.ErrInvalidScope
	}
	if sourceUserID == "" {
		return nil, nil
	}
	sourceKey, err := contactmigration.SourceKeyHMAC(r.key, audienceStaffSourceTable, sourceUserID)
	if err != nil {
		return nil, v1domain.ErrConflict
	}
	const source = contactport.HistoricalImportOwnerRoleMap
	if err = r.contacts.LockHistoricalImportSource(ctx, source, sourceKey); err != nil {
		return nil, err
	}
	lineage, found, err := r.contacts.LockHistoricalImportLineage(ctx, source, sourceKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	if lineage.TargetID < 1 || lineage.LastRunID != r.run || !audienceHistoryDigest(lineage.PayloadHMAC) || !audienceHistoryDigest(lineage.FieldDigest) {
		return nil, v1domain.ErrConflict
	}
	receipt, found, err := r.contacts.FindHistoricalImportRowReceipt(ctx, r.run, source, sourceKey)
	if err != nil {
		return nil, err
	}
	if !found || receipt.Disposition != contactport.HistoricalImportImported || !sameAudienceHistoryDigest(receipt.PayloadHMAC, lineage.PayloadHMAC) || !sameAudienceHistoryDigest(receipt.FieldDigest, lineage.FieldDigest) {
		return nil, v1domain.ErrConflict
	}
	actual, err := r.contacts.LockHistoricalImportStaffTarget(ctx, lineage.TargetID)
	if err != nil {
		return nil, err
	}
	digest, err := audienceHistoryStaffDigest(r.key, actual)
	if err != nil || !sameAudienceHistoryDigest(digest, lineage.FieldDigest) || !sameAudienceHistoryDigest(digest, receipt.FieldDigest) {
		return nil, v1domain.ErrConflict
	}
	return &lineage.TargetID, nil
}

func audienceHistoryStaffDigest(key []byte, fact contactport.HistoricalImportStaffFact) ([]byte, error) {
	payload, err := json.Marshal(fact)
	if err != nil {
		return nil, err
	}
	return contactmigration.SourceFieldsHMAC(key, audienceStaffSourceTable, payload)
}

func audienceHistoryDigest(value []byte) bool { return len(value) == sha256.Size }

func sameAudienceHistoryDigest(left, right []byte) bool {
	return audienceHistoryDigest(left) && audienceHistoryDigest(right) && hmac.Equal(left, right)
}
