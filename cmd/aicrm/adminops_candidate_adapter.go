package main

import (
	"net/http"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
)

func (handler *candidateHandler) serveAdminOps(writer http.ResponseWriter, request *http.Request) {
	if handler.adminOps == nil {
		writeAdminOpsError(writer, http.StatusServiceUnavailable, "admin_ops_unavailable")
		return
	}
	handler.adminOps.ServeHTTP(writer, request)
}

func (handler *candidateHandler) GetAdminOpsConfigPage(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) GetAdminOpsReleasesPage(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) GetAdminOpsNewReleasePage(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) GetAdminOpsReleasePage(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsReleaseID) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) ListAdminOpsBroadcastJobs(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) RunAdminOpsFeishuHourlyReportPlan(writer http.ResponseWriter, request *http.Request, _ api.RunAdminOpsFeishuHourlyReportPlanParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) GetAdminOpsFeishuNotificationSetting(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) SaveAdminOpsFeishuNotificationSetting(writer http.ResponseWriter, request *http.Request, _ api.SaveAdminOpsFeishuNotificationSettingParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) ValidateAdminOpsFeishuNotificationPlan(writer http.ResponseWriter, request *http.Request, _ api.ValidateAdminOpsFeishuNotificationPlanParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) GetAdminOpsBroadcastJob(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsJobID) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) ApproveAdminOpsBroadcastJob(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsJobID, _ api.ApproveAdminOpsBroadcastJobParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) CancelAdminOpsBroadcastJob(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsJobID, _ api.CancelAdminOpsBroadcastJobParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) ListAdminOpsCategories(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) GetAdminOpsCategory(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsCategoryKey) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) CheckAdminOpsCategory(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsCategoryKey, _ api.CheckAdminOpsCategoryParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) SetAdminOpsCategoryEnabled(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsCategoryKey, _ api.SetAdminOpsCategoryEnabledParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) SetAdminOpsCategorySettings(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsCategoryKey, _ api.SetAdminOpsCategorySettingsParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) GetAdminOpsPushCapabilities(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) SetAdminOpsPushScheduler(writer http.ResponseWriter, request *http.Request, _ api.SetAdminOpsPushSchedulerParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) SetAdminOpsPushCapability(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsCapabilityKey, _ api.SetAdminOpsPushCapabilityParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) ListAdminOpsReleases(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) CreateAdminOpsRelease(writer http.ResponseWriter, request *http.Request, _ api.CreateAdminOpsReleaseParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) GetAdminOpsRelease(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsReleaseID) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) PublishAdminOpsRelease(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsReleaseID, _ api.PublishAdminOpsReleaseParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) RollbackAdminOpsRelease(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsReleaseID, _ api.RollbackAdminOpsReleaseParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) CompareAdminOpsReleaseShadow(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsReleaseID) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) ValidateAdminOpsRelease(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsReleaseID, _ api.ValidateAdminOpsReleaseParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) ListAdminOpsArchiveSyncJobs(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) RunAdminOpsArchiveSyncPlan(writer http.ResponseWriter, request *http.Request, _ api.RunAdminOpsArchiveSyncPlanParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) ListAdminOpsCallbackJobs(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) ListAdminOpsDeferredJobs(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) ListAdminOpsMessageBatchJobs(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) GetAdminOpsMessageBatch(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsBatchID) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) AcknowledgeAdminOpsMessageBatch(writer http.ResponseWriter, request *http.Request, _ api.AdminOpsBatchID, _ api.AcknowledgeAdminOpsMessageBatchParams) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) GetAdminOpsJobsSummary(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
func (handler *candidateHandler) ListAdminOpsWebhookDeliveryJobs(writer http.ResponseWriter, request *http.Request) {
	handler.serveAdminOps(writer, request)
}
