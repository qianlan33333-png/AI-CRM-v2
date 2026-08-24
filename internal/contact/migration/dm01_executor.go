package migration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const executorPageSize = 500

var ErrInvalidExecutor = errors.New("invalid DM01 executor")

type RunReservation struct {
	ManifestDigest []byte
	RepositorySHA  string
	SnapshotID     string
	ParentRunID    int64
	Mode           RunMode
	UpperWatermark time.Time
	HMACKeyVersion int16
}

type RunFence struct {
	RunID      int64
	Generation int64
	TokenHMAC  []byte
}

type ReservedRun struct {
	ID, Generation int64
	State          string
}

type FinalCheckpoint struct {
	Table           string
	FinalFact       contactport.HistoricalImportSourceFact
	Watermark       time.Time
	UpperKeyHMAC    []byte
	UpperBoundEmpty bool
}

type RunStore interface {
	ReserveRun(context.Context, RunReservation) (ReservedRun, error)
	ClaimRun(context.Context, int64, int64, []byte, time.Time) (int64, error)
	RenewRun(context.Context, RunFence, time.Time) error
	TransitionRun(context.Context, RunFence, string) error
	AppendCheckpoint(context.Context, RunFence, FinalCheckpoint) error
}

type executionTarget interface {
	contactport.HistoricalImportTarget
	contactport.NonActiveTarget
	ReconcileTarget
}

type ExecuteCommand struct {
	Manifest       Manifest
	ManifestDigest []byte
	Mode           RunMode
	ParentRunID    int64
	HMACKey        []byte
	ArchiveKey     []byte
}

type ExecuteResult struct {
	RunID      int64
	Generation int64
	State      string
}

type Executor struct {
	source    SourceReader
	uow       platformport.UnitOfWork
	runs      RunStore
	active    *ActiveRootService
	nonActive *NonActiveService
	reconcile *ReconcileService
	now       func() time.Time
	random    io.Reader
}

func NewExecutor(source SourceReader, uow platformport.UnitOfWork, runs RunStore, contacts executionTarget, identities identityport.HistoricalScopedIdentityBinder) *Executor {
	return &Executor{source: source, uow: uow, runs: runs, active: NewActiveRootService(uow, contacts, identities), nonActive: NewNonActiveService(uow, contacts), reconcile: NewReconcileService(uow, contacts), now: time.Now, random: rand.Reader}
}

