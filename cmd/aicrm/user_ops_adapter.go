package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/userops/domain"
	useropshttp "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/http"
	useropsport "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/port"
)

// userOpsDirectoryReader is the only User Ops bridge to Contact and Identity.
// Phone input is resolved inside Identity and never crosses back into a User
// Ops projection.
type userOpsDirectoryReader struct {
	customers  *contactapp.CustomerListService
	references *contactapp.CustomerReferenceReader
	identities *identityapp.ResolveService
}

var _ useropsport.CustomerDirectoryReader = userOpsDirectoryReader{}

func (reader userOpsDirectoryReader) ReadOverview(ctx context.Context, query useropsport.DirectoryQuery) (useropsport.DirectoryOverviewRead, error) {
	input, err := reader.customerListInput(ctx, query, 1)
	if err != nil {
		return useropsport.DirectoryOverviewRead{}, err
	}
	page, err := reader.customers.ListInTransaction(ctx, input)
	if err != nil {
		return useropsport.DirectoryOverviewRead{}, userOpsContactError(err)
	}
	return useropsport.DirectoryOverviewRead{
		CustomerCount:           page.Total,
		CustomerCountIsEstimate: page.TotalIsEstimate,
	}, nil
}

func (reader userOpsDirectoryReader) ListCustomers(ctx context.Context, query useropsport.DirectoryQuery) (useropsport.DirectoryPageRead, error) {
	input, err := reader.customerListInput(ctx, query, query.Limit)
	if err != nil {
		return useropsport.DirectoryPageRead{}, err
	}
	page, err := reader.customers.ListInTransaction(ctx, input)
	if err != nil {
		return useropsport.DirectoryPageRead{}, userOpsContactError(err)
	}
	items := make([]useropsport.CustomerSummary, len(page.Items))
	for index, item := range page.Items {
		items[index] = userOpsCustomerSummary(item)
	}
	return useropsport.DirectoryPageRead{
		Items:           items,
		NextCursor:      cloneUserOpsString(page.NextCursor),
		Total:           page.Total,
		TotalIsEstimate: page.TotalIsEstimate,
	}, nil
}

func (reader userOpsDirectoryReader) ResolveCustomers(ctx context.Context, customerIDs []domain.CustomerID, mode useropsport.CustomerResolutionMode) ([]useropsport.CustomerSummary, error) {
	if reader.references == nil {
		return nil, useropsport.ErrUnavailable
	}
	if mode != useropsport.CustomerResolutionRead && mode != useropsport.CustomerResolutionForWrite {
		return nil, useropsport.ErrInvalid
	}
	contactIDs := make([]contactport.CustomerID, len(customerIDs))
	for index, customerID := range customerIDs {
		if !customerID.Valid() {
			return nil, useropsport.ErrInvalid
		}
		contactIDs[index] = contactport.CustomerID(customerID)
	}
	references, err := reader.references.ReadInTransaction(ctx, contactIDs)
	if errors.Is(err, contactapp.ErrCustomerNotFound) {
		return nil, useropsport.ErrNotFound
	}
	if err != nil {
		return nil, useropsport.ErrUnavailable
	}
	items := make([]useropsport.CustomerSummary, len(references))
	for index, reference := range references {
		items[index] = userOpsCustomerSummaryFromReference(reference)
	}
	return items, nil
}

func (reader userOpsDirectoryReader) customerListInput(ctx context.Context, query useropsport.DirectoryQuery, limit int32) (contactapp.CustomerListInput, error) {
	if reader.customers == nil || reader.identities == nil || ctx == nil || limit < 1 {
		return contactapp.CustomerListInput{}, useropsport.ErrUnavailable
	}
	input := contactapp.CustomerListInput{
		Keyword:      query.Keyword,
		OwnerStaffID: cloneUserOpsInt64(query.OwnerStaffID),
		StageID:      cloneUserOpsInt64(query.StageID),
		ChannelID:    cloneUserOpsInt64(query.ChannelID),
		TagID:        cloneUserOpsInt64(query.TagID),
		Cursor:       query.Cursor,
		Limit:        limit,
	}
	if query.PhoneExact == "" {
		return input, nil
	}
	ref := identityport.IDRef{
		Kind:      identityport.KindPhone,
		Scope:     "phone:e164",
		Value:     query.PhoneExact,
		Assurance: identityport.AssuranceVerified,
		Source:    "user_ops.directory.phone",
	}
	if _, err := identityapp.Normalize(ref); err != nil {
		return contactapp.CustomerListInput{}, useropsport.ErrInvalid
	}
	resolved, err := reader.identities.ResolveInTransaction(ctx, ref)
	if err != nil {
		return contactapp.CustomerListInput{}, useropsport.ErrUnavailable
	}
	switch resolved.Status {
	case identityport.ResolveFound:
		if resolved.CustomerID < 1 {
			return contactapp.CustomerListInput{}, useropsport.ErrUnavailable
		}
		customerID := resolved.CustomerID
		input.CustomerID = &customerID
	case identityport.ResolveNotFound:
		input.MatchNone = true
	case identityport.ResolveConflict:
		return contactapp.CustomerListInput{}, useropsport.ErrConflict
	default:
		return contactapp.CustomerListInput{}, useropsport.ErrUnavailable
	}
	return input, nil
}

