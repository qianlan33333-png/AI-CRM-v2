package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/userops/domain"
	useropsport "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/port"
)

var testNow = time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

func TestPreviewBatchExcludesDNDAndUsesCanonicalDigest(t *testing.T) {
	repository := &repositoryStub{listDnd: func(_ context.Context, ids []domain.CustomerID) ([]domain.DoNotDisturb, error) {
		if !reflect.DeepEqual(ids, []domain.CustomerID{1, 2, 3}) {
			t.Fatalf("ids = %#v", ids)
		}
		return []domain.DoNotDisturb{testDND(2, "local suppression", 1)}, nil
	}}
	service := testService(&directoryStub{resolve: func(_ context.Context, ids []domain.CustomerID) ([]useropsport.CustomerSummary, error) {
		return []useropsport.CustomerSummary{testCustomer(3), testCustomer(1), testCustomer(2)}, nil
	}}, &detailStub{}, repository, &eventStub{})

	content := testContentSnapshot("planned local text")
	result, err := service.PreviewBatch(context.Background(), useropsport.BatchPreviewInput{CustomerIDs: []domain.CustomerID{3, 1, 2}, Content: contentInputFromSnapshot(content)})
	if err != nil {
		t.Fatalf("PreviewBatch() error = %v", err)
	}
	if !reflect.DeepEqual(result.TargetCustomerIDs, []domain.CustomerID{1, 3}) || result.ExcludedDNDCount != 1 || result.TargetDigest != targetDigest([]domain.CustomerID{1, 3}) || !sameContentSnapshot(result.Content, content) {
		t.Fatalf("result = %#v", result)
	}
	assertLocalSafety(t, result.Safety)
	if repository.lockDndCalls != 0 || repository.listDndCalls != 1 {
		t.Fatalf("DND calls lock/list=%d/%d", repository.lockDndCalls, repository.listDndCalls)
	}
}

func TestCreateLocalPlanStrictReadbackAndNoExternalEffect(t *testing.T) {
	plan := testPlan(71, domain.LocalPlanPendingReview, []domain.CustomerID{1})
	repository := &repositoryStub{
		lockDnd: func(context.Context, []domain.CustomerID) ([]domain.DoNotDisturb, error) {
			return []domain.DoNotDisturb{testDND(2, "local suppression", 1)}, nil
		},
		createPlan: func(_ context.Context, input useropsport.CreateLocalPlanInput, targets []domain.CustomerID, digest string, content domain.ContentSnapshot) (useropsport.PlanMutation, error) {
			if !reflect.DeepEqual(input.CustomerIDs, []domain.CustomerID{1, 2}) || !reflect.DeepEqual(targets, []domain.CustomerID{1}) || digest != plan.TargetDigest || input.ExpectedContentDigest != plan.Content.ContentDigest || !sameContentSnapshot(content, plan.Content) {
				t.Fatalf("create input=%#v targets=%#v digest=%q content=%#v", input, targets, digest, content)
			}
			return useropsport.PlanMutation{PlanID: plan.ID}, nil
		},
		readPlan: func(_ context.Context, id domain.PlanID) (domain.LocalPlan, error) {
			if id != plan.ID {
				t.Fatalf("plan id = %d", id)
			}
			return plan, nil
		},
	}
	events := &eventStub{}
	service := testService(&directoryStub{resolve: customersForIDs}, &detailStub{}, repository, events)

	result, err := service.CreateLocalPlan(context.Background(), useropsport.CreateLocalPlanInput{
		CustomerIDs:           []domain.CustomerID{2, 1},
		ExpectedTargetDigest:  plan.TargetDigest,
		Content:               contentInputFromSnapshot(plan.Content),
		ExpectedContentDigest: plan.Content.ContentDigest,
		State:                 domain.LocalPlanPendingReview,
		ActorID:               19,
		IdempotencyKey:        "userops-plan-0001",
	})
	if err != nil {
		t.Fatalf("CreateLocalPlan() error = %v", err)
	}
	if !reflect.DeepEqual(result.Plan, plan) || repository.lockDndCalls != 1 || repository.createPlanCalls != 1 || repository.readPlanCalls != 1 {
		t.Fatalf("result/calls = %#v %d/%d/%d", result, repository.lockDndCalls, repository.createPlanCalls, repository.readPlanCalls)
	}
	assertLocalSafety(t, result.Safety)
	if len(events.values) != 1 {
		t.Fatalf("events = %#v", events.values)
	}
	event := events.values[0]
	if event.Type != eventLocalPlanCreated || event.PlanID != plan.ID || event.TargetCount != 1 || event.CustomerID != 0 || event.IdempotencyKey != "userops-plan-0001" {
		t.Fatalf("event = %#v", event)
	}
}

