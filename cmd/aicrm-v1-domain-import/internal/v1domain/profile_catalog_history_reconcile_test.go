package v1domain

import (
	"context"
	"errors"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1profilecatalog"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	"strconv"
	"testing"
	"time"
)

type profileCatalogReconcileReader struct {
	segmentport.ProfileCatalogHistoryReader
	contactport.SignupTagHistoryReader
	template segmentport.HistoricalProfileTemplate
	category segmentport.HistoricalProfileCategory
	mapping  segmentport.HistoricalProfileOptionMapping
	rule     contactport.HistoricalSignupTagRule
}

func (r profileCatalogReconcileReader) GetHistoricalProfileTemplate(ctx context.Context, id int64) (segmentport.HistoricalProfileTemplate, error) {
	return r.template, nil
}
func (r profileCatalogReconcileReader) GetHistoricalProfileCategory(ctx context.Context, id int64) (segmentport.HistoricalProfileCategory, error) {
	return r.category, nil
}
func (r profileCatalogReconcileReader) GetHistoricalProfileOptionMapping(ctx context.Context, id int64) (segmentport.HistoricalProfileOptionMapping, error) {
	return r.mapping, nil
}
func (r profileCatalogReconcileReader) GetHistoricalSignupTagRule(ctx context.Context, id int64) (contactport.HistoricalSignupTagRule, error) {
	return r.rule, nil
}
func profileCatalogReconcileFixture() profileCatalogReconcileReader {
	return profileCatalogReconcileReader{template: segmentport.HistoricalProfileTemplate{ID: 71, SourceID: 101, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, TemplateCode: "old", TemplateName: "old", QuestionnaireSourceID: nil, SegmentationQuestionSourceID: nil, ProgramSourceID: nil, Description: "old", OriginalEnabled: false, Version: -7, CreatedByDigest: [32]byte{1}, UpdatedByDigest: [32]byte{1}, CreatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC), UpdatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC)}, category: segmentport.HistoricalProfileCategory{ID: 72, SourceID: 102, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, TemplateSourceID: 101, TemplateHistoryID: 71, CategoryKey: "old", CategoryName: "old", Description: "old", SortOrder: -7, OriginalEnabled: false, CreatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC), UpdatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC)}, mapping: segmentport.HistoricalProfileOptionMapping{ID: 73, SourceID: 103, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, TemplateSourceID: 101, CategorySourceID: 102, TemplateHistoryID: 71, CategoryHistoryID: 72, QuestionSourceID: -7, OptionSourceID: -7, CreatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC)}, rule: contactport.HistoricalSignupTagRule{ID: 74, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, TagSourceID: "old", TagName: "old", SignupStatus: "old", OriginalActive: false, UpdatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC)}}
}
func TestProfileCatalogReconcileTypedFactsAndParentBinding(t *testing.T) {
	sourceTables := []string{v1profilecatalog.ProfileTemplatesTableID, v1profilecatalog.ProfileCategoriesTableID, v1profilecatalog.ProfileOptionMappingsTableID, v1profilecatalog.SignupTagRulesTableID}
	for index, source := range sourceTables {
		reader := profileCatalogReconcileFixture()
		var digest [32]byte
		var err error
		switch index {
		case 0:
			digest, err = segmentapp.HistoricalProfileTemplateDigest(reader.template)
		case 1:
			digest, err = segmentapp.HistoricalProfileCategoryDigest(reader.category)
		case 2:
			digest, err = segmentapp.HistoricalProfileOptionMappingDigest(reader.mapping)
		case 3:
			digest, err = contactapp.HistoricalSignupTagRuleDigest(reader.rule)
		}
		if err != nil {
			t.Fatal(err)
		}
		id := strconv.Itoa(71 + index)
		scope := targetBySourceTable[source]
		key, payload := [32]byte{1}, [32]byte{1}
		row := reconciliationRow{TableID: source, SourceKeyDigest: key[:], PayloadDigest: payload[:], TargetDomain: &scope.domain, TargetTable: &scope.table, TargetID: &id, TargetDigest: digest[:]}
		targets := map[string]map[string]struct{}{v1profilecatalog.ProfileTemplatesTargetTable: {"71": {}}, v1profilecatalog.ProfileCategoriesTargetTable: {"72": {}}}
		if _, err := verifyProfileCatalogHistoryRow(context.Background(), reader, reader, row, targets); err != nil {
			t.Fatalf("source %s: %v", source, err)
		}
		for _, mutate := range []func(*reconciliationRow){
			func(r *reconciliationRow) { r.TargetDigest = make([]byte, 32) },
			func(r *reconciliationRow) { r.SourceKeyDigest = make([]byte, 32) },
			func(r *reconciliationRow) { r.PayloadDigest = make([]byte, 32) },
			func(r *reconciliationRow) { x := "current"; r.TargetTable = &x },
			func(r *reconciliationRow) { x := "0" + *r.TargetID; r.TargetID = &x },
		} {
			changed := row
			mutate(&changed)
			if _, err := verifyProfileCatalogHistoryRow(context.Background(), reader, reader, changed, targets); !errors.Is(err, ErrConflict) {
				t.Fatal("target drift accepted")
			}
		}
		changed := reader
		switch index {
		case 0:
			changed.template.TemplateName = "drift"
		case 1:
			changed.category.CategoryName = "drift"
		case 2:
			changed.mapping.OptionSourceID++
		case 3:
			changed.rule.SignupStatus = "drift"
		}
		if _, err := verifyProfileCatalogHistoryRow(context.Background(), changed, changed, row, targets); !errors.Is(err, ErrConflict) {
			t.Fatal("full fact digest not checked")
		}
		if index == 1 || index == 2 {
			if _, err := verifyProfileCatalogHistoryRow(context.Background(), reader, reader, row, nil); !errors.Is(err, ErrConflict) {
				t.Fatal("parent from another batch accepted")
			}
			changed = reader
			changed.template.SourceID++
			if _, err := verifyProfileCatalogHistoryRow(context.Background(), changed, changed, row, targets); !errors.Is(err, ErrConflict) {
				t.Fatal("wrong source parent accepted")
			}
		}
	}
}
func TestProfileCatalogReconcileScope(t *testing.T) {
	for _, table := range []string{"public/segments", "public/segment_members", "public/wecom_corp_tags"} {
		if isProfileCatalogHistorySource(table) {
			t.Fatal("runtime table selected")
		}
	}
	if _, err := ReconcileProfileCatalogHistory(context.Background(), nil, "wrong", "run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("wrong version entered DB")
	}
}
