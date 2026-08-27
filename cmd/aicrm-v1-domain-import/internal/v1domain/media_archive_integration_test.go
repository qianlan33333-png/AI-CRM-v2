package v1domain

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
	mediadomain "github.com/qianlan33333-png/AI-CRM-v2/internal/media/domain"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var mediaArchiveRun = flag.String("media-archive-run", "", "optional reconciled V2 archive run for read-only media diagnostics")
var mediaArchiveRollback = flag.Bool("media-archive-rollback", false, "exercise encoded-length image SQL imports in one forced rollback transaction")

var errMediaArchiveRollback = errors.New("media archive SQL rehearsal forced rollback")

type mediaArchiveImageCounts struct {
	total, valid, redacted, base64Size, filenameMIMEDecode, metadataTagsTimes                       int
	encodedLengthFileSize, encodedLengthDecoded, encodedLengthImageValid, encodedLengthOwnerAdapter int
}

// TestReconciledMediaArchiveDiagnosesImagesWithoutWrites classifies the V1
// image-library rows using the exact adapter and image inspector. It is opt-in
// because it reads the encrypted reconciled archive, but never opens a target
// write transaction or prints source data.
func TestReconciledMediaArchiveDiagnosesImagesWithoutWrites(t *testing.T) {
	if *mediaArchiveRun == "" {
		t.Skip("supply -media-archive-run and V2 archive environment for read-only media diagnostics")
	}
	ctx := context.Background()
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("cannot open V2 archive reader")
	}
	defer archive.Close()

	counts := mediaArchiveImageCounts{}
	err = archive.EachTableRow(ctx, *mediaArchiveRun, "public/image_library", func(row v1archive.ArchivedRow) error {
		counts.add(row)
		return nil
	})
	if err != nil {
		t.Fatal("cannot read image archive")
	}
	if counts.total == 0 {
		t.Fatal("archive contains no image-library rows")
	}
	t.Logf("read-only image archive diagnostic: total=%d valid=%d redacted=%d base64_size=%d filename_mime_decode=%d metadata_tags_times=%d encoded_length_file_size=%d encoded_length_decoded=%d encoded_length_image_inspected=%d encoded_length_owner_adapter=%d",
		counts.total, counts.valid, counts.redacted, counts.base64Size, counts.filenameMIMEDecode, counts.metadataTagsTimes,
		counts.encodedLengthFileSize, counts.encodedLengthDecoded, counts.encodedLengthImageValid, counts.encodedLengthOwnerAdapter)
}

// TestReconciledMediaArchiveImagesRollback executes the real Media store and
// migration journal only when both explicit flags are supplied. The sentinel
// always rolls its one UnitOfWork back, so this rehearsal cannot persist image
// rows or receipts and never reuses the immutable static-import receipts.
func TestReconciledMediaArchiveImagesRollback(t *testing.T) {
	if *mediaArchiveRun == "" || !*mediaArchiveRollback {
		t.Skip("supply -media-archive-run and -media-archive-rollback for the forced-rollback Media SQL rehearsal")
	}
	ctx := context.Background()
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("cannot open V2 archive reader")
	}
	defer archive.Close()

	definitions := make([]media.HistoricalImageDefinition, 0)
	err = archive.EachTableRow(ctx, *mediaArchiveRun, "public/image_library", func(row v1archive.ArchivedRow) error {
		if definition, valid := encodedLengthMediaArchiveImage(row); valid {
			definitions = append(definitions, definition)
		}
		return nil
	})
	if err != nil {
		t.Fatal("cannot read image archive")
	}
	if len(definitions) == 0 {
		t.Fatal("archive has no encoded-length images accepted by the Media adapter")
	}

	pool, err := pgxpool.New(ctx, environment.TargetDatabaseURL)
	if err != nil {
		t.Fatal("cannot open V2 rehearsal database")
	}
	defer pool.Close()
	journal, err := NewJournal(Scope{ImportVersion: "v1-static-media-probe", ArchiveRunID: *mediaArchiveRun,
		AdapterID: v1archive.DefaultAdapterID, TableID: "public/image_library", TargetDomain: "media", TargetTable: "media_images"})
	if err != nil {
		t.Fatal("cannot create Media rehearsal journal")
	}
	writer, err := media.NewHistoricalStaticWriter(mediastore.NewHistoricalStaticStore(), journal)
	if err != nil {
		t.Fatal("cannot create Media rehearsal writer")
	}
	imported := 0
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		for _, definition := range definitions {
			receipt, writeErr := writer.ImportImage(tx, definition)
			if writeErr != nil || receipt.Replayed {
				return errors.New("media archive rehearsal writer failed")
			}
			imported++
		}
		return errMediaArchiveRollback
	})
	if !errors.Is(err, errMediaArchiveRollback) {
		t.Fatal("Media rehearsal did not force rollback")
	}
	if imported != len(definitions) {
		t.Fatal("Media rehearsal did not write every accepted image before rollback")
	}
	t.Logf("forced rollback Media SQL rehearsal: encoded_length_images=%d", imported)
}

