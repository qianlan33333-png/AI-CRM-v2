package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1coupon"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	couponBindingsKind    = "bindings"
	couponBindingsTableID = "public/commerce_coupon_product_bindings"
)

// CouponHistoryWriter is the owner-owned write boundary. It records only
// read-only history in the caller transaction and never invokes runtime
// coupon operations.
type CouponHistoryWriter interface {
	ImportDefinition(context.Context, string, [sha256.Size]byte, couponport.HistoricalDefinition) (couponport.HistoricalReceipt, error)
	ImportClaim(context.Context, string, [sha256.Size]byte, couponport.HistoricalClaim) (couponport.HistoricalReceipt, error)
	ImportRedemption(context.Context, string, [sha256.Size]byte, couponport.HistoricalRedemption) (couponport.HistoricalReceipt, error)
}

// CouponReferenceResolver returns only already-verified V2 references. A nil
// customer/order is a valid unresolved historical relation; source IDs are
// never V2 IDs.
type CouponReferenceResolver interface {
	ResolveCouponProduct(context.Context, int64, int64) (int64, error)
	ResolveCouponCustomer(context.Context, string) (*int64, error)
	ResolveCouponOrder(context.Context, int64, string) (*int64, error)
}

// CouponImportResult keeps all four frozen source tables independently
// reconcilable.
type CouponImportResult struct {
	ImportedDefinitions, ImportedBindings, ImportedClaims, ImportedRedemptions             int
	ArchivedDefinitions, ArchivedBindings, ArchivedClaims, ArchivedRedemptions             int
	QuarantinedDefinitions, QuarantinedBindings, QuarantinedClaims, QuarantinedRedemptions int
	ReplayedDefinitions, ReplayedBindings, ReplayedClaims, ReplayedRedemptions             int
}

type CouponImporter struct {
	archive      ArchiveSource
	uow          UnitOfWork
	writer       CouponHistoryWriter
	resolver     CouponReferenceResolver
	journals     map[string]couponTerminalJournal
	archiveRunID string
	actorID      int64
}

func NewCouponImporter(archive ArchiveSource, uow UnitOfWork, writer CouponHistoryWriter, resolver CouponReferenceResolver, journals map[string]*Journal, actorID int64) (*CouponImporter, error) {
	if archive == nil || uow == nil || writer == nil || resolver == nil || actorID < 1 || !validCouponImportJournals(journals) {
		return nil, ErrInvalidScope
	}
	terminals := make(map[string]couponTerminalJournal, len(journals))
	for kind, journal := range journals {
		terminals[kind] = journal
	}
	return newCouponImporter(archive, uow, writer, resolver, terminals, journals[couponDefinitionsKind].scope.ArchiveRunID, actorID)
}

// newCouponImporter keeps the production constructor tied to the concrete
// caller-bound Journals while permitting isolated importer tests.
func newCouponImporter(archive ArchiveSource, uow UnitOfWork, writer CouponHistoryWriter, resolver CouponReferenceResolver, journals map[string]couponTerminalJournal, archiveRunID string, actorID int64) (*CouponImporter, error) {
	if archive == nil || uow == nil || writer == nil || resolver == nil || archiveRunID == "" || actorID < 1 || len(journals) != 4 {
		return nil, ErrInvalidScope
	}
	for _, kind := range []string{couponDefinitionsKind, couponBindingsKind, couponClaimsKind, couponRedemptionsKind} {
		if journals[kind] == nil {
			return nil, ErrInvalidScope
		}
	}
	return &CouponImporter{archive: archive, uow: uow, writer: writer, resolver: resolver, journals: journals, archiveRunID: archiveRunID, actorID: actorID}, nil
}

