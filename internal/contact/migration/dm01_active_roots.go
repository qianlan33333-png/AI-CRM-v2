package migration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrInvalidActiveRoots = errors.New("invalid DM01 active roots")
	ErrActiveRootDrift    = errors.New("DM01 active root drift")
)

const (
	quarantineMissingCustomerRoot = "missing_customer_root"
	quarantineIdentityConflict    = "scoped_identity_customer_conflict"
	quarantineOwnerUnresolved     = "owner_unresolved"
	quarantineTargetDrift         = "target_drift_since_last_import"
	MaximumActiveRootBatchRows    = 1000
)

type StaffActiveRoot struct {
	Source contactport.HistoricalImportSourceFact
	Target contactport.HistoricalImportStaffFact
}

type CustomerActiveRoot struct {
	Source                  contactport.HistoricalImportSourceFact
	OwnerStaffSourceKeyHMAC []byte
	Target                  contactport.HistoricalImportCustomerFact
}

type ExternalIdentityActiveRoot struct {
	Source                contactport.HistoricalImportSourceFact
	CustomerSourceKeyHMAC []byte
	CorpID                string
	ExternalUserID        string
}

type ActiveRootsCommand struct {
	Fence          contactport.NonActiveLeaseFence
	CorpID         string
	HMACKeyVersion int16
	DigestKey      []byte
	Staff          []StaffActiveRoot
	SkippedOwners  []contactport.HistoricalImportSourceFact
	Customers      []CustomerActiveRoot
	Identities     []ExternalIdentityActiveRoot
}

type ActiveRootsResult struct {
	Imported                int
	Replayed                int
	Quarantined             int
	ChangedSourceCandidates int
	Updated                 int
}

type lineageState uint8

const (
	lineageMissing lineageState = iota
	lineageSamePayload
	lineageChangedSource
)

// ActiveRootService is the only DM01 active-root application. It deliberately
// has no ordinary event, merge, Provider, source SQL, or arbitrary target SQL
// dependency. A whole 230 -> 152 -> 314 batch commits in one target UoW.
type ActiveRootService struct {
	uow        platformport.UnitOfWork
	contacts   contactport.HistoricalImportTarget
	identities identityport.HistoricalScopedIdentityBinder
}

func NewActiveRootService(uow platformport.UnitOfWork, contacts contactport.HistoricalImportTarget, identities identityport.HistoricalScopedIdentityBinder) *ActiveRootService {
	return &ActiveRootService{uow: uow, contacts: contacts, identities: identities}
}

func (service *ActiveRootService) Process(ctx context.Context, command ActiveRootsCommand) (ActiveRootsResult, error) {
	if service == nil || service.uow == nil || service.contacts == nil || service.identities == nil || ctx == nil || !validActiveRoots(command) {
		return ActiveRootsResult{}, ErrInvalidActiveRoots
	}
	var result ActiveRootsResult
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		for _, fact := range command.SkippedOwners {
			if err := service.processSkippedOwner(txCtx, command, fact, &result); err != nil {
				return err
			}
		}
		for _, row := range command.Staff {
			if err := service.processStaff(txCtx, command, row, &result); err != nil {
				return err
			}
		}
		for _, row := range command.Customers {
			if err := service.processCustomer(txCtx, command, row, &result); err != nil {
				return err
			}
		}
		for _, row := range command.Identities {
			if err := service.processIdentity(txCtx, command, row, &result); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ActiveRootsResult{}, err
	}
	return result, nil
}

