package http

import (
	"reflect"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

const BasePath = "/api/admin/radar-links"

type Route struct {
	Method       string
	Pattern      string
	Permission   Permission
	RequiresCSRF bool
}

type RouteFragment struct {
	application radarport.Application
	authorizer  Authorizer
	csrf        CSRFVerifier
}

func NewRouteFragment(application radarport.Application, authorizer Authorizer, csrf CSRFVerifier) (*RouteFragment, error) {
	if nilInterface(application) || nilInterface(authorizer) || nilInterface(csrf) {
		return nil, radarport.ErrUnavailable
	}
	return &RouteFragment{application: application, authorizer: authorizer, csrf: csrf}, nil
}

func (fragment *RouteFragment) Routes() []Route {
	return []Route{
		{Method: "GET", Pattern: BasePath, Permission: PermissionAdminRead},
		{Method: "POST", Pattern: BasePath, Permission: PermissionAdminWrite, RequiresCSRF: true},
		{Method: "GET", Pattern: BasePath + "/{link_id}", Permission: PermissionAdminRead},
		{Method: "PATCH", Pattern: BasePath + "/{link_id}", Permission: PermissionAdminWrite, RequiresCSRF: true},
		{Method: "POST", Pattern: BasePath + "/{link_id}/enable", Permission: PermissionAdminWrite, RequiresCSRF: true},
		{Method: "POST", Pattern: BasePath + "/{link_id}/disable", Permission: PermissionAdminWrite, RequiresCSRF: true},
		{Method: "GET", Pattern: BasePath + "/{link_id}/share", Permission: PermissionAdminRead},
		{Method: "GET", Pattern: BasePath + "/new/options", Permission: PermissionAdminRead},
	}
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
