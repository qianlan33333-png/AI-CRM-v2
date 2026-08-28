package app

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type profileCatalogHistoryFake struct {
	ctx                     context.Context
	receipts                map[string]segmentport.ProfileCatalogHistoryReceipt
	templates               map[int64]segmentport.HistoricalProfileTemplate
	categories              map[int64]segmentport.HistoricalProfileCategory
	mappings                map[int64]segmentport.HistoricalProfileOptionMapping
	nextID                  int64
	loads, records, creates int
}

func newProfileCatalogHistoryFake(ctx context.Context) *profileCatalogHistoryFake {
	return &profileCatalogHistoryFake{ctx: ctx, receipts: map[string]segmentport.ProfileCatalogHistoryReceipt{}, templates: map[int64]segmentport.HistoricalProfileTemplate{}, categories: map[int64]segmentport.HistoricalProfileCategory{}, mappings: map[int64]segmentport.HistoricalProfileOptionMapping{}, nextID: 20}
}
func (f *profileCatalogHistoryFake) LoadProfileCatalogHistory(ctx context.Context, kind, source string) (segmentport.ProfileCatalogHistoryReceipt, bool, error) {
	f.loads++
	if ctx != f.ctx {
		return segmentport.ProfileCatalogHistoryReceipt{}, false, errors.New("wrong context")
	}
	value, found := f.receipts[kind+":"+source]
	return value, found, nil
}
func (f *profileCatalogHistoryFake) RecordProfileCatalogHistory(ctx context.Context, value segmentport.ProfileCatalogHistoryReceipt) error {
	f.records++
	if ctx != f.ctx {
		return errors.New("wrong context")
	}
	key := value.Kind + ":" + value.SourceIdentifier
	if _, found := f.receipts[key]; found {
		return segmentport.ErrProfileCatalogHistoryConflict
	}
	f.receipts[key] = value
	return nil
}
func (f *profileCatalogHistoryFake) CreateHistoricalProfileTemplate(ctx context.Context, value segmentport.HistoricalProfileTemplate) (segmentport.HistoricalProfileTemplate, error) {
	if ctx != f.ctx {
		return value, errors.New("wrong context")
	}
	f.creates++
	f.nextID++
	value.ID = f.nextID
	f.templates[value.ID] = value
	return value, nil
}
func (f *profileCatalogHistoryFake) GetHistoricalProfileTemplate(ctx context.Context, id int64) (segmentport.HistoricalProfileTemplate, error) {
	if ctx != f.ctx {
		return segmentport.HistoricalProfileTemplate{}, errors.New("wrong context")
	}
	value, found := f.templates[id]
	if !found {
		return segmentport.HistoricalProfileTemplate{}, segmentport.ErrProfileCatalogHistoryConflict
	}
	return value, nil
}
func (f *profileCatalogHistoryFake) CreateHistoricalProfileCategory(ctx context.Context, value segmentport.HistoricalProfileCategory) (segmentport.HistoricalProfileCategory, error) {
	if ctx != f.ctx {
		return value, errors.New("wrong context")
	}
	f.creates++
	f.nextID++
	value.ID = f.nextID
	f.categories[value.ID] = value
	return value, nil
}
func (f *profileCatalogHistoryFake) GetHistoricalProfileCategory(ctx context.Context, id int64) (segmentport.HistoricalProfileCategory, error) {
	if ctx != f.ctx {
		return segmentport.HistoricalProfileCategory{}, errors.New("wrong context")
	}
	value, found := f.categories[id]
	if !found {
		return segmentport.HistoricalProfileCategory{}, segmentport.ErrProfileCatalogHistoryConflict
	}
	return value, nil
}
func (f *profileCatalogHistoryFake) CreateHistoricalProfileOptionMapping(ctx context.Context, value segmentport.HistoricalProfileOptionMapping) (segmentport.HistoricalProfileOptionMapping, error) {
	if ctx != f.ctx {
		return value, errors.New("wrong context")
	}
	f.creates++
	f.nextID++
	value.ID = f.nextID
	f.mappings[value.ID] = value
	return value, nil
}
func (f *profileCatalogHistoryFake) GetHistoricalProfileOptionMapping(ctx context.Context, id int64) (segmentport.HistoricalProfileOptionMapping, error) {
	if ctx != f.ctx {
		return segmentport.HistoricalProfileOptionMapping{}, errors.New("wrong context")
	}
	value, found := f.mappings[id]
	if !found {
		return segmentport.HistoricalProfileOptionMapping{}, segmentport.ErrProfileCatalogHistoryConflict
	}
	return value, nil
}

