package main

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/userops/domain"
	useropshttp "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/http"
	useropsport "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/port"
)

const userOpsMaximumBodyBytes = 256 << 10

var errInvalidUserOpsBody = errors.New("invalid user ops request body")

func (handler *candidateHandler) GetUserOpsOverview(writer http.ResponseWriter, request *http.Request) {
	leaf, ok := userOpsLeaf(writer, request, handler)
	if !ok {
		return
	}
	result, err := leaf.Overview(request, useropsport.DirectoryQuery{})
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	response, err := userOpsOverviewResponse(result)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *candidateHandler) ListUserOpsCustomers(writer http.ResponseWriter, request *http.Request, params api.ListUserOpsCustomersParams) {
	leaf, ok := userOpsLeaf(writer, request, handler)
	if !ok {
		return
	}
	query, err := userOpsDirectoryQueryFromList(params)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	result, err := leaf.ListCustomers(request, query)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	response, err := userOpsCustomerPageResponse(result)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *candidateHandler) GetUserOpsCustomerDetail(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID) {
	leaf, ok := userOpsLeaf(writer, request, handler)
	if !ok {
		return
	}
	if customerID < 1 {
		writeUserOpsError(writer, request, useropsport.ErrInvalid)
		return
	}
	result, err := leaf.GetCustomerDetail(request, domain.CustomerID(customerID))
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	response, err := userOpsCustomerDetailResponse(result)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *candidateHandler) PreviewUserOpsSafeExport(writer http.ResponseWriter, request *http.Request) {
	leaf, ok := userOpsLeaf(writer, request, handler)
	if !ok {
		return
	}
	body, err := decodeUserOpsJSON[api.UserOpsSafeExportRequest](writer, request)
	if err != nil {
		writeUserOpsError(writer, request, useropsport.ErrInvalid)
		return
	}
	query, err := userOpsDirectoryQueryFromBody(body.Query)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	fields := make([]useropsport.SafeExportField, len(body.Fields))
	for index, field := range body.Fields {
		fields[index] = useropsport.SafeExportField(field)
	}
	result, err := leaf.SafeExport(request, useropsport.SafeExportRequest{Query: query, Fields: fields})
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	response, err := userOpsSafeExportResponse(result)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *candidateHandler) PreviewUserOpsBatch(writer http.ResponseWriter, request *http.Request) {
	leaf, ok := userOpsLeaf(writer, request, handler)
	if !ok {
		return
	}
	body, err := decodeUserOpsJSON[api.UserOpsBatchPreviewRequest](writer, request)
	if err != nil {
		writeUserOpsError(writer, request, useropsport.ErrInvalid)
		return
	}
	result, err := leaf.PreviewBatch(request, useropsport.BatchPreviewInput{
		CustomerIDs: userOpsCustomerIDs(body.CustomerIds),
		Content:     userOpsContentInput(body.Content),
	})
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	response, err := userOpsBatchPreviewResponse(result)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *candidateHandler) CreateUserOpsLocalPlan(writer http.ResponseWriter, request *http.Request, _ api.CreateUserOpsLocalPlanParams) {
	leaf, ok := userOpsLeaf(writer, request, handler)
	if !ok {
		return
	}
	body, err := decodeUserOpsJSON[api.UserOpsCreateLocalPlanRequest](writer, request)
	if err != nil {
		writeUserOpsError(writer, request, useropsport.ErrInvalid)
		return
	}
	result, err := leaf.CreateLocalPlan(request, useropsport.CreateLocalPlanInput{
		CustomerIDs:           userOpsCustomerIDs(body.CustomerIds),
		ExpectedTargetDigest:  body.ExpectedTargetDigest,
		Content:               userOpsContentInput(body.Content),
		ExpectedContentDigest: body.ExpectedContentDigest,
		State:                 domain.LocalPlanState(body.State),
	})
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	response, err := userOpsLocalPlanResponse(result)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, response)
}