func validCouponImportJournals(journals map[string]*Journal) bool {
	definitions, bindings := journals[couponDefinitionsKind], journals[couponBindingsKind]
	claims, redemptions := journals[couponClaimsKind], journals[couponRedemptionsKind]
	return len(journals) == 4 && validCouponJournalScope(definitions, couponDefinitionsTableID, "coupons") &&
		validCouponJournalScope(bindings, couponBindingsTableID, "coupon_targets") &&
		validCouponJournalScope(claims, couponClaimsTableID, "coupon_v1_history_claims") &&
		validCouponJournalScope(redemptions, couponRedemptionsTableID, "coupon_v1_history_redemptions") &&
		definitions.scope.ArchiveRunID == bindings.scope.ArchiveRunID && definitions.scope.ArchiveRunID == claims.scope.ArchiveRunID &&
		definitions.scope.ArchiveRunID == redemptions.scope.ArchiveRunID
}

func (importer *CouponImporter) Import(ctx context.Context, archiveRunID string) (CouponImportResult, error) {
	if importer == nil || ctx == nil || archiveRunID == "" || importer.archiveRunID != archiveRunID || len(importer.journals) != 4 {
		return CouponImportResult{}, ErrInvalidScope
	}
	definitions, err := importer.readRows(ctx, archiveRunID, couponDefinitionsTableID)
	if err != nil {
		return CouponImportResult{}, err
	}
	bindings, err := importer.readRows(ctx, archiveRunID, couponBindingsTableID)
	if err != nil {
		return CouponImportResult{}, err
	}
	claims, err := importer.readRows(ctx, archiveRunID, couponClaimsTableID)
	if err != nil {
		return CouponImportResult{}, err
	}
	redemptions, err := importer.readRows(ctx, archiveRunID, couponRedemptionsTableID)
	if err != nil {
		return CouponImportResult{}, err
	}
	history := v1coupon.AdaptHistory(couponPayloads(definitions), couponPayloads(bindings), couponPayloads(claims), couponPayloads(redemptions))
	if len(history.Coupons) != len(definitions) || len(history.Bindings) != len(bindings) || len(history.Claims) != len(claims) || len(history.Redemptions) != len(redemptions) {
		return CouponImportResult{}, ErrConflict
	}

	result := CouponImportResult{}
	bindingIndexes := make(map[int64][]int, len(bindings))
	for index, decision := range history.Bindings {
		sourceID := decision.CouponSourceID
		if decision.Fact != nil {
			sourceID = decision.Fact.CouponSourceID
		}
		if sourceID > 0 {
			bindingIndexes[sourceID] = append(bindingIndexes[sourceID], index)
		}
	}
	processedBindings := make([]bool, len(bindings))
	definitionTargets := make(map[int64]int64, len(definitions))
	for index, decision := range history.Coupons {
		var sourceID int64
		if decision.Fact != nil {
			sourceID = decision.Fact.SourceID
		}
		indexes := bindingIndexes[sourceID]
		for _, bindingIndex := range indexes {
			processedBindings[bindingIndex] = true
		}
		if err := importer.importDefinitionGroup(ctx, definitions[index], decision, bindings, history.Bindings, indexes, definitionTargets, &result); err != nil {
			return CouponImportResult{}, err
		}
	}
	for index, row := range bindings {
		if processedBindings[index] {
			continue
		}
		if err := importer.recordDecision(ctx, couponBindingsKind, row, dispositionForBinding(history.Bindings[index]), reasonForBinding(history.Bindings[index], "coupon_binding_parent_unavailable"), &result); err != nil {
			return CouponImportResult{}, err
		}
	}

	claimTargets := make(map[int64]int64, len(claims))
	for index, decision := range history.Claims {
		if err := importer.importClaim(ctx, claims[index], decision, definitionTargets, claimTargets, &result); err != nil {
			return CouponImportResult{}, err
		}
	}
	for index, decision := range history.Redemptions {
		if err := importer.importRedemption(ctx, redemptions[index], decision, claimTargets, &result); err != nil {
			return CouponImportResult{}, err
		}
	}
	return result, nil
}

type couponArchiveRow struct {
	archive         v1archive.ArchivedRow
	redactionReason string
}

func (importer *CouponImporter) readRows(ctx context.Context, archiveRunID, tableID string) ([]couponArchiveRow, error) {
	rows := make([]couponArchiveRow, 0)
	err := importer.archive.EachTableRow(ctx, archiveRunID, tableID, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != tableID || row.SourceOrdinal < 1 || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
			return ErrConflict
		}
		rows = append(rows, couponArchiveRow{archive: row, redactionReason: couponRedactionReason(tableID, row)})
		return nil
	})
	return rows, err
}