func TestCreateLocalPlanReplaySurvivesLaterDNDAndMaterialDrift(t *testing.T) {
	plan := testPlan(72, domain.LocalPlanPendingReview, []domain.CustomerID{1})
	repository := &repositoryStub{replayPlan: func(_ context.Context, _ useropsport.CreateLocalPlanInput, content domain.ContentSnapshot) (useropsport.PlanMutation, error) {
		if !sameContentSnapshot(content, plan.Content) {
			t.Fatalf("content = %#v", content)
		}
		return useropsport.PlanMutation{Mutation: useropsport.Mutation{Replayed: true}, Plan: &plan}, nil
	}}
	service := testServiceWithMaterials(&directoryStub{resolve: func(context.Context, []domain.CustomerID) ([]useropsport.CustomerSummary, error) {
		t.Fatal("directory must not run for replay after drift")
		return nil, nil
	}}, &detailStub{}, &materialStub{image: func(context.Context, int64) (bool, error) {
		t.Fatal("material validation must not run for replay after drift")
		return false, nil
	}}, repository, &eventStub{})
	result, err := service.CreateLocalPlan(context.Background(), useropsport.CreateLocalPlanInput{CustomerIDs: []domain.CustomerID{1}, ExpectedTargetDigest: plan.TargetDigest, Content: contentInputFromSnapshot(plan.Content), ExpectedContentDigest: plan.Content.ContentDigest, State: plan.State, ActorID: 19, IdempotencyKey: "userops-plan-replay"})
	if err != nil || !reflect.DeepEqual(result.Plan, plan) {
		t.Fatalf("result/error = %#v / %v", result, err)
	}
	assertLocalSafety(t, result.Safety)
}

func TestCreateLocalPlanRejectsStalePreviewBeforeWrite(t *testing.T) {
	repository := &repositoryStub{}
	events := &eventStub{}
	service := testService(&directoryStub{resolve: customersForIDs}, &detailStub{}, repository, events)

	content := testContentSnapshot("draft local text")
	_, err := service.CreateLocalPlan(context.Background(), useropsport.CreateLocalPlanInput{
		CustomerIDs:           []domain.CustomerID{1},
		ExpectedTargetDigest:  targetDigest([]domain.CustomerID{2}),
		Content:               contentInputFromSnapshot(content),
		ExpectedContentDigest: content.ContentDigest,
		State:                 domain.LocalPlanDraft,
		ActorID:               19,
		IdempotencyKey:        "userops-plan-0002",
	})
	if !errors.Is(err, useropsport.ErrPreviewStale) {
		t.Fatalf("error = %v", err)
	}
	if repository.createPlanCalls != 0 || len(events.values) != 0 {
		t.Fatalf("write/event calls = %d/%d", repository.createPlanCalls, len(events.values))
	}
}

func TestCreateLocalPlanReplayUsesSnapshotWithoutDuplicateEvent(t *testing.T) {
	plan := testPlan(73, domain.LocalPlanDraft, []domain.CustomerID{1})
	repository := &repositoryStub{
		replayPlan: func(context.Context, useropsport.CreateLocalPlanInput, domain.ContentSnapshot) (useropsport.PlanMutation, error) {
			return useropsport.PlanMutation{Mutation: useropsport.Mutation{Replayed: true}, Plan: &plan}, nil
		},
		readPlan: func(context.Context, domain.PlanID) (domain.LocalPlan, error) {
			t.Fatal("strict read is not allowed for an idempotent snapshot replay")
			return domain.LocalPlan{}, nil
		},
	}
	events := &eventStub{}
	service := testService(&directoryStub{resolve: customersForIDs}, &detailStub{}, repository, events)

	result, err := service.CreateLocalPlan(context.Background(), useropsport.CreateLocalPlanInput{
		CustomerIDs:           []domain.CustomerID{1},
		ExpectedTargetDigest:  plan.TargetDigest,
		Content:               contentInputFromSnapshot(plan.Content),
		ExpectedContentDigest: plan.Content.ContentDigest,
		State:                 domain.LocalPlanDraft,
		ActorID:               19,
		IdempotencyKey:        "userops-plan-replay",
	})
	if err != nil || result.Plan.ID != plan.ID || len(events.values) != 0 {
		t.Fatalf("result/error/events = %#v / %v / %#v", result, err, events.values)
	}
}

