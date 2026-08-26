package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

type initiationTransactionKey struct{}

type initiationTransaction struct {
	id        int
	rollbacks []func()
}

type initiationUoW struct {
	mu     sync.Mutex
	nextID int
}

func (uow *initiationUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.mu.Lock()
	defer uow.mu.Unlock()
	uow.nextID++
	transaction := &initiationTransaction{id: uow.nextID}
	err := callback(context.WithValue(ctx, initiationTransactionKey{}, transaction))
	if err != nil {
		for index := len(transaction.rollbacks) - 1; index >= 0; index-- {
			transaction.rollbacks[index]()
		}
	}
	return err
}

func initiationTransactionID(ctx context.Context) int {
	transaction, _ := ctx.Value(initiationTransactionKey{}).(*initiationTransaction)
	if transaction == nil {
		return 0
	}
	return transaction.id
}

func registerInitiationRollback(ctx context.Context, rollback func()) {
	transaction, _ := ctx.Value(initiationTransactionKey{}).(*initiationTransaction)
	if transaction != nil {
		transaction.rollbacks = append(transaction.rollbacks, rollback)
	}
}

type initiationDraftStub struct {
	mu    sync.Mutex
	fact  campaignport.CampaignDraftFact
	err   error
	calls int
	txIDs []int
}

func (stub *initiationDraftStub) LockDraftCampaign(ctx context.Context, code string) (campaignport.CampaignDraftFact, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.calls++
	stub.txIDs = append(stub.txIDs, initiationTransactionID(ctx))
	if stub.err != nil {
		return campaignport.CampaignDraftFact{}, stub.err
	}
	if code != stub.fact.CampaignCode {
		return campaignport.CampaignDraftFact{}, campaignport.ErrSourceFactsUnavailable
	}
	result := stub.fact
	result.Steps = append([]campaign.Step(nil), stub.fact.Steps...)
	return result, nil
}

type initiationSourceStub struct {
	mu         sync.Mutex
	resolution campaignport.SourceResolution
	err        error
	calls      int
	txIDs      []int
	requests   []campaign.InitiationSourceRequest
}

func (stub *initiationSourceStub) ResolveCampaignTargets(ctx context.Context, request campaign.InitiationSourceRequest) (campaignport.SourceResolution, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.calls++
	stub.txIDs = append(stub.txIDs, initiationTransactionID(ctx))
	stub.requests = append(stub.requests, request)
	if stub.err != nil {
		return campaignport.SourceResolution{}, stub.err
	}
	if !request.Matches(stub.resolution.Source) {
		return campaignport.SourceResolution{}, campaignport.ErrSourceFactsUnavailable
	}
	result := stub.resolution
	result.CustomerIDs = append([]int64(nil), result.CustomerIDs...)
	return result, nil
}

type initiationEligibilityStub struct {
	mu        sync.Mutex
	decisions []campaignport.EligibilityDecision
	err       error
	calls     int
	txIDs     []int
	requests  []campaignport.EligibilityRequest
}

func (stub *initiationEligibilityStub) CheckCampaignEligibility(ctx context.Context, request campaignport.EligibilityRequest) ([]campaignport.EligibilityDecision, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.calls++
	stub.txIDs = append(stub.txIDs, initiationTransactionID(ctx))
	copyRequest := request
	copyRequest.CustomerIDs = append([]int64(nil), request.CustomerIDs...)
	stub.requests = append(stub.requests, copyRequest)
	if stub.err != nil {
		return nil, stub.err
	}
	return append([]campaignport.EligibilityDecision(nil), stub.decisions...), nil
}

type initiationRepositoryStub struct {
	mu         sync.Mutex
	nextID     int64
	receipts   map[string]campaignport.CreateReceipt
	plans      map[string]campaign.DraftTouchPlan
	reviews    map[string]campaign.TouchPlanReview
	reserveTx  []int
	saveTx     []int
	completeTx []int
	readTx     []int
	readAlter  func(campaign.DraftTouchPlan) campaign.DraftTouchPlan
}

func newInitiationRepository() *initiationRepositoryStub {
	return &initiationRepositoryStub{receipts: map[string]campaignport.CreateReceipt{}, plans: map[string]campaign.DraftTouchPlan{}, reviews: map[string]campaign.TouchPlanReview{}}
}

