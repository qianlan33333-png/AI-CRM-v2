package port

import "testing"

func TestAdminReadRegistryIsImmutableAndOrdered(t *testing.T) {
	bindings := AdminReadBindings()
	if len(bindings) != 5 || bindings[0].Consumer != ConsumerAutomationTagTrigger || bindings[1].Consumer != ConsumerStatsTagApplied || bindings[2].Consumer != ConsumerOperationCycleFact || bindings[3].Consumer != ConsumerCloudCampaignFact || bindings[4].Consumer != ConsumerOutboundCampaignHandoffFact {
		t.Fatalf("bindings=%+v", bindings)
	}
	bindings[0].EventTypes[0] = "tampered"
	if AdminReadBindings()[0].EventTypes[0] != EvTagApplied {
		t.Fatal("registry leaked mutable backing storage")
	}
	statuses := AdminReadStatuses()
	if len(statuses) != 5 || statuses[0] != string(DeliveryPending) || statuses[4] != string(DeliveryOutcomeUnknown) {
		t.Fatalf("statuses=%v", statuses)
	}
}
