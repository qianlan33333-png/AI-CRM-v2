package p2s16

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contacthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/http"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type snapshotDocument struct {
	Version int            `json:"version"`
	Cases   []snapshotCase `json:"cases"`
}

type snapshotCase struct {
	OperationID    string           `json:"operation_id"`
	CaseID         string           `json:"case_id"`
	Request        snapshotRequest  `json:"request"`
	ActualResponse snapshotResponse `json:"actual_response"`
}

type snapshotRequest struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body"`
}

type snapshotResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// GenerateStageSnapshot exercises the generated router and the real stages handler.
// The returned actual document is ephemeral and is intended only for stdin comparison.
func GenerateStageSnapshot() ([]byte, error) {
	service := &snapshotStageService{}
	stageHandler, err := contacthttp.NewHandler(service)
	if err != nil {
		return nil, err
	}
	groupID, groupName, groupSort := int64(31), "Lifecycle", int32(1)
	tagRecords := []contactapp.TagCatalogRecord{
		{ID: 81, GroupID: &groupID, GroupName: &groupName, GroupSortOrder: &groupSort, Name: "Priority", SortOrder: 6},
		{ID: 82, Name: "Ungrouped", SortOrder: 7},
	}
	cases := []struct {
		operationID string
		caseID      string
		method      string
		path        string
		body        string
		capability  authport.Capability
		tags        snapshotTagApplication
	}{
		{operationID: "createStage", caseID: "success", method: http.MethodPost, path: "/api/v1/stages", body: `{"name":"Qualified","sort_order":2,"config":{"color":"blue"}}`, capability: authport.CapabilityStagesWrite},
		{operationID: "listStages", caseID: "success", method: http.MethodGet, path: "/api/v1/stages", body: `null`, capability: authport.CapabilityStagesRead},
		{operationID: "listTags", caseID: "empty", method: http.MethodGet, path: "/api/v1/tags", body: `null`, capability: authport.CapabilityCustomersRead, tags: snapshotTagApplication{records: []contactapp.TagCatalogRecord{}}},
		{operationID: "listTags", caseID: "success", method: http.MethodGet, path: "/api/v1/tags", body: `null`, capability: authport.CapabilityCustomersRead, tags: snapshotTagApplication{records: tagRecords}},
		{operationID: "listTags", caseID: "unavailable", method: http.MethodGet, path: "/api/v1/tags", body: `null`, capability: authport.CapabilityCustomersRead, tags: snapshotTagApplication{err: contactapp.ErrTagCatalogUnavailable}},
		{operationID: "renameStage", caseID: "success", method: http.MethodPatch, path: "/api/v1/stages/101", body: `{"name":"Customer"}`, capability: authport.CapabilityStagesWrite},
	}
	document := snapshotDocument{Version: 1, Cases: make([]snapshotCase, 0, len(cases))}
	for _, item := range cases {
		tagHandler, tagErr := contacthttp.NewTagCatalogHandler(item.tags)
		if tagErr != nil {
			return nil, tagErr
		}
		handler := &snapshotContactHandler{Handler: stageHandler, tags: tagHandler}
		router := generated.Handler(handler)
		requestBody := bytes.NewBuffer(nil)
		if item.body != "null" {
			requestBody.WriteString(item.body)
		}
		request := httptest.NewRequest(item.method, item.path, requestBody)
		request.Header.Set("Content-Type", "application/json")
		if item.capability == authport.CapabilityStagesWrite {
			request.Header.Set("X-CSRF-Token", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		}
		ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, "snapshot-session")
		ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: item.capability, Scope: authport.ScopeGlobal})
		if err != nil {
			return nil, err
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request.WithContext(ctx))
		body := bytes.TrimSpace(response.Body.Bytes())
		if len(body) == 0 {
			body = []byte("null")
		}
		document.Cases = append(document.Cases, snapshotCase{
			OperationID:    item.operationID,
			CaseID:         item.caseID,
			Request:        snapshotRequest{Method: item.method, Path: item.path, Body: json.RawMessage(item.body)},
			ActualResponse: snapshotResponse{Status: response.Code, Body: append(json.RawMessage(nil), body...)},
		})
	}
	return json.Marshal(document)
}

type snapshotContactHandler struct {
	*contacthttp.Handler
	tags *contacthttp.TagCatalogHandler
}

func (handler *snapshotContactHandler) ListTags(writer http.ResponseWriter, request *http.Request) {
	handler.tags.ListTags(writer, request)
}

type snapshotTagApplication struct {
	records []contactapp.TagCatalogRecord
	err     error
}

func (application snapshotTagApplication) List(context.Context) ([]contactapp.TagCatalogRecord, error) {
	return application.records, application.err
}

type snapshotStageService struct{ stage contactport.Stage }

func (service *snapshotStageService) ListStages(context.Context) ([]contactport.Stage, error) {
	if service.stage.ID == 0 {
		return []contactport.Stage{}, nil
	}
	return []contactport.Stage{service.stage}, nil
}

func (service *snapshotStageService) CreateStage(_ context.Context, command contactport.CreateStageCommand) (contactport.Stage, error) {
	if command.Actor != "admin:7" {
		return contactport.Stage{}, errors.New("unexpected actor")
	}
	service.stage = contactport.Stage{ID: 101, Name: command.Name, SortOrder: command.SortOrder, Config: append(json.RawMessage(nil), command.Config...)}
	return service.stage, nil
}

func (service *snapshotStageService) RenameStage(_ context.Context, command contactport.RenameStageCommand) (contactport.Stage, error) {
	if command.Actor != "admin:7" || command.ID != service.stage.ID {
		return contactport.Stage{}, errors.New("unexpected rename")
	}
	service.stage.Name = command.Name
	return service.stage, nil
}
