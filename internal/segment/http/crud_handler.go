package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const segmentBodyMaximum = 64 << 10

type crudApplication interface {
	List(context.Context, string, int32) (segmentport.Page, error)
	Get(context.Context, segmentport.SegmentID) (segmentport.Segment, error)
	Create(context.Context, segmentport.CreateCommand) (segmentport.Segment, error)
	UpdateHTTP(context.Context, segmentapp.UpdateInput) (segmentport.Segment, error)
	Archive(context.Context, segmentport.ArchiveCommand) (segmentport.Segment, error)
	ListMemberRecords(context.Context, segmentport.SegmentID, string, int32) (segmentapp.MemberPage, error)
}

type CRUDHandler struct{ application crudApplication }

func NewCRUDHandler(application crudApplication) (*CRUDHandler, error) {
	if nilCRUDApplication(application) {
		return nil, errors.New("segment CRUD application is required")
	}
	return &CRUDHandler{application: application}, nil
}

func (handler *CRUDHandler) ListSegments(writer http.ResponseWriter, request *http.Request, params generated.ListSegmentsParams) {
	if _, err := handler.operation(request, authport.CapabilitySegmentsRead); err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	cursor, limit, supplied, err := crudPageParams(params.Cursor, params.Limit)
	if err != nil {
		writeCRUDError(writer, request, err, supplied)
		return
	}
	page, err := handler.application.List(request.Context(), cursor, limit)
	if err != nil {
		writeCRUDError(writer, request, err, supplied)
		return
	}
	items := make([]generated.Segment, len(page.Items))
	for index, item := range page.Items {
		items[index], err = generatedSegment(item)
		if err != nil {
			writeCRUDError(writer, request, err, supplied)
			return
		}
	}
	writeCRUDJSON(writer, http.StatusOK, generated.SegmentPage{Items: items, NextCursor: optionalCursor(page.NextCursor)})
}

func (handler *CRUDHandler) CreateSegment(writer http.ResponseWriter, request *http.Request, params generated.CreateSegmentParams) {
	principal, err := handler.operation(request, authport.CapabilitySegmentsWrite)
	if err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	body, err := decodeCreateSegment(request)
	if err != nil {
		writeCRUDError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err), false)
		return
	}
	segment, err := handler.application.Create(request.Context(), segmentport.CreateCommand{
		Name: body.Name, Definition: body.Definition, RefreshMode: body.RefreshMode, RefreshCron: body.RefreshCron,
		Actor: segmentActor(principal), IdempotencyKey: string(params.IdempotencyKey),
	})
	if err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	response, err := generatedSegment(segment)
	if err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	writeCRUDJSON(writer, http.StatusCreated, response)
}

func (handler *CRUDHandler) GetSegment(writer http.ResponseWriter, request *http.Request, segmentID generated.SegmentID) {
	if _, err := handler.operation(request, authport.CapabilitySegmentsRead); err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	segment, err := handler.application.Get(request.Context(), segmentport.SegmentID(segmentID))
	if err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	response, err := generatedSegment(segment)
	if err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	writeCRUDJSON(writer, http.StatusOK, response)
}

func (handler *CRUDHandler) UpdateSegment(writer http.ResponseWriter, request *http.Request, segmentID generated.SegmentID, params generated.UpdateSegmentParams) {
	principal, err := handler.operation(request, authport.CapabilitySegmentsWrite)
	if err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	if segmentID <= 0 {
		writeCRUDError(writer, request, segmentapp.ErrSegmentNotFound, false)
		return
	}
	body, err := decodeUpdateSegment(request)
	if err != nil {
		writeCRUDError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err), false)
		return
	}
	body.SegmentID = segmentport.SegmentID(segmentID)
	body.Actor = segmentActor(principal)
	body.IdempotencyKey = string(params.IdempotencyKey)
	segment, err := handler.application.UpdateHTTP(request.Context(), body)
	if err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	response, err := generatedSegment(segment)
	if err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	writeCRUDJSON(writer, http.StatusOK, response)
}

func (handler *CRUDHandler) ArchiveSegment(writer http.ResponseWriter, request *http.Request, segmentID generated.SegmentID, params generated.ArchiveSegmentParams) {
	principal, err := handler.operation(request, authport.CapabilitySegmentsWrite)
	if err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	segment, err := handler.application.Archive(request.Context(), segmentport.ArchiveCommand{
		SegmentID: segmentport.SegmentID(segmentID), Actor: segmentActor(principal), IdempotencyKey: string(params.IdempotencyKey),
	})
	if err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	response, err := generatedSegment(segment)
	if err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	writeCRUDJSON(writer, http.StatusOK, response)
}