func TestCreateLocalPlanRejectsTamperedContentBeforeWrite(t *testing.T) {
	previewContent := testContentSnapshot("original local text")
	tampered := contentInputFromSnapshot(previewContent)
	tampered.Text = "changed local text"
	repository := &repositoryStub{}
	events := &eventStub{}
	service := testService(&directoryStub{resolve: customersForIDs}, &detailStub{}, repository, events)

	_, err := service.CreateLocalPlan(context.Background(), useropsport.CreateLocalPlanInput{
		CustomerIDs:           []domain.CustomerID{1},
		ExpectedTargetDigest:  targetDigest([]domain.CustomerID{1}),
		Content:               tampered,
		ExpectedContentDigest: previewContent.ContentDigest,
		State:                 domain.LocalPlanPendingReview,
		ActorID:               19,
		IdempotencyKey:        "userops-plan-tampered",
	})
	if !errors.Is(err, useropsport.ErrPreviewStale) {
		t.Fatalf("error = %v", err)
	}
	if repository.createPlanCalls != 0 || len(events.values) != 0 {
		t.Fatalf("write/event calls = %d/%d", repository.createPlanCalls, len(events.values))
	}
}

func TestCreateLocalPlanRejectsExpiredMaterialBeforeWrite(t *testing.T) {
	content, err := normalizeContent(domain.ContentInput{
		Text:                  "planned local text",
		ImageLibraryIDs:       []int64{11},
		MiniProgramLibraryIDs: []int64{12},
		AttachmentLibraryIDs:  []int64{13},
	})
	if err != nil {
		t.Fatalf("normalizeContent() error = %v", err)
	}
	repository := &repositoryStub{}
	events := &eventStub{}
	materials := &materialStub{
		image:       func(context.Context, int64) (bool, error) { return true, nil },
		miniProgram: func(context.Context, int64) (bool, error) { return true, nil },
		attachment:  func(context.Context, int64) (bool, error) { return false, nil },
	}
	service := testServiceWithMaterials(&directoryStub{resolve: customersForIDs}, &detailStub{}, materials, repository, events)

	_, err = service.CreateLocalPlan(context.Background(), useropsport.CreateLocalPlanInput{
		CustomerIDs:           []domain.CustomerID{1},
		ExpectedTargetDigest:  targetDigest([]domain.CustomerID{1}),
		Content:               contentInputFromSnapshot(content),
		ExpectedContentDigest: content.ContentDigest,
		State:                 domain.LocalPlanPendingReview,
		ActorID:               19,
		IdempotencyKey:        "userops-plan-expired",
	})
	if !errors.Is(err, useropsport.ErrConflict) {
		t.Fatalf("error = %v", err)
	}
	if repository.createPlanCalls != 0 || len(events.values) != 0 {
		t.Fatalf("write/event calls = %d/%d", repository.createPlanCalls, len(events.values))
	}
}

