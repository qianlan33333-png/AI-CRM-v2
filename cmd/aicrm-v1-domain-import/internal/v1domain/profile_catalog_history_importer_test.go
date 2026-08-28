package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1profilecatalog"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type profileCatalogImporterContextKey struct{}

func TestProfileCatalogHistoryImporterImportsAndReplaysInCallerTransaction(t *testing.T) {
	ctx := context.WithValue(context.Background(), profileCatalogImporterContextKey{}, "tx")
	archive := profileCatalogArchiveFixture(t)
	terminals := profileCatalogTerminalFixtures(ctx)
	owner := newProfileCatalogImporterOwner(ctx, terminals)
	importer, err := newProfileCatalogHistoryImporter(archive, profileCatalogTestUOW{}, owner, owner, terminals, "archive-run")
	if err != nil {
		t.Fatal(err)
	}

	result, err := importer.Import(ctx, "archive-run")
	if err != nil || result.SourceCount() != 4 || result.Templates.Imported != 1 || result.Categories.Imported != 1 || result.OptionMappings.Imported != 1 || result.SignupTagRules.Imported != 1 || result.Templates.Replayed != 0 {
		t.Fatalf("first import result=%+v err=%v", result, err)
	}
	if owner.calls != 4 || owner.categories[102].TemplateHistoryID != 101 || owner.mappings[103].CategoryHistoryID != 102 {
		t.Fatalf("owner did not receive actual history parents: calls=%d categories=%+v mappings=%+v", owner.calls, owner.categories, owner.mappings)
	}

	replay, err := importer.Import(ctx, "archive-run")
	if err != nil || replay.SourceCount() != 4 || replay.Templates.Replayed != 1 || replay.Categories.Replayed != 1 || replay.OptionMappings.Replayed != 1 || replay.SignupTagRules.Replayed != 1 || owner.calls != 8 {
		t.Fatalf("replay did not verify each typed target: result=%+v err=%v calls=%d", replay, err, owner.calls)
	}
}

