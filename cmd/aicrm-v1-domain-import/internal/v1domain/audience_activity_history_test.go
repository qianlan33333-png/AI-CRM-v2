package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	activity "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1audienceactivityhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segment "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestAudienceActivityTerminalAndSealFailClosed(t *testing.T) {
	key := sha256.Sum256([]byte("source"))
	payload := sha256.Sum256([]byte("payload"))
	field := sha256.Sum256([]byte("field"))
	target := sha256.Sum256([]byte("target"))
	for _, value := range []AudienceActivityTerminal{
		{Version: AudienceActivityHistoryImportVersion, ArchiveRunID: "run", TableID: activity.PackageRunsTableID, Kind: "package_runs", SourceKeyHMAC: key, PayloadHMAC: payload, FieldHMAC: field, Disposition: string(activity.DispositionCandidate), TargetID: 1, TargetDigest: target},
		{Version: AudienceActivityHistoryImportVersion, ArchiveRunID: "run", TableID: activity.MemberEventsTableID, Kind: "member_events", SourceKeyHMAC: key, PayloadHMAC: payload, FieldHMAC: field, Disposition: string(activity.DispositionQuarantine), Reason: "parent_unresolved"},
	} {
		if !validAudienceActivityTerminal(value) {
			t.Fatalf("valid terminal rejected: %#v", value)
		}
	}
	wrong := AudienceActivityTerminal{Version: AudienceActivityHistoryImportVersion, ArchiveRunID: "run", TableID: activity.PackageRunsTableID, Kind: "member_events", SourceKeyHMAC: key, PayloadHMAC: payload, FieldHMAC: field, Disposition: string(activity.DispositionCandidate), TargetID: 1, TargetDigest: target}
	if validAudienceActivityTerminal(wrong) {
		t.Fatal("mixed table/kind accepted")
	}
	seal := AudienceActivitySeal{Version: AudienceActivityHistoryImportVersion, ArchiveRunID: "run", SelectedSourceCount: 2, ReceiptCount: 2, ImportedCount: 1, QuarantinedCount: 1, VerifiedCount: 2, ComparisonDigest: target}
	if !validAudienceActivitySeal(seal) {
		t.Fatal("valid aggregate seal rejected")
	}
	seal.ReceiptCount--
	if validAudienceActivitySeal(seal) {
		t.Fatal("inconsistent aggregate seal accepted")
	}
}

func TestAudienceActivityReconcileSealsAndReplaysEmptySources(t *testing.T) {
	state := &audienceActivityEmptySealState{}
	archive := &audienceActivityEmptyArchive{}
	importer, err := NewAudienceActivityHistoryImporter(
		&audienceActivityEmptyReady{}, archive, audienceActivityEmptyUOW{},
		audienceActivityEmptyWriter{}, audienceActivityEmptyReferences{}, audienceActivityEmptyTargets{},
		audienceActivityEmptyJournal{}, state,
	)
	if err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{0x71}, sha256.Size)
	first, err := importer.Reconcile(context.Background(), "empty-run", key)
	if err != nil {
		t.Fatal(err)
	}
	emptyDigest := sha256.Sum256(nil)
	if first != (AudienceActivityHistoryReconciliationResult{ComparisonDigest: emptyDigest}) || state.records != 1 || state.seal == nil || *state.seal != (AudienceActivitySeal{Version: AudienceActivityHistoryImportVersion, ArchiveRunID: "empty-run", ComparisonDigest: emptyDigest}) {
		t.Fatalf("first empty reconciliation = %#v seal=%#v records=%d", first, state.seal, state.records)
	}
	second, err := importer.Reconcile(context.Background(), "empty-run", key)
	if err != nil {
		t.Fatal(err)
	}
	if second != (AudienceActivityHistoryReconciliationResult{ComparisonDigest: emptyDigest, Replayed: true}) || state.records != 1 || archive.packageCalls != 2 || archive.eventCalls != 2 {
		t.Fatalf("empty reconciliation replay = %#v records=%d source_calls=%d/%d", second, state.records, archive.packageCalls, archive.eventCalls)
	}
}

func TestAudienceActivityTerminalMatchesAllThreeArchiveProofs(t *testing.T) {
	key := sha256.Sum256([]byte("source"))
	payload := sha256.Sum256([]byte("payload"))
	field := sha256.Sum256([]byte("field"))
	target := sha256.Sum256([]byte("target"))
	source := activity.SourceEnvelope{SourceOrdinal: 1, SourceKeyHMAC: key, PayloadHMAC: payload, FieldHMAC: field}
	value := AudienceActivityTerminal{Version: AudienceActivityHistoryImportVersion, ArchiveRunID: "run", TableID: activity.PackageRunsTableID, Kind: "package_runs", SourceKeyHMAC: key, PayloadHMAC: payload, FieldHMAC: field, Disposition: string(activity.DispositionCandidate), TargetID: 1, TargetDigest: target}
	if !audienceActivityTerminalMatches(value, "run", activity.PackageRunsTableID, "package_runs", source, activity.DispositionCandidate, "") {
		t.Fatal("exact terminal did not match")
	}
	source.FieldHMAC = sha256.Sum256([]byte("tampered"))
	if audienceActivityTerminalMatches(value, "run", activity.PackageRunsTableID, "package_runs", source, activity.DispositionCandidate, "") {
		t.Fatal("field proof drift accepted")
	}
}

