package main

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	cycleapp "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/app"
	cycleport "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	"net/http"
	"net/url"
)

var errStaticHistoryResponse = errors.New("invalid static history response")

func (h *Handler) ListStaticHistoryGroupInvite(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "GroupInvite", false)
}
func (h *Handler) GetStaticHistoryGroupInvite(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "GroupInvite", true)
}
func (h *Handler) ListStaticHistoryProductPageSlice(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "ProductPageSlice", false)
}
func (h *Handler) GetStaticHistoryProductPageSlice(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "ProductPageSlice", true)
}
func (h *Handler) ListStaticHistoryCycleStrategy(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "CycleStrategy", false)
}
func (h *Handler) GetStaticHistoryCycleStrategy(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "CycleStrategy", true)
}
func (h *Handler) ListStaticHistoryCycleVersion(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "CycleVersion", false)
}
func (h *Handler) GetStaticHistoryCycleVersion(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "CycleVersion", true)
}
func (h *Handler) ListStaticHistoryCycleDocument(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "CycleDocument", false)
}
func (h *Handler) GetStaticHistoryCycleDocument(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "CycleDocument", true)
}

func (h *Handler) ListStaticHistoryCycleMetric(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "CycleMetric", false)
}
func (h *Handler) GetStaticHistoryCycleMetric(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "CycleMetric", true)
}
func (h *Handler) ListStaticHistoryCycleReference(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "CycleReference", false)
}
func (h *Handler) GetStaticHistoryCycleReference(w http.ResponseWriter, r *http.Request) {
	h.serveStaticHistory(w, r, "CycleReference", true)
}

func (h *Handler) serveStaticHistory(w http.ResponseWriter, r *http.Request, kind string, detail bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil {
		staticHistoryUnavailable(w)
		return
	}
	limit, offset, parent, id, ok := parseStaticHistoryQuery(r, kind, detail)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_static_history_query"})
		return
	}
	switch kind {
	case "GroupInvite":
		if nilLegacyDependency(h.staticMediaHistory) {
			staticHistoryUnavailable(w)
			return
		}
		query := mediaport.StaticMediaHistoryQuery{Limit: limit, Offset: offset}
		writeStaticHistory(w, r, detail, id, limit, offset, h.staticMediaHistory.GetHistoricalGroupInvite,
			func(ctx context.Context) ([]mediaport.HistoricalGroupInvite, int64, error) {
				return h.staticMediaHistory.ListHistoricalGroupInvite(ctx, query)
			},
			func(v mediaport.HistoricalGroupInvite) (int64, error) {
				_, err := mediaapp.HistoricalGroupInviteDigest(v)
				return v.ID, err
			})
	case "ProductPageSlice":
		if nilLegacyDependency(h.staticProductHistory) {
			staticHistoryUnavailable(w)
			return
		}
		query := productport.StaticProductHistoryQuery{Limit: limit, Offset: offset}
		writeStaticHistory(w, r, detail, id, limit, offset, h.staticProductHistory.GetHistoricalProductPageSlice,
			func(ctx context.Context) ([]productport.HistoricalProductPageSlice, int64, error) {
				return h.staticProductHistory.ListHistoricalProductPageSlice(ctx, query)
			},
			func(v productport.HistoricalProductPageSlice) (int64, error) {
				_, err := productapp.HistoricalProductPageSliceDigest(v)
				return v.ID, err
			})
	case "CycleStrategy":
		if nilLegacyDependency(h.staticCycleHistory) {
			staticHistoryUnavailable(w)
			return
		}
		query := cycleport.StaticCycleHistoryQuery{Limit: limit, Offset: offset}
		writeStaticHistory(w, r, detail, id, limit, offset, h.staticCycleHistory.GetHistoricalCycleStrategy,
			func(ctx context.Context) ([]cycleport.HistoricalCycleStrategy, int64, error) {
				return h.staticCycleHistory.ListHistoricalCycleStrategy(ctx, query)
			},
			func(v cycleport.HistoricalCycleStrategy) (int64, error) {
				_, err := cycleapp.HistoricalCycleStrategyDigest(v)
				return v.ID, err
			})
	case "CycleVersion":
		if nilLegacyDependency(h.staticCycleHistory) {
			staticHistoryUnavailable(w)
			return
		}
		query := cycleport.StaticCycleHistoryQuery{Limit: limit, Offset: offset, StrategyHistoryID: parent}
		writeStaticHistory(w, r, detail, id, limit, offset, h.staticCycleHistory.GetHistoricalCycleVersion,
			func(ctx context.Context) ([]cycleport.HistoricalCycleVersion, int64, error) {
				return h.staticCycleHistory.ListHistoricalCycleVersion(ctx, query)
			},
			func(v cycleport.HistoricalCycleVersion) (int64, error) {
				_, err := cycleapp.HistoricalCycleVersionDigest(v)
				if err == nil && parent != nil && v.StrategyHistoryID != *parent {
					err = errStaticHistoryResponse
				}
				return v.ID, err
			})
	case "CycleDocument":
		if nilLegacyDependency(h.staticCycleHistory) {
			staticHistoryUnavailable(w)
			return
		}
		query := cycleport.StaticCycleHistoryQuery{Limit: limit, Offset: offset, VersionHistoryID: parent}
		writeStaticHistory(w, r, detail, id, limit, offset, h.staticCycleHistory.GetHistoricalCycleDocument,
			func(ctx context.Context) ([]cycleport.HistoricalCycleDocument, int64, error) {
				return h.staticCycleHistory.ListHistoricalCycleDocument(ctx, query)
			},
			func(v cycleport.HistoricalCycleDocument) (int64, error) {
				_, err := cycleapp.HistoricalCycleDocumentDigest(v)
				if err == nil && parent != nil && v.VersionHistoryID != *parent {
					err = errStaticHistoryResponse
				}
				return v.ID, err
			})
	case "CycleMetric":
		if nilLegacyDependency(h.cycleObservationHistory) {
			staticHistoryUnavailable(w)
			return
		}
		query := cycleport.CycleObservationQuery{Limit: limit, Offset: offset}
		writeStaticHistory(w, r, detail, id, limit, offset, h.cycleObservationHistory.GetHistoricalCycleMetric,
			func(ctx context.Context) ([]cycleport.HistoricalCycleMetric, int64, error) {
				return h.cycleObservationHistory.ListHistoricalCycleMetric(ctx, query)
			},
			func(v cycleport.HistoricalCycleMetric) (int64, error) {
				_, err := cycleapp.HistoricalCycleMetricDigest(v)
				return v.ID, err
			})
	case "CycleReference":
		if nilLegacyDependency(h.cycleObservationHistory) {
			staticHistoryUnavailable(w)
			return
		}
		query := cycleport.CycleObservationQuery{Limit: limit, Offset: offset}
		writeStaticHistory(w, r, detail, id, limit, offset, h.cycleObservationHistory.GetHistoricalCycleReference,
			func(ctx context.Context) ([]cycleport.HistoricalCycleReference, int64, error) {
				return h.cycleObservationHistory.ListHistoricalCycleReference(ctx, query)
			},
			func(v cycleport.HistoricalCycleReference) (int64, error) {
				_, err := cycleapp.HistoricalCycleReferenceDigest(v)
				return v.ID, err
			})
	default:
		staticHistoryUnavailable(w)
	}
}

