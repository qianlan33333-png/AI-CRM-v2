// Package app implements User Ops local read and planning use cases.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/userops/domain"
	useropsport "github.com/qianlan33333-png/AI-CRM-v2/internal/userops/port"
)

const (
	maximumKeywordRunes = 200
	maximumPhoneBytes   = 64
	maximumCursorBytes  = 512
	maximumReasonRunes  = 500
	minimumKeyBytes     = 8
	maximumKeyBytes     = 200
	maximumContentRunes = 4_000
	maximumImages       = 3
	maximumMiniPrograms = 1
	maximumAttachments  = 9
	maximumMaterials    = 9
)

const (
	eventDNDSet           = "user_ops.dnd_set"
	eventDNDCleared       = "user_ops.dnd_cleared"
	eventLocalPlanCreated = "user_ops.local_plan_created"
)

// Service depends solely on User Ops ports. The composition root supplies
// safe Contact, Customer 360, Identity, UoW and event-log adapters later.
type Service struct {
	uow        useropsport.UnitOfWork
	directory  useropsport.CustomerDirectoryReader
	details    useropsport.CustomerDetailReader
	materials  useropsport.MaterialReader
	repository useropsport.Repository
	events     useropsport.EventAppender
	now        func() time.Time
}

func NewService(
	uow useropsport.UnitOfWork,
	directory useropsport.CustomerDirectoryReader,
	details useropsport.CustomerDetailReader,
	materials useropsport.MaterialReader,
	repository useropsport.Repository,
	events useropsport.EventAppender,
) *Service {
	return &Service{
		uow:        uow,
		directory:  directory,
		details:    details,
		materials:  materials,
		repository: repository,
		events:     events,
		now:        time.Now,
	}
}

var _ useropsport.Application = (*Service)(nil)

func (service *Service) Overview(ctx context.Context, input useropsport.DirectoryQuery) (useropsport.Overview, error) {
	query, err := normalizeDirectoryQuery(input)
	if err != nil || !service.ready(service.directory, service.repository) || ctx == nil {
		return useropsport.Overview{}, invalidOrUnavailable(err, service.ready(service.directory, service.repository), ctx)
	}
	var directory useropsport.DirectoryOverviewRead
	var local useropsport.LocalOverviewRead
	err = service.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		directory, readErr = service.directory.ReadOverview(tx, query)
		if readErr != nil {
			return readErr
		}
		local, readErr = service.repository.ReadLocalOverview(tx)
		return readErr
	})
	if err != nil {
		return useropsport.Overview{}, classify(err)
	}
	if !validDirectoryOverview(directory) || !validLocalOverview(local) {
		return useropsport.Overview{}, useropsport.ErrUnavailable
	}
	return useropsport.Overview{
		CustomerCount:           directory.CustomerCount,
		CustomerCountIsEstimate: directory.CustomerCountIsEstimate,
		ActiveDNDCount:          local.ActiveDNDCount,
		DraftPlanCount:          local.DraftPlanCount,
		PendingReviewPlanCount:  local.PendingReviewPlanCount,
		Safety:                  useropsport.LocalSafety(),
	}, nil
}

func (service *Service) ListCustomers(ctx context.Context, input useropsport.DirectoryQuery) (useropsport.DirectoryPage, error) {
	query, err := normalizeDirectoryQuery(input)
	if err != nil || !service.ready(service.directory) || ctx == nil {
		return useropsport.DirectoryPage{}, invalidOrUnavailable(err, service.ready(service.directory), ctx)
	}
	var page useropsport.DirectoryPageRead
	err = service.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		page, readErr = service.directory.ListCustomers(tx, query)
		return readErr
	})
	if err != nil {
		return useropsport.DirectoryPage{}, classify(err)
	}
	if !validDirectoryPage(page, query.Limit) {
		return useropsport.DirectoryPage{}, useropsport.ErrUnavailable
	}
	return useropsport.DirectoryPage{
		Items:           page.Items,
		NextCursor:      page.NextCursor,
		Total:           page.Total,
		TotalIsEstimate: page.TotalIsEstimate,
		Safety:          useropsport.LocalSafety(),
	}, nil
}

