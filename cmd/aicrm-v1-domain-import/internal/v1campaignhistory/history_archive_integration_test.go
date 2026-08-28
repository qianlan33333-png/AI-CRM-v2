package v1campaignhistory

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"sort"
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var campaignHistoryArchiveRun = flag.String("campaign-history-archive-run", "", "optional reconciled V2 archive run for read-only Campaign history candidate validation")

func TestReconciledCampaignHistoryArchiveShape(t *testing.T) {
	if *campaignHistoryArchiveRun == "" {
		t.Skip("supply -campaign-history-archive-run and V2 archive environment for read-only candidate validation")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("campaign_history_archive_open_failed")
	}
	defer archive.Close()
	var payloads [3][]json.RawMessage
	for i, table := range []struct {
		name, retained string
		count          int
	}{
		{CampaignTable, campaignRequired + " " + campaignNullable, 6382},
		{SegmentTable, segmentRequired, 7338},
		{MemberTable, memberRequired + " " + memberNullable, 6707},
	} {
		seen := map[[32]byte]bool{}
		reason := ""
		err = archive.EachTableRow(ctx, *campaignHistoryArchiveRun, table.name, func(row v1archive.ArchivedRow) error {
			reason = archiveRowReason(row, table.name, table.retained, int64(len(payloads[i])+1))
			if reason == "" && seen[row.SourceKeyHMAC] {
				reason = "archive_duplicate_source_key"
			}
			if reason != "" {
				return errors.New(reason)
			}
			seen[row.SourceKeyHMAC] = true
			payloads[i] = append(payloads[i], row.Payload)
			return nil
		})
		if err != nil {
			// Reader errors can include source material; log only fixed codes.
			if reason != "" {
				t.Fatal(reason)
			}
			t.Fatal("campaign_history_archive_read_failed")
		}
		if len(payloads[i]) != table.count {
			t.Fatalf("archive_count_mismatch table_index=%d count=%d", i, len(payloads[i]))
		}
	}
	h := AdaptHistory(payloads[0], payloads[1], payloads[2])
	if len(h.Campaigns) != len(payloads[0]) || len(h.Segments) != len(payloads[1]) || len(h.Members) != len(payloads[2]) {
		t.Fatal("candidate_row_conservation_failed")
	}
	counts, reasons := [3]int{}, map[string]int{}
	count := func(kind int, disposition Disposition, reason string, hasFact bool) {
		if disposition == Candidate && hasFact && reason == "" {
			counts[kind]++
			return
		}
		if (disposition != Pending && disposition != Invalid) || hasFact || reason == "" {
			t.Fatal("candidate_disposition_invalid")
		}
		reasons[reason]++
	}
	for _, row := range h.Campaigns {
		count(0, row.Disposition, row.Reason, row.Fact != nil)
	}
	for _, row := range h.Segments {
		count(1, row.Disposition, row.Reason, row.Fact != nil)
	}
	for _, row := range h.Members {
		count(2, row.Disposition, row.Reason, row.Fact != nil)
	}
	codes, isolated := make([]string, 0, len(reasons)), 0
	for reason, count := range reasons {
		codes = append(codes, reason)
		isolated += count
	}
	sort.Strings(codes)
	for _, reason := range codes {
		t.Logf("reason=%s count=%d", reason, reasons[reason])
	}
	t.Logf("source_campaigns=%d source_segments=%d source_members=%d candidate_campaigns=%d candidate_segments=%d candidate_members=%d isolated_rows=%d source_relations_only=1 target_writer_checked=0 target_fk_checked=0 target_writes=0", len(payloads[0]), len(payloads[1]), len(payloads[2]), counts[0], counts[1], counts[2], isolated)
	if counts[0]+counts[1]+counts[2]+isolated != 6382+7338+6707 {
		t.Fatal("candidate_terminal_count_mismatch")
	}
	if isolated > 0 {
		t.Errorf("campaign_history_rows_isolated count=%d", isolated)
	}
}

func archiveRowReason(row v1archive.ArchivedRow, table, retained string, ordinal int64) string {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal {
		return "archive_scope_or_ordinal_mismatch"
	}
	if row.SourceKeyHMAC == [32]byte{} || row.PayloadHMAC == [32]byte{} || row.FieldHMAC == [32]byte{} {
		return "archive_hmac_missing"
	}
	for _, field := range strings.Fields(retained) {
		for _, redacted := range row.RedactedFields {
			if redacted == field || strings.HasPrefix(redacted, field+".") {
				return "archive_retained_field_redacted"
			}
		}
	}
	return ""
}

func TestCampaignHistoryArchiveScopeAndRedaction(t *testing.T) {
	row := v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: CampaignTable, SourceOrdinal: 1, SourceKeyHMAC: [32]byte{1}, PayloadHMAC: [32]byte{2}, FieldHMAC: [32]byte{3}, RedactedFields: []string{"approval_token_hash", "metadata_json.access_token"}}
	retained := campaignRequired + " " + campaignNullable
	if archiveRowReason(row, CampaignTable, retained, 1) != "" {
		t.Fatal("unretained_redaction_rejected")
	}
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(r *v1archive.ArchivedRow) { r.AdapterID = "wrong" },
		func(r *v1archive.ArchivedRow) { r.TableID = MemberTable },
		func(r *v1archive.ArchivedRow) { r.SourceOrdinal = 2 },
		func(r *v1archive.ArchivedRow) { r.SourceKeyHMAC = [32]byte{} },
		func(r *v1archive.ArchivedRow) { r.PayloadHMAC = [32]byte{} },
		func(r *v1archive.ArchivedRow) { r.FieldHMAC = [32]byte{} },
		func(r *v1archive.ArchivedRow) { r.RedactedFields = []string{"owner_userid"} },
		func(r *v1archive.ArchivedRow) { r.RedactedFields = []string{"run_status.child"} },
	} {
		changed := row
		mutate(&changed)
		if archiveRowReason(changed, CampaignTable, retained, 1) == "" {
			t.Fatal("invalid_archive_row_accepted")
		}
	}
}
