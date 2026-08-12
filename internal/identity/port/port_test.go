package port

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestFrozenIdentityKindsAssurancesAndStatuses(t *testing.T) {
	for _, test := range []struct {
		name string
		got  IDKind
		want IDKind
	}{
		{"wecom external user ID", KindWeComExternalUserID, "wecom_external_userid"},
		{"union ID", KindUnionID, "unionid"},
		{"mini-program open ID", KindMPOpenID, "mp_openid"},
		{"OA open ID", KindOAOpenID, "oa_openid"},
		{"Alipay user ID", KindAlipayUserID, "alipay_user_id"},
		{"phone", KindPhone, "phone"},
		{"extension", KindExtension, "ext"},
	} {
		if test.got != test.want {
			t.Fatalf("%s = %q; want %q", test.name, test.got, test.want)
		}
	}
	if AssuranceVerified != "verified" || AssuranceDeclared != "declared" {
		t.Fatal("identity assurance values drifted")
	}
	if MergePolicyVerifiedUnionIDUniqueWeCom != "verified_unionid_unique_wecom_v1" {
		t.Fatal("merge policy value drifted")
	}
	if ResolveFound != "found" || ResolveNotFound != "not_found" || ResolveConflict != "conflict" {
		t.Fatal("resolve status values drifted")
	}
	if BindBound != "bound" || BindAlreadyBound != "already_bound" || BindMerged != "merged" ||
		BindManualReview != "manual_review" || BindRejected != "rejected" {
		t.Fatal("bind status values drifted")
	}
	if IngestAttributed != "attributed" || IngestPending != "pending" || IngestConflict != "conflict" {
		t.Fatal("ingest status values drifted")
	}
	if MergeReviewPending != "pending" || MergeReviewApproved != "approved" || MergeReviewRejected != "rejected" {
		t.Fatal("merge review status values drifted")
	}
}

func TestServerDerivedIdempotencyScopesAreClosedAndStable(t *testing.T) {
	admin, err := NewAdminIdempotencyScope(17)
	if err != nil || admin.String() != "admin:17" || !admin.Valid() {
		t.Fatalf("admin scope = %q/%v, error = %v", admin.String(), admin.Valid(), err)
	}
	integration, err := NewIntegrationIdempotencyScope("gateway", 23)
	if err != nil || integration.String() != "integration:gateway:23" || !integration.Valid() {
		t.Fatalf("integration scope = %q/%v, error = %v", integration.String(), integration.Valid(), err)
	}
	for _, invalid := range []struct {
		provider string
		id       int64
	}{
		{"", 1}, {"Gateway", 1}, {"gateway/raw", 1}, {"gateway", 0},
	} {
		if _, err := NewIntegrationIdempotencyScope(invalid.provider, invalid.id); !errors.Is(err, ErrInvalidIdempotencyScope) {
			t.Fatalf("NewIntegrationIdempotencyScope(%q, %d) error = %v", invalid.provider, invalid.id, err)
		}
	}
	if _, err := NewAdminIdempotencyScope(0); !errors.Is(err, ErrInvalidIdempotencyScope) {
		t.Fatalf("NewAdminIdempotencyScope(0) error = %v", err)
	}
}

func TestMutationCommandsCarryTypedServerScope(t *testing.T) {
	scope, err := NewAdminIdempotencyScope(9)
	if err != nil {
		t.Fatal(err)
	}
	bind := BindCommand{IdempotencyScope: scope, IdempotencyKey: "bind-key"}
	ingest := IngestCommand{IdempotencyScope: scope, IdempotencyKey: "ingest-key"}
	approve := ApproveMergeReviewCommand{IdempotencyScope: scope, IdempotencyKey: "approve-key"}
	reject := RejectMergeReviewCommand{IdempotencyScope: scope, IdempotencyKey: "reject-key"}
	if bind.IdempotencyScope.String() != scope.String() || ingest.IdempotencyScope.String() != scope.String() ||
		approve.IdempotencyScope.String() != scope.String() || reject.IdempotencyScope.String() != scope.String() {
		t.Fatal("mutation command lost typed idempotency scope")
	}
	if NormalizerV1 != 1 || CommandSchemaV1 != 1 || ResultSchemaV1 != 1 {
		t.Fatal("identity schema version contract drifted")
	}
}

func TestFrozenMergeReviewPortSurface(t *testing.T) {
	var _ ReviewService = reviewServiceContractStub{}

	ref := IDRef{
		Kind:      KindUnionID,
		Scope:     "wechat-open-platform:tenant-1",
		Value:     "internal-only",
		Assurance: AssuranceVerified,
		Source:    "wecom",
	}
	actor := contactport.Actor("admin:17")
	createdAt := time.Date(2026, time.August, 12, 1, 2, 3, 0, time.UTC)
	review := MergeReview{
		ReviewID:            41,
		Status:              MergeReviewPending,
		Kind:                KindPhone,
		Scope:               ref.Scope,
		IdentityFingerprint: "fingerprint-only",
		CustomerIDs:         []contactport.CustomerID{7, 9},
		Version:             3,
		CreatedAt:           createdAt,
	}
	page := MergeReviewPage{Items: []MergeReview{review}, NextCursor: "next"}
	bind := BindResult{Status: BindManualReview, ReviewID: review.ReviewID}
	approve := ApproveMergeReviewCommand{
		ReviewID:          review.ReviewID,
		ExpectedVersion:   review.Version,
		PrimaryCustomerID: review.CustomerIDs[0],
		Reason:            "verified operator decision",
		Actor:             actor,
		IdempotencyKey:    "review-approve-key",
	}
	reject := RejectMergeReviewCommand{
		ReviewID:        review.ReviewID,
		ExpectedVersion: review.Version,
		Reason:          "evidence is insufficient",
		Actor:           actor,
		IdempotencyKey:  "review-reject-key",
	}

	if len(page.Items) != 1 || page.Items[0].IdentityFingerprint == "" || page.Items[0].CreatedAt != createdAt ||
		page.Items[0].ResolvedAt != nil || bind.ReviewID == 0 ||
		approve.IdempotencyKey == "" || reject.IdempotencyKey == "" {
		t.Fatal("merge review port surface drifted")
	}
	for _, field := range []string{"Value", "NormalizedValue"} {
		if _, found := reflect.TypeOf(MergeReview{}).FieldByName(field); found {
			t.Fatalf("merge review exposes raw identity field %q", field)
		}
	}
}

type reviewServiceContractStub struct{}

func (reviewServiceContractStub) ListMergeReviews(context.Context, string, int32) (MergeReviewPage, error) {
	return MergeReviewPage{}, nil
}

func (reviewServiceContractStub) ApproveMergeReview(context.Context, ApproveMergeReviewCommand) (MergeReview, error) {
	return MergeReview{}, nil
}

func (reviewServiceContractStub) RejectMergeReview(context.Context, RejectMergeReviewCommand) (MergeReview, error) {
	return MergeReview{}, nil
}
