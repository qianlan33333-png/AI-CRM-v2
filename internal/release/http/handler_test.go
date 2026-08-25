package releasehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	releaseapp "github.com/qianlan33333-png/AI-CRM-v2/internal/release/app"
	releaseport "github.com/qianlan33333-png/AI-CRM-v2/internal/release/port"
)

func TestDetailProjectionUsesContractKeysAndNeverExposesFence(t *testing.T) {
	now := time.Date(2026, 8, 25, 1, 2, 3, 0, time.UTC)
	detail := detailOf(releaseport.Detail{
		Candidate:           releaseport.Candidate{ID: 7, CommitSHA: strings.Repeat("a", 40), ArtifactDigest: strings.Repeat("b", 64), ManifestDigest: strings.Repeat("c", 64), ConfigDigest: strings.Repeat("d", 64), TargetSchemaVersion: 74, State: releaseport.CandidatePrepared, CreatedBy: 9, CreatedAt: now},
		Readiness:           releaseport.Readiness{CandidateID: 7, Ready: true, CheckedAt: now},
		RollbackEligibility: releaseport.RollbackEligibility{CandidateID: 7, Eligible: true, CheckedAt: now},
		ActiveWorker:        &releaseport.WorkerSummary{CandidateID: 7, Generation: 2, StartedBy: 9, StartedAt: now},
	})
	raw, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, key := range []string{`"candidate_id"`, `"checked_at"`, `"rollback_eligibility"`, `"active_worker"`} {
		if !strings.Contains(text, key) {
			t.Fatalf("detail JSON missing %s: %s", key, text)
		}
	}
	for _, forbidden := range []string{`CandidateID`, `CheckedAt`, `Fence`, `fence`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("detail JSON exposed %s: %s", forbidden, text)
		}
	}
}

type applicationStub struct {
	list     func(context.Context, int32) ([]releaseport.Candidate, error)
	register func(context.Context, releaseapp.RegisterCommand) (releaseport.Candidate, error)
	detail   func(context.Context, int64) (releaseport.Detail, error)
	prepare  func(context.Context, releaseapp.CandidateCommand) (releaseport.Candidate, error)
	start    func(context.Context, releaseapp.CandidateCommand) (releaseport.WorkerLease, error)
}

func (s applicationStub) List(ctx context.Context, limit int32) ([]releaseport.Candidate, error) {
	if s.list != nil {
		return s.list(ctx, limit)
	}
	return nil, nil
}
func (s applicationStub) Register(ctx context.Context, command releaseapp.RegisterCommand) (releaseport.Candidate, error) {
	if s.register != nil {
		return s.register(ctx, command)
	}
	return releaseport.Candidate{}, nil
}
func (s applicationStub) Detail(ctx context.Context, candidateID int64) (releaseport.Detail, error) {
	if s.detail != nil {
		return s.detail(ctx, candidateID)
	}
	return releaseport.Detail{}, nil
}
func (s applicationStub) RecordPrerequisite(context.Context, releaseapp.ReceiptCommand) (releaseport.PrerequisiteReceipt, error) {
	return releaseport.PrerequisiteReceipt{}, nil
}
func (s applicationStub) Prepare(ctx context.Context, command releaseapp.CandidateCommand) (releaseport.Candidate, error) {
	if s.prepare != nil {
		return s.prepare(ctx, command)
	}
	return releaseport.Candidate{}, nil
}
func (s applicationStub) StartCutover(ctx context.Context, command releaseapp.CandidateCommand) (releaseport.WorkerLease, error) {
	if s.start != nil {
		return s.start(ctx, command)
	}
	return releaseport.WorkerLease{}, nil
}
func (s applicationStub) RestartCutover(context.Context, releaseapp.WorkerCommand) (releaseport.WorkerLease, error) {
	return releaseport.WorkerLease{}, nil
}
func (s applicationStub) CompleteStep(context.Context, releaseapp.StepCommand) (releaseport.CutoverJournalEntry, error) {
	return releaseport.CutoverJournalEntry{}, nil
}
func (s applicationStub) Activate(context.Context, releaseapp.WorkerCommand) (releaseport.Candidate, error) {
	return releaseport.Candidate{}, nil
}
func (s applicationStub) RecordRollbackCheck(context.Context, releaseapp.RollbackCheckCommand) (releaseport.RollbackCheck, error) {
	return releaseport.RollbackCheck{}, nil
}
func (s applicationStub) RequestRollback(context.Context, releaseapp.CandidateCommand) (releaseport.Candidate, error) {
	return releaseport.Candidate{}, nil
}
func (s applicationStub) CompleteRollback(context.Context, releaseapp.CandidateCommand) (releaseport.Candidate, error) {
	return releaseport.Candidate{}, nil
}

func releaseRequest(method, path, body string, role authport.Role, capability authport.Capability) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 42, Role: role}, "session")
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	if err != nil {
		panic(err)
	}
	return request.WithContext(ctx)
}

