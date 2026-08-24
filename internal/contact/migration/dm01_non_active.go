package migration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"errors"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrInvalidNonActive = errors.New("invalid DM01 non-active rows")
	ErrNonActiveDrift   = errors.New("DM01 non-active row drift")
)

const MaximumNonActiveBatchRows = 500

type NonActiveRow struct {
	Source         contactport.NonActiveSource
	Fact           contactport.HistoricalImportSourceFact
	ArchivePayload []byte
}

type NonActiveCommand struct {
	Fence             contactport.NonActiveLeaseFence
	ArchiveKey        []byte
	ArchiveKeyVersion int16
	PayloadHMACKey    []byte
	Rows              []NonActiveRow
}

type NonActiveResult struct {
	Archived    int
	Skipped     int
	Quarantined int
	Replayed    int
}

type NonActiveService struct {
	uow    platformport.UnitOfWork
	target contactport.NonActiveTarget
}

func NewNonActiveService(uow platformport.UnitOfWork, target contactport.NonActiveTarget) *NonActiveService {
	return &NonActiveService{uow: uow, target: target}
}

func (service *NonActiveService) Process(ctx context.Context, command NonActiveCommand) (NonActiveResult, error) {
	if service == nil || service.uow == nil || service.target == nil || ctx == nil || !validNonActiveCommand(command) {
		return NonActiveResult{}, ErrInvalidNonActive
	}
	var result NonActiveResult
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		if err := service.target.AssertNonActiveLease(txCtx, command.Fence); err != nil {
			return err
		}
		for _, row := range command.Rows {
			if err := service.processRow(txCtx, command, row, &result); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return NonActiveResult{}, err
	}
	return result, nil
}

func (service *NonActiveService) processRow(ctx context.Context, command NonActiveCommand, row NonActiveRow, result *NonActiveResult) error {
	disposition, reason, archive := nonActivePolicy(row.Source)
	if err := service.target.LockNonActiveSource(ctx, row.Source, row.Fact.SourceKeyHMAC); err != nil {
		return err
	}
	receipt, found, err := service.target.FindNonActiveReceipt(ctx, command.Fence.RunID, row.Source, row.Fact.SourceKeyHMAC)
	if err != nil {
		return err
	}
	if found {
		if receipt.Disposition != disposition || !hmac.Equal(receipt.PayloadHMAC, row.Fact.PayloadHMAC) || !hmac.Equal(receipt.FieldDigest, row.Fact.FieldDigest) {
			return ErrNonActiveDrift
		}
		if err := service.validateCompanion(ctx, command, row, reason, archive); err != nil {
			return err
		}
		result.Replayed++
		return nil
	}
	if archive {
		aad, err := ArchiveAAD(command.Fence.RunID, nonActiveSourceTable(row.Source), row.Fact.SourceKeyHMAC, row.Fact.PayloadHMAC, row.Fact.FieldDigest, int(command.ArchiveKeyVersion))
		if err != nil {
			return err
		}
		nonce, ciphertext, err := EncryptArchiveBound(command.ArchiveKey, aad, row.ArchivePayload)
		if err != nil {
			return err
		}
		if err = service.target.AppendNonActiveArchive(ctx, contactport.NonActiveArchive{RunID: command.Fence.RunID, Source: row.Source, SourceFact: row.Fact, Nonce: nonce, Ciphertext: ciphertext, KeyVersion: command.ArchiveKeyVersion}); err != nil {
			return err
		}
		result.Archived++
	} else if reason != "" {
		if err = service.target.AppendNonActiveQuarantine(ctx, contactport.NonActiveQuarantine{RunID: command.Fence.RunID, Source: row.Source, SourceFact: row.Fact, ReasonCode: reason}); err != nil {
			return err
		}
		result.Quarantined++
	} else {
		result.Skipped++
	}
	// This must remain the final write for a row. The repository performs the
	// lease generation/token/expiry predicate in the INSERT itself.
	if err = service.target.AppendNonActiveReceipt(ctx, command.Fence, row.Source, row.Fact, disposition); err != nil {
		return err
	}
	return nil
}