func TestProfileCatalogHistoryImporterQuarantinesRedactionAndMissingParent(t *testing.T) {
	ctx := context.WithValue(context.Background(), profileCatalogImporterContextKey{}, "tx")
	archive := profileCatalogArchiveFixture(t)
	category := archive.tables[v1profilecatalog.ProfileCategoriesTableID][0]
	category.Payload = profileCatalogPayload(t, map[string]any{
		"id": int64(10), "template_id": int64(999), "category_key": "journey", "category_name": "旅程", "description": "分类", "sort_order": int64(0), "enabled": true,
		"created_at": profileCatalogStamp(), "updated_at": profileCatalogStamp(),
	})
	archive.tables[v1profilecatalog.ProfileCategoriesTableID][0] = category
	rule := archive.tables[v1profilecatalog.SignupTagRulesTableID][0]
	rule.RedactedFields = []string{"tag_name"}
	archive.tables[v1profilecatalog.SignupTagRulesTableID][0] = rule
	terminals := profileCatalogTerminalFixtures(ctx)
	owner := newProfileCatalogImporterOwner(ctx, terminals)
	importer, err := newProfileCatalogHistoryImporter(archive, profileCatalogTestUOW{}, owner, owner, terminals, "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(ctx, "archive-run")
	if err != nil || result.Templates.Imported != 1 || result.Categories.Quarantined != 1 || result.OptionMappings.Quarantined != 1 || result.SignupTagRules.Quarantined != 1 || result.SourceCount() != 4 {
		t.Fatalf("redaction/parent terminal result=%+v err=%v", result, err)
	}
	if owner.calls != 1 {
		t.Fatalf("unresolved/redacted rows reached target writer: calls=%d", owner.calls)
	}
	terminal := terminals[v1profilecatalog.SignupTagRulesTableID].(*profileCatalogTerminalFake).receipts[SourceIdentifier(rule.SourceKeyHMAC)]
	if terminal.Disposition != "quarantine" || terminal.Reason != "profile_catalog_redacted_field" {
		t.Fatalf("redacted rule was not fail-closed: %+v", terminal)
	}
}

func TestProfileCatalogHistoryImporterRejectsReceiptDriftAndTransactionFailure(t *testing.T) {
	ctx := context.WithValue(context.Background(), profileCatalogImporterContextKey{}, "tx")
	archive := profileCatalogArchiveFixture(t)
	terminals := profileCatalogTerminalFixtures(ctx)
	owner := newProfileCatalogImporterOwner(ctx, terminals)
	owner.driftTable = v1profilecatalog.ProfileTemplatesTableID
	importer, err := newProfileCatalogHistoryImporter(archive, profileCatalogTestUOW{}, owner, owner, terminals, "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(ctx, "archive-run"); !errors.Is(err, ErrConflict) {
		t.Fatalf("receipt drift accepted: %v", err)
	}

	terminals = profileCatalogTerminalFixtures(ctx)
	owner = newProfileCatalogImporterOwner(ctx, terminals)
	importer, err = newProfileCatalogHistoryImporter(archive, profileCatalogTestUOW{err: errors.New("rollback")}, owner, owner, terminals, "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(ctx, "archive-run"); err == nil || owner.calls != 0 {
		t.Fatalf("transaction failure wrote a target: err=%v calls=%d", err, owner.calls)
	}
}

func TestProfileCatalogHistoryImporterRejectsBadArchiveEnvelope(t *testing.T) {
	ctx := context.WithValue(context.Background(), profileCatalogImporterContextKey{}, "tx")
	archive := profileCatalogArchiveFixture(t)
	row := archive.tables[v1profilecatalog.ProfileTemplatesTableID][0]
	row.SourceOrdinal = 2
	archive.tables[v1profilecatalog.ProfileTemplatesTableID][0] = row
	terminals := profileCatalogTerminalFixtures(ctx)
	owner := newProfileCatalogImporterOwner(ctx, terminals)
	importer, err := newProfileCatalogHistoryImporter(archive, profileCatalogTestUOW{}, owner, owner, terminals, "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(ctx, "archive-run"); !errors.Is(err, ErrConflict) || owner.calls != 0 {
		t.Fatalf("bad source ordinal accepted: err=%v calls=%d", err, owner.calls)
	}
}

type profileCatalogArchiveFake struct {
	tables map[string][]v1archive.ArchivedRow
}

func (fake profileCatalogArchiveFake) EachTableRow(_ context.Context, run, table string, visit func(v1archive.ArchivedRow) error) error {
	if run != "archive-run" {
		return ErrInvalidScope
	}
	for _, row := range fake.tables[table] {
		if err := visit(row); err != nil {
			return err
		}
	}
	return nil
}

type profileCatalogTestUOW struct{ err error }

func (uow profileCatalogTestUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	if uow.err != nil {
		return uow.err
	}
	return callback(ctx)
}

type profileCatalogTerminalFake struct {
	ctx      context.Context
	receipts map[string]TerminalReceipt
}

func (fake *profileCatalogTerminalFake) LoadTerminal(ctx context.Context, source string) (TerminalReceipt, bool, error) {
	if ctx != fake.ctx {
		return TerminalReceipt{}, false, errors.New("wrong caller transaction")
	}
	value, found := fake.receipts[source]
	return value, found, nil
}

func (fake *profileCatalogTerminalFake) Record(ctx context.Context, value TerminalReceipt) error {
	if ctx != fake.ctx {
		return errors.New("wrong caller transaction")
	}
	source := SourceIdentifier(value.SourceKeyDigest)
	if existing, found := fake.receipts[source]; found {
		if existing.SourceKeyDigest != value.SourceKeyDigest || existing.PayloadDigest != value.PayloadDigest || existing.Disposition != value.Disposition || existing.Reason != value.Reason || existing.TargetID != value.TargetID || existing.TargetDigest != value.TargetDigest || !reflect.DeepEqual(existing.Metadata, value.Metadata) {
			return ErrConflict
		}
		return nil
	}
	fake.receipts[source] = value
	return nil
}

type profileCatalogImporterOwner struct {
	ctx        context.Context
	terminals  map[string]profileCatalogTerminalJournal
	templates  map[int64]segmentport.HistoricalProfileTemplate
	categories map[int64]segmentport.HistoricalProfileCategory
	mappings   map[int64]segmentport.HistoricalProfileOptionMapping
	rules      map[int64]contactport.HistoricalSignupTagRule
	nextID     int64
	calls      int
	driftTable string
}

func newProfileCatalogImporterOwner(ctx context.Context, terminals map[string]profileCatalogTerminalJournal) *profileCatalogImporterOwner {
	return &profileCatalogImporterOwner{ctx: ctx, terminals: terminals, templates: map[int64]segmentport.HistoricalProfileTemplate{}, categories: map[int64]segmentport.HistoricalProfileCategory{}, mappings: map[int64]segmentport.HistoricalProfileOptionMapping{}, rules: map[int64]contactport.HistoricalSignupTagRule{}, nextID: 100}
}

func (owner *profileCatalogImporterOwner) ApplyTemplate(ctx context.Context, binding v1profilecatalog.SourceBinding, fact v1profilecatalog.TemplateFact) (segmentport.ProfileCatalogHistoryReceipt, error) {
	owner.calls++
	if ctx != owner.ctx {
		return segmentport.ProfileCatalogHistoryReceipt{}, errors.New("wrong caller transaction")
	}
	if receipt, found := owner.profileReceipt(binding, v1profilecatalog.ProfileTemplatesKind); found {
		return receipt, nil
	}
	owner.nextID++
	value := segmentport.HistoricalProfileTemplate{ID: owner.nextID, SourceID: fact.SourceID, SourceKeyDigest: binding.SourceKeyDigest, SourcePayloadDigest: binding.SourcePayloadDigest}
	owner.templates[value.ID] = value
	return owner.recordProfile(binding, v1profilecatalog.ProfileTemplatesKind, value.ID)
}

func (owner *profileCatalogImporterOwner) ApplyCategory(ctx context.Context, binding v1profilecatalog.SourceBinding, fact v1profilecatalog.CategoryFact, parent segmentport.HistoricalProfileTemplate) (segmentport.ProfileCatalogHistoryReceipt, error) {
	owner.calls++
	if ctx != owner.ctx {
		return segmentport.ProfileCatalogHistoryReceipt{}, errors.New("wrong caller transaction")
	}
	if receipt, found := owner.profileReceipt(binding, v1profilecatalog.ProfileCategoriesKind); found {
		return receipt, nil
	}
	owner.nextID++
	value := segmentport.HistoricalProfileCategory{ID: owner.nextID, SourceID: fact.SourceID, SourceKeyDigest: binding.SourceKeyDigest, SourcePayloadDigest: binding.SourcePayloadDigest, TemplateSourceID: fact.TemplateSourceID, TemplateHistoryID: parent.ID}
	owner.categories[value.ID] = value
	return owner.recordProfile(binding, v1profilecatalog.ProfileCategoriesKind, value.ID)
}

func (owner *profileCatalogImporterOwner) ApplyOptionMapping(ctx context.Context, binding v1profilecatalog.SourceBinding, fact v1profilecatalog.OptionMappingFact, template segmentport.HistoricalProfileTemplate, category segmentport.HistoricalProfileCategory) (segmentport.ProfileCatalogHistoryReceipt, error) {
	owner.calls++
	if ctx != owner.ctx {
		return segmentport.ProfileCatalogHistoryReceipt{}, errors.New("wrong caller transaction")
	}
	if receipt, found := owner.profileReceipt(binding, v1profilecatalog.ProfileOptionMappingsKind); found {
		return receipt, nil
	}
	owner.nextID++
	value := segmentport.HistoricalProfileOptionMapping{ID: owner.nextID, SourceID: fact.SourceID, SourceKeyDigest: binding.SourceKeyDigest, SourcePayloadDigest: binding.SourcePayloadDigest, TemplateSourceID: fact.TemplateSourceID, CategorySourceID: fact.CategorySourceID, TemplateHistoryID: template.ID, CategoryHistoryID: category.ID}
	owner.mappings[value.ID] = value
	return owner.recordProfile(binding, v1profilecatalog.ProfileOptionMappingsKind, value.ID)
}

func (owner *profileCatalogImporterOwner) ApplySignupTagRule(ctx context.Context, binding v1profilecatalog.SourceBinding, fact v1profilecatalog.SignupTagRuleFact) (contactport.SignupTagHistoryReceipt, error) {
	owner.calls++
	if ctx != owner.ctx {
		return contactport.SignupTagHistoryReceipt{}, errors.New("wrong caller transaction")
	}
	if terminal, found, _ := owner.terminals[v1profilecatalog.SignupTagRulesTableID].LoadTerminal(ctx, binding.SourceIdentifier); found {
		id, _ := strconv.ParseInt(terminal.TargetID, 10, 64)
		return contactport.SignupTagHistoryReceipt{SourceIdentifier: binding.SourceIdentifier, PayloadDigest: binding.SourcePayloadDigest, TargetID: id, TargetDigest: digestForProfileCatalogTarget(id), Replayed: true}, nil
	}
	owner.nextID++
	value := contactport.HistoricalSignupTagRule{ID: owner.nextID, SourceKeyDigest: binding.SourceKeyDigest, SourcePayloadDigest: binding.SourcePayloadDigest, TagSourceID: fact.TagSourceID}
	owner.rules[value.ID] = value
	receipt := contactport.SignupTagHistoryReceipt{SourceIdentifier: binding.SourceIdentifier, PayloadDigest: binding.SourcePayloadDigest, TargetID: value.ID, TargetDigest: digestForProfileCatalogTarget(value.ID)}
	terminal := TerminalReceipt{SourceKeyDigest: binding.SourceKeyDigest, PayloadDigest: binding.SourcePayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(value.ID, 10), TargetDigest: receipt.TargetDigest}
	if owner.driftTable == binding.TableID {
		terminal.TargetDigest = digestForProfileCatalogTarget(value.ID + 99)
	}
	if err := owner.terminals[binding.TableID].Record(ctx, terminal); err != nil {
		return contactport.SignupTagHistoryReceipt{}, err
	}
	return receipt, nil
}

func (owner *profileCatalogImporterOwner) profileReceipt(binding v1profilecatalog.SourceBinding, kind string) (segmentport.ProfileCatalogHistoryReceipt, bool) {
	terminal, found, _ := owner.terminals[binding.TableID].LoadTerminal(owner.ctx, binding.SourceIdentifier)
	if !found {
		return segmentport.ProfileCatalogHistoryReceipt{}, false
	}
	id, _ := strconv.ParseInt(terminal.TargetID, 10, 64)
	return segmentport.ProfileCatalogHistoryReceipt{Kind: kind, SourceIdentifier: binding.SourceIdentifier, PayloadDigest: binding.SourcePayloadDigest, TargetID: id, TargetDigest: digestForProfileCatalogTarget(id), Replayed: true}, true
}

func (owner *profileCatalogImporterOwner) recordProfile(binding v1profilecatalog.SourceBinding, kind string, id int64) (segmentport.ProfileCatalogHistoryReceipt, error) {
	receipt := segmentport.ProfileCatalogHistoryReceipt{Kind: kind, SourceIdentifier: binding.SourceIdentifier, PayloadDigest: binding.SourcePayloadDigest, TargetID: id, TargetDigest: digestForProfileCatalogTarget(id)}
	terminal := TerminalReceipt{SourceKeyDigest: binding.SourceKeyDigest, PayloadDigest: binding.SourcePayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(id, 10), TargetDigest: receipt.TargetDigest}
	if owner.driftTable == binding.TableID {
		terminal.TargetDigest = digestForProfileCatalogTarget(id + 99)
	}
	if err := owner.terminals[binding.TableID].Record(owner.ctx, terminal); err != nil {
		return segmentport.ProfileCatalogHistoryReceipt{}, err
	}
	return receipt, nil
}

func (owner *profileCatalogImporterOwner) ReadTemplate(_ context.Context, id int64) (segmentport.HistoricalProfileTemplate, error) {
	value, ok := owner.templates[id]
	if !ok {
		return segmentport.HistoricalProfileTemplate{}, ErrConflict
	}
	return value, nil
}
func (owner *profileCatalogImporterOwner) ReadCategory(_ context.Context, id int64) (segmentport.HistoricalProfileCategory, error) {
	value, ok := owner.categories[id]
	if !ok {
		return segmentport.HistoricalProfileCategory{}, ErrConflict
	}
	return value, nil
}
func (owner *profileCatalogImporterOwner) ReadOptionMapping(_ context.Context, id int64) (segmentport.HistoricalProfileOptionMapping, error) {
	value, ok := owner.mappings[id]
	if !ok {
		return segmentport.HistoricalProfileOptionMapping{}, ErrConflict
	}
	return value, nil
}
func (owner *profileCatalogImporterOwner) ReadSignupTagRule(_ context.Context, id int64) (contactport.HistoricalSignupTagRule, error) {
	value, ok := owner.rules[id]
	if !ok {
		return contactport.HistoricalSignupTagRule{}, ErrConflict
	}
	return value, nil
}

func profileCatalogArchiveFixture(t *testing.T) profileCatalogArchiveFake {
	t.Helper()
	stamp := profileCatalogStamp()
	return profileCatalogArchiveFake{tables: map[string][]v1archive.ArchivedRow{
		v1profilecatalog.ProfileTemplatesTableID:      {profileCatalogRow(t, v1profilecatalog.ProfileTemplatesTableID, 1, 1, map[string]any{"id": int64(-7), "template_code": "legacy", "template_name": "历史", "questionnaire_id": int64(901), "segmentation_question_id": nil, "description": "说明", "enabled": false, "version": int64(-3), "created_by": "actor", "updated_by": "actor", "created_at": stamp, "updated_at": stamp, "program_id": nil})},
		v1profilecatalog.ProfileCategoriesTableID:     {profileCatalogRow(t, v1profilecatalog.ProfileCategoriesTableID, 1, 2, map[string]any{"id": int64(10), "template_id": int64(-7), "category_key": "journey", "category_name": "旅程", "description": "分类", "sort_order": int64(-2), "enabled": false, "created_at": stamp, "updated_at": stamp})},
		v1profilecatalog.ProfileOptionMappingsTableID: {profileCatalogRow(t, v1profilecatalog.ProfileOptionMappingsTableID, 1, 3, map[string]any{"id": int64(100), "template_id": int64(-7), "category_id": int64(10), "question_id": int64(777), "option_id": int64(778), "created_at": stamp})},
		v1profilecatalog.SignupTagRulesTableID:        {profileCatalogRow(t, v1profilecatalog.SignupTagRulesTableID, 1, 4, map[string]any{"tag_id": "tag-v1", "tag_name": "报名", "signup_status": "approved", "active": false, "updated_at": stamp})},
	}}
}

func profileCatalogRow(t *testing.T, table string, ordinal int64, n byte, value map[string]any) v1archive.ArchivedRow {
	t.Helper()
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: profileCatalogDigest(n), PayloadHMAC: profileCatalogDigest(n + 20), FieldHMAC: profileCatalogDigest(n + 40), Payload: profileCatalogPayload(t, value)}
}

func profileCatalogPayload(t *testing.T, value map[string]any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
func profileCatalogStamp() time.Time { return time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.UTC) }
func profileCatalogDigest(n byte) [sha256.Size]byte {
	var digest [sha256.Size]byte
	digest[0] = n
	return digest
}
func digestForProfileCatalogTarget(id int64) [sha256.Size]byte { return profileCatalogDigest(byte(id)) }

func profileCatalogTerminalFixtures(ctx context.Context) map[string]profileCatalogTerminalJournal {
	result := map[string]profileCatalogTerminalJournal{}
	for _, scope := range profileCatalogHistoryScopes {
		result[scope.source] = &profileCatalogTerminalFake{ctx: ctx, receipts: map[string]TerminalReceipt{}}
	}
	return result
}
