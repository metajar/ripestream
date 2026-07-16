package api

import (
	"net/http"
	"strconv"
	"time"
)

// tsWrite handles /api/timeseries: bucketed ping-metric history from ClickHouse.
// Query params: metric (loss_pct|avg_rtt_ms|min_rtt_ms|max_rtt_ms), probe, target,
// from (unix), to (unix), buckets.
func tsWrite(w http.ResponseWriter, r *http.Request, s *Server, metric string) {
	var from, to time.Time
	if v := r.URL.Query().Get("from"); v != "" {
		if sec, err := strconv.ParseInt(v, 10, 64); err == nil {
			from = time.Unix(sec, 0).UTC()
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if sec, err := strconv.ParseInt(v, 10, 64); err == nil {
			to = time.Unix(sec, 0).UTC()
		}
	}
	probe := qInt64(r, "probe")
	target := r.URL.Query().Get("target")
	buckets := qInt(r, "buckets", 60)

	pts, err := s.ch.PingSeries(r.Context(), metric, probe, target, from, to, buckets)
	if err != nil {
		respondError(w, http.StatusBadGateway, "clickhouse query failed: "+err.Error())
		return
	}
	respondOK(w, pts)
}