func (stub *initiationRepositoryStub) ReserveDraftCreate(ctx context.Context, reservation campaignport.CreateReservation) (campaignport.CreateReceipt, bool, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	transactionID := initiationTransactionID(ctx)
	stub.reserveTx = append(stub.reserveTx, transactionID)
	if transactionID < 1 {
		return campaignport.CreateReceipt{}, false, errors.New("reserve outside transaction")
	}
	key := receiptKey(reservation.ActorID, reservation.KeyDigest)
	if receipt, exists := stub.receipts[key]; exists {
		return receipt, false, nil
	}
	stub.nextID++
	receipt := campaignport.CreateReceipt{ID: stub.nextID, ActorID: reservation.ActorID, KeyDigest: reservation.KeyDigest, PayloadDigest: reservation.PayloadDigest, PlanID: reservation.PlanID}
	stub.receipts[key] = receipt
	registerInitiationRollback(ctx, func() {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		delete(stub.receipts, key)
	})
	return receipt, true, nil
}

func (stub *initiationRepositoryStub) SaveDraftTouchPlan(ctx context.Context, plan campaign.DraftTouchPlan) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	transactionID := initiationTransactionID(ctx)
	stub.saveTx = append(stub.saveTx, transactionID)
	if transactionID < 1 {
		return errors.New("save outside transaction")
	}
	if _, exists := stub.plans[plan.ID]; exists {
		return errors.New("duplicate plan")
	}
	stub.plans[plan.ID] = campaign.CloneDraftTouchPlan(plan)
	stub.reviews[plan.ID] = campaign.TouchPlanReview{PlanID: plan.ID, CampaignCode: plan.CampaignCode, Status: campaign.TouchPlanReviewDraft, Version: 1}
	registerInitiationRollback(ctx, func() {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		delete(stub.plans, plan.ID)
		delete(stub.reviews, plan.ID)
	})
	return nil
}

func (stub *initiationRepositoryStub) ListTouchPlanIndex(ctx context.Context, reviewStatus campaign.TouchPlanReviewStatus, after *campaignport.DraftTouchPlanKeyset, limit int32) ([]campaign.TouchPlanIndexItem, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if initiationTransactionID(ctx) < 1 || limit < 1 {
		return nil, errors.New("index outside transaction")
	}
	items := make([]campaign.TouchPlanIndexItem, 0)
	for id, plan := range stub.plans {
		review, exists := stub.reviews[id]
		if !exists || reviewStatus != "" && review.Status != reviewStatus || after != nil && (plan.CreatedAt.After(after.CreatedAt) || plan.CreatedAt.Equal(after.CreatedAt) && plan.ID >= after.PlanID) {
			continue
		}
		items = append(items, campaign.TouchPlanIndexItem{Plan: campaign.DraftTouchPlanSummaryOf(plan), ReviewStatus: review.Status, ReviewVersion: review.Version})
	}
	sort.Slice(items, func(left, right int) bool {
		if items[left].Plan.CreatedAt.Equal(items[right].Plan.CreatedAt) {
			return items[left].Plan.ID > items[right].Plan.ID
		}
		return items[left].Plan.CreatedAt.After(items[right].Plan.CreatedAt)
	})
	if len(items) > int(limit) {
		items = items[:limit]
	}
	return append([]campaign.TouchPlanIndexItem(nil), items...), nil
}

func (stub *initiationRepositoryStub) CompleteDraftCreate(ctx context.Context, receipt campaignport.CreateReceipt, eventID int64) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	transactionID := initiationTransactionID(ctx)
	stub.completeTx = append(stub.completeTx, transactionID)
	if transactionID < 1 {
		return errors.New("complete outside transaction")
	}
	key := receiptKey(receipt.ActorID, receipt.KeyDigest)
	current, exists := stub.receipts[key]
	if !exists || current != receipt || eventID < 1 {
		return errors.New("unknown receipt")
	}
	current.Completed = true
	current.EventID = eventID
	stub.receipts[key] = current
	registerInitiationRollback(ctx, func() {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		stub.receipts[key] = receipt
	})
	return nil
}

