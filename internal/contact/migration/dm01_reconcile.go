package migration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrInvalidReconcile  = errors.New("invalid DM01 reconcile")
	ErrReconcileMismatch = errors.New("DM01 reconcile mismatch")
)

type ReconcileTable uint8

const (
	ReconcileOwnerRoleMap ReconcileTable = iota + 1
	ReconcileCustomerIdentity
	ReconcileExternalIdentity
	ReconcileMergeAudit
	ReconcileResolutionQueue
	ReconcileDirectoryMembers
	ReconcileContacts
	ReconcileIdentityConflicts
	ReconcileExternalBindings
	ReconcilePeople
	ReconcileFollowUsers
)

var reconcileTableOrder = [...]ReconcileTable{ReconcileOwnerRoleMap, ReconcileCustomerIdentity, ReconcileExternalIdentity, ReconcileMergeAudit, ReconcileResolutionQueue, ReconcileDirectoryMembers, ReconcileContacts, ReconcileIdentityConflicts, ReconcileExternalBindings, ReconcilePeople, ReconcileFollowUsers}

type ReconcileSourceSummary struct {
	Table  ReconcileTable
	Count  int64
	Digest []byte
}

type ReconcileReceipt struct {
	SourceFact      contactport.HistoricalImportSourceFact
	Disposition     string
	ArchiveCount    int64
	QuarantineCount int64
}

type ReconcileCompanionCounts struct{ Archives, Quarantines int64 }

type ReconcileArchive struct {
	SourceFact contactport.HistoricalImportSourceFact
	Nonce      []byte
	Ciphertext []byte
	KeyVersion int16
}

type ReconcileTarget interface {
	LockReconcileRun(context.Context, contactport.NonActiveLeaseFence) (int64, error)
	StreamReconcileReceipts(context.Context, int64, ReconcileTable, func(ReconcileReceipt) error) error
	CountReconcileCompanions(context.Context, int64, ReconcileTable) (ReconcileCompanionCounts, error)
	ReadReconcileArchive(context.Context, int64, ReconcileTable, []byte) (ReconcileArchive, bool, error)
	AppendReconcileResult(context.Context, contactport.NonActiveLeaseFence, []byte) error
	CompleteReconcileRun(context.Context, contactport.NonActiveLeaseFence) error
}

type ReconcileCommand struct {
	Fence      contactport.NonActiveLeaseFence
	Sources    []ReconcileSourceSummary
	ArchiveKey []byte
	HMACKey    []byte
}

type ReconcileDispositionCounts struct{ Imported, Quarantined, Archived, Skipped int64 }
type ReconcileResult struct {
	ParentRunID, SourceRows int64
	Dispositions            ReconcileDispositionCounts
	ResultDigest            []byte
}

type ReconcileService struct {
	uow    platformport.UnitOfWork
	target ReconcileTarget
}

func NewReconcileService(uow platformport.UnitOfWork, target ReconcileTarget) *ReconcileService {
	return &ReconcileService{uow: uow, target: target}
}

