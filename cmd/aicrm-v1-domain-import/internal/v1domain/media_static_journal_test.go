package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"

	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
)

func mediaStaticReceiptFixture() media.HistoricalStaticReceipt {
	return media.HistoricalStaticReceipt{Origin: media.HistoricalStaticOrigin{SourceIdentifier: SourceIdentifier(sha256.Sum256([]byte("source-key"))),
		SourceID: 9007199254740993, PayloadDigest: sha256.Sum256([]byte("payload"))}, Kind: media.HistoricalImage, TargetID: 901,
		Checksum: sha256.Sum256([]byte("bytes")), DefinitionDigest: sha256.Sum256([]byte("definition")), ProviderMaterialDropped: false}
}

func TestMediaStaticJournalJSONBRoundTripAndSealedReplay(t *testing.T) {
	row, _ := mediaStaticFixture(t, media.HistoricalImage)
	importer, db := mediaStaticImporterFixture(t, media.HistoricalImage, row)
	want := mediaStaticReceiptFixture()
	err := db.Within(context.Background(), func(ctx context.Context) error {
		if err := importer.journal.RecordHistoricalStatic(ctx, want); err != nil {
			return err
		}
		var metadata map[string]any
		if err := json.Unmarshal(db.stored[12].([]byte), &metadata); err != nil {
			return err
		}
		if metadata["source_id"] != "9007199254740993" {
			t.Fatal("source ID lost integer precision")
		}
		// JSONB formatting/key ordering is not a different receipt.
		encoded, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return err
		}
		db.stored[12] = encoded
		got, found, err := importer.journal.LoadHistoricalStatic(ctx, want.Origin.SourceIdentifier)
		if err != nil || !found || got != want {
			t.Fatalf("roundtrip=%+v found=%v err=%v", got, found, err)
		}
		return importer.journal.RecordHistoricalStatic(ctx, want)
	})
	if err != nil || db.recordCalls != 1 {
		t.Fatalf("replay inserted receipt: calls=%d err=%v", db.recordCalls, err)
	}
}

func TestMediaStaticJournalRejectsCorruptMetadataOrTarget(t *testing.T) {
	for _, failure := range []string{"source-number", "source-leading-zero", "missing-dropped", "null-dropped", "unknown-field", "wrong-kind", "zero-checksum", "zero-definition", "target-digest", "target-leading-zero", "zero-payload", "quarantine"} {
		t.Run(failure, func(t *testing.T) {
			row, _ := mediaStaticFixture(t, media.HistoricalImage)
			importer, db := mediaStaticImporterFixture(t, media.HistoricalImage, row)
			want := mediaStaticReceiptFixture()
			err := db.Within(context.Background(), func(ctx context.Context) error {
				if err := importer.journal.RecordHistoricalStatic(ctx, want); err != nil {
					return err
				}
				var metadata map[string]any
				if err := json.Unmarshal(db.stored[12].([]byte), &metadata); err != nil {
					return err
				}
				switch failure {
				case "source-number":
					metadata["source_id"] = json.Number("9007199254740993")
				case "source-leading-zero":
					metadata["source_id"] = "09007199254740993"
				case "missing-dropped":
					delete(metadata, "provider_material_dropped")
				case "null-dropped":
					metadata["provider_material_dropped"] = nil
				case "unknown-field":
					metadata["provider_verified"] = true
				case "wrong-kind":
					metadata["kind"] = string(media.HistoricalAttachment)
				case "zero-checksum":
					metadata["checksum"] = SourceIdentifier([32]byte{})
				case "zero-definition":
					metadata["definition_digest"] = SourceIdentifier([32]byte{})
				case "target-digest":
					digest := sha256.Sum256([]byte("wrong target"))
					db.stored[11] = digest[:]
				case "target-leading-zero":
					db.stored[10] = "0901"
					digest := mediaStaticTargetDigest("media_images", "0901", want.Origin.PayloadDigest)
					db.stored[11] = digest[:]
				case "zero-payload":
					payload := [32]byte{}
					db.stored[5] = payload[:]
					digest := mediaStaticTargetDigest("media_images", "901", payload)
					db.stored[11] = digest[:]
				case "quarantine":
					db.stored[6], db.stored[7] = "quarantine", "invalid_static_media_definition"
					db.stored[8], db.stored[9], db.stored[10], db.stored[11] = nil, nil, nil, nil
				}
				encoded, err := json.Marshal(metadata)
				if err != nil {
					return err
				}
				db.stored[12] = encoded
				_, found, err := importer.journal.LoadHistoricalStatic(ctx, want.Origin.SourceIdentifier)
				if found || !errors.Is(err, ErrConflict) {
					t.Fatalf("corrupt receipt accepted: found=%v err=%v", found, err)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMediaStaticJournalRejectsInvalidRecordsAndScopeBeforeIO(t *testing.T) {
	for _, failure := range []string{"zero-source-key", "invalid-source-key", "zero-payload", "zero-source-id", "zero-target-id", "zero-checksum", "zero-definition", "replayed", "wrong-kind", "adapter", "source-table", "target-domain", "target-table", "nil-context"} {
		t.Run(failure, func(t *testing.T) {
			row, _ := mediaStaticFixture(t, media.HistoricalImage)
			importer, db := mediaStaticImporterFixture(t, media.HistoricalImage, row)
			receipt := mediaStaticReceiptFixture()
			ctx := context.Background()
			switch failure {
			case "zero-source-key":
				receipt.Origin.SourceIdentifier = SourceIdentifier([32]byte{})
			case "invalid-source-key":
				receipt.Origin.SourceIdentifier = "not-a-digest"
			case "zero-payload":
				receipt.Origin.PayloadDigest = [32]byte{}
			case "zero-source-id":
				receipt.Origin.SourceID = 0
			case "zero-target-id":
				receipt.TargetID = 0
			case "zero-checksum":
				receipt.Checksum = [32]byte{}
			case "zero-definition":
				receipt.DefinitionDigest = [32]byte{}
			case "replayed":
				receipt.Replayed = true
			case "wrong-kind":
				receipt.Kind = media.HistoricalAttachment
			case "adapter":
				importer.journal.scope.AdapterID = "untrusted"
			case "source-table":
				importer.journal.scope.TableID = "public/image_library_variants"
			case "target-domain":
				importer.journal.scope.TargetDomain = "campaign"
			case "target-table":
				importer.journal.scope.TargetTable = "media_attachments"
			case "nil-context":
				ctx = nil
			}
			if err := importer.journal.RecordHistoricalStatic(ctx, receipt); !errors.Is(err, ErrInvalidScope) {
				t.Fatalf("unsafe receipt accepted: %v", err)
			}
			if db.recordCalls != 0 || len(db.lockKeys) != 0 {
				t.Fatal("invalid receipt accessed storage")
			}
		})
	}
	row, _ := mediaStaticFixture(t, media.HistoricalImage)
	importer, _ := mediaStaticImporterFixture(t, media.HistoricalImage, row)
	for _, key := range []string{"invalid", SourceIdentifier([32]byte{})} {
		if _, _, err := importer.journal.LoadHistoricalStatic(context.Background(), key); !errors.Is(err, ErrInvalidScope) {
			t.Fatalf("unsafe source identity: %v", err)
		}
	}
	var missing *Journal
	if _, _, err := missing.LoadHistoricalStatic(context.Background(), "source"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal(err)
	}
}
