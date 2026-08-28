package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type audienceHistoryFake struct {
	context                               context.Context
	receipts                              map[string]segmentport.AudienceHistoryReceipt
	creates, gets, loads, records         int
	createErr, getErr, loadErr, recordErr error
	mutateCreate                          func(*audienceHistoryFake)
	Group                                 segmentport.HistoricalAudienceGroup
	Package                               segmentport.HistoricalAudiencePackage
	Version                               segmentport.HistoricalAudienceVersion
	Sender                                segmentport.HistoricalAudienceSender
	Rule                                  segmentport.HistoricalAudienceRule
	RuleVersion                           segmentport.HistoricalAudienceRuleVersion
	Definition                            segmentport.HistoricalAudienceDefinition
	Member                                segmentport.HistoricalAudienceMember
}

func newAudienceHistoryFake(ctx context.Context) *audienceHistoryFake {
	return &audienceHistoryFake{context: ctx, receipts: map[string]segmentport.AudienceHistoryReceipt{}}
}

func (f *audienceHistoryFake) LoadAudienceHistory(ctx context.Context, kind, source string) (segmentport.AudienceHistoryReceipt, bool, error) {
	f.loads++
	if ctx != f.context {
		return segmentport.AudienceHistoryReceipt{}, false, errors.New("wrong caller context")
	}
	if f.loadErr != nil {
		return segmentport.AudienceHistoryReceipt{}, false, f.loadErr
	}
	receipt, found := f.receipts[kind+":"+source]
	return receipt, found, nil
}

func (f *audienceHistoryFake) RecordAudienceHistory(ctx context.Context, kind string, receipt segmentport.AudienceHistoryReceipt) error {
	f.records++
	if ctx != f.context {
		return errors.New("wrong caller context")
	}
	if f.recordErr != nil {
		return f.recordErr
	}
	key := kind + ":" + receipt.SourceIdentifier
	if _, found := f.receipts[key]; found {
		return segmentport.ErrAudienceHistoryConflict
	}
	f.receipts[key] = receipt
	return nil
}

func (f *audienceHistoryFake) CreateHistoricalAudienceGroup(ctx context.Context, value segmentport.HistoricalAudienceGroup) (segmentport.HistoricalAudienceGroup, error) {
	f.creates++
	if ctx != f.context {
		return segmentport.HistoricalAudienceGroup{}, errors.New("wrong caller context")
	}
	if f.createErr != nil {
		return segmentport.HistoricalAudienceGroup{}, f.createErr
	}
	value.ID = 31
	f.Group = value
	if f.mutateCreate != nil {
		f.mutateCreate(f)
	}
	return f.Group, nil
}

func (f *audienceHistoryFake) GetHistoricalAudienceGroup(ctx context.Context, id int64) (segmentport.HistoricalAudienceGroup, error) {
	f.gets++
	if ctx != f.context || id != 31 {
		return segmentport.HistoricalAudienceGroup{}, errors.New("wrong caller context or target ID")
	}
	return f.Group, f.getErr
}

func audienceHistoryGroupFixture() segmentport.HistoricalAudienceGroup {
	return segmentport.HistoricalAudienceGroup{SourceID: 101, Name: " group \n ",
		CreatedAt: audienceHistoryTestTime(),
		UpdatedAt: audienceHistoryTestTime().Add(-time.Hour),
	}
}

func (f *audienceHistoryFake) CreateHistoricalAudiencePackage(ctx context.Context, value segmentport.HistoricalAudiencePackage) (segmentport.HistoricalAudiencePackage, error) {
	f.creates++
	if ctx != f.context {
		return segmentport.HistoricalAudiencePackage{}, errors.New("wrong caller context")
	}
	if f.createErr != nil {
		return segmentport.HistoricalAudiencePackage{}, f.createErr
	}
	value.ID = 31
	f.Package = value
	if f.mutateCreate != nil {
		f.mutateCreate(f)
	}
	return f.Package, nil
}

func (f *audienceHistoryFake) GetHistoricalAudiencePackage(ctx context.Context, id int64) (segmentport.HistoricalAudiencePackage, error) {
	f.gets++
	if ctx != f.context || id != 31 {
		return segmentport.HistoricalAudiencePackage{}, errors.New("wrong caller context or target ID")
	}
	return f.Package, f.getErr
}

func audienceHistoryPackageFixture() segmentport.HistoricalAudiencePackage {
	return segmentport.HistoricalAudiencePackage{SourceID: 101, GroupHistoryID: audienceHistoryPtr(int64(11)), CurrentVersionSourceID: audienceHistoryPtr(int64(17)), PackageKey: " key ", Name: " package ", NaturalLanguageDefinition: " 说明\n ", OriginalStatus: " raw-status ", QueryMode: " old-mode ", IdentityPolicy: " raw-policy ", IncrementalEnabled: true, DailyEnabled: true, IncrementalIntervalSecs: -5, DailyRefreshTime: " raw-time ", Timezone: " raw-zone ", LookbackSecs: -9, PausedReason: " pause ",
		CreatedAt:            audienceHistoryTestTime(),
		UpdatedAt:            audienceHistoryTestTime().Add(-time.Hour),
		LastIncrementalAt:    audienceHistoryPtr(audienceHistoryTestTime().Add(-2 * time.Hour)),
		LastDailyRefreshedAt: audienceHistoryPtr(audienceHistoryTestTime().Add(-2 * time.Hour)),
		NextIncrementalAt:    audienceHistoryPtr(audienceHistoryTestTime().Add(-2 * time.Hour)),
		NextDailyAt:          audienceHistoryPtr(audienceHistoryTestTime().Add(-2 * time.Hour)),
		RuntimeDigest:        [32]byte{9},
	}
}

