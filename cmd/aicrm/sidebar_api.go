package main

import (
	"net/http"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func (handler *candidateHandler) MintSidebarContext(writer http.ResponseWriter, request *http.Request) {
	handler.sidebar.MintContext(writer, request)
}

func (handler *candidateHandler) GetSidebarWorkbench(writer http.ResponseWriter, request *http.Request, params api.GetSidebarWorkbenchParams) {
	handler.sidebar.Workbench(writer, request, string(params.XSidebarContextToken))
}

func (handler *candidateHandler) UpdateSidebarProfile(writer http.ResponseWriter, request *http.Request, params api.UpdateSidebarProfileParams) {
	handler.sidebar.UpdateProfile(writer, request, string(params.XSidebarContextToken), string(params.IdempotencyKey))
}

func (handler *candidateHandler) ListSidebarQuestionnaires(writer http.ResponseWriter, request *http.Request, params api.ListSidebarQuestionnairesParams) {
	limit := int32(20)
	if params.Limit != nil {
		limit = *params.Limit
	}
	handler.sidebar.Questionnaires(writer, request, string(params.XSidebarContextToken), limit)
}

func (handler *candidateHandler) ListSidebarOrders(writer http.ResponseWriter, request *http.Request, params api.ListSidebarOrdersParams) {
	limit, offset := int32(20), int32(0)
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	handler.sidebar.Orders(writer, request, string(params.XSidebarContextToken), limit, offset)
}

func (handler *candidateHandler) ListSidebarPeriodicOrders(writer http.ResponseWriter, request *http.Request, params api.ListSidebarPeriodicOrdersParams) {
	limit, offset := 20, 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	handler.sidebar.PeriodicOrders(writer, request, string(params.XSidebarContextToken), limit, offset)
}

func (handler *candidateHandler) UpdateSidebarPeriodicRemark(writer http.ResponseWriter, request *http.Request, serviceProductID int64, memberRef string, params api.UpdateSidebarPeriodicRemarkParams) {
	handler.sidebar.UpdatePeriodicRemark(writer, request, string(params.XSidebarContextToken), string(params.IdempotencyKey), serviceProductID, memberRef)
}

func (handler *candidateHandler) ListSidebarMaterials(writer http.ResponseWriter, request *http.Request, params api.ListSidebarMaterialsParams) {
	query := mediaport.ImageListQuery{Limit: 20, EnabledOnly: true}
	if params.Q != nil {
		query.Search = *params.Q
	}
	if params.Category != nil {
		query.Category = *params.Category
	}
	if params.Tags != nil {
		query.Tags = *params.Tags
	}
	if params.Limit != nil {
		query.Limit = *params.Limit
	}
	if params.Offset != nil {
		query.Offset = *params.Offset
	}
	handler.sidebar.Materials(writer, request, string(params.XSidebarContextToken), query)
}

func (handler *candidateHandler) GetSidebarMaterialThumbnailStatus(writer http.ResponseWriter, request *http.Request, imageID int64, params api.GetSidebarMaterialThumbnailStatusParams) {
	handler.sidebar.ThumbnailStatus(writer, request, string(params.XSidebarContextToken), imageID)
}

func (handler *candidateHandler) ListSidebarTimeline(writer http.ResponseWriter, request *http.Request, params api.ListSidebarTimelineParams) {
	if handler == nil || handler.sidebarActivity == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	limit := int32(0)
	if params.Limit != nil {
		if *params.Limit == 0 {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, nil))
			return
		}
		limit = *params.Limit
	}
	handler.sidebarActivity.Timeline(writer, request, cursor, limit)
}

func (handler *candidateHandler) ListSidebarChatActivity(writer http.ResponseWriter, request *http.Request, params api.ListSidebarChatActivityParams) {
	if handler == nil || handler.sidebarActivity == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	chatType, cursor := "", ""
	if params.ChatType != nil {
		chatType = string(*params.ChatType)
	}
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	limit := int32(0)
	if params.Limit != nil {
		if *params.Limit == 0 {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, nil))
			return
		}
		limit = *params.Limit
	}
	handler.sidebarActivity.Chat(writer, request, chatType, cursor, limit)
}

func writeSidebarActivityMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Allow", http.MethodGet)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusMethodNotAllowed)
	_, _ = writer.Write([]byte("{\"code\":\"method_not_allowed\"}\n"))
}
