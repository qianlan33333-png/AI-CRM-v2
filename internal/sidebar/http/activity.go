package http

import (
	"net/http"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
)

// ActivityHandler is intentionally not registered here. Composition and route
// registration remain serial because they share the Sidebar public contract.
type ActivityHandler struct {
	context  *Handler
	activity *sidebarapp.ActivityService
}

func NewActivityHandler(contextHandler *Handler, activity *sidebarapp.ActivityService) (*ActivityHandler, error) {
	if contextHandler == nil || nilValue(contextHandler.service) || nilValue(activity) {
		return nil, sidebarapp.ErrUnavailable
	}
	return &ActivityHandler{context: contextHandler, activity: activity}, nil
}

func (handler *ActivityHandler) Timeline(writer http.ResponseWriter, request *http.Request, cursor string, limit int32) {
	scope, ok := handler.readScope(writer, request)
	if !ok {
		return
	}
	page, err := handler.activity.Timeline(request.Context(), scope, cursor, limit)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (handler *ActivityHandler) Chat(writer http.ResponseWriter, request *http.Request, chatType, cursor string, limit int32) {
	scope, ok := handler.readScope(writer, request)
	if !ok {
		return
	}
	page, err := handler.activity.Chat(request.Context(), scope, chatType, cursor, limit)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (handler *ActivityHandler) readScope(writer http.ResponseWriter, request *http.Request) (sidebarapp.Scope, bool) {
	if request == nil || request.Method != http.MethodGet {
		writeGetMethodNotAllowed(writer)
		return sidebarapp.Scope{}, false
	}
	if handler == nil || handler.context == nil || nilValue(handler.activity) {
		writeError(writer, request, sidebarapp.ErrUnavailable)
		return sidebarapp.Scope{}, false
	}
	tokens := request.Header.Values("X-Sidebar-Context-Token")
	if len(tokens) != 1 || !ValidContextToken(tokens[0]) {
		writeError(writer, request, sidebarapp.ErrTokenInvalid)
		return sidebarapp.Scope{}, false
	}
	scope, err := handler.context.scope(request, tokens[0], authport.CapabilityCustomersRead)
	if err != nil {
		writeError(writer, request, err)
		return sidebarapp.Scope{}, false
	}
	return scope, true
}

func writeGetMethodNotAllowed(writer http.ResponseWriter) {
	if writer == nil {
		return
	}
	writer.Header().Set("Allow", http.MethodGet)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusMethodNotAllowed)
}