func (f *audienceHistoryFake) CreateHistoricalAudienceVersion(ctx context.Context, value segmentport.HistoricalAudienceVersion) (segmentport.HistoricalAudienceVersion, error) {
	f.creates++
	if ctx != f.context {
		return segmentport.HistoricalAudienceVersion{}, errors.New("wrong caller context")
	}
	if f.createErr != nil {
		return segmentport.HistoricalAudienceVersion{}, f.createErr
	}
	value.ID = 31
	f.Version = value
	if f.mutateCreate != nil {
		f.mutateCreate(f)
	}
	return f.Version, nil
}

func (f *audienceHistoryFake) GetHistoricalAudienceVersion(ctx context.Context, id int64) (segmentport.HistoricalAudienceVersion, error) {
	f.gets++
	if ctx != f.context || id != 31 {
		return segmentport.HistoricalAudienceVersion{}, errors.New("wrong caller context or target ID")
	}
	return f.Version, f.getErr
}

func audienceHistoryVersionFixture() segmentport.HistoricalAudienceVersion {
	return segmentport.HistoricalAudienceVersion{SourceID: 101, PackageHistoryID: 12, VersionNumber: -3, OriginalStatus: " raw-status ", AIPrompt: " prompt\n ", AIRationale: " rationale ", NaturalLanguageExplanation: " explain ", TemplateKey: " template ", TemplateVersion: audienceHistoryPtr(int64(-2)), TemplateFingerprint: " fp ",
		CreatedAt:        audienceHistoryTestTime(),
		PublishedAt:      audienceHistoryPtr(audienceHistoryTestTime().Add(-2 * time.Hour)),
		DefinitionDigest: [32]byte{9},
	}
}

func (f *audienceHistoryFake) CreateHistoricalAudienceSender(ctx context.Context, value segmentport.HistoricalAudienceSender) (segmentport.HistoricalAudienceSender, error) {
	f.creates++
	if ctx != f.context {
		return segmentport.HistoricalAudienceSender{}, errors.New("wrong caller context")
	}
	if f.createErr != nil {
		return segmentport.HistoricalAudienceSender{}, f.createErr
	}
	value.ID = 31
	f.Sender = value
	if f.mutateCreate != nil {
		f.mutateCreate(f)
	}
	return f.Sender, nil
}

func (f *audienceHistoryFake) GetHistoricalAudienceSender(ctx context.Context, id int64) (segmentport.HistoricalAudienceSender, error) {
	f.gets++
	if ctx != f.context || id != 31 {
		return segmentport.HistoricalAudienceSender{}, errors.New("wrong caller context or target ID")
	}
	return f.Sender, f.getErr
}

func audienceHistorySenderFixture() segmentport.HistoricalAudienceSender {
	return segmentport.HistoricalAudienceSender{SourceID: 101, PackageHistoryID: 12, StaffID: audienceHistoryPtr(int64(15)), DisplayName: " sender ", Priority: -5, OriginalStatus: " raw-status ",
		CreatedAt: audienceHistoryTestTime(),
		UpdatedAt: audienceHistoryTestTime().Add(-time.Hour),
	}
}

func (f *audienceHistoryFake) CreateHistoricalAudienceRule(ctx context.Context, value segmentport.HistoricalAudienceRule) (segmentport.HistoricalAudienceRule, error) {
	f.creates++
	if ctx != f.context {
		return segmentport.HistoricalAudienceRule{}, errors.New("wrong caller context")
	}
	if f.createErr != nil {
		return segmentport.HistoricalAudienceRule{}, f.createErr
	}
	value.ID = 31
	f.Rule = value
	if f.mutateCreate != nil {
		f.mutateCreate(f)
	}
	return f.Rule, nil
}

func (f *audienceHistoryFake) GetHistoricalAudienceRule(ctx context.Context, id int64) (segmentport.HistoricalAudienceRule, error) {
	f.gets++
	if ctx != f.context || id != 31 {
		return segmentport.HistoricalAudienceRule{}, errors.New("wrong caller context or target ID")
	}
	return f.Rule, f.getErr
}

func audienceHistoryRuleFixture() segmentport.HistoricalAudienceRule {
	return segmentport.HistoricalAudienceRule{SourceID: 101, RuleKey: " key ", DisplayName: " rule ", Description: " desc\n ", RuleType: " raw-type ", OwnerStaffID: audienceHistoryPtr(int64(15)), OriginalStatus: " raw-status ",
		CreatedAt: audienceHistoryTestTime(),
		UpdatedAt: audienceHistoryTestTime().Add(-time.Hour),
	}
}