func writeStaticHistory[T any](w http.ResponseWriter, r *http.Request, detail bool, id int64, limit, offset int32,
	get func(context.Context, int64) (T, error), list func(context.Context) ([]T, int64, error), validate func(T) (int64, error)) {
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false}
	if detail {
		item, err := get(r.Context(), id)
		if err != nil {
			staticHistoryUnavailable(w)
			return
		}
		got, err := validate(item)
		if err != nil || got != id {
			staticHistoryUnavailable(w)
			return
		}
		response["item"] = item
	} else {
		items, total, err := list(r.Context())
		if err != nil || total < 0 || int64(len(items)) != min(int64(limit), max(0, total-int64(offset))) {
			staticHistoryUnavailable(w)
			return
		}
		seen := make(map[int64]bool, len(items))
		for _, item := range items {
			got, err := validate(item)
			if err != nil || got < 1 || seen[got] {
				staticHistoryUnavailable(w)
				return
			}
			seen[got] = true
		}
		if items == nil {
			items = []T{}
		}
		response["items"], response["total"], response["limit"], response["offset"] = items, total, limit, offset
	}
	writeJSON(w, http.StatusOK, response)
}

func parseStaticHistoryQuery(r *http.Request, kind string, detail bool) (limit, offset int32, parent *int64, id int64, valid bool) {
	if detail {
		id, valid = audienceHistoryID(chi.URLParam(r, "history_id"))
		valid = valid && r.URL.RawQuery == ""
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return
	}
	for _, key := range []string{"strategy_history_id", "version_history_id"} {
		if raw, present := values[key]; present {
			if len(raw) != 1 || (key == "strategy_history_id" && kind != "CycleVersion") || (key == "version_history_id" && kind != "CycleDocument") {
				return
			}
			parsed, ok := audienceHistoryID(raw[0])
			if !ok {
				return
			}
			parent = &parsed
			values.Del(key)
		}
	}
	limit, offset, valid = audienceHistoryPage(values.Encode())
	return
}
func staticHistoryUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "static_history_unavailable"})
}
