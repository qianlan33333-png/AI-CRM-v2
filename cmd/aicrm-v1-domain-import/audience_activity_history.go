package main

import (
	"context"
	"crypto/sha256"
	"strconv"
	"strings"

	segmentactivity "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1audienceactivityhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1audiencehistory"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

const audienceActivityImportVersion = "v1-audience-activity-history-a1"

// audienceActivityReferences is deliberately private to the archive command.
// It does not infer a parent from a source_id alone: every reference first
// proves the old audience-history receipt for this same sealed archive run.
type audienceActivityReferences struct {
	archiveRun string
	sourceKey  []byte
	reader     *segmentstore.AudienceActivityHistoryReader
}

var _ segmentport.AudienceActivityHistoryReferences = (*audienceActivityReferences)(nil)

func newAudienceActivityReferences(run string, sourceKey []byte, reader *segmentstore.AudienceActivityHistoryReader) (*audienceActivityReferences, error) {
	if run == "" || strings.TrimSpace(run) != run || len(sourceKey) < sha256.Size || reader == nil {
		return nil, v1domain.ErrInvalidScope
	}
	return &audienceActivityReferences{archiveRun: run, sourceKey: append([]byte(nil), sourceKey...), reader: reader}, nil
}

func (references *audienceActivityReferences) ResolveAudienceActivityPackage(ctx context.Context, sourceID int64) (segmentport.AudienceActivityPackageReference, error) {
	if references == nil || sourceID < 1 {
		return segmentport.AudienceActivityPackageReference{}, v1domain.ErrInvalidScope
	}
	actual, err := references.reader.ResolveAudienceActivityPackage(ctx, sourceID)
	if err != nil || actual.ID < 1 {
		return segmentport.AudienceActivityPackageReference{}, audienceActivityReferenceError(err)
	}
	if err = references.verifyPriorTarget(ctx, v1audiencehistory.PackagesTableID, "segment_v1_audience_packages", sourceID, actual.ID); err != nil {
		return segmentport.AudienceActivityPackageReference{}, err
	}
	return actual, nil
}

func (references *audienceActivityReferences) ResolveAudienceActivityVersion(ctx context.Context, sourceID int64) (segmentport.AudienceActivityVersionReference, error) {
	if references == nil || sourceID < 1 {
		return segmentport.AudienceActivityVersionReference{}, v1domain.ErrInvalidScope
	}
	actual, err := references.reader.ResolveAudienceActivityVersion(ctx, sourceID)
	if err != nil || actual.ID < 1 || actual.PackageHistoryID < 1 {
		return segmentport.AudienceActivityVersionReference{}, audienceActivityReferenceError(err)
	}
	if err = references.verifyPriorTarget(ctx, v1audiencehistory.PackageVersionsTableID, "segment_v1_audience_versions", sourceID, actual.ID); err != nil {
		return segmentport.AudienceActivityVersionReference{}, err
	}
	return actual, nil
}

func (references *audienceActivityReferences) ResolveAudienceActivityMember(ctx context.Context, sourceID int64) (segmentport.AudienceActivityMemberReference, error) {
	if references == nil || sourceID < 1 {
		return segmentport.AudienceActivityMemberReference{}, v1domain.ErrInvalidScope
	}
	actual, err := references.reader.ResolveAudienceActivityMember(ctx, sourceID)
	if err != nil || actual.ID < 1 || actual.PackageHistoryID < 1 {
		return segmentport.AudienceActivityMemberReference{}, audienceActivityReferenceError(err)
	}
	if err = references.verifyPriorTarget(ctx, v1audiencehistory.AudienceMembersTableID, "segment_v1_audience_members", sourceID, actual.ID); err != nil {
		return segmentport.AudienceActivityMemberReference{}, err
	}
	return actual, nil
}

func (references *audienceActivityReferences) ResolveAudienceActivityRun(ctx context.Context, sourceID int64) (segmentport.HistoricalAudienceActivityRun, error) {
	if references == nil || sourceID < 1 {
		return segmentport.HistoricalAudienceActivityRun{}, v1domain.ErrInvalidScope
	}
	actual, err := references.reader.ResolveAudienceActivityRun(ctx, sourceID)
	if err != nil || actual.ID < 1 || actual.SourceID != sourceID {
		return segmentport.HistoricalAudienceActivityRun{}, audienceActivityReferenceError(err)
	}
	if err = references.verifyCurrentTarget(ctx, segmentactivity.PackageRunsTableID, "segment_v1_audience_activity_runs", actual.SourceKeyDigest, actual.SourcePayloadDigest, actual.ID); err != nil {
		return segmentport.HistoricalAudienceActivityRun{}, err
	}
	return actual, nil
}

func (references *audienceActivityReferences) verifyPriorTarget(ctx context.Context, table, targetTable string, sourceID, targetID int64) error {
	if sourceID < 1 || targetID < 1 {
		return v1domain.ErrConflict
	}
	key, err := audienceActivityParentSourceKey(references.sourceKey, table, sourceID)
	if err != nil {
		return v1domain.ErrConflict
	}
	return references.verifyReceipt(ctx, v1domain.AudienceHistoryImportVersion, table, targetTable, key, targetID, [sha256.Size]byte{})
}

func audienceActivityParentSourceKey(key []byte, table string, sourceID int64) ([sha256.Size]byte, error) {
	if len(key) < sha256.Size || sourceID < 1 || !strings.HasPrefix(table, "public/") {
		return [sha256.Size]byte{}, v1domain.ErrInvalidScope
	}
	return v1archive.SourceKeyHMAC(key, strings.TrimPrefix(table, "public/"), []byte("["+strconv.FormatInt(sourceID, 10)+"]"))
}

func (references *audienceActivityReferences) verifyCurrentTarget(ctx context.Context, table, targetTable string, source, payload [sha256.Size]byte, targetID int64) error {
	if source == ([sha256.Size]byte{}) || payload == ([sha256.Size]byte{}) || targetID < 1 {
		return v1domain.ErrConflict
	}
	return references.verifyReceipt(ctx, audienceActivityImportVersion, table, targetTable, source, targetID, payload)
}

// verifyReceipt is intentionally a scalar exact-match query. A missing,
// non-import, unverified, wrong-run, or duplicate receipt is indistinguishable
// to callers from an unresolved historical relation and therefore fails closed.
func (references *audienceActivityReferences) verifyReceipt(ctx context.Context, version, table, targetTable string, source [sha256.Size]byte, targetID int64, payload [sha256.Size]byte) error {
	if references == nil || ctx == nil || source == ([sha256.Size]byte{}) || targetID < 1 {
		return v1domain.ErrInvalidScope
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	var verified bool
	arguments := []any{version, references.archiveRun, v1archive.DefaultAdapterID, table, source[:], "segment", targetTable, strconv.FormatInt(targetID, 10)}
	query := `SELECT EXISTS(SELECT 1 FROM public.v1_domain_import_receipts
WHERE import_version=$1 AND archive_run_id=$2 AND adapter_id=$3 AND table_id=$4 AND source_key_digest=$5
AND verified AND disposition='import' AND reason='' AND target_domain=$6 AND target_table=$7 AND target_id=$8`
	if payload != ([sha256.Size]byte{}) {
		query += " AND payload_digest=$9"
		arguments = append(arguments, payload[:])
	}
	query += ")"
	if err = tx.QueryRow(ctx, query, arguments...).Scan(&verified); err != nil || !verified {
		return audienceActivityReferenceError(err)
	}
	return nil
}

func audienceActivityReferenceError(err error) error {
	if err != nil {
		return err
	}
	return v1domain.ErrConflict
}
