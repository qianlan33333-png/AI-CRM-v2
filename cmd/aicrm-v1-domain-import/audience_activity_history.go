package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	segmentactivity "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1audienceactivityhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1audiencehistory"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

const (
	audienceActivityHistoryDomain = "audience-activity-history"
	audienceActivityImportVersion = "v1-audience-activity-history-a1"
)

const audienceActivityHistoryFieldMetadata = "field_hmac"

// audienceActivityHistoryJournal maps the two app journal kinds to their
// exact generic receipt scope. The generic receipt retains field HMAC in its
// metadata, so an app replay cannot silently lose archive field proof.
type audienceActivityHistoryJournal struct {
	runs, events *v1domain.Journal
	targets      segmentport.AudienceActivityHistoryStore
	archiveRun   string
}

var _ segmentport.AudienceActivityHistoryJournal = (*audienceActivityHistoryJournal)(nil)
var _ v1domain.AudienceActivityJournal = (*audienceActivityHistoryJournal)(nil)

func newAudienceActivityHistoryJournal(run string, targets segmentport.AudienceActivityHistoryStore) (*audienceActivityHistoryJournal, error) {
	if run == "" || targets == nil {
		return nil, v1domain.ErrInvalidScope
	}
	runs, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: audienceActivityImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: segmentactivity.PackageRunsTableID, TargetDomain: "segment", TargetTable: "segment_v1_audience_activity_runs"})
	if err != nil {
		return nil, err
	}
	events, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: audienceActivityImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: segmentactivity.MemberEventsTableID, TargetDomain: "segment", TargetTable: "segment_v1_audience_activity_member_events"})
	if err != nil {
		return nil, err
	}
	return &audienceActivityHistoryJournal{runs: runs, events: events, targets: targets, archiveRun: run}, nil
}
func (j *audienceActivityHistoryJournal) terminal(kind string) (*v1domain.Journal, error) {
	if j == nil {
		return nil, v1domain.ErrInvalidScope
	}
	if kind == "package_runs" {
		return j.runs, nil
	}
	if kind == "member_events" {
		return j.events, nil
	}
	return nil, v1domain.ErrInvalidScope
}
func (j *audienceActivityHistoryJournal) LoadAudienceActivityHistory(ctx context.Context, kind, source string) (segmentport.AudienceActivityHistoryReceipt, bool, error) {
	t, err := j.terminal(kind)
	if err != nil || ctx == nil {
		return segmentport.AudienceActivityHistoryReceipt{}, false, v1domain.ErrInvalidScope
	}
	value, found, err := t.LoadTerminal(ctx, source)
	if err != nil || !found {
		return segmentport.AudienceActivityHistoryReceipt{}, found, err
	}
	return j.receipt(kind, source, value)
}
func (j *audienceActivityHistoryJournal) RecordAudienceActivityHistory(ctx context.Context, kind string, receipt segmentport.AudienceActivityHistoryReceipt) error {
	t, err := j.terminal(kind)
	if err != nil || ctx == nil || receipt.Replayed || receipt.TargetID < 1 || receipt.PayloadDigest == ([32]byte{}) || receipt.TargetDigest == ([32]byte{}) {
		return v1domain.ErrInvalidScope
	}
	source, err := v1domain.ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil {
		return v1domain.ErrInvalidScope
	}
	var field [32]byte
	if kind == "package_runs" {
		value, e := j.targets.GetHistoricalAudienceActivityRun(ctx, receipt.TargetID)
		if e != nil {
			return e
		}
		digest, e := segmentapp.HistoricalAudienceActivityRunDigest(value)
		if e != nil || value.ID != receipt.TargetID || value.SourceKeyDigest != source || value.SourcePayloadDigest != receipt.PayloadDigest || digest != receipt.TargetDigest {
			return v1domain.ErrConflict
		}
		field = value.SourceFieldDigest
	} else {
		value, e := j.targets.GetHistoricalAudienceActivityMemberEvent(ctx, receipt.TargetID)
		if e != nil {
			return e
		}
		digest, e := segmentapp.HistoricalAudienceActivityMemberEventDigest(value)
		if e != nil || value.ID != receipt.TargetID || value.SourceKeyDigest != source || value.SourcePayloadDigest != receipt.PayloadDigest || digest != receipt.TargetDigest {
			return v1domain.ErrConflict
		}
		field = value.SourceFieldDigest
	}
	if field == ([32]byte{}) {
		return v1domain.ErrConflict
	}
	return t.Record(ctx, v1domain.TerminalReceipt{SourceKeyDigest: source, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest, Metadata: audienceActivityMetadata(field)})
}
func (j *audienceActivityHistoryJournal) LoadAudienceActivityTerminal(ctx context.Context, version string, source [32]byte) (v1domain.AudienceActivityTerminal, bool, error) {
	if j == nil || version != audienceActivityImportVersion || source == ([32]byte{}) {
		return v1domain.AudienceActivityTerminal{}, false, v1domain.ErrInvalidScope
	}
	for _, kind := range []string{"package_runs", "member_events"} {
		t, _ := j.terminal(kind)
		value, found, err := t.LoadTerminal(ctx, v1domain.SourceIdentifier(source))
		if err != nil {
			return v1domain.AudienceActivityTerminal{}, false, err
		}
		if found {
			return j.terminalValue(kind, value)
		}
	}
	return v1domain.AudienceActivityTerminal{}, false, nil
}
func (j *audienceActivityHistoryJournal) RecordAudienceActivityTerminal(ctx context.Context, value v1domain.AudienceActivityTerminal) error {
	if j == nil || value.Version != audienceActivityImportVersion || value.ArchiveRunID != j.archiveRun || value.Disposition != "quarantine" || value.TargetID != 0 || value.TargetDigest != ([32]byte{}) {
		return v1domain.ErrInvalidScope
	}
	t, err := j.terminal(value.Kind)
	if err != nil {
		return err
	}
	return t.Record(ctx, v1domain.TerminalReceipt{SourceKeyDigest: value.SourceKeyHMAC, PayloadDigest: value.PayloadHMAC, Disposition: "quarantine", Reason: value.Reason, Metadata: audienceActivityMetadata(value.FieldHMAC)})
}
func (j *audienceActivityHistoryJournal) receipt(kind, source string, value v1domain.TerminalReceipt) (segmentport.AudienceActivityHistoryReceipt, bool, error) {
	if _, err := audienceActivityFieldHMAC(value.Metadata); err != nil {
		return segmentport.AudienceActivityHistoryReceipt{}, false, err
	}
	key, err := v1domain.ParseSourceIdentifier(source)
	id, e := strconv.ParseInt(value.TargetID, 10, 64)
	if err != nil || e != nil || key == ([32]byte{}) || value.SourceKeyDigest != key || value.Disposition != "import" || value.Reason != "" || id < 1 || value.TargetDigest == ([32]byte{}) {
		return segmentport.AudienceActivityHistoryReceipt{}, false, v1domain.ErrConflict
	}
	return segmentport.AudienceActivityHistoryReceipt{SourceIdentifier: source, PayloadDigest: value.PayloadDigest, TargetID: id, TargetDigest: value.TargetDigest}, true, nil
}
func (j *audienceActivityHistoryJournal) terminalValue(kind string, value v1domain.TerminalReceipt) (v1domain.AudienceActivityTerminal, bool, error) {
	field, err := audienceActivityFieldHMAC(value.Metadata)
	if err != nil {
		return v1domain.AudienceActivityTerminal{}, false, err
	}
	table := ""
	if kind == "package_runs" {
		table = segmentactivity.PackageRunsTableID
	} else if kind == "member_events" {
		table = segmentactivity.MemberEventsTableID
	} else {
		return v1domain.AudienceActivityTerminal{}, false, v1domain.ErrInvalidScope
	}
	out := v1domain.AudienceActivityTerminal{Version: audienceActivityImportVersion, ArchiveRunID: j.archiveRun, TableID: table, Kind: kind, SourceKeyHMAC: value.SourceKeyDigest, PayloadHMAC: value.PayloadDigest, FieldHMAC: field, Disposition: value.Disposition, Reason: value.Reason, TargetDigest: value.TargetDigest}
	if value.Disposition == "import" {
		id, e := strconv.ParseInt(value.TargetID, 10, 64)
		if e != nil || id < 1 {
			return v1domain.AudienceActivityTerminal{}, false, v1domain.ErrConflict
		}
		out.TargetID = id
	} else if value.Disposition != "quarantine" || value.TargetID != "" || value.TargetDigest != ([32]byte{}) {
		return v1domain.AudienceActivityTerminal{}, false, v1domain.ErrConflict
	}
	return out, true, nil
}
func audienceActivityMetadata(field [32]byte) map[string]any {
	return map[string]any{audienceActivityHistoryFieldMetadata: hex.EncodeToString(field[:])}
}
func audienceActivityFieldHMAC(metadata map[string]any) ([32]byte, error) {
	if len(metadata) != 1 {
		return [32]byte{}, v1domain.ErrConflict
	}
	text, ok := metadata[audienceActivityHistoryFieldMetadata].(string)
	if !ok || len(text) != 64 {
		return [32]byte{}, v1domain.ErrConflict
	}
	raw, err := hex.DecodeString(text)
	if err != nil || len(raw) != 32 {
		return [32]byte{}, v1domain.ErrConflict
	}
	var out [32]byte
	copy(out[:], raw)
	if out == ([32]byte{}) || text != hex.EncodeToString(out[:]) {
		return [32]byte{}, v1domain.ErrConflict
	}
	return out, nil
}

func importAudienceActivityHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, sourceKey []byte, reconcile bool) (any, error) {
	if ctx == nil || archive == nil || uow == nil || run == "" || len(sourceKey) < sha256.Size {
		return nil, v1domain.ErrInvalidScope
	}
	targets := segmentstore.NewAudienceActivityHistoryStore()
	journal, err := newAudienceActivityHistoryJournal(run, targets)
	if err != nil {
		return nil, err
	}
	writer := segmentapp.NewAudienceActivityHistoryWriter(targets, journal)
	refs, err := newAudienceActivityReferences(run, sourceKey, segmentstore.NewAudienceActivityHistoryReader(nil))
	if err != nil {
		return nil, err
	}
	importer, err := v1domain.NewAudienceActivityHistoryImporter(v1domain.NewAudienceActivityArchiveReadySQL(), archive, uow, writer, refs, targets, journal, v1domain.NewAudienceActivityReconciliationSealStore())
	if err != nil {
		return nil, err
	}
	if reconcile {
		return importer.Reconcile(ctx, run, sourceKey)
	}
	return importer.Import(ctx, run, sourceKey)
}

// audienceActivityReferences is deliberately private to the archive command.
// It does not infer a parent from a source_id alone: every reference first
// proves the old audience-history receipt for this same sealed archive run.
type audienceActivityReferences struct {
	archiveRun string
	sourceKey  []byte
	reader     *segmentstore.AudienceActivityHistoryReader
}

