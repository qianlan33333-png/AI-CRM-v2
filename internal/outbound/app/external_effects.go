package app

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	ExternalEffectsDefaultLimit int32 = 50
	ExternalEffectsMaximumLimit int32 = 100

	ExternalEffectJobIDPrefix   = "eej_v1_"
	ExternalEffectsCursorPrefix = "eec_v1_"

	ExternalEffectsDeliverySemantics = "local_state_not_delivery_proof"
)

var (
	ErrInvalidExternalEffectsQuery         = errors.New("invalid external effects query")
	ErrInvalidExternalEffectsCursor        = errors.New("invalid external effects cursor")
	ErrExternalEffectsUnavailable          = errors.New("external effects read unavailable")
	ErrInvalidExternalEffectsConfiguration = errors.New("invalid external effects configuration")
)

type ExternalEffectHandling string

const (
	ExternalEffectSafeLocalHandling ExternalEffectHandling = "safe_local_handling"
	ExternalEffectFrozen            ExternalEffectHandling = "frozen"
	ExternalEffectManualReview      ExternalEffectHandling = "manual_review"
)

type ExternalEffectRiskLevel string

const (
	ExternalEffectRiskNone                  ExternalEffectRiskLevel = "none"
	ExternalEffectRiskManualReviewRequired  ExternalEffectRiskLevel = "manual_review_required"
	ExternalEffectRiskOutcomeUnknownPresent ExternalEffectRiskLevel = "outcome_unknown_present"
)

// ExternalEffectSource is the only storage projection consumed by this read
// package. It deliberately excludes recipients, payloads, provider identifiers,
// failure text, receipts, and queue-control data.
type ExternalEffectSource struct {
	TaskID          TaskID
	Status          TaskStatus
	AttemptCount    int32
	CreatedAt       time.Time
	StatusUpdatedAt time.Time
}

// ExternalEffectStoreQuery maps only to the existing closed Outbound task-list
// port. Offset is never returned directly; the service seals it inside an
// authenticated cursor bound to the complete filter set.
type ExternalEffectStoreQuery struct {
	Status TaskStatus
	Offset int32
	Limit  int32
}

type ExternalEffectStatusCounts struct {
	Pending         int64
	Sending         int64
	Sent            int64
	RetryableFailed int64
	FinalFailed     int64
	OutcomeUnknown  int64
	Cancelled       int64
}

func (counts ExternalEffectStatusCounts) Total() int64 {
	total, ok := checkedExternalEffectCountSum(
		counts.Pending, counts.Sending, counts.Sent, counts.RetryableFailed,
		counts.FinalFailed, counts.OutcomeUnknown, counts.Cancelled,
	)
	if !ok {
		return -1
	}
	return total
}

type ExternalEffectsReadStore interface {
	ListExternalEffectSources(context.Context, ExternalEffectStoreQuery) ([]ExternalEffectSource, error)
	CountExternalEffectStatuses(context.Context) (ExternalEffectStatusCounts, error)
}

type ExternalEffectJobQuery struct {
	Cursor   string
	Status   TaskStatus
	Handling ExternalEffectHandling
	Limit    int32
}

type ExternalEffectAppliedFilters struct {
	Status   TaskStatus
	Handling ExternalEffectHandling
}

type ExternalEffectJob struct {
	ID              string
	Status          TaskStatus
	Handling        ExternalEffectHandling
	AttemptCount    int32
	CreatedAt       time.Time
	StatusUpdatedAt time.Time
}

type ExternalEffectJobPage struct {
	Items                     []ExternalEffectJob
	NextCursor                *string
	PageSize                  int32
	AppliedFilters            ExternalEffectAppliedFilters
	ProviderExecutionEligible bool
	RealExternalCallExecuted  bool
	DeliveryProven            bool
	LocalFactOnly             bool
	DeliverySemantics         string
}

type ExternalEffectClassificationCounts struct {
	SafeLocalHandling int64
	Frozen            int64
	ManualReview      int64
}