func (service *ActiveRootService) processSkippedOwner(ctx context.Context, command ActiveRootsCommand, fact contactport.HistoricalImportSourceFact, result *ActiveRootsResult) error {
	receipt, replay, err := service.lockReceipt(ctx, command.Fence.RunID, contactport.HistoricalImportOwnerRoleMap, fact)
	if err != nil {
		return err
	}
	lineage, state, err := service.matchLineage(ctx, contactport.HistoricalImportOwnerRoleMap, fact)
	if err != nil {
		return err
	}
	if state != lineageMissing || lineage.TargetID != 0 {
		return ErrActiveRootDrift
	}
	if replay {
		if receipt.Disposition != contactport.HistoricalImportSkipped || !hmac.Equal(receipt.PayloadHMAC, fact.PayloadHMAC) || !hmac.Equal(receipt.FieldDigest, fact.FieldDigest) {
			return ErrSourcePayloadDrift
		}
		result.Replayed++
		return nil
	}
	if err = service.contacts.LockHistoricalImportSource(ctx, contactport.HistoricalImportOwnerRoleMap, fact.SourceKeyHMAC); err != nil {
		return err
	}
	if err = service.contacts.AppendHistoricalImportRowReceipt(ctx, command.Fence, contactport.HistoricalImportOwnerRoleMap, fact, contactport.HistoricalImportSkipped); err != nil {
		return err
	}
	result.Replayed++
	return nil
}

func (service *ActiveRootService) processStaff(ctx context.Context, command ActiveRootsCommand, row StaffActiveRoot, result *ActiveRootsResult) error {
	fence, runID := command.Fence, command.Fence.RunID
	receipt, replay, err := service.lockReceipt(ctx, runID, contactport.HistoricalImportOwnerRoleMap, row.Source)
	if err != nil {
		return err
	}
	if replay {
		if receipt.Disposition == contactport.HistoricalImportQuarantined {
			if !hmac.Equal(receipt.FieldDigest, row.Source.FieldDigest) {
				return ErrSourcePayloadDrift
			}
			result.Replayed++
			return nil
		}
		if receipt.Disposition != contactport.HistoricalImportImported {
			return ErrActiveRootDrift
		}
		lineage, state, err := service.matchLineage(ctx, contactport.HistoricalImportOwnerRoleMap, row.Source)
		if err != nil || state != lineageSamePayload {
			return driftOr(err)
		}
		current, err := service.contacts.LockHistoricalImportStaffTarget(ctx, lineage.TargetID)
		if err != nil || !sameDigest(receipt.FieldDigest, staffTargetDigest(command.DigestKey, current)) || !sameDigest(receipt.FieldDigest, staffTargetDigest(command.DigestKey, row.Target)) {
			return driftOr(err)
		}
		if err = service.contacts.ValidateHistoricalImportStaff(ctx, lineage.TargetID, row.Target); err != nil {
			return err
		}
		result.Replayed++
		return nil
	}
	lineage, state, err := service.matchLineage(ctx, contactport.HistoricalImportOwnerRoleMap, row.Source)
	if err != nil {
		return err
	}
	if state == lineageChangedSource {
		current, lockErr := service.contacts.LockHistoricalImportStaffTarget(ctx, lineage.TargetID)
		if lockErr != nil {
			return lockErr
		}
		if !sameDigest(lineage.FieldDigest, staffTargetDigest(command.DigestKey, current)) {
			return service.quarantineChangedSource(ctx, fence, contactport.HistoricalImportOwnerRoleMap, row.Source, quarantineTargetDrift, result)
		}
		if err = service.contacts.UpdateHistoricalImportStaffCAS(ctx, lineage.TargetID, current, row.Target); err != nil {
			return err
		}
		if err = service.contacts.UpdateHistoricalImportLineageCAS(ctx, runID, contactport.HistoricalImportOwnerRoleMap, row.Source, lineage); err != nil {
			return err
		}
		if err = service.contacts.AppendHistoricalImportRowReceipt(ctx, fence, contactport.HistoricalImportOwnerRoleMap, withFieldDigest(row.Source, staffTargetDigest(command.DigestKey, row.Target)), contactport.HistoricalImportImported); err != nil {
			return err
		}
		result.Updated++
		return nil
	}
	staffID := lineage.TargetID
	if state == lineageSamePayload {
		if err = service.contacts.ValidateHistoricalImportStaff(ctx, staffID, row.Target); err != nil {
			return err
		}
	} else {
		staffID, err = service.contacts.EnsureHistoricalImportStaff(ctx, row.Target)
		if err != nil {
			return err
		}
		if staffID < 1 {
			return ErrActiveRootDrift
		}
		if err = service.contacts.AppendHistoricalImportLineage(ctx, runID, contactport.HistoricalImportOwnerRoleMap, row.Source, staffID); err != nil {
			return err
		}
	}
	if err = service.contacts.AppendHistoricalImportRowReceipt(ctx, fence, contactport.HistoricalImportOwnerRoleMap, withFieldDigest(row.Source, staffTargetDigest(command.DigestKey, row.Target)), contactport.HistoricalImportImported); err != nil {
		return err
	}
	result.Imported++
	return nil
}