func (f *audienceHistoryFake) CreateHistoricalAudienceRuleVersion(ctx context.Context, value segmentport.HistoricalAudienceRuleVersion) (segmentport.HistoricalAudienceRuleVersion, error) {
	f.creates++
	if ctx != f.context {
		return segmentport.HistoricalAudienceRuleVersion{}, errors.New("wrong caller context")
	}
	if f.createErr != nil {
		return segmentport.HistoricalAudienceRuleVersion{}, f.createErr
	}
	value.ID = 31
	f.RuleVersion = value
	if f.mutateCreate != nil {
		f.mutateCreate(f)
	}
	return f.RuleVersion, nil
}

func (f *audienceHistoryFake) GetHistoricalAudienceRuleVersion(ctx context.Context, id int64) (segmentport.HistoricalAudienceRuleVersion, error) {
	f.gets++
	if ctx != f.context || id != 31 {
		return segmentport.HistoricalAudienceRuleVersion{}, errors.New("wrong caller context or target ID")
	}
	return f.RuleVersion, f.getErr
}

func audienceHistoryRuleVersionFixture() segmentport.HistoricalAudienceRuleVersion {
	return segmentport.HistoricalAudienceRuleVersion{SourceID: 101, RuleHistoryID: 18, Version: -4, ExecutorType: " raw-executor ", OriginalStatus: " raw-status ",
		CreatedAt:        audienceHistoryTestTime(),
		PublishedAt:      audienceHistoryPtr(audienceHistoryTestTime().Add(-2 * time.Hour)),
		DefinitionDigest: [32]byte{9},
	}
}

func (f *audienceHistoryFake) CreateHistoricalAudienceDefinition(ctx context.Context, value segmentport.HistoricalAudienceDefinition) (segmentport.HistoricalAudienceDefinition, error) {
	f.creates++
	if ctx != f.context {
		return segmentport.HistoricalAudienceDefinition{}, errors.New("wrong caller context")
	}
	if f.createErr != nil {
		return segmentport.HistoricalAudienceDefinition{}, f.createErr
	}
	value.ID = 31
	f.Definition = value
	if f.mutateCreate != nil {
		f.mutateCreate(f)
	}
	return f.Definition, nil
}

func (f *audienceHistoryFake) GetHistoricalAudienceDefinition(ctx context.Context, id int64) (segmentport.HistoricalAudienceDefinition, error) {
	f.gets++
	if ctx != f.context || id != 31 {
		return segmentport.HistoricalAudienceDefinition{}, errors.New("wrong caller context or target ID")
	}
	return f.Definition, f.getErr
}

func audienceHistoryDefinitionFixture() segmentport.HistoricalAudienceDefinition {
	return segmentport.HistoricalAudienceDefinition{SourceID: 101, Code: " code ", DisplayName: " definition ", Description: " desc ", SourceType: " raw-type ", SQLDialect: " raw-dialect ", OriginalStatus: " raw-status ", Version: -4, CachedHeadcount: -6, UsageCount: -7,
		CreatedAt:        audienceHistoryTestTime(),
		UpdatedAt:        audienceHistoryTestTime().Add(-time.Hour),
		LastRefreshedAt:  audienceHistoryPtr(audienceHistoryTestTime().Add(-2 * time.Hour)),
		DefinitionDigest: [32]byte{9},
	}
}

func (f *audienceHistoryFake) CreateHistoricalAudienceMember(ctx context.Context, value segmentport.HistoricalAudienceMember) (segmentport.HistoricalAudienceMember, error) {
	f.creates++
	if ctx != f.context {
		return segmentport.HistoricalAudienceMember{}, errors.New("wrong caller context")
	}
	if f.createErr != nil {
		return segmentport.HistoricalAudienceMember{}, f.createErr
	}
	value.ID = 31
	f.Member = value
	if f.mutateCreate != nil {
		f.mutateCreate(f)
	}
	return f.Member, nil
}

func (f *audienceHistoryFake) GetHistoricalAudienceMember(ctx context.Context, id int64) (segmentport.HistoricalAudienceMember, error) {
	f.gets++
	if ctx != f.context || id != 31 {
		return segmentport.HistoricalAudienceMember{}, errors.New("wrong caller context or target ID")
	}
	return f.Member, f.getErr
}

func audienceHistoryMemberFixture() segmentport.HistoricalAudienceMember {
	return segmentport.HistoricalAudienceMember{SourceID: 101, PackageHistoryID: 12, CustomerID: audienceHistoryPtr(int64(15)), IdentityKind: " raw-kind ", OriginalStatus: " raw-status ",
		FirstEnteredAt: audienceHistoryTestTime(),
		LastSeenAt:     audienceHistoryTestTime().Add(-time.Hour),
		LastUpdatedAt:  audienceHistoryTestTime(),
		CreatedAt:      audienceHistoryTestTime(),
		UpdatedAt:      audienceHistoryTestTime().Add(-time.Hour),
		ExitedAt:       audienceHistoryPtr(audienceHistoryTestTime().Add(-2 * time.Hour)),
		PayloadDigest:  [32]byte{9},
	}
}

