package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1membergridhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
)

const memberGridHistoryImportVersion = "v1-member-grid-history-a1"

func loadMemberGridHistoryRecovery(path, run string) ([]v1membergridhistory.UsageSnapshotRecoveryEntry, error) {
	invalid := errors.New("member-grid-history requires a valid frozen usage recovery file")
	if path == "" || run != v1membergridhistory.FixedUsageSnapshotRecoveryScope().ArchiveRunID {
		return nil, invalid
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, invalid
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var header struct {
		Scope       v1membergridhistory.UsageSnapshotRecoveryScope `json:"scope"`
		QuerySHA256 string                                         `json:"query_sha256"`
		RowCount    int                                            `json:"row_count"`
	}
	if err := decoder.Decode(&header); err != nil || header.Scope != v1membergridhistory.FixedUsageSnapshotRecoveryScope() || header.RowCount < 1 {
		return nil, invalid
	}
	queryDigest, err := hex.DecodeString(header.QuerySHA256)
	if err != nil || len(queryDigest) != 32 || hex.EncodeToString(queryDigest) != header.QuerySHA256 {
		return nil, invalid
	}
	var entries []v1membergridhistory.UsageSnapshotRecoveryEntry
	seen := map[[32]byte]bool{}
	for {
		var entry v1membergridhistory.UsageSnapshotRecoveryEntry
		err := decoder.Decode(&entry)
		if err == io.EOF {
			break
		}
		if err != nil || entry.Scope != v1membergridhistory.FixedUsageSnapshotRecoveryScope() || entry.SourceKeyHMAC == ([32]byte{}) ||
			entry.OriginalPayloadHMAC == ([32]byte{}) || entry.OriginalFieldHMAC == ([32]byte{}) || entry.EntryHMAC == ([32]byte{}) || seen[entry.SourceKeyHMAC] {
			return nil, invalid
		}
		seen[entry.SourceKeyHMAC] = true
		entries = append(entries, entry)
	}
	if len(entries) != header.RowCount {
		return nil, invalid
	}
	return entries, nil
}

func importMemberGridHistory(ctx context.Context, pool *pgxpool.Pool, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, dm01Run int64, dm01Key, archiveSourceKey []byte, recovery []v1membergridhistory.UsageSnapshotRecoveryEntry) (v1domain.MemberGridHistoryImportResult, error) {
	// Reuse the existing source/receipt/actual-target verifier, not a guessed FK.
	if _, err := v1domain.ReconcileServicePeriod(ctx, pool, servicePeriodImportVersion, run); err != nil {
		return v1domain.MemberGridHistoryImportResult{}, err
	}
	resolver, err := newMemberGridHistoryReferences(ctx, archive, uow, run, dm01Run, dm01Key)
	if err != nil {
		return v1domain.MemberGridHistoryImportResult{}, err
	}
	journal, err := newMemberGridHistoryJournal(run)
	if err != nil {
		return v1domain.MemberGridHistoryImportResult{}, err
	}
	writer := productapp.NewMemberGridHistoryWriter(productstore.NewMemberGridHistoryStore(), journal)
	importer, err := v1domain.NewMemberGridHistoryImporter(archive, uow, writer, resolver, recovery, archiveSourceKey, journal)
	if err != nil {
		return v1domain.MemberGridHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

func newMemberGridHistoryJournal(run string) (*v1domain.MemberGridHistoryJournal, error) {
	var journals [5]*v1domain.Journal
	for i, mapping := range [][2]string{
		{v1membergridhistory.MemberViewsTableID, "product_v1_member_view_history"},
		{v1membergridhistory.UsageSnapshotsTableID, "product_v1_member_usage_history"},
		{v1membergridhistory.UsageSyncRunsTableID, "product_v1_member_grid_context_archive"},
		{v1membergridhistory.MemberCollaboratorsTableID, "product_v1_member_grid_context_archive"},
		{v1membergridhistory.MemberSharesTableID, "product_v1_member_grid_context_archive"},
	} {
		journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: memberGridHistoryImportVersion, ArchiveRunID: run,
			AdapterID: v1archive.DefaultAdapterID, TableID: mapping[0], TargetDomain: "product", TargetTable: mapping[1]})
		if err != nil {
			return nil, err
		}
		journals[i] = journal
	}
	return v1domain.NewMemberGridHistoryJournal(journals[0], journals[1], journals[2], journals[3], journals[4])
}