func (service *ActiveRootService) processCustomer(ctx context.Context, command ActiveRootsCommand, row CustomerActiveRoot, result *ActiveRootsResult) error {
	fence, runID := command.Fence, command.Fence.RunID
	target := row.Target
	ownerUnresolved := false
	if len(row.OwnerStaffSourceKeyHMAC) != 0 {
		if err := service.contacts.LockHistoricalImportSource(ctx, contactport.HistoricalImportOwnerRoleMap, row.OwnerStaffSourceKeyHMAC); err != nil {
			return err
		}
		owner, found, err := service.contacts.LockHistoricalImportLineage(ctx, contactport.HistoricalImportOwnerRoleMap, row.OwnerStaffSourceKeyHMAC)
		if err != nil {
			return err
		}
		if found && owner.TargetID > 0 {
			active, activeErr := service.contacts.IsHistoricalImportActiveStaff(ctx, owner.TargetID)
			if activeErr != nil {
				return activeErr
			}
			if active {
				ownerID := owner.TargetID
				target.OwnerStaffID = &ownerID
			} else {
				ownerUnresolved = true
			}
		} else {
			ownerUnresolved = true
		}
	}
	receipt, replay, err := service.lockReceipt(ctx, runID, contactport.HistoricalImportCustomerIdentity, row.Source)
	if err != nil {
		return err
	}
	if replay {
		if receipt.Disposition == contactport.HistoricalImportQuarantined {
			if !hmac.Equal(receipt.FieldDigest, row.Source.FieldDigest) {
				return ErrSourcePayloadDrift
			}
			result.Replayed++
			return nil
		}
		if receipt.Disposition != contactport.HistoricalImportImported {
			return ErrActiveRootDrift
		}
		lineage, state, err := service.matchLineage(ctx, contactport.HistoricalImportCustomerIdentity, row.Source)
		if err != nil || state != lineageSamePayload {
			return driftOr(err)
		}
		current, err := service.contacts.LockHistoricalImportCustomerTarget(ctx, lineage.TargetID)
		if err != nil || !sameDigest(receipt.FieldDigest, customerTargetDigest(command.DigestKey, current)) || !sameDigest(receipt.FieldDigest, customerTargetDigest(command.DigestKey, target)) {
			return driftOr(err)
		}
		if err = service.contacts.ValidateHistoricalImportCustomer(ctx, lineage.TargetID, target); err != nil {
			return err
		}
		result.Replayed++
		return nil
	}
	lineage, state, err := service.matchLineage(ctx, contactport.HistoricalImportCustomerIdentity, row.Source)
	if err != nil {
		return err
	}
	if state == lineageChangedSource {
		current, lockErr := service.contacts.LockHistoricalImportCustomerTarget(ctx, lineage.TargetID)
		if lockErr != nil {
			return lockErr
		}
		if !sameDigest(lineage.FieldDigest, customerTargetDigest(command.DigestKey, current)) {
			return service.quarantineChangedSource(ctx, fence, contactport.HistoricalImportCustomerIdentity, row.Source, quarantineTargetDrift, result)
		}
		if err = service.contacts.UpdateHistoricalImportCustomerCAS(ctx, lineage.TargetID, current, target); err != nil {
			return err
		}
		if err = service.contacts.UpdateHistoricalImportLineageCAS(ctx, runID, contactport.HistoricalImportCustomerIdentity, row.Source, lineage); err != nil {
			return err
		}
		if ownerUnresolved {
			if err = service.contacts.AppendHistoricalImportQuarantine(ctx, contactport.HistoricalImportQuarantine{RunID: runID, Source: contactport.HistoricalImportCustomerIdentity, SourceFact: row.Source, ReasonCode: quarantineOwnerUnresolved}); err != nil {
				return err
			}
			result.Quarantined++
		}
		if err = service.contacts.AppendHistoricalImportRowReceipt(ctx, fence, contactport.HistoricalImportCustomerIdentity, withFieldDigest(row.Source, customerTargetDigest(command.DigestKey, target)), contactport.HistoricalImportImported); err != nil {
			return err
		}
		result.Updated++
		return nil
	}
	customerID := lineage.TargetID
	if state == lineageSamePayload {
		if err = service.contacts.ValidateHistoricalImportCustomer(ctx, customerID, target); err != nil {
			return err
		}
	} else {
		customerID, err = service.contacts.CreateHistoricalImportCustomer(ctx, target)
		if err != nil {
			return err
		}
		if customerID < 1 {
			return ErrActiveRootDrift
		}
		if err = service.contacts.AppendHistoricalImportLineage(ctx, runID, contactport.HistoricalImportCustomerIdentity, row.Source, customerID); err != nil {
			return err
		}
	}
	if ownerUnresolved {
		if err = service.contacts.AppendHistoricalImportQuarantine(ctx, contactport.HistoricalImportQuarantine{RunID: runID, Source: contactport.HistoricalImportCustomerIdentity, SourceFact: row.Source, ReasonCode: quarantineOwnerUnresolved}); err != nil {
			return err
		}
		result.Quarantined++
	}
	if err = service.contacts.AppendHistoricalImportRowReceipt(ctx, fence, contactport.HistoricalImportCustomerIdentity, withFieldDigest(row.Source, customerTargetDigest(command.DigestKey, target)), contactport.HistoricalImportImported); err != nil {
		return err
	}
	result.Imported++
	return nil
}