func TestAudienceHistoryDigestsNormalizeTimeAndIncludeTargetID(t *testing.T) {
	t.Run("groups", func(t *testing.T) {
		value := audienceHistoryGroupFixture()
		value.ID = 31
		before, err := HistoricalAudienceGroupDigest(value)
		if err != nil {
			t.Fatal("valid digest rejected")
		}
		value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
		value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
		after, err := HistoricalAudienceGroupDigest(value)
		if err != nil || before != after {
			t.Fatal("timezone or microsecond normalization missing")
		}
		value.ID++
		changed, err := HistoricalAudienceGroupDigest(value)
		if err != nil || changed == before {
			t.Fatal("target ID omitted from digest")
		}
		value.ID = 0
		if _, err = HistoricalAudienceGroupDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("invalid target ID accepted")
		}
	})
	t.Run("packages", func(t *testing.T) {
		value := audienceHistoryPackageFixture()
		value.ID = 31
		before, err := HistoricalAudiencePackageDigest(value)
		if err != nil {
			t.Fatal("valid digest rejected")
		}
		value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
		value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
		normalizedLastIncrementalAt := value.LastIncrementalAt.UTC().Truncate(time.Microsecond)
		value.LastIncrementalAt = &normalizedLastIncrementalAt
		normalizedLastDailyRefreshedAt := value.LastDailyRefreshedAt.UTC().Truncate(time.Microsecond)
		value.LastDailyRefreshedAt = &normalizedLastDailyRefreshedAt
		normalizedNextIncrementalAt := value.NextIncrementalAt.UTC().Truncate(time.Microsecond)
		value.NextIncrementalAt = &normalizedNextIncrementalAt
		normalizedNextDailyAt := value.NextDailyAt.UTC().Truncate(time.Microsecond)
		value.NextDailyAt = &normalizedNextDailyAt
		after, err := HistoricalAudiencePackageDigest(value)
		if err != nil || before != after {
			t.Fatal("timezone or microsecond normalization missing")
		}
		value.ID++
		changed, err := HistoricalAudiencePackageDigest(value)
		if err != nil || changed == before {
			t.Fatal("target ID omitted from digest")
		}
		value.ID = 0
		if _, err = HistoricalAudiencePackageDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("invalid target ID accepted")
		}
		value.ID = 31
		value.RuntimeDigest = [32]byte{}
		if _, err = HistoricalAudiencePackageDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("empty opaque digest accepted")
		}
	})
	t.Run("versions", func(t *testing.T) {
		value := audienceHistoryVersionFixture()
		value.ID = 31
		before, err := HistoricalAudienceVersionDigest(value)
		if err != nil {
			t.Fatal("valid digest rejected")
		}
		value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
		normalizedPublishedAt := value.PublishedAt.UTC().Truncate(time.Microsecond)
		value.PublishedAt = &normalizedPublishedAt
		after, err := HistoricalAudienceVersionDigest(value)
		if err != nil || before != after {
			t.Fatal("timezone or microsecond normalization missing")
		}
		value.ID++
		changed, err := HistoricalAudienceVersionDigest(value)
		if err != nil || changed == before {
			t.Fatal("target ID omitted from digest")
		}
		value.ID = 0
		if _, err = HistoricalAudienceVersionDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("invalid target ID accepted")
		}
		value.ID = 31
		value.DefinitionDigest = [32]byte{}
		if _, err = HistoricalAudienceVersionDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("empty opaque digest accepted")
		}
	})
	t.Run("senders", func(t *testing.T) {
		value := audienceHistorySenderFixture()
		value.ID = 31
		before, err := HistoricalAudienceSenderDigest(value)
		if err != nil {
			t.Fatal("valid digest rejected")
		}
		value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
		value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
		after, err := HistoricalAudienceSenderDigest(value)
		if err != nil || before != after {
			t.Fatal("timezone or microsecond normalization missing")
		}
		value.ID++
		changed, err := HistoricalAudienceSenderDigest(value)
		if err != nil || changed == before {
			t.Fatal("target ID omitted from digest")
		}
		value.ID = 0
		if _, err = HistoricalAudienceSenderDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("invalid target ID accepted")
		}
	})
	t.Run("rules", func(t *testing.T) {
		value := audienceHistoryRuleFixture()
		value.ID = 31
		before, err := HistoricalAudienceRuleDigest(value)
		if err != nil {
			t.Fatal("valid digest rejected")
		}
		value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
		value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
		after, err := HistoricalAudienceRuleDigest(value)
		if err != nil || before != after {
			t.Fatal("timezone or microsecond normalization missing")
		}
		value.ID++
		changed, err := HistoricalAudienceRuleDigest(value)
		if err != nil || changed == before {
			t.Fatal("target ID omitted from digest")
		}
		value.ID = 0
		if _, err = HistoricalAudienceRuleDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("invalid target ID accepted")
		}
	})
	t.Run("rule_versions", func(t *testing.T) {
		value := audienceHistoryRuleVersionFixture()
		value.ID = 31
		before, err := HistoricalAudienceRuleVersionDigest(value)
		if err != nil {
			t.Fatal("valid digest rejected")
		}
		value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
		normalizedPublishedAt := value.PublishedAt.UTC().Truncate(time.Microsecond)
		value.PublishedAt = &normalizedPublishedAt
		after, err := HistoricalAudienceRuleVersionDigest(value)
		if err != nil || before != after {
			t.Fatal("timezone or microsecond normalization missing")
		}
		value.ID++
		changed, err := HistoricalAudienceRuleVersionDigest(value)
		if err != nil || changed == before {
			t.Fatal("target ID omitted from digest")
		}
		value.ID = 0
		if _, err = HistoricalAudienceRuleVersionDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("invalid target ID accepted")
		}
		value.ID = 31
		value.DefinitionDigest = [32]byte{}
		if _, err = HistoricalAudienceRuleVersionDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("empty opaque digest accepted")
		}
	})
	t.Run("definitions", func(t *testing.T) {
		value := audienceHistoryDefinitionFixture()
		value.ID = 31
		before, err := HistoricalAudienceDefinitionDigest(value)
		if err != nil {
			t.Fatal("valid digest rejected")
		}
		value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
		value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
		normalizedLastRefreshedAt := value.LastRefreshedAt.UTC().Truncate(time.Microsecond)
		value.LastRefreshedAt = &normalizedLastRefreshedAt
		after, err := HistoricalAudienceDefinitionDigest(value)
		if err != nil || before != after {
			t.Fatal("timezone or microsecond normalization missing")
		}
		value.ID++
		changed, err := HistoricalAudienceDefinitionDigest(value)
		if err != nil || changed == before {
			t.Fatal("target ID omitted from digest")
		}
		value.ID = 0
		if _, err = HistoricalAudienceDefinitionDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("invalid target ID accepted")
		}
		value.ID = 31
		value.DefinitionDigest = [32]byte{}
		if _, err = HistoricalAudienceDefinitionDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("empty opaque digest accepted")
		}
	})
	t.Run("members", func(t *testing.T) {
		value := audienceHistoryMemberFixture()
		value.ID = 31
		before, err := HistoricalAudienceMemberDigest(value)
		if err != nil {
			t.Fatal("valid digest rejected")
		}
		value.FirstEnteredAt = value.FirstEnteredAt.UTC().Truncate(time.Microsecond)
		value.LastSeenAt = value.LastSeenAt.UTC().Truncate(time.Microsecond)
		value.LastUpdatedAt = value.LastUpdatedAt.UTC().Truncate(time.Microsecond)
		value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
		value.UpdatedAt = value.UpdatedAt.UTC().Truncate(time.Microsecond)
		normalizedExitedAt := value.ExitedAt.UTC().Truncate(time.Microsecond)
		value.ExitedAt = &normalizedExitedAt
		after, err := HistoricalAudienceMemberDigest(value)
		if err != nil || before != after {
			t.Fatal("timezone or microsecond normalization missing")
		}
		value.ID++
		changed, err := HistoricalAudienceMemberDigest(value)
		if err != nil || changed == before {
			t.Fatal("target ID omitted from digest")
		}
		value.ID = 0
		if _, err = HistoricalAudienceMemberDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("invalid target ID accepted")
		}
		value.ID = 31
		value.PayloadDigest = [32]byte{}
		if _, err = HistoricalAudienceMemberDigest(value); err != segmentport.ErrAudienceHistoryInvalid {
			t.Fatal("empty opaque digest accepted")
		}
	})
}