func couponPayloads(rows []couponArchiveRow) []json.RawMessage {
	values := make([]json.RawMessage, len(rows))
	for index := range rows {
		values[index] = rows[index].archive.Payload
	}
	return values
}

func couponRedactionReason(tableID string, row v1archive.ArchivedRow) string {
	var fields []string
	switch tableID {
	case couponDefinitionsTableID:
		fields = []string{"id", "tenant_id", "name", "discount_amount_total", "currency", "status", "total_issue_limit", "per_user_issue_limit", "issued_count", "claim_starts_at", "claim_ends_at", "validity_mode", "use_starts_at", "use_ends_at", "relative_validity_days", "instructions", "first_claim_at", "created_at", "updated_at"}
	case couponBindingsTableID:
		fields = []string{"id", "tenant_id", "coupon_id", "trade_product_id", "created_at"}
	case couponClaimsTableID:
		fields = []string{"id", "tenant_id", "coupon_id", "claim_no", "discount_amount_total", "currency", "valid_from", "valid_until", "status", "claimed_at", "reserved_at", "consumed_at", "expired_at", "created_at", "updated_at"}
	case couponRedemptionsTableID:
		fields = []string{"id", "tenant_id", "claim_id", "order_id", "out_trade_no", "status", "original_amount_total", "discount_amount_total", "payable_amount_total", "currency", "reserved_until", "release_reason", "reserved_at", "consumed_at", "released_at", "created_at", "updated_at"}
	default:
		return ""
	}
	for _, field := range fields {
		if v1archive.IsRedacted(row, field) {
			return "coupon_history_business_field_redacted"
		}
	}
	return ""
}

func (importer *CouponImporter) importDefinitionGroup(ctx context.Context, definition couponArchiveRow, decision v1coupon.CouponResult, bindings []couponArchiveRow, bindingDecisions []v1coupon.BindingResult, indexes []int, targets map[int64]int64, result *CouponImportResult) error {
	if definition.redactionReason != "" {
		return importer.recordDefinitionGroup(ctx, definition, bindings, indexes, "quarantine", definition.redactionReason, result)
	}
	if decision.Disposition != v1coupon.DispositionCandidate || decision.Fact == nil {
		return importer.recordDefinitionGroup(ctx, definition, bindings, indexes, string(decision.Disposition), reasonOr(decision.Reason, "coupon_definition_invalid"), result)
	}
	resolved, reason, err := importer.resolveDefinition(ctx, *decision.Fact, bindings, bindingDecisions, indexes)
	if err != nil {
		return err
	}
	if reason != "" {
		return importer.recordDefinitionGroup(ctx, definition, bindings, indexes, "quarantine", reason, result)
	}
	replayed, targetID := false, int64(0)
	err = importer.uow.Within(ctx, func(tx context.Context) error {
		replayed, targetID = false, 0
		receipt, writeErr := importer.writer.ImportDefinition(tx, SourceIdentifier(definition.archive.SourceKeyHMAC), definition.archive.PayloadHMAC, resolved.definition)
		if writeErr != nil {
			return writeErr
		}
		if !sameCouponWriterReceipt(receipt, definition.archive) {
			return ErrConflict
		}
		replayed = receipt.Replayed
		targetID = receipt.TargetID
		for position := range resolved.bindings {
			binding := &resolved.bindings[position]
			targetID := HistoricalCouponBindingTargetID(receipt.TargetID, int32(position))
			targetDigest := HistoricalCouponBindingTargetDigest(receipt.TargetID, int32(position), binding.productID)
			bindingReplayed, recordErr := importer.recordTerminal(tx, couponBindingsKind, binding.row.archive, TerminalReceipt{
				SourceKeyDigest: binding.row.archive.SourceKeyHMAC, PayloadDigest: binding.row.archive.PayloadHMAC,
				Disposition: "import", TargetID: targetID, TargetDigest: targetDigest,
			})
			if recordErr != nil {
				return recordErr
			}
			binding.replayed = bindingReplayed
		}
		return nil
	})
	if errors.Is(err, couponport.ErrHistoryInvalid) {
		return importer.recordDefinitionGroup(ctx, definition, bindings, indexes, "quarantine", "coupon_definition_target_invalid", result)
	}
	if err != nil {
		return err
	}
	targets[decision.Fact.SourceID] = targetID
	result.ImportedDefinitions++
	if replayed {
		result.ReplayedDefinitions++
	}
	for _, binding := range resolved.bindings {
		result.ImportedBindings++
		if binding.replayed {
			result.ReplayedBindings++
		}
	}
	return nil
}

