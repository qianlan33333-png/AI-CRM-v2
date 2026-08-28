package v1wecomcontacthistory

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"sort"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	wecomContactHistoryArchiveRunID = "v1-full-archive-20260827"
	eventLogArchiveRows             = 56880
	followUserArchiveRows           = 50872
)

var wecomContactHistoryPreflightDSN = flag.String("wecom-contact-history-preflight-dsn", "", "V2 archive PostgreSQL DSN for read-only WeCom contact history preflight")

// TestWeComContactHistoryArchivePreflight is opt-in: it opens the existing V2
// archive reader only when a local, explicit V2 archive DSN is supplied. The
// reader itself uses a read-only transaction; this test has no target writer.
func TestWeComContactHistoryArchivePreflight(t *testing.T) {
	if *wecomContactHistoryPreflightDSN == "" {
		t.Skip("supply -wecom-contact-history-preflight-dsn and V2 archive encryption key for read-only preflight")
	}
	ctx := context.Background()
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, *wecomContactHistoryPreflightDSN, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("wecom_contact_history_archive_open_failed")
	}
	defer archive.Close()

	eventLogs := readWeComContactHistoryRows(t, ctx, archive, ExternalContactEventLogsTableID, eventLogArchiveRows)
	followUsers := readWeComContactHistoryRows(t, ctx, archive, ExternalContactFollowUsersTableID, followUserArchiveRows)
	history := AdaptHistory(eventLogs, followUsers)
	if history.SourceCount() != eventLogArchiveRows+followUserArchiveRows || history.TerminalCount() != history.SourceCount() {
		t.Fatal("wecom_contact_history_preflight_row_conservation_failed")
	}
	report, err := wecomContactHistoryReport(history)
	if err != nil || report.Candidates+report.Quarantined != history.SourceCount() {
		t.Fatal("wecom_contact_history_preflight_result_invalid")
	}
	envelopeDigest := wecomContactArchiveEnvelopeDigest(eventLogs, followUsers)
	outcomeDigest := wecomContactHistoryOutcomeDigest(history)
	if envelopeDigest != wecomContactArchiveEnvelopeDigest(eventLogs, followUsers) || outcomeDigest != wecomContactHistoryOutcomeDigest(history) {
		t.Fatal("wecom_contact_history_preflight_nondeterministic")
	}
	for _, reason := range report.SortedReasons() {
		t.Logf("wecom_contact_history_preflight reason=%s count=%d", reason, report.Reasons[reason])
	}
	t.Logf("wecom_contact_history_preflight event_logs=%d follow_users=%d total=%d candidates=%d quarantined=%d archive_envelope_sha256=%x outcome_sha256=%x target_writes=0 provider_effects=0", len(eventLogs), len(followUsers), history.SourceCount(), report.Candidates, report.Quarantined, envelopeDigest, outcomeDigest)
}

func readWeComContactHistoryRows(t *testing.T, ctx context.Context, archive *v1archive.PostgresArchiveReader, tableID string, expected int) []v1archive.ArchivedRow {
	t.Helper()
	rows := make([]v1archive.ArchivedRow, 0, expected)
	err := archive.EachTableRow(ctx, wecomContactHistoryArchiveRunID, tableID, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != tableID || row.SourceOrdinal != int64(len(rows)+1) {
			return errors.New("archive row scope or ordinal mismatch")
		}
		rows = append(rows, row)
		return nil
	})
	if err != nil || len(rows) != expected {
		t.Fatal("wecom_contact_history_archive_count_or_ordinal_invalid")
	}
	return rows
}

type wecomContactHistoryPreflightReport struct {
	Candidates, Quarantined int
	Reasons                 map[string]int
}

func (report wecomContactHistoryPreflightReport) SortedReasons() []string {
	keys := make([]string, 0, len(report.Reasons))
	for reason := range report.Reasons {
		keys = append(keys, reason)
	}
	sort.Strings(keys)
	return keys
}

func wecomContactHistoryReport(history History) (wecomContactHistoryPreflightReport, error) {
	report := wecomContactHistoryPreflightReport{Reasons: make(map[string]int)}
	for _, result := range history.EventLogs {
		if err := report.record(result.Disposition, result.Fact != nil, result.Reason); err != nil {
			return wecomContactHistoryPreflightReport{}, err
		}
	}
	for _, result := range history.FollowUsers {
		if err := report.record(result.Disposition, result.Fact != nil, result.Reason); err != nil {
			return wecomContactHistoryPreflightReport{}, err
		}
	}
	return report, nil
}