func (stub *initiationRepositoryStub) ListDraftTouchPlanSummaries(ctx context.Context, code string, after *campaignport.DraftTouchPlanKeyset, limit int32) ([]campaign.DraftTouchPlanSummary, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if initiationTransactionID(ctx) < 1 || limit < 1 {
		return nil, errors.New("list outside transaction")
	}
	items := make([]campaign.DraftTouchPlanSummary, 0)
	for _, plan := range stub.plans {
		if plan.CampaignCode != code || after != nil && (plan.CreatedAt.After(after.CreatedAt) || plan.CreatedAt.Equal(after.CreatedAt) && plan.ID >= after.PlanID) {
			continue
		}
		items = append(items, campaign.DraftTouchPlanSummaryOf(plan))
	}
	sort.Slice(items, func(left, right int) bool {
		if items[left].CreatedAt.Equal(items[right].CreatedAt) {
			return items[left].ID > items[right].ID
		}
		return items[left].CreatedAt.After(items[right].CreatedAt)
	})
	if len(items) > int(limit) {
		items = items[:limit]
	}
	return campaign.CloneDraftTouchPlanSummaries(items), nil
}

func (stub *initiationRepositoryStub) ReadDraftTouchPlan(ctx context.Context, code, id string) (campaign.DraftTouchPlan, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	transactionID := initiationTransactionID(ctx)
	stub.readTx = append(stub.readTx, transactionID)
	if transactionID < 1 {
		return campaign.DraftTouchPlan{}, errors.New("strict readback outside transaction")
	}
	plan, exists := stub.plans[id]
	if !exists || plan.CampaignCode != code {
		return campaign.DraftTouchPlan{}, errors.New("not found")
	}
	plan = campaign.CloneDraftTouchPlan(plan)
	if stub.readAlter != nil {
		plan = stub.readAlter(plan)
	}
	return plan, nil
}

type initiationEventStub struct {
	mu     sync.Mutex
	events []campaignport.CampaignEvent
	txIDs  []int
	err    error
}

func (stub *initiationEventStub) AppendCampaignEvent(ctx context.Context, event campaignport.CampaignEvent) (int64, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	transactionID := initiationTransactionID(ctx)
	stub.txIDs = append(stub.txIDs, transactionID)
	if transactionID < 1 {
		return 0, errors.New("event outside transaction")
	}
	if stub.err != nil {
		return 0, stub.err
	}
	stub.events = append(stub.events, event)
	registerInitiationRollback(ctx, func() {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		stub.events = stub.events[:len(stub.events)-1]
	})
	return int64(len(stub.events)), nil
}

