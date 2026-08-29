package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"reflect"

	timeline "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1customertimelinehistory"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

const (
	customerTimelineHistoryVersion = "v1-customer-timeline-history-a1"
	customerTimelineHistoryKind    = "customer_timeline_event"
)

// CustomerTimelineArchiveReady is deliberately separate from the row stream:
// reconciliation readiness must be proven before the first target batch opens.
type CustomerTimelineArchiveReady interface {
	VerifyCustomerTimelineArchiveReady(context.Context, string) error
}

// CustomerTimelineResolver may return only one verified unionid customer. A
// nil result remains an unresolved historical relation; no fallback is allowed.
type CustomerTimelineResolver interface {
	ResolveVerifiedCustomerTimelineUnionID(context.Context, string) (*int64, error)
}

type CustomerTimelineWriter interface {
	ImportHistoricalCustomerTimelineEvent(context.Context, string, contact.HistoricalCustomerTimelineEvent) (contact.CustomerTimelineHistoryReceipt, error)
}

type CustomerTimelineTargetReader interface {
	GetHistoricalCustomerTimelineEvent(context.Context, int64) (contact.HistoricalCustomerTimelineEvent, error)
}

// CustomerTimelineTerminal is the narrow v1_domain_import_receipts boundary.
// The eventual SQL implementation owns generic receipt storage; this package
// never reaches across to current customer events.
type CustomerTimelineTerminal struct {
	Version, ArchiveRunID, TableID, Kind  string
	SourceKeyHMAC, PayloadHMAC, FieldHMAC [sha256.Size]byte
	Disposition, Reason                   string
	TargetID                              int64
	TargetDigest                          [sha256.Size]byte
}

type CustomerTimelineImportJournal interface {
	LoadCustomerTimelineTerminal(context.Context, string, [sha256.Size]byte) (CustomerTimelineTerminal, bool, error)
	RecordCustomerTimelineTerminal(context.Context, CustomerTimelineTerminal) error
}

// CustomerTimelineReconciliationSeal maps to the one formal reconciliation
// row for an import-version/archive-run pair. Per-source evidence remains in
// immutable terminal receipts and is verified before this seal is made.
type CustomerTimelineReconciliationSeal struct {
	Version, ArchiveRunID                                                                            string
	SelectedSourceCount, ReceiptCount, ImportedCount, ArchivedCount, QuarantinedCount, VerifiedCount int64
	ComparisonDigest                                                                                 [sha256.Size]byte
}

type CustomerTimelineReconciliationJournal interface {
	LoadCustomerTimelineReconciliationSeal(context.Context, string, string) (CustomerTimelineReconciliationSeal, bool, error)
	RecordCustomerTimelineReconciliationSeal(context.Context, CustomerTimelineReconciliationSeal) error
}

type CustomerTimelineHistoryImporter struct {
	ready          CustomerTimelineArchiveReady
	archive        timeline.ArchiveSource
	uow            UnitOfWork
	writer         CustomerTimelineWriter
	resolver       CustomerTimelineResolver
	targets        CustomerTimelineTargetReader
	journal        CustomerTimelineImportJournal
	reconciliation CustomerTimelineReconciliationJournal
}

type CustomerTimelineHistoryImportResult struct {
	Imported, Quarantined, Replayed int
}

type CustomerTimelineHistoryReconciliationResult struct {
	SelectedSourceCount, ReceiptCount, ImportedCount, ArchivedCount, QuarantinedCount, VerifiedCount int64
	ComparisonDigest                                                                                 [sha256.Size]byte
	Replayed                                                                                         bool
}