func TestReadAllowsOpsAndManageRejectsOps(t *testing.T) {
	listed := false
	prepared := false
	handler, err := NewHandler(applicationStub{
		list: func(context.Context, int32) ([]releaseport.Candidate, error) {
			listed = true
			return nil, nil
		},
		prepare: func(context.Context, releaseapp.CandidateCommand) (releaseport.Candidate, error) {
			prepared = true
			return releaseport.Candidate{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	read := httptest.NewRecorder()
	handler.List(read, releaseRequest(http.MethodGet, "/api/v1/admin/release-candidates", "", authport.RoleOps, authport.CapabilityReleaseRead), 50)
	if read.Code != http.StatusOK || !listed {
		t.Fatalf("ops read = %d listed=%t", read.Code, listed)
	}

	manage := httptest.NewRecorder()
	request := releaseRequest(http.MethodPost, "/api/v1/admin/release-candidates/1/prepare", "", authport.RoleOps, authport.CapabilityReleaseManage)
	request.Header.Set("Idempotency-Key", "release-key-00001")
	handler.Prepare(manage, request, 1)
	if manage.Code != http.StatusForbidden || prepared {
		t.Fatalf("ops manage = %d prepared=%t", manage.Code, prepared)
	}
}

func TestRegisterUsesContextActorAndRejectsUnknownJSON(t *testing.T) {
	var command releaseapp.RegisterCommand
	handler, err := NewHandler(applicationStub{register: func(_ context.Context, got releaseapp.RegisterCommand) (releaseport.Candidate, error) {
		command = got
		return releaseport.Candidate{}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	valid := `{"commit_sha":"` + strings.Repeat("a", 40) + `","artifact_digest":"` + strings.Repeat("b", 64) + `","manifest_digest":"` + strings.Repeat("c", 64) + `","config_digest":"` + strings.Repeat("d", 64) + `","target_schema_version":74}`
	request := releaseRequest(http.MethodPost, "/api/v1/admin/release-candidates", valid, authport.RoleAdmin, authport.CapabilityReleaseManage)
	request.Header.Set("Idempotency-Key", "release-key-00002")
	response := httptest.NewRecorder()
	handler.Register(response, request)
	if response.Code != http.StatusCreated || command.ActorID != 42 {
		t.Fatalf("register = %d actor=%d", response.Code, command.ActorID)
	}

	unknown := releaseRequest(http.MethodPost, "/api/v1/admin/release-candidates", valid[:len(valid)-1]+`,"actor_id":7}`, authport.RoleAdmin, authport.CapabilityReleaseManage)
	unknown.Header.Set("Idempotency-Key", "release-key-00003")
	response = httptest.NewRecorder()
	handler.Register(response, unknown)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown JSON = %d", response.Code)
	}
}

func TestMutationRequiresExactlyOneIdempotencyKey(t *testing.T) {
	called := false
	handler, err := NewHandler(applicationStub{prepare: func(context.Context, releaseapp.CandidateCommand) (releaseport.Candidate, error) {
		called = true
		return releaseport.Candidate{}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, keys := range [][]string{nil, {"release-key-00004", "release-key-00005"}} {
		response := httptest.NewRecorder()
		request := releaseRequest(http.MethodPost, "/api/v1/admin/release-candidates/1/prepare", "", authport.RoleAdmin, authport.CapabilityReleaseManage)
		for _, value := range keys {
			request.Header.Add("Idempotency-Key", value)
		}
		handler.Prepare(response, request, 1)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("keys %v = %d", keys, response.Code)
		}
	}
	if called {
		t.Fatal("application was called without exactly one idempotency key")
	}
}

func TestHandlerMapsReleaseErrorsAndFencesOnlyWorkerResponses(t *testing.T) {
	handler, err := NewHandler(applicationStub{
		list: func(context.Context, int32) ([]releaseport.Candidate, error) { return nil, releaseport.ErrUnavailable },
		detail: func(context.Context, int64) (releaseport.Detail, error) {
			return releaseport.Detail{}, releaseport.ErrNotFound
		},
		prepare: func(context.Context, releaseapp.CandidateCommand) (releaseport.Candidate, error) {
			return releaseport.Candidate{}, releaseport.ErrConflict
		},
		start: func(context.Context, releaseapp.CandidateCommand) (releaseport.WorkerLease, error) {
			return releaseport.WorkerLease{CandidateID: 1, Generation: 2, Fence: "manage-fence"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
		want int
	}{
		{"unavailable", func(w http.ResponseWriter, r *http.Request) { handler.List(w, r, 50) }, http.StatusServiceUnavailable},
		{"not found", func(w http.ResponseWriter, r *http.Request) { handler.Detail(w, r, 1) }, http.StatusNotFound},
		{"conflict", func(w http.ResponseWriter, r *http.Request) { handler.Prepare(w, r, 1) }, http.StatusConflict},
	}
	for _, test := range cases {
		response := httptest.NewRecorder()
		request := releaseRequest(http.MethodGet, "/api/v1/admin/release-candidates/1", "", authport.RoleAdmin, authport.CapabilityReleaseRead)
		if test.name == "conflict" {
			request = releaseRequest(http.MethodPost, "/api/v1/admin/release-candidates/1/prepare", "", authport.RoleAdmin, authport.CapabilityReleaseManage)
			request.Header.Set("Idempotency-Key", "release-key-00006")
		}
		test.call(response, request)
		if response.Code != test.want {
			t.Fatalf("%s = %d, want %d", test.name, response.Code, test.want)
		}
	}

	response := httptest.NewRecorder()
	request := releaseRequest(http.MethodPost, "/api/v1/admin/release-candidates/1/cutover/start", "", authport.RoleAdmin, authport.CapabilityReleaseManage)
	request.Header.Set("Idempotency-Key", "release-key-00007")
	handler.StartCutover(response, request, 1)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"fence":"manage-fence"`) {
		t.Fatalf("start = %d body=%s", response.Code, response.Body.String())
	}
}