func (executor *Executor) Execute(ctx context.Context, command ExecuteCommand) (ExecuteResult, error) {
	if executor == nil || executor.source == nil || executor.uow == nil || executor.runs == nil || executor.active == nil || executor.nonActive == nil || executor.reconcile == nil || ctx == nil || command.Manifest.Valid() != nil || !validManifestMode(command.Manifest, command.Mode) || len(command.ManifestDigest) != sha256.Size || len(command.HMACKey) < 32 || len(command.ArchiveKey) != 32 || command.Manifest.HMACKeyVersion > 32767 || (command.Mode == ModeReconcile) != (command.ParentRunID > 0) {
		return ExecuteResult{}, ErrInvalidExecutor
	}
	var result ExecuteResult
	err := executor.source.WithSnapshot(ctx, command.Manifest, func(snapshot SourceSnapshot) error {
		bounds, upper, err := validateBounds(snapshot.Bounds())
		if err != nil {
			return err
		}
		reservation := RunReservation{ManifestDigest: command.ManifestDigest, RepositorySHA: command.Manifest.LegacyRepositorySHA, SnapshotID: command.Manifest.SnapshotID, ParentRunID: command.ParentRunID, Mode: command.Mode, UpperWatermark: upper, HMACKeyVersion: int16(command.Manifest.HMACKeyVersion)}
		var reserved ReservedRun
		err = executor.withTargetOnly(ctx, func(txCtx context.Context) error {
			var reserveErr error
			reserved, reserveErr = executor.runs.ReserveRun(txCtx, reservation)
			result.RunID = reserved.ID
			return reserveErr
		})
		if err != nil {
			return err
		}
		terminal := map[RunMode]string{ModePreflight: "preflighted", ModeFull: "imported", ModeIncremental: "imported", ModeReconcile: "reconciled"}[command.Mode]
		if reserved.State == terminal {
			result.State, result.Generation = terminal, reserved.Generation
			return nil
		}
		token := make([]byte, 32)
		if _, err = io.ReadFull(executor.random, token); err != nil {
			return err
		}
		tokenHMAC, err := PayloadHMAC(command.HMACKey, token)
		if err != nil {
			return err
		}
		fence := RunFence{RunID: result.RunID, TokenHMAC: tokenHMAC}
		err = executor.withTargetOnly(ctx, func(txCtx context.Context) error {
			generation, claimErr := executor.runs.ClaimRun(txCtx, fence.RunID, reserved.Generation, fence.TokenHMAC, executor.now().Add(2*time.Minute))
			fence.Generation = generation
			result.Generation = generation
			return claimErr
		})
		if err != nil {
			return err
		}
		if command.Mode == ModePreflight {
			if reserved.State != "reserved" {
				return ErrInvalidExecutor
			}
			err = executor.transition(ctx, fence, "preflighted")
			result.State = "preflighted"
			return err
		}
		if command.Mode == ModeReconcile {
			if reserved.State == "reserved" {
				err = executor.transition(ctx, fence, "reconciling")
			} else if reserved.State != "reconciling" {
				err = ErrInvalidExecutor
			}
			if err != nil {
				return err
			}
			if err = executor.runReconcile(ctx, snapshot, bounds, fence, command); err == nil {
				result.State = "reconciled"
			} else {
				_ = executor.transition(ctx, fence, "failed")
			}
			return err
		}
		if reserved.State == "reserved" {
			if err = executor.transition(ctx, fence, "preflighted"); err != nil {
				return err
			}
			reserved.State = "preflighted"
		}
		if reserved.State == "preflighted" {
			if err = executor.transition(ctx, fence, "importing"); err != nil {
				return err
			}
		} else if reserved.State != "importing" {
			return ErrInvalidExecutor
		}
		if err = executor.runImport(ctx, snapshot, bounds, fence, command); err != nil {
			_ = executor.transition(ctx, fence, "failed")
			return err
		}
		err = executor.transition(ctx, fence, "imported")
		result.State = "imported"
		return err
	})
	return result, err
}

func validManifestMode(manifest Manifest, mode RunMode) bool {
	if len(manifest.Tables) == 0 {
		return false
	}
	declared := manifest.Tables[0].Mode
	for _, table := range manifest.Tables {
		if table.Mode != declared {
			return false
		}
	}
	return (mode == ModeFull && declared == "full") || (mode == ModeIncremental && declared == "incremental") || mode == ModePreflight || mode == ModeReconcile
}