// userOpsCustomerDetailReader narrows Contact's verified detail projection to
// the safe fields frozen by User Ops. It deliberately exposes no messages,
// identity values, avatars, extras, or provider metadata.
type userOpsCustomerDetailReader struct {
	details *contactapp.CustomerDetailService
	events  contactapp.CustomerEventStore
}

var _ useropsport.CustomerDetailReader = userOpsCustomerDetailReader{}

func (reader userOpsCustomerDetailReader) ReadCustomerDetail(ctx context.Context, customerID domain.CustomerID) (useropsport.CustomerDetail, error) {
	if reader.details == nil || !customerID.Valid() {
		return useropsport.CustomerDetail{}, useropsport.ErrUnavailable
	}
	result, err := reader.details.GetInTransaction(ctx, contactapp.CustomerDetailInput{ID: contactport.CustomerID(customerID)})
	if errors.Is(err, contactapp.ErrCustomerNotFound) {
		return useropsport.CustomerDetail{}, useropsport.ErrNotFound
	}
	if err != nil {
		return useropsport.CustomerDetail{}, useropsport.ErrUnavailable
	}
	if reader.events == nil {
		return useropsport.CustomerDetail{}, useropsport.ErrUnavailable
	}
	events, err := reader.events.ListCustomerEvents(ctx, contactapp.CustomerEventQuery{
		CustomerID: contactport.CustomerID(customerID),
		Limit:      30,
	})
	if errors.Is(err, contactapp.ErrCustomerNotFound) {
		return useropsport.CustomerDetail{}, useropsport.ErrNotFound
	}
	if err != nil {
		return useropsport.CustomerDetail{}, useropsport.ErrUnavailable
	}
	tags := make([]useropsport.CustomerTag, len(result.Tags))
	for index, tag := range result.Tags {
		tags[index] = useropsport.CustomerTag{
			ID:        tag.ID,
			GroupID:   cloneUserOpsInt64(tag.GroupID),
			GroupName: cloneUserOpsString(tag.GroupName),
			Name:      tag.Name,
		}
	}
	timeline := make([]useropsport.TimelineEntry, len(events.Items))
	for index, event := range events.Items {
		timeline[index] = useropsport.TimelineEntry{EventType: event.EventType, OccurredAt: event.OccurredAt.UTC()}
	}
	return useropsport.CustomerDetail{
		Customer: userOpsCustomerSummary(result.Customer),
		Tags:     tags,
		Timeline: timeline,
	}, nil
}

// userOpsMaterialReader locks existing local material facts in the current
// User Ops UnitOfWork. It returns only eligibility and never reads a URL or
// provider field.
type userOpsMaterialReader struct {
	images       mediaport.ImageMetadataReader
	miniPrograms mediaport.ChannelMiniProgramReferenceReader
	attachments  mediaport.ChannelAttachmentReferenceReader
}

var _ useropsport.MaterialReader = userOpsMaterialReader{}

func (reader userOpsMaterialReader) ImageEligible(ctx context.Context, id int64) (bool, error) {
	if reader.images == nil || id < 1 {
		return false, useropsport.ErrUnavailable
	}
	eligible, err := reader.images.ImageExists(ctx, id)
	if err != nil {
		return false, useropsport.ErrUnavailable
	}
	return eligible, nil
}

func (reader userOpsMaterialReader) MiniProgramEligible(ctx context.Context, id int64) (bool, error) {
	if reader.miniPrograms == nil || id < 1 {
		return false, useropsport.ErrUnavailable
	}
	eligible, err := reader.miniPrograms.ChannelMiniProgramEligible(ctx, id)
	if err != nil {
		return false, useropsport.ErrUnavailable
	}
	return eligible, nil
}

func (reader userOpsMaterialReader) AttachmentEligible(ctx context.Context, id int64) (bool, error) {
	if reader.attachments == nil || id < 1 {
		return false, useropsport.ErrUnavailable
	}
	eligible, err := reader.attachments.ChannelAttachmentEligible(ctx, id)
	if err != nil {
		return false, useropsport.ErrUnavailable
	}
	return eligible, nil
}

type userOpsEventAppender struct {
	appender eventport.Appender
}

var _ useropsport.EventAppender = userOpsEventAppender{}

