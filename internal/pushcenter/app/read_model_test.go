package app

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestReadNormalizesFiltersAndAcceptsReconciledEffectiveBucket(t *testing.T) {
	filter := Filter{Section: " questionnaire ", Status: " sent ", ExternalUserID: " ext-1 ", CreatedFrom: " 2026-08-01T00:00:00Z "}
	repository := &readModelRepositoryStub{summary: validSummary(filter)}
	service := NewService(readModelTestUoW{}, repository)

	summary, err := service.Read(context.Background(), filter)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if repository.filter.Section != "questionnaire" || repository.filter.Status != "sent" || repository.filter.ExternalUserID != "ext-1" {
		t.Fatalf("repository filter = %+v", repository.filter)
	}
	if summary.ByEffectiveStatus["reconciled"] != 1 || summary.Total != 3 {
		t.Fatalf("summary = %+v", summary)
	}
	if filters := summary.AppliedFilter.ResponseFilters(); len(filters) != 4 || filters["external_userid"] != "ext-1" || filters["created_from"] != "2026-08-01T00:00:00Z" {
		t.Fatalf("response filters = %#v", filters)
	}
	summary.ByStatus["pending"] = 99
	if repository.summary.ByStatus["pending"] != 1 {
		t.Fatalf("service leaked repository map: %+v", repository.summary.ByStatus)
	}
}

func TestReadFailsClosedForMalformedUnavailableAndCancelledContexts(t *testing.T) {
	filter := Filter{}
	malformed := validSummary(filter)
	malformed.ByEffectiveStatus = map[string]int64{"reconciled": 1}
	for _, testCase := range []struct {
		name    string
		service *Service
		ctx     context.Context
	}{
		{"malformed aggregate", NewService(readModelTestUoW{}, &readModelRepositoryStub{summary: malformed}), context.Background()},
		{"repository unavailable", NewService(readModelTestUoW{}, &readModelRepositoryStub{err: errors.New("database unavailable")}), context.Background()},
		{"missing dependency", NewService(nil, nil), context.Background()},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := testCase.service.Read(testCase.ctx, filter); !errors.Is(err, ErrReadModelUnavailable) {
				t.Fatalf("Read() error = %v, want unavailable", err)
			}
		})
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewService(readModelTestUoW{}, &readModelRepositoryStub{summary: validSummary(filter)}).Read(cancelled, filter); !errors.Is(err, ErrReadModelUnavailable) {
		t.Fatalf("cancelled Read() error = %v", err)
	}
}

func TestReadConcurrentQueriesRemainReadOnly(t *testing.T) {
	repository := &readModelRepositoryStub{summary: validSummary(Filter{})}
	service := NewService(readModelTestUoW{}, repository)
	errCh := make(chan error, 32)
	var group sync.WaitGroup
	for index := 0; index < cap(errCh); index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, readErr := service.Read(context.Background(), Filter{})
			errCh <- readErr
		}()
	}
	group.Wait()
	close(errCh)
	for readErr := range errCh {
		if readErr != nil {
			t.Fatalf("concurrent Read() error = %v", readErr)
		}
	}
}

func TestDefinitionsAreFrozenAndDefensivelyCopied(t *testing.T) {
	sections := SectionDefinitions()
	statuses := StatusDefinitions()
	if len(sections) != 13 || len(statuses) != 9 || sections[0].CapabilityKey != "questionnaire_external_push" || sections[12].CapabilityKey != "" {
		t.Fatalf("definitions sections=%+v statuses=%+v", sections, statuses)
	}
	if statuses[5].Key != "unknown_after_dispatch" || statuses[5].Definition != "外部调用结果不确定，必须先对账且不会自动重试。" {
		t.Fatalf("unknown status definition = %+v", statuses[5])
	}
	sections[0].EffectTypes[0] = "changed"
	if SectionDefinitions()[0].EffectTypes[0] != "webhook.questionnaire_submission.push" {
		t.Fatal("section definitions leaked mutable storage")
	}
}

type readModelTestUoW struct{}

func (readModelTestUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type readModelRepositoryStub struct {
	mu      sync.Mutex
	filter  Filter
	summary Summary
	err     error
}

func (stub *readModelRepositoryStub) ReadSummary(_ context.Context, filter Filter) (Summary, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.filter = filter
	if stub.err != nil {
		return Summary{}, stub.err
	}
	return cloneSummary(stub.summary), nil
}

func validSummary(filter Filter) Summary {
	filter = filter.Normalized()
	return Summary{AppliedFilter: filter, Total: 3,
		ByStatus:          map[string]int64{"pending": 1, "sent": 1, "sent_with_shadow_warning": 1},
		ByEffectiveStatus: map[string]int64{"pending": 1, "sent": 1, "reconciled": 1},
		BySection:         map[string]int64{"questionnaire": 3},
	}
}