type resolvedCouponBinding struct {
	row       couponArchiveRow
	fact      v1coupon.BindingFact
	productID int64
	replayed  bool
}

type resolvedCouponDefinition struct {
	definition couponport.HistoricalDefinition
	bindings   []resolvedCouponBinding
}

func (importer *CouponImporter) resolveDefinition(ctx context.Context, fact v1coupon.CouponDefinitionFact, bindings []couponArchiveRow, decisions []v1coupon.BindingResult, indexes []int) (resolvedCouponDefinition, string, error) {
	if len(indexes) == 0 {
		return resolvedCouponDefinition{}, "coupon_binding_unresolved", nil
	}
	resolved := make([]resolvedCouponBinding, 0, len(indexes))
	for _, index := range indexes {
		if bindings[index].redactionReason != "" {
			return resolvedCouponDefinition{}, bindings[index].redactionReason, nil
		}
		decision := decisions[index]
		if decision.Disposition != v1coupon.DispositionCandidate || decision.Fact == nil || decision.Fact.CouponSourceID != fact.SourceID {
			return resolvedCouponDefinition{}, reasonForBinding(decision, "coupon_binding_unresolved"), nil
		}
		productID, err := importer.resolver.ResolveCouponProduct(ctx, decision.Fact.ProductSourceID, fact.DiscountAmountMinor)
		if err != nil {
			return resolvedCouponDefinition{}, "", err
		}
		if productID < 1 {
			return resolvedCouponDefinition{}, "coupon_binding_product_unresolved", nil
		}
		resolved = append(resolved, resolvedCouponBinding{row: bindings[index], fact: *decision.Fact, productID: productID})
	}
	sort.SliceStable(resolved, func(left, right int) bool { return resolved[left].fact.SourceID < resolved[right].fact.SourceID })
	refs := make([]string, len(resolved))
	seen := make(map[int64]struct{}, len(resolved))
	for index, binding := range resolved {
		if _, found := seen[binding.productID]; found {
			return resolvedCouponDefinition{}, "coupon_binding_target_ambiguous", nil
		}
		seen[binding.productID] = struct{}{}
		refs[index] = "standard_product:" + strconv.FormatInt(binding.productID, 10)
	}
	definition := couponport.HistoricalDefinition{Coupon: couponport.Coupon{
		Name: fact.Name, DiscountAmountTotal: fact.DiscountAmountMinor, Currency: fact.Currency, Status: "archived", AvailabilityStatus: "archived",
		TotalIssueLimit: fact.TotalIssueLimit, PerUserIssueLimit: fact.PerUserIssueLimit, IssuedCount: fact.IssuedCount,
		ClaimStartsAt: couponTime(fact.ClaimStartsAt), ClaimEndsAt: couponTime(fact.ClaimEndsAt), ValidityMode: couponport.ValidityMode(fact.ValidityMode),
		UseStartsAt: couponTimePointer(fact.UseStartsAt), UseEndsAt: couponTimePointer(fact.UseEndsAt), RelativeValidityDays: fact.RelativeValidityDays,
		Instructions: fact.Instructions, TargetRefs: refs, CreatedBy: importer.actorID, UpdatedBy: importer.actorID, Version: 1,
		CreatedAt: couponTime(fact.CreatedAt), UpdatedAt: couponTime(fact.UpdatedAt), HistoryOnly: true,
	}, SourceCouponID: fact.SourceID, OriginalStatus: fact.OriginalStatus, FirstClaimAt: couponTimePointer(fact.FirstClaimAt)}
	return resolvedCouponDefinition{definition: definition, bindings: resolved}, "", nil
}

