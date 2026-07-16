package api

import (
	"net/http"
	"strconv"
	"time"
)

// windowSummary -> GET /api/window?target=&probe=&from=&to=
// Returns loss/RTT aggregates over [from,to) with a prior-window baseline.
// Validates the range (max 30 days); returns baseline_available=false when no
// prior data exists.
func (s *Server) windowSummary(w http.ResponseWriter, r *http.Request) {
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

	ws, err := s.ch.PingWindowSummary(r.Context(), target, probe, from, to)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(w, ws)
}