type ExternalEffectRiskSummary struct {
	Level                ExternalEffectRiskLevel
	OutcomeUnknownCount  int64
	ManualReviewCount    int64
	ManualReviewRequired bool
}

type ExternalEffectsDiagnostics struct {
	Total                     int64
	ByStatus                  ExternalEffectStatusCounts
	ByClassification          ExternalEffectClassificationCounts
	Risk                      ExternalEffectRiskSummary
	GeneratedAt               time.Time
	ProviderExecutionEligible bool
	RealExternalCallExecuted  bool
	DeliveryProven            bool
	LocalFactOnly             bool
	DeliverySemantics         string
}

type externalEffectsCursorOffset struct {
	Status TaskStatus `json:"status"`
	Offset int32      `json:"offset"`
}

type externalEffectsCursorPayload struct {
	Version  int                           `json:"version"`
	Offsets  []externalEffectsCursorOffset `json:"offsets"`
	Status   TaskStatus                    `json:"status"`
	Handling ExternalEffectHandling        `json:"handling"`
	Limit    int32                         `json:"limit"`
}

type ExternalEffectsService struct {
	uow        platformport.UnitOfWork
	store      ExternalEffectsReadStore
	idKey      [sha256.Size]byte
	cursorAEAD cipher.AEAD
	entropy    io.Reader
	clock      func() time.Time
}

