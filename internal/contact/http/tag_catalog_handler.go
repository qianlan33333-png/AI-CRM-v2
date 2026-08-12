package http

import (
	"context"
	"errors"
	"net/http"
	"reflect"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type tagCatalogApplication interface {
	List(context.Context) ([]contactapp.TagCatalogRecord, error)
}

type TagCatalogHandler struct {
	application tagCatalogApplication
}

func NewTagCatalogHandler(application tagCatalogApplication) (*TagCatalogHandler, error) {
	if nilTagCatalogApplication(application) {
		return nil, errors.New("tag catalog application is required")
	}
	return &TagCatalogHandler{application: application}, nil
}

func (handler *TagCatalogHandler) ListTags(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilTagCatalogApplication(handler.application) || request == nil {
		if request == nil {
			request = &http.Request{}
		}
		writeTagCatalogError(writer, request, contactapp.ErrTagCatalogUnavailable)
		return
	}
	if err := authorizeTagCatalog(request.Context()); err != nil {
		platformhttp.WriteError(writer, request, err)
		return
	}
	records, err := handler.application.List(request.Context())
	if err != nil {
		writeTagCatalogError(writer, request, err)
		return
	}
	items := make([]generated.Tag, 0, len(records))
	for _, record := range records {
		if record.ID <= 0 || record.Name == "" || (record.GroupID == nil) != (record.GroupName == nil) {
			writeTagCatalogError(writer, request, contactapp.ErrTagCatalogUnavailable)
			return
		}
		items = append(items, generated.Tag{
			Id: record.ID, GroupId: cloneInt64(record.GroupID), GroupName: cloneString(record.GroupName),
			Name: record.Name, SortOrder: record.SortOrder,
		})
	}
	writeCustomerListJSON(writer, http.StatusOK, generated.TagListResponse{Items: items})
}

func authorizeTagCatalog(ctx context.Context) error {
	authorization, ok := authport.AuthorizationFromContext(ctx)
	if !ok || authorization.Capability != authport.CapabilityCustomersRead {
		return platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(ctx)
	if !ok || principal.AdminUserID <= 0 {
		return platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	if authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0 &&
		(principal.Role == authport.RoleAdmin || principal.Role == authport.RoleOps) {
		return nil
	}
	if authorization.Scope == authport.ScopeOwnerStaff && principal.Role == authport.RoleSales &&
		principal.StaffID != nil && authorization.OwnerStaffID > 0 && *principal.StaffID == authorization.OwnerStaffID {
		return nil
	}
	return platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
}

func writeTagCatalogError(writer http.ResponseWriter, request *http.Request, err error) {
	platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, err))
}

func nilTagCatalogApplication(application tagCatalogApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
