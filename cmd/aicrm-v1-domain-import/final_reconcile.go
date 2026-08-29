package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	finalReconcileMode         = "final-reconcile"
	finalReconcileDomain       = "final"
	finalMigrationManifestPath = "docs/release/final-v1-domain-migration-manifest.json"
	finalManifestDomainCount   = 40
	finalManifestSchemaFrom    = 132
	finalManifestSchemaTo      = 141
	finalVerificationModel     = "36_fresh_domain_reconciliations_then_read_only_aggregate"
)

type finalMigrationManifest struct {
	Schema struct {
		From                int      `json:"from"`
		To                  int      `json:"to"`
		ForbiddenHXCObjects []string `json:"forbidden_hxc_objects"`
	} `json:"schema"`
	Scope struct {
		Source                   string `json:"source"`
		SourceDatabaseConnection string `json:"source_database_connection"`
		Target                   string `json:"target"`
		ExternalEffects          string `json:"external_effects"`
	} `json:"scope"`
	Phases  []finalMigrationPhase  `json:"phases"`
	Domains []finalMigrationDomain `json:"domains"`
}

type finalMigrationPhase struct {
	Mode   string `json:"mode"`
	Expect string `json:"expect"`
}

type finalMigrationDomain struct {
	Domain   string   `json:"domain"`
	Requires []string `json:"requires"`
}

type finalDomainSpec struct {
	Domain        string
	ImportVersion string
	SourceTables  []string
}

// finalDomainSpecs deliberately spells out the release manifest. In
// particular, the five first-package domains are kept separate from the old
// domain=all CLI shortcut, while still sharing its existing immutable receipt
// scope.
var finalDomainSpecs = map[string]finalDomainSpec{
	"campaign":                    {Domain: "campaign", ImportVersion: domainImportVersion, SourceTables: []string{"public/campaigns", "public/campaign_steps"}},
	"survey":                      {Domain: "survey", ImportVersion: domainImportVersion, SourceTables: []string{"public/questionnaires", "public/questionnaire_questions", "public/questionnaire_options", "public/questionnaire_submissions", "public/questionnaire_submission_answers"}},
	"media":                       {Domain: "media", ImportVersion: domainImportVersion, SourceTables: []string{"public/miniprogram_library"}},
	"radar":                       {Domain: "radar", ImportVersion: domainImportVersion, SourceTables: []string{"public/radar_links"}},
	"shop":                        {Domain: "shop", ImportVersion: domainImportVersion, SourceTables: []string{"public/wechat_shop_orders"}},
	"static":                      {Domain: "static", ImportVersion: staticImportVersion},
	"finance":                     {Domain: "finance", ImportVersion: financeImportVersion},
	"service-period":              {Domain: "service-period", ImportVersion: servicePeriodImportVersion},
	"coupon":                      {Domain: "coupon", ImportVersion: couponImportVersion},
	"channel":                     {Domain: "channel", ImportVersion: channelImportVersion},
	"groupops":                    {Domain: "groupops", ImportVersion: groupOpsHistoryImportVersion},
	"audience-history":            {Domain: "audience-history", ImportVersion: "v1-audience-history-a1"},
	"message-history":             {Domain: "message-history", ImportVersion: messageHistoryImportVersion},
	"contact-history":             {Domain: "contact-history", ImportVersion: contactHistoryImportVersion},
	"member-grid-history":         {Domain: "member-grid-history", ImportVersion: memberGridHistoryImportVersion},
	"campaign-history":            {Domain: "campaign-history", ImportVersion: campaignHistoryImportVersion},
	"campaign-definition-history": {Domain: "campaign-definition-history", ImportVersion: campaignDefinitionHistoryImportVersion},
	"automation-history":          {Domain: "automation-history", ImportVersion: automationHistoryImportVersion},
	"profile-catalog-history":     {Domain: "profile-catalog-history", ImportVersion: profileCatalogHistoryImportVersion},
	"hxc-history":                 {Domain: "hxc-history", ImportVersion: hxcHistoryImportVersion},
	"hxc-runtime-history":         {Domain: "hxc-runtime-history", ImportVersion: "v1-hxc-runtime-history-a1"},
	"hxc-chat-job-history":        {Domain: "hxc-chat-job-history", ImportVersion: "v1-hxc-chat-job-history-a1"},
	"hxc-member-usage-history":    {Domain: "hxc-member-usage-history", ImportVersion: "v1-hxc-member-usage-history-a1"},
	"contact-reference-history":   {Domain: "contact-reference-history", ImportVersion: "v1-contact-reference-history-a1"},
	"cycle-observation-history":   {Domain: "cycle-observation-history", ImportVersion: "v1-cycle-observation-history-a1"},
	"static-tail-history":         {Domain: "static-tail-history", ImportVersion: staticTailHistoryImportVersion},
	"customer-state-history":      {Domain: "customer-state-history", ImportVersion: customerStateHistoryImportVersion},
	"marketing-state-history":     {Domain: "marketing-state-history", ImportVersion: marketingStateHistoryImportVersion},
	"wecom-contact-history":       {Domain: "wecom-contact-history", ImportVersion: weComContactHistoryImportVersion},
	"radar-click-history":         {Domain: "radar-click-history", ImportVersion: "v1-radar-click-history-a1"},
	"marketing-config-history":    {Domain: "marketing-config-history", ImportVersion: "v1-marketing-config-history-a1"},
	"survey-unresolved-history":   {Domain: "survey-unresolved-history", ImportVersion: "v1-survey-unresolved-history-a1"},
	"legacy-marketing-history":    {Domain: "legacy-marketing-history", ImportVersion: legacyMarketingHistoryImportVersion},
	"broadcast-job-history":       {Domain: "broadcast-job-history", ImportVersion: broadcastJobHistoryImportVersion},
	"outbound-task-history":       {Domain: "outbound-task-history", ImportVersion: outboundTaskHistoryImportVersion},
	"deferred-identity-history":   {Domain: "deferred-identity-history", ImportVersion: "v1-deferred-identity-history-a1"},
	"external-identity-gap":       {Domain: "external-identity-gap", ImportVersion: "v1-external-identity-gap-a1"},
	"invalid-source-history":      {Domain: "invalid-source-history", ImportVersion: "v1-invalid-source-history-a1"},
	"customer-timeline-history":   {Domain: "customer-timeline-history", ImportVersion: customerTimelineHistoryImportVersion},
	"audience-activity-history":   {Domain: "audience-activity-history", ImportVersion: audienceActivityImportVersion},
}

