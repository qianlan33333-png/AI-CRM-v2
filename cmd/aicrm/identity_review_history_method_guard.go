package main

import "net/http"

const identityMergeReviewCollectionPath = "/api/v1/identity/merge-reviews"

// writeIdentityReviewMethodNotAllowed preserves the exact collection route's
// method contract before authentication. Child approve/reject routes keep their
// existing POST authorization, CSRF and idempotency middleware unchanged.
func writeIdentityReviewMethodNotAllowed(writer http.ResponseWriter, request *http.Request) bool {
	if writer == nil || request == nil || request.URL == nil ||
		request.URL.Path != identityMergeReviewCollectionPath || request.Method == http.MethodGet {
		return false
	}
	writer.Header().Set("Allow", http.MethodGet)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusMethodNotAllowed)
	return true
}
