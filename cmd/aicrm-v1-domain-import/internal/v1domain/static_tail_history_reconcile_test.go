package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"testing"

	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	cycleapp "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/app"
	cycleport "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestVerifyStaticTailHistoryRowChecksTypedTargetsSourceAndParents(t *testing.T) {
	media, product, cycle, rows, targets := staticTailReconcileFixture(t)
	for _, row := range rows {
		if _, err := verifyStaticTailHistoryRow(context.Background(), media, product, cycle, row, targets); err != nil {
			t.Fatalf("table=%s err=%v", row.TableID, err)
		}
	}
	brokenSource := rows[0]
	brokenSource.SourceKeyDigest = append([]byte(nil), brokenSource.SourceKeyDigest...)
	brokenSource.SourceKeyDigest[0]++
	if _, err := verifyStaticTailHistoryRow(context.Background(), media, product, cycle, brokenSource, targets); !errors.Is(err, ErrConflict) {
		t.Fatalf("source HMAC drift error=%v", err)
	}
	missingParent := rows[3]
	delete(targets, staticTailStrategyTarget)
	if _, err := verifyStaticTailHistoryRow(context.Background(), media, product, cycle, missingParent, targets); !errors.Is(err, ErrConflict) {
		t.Fatalf("missing historical parent error=%v", err)
	}
	if _, err := ReconcileStaticTailHistory(context.Background(), nil, "wrong", "archive-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("wrong version error=%v", err)
	}
}

func staticTailReconcileFixture(t *testing.T) (*staticTailReconcileMediaReader, *staticTailReconcileProductReader, *staticTailReconcileCycleReader, []reconciliationRow, map[string]map[string]struct{}) {
	t.Helper()
	archive := staticTailImporterFixture(t)
	journal := newStaticTailImporterJournal()
	mediaWriter := &staticTailImporterMediaWriter{base: staticTailImporterWriterBase{journal: journal}}
	productWriter := &staticTailImporterProductWriter{base: staticTailImporterWriterBase{journal: journal}}
	cycleWriter := &staticTailImporterCycleWriter{base: staticTailImporterWriterBase{journal: journal}}
	importer, err := NewStaticTailHistoryImporter(archive, staticTailImporterUOW{}, mediaWriter, productWriter, cycleWriter, journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "archive-run"); err != nil {
		t.Fatal(err)
	}
	group := mediaWriter.values[0]
	group.ID = 101
	page := productWriter.values[0]
	page.ID = 201
	strategy := cycleWriter.strategies[0]
	strategy.ID = 301
	version := cycleWriter.versions[1]
	version.ID, version.StrategyHistoryID = 302, strategy.ID
	document := cycleWriter.documents[0]
	document.ID, document.VersionHistoryID = 303, version.ID
	media := &staticTailReconcileMediaReader{value: group}
	product := &staticTailReconcileProductReader{value: page}
	cycle := &staticTailReconcileCycleReader{strategy: strategy, version: version, document: document}
	rows := []reconciliationRow{
		staticTailReconciliationRow(staticTailGroupInviteTable, "media", staticTailGroupInviteTarget, group.ID, group.SourceKeyDigest, group.SourcePayloadDigest, mediaDigest(t, group)),
		staticTailReconciliationRow(staticTailPageSliceTable, "product", staticTailPageSliceTarget, page.ID, page.SourceKeyDigest, page.SourcePayloadDigest, productDigest(t, page)),
		staticTailReconciliationRow(staticTailStrategyTable, "operationcycle", staticTailStrategyTarget, strategy.ID, strategy.SourceKeyDigest, strategy.SourcePayloadDigest, cycleStrategyDigest(t, strategy)),
		staticTailReconciliationRow(staticTailVersionTable, "operationcycle", staticTailVersionTarget, version.ID, version.SourceKeyDigest, version.SourcePayloadDigest, cycleVersionDigest(t, version)),
		staticTailReconciliationRow(staticTailDocumentTable, "operationcycle", staticTailDocumentTarget, document.ID, document.SourceKeyDigest, document.SourcePayloadDigest, cycleDocumentDigest(t, document)),
	}
	targets := map[string]map[string]struct{}{
		staticTailGroupInviteTarget: {strconv.FormatInt(group.ID, 10): {}},
		staticTailPageSliceTarget:   {strconv.FormatInt(page.ID, 10): {}},
		staticTailStrategyTarget:    {strconv.FormatInt(strategy.ID, 10): {}},
		staticTailVersionTarget:     {strconv.FormatInt(version.ID, 10): {}},
		staticTailDocumentTarget:    {strconv.FormatInt(document.ID, 10): {}},
	}
	return media, product, cycle, rows, targets
}

