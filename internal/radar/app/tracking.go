package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

func (service *Service) ResolvePublicRedirect(ctx context.Context, code string) (radarport.PublicRedirect, error) {
	code = strings.TrimSpace(code)
	if !validPublicCode(code) || len(code) > radarport.MaximumPublicCodeBytes {
		return radarport.PublicRedirect{}, radarport.ErrNotFound
	}
	if !service.trackingReady() {
		return radarport.PublicRedirect{}, radarport.ErrUnavailable
	}
	now := service.nowUTC()
	if now.IsZero() {
		return radarport.PublicRedirect{}, radarport.ErrUnavailable
	}
	var result radarport.PublicRedirect
	err := service.uow.Within(ctx, func(tx context.Context) error {
		link, err := service.tracking.GetEnabledByCode(tx, code)
		if err != nil {
			return err
		}
		if !validateStoredLink(link) || link.Status != radarport.StatusEnabled {
			return radarport.ErrUnavailable
		}
		if _, err = service.insertTrackingEvent(tx, link.LinkID, radarport.EventStageLanding, nil, radarport.EventSourcePublicRedirect, nil, trackingDigest(radarport.EventStageLanding, nil), now); err != nil {
			return err
		}
		receipt, err := service.insertTrackingEvent(tx, link.LinkID, radarport.EventStageRedirect, nil, radarport.EventSourcePublicRedirect, nil, trackingDigest(radarport.EventStageRedirect, nil), now)
		if err != nil {
			return err
		}
		result = radarport.PublicRedirect{DestinationURL: link.DestinationURL, Receipt: receipt}
		return nil
	})
	if err != nil {
		return radarport.PublicRedirect{}, classify(err)
	}
	return result, nil
}

func (service *Service) RecordPublicEvent(ctx context.Context, command radarport.RecordEventCommand) (radarport.EventReceipt, error) {
	command.PublicCode = strings.TrimSpace(command.PublicCode)
	if !validPublicCode(command.PublicCode) || len(command.PublicCode) > radarport.MaximumPublicCodeBytes {
		return radarport.EventReceipt{}, radarport.ErrNotFound
	}
	if !command.Stage.PublicRecordable() {
		return radarport.EventReceipt{}, radarport.Invalid("stage", "unsupported_value")
	}
	if !validIdempotencyKey(command.IdempotencyKey) {
		return radarport.EventReceipt{}, radarport.Invalid("idempotency_key", "invalid")
	}
	if command.Page != nil && (*command.Page < 1 || *command.Page > radarport.MaximumEventPage) {
		return radarport.EventReceipt{}, radarport.Invalid("page", "out_of_range")
	}
	extra, err := json.Marshal(command.Extra)
	if err != nil || len(extra) > radarport.MaximumEventExtraJSON {
		return radarport.EventReceipt{}, radarport.Invalid("extra", "invalid")
	}
	if !service.trackingReady() {
		return radarport.EventReceipt{}, radarport.ErrUnavailable
	}
	digest, err := canonicalDigest(struct {
		Stage radarport.EventStage `json:"stage"`
		Page  *int32               `json:"page,omitempty"`
		Extra map[string]any       `json:"extra"`
	}{command.Stage, command.Page, command.Extra})
	if err != nil {
		return radarport.EventReceipt{}, radarport.ErrUnavailable
	}
	keyDigest := sha256.Sum256([]byte(command.IdempotencyKey))
	now := service.nowUTC()
	var receipt radarport.EventReceipt
	err = service.uow.Within(ctx, func(tx context.Context) error {
		link, loadErr := service.tracking.GetEnabledByCode(tx, command.PublicCode)
		if loadErr != nil {
			return loadErr
		}
		receipt, loadErr = service.insertTrackingEvent(tx, link.LinkID, command.Stage, command.Page, radarport.EventSourcePublicEvent, keyDigest[:], digest, now)
		return loadErr
	})
	if err != nil {
		return radarport.EventReceipt{}, classify(err)
	}
	return receipt, nil
}

func (service *Service) ListEvents(ctx context.Context, input radarport.EventListInput) (radarport.EventPage, error) {
	normalized, err := normalizeEventList(input)
	if err != nil {
		return radarport.EventPage{}, err
	}
	if !service.trackingReady() {
		return radarport.EventPage{}, radarport.ErrUnavailable
	}
	var items []radarport.Event
	var total int64
	err = service.uow.Within(ctx, func(tx context.Context) error {
		if _, loadErr := service.repository.Get(tx, normalized.LinkID); loadErr != nil {
			return loadErr
		}
		var listErr error
		items, total, listErr = service.tracking.ListEvents(tx, normalized)
		return listErr
	})
	if err != nil {
		return radarport.EventPage{}, classify(err)
	}
	return radarport.EventPage{
		Items: items, Events: append([]radarport.Event(nil), items...), Total: total,
		Limit: normalized.Limit, Offset: normalized.Offset,
		HasMore:            int64(normalized.Offset)+int64(len(items)) < total,
		IdentityAttributed: false, RealExternalCallExecuted: false,
	}, nil
}

