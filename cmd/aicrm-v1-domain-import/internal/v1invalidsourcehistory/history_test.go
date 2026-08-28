package v1invalidsourcehistory

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestSelectPreservesSixteenSealedInvalidFacts(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	archive, terminals := invalidSourceFixture(t, key)
	selected, err := Select(context.Background(), archive, terminals, Options{ArchiveRunID: "archive-run", SourceHMACKey: key})
	if err != nil || selected.Summary() != (Summary{UnboundTags: 5, InvalidChannels: 1, Images: 3, Attachments: 1, RadarLinks: 6}) || selected.Summary().Total() != 16 {
		t.Fatalf("selected=%+v summary=%+v err=%v", selected, selected.Summary(), err)
	}
	if selected.UnboundTags[0].Fact.UnionIDDigest != ([sha256.Size]byte{}) || selected.UnboundTags[0].Fact.PrivateDigest == ([sha256.Size]byte{}) || selected.InvalidAssets[0].Fact.ContentDigest == ([sha256.Size]byte{}) || selected.InvalidRadar[0].Fact.DestinationURLDigest == ([sha256.Size]byte{}) {
		t.Fatal("source-only private evidence missing or unbound tag acquired identity")
	}
	if selected.InvalidChannels[0].Fact.Code != "" || selected.InvalidChannels[0].Fact.QuarantineReason != "invalid_channel_definition" {
		t.Fatal("invalid channel was normalized or reclassified")
	}
}

func TestSelectRejectsEnvelopeAndTerminalDrift(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	for _, mutate := range []func(*invalidSourceArchive, *invalidSourceTerminals){
		func(archive *invalidSourceArchive, _ *invalidSourceTerminals) {
			archive.rows[ImageLibraryTable][0].PayloadHMAC = sha256.Sum256([]byte("drift"))
		},
		func(_ *invalidSourceArchive, terminals *invalidSourceTerminals) {
			for key, receipt := range terminals.receipts {
				if receipt.Reason == "invalid_radar_definition" {
					receipt.Reason = "wrong_reason"
					terminals.receipts[key] = receipt
					return
				}
			}
		},
	} {
		archive, terminals := invalidSourceFixture(t, key)
		mutate(archive, terminals)
		if _, err := Select(context.Background(), archive, terminals, Options{ArchiveRunID: "archive-run", SourceHMACKey: key}); err == nil {
			t.Fatal("sealed drift accepted")
		}
	}
}

type invalidSourceArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive *invalidSourceArchive) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	if run != "archive-run" || callback == nil {
		return ErrInvalidSelection
	}
	for _, row := range archive.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type invalidSourceTerminals struct{ receipts map[string]TerminalReceipt }

func (loader *invalidSourceTerminals) LoadTerminal(_ context.Context, scope TerminalScope, source string) (TerminalReceipt, bool, error) {
	receipt, found := loader.receipts[terminalKey(scope, source)]
	return receipt, found, nil
}

func invalidSourceFixture(t *testing.T, key []byte) (*invalidSourceArchive, *invalidSourceTerminals) {
	t.Helper()
	archive := &invalidSourceArchive{rows: map[string][]v1archive.ArchivedRow{}}
	terminals := &invalidSourceTerminals{receipts: map[string]TerminalReceipt{}}
	stamp := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	for _, spec := range invalidSourceSpecs {
		count := map[string]int{ContactTagsTable: 5, AutomationChannelTable: 1, ImageLibraryTable: 3, AttachmentLibraryTable: 1, RadarLinksTable: 6}[spec.scope.TableID]
		for ordinal := 1; ordinal <= count; ordinal++ {
			row := invalidSourceRow(t, key, spec.scope.TableID, int64(ordinal), invalidSourcePayload(spec.scope.TableID, ordinal, stamp))
			archive.rows[spec.scope.TableID] = append(archive.rows[spec.scope.TableID], row)
			terminals.receipts[terminalKey(spec.scope, sourceIdentifier(row.SourceKeyHMAC))] = TerminalReceipt{Verified: true, SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: spec.reason, Metadata: map[string]any{}}
		}
	}
	return archive, terminals
}

func invalidSourcePayload(table string, ordinal int, stamp time.Time) map[string]any {
	switch table {
	case ContactTagsTable:
		return map[string]any{"tag_id": fmt.Sprintf("tag-%d", ordinal), "unionid": "", "created_at": stamp}
	case AutomationChannelTable:
		return map[string]any{"id": ordinal, "channel_code": "", "channel_name": "legacy", "channel_type": "qrcode", "carrier_type": "qrcode", "created_at": stamp, "updated_at": stamp}
	case ImageLibraryTable, AttachmentLibraryTable:
		return map[string]any{"id": ordinal, "name": "legacy", "file_name": "legacy.bin", "mime_type": "application/octet-stream", "file_size": 1, "data_base64": base64.StdEncoding.EncodeToString([]byte{byte(ordinal)}), "enabled": false, "created_at": stamp, "updated_at": stamp}
	case RadarLinksTable:
		return map[string]any{"id": ordinal, "code": "", "title": "legacy", "original_url": "https://invalid.example/", "created_at": stamp, "updated_at": stamp}
	default:
		panic("unknown fixture table")
	}
}

func invalidSourceRow(t *testing.T, key []byte, table string, ordinal int64, payload map[string]any) v1archive.ArchivedRow {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	payloadDigest, err := v1archive.PayloadHMAC(key, table[len("public/"):], encoded)
	if err != nil {
		t.Fatal(err)
	}
	fieldDigest, err := v1archive.FieldHMAC(key, table[len("public/"):], nil)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/%d", table, ordinal))), PayloadHMAC: payloadDigest, FieldHMAC: fieldDigest, Payload: encoded}
}

func terminalKey(scope TerminalScope, source string) string {
	return scope.ImportVersion + "\x00" + scope.TableID + "\x00" + scope.TargetDomain + "\x00" + scope.TargetTable + "\x00" + source
}
