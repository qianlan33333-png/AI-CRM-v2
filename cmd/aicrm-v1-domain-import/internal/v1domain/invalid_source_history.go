package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	invalidhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1invalidsourcehistory"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	radarstore "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store"
)

const InvalidSourceHistoryVersion = "v1-invalid-source-history-a1"

type InvalidSourceHistoryResult struct {
	Selected, Imported, Replayed int
	Reconciliation               *ReconciliationResult `json:",omitempty"`
}

// This tail package is exactly the 16 sealed invalid definitions. It cannot
// create a current object, fix a source, or update any earlier import receipt.
func RunInvalidSourceHistory(ctx context.Context, pool *pgxpool.Pool, archive invalidhistory.ArchiveSource, run string, key []byte, reconcile bool) (InvalidSourceHistoryResult, error) {
	if ctx == nil || pool == nil || archive == nil || run == "" || len(key) < sha256.Size {
		return InvalidSourceHistoryResult{}, ErrInvalidScope
	}
	var result InvalidSourceHistoryResult
	uow := platformstore.NewUnitOfWork(externalIdentityGapSerializableBeginner{pool: pool})
	err := uow.Within(ctx, func(bound context.Context) error {
		result = InvalidSourceHistoryResult{}
		tx, err := platformstore.TxFromContext(bound)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(bound, "LOCK TABLE public.v1_domain_import_receipts IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return err
		}
		for _, version := range []string{"v1-static-a1", "v1-channel-a1", "v1-domain-a1"} {
			var ready bool
			err = tx.QueryRow(bound, `SELECT EXISTS(SELECT 1 FROM public.v1_domain_import_reconciliation_receipts
WHERE import_version=$1 AND archive_run_id=$2 AND selected_source_count=receipt_count AND verified_count=receipt_count
AND imported_count+archived_count+quarantined_count=receipt_count)`, version, run).Scan(&ready)
			if err != nil {
				return err
			}
			if !ready {
				return ErrConflict
			}
		}
		selected, err := invalidhistory.Select(bound, archive, invalidSourceTerminalLoader{run: run}, invalidhistory.Options{ArchiveRunID: run, SourceHMACKey: key})
		if err != nil {
			return err
		}
		entries, err := invalidSourceEntries(selected, run, tx)
		if err != nil {
			return err
		}
		result.Selected = len(entries)
		if reconcile {
			proof, err := reconcileInvalidSourceEntries(bound, tx, entries, run)
			if err != nil {
				return err
			}
			result.Reconciliation = &proof
			return nil
		}
		for _, entry := range entries {
			receipt, err := entry.write(bound)
			if err != nil {
				return fmt.Errorf("invalid source history %s: %w", entry.scope.TableID, err)
			}
			if receipt.Kind != entry.kind || receipt.SourceIdentifier != entry.source || receipt.PayloadDigest != entry.payload || receipt.TargetID < 1 {
				return ErrConflict
			}
			terminal, found, err := entry.journal.LoadTerminal(bound, entry.source)
			if err != nil || !found {
				return ErrConflict
			}
			digest, err := entry.verify(bound, receipt.TargetID)
			if err != nil || digest != receipt.TargetDigest || terminal.TargetDigest != digest || terminal.PayloadDigest != entry.payload || terminal.SourceKeyDigest != entry.key || terminal.Disposition != "import" || terminal.Reason != "" || terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || len(terminal.Metadata) != 0 {
				return ErrConflict
			}
			if receipt.Replayed {
				result.Replayed++
			} else {
				result.Imported++
			}
		}
		return nil
	})
	if err != nil {
		return InvalidSourceHistoryResult{}, err
	}
	return result, nil
}

type invalidSourceTerminalLoader struct{ run string }

