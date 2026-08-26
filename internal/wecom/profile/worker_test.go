package profile

import (
	"testing"

	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
)

func TestRegisterWorkerBindsProfileEffectToSyncQueue(t *testing.T) {
	registry := platformjobqueue.NewWorkerRegistry()
	if err := RegisterWorker(registry, &Service{}, &profileWriterFake{}); err != nil {
		t.Fatal(err)
	}
	options, err := registry.ExplicitOptions(platformjobqueue.QueueSync, JobArgs{EffectID: "eer_41"}, nil)
	if err != nil || options.Queue != string(platformjobqueue.QueueSync) {
		t.Fatalf("options=%+v err=%v", options, err)
	}
}