type finalReconciliationResult struct {
	ManifestDomainCount  int                        `json:"manifest_domain_count"`
	VerificationModel    string                     `json:"verification_model"`
	Domains              []finalDomainProof         `json:"domains"`
	ReconciliationGroups []finalReconciliationGroup `json:"reconciliation_groups"`
	IdentityMapping      finalIdentityProof         `json:"identity_mapping"`
	ExternalEffects      int64                      `json:"external_effects"`
}

type finalDomainProof struct {
	Domain                    string `json:"domain"`
	ImportVersion             string `json:"import_version"`
	ReconciliationScope       string `json:"reconciliation_scope"`
	SharedReconciliationGroup bool   `json:"shared_reconciliation_group"`
	SelectedSourceCount       int64  `json:"selected_source_count"`
	ReceiptCount              int64  `json:"receipt_count"`
	ImportedCount             int64  `json:"imported_count"`
	ArchivedCount             int64  `json:"archived_count"`
	QuarantinedCount          int64  `json:"quarantined_count"`
	VerifiedCount             int64  `json:"verified_count"`
	ReceiptDigest             string `json:"receipt_digest"`
}

// finalReconciliationGroup is the immutable proof produced by one existing
// domain reconcile invocation. v1-domain-a1 intentionally covers five
// explicit manifest domains; it is therefore never repeated as per-domain
// target proof above.
type finalReconciliationGroup struct {
	ImportVersion        string   `json:"import_version"`
	Domains              []string `json:"domains"`
	SelectedSourceCount  int64    `json:"selected_source_count"`
	ReceiptCount         int64    `json:"receipt_count"`
	ImportedCount        int64    `json:"imported_count"`
	ArchivedCount        int64    `json:"archived_count"`
	QuarantinedCount     int64    `json:"quarantined_count"`
	VerifiedCount        int64    `json:"verified_count"`
	ReconciliationDigest string   `json:"reconciliation_digest"`
}

type finalIdentityProof struct {
	DM01RunID       int64 `json:"dm01_run_id"`
	MappingCount    int64 `json:"mapping_count"`
	VerifiedMapping int64 `json:"verified_mapping_count"`
}

type reconciliationCounts struct {
	Selected    int64
	Receipts    int64
	Imported    int64
	Archived    int64
	Quarantined int64
	Verified    int64
	Digest      []byte
}