func (importer *CouponImporter) recordDefinitionGroup(ctx context.Context, definition couponArchiveRow, bindings []couponArchiveRow, indexes []int, disposition, reason string, result *CouponImportResult) error {
	if disposition != "archive" && disposition != "quarantine" || reason == "" {
		return ErrConflict
	}
	definitionReplayed := false
	bindingReplayed := make([]bool, len(indexes))
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		definitionReplayed = false
		for index := range bindingReplayed {
			bindingReplayed[index] = false
		}
		var err error
		definitionReplayed, err = importer.recordTerminal(tx, couponDefinitionsKind, definition.archive, terminalDecision(definition.archive, disposition, reason))
		if err != nil {
			return err
		}
		for offset, index := range indexes {
			bindingReplayed[offset], err = importer.recordTerminal(tx, couponBindingsKind, bindings[index].archive, terminalDecision(bindings[index].archive, disposition, reason))
			recordErr := err
			if recordErr != nil {
				return recordErr
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	result.add(couponDefinitionsKind, disposition, definitionReplayed)
	for _, replayed := range bindingReplayed {
		result.add(couponBindingsKind, disposition, replayed)
	}
	return nil
}

func (importer *CouponImporter) importClaim(ctx context.Context, row couponArchiveRow, decision v1coupon.ClaimResult, definitions, claims map[int64]int64, result *CouponImportResult) error {
	if row.redactionReason != "" {
		return importer.recordDecision(ctx, couponClaimsKind, row, "quarantine", row.redactionReason, result)
	}
	if decision.Disposition != v1coupon.DispositionCandidate || decision.Fact == nil {
		return importer.recordDecision(ctx, couponClaimsKind, row, string(decision.Disposition), reasonOr(decision.Reason, "coupon_claim_invalid"), result)
	}
	couponID, found := definitions[decision.Fact.CouponSourceID]
	if !found || couponID < 1 {
		return importer.recordDecision(ctx, couponClaimsKind, row, "quarantine", "coupon_claim_parent_coupon_unavailable", result)
	}
	var customerID *int64
	var err error
	if !v1archive.IsRedacted(row.archive, "unionid") && decision.Fact.UnionID != "" {
		customerID, err = importer.resolver.ResolveCouponCustomer(ctx, decision.Fact.UnionID)
		if err != nil {
			return err
		}
		if invalidCouponReferenceID(customerID) {
			return ErrConflict
		}
	}
	value := couponport.HistoricalClaim{SourceClaimID: decision.Fact.SourceID, SourceCouponID: decision.Fact.CouponSourceID, CouponID: couponID, CustomerID: customerID,
		ClaimNo: decision.Fact.ClaimNumber, Status: decision.Fact.OriginalStatus, DiscountAmountTotal: decision.Fact.DiscountAmountMinor, Currency: decision.Fact.Currency,
		ValidFrom: couponTime(decision.Fact.ValidFrom), ValidUntil: couponTime(decision.Fact.ValidUntil), ClaimedAt: couponTime(decision.Fact.ClaimedAt),
		ReservedAt: couponTimePointer(decision.Fact.ReservedAt), ConsumedAt: couponTimePointer(decision.Fact.ConsumedAt), ExpiredAt: couponTimePointer(decision.Fact.ExpiredAt),
		CreatedAt: couponTime(decision.Fact.CreatedAt), UpdatedAt: couponTime(decision.Fact.UpdatedAt)}
	replayed, targetID := false, int64(0)
	err = importer.uow.Within(ctx, func(tx context.Context) error {
		replayed, targetID = false, 0
		receipt, writeErr := importer.writer.ImportClaim(tx, SourceIdentifier(row.archive.SourceKeyHMAC), row.archive.PayloadHMAC, value)
		if writeErr != nil {
			return writeErr
		}
		if !sameCouponWriterReceipt(receipt, row.archive) {
			return ErrConflict
		}
		targetID, replayed = receipt.TargetID, receipt.Replayed
		return nil
	})
	if errors.Is(err, couponport.ErrHistoryInvalid) {
		return importer.recordDecision(ctx, couponClaimsKind, row, "quarantine", "coupon_claim_target_invalid", result)
	}
	if err != nil {
		return err
	}
	claims[decision.Fact.SourceID] = targetID
	result.ImportedClaims++
	if replayed {
		result.ReplayedClaims++
	}
	return nil
}

func (importer *CouponImporter) importRedemption(ctx context.Context, row couponArchiveRow, decision v1coupon.RedemptionResult, claims map[int64]int64, result *CouponImportResult) error {
	if row.redactionReason != "" {
		return importer.recordDecision(ctx, couponRedemptionsKind, row, "quarantine", row.redactionReason, result)
	}
	if decision.Disposition != v1coupon.DispositionCandidate || decision.Fact == nil {
		return importer.recordDecision(ctx, couponRedemptionsKind, row, string(decision.Disposition), reasonOr(decision.Reason, "coupon_redemption_invalid"), result)
	}
	claimID, found := claims[decision.Fact.ClaimSourceID]
	if !found || claimID < 1 {
		return importer.recordDecision(ctx, couponRedemptionsKind, row, "quarantine", "coupon_redemption_parent_claim_unavailable", result)
	}
	orderID, err := importer.resolver.ResolveCouponOrder(ctx, decision.Fact.OrderSourceID, decision.Fact.OutTradeNo)
	if err != nil {
		return err
	}
	if invalidCouponReferenceID(orderID) {
		return ErrConflict
	}
	value := couponport.HistoricalRedemption{SourceRedemptionID: decision.Fact.SourceID, SourceClaimID: decision.Fact.ClaimSourceID, SourceOrderID: decision.Fact.OrderSourceID,
		ClaimHistoryID: claimID, OrderID: orderID, OutTradeNo: decision.Fact.OutTradeNo, Status: decision.Fact.OriginalStatus,
		OriginalAmountTotal: decision.Fact.OriginalAmountMinor, DiscountAmountTotal: decision.Fact.DiscountAmountMinor, PayableAmountTotal: decision.Fact.PayableAmountMinor,
		Currency: decision.Fact.Currency, ReservedUntil: couponTime(decision.Fact.ReservedUntil), ReleaseReason: decision.Fact.ReleaseReason, ReservedAt: couponTime(decision.Fact.ReservedAt),
		ConsumedAt: couponTimePointer(decision.Fact.ConsumedAt), ReleasedAt: couponTimePointer(decision.Fact.ReleasedAt), CreatedAt: couponTime(decision.Fact.CreatedAt), UpdatedAt: couponTime(decision.Fact.UpdatedAt)}
	replayed := false
	err = importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		receipt, writeErr := importer.writer.ImportRedemption(tx, SourceIdentifier(row.archive.SourceKeyHMAC), row.archive.PayloadHMAC, value)
		if writeErr != nil {
			return writeErr
		}
		if !sameCouponWriterReceipt(receipt, row.archive) {
			return ErrConflict
		}
		replayed = receipt.Replayed
		return nil
	})
	if errors.Is(err, couponport.ErrHistoryInvalid) {
		return importer.recordDecision(ctx, couponRedemptionsKind, row, "quarantine", "coupon_redemption_target_invalid", result)
	}
	if err != nil {
		return err
	}
	result.ImportedRedemptions++
	if replayed {
		result.ReplayedRedemptions++
	}
	return nil
}