func (executor *Executor) runImport(ctx context.Context, snapshot SourceSnapshot, bounds map[string]SourceUpperBound, fence RunFence, command ExecuteCommand) error {
	allowlist := make(map[string]bool, len(command.Manifest.OwnerAllowlistHMACs))
	for _, value := range command.Manifest.OwnerAllowlistHMACs {
		allowlist[value] = false
	}
	staff := make([]StaffActiveRoot, 0, executorPageSize)
	skippedOwners := make([]contactport.HistoricalImportSourceFact, 0, executorPageSize)
	tracker := newTableTracker("owner_role_map", bounds["owner_role_map"], command.HMACKey)
	err := snapshot.EachOwnerRoleMap(ctx, bounds[tracker.table], func(row OwnerRoleMapRow) error {
		ownerHMAC, ownerErr := OwnerAllowlistHMAC(command.HMACKey, row.UserID)
		if ownerErr != nil {
			return ownerErr
		}
		ownerDigest := hex.EncodeToString(ownerHMAC)
		if _, ok := allowlist[ownerDigest]; ok {
			if !row.Active {
				return fmt.Errorf("%w: allowlisted owner is inactive", ErrInvalidExecutor)
			}
			allowlist[ownerDigest] = true
		}
		fact, err := tracker.add(row.UserID, row.Payload)
		if err != nil {
			return err
		}
		if !allowlist[ownerDigest] {
			skippedOwners = append(skippedOwners, fact)
		} else {
			staff = append(staff, StaffActiveRoot{Source: fact, Target: contactport.HistoricalImportStaffFact{WeComUserID: row.UserID, Name: row.DisplayName, Active: row.Active, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}})
		}
		if len(staff)+len(skippedOwners) == executorPageSize {
			if err = executor.renew(ctx, fence); err == nil {
				_, err = executor.active.Process(ctx, ActiveRootsCommand{Fence: nonActiveFence(fence), CorpID: command.Manifest.WeComCorpID, HMACKeyVersion: int16(command.Manifest.HMACKeyVersion), DigestKey: command.HMACKey, Staff: staff, SkippedOwners: skippedOwners})
			}
			staff = staff[:0]
			skippedOwners = skippedOwners[:0]
		}
		return err
	})
	if err == nil && len(staff)+len(skippedOwners) > 0 {
		if err = executor.renew(ctx, fence); err == nil {
			_, err = executor.active.Process(ctx, ActiveRootsCommand{Fence: nonActiveFence(fence), CorpID: command.Manifest.WeComCorpID, HMACKeyVersion: int16(command.Manifest.HMACKeyVersion), DigestKey: command.HMACKey, Staff: staff, SkippedOwners: skippedOwners})
		}
	}
	if err != nil {
		return err
	}
	for _, found := range allowlist {
		if !found {
			return fmt.Errorf("%w: owner allowlist missing", ErrInvalidExecutor)
		}
	}
	if err = executor.checkpoint(ctx, fence, tracker); err != nil {
		return err
	}

	customers := make([]CustomerActiveRoot, 0, executorPageSize)
	tracker = newTableTracker("crm_user_identity", bounds["crm_user_identity"], command.HMACKey)
	err = snapshot.EachCustomerIdentity(ctx, bounds[tracker.table], func(row CustomerIdentityRow) error {
		if !validCustomerSourceRow(row) {
			return ErrInvalidExecutor
		}
		fact, addErr := tracker.add(row.UnionID, row.Payload)
		if addErr != nil {
			return addErr
		}
		var avatar *string
		if row.AvatarURL != "" {
			value := row.AvatarURL
			avatar = &value
		}
		var owner []byte
		if row.PrimaryOwnerUser != "" {
			allowHMAC, _ := OwnerAllowlistHMAC(command.HMACKey, row.PrimaryOwnerUser)
			if allowlist[hex.EncodeToString(allowHMAC)] {
				owner, _ = SourceKeyHMAC(command.HMACKey, "owner_role_map", row.PrimaryOwnerUser)
			} else {
				owner, _ = domainHMAC(command.HMACKey, "owner-unresolved/v1", "owner_role_map", []byte(row.PrimaryOwnerUser))
			}
		}
		customers = append(customers, CustomerActiveRoot{Source: fact, OwnerStaffSourceKeyHMAC: owner, Target: contactport.HistoricalImportCustomerFact{Name: row.CustomerName, AvatarURL: avatar, Gender: row.Gender, FirstSeenAt: row.FirstSeenAt, LastSeenAt: row.LastSeenAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}})
		if len(customers) == executorPageSize {
			if addErr = executor.renew(ctx, fence); addErr == nil {
				_, addErr = executor.active.Process(ctx, ActiveRootsCommand{Fence: nonActiveFence(fence), CorpID: command.Manifest.WeComCorpID, HMACKeyVersion: int16(command.Manifest.HMACKeyVersion), DigestKey: command.HMACKey, Customers: customers})
			}
			customers = customers[:0]
		}
		return addErr
	})
	if err == nil && len(customers) > 0 {
		if err = executor.renew(ctx, fence); err == nil {
			_, err = executor.active.Process(ctx, ActiveRootsCommand{Fence: nonActiveFence(fence), CorpID: command.Manifest.WeComCorpID, HMACKeyVersion: int16(command.Manifest.HMACKeyVersion), DigestKey: command.HMACKey, Customers: customers})
		}
	}
	if err != nil {
		return err
	}
	if err = executor.checkpoint(ctx, fence, tracker); err != nil {
		return err
	}

	identities := make([]ExternalIdentityActiveRoot, 0, executorPageSize)
	tracker = newTableTracker("wecom_external_contact_identity_map", bounds["wecom_external_contact_identity_map"], command.HMACKey)
	err = snapshot.EachExternalIdentityMap(ctx, bounds[tracker.table], func(row ExternalIdentityMapRow) error {
		if !validExternalIdentitySourceRow(row) || row.CorpID != command.Manifest.WeComCorpID {
			return fmt.Errorf("%w: corp mismatch", ErrInvalidExecutor)
		}
		fact, addErr := tracker.add(strconv.FormatInt(row.ID, 10), row.Payload)
		if addErr != nil {
			return addErr
		}
		customerKey, _ := SourceKeyHMAC(command.HMACKey, "crm_user_identity", row.UnionID)
		identities = append(identities, ExternalIdentityActiveRoot{Source: fact, CustomerSourceKeyHMAC: customerKey, CorpID: row.CorpID, ExternalUserID: row.ExternalUserID})
		if len(identities) == executorPageSize {
			if addErr = executor.renew(ctx, fence); addErr == nil {
				_, addErr = executor.active.Process(ctx, ActiveRootsCommand{Fence: nonActiveFence(fence), CorpID: command.Manifest.WeComCorpID, HMACKeyVersion: int16(command.Manifest.HMACKeyVersion), DigestKey: command.HMACKey, Identities: identities})
			}
			identities = identities[:0]
		}
		return addErr
	})
	if err == nil && len(identities) > 0 {
		if err = executor.renew(ctx, fence); err == nil {
			_, err = executor.active.Process(ctx, ActiveRootsCommand{Fence: nonActiveFence(fence), CorpID: command.Manifest.WeComCorpID, HMACKeyVersion: int16(command.Manifest.HMACKeyVersion), DigestKey: command.HMACKey, Identities: identities})
		}
	}
	if err != nil {
		return err
	}
	if err = executor.checkpoint(ctx, fence, tracker); err != nil {
		return err
	}
	return executor.runNonActive(ctx, snapshot, bounds, fence, command)
}