func (handler *CRUDHandler) ListSegmentMembers(writer http.ResponseWriter, request *http.Request, segmentID generated.SegmentID, params generated.ListSegmentMembersParams) {
	if _, err := handler.operation(request, authport.CapabilitySegmentsRead); err != nil {
		writeCRUDError(writer, request, err, false)
		return
	}
	if segmentID <= 0 {
		writeCRUDError(writer, request, segmentapp.ErrSegmentNotFound, false)
		return
	}
	cursor, limit, supplied, err := crudPageParams(params.Cursor, params.Limit)
	if err != nil {
		writeCRUDError(writer, request, err, supplied)
		return
	}
	page, err := handler.application.ListMemberRecords(request.Context(), segmentport.SegmentID(segmentID), cursor, limit)
	if err != nil {
		writeCRUDError(writer, request, err, supplied)
		return
	}
	items := make([]generated.Customer, len(page.Items))
	for index, item := range page.Items {
		items[index], err = generatedMember(item)
		if err != nil {
			writeCRUDError(writer, request, err, supplied)
			return
		}
	}
	writeCRUDJSON(writer, http.StatusOK, generated.SegmentMemberPage{Items: items, NextCursor: optionalCursor(page.NextCursor)})
}

type createBody struct {
	Name        string
	Definition  segmentport.Definition
	RefreshMode segmentport.RefreshMode
	RefreshCron *string
}

func decodeCreateSegment(request *http.Request) (createBody, error) {
	raw, err := readBody(request)
	if err != nil {
		return createBody{}, err
	}
	fields, err := decodeObject(raw)
	if err != nil || len(fields) < 3 || len(fields) > 4 {
		return createBody{}, errors.New("invalid segment create body")
	}
	for field := range fields {
		if field != "name" && field != "definition" && field != "refresh_mode" && field != "refresh_cron" {
			return createBody{}, errors.New("unknown segment create field")
		}
	}
	name, nameOK := decodeJSONString(fields["name"])
	mode, modeOK := decodeJSONString(fields["refresh_mode"])
	definition, definitionOK := fields["definition"]
	if !nameOK || !modeOK || !definitionOK || bytes.Equal(bytes.TrimSpace(definition), []byte("null")) || !json.Valid(definition) {
		return createBody{}, errors.New("invalid segment create fields")
	}
	var cron *string
	if value, exists := fields["refresh_cron"]; exists {
		if !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			decoded, ok := decodeJSONString(value)
			if !ok {
				return createBody{}, errors.New("invalid create refresh cron")
			}
			cron = &decoded
		}
	}
	return createBody{Name: name, Definition: segmentport.Definition(definition), RefreshMode: segmentport.RefreshMode(mode), RefreshCron: cron}, nil
}

func decodeUpdateSegment(request *http.Request) (segmentapp.UpdateInput, error) {
	raw, err := readBody(request)
	if err != nil {
		return segmentapp.UpdateInput{}, err
	}
	fields, err := decodeObject(raw)
	if err != nil || len(fields) == 0 {
		return segmentapp.UpdateInput{}, errors.New("invalid segment mutation body")
	}
	for field := range fields {
		if field != "name" && field != "definition" && field != "refresh_mode" && field != "refresh_cron" {
			return segmentapp.UpdateInput{}, errors.New("unknown segment mutation field")
		}
	}
	var result segmentapp.UpdateInput
	if value, ok := fields["name"]; ok {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &result.Name) != nil || result.Name == nil {
			return segmentapp.UpdateInput{}, errors.New("invalid name")
		}
	}
	if value, ok := fields["definition"]; ok {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || !json.Valid(value) {
			return segmentapp.UpdateInput{}, errors.New("invalid definition")
		}
		definition := segmentport.Definition(append([]byte(nil), value...))
		result.Definition = &definition
	}
	if value, ok := fields["refresh_mode"]; ok {
		var mode string
		if json.Unmarshal(value, &mode) != nil || mode == "" {
			return segmentapp.UpdateInput{}, errors.New("invalid refresh mode")
		}
		converted := segmentport.RefreshMode(mode)
		result.RefreshMode = &converted
	}
	if value, ok := fields["refresh_cron"]; ok {
		result.RefreshCronSet = true
		if !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			var cron string
			if json.Unmarshal(value, &cron) != nil {
				return segmentapp.UpdateInput{}, errors.New("invalid refresh cron")
			}
			result.RefreshCron = &cron
		}
	}
	return result, nil
}

func readBody(request *http.Request) ([]byte, error) {
	if request == nil || request.Body == nil {
		return nil, errors.New("request body is required")
	}
	reader := io.LimitReader(request.Body, segmentBodyMaximum+1)
	raw, err := io.ReadAll(reader)
	if err != nil || len(raw) == 0 || len(raw) > segmentBodyMaximum {
		return nil, errors.New("invalid request body")
	}
	return raw, nil
}

func decodeObject(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("object body is required")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, tokenErr := decoder.Token()
		key, ok := keyToken.(string)
		if tokenErr != nil || !ok {
			return nil, errors.New("invalid object key")
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, errors.New("duplicate object key")
		}
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return nil, errors.New("invalid object value")
		}
		fields[key] = append(json.RawMessage(nil), value...)
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errors.New("invalid object end")
	}
	if !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return nil, errors.New("trailing object data")
	}
	return fields, nil
}

