package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"ripestream/internal/graph"
	"ripestream/internal/store"
)

// ---- ASN drill-down (UC2) ---------------------------------------------------

func (s *Server) asnDetail(w http.ResponseWriter, r *http.Request) {
	asn, ok := pathInt64(w, r, "asn")
	if !ok {
		return
	}
	d, err := s.graph.ASNDetail(r.Context(), asn)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondOK(w, d)
}

func (s *Server) asnProbes(w http.ResponseWriter, r *http.Request) {
	asn, ok := pathInt64(w, r, "asn")
	if !ok {
		return
	}
	out, err := s.graph.ASNProbes(r.Context(), asn, qInt(r, "limit", 50))
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
}

func (s *Server) asnTargets(w http.ResponseWriter, r *http.Request) {
	asn, ok := pathInt64(w, r, "asn")
	if !ok {
		return
	}
	out, err := s.graph.ASNTargets(r.Context(), asn, qInt(r, "limit", 50))
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
}

func (s *Server) asnTransit(w http.ResponseWriter, r *http.Request) {
	asn, ok := pathInt64(w, r, "asn")
	if !ok {
		return
	}
	out, err := s.graph.ASNTransit(r.Context(), asn, qInt(r, "limit", 50))
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
}

// ---- Probes (UC2) -----------------------------------------------------------

func (s *Server) probes(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r, 25)
	out, err := s.graph.Probes(r.Context(), probeFilterFromQuery(r))
	if err != nil {
		slog.Warn("probes query failed", "err", err)
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondPage(w, out, limit, offset)
}

func (s *Server) probeDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	d, err := s.graph.ProbeDetail(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondOK(w, d)
}

func (s *Server) probeTargets(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	limit, offset := pageParams(r, 25)
	out, err := s.graph.ProbeTargets(r.Context(), id, graph.TargetFilter{
		Limit: limit + 1, Offset: offset, Query: r.URL.Query().Get("q"),
		Sort: r.URL.Query().Get("sort"), Order: r.URL.Query().Get("order"),
	})
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondPage(w, out, limit, offset)
}

// ---- Targets (UC2) ----------------------------------------------------------

func (s *Server) targets(w http.ResponseWriter, r *http.Request) {
	filter := targetFilterFromQuery(r)
	requested := filter.Limit
	if requested <= 0 {
		requested = 50
	}
	if requested > 100 {
		requested = 100
	}
	// Pull a wider live candidate set before baseline filtering so persistent
	// non-responders cannot crowd new regressions out of the requested page.
	filter.Limit = 500
	candidates, err := s.graph.Targets(r.Context(), filter)
	if err != nil {
		slog.Warn("targets query failed", "err", err)
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	if s.ch == nil {
		respondError(w, http.StatusServiceUnavailable, "historical store unavailable")
		return
	}
	targets := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		targets = append(targets, candidate.Addr)
	}
	if len(targets) == 0 {
		respondOK(w, []targetIssueView{})
		return
	}
	baselineTo := time.Now().UTC().Add(-30 * time.Minute)
	baselines, err := s.ch.PingTargetBaselines(r.Context(), targets, baselineTo.Add(-24*time.Hour), baselineTo)
	if err != nil {
		slog.Warn("target baseline query failed", "err", err)
		respondError(w, http.StatusServiceUnavailable, "target baseline query failed")
		return
	}
	respondOK(w, targetRegressionViews(candidates, baselines, requested))
}

type targetIssueView struct {
	Addr            string  `json:"addr"`
	ASN             *int64  `json:"asn,omitempty"`
	Org             string  `json:"org,omitempty"`
	Probes          int64   `json:"probes"`
	SourceASes      int64   `json:"source_ases"`
	AvgRttMs        float64 `json:"avg_rtt_ms,omitempty"`
	LossPct         float64 `json:"loss_pct"`
	LastSeen        int64   `json:"last_seen"`
	BaselineLossPct float64 `json:"baseline_loss_pct"`
	LossChangePct   float64 `json:"loss_change_pct"`
	BaselineSamples int64   `json:"baseline_samples"`
}

