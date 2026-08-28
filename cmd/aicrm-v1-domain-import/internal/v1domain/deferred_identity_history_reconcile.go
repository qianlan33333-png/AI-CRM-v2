package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	v1deferredidentityhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1deferredidentityhistory"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

var deferredIdentityHistoryReconciledTables = []string{
	v1deferredidentityhistory.PeopleTableID,
	v1deferredidentityhistory.IdentityConflictsTableID,
	v1deferredidentityhistory.ExternalContactIdentityMapID,
}

type deferredIdentityHistoryReconcileKey struct {
	tableID   string
	sourceKey [sha256.Size]byte
}

type deferredIdentityHistoryReconcileExpected struct {
	kind, target string
	person       *v1deferredidentityhistory.SelectedPerson
	conflict     *v1deferredidentityhistory.SelectedIdentityConflict
	missingRoot  *v1deferredidentityhistory.SelectedMissingRootIdentity
}

// DeferredIdentityHistoryReconcileTargets verifies only the deterministic,
// already-selected DM01 deferred-evidence set. Its reader must be bound to the
// caller's reconciliation transaction.
type DeferredIdentityHistoryReconcileTargets struct {
	reader   contactport.DeferredIdentityHistoryReader
	options  v1deferredidentityhistory.DeferredIdentitySelectionOptions
	expected map[deferredIdentityHistoryReconcileKey]deferredIdentityHistoryReconcileExpected
	seen     map[deferredIdentityHistoryReconcileKey]struct{}
}

func NewDeferredIdentityHistoryReconcileTargets(
	selection v1deferredidentityhistory.DeferredIdentitySelection,
	options v1deferredidentityhistory.DeferredIdentitySelectionOptions,
	reader contactport.DeferredIdentityHistoryReader,
) (*DeferredIdentityHistoryReconcileTargets, error) {
	if !validDeferredIdentityHistoryOptions(options) || nilDeferredHistory(reader) {
		return nil, ErrInvalidScope
	}
	if selection.Count() != 1392 {
		return nil, ErrConflict
	}
	targets := &DeferredIdentityHistoryReconcileTargets{
		reader:   reader,
		options:  options,
		expected: make(map[deferredIdentityHistoryReconcileKey]deferredIdentityHistoryReconcileExpected, selection.Count()),
		seen:     make(map[deferredIdentityHistoryReconcileKey]struct{}, selection.Count()),
	}
	for index := range selection.People {
		value := &selection.People[index]
		if value.ArchivedRow.TableID != v1deferredidentityhistory.PeopleTableID || !deferredPersonSelectionMatches(*value) ||
			!targets.add(value.ArchivedRow.SourceKeyHMAC, value.ArchivedRow.TableID, deferredIdentityHistoryReconcileExpected{kind: DeferredPersonHistoryKind, target: DeferredPersonHistoryTarget, person: value}) {
			return nil, ErrConflict
		}
	}
	for index := range selection.IdentityConflicts {
		value := &selection.IdentityConflicts[index]
		if value.ArchivedRow.TableID != v1deferredidentityhistory.IdentityConflictsTableID || !deferredConflictSelectionMatches(*value) ||
			!targets.add(value.ArchivedRow.SourceKeyHMAC, value.ArchivedRow.TableID, deferredIdentityHistoryReconcileExpected{kind: DeferredConflictHistoryKind, target: DeferredConflictHistoryTarget, conflict: value}) {
			return nil, ErrConflict
		}
	}
	for index := range selection.MissingCustomerRootMaps {
		value := &selection.MissingCustomerRootMaps[index]
		if value.ArchivedRow.TableID != v1deferredidentityhistory.ExternalContactIdentityMapID || !missingRootSelectionMatches(*value, options) ||
			!targets.add(value.ArchivedRow.SourceKeyHMAC, value.ArchivedRow.TableID, deferredIdentityHistoryReconcileExpected{kind: MissingRootIdentityKind, target: MissingRootIdentityTarget, missingRoot: value}) {
			return nil, ErrConflict
		}
	}
	if len(targets.expected) != selection.Count() {
		return nil, ErrConflict
	}
	return targets, nil
}

func (targets *DeferredIdentityHistoryReconcileTargets) add(sourceKey [sha256.Size]byte, tableID string, expected deferredIdentityHistoryReconcileExpected) bool {
	key := deferredIdentityHistoryReconcileKey{tableID: tableID, sourceKey: sourceKey}
	if sourceKey == ([sha256.Size]byte{}) || !isDeferredIdentityHistorySource(tableID) || expected.kind == "" || expected.target == "" {
		return false
	}
	if _, duplicate := targets.expected[key]; duplicate {
		return false
	}
	targets.expected[key] = expected
	return true
}

