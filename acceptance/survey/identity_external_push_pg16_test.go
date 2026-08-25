package survey_acceptance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	eerstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/http"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveystore "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store"
)

func TestSurveyIdentityExternalPushPG16(t *testing.T) {
	pool, ctx := openPool(t)
	uow := platformstore.NewUnitOfWork(pool)
	slug := "survey-push-" + strings.ReplaceAll(unique("identity"), "_", "-")

	states := &surveyPushOAuthStates{}
	h5, err := surveyapp.NewH5OAuthService(states, surveyPushVerifiedProvider{}, surveyPushIdentityResolver{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = h5.Start(ctx, "/s/"+slug); err != nil {
		t.Fatalf("H5 start=%v", err)
	}
	h5Handler := surveyhttp.NewH5OAuthHandler(h5, [32]byte{1})
	callback := httptest.NewRecorder()
	h5Handler.Callback(callback, httptest.NewRequest(http.MethodGet, "/api/h5/surveys/oauth/callback?state="+states.state+"&code=verified-code&external_userid=forged", nil))
	if callback.Code != http.StatusFound || callback.Header().Get("Location") != "/s/"+slug || len(callback.Result().Cookies()) != 1 {
		t.Fatalf("H5 callback=%d location=%q cookies=%v", callback.Code, callback.Header().Get("Location"), callback.Result().Cookies())
	}
	identityRequest := httptest.NewRequest(http.MethodPost, "/api/h5/surveys/submit", nil)
	identityRequest.AddCookie(callback.Result().Cookies()[0])
	identity, err := h5Handler.Identity(identityRequest)
	if err != nil || identity.CustomerID != 4242 {
		t.Fatalf("H5 canonical identity=%+v err=%v", identity, err)
	}
	if _, _, err = h5.Callback(ctx, states.state, "verified-code"); err == nil {
		t.Fatal("replayed H5 OAuth state was accepted")
	}

	publicStore := surveystore.NewPublicRepository()
	definition := surveyPushDefinition(t, ctx, uow, publicStore, slug)
	runtimeStore := eerstore.NewRepository(pool, uow)
	runtime, err := eer.NewService(runtimeStore)
	if err != nil {
		t.Fatal(err)
	}
	push, err := surveyapp.NewExternalPushService(uow, surveystore.NewExternalPushRepository(), runtime)
	if err != nil {
		t.Fatal(err)
	}
	binder := &surveyPushBinder{delegate: surveyapp.PublicExternalPushBinder{Push: push}}
	public := surveyapp.NewPublicServiceWithBinder(uow, publicStore, eventstore.NewAppender(), [32]byte{2}, binder)
	input := surveyport.PublicSubmissionCommand{
		Slug: slug, Version: definition.View.Version, CanonicalCustomerID: identity.CustomerID,
		SubmissionKey: surveyPushKey(3), AnonymousDigest: sha256.Sum256([]byte("anonymous")), RateDigest: sha256.Sum256([]byte("rate")),
		Answers: []surveyport.PublicSubmissionAnswer{{QuestionID: definition.View.Questions[0].ID, OptionIDs: []int64{definition.View.Questions[0].Options[0].ID}}},
	}
	submission, _, err := public.Submit(ctx, input)
	if err != nil || submission.SubmissionID < 1 {
		t.Fatalf("identified submission=%+v err=%v binder=%v", submission, err, binder.err)
	}
	replay, _, err := public.Submit(ctx, input)
	if err != nil || replay.SubmissionID != submission.SubmissionID {
		t.Fatalf("identified replay=%+v err=%v", replay, err)
	}
	binding, err := push.Detail(ctx, submission.QuestionnaireID, submission.SubmissionID)
	if err != nil || binding.EffectID == "" || binding.State != eer.StateAccepted {
		t.Fatalf("accepted binding=%+v err=%v", binding, err)
	}

	queued, _, err := runtime.Queue(ctx, eer.QueueCommand{EffectID: binding.EffectID, Job: eer.RiverJobLink{JobID: 1, Generation: 1, Queue: "survey-webhook", ArgsDigest: surveyPushRuntimeDigest("args"), ScheduledAt: time.Now().UTC()}, ReceiptKeyDigest: surveyPushRuntimeDigest("queue")})
	if err != nil || queued.State != eer.StateQueued {
		t.Fatalf("queue=%+v err=%v", queued, err)
	}
	lease, _, err := runtime.Claim(ctx, eer.ClaimCommand{EffectID: binding.EffectID, WorkerDigest: surveyPushRuntimeDigest("worker")})
	if err != nil {
		t.Fatal(err)
	}
	unknown, _, runErr := runtime.RunAttempt(ctx, lease, surveyPushUnknownAdapter{})
	if !errors.Is(runErr, eer.ErrAdapterFailure) || unknown.State != eer.StateOutcomeUnknown {
		t.Fatalf("fake dispatch state=%+v err=%v", unknown, runErr)
	}

	evidence := sha256.Sum256([]byte("verified operator evidence"))
	command := surveyapp.ExternalPushReconcileCommand{QuestionnaireID: submission.QuestionnaireID, SubmissionID: submission.SubmissionID, Lease: lease, EvidenceDigest: evidence, ProviderAccepted: true, DeliveryProven: false, IdempotencyKey: "survey-push-reconcile-key"}
	reconciled, err := push.Reconcile(ctx, command)
	if err != nil || reconciled.State != eer.StateReconciled || !reconciled.ProviderAccepted || reconciled.DeliveryProven {
		t.Fatalf("manual reconcile=%+v err=%v", reconciled, err)
	}
	if _, err = push.Reconcile(ctx, command); err != nil {
		t.Fatalf("manual reconcile replay=%v", err)
	}

	requestContext, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityQuestionnairesRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	detail := httptest.NewRecorder()
	(&surveyhttp.ExternalPushDetailHandler{Application: push}).Get(detail, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(requestContext), submission.QuestionnaireID, submission.SubmissionID)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	var safe map[string]any
	if err = json.Unmarshal(detail.Body.Bytes(), &safe); err != nil || len(safe) != 5 || safe["state"] != string(eer.StateReconciled) || safe["provider_accepted"] != true || safe["delivery_proven"] != false {
		t.Fatalf("PII-free detail=%s err=%v", detail.Body.String(), err)
	}
	for _, forbidden := range []string{"customer", "identity", "digest", "receipt", "external_userid"} {
		if strings.Contains(detail.Body.String(), forbidden) {
			t.Fatalf("detail exposed %q: %s", forbidden, detail.Body.String())
		}
	}
	var bindings, receipts int
	var receiptEvidence *string
	if err = pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM questionnaire_submission_external_push_bindings),
  (SELECT count(*) FROM questionnaire_external_push_delivery_receipts),
  (SELECT evidence_digest FROM questionnaire_external_push_delivery_receipts LIMIT 1)`).Scan(&bindings, &receipts, &receiptEvidence); err != nil || bindings != 1 || receipts != 1 || receiptEvidence != nil {
		t.Fatalf("durable facts=%d/%d/evidence=%v err=%v", bindings, receipts, receiptEvidence, err)
	}
}

func surveyPushDefinition(t *testing.T, ctx context.Context, uow *platformstore.UnitOfWork, store *surveystore.PublicRepository, slug string) surveyapp.PublicDefinitionRecord {
	t.Helper()
	var questionnaireID int64
	now := time.Now().UTC()
	err := uow.Within(ctx, func(tx context.Context) error {
		db, err := platformstore.TxFromContext(tx)
		if err != nil {
			return err
		}
		if err := db.QueryRow(tx, `INSERT INTO questionnaires(slug,name,title,description,answer_display_mode,assessment_enabled,assessment_config,is_disabled,created_by,version,submission_count,created_at,updated_at)
VALUES($1,$1,$1,'','all_in_one',FALSE,'{}'::jsonb,FALSE,1,1,0,$2,$2) RETURNING id`, slug, now).Scan(&questionnaireID); err != nil {
			return err
		}
		_, err = store.CreatePublicDefinition(tx, surveyport.Questionnaire{ID: surveyport.ID(questionnaireID), Slug: slug, Title: slug, AnswerDisplayMode: surveyport.AllInOne, Version: 1, Questions: []surveyport.Question{{ID: 9001, Type: surveyport.SingleChoice, Title: "choice", Required: true, SortOrder: 0, Validation: surveyport.Validation{MinimumSelections: surveyPushInt(1), MaximumSelections: surveyPushInt(1)}, Options: []surveyport.Option{{ID: 9002, OptionText: "yes", SortOrder: 0}}}}}, now)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := uowWithinDefinition(ctx, uow, store, surveyport.ID(questionnaireID))
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func uowWithinDefinition(ctx context.Context, uow *platformstore.UnitOfWork, store *surveystore.PublicRepository, questionnaireID surveyport.ID) (surveyapp.PublicDefinitionRecord, error) {
	var definition surveyapp.PublicDefinitionRecord
	err := uow.Within(ctx, func(tx context.Context) error {
		var err error
		definition, err = store.GetCurrentPublicDefinition(tx, questionnaireID)
		return err
	})
	return definition, err
}

func surveyPushInt(value int) *int { return &value }
func surveyPushKey(value byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}
func surveyPushRuntimeDigest(value string) eer.Digest {
	sum := sha256.Sum256([]byte(value))
	return eer.Digest("sha256:" + fmtHex(sum))
}
func fmtHex(sum [32]byte) string { return hex.EncodeToString(sum[:]) }

type surveyPushOAuthStates struct {
	state, next string
	claimed     bool
}

func (s *surveyPushOAuthStates) Begin(_ context.Context, provider authport.Provider, next string) (authport.OAuthAttempt, error) {
	if provider != authport.ProviderWeCom || s.state != "" {
		return authport.OAuthAttempt{}, authport.ErrOAuthStateInvalid
	}
	s.state, s.next = strings.Repeat("s", 43), next
	return authport.OAuthAttempt{State: authport.OAuthState(s.state), ExpiresAt: time.Now().UTC().Add(time.Minute)}, nil
}
func (s *surveyPushOAuthStates) Claim(_ context.Context, provider authport.Provider, state authport.OAuthState) (authport.OAuthClaim, error) {
	if provider != authport.ProviderWeCom || s.claimed || string(state) != s.state {
		return authport.OAuthClaim{}, authport.ErrOAuthStateInvalid
	}
	s.claimed = true
	return authport.OAuthClaim{Provider: provider, NextPath: s.next}, nil
}

type surveyPushVerifiedProvider struct{}

func (surveyPushVerifiedProvider) AuthorizationURL(string) (string, error) {
	return "https://provider.invalid/authorize", nil
}
func (surveyPushVerifiedProvider) ExchangeExternalIdentity(_ context.Context, code string) (surveyapp.H5ProviderIdentity, error) {
	if code != "verified-code" {
		return surveyapp.H5ProviderIdentity{}, errors.New("unverified code")
	}
	return surveyapp.H5ProviderIdentity{CorpID: "corp-verified", ExternalUserID: "external-verified"}, nil
}

type surveyPushIdentityResolver struct{}

func (surveyPushIdentityResolver) Resolve(_ context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	if ref.Kind != identityport.KindWeComExternalUserID || ref.Assurance != identityport.AssuranceVerified || ref.Scope != "corp-verified" || ref.Value != "external-verified" || ref.Source != "wecom_h5_oauth" {
		return identityport.ResolveResult{}, errors.New("untrusted identity")
	}
	return identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 4242}, nil
}

type surveyPushUnknownAdapter struct{}

func (surveyPushUnknownAdapter) Execute(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error) {
	return eer.AdapterResult{}, errors.New("fake dispatch lost after handoff")
}

type surveyPushBinder struct {
	delegate surveyapp.PublicExternalPushBinder
	err      error
}

func (b *surveyPushBinder) BindPublicSubmission(ctx context.Context, record surveyapp.PublicDefinitionRecord, submissionID int64, input surveyport.PublicSubmissionCommand, now time.Time) error {
	b.err = b.delegate.BindPublicSubmission(ctx, record, submissionID, input, now)
	return b.err
}
