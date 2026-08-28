package v1domain

import (
	"context"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	invalidhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1invalidsourcehistory"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	radarstore "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store"
)

const InvalidSourceHistoryVersion = "v1-invalid-source-history-a1"

type InvalidSourceHistoryResult struct {
	Selected, Imported, Replayed int
	Reconciliation               *ReconciliationResult `json:",omitempty"`
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