func TestSetDNDStrictReadbackAndReplay(t *testing.T) {
	t.Run("fresh", func(t *testing.T) {
		dnd := testDND(7, "requested locally", 2)
		repository := &repositoryStub{
			upsertDnd: func(_ context.Context, input useropsport.UpsertDNDInput) (useropsport.DNDMutation, error) {
				if input.Reason != "requested locally" || input.ExpectedVersion == nil || *input.ExpectedVersion != 1 {
					t.Fatalf("input = %#v", input)
				}
				return useropsport.DNDMutation{}, nil
			},
			readDnd: func(_ context.Context, id domain.CustomerID) (*domain.DoNotDisturb, error) {
				if id != 7 {
					t.Fatalf("id = %d", id)
				}
				return &dnd, nil
			},
		}
		events := &eventStub{}
		service := testService(&directoryStub{}, &detailStub{read: func(context.Context, domain.CustomerID) (useropsport.CustomerDetail, error) {
			return testDetail(7), nil
		}}, repository, events)

		result, err := service.SetDND(context.Background(), useropsport.UpsertDNDInput{CustomerID: 7, Reason: " requested locally ", ExpectedVersion: pointer(int64(1)), ActorID: 9, IdempotencyKey: "userops-dnd-fresh"})
		if err != nil || result.DND == nil || result.DND.Reason != dnd.Reason || len(events.values) != 1 || events.values[0].Type != eventDNDSet {
			t.Fatalf("result/error/events = %#v / %v / %#v", result, err, events.values)
		}
		assertLocalSafety(t, result.Safety)
	})

	t.Run("replay", func(t *testing.T) {
		dnd := testDND(7, "requested locally", 2)
		repository := &repositoryStub{
			upsertDnd: func(context.Context, useropsport.UpsertDNDInput) (useropsport.DNDMutation, error) {
				return useropsport.DNDMutation{Mutation: useropsport.Mutation{Replayed: true}, DND: &dnd}, nil
			},
			readDnd: func(context.Context, domain.CustomerID) (*domain.DoNotDisturb, error) {
				t.Fatal("replay must not use mutable current DND state")
				return nil, nil
			},
		}
		events := &eventStub{}
		service := testService(&directoryStub{}, &detailStub{read: func(context.Context, domain.CustomerID) (useropsport.CustomerDetail, error) {
			return testDetail(7), nil
		}}, repository, events)

		result, err := service.SetDND(context.Background(), useropsport.UpsertDNDInput{CustomerID: 7, Reason: "requested locally", ActorID: 9, IdempotencyKey: "userops-dnd-replay"})
		if err != nil || result.DND == nil || len(events.values) != 0 {
			t.Fatalf("result/error/events = %#v / %v / %#v", result, err, events.values)
		}
	})
}

func TestClearDNDStrictReadbackAndReplay(t *testing.T) {
	t.Run("fresh", func(t *testing.T) {
		repository := &repositoryStub{
			clearDnd: func(context.Context, useropsport.ClearDNDInput) (useropsport.DNDMutation, error) {
				return useropsport.DNDMutation{Cleared: true}, nil
			},
			readDnd: func(context.Context, domain.CustomerID) (*domain.DoNotDisturb, error) { return nil, nil },
		}
		events := &eventStub{}
		result, err := testService(&directoryStub{}, &detailStub{}, repository, events).ClearDND(context.Background(), useropsport.ClearDNDInput{CustomerID: 7, ExpectedVersion: 2, ActorID: 9, IdempotencyKey: "userops-dnd-clear"})
		if err != nil || !result.Cleared || result.DND != nil || len(events.values) != 1 || events.values[0].Type != eventDNDCleared {
			t.Fatalf("result/error/events = %#v / %v / %#v", result, err, events.values)
		}
	})

	t.Run("replay", func(t *testing.T) {
		repository := &repositoryStub{
			clearDnd: func(context.Context, useropsport.ClearDNDInput) (useropsport.DNDMutation, error) {
				return useropsport.DNDMutation{Mutation: useropsport.Mutation{Replayed: true}, Cleared: true}, nil
			},
			readDnd: func(context.Context, domain.CustomerID) (*domain.DoNotDisturb, error) {
				t.Fatal("replay must use durable clear snapshot")
				return nil, nil
			},
		}
		events := &eventStub{}
		result, err := testService(&directoryStub{}, &detailStub{}, repository, events).ClearDND(context.Background(), useropsport.ClearDNDInput{CustomerID: 7, ExpectedVersion: 2, ActorID: 9, IdempotencyKey: "userops-dnd-clear-replay"})
		if err != nil || !result.Cleared || len(events.values) != 0 {
			t.Fatalf("result/error/events = %#v / %v / %#v", result, err, events.values)
		}
	})
}

func TestSafeExportUsesClosedWhitelistAndNeutralizesSpreadsheetFormula(t *testing.T) {
	directory := &directoryStub{list: func(context.Context, useropsport.DirectoryQuery) (useropsport.DirectoryPageRead, error) {
		return useropsport.DirectoryPageRead{Items: []useropsport.CustomerSummary{{CustomerID: 9, Name: "\t=2+2\r\nnext"}}, Total: 1}, nil
	}}
	service := testService(directory, &detailStub{}, &repositoryStub{}, &eventStub{})
	result, err := service.SafeExport(context.Background(), useropsport.SafeExportRequest{Query: useropsport.DirectoryQuery{Limit: 1}, Fields: []useropsport.SafeExportField{useropsport.SafeExportCustomerID, useropsport.SafeExportName}})
	if err != nil || !reflect.DeepEqual(result.Rows, [][]string{{"9", "' =2+2  next"}}) {
		t.Fatalf("result/error = %#v / %v", result, err)
	}
	assertLocalSafety(t, result.Safety)

	_, err = service.SafeExport(context.Background(), useropsport.SafeExportRequest{Query: useropsport.DirectoryQuery{}, Fields: []useropsport.SafeExportField{"phone"}})
	if !errors.Is(err, useropsport.ErrInvalid) {
		t.Fatalf("unknown export field error = %v", err)
	}
}