func loadFinalMigrationManifest(path string) (finalMigrationManifest, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return finalMigrationManifest{}, fmt.Errorf("final migration manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return finalMigrationManifest{}, fmt.Errorf("final migration manifest must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return finalMigrationManifest{}, err
	}
	defer file.Close()
	var manifest finalMigrationManifest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&manifest); err != nil {
		return finalMigrationManifest{}, fmt.Errorf("decode final migration manifest: %w", err)
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return finalMigrationManifest{}, fmt.Errorf("final migration manifest must contain one document")
	}
	if err = validateFinalMigrationManifest(manifest); err != nil {
		return finalMigrationManifest{}, err
	}
	return manifest, nil
}

func validateFinalMigrationManifest(manifest finalMigrationManifest) error {
	if manifest.Schema.From != finalManifestSchemaFrom || manifest.Schema.To != finalManifestSchemaTo ||
		manifest.Scope.Source != "V2 sealed archive only" || manifest.Scope.SourceDatabaseConnection != "forbidden" || manifest.Scope.ExternalEffects != "disabled" {
		return fmt.Errorf("final migration manifest scope is not sealed and external-effects disabled")
	}
	if len(manifest.Phases) != 2 || manifest.Phases[0].Mode != "import" || manifest.Phases[1].Mode != "reconcile" {
		return fmt.Errorf("final migration manifest must require import then reconcile")
	}
	if len(manifest.Domains) != finalManifestDomainCount {
		return fmt.Errorf("final migration manifest must enumerate %d domains, got %d", finalManifestDomainCount, len(manifest.Domains))
	}
	seen := make(map[string]struct{}, len(manifest.Domains))
	for _, item := range manifest.Domains {
		if item.Domain == "" || item.Domain == "all" {
			return fmt.Errorf("final migration manifest has an invalid domain %q", item.Domain)
		}
		if _, duplicate := seen[item.Domain]; duplicate {
			return fmt.Errorf("final migration manifest repeats domain %q", item.Domain)
		}
		if _, found := finalDomainSpecs[item.Domain]; !found {
			return fmt.Errorf("final migration manifest domain %q has no verifier", item.Domain)
		}
		seen[item.Domain] = struct{}{}
	}
	if len(seen) != len(finalDomainSpecs) {
		return fmt.Errorf("final migration manifest does not match the complete 40-domain verifier set")
	}
	return nil
}