func (service *NonActiveService) validateCompanion(ctx context.Context, command NonActiveCommand, row NonActiveRow, reason string, archive bool) error {
	if archive {
		stored, found, err := service.target.FindNonActiveArchive(ctx, command.Fence.RunID, row.Source, row.Fact.SourceKeyHMAC)
		if err != nil {
			return err
		}
		if !found || stored.Source != row.Source || stored.KeyVersion != command.ArchiveKeyVersion || !sameSourceFact(stored.SourceFact, row.Fact) {
			return ErrNonActiveDrift
		}
		aad, err := ArchiveAAD(command.Fence.RunID, nonActiveSourceTable(row.Source), row.Fact.SourceKeyHMAC, row.Fact.PayloadHMAC, row.Fact.FieldDigest, int(stored.KeyVersion))
		if err != nil {
			return err
		}
		plain, err := DecryptArchiveBound(command.ArchiveKey, command.PayloadHMACKey, nonActiveSourceTable(row.Source), aad, stored.Nonce, stored.Ciphertext, row.Fact.PayloadHMAC)
		if err != nil || !bytes.Equal(plain, row.ArchivePayload) {
			return ErrNonActiveDrift
		}
		return nil
	}
	if reason != "" {
		stored, found, err := service.target.FindNonActiveQuarantine(ctx, command.Fence.RunID, row.Source, row.Fact.SourceKeyHMAC)
		if err != nil {
			return err
		}
		if !found || stored.Source != row.Source || stored.ReasonCode != reason || !sameSourceFact(stored.SourceFact, row.Fact) {
			return ErrNonActiveDrift
		}
	}
	return nil
}

func validNonActiveCommand(command NonActiveCommand) bool {
	if command.Fence.RunID < 1 || command.Fence.Generation < 1 || len(command.Fence.TokenHMAC) != 32 || len(command.Rows) == 0 || len(command.Rows) > MaximumNonActiveBatchRows {
		return false
	}
	seen := make(map[string]struct{}, len(command.Rows))
	for _, row := range command.Rows {
		_, _, archive := nonActivePolicy(row.Source)
		if nonActiveSourceTable(row.Source) == "" || len(row.Fact.SourceKeyHMAC) != 32 || len(row.Fact.PayloadHMAC) != 32 || len(row.Fact.FieldDigest) != 32 || archive != (len(row.ArchivePayload) > 0) {
			return false
		}
		key := nonActiveSourceTable(row.Source) + ":" + string(row.Fact.SourceKeyHMAC)
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
		if archive && (len(command.ArchiveKey) != 32 || len(command.PayloadHMACKey) == 0 || command.ArchiveKeyVersion < 1) {
			return false
		}
		if archive {
			payloadHMAC, err := SourcePayloadHMAC(command.PayloadHMACKey, nonActiveSourceTable(row.Source), row.ArchivePayload)
			if err != nil || !hmac.Equal(payloadHMAC, row.Fact.PayloadHMAC) {
				return false
			}
		}
	}
	return true
}

func nonActivePolicy(source contactport.NonActiveSource) (contactport.NonActiveDisposition, string, bool) {
	switch source {
	case contactport.NonActiveMergeAudit, contactport.NonActiveResolutionQueue:
		return contactport.NonActiveArchived, "", true
	case contactport.NonActiveIdentityConflicts, contactport.NonActivePeople:
		return contactport.NonActiveQuarantined, "target_schema_deferred", false
	case contactport.NonActiveFollowUsers:
		return contactport.NonActiveQuarantined, "multiple_follow_users_deferred", false
	case contactport.NonActiveContacts, contactport.NonActiveDirectoryMembers, contactport.NonActiveExternalBindings:
		return contactport.NonActiveSkipped, "", false
	default:
		return 0, "", false
	}
}

func nonActiveSourceTable(source contactport.NonActiveSource) string {
	switch source {
	case contactport.NonActiveMergeAudit:
		return "crm_user_identity_merge_audit"
	case contactport.NonActiveResolutionQueue:
		return "crm_user_identity_resolution_queue"
	case contactport.NonActiveContacts:
		return "contacts"
	case contactport.NonActiveIdentityConflicts:
		return "crm_user_identity_conflicts"
	case contactport.NonActivePeople:
		return "people"
	case contactport.NonActiveFollowUsers:
		return "wecom_external_contact_follow_users"
	case contactport.NonActiveDirectoryMembers:
		return "admin_wecom_directory_members"
	case contactport.NonActiveExternalBindings:
		return "external_contact_bindings"
	default:
		return ""
	}
}

func sameSourceFact(left, right contactport.HistoricalImportSourceFact) bool {
	return hmac.Equal(left.SourceKeyHMAC, right.SourceKeyHMAC) && hmac.Equal(left.PayloadHMAC, right.PayloadHMAC) && hmac.Equal(left.FieldDigest, right.FieldDigest)
}
