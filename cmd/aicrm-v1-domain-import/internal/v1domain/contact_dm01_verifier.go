package v1domain

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"

	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
)

const dm01CustomerIdentitySourceTable = "crm_user_identity"

// DM01CustomerTagVerifier resolves a V1 unionid only through the frozen DM01
// customer-root mapping. It has no external identity or Provider capability.
type DM01CustomerTagVerifier struct {
	uow           UnitOfWork
	contacts      contactport.HistoricalImportTarget
	sourceHMACKey []byte
	expectedRunID int64
}

var _ VerifiedDM01CustomerTagWriter = (*DM01CustomerTagVerifier)(nil)

func NewDM01CustomerTagVerifier(uow UnitOfWork, contacts contactport.HistoricalImportTarget, sourceHMACKey []byte, expectedRunID int64) (*DM01CustomerTagVerifier, error) {
	if uow == nil || contacts == nil || len(sourceHMACKey) < sha256.Size || expectedRunID < 1 {
		return nil, ErrInvalidScope
	}
	return &DM01CustomerTagVerifier{
		uow: uow, contacts: contacts, sourceHMACKey: append([]byte(nil), sourceHMACKey...), expectedRunID: expectedRunID,
	}, nil
}

func (verifier *DM01CustomerTagVerifier) ResolveVerifiedDM01Customer(ctx context.Context, unionID string) (contactport.CustomerID, error) {
	key, err := verifier.sourceKey(unionID)
	if err != nil {
		return 0, err
	}
	var customerID contactport.CustomerID
	err = verifier.uow.Within(ctx, func(tx context.Context) error {
		customerID, err = verifier.verify(tx, key)
		return err
	})
	if err != nil {
		return 0, err
	}
	return customerID, nil
}

// VerifyHistoricalTagCustomer is invoked inside HistoricalTagImportService's
// existing UnitOfWork. It intentionally must not open another transaction.
func (verifier *DM01CustomerTagVerifier) VerifyHistoricalTagCustomer(ctx context.Context, unionID string, customerID contactport.CustomerID) error {
	if customerID < 1 {
		return contactport.ErrHistoricalTagBlocked
	}
	key, err := verifier.sourceKey(unionID)
	if err != nil {
		return err
	}
	verified, err := verifier.verify(ctx, key)
	if err != nil {
		return err
	}
	if verified != customerID {
		return contactport.ErrHistoricalTagBlocked
	}
	return nil
}

func (verifier *DM01CustomerTagVerifier) sourceKey(unionID string) ([]byte, error) {
	if verifier == nil || verifier.uow == nil || verifier.contacts == nil || len(verifier.sourceHMACKey) < sha256.Size || verifier.expectedRunID < 1 {
		return nil, contactport.ErrHistoricalTagUnavailable
	}
	key, err := contactmigration.SourceKeyHMAC(verifier.sourceHMACKey, dm01CustomerIdentitySourceTable, unionID)
	if err != nil {
		return nil, contactport.ErrHistoricalTagBlocked
	}
	return key, nil
}

func (verifier *DM01CustomerTagVerifier) verify(ctx context.Context, sourceKey []byte) (contactport.CustomerID, error) {
	if err := verifier.contacts.LockHistoricalImportSource(ctx, contactport.HistoricalImportCustomerIdentity, sourceKey); err != nil {
		return 0, dm01CustomerTagVerificationError(err)
	}
	lineage, found, err := verifier.contacts.LockHistoricalImportLineage(ctx, contactport.HistoricalImportCustomerIdentity, sourceKey)
	if err != nil {
		return 0, dm01CustomerTagVerificationError(err)
	}
	if !found || lineage.TargetID < 1 || lineage.LastRunID != verifier.expectedRunID || !validDM01Digest(lineage.PayloadHMAC) || !validDM01Digest(lineage.FieldDigest) {
		return 0, contactport.ErrHistoricalTagBlocked
	}
	receipt, found, err := verifier.contacts.FindHistoricalImportRowReceipt(ctx, verifier.expectedRunID, contactport.HistoricalImportCustomerIdentity, sourceKey)
	if err != nil {
		return 0, dm01CustomerTagVerificationError(err)
	}
	if !found || receipt.Disposition != contactport.HistoricalImportImported ||
		!sameDM01Digest(receipt.PayloadHMAC, lineage.PayloadHMAC) || !sameDM01Digest(receipt.FieldDigest, lineage.FieldDigest) {
		return 0, contactport.ErrHistoricalTagBlocked
	}
	if _, err = verifier.contacts.LockHistoricalImportCustomerTarget(ctx, lineage.TargetID); err != nil {
		return 0, dm01CustomerTagVerificationError(err)
	}
	if err = verifier.contacts.ValidateHistoricalImportCustomerRoot(ctx, lineage.TargetID); err != nil {
		return 0, dm01CustomerTagVerificationError(err)
	}
	return contactport.CustomerID(lineage.TargetID), nil
}

func dm01CustomerTagVerificationError(err error) error {
	if errors.Is(err, contactstore.ErrHistoricalImportTargetDrift) {
		return contactport.ErrHistoricalTagBlocked
	}
	return err
}

func validDM01Digest(value []byte) bool { return len(value) == sha256.Size }

func sameDM01Digest(left, right []byte) bool {
	return validDM01Digest(left) && validDM01Digest(right) && hmac.Equal(left, right)
}