func NewExternalEffectsService(
	uow platformport.UnitOfWork,
	store ExternalEffectsReadStore,
	secret []byte,
) (*ExternalEffectsService, error) {
	if nilExternalEffectsDependency(uow) || nilExternalEffectsDependency(store) || len(secret) < 32 {
		return nil, ErrInvalidExternalEffectsConfiguration
	}
	idKey := deriveExternalEffectsKey(secret, "job-id")
	cursorKey := deriveExternalEffectsKey(secret, "cursor-aead")
	block, err := aes.NewCipher(cursorKey[:])
	if err != nil {
		return nil, errors.Join(ErrInvalidExternalEffectsConfiguration, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Join(ErrInvalidExternalEffectsConfiguration, err)
	}
	return &ExternalEffectsService{
		uow: uow, store: store, idKey: idKey, cursorAEAD: aead, entropy: rand.Reader, clock: time.Now,
	}, nil
}

func (service *ExternalEffectsService) ListJobs(
	ctx context.Context,
	query ExternalEffectJobQuery,
) (ExternalEffectJobPage, error) {
	query, streams, offsets, err := service.normalizeJobQuery(ctx, query)
	if err != nil {
		return ExternalEffectJobPage{}, err
	}

	merged := make([]ExternalEffectSource, 0, len(streams)*int(query.Limit))
	streamSources := make(map[TaskStatus][]ExternalEffectSource, len(streams))
	moreAfterFullPage := false
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		for _, stream := range streams {
			storeQuery := ExternalEffectStoreQuery{
				Status: stream, Offset: offsets[stream], Limit: query.Limit,
			}
			sources, storeErr := service.store.ListExternalEffectSources(txCtx, storeQuery)
			if storeErr != nil {
				return storeErr
			}
			if validateErr := validateExternalEffectSources(sources, storeQuery); validateErr != nil {
				return validateErr
			}
			streamSources[stream] = sources
			merged = append(merged, sources...)
		}

		// The existing closed TaskQueryStore contract caps reads at 100. When
		// the merged page is exactly full, probe only streams that returned a
		// full read rather than requesting limit+1 and violating that contract.
		if len(merged) == int(query.Limit) {
			for _, stream := range streams {
				sources := streamSources[stream]
				if len(sources) != int(query.Limit) {
					continue
				}
				if offsets[stream] > math.MaxInt32-int32(len(sources)) {
					return errors.New("external effects probe offset overflow")
				}
				probeQuery := ExternalEffectStoreQuery{
					Status: stream, Offset: offsets[stream] + int32(len(sources)), Limit: 1,
				}
				probe, storeErr := service.store.ListExternalEffectSources(txCtx, probeQuery)
				if storeErr != nil {
					return storeErr
				}
				if validateErr := validateExternalEffectSources(probe, probeQuery); validateErr != nil {
					return validateErr
				}
				if len(probe) > 0 {
					if probe[0].TaskID >= sources[len(sources)-1].TaskID {
						return errors.New("external effects probe did not advance")
					}
					moreAfterFullPage = true
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		return ExternalEffectJobPage{}, errors.Join(ErrExternalEffectsUnavailable, err)
	}

	sort.Slice(merged, func(left, right int) bool { return merged[left].TaskID > merged[right].TaskID })
	seenTaskIDs := make(map[TaskID]struct{}, len(merged))
	for index, source := range merged {
		if _, exists := seenTaskIDs[source.TaskID]; exists {
			return ExternalEffectJobPage{}, errors.Join(
				ErrExternalEffectsUnavailable,
				fmt.Errorf("duplicate external effect source at merged index %d", index),
			)
		}
		seenTaskIDs[source.TaskID] = struct{}{}
	}

	hasMore := len(merged) > int(query.Limit) || moreAfterFullPage
	pageSources := merged
	if len(pageSources) > int(query.Limit) {
		pageSources = merged[:query.Limit]
	}
	items := make([]ExternalEffectJob, len(pageSources))
	for index, source := range pageSources {
		items[index] = ExternalEffectJob{
			ID: service.externalEffectJobID(source.TaskID), Status: source.Status,
			Handling: ExternalEffectHandlingForStatus(source.Status), AttemptCount: source.AttemptCount,
			CreatedAt: source.CreatedAt.UTC(), StatusUpdatedAt: source.StatusUpdatedAt.UTC(),
		}
	}

	var nextCursor *string
	if hasMore {
		nextOffsets := cloneExternalEffectsOffsets(offsets)
		for _, source := range pageSources {
			stream := externalEffectStreamForSource(query, source.Status)
			if nextOffsets[stream] == math.MaxInt32 {
				return ExternalEffectJobPage{}, errors.Join(
					ErrExternalEffectsUnavailable,
					errors.New("external effects cursor offset overflow"),
				)
			}
			nextOffsets[stream]++
		}
		encoded, encodeErr := service.encodeCursor(externalEffectsCursorPayload{
			Version: 1, Offsets: externalEffectsCursorOffsets(streams, nextOffsets),
			Status: query.Status, Handling: query.Handling, Limit: query.Limit,
		})
		if encodeErr != nil {
			return ExternalEffectJobPage{}, errors.Join(ErrExternalEffectsUnavailable, encodeErr)
		}
		nextCursor = &encoded
	}
	return ExternalEffectJobPage{
		Items: items, NextCursor: nextCursor, PageSize: query.Limit,
		AppliedFilters:            ExternalEffectAppliedFilters{Status: query.Status, Handling: query.Handling},
		ProviderExecutionEligible: false, RealExternalCallExecuted: false, DeliveryProven: false,
		LocalFactOnly: true, DeliverySemantics: ExternalEffectsDeliverySemantics,
	}, nil
}

func (service *ExternalEffectsService) Diagnostics(ctx context.Context) (ExternalEffectsDiagnostics, error) {
	if ctx == nil || ctx.Err() != nil || service == nil || nilExternalEffectsDependency(service.uow) ||
		nilExternalEffectsDependency(service.store) || service.clock == nil {
		return ExternalEffectsDiagnostics{}, ErrInvalidExternalEffectsQuery
	}
	var counts ExternalEffectStatusCounts
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		counts, storeErr = service.store.CountExternalEffectStatuses(txCtx)
		return storeErr
	})
	if err != nil {
		return ExternalEffectsDiagnostics{}, errors.Join(ErrExternalEffectsUnavailable, err)
	}
	total, countsOK := checkedExternalEffectCountSum(
		counts.Pending, counts.Sending, counts.Sent, counts.RetryableFailed,
		counts.FinalFailed, counts.OutcomeUnknown, counts.Cancelled,
	)
	safeLocalHandling, safeOK := checkedExternalEffectCountSum(counts.Pending, counts.RetryableFailed)
	frozen, frozenOK := checkedExternalEffectCountSum(counts.Sending, counts.Sent, counts.Cancelled)
	manualReview, reviewOK := checkedExternalEffectCountSum(counts.FinalFailed, counts.OutcomeUnknown)
	if !validExternalEffectStatusCounts(counts) || !countsOK || !safeOK || !frozenOK || !reviewOK {
		return ExternalEffectsDiagnostics{}, errors.Join(ErrExternalEffectsUnavailable, errors.New("invalid external effects status counts"))
	}
	classification := ExternalEffectClassificationCounts{
		SafeLocalHandling: safeLocalHandling,
		Frozen:            frozen,
		ManualReview:      manualReview,
	}
	risk := ExternalEffectRiskSummary{
		Level: ExternalEffectRiskNone, OutcomeUnknownCount: counts.OutcomeUnknown,
		ManualReviewCount: classification.ManualReview, ManualReviewRequired: classification.ManualReview > 0,
	}
	if counts.OutcomeUnknown > 0 {
		risk.Level = ExternalEffectRiskOutcomeUnknownPresent
	} else if classification.ManualReview > 0 {
		risk.Level = ExternalEffectRiskManualReviewRequired
	}
	generatedAt := service.clock().UTC()
	if generatedAt.IsZero() {
		return ExternalEffectsDiagnostics{}, errors.Join(ErrExternalEffectsUnavailable, errors.New("invalid external effects diagnostic time"))
	}
	return ExternalEffectsDiagnostics{
		Total: total, ByStatus: counts, ByClassification: classification, Risk: risk,
		GeneratedAt: generatedAt, ProviderExecutionEligible: false, RealExternalCallExecuted: false,
		DeliveryProven: false, LocalFactOnly: true, DeliverySemantics: ExternalEffectsDeliverySemantics,
	}, nil
}

func (service *ExternalEffectsService) normalizeJobQuery(
	ctx context.Context,
	query ExternalEffectJobQuery,
) (ExternalEffectJobQuery, []TaskStatus, map[TaskStatus]int32, error) {
	if ctx == nil || ctx.Err() != nil || service == nil || nilExternalEffectsDependency(service.uow) ||
		nilExternalEffectsDependency(service.store) || service.cursorAEAD == nil || service.entropy == nil {
		return ExternalEffectJobQuery{}, nil, nil, ErrInvalidExternalEffectsQuery
	}
	if query.Limit == 0 {
		query.Limit = ExternalEffectsDefaultLimit
	}
	if query.Limit < 1 || query.Limit > ExternalEffectsMaximumLimit ||
		(query.Status != "" && !ExternalEffectStatusKnown(query.Status)) ||
		(query.Handling != "" && !ExternalEffectHandlingKnown(query.Handling)) ||
		(query.Status != "" && query.Handling != "" && ExternalEffectHandlingForStatus(query.Status) != query.Handling) {
		return ExternalEffectJobQuery{}, nil, nil, ErrInvalidExternalEffectsQuery
	}
	streams := externalEffectQueryStreams(query)
	offsets := make(map[TaskStatus]int32, len(streams))
	for _, stream := range streams {
		offsets[stream] = 0
	}
	if query.Cursor == "" {
		return query, streams, offsets, nil
	}
	payload, err := service.decodeCursor(query.Cursor)
	if err != nil || payload.Status != query.Status || payload.Handling != query.Handling || payload.Limit != query.Limit ||
		len(payload.Offsets) != len(streams) {
		return ExternalEffectJobQuery{}, nil, nil, ErrInvalidExternalEffectsCursor
	}
	for index, stream := range streams {
		if payload.Offsets[index].Status != stream || payload.Offsets[index].Offset < 0 {
			return ExternalEffectJobQuery{}, nil, nil, ErrInvalidExternalEffectsCursor
		}
		offsets[stream] = payload.Offsets[index].Offset
	}
	return query, streams, offsets, nil
}

func externalEffectQueryStreams(query ExternalEffectJobQuery) []TaskStatus {
	if query.Status != "" {
		return []TaskStatus{query.Status}
	}
	switch query.Handling {
	case ExternalEffectSafeLocalHandling:
		return []TaskStatus{TaskStatusPending, TaskStatusRetryableFailed}
	case ExternalEffectFrozen:
		return []TaskStatus{TaskStatusSending, TaskStatusSent, TaskStatusCancelled}
	case ExternalEffectManualReview:
		return []TaskStatus{TaskStatusFinalFailed, TaskStatusOutcomeUnknown}
	default:
		return []TaskStatus{""}
	}
}

func externalEffectStreamForSource(query ExternalEffectJobQuery, status TaskStatus) TaskStatus {
	if query.Status == "" && query.Handling == "" {
		return ""
	}
	return status
}

func externalEffectsCursorOffsets(streams []TaskStatus, offsets map[TaskStatus]int32) []externalEffectsCursorOffset {
	result := make([]externalEffectsCursorOffset, len(streams))
	for index, stream := range streams {
		result[index] = externalEffectsCursorOffset{Status: stream, Offset: offsets[stream]}
	}
	return result
}

func cloneExternalEffectsOffsets(source map[TaskStatus]int32) map[TaskStatus]int32 {
	result := make(map[TaskStatus]int32, len(source))
	for status, offset := range source {
		result[status] = offset
	}
	return result
}

func (service *ExternalEffectsService) externalEffectJobID(taskID TaskID) string {
	mac := hmac.New(sha256.New, service.idKey[:])
	_, _ = mac.Write([]byte("task:" + strconv.FormatInt(int64(taskID), 10)))
	digest := mac.Sum(nil)
	return ExternalEffectJobIDPrefix + base64.RawURLEncoding.EncodeToString(digest[:16])
}

func (service *ExternalEffectsService) encodeCursor(payload externalEffectsCursorPayload) (string, error) {
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, service.cursorAEAD.NonceSize())
	if _, err = io.ReadFull(service.entropy, nonce); err != nil {
		return "", err
	}
	sealed := service.cursorAEAD.Seal(nil, nonce, plaintext, []byte(ExternalEffectsCursorPrefix))
	encoded := append(nonce, sealed...)
	return ExternalEffectsCursorPrefix + base64.RawURLEncoding.EncodeToString(encoded), nil
}

