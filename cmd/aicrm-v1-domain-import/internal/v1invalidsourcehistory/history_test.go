package v1invalidsourcehistory

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
)

func TestSelectPreservesSixteenSealedInvalidFacts(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	archive, terminals := invalidSourceFixture(t, key)
	selected, err := Select(context.Background(), archive, terminals, Options{ArchiveRunID: "archive-run", SourceHMACKey: key})
	if err != nil || selected.Summary() != (Summary{UnboundTags: 5, InvalidChannels: 1, Images: 3, Attachments: 1, RadarLinks: 6}) || selected.Summary().Total() != 16 {
		t.Fatalf("selected=%+v summary=%+v err=%v", selected, selected.Summary(), err)
	}
	if selected.UnboundTags[0].Fact.UnionIDDigest == ([sha256.Size]byte{}) || selected.UnboundTags[0].Fact.PrivateDigest == ([sha256.Size]byte{}) || selected.InvalidAssets[0].Fact.ContentDigest == ([sha256.Size]byte{}) || selected.InvalidRadar[0].Fact.DestinationURLDigest == ([sha256.Size]byte{}) {
		t.Fatal("source-only private evidence missing")
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

func TestSelectionFactsPassEachOwnerTypedDigest(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	archive, terminals := invalidSourceFixture(t, key)
	selected, err := Select(context.Background(), archive, terminals, Options{ArchiveRunID: "archive-run", SourceHMACKey: key})
	if err != nil || selected.Summary().Total() != 16 {
		t.Fatalf("select: summary=%+v err=%v", selected.Summary(), err)
	}
	for _, selected := range selected.UnboundTags {
		fact := selected.Fact
		fact.ID = 1
		if _, err := contactapp.DigestHistoricalUnboundTag(fact); err != nil {
			t.Fatalf("tag %d: %v", selected.SourceOrdinal, err)
		}
	}
	for _, selected := range selected.InvalidChannels {
		fact := selected.Fact
		fact.ID = 1
		if _, err := contactapp.DigestHistoricalInvalidChannel(fact); err != nil {
			t.Fatalf("channel %d: %v", selected.SourceOrdinal, err)
		}
	}
	for _, selected := range selected.InvalidAssets {
		fact := selected.Fact
		fact.ID = 1
		if _, err := mediaapp.DigestHistoricalInvalidAsset(fact); err != nil {
			t.Fatalf("asset %s/%d: %v", fact.Kind, selected.SourceOrdinal, err)
		}
	}
	for _, selected := range selected.InvalidRadar {
		fact := selected.Fact
		fact.ID = 1
		if _, err := radarapp.DigestHistoricalInvalidRadarLink(fact); err != nil {
			t.Fatalf("radar %d: %v", selected.SourceOrdinal, err)
		}
	}
}

func TestSelectDistinguishesMissingNullAndEmptyUnionID(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	var digests [][sha256.Size]byte
	for _, union := range []struct {
		name  string
		value any
		set   bool
	}{{"missing", nil, false}, {"null", nil, true}, {"empty", "", true}} {
		t.Run(union.name, func(t *testing.T) {
			archive, terminals := invalidSourceFixture(t, key)
			payload := invalidSourcePayload(ContactTagsTable, 1, time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC))
			if union.set {
				payload["unionid"] = union.value
			} else {
				delete(payload, "unionid")
			}
			row := invalidSourceRow(t, key, ContactTagsTable, 1, payload)
			archive.rows[ContactTagsTable][0] = row
			scope := invalidSourceSpecs[0].scope
			terminalKey := terminalKey(scope, sourceIdentifier(row.SourceKeyHMAC))
			receipt := terminals.receipts[terminalKey]
			receipt.PayloadDigest = row.PayloadHMAC
			terminals.receipts[terminalKey] = receipt
			selected, err := Select(context.Background(), archive, terminals, Options{ArchiveRunID: "archive-run", SourceHMACKey: key})
			if err != nil {
				t.Fatal(err)
			}
			fact := selected.UnboundTags[0].Fact
			fact.ID = 1
			if fact.UnionIDDigest == ([sha256.Size]byte{}) {
				t.Fatal("missing identity evidence digest")
			}
			if _, err := contactapp.DigestHistoricalUnboundTag(fact); err != nil {
				t.Fatalf("owner rejected source-only tag: %v", err)
			}
			digests = append(digests, fact.UnionIDDigest)
		})
	}
	if digests[0] == digests[1] || digests[0] == digests[2] || digests[1] == digests[2] {
		t.Fatalf("missing/null/empty unionid collapsed: %x", digests)
	}
}

func TestSelectPreservesInvalidBase64AsPrivateEvidence(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	archive, terminals := invalidSourceFixture(t, key)
	payload := invalidSourcePayload(ImageLibraryTable, 1, time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC))
	payload["data_base64"] = "not-valid-base64$"
	row := invalidSourceRow(t, key, ImageLibraryTable, 1, payload)
	archive.rows[ImageLibraryTable][0] = row
	scope := invalidSourceSpecs[2].scope
	keyName := terminalKey(scope, sourceIdentifier(row.SourceKeyHMAC))
	receipt := terminals.receipts[keyName]
	receipt.PayloadDigest = row.PayloadHMAC
	terminals.receipts[keyName] = receipt
	selected, err := Select(context.Background(), archive, terminals, Options{ArchiveRunID: "archive-run", SourceHMACKey: key})
	if err != nil {
		t.Fatalf("invalid base64 was treated as selector drift: %v", err)
	}
	fact := selected.InvalidAssets[0].Fact
	fact.ID = 1
	if fact.ContentDigest == ([sha256.Size]byte{}) {
		t.Fatal("invalid content lost its private evidence")
	}
	if _, err := mediaapp.DigestHistoricalInvalidAsset(fact); err != nil {
		t.Fatalf("owner rejected invalid source-only content: %v", err)
	}
	if _, err := base64.StdEncoding.Strict().DecodeString("not-valid-base64$"); err == nil {
		t.Fatal("fixture is unexpectedly decodable")
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