func (service *Service) GetCustomerDetail(ctx context.Context, customerID domain.CustomerID) (useropsport.CustomerDetailResult, error) {
	if !customerID.Valid() || !service.ready(service.details, service.repository) || ctx == nil {
		return useropsport.CustomerDetailResult{}, invalidOrUnavailable(nil, service.ready(service.details, service.repository), ctx)
	}
	var detail useropsport.CustomerDetail
	var dnd *domain.DoNotDisturb
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		detail, readErr = service.details.ReadCustomerDetail(tx, customerID)
		if readErr != nil {
			return readErr
		}
		dnd, readErr = service.repository.ReadDND(tx, customerID)
		return readErr
	})
	if err != nil {
		return useropsport.CustomerDetailResult{}, classify(err)
	}
	if !validCustomerDetail(detail, customerID) || dnd != nil && !validDND(*dnd, customerID) {
		return useropsport.CustomerDetailResult{}, useropsport.ErrUnavailable
	}
	return useropsport.CustomerDetailResult{
		Detail: detail,
		DND:    dnd,
		Safety: useropsport.LocalSafety(),
	}, nil
}

func (service *Service) SafeExport(ctx context.Context, input useropsport.SafeExportRequest) (useropsport.SafeExport, error) {
	query, err := normalizeDirectoryQuery(input.Query)
	fields, fieldErr := normalizeExportFields(input.Fields)
	if err != nil || fieldErr != nil || !service.ready(service.directory) || ctx == nil {
		if err == nil {
			err = fieldErr
		}
		return useropsport.SafeExport{}, invalidOrUnavailable(err, service.ready(service.directory), ctx)
	}
	var page useropsport.DirectoryPageRead
	err = service.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		page, readErr = service.directory.ListCustomers(tx, query)
		return readErr
	})
	if err != nil {
		return useropsport.SafeExport{}, classify(err)
	}
	if !validDirectoryPage(page, query.Limit) {
		return useropsport.SafeExport{}, useropsport.ErrUnavailable
	}
	rows := make([][]string, len(page.Items))
	for index, item := range page.Items {
		rows[index] = safeExportRow(item, fields)
	}
	return useropsport.SafeExport{
		Fields:          fields,
		Rows:            rows,
		NextCursor:      page.NextCursor,
		Total:           page.Total,
		TotalIsEstimate: page.TotalIsEstimate,
		Safety:          useropsport.LocalSafety(),
	}, nil
}

func (service *Service) PreviewBatch(ctx context.Context, input useropsport.BatchPreviewInput) (useropsport.BatchPreview, error) {
	ids, err := normalizeCustomerIDs(input.CustomerIDs)
	content, contentErr := normalizeContent(input.Content)
	if err != nil || contentErr != nil || !service.ready(service.directory, service.materials, service.repository) || ctx == nil {
		if err == nil {
			err = contentErr
		}
		return useropsport.BatchPreview{}, invalidOrUnavailable(err, service.ready(service.directory, service.materials, service.repository), ctx)
	}
	var targets []domain.CustomerID
	var excluded int32
	err = service.uow.Within(ctx, func(tx context.Context) error {
		var resolveErr error
		targets, excluded, resolveErr = service.resolveTargets(tx, ids, false)
		if resolveErr != nil {
			return resolveErr
		}
		return service.validateMaterialReferences(tx, content)
	})
	if err != nil {
		return useropsport.BatchPreview{}, classify(err)
	}
	return useropsport.BatchPreview{
		TargetCustomerIDs: append([]domain.CustomerID(nil), targets...),
		ExcludedDNDCount:  excluded,
		TargetDigest:      targetDigest(targets),
		Content:           content,
		Safety:            useropsport.LocalSafety(),
	}, nil
}