func (appender userOpsEventAppender) Append(ctx context.Context, event useropsport.LocalEvent) error {
	if appender.appender == nil || event.ActorID < 1 || event.Type == "" || event.OccurredAt.IsZero() || event.IdempotencyKey == "" {
		return useropsport.ErrUnavailable
	}
	payload, err := json.Marshal(struct {
		SchemaVersion string `json:"schema_version"`
		PlanID        int64  `json:"plan_id,omitempty"`
		CustomerID    int64  `json:"customer_id,omitempty"`
		Version       int64  `json:"version"`
		TargetCount   int32  `json:"target_count,omitempty"`
	}{
		SchemaVersion: "user_ops_local_v1",
		PlanID:        int64(event.PlanID),
		CustomerID:    int64(event.CustomerID),
		Version:       event.Version,
		TargetCount:   event.TargetCount,
	})
	if err != nil {
		return useropsport.ErrUnavailable
	}
	fingerprint := event.Type + "|" + strconv.FormatInt(event.ActorID, 10) + "|" + strconv.FormatInt(int64(event.CustomerID), 10) + "|" + strconv.FormatInt(int64(event.PlanID), 10) + "|" + strconv.FormatInt(event.Version, 10) + "|" + event.IdempotencyKey
	digest := sha256.Sum256([]byte(fingerprint))
	_, err = appender.appender.Append(ctx, eventport.Event{
		Type:           event.Type,
		CustomerID:     eventport.CustomerID(event.CustomerID),
		Payload:        payload,
		OccurredAt:     event.OccurredAt,
		IdempotencyKey: "user_ops:" + hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return useropsport.ErrUnavailable
	}
	return nil
}

// userOpsAuthorizer accepts only canonical global operations capabilities.
// It intentionally rejects sales/owner scoped contexts even if one is forged.
type userOpsAuthorizer struct{}

var _ useropshttp.Authorizer = userOpsAuthorizer{}

func (userOpsAuthorizer) Authorize(ctx context.Context, permission useropshttp.Permission) (useropshttp.Actor, error) {
	principal, principalOK := authport.PrincipalFromContext(ctx)
	_, sessionOK := authport.SessionFromContext(ctx)
	authorization, authorizationOK := authport.AuthorizationFromContext(ctx)
	if !principalOK || !sessionOK || principal.AdminUserID < 1 {
		return useropshttp.Actor{}, useropshttp.ErrUnauthenticated
	}
	if principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps {
		return useropshttp.Actor{}, useropshttp.ErrForbidden
	}
	expected := authport.CapabilityOperationsRead
	if permission == useropshttp.PermissionAdminWrite {
		expected = authport.CapabilityOperationsManage
	} else if permission != useropshttp.PermissionAdminRead {
		return useropshttp.Actor{}, useropshttp.ErrForbidden
	}
	if !authorizationOK || authorization.Capability != expected || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return useropshttp.Actor{}, useropshttp.ErrForbidden
	}
	return useropshttp.Actor{ID: principal.AdminUserID}, nil
}

// userOpsCanonicalCSRF intentionally does not validate a token. The root
// route uses auth.Handler.RequireCSRF as the single canonical validator; this
// seam merely rejects direct unwrapped leaf calls that lack its trusted
// session and global operations-manage authorization context.
type userOpsCanonicalCSRF struct{}

var _ useropshttp.CSRFVerifier = userOpsCanonicalCSRF{}

func (userOpsCanonicalCSRF) Verify(request *http.Request) error {
	if request == nil {
		return useropshttp.ErrCSRFInvalid
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	_, sessionOK := authport.SessionFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	if !principalOK || !sessionOK || principal.AdminUserID < 1 ||
		!authorizationOK || authorization.Capability != authport.CapabilityOperationsManage || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return useropshttp.ErrCSRFInvalid
	}
	return nil
}

func userOpsCustomerSummary(record contactapp.CustomerRecord) useropsport.CustomerSummary {
	return useropsport.CustomerSummary{
		CustomerID:     domain.CustomerID(record.ID),
		Name:           record.Name,
		OwnerStaffID:   cloneUserOpsInt64(record.OwnerStaffID),
		StageID:        cloneUserOpsInt64(record.StageID),
		ChannelID:      cloneUserOpsInt64(record.ChannelID),
		AddedAt:        cloneUserOpsTime(record.AddedAt),
		LastInteractAt: cloneUserOpsTime(record.LastInteractAt),
	}
}

func userOpsCustomerSummaryFromReference(record contactapp.CustomerReferenceRecord) useropsport.CustomerSummary {
	return useropsport.CustomerSummary{
		CustomerID: domain.CustomerID(record.ID),
	}
}

func userOpsContactError(err error) error {
	if errors.Is(err, contactapp.ErrInvalidCustomerListQuery) {
		return useropsport.ErrInvalid
	}
	return useropsport.ErrUnavailable
}

func cloneUserOpsInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneUserOpsString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneUserOpsTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
