package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

const maximumPublicCodeAttempts = 8

const (
	eventLinkCreated  = "radar.link_created"
	eventLinkUpdated  = "radar.link_updated"
	eventLinkEnabled  = "radar.link_enabled"
	eventLinkDisabled = "radar.link_disabled"
)

type Service struct {
	uow                platformport.UnitOfWork
	repository         radarport.Repository
	events             eventport.Appender
	now                func() time.Time
	generatePublicCode func() (string, error)
}

var _ radarport.Application = (*Service)(nil)

func NewService(uow platformport.UnitOfWork, repository radarport.Repository, events eventport.Appender) (*Service, error) {
	if nilDependency(uow) || nilDependency(repository) || nilDependency(events) {
		return nil, radarport.ErrUnavailable
	}
	return &Service{
		uow:                uow,
		repository:         repository,
		events:             events,
		now:                time.Now,
		generatePublicCode: randomPublicCode,
	}, nil
}

func (service *Service) List(ctx context.Context, input radarport.ListInput) (radarport.Page, error) {
	normalized, err := normalizeList(input)
	if err != nil {
		return radarport.Page{}, err
	}
	if !service.ready() {
		return radarport.Page{}, radarport.ErrUnavailable
	}
	var links []radarport.Link
	var total int64
	if err = service.uow.Within(ctx, func(tx context.Context) error {
		var listErr error
		links, total, listErr = service.repository.List(tx, normalized)
		return listErr
	}); err != nil {
		return radarport.Page{}, classify(err)
	}
	if total < 0 || len(links) > int(normalized.Limit) || int64(len(links)) > total {
		return radarport.Page{}, radarport.ErrUnavailable
	}
	if int64(normalized.Offset) >= total && len(links) != 0 {
		return radarport.Page{}, radarport.ErrUnavailable
	}
	for index := range links {
		if !validateStoredLink(links[index]) {
			return radarport.Page{}, radarport.ErrUnavailable
		}
		links[index] = cloneLink(links[index])
	}
	consumed := int64(normalized.Offset) + int64(len(links))
	return radarport.Page{
		Items:                    links,
		Total:                    total,
		Limit:                    normalized.Limit,
		Offset:                   normalized.Offset,
		HasMore:                  consumed < total,
		StatusFilter:             normalized.Status,
		Sort:                     normalized.Sort,
		LocalProjection:          true,
		RealExternalCallExecuted: false,
	}, nil
}

func (service *Service) Get(ctx context.Context, id radarport.LinkID) (radarport.LinkResponse, error) {
	if id < 1 {
		return radarport.LinkResponse{}, radarport.Invalid("link_id", "invalid")
	}
	if !service.ready() {
		return radarport.LinkResponse{}, radarport.ErrUnavailable
	}
	var link radarport.Link
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var getErr error
		link, getErr = service.repository.Get(tx, id)
		return getErr
	})
	if err != nil {
		return radarport.LinkResponse{}, classify(err)
	}
	return linkResult(link)
}