func (service *ExternalEffectsService) decodeCursor(cursor string) (externalEffectsCursorPayload, error) {
	if len(cursor) < len(ExternalEffectsCursorPrefix)+24 || len(cursor) > 1024 ||
		!strings.HasPrefix(cursor, ExternalEffectsCursorPrefix) {
		return externalEffectsCursorPayload{}, ErrInvalidExternalEffectsCursor
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(cursor, ExternalEffectsCursorPrefix))
	if err != nil || len(raw) <= service.cursorAEAD.NonceSize() {
		return externalEffectsCursorPayload{}, ErrInvalidExternalEffectsCursor
	}
	nonce, ciphertext := raw[:service.cursorAEAD.NonceSize()], raw[service.cursorAEAD.NonceSize():]
	plaintext, err := service.cursorAEAD.Open(nil, nonce, ciphertext, []byte(ExternalEffectsCursorPrefix))
	if err != nil {
		return externalEffectsCursorPayload{}, ErrInvalidExternalEffectsCursor
	}
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	var payload externalEffectsCursorPayload
	if err = decoder.Decode(&payload); err != nil {
		return externalEffectsCursorPayload{}, ErrInvalidExternalEffectsCursor
	}
	var trailing json.RawMessage
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return externalEffectsCursorPayload{}, ErrInvalidExternalEffectsCursor
	}
	if payload.Version != 1 || payload.Limit < 1 || payload.Limit > ExternalEffectsMaximumLimit ||
		len(payload.Offsets) < 1 || len(payload.Offsets) > 3 ||
		(payload.Status != "" && !ExternalEffectStatusKnown(payload.Status)) ||
		(payload.Handling != "" && !ExternalEffectHandlingKnown(payload.Handling)) ||
		(payload.Status != "" && payload.Handling != "" && ExternalEffectHandlingForStatus(payload.Status) != payload.Handling) {
		return externalEffectsCursorPayload{}, ErrInvalidExternalEffectsCursor
	}
	seen := make(map[TaskStatus]struct{}, len(payload.Offsets))
	for _, offset := range payload.Offsets {
		if offset.Offset < 0 || (offset.Status != "" && !ExternalEffectStatusKnown(offset.Status)) {
			return externalEffectsCursorPayload{}, ErrInvalidExternalEffectsCursor
		}
		if _, exists := seen[offset.Status]; exists {
			return externalEffectsCursorPayload{}, ErrInvalidExternalEffectsCursor
		}
		seen[offset.Status] = struct{}{}
	}
	return payload, nil
}