func (service *ActiveRootService) processIdentity(ctx context.Context, command ActiveRootsCommand, row ExternalIdentityActiveRoot, result *ActiveRootsResult) error {
	if err := service.contacts.LockHistoricalImportSource(ctx, contactport.HistoricalImportCustomerIdentity, row.CustomerSourceKeyHMAC); err != nil {
		return err
	}
	root, rootFound, err := service.contacts.LockHistoricalImportLineage(ctx, contactport.HistoricalImportCustomerIdentity, row.CustomerSourceKeyHMAC)
	if err != nil {
		return err
	}
	if rootFound {
		if root.TargetID < 1 {
			return ErrActiveRootDrift
		}
		if err = service.contacts.ValidateHistoricalImportCustomerRoot(ctx, root.TargetID); err != nil {
			return err
		}
	}
	receipt, replay, err := service.lockReceipt(ctx, command.Fence.RunID, contactport.HistoricalImportExternalIdentity, row.Source)
	if err != nil {
		return err
	}
	identity := identityport.HistoricalScopedIdentity{CustomerID: contactport.CustomerID(root.TargetID), Scope: "wecom-corp:" + command.CorpID, ExternalUserID: row.ExternalUserID, SourceKeyHMAC: row.Source.SourceKeyHMAC, HMACKeyVersion: command.HMACKeyVersion}
	if replay {
		switch receipt.Disposition {
		case contactport.HistoricalImportQuarantined:
			if !hmac.Equal(receipt.FieldDigest, row.Source.FieldDigest) {
				return ErrSourcePayloadDrift
			}
			result.Replayed++
			return nil
		case contactport.HistoricalImportImported:
			if !rootFound {
				return ErrActiveRootDrift
			}
			lineage, state, err := service.matchLineage(ctx, contactport.HistoricalImportExternalIdentity, row.Source)
			if err != nil || state != lineageSamePayload {
				return driftOr(err)
			}
			current, err := service.identities.LockHistoricalScopedWeComIdentity(ctx, lineage.TargetID, row.Source.SourceKeyHMAC)
			if err != nil || !sameDigest(receipt.FieldDigest, identityTargetDigest(command.DigestKey, current)) || !sameDigest(receipt.FieldDigest, identityTargetDigest(command.DigestKey, identity)) {
				return driftOr(err)
			}
			if err = service.identities.ValidateHistoricalScopedWeComIdentity(ctx, lineage.TargetID, identity); err != nil {
				return err
			}
			result.Replayed++
			return nil
		default:
			return ErrActiveRootDrift
		}
	}
	if !rootFound {
		return service.quarantineIdentity(ctx, command.Fence, row.Source, quarantineMissingCustomerRoot, result)
	}
	lineage, state, err := service.matchLineage(ctx, contactport.HistoricalImportExternalIdentity, row.Source)
	if err != nil {
		return err
	}
	if state == lineageChangedSource {
		current, lockErr := service.identities.LockHistoricalScopedWeComIdentity(ctx, lineage.TargetID, row.Source.SourceKeyHMAC)
		if lockErr != nil {
			return lockErr
		}
		if current.CustomerID != identity.CustomerID {
			return service.quarantineIdentity(ctx, command.Fence, row.Source, quarantineIdentityConflict, result)
		}
		if !sameDigest(lineage.FieldDigest, identityTargetDigest(command.DigestKey, current)) {
			return service.quarantineChangedSource(ctx, command.Fence, contactport.HistoricalImportExternalIdentity, row.Source, quarantineTargetDrift, result)
		}
		if err = service.identities.UpdateHistoricalScopedWeComIdentityCAS(ctx, lineage.TargetID, current, identity); errors.Is(err, identityport.ErrHistoricalScopedIdentityConflict) {
			return service.quarantineIdentity(ctx, command.Fence, row.Source, quarantineIdentityConflict, result)
		} else if err != nil {
			return err
		}
		if err = service.contacts.UpdateHistoricalImportLineageCAS(ctx, command.Fence.RunID, contactport.HistoricalImportExternalIdentity, row.Source, lineage); err != nil {
			return err
		}
		if err = service.contacts.AppendHistoricalImportRowReceipt(ctx, command.Fence, contactport.HistoricalImportExternalIdentity, withFieldDigest(row.Source, identityTargetDigest(command.DigestKey, identity)), contactport.HistoricalImportImported); err != nil {
			return err
		}
		result.Updated++
		return nil
	}
	if state == lineageSamePayload {
		if err = service.identities.ValidateHistoricalScopedWeComIdentity(ctx, lineage.TargetID, identity); err != nil {
			return err
		}
	} else {
		bound, bindErr := service.identities.BindHistoricalScopedWeComIdentity(ctx, identity)
		if errors.Is(bindErr, identityport.ErrHistoricalScopedIdentityConflict) {
			return service.quarantineIdentity(ctx, command.Fence, row.Source, quarantineIdentityConflict, result)
		}
		if bindErr != nil {
			return bindErr
		}
		if bound.IdentityID < 1 || !bound.Bound {
			return ErrActiveRootDrift
		}
		if err = service.contacts.AppendHistoricalImportLineage(ctx, command.Fence.RunID, contactport.HistoricalImportExternalIdentity, row.Source, bound.IdentityID); err != nil {
			return err
		}
	}
	if err = service.contacts.AppendHistoricalImportRowReceipt(ctx, command.Fence, contactport.HistoricalImportExternalIdentity, withFieldDigest(row.Source, identityTargetDigest(command.DigestKey, identity)), contactport.HistoricalImportImported); err != nil {
		return err
	}
	result.Imported++
	return nil
}