func (service *Service) Create(ctx context.Context, command radarport.CreateCommand) (radarport.LinkResponse, error) {
	normalized, err := normalizeCreate(command)
	if err != nil {
		return radarport.LinkResponse{}, err
	}
	if !service.ready() {
		return radarport.LinkResponse{}, radarport.ErrUnavailable
	}
	digest, err := canonicalDigest(struct {
		ExpectedVersion int64  `json:"expected_version"`
		Name            string `json:"name"`
		Title           string `json:"title"`
		DestinationURL  string `json:"destination_url"`
		CoverImageID    *int64 `json:"cover_image_id"`
		AttachmentID    *int64 `json:"attachment_id"`
	}{
		ExpectedVersion: normalized.ExpectedVersion,
		Name:            normalized.Name,
		Title:           normalized.Title,
		DestinationURL:  normalized.DestinationURL,
		CoverImageID:    normalized.CoverImageID,
		AttachmentID:    normalized.AttachmentID,
	})
	if err != nil {
		return radarport.LinkResponse{}, err
	}
	now := service.nowUTC()
	if now.IsZero() {
		return radarport.LinkResponse{}, radarport.ErrUnavailable
	}

	var result radarport.LinkResponse
	err = service.uow.Within(ctx, func(tx context.Context) error {
		record, replay, reserveErr := service.reserve(tx, normalized.ActorID, normalized.IdempotencyKey, "create", digest, now)
		if reserveErr != nil {
			return reserveErr
		}
		if replay {
			var replayErr error
			result, replayErr = linkResult(*record.Result)
			return replayErr
		}

		var created radarport.Link
		for attempt := 0; attempt < maximumPublicCodeAttempts; attempt++ {
			publicCode, codeErr := service.generatePublicCode()
			if codeErr != nil || !validPublicCode(publicCode) {
				return radarport.ErrUnavailable
			}
			created, codeErr = service.repository.Create(tx, radarport.CreateRecord{
				PublicCode:     publicCode,
				Name:           normalized.Name,
				Title:          normalized.Title,
				DestinationURL: normalized.DestinationURL,
				CoverImageID:   normalized.CoverImageID,
				AttachmentID:   normalized.AttachmentID,
				Status:         radarport.StatusDraft,
				ActorID:        normalized.ActorID,
			}, now)
			if errors.Is(codeErr, radarport.ErrPublicCodeCollision) {
				continue
			}
			if codeErr != nil {
				return codeErr
			}
			break
		}
		if created.LinkID < 1 {
			return radarport.ErrUnavailable
		}
		var resultErr error
		result, resultErr = linkResult(created)
		if resultErr != nil || result.Link.Version != 1 || result.Link.Status != radarport.StatusDraft {
			return radarport.ErrUnavailable
		}
		if appendErr := service.appendEvent(tx, eventLinkCreated, "create", result.Link, normalized.ActorID, normalized.IdempotencyKey, now); appendErr != nil {
			return appendErr
		}
		return service.complete(tx, record.RecordID, result, now)
	})
	if err != nil {
		return radarport.LinkResponse{}, classify(err)
	}
	return result, nil
}

func (service *Service) Update(ctx context.Context, command radarport.UpdateCommand) (radarport.LinkResponse, error) {
	normalized, err := normalizeUpdate(command)
	if err != nil {
		return radarport.LinkResponse{}, err
	}
	if !service.ready() {
		return radarport.LinkResponse{}, radarport.ErrUnavailable
	}
	digest, err := updateDigest(normalized)
	if err != nil {
		return radarport.LinkResponse{}, err
	}
	now := service.nowUTC()
	if now.IsZero() {
		return radarport.LinkResponse{}, radarport.ErrUnavailable
	}

	var result radarport.LinkResponse
	err = service.uow.Within(ctx, func(tx context.Context) error {
		record, replay, reserveErr := service.reserve(tx, normalized.ActorID, normalized.IdempotencyKey, "update", digest, now)
		if reserveErr != nil {
			return reserveErr
		}
		if replay {
			var replayErr error
			result, replayErr = linkResult(*record.Result)
			return replayErr
		}
		current, loadErr := service.repository.GetForUpdate(tx, normalized.LinkID)
		if loadErr != nil {
			return loadErr
		}
		if !validateStoredLink(current) {
			return radarport.ErrUnavailable
		}
		if current.Version != normalized.ExpectedVersion {
			return radarport.ErrConflict
		}

		updated := cloneLink(current)
		if normalized.Name.Set {
			updated.Name = normalized.Name.Value
		}
		if normalized.Title.Set {
			updated.Title = normalized.Title.Value
		}
		if normalized.DestinationURL.Set {
			updated.DestinationURL = normalized.DestinationURL.Value
		}
		if normalized.CoverImageID.Set {
			updated.CoverImageID = cloneID(normalized.CoverImageID.Value)
		}
		if normalized.AttachmentID.Set {
			updated.AttachmentID = cloneID(normalized.AttachmentID.Value)
		}
		if !validateStoredLink(updated) {
			return radarport.ErrInvalidArgument
		}
		changed := updated.Name != current.Name || updated.Title != current.Title || updated.DestinationURL != current.DestinationURL || !equivalentID(updated.CoverImageID, current.CoverImageID) || !equivalentID(updated.AttachmentID, current.AttachmentID)
		if changed {
			updated, loadErr = service.repository.Update(tx, radarport.UpdateRecord{
				LinkID:          current.LinkID,
				ExpectedVersion: current.Version,
				Name:            updated.Name,
				Title:           updated.Title,
				DestinationURL:  updated.DestinationURL,
				CoverImageID:    updated.CoverImageID,
				AttachmentID:    updated.AttachmentID,
				ActorID:         normalized.ActorID,
			}, now)
			if loadErr != nil {
				return loadErr
			}
			if updated.Version != current.Version+1 || updated.PublicCode != current.PublicCode || !updated.CreatedAt.Equal(current.CreatedAt) {
				return radarport.ErrUnavailable
			}
		}
		var resultErr error
		result, resultErr = linkResult(updated)
		if resultErr != nil {
			return resultErr
		}
		if changed {
			if appendErr := service.appendEvent(tx, eventLinkUpdated, "update", result.Link, normalized.ActorID, normalized.IdempotencyKey, now); appendErr != nil {
				return appendErr
			}
		}
		return service.complete(tx, record.RecordID, result, now)
	})
	if err != nil {
		return radarport.LinkResponse{}, classify(err)
	}
	return result, nil
}

