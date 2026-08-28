// Package v1statictail keeps the remaining V1 static tables as non-executable
// historical facts. It has no target store, current-ID resolution, or runtime
// operation-cycle/group-invite behaviour.
package v1statictail

import (
	"crypto/sha256"
	"encoding/json"
	"time"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

// OpaqueDigest identifies source material retained by the sealed archive. It
// is deliberately neither recoverable source content nor an executable input.
type OpaqueDigest [sha256.Size]byte

// SourceRecord binds a decoded candidate to the immutable archive payload.
// It intentionally carries no recoverable source key or source content beyond
// the payload supplied to the private adapter.
type SourceRecord struct {
	Payload     json.RawMessage
	PayloadHMAC OpaqueDigest
}

type GroupInviteFact struct {
	SourceID             int64
	Name                 string
	Title                string
	Description          string
	OriginalState        string
	OriginalAutoCreate   bool
	RoomBaseName         string
	RoomBaseSourceID     *int64
	OriginalEnabled      bool
	OriginalBindingState string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	SealedSourceDigest   OpaqueDigest
}

// ProductSourceID and ImageSourceID remain V1 references. They are never
// translated into current products or product_images.
type ProductPageSliceFact struct {
	SourceID           int64
	ProductSourceID    int64
	ImageSourceID      int64
	SortOrder          int64
	OriginalEnabled    bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
	SealedSourceDigest OpaqueDigest
}

type OperationCycleStrategyFact struct {
	SourceID           int64
	StrategyKey        string
	Title              string
	Description        string
	Cadence            string
	Timezone           string
	OriginalStatus     string
	CurrentVersion     int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	SealedSourceDigest OpaqueDigest
}

type OperationCycleVersionFact struct {
	SourceID           int64
	StrategySourceID   int64
	Version            int64
	Label              string
	Objective          string
	VersionHash        string
	EffectiveFrom      *time.Time
	OriginalGovernance string
	ConfirmedAt        *time.Time
	OperationSkillHash string
	CreatedAt          time.Time
	SealedSourceDigest OpaqueDigest
}

type OperationCycleDocumentFact struct {
	SourceID                    int64
	StrategyVersionSourceID     int64
	SchemaVersion               string
	ExecutionGuideSHA256        string
	ExecutionGuideGeneratedAt   *time.Time
	CopyGuideSHA256             string
	CopyGuideGeneratedAt        *time.Time
	MeasurementGuideSHA256      string
	MeasurementGuideGeneratedAt *time.Time
	DocumentPackHash            string
	CreatedAt                   time.Time
	SealedSourceDigest          OpaqueDigest
}

type GroupInviteResult struct {
	Disposition Disposition
	Reason      string
	Fact        *GroupInviteFact
}

type ProductPageSliceResult struct {
	Disposition Disposition
	Reason      string
	Fact        *ProductPageSliceFact
}

type OperationCycleStrategyResult struct {
	Disposition Disposition
	Reason      string
	Fact        *OperationCycleStrategyFact
}

type OperationCycleVersionResult struct {
	Disposition Disposition
	Reason      string
	Fact        *OperationCycleVersionFact
}

type OperationCycleDocumentResult struct {
	Disposition Disposition
	Reason      string
	Fact        *OperationCycleDocumentFact
}

// History keeps each source table separate and conserves row order/count.
type History struct {
	GroupInvites []GroupInviteResult
	PageSlices   []ProductPageSliceResult
	Strategies   []OperationCycleStrategyResult
	Versions     []OperationCycleVersionResult
	Documents    []OperationCycleDocumentResult
}

// SourceCount and TerminalCount make row-level conservation explicit for an
// archive caller: every source row becomes exactly one candidate or quarantine.
func (h History) SourceCount() int {
	return len(h.GroupInvites) + len(h.PageSlices) + len(h.Strategies) + len(h.Versions) + len(h.Documents)
}

func (h History) TerminalCount() int {
	count := 0
	for _, row := range h.GroupInvites {
		if row.Disposition == DispositionCandidate || row.Disposition == DispositionQuarantine {
			count++
		}
	}
	for _, row := range h.PageSlices {
		if row.Disposition == DispositionCandidate || row.Disposition == DispositionQuarantine {
			count++
		}
	}
	for _, row := range h.Strategies {
		if row.Disposition == DispositionCandidate || row.Disposition == DispositionQuarantine {
			count++
		}
	}
	for _, row := range h.Versions {
		if row.Disposition == DispositionCandidate || row.Disposition == DispositionQuarantine {
			count++
		}
	}
	for _, row := range h.Documents {
		if row.Disposition == DispositionCandidate || row.Disposition == DispositionQuarantine {
			count++
		}
	}
	return count
}

func AdaptHistory(groupInvites, pageSlices, strategies, versions, documents []SourceRecord) History {
	history := History{
		GroupInvites: make([]GroupInviteResult, len(groupInvites)),
		PageSlices:   make([]ProductPageSliceResult, len(pageSlices)),
		Strategies:   make([]OperationCycleStrategyResult, len(strategies)),
		Versions:     make([]OperationCycleVersionResult, len(versions)),
		Documents:    make([]OperationCycleDocumentResult, len(documents)),
	}
	for i, row := range groupInvites {
		history.GroupInvites[i] = adaptGroupInvite(row)
	}
	quarantineDuplicateGroupInvites(history.GroupInvites)
	for i, row := range pageSlices {
		history.PageSlices[i] = adaptPageSlice(row)
	}
	quarantineDuplicatePageSlices(history.PageSlices)
	for i, row := range strategies {
		history.Strategies[i] = adaptStrategy(row)
	}
	quarantineDuplicateStrategies(history.Strategies)
	for i, row := range versions {
		history.Versions[i] = adaptVersion(row)
	}
	quarantineDuplicateVersions(history.Versions)

	strategyIDs := candidateStrategyIDs(history.Strategies)
	for i := range history.Versions {
		if fact := history.Versions[i].Fact; fact != nil && !strategyIDs[fact.StrategySourceID] {
			quarantineVersion(&history.Versions[i], "operation_cycle_strategy_version_strategy_unresolved")
		}
	}
	for i := range history.Strategies {
		fact := history.Strategies[i].Fact
		if fact == nil {
			continue
		}
		if !hasCandidateVersion(history.Versions, fact.SourceID, fact.CurrentVersion) {
			quarantineStrategy(&history.Strategies[i], "operation_cycle_strategy_current_version_unresolved")
		}
	}
	strategyIDs = candidateStrategyIDs(history.Strategies)
	for i := range history.Versions {
		if fact := history.Versions[i].Fact; fact != nil && !strategyIDs[fact.StrategySourceID] {
			quarantineVersion(&history.Versions[i], "operation_cycle_strategy_version_strategy_unresolved")
		}
	}
	for i, row := range documents {
		history.Documents[i] = adaptDocument(row)
	}
	quarantineDuplicateDocuments(history.Documents)
	versionIDs := candidateVersionIDs(history.Versions)
	for i := range history.Documents {
		if fact := history.Documents[i].Fact; fact != nil && !versionIDs[fact.StrategyVersionSourceID] {
			quarantineDocument(&history.Documents[i], "operation_cycle_strategy_version_document_version_unresolved")
		}
	}
	return history
}

type groupInviteJSON struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	PicURL         string          `json:"pic_url"`
	JoinURL        string          `json:"join_url"`
	ConfigID       string          `json:"config_id"`
	State          string          `json:"state"`
	ChatIDList     json.RawMessage `json:"chat_id_list"`
	AutoCreateRoom bool            `json:"auto_create_room"`
	RoomBaseName   string          `json:"room_base_name"`
	RoomBaseID     *int64          `json:"room_base_id"`
	Enabled        bool            `json:"enabled"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	ChatID         string          `json:"chat_id"`
	BindingStatus  string          `json:"binding_status"`
}

type pageSliceJSON struct {
	ID             int64     `json:"id"`
	ProductID      int64     `json:"product_id"`
	ImageLibraryID int64     `json:"image_library_id"`
	SortOrder      int64     `json:"sort_order"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type strategyJSON struct {
	ID             int64     `json:"id"`
	TenantID       string    `json:"tenant_id"`
	StrategyKey    string    `json:"strategy_key"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Cadence        string    `json:"cadence"`
	Timezone       string    `json:"timezone"`
	Status         string    `json:"status"`
	CurrentVersion int64     `json:"current_version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type versionJSON struct {
	ID                 int64           `json:"id"`
	StrategyID         int64           `json:"strategy_id"`
	Version            int64           `json:"version"`
	Label              string          `json:"label"`
	Objective          string          `json:"objective"`
	DefinitionJSON     json.RawMessage `json:"definition_json"`
	VersionHash        string          `json:"version_hash"`
	EffectiveFrom      *time.Time      `json:"effective_from"`
	CreatedAt          time.Time       `json:"created_at"`
	GovernanceStatus   string          `json:"governance_status"`
	ConfirmedBy        string          `json:"confirmed_by"`
	ConfirmedAt        *time.Time      `json:"confirmed_at"`
	ConfirmationNote   string          `json:"confirmation_note"`
	OperationSkillJSON json.RawMessage `json:"operation_skill_json"`
	OperationSkillHash string          `json:"operation_skill_hash"`
}

type documentJSON struct {
	ID                          int64           `json:"id"`
	StrategyVersionID           int64           `json:"strategy_version_id"`
	SchemaVersion               string          `json:"schema_version"`
	ExecutionGuideMarkdown      string          `json:"execution_guide_markdown"`
	ExecutionGuideSHA256        string          `json:"execution_guide_sha256"`
	ExecutionGuideGeneratedAt   *time.Time      `json:"execution_guide_generated_at"`
	ExecutionGuideSource        string          `json:"execution_guide_source"`
	CopyGuideMarkdown           string          `json:"copy_guide_markdown"`
	CopyGuideSHA256             string          `json:"copy_guide_sha256"`
	CopyGuideGeneratedAt        *time.Time      `json:"copy_guide_generated_at"`
	CopyGuideSource             string          `json:"copy_guide_source"`
	MeasurementGuideMarkdown    string          `json:"measurement_guide_markdown"`
	MeasurementGuideSHA256      string          `json:"measurement_guide_sha256"`
	MeasurementGuideGeneratedAt *time.Time      `json:"measurement_guide_generated_at"`
	MeasurementGuideSource      string          `json:"measurement_guide_source"`
	ExecutionContractJSON       json.RawMessage `json:"execution_contract_json"`
	DocumentPackHash            string          `json:"document_pack_hash"`
	CreatedAt                   time.Time       `json:"created_at"`
}

func adaptGroupInvite(row SourceRecord) GroupInviteResult {
	var source groupInviteJSON
	if !decode(row, &source, "id", "name", "title", "description", "pic_url", "join_url", "config_id", "state", "chat_id_list", "auto_create_room", "room_base_name", "enabled", "created_at", "updated_at", "chat_id", "binding_status") || !present(row.Payload, "room_base_id") {
		return GroupInviteResult{Disposition: DispositionQuarantine, Reason: "group_invite_library_shape_invalid"}
	}
	return GroupInviteResult{Disposition: DispositionCandidate, Fact: &GroupInviteFact{SourceID: source.ID, Name: source.Name, Title: source.Title, Description: source.Description, OriginalState: source.State, OriginalAutoCreate: source.AutoCreateRoom, RoomBaseName: source.RoomBaseName, RoomBaseSourceID: source.RoomBaseID, OriginalEnabled: source.Enabled, OriginalBindingState: source.BindingStatus, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt, SealedSourceDigest: row.PayloadHMAC}}
}

func adaptPageSlice(row SourceRecord) ProductPageSliceResult {
	var source pageSliceJSON
	if !decode(row, &source, "id", "product_id", "image_library_id", "sort_order", "enabled", "created_at", "updated_at") {
		return ProductPageSliceResult{Disposition: DispositionQuarantine, Reason: "wechat_pay_product_page_slice_shape_invalid"}
	}
	return ProductPageSliceResult{Disposition: DispositionCandidate, Fact: &ProductPageSliceFact{SourceID: source.ID, ProductSourceID: source.ProductID, ImageSourceID: source.ImageLibraryID, SortOrder: source.SortOrder, OriginalEnabled: source.Enabled, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt, SealedSourceDigest: row.PayloadHMAC}}
}

func adaptStrategy(row SourceRecord) OperationCycleStrategyResult {
	var source strategyJSON
	if !decode(row, &source, "id", "tenant_id", "strategy_key", "title", "description", "cadence", "timezone", "status", "current_version", "created_at", "updated_at") {
		return OperationCycleStrategyResult{Disposition: DispositionQuarantine, Reason: "operation_cycle_strategy_shape_invalid"}
	}
	return OperationCycleStrategyResult{Disposition: DispositionCandidate, Fact: &OperationCycleStrategyFact{SourceID: source.ID, StrategyKey: source.StrategyKey, Title: source.Title, Description: source.Description, Cadence: source.Cadence, Timezone: source.Timezone, OriginalStatus: source.Status, CurrentVersion: source.CurrentVersion, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt, SealedSourceDigest: row.PayloadHMAC}}
}

func adaptVersion(row SourceRecord) OperationCycleVersionResult {
	var source versionJSON
	if !decode(row, &source, "id", "strategy_id", "version", "label", "objective", "definition_json", "version_hash", "created_at", "governance_status", "confirmed_by", "confirmation_note", "operation_skill_json", "operation_skill_hash") || !present(row.Payload, "effective_from", "confirmed_at") {
		return OperationCycleVersionResult{Disposition: DispositionQuarantine, Reason: "operation_cycle_strategy_version_shape_invalid"}
	}
	return OperationCycleVersionResult{Disposition: DispositionCandidate, Fact: &OperationCycleVersionFact{SourceID: source.ID, StrategySourceID: source.StrategyID, Version: source.Version, Label: source.Label, Objective: source.Objective, VersionHash: source.VersionHash, EffectiveFrom: source.EffectiveFrom, OriginalGovernance: source.GovernanceStatus, ConfirmedAt: source.ConfirmedAt, OperationSkillHash: source.OperationSkillHash, CreatedAt: source.CreatedAt, SealedSourceDigest: row.PayloadHMAC}}
}

func adaptDocument(row SourceRecord) OperationCycleDocumentResult {
	var source documentJSON
	if !decode(row, &source, "id", "strategy_version_id", "schema_version", "execution_guide_markdown", "execution_guide_sha256", "execution_guide_source", "copy_guide_markdown", "copy_guide_sha256", "copy_guide_source", "measurement_guide_markdown", "measurement_guide_sha256", "measurement_guide_source", "execution_contract_json", "document_pack_hash", "created_at") || !present(row.Payload, "execution_guide_generated_at", "copy_guide_generated_at", "measurement_guide_generated_at") {
		return OperationCycleDocumentResult{Disposition: DispositionQuarantine, Reason: "operation_cycle_strategy_version_document_shape_invalid"}
	}
	return OperationCycleDocumentResult{Disposition: DispositionCandidate, Fact: &OperationCycleDocumentFact{SourceID: source.ID, StrategyVersionSourceID: source.StrategyVersionID, SchemaVersion: source.SchemaVersion, ExecutionGuideSHA256: source.ExecutionGuideSHA256, ExecutionGuideGeneratedAt: source.ExecutionGuideGeneratedAt, CopyGuideSHA256: source.CopyGuideSHA256, CopyGuideGeneratedAt: source.CopyGuideGeneratedAt, MeasurementGuideSHA256: source.MeasurementGuideSHA256, MeasurementGuideGeneratedAt: source.MeasurementGuideGeneratedAt, DocumentPackHash: source.DocumentPackHash, CreatedAt: source.CreatedAt, SealedSourceDigest: row.PayloadHMAC}}
}

func decode(row SourceRecord, target any, required ...string) bool {
	if row.PayloadHMAC == (OpaqueDigest{}) {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(row.Payload, &fields) != nil {
		return false
	}
	for _, name := range required {
		value, found := fields[name]
		if !found || len(value) == 0 || string(value) == "null" || !json.Valid(value) {
			return false
		}
	}
	return json.Unmarshal(row.Payload, target) == nil
}

func present(row json.RawMessage, names ...string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(row, &fields) != nil {
		return false
	}
	for _, name := range names {
		if value, found := fields[name]; !found || len(value) == 0 || !json.Valid(value) {
			return false
		}
	}
	return true
}

func quarantineDuplicateGroupInvites(rows []GroupInviteResult) {
	counts := map[int64]int{}
	for _, row := range rows {
		if row.Fact != nil {
			counts[row.Fact.SourceID]++
		}
	}
	for i := range rows {
		if rows[i].Fact != nil && counts[rows[i].Fact.SourceID] != 1 {
			rows[i] = GroupInviteResult{Disposition: DispositionQuarantine, Reason: "group_invite_library_source_ambiguous"}
		}
	}
}

func quarantineDuplicatePageSlices(rows []ProductPageSliceResult) {
	counts := map[int64]int{}
	for _, row := range rows {
		if row.Fact != nil {
			counts[row.Fact.SourceID]++
		}
	}
	for i := range rows {
		if rows[i].Fact != nil && counts[rows[i].Fact.SourceID] != 1 {
			rows[i] = ProductPageSliceResult{Disposition: DispositionQuarantine, Reason: "wechat_pay_product_page_slice_source_ambiguous"}
		}
	}
}

func quarantineDuplicateStrategies(rows []OperationCycleStrategyResult) {
	counts := map[int64]int{}
	for _, row := range rows {
		if row.Fact != nil {
			counts[row.Fact.SourceID]++
		}
	}
	for i := range rows {
		if rows[i].Fact != nil && counts[rows[i].Fact.SourceID] != 1 {
			quarantineStrategy(&rows[i], "operation_cycle_strategy_source_ambiguous")
		}
	}
}

func quarantineDuplicateVersions(rows []OperationCycleVersionResult) {
	counts := map[int64]int{}
	for _, row := range rows {
		if row.Fact != nil {
			counts[row.Fact.SourceID]++
		}
	}
	for i := range rows {
		if rows[i].Fact != nil && counts[rows[i].Fact.SourceID] != 1 {
			quarantineVersion(&rows[i], "operation_cycle_strategy_version_source_ambiguous")
		}
	}
}

func quarantineDuplicateDocuments(rows []OperationCycleDocumentResult) {
	counts := map[int64]int{}
	for _, row := range rows {
		if row.Fact != nil {
			counts[row.Fact.SourceID]++
		}
	}
	for i := range rows {
		if rows[i].Fact != nil && counts[rows[i].Fact.SourceID] != 1 {
			quarantineDocument(&rows[i], "operation_cycle_strategy_version_document_source_ambiguous")
		}
	}
}

func candidateStrategyIDs(rows []OperationCycleStrategyResult) map[int64]bool {
	result := map[int64]bool{}
	for _, row := range rows {
		if row.Disposition == DispositionCandidate && row.Fact != nil {
			result[row.Fact.SourceID] = true
		}
	}
	return result
}

func candidateVersionIDs(rows []OperationCycleVersionResult) map[int64]bool {
	result := map[int64]bool{}
	for _, row := range rows {
		if row.Disposition == DispositionCandidate && row.Fact != nil {
			result[row.Fact.SourceID] = true
		}
	}
	return result
}

func hasCandidateVersion(rows []OperationCycleVersionResult, strategyID, version int64) bool {
	for _, row := range rows {
		if row.Disposition == DispositionCandidate && row.Fact != nil && row.Fact.StrategySourceID == strategyID && row.Fact.Version == version {
			return true
		}
	}
	return false
}

func quarantineStrategy(row *OperationCycleStrategyResult, reason string) {
	row.Disposition, row.Reason, row.Fact = DispositionQuarantine, reason, nil
}
func quarantineVersion(row *OperationCycleVersionResult, reason string) {
	row.Disposition, row.Reason, row.Fact = DispositionQuarantine, reason, nil
}
func quarantineDocument(row *OperationCycleDocumentResult, reason string) {
	row.Disposition, row.Reason, row.Fact = DispositionQuarantine, reason, nil
}
