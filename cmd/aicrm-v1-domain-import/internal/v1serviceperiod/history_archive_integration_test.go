package v1serviceperiod

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"sort"
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var servicePeriodArchiveRun = flag.String("service-period-archive-run", "", "optional reconciled V2 archive run for read-only service-period candidate validation")

func TestReconciledServicePeriodArchiveShape(t *testing.T) {
	if *servicePeriodArchiveRun == "" {
		t.Skip("supply -service-period-archive-run and V2 archive environment for read-only candidate validation")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("archive_open_failed")
	}
	defer archive.Close()
	var payloads [3][]json.RawMessage
	for index, table := range []struct {
		name, retained string
		count          int
	}{
		{"service_period_products", "id trade_product_id membership_config_id membership_config_name duration_days deleted created_at updated_at", 2},
		{"service_period_entitlements", "id service_product_id trade_product_id unionid external_userid_snapshot membership_config_id status start_at end_at last_order_id last_out_trade_no renewal_count created_at updated_at", 93},
		{"service_period_events", "id event_id service_product_id entitlement_id trade_product_id order_id out_trade_no unionid event_type duration_days before_start_at before_end_at after_start_at after_end_at created_at", 118},
	} {
		seen := make(map[[sha256.Size]byte]bool)
		verificationReason := ""
		err = archive.EachTableRow(ctx, *servicePeriodArchiveRun, "public/"+table.name, func(row v1archive.ArchivedRow) error {
			verificationReason = servicePeriodArchiveRowReason(row, table.name, table.retained, int64(len(payloads[index])+1))
			if verificationReason != "" {
				return errors.New(verificationReason)
			}
			if seen[row.SourceKeyHMAC] {
				verificationReason = "archive_duplicate_source_key"
				return errors.New(verificationReason)
			}
			seen[row.SourceKeyHMAC] = true
			payloads[index] = append(payloads[index], row.Payload)
			return nil
		})
		if err != nil {
			// Reader errors may contain source values. Only callback codes are safe.
			if verificationReason != "" {
				t.Fatal(verificationReason)
			}
			t.Fatal("archive_read_failed")
		}
		if len(payloads[index]) != table.count {
			t.Fatalf("archive_count_mismatch table_index=%d count=%d", index, len(payloads[index]))
		}
	}
	history := AdaptHistory(payloads[0], payloads[1], payloads[2])
	if len(history.Products) != 2 || len(history.Entitlements) != 93 || len(history.Events) != 118 {
		t.Fatal("candidate_row_conservation_failed")
	}
	counts, reasons := map[string]int{}, map[string]int{}
	quarantined := 0
	count := func(kind string, disposition Disposition, reason string, hasFact bool) {
		if disposition == DispositionCandidate && hasFact && reason == "" {
			counts[kind]++
			return
		}
		if (disposition != DispositionPending && disposition != DispositionInvalid) || hasFact || reason == "" {
			t.Fatal("candidate_disposition_invalid")
		}
		quarantined++
		reasons[reason]++
	}
	for _, row := range history.Products {
		count("products", row.Disposition, row.Reason, row.Fact != nil)
	}
	for _, row := range history.Entitlements {
		count("entitlements", row.Disposition, row.Reason, row.Fact != nil)
	}
	for _, row := range history.Events {
		count("events", row.Disposition, row.Reason, row.Fact != nil)
	}
	codes := make([]string, 0, len(reasons))
	for reason := range reasons {
		codes = append(codes, reason)
	}
	sort.Strings(codes)
	for _, reason := range codes {
		t.Logf("reason=%s count=%d", reason, reasons[reason])
	}
	t.Logf("source_products=2 source_entitlements=93 source_events=118 candidate_products=%d candidate_entitlements=%d candidate_events=%d quarantine_rows=%d target_writer_checked=0 target_fk_checked=0 target_writes=0", counts["products"], counts["entitlements"], counts["events"], quarantined)
	if counts["products"]+counts["entitlements"]+counts["events"]+quarantined != 213 {
		t.Fatal("candidate_terminal_count_mismatch")
	}
	if quarantined > 0 {
		t.Errorf("candidate_rows_quarantined count=%d", quarantined)
	}
}

func servicePeriodArchiveRowReason(row v1archive.ArchivedRow, table, retained string, ordinal int64) string {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != "public/"+table || row.SourceOrdinal != ordinal {
		return "archive_scope_or_ordinal_mismatch"
	}
	if row.SourceKeyHMAC == [sha256.Size]byte{} || row.PayloadHMAC == [sha256.Size]byte{} || row.FieldHMAC == [sha256.Size]byte{} {
		return "archive_hmac_missing"
	}
	for _, field := range strings.Fields(retained) {
		for _, path := range row.RedactedFields {
			if path == field || strings.HasPrefix(path, field+".") {
				return "archive_retained_field_redacted"
			}
		}
	}
	return ""
}

func TestServicePeriodArchiveRowChecksScopeDigestsAndRedaction(t *testing.T) {
	const table = "service_period_products"
	const retained = "id trade_product_id membership_config_id"
	row := v1archive.ArchivedRow{
		AdapterID: v1archive.DefaultAdapterID, TableID: "public/" + table, SourceOrdinal: 1,
		SourceKeyHMAC: [sha256.Size]byte{1}, PayloadHMAC: [sha256.Size]byte{2}, FieldHMAC: [sha256.Size]byte{3},
		RedactedFields: []string{"metadata_json.access_token"},
	}
	if reason := servicePeriodArchiveRowReason(row, table, retained, 1); reason != "" {
		t.Fatal(reason)
	}
	for _, test := range []struct {
		mutate func(*v1archive.ArchivedRow)
		reason string
	}{
		{func(r *v1archive.ArchivedRow) { r.AdapterID = "other" }, "archive_scope_or_ordinal_mismatch"},
		{func(r *v1archive.ArchivedRow) { r.TableID = "public/other" }, "archive_scope_or_ordinal_mismatch"},
		{func(r *v1archive.ArchivedRow) { r.SourceOrdinal++ }, "archive_scope_or_ordinal_mismatch"},
		{func(r *v1archive.ArchivedRow) { r.RedactedFields = []string{"membership_config_id"} }, "archive_retained_field_redacted"},
		{func(r *v1archive.ArchivedRow) { r.SourceKeyHMAC = [sha256.Size]byte{} }, "archive_hmac_missing"},
		{func(r *v1archive.ArchivedRow) { r.PayloadHMAC = [sha256.Size]byte{} }, "archive_hmac_missing"},
		{func(r *v1archive.ArchivedRow) { r.FieldHMAC = [sha256.Size]byte{} }, "archive_hmac_missing"},
	} {
		changed := row
		test.mutate(&changed)
		if reason := servicePeriodArchiveRowReason(changed, table, retained, 1); reason != test.reason {
			t.Fatal("archive_verification_reason_mismatch")
		}
	}
}