func (handler *candidateHandler) SetUserOpsCustomerDnd(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, _ api.SetUserOpsCustomerDndParams) {
	leaf, ok := userOpsLeaf(writer, request, handler)
	if !ok {
		return
	}
	if customerID < 1 {
		writeUserOpsError(writer, request, useropsport.ErrInvalid)
		return
	}
	body, err := decodeUserOpsJSON[api.UserOpsSetDndRequest](writer, request)
	if err != nil {
		writeUserOpsError(writer, request, useropsport.ErrInvalid)
		return
	}
	result, err := leaf.SetDND(request, useropsport.UpsertDNDInput{
		CustomerID:      domain.CustomerID(customerID),
		Reason:          body.Reason,
		ExpectedVersion: cloneUserOpsAPIInt64(body.ExpectedVersion),
	})
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	response, err := userOpsDNDMutationResponse(result)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *candidateHandler) ClearUserOpsCustomerDnd(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, _ api.ClearUserOpsCustomerDndParams) {
	leaf, ok := userOpsLeaf(writer, request, handler)
	if !ok {
		return
	}
	if customerID < 1 {
		writeUserOpsError(writer, request, useropsport.ErrInvalid)
		return
	}
	body, err := decodeUserOpsJSON[api.UserOpsClearDndRequest](writer, request)
	if err != nil {
		writeUserOpsError(writer, request, useropsport.ErrInvalid)
		return
	}
	result, err := leaf.ClearDND(request, useropsport.ClearDNDInput{
		CustomerID:      domain.CustomerID(customerID),
		ExpectedVersion: body.ExpectedVersion,
	})
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	response, err := userOpsDNDMutationResponse(result)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *candidateHandler) ListUserOpsSendRecords(writer http.ResponseWriter, request *http.Request, planID api.UserOpsPlanID, params api.ListUserOpsSendRecordsParams) {
	leaf, ok := userOpsLeaf(writer, request, handler)
	if !ok {
		return
	}
	parsedPlanID, err := parseUserOpsPlanID(string(planID))
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	limit, err := userOpsPageLimitFromInt(params.Limit)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	query := useropsport.SendRecordQuery{PlanID: parsedPlanID, Limit: limit}
	if params.Cursor != nil {
		query.Cursor = string(*params.Cursor)
	}
	result, err := leaf.ListSendRecords(request, query)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	response, err := userOpsSendRecordPageResponse(result)
	if err != nil {
		writeUserOpsError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func userOpsLeaf(writer http.ResponseWriter, request *http.Request, handler *candidateHandler) (*useropshttp.Handler, bool) {
	if handler == nil || handler.userOps == nil || request == nil {
		writeUserOpsError(writer, request, useropsport.ErrUnavailable)
		return nil, false
	}
	return handler.userOps, true
}

func decodeUserOpsJSON[T any](writer http.ResponseWriter, request *http.Request) (T, error) {
	var value T
	if request == nil || request.Body == nil {
		return value, errInvalidUserOpsBody
	}
	contentType := request.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "application/json" {
			return value, errInvalidUserOpsBody
		}
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, userOpsMaximumBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, errInvalidUserOpsBody
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return value, errInvalidUserOpsBody
	}
	return value, nil
}

func userOpsDirectoryQueryFromList(params api.ListUserOpsCustomersParams) (useropsport.DirectoryQuery, error) {
	limit, err := userOpsPageLimitFromInt(params.Limit)
	if err != nil {
		return useropsport.DirectoryQuery{}, err
	}
	query := useropsport.DirectoryQuery{
		OwnerStaffID: cloneUserOpsAPIInt64(params.OwnerStaffId),
		StageID:      cloneUserOpsAPIInt64(params.StageId),
		ChannelID:    cloneUserOpsAPIInt64(params.ChannelId),
		TagID:        cloneUserOpsAPIInt64(params.TagId),
		Limit:        limit,
	}
	if params.Keyword != nil {
		query.Keyword = string(*params.Keyword)
	}
	if params.PhoneExact != nil {
		query.PhoneExact = string(*params.PhoneExact)
	}
	if params.Cursor != nil {
		query.Cursor = string(*params.Cursor)
	}
	return query, nil
}

func userOpsDirectoryQueryFromBody(input api.UserOpsDirectoryQuery) (useropsport.DirectoryQuery, error) {
	limit, err := userOpsPageLimitFromInt32(input.Limit)
	if err != nil {
		return useropsport.DirectoryQuery{}, err
	}
	query := useropsport.DirectoryQuery{
		OwnerStaffID: cloneUserOpsAPIInt64(input.OwnerStaffId),
		StageID:      cloneUserOpsAPIInt64(input.StageId),
		ChannelID:    cloneUserOpsAPIInt64(input.ChannelId),
		TagID:        cloneUserOpsAPIInt64(input.TagId),
		Limit:        limit,
	}
	if input.Keyword != nil {
		query.Keyword = *input.Keyword
	}
	if input.PhoneExact != nil {
		query.PhoneExact = *input.PhoneExact
	}
	if input.Cursor != nil {
		query.Cursor = *input.Cursor
	}
	return query, nil
}

func userOpsPageLimitFromInt(value *int) (int32, error) {
	if value == nil {
		return 0, nil
	}
	if *value < 1 || *value > int(useropsport.MaximumPageLimit) {
		return 0, useropsport.ErrInvalid
	}
	return int32(*value), nil
}

func userOpsPageLimitFromInt32(value *int32) (int32, error) {
	if value == nil {
		return 0, nil
	}
	if *value < 1 || *value > useropsport.MaximumPageLimit {
		return 0, useropsport.ErrInvalid
	}
	return *value, nil
}

func parseUserOpsPlanID(raw string) (domain.PlanID, error) {
	if len(raw) == 0 || len(raw) > 19 || raw[0] < '1' || raw[0] > '9' {
		return 0, useropsport.ErrInvalid
	}
	for _, character := range raw[1:] {
		if character < '0' || character > '9' {
			return 0, useropsport.ErrInvalid
		}
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return 0, useropsport.ErrInvalid
	}
	return domain.PlanID(value), nil
}

func userOpsCustomerIDs(values []int64) []domain.CustomerID {
	result := make([]domain.CustomerID, len(values))
	for index, value := range values {
		result[index] = domain.CustomerID(value)
	}
	return result
}

func userOpsContentInput(value api.UserOpsContentInput) domain.ContentInput {
	return domain.ContentInput{
		Text:                  value.Text,
		ImageLibraryIDs:       append([]int64(nil), value.ImageLibraryIds...),
		MiniProgramLibraryIDs: append([]int64(nil), value.MiniprogramLibraryIds...),
		AttachmentLibraryIDs:  append([]int64(nil), value.AttachmentLibraryIds...),
	}
}

func cloneUserOpsAPIInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func writeUserOpsError(writer http.ResponseWriter, request *http.Request, err error) {
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, useropshttp.ErrUnauthenticated):
		code = platformhttp.CodeUnauthenticated
	case errors.Is(err, useropshttp.ErrForbidden), errors.Is(err, useropshttp.ErrCSRFInvalid):
		code = platformhttp.CodeUnauthorized
	case errors.Is(err, useropsport.ErrInvalid), errors.Is(err, errInvalidUserOpsBody):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, useropsport.ErrNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, useropsport.ErrConflict), errors.Is(err, useropsport.ErrPreviewStale):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func userOpsLocalSafety(value useropsport.Safety) error {
	if value.ProviderExecutionEligible || value.RealExternalCallExecuted || value.DeliveryProven {
		return useropsport.ErrUnavailable
	}
	return nil
}

