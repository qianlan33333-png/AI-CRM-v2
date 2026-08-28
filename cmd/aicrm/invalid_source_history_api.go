package main

import (
	"context"
	"github.com/go-chi/chi/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	"net/http"
)

func (h *Handler) ListUnboundTagHistory(w http.ResponseWriter, r *http.Request) {
	h.serveUnboundTagHistory(w, r, false)
}
func (h *Handler) GetUnboundTagHistory(w http.ResponseWriter, r *http.Request) {
	h.serveUnboundTagHistory(w, r, true)
}
func (h *Handler) serveUnboundTagHistory(w http.ResponseWriter, r *http.Request, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.contactInvalidSourceHistory) {
		staticHistoryUnavailable(w)
		return
	}
	var id int64
	var limit, offset int32
	var valid bool
	if detail {
		id, valid = audienceHistoryID(chi.URLParam(r, "history_id"))
		valid = valid && r.URL.RawQuery == ""
	} else {
		limit, offset, valid = audienceHistoryPage(r.URL.RawQuery)
	}
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_source_history_query"})
		return
	}
	writeStaticHistory(w, r, detail, id, limit, offset, h.contactInvalidSourceHistory.GetHistoricalUnboundTag, func(ctx context.Context) ([]contactport.HistoricalUnboundTag, int64, error) {
		return h.contactInvalidSourceHistory.ListHistoricalUnboundTag(ctx, contactport.InvalidSourceHistoryQuery{Limit: limit, Offset: offset})
	}, func(v contactport.HistoricalUnboundTag) (int64, error) {
		_, err := contactapp.DigestHistoricalUnboundTag(v)
		return v.ID, err
	})
}

func (h *Handler) ListInvalidChannelHistory(w http.ResponseWriter, r *http.Request) {
	h.serveInvalidChannelHistory(w, r, false)
}
func (h *Handler) GetInvalidChannelHistory(w http.ResponseWriter, r *http.Request) {
	h.serveInvalidChannelHistory(w, r, true)
}
func (h *Handler) serveInvalidChannelHistory(w http.ResponseWriter, r *http.Request, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.contactInvalidSourceHistory) {
		staticHistoryUnavailable(w)
		return
	}
	var id int64
	var limit, offset int32
	var valid bool
	if detail {
		id, valid = audienceHistoryID(chi.URLParam(r, "history_id"))
		valid = valid && r.URL.RawQuery == ""
	} else {
		limit, offset, valid = audienceHistoryPage(r.URL.RawQuery)
	}
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_source_history_query"})
		return
	}
	writeStaticHistory(w, r, detail, id, limit, offset, h.contactInvalidSourceHistory.GetHistoricalInvalidChannel, func(ctx context.Context) ([]contactport.HistoricalInvalidChannel, int64, error) {
		return h.contactInvalidSourceHistory.ListHistoricalInvalidChannel(ctx, contactport.InvalidSourceHistoryQuery{Limit: limit, Offset: offset})
	}, func(v contactport.HistoricalInvalidChannel) (int64, error) {
		_, err := contactapp.DigestHistoricalInvalidChannel(v)
		return v.ID, err
	})
}

func (h *Handler) ListInvalidAssetHistory(w http.ResponseWriter, r *http.Request) {
	h.serveInvalidAssetHistory(w, r, false)
}
func (h *Handler) GetInvalidAssetHistory(w http.ResponseWriter, r *http.Request) {
	h.serveInvalidAssetHistory(w, r, true)
}
func (h *Handler) serveInvalidAssetHistory(w http.ResponseWriter, r *http.Request, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.mediaInvalidSourceHistory) {
		staticHistoryUnavailable(w)
		return
	}
	var id int64
	var limit, offset int32
	var valid bool
	if detail {
		id, valid = audienceHistoryID(chi.URLParam(r, "history_id"))
		valid = valid && r.URL.RawQuery == ""
	} else {
		limit, offset, valid = audienceHistoryPage(r.URL.RawQuery)
	}
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_source_history_query"})
		return
	}
	writeStaticHistory(w, r, detail, id, limit, offset, h.mediaInvalidSourceHistory.GetHistoricalInvalidAsset, func(ctx context.Context) ([]mediaport.HistoricalInvalidAsset, int64, error) {
		return h.mediaInvalidSourceHistory.ListHistoricalInvalidAsset(ctx, mediaport.InvalidSourceHistoryQuery{Limit: limit, Offset: offset})
	}, func(v mediaport.HistoricalInvalidAsset) (int64, error) {
		_, err := mediaapp.DigestHistoricalInvalidAsset(v)
		return v.ID, err
	})
}

func (h *Handler) ListInvalidRadarLinkHistory(w http.ResponseWriter, r *http.Request) {
	h.serveInvalidRadarLinkHistory(w, r, false)
}
func (h *Handler) GetInvalidRadarLinkHistory(w http.ResponseWriter, r *http.Request) {
	h.serveInvalidRadarLinkHistory(w, r, true)
}
func (h *Handler) serveInvalidRadarLinkHistory(w http.ResponseWriter, r *http.Request, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.radarInvalidSourceHistory) {
		staticHistoryUnavailable(w)
		return
	}
	var id int64
	var limit, offset int32
	var valid bool
	if detail {
		id, valid = audienceHistoryID(chi.URLParam(r, "history_id"))
		valid = valid && r.URL.RawQuery == ""
	} else {
		limit, offset, valid = audienceHistoryPage(r.URL.RawQuery)
	}
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_source_history_query"})
		return
	}
	writeStaticHistory(w, r, detail, id, limit, offset, h.radarInvalidSourceHistory.GetHistoricalInvalidRadarLink, func(ctx context.Context) ([]radarport.HistoricalInvalidRadarLink, int64, error) {
		return h.radarInvalidSourceHistory.ListHistoricalInvalidRadarLink(ctx, radarport.InvalidSourceHistoryQuery{Limit: limit, Offset: offset})
	}, func(v radarport.HistoricalInvalidRadarLink) (int64, error) {
		_, err := radarapp.DigestHistoricalInvalidRadarLink(v)
		return v.ID, err
	})
}