var _ segmentport.AudienceActivityHistoryReferences = (*audienceActivityReferences)(nil)

func newAudienceActivityReferences(run string, sourceKey []byte, reader *segmentstore.AudienceActivityHistoryReader) (*audienceActivityReferences, error) {
	if run == "" || strings.TrimSpace(run) != run || len(sourceKey) < sha256.Size || reader == nil {
		return nil, v1domain.ErrInvalidScope
	}
	return &audienceActivityReferences{archiveRun: run, sourceKey: append([]byte(nil), sourceKey...), reader: reader}, nil
}

func (references *audienceActivityReferences) ResolveAudienceActivityPackage(ctx context.Context, sourceID int64) (segmentport.AudienceActivityPackageReference, error) {
	if references == nil || sourceID < 1 {
		return segmentport.AudienceActivityPackageReference{}, v1domain.ErrInvalidScope
	}
	actual, err := references.reader.ResolveAudienceActivityPackage(ctx, sourceID)
	if err != nil || actual.ID < 1 {
		return segmentport.AudienceActivityPackageReference{}, audienceActivityReferenceError(err)
	}
	if err = references.verifyPriorTarget(ctx, v1audiencehistory.PackagesTableID, "segment_v1_audience_packages", sourceID, actual.ID); err != nil {
		return segmentport.AudienceActivityPackageReference{}, err
	}
	return actual, nil
}

func (references *audienceActivityReferences) ResolveAudienceActivityVersion(ctx context.Context, sourceID int64) (segmentport.AudienceActivityVersionReference, error) {
	if references == nil || sourceID < 1 {
		return segmentport.AudienceActivityVersionReference{}, v1domain.ErrInvalidScope
	}
	actual, err := references.reader.ResolveAudienceActivityVersion(ctx, sourceID)
	if err != nil || actual.ID < 1 || actual.PackageHistoryID < 1 {
		return segmentport.AudienceActivityVersionReference{}, audienceActivityReferenceError(err)
	}
	if err = references.verifyPriorTarget(ctx, v1audiencehistory.PackageVersionsTableID, "segment_v1_audience_versions", sourceID, actual.ID); err != nil {
		return segmentport.AudienceActivityVersionReference{}, err
	}
	return actual, nil
}

