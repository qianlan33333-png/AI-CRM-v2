// Package http exposes contact-owned HTTP handlers without importing another
// domain's implementation packages.
package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"
	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const maxStageBodyBytes = 1 << 20

type Handler struct {
	generated.Unimplemented
	stages contactport.StageService
}

var _ generated.ServerInterface = (*Handler)(nil)

func NewHandler(stages contactport.StageService) (*Handler, error) {
	if nilService(stages) {
		return nil, errors.New("stage service is required")
	}
	return &Handler{stages: stages}, nil
}

func (handler *Handler) ListStages(writer http.ResponseWriter, request *http.Request) {
	if _, err := handler.operation(request, authport.CapabilityStagesRead); err != nil {
		writeStageError(writer, request, err)
		return
	}
	stages, err := handler.stages.ListStages(request.Context())
	if err != nil {
		writeStageError(writer, request, err)
		return
	}
	items := make([]generated.Stage, 0, len(stages))
	for _, stage := range stages {
		item, convertErr := responseStage(stage)
		if convertErr != nil {
			writeStageError(writer, request, convertErr)
			return
		}
		items = append(items, item)
	}
	writeStageJSON(writer, http.StatusOK, generated.StageListResponse{Items: items})
}

func (handler *Handler) CreateStage(writer http.ResponseWriter, request *http.Request, _ generated.CreateStageParams) {
	principal, err := handler.operation(request, authport.CapabilityStagesWrite)
	if err != nil {
		writeStageError(writer, request, err)
		return
	}
	var body generated.CreateStageRequest
	if err = decodeStageBody(writer, request, &body); err != nil {
		writeStageError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	key, err := stageIdempotencyKey(request)
	if err != nil {
		writeStageError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	var config json.RawMessage
	if body.Config != nil {
		config, err = json.Marshal(body.Config)
		if err != nil {
			writeStageError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
			return
		}
	}
	sortOrder := int32(0)
	if body.SortOrder != nil {
		sortOrder = *body.SortOrder
	}
	stage, err := handler.stages.CreateStage(request.Context(), contactport.CreateStageCommand{
		Name: body.Name, SortOrder: sortOrder, Config: config, Actor: actor(principal), IdempotencyKey: key,
	})
	if err != nil {
		writeStageError(writer, request, err)
		return
	}
	response, err := responseStage(stage)
	if err != nil {
		writeStageError(writer, request, err)
		return
	}
	writeStageJSON(writer, http.StatusCreated, response)
}

func (handler *Handler) RenameStage(writer http.ResponseWriter, request *http.Request, stageID generated.StageID, _ generated.RenameStageParams) {
	principal, err := handler.operation(request, authport.CapabilityStagesWrite)
	if err != nil {
		writeStageError(writer, request, err)
		return
	}
	var body generated.RenameStageRequest
	if err = decodeStageBody(writer, request, &body); err != nil {
		writeStageError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	key, err := stageIdempotencyKey(request)
	if err != nil {
		writeStageError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	stage, err := handler.stages.RenameStage(request.Context(), contactport.RenameStageCommand{
		ID: contactport.StageID(stageID), Name: body.Name, Actor: actor(principal), IdempotencyKey: key,
	})
	if err != nil {
		writeStageError(writer, request, err)
		return
	}
	response, err := responseStage(stage)
	if err != nil {
		writeStageError(writer, request, err)
		return
	}
	writeStageJSON(writer, http.StatusOK, response)
}

func (handler *Handler) ReorderStages(writer http.ResponseWriter, request *http.Request, _ generated.ReorderStagesParams) {
	principal, err := handler.operation(request, authport.CapabilityStagesWrite)
	if err != nil {
		writeStageError(writer, request, err)
		return
	}
	var body generated.ReorderStagesRequest
	if err = decodeStageBody(writer, request, &body); err != nil {
		writeStageError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	key, err := stageIdempotencyKey(request)
	if err != nil {
		writeStageError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	ids := make([]contactport.StageID, len(body.Ids))
	for index, id := range body.Ids {
		ids[index] = contactport.StageID(id)
	}
	stages, err := handler.stages.ReorderStages(request.Context(), contactport.ReorderStagesCommand{
		IDs: ids, Actor: actor(principal), IdempotencyKey: key,
	})
	if err != nil {
		writeStageError(writer, request, err)
		return
	}
	items := make([]generated.Stage, len(stages))
	for index, stage := range stages {
		items[index], err = responseStage(stage)
		if err != nil {
			writeStageError(writer, request, err)
			return
		}
	}
	writeStageJSON(writer, http.StatusOK, generated.StageListResponse{Items: items})
}

func (handler *Handler) ArchiveStage(writer http.ResponseWriter, request *http.Request, stageID generated.StageID, _ generated.ArchiveStageParams) {
	principal, err := handler.operation(request, authport.CapabilityStagesWrite)
	if err != nil {
		writeStageError(writer, request, err)
		return
	}
	key, err := stageIdempotencyKey(request)
	if err != nil {
		writeStageError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	stage, err := handler.stages.ArchiveStage(request.Context(), contactport.ArchiveStageCommand{
		ID: contactport.StageID(stageID), Actor: actor(principal), IdempotencyKey: key,
	})
	if err != nil {
		writeStageError(writer, request, err)
		return
	}
	response, err := responseStage(stage)
	if err != nil {
		writeStageError(writer, request, err)
		return
	}
	writeStageJSON(writer, http.StatusOK, response)
}

func stageIdempotencyKey(request *http.Request) (string, error) {
	if request == nil {
		return "", errors.New("missing idempotency key")
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || len(values[0]) < 16 || len(values[0]) > 128 || values[0] != string(bytes.TrimSpace([]byte(values[0]))) {
		return "", errors.New("invalid idempotency key")
	}
	return values[0], nil
}

func (handler *Handler) operation(request *http.Request, capability authport.Capability) (authport.Principal, error) {
	if handler == nil || nilService(handler.stages) || request == nil {
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
	return principal, nil
}

func decodeStageBody(writer http.ResponseWriter, request *http.Request, destination any) error {
	if request.Body == nil {
		return io.EOF
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxStageBodyBytes)
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

func responseStage(stage contactport.Stage) (generated.Stage, error) {
	if stage.ID < 1 || stage.Name == "" || len(stage.Config) == 0 || !json.Valid(stage.Config) {
		return generated.Stage{}, errors.New("stage service returned an invalid stage")
	}
	var config any
	decoder := json.NewDecoder(bytes.NewReader(stage.Config))
	decoder.UseNumber()
	if err := decoder.Decode(&config); err != nil {
		return generated.Stage{}, err
	}
	return generated.Stage{Id: int64(stage.ID), Name: stage.Name, SortOrder: stage.SortOrder, Config: config}, nil
}

func writeStageJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeStageError(writer http.ResponseWriter, request *http.Request, err error) {
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	if errors.Is(err, contactport.ErrInvalidStage) {
		code = platformhttp.CodeValidationFailed
	} else if errors.Is(err, contactport.ErrStageNotFound) {
		code = platformhttp.CodeNotFound
	} else if errors.Is(err, contactport.ErrStageConflict) || errors.Is(err, contactport.ErrStageReferenced) {
		code = platformhttp.CodeConflict
	} else {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			code = platformhttp.CodeConflict
		}
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func actor(principal authport.Principal) contactport.Actor {
	return contactport.Actor("admin:" + strconv.FormatInt(principal.AdminUserID, 10))
}

func nilService(service contactport.StageService) bool {
	if service == nil {
		return true
	}
	value := reflect.ValueOf(service)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