// Verify checks the generic receipt binding and every field in the stored
// owner fact. The owner digest deliberately includes all json:"-" evidence.
func (targets *DeferredIdentityHistoryReconcileTargets) Verify(ctx context.Context, row reconciliationRow) (string, error) {
	if targets == nil || ctx == nil || nilDeferredHistory(targets.reader) || row.TargetDomain == nil || row.TargetTable == nil || row.TargetID == nil ||
		row.Disposition != "import" || row.Reason != "" || !row.Verified || *row.TargetDomain != DeferredIdentityHistoryDomain ||
		len(row.SourceKeyDigest) != sha256.Size || len(row.PayloadDigest) != sha256.Size || len(row.FieldDigest) != sha256.Size || len(row.TargetDigest) != sha256.Size {
		return "", ErrConflict
	}
	var sourceKey, payload, field [sha256.Size]byte
	copy(sourceKey[:], row.SourceKeyDigest)
	copy(payload[:], row.PayloadDigest)
	copy(field[:], row.FieldDigest)
	expected, found := targets.expected[deferredIdentityHistoryReconcileKey{tableID: row.TableID, sourceKey: sourceKey}]
	if !found || *row.TargetTable != expected.target {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	if _, duplicate := targets.seen[deferredIdentityHistoryReconcileKey{tableID: row.TableID, sourceKey: sourceKey}]; duplicate {
		return "", ErrConflict
	}

	var actualSourceKey, actualPayload, actualField, actualDigest, expectedDigest [sha256.Size]byte
	switch expected.kind {
	case DeferredPersonHistoryKind:
		actual, readErr := targets.reader.GetHistoricalDeferredPerson(ctx, id)
		if readErr != nil || actual.ID != id || expected.person == nil {
			return "", ErrConflict
		}
		value := deferredPersonValue(*expected.person)
		value.ID = id
		actualSourceKey, actualPayload, actualField = actual.SourceKeyDigest, actual.SourcePayloadDigest, actual.SourceFieldDigest
		actualDigest, readErr = contactapp.HistoricalDeferredPersonDigest(actual)
		expectedDigest, err = contactapp.HistoricalDeferredPersonDigest(value)
		if readErr != nil || err != nil {
			return "", ErrConflict
		}
	case DeferredConflictHistoryKind:
		actual, readErr := targets.reader.GetHistoricalDeferredIdentityConflict(ctx, id)
		if readErr != nil || actual.ID != id || expected.conflict == nil {
			return "", ErrConflict
		}
		value := deferredConflictValue(*expected.conflict)
		value.ID = id
		actualSourceKey, actualPayload, actualField = actual.SourceKeyDigest, actual.SourcePayloadDigest, actual.SourceFieldDigest
		actualDigest, readErr = contactapp.HistoricalDeferredIdentityConflictDigest(actual)
		expectedDigest, err = contactapp.HistoricalDeferredIdentityConflictDigest(value)
		if readErr != nil || err != nil {
			return "", ErrConflict
		}
	case MissingRootIdentityKind:
		actual, readErr := targets.reader.GetHistoricalMissingRootIdentity(ctx, id)
		if readErr != nil || actual.ID != id || expected.missingRoot == nil {
			return "", ErrConflict
		}
		value := missingRootValue(*expected.missingRoot, targets.options)
		value.ID = id
		actualSourceKey, actualPayload, actualField = actual.SourceKeyDigest, actual.SourcePayloadDigest, actual.SourceFieldDigest
		actualDigest, readErr = contactapp.HistoricalMissingRootIdentityDigest(actual)
		expectedDigest, err = contactapp.HistoricalMissingRootIdentityDigest(value)
		if readErr != nil || err != nil {
			return "", ErrConflict
		}
	default:
		return "", ErrConflict
	}
	if actualSourceKey != sourceKey || actualPayload != payload || actualField != field || actualDigest != expectedDigest || !equalBytes(actualDigest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	targets.seen[deferredIdentityHistoryReconcileKey{tableID: row.TableID, sourceKey: sourceKey}] = struct{}{}
	return "history_only:" + hex.EncodeToString(actualDigest[:]), nil
}

func (targets *DeferredIdentityHistoryReconcileTargets) VerifyComplete() error {
	if targets == nil || len(targets.expected) != 1392 || len(targets.seen) != len(targets.expected) {
		return ErrConflict
	}
	return nil
}

func isDeferredIdentityHistorySource(tableID string) bool {
	for _, candidate := range deferredIdentityHistoryReconciledTables {
		if tableID == candidate {
			return true
		}
	}
	return false
}