func reconcileFinalMigration(ctx context.Context, pool *pgxpool.Pool, archiveRunID string, dm01RunID int64, manifest finalMigrationManifest) (finalReconciliationResult, error) {
	if ctx == nil || pool == nil || archiveRunID == "" || dm01RunID < 1 {
		return finalReconciliationResult{}, fmt.Errorf("invalid final reconciliation scope")
	}
	if err := validateFinalMigrationManifest(manifest); err != nil {
		return finalReconciliationResult{}, err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return finalReconciliationResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err = verifyReconciledArchive(ctx, tx, archiveRunID); err != nil {
		return finalReconciliationResult{}, err
	}
	// The controlled apply runs each existing domain reconcile verifier before
	// this read-only aggregate. Those 36 immutable receipts contain the fresh
	// target comparison digests; this command only proves their complete scope.
	result := finalReconciliationResult{
		ManifestDomainCount: len(manifest.Domains),
		VerificationModel:   finalVerificationModel,
	}
	domainsByVersion := finalDomainsByReconciliationScope(manifest)
	byVersion := make(map[string]reconciliationCounts)
	for _, item := range manifest.Domains {
		spec := finalDomainSpecs[item.Domain]
		proof, counts, err := reconcileFinalDomain(ctx, tx, archiveRunID, spec, len(domainsByVersion[spec.ImportVersion]) > 1)
		if err != nil {
			return finalReconciliationResult{}, fmt.Errorf("final reconciliation %s: %w", spec.Domain, err)
		}
		result.Domains = append(result.Domains, proof)
		total := byVersion[spec.ImportVersion]
		total.Selected += counts.Selected
		total.Receipts += counts.Receipts
		total.Imported += counts.Imported
		total.Archived += counts.Archived
		total.Quarantined += counts.Quarantined
		total.Verified += counts.Verified
		byVersion[spec.ImportVersion] = total
	}
	reconciledByVersion := make(map[string]reconciliationCounts, len(domainsByVersion))
	for version := range domainsByVersion {
		reconciled, err := loadReconciliationCounts(ctx, tx, archiveRunID, version)
		if err != nil {
			return finalReconciliationResult{}, fmt.Errorf("final reconciliation receipt %s: %w", version, err)
		}
		reconciledByVersion[version] = reconciled
	}
	if err = validateFinalReconciliationGroups(domainsByVersion, byVersion, reconciledByVersion); err != nil {
		return finalReconciliationResult{}, err
	}
	for version, reconciled := range reconciledByVersion {
		result.ReconciliationGroups = append(result.ReconciliationGroups, finalReconciliationGroup{
			ImportVersion:        version,
			Domains:              domainsByVersion[version],
			SelectedSourceCount:  reconciled.Selected,
			ReceiptCount:         reconciled.Receipts,
			ImportedCount:        reconciled.Imported,
			ArchivedCount:        reconciled.Archived,
			QuarantinedCount:     reconciled.Quarantined,
			VerifiedCount:        reconciled.Verified,
			ReconciliationDigest: hex.EncodeToString(reconciled.Digest),
		})
	}
	identity, err := verifyFinalIdentityMapping(ctx, tx, dm01RunID)
	if err != nil {
		return finalReconciliationResult{}, err
	}
	result.IdentityMapping = identity
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM public.external_effects").Scan(&result.ExternalEffects); err != nil {
		return finalReconciliationResult{}, err
	}
	if result.ExternalEffects != 0 {
		return finalReconciliationResult{}, fmt.Errorf("external_effects must be zero, got %d", result.ExternalEffects)
	}
	sort.Slice(result.Domains, func(left, right int) bool { return result.Domains[left].Domain < result.Domains[right].Domain })
	sort.Slice(result.ReconciliationGroups, func(left, right int) bool {
		return result.ReconciliationGroups[left].ImportVersion < result.ReconciliationGroups[right].ImportVersion
	})
	if err = tx.Commit(ctx); err != nil {
		return finalReconciliationResult{}, err
	}
	return result, nil
}