func TestCreateDraftTouchPlanFreezesCanonicalLocalSnapshot(t *testing.T) {
	service, deps, command := testInitiationService(t)
	plan, err := service.CreateDraftTouchPlan(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CampaignCode != command.CampaignCode || plan.CampaignVersion != command.ExpectedCampaignVersion || !reflect.DeepEqual(plan.Source, deps.source.resolution.Source) || plan.OwnerActorID != command.Owner.ID ||
		!reflect.DeepEqual(plan.Targets.CustomerIDs, []int64{1}) || plan.Targets.Digest != campaign.CanonicalTargetDigest(plan.Source, []int64{1}) ||
		plan.Exclusions != (campaign.PreviewExclusionSummary{CandidateCount: 3, ActiveCustomerCount: 2, InactiveExcludedCount: 1, PolicyExcludedCount: 1}) ||
		!plan.Safety.LocalOnly || plan.Safety.ProviderExecutionEligible || plan.Safety.RuntimeExecuted || plan.Safety.RealExternalCallExecuted || plan.Safety.DeliveryProven ||
		!reflect.DeepEqual(plan.Content.Steps, deps.draft.fact.Steps) {
		t.Fatalf("plan=%+v", plan)
	}
	if len(deps.eligibility.requests) != 1 || deps.eligibility.requests[0].Checkpoint != campaignport.EligibilityCheckpointPreview ||
		deps.eligibility.requests[0].MaximumTargets != campaign.MaximumDraftTouchTargets || !reflect.DeepEqual(deps.eligibility.requests[0].CustomerIDs, []int64{1, 2, 3}) {
		t.Fatalf("eligibility=%+v", deps.eligibility.requests)
	}
	if len(deps.events.events) != 1 || deps.events.events[0].TargetDigest != plan.Targets.Digest || deps.events.events[0].TargetCount != 1 {
		t.Fatalf("events=%+v", deps.events.events)
	}
	assertSameInitiationTransaction(t, deps.draft.txIDs, deps.source.txIDs, deps.eligibility.txIDs, deps.repository.reserveTx, deps.repository.saveTx, deps.repository.completeTx, deps.repository.readTx, deps.events.txIDs)

	replay, err := service.CreateDraftTouchPlan(context.Background(), command)
	if err != nil || !reflect.DeepEqual(replay, plan) || deps.draft.calls != 1 || deps.source.calls != 1 || deps.eligibility.calls != 1 || len(deps.events.events) != 1 || len(deps.repository.readTx) != 2 {
		t.Fatalf("replay=%+v err=%v draft=%d source=%d eligibility=%d events=%d read=%v", replay, err, deps.draft.calls, deps.source.calls, deps.eligibility.calls, len(deps.events.events), deps.repository.readTx)
	}
}

func TestListTouchPlanIndexFiltersAndBindsCursorToReviewStatus(t *testing.T) {
	service, deps, command := testInitiationService(t)
	base := time.Date(2026, time.August, 23, 2, 3, 4, 0, time.UTC)
	plans := make([]campaign.DraftTouchPlan, 3)
	for index := range plans {
		service.now = func() time.Time { return base.Add(time.Duration(index) * time.Minute) }
		command.IdempotencyKey = "draft-touch-key-000" + strconv.Itoa(index+1)
		plan, err := service.CreateDraftTouchPlan(context.Background(), command)
		if err != nil {
			t.Fatal(err)
		}
		plans[index] = plan
	}
	deps.repository.reviews[plans[1].ID] = campaign.TouchPlanReview{PlanID: plans[1].ID, CampaignCode: plans[1].CampaignCode, Status: campaign.TouchPlanReviewPending, Version: 2}
	deps.repository.reviews[plans[2].ID] = campaign.TouchPlanReview{PlanID: plans[2].ID, CampaignCode: plans[2].CampaignCode, Status: campaign.TouchPlanReviewPending, Version: 3}

	first, err := service.ListTouchPlanIndex(context.Background(), campaign.TouchPlanReviewPending, "", 1)
	if err != nil || len(first.Items) != 1 || first.Items[0].Plan.ID != plans[2].ID || first.Items[0].ReviewVersion != 3 || first.NextCursor == "" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := service.ListTouchPlanIndex(context.Background(), campaign.TouchPlanReviewPending, first.NextCursor, 1)
	if err != nil || len(second.Items) != 1 || second.Items[0].Plan.ID != plans[1].ID || second.Items[0].ReviewVersion != 2 || second.NextCursor != "" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if _, err = service.ListTouchPlanIndex(context.Background(), campaign.TouchPlanReviewDraft, first.NextCursor, 1); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("cross-filter cursor err=%v", err)
	}
}

func TestCreateDraftTouchPlanBlocksIncompleteSourceAndInvalidEligibility(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*initiationDependencies)
	}{
		{"missing source watermark", func(deps *initiationDependencies) {
			deps.source.resolution.Source.Segment.MemberSnapshotWatermark = time.Time{}
		}},
		{"unknown eligibility exclusion", func(deps *initiationDependencies) {
			deps.eligibility.decisions[2].Exclusion = campaignport.EligibilityExclusion("touch_policy_excluded")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, deps, command := testInitiationService(t)
			test.mutate(deps)
			if _, err := service.CreateDraftTouchPlan(context.Background(), command); !errors.Is(err, campaign.ErrBlockedRedline) {
				t.Fatalf("err=%v", err)
			}
			if len(deps.repository.saveTx) != 0 || len(deps.events.events) != 0 {
				t.Fatalf("save=%v events=%+v", deps.repository.saveTx, deps.events.events)
			}
		})
	}
}

func TestCreateDraftTouchPlanBlocksCustomerFilterWithoutTouchingFacts(t *testing.T) {
	service, deps, command := testInitiationService(t)
	command.Source = campaign.InitiationSourceRequest{Kind: campaign.InitiationSourceCustomerFilter}
	if _, err := service.CreateDraftTouchPlan(context.Background(), command); !errors.Is(err, campaign.ErrBlockedRedline) {
		t.Fatalf("err=%v", err)
	}
	if deps.draft.calls != 0 || deps.source.calls != 0 || deps.eligibility.calls != 0 || len(deps.repository.reserveTx) != 0 || len(deps.events.events) != 0 {
		t.Fatalf("unexpected fact access draft=%d source=%d eligibility=%d reserve=%v events=%+v", deps.draft.calls, deps.source.calls, deps.eligibility.calls, deps.repository.reserveTx, deps.events.events)
	}
}

