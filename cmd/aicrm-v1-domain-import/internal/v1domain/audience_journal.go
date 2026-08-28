package v1domain

import (
	"context"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1audiencehistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

var audienceHistoryScopes = []struct{ kind, source, target string }{
	{"groups", v1audiencehistory.PackageGroupsTableID, "segment_v1_audience_groups"},
	{"packages", v1audiencehistory.PackagesTableID, "segment_v1_audience_packages"},
	{"versions", v1audiencehistory.PackageVersionsTableID, "segment_v1_audience_versions"},
	{"senders", v1audiencehistory.PackageSendersTableID, "segment_v1_audience_senders"},
	{"rules", v1audiencehistory.RulesTableID, "segment_v1_audience_rules"},
	{"rule_versions", v1audiencehistory.RuleVersionsTableID, "segment_v1_audience_rule_versions"},
	{"definitions", v1audiencehistory.SegmentsTableID, "segment_v1_definitions"},
	{"members", v1audiencehistory.AudienceMembersTableID, "segment_v1_audience_members"},
}

// Eight immutable source journals share the caller transaction. Runtime tables
// remain in the existing encrypted archive; this package never resumes them.
type AudienceHistoryJournal struct{ journals map[string]*Journal }

var _ segmentport.AudienceHistoryJournal = (*AudienceHistoryJournal)(nil)

// The map is keyed by source table ID, not by a caller-provided target name.
func NewAudienceHistoryJournal(journals map[string]*Journal) (*AudienceHistoryJournal, error) {
	if !validAudienceHistoryJournals(journals) {
		return nil, ErrInvalidScope
	}
	copy := make(map[string]*Journal, len(journals))
	for key, value := range journals {
		copy[key] = value
	}
	return &AudienceHistoryJournal{journals: copy}, nil
}

func validAudienceHistoryJournals(journals map[string]*Journal) bool {
	if len(journals) != len(audienceHistoryScopes) {
		return false
	}
	var version, run string
	for _, scope := range audienceHistoryScopes {
		j := journals[scope.source]
		if j == nil || j.tx == nil || !j.scope.valid() || j.scope.AdapterID != v1archive.DefaultAdapterID ||
			j.scope.TableID != scope.source || j.scope.TargetDomain != "segment" || j.scope.TargetTable != scope.target {
			return false
		}
		if version == "" {
			version, run = j.scope.ImportVersion, j.scope.ArchiveRunID
		}
		if j.scope.ImportVersion != version || j.scope.ArchiveRunID != run {
			return false
		}
	}
	return true
}

func (j *AudienceHistoryJournal) scope(kind string) (*Journal, error) {
	if j == nil || !validAudienceHistoryJournals(j.journals) {
		return nil, ErrInvalidScope
	}
	for _, scope := range audienceHistoryScopes {
		if scope.kind == kind {
			return j.journals[scope.source], nil
		}
	}
	return nil, ErrInvalidScope
}

func (j *AudienceHistoryJournal) LoadAudienceHistory(ctx context.Context, kind, source string) (segmentport.AudienceHistoryReceipt, bool, error) {
	selected, err := j.scope(kind)
	if err != nil {
		return segmentport.AudienceHistoryReceipt{}, false, err
	}
	terminal, found, err := selected.LoadTerminal(ctx, source)
	if err != nil || !found {
		return segmentport.AudienceHistoryReceipt{}, found, err
	}
	receipt, err := audienceHistoryReceiptFromTerminal(source, terminal)
	return receipt, err == nil, err
}

func audienceHistoryReceiptFromTerminal(source string, terminal TerminalReceipt) (segmentport.AudienceHistoryReceipt, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := positiveID(terminal.TargetID)
	if err != nil || key == [32]byte{} || idErr != nil || strconv.FormatInt(id, 10) != terminal.TargetID ||
		key != terminal.SourceKeyDigest || terminal.PayloadDigest == [32]byte{} || terminal.TargetDigest == [32]byte{} ||
		terminal.Disposition != "import" || terminal.Reason != "" || len(terminal.Metadata) != 0 {
		return segmentport.AudienceHistoryReceipt{}, ErrConflict
	}
	return segmentport.AudienceHistoryReceipt{SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest,
		TargetID: id, TargetDigest: terminal.TargetDigest}, nil
}

func (j *AudienceHistoryJournal) RecordAudienceHistory(ctx context.Context, kind string, receipt segmentport.AudienceHistoryReceipt) error {
	selected, err := j.scope(kind)
	if err != nil {
		return err
	}
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || key == [32]byte{} || receipt.PayloadDigest == [32]byte{} || receipt.TargetDigest == [32]byte{} || receipt.TargetID < 1 || receipt.Replayed {
		return ErrInvalidScope
	}
	return selected.Record(ctx, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest,
		Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest})
}