type audienceActivityEmptyReady struct{}

func (audienceActivityEmptyReady) VerifyAudienceActivityArchiveReady(_ context.Context, run string) error {
	if run != "empty-run" {
		return ErrInvalidScope
	}
	return nil
}

type audienceActivityEmptyArchive struct{ packageCalls, eventCalls int }

func (archive *audienceActivityEmptyArchive) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	if run != "empty-run" || callback == nil || (table != activity.PackageRunsTableID && table != activity.MemberEventsTableID) {
		return ErrInvalidScope
	}
	if table == activity.PackageRunsTableID {
		archive.packageCalls++
	} else {
		archive.eventCalls++
	}
	return nil
}

type audienceActivityEmptyUOW struct{}

func (audienceActivityEmptyUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	if ctx == nil || callback == nil {
		return ErrInvalidScope
	}
	return callback(ctx)
}

type audienceActivityEmptyWriter struct{}

func (audienceActivityEmptyWriter) WriteRun(context.Context, string, [32]byte, segment.HistoricalAudienceActivityRun) (segment.AudienceActivityHistoryReceipt, error) {
	return segment.AudienceActivityHistoryReceipt{}, ErrConflict
}
func (audienceActivityEmptyWriter) WriteMemberEvent(context.Context, string, [32]byte, segment.HistoricalAudienceActivityMemberEvent) (segment.AudienceActivityHistoryReceipt, error) {
	return segment.AudienceActivityHistoryReceipt{}, ErrConflict
}

type audienceActivityEmptyReferences struct{}

func (audienceActivityEmptyReferences) ResolveAudienceActivityPackage(context.Context, int64) (segment.AudienceActivityPackageReference, error) {
	return segment.AudienceActivityPackageReference{}, ErrConflict
}
func (audienceActivityEmptyReferences) ResolveAudienceActivityVersion(context.Context, int64) (segment.AudienceActivityVersionReference, error) {
	return segment.AudienceActivityVersionReference{}, ErrConflict
}
func (audienceActivityEmptyReferences) ResolveAudienceActivityMember(context.Context, int64) (segment.AudienceActivityMemberReference, error) {
	return segment.AudienceActivityMemberReference{}, ErrConflict
}
func (audienceActivityEmptyReferences) ResolveAudienceActivityRun(context.Context, int64) (segment.HistoricalAudienceActivityRun, error) {
	return segment.HistoricalAudienceActivityRun{}, ErrConflict
}

type audienceActivityEmptyTargets struct{}

func (audienceActivityEmptyTargets) GetHistoricalAudienceActivityRun(context.Context, int64) (segment.HistoricalAudienceActivityRun, error) {
	return segment.HistoricalAudienceActivityRun{}, ErrConflict
}
func (audienceActivityEmptyTargets) GetHistoricalAudienceActivityMemberEvent(context.Context, int64) (segment.HistoricalAudienceActivityMemberEvent, error) {
	return segment.HistoricalAudienceActivityMemberEvent{}, ErrConflict
}

type audienceActivityEmptyJournal struct{}

func (audienceActivityEmptyJournal) LoadAudienceActivityTerminal(context.Context, string, [32]byte) (AudienceActivityTerminal, bool, error) {
	return AudienceActivityTerminal{}, false, nil
}
func (audienceActivityEmptyJournal) RecordAudienceActivityTerminal(context.Context, AudienceActivityTerminal) error {
	return ErrConflict
}

type audienceActivityEmptySealState struct {
	seal    *AudienceActivitySeal
	records int
}

func (s *audienceActivityEmptySealState) LoadAudienceActivityReconciliationSeal(_ context.Context, version, run string) (AudienceActivitySeal, bool, error) {
	if version != AudienceActivityHistoryImportVersion || run != "empty-run" {
		return AudienceActivitySeal{}, false, ErrInvalidScope
	}
	if s.seal == nil {
		return AudienceActivitySeal{}, false, nil
	}
	return *s.seal, true, nil
}
func (s *audienceActivityEmptySealState) RecordAudienceActivityReconciliationSeal(_ context.Context, value AudienceActivitySeal) error {
	if s.seal != nil {
		return ErrConflict
	}
	s.records++
	copy := value
	s.seal = &copy
	return nil
}