func NewCustomerTimelineHistoryImporter(ready CustomerTimelineArchiveReady, archive timeline.ArchiveSource, uow UnitOfWork, writer CustomerTimelineWriter, resolver CustomerTimelineResolver, targets CustomerTimelineTargetReader, journal CustomerTimelineImportJournal, reconciliation CustomerTimelineReconciliationJournal) (*CustomerTimelineHistoryImporter, error) {
	if nilCustomerTimelineDependency(ready) || nilCustomerTimelineDependency(archive) || nilCustomerTimelineDependency(uow) || nilCustomerTimelineDependency(writer) || nilCustomerTimelineDependency(resolver) || nilCustomerTimelineDependency(targets) || nilCustomerTimelineDependency(journal) || nilCustomerTimelineDependency(reconciliation) {
		return nil, ErrInvalidScope
	}
	return &CustomerTimelineHistoryImporter{ready: ready, archive: archive, uow: uow, writer: writer, resolver: resolver, targets: targets, journal: journal, reconciliation: reconciliation}, nil
}

// Import verifies archive readiness before streaming. Each source batch is one
// UoW transaction: target write and generic terminal receipt both commit or
// roll back together.
func (importer *CustomerTimelineHistoryImporter) Import(ctx context.Context, archiveRunID string, sourceHMACKey []byte) (CustomerTimelineHistoryImportResult, error) {
	if !importer.valid(ctx, archiveRunID, sourceHMACKey) {
		return CustomerTimelineHistoryImportResult{}, ErrInvalidScope
	}
	if err := importer.uow.Within(ctx, func(bound context.Context) error {
		return importer.ready.VerifyCustomerTimelineArchiveReady(bound, archiveRunID)
	}); err != nil {
		return CustomerTimelineHistoryImportResult{}, err
	}
	consumer := &customerTimelineImportConsumer{importer: importer, archiveRunID: archiveRunID}
	if _, err := timeline.Stream(ctx, importer.archive, archiveRunID, sourceHMACKey, customerTimelineNoopVerifier{}, consumer); err != nil {
		return CustomerTimelineHistoryImportResult{}, err
	}
	return consumer.result, nil
}

// Reconcile streams the same sealed source and verifies each target through its
// typed digest, then creates or replays a matching reconciliation receipt.
func (importer *CustomerTimelineHistoryImporter) Reconcile(ctx context.Context, archiveRunID string, sourceHMACKey []byte) (CustomerTimelineHistoryReconciliationResult, error) {
	if !importer.valid(ctx, archiveRunID, sourceHMACKey) {
		return CustomerTimelineHistoryReconciliationResult{}, ErrInvalidScope
	}
	if err := importer.uow.Within(ctx, func(bound context.Context) error {
		return importer.ready.VerifyCustomerTimelineArchiveReady(bound, archiveRunID)
	}); err != nil {
		return CustomerTimelineHistoryReconciliationResult{}, err
	}
	consumer := &customerTimelineReconcileConsumer{importer: importer, archiveRunID: archiveRunID, hash: sha256.New()}
	if _, err := timeline.Stream(ctx, importer.archive, archiveRunID, sourceHMACKey, customerTimelineNoopVerifier{}, consumer); err != nil {
		return CustomerTimelineHistoryReconciliationResult{}, err
	}
	return consumer.seal(ctx)
}

func (importer *CustomerTimelineHistoryImporter) valid(ctx context.Context, run string, key []byte) bool {
	return importer != nil && ctx != nil && ctx.Err() == nil && run != "" && len(key) >= sha256.Size &&
		!nilCustomerTimelineDependency(importer.ready) && !nilCustomerTimelineDependency(importer.archive) && !nilCustomerTimelineDependency(importer.uow) && !nilCustomerTimelineDependency(importer.writer) && !nilCustomerTimelineDependency(importer.resolver) && !nilCustomerTimelineDependency(importer.targets) && !nilCustomerTimelineDependency(importer.journal) && !nilCustomerTimelineDependency(importer.reconciliation)
}

type customerTimelineImportConsumer struct {
	importer     *CustomerTimelineHistoryImporter
	archiveRunID string
	result       CustomerTimelineHistoryImportResult
}