func userOpsOverviewResponse(value useropsport.Overview) (api.UserOpsOverviewResponse, error) {
	if err := userOpsLocalSafety(value.Safety); err != nil {
		return api.UserOpsOverviewResponse{}, err
	}
	return api.UserOpsOverviewResponse{
		CustomerCount: value.CustomerCount, CustomerCountIsEstimate: value.CustomerCountIsEstimate,
		ActiveDndCount: value.ActiveDNDCount, DraftPlanCount: value.DraftPlanCount, PendingReviewPlanCount: value.PendingReviewPlanCount,
		ProviderExecutionEligible: false, RealExternalCallExecuted: false, DeliveryProven: false,
	}, nil
}

func userOpsCustomerPageResponse(value useropsport.DirectoryPage) (api.UserOpsCustomerPage, error) {
	if err := userOpsLocalSafety(value.Safety); err != nil {
		return api.UserOpsCustomerPage{}, err
	}
	items := make([]api.UserOpsCustomer, len(value.Items))
	for index, item := range value.Items {
		items[index] = userOpsCustomer(item)
	}
	return api.UserOpsCustomerPage{
		Items: items, NextCursor: cloneUserOpsString(value.NextCursor), Total: value.Total, TotalIsEstimate: value.TotalIsEstimate,
		ProviderExecutionEligible: false, RealExternalCallExecuted: false, DeliveryProven: false,
	}, nil
}

