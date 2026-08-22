package main

import "net/http"

// SetupWizard delegates the isolated local configuration flow to its adapter.
// Authentication and CSRF are applied by the shared router before this method.
func (handler *Handler) SetupWizard(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.setupWizard == nil {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]any{
			"ok":    false,
			"error": "setup_wizard_unavailable",
		})
		return
	}
	handler.setupWizard.ServeHTTP(writer, request)
}
