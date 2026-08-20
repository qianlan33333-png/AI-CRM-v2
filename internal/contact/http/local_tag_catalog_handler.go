package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"unicode/utf8"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const maxLocalTagCatalogBodyBytes = 1 << 20

type localTagCatalogApplication interface {
	List(context.Context) (contactapp.LegacyTagCatalog, error)
	CreateGroup(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, contactapp.LegacyTag, error)
	UpdateGroup(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error)
	ArchiveGroup(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTagGroup, error)
	ReorderGroups(context.Context, contactapp.LegacyTagCommand) ([]contactapp.LegacyTagGroup, error)
	CreateTag(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTag, error)
	UpdateTag(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTag, error)
	ArchiveTag(context.Context, contactapp.LegacyTagCommand) (contactapp.LegacyTag, error)
	ReorderTags(context.Context, contactapp.LegacyTagCommand) ([]contactapp.LegacyTag, error)
}

// LocalTagCatalogHandler exposes only Contact-owned, local catalog mutations.
// It deliberately has no provider dependency and independently repeats the
// admin/ops global guard so direct wiring cannot widen the generated route.
type LocalTagCatalogHandler struct {
	generated.Unimplemented
	application localTagCatalogApplication
}

var _ generated.ServerInterface = (*LocalTagCatalogHandler)(nil)

func NewLocalTagCatalogHandler(application localTagCatalogApplication) (*LocalTagCatalogHandler, error) {
	if nilLocalTagCatalogApplication(application) {
		return nil, errors.New("local tag catalog application is required")
	}
	return &LocalTagCatalogHandler{application: application}, nil
}

func (handler *LocalTagCatalogHandler) ListTagGroups(writer http.ResponseWriter, request *http.Request) {
	if _, err := handler.operation(request, authport.CapabilityCustomersRead); err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	catalog, err := handler.application.List(request.Context())
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	response, err := localTagCatalogResponse(catalog)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	writeLocalTagCatalogJSON(writer, http.StatusOK, response)
}

// ListTags preserves the existing generated listTags operation while applying
// the stricter native catalog authorization boundary.
func (handler *LocalTagCatalogHandler) ListTags(writer http.ResponseWriter, request *http.Request) {
	if _, err := handler.operation(request, authport.CapabilityCustomersRead); err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	catalog, err := handler.application.List(request.Context())
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	items := make([]generated.Tag, 0, len(catalog.Tags))
	for _, tag := range catalog.Tags {
		item, convertErr := localTagListItem(tag)
		if convertErr != nil {
			writeLocalTagCatalogError(writer, request, convertErr)
			return
		}
		items = append(items, item)
	}
	writeLocalTagCatalogJSON(writer, http.StatusOK, generated.TagListResponse{Items: items})
}

