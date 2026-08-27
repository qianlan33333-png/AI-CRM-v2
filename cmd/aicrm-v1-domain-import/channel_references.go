package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type channelCustomerResolver struct {
	contacts contactport.HistoricalImportTarget
	run      int64
	key      []byte
}

func newChannelCustomerResolver(ctx context.Context, uow *platformstore.UnitOfWork, run int64, key []byte) (*channelCustomerResolver, error) {
	if ctx == nil || uow == nil || run < 1 || len(key) < sha256.Size {
		return nil, v1domain.ErrInvalidScope
	}
	contacts := contactstore.HistoricalImportRepository{}
	err := uow.Within(ctx, func(ctx context.Context) error {
		mode, state, err := contacts.ReadHistoricalImportRun(ctx, run)
		if err != nil {
			return err
		}
		if mode != "full" || state != "imported" {
			return v1domain.ErrConflict
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &channelCustomerResolver{contacts: contacts, run: run, key: append([]byte(nil), key...)}, nil
}

// Caller-bound: the verified customer link and the historical channel fact
// are written in one transaction. Missing lineage stays NULL, never guessed.
func (r *channelCustomerResolver) ResolveHistoricalChannelCustomer(ctx context.Context, unionID string) (*int64, error) {
	if r == nil || r.contacts == nil || r.run < 1 || len(r.key) < sha256.Size {
		return nil, v1domain.ErrInvalidScope
	}
	if unionID == "" {
		return nil, nil
	}
	key, err := contactmigration.SourceKeyHMAC(r.key, "crm_user_identity", unionID)
	if err != nil {
		return nil, v1domain.ErrConflict
	}
	const source = contactport.HistoricalImportCustomerIdentity
	if err = r.contacts.LockHistoricalImportSource(ctx, source, key); err != nil {
		return nil, err
	}
	lineage, found, err := r.contacts.LockHistoricalImportLineage(ctx, source, key)
	if err != nil || !found {
		return nil, err
	}
	if lineage.TargetID < 1 || lineage.LastRunID != r.run || len(lineage.PayloadHMAC) != sha256.Size || len(lineage.FieldDigest) != sha256.Size {
		return nil, v1domain.ErrConflict
	}
	receipt, found, err := r.contacts.FindHistoricalImportRowReceipt(ctx, r.run, source, key)
	if err != nil {
		return nil, err
	}
	if !found || receipt.Disposition != contactport.HistoricalImportImported || !hmac.Equal(receipt.PayloadHMAC, lineage.PayloadHMAC) || !hmac.Equal(receipt.FieldDigest, lineage.FieldDigest) {
		return nil, v1domain.ErrConflict
	}
	if _, err = r.contacts.LockHistoricalImportCustomerTarget(ctx, lineage.TargetID); err != nil {
		return nil, err
	}
	if err = r.contacts.ValidateHistoricalImportCustomerRoot(ctx, lineage.TargetID); err != nil {
		return nil, err
	}
	return &lineage.TargetID, nil
}