func audienceHistoryTestTime() time.Time {
	return time.Date(2026, 8, 28, 12, 0, 0, 123456789, time.FixedZone("source", 8*3600))
}

func TestAudienceHistoryPreservesStringsNegativeCountersAndPointerInputs(t *testing.T) {
	ctx := context.Background()
	f := newAudienceHistoryFake(ctx)
	w := NewAudienceHistoryWriter(f, f)
	for _, tc := range audienceHistoryCases() {
		if _, err := tc.write(w, ctx, "s", [32]byte{1}, ""); err != nil {
			t.Fatal("fixture failed")
		}
	}
	if f.Group.Name != " group \n " || f.Package.NaturalLanguageDefinition != " 说明\n " || f.Package.IncrementalIntervalSecs != -5 || f.Package.LookbackSecs != -9 || f.Package.QueryMode != " old-mode " || f.Package.IdentityPolicy != " raw-policy " || f.Package.DailyRefreshTime != " raw-time " || f.Package.Timezone != " raw-zone " || f.Version.VersionNumber != -3 || *f.Version.TemplateVersion != -2 || f.Version.AIPrompt != " prompt\n " || f.Sender.Priority != -5 || f.Rule.Description != " desc\n " || f.RuleVersion.Version != -4 || f.RuleVersion.ExecutorType != " raw-executor " || f.Definition.Version != -4 || f.Definition.CachedHeadcount != -6 || f.Definition.UsageCount != -7 || f.Member.IdentityKind != " raw-kind " || f.Member.OriginalStatus != " raw-status " {
		t.Fatal("historical strings or negative facts rewritten")
	}
	// A store must not alter the writer's expected value through shared pointers.
	f = newAudienceHistoryFake(ctx)
	w = NewAudienceHistoryWriter(f, f)
	value := audienceHistoryPackageFixture()
	originalGroup, originalTime := *value.GroupHistoryID, *value.LastDailyRefreshedAt
	f.mutateCreate = func(f *audienceHistoryFake) {
		*f.Package.GroupHistoryID = 77
		*f.Package.LastDailyRefreshedAt = f.Package.LastDailyRefreshedAt.Add(time.Hour)
	}
	if _, err := w.WritePackage(ctx, "s", [32]byte{1}, value); err != segmentport.ErrAudienceHistoryConflict || f.records != 0 {
		t.Fatal("pointer target drift accepted")
	}
	if *value.GroupHistoryID != originalGroup || !value.LastDailyRefreshedAt.Equal(originalTime) {
		t.Fatal("caller pointer changed")
	}
}

func audienceHistoryPtr[T any](value T) *T { return &value }

