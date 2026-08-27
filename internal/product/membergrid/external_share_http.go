package membergrid

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const PublicSharePagePath = "/member-grid-share/index.html"

type ExternalShareManagementApplication interface {
	SetExternalShare(context.Context, SetExternalShareCommand) (SetExternalShareResult, error)
}

type ExternalShareManagementHandler struct {
	application ExternalShareManagementApplication
	authorizer  ManagementAuthorizer
	csrf        ManagementCSRFVerifier
}

type SetExternalShareRequest struct {
	Enabled         *bool  `json:"enabled"`
	ExpectedVersion *int64 `json:"expected_version"`
}

type SetExternalShareResponse struct {
	OK                       bool   `json:"ok"`
	ExternalShareEnabled     bool   `json:"external_share_enabled"`
	ExternalShareVersion     int64  `json:"external_share_version"`
	TokenIssued              bool   `json:"token_issued"`
	PublicPath               string `json:"public_path,omitempty"`
	RealExternalCallExecuted bool   `json:"real_external_call_executed"`
}

func NewExternalShareManagementHandler(application ExternalShareManagementApplication, authorizer ManagementAuthorizer, csrf ManagementCSRFVerifier) (*ExternalShareManagementHandler, error) {
	if nilDependency(application) || nilDependency(authorizer) || nilDependency(csrf) {
		return nil, errors.New("member grid external share HTTP dependencies are required")
	}
	return &ExternalShareManagementHandler{application: application, authorizer: authorizer, csrf: csrf}, nil
}

func (handler *ExternalShareManagementHandler) SetExternalShare(writer http.ResponseWriter, request *http.Request, rawProductID string) {
	setSecurityHeaders(writer)
	if handler == nil || nilDependency(handler.application) || nilDependency(handler.authorizer) || nilDependency(handler.csrf) || request == nil {
		writeManagementFailure(writer, request, ErrUnavailable)
		return
	}
	if !requireManagementMethod(writer, request, http.MethodPut) {
		return
	}
	actor, err := handler.authorizer.Authorize(request.Context(), CapabilityProductsWrite)
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	if actor.ID < 1 {
		writeManagementFailure(writer, request, ErrAuthenticationRequired)
		return
	}
	if err = handler.csrf.Verify(request); err != nil {
		writeManagementFailure(writer, request, ErrCSRFRejected)
		return
	}
	key, problem := managementIdempotencyKey(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	productID, err := parseProductID(rawProductID)
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	var body SetExternalShareRequest
	_, problem = decodeManagementObject(writer, request, map[string]struct{}{"enabled": {}, "expected_version": {}}, &body)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if body.Enabled == nil || body.ExpectedVersion == nil {
		writeProblem(writer, request, validation("body", "required_fields", ErrInvalidManagementInput))
		return
	}
	result, err := handler.application.SetExternalShare(request.Context(), SetExternalShareCommand{
		ServiceProductID: productID,
		Enabled:          *body.Enabled,
		ExpectedVersion:  *body.ExpectedVersion,
		ActorID:          actor.ID,
		IdempotencyKey:   key,
	})
	if err != nil {
		writeManagementFailure(writer, request, err)
		return
	}
	response := SetExternalShareResponse{
		OK:                       true,
		ExternalShareEnabled:     result.Share.Enabled,
		ExternalShareVersion:     result.Share.Version,
		TokenIssued:              result.TokenIssued,
		RealExternalCallExecuted: false,
	}
	if result.TokenIssued {
		if result.PublicToken == "" || strings.ContainsAny(result.PublicToken, "#/?&=%") {
			writeManagementFailure(writer, request, ErrUnavailable)
			return
		}
		response.PublicPath = PublicSharePagePath + "#" + result.PublicToken
	}
	writeJSON(writer, http.StatusOK, response)
}

type externalShareManagementRouteFragment struct {
	handler *ExternalShareManagementHandler
}

func NewExternalShareManagementRouteFragment(handler *ExternalShareManagementHandler) (http.Handler, error) {
	if handler == nil || nilDependency(handler.application) || nilDependency(handler.authorizer) || nilDependency(handler.csrf) {
		return nil, errors.New("member grid external share handler is required")
	}
	return &externalShareManagementRouteFragment{handler: handler}, nil
}

func (fragment *externalShareManagementRouteFragment) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setSecurityHeaders(writer)
	if fragment == nil || fragment.handler == nil || request == nil || request.URL == nil {
		writeManagementFailure(writer, request, ErrUnavailable)
		return
	}
	path := request.URL.Path
	if request.URL.RawQuery != "" || request.URL.ForceQuery || request.URL.RawPath != "" || request.URL.EscapedPath() != path || !strings.HasPrefix(path, RoutePrefix+"/") || strings.HasSuffix(path, "/") || strings.Contains(path, "\\") || strings.Contains(path, "//") {
		writeProblem(writer, request, malformed("path", "invalid", ErrInvalidManagementInput))
		return
	}
	segments := strings.Split(strings.TrimPrefix(path, RoutePrefix+"/"), "/")
	if len(segments) != 3 || segments[1] != "member-grid" || segments[2] != "share-settings" {
		writeManagementFailure(writer, request, ErrNotFound)
		return
	}
	fragment.handler.SetExternalShare(writer, request, segments[0])
}