func (service *ReconcileService) Reconcile(ctx context.Context, command ReconcileCommand) (ReconcileResult, error) {
	sources, ok := validReconcileCommand(command)
	if service == nil || service.uow == nil || service.target == nil || ctx == nil || !ok {
		return ReconcileResult{}, ErrInvalidReconcile
	}
	var result ReconcileResult
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		parentRunID, err := service.target.LockReconcileRun(txCtx, command.Fence)
		if err != nil {
			return err
		}
		if parentRunID < 1 {
			return ErrReconcileMismatch
		}
		result.ParentRunID = parentRunID
		resultHash := sha256.New()
		resultHash.Write([]byte("dm01-reconcile-result-v1"))
		var runIDs [16]byte
		binary.BigEndian.PutUint64(runIDs[:8], uint64(command.Fence.RunID))
		binary.BigEndian.PutUint64(runIDs[8:], uint64(parentRunID))
		resultHash.Write(runIDs[:])
		for _, table := range reconcileTableOrder {
			source := sources[table]
			digest := NewReconcileDigest()
			var counts ReconcileDispositionCounts
			var archives, quarantines int64
			err = service.target.StreamReconcileReceipts(txCtx, parentRunID, table, func(receipt ReconcileReceipt) error {
				if err := digest.Add(receipt.SourceFact); err != nil || !validReconcileCompanion(table, receipt) {
					return ErrReconcileMismatch
				}
				if receipt.Disposition == "archived" {
					archive, found, readErr := service.target.ReadReconcileArchive(txCtx, parentRunID, table, receipt.SourceFact.SourceKeyHMAC)
					if readErr != nil {
						return readErr
					}
					if !found || archive.KeyVersion < 1 || !sameSourceFact(archive.SourceFact, receipt.SourceFact) {
						return ErrReconcileMismatch
					}
					aad, aadErr := ArchiveAAD(parentRunID, ReconcileTableName(table), receipt.SourceFact.SourceKeyHMAC, receipt.SourceFact.PayloadHMAC, receipt.SourceFact.FieldDigest, int(archive.KeyVersion))
					if aadErr != nil {
						return ErrReconcileMismatch
					}
					plaintext, decryptErr := DecryptArchiveBound(command.ArchiveKey, command.HMACKey, ReconcileTableName(table), aad, archive.Nonce, archive.Ciphertext, receipt.SourceFact.PayloadHMAC)
					for index := range plaintext {
						plaintext[index] = 0
					}
					if decryptErr != nil {
						return ErrReconcileMismatch
					}
				}
				archives += receipt.ArchiveCount
				quarantines += receipt.QuarantineCount
				switch receipt.Disposition {
				case "imported":
					counts.Imported++
				case "quarantined":
					counts.Quarantined++
				case "archived":
					counts.Archived++
				case "skipped":
					counts.Skipped++
				default:
					return ErrReconcileMismatch
				}
				return nil
			})
			if err != nil {
				return err
			}
			gotDigest := digest.Sum()
			if digest.Count() != source.Count || !hmac.Equal(gotDigest, source.Digest) {
				return ErrReconcileMismatch
			}
			companions, err := service.target.CountReconcileCompanions(txCtx, parentRunID, table)
			if err != nil {
				return err
			}
			if companions.Archives != archives || companions.Quarantines != quarantines {
				return ErrReconcileMismatch
			}
			writeReconcileResultTable(resultHash, table, source.Count, counts, gotDigest)
			result.SourceRows += source.Count
			result.Dispositions.Imported += counts.Imported
			result.Dispositions.Quarantined += counts.Quarantined
			result.Dispositions.Archived += counts.Archived
			result.Dispositions.Skipped += counts.Skipped
		}
		result.ResultDigest = resultHash.Sum(nil)
		if err = service.target.AppendReconcileResult(txCtx, command.Fence, result.ResultDigest); err != nil {
			return err
		}
		return service.target.CompleteReconcileRun(txCtx, command.Fence)
	})
	if err != nil {
		return ReconcileResult{}, err
	}
	return result, nil
}

type ReconcileDigest struct {
	stream hash.Hash
	count  int64
}

func NewReconcileDigest() *ReconcileDigest {
	stream := sha256.New()
	stream.Write([]byte("dm01-reconcile-stream-v2"))
	return &ReconcileDigest{stream: stream}
}
func (digest *ReconcileDigest) Add(fact contactport.HistoricalImportSourceFact) error {
	if digest == nil || digest.stream == nil || !validSourceFact(fact) {
		return ErrInvalidReconcile
	}
	digest.count++
	var ordinal [8]byte
	binary.BigEndian.PutUint64(ordinal[:], uint64(digest.count))
	digest.stream.Write(ordinal[:])
	digest.stream.Write(fact.SourceKeyHMAC)
	digest.stream.Write(fact.PayloadHMAC)
	return nil
}
func (digest *ReconcileDigest) Count() int64 {
	if digest == nil || digest.stream == nil {
		return 0
	}
	return digest.count
}
func (digest *ReconcileDigest) Sum() []byte {
	if digest == nil {
		return nil
	}
	output := sha256.New()
	output.Write([]byte("dm01-reconcile-source-v2"))
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(digest.count))
	output.Write(count[:])
	output.Write(digest.stream.Sum(nil))
	return output.Sum(nil)
}

