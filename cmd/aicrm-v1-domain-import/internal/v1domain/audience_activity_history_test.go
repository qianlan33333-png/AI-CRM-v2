package v1domain

import (
	"crypto/sha256"
	"testing"

	activity "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1audienceactivityhistory"
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