func (report *wecomContactHistoryPreflightReport) record(disposition Disposition, hasFact bool, reason string) error {
	switch disposition {
	case DispositionCandidate:
		if !hasFact || reason != "" {
			return errors.New("inconsistent candidate")
		}
		report.Candidates++
	case DispositionQuarantine:
		if hasFact || reason == "" {
			return errors.New("inconsistent quarantine")
		}
		report.Quarantined++
		report.Reasons[reason]++
	default:
		return errors.New("unknown disposition")
	}
	return nil
}

// wecomContactArchiveEnvelopeDigest summarizes archive binding without source
// payloads. It is deliberately separate from the outcome digest below.
func wecomContactArchiveEnvelopeDigest(eventLogs, followUsers []v1archive.ArchivedRow) [sha256.Size]byte {
	hash := sha256.New()
	for _, set := range []struct {
		table string
		rows  []v1archive.ArchivedRow
	}{
		{ExternalContactEventLogsTableID, eventLogs},
		{ExternalContactFollowUsersTableID, followUsers},
	} {
		for _, row := range set.rows {
			_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%x\x00%x\x00%x\x00", set.table, row.SourceOrdinal, row.SourceKeyHMAC, row.PayloadHMAC, row.FieldHMAC)
		}
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result
}

// wecomContactHistoryOutcomeDigest summarizes only classifications and fixed
// reason codes. It cannot reveal source values and is deterministic by table
// order and archive position.
func wecomContactHistoryOutcomeDigest(history History) [sha256.Size]byte {
	hash := sha256.New()
	for _, set := range []struct {
		table  string
		states []resultState
	}{
		{ExternalContactEventLogsTableID, eventLogResultStates(history.EventLogs)},
		{ExternalContactFollowUsersTableID, followUserResultStates(history.FollowUsers)},
	} {
		for ordinal, state := range set.states {
			_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%s\x00%s\x00", set.table, ordinal+1, state.Disposition, state.Reason)
		}
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result
}

type resultState struct {
	Disposition Disposition
	Reason      string
}

func eventLogResultStates(values []Result[ExternalContactEventLogFact]) []resultState {
	states := make([]resultState, len(values))
	for i, value := range values {
		states[i] = resultState{Disposition: value.Disposition, Reason: value.Reason}
	}
	return states
}

func followUserResultStates(values []Result[ExternalContactFollowUserFact]) []resultState {
	states := make([]resultState, len(values))
	for i, value := range values {
		states[i] = resultState{Disposition: value.Disposition, Reason: value.Reason}
	}
	return states
}

func TestWeComContactHistoryPreflightSummaryIsDeterministic(t *testing.T) {
	event, follow := fixtures()
	eventRows := []v1archive.ArchivedRow{archiveRow(t, ExternalContactEventLogsTableID, 1, event)}
	followRows := []v1archive.ArchivedRow{archiveRow(t, ExternalContactFollowUsersTableID, 1, follow)}
	history := AdaptHistory(eventRows, followRows)
	report, err := wecomContactHistoryReport(history)
	if err != nil || report.Candidates != 2 || report.Quarantined != 0 || len(report.Reasons) != 0 {
		t.Fatalf("unexpected preflight report: %+v err=%v", report, err)
	}
	if wecomContactArchiveEnvelopeDigest(eventRows, followRows) != wecomContactArchiveEnvelopeDigest(eventRows, followRows) || wecomContactHistoryOutcomeDigest(history) != wecomContactHistoryOutcomeDigest(history) {
		t.Fatal("preflight summaries are not deterministic")
	}
	history.EventLogs[0] = quarantine[ExternalContactEventLogFact](history.EventLogs[0].SourceID, "forced_test_isolation")
	report, err = wecomContactHistoryReport(history)
	if err != nil || report.Candidates != 1 || report.Quarantined != 1 || report.Reasons["forced_test_isolation"] != 1 {
		t.Fatalf("isolation reason count lost: %+v err=%v", report, err)
	}
	if wecomContactHistoryOutcomeDigest(history) == wecomContactHistoryOutcomeDigest(AdaptHistory(eventRows, followRows)) {
		t.Fatal("outcome digest did not bind the classification")
	}
}