func (service *Service) SetStatus(ctx context.Context, command radarport.SetStatusCommand) (radarport.LinkResponse, error) {
	normalized, err := normalizeStatus(command)
	if err != nil {
		return radarport.LinkResponse{}, err
	}
	if !service.ready() {
		return radarport.LinkResponse{}, radarport.ErrUnavailable
	}
	operation := "disable"
	eventType := eventLinkDisabled
	if normalized.Target == radarport.StatusEnabled {
		operation = "enable"
		eventType = eventLinkEnabled
	}
	digest, err := canonicalDigest(struct {
		LinkID          radarport.LinkID `json:"link_id"`
		ExpectedVersion int64            `json:"expected_version"`
		Target          radarport.Status `json:"target"`
	}{normalized.LinkID, normalized.ExpectedVersion, normalized.Target})
	if err != nil {
		return radarport.LinkResponse{}, err
	}
	now := service.nowUTC()
	if now.IsZero() {
		return radarport.LinkResponse{}, radarport.ErrUnavailable
	}

	var result radarport.LinkResponse
	err = service.uow.Within(ctx, func(tx context.Context) error {
		record, replay, reserveErr := service.reserve(tx, normalized.ActorID, normalized.IdempotencyKey, operation, digest, now)
		if reserveErr != nil {
			return reserveErr
		}
		if replay {
			var replayErr error
			result, replayErr = linkResult(*record.Result)
			return replayErr
		}
		current, loadErr := service.repository.GetForUpdate(tx, normalized.LinkID)
		if loadErr != nil {
			return loadErr
		}
		if !validateStoredLink(current) {
			return radarport.ErrUnavailable
		}

		// CAS remains authoritative for every new idempotency key. Replaying the
		// same key is handled above; a fresh same-state request is a no-op only
		// when its expected_version matches the current row.
		if current.Version != normalized.ExpectedVersion {
			return radarport.ErrConflict
		}
		changed := current.Status != normalized.Target
		if changed {
			if !allowedTransition(current.Status, normalized.Target) {
				return radarport.ErrStateConflict
			}
			previous := cloneLink(current)
			current, loadErr = service.repository.SetStatus(tx, radarport.StatusRecord{
				LinkID:          previous.LinkID,
				ExpectedVersion: previous.Version,
				Target:          normalized.Target,
				ActorID:         normalized.ActorID,
			}, now)
			if loadErr != nil {
				return loadErr
			}
			if current.LinkID != previous.LinkID || current.PublicCode != previous.PublicCode || current.Version != previous.Version+1 || current.Status != normalized.Target || !current.CreatedAt.Equal(previous.CreatedAt) {
				return radarport.ErrUnavailable
			}
		}
		var resultErr error
		result, resultErr = linkResult(current)
		if resultErr != nil {
			return resultErr
		}
		if changed {
			if appendErr := service.appendEvent(tx, eventType, operation, result.Link, normalized.ActorID, normalized.IdempotencyKey, now); appendErr != nil {
				return appendErr
			}
		}
		return service.complete(tx, record.RecordID, result, now)
	})
	if err != nil {
		return radarport.LinkResponse{}, classify(err)
	}
	return result, nil
}

func (service *Service) Share(ctx context.Context, id radarport.LinkID) (radarport.ShareProjection, error) {
	result, err := service.Get(ctx, id)
	if err != nil {
		return radarport.ShareProjection{}, err
	}
	return radarport.ShareProjection{
		LinkID:                   result.Link.LinkID,
		PublicCode:               result.Link.PublicCode,
		Status:                   result.Link.Status,
		SharePath:                "/local/radar-links/" + result.Link.PublicCode,
		QRPayload:                "aicrm-local://radar-links/" + result.Link.PublicCode,
		LocalProjection:          true,
		PublicRouteReady:         false,
		RealExternalCallExecuted: false,
	}, nil
}