func (service *Service) CreateLocalPlan(ctx context.Context, input useropsport.CreateLocalPlanInput) (useropsport.LocalPlanResult, error) {
	ids, err := normalizeCustomerIDs(input.CustomerIDs)
	content, contentErr := normalizeContent(input.Content)
	if err != nil || contentErr != nil || !validCreatePlanInput(input, content) || !service.ready(service.directory, service.materials, service.repository, service.events) || ctx == nil {
		if err == nil {
			err = contentErr
		}
		return useropsport.LocalPlanResult{}, invalidOrUnavailable(err, service.ready(service.directory, service.materials, service.repository, service.events), ctx)
	}
	now, err := service.nowUTC()
	if err != nil {
		return useropsport.LocalPlanResult{}, err
	}
	input.CustomerIDs = append([]domain.CustomerID(nil), ids...)
	input.Content = contentInputFromSnapshot(content)
	if input.ExpectedContentDigest != content.ContentDigest {
		return useropsport.LocalPlanResult{}, useropsport.ErrPreviewStale
	}
	var result domain.LocalPlan
	err = service.uow.Within(ctx, func(tx context.Context) error {
		replay, replayErr := service.repository.ReplayLocalPlan(tx, input, content)
		if replayErr != nil {
			return replayErr
		}
		if replay.Replayed {
			if replay.Plan == nil || !validPlan(*replay.Plan) || replay.Plan.State != input.State || !sameContentSnapshot(replay.Plan.Content, content) {
				return useropsport.ErrUnavailable
			}
			result = *replay.Plan
			return nil
		}
		targets, _, resolveErr := service.resolveTargets(tx, ids, true)
		if resolveErr != nil {
			return resolveErr
		}
		if len(targets) == 0 {
			return useropsport.ErrConflict
		}
		digest := targetDigest(targets)
		if input.ExpectedTargetDigest != digest {
			return useropsport.ErrPreviewStale
		}
		if materialErr := service.validateMaterialReferences(tx, content); materialErr != nil {
			return materialErr
		}
		mutation, mutationErr := service.repository.CreateLocalPlan(tx, input, targets, digest, content)
		if mutationErr != nil {
			return mutationErr
		}
		if mutation.Replayed {
			return useropsport.ErrUnavailable
		}
		if !mutation.PlanID.Valid() {
			return useropsport.ErrUnavailable
		}
		result, mutationErr = service.repository.ReadLocalPlan(tx, mutation.PlanID)
		if mutationErr != nil {
			return mutationErr
		}
		if !validPlan(result) || result.TargetDigest != digest || result.TargetCount != int32(len(targets)) || result.State != input.State || !sameContentSnapshot(result.Content, content) {
			return useropsport.ErrUnavailable
		}
		return service.events.Append(tx, useropsport.LocalEvent{
			Type:           eventLocalPlanCreated,
			ActorID:        input.ActorID,
			PlanID:         result.ID,
			Version:        result.Version,
			TargetCount:    result.TargetCount,
			OccurredAt:     now,
			IdempotencyKey: input.IdempotencyKey,
		})
	})
	if err != nil {
		return useropsport.LocalPlanResult{}, classify(err)
	}
	return useropsport.LocalPlanResult{Plan: result, Safety: useropsport.LocalSafety()}, nil
}

func (service *Service) SetDND(ctx context.Context, input useropsport.UpsertDNDInput) (useropsport.DNDMutationResult, error) {
	input, err := normalizeDNDInput(input)
	if err != nil || !service.ready(service.details, service.repository, service.events) || ctx == nil {
		return useropsport.DNDMutationResult{}, invalidOrUnavailable(err, service.ready(service.details, service.repository, service.events), ctx)
	}
	now, err := service.nowUTC()
	if err != nil {
		return useropsport.DNDMutationResult{}, err
	}
	var result domain.DoNotDisturb
	err = service.uow.Within(ctx, func(tx context.Context) error {
		detail, readErr := service.details.ReadCustomerDetail(tx, input.CustomerID)
		if readErr != nil {
			return readErr
		}
		if !validCustomerDetail(detail, input.CustomerID) {
			return useropsport.ErrUnavailable
		}
		mutation, mutationErr := service.repository.UpsertDND(tx, input)
		if mutationErr != nil {
			return mutationErr
		}
		if mutation.Cleared {
			return useropsport.ErrUnavailable
		}
		if mutation.Replayed {
			if mutation.DND == nil || !validDND(*mutation.DND, input.CustomerID) || mutation.DND.Reason != input.Reason {
				return useropsport.ErrUnavailable
			}
			result = *mutation.DND
			return nil
		}
		stored, storedErr := service.repository.ReadDND(tx, input.CustomerID)
		if storedErr != nil {
			return storedErr
		}
		if stored == nil || !validDND(*stored, input.CustomerID) || stored.Reason != input.Reason {
			return useropsport.ErrUnavailable
		}
		result = *stored
		return service.events.Append(tx, useropsport.LocalEvent{
			Type:           eventDNDSet,
			ActorID:        input.ActorID,
			CustomerID:     result.CustomerID,
			Version:        result.Version,
			OccurredAt:     now,
			IdempotencyKey: input.IdempotencyKey,
		})
	})
	if err != nil {
		return useropsport.DNDMutationResult{}, classify(err)
	}
	return useropsport.DNDMutationResult{DND: &result, Safety: useropsport.LocalSafety()}, nil
}