func verifyReconciledArchive(ctx context.Context, tx pgx.Tx, archiveRunID string) error {
	var ready bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(
SELECT 1 FROM public.v1_archive_reconciliation_receipts receipt
JOIN public.data_migration_runs run USING (run_id)
WHERE receipt.run_id=$1 AND run.phase='reconciled' AND run.adapter_id=$2
AND receipt.source_row_count=receipt.archive_record_count
AND receipt.source_row_count=receipt.terminal_disposition_count
AND receipt.source_table_count=receipt.archived_table_count
)`, archiveRunID, v1archive.DefaultAdapterID).Scan(&ready)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("archive run is not fully reconciled")
	}
	return nil
}

func reconcileFinalDomain(ctx context.Context, tx pgx.Tx, archiveRunID string, spec finalDomainSpec, sharedReconciliationGroup bool) (finalDomainProof, reconciliationCounts, error) {
	counts, digest, err := loadCurrentReceiptCounts(ctx, tx, archiveRunID, spec)
	if err != nil {
		return finalDomainProof{}, reconciliationCounts{}, err
	}
	if counts.Receipts != counts.Verified || counts.Receipts != counts.Imported+counts.Archived+counts.Quarantined || len(digest) != sha256.Size {
		return finalDomainProof{}, reconciliationCounts{}, fmt.Errorf("selected receipts are not terminal and verified")
	}
	if len(spec.SourceTables) > 0 {
		selected, sourceRows, err := sourceCounts(ctx, tx, archiveRunID, spec.SourceTables)
		if err != nil {
			return finalDomainProof{}, reconciliationCounts{}, err
		}
		if selected != sourceRows || selected != counts.Receipts {
			return finalDomainProof{}, reconciliationCounts{}, fmt.Errorf("selected source count does not match archived rows and receipts")
		}
		counts.Selected = selected
	}
	reconciled, err := loadReconciliationCounts(ctx, tx, archiveRunID, spec.ImportVersion)
	if err != nil {
		return finalDomainProof{}, reconciliationCounts{}, err
	}
	if len(spec.SourceTables) == 0 && !sameReceiptCounts(counts, reconciled) {
		return finalDomainProof{}, reconciliationCounts{}, fmt.Errorf("current receipts do not match the domain reconciliation receipt")
	}
	if len(spec.SourceTables) == 0 {
		counts.Selected = reconciled.Selected
	}
	proof := finalDomainProof{
		Domain:                    spec.Domain,
		ImportVersion:             spec.ImportVersion,
		ReconciliationScope:       spec.ImportVersion,
		SharedReconciliationGroup: sharedReconciliationGroup,
		SelectedSourceCount:       counts.Selected,
		ReceiptCount:              counts.Receipts,
		ImportedCount:             counts.Imported,
		ArchivedCount:             counts.Archived,
		QuarantinedCount:          counts.Quarantined,
		VerifiedCount:             counts.Verified,
		ReceiptDigest:             hex.EncodeToString(digest),
	}
	return proof, counts, nil
}

func loadCurrentReceiptCounts(ctx context.Context, tx pgx.Tx, archiveRunID string, spec finalDomainSpec) (reconciliationCounts, []byte, error) {
	query := `SELECT table_id,source_key_digest,payload_digest,disposition,reason,target_domain,target_table,target_id,target_digest,verified
FROM public.v1_domain_import_receipts WHERE import_version=$1 AND archive_run_id=$2`
	args := []any{spec.ImportVersion, archiveRunID}
	if len(spec.SourceTables) > 0 {
		query += " AND table_id=ANY($3::text[])"
		args = append(args, spec.SourceTables)
	}
	query += " ORDER BY table_id,source_key_digest"
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return reconciliationCounts{}, nil, err
	}
	defer rows.Close()
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	var counts reconciliationCounts
	for rows.Next() {
		var tableID, disposition, reason string
		var source, payload, targetDigest []byte
		var targetDomain, targetTable, targetID *string
		var verified bool
		if err = rows.Scan(&tableID, &source, &payload, &disposition, &reason, &targetDomain, &targetTable, &targetID, &targetDigest, &verified); err != nil {
			return reconciliationCounts{}, nil, err
		}
		if len(source) != sha256.Size || len(payload) != sha256.Size {
			return reconciliationCounts{}, nil, fmt.Errorf("invalid receipt digest")
		}
		counts.Receipts++
		if verified {
			counts.Verified++
		}
		switch disposition {
		case "import":
			if targetDomain == nil || targetTable == nil || targetID == nil || len(targetDigest) != sha256.Size {
				return reconciliationCounts{}, nil, fmt.Errorf("import receipt has no target proof")
			}
			counts.Imported++
		case "archive":
			counts.Archived++
		case "quarantine":
			counts.Quarantined++
		default:
			return reconciliationCounts{}, nil, fmt.Errorf("unknown receipt disposition %q", disposition)
		}
		if err = encoder.Encode([]any{tableID, hex.EncodeToString(source), hex.EncodeToString(payload), disposition, reason, stringValue(targetDomain), stringValue(targetTable), stringValue(targetID), hex.EncodeToString(targetDigest), verified}); err != nil {
			return reconciliationCounts{}, nil, err
		}
	}
	if err = rows.Err(); err != nil {
		return reconciliationCounts{}, nil, err
	}
	return counts, hash.Sum(nil), nil
}

func sourceCounts(ctx context.Context, tx pgx.Tx, archiveRunID string, tables []string) (int64, int64, error) {
	var tableCount int
	var selected, sourceRows int64
	err := tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(row_count),0)
FROM public.v1_archive_tables WHERE run_id=$1 AND table_id=ANY($2::text[])`, archiveRunID, tables).Scan(&tableCount, &selected)
	if err != nil {
		return 0, 0, err
	}
	if tableCount != len(tables) {
		return 0, 0, fmt.Errorf("required archive source table is missing")
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM public.v1_archive_records
WHERE run_id=$1 AND adapter_id=$2 AND table_id=ANY($3::text[])`, archiveRunID, v1archive.DefaultAdapterID, tables).Scan(&sourceRows)
	return selected, sourceRows, err
}

func loadReconciliationCounts(ctx context.Context, tx pgx.Tx, archiveRunID, importVersion string) (reconciliationCounts, error) {
	var value reconciliationCounts
	err := tx.QueryRow(ctx, `SELECT selected_source_count,receipt_count,imported_count,archived_count,quarantined_count,verified_count,comparison_digest
FROM public.v1_domain_import_reconciliation_receipts WHERE import_version=$1 AND archive_run_id=$2`, importVersion, archiveRunID).
		Scan(&value.Selected, &value.Receipts, &value.Imported, &value.Archived, &value.Quarantined, &value.Verified, &value.Digest)
	if err != nil {
		return reconciliationCounts{}, err
	}
	if value.Selected != value.Receipts || value.Receipts != value.Imported+value.Archived+value.Quarantined || value.Receipts != value.Verified || len(value.Digest) != sha256.Size {
		return reconciliationCounts{}, fmt.Errorf("stored reconciliation receipt is invalid")
	}
	return value, nil
}

func sameCounts(left, right reconciliationCounts) bool {
	return left.Selected == right.Selected && left.Receipts == right.Receipts && left.Imported == right.Imported && left.Archived == right.Archived && left.Quarantined == right.Quarantined && left.Verified == right.Verified
}

func sameReceiptCounts(left, right reconciliationCounts) bool {
	return left.Receipts == right.Receipts && left.Imported == right.Imported && left.Archived == right.Archived && left.Quarantined == right.Quarantined && left.Verified == right.Verified
}

func validateFinalReconciliationGroups(scopes map[string][]string, actual, reconciled map[string]reconciliationCounts) error {
	if len(scopes) != 36 {
		return fmt.Errorf("final reconciliation requires exactly 36 reconciliation groups")
	}
	for version, domains := range scopes {
		if len(domains) == 0 {
			return fmt.Errorf("final reconciliation scope %s has no manifest domains", version)
		}
		current, found := actual[version]
		if !found {
			return fmt.Errorf("final reconciliation scope %s has no current domain receipts", version)
		}
		stored, found := reconciled[version]
		if !found {
			return fmt.Errorf("final reconciliation receipt %s is missing", version)
		}
		if !sameCounts(current, stored) {
			return fmt.Errorf("final reconciliation receipt %s does not cover exactly its manifest domains", version)
		}
	}
	return nil
}

func verifyFinalIdentityMapping(ctx context.Context, tx pgx.Tx, dm01RunID int64) (finalIdentityProof, error) {
	var ready bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.legacy_contact_identity_import_runs
WHERE id=$1 AND mode='full' AND state='imported')`, dm01RunID).Scan(&ready); err != nil {
		return finalIdentityProof{}, err
	}
	if !ready {
		return finalIdentityProof{}, fmt.Errorf("DM01 full import run is not imported")
	}
	result := finalIdentityProof{DM01RunID: dm01RunID}
	err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE
EXISTS(SELECT 1 FROM public.legacy_contact_identity_import_row_receipts receipt
WHERE receipt.run_id=$1 AND receipt.source_table=mapping.source_table
AND receipt.source_key_hmac=mapping.source_key_hmac AND receipt.payload_hmac=mapping.payload_hmac
AND receipt.disposition='imported')
AND (mapping.staff_id IS NULL OR EXISTS(SELECT 1 FROM public.staff WHERE id=mapping.staff_id))
AND (mapping.customer_id IS NULL OR EXISTS(SELECT 1 FROM public.customers WHERE id=mapping.customer_id AND NOT is_deleted))
AND (mapping.identity_id IS NULL OR EXISTS(SELECT 1 FROM public.identities WHERE id=mapping.identity_id))
) FROM public.legacy_contact_identity_source_mappings mapping
WHERE mapping.last_run_id=$1`, dm01RunID).Scan(&result.MappingCount, &result.VerifiedMapping)
	if err != nil {
		return finalIdentityProof{}, err
	}
	if err = validateFinalIdentityProof(result); err != nil {
		return finalIdentityProof{}, err
	}
	return result, nil
}

func validateFinalIdentityProof(proof finalIdentityProof) error {
	if proof.MappingCount == 0 {
		return fmt.Errorf("DM01 run has no identity mappings")
	}
	if proof.MappingCount != proof.VerifiedMapping {
		return fmt.Errorf("identity mappings are not receipt-bound live targets")
	}
	return nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func finalDomainNames(manifest finalMigrationManifest) []string {
	result := make([]string, 0, len(manifest.Domains))
	for _, domain := range manifest.Domains {
		result = append(result, domain.Domain)
	}
	sort.Strings(result)
	return result
}

func finalDomainsByReconciliationScope(manifest finalMigrationManifest) map[string][]string {
	result := make(map[string][]string)
	for _, domain := range manifest.Domains {
		spec := finalDomainSpecs[domain.Domain]
		result[spec.ImportVersion] = append(result[spec.ImportVersion], domain.Domain)
	}
	for version := range result {
		sort.Strings(result[version])
	}
	return result
}