func staticTailReconciliationRow(table, domain, target string, id int64, source, payload [sha256.Size]byte, digest [sha256.Size]byte) reconciliationRow {
	domainCopy, targetCopy, idCopy := domain, target, strconv.FormatInt(id, 10)
	return reconciliationRow{TableID: table, SourceKeyDigest: source[:], PayloadDigest: payload[:], Disposition: "import", TargetDomain: &domainCopy, TargetTable: &targetCopy, TargetID: &idCopy, TargetDigest: digest[:], Metadata: []byte("{}"), Verified: true}
}

func mediaDigest(t *testing.T, value mediaport.HistoricalGroupInvite) [sha256.Size]byte {
	t.Helper()
	digest, err := mediaapp.HistoricalGroupInviteDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
func productDigest(t *testing.T, value productport.HistoricalProductPageSlice) [sha256.Size]byte {
	t.Helper()
	digest, err := productapp.HistoricalProductPageSliceDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
func cycleStrategyDigest(t *testing.T, value cycleport.HistoricalCycleStrategy) [sha256.Size]byte {
	t.Helper()
	digest, err := cycleapp.HistoricalCycleStrategyDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
func cycleVersionDigest(t *testing.T, value cycleport.HistoricalCycleVersion) [sha256.Size]byte {
	t.Helper()
	digest, err := cycleapp.HistoricalCycleVersionDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
func cycleDocumentDigest(t *testing.T, value cycleport.HistoricalCycleDocument) [sha256.Size]byte {
	t.Helper()
	digest, err := cycleapp.HistoricalCycleDocumentDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

type staticTailReconcileMediaReader struct {
	value mediaport.HistoricalGroupInvite
}

func (reader *staticTailReconcileMediaReader) GetHistoricalGroupInvite(_ context.Context, id int64) (mediaport.HistoricalGroupInvite, error) {
	if reader == nil || id != reader.value.ID {
		return mediaport.HistoricalGroupInvite{}, errors.New("missing")
	}
	return reader.value, nil
}
func (reader *staticTailReconcileMediaReader) ListHistoricalGroupInvite(context.Context, mediaport.StaticMediaHistoryQuery) ([]mediaport.HistoricalGroupInvite, int64, error) {
	return nil, 0, nil
}

type staticTailReconcileProductReader struct {
	value productport.HistoricalProductPageSlice
}

func (reader *staticTailReconcileProductReader) GetHistoricalProductPageSlice(_ context.Context, id int64) (productport.HistoricalProductPageSlice, error) {
	if reader == nil || id != reader.value.ID {
		return productport.HistoricalProductPageSlice{}, errors.New("missing")
	}
	return reader.value, nil
}
func (reader *staticTailReconcileProductReader) ListHistoricalProductPageSlice(context.Context, productport.StaticProductHistoryQuery) ([]productport.HistoricalProductPageSlice, int64, error) {
	return nil, 0, nil
}

type staticTailReconcileCycleReader struct {
	strategy cycleport.HistoricalCycleStrategy
	version  cycleport.HistoricalCycleVersion
	document cycleport.HistoricalCycleDocument
}

func (reader *staticTailReconcileCycleReader) GetHistoricalCycleStrategy(_ context.Context, id int64) (cycleport.HistoricalCycleStrategy, error) {
	if reader == nil || id != reader.strategy.ID {
		return cycleport.HistoricalCycleStrategy{}, errors.New("missing")
	}
	return reader.strategy, nil
}
func (reader *staticTailReconcileCycleReader) GetHistoricalCycleVersion(_ context.Context, id int64) (cycleport.HistoricalCycleVersion, error) {
	if reader == nil || id != reader.version.ID {
		return cycleport.HistoricalCycleVersion{}, errors.New("missing")
	}
	return reader.version, nil
}
func (reader *staticTailReconcileCycleReader) GetHistoricalCycleDocument(_ context.Context, id int64) (cycleport.HistoricalCycleDocument, error) {
	if reader == nil || id != reader.document.ID {
		return cycleport.HistoricalCycleDocument{}, errors.New("missing")
	}
	return reader.document, nil
}
func (reader *staticTailReconcileCycleReader) ListHistoricalCycleStrategy(context.Context, cycleport.StaticCycleHistoryQuery) ([]cycleport.HistoricalCycleStrategy, int64, error) {
	return nil, 0, nil
}
func (reader *staticTailReconcileCycleReader) ListHistoricalCycleVersion(context.Context, cycleport.StaticCycleHistoryQuery) ([]cycleport.HistoricalCycleVersion, int64, error) {
	return nil, 0, nil
}
func (reader *staticTailReconcileCycleReader) ListHistoricalCycleDocument(context.Context, cycleport.StaticCycleHistoryQuery) ([]cycleport.HistoricalCycleDocument, int64, error) {
	return nil, 0, nil
}