func TestDirectoryPreservesSupportedFiltersWithoutPhoneProjection(t *testing.T) {
	ownerID, stageID, channelID, tagID := int64(11), int64(12), int64(13), int64(14)
	directory := &directoryStub{list: func(_ context.Context, query useropsport.DirectoryQuery) (useropsport.DirectoryPageRead, error) {
		if query.Keyword != "founder" || query.OwnerStaffID == nil || *query.OwnerStaffID != ownerID || query.StageID == nil || *query.StageID != stageID || query.ChannelID == nil || *query.ChannelID != channelID || query.TagID == nil || *query.TagID != tagID || query.PhoneExact != "13800138000" || query.Limit != useropsport.DefaultPageLimit {
			t.Fatalf("query = %#v", query)
		}
		return useropsport.DirectoryPageRead{Items: []useropsport.CustomerSummary{testCustomer(7)}, Total: 1}, nil
	}}
	service := testService(directory, &detailStub{}, &repositoryStub{}, &eventStub{})
	result, err := service.ListCustomers(context.Background(), useropsport.DirectoryQuery{
		Keyword:      " founder ",
		OwnerStaffID: &ownerID,
		StageID:      &stageID,
		ChannelID:    &channelID,
		TagID:        &tagID,
		PhoneExact:   "13800138000",
	})
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("result/error = %#v / %v", result, err)
	}

	raw, err := json.Marshal(result.Items[0])
	if err != nil || strings.Contains(string(raw), "13800138000") {
		t.Fatalf("customer JSON/error = %s / %v", raw, err)
	}
}