func userOpsCustomerDetailResponse(value useropsport.CustomerDetailResult) (api.UserOpsCustomerDetailResponse, error) {
	if err := userOpsLocalSafety(value.Safety); err != nil {
		return api.UserOpsCustomerDetailResponse{}, err
	}
	tags := make([]api.UserOpsCustomerTag, len(value.Detail.Tags))
	for index, tag := range value.Detail.Tags {
		tags[index] = api.UserOpsCustomerTag{Id: tag.ID, GroupId: cloneUserOpsAPIInt64(tag.GroupID), GroupName: cloneUserOpsString(tag.GroupName), Name: tag.Name}
	}
	timeline := make([]api.UserOpsTimelineEntry, len(value.Detail.Timeline))
	for index, event := range value.Detail.Timeline {
		timeline[index] = api.UserOpsTimelineEntry{EventType: event.EventType, OccurredAt: event.OccurredAt.UTC()}
	}
	return api.UserOpsCustomerDetailResponse{
		Customer: userOpsCustomer(value.Detail.Customer), Tags: tags, Timeline: timeline, Dnd: userOpsDND(value.DND),
		ProviderExecutionEligible: false, RealExternalCallExecuted: false, DeliveryProven: false,
	}, nil
}

func userOpsSafeExportResponse(value useropsport.SafeExport) (api.UserOpsSafeExportResponse, error) {
	if err := userOpsLocalSafety(value.Safety); err != nil {
		return api.UserOpsSafeExportResponse{}, err
	}
	fields := make([]api.UserOpsSafeExportField, len(value.Fields))
	for index, field := range value.Fields {
		fields[index] = api.UserOpsSafeExportField(field)
	}
	rows := make([][]string, len(value.Rows))
	for index, row := range value.Rows {
		rows[index] = append([]string(nil), row...)
	}
	return api.UserOpsSafeExportResponse{
		Fields: fields, Rows: rows, NextCursor: cloneUserOpsString(value.NextCursor), Total: value.Total, TotalIsEstimate: value.TotalIsEstimate,
		ProviderExecutionEligible: false, RealExternalCallExecuted: false, DeliveryProven: false,
	}, nil
}

func userOpsBatchPreviewResponse(value useropsport.BatchPreview) (api.UserOpsBatchPreviewResponse, error) {
	if err := userOpsLocalSafety(value.Safety); err != nil {
		return api.UserOpsBatchPreviewResponse{}, err
	}
	ids := make([]int64, len(value.TargetCustomerIDs))
	for index, id := range value.TargetCustomerIDs {
		ids[index] = int64(id)
	}
	return api.UserOpsBatchPreviewResponse{
		TargetCustomerIds: ids, ExcludedDndCount: value.ExcludedDNDCount, TargetDigest: value.TargetDigest, Content: userOpsContentSnapshot(value.Content),
		ProviderExecutionEligible: false, RealExternalCallExecuted: false, DeliveryProven: false,
	}, nil
}

