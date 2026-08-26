package tag

import "testing"

func TestWorkerConstructionRequiresExplicitProvider(t *testing.T) {
	if worker, err := NewWorker(nil, DisabledProvider{}); err == nil || worker != nil {
		t.Fatalf("NewWorker(nil, disabled) = %#v, %v", worker, err)
	}
	service, _, _ := queuedTestService(t, OperationCatalogSync)
	if worker, err := NewWorker(service, nil); err == nil || worker != nil {
		t.Fatalf("NewWorker(service, nil) = %#v, %v", worker, err)
	}
	worker, err := NewDisabledWorker(service)
	if err != nil || worker == nil || worker.provider == nil {
		t.Fatalf("NewDisabledWorker() = %#v, %v", worker, err)
	}
}
