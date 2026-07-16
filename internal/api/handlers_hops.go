package api

import (
	"log/slog"
	"net/http"
	"time"

	"ripestream/internal/graph"
	"ripestream/internal/store"
)

// ---- Hops / Transit (UC3) ---------------------------------------------------

type hopCommonalityView struct {
	store.HopCommonality
	ASN      *int64 `json:"asn,omitempty"`
	Org      string `json:"org,omitempty"`
	Incoming int64  `json:"incoming"`
	Outgoing int64  `json:"outgoing"`
}

type hopCommonalityResponse struct {
	GeneratedAt   int64                `json:"generated_at"`
	RecentMinutes int64                `json:"recent_minutes"`
	BaselineHours int64                `json:"baseline_hours"`
	Candidates    []hopCommonalityView `json:"candidates"`
}

func (s *Server) hopCommonalities(w http.ResponseWriter, r *http.Request) {
	if s.ch == nil {
		respondError(w, http.StatusServiceUnavailable, "ClickHouse is required")
		return
	}
	recent := 30 * time.Minute
	if requested, valid := parseRange(r.URL.Query().Get("range")); valid {
		recent = requested
	}
	if recent < 5*time.Minute {
		recent = 5 * time.Minute
	}
	if recent > 6*time.Hour {
		recent = 6 * time.Hour
	}
	const baseline = 24 * time.Hour
	now := time.Now().UTC()
	rows, err := s.ch.HopCommonalities(r.Context(), now, store.HopCommonalityFilter{
		Recent: recent, Baseline: baseline, MinProbes: qInt(r, "min_probes", 3), Limit: qInt(r, "limit", 40),
	})
	if err != nil {
		respondError(w, http.StatusBadGateway, "route correlation query failed: "+err.Error())
		return
	}

	contexts := map[string]graph.HopContext{}
	if s.graph != nil && len(rows) > 0 {
		addrs := make([]string, 0, len(rows))
		for _, row := range rows {
			addrs = append(addrs, row.Addr)
		}
		if enriched, enrichErr := s.graph.HopContexts(r.Context(), addrs); enrichErr != nil {
			slog.Warn("hop commonality graph enrichment failed", "err", enrichErr)
		} else {
			contexts = enriched
		}
	}

	views := make([]hopCommonalityView, 0, len(rows))
	for _, row := range rows {
		view := hopCommonalityView{HopCommonality: row}
		if c, ok := contexts[row.Addr]; ok {
			view.ASN, view.Org, view.Incoming, view.Outgoing = c.ASN, c.Org, c.Incoming, c.Outgoing
		}
		views = append(views, view)
	}
	respondOK(w, hopCommonalityResponse{
		GeneratedAt: now.Unix(), RecentMinutes: int64(recent / time.Minute),
		BaselineHours: int64(baseline / time.Hour), Candidates: views,
	})
}

func (s *Server) hotHops(w http.ResponseWriter, r *http.Request) {
	out, err := s.graph.HotHops(r.Context(), graph.HopFilter{
		MinRtt: qFloat(r, "min_rtt", 100),
		Limit:  qInt(r, "limit", 50),
	})
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
}

func (s *Server) transitEdges(w http.ResponseWriter, r *http.Request) {
	out, err := s.graph.TransitEdges(r.Context(), graph.TransitFilter{
		Limit: qInt(r, "limit", 50),
	})
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
}

func (s *Server) transitPairDetail(w http.ResponseWriter, r *http.Request) {
	a, ok := pathInt64(w, r, "asnA")
	if !ok {
		return
	}
	b, ok := pathInt64(w, r, "asnB")
	if !ok {
		return
	}
	d, err := s.graph.TransitPairDetail(r.Context(), a, b)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondOK(w, d)
}

func (s *Server) transitPairSeries(w http.ResponseWriter, r *http.Request) {
	if s.graph == nil || s.ch == nil {
		respondError(w, http.StatusServiceUnavailable, "graph and ClickHouse are required")
		return
	}
	a, ok := pathInt64(w, r, "asnA")
	if !ok {
		return
	}
	b, ok := pathInt64(w, r, "asnB")
	if !ok {
		return
	}

	tests, err := s.graph.TransitPairTests(r.Context(), a, b, 500)
	if err != nil {
		respondError(w, http.StatusBadGateway, "graph query failed: "+err.Error())
		return
	}
	if len(tests) == 0 {
		respondOK(w, []any{})
		return
	}
	probes := make([]int64, 0, len(tests))
	targets := make([]string, 0, len(tests))
	for _, test := range tests {
		probes = append(probes, test.ProbeID)
		targets = append(targets, test.TargetIP)
	}

	duration := 24 * time.Hour
	if requested, valid := parseRange(r.URL.Query().Get("range")); valid {
		duration = requested
	}
	to := time.Now().UTC()
	points, err := s.ch.PingPairSeries(r.Context(), probes, targets, to.Add(-duration), to, qInt(r, "buckets", 60))
	if err != nil {
		respondError(w, http.StatusBadGateway, "clickhouse query failed: "+err.Error())
		return
	}
	respondOK(w, points)
}

// ---- Compete-with-TE views --------------------------------------------------

func (s *Server) path(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	dst := r.URL.Query().Get("dst")
	if src == "" || dst == "" {
		respondError(w, http.StatusBadRequest, "src and dst query params required")
		return
	}
	p, err := s.graph.Path(r.Context(), src, dst, qInt(r, "max_hops", 15))
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, p)
}

// pathDestinations returns the endpoints reachable from a chosen source, grouped
// by AS — used to constrain the Path Explorer destination picker to only
// destinations that actually connect to the source.
func (s *Server) pathDestinations(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	if src == "" {
		respondError(w, http.StatusBadRequest, "src query param required")
		return
	}
	groups, err := s.graph.ReachableDestinations(r.Context(), src)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, groups)
}

func (s *Server) subgraph(w http.ResponseWriter, r *http.Request) {
	f := graph.SubgraphFilter{
		ASN:    qInt64(r, "asn"),
		Probe:  qInt64(r, "probe"),
		Target: r.URL.Query().Get("target"),
		Depth:  qInt(r, "depth", 2),
		Limit:  qInt(r, "limit", 80),
	}
	sg, err := s.graph.Subgraph(r.Context(), f)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, sg)
}

func (s *Server) timeseries(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "loss_pct"
	}
	tsWrite(w, r, s, metric)
}

// search -> typeahead lookup for the Path Explorer (and any picker).
// Returns AS/IP/probe hits matching an org name, ASN, IP prefix, or probe id.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		respondOK(w, []any{})
		return
	}
	out, err := s.graph.Search(r.Context(), q, qInt(r, "limit", 12))
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
}