func TestProfileCatalogHistoryImportsReplayAndValidateParents(t *testing.T) {
	ctx := context.WithValue(context.Background(), "profile-catalog", "tx")
	fake := newProfileCatalogHistoryFake(ctx)
	service := NewProfileCatalogHistoryService(fake, fake)
	template := templateFixture()
	templateSource := hex.EncodeToString(template.SourceKeyDigest[:])
	templateReceipt, err := service.ImportTemplate(ctx, templateSource, template.SourcePayloadDigest, template)
	if err != nil || templateReceipt.TargetID < 1 || fake.creates != 1 || fake.records != 1 {
		t.Fatal("template was not written with a caller-tx receipt")
	}
	if replay, err := service.ImportTemplate(ctx, templateSource, template.SourcePayloadDigest, template); err != nil || !replay.Replayed || fake.creates != 1 {
		t.Fatal("template replay was not verified")
	}

	category := categoryFixture(templateReceipt.TargetID)
	categorySource := hex.EncodeToString(category.SourceKeyDigest[:])
	categoryReceipt, err := service.ImportCategory(ctx, categorySource, category.SourcePayloadDigest, category)
	if err != nil || categoryReceipt.TargetID < 1 {
		t.Fatal("category parent was not accepted")
	}
	mapping := mappingFixture(templateReceipt.TargetID, categoryReceipt.TargetID)
	mappingSource := hex.EncodeToString(mapping.SourceKeyDigest[:])
	if _, err = service.ImportOptionMapping(ctx, mappingSource, mapping.SourcePayloadDigest, mapping); err != nil {
		t.Fatal("mapping same-template parent was not accepted")
	}

	mapping.CategorySourceID++
	if _, err = service.ImportOptionMapping(ctx, mappingSource, mapping.SourcePayloadDigest, mapping); !errors.Is(err, segmentport.ErrProfileCatalogHistoryConflict) {
		t.Fatal("mapping/category source parent mismatch was accepted")
	}
}

func TestProfileCatalogHistoryRejectsNoncanonicalSourceBeforeDependencies(t *testing.T) {
	wrongKey := [32]byte{9}
	for _, source := range []string{"not-hex", hex.EncodeToString(wrongKey[:])} {
		t.Run(source, func(t *testing.T) {
			ctx := context.Background()
			fake := newProfileCatalogHistoryFake(ctx)
			service := NewProfileCatalogHistoryService(fake, fake)
			template := templateFixture()
			if _, err := service.ImportTemplate(ctx, source, template.SourcePayloadDigest, template); !errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid) {
				t.Fatalf("template error=%v", err)
			}
			category := categoryFixture(1)
			if _, err := service.ImportCategory(ctx, source, category.SourcePayloadDigest, category); !errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid) {
				t.Fatalf("category error=%v", err)
			}
			mapping := mappingFixture(1, 2)
			if _, err := service.ImportOptionMapping(ctx, source, mapping.SourcePayloadDigest, mapping); !errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid) {
				t.Fatalf("mapping error=%v", err)
			}
			if fake.loads != 0 || fake.creates != 0 || fake.records != 0 {
				t.Fatalf("source rejection reached dependencies loads=%d creates=%d records=%d", fake.loads, fake.creates, fake.records)
			}
		})
	}
}

func TestProfileCatalogHistoryRejectsPayloadBindingAndDigestCoversTarget(t *testing.T) {
	ctx := context.Background()
	fake := newProfileCatalogHistoryFake(ctx)
	service := NewProfileCatalogHistoryService(fake, fake)
	template := templateFixture()
	if _, err := service.ImportTemplate(ctx, "source", [32]byte{1}, template); !errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid) {
		t.Fatal("payload digest mismatch was accepted")
	}
	template.ID = 21
	before, err := HistoricalProfileTemplateDigest(template)
	if err != nil {
		t.Fatal(err)
	}
	template.ID++
	after, err := HistoricalProfileTemplateDigest(template)
	if err != nil || before == after {
		t.Fatal("target ID omitted from template digest")
	}
	template.CreatedAt = template.CreatedAt.UTC().Truncate(time.Microsecond)
	template.UpdatedAt = template.UpdatedAt.UTC().Truncate(time.Microsecond)
	if normalized, err := HistoricalProfileTemplateDigest(template); err != nil || normalized != after {
		t.Fatal("time was not normalized to UTC microseconds")
	}
}

func templateFixture() segmentport.HistoricalProfileTemplate {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 123456789, time.FixedZone("source", 8*60*60))
	question := int64(0)
	program := int64(-2)
	return segmentport.HistoricalProfileTemplate{SourceID: -7, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{2}, TemplateCode: " code ", TemplateName: " name ", QuestionnaireSourceID: &question, ProgramSourceID: &program, Description: " text ", OriginalEnabled: false, Version: 0, CreatedByDigest: [32]byte{3}, UpdatedByDigest: [32]byte{4}, CreatedAt: stamp, UpdatedAt: stamp}
}
func categoryFixture(templateID int64) segmentport.HistoricalProfileCategory {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 123456789, time.UTC)
	return segmentport.HistoricalProfileCategory{SourceID: 0, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{2}, TemplateSourceID: -7, TemplateHistoryID: templateID, CategoryKey: "key", CategoryName: "name", Description: "description", SortOrder: -1, OriginalEnabled: false, CreatedAt: stamp, UpdatedAt: stamp}
}
func mappingFixture(templateID, categoryID int64) segmentport.HistoricalProfileOptionMapping {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 123456789, time.UTC)
	return segmentport.HistoricalProfileOptionMapping{SourceID: -9, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{2}, TemplateSourceID: -7, CategorySourceID: 0, TemplateHistoryID: templateID, CategoryHistoryID: categoryID, QuestionSourceID: 0, OptionSourceID: -4, CreatedAt: stamp}
}
