package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	memberusage "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxcmemberusagehistory"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const HXCMemberUsageHistoryVersion = "v1-hxc-member-usage-history-a1"

const hxcMemberUsageHistoryTarget = "hxc_v1_member_usage_history"

// HXCMemberUsageHistoryResult counts only inert HXC generation observations.
type HXCMemberUsageHistoryResult struct {
	Selected, Imported, Replayed int
	Reconciliation               *ReconciliationResult `json:",omitempty"`
}

// hxcMemberUsageHistoryJournal bridges the typed owner writer to the one
// scoped generic journal owned by the caller transaction.
type hxcMemberUsageHistoryJournal struct{ journal *Journal }

var _ hxcport.HXCHistoryJournal = hxcMemberUsageHistoryJournal{}

func (bridge hxcMemberUsageHistoryJournal) LoadHXCHistory(ctx context.Context, kind, source string) (hxcport.HXCHistoryReceipt, bool, error) {
	if bridge.journal == nil || bridge.journal.scope.ImportVersion != HXCMemberUsageHistoryVersion || kind != hxcport.HXCHistoryMemberUsage {
		return hxcport.HXCHistoryReceipt{}, false, ErrInvalidScope
	}
	value, found, err := bridge.journal.LoadTerminal(ctx, source)
	if err != nil || !found {
		return hxcport.HXCHistoryReceipt{}, found, err
	}
	key, keyErr := ParseSourceIdentifier(source)
	id, idErr := positiveID(value.TargetID)
	if keyErr != nil || idErr != nil || key == ([sha256.Size]byte{}) || source != SourceIdentifier(key) || value.SourceKeyDigest != key || value.PayloadDigest == ([sha256.Size]byte{}) || value.TargetDigest == ([sha256.Size]byte{}) || value.Disposition != "import" || value.Reason != "" || len(value.Metadata) != 0 || value.TargetID != strconv.FormatInt(id, 10) {
		return hxcport.HXCHistoryReceipt{}, false, ErrConflict
	}
	return hxcport.HXCHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: value.PayloadDigest, TargetID: id, TargetDigest: value.TargetDigest}, true, nil
}

func (bridge hxcMemberUsageHistoryJournal) RecordHXCHistory(ctx context.Context, value hxcport.HXCHistoryReceipt) error {
	if bridge.journal == nil || bridge.journal.scope.ImportVersion != HXCMemberUsageHistoryVersion || value.Kind != hxcport.HXCHistoryMemberUsage || value.Replayed || value.TargetID < 1 || value.PayloadDigest == ([sha256.Size]byte{}) || value.TargetDigest == ([sha256.Size]byte{}) {
		return ErrInvalidScope
	}
	key, err := ParseSourceIdentifier(value.SourceIdentifier)
	if err != nil || key == ([sha256.Size]byte{}) || value.SourceIdentifier != SourceIdentifier(key) {
		return ErrInvalidScope
	}
	return bridge.journal.Record(ctx, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: value.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(value.TargetID, 10), TargetDigest: value.TargetDigest})
}

type hxcMemberUsageHistoryEntry struct {
	scope               Scope
	kind, source        string
	ordinal             int64
	key, payload, field [sha256.Size]byte
	journal             *Journal
	write               func(context.Context) (hxcport.HXCHistoryReceipt, error)
	verify              func(context.Context, int64) ([sha256.Size]byte, error)
}