type audienceHistoryCase struct {
	kind    string
	write   func(*AudienceHistoryWriter, context.Context, string, [32]byte, string) (segmentport.AudienceHistoryReceipt, error)
	drift   func(*audienceHistoryFake)
	driftID func(*audienceHistoryFake)
	digest  func(*audienceHistoryFake) ([32]byte, error)
}

func audienceHistoryCases() []audienceHistoryCase {
	return []audienceHistoryCase{
		{kind: "groups",
			write: func(w *AudienceHistoryWriter, ctx context.Context, source string, payload [32]byte, change string) (segmentport.AudienceHistoryReceipt, error) {
				v := audienceHistoryGroupFixture()
				switch change {
				case "input_id":
					v.ID = 99
				case "source_id":
					v.SourceID = 0
				case "reference":
					v.SourceID = 0
				case "input_drift":
					v.SourceID++
				case "nulls":
				}
				return w.WriteGroup(ctx, source, payload, v)
			},
			drift:   func(f *audienceHistoryFake) { f.Group.SourceID++ },
			driftID: func(f *audienceHistoryFake) { f.Group.ID++ },
			digest:  func(f *audienceHistoryFake) ([32]byte, error) { return HistoricalAudienceGroupDigest(f.Group) },
		},
		{kind: "packages",
			write: func(w *AudienceHistoryWriter, ctx context.Context, source string, payload [32]byte, change string) (segmentport.AudienceHistoryReceipt, error) {
				v := audienceHistoryPackageFixture()
				switch change {
				case "input_id":
					v.ID = 99
				case "source_id":
					v.SourceID = 0
				case "reference":
					v.GroupHistoryID = audienceHistoryPtr(int64(0))
				case "input_drift":
					v.SourceID++
				case "nulls":
					v.GroupHistoryID = nil
					v.CurrentVersionSourceID = nil
					v.LastIncrementalAt = nil
					v.LastDailyRefreshedAt = nil
					v.NextIncrementalAt = nil
					v.NextDailyAt = nil
				case "zero_digest":
					v.RuntimeDigest = [32]byte{}
				}
				return w.WritePackage(ctx, source, payload, v)
			},
			drift:   func(f *audienceHistoryFake) { f.Package.SourceID++ },
			driftID: func(f *audienceHistoryFake) { f.Package.ID++ },
			digest:  func(f *audienceHistoryFake) ([32]byte, error) { return HistoricalAudiencePackageDigest(f.Package) },
		},
		{kind: "versions",
			write: func(w *AudienceHistoryWriter, ctx context.Context, source string, payload [32]byte, change string) (segmentport.AudienceHistoryReceipt, error) {
				v := audienceHistoryVersionFixture()
				switch change {
				case "input_id":
					v.ID = 99
				case "source_id":
					v.SourceID = 0
				case "reference":
					v.PackageHistoryID = 0
				case "input_drift":
					v.SourceID++
				case "nulls":
					v.PublishedAt = nil
					v.TemplateVersion = nil
				case "zero_digest":
					v.DefinitionDigest = [32]byte{}
				}
				return w.WriteVersion(ctx, source, payload, v)
			},
			drift:   func(f *audienceHistoryFake) { f.Version.SourceID++ },
			driftID: func(f *audienceHistoryFake) { f.Version.ID++ },
			digest:  func(f *audienceHistoryFake) ([32]byte, error) { return HistoricalAudienceVersionDigest(f.Version) },
		},
		{kind: "senders",
			write: func(w *AudienceHistoryWriter, ctx context.Context, source string, payload [32]byte, change string) (segmentport.AudienceHistoryReceipt, error) {
				v := audienceHistorySenderFixture()
				switch change {
				case "input_id":
					v.ID = 99
				case "source_id":
					v.SourceID = 0
				case "reference":
					v.PackageHistoryID = 0
				case "input_drift":
					v.SourceID++
				case "nulls":
					v.StaffID = nil
				}
				return w.WriteSender(ctx, source, payload, v)
			},
			drift:   func(f *audienceHistoryFake) { f.Sender.SourceID++ },
			driftID: func(f *audienceHistoryFake) { f.Sender.ID++ },
			digest:  func(f *audienceHistoryFake) ([32]byte, error) { return HistoricalAudienceSenderDigest(f.Sender) },
		},
		{kind: "rules",
			write: func(w *AudienceHistoryWriter, ctx context.Context, source string, payload [32]byte, change string) (segmentport.AudienceHistoryReceipt, error) {
				v := audienceHistoryRuleFixture()
				switch change {
				case "input_id":
					v.ID = 99
				case "source_id":
					v.SourceID = 0
				case "reference":
					v.OwnerStaffID = audienceHistoryPtr(int64(0))
				case "input_drift":
					v.SourceID++
				case "nulls":
					v.OwnerStaffID = nil
				}
				return w.WriteRule(ctx, source, payload, v)
			},
			drift:   func(f *audienceHistoryFake) { f.Rule.SourceID++ },
			driftID: func(f *audienceHistoryFake) { f.Rule.ID++ },
			digest:  func(f *audienceHistoryFake) ([32]byte, error) { return HistoricalAudienceRuleDigest(f.Rule) },
		},
		{kind: "rule_versions",
			write: func(w *AudienceHistoryWriter, ctx context.Context, source string, payload [32]byte, change string) (segmentport.AudienceHistoryReceipt, error) {
				v := audienceHistoryRuleVersionFixture()
				switch change {
				case "input_id":
					v.ID = 99
				case "source_id":
					v.SourceID = 0
				case "reference":
					v.RuleHistoryID = 0
				case "input_drift":
					v.SourceID++
				case "nulls":
					v.PublishedAt = nil
				case "zero_digest":
					v.DefinitionDigest = [32]byte{}
				}
				return w.WriteRuleVersion(ctx, source, payload, v)
			},
			drift:   func(f *audienceHistoryFake) { f.RuleVersion.SourceID++ },
			driftID: func(f *audienceHistoryFake) { f.RuleVersion.ID++ },
			digest: func(f *audienceHistoryFake) ([32]byte, error) {
				return HistoricalAudienceRuleVersionDigest(f.RuleVersion)
			},
		},
		{kind: "definitions",
			write: func(w *AudienceHistoryWriter, ctx context.Context, source string, payload [32]byte, change string) (segmentport.AudienceHistoryReceipt, error) {
				v := audienceHistoryDefinitionFixture()
				switch change {
				case "input_id":
					v.ID = 99
				case "source_id":
					v.SourceID = 0
				case "reference":
					v.SourceID = 0
				case "input_drift":
					v.SourceID++
				case "nulls":
					v.LastRefreshedAt = nil
				case "zero_digest":
					v.DefinitionDigest = [32]byte{}
				}
				return w.WriteDefinition(ctx, source, payload, v)
			},
			drift:   func(f *audienceHistoryFake) { f.Definition.SourceID++ },
			driftID: func(f *audienceHistoryFake) { f.Definition.ID++ },
			digest: func(f *audienceHistoryFake) ([32]byte, error) {
				return HistoricalAudienceDefinitionDigest(f.Definition)
			},
		},
		{kind: "members",
			write: func(w *AudienceHistoryWriter, ctx context.Context, source string, payload [32]byte, change string) (segmentport.AudienceHistoryReceipt, error) {
				v := audienceHistoryMemberFixture()
				switch change {
				case "input_id":
					v.ID = 99
				case "source_id":
					v.SourceID = 0
				case "reference":
					v.PackageHistoryID = 0
				case "input_drift":
					v.SourceID++
				case "nulls":
					v.CustomerID = nil
					v.ExitedAt = nil
				case "zero_digest":
					v.PayloadDigest = [32]byte{}
				}
				return w.WriteMember(ctx, source, payload, v)
			},
			drift:   func(f *audienceHistoryFake) { f.Member.SourceID++ },
			driftID: func(f *audienceHistoryFake) { f.Member.ID++ },
			digest:  func(f *audienceHistoryFake) ([32]byte, error) { return HistoricalAudienceMemberDigest(f.Member) },
		},
	}
}