func (service *Service) ClearDND(ctx context.Context, input useropsport.ClearDNDInput) (useropsport.DNDMutationResult, error) {
	if !validClearDNDInput(input) || !service.ready(service.repository, service.events) || ctx == nil {
		return useropsport.DNDMutationResult{}, invalidOrUnavailable(nil, service.ready(service.repository, service.events), ctx)
	}
	now, err := service.nowUTC()
	if err != nil {
		return useropsport.DNDMutationResult{}, err
	}
	err = service.uow.Within(ctx, func(tx context.Context) error {
		mutation, mutationErr := service.repository.ClearDND(tx, input)
		if mutationErr != nil {
			return mutationErr
		}
		if mutation.Replayed {
			if !mutation.Cleared || mutation.DND != nil {
				return useropsport.ErrUnavailable
			}
			return nil
		}
		if !mutation.Cleared || mutation.DND != nil {
			return useropsport.ErrUnavailable
		}
		stored, storedErr := service.repository.ReadDND(tx, input.CustomerID)
		if storedErr != nil {
			return storedErr
		}
		if stored != nil {
			return useropsport.ErrUnavailable
		}
		return service.events.Append(tx, useropsport.LocalEvent{
			Type:           eventDNDCleared,
			ActorID:        input.ActorID,
			CustomerID:     input.CustomerID,
			Version:        input.ExpectedVersion,
			OccurredAt:     now,
			IdempotencyKey: input.IdempotencyKey,
		})
	})
	if err != nil {
		return useropsport.DNDMutationResult{}, classify(err)
	}
	return useropsport.DNDMutationResult{Cleared: true, Safety: useropsport.LocalSafety()}, nil
}

func (service *Service) ListSendRecords(ctx context.Context, input useropsport.SendRecordQuery) (useropsport.SendRecordPage, error) {
	query, err := normalizeSendRecordQuery(input)
	if err != nil || !service.ready(service.repository) || ctx == nil {
		return useropsport.SendRecordPage{}, invalidOrUnavailable(err, service.ready(service.repository), ctx)
	}
	var plan domain.LocalPlan
	var page useropsport.SendRecordPageRead
	err = service.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		plan, readErr = service.repository.ReadLocalPlan(tx, query.PlanID)
		if readErr != nil {
			return readErr
		}
		if !validPlan(plan) || plan.ID != query.PlanID {
			return useropsport.ErrUnavailable
		}
		page, readErr = service.repository.ListSendRecords(tx, query)
		return readErr
	})
	if err != nil {
		return useropsport.SendRecordPage{}, classify(err)
	}
	if !validSendRecordPage(page, query) {
		return useropsport.SendRecordPage{}, useropsport.ErrUnavailable
	}
	return useropsport.SendRecordPage{
		Items:      page.Items,
		NextCursor: page.NextCursor,
		Total:      page.Total,
		Safety:     useropsport.LocalSafety(),
	}, nil
}

func (service *Service) resolveTargets(ctx context.Context, ids []domain.CustomerID, lockDND bool) ([]domain.CustomerID, int32, error) {
	resolutionMode := useropsport.CustomerResolutionRead
	if lockDND {
		resolutionMode = useropsport.CustomerResolutionForWrite
	}
	resolved, err := service.directory.ResolveCustomers(ctx, ids, resolutionMode)
	if err != nil {
		return nil, 0, err
	}
	if !validResolvedCustomers(resolved, ids) {
		return nil, 0, useropsport.ErrUnavailable
	}
	var dnds []domain.DoNotDisturb
	if lockDND {
		dnds, err = service.repository.LockActiveDND(ctx, ids)
	} else {
		dnds, err = service.repository.ListActiveDND(ctx, ids)
	}
	if err != nil {
		return nil, 0, err
	}
	dndIDs, valid := dndCustomerIDs(dnds, ids)
	if !valid {
		return nil, 0, useropsport.ErrUnavailable
	}
	targets := make([]domain.CustomerID, 0, len(ids)-len(dndIDs))
	for _, id := range ids {
		if _, excluded := dndIDs[id]; !excluded {
			targets = append(targets, id)
		}
	}
	return targets, int32(len(dndIDs)), nil
}