func targetRegressionViews(candidates []graph.TargetInfo, baselines map[string]store.TargetBaseline, limit int) []targetIssueView {
	if limit <= 0 {
		limit = 50
	}
	out := make([]targetIssueView, 0, min(limit, len(candidates)))
	for _, candidate := range candidates {
		baseline, ok := baselines[candidate.Addr]
		if !ok || baseline.Sent <= 0 || baseline.Samples < 3 || baseline.Probes < 3 {
			continue
		}
		change := candidate.LossPct - baseline.LossPct
		// The issue list is for new degradation, not destinations that have always
		// ignored PING. Require a reasonably healthy baseline and a material jump.
		if baseline.LossPct > 20 || change < 20 {
			continue
		}
		out = append(out, targetIssueView{
			Addr: candidate.Addr, ASN: candidate.ASN, Org: candidate.Org,
			Probes: candidate.Probes, SourceASes: candidate.SourceASes,
			AvgRttMs: candidate.AvgRttMs, LossPct: candidate.LossPct, LastSeen: candidate.LastSeen,
			BaselineLossPct: baseline.LossPct, LossChangePct: change, BaselineSamples: baseline.Samples,
		})
		if len(out) == limit {
			break
		}
	}
	return out
}

func (s *Server) targetDetail(w http.ResponseWriter, r *http.Request) {
	addr := r.PathValue("addr")
	if addr == "" {
		respondError(w, http.StatusBadRequest, "missing addr")
		return
	}
	d, err := s.graph.TargetDetail(r.Context(), addr)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondOK(w, d)
}

func (s *Server) targetProbes(w http.ResponseWriter, r *http.Request) {
	addr := r.PathValue("addr")
	if addr == "" {
		respondError(w, http.StatusBadRequest, "missing addr")
		return
	}
	limit, offset := pageParams(r, 25)
	out, err := s.graph.TargetProbes(r.Context(), addr, graph.ProbeFilter{
		Limit: limit + 1, Offset: offset, Query: r.URL.Query().Get("q"),
		Sort: r.URL.Query().Get("sort"), Order: r.URL.Query().Get("order"),
	})
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondPage(w, out, limit, offset)
}

// ---- IP detail (UC2/UC3) ----------------------------------------------------

func (s *Server) ipDetail(w http.ResponseWriter, r *http.Request) {
	addr := r.PathValue("addr")
	if addr == "" {
		respondError(w, http.StatusBadRequest, "missing addr")
		return
	}
	d, err := s.graph.IPDetail(r.Context(), addr)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondOK(w, d)
}

func (s *Server) ipHops(w http.ResponseWriter, r *http.Request) {
	addr := r.PathValue("addr")
	if addr == "" {
		respondError(w, http.StatusBadRequest, "missing addr")
		return
	}
	direction := r.URL.Query().Get("direction")
	if direction != "in" && direction != "out" {
		respondError(w, http.StatusBadRequest, "direction must be in or out")
		return
	}
	limit, offset := pageParams(r, 25)
	out, err := s.graph.IPHops(r.Context(), addr, direction, graph.HopFilter{
		Limit: limit + 1, Offset: offset, Query: r.URL.Query().Get("q"),
		Sort: r.URL.Query().Get("sort"), Order: r.URL.Query().Get("order"),
	})
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondPage(w, out, limit, offset)
}

func (s *Server) ipRouteGraph(w http.ResponseWriter, r *http.Request) {
	if s.graph == nil {
		respondError(w, http.StatusServiceUnavailable, "graph not enabled")
		return
	}
	addr := r.PathValue("addr")
	if addr == "" {
		respondError(w, http.StatusBadRequest, "missing addr")
		return
	}
	out, err := s.graph.IPRouteGraph(r.Context(), addr, graph.IPRouteGraphFilter{
		MaxDepth: qInt(r, "max_depth", 15), NodeLimit: qInt(r, "node_limit", 400),
	})
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
}

// ---- path-param helpers -----------------------------------------------------

// pathInt64 reads an int64 path parameter, writing a 400 on parse failure.
func pathInt64(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	v := r.PathValue(key)
	if v == "" {
		respondError(w, http.StatusBadRequest, "missing "+key)
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid "+key)
		return 0, false
	}
	return n, true
}