func (executor *Executor) runNonActive(ctx context.Context, snapshot SourceSnapshot, bounds map[string]SourceUpperBound, fence RunFence, command ExecuteCommand) error {
	for _, spec := range nonActiveScans(snapshot, command.Manifest.WeComCorpID) {
		tracker := newTableTracker(spec.table, bounds[spec.table], command.HMACKey)
		rows := make([]NonActiveRow, 0, executorPageSize)
		err := spec.each(ctx, bounds[spec.table], func(key string, payload []byte) error {
			fact, addErr := tracker.add(key, payload)
			if addErr != nil {
				return addErr
			}
			row := NonActiveRow{Source: spec.source, Fact: fact}
			if spec.archive {
				row.ArchivePayload = append([]byte(nil), payload...)
			}
			rows = append(rows, row)
			if len(rows) == executorPageSize {
				if addErr = executor.renew(ctx, fence); addErr == nil {
					_, addErr = executor.nonActive.Process(ctx, NonActiveCommand{Fence: nonActiveFence(fence), ArchiveKey: command.ArchiveKey, ArchiveKeyVersion: int16(command.Manifest.HMACKeyVersion), PayloadHMACKey: command.HMACKey, Rows: rows})
				}
				rows = rows[:0]
			}
			return addErr
		})
		if err == nil && len(rows) > 0 {
			if err = executor.renew(ctx, fence); err == nil {
				_, err = executor.nonActive.Process(ctx, NonActiveCommand{Fence: nonActiveFence(fence), ArchiveKey: command.ArchiveKey, ArchiveKeyVersion: int16(command.Manifest.HMACKeyVersion), PayloadHMACKey: command.HMACKey, Rows: rows})
			}
		}
		if err != nil {
			return err
		}
		if err = executor.checkpoint(ctx, fence, tracker); err != nil {
			return err
		}
	}
	return nil
}

