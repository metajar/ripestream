package api

import (
	"net/http"

	"ripestream/internal/graph"
)

// ---- Hops / Transit (UC3) ---------------------------------------------------

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