func (loader invalidSourceTerminalLoader) LoadTerminal(ctx context.Context, scope invalidhistory.TerminalScope, source string) (invalidhistory.TerminalReceipt, bool, error) {
	journal, err := NewJournal(Scope{ImportVersion: scope.ImportVersion, ArchiveRunID: loader.run, AdapterID: v1archive.DefaultAdapterID, TableID: scope.TableID, TargetDomain: scope.TargetDomain, TargetTable: scope.TargetTable})
	if err != nil {
		return invalidhistory.TerminalReceipt{}, false, err
	}
	value, found, err := journal.LoadTerminal(ctx, source)
	if err != nil || !found {
		return invalidhistory.TerminalReceipt{}, found, err
	}
	return invalidhistory.TerminalReceipt{SourceKeyDigest: value.SourceKeyDigest, PayloadDigest: value.PayloadDigest, TargetDigest: value.TargetDigest, Disposition: value.Disposition, Reason: value.Reason, TargetID: value.TargetID, Metadata: value.Metadata, Verified: true}, true, nil
}

type invalidSourceEntry struct {
	scope               Scope
	kind, source        string
	key, payload, field [32]byte
	journal             *Journal
	write               func(context.Context) (contactport.InvalidSourceHistoryReceipt, error)
	verify              func(context.Context, int64) ([32]byte, error)
}

