package app

import (
	"context"
	"reflect"
	"sync"
	"testing"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
)

func TestCreateDraftTouchPlanConcurrentReplayIsEquivalent(t *testing.T) {
	service, deps, command := testInitiationService(t)
	type result struct {
		plan campaign.DraftTouchPlan
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			plan, err := service.CreateDraftTouchPlan(context.Background(), command)
			results <- result{plan: plan, err: err}
		}()
	}
	close(start)
	group.Wait()
	close(results)
	var all []result
	for value := range results {
		all = append(all, value)
	}
	if len(all) != 2 || all[0].err != nil || all[1].err != nil || !reflect.DeepEqual(all[0].plan, all[1].plan) ||
		deps.draft.calls != 1 || deps.source.calls != 1 || deps.eligibility.calls != 1 || len(deps.events.events) != 1 || len(deps.repository.readTx) != 2 {
		t.Fatalf("results=%+v draft=%d source=%d eligibility=%d events=%d read=%v", all, deps.draft.calls, deps.source.calls, deps.eligibility.calls, len(deps.events.events), deps.repository.readTx)
	}
}