func (service *ActiveRootService) quarantineIdentity(ctx context.Context, fence contactport.NonActiveLeaseFence, source contactport.HistoricalImportSourceFact, reason string, result *ActiveRootsResult) error {
	runID := fence.RunID
	if err := service.contacts.AppendHistoricalImportQuarantine(ctx, contactport.HistoricalImportQuarantine{RunID: runID, Source: contactport.HistoricalImportExternalIdentity, SourceFact: source, ReasonCode: reason}); err != nil {
		return err
	}
	if err := service.contacts.AppendHistoricalImportRowReceipt(ctx, fence, contactport.HistoricalImportExternalIdentity, source, contactport.HistoricalImportQuarantined); err != nil {
		return err
	}
	result.Quarantined++
	return nil
}

func (service *ActiveRootService) quarantineChangedSource(ctx context.Context, fence contactport.NonActiveLeaseFence, sourceType contactport.HistoricalImportSource, source contactport.HistoricalImportSourceFact, reason string, result *ActiveRootsResult) error {
	runID := fence.RunID
	if err := service.contacts.AppendHistoricalImportQuarantine(ctx, contactport.HistoricalImportQuarantine{RunID: runID, Source: sourceType, SourceFact: source, ReasonCode: reason}); err != nil {
		return err
	}
	if err := service.contacts.AppendHistoricalImportRowReceipt(ctx, fence, sourceType, source, contactport.HistoricalImportQuarantined); err != nil {
		return err
	}
	result.Quarantined++
	result.ChangedSourceCandidates++
	return nil
}