func TestReusedCustomerAndTagIDsEncodeAsJSONNumbers(t *testing.T) {
	ownerID, stageID, channelID, groupID := int64(2), int64(3), int64(4), int64(6)
	detail := useropsport.CustomerDetail{
		Customer: useropsport.CustomerSummary{CustomerID: 1, Name: "customer", OwnerStaffID: &ownerID, StageID: &stageID, ChannelID: &channelID},
		Tags:     []useropsport.CustomerTag{{ID: 5, GroupID: &groupID, Name: "tag"}},
		Timeline: []useropsport.TimelineEntry{},
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	assertJSONNumber(t, value["customer"], "customer_id", "1")
	assertJSONNumber(t, value["customer"], "owner_staff_id", "2")
	assertJSONNumber(t, value["customer"], "stage_id", "3")
	assertJSONNumber(t, value["customer"], "channel_id", "4")
	var tags []json.RawMessage
	if err := json.Unmarshal(value["tags"], &tags); err != nil || len(tags) != 1 {
		t.Fatalf("tags/error = %s / %v", value["tags"], err)
	}
	assertJSONNumber(t, tags[0], "id", "5")
	assertJSONNumber(t, tags[0], "group_id", "6")
}

func TestOverviewListDetailAndSendRecordsFailClosedOnUnsafeShapes(t *testing.T) {
	t.Run("overview", func(t *testing.T) {
		service := testService(&directoryStub{overview: func(context.Context, useropsport.DirectoryQuery) (useropsport.DirectoryOverviewRead, error) {
			return useropsport.DirectoryOverviewRead{CustomerCount: 4}, nil
		}}, &detailStub{}, &repositoryStub{overview: func(context.Context) (useropsport.LocalOverviewRead, error) {
			return useropsport.LocalOverviewRead{ActiveDNDCount: 1, DraftPlanCount: 2, PendingReviewPlanCount: 3}, nil
		}}, &eventStub{})
		result, err := service.Overview(context.Background(), useropsport.DirectoryQuery{})
		if err != nil || result.CustomerCount != 4 || result.ActiveDNDCount != 1 {
			t.Fatalf("result/error = %#v / %v", result, err)
		}
		assertLocalSafety(t, result.Safety)
	})

	t.Run("detail wrong customer", func(t *testing.T) {
		service := testService(&directoryStub{}, &detailStub{read: func(context.Context, domain.CustomerID) (useropsport.CustomerDetail, error) {
			return testDetail(8), nil
		}}, &repositoryStub{}, &eventStub{})
		_, err := service.GetCustomerDetail(context.Background(), 7)
		if !errors.Is(err, useropsport.ErrUnavailable) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("send records", func(t *testing.T) {
		plan := testPlan(81, domain.LocalPlanDraft, []domain.CustomerID{7})
		service := testService(&directoryStub{}, &detailStub{}, &repositoryStub{records: func(context.Context, useropsport.SendRecordQuery) (useropsport.SendRecordPageRead, error) {
			return useropsport.SendRecordPageRead{Items: []domain.SendRecord{{ID: 1, PlanID: plan.ID, CustomerID: 7, TechnicalStatus: domain.SendTechnicalStateNotDispatched, CreatedAt: testNow, UpdatedAt: testNow}}, Total: 1}, nil
		}}, &eventStub{})
		result, err := service.ListSendRecords(context.Background(), useropsport.SendRecordQuery{PlanID: plan.ID})
		if err != nil || len(result.Items) != 1 || result.Items[0].TechnicalStatus != domain.SendTechnicalStateNotDispatched {
			t.Fatalf("result/error = %#v / %v", result, err)
		}
		assertLocalSafety(t, result.Safety)
	})
}

func TestRejectsUnsafeInputsBeforeDependencies(t *testing.T) {
	service := testService(&directoryStub{}, &detailStub{}, &repositoryStub{}, &eventStub{})
	for _, input := range []useropsport.BatchPreviewInput{
		{CustomerIDs: nil},
		{CustomerIDs: []domain.CustomerID{1, 1}},
		{CustomerIDs: []domain.CustomerID{0}},
	} {
		if _, err := service.PreviewBatch(context.Background(), input); !errors.Is(err, useropsport.ErrInvalid) {
			t.Fatalf("input %#v error = %v", input, err)
		}
	}
	overLimit := make([]domain.CustomerID, useropsport.MaximumBatchSize+1)
	for index := range overLimit {
		overLimit[index] = domain.CustomerID(index + 1)
	}
	if _, err := service.PreviewBatch(context.Background(), useropsport.BatchPreviewInput{CustomerIDs: overLimit}); !errors.Is(err, useropsport.ErrInvalid) {
		t.Fatalf("over-limit preview error = %v", err)
	}
	if _, err := service.SetDND(context.Background(), useropsport.UpsertDNDInput{CustomerID: 1, Reason: "bad\nreason", ActorID: 2, IdempotencyKey: "userops-dnd-key"}); !errors.Is(err, useropsport.ErrInvalid) {
		t.Fatalf("DND input error = %v", err)
	}
	emptyContent := testContentSnapshot("")
	if _, err := service.CreateLocalPlan(context.Background(), useropsport.CreateLocalPlanInput{
		CustomerIDs:           []domain.CustomerID{1},
		ExpectedTargetDigest:  targetDigest([]domain.CustomerID{1}),
		ExpectedContentDigest: emptyContent.ContentDigest,
		State:                 domain.LocalPlanPendingReview,
		ActorID:               2,
		IdempotencyKey:        "userops-plan-empty-content",
	}); !errors.Is(err, useropsport.ErrInvalid) {
		t.Fatalf("empty pending-review content error = %v", err)
	}
}

func testService(directory useropsport.CustomerDirectoryReader, details useropsport.CustomerDetailReader, repository useropsport.Repository, events useropsport.EventAppender) *Service {
	return testServiceWithMaterials(directory, details, &materialStub{}, repository, events)
}

func testServiceWithMaterials(directory useropsport.CustomerDirectoryReader, details useropsport.CustomerDetailReader, materials useropsport.MaterialReader, repository useropsport.Repository, events useropsport.EventAppender) *Service {
	service := NewService(uowStub{}, directory, details, materials, repository, events)
	service.now = func() time.Time { return testNow }
	return service
}

func testCustomer(id domain.CustomerID) useropsport.CustomerSummary {
	return useropsport.CustomerSummary{CustomerID: id, Name: "customer", AddedAt: pointer(testNow), LastInteractAt: pointer(testNow)}
}

func testDetail(id domain.CustomerID) useropsport.CustomerDetail {
	return useropsport.CustomerDetail{Customer: testCustomer(id), Tags: []useropsport.CustomerTag{}, Timeline: []useropsport.TimelineEntry{}}
}

func testDND(id domain.CustomerID, reason string, version int64) domain.DoNotDisturb {
	return domain.DoNotDisturb{CustomerID: id, Reason: reason, Version: version, CreatedAt: testNow, UpdatedAt: testNow}
}

func testPlan(id domain.PlanID, state domain.LocalPlanState, targets []domain.CustomerID) domain.LocalPlan {
	return domain.LocalPlan{ID: id, State: state, Content: testContentSnapshot("planned local text"), TargetDigest: targetDigest(targets), TargetCount: int32(len(targets)), Version: 1, CreatedAt: testNow, UpdatedAt: testNow}
}

func testContentSnapshot(text string) domain.ContentSnapshot {
	content, err := normalizeContent(domain.ContentInput{Text: text})
	if err != nil {
		panic(err)
	}
	return content
}

func customersForIDs(_ context.Context, ids []domain.CustomerID) ([]useropsport.CustomerSummary, error) {
	items := make([]useropsport.CustomerSummary, len(ids))
	for index, id := range ids {
		items[index] = testCustomer(id)
	}
	return items, nil
}

func assertLocalSafety(t *testing.T, safety useropsport.Safety) {
	t.Helper()
	if safety.ProviderExecutionEligible || safety.RealExternalCallExecuted || safety.DeliveryProven {
		t.Fatalf("safety = %#v", safety)
	}
}

func pointer[T any](value T) *T { return &value }

func assertJSONNumber(t *testing.T, raw json.RawMessage, field, want string) {
	t.Helper()
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", raw, err)
	}
	if string(value[field]) != want {
		t.Fatalf("%s = %s, want JSON number %s", field, value[field], want)
	}
}

type uowStub struct{}

func (uowStub) Within(ctx context.Context, callback func(context.Context) error) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	return callback(ctx)
}

type directoryStub struct {
	overview func(context.Context, useropsport.DirectoryQuery) (useropsport.DirectoryOverviewRead, error)
	list     func(context.Context, useropsport.DirectoryQuery) (useropsport.DirectoryPageRead, error)
	resolve  func(context.Context, []domain.CustomerID) ([]useropsport.CustomerSummary, error)
}

func (stub *directoryStub) ReadOverview(ctx context.Context, input useropsport.DirectoryQuery) (useropsport.DirectoryOverviewRead, error) {
	if stub.overview == nil {
		return useropsport.DirectoryOverviewRead{}, nil
	}
	return stub.overview(ctx, input)
}

func (stub *directoryStub) ListCustomers(ctx context.Context, input useropsport.DirectoryQuery) (useropsport.DirectoryPageRead, error) {
	if stub.list == nil {
		return useropsport.DirectoryPageRead{}, nil
	}
	return stub.list(ctx, input)
}

func (stub *directoryStub) ResolveCustomers(ctx context.Context, ids []domain.CustomerID) ([]useropsport.CustomerSummary, error) {
	if stub.resolve == nil {
		return customersForIDs(ctx, ids)
	}
	return stub.resolve(ctx, ids)
}

type detailStub struct {
	read func(context.Context, domain.CustomerID) (useropsport.CustomerDetail, error)
}

func (stub *detailStub) ReadCustomerDetail(ctx context.Context, id domain.CustomerID) (useropsport.CustomerDetail, error) {
	if stub.read == nil {
		return testDetail(id), nil
	}
	return stub.read(ctx, id)
}

type repositoryStub struct {
	overview        func(context.Context) (useropsport.LocalOverviewRead, error)
	readDnd         func(context.Context, domain.CustomerID) (*domain.DoNotDisturb, error)
	listDnd         func(context.Context, []domain.CustomerID) ([]domain.DoNotDisturb, error)
	lockDnd         func(context.Context, []domain.CustomerID) ([]domain.DoNotDisturb, error)
	upsertDnd       func(context.Context, useropsport.UpsertDNDInput) (useropsport.DNDMutation, error)
	clearDnd        func(context.Context, useropsport.ClearDNDInput) (useropsport.DNDMutation, error)
	replayPlan      func(context.Context, useropsport.CreateLocalPlanInput, domain.ContentSnapshot) (useropsport.PlanMutation, error)
	createPlan      func(context.Context, useropsport.CreateLocalPlanInput, []domain.CustomerID, string, domain.ContentSnapshot) (useropsport.PlanMutation, error)
	readPlan        func(context.Context, domain.PlanID) (domain.LocalPlan, error)
	records         func(context.Context, useropsport.SendRecordQuery) (useropsport.SendRecordPageRead, error)
	listDndCalls    int
	lockDndCalls    int
	createPlanCalls int
	readPlanCalls   int
}

func (stub *repositoryStub) ReadLocalOverview(ctx context.Context) (useropsport.LocalOverviewRead, error) {
	if stub.overview == nil {
		return useropsport.LocalOverviewRead{}, nil
	}
	return stub.overview(ctx)
}

func (stub *repositoryStub) ReadDND(ctx context.Context, id domain.CustomerID) (*domain.DoNotDisturb, error) {
	if stub.readDnd == nil {
		return nil, nil
	}
	return stub.readDnd(ctx, id)
}

func (stub *repositoryStub) ListActiveDND(ctx context.Context, ids []domain.CustomerID) ([]domain.DoNotDisturb, error) {
	stub.listDndCalls++
	if stub.listDnd == nil {
		return nil, nil
	}
	return stub.listDnd(ctx, ids)
}

func (stub *repositoryStub) LockActiveDND(ctx context.Context, ids []domain.CustomerID) ([]domain.DoNotDisturb, error) {
	stub.lockDndCalls++
	if stub.lockDnd == nil {
		return nil, nil
	}
	return stub.lockDnd(ctx, ids)
}

func (stub *repositoryStub) UpsertDND(ctx context.Context, input useropsport.UpsertDNDInput) (useropsport.DNDMutation, error) {
	if stub.upsertDnd == nil {
		return useropsport.DNDMutation{}, nil
	}
	return stub.upsertDnd(ctx, input)
}

func (stub *repositoryStub) ClearDND(ctx context.Context, input useropsport.ClearDNDInput) (useropsport.DNDMutation, error) {
	if stub.clearDnd == nil {
		return useropsport.DNDMutation{Cleared: true}, nil
	}
	return stub.clearDnd(ctx, input)
}

func (stub *repositoryStub) ReplayLocalPlan(ctx context.Context, input useropsport.CreateLocalPlanInput, content domain.ContentSnapshot) (useropsport.PlanMutation, error) {
	if stub.replayPlan == nil {
		return useropsport.PlanMutation{}, nil
	}
	return stub.replayPlan(ctx, input, content)
}

func (stub *repositoryStub) CreateLocalPlan(ctx context.Context, input useropsport.CreateLocalPlanInput, targets []domain.CustomerID, digest string, content domain.ContentSnapshot) (useropsport.PlanMutation, error) {
	stub.createPlanCalls++
	if stub.createPlan == nil {
		return useropsport.PlanMutation{}, useropsport.ErrUnavailable
	}
	return stub.createPlan(ctx, input, targets, digest, content)
}

func (stub *repositoryStub) ReadLocalPlan(ctx context.Context, id domain.PlanID) (domain.LocalPlan, error) {
	stub.readPlanCalls++
	if stub.readPlan == nil {
		return domain.LocalPlan{}, useropsport.ErrUnavailable
	}
	return stub.readPlan(ctx, id)
}

func (stub *repositoryStub) ListSendRecords(ctx context.Context, input useropsport.SendRecordQuery) (useropsport.SendRecordPageRead, error) {
	if stub.records == nil {
		return useropsport.SendRecordPageRead{}, nil
	}
	return stub.records(ctx, input)
}

type eventStub struct {
	values []useropsport.LocalEvent
	err    error
}

func (stub *eventStub) Append(_ context.Context, event useropsport.LocalEvent) error {
	if stub.err != nil {
		return stub.err
	}
	stub.values = append(stub.values, event)
	return nil
}

type materialStub struct {
	image       func(context.Context, int64) (bool, error)
	miniProgram func(context.Context, int64) (bool, error)
	attachment  func(context.Context, int64) (bool, error)
}

func (stub *materialStub) ImageEligible(ctx context.Context, id int64) (bool, error) {
	if stub.image == nil {
		return true, nil
	}
	return stub.image(ctx, id)
}

func (stub *materialStub) MiniProgramEligible(ctx context.Context, id int64) (bool, error) {
	if stub.miniProgram == nil {
		return true, nil
	}
	return stub.miniProgram(ctx, id)
}

func (stub *materialStub) AttachmentEligible(ctx context.Context, id int64) (bool, error) {
	if stub.attachment == nil {
		return true, nil
	}
	return stub.attachment(ctx, id)
}