func (consumer *customerTimelineImportConsumer) ConsumeCustomerTimelineBatch(ctx context.Context, batch timeline.Batch) error {
	if consumer == nil || consumer.importer == nil || len(batch.Rows) == 0 || len(batch.Rows) > timeline.FixedBatchSize {
		return ErrInvalidScope
	}
	var imported, quarantined, replayed int
	err := consumer.importer.uow.Within(ctx, func(tx context.Context) error {
		imported, quarantined, replayed = 0, 0, 0
		for _, row := range batch.Rows {
			if row.Disposition == timeline.DispositionQuarantine {
				wasReplayed, err := consumer.importer.recordTerminal(tx, consumer.archiveRunID, row, CustomerTimelineTerminal{Disposition: timeline.DispositionQuarantine, Reason: row.Reason})
				if err != nil {
					return err
				}
				if wasReplayed {
					replayed++
				} else {
					quarantined++
				}
				continue
			}
			if row.Disposition != timeline.DispositionCandidate || row.Fact == nil {
				return ErrConflict
			}
			value, err := consumer.importer.targetValue(tx, *row.Fact)
			if err != nil {
				return err
			}
			receipt, err := consumer.importer.writer.ImportHistoricalCustomerTimelineEvent(tx, hex.EncodeToString(value.SourceKeyDigest[:]), value)
			if err != nil {
				return err
			}
			if err := validateCustomerTimelineWriterReceipt(receipt, value); err != nil {
				return err
			}
			terminal, found, err := consumer.importer.journal.LoadCustomerTimelineTerminal(tx, customerTimelineHistoryVersion, row.Source.SourceKeyHMAC)
			if err != nil {
				return err
			}
			if !found || !customerTimelineTerminalMatchesRow(terminal, consumer.archiveRunID, row) || terminal.TargetID != receipt.TargetID || terminal.TargetDigest != receipt.TargetDigest {
				return ErrConflict
			}
			if receipt.Replayed {
				replayed++
			} else {
				imported++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	consumer.result.Imported += imported
	consumer.result.Quarantined += quarantined
	consumer.result.Replayed += replayed
	return nil
}

func (importer *CustomerTimelineHistoryImporter) targetValue(ctx context.Context, fact timeline.TimelineEventFact) (contact.HistoricalCustomerTimelineEvent, error) {
	var customerID *int64
	if fact.UnionID != "" {
		resolved, err := importer.resolver.ResolveVerifiedCustomerTimelineUnionID(ctx, fact.UnionID)
		if err != nil {
			return contact.HistoricalCustomerTimelineEvent{}, err
		}
		if resolved != nil {
			if *resolved < 1 {
				return contact.HistoricalCustomerTimelineEvent{}, contact.ErrCustomerTimelineHistoryInvalid
			}
			copied := *resolved
			customerID = &copied
		}
	}
	return contact.HistoricalCustomerTimelineEvent{
		SourceKeyDigest: fact.Source.SourceKeyHMAC, SourcePayloadDigest: fact.Source.PayloadHMAC, SourceFieldDigest: fact.Source.FieldHMAC,
		SourceID: fact.SourceID, EventID: fact.EventID, EventType: fact.EventType, EventTime: fact.EventTime,
		Title: fact.Title, Summary: fact.Summary, SourceTable: fact.SourceTable, SourceValue: fact.SourceValue,
		MetadataJSON: append([]byte(nil), fact.MetadataJSON...), CreatedAt: fact.CreatedAt, UnionID: fact.UnionID, CustomerID: customerID,
	}, nil
}

func (importer *CustomerTimelineHistoryImporter) recordTerminal(ctx context.Context, archiveRunID string, row timeline.Result, value CustomerTimelineTerminal) (bool, error) {
	value.Version, value.ArchiveRunID, value.TableID, value.Kind = customerTimelineHistoryVersion, archiveRunID, timeline.TableID, customerTimelineHistoryKind
	value.SourceKeyHMAC, value.PayloadHMAC, value.FieldHMAC = row.Source.SourceKeyHMAC, row.Source.PayloadHMAC, row.Source.FieldHMAC
	if !validCustomerTimelineTerminal(value) {
		return false, ErrConflict
	}
	existing, found, err := importer.journal.LoadCustomerTimelineTerminal(ctx, customerTimelineHistoryVersion, value.SourceKeyHMAC)
	if err != nil {
		return false, err
	}
	if found {
		return true, boolCustomerTimelineTerminalEqual(existing, value)
	}
	if err := importer.journal.RecordCustomerTimelineTerminal(ctx, value); err != nil {
		return false, err
	}
	return false, nil
}

type customerTimelineReconcileConsumer struct {
	importer     *CustomerTimelineHistoryImporter
	archiveRunID string
	result       CustomerTimelineHistoryReconciliationResult
	hash         hash.Hash
}

func (consumer *customerTimelineReconcileConsumer) ConsumeCustomerTimelineBatch(ctx context.Context, batch timeline.Batch) error {
	if consumer == nil || consumer.importer == nil || len(batch.Rows) == 0 || len(batch.Rows) > timeline.FixedBatchSize {
		return ErrInvalidScope
	}
	var selected, imported, quarantined, verified int64
	var comparison bytes.Buffer
	err := consumer.importer.uow.Within(ctx, func(tx context.Context) error {
		selected, imported, quarantined, verified = 0, 0, 0, 0
		comparison.Reset()
		for _, row := range batch.Rows {
			terminal, found, err := consumer.importer.journal.LoadCustomerTimelineTerminal(tx, customerTimelineHistoryVersion, row.Source.SourceKeyHMAC)
			if err != nil || !found || !customerTimelineTerminalMatchesRow(terminal, consumer.archiveRunID, row) {
				return ErrConflict
			}
			if row.Disposition == timeline.DispositionCandidate {
				if row.Fact == nil || terminal.TargetID < 1 || terminal.TargetDigest == ([sha256.Size]byte{}) {
					return ErrConflict
				}
				actual, getErr := consumer.importer.targets.GetHistoricalCustomerTimelineEvent(tx, terminal.TargetID)
				if getErr != nil {
					return getErr
				}
				expected, valueErr := consumer.importer.targetValue(tx, *row.Fact)
				if valueErr != nil {
					return valueErr
				}
				expected.ID = terminal.TargetID
				actualDigest, actualErr := contactapp.HistoricalCustomerTimelineEventDigest(actual)
				expectedDigest, expectedErr := contactapp.HistoricalCustomerTimelineEventDigest(expected)
				if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != terminal.TargetDigest {
					return ErrConflict
				}
				imported++
			} else if row.Disposition != timeline.DispositionQuarantine || terminal.TargetID != 0 || terminal.TargetDigest != ([sha256.Size]byte{}) {
				return ErrConflict
			} else {
				quarantined++
			}
			if err := json.NewEncoder(&comparison).Encode([]any{
				timeline.TableID, row.Source.SourceOrdinal, hex.EncodeToString(row.Source.SourceKeyHMAC[:]),
				hex.EncodeToString(row.Source.PayloadHMAC[:]), hex.EncodeToString(row.Source.FieldHMAC[:]),
				row.Disposition, row.Reason, terminal.TargetID, hex.EncodeToString(terminal.TargetDigest[:]),
			}); err != nil {
				return err
			}
			selected++
			verified++
		}
		return nil
	})
	if err != nil {
		return err
	}
	if _, err := consumer.hash.Write(comparison.Bytes()); err != nil {
		return err
	}
	consumer.result.SelectedSourceCount += selected
	consumer.result.ReceiptCount += selected
	consumer.result.ImportedCount += imported
	consumer.result.QuarantinedCount += quarantined
	consumer.result.VerifiedCount += verified
	return nil
}

func (consumer *customerTimelineReconcileConsumer) seal(ctx context.Context) (CustomerTimelineHistoryReconciliationResult, error) {
	if consumer == nil || consumer.importer == nil || consumer.hash == nil {
		return CustomerTimelineHistoryReconciliationResult{}, ErrConflict
	}
	seal := customerTimelineReconciliationSeal(consumer)
	if !validCustomerTimelineReconciliationSeal(seal) {
		return CustomerTimelineHistoryReconciliationResult{}, ErrConflict
	}
	if err := consumer.importer.uow.Within(ctx, func(tx context.Context) error {
		existing, found, err := consumer.importer.reconciliation.LoadCustomerTimelineReconciliationSeal(tx, seal.Version, seal.ArchiveRunID)
		if err != nil {
			return err
		}
		if found {
			if existing != seal {
				return ErrConflict
			}
			consumer.result.Replayed = true
			return nil
		}
		return consumer.importer.reconciliation.RecordCustomerTimelineReconciliationSeal(tx, seal)
	}); err != nil {
		return CustomerTimelineHistoryReconciliationResult{}, err
	}
	consumer.result.ComparisonDigest = seal.ComparisonDigest
	return consumer.result, nil
}

func customerTimelineReconciliationSeal(consumer *customerTimelineReconcileConsumer) CustomerTimelineReconciliationSeal {
	var digest [sha256.Size]byte
	copy(digest[:], consumer.hash.Sum(nil))
	return CustomerTimelineReconciliationSeal{
		Version: customerTimelineHistoryVersion, ArchiveRunID: consumer.archiveRunID,
		SelectedSourceCount: consumer.result.SelectedSourceCount, ReceiptCount: consumer.result.ReceiptCount,
		ImportedCount: consumer.result.ImportedCount, ArchivedCount: consumer.result.ArchivedCount,
		QuarantinedCount: consumer.result.QuarantinedCount, VerifiedCount: consumer.result.VerifiedCount,
		ComparisonDigest: digest,
	}
}

func validCustomerTimelineReconciliationSeal(value CustomerTimelineReconciliationSeal) bool {
	return value.Version == customerTimelineHistoryVersion && value.ArchiveRunID != "" &&
		value.SelectedSourceCount > 0 && value.ReceiptCount == value.SelectedSourceCount &&
		value.VerifiedCount == value.ReceiptCount &&
		value.ImportedCount+value.ArchivedCount+value.QuarantinedCount == value.ReceiptCount &&
		value.ComparisonDigest != ([sha256.Size]byte{})
}

func validateCustomerTimelineWriterReceipt(receipt contact.CustomerTimelineHistoryReceipt, value contact.HistoricalCustomerTimelineEvent) error {
	if receipt.Kind != customerTimelineHistoryKind || receipt.SourceIdentifier != hex.EncodeToString(value.SourceKeyDigest[:]) || receipt.PayloadDigest != value.SourcePayloadDigest || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	return nil
}

func validCustomerTimelineTerminal(value CustomerTimelineTerminal) bool {
	if value.Version != customerTimelineHistoryVersion || value.ArchiveRunID == "" || value.TableID != timeline.TableID || value.Kind != customerTimelineHistoryKind || value.SourceKeyHMAC == ([sha256.Size]byte{}) || value.PayloadHMAC == ([sha256.Size]byte{}) || value.FieldHMAC == ([sha256.Size]byte{}) {
		return false
	}
	if value.Disposition == timeline.DispositionCandidate {
		return value.Reason == "" && value.TargetID > 0 && value.TargetDigest != ([sha256.Size]byte{})
	}
	return value.Disposition == timeline.DispositionQuarantine && value.Reason != "" && value.TargetID == 0 && value.TargetDigest == ([sha256.Size]byte{})
}

func customerTimelineTerminalMatchesRow(value CustomerTimelineTerminal, archiveRunID string, row timeline.Result) bool {
	return validCustomerTimelineTerminal(value) && value.ArchiveRunID == archiveRunID && value.SourceKeyHMAC == row.Source.SourceKeyHMAC && value.PayloadHMAC == row.Source.PayloadHMAC && value.FieldHMAC == row.Source.FieldHMAC && value.Disposition == row.Disposition && value.Reason == row.Reason
}

func boolCustomerTimelineTerminalEqual(left, right CustomerTimelineTerminal) error {
	if left == right {
		return nil
	}
	return ErrConflict
}

type customerTimelineNoopVerifier struct{}

func (customerTimelineNoopVerifier) VerifyCustomerTimelineTerminal(context.Context, timeline.SourceEnvelope, string, string) error {
	return nil
}

func nilCustomerTimelineDependency(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	return (ref.Kind() == reflect.Ptr || ref.Kind() == reflect.Interface) && ref.IsNil()
}