func userOpsLocalPlanResponse(value useropsport.LocalPlanResult) (api.UserOpsLocalPlanResponse, error) {
	if err := userOpsLocalSafety(value.Safety); err != nil {
		return api.UserOpsLocalPlanResponse{}, err
	}
	return api.UserOpsLocalPlanResponse{
		Plan: userOpsLocalPlan(value.Plan), ProviderExecutionEligible: false, RealExternalCallExecuted: false, DeliveryProven: false,
	}, nil
}

func userOpsDNDMutationResponse(value useropsport.DNDMutationResult) (api.UserOpsDndMutationResponse, error) {
	if err := userOpsLocalSafety(value.Safety); err != nil {
		return api.UserOpsDndMutationResponse{}, err
	}
	return api.UserOpsDndMutationResponse{
		Dnd: userOpsDND(value.DND), Cleared: value.Cleared,
		ProviderExecutionEligible: false, RealExternalCallExecuted: false, DeliveryProven: false,
	}, nil
}

func userOpsSendRecordPageResponse(value useropsport.SendRecordPage) (api.UserOpsSendRecordPage, error) {
	if err := userOpsLocalSafety(value.Safety); err != nil {
		return api.UserOpsSendRecordPage{}, err
	}
	items := make([]api.UserOpsSendRecord, len(value.Items))
	for index, item := range value.Items {
		items[index] = api.UserOpsSendRecord{
			SendRecordId: strconv.FormatInt(int64(item.ID), 10), PlanId: strconv.FormatInt(int64(item.PlanID), 10), CustomerId: int64(item.CustomerID),
			TechnicalStatus: api.UserOpsSendTechnicalState(item.TechnicalStatus), CreatedAt: item.CreatedAt.UTC(), UpdatedAt: item.UpdatedAt.UTC(),
		}
	}
	return api.UserOpsSendRecordPage{
		Items: items, NextCursor: cloneUserOpsString(value.NextCursor), Total: value.Total,
		ProviderExecutionEligible: false, RealExternalCallExecuted: false, DeliveryProven: false,
	}, nil
}

func userOpsCustomer(value useropsport.CustomerSummary) api.UserOpsCustomer {
	return api.UserOpsCustomer{
		CustomerId: int64(value.CustomerID), Name: value.Name,
		OwnerStaffId: cloneUserOpsAPIInt64(value.OwnerStaffID), StageId: cloneUserOpsAPIInt64(value.StageID), ChannelId: cloneUserOpsAPIInt64(value.ChannelID),
		AddedAt: cloneUserOpsTime(value.AddedAt), LastInteractAt: cloneUserOpsTime(value.LastInteractAt),
	}
}

func userOpsDND(value *domain.DoNotDisturb) *api.UserOpsDnd {
	if value == nil {
		return nil
	}
	return &api.UserOpsDnd{
		CustomerId: int64(value.CustomerID), Reason: value.Reason, Version: value.Version,
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func userOpsLocalPlan(value domain.LocalPlan) api.UserOpsLocalPlan {
	return api.UserOpsLocalPlan{
		PlanId: strconv.FormatInt(int64(value.ID), 10), State: api.UserOpsLocalPlanState(value.State), Content: userOpsContentSnapshot(value.Content),
		TargetDigest: value.TargetDigest, TargetCount: value.TargetCount, Version: value.Version,
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func userOpsContentSnapshot(value domain.ContentSnapshot) api.UserOpsContentSnapshot {
	return api.UserOpsContentSnapshot{
		Text: value.Text, ImageLibraryIds: append([]int64(nil), value.ImageLibraryIDs...),
		MiniprogramLibraryIds: append([]int64(nil), value.MiniProgramLibraryIDs...), AttachmentLibraryIds: append([]int64(nil), value.AttachmentLibraryIDs...),
		ContentDigest: value.ContentDigest,
	}
}
