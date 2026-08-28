package v1hxcmemberusagehistory

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"testing"
	"time"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var memberUsageArchivePreflight = flag.Bool("hxc-member-usage-archive-preflight", false, "stream the frozen V2 archive without target writes")

func TestReconciledHXCMemberUsageArchivePreflight(t *testing.T) {
	if !*memberUsageArchivePreflight {
		t.Skip("opt-in frozen V2 archive validation")
	}
	env := appconfig.LoadV1ArchiveRuntimeEnvironment()
	if env.SourceDatabaseURL != "" || len(env.SourceHMACKey) < sha256.Size || len(env.ArchiveKey) != sha256.Size {
		t.Fatal("local_archive_keys_required_source_dsn_forbidden")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 14*time.Minute)
	defer cancel()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, env.TargetDatabaseURL, []byte(env.ArchiveKey))
	if err != nil {
		t.Fatal("archive_open_failed")
	}
	defer archive.Close()
	var count int64
	var ordinal [8]byte
	digest := sha256.New()
	started := time.Now()
	failureReason := "archive_read"
	err = archive.EachTableRow(ctx, "v1-full-archive-20260827", MemberUsageProjectionTableID, func(row v1archive.ArchivedRow) error {
		result := AdaptMemberUsageObservation(row, []byte(env.SourceHMACKey), count+1)
		if result.Disposition != DispositionCandidate || result.Fact == nil || result.Reason != "" {
			failureReason = result.Reason
			var fields map[string]json.RawMessage
			var decoded sourceJSON
			shapeDecoded := json.Unmarshal(row.Payload, &fields) == nil
			typedDecoded := json.Unmarshal(row.Payload, &decoded) == nil
			payload, payloadErr := v1archive.PayloadHMAC([]byte(env.SourceHMACKey), "ai_audience_hxc_member_usage_projection", row.Payload)
			field, fieldErr := v1archive.FieldHMAC([]byte(env.SourceHMACKey), "ai_audience_hxc_member_usage_projection", row.RedactedFields)
			keyJSON, _ := memberUsageSourceKeyJSON(decoded.Generation, decoded.UnionID, decoded.OwnerUserID)
			key, keyErr := v1archive.SourceKeyHMAC([]byte(env.SourceHMACKey), "ai_audience_hxc_member_usage_projection", keyJSON)
			t.Logf("reason=%s ordinal_match=%t shape_decode=%t exact_shape=%t typed_decode=%t redacted_count=%d projected_nonzero=%t payload_json_valid=%t key_match=%t payload_match=%t field_match=%t", result.Reason, row.SourceOrdinal == count+1, shapeDecoded, hasExactShape(fields), typedDecoded, len(row.RedactedFields), !decoded.ProjectedAt.IsZero(), json.Valid(decoded.PayloadJSON), keyErr == nil && key == row.SourceKeyHMAC, payloadErr == nil && payload == row.PayloadHMAC, fieldErr == nil && field == row.FieldHMAC)
			return errors.New("member_usage_preflight_" + result.Reason)
		}
		binary.BigEndian.PutUint64(ordinal[:], uint64(row.SourceOrdinal))
		_, _ = digest.Write(ordinal[:])
		_, _ = digest.Write(row.SourceKeyHMAC[:])
		_, _ = digest.Write(row.PayloadHMAC[:])
		_, _ = digest.Write(row.FieldHMAC[:])
		count++
		if count%100000 == 0 {
			t.Logf("accepted=%d target_writes=0", count)
		}
		return nil
	})
	if err != nil || count != 810554 {
		t.Fatalf("archive_preflight_failed accepted=%d expected=810554 reason=%s", count, failureReason)
	}
	t.Logf("table=%s rows=%d accepted=%d source_payload_field_hmac=verified ordered_digest=%x elapsed_seconds=%.2f target_writes=0", MemberUsageProjectionTableID, count, count, digest.Sum(nil), time.Since(started).Seconds())
}