func (service *ActiveRootService) lockReceipt(ctx context.Context, runID int64, source contactport.HistoricalImportSource, fact contactport.HistoricalImportSourceFact) (contactport.HistoricalImportRowReceipt, bool, error) {
	if err := service.contacts.LockHistoricalImportSource(ctx, source, fact.SourceKeyHMAC); err != nil {
		return contactport.HistoricalImportRowReceipt{}, false, err
	}
	receipt, found, err := service.contacts.FindHistoricalImportRowReceipt(ctx, runID, source, fact.SourceKeyHMAC)
	if err != nil || !found {
		return receipt, found, err
	}
	if !hmac.Equal(receipt.PayloadHMAC, fact.PayloadHMAC) {
		return contactport.HistoricalImportRowReceipt{}, false, ErrSourcePayloadDrift
	}
	return receipt, true, nil
}

func staffTargetDigest(key []byte, fact contactport.HistoricalImportStaffFact) []byte {
	return targetDigest(key, "owner_role_map", fact)
}
func customerTargetDigest(key []byte, fact contactport.HistoricalImportCustomerFact) []byte {
	return targetDigest(key, "crm_user_identity", fact)
}
func identityTargetDigest(key []byte, fact identityport.HistoricalScopedIdentity) []byte {
	return targetDigest(key, "wecom_external_contact_identity_map", fact)
}
func targetDigest(key []byte, table string, fact any) []byte {
	payload, err := json.Marshal(fact)
	if err != nil {
		return nil
	}
	digest, err := SourceFieldsHMAC(key, table, payload)
	if err != nil {
		return nil
	}
	return digest
}
func sameDigest(left, right []byte) bool { return len(left) == sha256.Size && hmac.Equal(left, right) }
func withFieldDigest(fact contactport.HistoricalImportSourceFact, digest []byte) contactport.HistoricalImportSourceFact {
	fact.FieldDigest = digest
	return fact
}