func ExternalEffectStatusKnown(status TaskStatus) bool { return validTaskStatus(status) }

func ExternalEffectHandlingKnown(handling ExternalEffectHandling) bool {
	switch handling {
	case ExternalEffectSafeLocalHandling, ExternalEffectFrozen, ExternalEffectManualReview:
		return true
	default:
		return false
	}
}

func ExternalEffectHandlingForStatus(status TaskStatus) ExternalEffectHandling {
	switch status {
	case TaskStatusPending, TaskStatusRetryableFailed:
		return ExternalEffectSafeLocalHandling
	case TaskStatusSending, TaskStatusSent, TaskStatusCancelled:
		return ExternalEffectFrozen
	case TaskStatusFinalFailed, TaskStatusOutcomeUnknown:
		return ExternalEffectManualReview
	default:
		return ""
	}
}

func validateExternalEffectSources(sources []ExternalEffectSource, query ExternalEffectStoreQuery) error {
	if query.Offset < 0 || query.Limit < 1 || query.Limit > TaskQueryMaximumLimit || len(sources) > int(query.Limit) ||
		(query.Status != "" && !ExternalEffectStatusKnown(query.Status)) {
		return errors.New("external effects store violated its query boundary")
	}
	var previous TaskID
	for index, source := range sources {
		if source.TaskID < 1 || !ExternalEffectStatusKnown(source.Status) ||
			!validExternalEffectAttemptCount(source.Status, source.AttemptCount) ||
			source.CreatedAt.IsZero() || source.StatusUpdatedAt.IsZero() || source.StatusUpdatedAt.Before(source.CreatedAt) ||
			(query.Status != "" && source.Status != query.Status) ||
			(index > 0 && source.TaskID >= previous) {
			return fmt.Errorf("invalid external effect source at index %d", index)
		}
		previous = source.TaskID
	}
	return nil
}

func validExternalEffectAttemptCount(status TaskStatus, count int32) bool {
	if count < 0 {
		return false
	}
	switch status {
	case TaskStatusPending, TaskStatusCancelled:
		return count == 0
	case TaskStatusSending, TaskStatusSent, TaskStatusRetryableFailed, TaskStatusFinalFailed, TaskStatusOutcomeUnknown:
		return count > 0
	default:
		return false
	}
}

func validExternalEffectStatusCounts(counts ExternalEffectStatusCounts) bool {
	_, ok := checkedExternalEffectCountSum(
		counts.Pending, counts.Sending, counts.Sent, counts.RetryableFailed,
		counts.FinalFailed, counts.OutcomeUnknown, counts.Cancelled,
	)
	return ok
}

func checkedExternalEffectCountSum(values ...int64) (int64, bool) {
	const maximumInt64 = int64(1<<63 - 1)
	var total int64
	for _, value := range values {
		if value < 0 || total > maximumInt64-value {
			return 0, false
		}
		total += value
	}
	return total, true
}

func deriveExternalEffectsKey(secret []byte, purpose string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("ai-crm-v2/external-effects/" + purpose + "/v1"))
	var result [sha256.Size]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func nilExternalEffectsDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}