func (service *Service) validateMaterialReferences(ctx context.Context, content domain.ContentSnapshot) error {
	for _, imageID := range content.ImageLibraryIDs {
		eligible, err := service.materials.ImageEligible(ctx, imageID)
		if err != nil {
			return err
		}
		if !eligible {
			return useropsport.ErrConflict
		}
	}
	for _, miniProgramID := range content.MiniProgramLibraryIDs {
		eligible, err := service.materials.MiniProgramEligible(ctx, miniProgramID)
		if err != nil {
			return err
		}
		if !eligible {
			return useropsport.ErrConflict
		}
	}
	for _, attachmentID := range content.AttachmentLibraryIDs {
		eligible, err := service.materials.AttachmentEligible(ctx, attachmentID)
		if err != nil {
			return err
		}
		if !eligible {
			return useropsport.ErrConflict
		}
	}
	return nil
}

func (service *Service) ready(dependencies ...any) bool {
	if service == nil || nilInterface(service.uow) || service.now == nil {
		return false
	}
	for _, dependency := range dependencies {
		if nilInterface(dependency) {
			return false
		}
	}
	return true
}

func (service *Service) nowUTC() (time.Time, error) {
	if service == nil || service.now == nil {
		return time.Time{}, useropsport.ErrUnavailable
	}
	now := service.now().UTC()
	if now.IsZero() {
		return time.Time{}, useropsport.ErrUnavailable
	}
	return now, nil
}

func normalizeDirectoryQuery(input useropsport.DirectoryQuery) (useropsport.DirectoryQuery, error) {
	limit := input.Limit
	if limit == 0 {
		limit = useropsport.DefaultPageLimit
	}
	keyword := strings.TrimSpace(input.Keyword)
	if limit < 1 || limit > useropsport.MaximumPageLimit || len(input.Cursor) > maximumCursorBytes ||
		!utf8.ValidString(keyword) || utf8.RuneCountInString(keyword) > maximumKeywordRunes ||
		!validOpaqueCursor(input.Cursor) || !validOptionalID(input.OwnerStaffID) || !validOptionalID(input.StageID) || !validOptionalID(input.ChannelID) || !validOptionalID(input.TagID) || !validOpaquePhone(input.PhoneExact) {
		return useropsport.DirectoryQuery{}, useropsport.ErrInvalid
	}
	return useropsport.DirectoryQuery{
		Keyword:      keyword,
		OwnerStaffID: cloneInt64(input.OwnerStaffID),
		StageID:      cloneInt64(input.StageID),
		ChannelID:    cloneInt64(input.ChannelID),
		TagID:        cloneInt64(input.TagID),
		PhoneExact:   input.PhoneExact,
		Cursor:       input.Cursor,
		Limit:        limit,
	}, nil
}

func normalizeExportFields(fields []useropsport.SafeExportField) ([]useropsport.SafeExportField, error) {
	if len(fields) == 0 || len(fields) > 7 {
		return nil, useropsport.ErrInvalid
	}
	seen := make(map[useropsport.SafeExportField]struct{}, len(fields))
	result := make([]useropsport.SafeExportField, len(fields))
	for index, field := range fields {
		if !field.Valid() {
			return nil, useropsport.ErrInvalid
		}
		if _, duplicate := seen[field]; duplicate {
			return nil, useropsport.ErrInvalid
		}
		seen[field] = struct{}{}
		result[index] = field
	}
	return result, nil
}

func normalizeCustomerIDs(ids []domain.CustomerID) ([]domain.CustomerID, error) {
	if len(ids) == 0 || len(ids) > useropsport.MaximumBatchSize {
		return nil, useropsport.ErrInvalid
	}
	result := append([]domain.CustomerID(nil), ids...)
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	for index, id := range result {
		if !id.Valid() || index > 0 && result[index-1] == id {
			return nil, useropsport.ErrInvalid
		}
	}
	return result, nil
}

func normalizeDNDInput(input useropsport.UpsertDNDInput) (useropsport.UpsertDNDInput, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if !input.CustomerID.Valid() || !validReason(input.Reason) || !validActorAndKey(input.ActorID, input.IdempotencyKey) ||
		input.ExpectedVersion != nil && *input.ExpectedVersion < 1 {
		return useropsport.UpsertDNDInput{}, useropsport.ErrInvalid
	}
	input.ExpectedVersion = cloneInt64(input.ExpectedVersion)
	return input, nil
}

func validClearDNDInput(input useropsport.ClearDNDInput) bool {
	return input.CustomerID.Valid() && input.ExpectedVersion > 0 && validActorAndKey(input.ActorID, input.IdempotencyKey)
}