func (service *Service) EventStats(ctx context.Context, linkID radarport.LinkID) (radarport.EventStats, error) {
	if linkID < 1 {
		return radarport.EventStats{}, radarport.Invalid("link_id", "invalid")
	}
	if !service.trackingReady() {
		return radarport.EventStats{}, radarport.ErrUnavailable
	}
	var record radarport.EventStatsRecord
	err := service.uow.Within(ctx, func(tx context.Context) error {
		if _, loadErr := service.repository.Get(tx, linkID); loadErr != nil {
			return loadErr
		}
		var statsErr error
		record, statsErr = service.tracking.EventStats(tx, linkID)
		return statsErr
	})
	if err != nil {
		return radarport.EventStats{}, classify(err)
	}
	return radarport.EventStats{
		LinkID: linkID, TotalEvents: record.TotalEvents,
		TotalClicks: record.TotalLandings, TotalLandings: record.TotalLandings,
		Redirects: record.Redirects, ViewerOpens: record.ViewerOpens, ViewOpens: record.ViewerOpens,
		ImageLoaded: record.ImageLoaded, PDFOpened: record.PDFOpened,
		TodayClicks: record.TodayLandings, TodayLandings: record.TodayLandings,
		LastClickedAt: record.LastClickedAt, LastEventAt: record.LastEventAt, LastViewedAt: record.LastViewedAt,
		IdentityAttributed: false, RealExternalCallExecuted: false,
	}, nil
}

func (service *Service) SidebarLinks(ctx context.Context, limit, offset int32, baseURL string) (radarport.SidebarPage, error) {
	if limit == 0 {
		limit = radarport.DefaultEventLimit
	}
	if limit < 1 || limit > radarport.MaximumSidebarLimit || offset < 0 || offset > radarport.MaximumEventOffset {
		return radarport.SidebarPage{}, radarport.Invalid("pagination", "out_of_range")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return radarport.SidebarPage{}, radarport.Invalid("base_url", "required")
	}
	if !service.trackingReady() {
		return radarport.SidebarPage{}, radarport.ErrUnavailable
	}
	var items []radarport.SidebarLink
	var total int64
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var listErr error
		items, total, listErr = service.tracking.ListEnabledForSidebar(tx, limit, offset)
		return listErr
	})
	if err != nil {
		return radarport.SidebarPage{}, classify(err)
	}
	for index := range items {
		items[index].URL = baseURL + "/r/" + items[index].URL
	}
	return radarport.SidebarPage{Items: items, Total: total, Limit: limit, Offset: offset, LocalProjection: true, IdentityAttributed: false, RealExternalCallExecuted: false}, nil
}

func (service *Service) insertTrackingEvent(ctx context.Context, linkID radarport.LinkID, stage radarport.EventStage, page *int32, source radarport.EventSource, key []byte, payload [32]byte, now time.Time) (radarport.EventReceipt, error) {
	receiptID, err := service.generateReceiptID()
	if err != nil {
		return radarport.EventReceipt{}, radarport.ErrUnavailable
	}
	event, created, err := service.tracking.InsertEvent(ctx, radarport.InsertEventRecord{
		ReceiptID: receiptID, LinkID: linkID, Stage: stage, Page: page, Source: source,
		KeyDigest: append([]byte(nil), key...), PayloadDigest: payload, CreatedAt: now,
	})
	if err != nil {
		return radarport.EventReceipt{}, err
	}
	if !created {
		existing, storedDigest, loadErr := service.tracking.GetEventByKey(ctx, linkID, key)
		if loadErr != nil {
			return radarport.EventReceipt{}, loadErr
		}
		if subtle.ConstantTimeCompare(storedDigest[:], payload[:]) != 1 || existing.Stage != stage {
			return radarport.EventReceipt{}, radarport.ErrIdempotencyConflict
		}
		event = existing
	}
	return radarport.EventReceipt{EventID: event.EventID, ReceiptID: event.ReceiptID, CreatedAt: event.CreatedAt, Replayed: !created, LocalReceipt: true, IdentityAttributed: false, RealExternalCallExecuted: false}, nil
}

func normalizeEventList(input radarport.EventListInput) (radarport.EventListInput, error) {
	if input.LinkID < 1 {
		return radarport.EventListInput{}, radarport.Invalid("link_id", "invalid")
	}
	if input.Stage != nil && !input.Stage.Valid() {
		return radarport.EventListInput{}, radarport.Invalid("stage", "unsupported_value")
	}
	if input.Limit == 0 {
		input.Limit = radarport.DefaultEventLimit
	}
	if input.Limit < 1 || input.Limit > radarport.MaximumEventLimit || input.Offset < 0 || input.Offset > radarport.MaximumEventOffset {
		return radarport.EventListInput{}, radarport.Invalid("pagination", "out_of_range")
	}
	if input.Start != nil && input.End != nil && input.Start.After(*input.End) {
		return radarport.EventListInput{}, radarport.Invalid("time_range", "invalid")
	}
	return input, nil
}

func trackingDigest(stage radarport.EventStage, page *int32) [32]byte {
	return sha256.Sum256([]byte(string(stage) + ":" + pageToken(page)))
}

func pageToken(page *int32) string {
	if page == nil {
		return "-"
	}
	return hex.EncodeToString([]byte{byte(*page >> 24), byte(*page >> 16), byte(*page >> 8), byte(*page)})
}

func randomReceiptID() (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return "rre_" + hex.EncodeToString(entropy[:]), nil
}

func (service *Service) trackingReady() bool {
	return service != nil && service.ready() && !nilDependency(service.tracking) && service.generateReceiptID != nil
}

var _ radarport.TrackingApplication = (*Service)(nil)