func TestCreateDraftTouchPlanCanonicalizesUnorderedCustomerSelectionForReplay(t *testing.T) {
	service, deps, command := testInitiationService(t)
	service.sources = nil
	command.Source = campaign.InitiationSourceRequest{Kind: campaign.InitiationSourceCustomerSelection, CustomerIDs: []int64{3, 1, 2}}
	plan, err := service.CreateDraftTouchPlan(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	expectedSource, valid := campaign.NewCustomerSelectionSourceRef([]int64{1, 2, 3})
	if !valid || !reflect.DeepEqual(plan.Source, expectedSource) || !reflect.DeepEqual(plan.Targets.CustomerIDs, []int64{1}) || deps.source.calls != 0 || deps.eligibility.calls != 1 {
		t.Fatalf("plan=%+v valid=%v source_calls=%d eligibility_calls=%d", plan, valid, deps.source.calls, deps.eligibility.calls)
	}
	command.Source.CustomerIDs = []int64{2, 3, 1}
	replay, err := service.CreateDraftTouchPlan(context.Background(), command)
	if err != nil || !reflect.DeepEqual(replay, plan) || deps.draft.calls != 1 || deps.source.calls != 0 || deps.eligibility.calls != 1 || len(deps.events.events) != 1 {
		t.Fatalf("replay=%+v err=%v draft=%d source=%d eligibility=%d events=%d", replay, err, deps.draft.calls, deps.source.calls, deps.eligibility.calls, len(deps.events.events))
	}
}

func TestCreateDraftTouchPlanRejectsDuplicateCustomerSelectionBeforeReservation(t *testing.T) {
	for _, test := range []struct {
		name        string
		customerIDs []int64
	}{
		{name: "duplicate", customerIDs: []int64{1, 1}},
		{name: "nonpositive", customerIDs: []int64{0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, deps, command := testInitiationService(t)
			command.Source = campaign.InitiationSourceRequest{Kind: campaign.InitiationSourceCustomerSelection, CustomerIDs: test.customerIDs}
			if _, err := service.CreateDraftTouchPlan(context.Background(), command); !errors.Is(err, ErrInvalidCommand) {
				t.Fatalf("err=%v", err)
			}
			if len(deps.repository.reserveTx) != 0 || deps.source.calls != 0 || deps.eligibility.calls != 0 {
				t.Fatalf("reserve=%v source=%d eligibility=%d", deps.repository.reserveTx, deps.source.calls, deps.eligibility.calls)
			}
		})
	}
}

func TestCreateDraftTouchPlanBindsCustomerSelectionToIdempotencyPayload(t *testing.T) {
	service, deps, command := testInitiationService(t)
	command.Source = campaign.InitiationSourceRequest{Kind: campaign.InitiationSourceCustomerSelection, CustomerIDs: []int64{1, 2, 3}}
	if _, err := service.CreateDraftTouchPlan(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	command.Source.CustomerIDs = []int64{1, 2}
	if _, err := service.CreateDraftTouchPlan(context.Background(), command); !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	if deps.draft.calls != 1 || deps.source.calls != 0 || deps.eligibility.calls != 1 || len(deps.events.events) != 1 {
		t.Fatalf("draft=%d source=%d eligibility=%d events=%d", deps.draft.calls, deps.source.calls, deps.eligibility.calls, len(deps.events.events))
	}
}

func TestCreateDraftTouchPlanBlocksZeroEligibleBeforeCommit(t *testing.T) {
	service, deps, command := testInitiationService(t)
	deps.eligibility.decisions = []campaignport.EligibilityDecision{
		{CustomerID: 1, CustomerActive: true, Eligible: false, Exclusion: campaignport.EligibilityExclusionContactPolicy},
		{CustomerID: 2, CustomerActive: false, Eligible: false, Exclusion: campaignport.EligibilityExclusionInactiveCustomer},
		{CustomerID: 3, CustomerActive: true, Eligible: false, Exclusion: campaignport.EligibilityExclusionContactPolicy},
	}
	if _, err := service.CreateDraftTouchPlan(context.Background(), command); !errors.Is(err, campaign.ErrBlockedRedline) {
		t.Fatalf("err=%v", err)
	}
	if len(deps.repository.receipts) != 0 || len(deps.repository.plans) != 0 || len(deps.repository.saveTx) != 0 || len(deps.repository.completeTx) != 0 || len(deps.repository.readTx) != 0 || len(deps.events.events) != 0 {
		t.Fatalf("receipts=%+v plans=%+v save=%v complete=%v read=%v events=%+v", deps.repository.receipts, deps.repository.plans, deps.repository.saveTx, deps.repository.completeTx, deps.repository.readTx, deps.events.events)
	}
}

func TestValidDraftTouchPlanRejectsEmptyTargets(t *testing.T) {
	service, _, command := testInitiationService(t)
	plan, err := service.CreateDraftTouchPlan(context.Background(), command)
	if err != nil || !campaign.ValidDraftTouchPlan(plan) {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	plan.Targets.CustomerIDs = nil
	plan.Targets.Digest = campaign.CanonicalTargetDigest(plan.Source, nil)
	plan.Exclusions = campaign.PreviewExclusionSummary{}
	if campaign.ValidDraftTouchPlan(plan) {
		t.Fatal("empty target snapshot was accepted")
	}
}

func TestCreateDraftTouchPlanBlocksOversizedSourceBeforeEligibility(t *testing.T) {
	service, deps, command := testInitiationService(t)
	oversized := make([]int64, campaign.MaximumDraftTouchTargets+1)
	for index := range oversized {
		oversized[index] = int64(index + 1)
	}
	deps.source.resolution.CustomerIDs = oversized
	if _, err := service.CreateDraftTouchPlan(context.Background(), command); !errors.Is(err, campaign.ErrBlockedRedline) {
		t.Fatalf("err=%v", err)
	}
	if deps.eligibility.calls != 0 || len(deps.repository.saveTx) != 0 || len(deps.events.events) != 0 {
		t.Fatalf("eligibility=%d save=%v events=%+v", deps.eligibility.calls, deps.repository.saveTx, deps.events.events)
	}
}

func TestCreateDraftTouchPlanFreezesAudiencePackageVersionAndMemberWatermark(t *testing.T) {
	service, deps, command := testInitiationService(t)
	digest := sha256.Sum256([]byte("audience-package-members"))
	deps.source.resolution.Source = campaign.InitiationSourceRef{
		Kind: campaign.InitiationSourceAudiencePackageMembers,
		AudiencePackage: &campaign.AudiencePackageMemberSourceFact{
			PackageID:               11,
			PackageVersion:          6,
			MemberSnapshotWatermark: time.Date(2026, time.August, 23, 2, 4, 0, 0, time.UTC),
			Digest:                  hex.EncodeToString(digest[:]),
		},
	}
	command.Source = campaign.InitiationSourceRequest{Kind: campaign.InitiationSourceAudiencePackageMembers, AudiencePackageID: 11}
	plan, err := service.CreateDraftTouchPlan(context.Background(), command)
	if err != nil || !reflect.DeepEqual(plan.Source, deps.source.resolution.Source) || plan.Source.AudiencePackage == nil || plan.Source.AudiencePackage.PackageVersion != 6 || plan.Source.AudiencePackage.MemberSnapshotWatermark.IsZero() {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
}

func TestCreateDraftTouchPlanRejectsCampaignVersionDriftBeforeSourceRead(t *testing.T) {
	service, deps, command := testInitiationService(t)
	deps.draft.fact.Version++
	if _, err := service.CreateDraftTouchPlan(context.Background(), command); !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	if deps.source.calls != 0 || deps.eligibility.calls != 0 || len(deps.repository.saveTx) != 0 {
		t.Fatalf("source=%d eligibility=%d save=%v", deps.source.calls, deps.eligibility.calls, deps.repository.saveTx)
	}
}

func TestCreateDraftTouchPlanRequiresStrictReadback(t *testing.T) {
	service, deps, command := testInitiationService(t)
	deps.repository.readAlter = func(plan campaign.DraftTouchPlan) campaign.DraftTouchPlan {
		plan.Content.Digest = strings.Repeat("0", 64)
		return plan
	}
	if _, err := service.CreateDraftTouchPlan(context.Background(), command); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v", err)
	}
	if len(deps.repository.readTx) != 1 || deps.repository.readTx[0] < 1 {
		t.Fatalf("readback transaction=%v", deps.repository.readTx)
	}
}

func TestCreateDraftTouchPlanRejectsEmptyTargetReplayReadback(t *testing.T) {
	service, deps, command := testInitiationService(t)
	if _, err := service.CreateDraftTouchPlan(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	deps.repository.readAlter = func(plan campaign.DraftTouchPlan) campaign.DraftTouchPlan {
		plan.Targets.CustomerIDs = nil
		plan.Targets.Digest = campaign.CanonicalTargetDigest(plan.Source, nil)
		plan.Exclusions = campaign.PreviewExclusionSummary{}
		return plan
	}
	if _, err := service.CreateDraftTouchPlan(context.Background(), command); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v", err)
	}
	if deps.draft.calls != 1 || deps.source.calls != 1 || deps.eligibility.calls != 1 || len(deps.events.events) != 1 || len(deps.repository.readTx) != 2 {
		t.Fatalf("draft=%d source=%d eligibility=%d events=%d read=%v", deps.draft.calls, deps.source.calls, deps.eligibility.calls, len(deps.events.events), deps.repository.readTx)
	}
}

type initiationDependencies struct {
	draft       *initiationDraftStub
	source      *initiationSourceStub
	eligibility *initiationEligibilityStub
	repository  *initiationRepositoryStub
	events      *initiationEventStub
}

func testInitiationService(t *testing.T) (*Service, *initiationDependencies, campaign.CreateDraftTouchPlanCommand) {
	t.Helper()
	sourceDigest := sha256.Sum256([]byte("segment-members-snapshot"))
	source := campaign.InitiationSourceRef{
		Kind: campaign.InitiationSourceSegmentMembers,
		Segment: &campaign.SegmentMemberSourceFact{
			SegmentID:               7,
			MemberSnapshotWatermark: time.Date(2026, time.August, 23, 2, 3, 0, 0, time.UTC),
			Digest:                  hex.EncodeToString(sourceDigest[:]),
		},
	}
	deps := &initiationDependencies{
		draft: &initiationDraftStub{fact: campaignport.CampaignDraftFact{
			CampaignCode: "spring-campaign", Version: 4, ApprovalStatus: campaign.ApprovalDraft, RuntimeStatus: campaign.RuntimeIdle,
			Steps: []campaign.Step{{Index: 1, DelayMinutes: 0, Content: "hello"}},
		}},
		source: &initiationSourceStub{resolution: campaignport.SourceResolution{Source: source, CustomerIDs: []int64{1, 2, 3}}},
		eligibility: &initiationEligibilityStub{decisions: []campaignport.EligibilityDecision{
			{CustomerID: 1, CustomerActive: true, Eligible: true, Exclusion: campaignport.EligibilityExclusionNone},
			{CustomerID: 2, CustomerActive: false, Eligible: false, Exclusion: campaignport.EligibilityExclusionInactiveCustomer},
			{CustomerID: 3, CustomerActive: true, Eligible: false, Exclusion: campaignport.EligibilityExclusionContactPolicy},
		}},
		repository: newInitiationRepository(), events: &initiationEventStub{},
	}
	service, err := NewService(&initiationUoW{}, deps.draft, deps.source, deps.eligibility, deps.repository, deps.events)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, time.August, 23, 2, 3, 4, 0, time.UTC) }
	return service, deps, campaign.CreateDraftTouchPlanCommand{
		CampaignCode: "spring-campaign", ExpectedCampaignVersion: 4,
		Source: campaign.InitiationSourceRequest{Kind: campaign.InitiationSourceSegmentMembers, SegmentID: 7},
		Owner:  campaign.Actor{ID: 7}, IdempotencyKey: "draft-touch-key-0001",
	}
}

func assertSameInitiationTransaction(t *testing.T, collections ...[]int) {
	t.Helper()
	var want int
	for _, values := range collections {
		for _, value := range values {
			if value < 1 {
				t.Fatalf("missing transaction id in %v", collections)
			}
			if want == 0 {
				want = value
			} else if value != want {
				t.Fatalf("transaction mismatch want=%d collections=%v", want, collections)
			}
		}
	}
}

func receiptKey(actorID int64, digest [sha256.Size]byte) string {
	return strconv.FormatInt(actorID, 10) + ":" + hex.EncodeToString(digest[:])
}