func invalidSourceEntries(selection invalidhistory.Selection, run string, tx pgx.Tx) ([]invalidSourceEntry, error) {
	if run == "" || tx == nil || selection.Summary() != (invalidhistory.Summary{UnboundTags: 5, InvalidChannels: 1, Images: 3, Attachments: 1, RadarLinks: 6}) {
		return nil, ErrConflict
	}
	entries := make([]invalidSourceEntry, 0, 16)
	for _, selected := range selection.UnboundTags {
		fact := selected.Fact
		table := invalidhistory.ContactTagsTable

		scope := Scope{ImportVersion: InvalidSourceHistoryVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: table, TargetDomain: "contact", TargetTable: "contact_v1_unbound_tag_history"}
		journal, err := NewJournal(scope)
		if err != nil {
			return nil, err
		}
		binding := invalidSourceJournal{journal: journal, kind: "unbound_tag"}
		writer := contactapp.NewInvalidSourceHistoryWriter(contactstore.NewInvalidSourceHistoryStore(), binding)

		reader := contactstore.NewInvalidSourceHistoryReader(tx)
		probe := fact
		probe.ID = 1
		if _, err := contactapp.DigestHistoricalUnboundTag(probe); err != nil {
			return nil, err
		}
		entries = append(entries, invalidSourceEntry{scope: scope, kind: "unbound_tag", source: selected.SourceIdentifier, key: fact.SourceKeyDigest, payload: fact.SourcePayloadDigest, field: fact.SourceFieldDigest, journal: journal,
			write: func(ctx context.Context) (contactport.InvalidSourceHistoryReceipt, error) {
				receipt, err := writer.ImportHistoricalUnboundTag(ctx, selected.SourceIdentifier, fact)
				return contactport.InvalidSourceHistoryReceipt(receipt), err
			},
			verify: func(ctx context.Context, id int64) ([32]byte, error) {
				actual, err := reader.GetHistoricalUnboundTag(ctx, id)
				if err != nil || actual.ID != id {
					return [32]byte{}, ErrConflict
				}
				expected := fact
				expected.ID = id
				want, e1 := contactapp.DigestHistoricalUnboundTag(expected)
				got, e2 := contactapp.DigestHistoricalUnboundTag(actual)
				if e1 != nil || e2 != nil || want != got {
					return [32]byte{}, ErrConflict
				}
				return got, nil
			},
		})
	}
	for _, selected := range selection.InvalidChannels {
		fact := selected.Fact
		table := invalidhistory.AutomationChannelTable

		scope := Scope{ImportVersion: InvalidSourceHistoryVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: table, TargetDomain: "contact", TargetTable: "contact_v1_invalid_channel_history"}
		journal, err := NewJournal(scope)
		if err != nil {
			return nil, err
		}
		binding := invalidSourceJournal{journal: journal, kind: "invalid_channel"}
		writer := contactapp.NewInvalidSourceHistoryWriter(contactstore.NewInvalidSourceHistoryStore(), binding)

		reader := contactstore.NewInvalidSourceHistoryReader(tx)
		probe := fact
		probe.ID = 1
		if _, err := contactapp.DigestHistoricalInvalidChannel(probe); err != nil {
			return nil, err
		}
		entries = append(entries, invalidSourceEntry{scope: scope, kind: "invalid_channel", source: selected.SourceIdentifier, key: fact.SourceKeyDigest, payload: fact.SourcePayloadDigest, field: fact.SourceFieldDigest, journal: journal,
			write: func(ctx context.Context) (contactport.InvalidSourceHistoryReceipt, error) {
				receipt, err := writer.ImportHistoricalInvalidChannel(ctx, selected.SourceIdentifier, fact)
				return contactport.InvalidSourceHistoryReceipt(receipt), err
			},
			verify: func(ctx context.Context, id int64) ([32]byte, error) {
				actual, err := reader.GetHistoricalInvalidChannel(ctx, id)
				if err != nil || actual.ID != id {
					return [32]byte{}, ErrConflict
				}
				expected := fact
				expected.ID = id
				want, e1 := contactapp.DigestHistoricalInvalidChannel(expected)
				got, e2 := contactapp.DigestHistoricalInvalidChannel(actual)
				if e1 != nil || e2 != nil || want != got {
					return [32]byte{}, ErrConflict
				}
				return got, nil
			},
		})
	}
	for _, selected := range selection.InvalidAssets {
		fact := selected.Fact
		table := invalidhistory.ImageLibraryTable
		if fact.Kind == "attachment" {
			table = invalidhistory.AttachmentLibraryTable
		}
		scope := Scope{ImportVersion: InvalidSourceHistoryVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: table, TargetDomain: "media", TargetTable: "media_v1_invalid_asset_history"}
		journal, err := NewJournal(scope)
		if err != nil {
			return nil, err
		}
		binding := invalidSourceJournal{journal: journal, kind: "invalid_asset"}
		writer, err := mediaapp.NewInvalidSourceHistoryWriter(mediastore.NewInvalidSourceHistoryStore(), mediaInvalidSourceJournal{binding})
		if err != nil {
			return nil, err
		}
		reader := mediastore.NewInvalidSourceHistoryReader(tx)
		probe := fact
		probe.ID = 1
		if _, err := mediaapp.DigestHistoricalInvalidAsset(probe); err != nil {
			return nil, err
		}
		entries = append(entries, invalidSourceEntry{scope: scope, kind: "invalid_asset", source: selected.SourceIdentifier, key: fact.SourceKeyDigest, payload: fact.SourcePayloadDigest, field: fact.SourceFieldDigest, journal: journal,
			write: func(ctx context.Context) (contactport.InvalidSourceHistoryReceipt, error) {
				receipt, err := writer.ImportHistoricalInvalidAsset(ctx, selected.SourceIdentifier, fact)
				return contactport.InvalidSourceHistoryReceipt(receipt), err
			},
			verify: func(ctx context.Context, id int64) ([32]byte, error) {
				actual, err := reader.GetHistoricalInvalidAsset(ctx, id)
				if err != nil || actual.ID != id {
					return [32]byte{}, ErrConflict
				}
				expected := fact
				expected.ID = id
				want, e1 := mediaapp.DigestHistoricalInvalidAsset(expected)
				got, e2 := mediaapp.DigestHistoricalInvalidAsset(actual)
				if e1 != nil || e2 != nil || want != got {
					return [32]byte{}, ErrConflict
				}
				return got, nil
			},
		})
	}
	for _, selected := range selection.InvalidRadar {
		fact := selected.Fact
		table := invalidhistory.RadarLinksTable

		scope := Scope{ImportVersion: InvalidSourceHistoryVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: table, TargetDomain: "radar", TargetTable: "radar_v1_invalid_link_history"}
		journal, err := NewJournal(scope)
		if err != nil {
			return nil, err
		}
		binding := invalidSourceJournal{journal: journal, kind: "invalid_link"}
		writer, err := radarapp.NewInvalidSourceHistoryWriter(radarstore.NewInvalidSourceHistoryStore(), radarInvalidSourceJournal{binding})
		if err != nil {
			return nil, err
		}
		reader := radarstore.NewInvalidSourceHistoryReader(tx)
		probe := fact
		probe.ID = 1
		if _, err := radarapp.DigestHistoricalInvalidRadarLink(probe); err != nil {
			return nil, err
		}
		entries = append(entries, invalidSourceEntry{scope: scope, kind: "invalid_link", source: selected.SourceIdentifier, key: fact.SourceKeyDigest, payload: fact.SourcePayloadDigest, field: fact.SourceFieldDigest, journal: journal,
			write: func(ctx context.Context) (contactport.InvalidSourceHistoryReceipt, error) {
				receipt, err := writer.ImportHistoricalInvalidRadarLink(ctx, selected.SourceIdentifier, fact)
				return contactport.InvalidSourceHistoryReceipt(receipt), err
			},
			verify: func(ctx context.Context, id int64) ([32]byte, error) {
				actual, err := reader.GetHistoricalInvalidRadarLink(ctx, id)
				if err != nil || actual.ID != id {
					return [32]byte{}, ErrConflict
				}
				expected := fact
				expected.ID = id
				want, e1 := radarapp.DigestHistoricalInvalidRadarLink(expected)
				got, e2 := radarapp.DigestHistoricalInvalidRadarLink(actual)
				if e1 != nil || e2 != nil || want != got {
					return [32]byte{}, ErrConflict
				}
				return got, nil
			},
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].scope.TableID+"/"+entries[i].source < entries[j].scope.TableID+"/"+entries[j].source
	})
	for i, entry := range entries {
		if entry.source != SourceIdentifier(entry.key) || entry.key == ([32]byte{}) || entry.payload == ([32]byte{}) || entry.field == ([32]byte{}) {
			return nil, ErrConflict
		}
		if i > 0 && entries[i-1].scope.TableID == entry.scope.TableID && entries[i-1].source == entry.source {
			return nil, ErrConflict
		}
	}
	return entries, nil
}