// hxcMemberUsageEntries converts one already-authenticated source
// batch. It intentionally preserves stream order and never buffers, sorts, or
// maps the full 810554-row source.
func hxcMemberUsageEntries(ctx context.Context, facts []memberusage.MemberUsageObservationFact, run string, tx pgx.Tx, store hxcport.HXCMemberUsageHistoryStore, reader hxcport.HXCMemberUsageHistoryReader) ([]hxcMemberUsageHistoryEntry, error) {
	if ctx == nil || ctx.Err() != nil || run == "" || tx == nil || store == nil || reader == nil || len(facts) > memberusage.StreamBatchSize {
		return nil, ErrInvalidScope
	}
	if len(facts) == 0 {
		return []hxcMemberUsageHistoryEntry{}, nil
	}
	scope := Scope{ImportVersion: HXCMemberUsageHistoryVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: memberusage.MemberUsageProjectionTableID, TargetDomain: "hxc", TargetTable: hxcMemberUsageHistoryTarget}
	journal, err := NewJournal(scope)
	if err != nil {
		return nil, err
	}
	writer, err := hxcapp.NewHXCMemberUsageHistoryWriter(store, hxcMemberUsageHistoryJournal{journal: journal})
	if err != nil {
		return nil, err
	}
	entries := make([]hxcMemberUsageHistoryEntry, 0, len(facts))
	var previousOrdinal int64
	for _, source := range facts {
		value, err := hxcMemberUsageHistoryFact(source)
		if err != nil || source.Source.SourceOrdinal <= previousOrdinal {
			return nil, ErrConflict
		}
		previousOrdinal = source.Source.SourceOrdinal
		probe := value
		probe.ID = 1
		if _, err = hxcapp.HistoricalHXCMemberUsageDigest(probe); err != nil {
			return nil, ErrConflict
		}
		identifier := SourceIdentifier(value.SourceKeyDigest)
		for _, existing := range entries {
			if existing.source == identifier {
				return nil, ErrConflict
			}
		}
		fact := value
		entries = append(entries, hxcMemberUsageHistoryEntry{
			scope: scope, kind: hxcport.HXCHistoryMemberUsage, source: identifier, ordinal: source.Source.SourceOrdinal,
			key: value.SourceKeyDigest, payload: value.SourcePayloadDigest, field: value.SourceFieldDigest, journal: journal,
			write: func(ctx context.Context) (hxcport.HXCHistoryReceipt, error) {
				return writer.ImportMemberUsage(ctx, identifier, fact)
			},
			verify: func(ctx context.Context, id int64) ([sha256.Size]byte, error) {
				actual, err := reader.GetHistoricalHXCMemberUsage(ctx, id)
				if err != nil || actual.ID != id {
					return [sha256.Size]byte{}, ErrConflict
				}
				expected := fact
				expected.ID = id
				want, wantErr := hxcapp.HistoricalHXCMemberUsageDigest(expected)
				got, gotErr := hxcapp.HistoricalHXCMemberUsageDigest(actual)
				if wantErr != nil || gotErr != nil || want != got {
					return [sha256.Size]byte{}, ErrConflict
				}
				return got, nil
			},
		})
	}
	return entries, nil
}

func hxcMemberUsageHistoryFact(source memberusage.MemberUsageObservationFact) (hxcport.HistoricalHXCMemberUsage, error) {
	if source.Source.SourceOrdinal < 1 || source.Source.SourceKeyHMAC == ([sha256.Size]byte{}) || source.Source.PayloadHMAC == ([sha256.Size]byte{}) || source.Source.FieldHMAC == ([sha256.Size]byte{}) || len(source.Source.RedactedFields) != 0 || len(source.PayloadJSON) == 0 || !json.Valid(source.PayloadJSON) || source.ProjectedAt.IsZero() {
		return hxcport.HistoricalHXCMemberUsage{}, ErrConflict
	}
	return hxcport.HistoricalHXCMemberUsage{
		SourceKeyDigest: source.Source.SourceKeyHMAC, SourcePayloadDigest: source.Source.PayloadHMAC, SourceFieldDigest: source.Source.FieldHMAC,
		Generation: source.Generation, UnionID: source.ResolverUnionID(), OwnerUserID: source.LegacyOwnerUserID(), MobileHash: source.MobileHash(),
		IsMember: source.IsMember, IsRegistered: source.IsRegistered, RegisteredAt: hxcHistoryTimePtr(source.RegisteredAt), HasRealUsage: source.HasRealUsage,
		FirstUsedAt: hxcHistoryTimePtr(source.FirstUsedAt), LastUsedAt: hxcHistoryTimePtr(source.LastUsedAt), MemberSince: hxcHistoryTimePtr(source.MemberSince), MembershipExpiresAt: hxcHistoryTimePtr(source.MembershipExpiresAt),
		MembershipTier: source.MembershipTier, MembershipStatus: source.MembershipStatus, MembershipSource: source.MembershipSource, RegistrationSource: source.RegistrationSource, UsageSource: source.UsageSource,
		UpdatedAt: hxcHistoryTimePtr(source.UpdatedAt), PayloadJSON: append(json.RawMessage(nil), source.PayloadJSON...), ProjectedAt: source.ProjectedAt.UTC().Truncate(time.Microsecond),
	}, nil
}