func TestAudienceHistoryAllKindsCreateReplayAndDrift(t *testing.T) {
	for _, tc := range audienceHistoryCases() {
		t.Run(tc.kind, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), struct{}{}, tc.kind)
			f := newAudienceHistoryFake(ctx)
			w := NewAudienceHistoryWriter(f, f)
			source, payload := "stable-source", [32]byte{1}
			first, err := tc.write(w, ctx, source, payload, "")
			if err != nil || first.Replayed || first.TargetID != 31 || first.SourceIdentifier != source || first.PayloadDigest != payload || first.TargetDigest == ([32]byte{}) || f.creates != 1 || f.gets != 0 || f.records != 1 {
				t.Fatal("create receipt invalid")
			}
			if f.receipts[tc.kind+":"+source] != first {
				t.Fatal("journal kind mismatch")
			}
			digest, err := tc.digest(f)
			if err != nil || digest != first.TargetDigest {
				t.Fatal("target digest mismatch")
			}
			replay, err := tc.write(w, ctx, source, payload, "")
			if err != nil || !replay.Replayed || replay.TargetDigest != first.TargetDigest || f.creates != 1 || f.gets != 1 || f.records != 1 {
				t.Fatal("replay did not read actual target")
			}
			if _, err = tc.write(w, ctx, source, [32]byte{2}, ""); !errors.Is(err, segmentport.ErrAudienceHistoryConflict) {
				t.Fatal("payload drift accepted")
			}
			if _, err = tc.write(w, ctx, source, payload, "input_drift"); !errors.Is(err, segmentport.ErrAudienceHistoryConflict) {
				t.Fatal("input target drift accepted")
			}
			tc.drift(f)
			if _, err = tc.write(w, ctx, source, payload, ""); !errors.Is(err, segmentport.ErrAudienceHistoryConflict) {
				t.Fatal("stored target drift accepted")
			}
		})
	}
}