// Narrow bridges for the three owners' typed receipt ports.
type invalidSourceJournal struct {
	journal *Journal
	kind    string
}

func (bridge invalidSourceJournal) LoadInvalidSourceHistory(ctx context.Context, kind, source string) (contactport.InvalidSourceHistoryReceipt, bool, error) {
	if bridge.journal == nil || bridge.journal.scope.ImportVersion != InvalidSourceHistoryVersion || kind != bridge.kind {
		return contactport.InvalidSourceHistoryReceipt{}, false, ErrInvalidScope
	}
	value, found, err := bridge.journal.LoadTerminal(ctx, source)
	if err != nil || !found {
		return contactport.InvalidSourceHistoryReceipt{}, found, err
	}
	id, e := positiveID(value.TargetID)
	if e != nil || value.TargetID != strconv.FormatInt(id, 10) || value.Disposition != "import" || value.Reason != "" || len(value.Metadata) != 0 || value.TargetDigest == ([32]byte{}) {
		return contactport.InvalidSourceHistoryReceipt{}, false, ErrConflict
	}
	return contactport.InvalidSourceHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: value.PayloadDigest, TargetID: id, TargetDigest: value.TargetDigest}, true, nil
}
func (bridge invalidSourceJournal) RecordInvalidSourceHistory(ctx context.Context, value contactport.InvalidSourceHistoryReceipt) error {
	if bridge.journal == nil || bridge.journal.scope.ImportVersion != InvalidSourceHistoryVersion || value.Kind != bridge.kind || value.Replayed || value.TargetID < 1 || value.PayloadDigest == ([32]byte{}) || value.TargetDigest == ([32]byte{}) {
		return ErrInvalidScope
	}
	key, err := ParseSourceIdentifier(value.SourceIdentifier)
	if err != nil || key == ([32]byte{}) || value.SourceIdentifier != SourceIdentifier(key) {
		return ErrInvalidScope
	}
	return bridge.journal.Record(ctx, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: value.PayloadDigest, TargetID: strconv.FormatInt(value.TargetID, 10), TargetDigest: value.TargetDigest, Disposition: "import"})
}

type mediaInvalidSourceJournal struct{ invalidSourceJournal }

func (bridge mediaInvalidSourceJournal) LoadInvalidSourceHistory(ctx context.Context, kind, source string) (mediaport.InvalidSourceHistoryReceipt, bool, error) {
	value, found, err := bridge.invalidSourceJournal.LoadInvalidSourceHistory(ctx, kind, source)
	return mediaport.InvalidSourceHistoryReceipt(value), found, err
}
func (bridge mediaInvalidSourceJournal) RecordInvalidSourceHistory(ctx context.Context, value mediaport.InvalidSourceHistoryReceipt) error {
	return bridge.invalidSourceJournal.RecordInvalidSourceHistory(ctx, contactport.InvalidSourceHistoryReceipt(value))
}