func validCreatePlanInput(input useropsport.CreateLocalPlanInput, content domain.ContentSnapshot) bool {
	return input.State.Valid() && validDigest(input.ExpectedTargetDigest) && validDigest(input.ExpectedContentDigest) &&
		validContentSnapshot(content) && (input.State != domain.LocalPlanPendingReview || contentHasMaterial(content)) &&
		validActorAndKey(input.ActorID, input.IdempotencyKey)
}

func normalizeSendRecordQuery(input useropsport.SendRecordQuery) (useropsport.SendRecordQuery, error) {
	limit := input.Limit
	if limit == 0 {
		limit = useropsport.DefaultPageLimit
	}
	if !input.PlanID.Valid() || limit < 1 || limit > useropsport.MaximumPageLimit || !validOpaqueCursor(input.Cursor) {
		return useropsport.SendRecordQuery{}, useropsport.ErrInvalid
	}
	return useropsport.SendRecordQuery{PlanID: input.PlanID, Cursor: input.Cursor, Limit: limit}, nil
}

func validActorAndKey(actorID int64, key string) bool {
	if actorID < 1 || len(key) < minimumKeyBytes || len(key) > maximumKeyBytes || strings.TrimSpace(key) != key {
		return false
	}
	for _, character := range []byte(key) {
		if character < 0x21 || character > 0x7e || character == ',' {
			return false
		}
	}
	return true
}

