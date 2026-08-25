package main

import (
	"net/http"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func (handler *candidateHandler) ListReleaseCandidates(writer http.ResponseWriter, request *http.Request, params api.ListReleaseCandidatesParams) {
	if !handler.releaseAvailable(writer, request) {
		return
	}
	limit := int32(50)
	if params.Limit != nil {
		limit = int32(*params.Limit)
	}
	handler.release.List(writer, request, limit)
}
func (handler *candidateHandler) RegisterReleaseCandidate(writer http.ResponseWriter, request *http.Request, _ api.RegisterReleaseCandidateParams) {
	if !handler.releaseAvailable(writer, request) {
		return
	}
	handler.release.Register(writer, request)
}
func (handler *candidateHandler) GetReleaseCandidate(writer http.ResponseWriter, request *http.Request, candidateID api.ReleaseCandidateID) {
	if !handler.releaseAvailable(writer, request) {
		return
	}
	handler.release.Detail(writer, request, int64(candidateID))
}
func (handler *candidateHandler) RecordReleasePrerequisite(writer http.ResponseWriter, request *http.Request, candidateID api.ReleaseCandidateID, _ api.RecordReleasePrerequisiteParams) {
	if !handler.releaseAvailable(writer, request) {
		return
	}
	handler.release.RecordPrerequisite(writer, request, int64(candidateID))
}
func (handler *candidateHandler) PrepareReleaseCandidate(writer http.ResponseWriter, request *http.Request, candidateID api.ReleaseCandidateID, _ api.PrepareReleaseCandidateParams) {
	if !handler.releaseAvailable(writer, request) {
		return
	}
	handler.release.Prepare(writer, request, int64(candidateID))
}
func (handler *candidateHandler) StartReleaseCutover(writer http.ResponseWriter, request *http.Request, candidateID api.ReleaseCandidateID, _ api.StartReleaseCutoverParams) {
	if !handler.releaseAvailable(writer, request) {
		return
	}
	handler.release.StartCutover(writer, request, int64(candidateID))
}
func (handler *candidateHandler) RestartReleaseCutover(writer http.ResponseWriter, request *http.Request, candidateID api.ReleaseCandidateID, _ api.RestartReleaseCutoverParams) {
	if !handler.releaseAvailable(writer, request) {
		return
	}
	handler.release.RestartCutover(writer, request, int64(candidateID))
}
func (handler *candidateHandler) CompleteReleaseCutoverStep(writer http.ResponseWriter, request *http.Request, candidateID api.ReleaseCandidateID, step api.ReleaseCutoverStep, _ api.CompleteReleaseCutoverStepParams) {
	if !handler.releaseAvailable(writer, request) {
		return
	}
	handler.release.CompleteStep(writer, request, int64(candidateID), string(step))
}
func (handler *candidateHandler) ActivateReleaseCandidate(writer http.ResponseWriter, request *http.Request, candidateID api.ReleaseCandidateID, _ api.ActivateReleaseCandidateParams) {
	if !handler.releaseAvailable(writer, request) {
		return
	}
	handler.release.Activate(writer, request, int64(candidateID))
}
func (handler *candidateHandler) RecordReleaseRollbackCheck(writer http.ResponseWriter, request *http.Request, candidateID api.ReleaseCandidateID, _ api.RecordReleaseRollbackCheckParams) {
	if !handler.releaseAvailable(writer, request) {
		return
	}
	handler.release.RecordRollbackCheck(writer, request, int64(candidateID))
}
func (handler *candidateHandler) RequestReleaseRollback(writer http.ResponseWriter, request *http.Request, candidateID api.ReleaseCandidateID, _ api.RequestReleaseRollbackParams) {
	if !handler.releaseAvailable(writer, request) {
		return
	}
	handler.release.RequestRollback(writer, request, int64(candidateID))
}
func (handler *candidateHandler) CompleteReleaseRollback(writer http.ResponseWriter, request *http.Request, candidateID api.ReleaseCandidateID, _ api.CompleteReleaseRollbackParams) {
	if !handler.releaseAvailable(writer, request) {
		return
	}
	handler.release.CompleteRollback(writer, request, int64(candidateID))
}

func (handler *candidateHandler) releaseAvailable(writer http.ResponseWriter, request *http.Request) bool {
	if handler != nil && handler.release != nil {
		return true
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
	return false
}
