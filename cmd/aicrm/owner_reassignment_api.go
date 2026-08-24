package main

import (
	"net/http"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
)

func (h *candidateHandler) DownloadContactOwnerReassignmentTemplate(w http.ResponseWriter, r *http.Request) {
	h.ownerReassignments.Template(w, r)
}
func (h *candidateHandler) CreateContactOwnerReassignmentPreview(w http.ResponseWriter, r *http.Request, _ api.CreateContactOwnerReassignmentPreviewParams) {
	h.ownerReassignments.CreatePreview(w, r)
}
func (h *candidateHandler) GetContactOwnerReassignmentPreview(w http.ResponseWriter, r *http.Request, id api.ContactOwnerReassignmentPreviewID) {
	h.ownerReassignments.Preview(w, r, string(id))
}
func (h *candidateHandler) ExecuteContactOwnerReassignmentPreview(w http.ResponseWriter, r *http.Request, id api.ContactOwnerReassignmentPreviewID, _ api.ExecuteContactOwnerReassignmentPreviewParams) {
	h.ownerReassignments.Execute(w, r, string(id))
}
func (h *candidateHandler) DownloadContactOwnerReassignmentErrors(w http.ResponseWriter, r *http.Request, id api.ContactOwnerReassignmentPreviewID) {
	h.ownerReassignments.ErrorsCSV(w, r, string(id))
}
func (h *candidateHandler) DownloadContactOwnerReassignmentResults(w http.ResponseWriter, r *http.Request, id api.ContactOwnerReassignmentPreviewID) {
	h.ownerReassignments.ResultsCSV(w, r, string(id))
}
