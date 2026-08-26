package main

import "net/http"

// AdminAccess delegates local admin login permission management after the
// central authenticated-session, authorization, CSRF, and route middleware.
func (handler *Handler) AdminAccess(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.adminAccess == nil {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "admin_access_unavailable"})
		return
	}
	handler.adminAccess.ServeHTTP(writer, request)
}