func TestAudienceHistoryAllKindsInvalidInputsAndNullableFacts(t *testing.T) {
	for _, tc := range audienceHistoryCases() {
		t.Run(tc.kind, func(t *testing.T) {
			for _, change := range []string{"input_id", "source_id", "reference"} {
				ctx := context.Background()
				f := newAudienceHistoryFake(ctx)
				w := NewAudienceHistoryWriter(f, f)
				if _, err := tc.write(w, ctx, "s", [32]byte{1}, change); !errors.Is(err, segmentport.ErrAudienceHistoryInvalid) || f.loads != 0 || f.creates != 0 {
					t.Fatal("invalid input touched storage")
				}
			}
			for _, input := range []struct {
				source  string
				payload [32]byte
			}{{"", [32]byte{1}}, {"s", [32]byte{}}} {
				ctx := context.Background()
				f := newAudienceHistoryFake(ctx)
				if _, err := tc.write(NewAudienceHistoryWriter(f, f), ctx, input.source, input.payload, ""); !errors.Is(err, segmentport.ErrAudienceHistoryInvalid) || f.loads != 0 {
					t.Fatal("invalid source fact touched journal")
				}
			}
			ctx := context.Background()
			f := newAudienceHistoryFake(ctx)
			w := NewAudienceHistoryWriter(f, f)
			if _, err := tc.write(w, ctx, "s", [32]byte{1}, "nulls"); err != nil {
				t.Fatal("nullable historical facts rejected")
			}
			if replay, err := tc.write(w, ctx, "s", [32]byte{1}, "nulls"); err != nil || !replay.Replayed {
				t.Fatal("nullable historical facts did not replay")
			}
		})
	}
}

func TestAudienceHistoryAllKindsReceiptTamperingAndFailures(t *testing.T) {
	for _, tc := range audienceHistoryCases() {
		t.Run(tc.kind, func(t *testing.T) {
			for _, tamper := range []func(*segmentport.AudienceHistoryReceipt){
				func(r *segmentport.AudienceHistoryReceipt) { r.SourceIdentifier = "other" },
				func(r *segmentport.AudienceHistoryReceipt) { r.PayloadDigest = [32]byte{2} },
				func(r *segmentport.AudienceHistoryReceipt) { r.TargetID = 0 },
				func(r *segmentport.AudienceHistoryReceipt) { r.TargetDigest = [32]byte{} },
				func(r *segmentport.AudienceHistoryReceipt) { r.TargetDigest = [32]byte{2} },
			} {
				ctx := context.Background()
				f := newAudienceHistoryFake(ctx)
				w := NewAudienceHistoryWriter(f, f)
				receipt, err := tc.write(w, ctx, "s", [32]byte{1}, "")
				if err != nil {
					t.Fatal("setup failed")
				}
				tamper(&receipt)
				f.receipts[tc.kind+":s"] = receipt
				if _, err = tc.write(w, ctx, "s", [32]byte{1}, ""); !errors.Is(err, segmentport.ErrAudienceHistoryConflict) || f.creates != 1 || f.records != 1 {
					t.Fatal("receipt tampering accepted")
				}
			}
			for _, stage := range []string{"load", "create", "get", "record", "create_drift", "get_id"} {
				ctx := context.Background()
				f := newAudienceHistoryFake(ctx)
				w := NewAudienceHistoryWriter(f, f)
				if stage == "get" || stage == "get_id" {
					if _, err := tc.write(w, ctx, "s", [32]byte{1}, ""); err != nil {
						t.Fatal("setup failed")
					}
				}
				failure := errors.New("database secret must not escape")
				expected := segmentport.ErrAudienceHistoryUnavailable
				switch stage {
				case "load":
					f.loadErr = failure
				case "create":
					f.createErr = failure
				case "get":
					f.getErr = failure
				case "record":
					f.recordErr = failure
				case "create_drift":
					f.mutateCreate = tc.drift
					expected = segmentport.ErrAudienceHistoryConflict
				case "get_id":
					tc.driftID(f)
					expected = segmentport.ErrAudienceHistoryConflict
				}
				receipt, err := tc.write(w, ctx, "s", [32]byte{1}, "")
				if err != expected || receipt != (segmentport.AudienceHistoryReceipt{}) {
					t.Fatal("failure was exposed or reported as success")
				}
				if stage == "create_drift" && f.records != 0 {
					t.Fatal("drifted create recorded receipt")
				}
			}
			for _, failure := range []error{segmentport.ErrAudienceHistoryInvalid, segmentport.ErrAudienceHistoryConflict} {
				ctx := context.Background()
				f := newAudienceHistoryFake(ctx)
				f.createErr = fmt.Errorf("wrapped: %w", failure)
				if _, err := tc.write(NewAudienceHistoryWriter(f, f), ctx, "s", [32]byte{1}, ""); err != failure {
					t.Fatal("known store error lost")
				}
			}
		})
	}
}

func TestAudienceHistoryMissingDependenciesAndCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := newAudienceHistoryFake(context.Background())
	for _, w := range []*AudienceHistoryWriter{nil, NewAudienceHistoryWriter(nil, f), NewAudienceHistoryWriter(f, nil), NewAudienceHistoryWriter((*audienceHistoryFake)(nil), f)} {
		for _, tc := range audienceHistoryCases() {
			if _, err := tc.write(w, context.Background(), "s", [32]byte{1}, ""); err != segmentport.ErrAudienceHistoryUnavailable {
				t.Fatal("unavailable writer accepted")
			}
		}
	}
	for _, badCtx := range []context.Context{nil, ctx} {
		for _, tc := range audienceHistoryCases() {
			if _, err := tc.write(NewAudienceHistoryWriter(f, f), badCtx, "s", [32]byte{1}, ""); err != segmentport.ErrAudienceHistoryUnavailable {
				t.Fatal("unavailable context accepted")
			}
		}
	}
}
