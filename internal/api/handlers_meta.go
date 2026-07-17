package api

import (
	"net/http"
)

// health is a lightweight liveness probe.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	respondOK(w, map[string]any{"status": "ok"})
}

// ingestion returns the last minute of stream reads from process memory.
func (s *Server) ingestion(w http.ResponseWriter, r *http.Request) {
	if s.ingest == nil {
		respondError(w, http.StatusServiceUnavailable, "ingestion metrics are unavailable")
		return
	}
	respondOK(w, s.ingest.Snapshot())
}

// schema describes the graph model so the UI can render field names/labels.
func (s *Server) schema(w http.ResponseWriter, r *http.Request) {
	respondOK(w, graphSchema)
}

// graphSchema documents the FalkorDB model statically (it doesn't change at runtime).
var graphSchema = map[string]any{
	"nodes": map[string]any{
		"IP":    map[string]any{"key": "addr", "props": []string{"af", "asn", "last_seen"}},
		"Probe": map[string]any{"key": "id"},
		"AS":    map[string]any{"key": "asn", "props": []string{"org"}},
	},
	"edges": map[string]any{
		"NEXT_HOP":   map[string]any{"from": "IP", "to": "IP", "props": []string{"last_rtt_ms", "seen_count", "proto", "last_seen"}},
		"PING":       map[string]any{"from": "Probe", "to": "IP", "props": []string{"avg_rtt_ms", "min_rtt_ms", "max_rtt_ms", "loss_ratio", "sent", "rcvd", "last_seen", "msm_id"}},
		"TRANSITS":   map[string]any{"from": "AS", "to": "AS", "props": []string{"seen_count", "last_seen"}},
		"TARGETS":    map[string]any{"from": "Probe", "to": "IP", "props": []string{"msm_id", "last_seen"}},
		"LOCATED_AT": map[string]any{"from": "Probe", "to": "IP"},
		"IN_AS":      map[string]any{"from": "IP", "to": "AS"},
	},
	"notes": "last_seen values are unix-epoch seconds. FalkorDB holds last-seen state; time-series history lives in ClickHouse (GET /api/timeseries).",
}