func decodeJSONString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func (handler *CRUDHandler) operation(request *http.Request, capability authport.Capability) (authport.Principal, error) {
	if handler == nil || nilCRUDApplication(handler.application) || request == nil {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil)
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	return principal, nil
}

func crudPageParams(cursor *generated.Cursor, limit *generated.Limit) (string, int32, bool, error) {
	var raw string
	if cursor != nil {
		raw = string(*cursor)
	}
	var normalized int32
	if limit != nil {
		if *limit < 1 || *limit > int(segmentapp.SegmentMaximumLimit) {
			return "", 0, cursor != nil, segmentapp.ErrInvalidSegmentCursor
		}
		normalized = int32(*limit)
	}
	return raw, normalized, cursor != nil, nil
}

func generatedSegment(item segmentport.Segment) (generated.Segment, error) {
	if item.ID <= 0 || item.Name == "" || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() || item.CreatedAt.After(item.UpdatedAt) ||
		(item.LifecycleStatus != segmentport.LifecycleStatusActive && item.LifecycleStatus != segmentport.LifecycleStatusArchived) {
		return generated.Segment{}, errors.New("invalid segment projection")
	}
	var definition generated.SegmentDefinition
	if err := json.Unmarshal(item.Definition, &definition); err != nil {
		return generated.Segment{}, err
	}
	return generated.Segment{
		Id: int64(item.ID), Name: item.Name, Definition: definition, RefreshMode: generated.SegmentRefreshMode(item.RefreshMode),
		RefreshCron: cloneCRUDString(item.RefreshCron), MemberCount: item.MemberCount, RefreshedAt: cloneCRUDTime(item.RefreshedAt),
		RefreshStatus: generated.SegmentRefreshStatus(item.RefreshStatus), LifecycleStatus: generated.SegmentLifecycleStatus(item.LifecycleStatus),
		CreatedAt: item.CreatedAt.UTC(), UpdatedAt: item.UpdatedAt.UTC(),
	}, nil
}

func generatedMember(item segmentapp.MemberRecord) (generated.Customer, error) {
	if item.ID <= 0 || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() || item.CreatedAt.After(item.UpdatedAt) || invalidMemberURI(item.AvatarURL) {
		return generated.Customer{}, errors.New("invalid member projection")
	}
	var extra map[string]any
	decoder := json.NewDecoder(bytes.NewReader(item.Extra))
	decoder.UseNumber()
	if err := decoder.Decode(&extra); err != nil || extra == nil || !channelNeutral(extra) || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return generated.Customer{}, errors.New("invalid member extra")
	}
	var gender *int32
	if item.Gender != nil {
		value := int32(*item.Gender)
		gender = &value
	}
	return generated.Customer{
		Id: int64(item.ID), Name: item.Name, AvatarUrl: cloneCRUDString(item.AvatarURL), Gender: gender,
		StageId: cloneCRUDInt64(item.StageID), OwnerStaffId: cloneCRUDInt64(item.OwnerStaffID), ChannelId: cloneCRUDInt64(item.ChannelID),
		AddedAt: cloneCRUDTime(item.AddedAt), LastInteractAt: cloneCRUDTime(item.LastInteractAt), IsDeleted: item.IsDeleted,
		Extra: extra, CreatedAt: item.CreatedAt.UTC(), UpdatedAt: item.UpdatedAt.UTC(),
	}, nil
}

func channelNeutral(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			if normalized == "externaluserid" || normalized == "unionid" || normalized == "openid" || normalized == "mobile" || normalized == "phone" || !channelNeutral(child) {
				return false
			}
		}
	case []any:
		for _, child := range typed {
			if !channelNeutral(child) {
				return false
			}
		}
	}
	return true
}

func invalidMemberURI(value *string) bool {
	if value == nil {
		return false
	}
	parsed, err := url.ParseRequestURI(*value)
	return err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https")
}

func writeCRUDError(writer http.ResponseWriter, request *http.Request, err error, cursorSupplied bool) {
	if request == nil {
		request = &http.Request{}
	}
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, segmentapp.ErrInvalidSegmentCursor):
		code = platformhttp.CodeMalformedRequest
		if cursorSupplied {
			code = platformhttp.CodeCursorInvalid
		}
	case errors.Is(err, segmentapp.ErrInvalidSegmentCommand):
		code = platformhttp.CodeValidationFailed
	case errors.Is(err, segmentapp.ErrSegmentNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, segmentapp.ErrSegmentCommandConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func writeCRUDJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func segmentActor(principal authport.Principal) segmentport.Actor {
	return segmentport.Actor("admin:" + strconv.FormatInt(principal.AdminUserID, 10))
}
func optionalCursor(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func cloneCRUDString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func cloneCRUDInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func cloneCRUDTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}
func nilCRUDApplication(application crudApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