func validReconcileCommand(command ReconcileCommand) (map[ReconcileTable]ReconcileSourceSummary, bool) {
	if command.Fence.RunID < 1 || command.Fence.Generation < 1 || len(command.Fence.TokenHMAC) != sha256.Size || len(command.Sources) != len(reconcileTableOrder) || len(command.ArchiveKey) != 32 || len(command.HMACKey) < 32 {
		return nil, false
	}
	result := make(map[ReconcileTable]ReconcileSourceSummary, len(command.Sources))
	for _, source := range command.Sources {
		if ReconcileTableName(source.Table) == "" || source.Count < 0 || len(source.Digest) != sha256.Size {
			return nil, false
		}
		if _, exists := result[source.Table]; exists {
			return nil, false
		}
		result[source.Table] = source
	}
	return result, len(result) == len(reconcileTableOrder)
}

func validReconcileCompanion(table ReconcileTable, receipt ReconcileReceipt) bool {
	if !validSourceFact(receipt.SourceFact) || receipt.ArchiveCount < 0 || receipt.QuarantineCount < 0 || !validReconcileDisposition(table, receipt.Disposition) {
		return false
	}
	switch receipt.Disposition {
	case "archived":
		return receipt.ArchiveCount == 1 && receipt.QuarantineCount == 0 && (table == ReconcileMergeAudit || table == ReconcileResolutionQueue)
	case "quarantined":
		return receipt.ArchiveCount == 0 && receipt.QuarantineCount == 1
	case "skipped":
		return receipt.ArchiveCount == 0 && receipt.QuarantineCount == 0
	case "imported":
		return receipt.ArchiveCount == 0 && (receipt.QuarantineCount == 0 || (table == ReconcileCustomerIdentity && receipt.QuarantineCount == 1))
	default:
		return false
	}
}

func validReconcileDisposition(table ReconcileTable, disposition string) bool {
	switch table {
	case ReconcileOwnerRoleMap:
		return disposition == "imported" || disposition == "quarantined" || disposition == "skipped"
	case ReconcileCustomerIdentity, ReconcileExternalIdentity:
		return disposition == "imported" || disposition == "quarantined"
	case ReconcileMergeAudit, ReconcileResolutionQueue:
		return disposition == "archived"
	case ReconcileDirectoryMembers, ReconcileContacts, ReconcileExternalBindings:
		return disposition == "skipped"
	case ReconcileIdentityConflicts, ReconcilePeople, ReconcileFollowUsers:
		return disposition == "quarantined"
	default:
		return false
	}
}

func writeReconcileResultTable(target hash.Hash, table ReconcileTable, count int64, dispositions ReconcileDispositionCounts, digest []byte) {
	target.Write([]byte{byte(table)})
	values := [...]int64{count, dispositions.Imported, dispositions.Quarantined, dispositions.Archived, dispositions.Skipped}
	var encoded [8]byte
	for _, value := range values {
		binary.BigEndian.PutUint64(encoded[:], uint64(value))
		target.Write(encoded[:])
	}
	target.Write(digest)
}

func ReconcileTableName(table ReconcileTable) string {
	switch table {
	case ReconcileOwnerRoleMap:
		return "owner_role_map"
	case ReconcileCustomerIdentity:
		return "crm_user_identity"
	case ReconcileExternalIdentity:
		return "wecom_external_contact_identity_map"
	case ReconcileMergeAudit:
		return "crm_user_identity_merge_audit"
	case ReconcileResolutionQueue:
		return "crm_user_identity_resolution_queue"
	case ReconcileDirectoryMembers:
		return "admin_wecom_directory_members"
	case ReconcileContacts:
		return "contacts"
	case ReconcileIdentityConflicts:
		return "crm_user_identity_conflicts"
	case ReconcileExternalBindings:
		return "external_contact_bindings"
	case ReconcilePeople:
		return "people"
	case ReconcileFollowUsers:
		return "wecom_external_contact_follow_users"
	default:
		return ""
	}
}
