package migration

import (
	"context"
	"crypto/hmac"
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
	quarantineChangedSource       = "changed_source_candidate"
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
	RunID          int64
	CorpID         string
	HMACKeyVersion int16
	Staff          []StaffActiveRoot
	Customers      []CustomerActiveRoot
	Identities     []ExternalIdentityActiveRoot
}

type ActiveRootsResult struct {
	Imported                int
	Replayed                int
	Quarantined             int
	ChangedSourceCandidates int
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
		for _, row := range command.Staff {
			if err := service.processStaff(txCtx, command.RunID, row, &result); err != nil {
				return err
			}
		}
		for _, row := range command.Customers {
			if err := service.processCustomer(txCtx, command.RunID, row, &result); err != nil {
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

func (service *ActiveRootService) processStaff(ctx context.Context, runID int64, row StaffActiveRoot, result *ActiveRootsResult) error {
	receipt, replay, err := service.lockReceipt(ctx, runID, contactport.HistoricalImportOwnerRoleMap, row.Source)
	if err != nil {
		return err
	}
	if replay {
		if receipt.Disposition == contactport.HistoricalImportQuarantined {
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
		return service.quarantineChangedSource(ctx, runID, contactport.HistoricalImportOwnerRoleMap, row.Source, result)
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
	if err = service.contacts.AppendHistoricalImportRowReceipt(ctx, runID, contactport.HistoricalImportOwnerRoleMap, row.Source, contactport.HistoricalImportImported); err != nil {
		return err
	}
	result.Imported++
	return nil
}

func (service *ActiveRootService) processCustomer(ctx context.Context, runID int64, row CustomerActiveRoot, result *ActiveRootsResult) error {
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
		return service.quarantineChangedSource(ctx, runID, contactport.HistoricalImportCustomerIdentity, row.Source, result)
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
	if err = service.contacts.AppendHistoricalImportRowReceipt(ctx, runID, contactport.HistoricalImportCustomerIdentity, row.Source, contactport.HistoricalImportImported); err != nil {
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
	receipt, replay, err := service.lockReceipt(ctx, command.RunID, contactport.HistoricalImportExternalIdentity, row.Source)
	if err != nil {
		return err
	}
	identity := identityport.HistoricalScopedIdentity{CustomerID: contactport.CustomerID(root.TargetID), Scope: "wecom-corp:" + command.CorpID, ExternalUserID: row.ExternalUserID, SourceKeyHMAC: row.Source.SourceKeyHMAC, HMACKeyVersion: command.HMACKeyVersion}
	if replay {
		switch receipt.Disposition {
		case contactport.HistoricalImportQuarantined:
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
		return service.quarantineIdentity(ctx, command.RunID, row.Source, quarantineMissingCustomerRoot, result)
	}
	lineage, state, err := service.matchLineage(ctx, contactport.HistoricalImportExternalIdentity, row.Source)
	if err != nil {
		return err
	}
	if state == lineageChangedSource {
		return service.quarantineChangedSource(ctx, command.RunID, contactport.HistoricalImportExternalIdentity, row.Source, result)
	}
	if state == lineageSamePayload {
		if err = service.identities.ValidateHistoricalScopedWeComIdentity(ctx, lineage.TargetID, identity); err != nil {
			return err
		}
	} else {
		bound, bindErr := service.identities.BindHistoricalScopedWeComIdentity(ctx, identity)
		if errors.Is(bindErr, identityport.ErrHistoricalScopedIdentityConflict) {
			return service.quarantineIdentity(ctx, command.RunID, row.Source, quarantineIdentityConflict, result)
		}
		if bindErr != nil {
			return bindErr
		}
		if bound.IdentityID < 1 || !bound.Bound {
			return ErrActiveRootDrift
		}
		if err = service.contacts.AppendHistoricalImportLineage(ctx, command.RunID, contactport.HistoricalImportExternalIdentity, row.Source, bound.IdentityID); err != nil {
			return err
		}
	}
	if err = service.contacts.AppendHistoricalImportRowReceipt(ctx, command.RunID, contactport.HistoricalImportExternalIdentity, row.Source, contactport.HistoricalImportImported); err != nil {
		return err
	}
	result.Imported++
	return nil
}

func (service *ActiveRootService) quarantineIdentity(ctx context.Context, runID int64, source contactport.HistoricalImportSourceFact, reason string, result *ActiveRootsResult) error {
	if err := service.contacts.AppendHistoricalImportQuarantine(ctx, contactport.HistoricalImportQuarantine{RunID: runID, Source: contactport.HistoricalImportExternalIdentity, SourceFact: source, ReasonCode: reason}); err != nil {
		return err
	}
	if err := service.contacts.AppendHistoricalImportRowReceipt(ctx, runID, contactport.HistoricalImportExternalIdentity, source, contactport.HistoricalImportQuarantined); err != nil {
		return err
	}
	result.Quarantined++
	return nil
}

func (service *ActiveRootService) quarantineChangedSource(ctx context.Context, runID int64, sourceType contactport.HistoricalImportSource, source contactport.HistoricalImportSourceFact, result *ActiveRootsResult) error {
	if err := service.contacts.AppendHistoricalImportQuarantine(ctx, contactport.HistoricalImportQuarantine{RunID: runID, Source: sourceType, SourceFact: source, ReasonCode: quarantineChangedSource}); err != nil {
		return err
	}
	if err := service.contacts.AppendHistoricalImportRowReceipt(ctx, runID, sourceType, source, contactport.HistoricalImportQuarantined); err != nil {
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
	if !hmac.Equal(receipt.PayloadHMAC, fact.PayloadHMAC) || !hmac.Equal(receipt.FieldDigest, fact.FieldDigest) {
		return contactport.HistoricalImportRowReceipt{}, false, ErrSourcePayloadDrift
	}
	return receipt, true, nil
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
	rowCount := len(command.Staff) + len(command.Customers) + len(command.Identities)
	if command.RunID < 1 || command.HMACKeyVersion < 1 || command.CorpID == "" || strings.TrimSpace(command.CorpID) != command.CorpID || rowCount < 1 || rowCount > MaximumActiveRootBatchRows {
		return false
	}
	seen := map[contactport.HistoricalImportSource]map[string]bool{contactport.HistoricalImportOwnerRoleMap: {}, contactport.HistoricalImportCustomerIdentity: {}, contactport.HistoricalImportExternalIdentity: {}}
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
