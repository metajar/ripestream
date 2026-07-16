package api

import (
	"log/slog"
	"net/http"
	"time"

	"ripestream/internal/graph"
)

// overview -> UC1: internet overview with KPIs and top problematic ASes.
//
// Served from a background-refreshed cache (see cache.go). The expensive
// full-graph aggregation runs on a schedule and logs its elapsed time; this
// handler returns instantly. When no cached value is ready yet (first load or
// graph disabled), it falls back to a direct (possibly slow) query.
func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	if s.graph == nil {
		respondError(w, http.StatusServiceUnavailable, "graph is disabled")
		return
	}
	// Active-alert count is cheap and live; merge it into whatever we serve.
	mergeActive := func(o *graph.Overview) {
		if s.alerter == nil {
			return
		}
		if act, err := s.alerter.ActiveAlerts(r.Context()); err == nil {
			o.ActiveAlert = len(act)
		}
	}

	if s.overviewCache != nil {
		if val, stale, ok := s.overviewCache.get(); ok {
			mergeActive(&val)
			writeJSON(w, http.StatusOK, envelope{
				Data: val,
				Meta: overviewMeta{
					Cached: true,
					Stale:  stale,
					TookMS: s.overviewCache.lastMS.Load(),
					TTLSec: int(s.overviewTTL / time.Second),
					LastOK: s.overviewCache.lastOK.Load(),
				},
			})
			return
		}
		// No cached value yet: fall through to a direct query so the first
		// request after boot isn't empty while the initial refresh runs.
	}

	o, err := s.graph.Overview(r.Context())
	if err != nil {
		slog.Warn("overview query failed", "err", err)
		respondError(w, http.StatusBadGateway, "graph query failed: "+err.Error())
		return
	}
	mergeActive(&o)
	respondOK(w, o)
}

// overviewMeta describes cache freshness, surfaced in the response envelope.
type overviewMeta struct {
	Cached bool  `json:"cached"`
	Stale  bool  `json:"stale"`
	TookMS int64 `json:"took_ms"`
	TTLSec int   `json:"ttl_sec"`
	LastOK bool  `json:"last_ok"`
}

// asnIssues -> UC1: ranked ASes by loss, role = src|dst, drill seed for UC2.
// Supports validated sort (loss|probes|samples|last_seen|impact, default impact),
// order (asc|desc), min_probes, min_loss, and limit.
func (s *Server) asnIssues(w http.ResponseWriter, r *http.Request) {
	f := graph.ASNIssueFilter{
		Role:      r.URL.Query().Get("role"),
		MinLoss:   qFloat(r, "min_loss", 0.1),
		MinProbes: qInt64(r, "min_probes"),
		Limit:     qInt(r, "limit", 20),
		Offset:    qInt(r, "offset", 0),
		Sort:      r.URL.Query().Get("sort"),
		Order:     r.URL.Query().Get("order"),
	}
	out, err := s.graph.ASNIssues(r.Context(), f)
	if err != nil {
		slog.Warn("asn issues query failed", "err", err)
		respondError(w, http.StatusBadGateway, "graph query failed: "+err.Error())
		return
	}
	// Pagination meta: report effective limit/offset and whether more may exist.
	// We don't claim an exact total (a separate count query is expensive); the
	// frontend uses has_more (returned a full page) to offer Load more.
	hasMore := len(out) >= f.Limit
	writeJSON(w, http.StatusOK, envelope{
		Data: out,
		Meta: map[string]any{
			"limit": f.Limit, "offset": f.Offset, "sort": f.Sort,
			"order": f.Order, "has_more": hasMore,
		},
	})
}