func validOpaquePhone(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > maximumPhoneBytes || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validOpaqueCursor(value string) bool {
	return len(value) <= maximumCursorBytes && utf8.ValidString(value) && !containsControl(value)
}

func validReason(value string) bool {
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maximumReasonRunes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func normalizeContent(input domain.ContentInput) (domain.ContentSnapshot, error) {
	text := strings.TrimSpace(input.Text)
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > maximumContentRunes || containsControl(text) {
		return domain.ContentSnapshot{}, useropsport.ErrInvalid
	}
	images, err := normalizeMaterialIDs(input.ImageLibraryIDs, maximumImages)
	if err != nil {
		return domain.ContentSnapshot{}, err
	}
	miniPrograms, err := normalizeMaterialIDs(input.MiniProgramLibraryIDs, maximumMiniPrograms)
	if err != nil {
		return domain.ContentSnapshot{}, err
	}
	attachments, err := normalizeMaterialIDs(input.AttachmentLibraryIDs, maximumAttachments)
	if err != nil {
		return domain.ContentSnapshot{}, err
	}
	if len(images)+len(miniPrograms)+len(attachments) > maximumMaterials {
		return domain.ContentSnapshot{}, useropsport.ErrInvalid
	}
	content := domain.ContentSnapshot{
		Text:                  text,
		ImageLibraryIDs:       images,
		MiniProgramLibraryIDs: miniPrograms,
		AttachmentLibraryIDs:  attachments,
	}
	content.ContentDigest = contentDigest(content)
	return content, nil
}

func normalizeMaterialIDs(values []int64, maximum int) ([]int64, error) {
	if len(values) > maximum {
		return nil, useropsport.ErrInvalid
	}
	result := append([]int64(nil), values...)
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	for index, value := range result {
		if value < 1 || index > 0 && result[index-1] == value {
			return nil, useropsport.ErrInvalid
		}
	}
	return result, nil
}

func contentDigest(content domain.ContentSnapshot) string {
	canonical, err := json.Marshal(struct {
		Text                  string  `json:"text"`
		ImageLibraryIDs       []int64 `json:"image_library_ids"`
		MiniProgramLibraryIDs []int64 `json:"miniprogram_library_ids"`
		AttachmentLibraryIDs  []int64 `json:"attachment_library_ids"`
	}{
		Text:                  content.Text,
		ImageLibraryIDs:       content.ImageLibraryIDs,
		MiniProgramLibraryIDs: content.MiniProgramLibraryIDs,
		AttachmentLibraryIDs:  content.AttachmentLibraryIDs,
	})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(append([]byte("user_ops_content:v1:"), canonical...))
	return hex.EncodeToString(digest[:])
}

func contentInputFromSnapshot(content domain.ContentSnapshot) domain.ContentInput {
	return domain.ContentInput{
		Text:                  content.Text,
		ImageLibraryIDs:       append([]int64(nil), content.ImageLibraryIDs...),
		MiniProgramLibraryIDs: append([]int64(nil), content.MiniProgramLibraryIDs...),
		AttachmentLibraryIDs:  append([]int64(nil), content.AttachmentLibraryIDs...),
	}
}

func contentHasMaterial(content domain.ContentSnapshot) bool {
	return content.Text != "" || len(content.ImageLibraryIDs) > 0 || len(content.MiniProgramLibraryIDs) > 0 || len(content.AttachmentLibraryIDs) > 0
}

func validContentSnapshot(content domain.ContentSnapshot) bool {
	normalized, err := normalizeContent(contentInputFromSnapshot(content))
	return err == nil && content.ContentDigest != "" && sameContentSnapshot(content, normalized)
}

func sameContentSnapshot(left, right domain.ContentSnapshot) bool {
	return left.Text == right.Text && left.ContentDigest == right.ContentDigest &&
		reflect.DeepEqual(left.ImageLibraryIDs, right.ImageLibraryIDs) &&
		reflect.DeepEqual(left.MiniProgramLibraryIDs, right.MiniProgramLibraryIDs) &&
		reflect.DeepEqual(left.AttachmentLibraryIDs, right.AttachmentLibraryIDs)
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func validOptionalID(value *int64) bool { return value == nil || *value > 0 }

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func targetDigest(ids []domain.CustomerID) string {
	var builder strings.Builder
	builder.WriteString("user_ops_targets:v1:")
	for index, id := range ids {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatInt(int64(id), 10))
	}
	digest := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(digest[:])
}

func validDirectoryOverview(value useropsport.DirectoryOverviewRead) bool {
	return value.CustomerCount >= 0
}

func validLocalOverview(value useropsport.LocalOverviewRead) bool {
	return value.ActiveDNDCount >= 0 && value.DraftPlanCount >= 0 && value.PendingReviewPlanCount >= 0
}

func validDirectoryPage(value useropsport.DirectoryPageRead, limit int32) bool {
	if value.Total < int64(len(value.Items)) || len(value.Items) > int(limit) || value.NextCursor != nil && *value.NextCursor == "" {
		return false
	}
	seen := make(map[domain.CustomerID]struct{}, len(value.Items))
	for _, item := range value.Items {
		if !validCustomerSummary(item) {
			return false
		}
		if _, duplicate := seen[item.CustomerID]; duplicate {
			return false
		}
		seen[item.CustomerID] = struct{}{}
	}
	return true
}

func validCustomerSummary(value useropsport.CustomerSummary) bool {
	return value.CustomerID.Valid() && utf8.ValidString(value.Name) && validOptionalID(value.OwnerStaffID) &&
		validOptionalID(value.StageID) && validOptionalID(value.ChannelID) && validOptionalTime(value.AddedAt) && validOptionalTime(value.LastInteractAt)
}

func validCustomerDetail(value useropsport.CustomerDetail, customerID domain.CustomerID) bool {
	if !validCustomerSummary(value.Customer) || value.Customer.CustomerID != customerID || value.Tags == nil || value.Timeline == nil {
		return false
	}
	seenTags := make(map[int64]struct{}, len(value.Tags))
	for _, tag := range value.Tags {
		if tag.ID < 1 || !validOptionalID(tag.GroupID) || !utf8.ValidString(tag.Name) || containsControl(tag.Name) || tag.GroupName != nil && (!utf8.ValidString(*tag.GroupName) || containsControl(*tag.GroupName)) {
			return false
		}
		if _, duplicate := seenTags[tag.ID]; duplicate {
			return false
		}
		seenTags[tag.ID] = struct{}{}
	}
	for _, entry := range value.Timeline {
		if entry.EventType == "" || !utf8.ValidString(entry.EventType) || containsControl(entry.EventType) || entry.OccurredAt.IsZero() {
			return false
		}
	}
	return true
}

func validDND(value domain.DoNotDisturb, customerID domain.CustomerID) bool {
	return value.CustomerID == customerID && value.CustomerID.Valid() && validReason(value.Reason) && value.Version > 0 &&
		!value.CreatedAt.IsZero() && !value.UpdatedAt.IsZero() && !value.UpdatedAt.Before(value.CreatedAt)
}

func validPlan(value domain.LocalPlan) bool {
	return value.ID.Valid() && value.State.Valid() && validContentSnapshot(value.Content) &&
		(value.State != domain.LocalPlanPendingReview || contentHasMaterial(value.Content)) && validDigest(value.TargetDigest) && value.TargetCount > 0 && value.Version > 0 &&
		!value.CreatedAt.IsZero() && !value.UpdatedAt.IsZero() && !value.UpdatedAt.Before(value.CreatedAt)
}

func validSendRecordPage(value useropsport.SendRecordPageRead, query useropsport.SendRecordQuery) bool {
	if value.Total < int64(len(value.Items)) || len(value.Items) > int(query.Limit) || value.NextCursor != nil && *value.NextCursor == "" {
		return false
	}
	seen := make(map[domain.SendRecordID]struct{}, len(value.Items))
	for _, item := range value.Items {
		if !item.ID.Valid() || item.PlanID != query.PlanID || !item.CustomerID.Valid() || !item.TechnicalStatus.Valid() || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() || item.UpdatedAt.Before(item.CreatedAt) {
			return false
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return false
		}
		seen[item.ID] = struct{}{}
	}
	return true
}

func validResolvedCustomers(values []useropsport.CustomerSummary, ids []domain.CustomerID) bool {
	if len(values) != len(ids) {
		return false
	}
	seen := make(map[domain.CustomerID]struct{}, len(values))
	for _, value := range values {
		if !validCustomerSummary(value) {
			return false
		}
		if _, duplicate := seen[value.CustomerID]; duplicate {
			return false
		}
		seen[value.CustomerID] = struct{}{}
	}
	for _, id := range ids {
		if _, exists := seen[id]; !exists {
			return false
		}
	}
	return true
}

func dndCustomerIDs(values []domain.DoNotDisturb, ids []domain.CustomerID) (map[domain.CustomerID]struct{}, bool) {
	allowed := make(map[domain.CustomerID]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	result := make(map[domain.CustomerID]struct{}, len(values))
	for _, value := range values {
		if _, exists := allowed[value.CustomerID]; !exists || !validDND(value, value.CustomerID) {
			return nil, false
		}
		if _, duplicate := result[value.CustomerID]; duplicate {
			return nil, false
		}
		result[value.CustomerID] = struct{}{}
	}
	return result, true
}

func safeExportRow(value useropsport.CustomerSummary, fields []useropsport.SafeExportField) []string {
	row := make([]string, len(fields))
	for index, field := range fields {
		switch field {
		case useropsport.SafeExportCustomerID:
			row[index] = strconv.FormatInt(int64(value.CustomerID), 10)
		case useropsport.SafeExportName:
			row[index] = safeExportCell(value.Name)
		case useropsport.SafeExportOwnerStaffID:
			row[index] = safeOptionalInt(value.OwnerStaffID)
		case useropsport.SafeExportStageID:
			row[index] = safeOptionalInt(value.StageID)
		case useropsport.SafeExportChannelID:
			row[index] = safeOptionalInt(value.ChannelID)
		case useropsport.SafeExportAddedAt:
			row[index] = safeOptionalTime(value.AddedAt)
		case useropsport.SafeExportLastInteractAt:
			row[index] = safeOptionalTime(value.LastInteractAt)
		}
	}
	return row
}

func safeExportCell(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if trimmed != "" {
		switch trimmed[0] {
		case '=', '+', '-', '@':
			return "'" + value
		}
	}
	return value
}

func safeOptionalInt(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

func safeOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func validOptionalTime(value *time.Time) bool { return value == nil || !value.IsZero() }

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func invalidOrUnavailable(validation error, ready bool, ctx context.Context) error {
	if validation != nil || !ready {
		if validation != nil && errors.Is(validation, useropsport.ErrInvalid) {
			return useropsport.ErrInvalid
		}
		if validation != nil {
			return validation
		}
		return useropsport.ErrUnavailable
	}
	if ctx == nil || ctx.Err() != nil {
		return useropsport.ErrUnavailable
	}
	return useropsport.ErrInvalid
}

func classify(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, useropsport.ErrInvalid):
		return useropsport.ErrInvalid
	case errors.Is(err, useropsport.ErrNotFound):
		return useropsport.ErrNotFound
	case errors.Is(err, useropsport.ErrPreviewStale):
		return useropsport.ErrPreviewStale
	case errors.Is(err, useropsport.ErrConflict):
		return useropsport.ErrConflict
	case errors.Is(err, useropsport.ErrUnavailable):
		return useropsport.ErrUnavailable
	default:
		return errors.Join(useropsport.ErrUnavailable, err)
	}
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
