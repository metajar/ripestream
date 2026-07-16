package api

import (
	"log/slog"
	"net/http"
	"strconv"
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
	out, err := s.graph.Probes(r.Context(), probeFilterFromQuery(r))
	if err != nil {
		slog.Warn("probes query failed", "err", err)
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
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

// ---- Targets (UC2) ----------------------------------------------------------

func (s *Server) targets(w http.ResponseWriter, r *http.Request) {
	out, err := s.graph.Targets(r.Context(), targetFilterFromQuery(r))
	if err != nil {
		slog.Warn("targets query failed", "err", err)
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
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