func (executor *Executor) runReconcile(ctx context.Context, snapshot SourceSnapshot, bounds map[string]SourceUpperBound, fence RunFence, command ExecuteCommand) error {
	summaries := make([]ReconcileSourceSummary, 0, len(reconcileTableOrder))
	for _, spec := range allScans(snapshot, command.Manifest.WeComCorpID) {
		tracker := newTableTracker(spec.table, bounds[spec.table], command.HMACKey)
		digest := NewReconcileDigest()
		var pageRows int
		if err := spec.each(ctx, bounds[spec.table], func(raw string, payload []byte) error {
			fact, err := tracker.add(raw, payload)
			if err == nil {
				err = digest.Add(fact)
			}
			pageRows++
			if err == nil && pageRows == executorPageSize {
				err = executor.renew(ctx, fence)
				pageRows = 0
			}
			return err
		}); err != nil {
			return err
		}
		if err := executor.checkpoint(ctx, fence, tracker); err != nil {
			return err
		}
		summaries = append(summaries, ReconcileSourceSummary{Table: spec.reconcile, Count: digest.Count(), Digest: digest.Sum()})
	}
	_, err := executor.reconcile.Reconcile(ctx, ReconcileCommand{Fence: nonActiveFence(fence), Sources: summaries, ArchiveKey: command.ArchiveKey, HMACKey: command.HMACKey})
	return err
}

func (executor *Executor) checkpoint(ctx context.Context, fence RunFence, tracker *tableTracker) error {
	if err := executor.renew(ctx, fence); err != nil {
		return err
	}
	return executor.withTargetOnly(ctx, func(txCtx context.Context) error {
		return executor.runs.AppendCheckpoint(txCtx, fence, tracker.checkpoint())
	})
}
func (executor *Executor) renew(ctx context.Context, fence RunFence) error {
	return executor.withTargetOnly(ctx, func(txCtx context.Context) error {
		return executor.runs.RenewRun(txCtx, fence, executor.now().Add(2*time.Minute))
	})
}
func (executor *Executor) transition(ctx context.Context, fence RunFence, state string) error {
	return executor.withTargetOnly(ctx, func(txCtx context.Context) error { return executor.runs.TransitionRun(txCtx, fence, state) })
}
func (executor *Executor) withTargetOnly(ctx context.Context, callback func(context.Context) error) error {
	return executor.uow.Within(ctx, callback)
}

type tableTracker struct {
	table     string
	bound     SourceUpperBound
	key       []byte
	count     int64
	last      contactport.HistoricalImportSourceFact
	watermark time.Time
}