func (importer *CouponImporter) recordDecision(ctx context.Context, kind string, row couponArchiveRow, disposition, reason string, result *CouponImportResult) error {
	if disposition != "archive" && disposition != "quarantine" || reason == "" {
		return ErrConflict
	}
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		var err error
		replayed, err = importer.recordTerminal(tx, kind, row.archive, terminalDecision(row.archive, disposition, reason))
		return err
	})
	if err != nil {
		return err
	}
	result.add(kind, disposition, replayed)
	return nil
}

func terminalDecision(row v1archive.ArchivedRow, disposition, reason string) TerminalReceipt {
	return TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: disposition, Reason: reason}
}

func (importer *CouponImporter) recordTerminal(ctx context.Context, kind string, row v1archive.ArchivedRow, want TerminalReceipt) (bool, error) {
	journal := importer.journals[kind]
	if journal == nil || want.SourceKeyDigest != row.SourceKeyHMAC || want.PayloadDigest != row.PayloadHMAC {
		return false, ErrConflict
	}
	found, exists, err := journal.LoadTerminal(ctx, SourceIdentifier(row.SourceKeyHMAC))
	if err != nil {
		return false, err
	}
	if exists {
		if !sameCouponImportTerminal(found, want) {
			return false, ErrConflict
		}
		return true, nil
	}
	if err = journal.Record(ctx, want); err != nil {
		return false, err
	}
	return false, nil
}