func (counts *mediaArchiveImageCounts) add(row v1archive.ArchivedRow) {
	counts.total++
	if redactedMediaStaticDefinition(row.RedactedFields) {
		counts.redacted++
		return
	}
	var source mediaStaticJSON
	if json.Unmarshal(row.Payload, &source) != nil {
		counts.filenameMIMEDecode++
		return
	}
	encodedLength := source.FileSize == int64(len(source.DataBase64))
	if encodedLength {
		counts.encodedLengthFileSize++
	}
	content, decoded := canonicalMediaArchiveBase64(source.DataBase64)
	if decoded && encodedLength {
		counts.encodedLengthDecoded++
	}
	inspected := false
	if decoded {
		_, inspectErr := mediadomain.Inspect(source.FileName, source.MimeType, content)
		inspected = inspectErr == nil
		if inspected && encodedLength {
			counts.encodedLengthImageValid++
		}
	}

	adapterErr := adaptMediaArchiveImage(row, source, source.FileSize)
	if encodedLength && inspected && adaptMediaArchiveImage(row, source, int64(len(content))) == nil {
		counts.encodedLengthOwnerAdapter++
	}
	if !strictMediaArchiveSize(source.FileSize, source.DataBase64, content, decoded) {
		counts.base64Size++
		return
	}
	if !inspected {
		counts.filenameMIMEDecode++
		return
	}
	if adapterErr != nil {
		counts.metadataTagsTimes++
		return
	}
	counts.valid++
}

func adaptMediaArchiveImage(row v1archive.ArchivedRow, source mediaStaticJSON, fileSize int64) error {
	_, err := mediaArchiveImageDefinition(row, source, fileSize)
	return err
}

func mediaArchiveImageDefinition(row v1archive.ArchivedRow, source mediaStaticJSON, fileSize int64) (media.HistoricalImageDefinition, error) {
	return media.AdaptV1ImageLibrary(media.V1ImageLibraryRow{
		ID: source.ID, Name: source.Name, FileName: source.FileName, MimeType: source.MimeType, FileSize: fileSize,
		DataBase64: source.DataBase64, Description: source.Description, Tags: source.Tags, Category: source.Category,
		SourceURL: source.SourceURL, ThumbMediaID: source.ThumbMediaID, ThumbMediaExpiresAt: source.ThumbMediaExpiresAt,
		Enabled: source.Enabled, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
	}, media.HistoricalStaticOrigin{SourceIdentifier: SourceIdentifier(row.SourceKeyHMAC), SourceID: source.ID, PayloadDigest: row.PayloadHMAC}, 1)
}

func encodedLengthMediaArchiveImage(row v1archive.ArchivedRow) (media.HistoricalImageDefinition, bool) {
	if redactedMediaStaticDefinition(row.RedactedFields) {
		return media.HistoricalImageDefinition{}, false
	}
	var source mediaStaticJSON
	if json.Unmarshal(row.Payload, &source) != nil || source.FileSize != int64(len(source.DataBase64)) {
		return media.HistoricalImageDefinition{}, false
	}
	definition, err := mediaArchiveImageDefinition(row, source, source.FileSize)
	return definition, err == nil
}

func canonicalMediaArchiveBase64(encoded string) ([]byte, bool) {
	content, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(content) == 0 || base64.StdEncoding.EncodeToString(content) != encoded {
		return nil, false
	}
	return content, true
}

func strictMediaArchiveSize(size int64, encoded string, content []byte, decoded bool) bool {
	return decoded && size > 0 && size <= int64(mediadomain.MaxImageBytes) && len(encoded) == base64.StdEncoding.EncodedLen(int(size)) && int64(len(content)) == size
}