func newTableTracker(table string, bound SourceUpperBound, key []byte) *tableTracker {
	return &tableTracker{table: table, bound: bound, key: key}
}
func (tracker *tableTracker) add(raw string, payload []byte) (contactport.HistoricalImportSourceFact, error) {
	if raw == "" || len(payload) == 0 {
		return contactport.HistoricalImportSourceFact{}, ErrInvalidExecutor
	}
	payloadHMAC, err := SourcePayloadHMAC(tracker.key, tracker.table, payload)
	if err != nil {
		return contactport.HistoricalImportSourceFact{}, err
	}
	field, err := SourceFieldsHMAC(tracker.key, tracker.table, payload)
	if err != nil {
		return contactport.HistoricalImportSourceFact{}, err
	}
	sourceKey, err := SourceKeyHMAC(tracker.key, tracker.table, raw)
	if err != nil {
		return contactport.HistoricalImportSourceFact{}, err
	}
	fact := contactport.HistoricalImportSourceFact{SourceKeyHMAC: sourceKey, PayloadHMAC: payloadHMAC, FieldDigest: field}
	tracker.last = fact
	tracker.count++
	return fact, nil
}
func (tracker *tableTracker) checkpoint() FinalCheckpoint {
	if tracker.count == 0 {
		payload, _ := SourcePayloadHMAC(tracker.key, tracker.table, nil)
		fields, _ := SourceFieldsHMAC(tracker.key, tracker.table, nil)
		sourceKey, _ := emptySourceKeyHMAC(tracker.key, tracker.table)
		tracker.last = contactport.HistoricalImportSourceFact{SourceKeyHMAC: sourceKey, PayloadHMAC: payload, FieldDigest: fields}
	}
	var upper []byte
	if !tracker.bound.Empty {
		upper, _ = SourceKeyHMAC(tracker.key, tracker.table, tracker.bound.SourceKey)
	}
	return FinalCheckpoint{Table: tracker.table, FinalFact: tracker.last, Watermark: tracker.bound.Watermark, UpperKeyHMAC: upper, UpperBoundEmpty: tracker.bound.Empty}
}

func nonActiveFence(f RunFence) contactport.NonActiveLeaseFence {
	return contactport.NonActiveLeaseFence{RunID: f.RunID, Generation: f.Generation, TokenHMAC: f.TokenHMAC}
}

func validateBounds(input []SourceUpperBound) (map[string]SourceUpperBound, time.Time, error) {
	want := allTableNames()
	if len(input) != len(want) {
		return nil, time.Time{}, ErrSourceSchemaDrift
	}
	result := make(map[string]SourceUpperBound, len(input))
	var upper time.Time
	for _, bound := range input {
		if _, ok := want[bound.Table]; !ok || result[bound.Table].Table != "" || (!bound.Empty && (bound.Watermark.IsZero() || bound.SourceKey == "" || strings.TrimSpace(bound.SourceKey) != bound.SourceKey)) {
			return nil, time.Time{}, ErrSourceSchemaDrift
		}
		result[bound.Table] = bound
		if bound.Watermark.After(upper) {
			upper = bound.Watermark
		}
	}
	if upper.IsZero() {
		upper = time.Unix(0, 0).UTC()
	}
	return result, upper, nil
}
func allTableNames() map[string]struct{} {
	result := map[string]struct{}{}
	for _, table := range reconcileTableOrder {
		result[ReconcileTableName(table)] = struct{}{}
	}
	return result
}

type sourceScan struct {
	table     string
	reconcile ReconcileTable
	source    contactport.NonActiveSource
	archive   bool
	each      func(context.Context, SourceUpperBound, func(string, []byte) error) error
}