func (service *Service) Options(context.Context) radarport.Options {
	return radarport.Options{
		Statuses:      []radarport.Status{radarport.StatusDraft, radarport.StatusEnabled, radarport.StatusDisabled},
		StatusFilters: []radarport.StatusFilter{radarport.StatusFilterAll, radarport.StatusFilterDraft, radarport.StatusFilterEnabled, radarport.StatusFilterDisabled},
		Sorts:         []radarport.Sort{radarport.SortUpdatedDesc, radarport.SortCreatedDesc, radarport.SortNameAsc},
		Defaults: radarport.OptionDefaults{
			InitialStatus: radarport.StatusDraft,
			StatusFilter:  radarport.StatusFilterAll,
			Sort:          radarport.SortUpdatedDesc,
			Limit:         radarport.DefaultLimit,
		},
		Limits: radarport.OptionLimits{
			NameRunes:                  radarport.MaximumNameRunes,
			TitleRunes:                 radarport.MaximumTitleRunes,
			DestinationURLBytes:        radarport.MaximumURLBytes,
			ListLimitMinimum:           1,
			ListLimitMaximum:           radarport.MaximumLimit,
			ListOffsetMaximum:          radarport.MaximumOffset,
			RequestBodyBytes:           radarport.MaximumRequestBodyBytes,
			IdempotencyKeyBytesMinimum: radarport.MinimumIdempotencyKeyBytes,
			IdempotencyKeyBytesMaximum: radarport.MaximumIdempotencyKeyBytes,
		},
		DestinationSchemes:       []string{"https"},
		LocalProjection:          true,
		PublicRouteReady:         false,
		RealExternalCallExecuted: false,
	}
}

func (service *Service) reserve(ctx context.Context, actorID int64, key, operation string, payloadDigest [32]byte, now time.Time) (radarport.IdempotencyRecord, bool, error) {
	keyDigest := sha256.Sum256([]byte(key))
	record, owned, err := service.repository.ReserveIdempotency(ctx, radarport.ReserveIdempotencyRecord{
		ActorID:       actorID,
		KeyDigest:     keyDigest,
		Operation:     operation,
		PayloadDigest: payloadDigest,
		CreatedAt:     now,
	})
	if err != nil {
		return radarport.IdempotencyRecord{}, false, err
	}
	if record.RecordID < 1 || record.ActorID != actorID || record.Operation == "" || record.CreatedAt.IsZero() || subtle.ConstantTimeCompare(record.KeyDigest[:], keyDigest[:]) != 1 {
		return radarport.IdempotencyRecord{}, false, radarport.ErrUnavailable
	}
	if record.Operation != operation || subtle.ConstantTimeCompare(record.PayloadDigest[:], payloadDigest[:]) != 1 {
		return radarport.IdempotencyRecord{}, false, radarport.ErrIdempotencyConflict
	}
	if owned {
		if record.State != radarport.IdempotencyReserved || record.Result != nil || record.CompletedAt != nil {
			return radarport.IdempotencyRecord{}, false, radarport.ErrUnavailable
		}
		return record, false, nil
	}
	if record.State != radarport.IdempotencyCompleted || record.Result == nil || record.CompletedAt == nil || !validateStoredLink(*record.Result) {
		return radarport.IdempotencyRecord{}, false, radarport.ErrIdempotencyStateInvalid
	}
	cloned := cloneLink(*record.Result)
	record.Result = &cloned
	return record, true, nil
}

func (service *Service) complete(ctx context.Context, recordID int64, result radarport.LinkResponse, now time.Time) error {
	if _, err := linkResult(result.Link); err != nil || !result.LocalProjection || result.RealExternalCallExecuted {
		return radarport.ErrUnavailable
	}
	record, err := service.repository.CompleteIdempotency(ctx, recordID, result.Link, now)
	if err != nil {
		return err
	}
	if record.RecordID != recordID || record.State != radarport.IdempotencyCompleted || record.Result == nil || record.CompletedAt == nil || !equalLink(*record.Result, result.Link) {
		return radarport.ErrUnavailable
	}
	return nil
}