func (references *audienceActivityReferences) ResolveAudienceActivityMember(ctx context.Context, sourceID int64) (segmentport.AudienceActivityMemberReference, error) {
	if references == nil || sourceID < 1 {
		return segmentport.AudienceActivityMemberReference{}, v1domain.ErrInvalidScope
	}
	actual, err := references.reader.ResolveAudienceActivityMember(ctx, sourceID)
	if err != nil || actual.ID < 1 || actual.PackageHistoryID < 1 {
		return segmentport.AudienceActivityMemberReference{}, audienceActivityReferenceError(err)
	}
	if err = references.verifyPriorTarget(ctx, v1audiencehistory.AudienceMembersTableID, "segment_v1_audience_members", sourceID, actual.ID); err != nil {
		return segmentport.AudienceActivityMemberReference{}, err
	}
	return actual, nil
}

func (references *audienceActivityReferences) ResolveAudienceActivityRun(ctx context.Context, sourceID int64) (segmentport.HistoricalAudienceActivityRun, error) {
	if references == nil || sourceID < 1 {
		return segmentport.HistoricalAudienceActivityRun{}, v1domain.ErrInvalidScope
	}
	actual, err := references.reader.ResolveAudienceActivityRun(ctx, sourceID)
	if err != nil || actual.ID < 1 || actual.SourceID != sourceID {
		return segmentport.HistoricalAudienceActivityRun{}, audienceActivityReferenceError(err)
	}
	if err = references.verifyCurrentTarget(ctx, segmentactivity.PackageRunsTableID, "segment_v1_audience_activity_runs", actual.SourceKeyDigest, actual.SourcePayloadDigest, actual.ID); err != nil {
		return segmentport.HistoricalAudienceActivityRun{}, err
	}
	return actual, nil
}

func (references *audienceActivityReferences) verifyPriorTarget(ctx context.Context, table, targetTable string, sourceID, targetID int64) error {
	if sourceID < 1 || targetID < 1 {
		return v1domain.ErrConflict
	}
	key, err := audienceActivityParentSourceKey(references.sourceKey, table, sourceID)
	if err != nil {
		return v1domain.ErrConflict
	}
	return references.verifyReceipt(ctx, v1domain.AudienceHistoryImportVersion, table, targetTable, key, targetID, [sha256.Size]byte{})
}

func audienceActivityParentSourceKey(key []byte, table string, sourceID int64) ([sha256.Size]byte, error) {
	if len(key) < sha256.Size || sourceID < 1 || !strings.HasPrefix(table, "public/") {
		return [sha256.Size]byte{}, v1domain.ErrInvalidScope
	}
	return v1archive.SourceKeyHMAC(key, strings.TrimPrefix(table, "public/"), []byte("["+strconv.FormatInt(sourceID, 10)+"]"))
}

func (references *audienceActivityReferences) verifyCurrentTarget(ctx context.Context, table, targetTable string, source, payload [sha256.Size]byte, targetID int64) error {
	if source == ([sha256.Size]byte{}) || payload == ([sha256.Size]byte{}) || targetID < 1 {
		return v1domain.ErrConflict
	}
	return references.verifyReceipt(ctx, audienceActivityImportVersion, table, targetTable, source, targetID, payload)
}

// verifyReceipt is intentionally a scalar exact-match query. A missing,
// non-import, unverified, wrong-run, or duplicate receipt is indistinguishable
// to callers from an unresolved historical relation and therefore fails closed.
func (references *audienceActivityReferences) verifyReceipt(ctx context.Context, version, table, targetTable string, source [sha256.Size]byte, targetID int64, payload [sha256.Size]byte) error {
	if references == nil || ctx == nil || source == ([sha256.Size]byte{}) || targetID < 1 {
		return v1domain.ErrInvalidScope
	}
	return audienceActivityReferenceError(v1domain.VerifyAudienceActivityReceiptCrosswalk(ctx, version, references.archiveRun, v1archive.DefaultAdapterID, table, "segment", targetTable, source, targetID, payload))
}

func audienceActivityReferenceError(err error) error {
	if err != nil {
		return err
	}
	return v1domain.ErrConflict
}
