package main

import (
	"net/http"
	"strconv"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
)

func (handler *candidateHandler) SetServicePeriodMemberGridExternalShare(writer http.ResponseWriter, request *http.Request, serviceProductID int64, _ api.SetServicePeriodMemberGridExternalShareParams) {
	if handler == nil || handler.memberGridExternalShare == nil {
		writeSurveyPublicAdminUnavailable(writer, request)
		return
	}
	handler.memberGridExternalShare.SetExternalShare(writer, request, strconv.FormatInt(serviceProductID, 10))
}

func (handler *candidateHandler) QueryPublicServicePeriodMemberGridSummary(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.memberGridPublic == nil {
		writeSurveyPublicUnavailable(writer)
		return
	}
	handler.memberGridPublic.ServeHTTP(writer, request)
}
