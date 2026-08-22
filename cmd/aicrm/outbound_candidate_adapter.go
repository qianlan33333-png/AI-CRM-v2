package main

import (
	"net/http"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
)

func (handler *candidateHandler) legacyOutboundHandler(writer http.ResponseWriter, request *http.Request) *Handler {
	if handler == nil || handler.outboundLegacy == nil {
		legacyOutboundError(writer, request, outboundapp.ErrTaskQueryUnavailable)
		return nil
	}
	return handler.outboundLegacy
}

func (handler *candidateHandler) ListLegacyOutboundJobs(writer http.ResponseWriter, request *http.Request, _ api.ListLegacyOutboundJobsParams) {
	if legacy := handler.legacyOutboundHandler(writer, request); legacy != nil {
		legacy.ListOutboundJobs(writer, request)
	}
}

func (handler *candidateHandler) GetLegacyOutboundJob(writer http.ResponseWriter, request *http.Request, _ int64) {
	if legacy := handler.legacyOutboundHandler(writer, request); legacy != nil {
		legacy.GetOutboundJob(writer, request)
	}
}

func (handler *candidateHandler) GetLegacyOutboundJobReconciliation(writer http.ResponseWriter, request *http.Request, _ int64) {
	if legacy := handler.legacyOutboundHandler(writer, request); legacy != nil {
		legacy.ReconcileOutboundJob(writer, request)
	}
}

func (handler *candidateHandler) CancelLegacyOutboundJob(writer http.ResponseWriter, request *http.Request, _ int64, _ api.CancelLegacyOutboundJobParams) {
	if legacy := handler.legacyOutboundHandler(writer, request); legacy != nil {
		legacy.CancelOutboundJob(writer, request)
	}
}

func (handler *candidateHandler) RetryLegacyOutboundJob(writer http.ResponseWriter, request *http.Request, _ int64, _ api.RetryLegacyOutboundJobParams) {
	if legacy := handler.legacyOutboundHandler(writer, request); legacy != nil {
		legacy.RetryOutboundJob(writer, request)
	}
}