func allScans(snapshot SourceSnapshot, corpID string) []sourceScan {
	return []sourceScan{
		{"owner_role_map", ReconcileOwnerRoleMap, 0, false, func(c context.Context, b SourceUpperBound, e func(string, []byte) error) error {
			return snapshot.EachOwnerRoleMap(c, b, func(r OwnerRoleMapRow) error { return e(r.UserID, r.Payload) })
		}},
		{"crm_user_identity", ReconcileCustomerIdentity, 0, false, func(c context.Context, b SourceUpperBound, e func(string, []byte) error) error {
			return snapshot.EachCustomerIdentity(c, b, func(r CustomerIdentityRow) error {
				if !validCustomerSourceRow(r) {
					return ErrInvalidExecutor
				}
				return e(r.UnionID, r.Payload)
			})
		}},
		{"wecom_external_contact_identity_map", ReconcileExternalIdentity, 0, false, func(c context.Context, b SourceUpperBound, e func(string, []byte) error) error {
			return snapshot.EachExternalIdentityMap(c, b, func(r ExternalIdentityMapRow) error {
				if !validExternalIdentitySourceRow(r) || r.CorpID != corpID {
					return ErrInvalidExecutor
				}
				return e(strconv.FormatInt(r.ID, 10), r.Payload)
			})
		}},
		{"crm_user_identity_merge_audit", ReconcileMergeAudit, contactport.NonActiveMergeAudit, true, func(c context.Context, b SourceUpperBound, e func(string, []byte) error) error {
			return snapshot.EachMergeAudit(c, b, func(r MergeAuditRow) error { return e(strconv.FormatInt(r.ID, 10), r.Payload) })
		}},
		{"crm_user_identity_resolution_queue", ReconcileResolutionQueue, contactport.NonActiveResolutionQueue, true, func(c context.Context, b SourceUpperBound, e func(string, []byte) error) error {
			return snapshot.EachResolutionQueue(c, b, func(r ResolutionQueueRow) error {
				if r.CorpID != corpID {
					return ErrInvalidExecutor
				}
				return e(strconv.FormatInt(r.ID, 10), r.Payload)
			})
		}},
		{"admin_wecom_directory_members", ReconcileDirectoryMembers, contactport.NonActiveDirectoryMembers, false, func(c context.Context, b SourceUpperBound, e func(string, []byte) error) error {
			return snapshot.EachDirectoryMember(c, b, func(r DirectoryMemberRow) error {
				if r.CorpID != corpID {
					return ErrInvalidExecutor
				}
				return e(strconv.FormatInt(r.ID, 10), r.Payload)
			})
		}},
		{"contacts", ReconcileContacts, contactport.NonActiveContacts, false, func(c context.Context, b SourceUpperBound, e func(string, []byte) error) error {
			return snapshot.EachContact(c, b, func(r ContactRow) error { return e(strconv.FormatInt(r.ID, 10), r.Payload) })
		}},
		{"crm_user_identity_conflicts", ReconcileIdentityConflicts, contactport.NonActiveIdentityConflicts, false, func(c context.Context, b SourceUpperBound, e func(string, []byte) error) error {
			return snapshot.EachIdentityConflict(c, b, func(r IdentityConflictRow) error { return e(strconv.FormatInt(r.ID, 10), r.Payload) })
		}},
		{"external_contact_bindings", ReconcileExternalBindings, contactport.NonActiveExternalBindings, false, func(c context.Context, b SourceUpperBound, e func(string, []byte) error) error {
			return snapshot.EachExternalBinding(c, b, func(r ExternalBindingRow) error { return e(r.ExternalUserID, r.Payload) })
		}},
		{"people", ReconcilePeople, contactport.NonActivePeople, false, func(c context.Context, b SourceUpperBound, e func(string, []byte) error) error {
			return snapshot.EachPerson(c, b, func(r PersonRow) error { return e(strconv.FormatInt(r.ID, 10), r.Payload) })
		}},
		{"wecom_external_contact_follow_users", ReconcileFollowUsers, contactport.NonActiveFollowUsers, false, func(c context.Context, b SourceUpperBound, e func(string, []byte) error) error {
			return snapshot.EachFollowUser(c, b, func(r FollowUserRow) error {
				if r.CorpID != corpID {
					return ErrInvalidExecutor
				}
				return e(strconv.FormatInt(r.ID, 10), r.Payload)
			})
		}},
	}
}
func nonActiveScans(snapshot SourceSnapshot, corpID string) []sourceScan {
	return allScans(snapshot, corpID)[3:]
}

func validCustomerSourceRow(row CustomerIdentityRow) bool {
	return row.UnionID != "" && strings.TrimSpace(row.UnionID) == row.UnionID && strings.TrimSpace(row.PrimaryOwnerUser) == row.PrimaryOwnerUser
}

func validExternalIdentitySourceRow(row ExternalIdentityMapRow) bool {
	return row.ID > 0 && row.ExternalUserID != "" && strings.TrimSpace(row.ExternalUserID) == row.ExternalUserID && row.UnionID != "" && strings.TrimSpace(row.UnionID) == row.UnionID && row.CorpID != "" && strings.TrimSpace(row.CorpID) == row.CorpID
}