func (service *Service) appendEvent(ctx context.Context, eventType, action string, link radarport.Link, actorID int64, key string, now time.Time) error {
	payload, err := json.Marshal(struct {
		Action     string           `json:"action"`
		LinkID     radarport.LinkID `json:"link_id"`
		PublicCode string           `json:"public_code"`
		Status     radarport.Status `json:"status"`
		Version    int64            `json:"version"`
		ActorID    int64            `json:"actor_id"`
	}{action, link.LinkID, link.PublicCode, link.Status, link.Version, actorID})
	if err != nil {
		return radarport.ErrUnavailable
	}
	keyDigest := sha256.Sum256([]byte(strconv.FormatInt(actorID, 10) + "\x00" + key + "\x00" + action))
	_, err = service.events.Append(ctx, eventport.Event{
		Type:           eventType,
		Payload:        payload,
		OccurredAt:     now,
		IdempotencyKey: "radar." + action + ":" + hex.EncodeToString(keyDigest[:]),
	})
	return err
}

func updateDigest(command radarport.UpdateCommand) ([32]byte, error) {
	return canonicalDigest(struct {
		LinkID            radarport.LinkID `json:"link_id"`
		ExpectedVersion   int64            `json:"expected_version"`
		NameSet           bool             `json:"name_set"`
		Name              string           `json:"name"`
		TitleSet          bool             `json:"title_set"`
		Title             string           `json:"title"`
		DestinationURLSet bool             `json:"destination_url_set"`
		DestinationURL    string           `json:"destination_url"`
		CoverImageIDSet   bool             `json:"cover_image_id_set"`
		CoverImageID      *int64           `json:"cover_image_id"`
		AttachmentIDSet   bool             `json:"attachment_id_set"`
		AttachmentID      *int64           `json:"attachment_id"`
	}{
		LinkID:            command.LinkID,
		ExpectedVersion:   command.ExpectedVersion,
		NameSet:           command.Name.Set,
		Name:              command.Name.Value,
		TitleSet:          command.Title.Set,
		Title:             command.Title.Value,
		DestinationURLSet: command.DestinationURL.Set,
		DestinationURL:    command.DestinationURL.Value,
		CoverImageIDSet:   command.CoverImageID.Set,
		CoverImageID:      command.CoverImageID.Value,
		AttachmentIDSet:   command.AttachmentID.Set,
		AttachmentID:      command.AttachmentID.Value,
	})
}

func linkResult(link radarport.Link) (radarport.LinkResponse, error) {
	if !validateStoredLink(link) {
		return radarport.LinkResponse{}, radarport.ErrUnavailable
	}
	return radarport.LinkResponse{
		Link:                     cloneLink(link),
		LocalProjection:          true,
		RealExternalCallExecuted: false,
	}, nil
}

func cloneLink(link radarport.Link) radarport.Link {
	link.CoverImageID = cloneID(link.CoverImageID)
	link.AttachmentID = cloneID(link.AttachmentID)
	return link
}

func allowedTransition(current, target radarport.Status) bool {
	switch target {
	case radarport.StatusEnabled:
		return current == radarport.StatusDraft || current == radarport.StatusDisabled
	case radarport.StatusDisabled:
		return current == radarport.StatusEnabled
	default:
		return false
	}
}

func randomPublicCode() (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return "rd_" + base64.RawURLEncoding.EncodeToString(entropy[:]), nil
}

func (service *Service) nowUTC() time.Time {
	if service == nil || service.now == nil {
		return time.Time{}
	}
	return service.now().UTC()
}

func (service *Service) ready() bool {
	return service != nil && !nilDependency(service.uow) && !nilDependency(service.repository) && !nilDependency(service.events) && service.now != nil && service.generatePublicCode != nil
}

func nilDependency(value any) bool {
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

func classify(err error) error {
	if err == nil {
		return nil
	}
	var validation *radarport.ValidationError
	if errors.As(err, &validation) {
		return validation
	}
	for _, known := range []error{
		radarport.ErrNotFound,
		radarport.ErrConflict,
		radarport.ErrStateConflict,
		radarport.ErrIdempotencyConflict,
	} {
		if errors.Is(err, known) {
			return known
		}
	}
	return radarport.ErrUnavailable
}

func equalLink(left, right radarport.Link) bool {
	return left.LinkID == right.LinkID &&
		left.PublicCode == right.PublicCode &&
		left.Name == right.Name &&
		left.Title == right.Title &&
		left.DestinationURL == right.DestinationURL &&
		equivalentID(left.CoverImageID, right.CoverImageID) &&
		equivalentID(left.AttachmentID, right.AttachmentID) &&
		left.Status == right.Status &&
		left.Version == right.Version &&
		left.CreatedBy == right.CreatedBy &&
		left.UpdatedBy == right.UpdatedBy &&
		left.CreatedAt.Equal(right.CreatedAt) &&
		left.UpdatedAt.Equal(right.UpdatedAt)
}
