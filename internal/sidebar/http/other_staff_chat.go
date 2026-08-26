package http

import (
	"net/http"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
)

// OtherStaffChatHandler is intentionally registered by the API composition
// layer. Its only source is the existing local archive projection.
type OtherStaffChatHandler struct {
	context *Handler
	chats   *sidebarapp.OtherStaffChatService
}

func NewOtherStaffChatHandler(contextHandler *Handler, chats *sidebarapp.OtherStaffChatService) (*OtherStaffChatHandler, error) {
	if contextHandler == nil || nilValue(contextHandler.service) || nilValue(chats) {
		return nil, sidebarapp.ErrUnavailable
	}
	return &OtherStaffChatHandler{context: contextHandler, chats: chats}, nil
}

func (handler *OtherStaffChatHandler) List(writer http.ResponseWriter, request *http.Request) {
	if request == nil || request.Method != http.MethodGet {
		writeGetMethodNotAllowed(writer)
		return
	}
	if handler == nil || handler.context == nil || nilValue(handler.chats) {
		writeError(writer, request, sidebarapp.ErrUnavailable)
		return
	}
	tokens := request.Header.Values("X-Sidebar-Context-Token")
	if len(tokens) != 1 || !ValidContextToken(tokens[0]) {
		writeError(writer, request, sidebarapp.ErrTokenInvalid)
		return
	}
	scope, err := handler.context.scope(request, tokens[0], authport.CapabilityCustomersRead)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	items, err := handler.chats.List(request.Context(), scope)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		Items  []sidebarapp.OtherStaffChat `json:"items"`
		Safety sidebarapp.Safety           `json:"safety"`
	}{Items: items, Safety: sidebarapp.Safety{LocalOnly: true}})
}