func (service *ActiveRootService) matchLineage(ctx context.Context, source contactport.HistoricalImportSource, fact contactport.HistoricalImportSourceFact) (contactport.HistoricalImportLineage, lineageState, error) {
	lineage, found, err := service.contacts.LockHistoricalImportLineage(ctx, source, fact.SourceKeyHMAC)
	if err != nil || !found {
		return lineage, lineageMissing, err
	}
	if lineage.TargetID < 1 {
		return contactport.HistoricalImportLineage{}, lineageMissing, ErrActiveRootDrift
	}
	if !hmac.Equal(lineage.PayloadHMAC, fact.PayloadHMAC) {
		return lineage, lineageChangedSource, nil
	}
	return lineage, lineageSamePayload, nil
}

func driftOr(err error) error {
	if err != nil {
		return err
	}
	return ErrActiveRootDrift
}

func validActiveRoots(command ActiveRootsCommand) bool {
	rowCount := len(command.Staff) + len(command.SkippedOwners) + len(command.Customers) + len(command.Identities)
	if command.Fence.RunID < 1 || command.Fence.Generation < 1 || len(command.Fence.TokenHMAC) != 32 || command.HMACKeyVersion < 1 || len(command.DigestKey) < 32 || command.CorpID == "" || strings.TrimSpace(command.CorpID) != command.CorpID || rowCount < 1 || rowCount > MaximumActiveRootBatchRows {
		return false
	}
	seen := map[contactport.HistoricalImportSource]map[string]bool{contactport.HistoricalImportOwnerRoleMap: {}, contactport.HistoricalImportCustomerIdentity: {}, contactport.HistoricalImportExternalIdentity: {}}
	for _, fact := range command.SkippedOwners {
		if !validSourceFact(fact) || !uniqueSource(seen, contactport.HistoricalImportOwnerRoleMap, fact.SourceKeyHMAC) {
			return false
		}
	}
	for _, row := range command.Staff {
		if !validSourceFact(row.Source) || !uniqueSource(seen, contactport.HistoricalImportOwnerRoleMap, row.Source.SourceKeyHMAC) || row.Target.WeComUserID == "" || strings.TrimSpace(row.Target.WeComUserID) != row.Target.WeComUserID || row.Target.Name == "" || strings.TrimSpace(row.Target.Name) != row.Target.Name || row.Target.CreatedAt.IsZero() || row.Target.UpdatedAt.IsZero() || row.Target.CreatedAt.After(row.Target.UpdatedAt) {
			return false
		}
	}
	for _, row := range command.Customers {
		if !validSourceFact(row.Source) || !uniqueSource(seen, contactport.HistoricalImportCustomerIdentity, row.Source.SourceKeyHMAC) || (len(row.OwnerStaffSourceKeyHMAC) != 0 && len(row.OwnerStaffSourceKeyHMAC) != 32) || row.Target.OwnerStaffID != nil || row.Target.FirstSeenAt.IsZero() || row.Target.LastSeenAt.IsZero() || row.Target.CreatedAt.IsZero() || row.Target.UpdatedAt.IsZero() || row.Target.FirstSeenAt.After(row.Target.LastSeenAt) || row.Target.CreatedAt.After(row.Target.UpdatedAt) {
			return false
		}
	}
	for _, row := range command.Identities {
		if !validSourceFact(row.Source) || !uniqueSource(seen, contactport.HistoricalImportExternalIdentity, row.Source.SourceKeyHMAC) || len(row.CustomerSourceKeyHMAC) != 32 || row.CorpID != command.CorpID || row.ExternalUserID == "" || strings.TrimSpace(row.ExternalUserID) != row.ExternalUserID {
			return false
		}
	}
	return true
}

func validSourceFact(fact contactport.HistoricalImportSourceFact) bool {
	return len(fact.SourceKeyHMAC) == 32 && len(fact.PayloadHMAC) == 32 && len(fact.FieldDigest) == 32
}

func uniqueSource(seen map[contactport.HistoricalImportSource]map[string]bool, source contactport.HistoricalImportSource, key []byte) bool {
	encoded := string(key)
	if seen[source][encoded] {
		return false
	}
	seen[source][encoded] = true
	return true
}