func (handler *LocalTagCatalogHandler) CreateTagGroup(writer http.ResponseWriter, request *http.Request, _ generated.CreateTagGroupParams) {
	principal, command, err := handler.mutation(request, authport.CapabilityCustomersWrite)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	var body generated.CreateLocalTagGroupRequest
	if err := decodeLocalTagCatalogBody(writer, request, &body); err != nil {
		writeLocalTagCatalogError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	command.Actor = principal.AdminUserID
	command.GroupName = body.Name
	command.FirstTagName = body.FirstTagName
	group, tag, err := handler.application.CreateGroup(request.Context(), command)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	groupResponse, err := localTagGroupResponse(group)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	tagResponse, err := localTagResponse(tag)
	if err != nil || tagResponse.GroupId != groupResponse.Id {
		writeLocalTagCatalogError(writer, request, errors.Join(contactapp.ErrLegacyTagUnavailable, err))
		return
	}
	writeLocalTagCatalogJSON(writer, http.StatusCreated, generated.LocalTagGroupCreateResponse{Group: groupResponse, Tag: tagResponse})
}

func (handler *LocalTagCatalogHandler) ReorderTagGroups(writer http.ResponseWriter, request *http.Request, _ generated.ReorderTagGroupsParams) {
	principal, command, err := handler.mutation(request, authport.CapabilityCustomersWrite)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	var body generated.ReorderLocalCatalogRequest
	if err := decodeLocalTagCatalogBody(writer, request, &body); err != nil {
		writeLocalTagCatalogError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	command.Actor = principal.AdminUserID
	command.IDs = append([]int64(nil), body.Ids...)
	groups, err := handler.application.ReorderGroups(request.Context(), command)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	items := make([]generated.LocalTagGroup, 0, len(groups))
	for _, group := range groups {
		item, convertErr := localTagGroupResponse(group)
		if convertErr != nil {
			writeLocalTagCatalogError(writer, request, convertErr)
			return
		}
		items = append(items, item)
	}
	writeLocalTagCatalogJSON(writer, http.StatusOK, generated.LocalTagGroupListResponse{Items: items})
}

func (handler *LocalTagCatalogHandler) UpdateTagGroup(writer http.ResponseWriter, request *http.Request, groupID generated.TagGroupID, _ generated.UpdateTagGroupParams) {
	principal, command, err := handler.mutation(request, authport.CapabilityCustomersWrite)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	var body generated.UpdateLocalTagNameRequest
	if err := decodeLocalTagCatalogBody(writer, request, &body); err != nil {
		writeLocalTagCatalogError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	command.Actor, command.GroupID, command.GroupName = principal.AdminUserID, int64(groupID), body.Name
	group, err := handler.application.UpdateGroup(request.Context(), command)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	response, err := localTagGroupResponse(group)
	if err != nil || response.Id != int64(groupID) {
		writeLocalTagCatalogError(writer, request, errors.Join(contactapp.ErrLegacyTagUnavailable, err))
		return
	}
	writeLocalTagCatalogJSON(writer, http.StatusOK, response)
}

func (handler *LocalTagCatalogHandler) ArchiveTagGroup(writer http.ResponseWriter, request *http.Request, groupID generated.TagGroupID, _ generated.ArchiveTagGroupParams) {
	principal, command, err := handler.mutation(request, authport.CapabilityCustomersWrite)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	command.Actor, command.GroupID = principal.AdminUserID, int64(groupID)
	group, err := handler.application.ArchiveGroup(request.Context(), command)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	if group.ID != int64(groupID) {
		writeLocalTagCatalogError(writer, request, contactapp.ErrLegacyTagUnavailable)
		return
	}
	writeLocalTagCatalogJSON(writer, http.StatusOK, localTagArchiveResponse(group.ID))
}

func (handler *LocalTagCatalogHandler) CreateTag(writer http.ResponseWriter, request *http.Request, _ generated.CreateTagParams) {
	principal, command, err := handler.mutation(request, authport.CapabilityCustomersWrite)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	var body generated.CreateLocalTagRequest
	if err := decodeLocalTagCatalogBody(writer, request, &body); err != nil {
		writeLocalTagCatalogError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	command.Actor, command.GroupID, command.TagName = principal.AdminUserID, body.GroupId, body.Name
	tag, err := handler.application.CreateTag(request.Context(), command)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	response, err := localTagResponse(tag)
	if err != nil || response.GroupId != body.GroupId {
		writeLocalTagCatalogError(writer, request, errors.Join(contactapp.ErrLegacyTagUnavailable, err))
		return
	}
	writeLocalTagCatalogJSON(writer, http.StatusCreated, response)
}

func (handler *LocalTagCatalogHandler) ReorderTags(writer http.ResponseWriter, request *http.Request, _ generated.ReorderTagsParams) {
	principal, command, err := handler.mutation(request, authport.CapabilityCustomersWrite)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	var body generated.ReorderLocalCatalogRequest
	if err := decodeLocalTagCatalogBody(writer, request, &body); err != nil {
		writeLocalTagCatalogError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	command.Actor = principal.AdminUserID
	command.IDs = append([]int64(nil), body.Ids...)
	tags, err := handler.application.ReorderTags(request.Context(), command)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	items := make([]generated.LocalTag, 0, len(tags))
	for _, tag := range tags {
		item, convertErr := localTagResponse(tag)
		if convertErr != nil {
			writeLocalTagCatalogError(writer, request, convertErr)
			return
		}
		items = append(items, item)
	}
	writeLocalTagCatalogJSON(writer, http.StatusOK, generated.LocalTagListResponse{Items: items})
}

func (handler *LocalTagCatalogHandler) UpdateTag(writer http.ResponseWriter, request *http.Request, tagID generated.TagID, _ generated.UpdateTagParams) {
	principal, command, err := handler.mutation(request, authport.CapabilityCustomersWrite)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	var body generated.UpdateLocalTagNameRequest
	if err := decodeLocalTagCatalogBody(writer, request, &body); err != nil {
		writeLocalTagCatalogError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	command.Actor, command.TagID, command.TagName = principal.AdminUserID, int64(tagID), body.Name
	tag, err := handler.application.UpdateTag(request.Context(), command)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	response, err := localTagResponse(tag)
	if err != nil || response.Id != int64(tagID) {
		writeLocalTagCatalogError(writer, request, errors.Join(contactapp.ErrLegacyTagUnavailable, err))
		return
	}
	writeLocalTagCatalogJSON(writer, http.StatusOK, response)
}

func (handler *LocalTagCatalogHandler) ArchiveTag(writer http.ResponseWriter, request *http.Request, tagID generated.TagID, _ generated.ArchiveTagParams) {
	principal, command, err := handler.mutation(request, authport.CapabilityCustomersWrite)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	command.Actor, command.TagID = principal.AdminUserID, int64(tagID)
	tag, err := handler.application.ArchiveTag(request.Context(), command)
	if err != nil {
		writeLocalTagCatalogError(writer, request, err)
		return
	}
	if tag.ID != int64(tagID) {
		writeLocalTagCatalogError(writer, request, contactapp.ErrLegacyTagUnavailable)
		return
	}
	writeLocalTagCatalogJSON(writer, http.StatusOK, localTagArchiveResponse(tag.ID))
}

func (handler *LocalTagCatalogHandler) mutation(request *http.Request, capability authport.Capability) (authport.Principal, contactapp.LegacyTagCommand, error) {
	principal, err := handler.operation(request, capability)
	if err != nil {
		return authport.Principal{}, contactapp.LegacyTagCommand{}, err
	}
	key, err := localTagCatalogIdempotencyKey(request)
	if err != nil {
		return authport.Principal{}, contactapp.LegacyTagCommand{}, platformhttp.NewError(platformhttp.CodeMalformedRequest, err)
	}
	return principal, contactapp.LegacyTagCommand{IdempotencyKey: key}, nil
}

func (handler *LocalTagCatalogHandler) operation(request *http.Request, capability authport.Capability) (authport.Principal, error) {
	if request == nil {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil)
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	if principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	if handler == nil || nilLocalTagCatalogApplication(handler.application) {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil)
	}
	return principal, nil
}

func localTagCatalogIdempotencyKey(request *http.Request) (string, error) {
	if request == nil {
		return "", errors.New("missing idempotency key")
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || len(values[0]) < 16 || len(values[0]) > 128 || values[0] != string(bytes.TrimSpace([]byte(values[0]))) {
		return "", errors.New("invalid idempotency key")
	}
	return values[0], nil
}

func decodeLocalTagCatalogBody(writer http.ResponseWriter, request *http.Request, destination any) error {
	if request == nil || request.Body == nil {
		return io.EOF
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxLocalTagCatalogBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are forbidden")
		}
		return err
	}
	return nil
}

func localTagCatalogResponse(catalog contactapp.LegacyTagCatalog) (generated.LocalTagCatalog, error) {
	if catalog.Groups == nil || catalog.Tags == nil {
		return generated.LocalTagCatalog{}, contactapp.ErrLegacyTagUnavailable
	}
	groups := make([]generated.LocalTagGroup, 0, len(catalog.Groups))
	groupNames := make(map[int64]string, len(catalog.Groups))
	for _, group := range catalog.Groups {
		item, err := localTagGroupResponse(group)
		if err != nil {
			return generated.LocalTagCatalog{}, err
		}
		if _, duplicate := groupNames[item.Id]; duplicate || (len(groups) > 0 && groups[len(groups)-1].SortOrder > item.SortOrder) {
			return generated.LocalTagCatalog{}, contactapp.ErrLegacyTagUnavailable
		}
		groupNames[item.Id] = item.Name
		groups = append(groups, item)
	}
	tags := make([]generated.LocalTag, 0, len(catalog.Tags))
	tagIDs := make(map[int64]struct{}, len(catalog.Tags))
	for _, tag := range catalog.Tags {
		item, err := localTagResponse(tag)
		if err != nil {
			return generated.LocalTagCatalog{}, err
		}
		_, duplicate := tagIDs[item.Id]
		groupName, exists := groupNames[item.GroupId]
		if duplicate || !exists || groupName != item.GroupName {
			return generated.LocalTagCatalog{}, contactapp.ErrLegacyTagUnavailable
		}
		tagIDs[item.Id] = struct{}{}
		tags = append(tags, item)
	}
	return generated.LocalTagCatalog{Groups: groups, Tags: tags}, nil
}

func localTagGroupResponse(group contactapp.LegacyTagGroup) (generated.LocalTagGroup, error) {
	if group.ID < 1 || group.SortOrder < 0 || !validLocalTagCatalogText(group.Name) {
		return generated.LocalTagGroup{}, contactapp.ErrLegacyTagUnavailable
	}
	return generated.LocalTagGroup{Id: group.ID, Name: group.Name, SortOrder: group.SortOrder}, nil
}

func localTagResponse(tag contactapp.LegacyTag) (generated.LocalTag, error) {
	if tag.ID < 1 || tag.GroupID < 1 || tag.SortOrder < 0 || !validLocalTagCatalogText(tag.GroupName) || !validLocalTagCatalogText(tag.Name) {
		return generated.LocalTag{}, contactapp.ErrLegacyTagUnavailable
	}
	return generated.LocalTag{Id: tag.ID, GroupId: tag.GroupID, GroupName: tag.GroupName, Name: tag.Name, SortOrder: tag.SortOrder}, nil
}

func localTagListItem(tag contactapp.LegacyTag) (generated.Tag, error) {
	response, err := localTagResponse(tag)
	if err != nil {
		return generated.Tag{}, err
	}
	groupID, groupName := response.GroupId, response.GroupName
	return generated.Tag{Id: response.Id, GroupId: &groupID, GroupName: &groupName, Name: response.Name, SortOrder: response.SortOrder}, nil
}

func validLocalTagCatalogText(value string) bool {
	return value != "" && value == string(bytes.TrimSpace([]byte(value))) && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 200
}

func localTagArchiveResponse(id int64) generated.LocalCatalogArchiveResponse {
	return generated.LocalCatalogArchiveResponse{Id: id, Archived: generated.LocalCatalogArchiveResponseArchived(true)}
}

func writeLocalTagCatalogJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeLocalTagCatalogError(writer http.ResponseWriter, request *http.Request, err error) {
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, contactapp.ErrInvalidLegacyTag):
		code = platformhttp.CodeValidationFailed
	case errors.Is(err, contactapp.ErrLegacyTagNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, contactapp.ErrLegacyTagReferenced), errors.Is(err, contactapp.ErrLegacyTagConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func nilLocalTagCatalogApplication(application localTagCatalogApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
