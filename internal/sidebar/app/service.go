package app

import (
	"context"
	"crypto/hmac"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

var (
	ErrInvalidInput     = errors.New("invalid sidebar input")
	ErrViewerSession    = errors.New("sidebar viewer session required")
	ErrCustomerNotBound = errors.New("sidebar customer is not bound")
	ErrForbidden        = errors.New("sidebar customer scope forbidden")
	ErrTokenInvalid     = errors.New("sidebar context token invalid")
	ErrTokenExpired     = errors.New("sidebar context token expired")
	ErrNotFound         = errors.New("sidebar resource not found")
	ErrConflict         = errors.New("sidebar write conflict")
	ErrUnavailable      = errors.New("sidebar unavailable")
)

type CorpReader interface {
	CorpID(context.Context) (string, error)
}

type IdentityResolver interface {
	Resolve(context.Context, identityport.IDRef) (identityport.ResolveResult, error)
}

type PhoneBindingCommand struct {
	CustomerID     int64
	Mobile         string
	ActorID        int64
	IdempotencyKey string
}

type PhoneBinder interface {
	BindPhone(context.Context, PhoneBindingCommand) (string, error)
}

type MemberApplication interface {
	Get(context.Context, int64, string) (PeriodicMember, error)
	UpdateRemark(context.Context, PeriodicRemarkCommand) (PeriodicMember, error)
	ListCustomer(context.Context, PeriodicListQuery) (PeriodicListResult, error)
}

type Safety struct {
	LocalOnly                 bool `json:"local_only"`
	ProviderExecutionEligible bool `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool `json:"real_external_call_executed"`
}

func localSafety() Safety { return Safety{LocalOnly: true} }

type ContextResult struct {
	State        string    `json:"state"`
	Token        string    `json:"context_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	CustomerID   int64     `json:"customer_id,omitempty"`
	OwnerStaffID int64     `json:"owner_staff_id,omitempty"`
	Safety       Safety    `json:"safety"`
}

type Scope struct {
	CustomerID, OwnerStaffID int64
	Principal                authport.Principal
}

type ProfileResult struct {
	Profile contactport.SidebarProfile `json:"profile"`
	Safety  Safety                     `json:"safety"`
}

type ProfileUpdateSafety struct {
	LocalOnly                 bool `json:"local_only"`
	EffectQueued              bool `json:"effect_queued"`
	ProviderExecutionEligible bool `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool `json:"real_external_call_executed"`
}

type ProfileUpdateResult struct {
	Profile contactport.SidebarProfile `json:"profile"`
	Safety  ProfileUpdateSafety        `json:"safety"`
}

type QuestionnaireItem struct {
	SubmissionID    int64                                `json:"submission_id"`
	QuestionnaireID int64                                `json:"questionnaire_id"`
	SubmittedAt     time.Time                            `json:"submitted_at"`
	Score           float64                              `json:"score"`
	ChoiceAnswers   []surveyport.SafeChoiceAnswerPreview `json:"choice_answers"`
}

type QuestionnaireResult struct {
	Items           []QuestionnaireItem `json:"items"`
	ScanTruncated   bool                `json:"scan_truncated"`
	ResultTruncated bool                `json:"result_truncated"`
	Safety          Safety              `json:"safety"`
}

type OrderItem struct {
	CreatedAt       time.Time `json:"created_at"`
	MerchantOrderNo string    `json:"merchant_order_no"`
	ProductCode     string    `json:"product_code"`
	ProductName     string    `json:"product_name"`
	AmountYuan      string    `json:"amount_yuan"`
	Currency        string    `json:"currency"`
	Status          string    `json:"status"`
	StatusLabel     string    `json:"status_label"`
	Provider        string    `json:"provider"`
	ProviderLabel   string    `json:"provider_label"`
}

type OrderResult struct {
	Items   []OrderItem `json:"items"`
	Total   int64       `json:"total"`
	Limit   int32       `json:"limit"`
	HasMore bool        `json:"has_more"`
	Safety  Safety      `json:"safety"`
}

type PeriodicResult struct {
	Items   []PeriodicMember `json:"items"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
	HasMore bool             `json:"has_more"`
	Safety  Safety           `json:"safety"`
}

type PeriodicMember struct {
	MemberRef        string     `json:"member_ref"`
	ServiceProductID int64      `json:"service_product_id"`
	CustomerID       int64      `json:"customer_id"`
	State            string     `json:"state"`
	Source           string     `json:"source"`
	StartsAt         time.Time  `json:"starts_at"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	ExpiredAt        *time.Time `json:"expired_at,omitempty"`
	RemovedAt        *time.Time `json:"removed_at,omitempty"`
	Remark           *string    `json:"remark,omitempty"`
	Alliance         *string    `json:"alliance,omitempty"`
	Version          int64      `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type PeriodicListQuery struct {
	CustomerID int64
	Limit      int
	Offset     int
}

type PeriodicListResult struct {
	Items   []PeriodicMember
	Limit   int
	Offset  int
	HasMore bool
}

type PeriodicRemarkCommand struct {
	ServiceProductID int64
	MemberRef        string
	ExpectedVersion  int64
	Remark           *string
	Alliance         *string
	ActorID          int64
	IdempotencyKey   string
}

type PeriodicRemarkResult struct {
	Member PeriodicMember `json:"member"`
	Safety Safety         `json:"safety"`
}

type MaterialItem struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	FileName    string   `json:"file_name"`
	MimeType    string   `json:"mime_type"`
	FileSize    int32    `json:"file_size"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Category    string   `json:"category"`
	Width       int32    `json:"width"`
	Height      int32    `json:"height"`
	UpdatedAt   string   `json:"updated_at"`
	Thumbnail   string   `json:"thumbnail_status"`
}

type MaterialResult struct {
	Items         []MaterialItem `json:"items"`
	Total         int64          `json:"total"`
	Limit         int64          `json:"limit"`
	Offset        int64          `json:"offset"`
	QuickKeywords []string       `json:"quick_keywords"`
	Safety        Safety         `json:"safety"`
}

type WorkbenchResult struct {
	Profile            contactport.SidebarProfile `json:"profile"`
	QuestionnaireCount int                        `json:"questionnaire_count"`
	OrderCount         int64                      `json:"order_count"`
	PeriodicOrderCount int                        `json:"periodic_order_count"`
	MaterialCount      int64                      `json:"material_count"`
	Safety             Safety                     `json:"safety"`
}

type BootstrapResult struct {
	State        string           `json:"state"`
	Token        string           `json:"context_token,omitempty"`
	ExpiresAt    time.Time        `json:"expires_at,omitempty"`
	CustomerID   int64            `json:"customer_id,omitempty"`
	OwnerStaffID int64            `json:"owner_staff_id,omitempty"`
	Workbench    *WorkbenchResult `json:"workbench,omitempty"`
	Safety       Safety           `json:"safety"`
}

type resolvedContext struct {
	result  ContextResult
	profile contactport.SidebarProfile
}

type Service struct {
	corp           CorpReader
	identity       IdentityResolver
	phones         PhoneBinder
	profiles       contactport.SidebarProfileService
	surveys        surveyport.CustomerSurveyAnswerReader
	orders         orderport.Query
	members        MemberApplication
	media          mediaport.ImageLibraryReader
	workbench      WorkbenchReader
	variants       mediaport.ImageVariantReader
	products       ShareableProductCatalog
	temporaryMedia TemporaryMediaPreparer
	codec          *tokenCodec
	now            func() time.Time
	tokenTTL       time.Duration
}

func NewService(corp CorpReader, identity IdentityResolver, phones PhoneBinder, profiles contactport.SidebarProfileService, surveys surveyport.CustomerSurveyAnswerReader, orders orderport.Query, members MemberApplication, media mediaport.ImageLibraryReader, tokenKey []byte, options ...ServiceOptions) (*Service, error) {
	codec, err := newTokenCodec(tokenKey)
	if err != nil {
		return nil, err
	}
	if corp == nil || identity == nil || phones == nil || profiles == nil || surveys == nil || orders == nil || members == nil || media == nil {
		return nil, ErrUnavailable
	}
	service := &Service{corp: corp, identity: identity, phones: phones, profiles: profiles, surveys: surveys, orders: orders, members: members, media: media, workbench: domainWorkbenchReader{surveys: surveys, orders: orders, members: members, media: media}, codec: codec, now: time.Now, tokenTTL: 15 * time.Minute}
	if len(options) > 1 {
		return nil, ErrUnavailable
	}
	if len(options) == 1 {
		service.products = options[0].Products
		service.temporaryMedia = options[0].Media
		if options[0].Workbench != nil {
			service.workbench = options[0].Workbench
		}
	}
	service.variants, _ = media.(mediaport.ImageVariantReader)
	return service, nil
}

func (service *Service) MintContext(ctx context.Context, principal authport.Principal, session authport.SessionRef, authenticated bool, externalUserID string) (ContextResult, error) {
	resolved, err := service.resolveContext(ctx, principal, session, authenticated, externalUserID)
	return resolved.result, err
}

func (service *Service) Bootstrap(ctx context.Context, principal authport.Principal, session authport.SessionRef, authenticated bool, externalUserID string) (BootstrapResult, error) {
	resolved, err := service.resolveContext(ctx, principal, session, authenticated, externalUserID)
	if err != nil {
		return BootstrapResult{}, err
	}
	result := BootstrapResult{
		State:  resolved.result.State,
		Safety: resolved.result.Safety,
	}
	if resolved.result.State != "ready" {
		return result, nil
	}
	workbench, err := service.workbenchWithProfile(ctx, Scope{
		CustomerID: resolved.result.CustomerID, OwnerStaffID: resolved.result.OwnerStaffID, Principal: principal,
	}, resolved.profile)
	if err != nil {
		return BootstrapResult{}, err
	}
	result.Token = resolved.result.Token
	result.ExpiresAt = resolved.result.ExpiresAt
	result.CustomerID = resolved.result.CustomerID
	result.OwnerStaffID = resolved.result.OwnerStaffID
	result.Workbench = &workbench
	return result, nil
}

func (service *Service) resolveContext(ctx context.Context, principal authport.Principal, session authport.SessionRef, authenticated bool, externalUserID string) (resolvedContext, error) {
	if !validExternalUserID(externalUserID) {
		return resolvedContext{}, ErrInvalidInput
	}
	if !authenticated {
		return resolvedContext{result: ContextResult{State: "viewer_session_required", Safety: localSafety()}}, nil
	}
	if !validPrincipal(principal) {
		return resolvedContext{}, ErrInvalidInput
	}
	sessionFingerprint, err := service.codec.sessionFingerprint(session)
	if err != nil {
		return resolvedContext{}, ErrViewerSession
	}
	corpID, err := service.corp.CorpID(ctx)
	if err != nil || corpID == "" {
		return resolvedContext{}, ErrUnavailable
	}
	resolved, err := service.identity.Resolve(ctx, identityport.IDRef{Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:" + corpID, Value: externalUserID, Assurance: identityport.AssuranceVerified, Source: "sidebar"})
	if err != nil {
		return resolvedContext{}, ErrUnavailable
	}
	if resolved.Status != identityport.ResolveFound || resolved.CustomerID < 1 {
		return resolvedContext{result: ContextResult{State: "customer_not_bound", Safety: localSafety()}}, nil
	}
	profile, err := service.profiles.ResolveSidebarProfile(ctx, resolved.CustomerID)
	if err != nil {
		return resolvedContext{}, mapDependencyError(err)
	}
	if !principalAllowsOwner(principal, profile.OwnerStaffID) {
		return resolvedContext{result: ContextResult{State: "customer_not_bound", Safety: localSafety()}}, nil
	}
	now := service.now().UTC().Truncate(time.Second)
	claims := tokenClaims{Version: tokenVersion, CorpID: corpID, CustomerID: int64(profile.CustomerID), OwnerStaffID: profile.OwnerStaffID, AdminUserID: principal.AdminUserID, Role: principal.Role, SessionFingerprint: sessionFingerprint, IssuedAt: now, ExpiresAt: now.Add(service.tokenTTL)}
	token, err := service.codec.encode(claims)
	if err != nil {
		return resolvedContext{}, err
	}
	return resolvedContext{
		result:  ContextResult{State: "ready", Token: token, ExpiresAt: claims.ExpiresAt, CustomerID: claims.CustomerID, OwnerStaffID: claims.OwnerStaffID, Safety: localSafety()},
		profile: profile,
	}, nil
}

func (service *Service) VerifyContext(ctx context.Context, principal authport.Principal, session authport.SessionRef, token string) (Scope, error) {
	if service == nil || ctx == nil || !validPrincipal(principal) {
		return Scope{}, ErrTokenInvalid
	}
	claims, err := service.codec.decode(token)
	if err != nil {
		return Scope{}, tokenError(err)
	}
	if !service.now().UTC().Before(claims.ExpiresAt) {
		return Scope{}, ErrTokenExpired
	}
	sessionFingerprint, err := service.codec.sessionFingerprint(session)
	if err != nil || !hmac.Equal([]byte(claims.SessionFingerprint), []byte(sessionFingerprint)) {
		return Scope{}, ErrTokenInvalid
	}
	if claims.AdminUserID != principal.AdminUserID || claims.Role != principal.Role || !principalAllowsOwner(principal, claims.OwnerStaffID) {
		return Scope{}, ErrForbidden
	}
	corpID, err := service.corp.CorpID(ctx)
	if err != nil || corpID == "" {
		return Scope{}, ErrUnavailable
	}
	if claims.CorpID != corpID {
		return Scope{}, ErrTokenInvalid
	}
	profile, err := service.profiles.ResolveSidebarProfile(ctx, contactport.CustomerID(claims.CustomerID))
	if errors.Is(err, contactport.ErrSidebarProfileNotFound) {
		return Scope{}, ErrTokenInvalid
	}
	if err != nil {
		return Scope{}, ErrUnavailable
	}
	if int64(profile.CustomerID) != claims.CustomerID || profile.OwnerStaffID != claims.OwnerStaffID {
		return Scope{}, ErrTokenInvalid
	}
	return Scope{CustomerID: claims.CustomerID, OwnerStaffID: claims.OwnerStaffID, Principal: principal}, nil
}

func (service *Service) Profile(ctx context.Context, scope Scope) (ProfileResult, error) {
	profile, err := service.profiles.ReadSidebarProfile(ctx, contactport.CustomerID(scope.CustomerID), scope.OwnerStaffID)
	return ProfileResult{Profile: profile, Safety: localSafety()}, mapDependencyError(err)
}

func (service *Service) UpdateProfile(ctx context.Context, scope Scope, expected time.Time, patch contactport.SidebarProfilePatch, key string) (ProfileUpdateResult, error) {
	command := contactport.SidebarProfileUpdateCommand{CustomerID: contactport.CustomerID(scope.CustomerID), OwnerStaffID: scope.OwnerStaffID, ExpectedUpdatedAt: expected, Patch: patch, Actor: contactport.Actor("admin:" + int64String(scope.Principal.AdminUserID)), IdempotencyKey: key}
	if effects, ok := service.profiles.(contactport.SidebarProfileEffectService); ok {
		result, err := effects.UpdateSidebarProfileWithEffect(ctx, command)
		return ProfileUpdateResult{Profile: result.Profile, Safety: ProfileUpdateSafety{
			LocalOnly: !result.EffectQueued, EffectQueued: result.EffectQueued,
			ProviderExecutionEligible: result.ProviderExecutionEligible,
		}}, mapDependencyError(err)
	}
	profile, err := service.profiles.UpdateSidebarProfile(ctx, command)
	return ProfileUpdateResult{Profile: profile, Safety: ProfileUpdateSafety{LocalOnly: true}}, mapDependencyError(err)
}

func (service *Service) BindPhone(ctx context.Context, scope Scope, mobile, key string) (string, error) {
	if ctx == nil || scope.CustomerID < 1 || scope.Principal.AdminUserID < 1 || service == nil || service.phones == nil {
		return "", ErrInvalidInput
	}
	status, err := service.phones.BindPhone(ctx, PhoneBindingCommand{CustomerID: scope.CustomerID, Mobile: mobile, ActorID: scope.Principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		return "", err
	}
	if status != "bound" && status != "already_bound" && status != "rejected" {
		return "", ErrUnavailable
	}
	return status, nil
}

func (service *Service) Questionnaires(ctx context.Context, scope Scope, limit int32) (QuestionnaireResult, error) {
	page, err := service.surveys.ListCustomerSurveyAnswers(ctx, contactport.CustomerID(scope.CustomerID), limit)
	if err != nil {
		return QuestionnaireResult{}, mapDependencyError(err)
	}
	items := make([]QuestionnaireItem, len(page.Items))
	for index, item := range page.Items {
		items[index] = QuestionnaireItem{SubmissionID: item.SubmissionID, QuestionnaireID: int64(item.QuestionnaireID), SubmittedAt: item.SubmittedAt, Score: item.Score, ChoiceAnswers: append([]surveyport.SafeChoiceAnswerPreview(nil), item.ChoiceAnswers...)}
	}
	return QuestionnaireResult{Items: items, ScanTruncated: page.ScanTruncated, ResultTruncated: page.ResultTruncated, Safety: localSafety()}, nil
}

func (service *Service) Orders(ctx context.Context, scope Scope, limit, offset int32) (OrderResult, error) {
	customerID := scope.CustomerID
	page, err := service.orders.List(ctx, orderport.Filter{CustomerID: &customerID, Limit: limit, Offset: offset})
	if err != nil {
		return OrderResult{}, mapDependencyError(err)
	}
	items := make([]OrderItem, len(page.Items))
	for index, item := range page.Items {
		items[index] = OrderItem{CreatedAt: item.CreatedAt, MerchantOrderNo: item.MerchantOrderNo, ProductCode: item.ProductCode, ProductName: item.ProductName, AmountYuan: item.AmountYuan, Currency: item.Currency, Status: item.Status, StatusLabel: item.StatusLabel, Provider: item.Provider, ProviderLabel: item.ProviderLabel}
	}
	return OrderResult{Items: items, Total: page.Total, Limit: page.Limit, HasMore: page.HasMore, Safety: localSafety()}, nil
}

func (service *Service) PeriodicOrders(ctx context.Context, scope Scope, limit, offset int) (PeriodicResult, error) {
	page, err := service.members.ListCustomer(ctx, PeriodicListQuery{CustomerID: scope.CustomerID, Limit: limit, Offset: offset})
	if err != nil {
		return PeriodicResult{}, mapDependencyError(err)
	}
	for _, member := range page.Items {
		if member.CustomerID != scope.CustomerID {
			return PeriodicResult{}, ErrUnavailable
		}
	}
	return PeriodicResult{Items: page.Items, Limit: page.Limit, Offset: page.Offset, HasMore: page.HasMore, Safety: localSafety()}, nil
}

func (service *Service) UpdatePeriodicRemark(ctx context.Context, scope Scope, serviceProductID int64, memberRef string, expectedVersion int64, remark *string, key string) (PeriodicRemarkResult, error) {
	current, err := service.members.Get(ctx, serviceProductID, memberRef)
	if err != nil {
		return PeriodicRemarkResult{}, mapDependencyError(err)
	}
	if current.CustomerID != scope.CustomerID {
		return PeriodicRemarkResult{}, ErrNotFound
	}
	updated, err := service.members.UpdateRemark(ctx, PeriodicRemarkCommand{ServiceProductID: serviceProductID, MemberRef: memberRef, ExpectedVersion: expectedVersion, Remark: remark, Alliance: current.Alliance, ActorID: scope.Principal.AdminUserID, IdempotencyKey: key})
	return PeriodicRemarkResult{Member: updated, Safety: localSafety()}, mapDependencyError(err)
}

func (service *Service) Materials(ctx context.Context, query mediaport.ImageListQuery) (MaterialResult, error) {
	page, err := service.media.ListImages(ctx, query)
	if err != nil {
		return MaterialResult{}, ErrUnavailable
	}
	facets, err := service.media.Facets(ctx)
	if err != nil {
		return MaterialResult{}, ErrUnavailable
	}
	items := make([]MaterialItem, len(page.Items))
	for index, item := range page.Items {
		items[index] = MaterialItem{ID: item.ID, Name: item.Name, FileName: item.FileName, MimeType: item.MimeType, FileSize: item.FileSize, Description: item.Description, Tags: append([]string(nil), item.Tags...), Category: item.Category, Width: item.Width, Height: item.Height, UpdatedAt: item.UpdatedAt, Thumbnail: "pending"}
	}
	keywords := append(append([]string(nil), facets.Categories...), facets.Tags...)
	sort.Strings(keywords)
	keywords = compactStrings(keywords)
	return MaterialResult{Items: items, Total: page.Total, Limit: page.Limit, Offset: page.Offset, QuickKeywords: keywords, Safety: localSafety()}, nil
}

func (service *Service) ThumbnailStatus(ctx context.Context, imageID int64) (string, error) {
	if imageID < 1 {
		return "", ErrInvalidInput
	}
	exists, err := service.media.LocalImageExists(ctx, imageID)
	if err != nil {
		return "", ErrUnavailable
	}
	if !exists {
		return "", ErrNotFound
	}
	return "pending", nil
}

func (service *Service) ThumbnailPreview(ctx context.Context, imageID int64) (mediaport.ImageVariant, error) {
	if imageID < 1 {
		return mediaport.ImageVariant{}, ErrInvalidInput
	}
	exists, err := service.media.LocalImageExists(ctx, imageID)
	if err != nil {
		return mediaport.ImageVariant{}, ErrUnavailable
	}
	if !exists {
		return mediaport.ImageVariant{}, ErrNotFound
	}
	if service.variants == nil {
		return mediaport.ImageVariant{}, ErrUnavailable
	}
	variant, err := service.variants.GetImageVariant(ctx, imageID, "thumb_320")
	if err != nil || len(variant.Content) == 0 || (variant.MediaType != "image/png" && variant.MediaType != "image/jpeg" && variant.MediaType != "image/gif") || variant.ETag == "" {
		return mediaport.ImageVariant{}, ErrUnavailable
	}
	return variant, nil
}

func (service *Service) Workbench(ctx context.Context, scope Scope) (WorkbenchResult, error) {
	profile, err := service.Profile(ctx, scope)
	if err != nil {
		return WorkbenchResult{}, err
	}
	return service.workbenchWithProfile(ctx, scope, profile.Profile)
}

func (service *Service) workbenchWithProfile(ctx context.Context, scope Scope, profile contactport.SidebarProfile) (WorkbenchResult, error) {
	counts, err := service.workbench.Read(ctx, contactport.CustomerID(scope.CustomerID))
	if err != nil {
		return WorkbenchResult{}, err
	}
	return WorkbenchResult{Profile: profile, QuestionnaireCount: counts.Questionnaires, OrderCount: counts.Orders, PeriodicOrderCount: counts.PeriodicOrders, MaterialCount: counts.Materials, Safety: localSafety()}, nil
}

func validPrincipal(value authport.Principal) bool {
	return value.AdminUserID > 0 && (value.Role == authport.RoleAdmin || value.Role == authport.RoleOps || value.Role == authport.RoleSales) && (value.Role != authport.RoleSales || value.StaffID != nil && *value.StaffID > 0)
}

func principalAllowsOwner(value authport.Principal, owner int64) bool {
	return owner > 0 && (value.Role == authport.RoleAdmin || value.Role == authport.RoleOps || value.Role == authport.RoleSales && value.StaffID != nil && *value.StaffID == owner)
}

func validExternalUserID(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 1024 {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func mapDependencyError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, contactport.ErrSidebarProfileNotFound):
		return ErrNotFound
	case errors.Is(err, ErrConflict), errors.Is(err, contactport.ErrSidebarProfileConflict):
		return ErrConflict
	case errors.Is(err, ErrInvalidInput), errors.Is(err, contactport.ErrSidebarProfileInvalid):
		return ErrInvalidInput
	default:
		return ErrUnavailable
	}
}

func compactStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && (len(result) == 0 || result[len(result)-1] != value) {
			result = append(result, value)
		}
	}
	return result
}

func int64String(value int64) string {
	const digits = "0123456789"
	if value <= 0 {
		return "0"
	}
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = digits[value%10]
		value /= 10
	}
	return string(buffer[index:])
}