type radarInvalidSourceJournal struct{ invalidSourceJournal }

func (bridge radarInvalidSourceJournal) LoadInvalidSourceHistory(ctx context.Context, kind, source string) (radarport.InvalidSourceHistoryReceipt, bool, error) {
	value, found, err := bridge.invalidSourceJournal.LoadInvalidSourceHistory(ctx, kind, source)
	return radarport.InvalidSourceHistoryReceipt(value), found, err
}
func (bridge radarInvalidSourceJournal) RecordInvalidSourceHistory(ctx context.Context, value radarport.InvalidSourceHistoryReceipt) error {
	return bridge.invalidSourceJournal.RecordInvalidSourceHistory(ctx, contactport.InvalidSourceHistoryReceipt(value))
}

func reconcileInvalidSourceEntries(ctx context.Context, tx pgx.Tx, entries []invalidSourceEntry, run string) (ReconciliationResult, error) {
	if ctx == nil || tx == nil || len(entries) != 16 || run == "" {
		return ReconciliationResult{}, ErrInvalidScope
	}
	var count int64
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM public.v1_domain_import_receipts WHERE import_version=$1 AND archive_run_id=$2", InvalidSourceHistoryVersion, run).Scan(&count); err != nil {
		return ReconciliationResult{}, err
	}
	if count != 16 {
		return ReconciliationResult{}, ErrConflict
	}
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	for _, entry := range entries {
		value, found, err := entry.journal.LoadTerminal(ctx, entry.source)
		if err != nil || !found || value.SourceKeyDigest != entry.key || value.PayloadDigest != entry.payload || value.Disposition != "import" || value.Reason != "" || len(value.Metadata) != 0 {
			return ReconciliationResult{}, ErrConflict
		}
		id, err := positiveID(value.TargetID)
		if err != nil || value.TargetID != strconv.FormatInt(id, 10) {
			return ReconciliationResult{}, ErrConflict
		}
		digest, err := entry.verify(ctx, id)
		if err != nil || value.TargetDigest != digest {
			return ReconciliationResult{}, ErrConflict
		}
		if err = encoder.Encode([]any{entry.scope.TableID, entry.source, hex.EncodeToString(entry.payload[:]), hex.EncodeToString(entry.field[:]), entry.scope.TargetDomain, entry.scope.TargetTable, value.TargetID, hex.EncodeToString(digest[:])}); err != nil {
			return ReconciliationResult{}, err
		}
	}
	digest := hash.Sum(nil)
	result := ReconciliationResult{SelectedSourceCount: count, ReceiptCount: count, ImportedCount: count, VerifiedCount: count, ComparisonDigest: hex.EncodeToString(digest)}
	command, err := tx.Exec(ctx, `INSERT INTO public.v1_domain_import_reconciliation_receipts
(import_version,archive_run_id,selected_source_count,receipt_count,imported_count,archived_count,quarantined_count,verified_count,comparison_digest)
VALUES($1,$2,$3,$3,$3,0,0,$3,$4) ON CONFLICT(import_version,archive_run_id) DO NOTHING`, InvalidSourceHistoryVersion, run, count, digest)
	if err != nil {
		return ReconciliationResult{}, err
	}
	result.Replayed = command.RowsAffected() == 0
	if result.Replayed {
		var selected, receipts, imported, archived, quarantined, verified int64
		var old []byte
		err = tx.QueryRow(ctx, `SELECT selected_source_count,receipt_count,imported_count,archived_count,quarantined_count,verified_count,comparison_digest
FROM public.v1_domain_import_reconciliation_receipts WHERE import_version=$1 AND archive_run_id=$2`, InvalidSourceHistoryVersion, run).Scan(&selected, &receipts, &imported, &archived, &quarantined, &verified, &old)
		if err != nil || selected != count || receipts != count || imported != count || verified != count || archived != 0 || quarantined != 0 || !equalBytes(old, digest) {
			return ReconciliationResult{}, ErrConflict
		}
	}
	return result, nil
}
