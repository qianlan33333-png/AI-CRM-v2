package v1hxcmemberusagehistory

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestStreamerVerifiesFixedBatchesBeforeDeliveryAndReturnsRollingDigest(t *testing.T) {
	if StreamBatchSize != 250 {
		t.Fatalf("fixed batch size changed: %d", StreamBatchSize)
	}
	rows := makeMemberUsageStreamRows(t, 251)
	events := make([]string, 0, 4)
	terminals := &streamTerminalRecorder{events: &events}
	archive := &streamArchive{rows: rows}
	streamer, err := NewStreamer(archive, terminals)
	if err != nil {
		t.Fatal(err)
	}
	var delivered []int
	summary, err := streamer.Stream(context.Background(), StreamOptions{ArchiveRunID: "archive-run", SourceHMACKey: adapterTestKey}, func(_ context.Context, values []MemberUsageObservationFact) error {
		events = append(events, fmt.Sprintf("consume:%d", len(values)))
		delivered = append(delivered, len(values))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.SourceCount != 251 || summary.SourceRollingDigest != streamDigest(rows) ||
		!equalInts(terminals.batchSizes, []int{250, 1}) || !equalInts(delivered, []int{250, 1}) ||
		strings.Join(events, ",") != "terminal:250,consume:250,terminal:1,consume:1" {
		t.Fatalf("stream summary/batches changed: summary=%#v terminals=%v delivered=%v events=%v", summary, terminals.batchSizes, delivered, events)
	}
	if len(terminals.batches) != 2 || len(terminals.batches[0]) != 250 || terminals.batches[0][0].SourceKeyHMAC != rows[0].SourceKeyHMAC ||
		terminals.batches[0][0].PayloadHMAC != rows[0].PayloadHMAC || terminals.batches[0][0].FieldHMAC != rows[0].FieldHMAC {
		t.Fatalf("terminal did not receive complete source proof: %#v", terminals.batches)
	}
}

func TestStreamerNilConsumerIsCompleteReadOnlyPreflight(t *testing.T) {
	rows := makeMemberUsageStreamRows(t, 1)
	terminals := &streamTerminalRecorder{}
	streamer, err := NewStreamer(&streamArchive{rows: rows}, terminals)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := streamer.Stream(context.Background(), StreamOptions{ArchiveRunID: "archive-run", SourceHMACKey: adapterTestKey}, nil)
	if err != nil || summary.SourceCount != 1 || summary.SourceRollingDigest != streamDigest(rows) || !equalInts(terminals.batchSizes, []int{1}) {
		t.Fatalf("preflight result summary=%#v terminal=%v err=%v", summary, terminals.batchSizes, err)
	}
}

func TestStreamerFailsClosedBeforeUnverifiedBatchDelivery(t *testing.T) {
	for _, test := range []struct {
		name     string
		rows     func() []v1archive.ArchivedRow
		terminal error
		consume  error
		want     error
	}{
		{name: "ordinal gap", rows: func() []v1archive.ArchivedRow {
			rows := makeMemberUsageStreamRows(t, 1)
			rows[0].SourceOrdinal = 2
			return rows
		}, want: ErrSealedStreamDrift},
		{name: "source HMAC", rows: func() []v1archive.ArchivedRow {
			rows := makeMemberUsageStreamRows(t, 1)
			rows[0].SourceKeyHMAC[0]++
			return rows
		}, want: ErrSealedStreamDrift},
		{name: "payload HMAC", rows: func() []v1archive.ArchivedRow {
			rows := makeMemberUsageStreamRows(t, 1)
			rows[0].PayloadHMAC[0]++
			return rows
		}, want: ErrSealedStreamDrift},
		{name: "field HMAC", rows: func() []v1archive.ArchivedRow {
			rows := makeMemberUsageStreamRows(t, 1)
			rows[0].FieldHMAC[0]++
			return rows
		}, want: ErrSealedStreamDrift},
		{name: "duplicate source key in batch", rows: func() []v1archive.ArchivedRow {
			rows := makeMemberUsageStreamRows(t, 1)
			duplicate := rows[0]
			duplicate.SourceOrdinal = 2
			return append(rows, duplicate)
		}, want: ErrSealedStreamDrift},
		{name: "terminal failure", rows: func() []v1archive.ArchivedRow { return makeMemberUsageStreamRows(t, 1) }, terminal: errors.New("terminal failed"), want: ErrTerminalVerification},
		{name: "consumer failure", rows: func() []v1archive.ArchivedRow { return makeMemberUsageStreamRows(t, 1) }, consume: errors.New("consume failed"), want: ErrStreamConsumer},
	} {
		t.Run(test.name, func(t *testing.T) {
			terminals := &streamTerminalRecorder{err: test.terminal}
			streamer, err := NewStreamer(&streamArchive{rows: test.rows()}, terminals)
			if err != nil {
				t.Fatal(err)
			}
			var delivered int
			summary, err := streamer.Stream(context.Background(), StreamOptions{ArchiveRunID: "archive-run", SourceHMACKey: adapterTestKey}, func(_ context.Context, values []MemberUsageObservationFact) error {
				delivered += len(values)
				return test.consume
			})
			if !errors.Is(err, test.want) || summary != (StreamSummary{}) {
				t.Fatalf("stream error=%v summary=%#v", err, summary)
			}
			if test.want == ErrTerminalVerification && delivered != 0 {
				t.Fatalf("unverified batch reached consumer: delivered=%d", delivered)
			}
		})
	}
}

func TestStreamerRejectsInvalidConstructionAndOptions(t *testing.T) {
	if _, err := NewStreamer(nil, &streamTerminalRecorder{}); !errors.Is(err, ErrInvalidStream) {
		t.Fatalf("nil archive error=%v", err)
	}
	streamer, err := NewStreamer(&streamArchive{}, &streamTerminalRecorder{})
	if err != nil {
		t.Fatal(err)
	}
	for _, options := range []StreamOptions{{ArchiveRunID: " archive-run", SourceHMACKey: adapterTestKey}, {ArchiveRunID: "archive-run", SourceHMACKey: adapterTestKey[:sha256.Size-1]}} {
		if _, err := streamer.Stream(context.Background(), options, nil); !errors.Is(err, ErrInvalidStream) {
			t.Fatalf("invalid options accepted: %#v err=%v", options, err)
		}
	}
}

type streamArchive struct {
	rows []v1archive.ArchivedRow
}

func (source *streamArchive) EachTableRow(_ context.Context, run, table string, emit func(v1archive.ArchivedRow) error) error {
	if run != "archive-run" || table != MemberUsageProjectionTableID {
		return errors.New("unexpected archive scope")
	}
	for _, row := range source.rows {
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

type streamTerminalRecorder struct {
	batchSizes []int
	batches    [][]SourceEnvelope
	err        error
	events     *[]string
}

func (reader *streamTerminalRecorder) VerifyArchiveTerminals(_ context.Context, run string, rows []SourceEnvelope) error {
	if run != "archive-run" {
		return errors.New("unexpected terminal run")
	}
	reader.batchSizes = append(reader.batchSizes, len(rows))
	reader.batches = append(reader.batches, append([]SourceEnvelope(nil), rows...))
	if reader.events != nil {
		*reader.events = append(*reader.events, fmt.Sprintf("terminal:%d", len(rows)))
	}
	return reader.err
}

func makeMemberUsageStreamRows(t *testing.T, count int) []v1archive.ArchivedRow {
	t.Helper()
	rows := make([]v1archive.ArchivedRow, 0, count)
	for ordinal := 1; ordinal <= count; ordinal++ {
		value := memberUsagePayload()
		value["generation"] = int64(ordinal)
		value["owner_userid"] = fmt.Sprintf("owner-%d", ordinal)
		value["unionid"] = fmt.Sprintf("union-%d", ordinal)
		row := memberUsageRow(t, value)
		row.SourceOrdinal = int64(ordinal)
		rows = append(rows, row)
	}
	return rows
}

func streamDigest(rows []v1archive.ArchivedRow) [sha256.Size]byte {
	digest := sha256.New()
	for _, row := range rows {
		writeEnvelopeDigest(digest, SourceEnvelope{SourceOrdinal: row.SourceOrdinal, SourceKeyHMAC: row.SourceKeyHMAC, PayloadHMAC: row.PayloadHMAC, FieldHMAC: row.FieldHMAC})
	}
	var result [sha256.Size]byte
	copy(result[:], digest.Sum(nil))
	return result
}

func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