func sameCouponImportTerminal(left, right TerminalReceipt) bool {
	return left.SourceKeyDigest == right.SourceKeyDigest && left.PayloadDigest == right.PayloadDigest &&
		left.Disposition == right.Disposition && left.Reason == right.Reason && left.TargetID == right.TargetID &&
		left.TargetDigest == right.TargetDigest && len(left.Metadata) == 0 && len(right.Metadata) == 0
}

func sameCouponWriterReceipt(receipt couponport.HistoricalReceipt, row v1archive.ArchivedRow) bool {
	return receipt.SourceIdentifier == SourceIdentifier(row.SourceKeyHMAC) && receipt.PayloadDigest == row.PayloadHMAC && receipt.TargetID > 0 && receipt.TargetDigest != ([sha256.Size]byte{})
}

// HistoricalCouponBindingTargetID identifies one target relation without ever
// using the V1 binding ID as a V2 primary key.
func HistoricalCouponBindingTargetID(couponID int64, position int32) string {
	if couponID < 1 || position < 0 {
		return ""
	}
	return strconv.FormatInt(couponID, 10) + ":" + strconv.FormatInt(int64(position), 10)
}

// HistoricalCouponBindingTargetDigest is the stable V2 target tuple used by
// the receipt and later reconciliation.
func HistoricalCouponBindingTargetDigest(couponID int64, position int32, productID int64) [sha256.Size]byte {
	targetID := HistoricalCouponBindingTargetID(couponID, position)
	if targetID == "" || productID < 1 {
		return [sha256.Size]byte{}
	}
	payload, _ := json.Marshal(struct {
		Kind, TargetID, TargetRef     string
		CouponID, Position, ProductID int64
	}{Kind: couponBindingsKind, TargetID: targetID, TargetRef: "standard_product:" + strconv.FormatInt(productID, 10), CouponID: couponID, Position: int64(position), ProductID: productID})
	return sha256.Sum256(append([]byte("v1-coupon-binding-target/v1\x00"), payload...))
}

func dispositionForBinding(value v1coupon.BindingResult) string {
	if value.Disposition == v1coupon.DispositionArchive {
		return "archive"
	}
	return "quarantine"
}

func reasonForBinding(value v1coupon.BindingResult, fallback string) string {
	return reasonOr(value.Reason, fallback)
}

func reasonOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func invalidCouponReferenceID(value *int64) bool { return value != nil && *value < 1 }

func couponTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func couponTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	converted := couponTime(*value)
	return &converted
}

func (result *CouponImportResult) add(kind, disposition string, replayed bool) {
	switch kind {
	case couponDefinitionsKind:
		if disposition == "archive" {
			result.ArchivedDefinitions++
		} else {
			result.QuarantinedDefinitions++
		}
		if replayed {
			result.ReplayedDefinitions++
		}
	case couponBindingsKind:
		if disposition == "archive" {
			result.ArchivedBindings++
		} else {
			result.QuarantinedBindings++
		}
		if replayed {
			result.ReplayedBindings++
		}
	case couponClaimsKind:
		if disposition == "archive" {
			result.ArchivedClaims++
		} else {
			result.QuarantinedClaims++
		}
		if replayed {
			result.ReplayedClaims++
		}
	case couponRedemptionsKind:
		if disposition == "archive" {
			result.ArchivedRedemptions++
		} else {
			result.QuarantinedRedemptions++
		}
		if replayed {
			result.ReplayedRedemptions++
		}
	}
}
