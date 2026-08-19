package port

import "testing"

func TestAdminDetailSnapshotKeepsReadContractSeparate(t *testing.T) {
	var repository AdminDetailRepository
	if repository != nil {
		t.Fatal("nil interface unexpectedly non-nil")
	}
	snapshot := AdminDetailSnapshot{Deliveries: make([]AdminReadDelivery, 0)}
	if snapshot.Found || snapshot.Event.EventID != 0 || snapshot.Deliveries == nil {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}
