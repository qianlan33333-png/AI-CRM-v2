package http

import (
	"context"
	"errors"
	"net/http"
	"reflect"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type customerDetailApplication interface {
	Get(context.Context, contactapp.CustomerDetailInput) (contactapp.CustomerDetailStoreResult, error)
}

type CustomerDetailHandler struct {
	application customerDetailApplication
}

func NewCustomerDetailHandler(application customerDetailApplication) (*CustomerDetailHandler, error) {
	if nilCustomerDetailApplication(application) {
		return nil, errors.New("customer detail application is required")
	}
	return &CustomerDetailHandler{application: application}, nil
}

func (handler *CustomerDetailHandler) GetCustomer(
	writer http.ResponseWriter,
	request *http.Request,
	customerID generated.CustomerID,
) {
	if handler == nil || nilCustomerDetailApplication(handler.application) || request == nil {
		if request == nil {
			request = &http.Request{}
		}
		writeCustomerDetailError(writer, request, contactapp.ErrCustomerDetailUnavailable)
		return
	}

	ownerStaffID, err := customerDetailOwner(request.Context())
	if err != nil {
		platformhttp.WriteError(writer, request, err)
		return
	}
	result, err := handler.application.Get(request.Context(), contactapp.CustomerDetailInput{
		ID: contactport.CustomerID(customerID), OwnerStaffID: ownerStaffID,
	})
	if err != nil {
		writeCustomerDetailError(writer, request, err)
		return
	}
	response, err := customerDetailResponse(result)
	if err != nil {
		writeCustomerDetailError(writer, request, contactapp.ErrCustomerDetailUnavailable)
		return
	}
	writeCustomerListJSON(writer, http.StatusOK, response)
}

func customerDetailOwner(ctx context.Context) (*int64, error) {
	authorization, ok := authport.AuthorizationFromContext(ctx)
	if !ok || authorization.Capability != authport.CapabilityCustomersRead {
		return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(ctx)
	if !ok || principal.AdminUserID < 1 {
		return nil, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}

	switch authorization.Scope {
	case authport.ScopeGlobal:
		if authorization.OwnerStaffID != 0 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		return nil, nil
	case authport.ScopeOwnerStaff:
		if principal.Role != authport.RoleSales || principal.StaffID == nil ||
			*principal.StaffID != authorization.OwnerStaffID || authorization.OwnerStaffID <= 0 {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		ownerStaffID := authorization.OwnerStaffID
		return &ownerStaffID, nil
	default:
		return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
}

func customerDetailResponse(result contactapp.CustomerDetailStoreResult) (generated.CustomerDetailResponse, error) {
	customer, err := customerResponse(result.Customer)
	if err != nil {
		return generated.CustomerDetailResponse{}, err
	}
	tags := make([]generated.Tag, 0, len(result.Tags))
	for _, item := range result.Tags {
		if item.ID <= 0 || item.Name == "" || (item.GroupID == nil) != (item.GroupName == nil) {
			return generated.CustomerDetailResponse{}, errors.New("customer detail application returned an invalid tag")
		}
		tags = append(tags, generated.Tag{
			Id: int64(item.ID), GroupId: cloneInt64(item.GroupID), GroupName: cloneString(item.GroupName),
			Name: item.Name, SortOrder: item.SortOrder,
		})
	}
	return generated.CustomerDetailResponse{Customer: customer, Tags: tags}, nil
}

func writeCustomerDetailError(writer http.ResponseWriter, request *http.Request, err error) {
	if request == nil {
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, contactapp.ErrInvalidCustomerDetailQuery):
		code = platformhttp.CodeNotFound
	case errors.Is(err, contactapp.ErrCustomerNotFound):
		code = platformhttp.CodeNotFound
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func nilCustomerDetailApplication(application customerDetailApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
