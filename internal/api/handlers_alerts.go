package api

import (
	"encoding/json"
	"net/http"
)

// alerterOr503 short-circuits with 503 when the alert subsystem isn't wired yet.
func (s *Server) alerterOr503(w http.ResponseWriter) (Alerter, bool) {
	if s.alerter == nil {
		respondError(w, http.StatusServiceUnavailable, "alerting subsystem not configured")
		return nil, false
	}
	return s.alerter, true
}

func (s *Server) alertsActive(w http.ResponseWriter, r *http.Request) {
	a, ok := s.alerterOr503(w)
	if !ok {
		return
	}
	out, err := a.ActiveAlerts(r.Context())
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
}

func (s *Server) alertsStates(w http.ResponseWriter, r *http.Request) {
	a, ok := s.alerterOr503(w)
	if !ok {
		return
	}
	out, err := a.AlertStates(r.Context())
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
}

func (s *Server) alertsEvents(w http.ResponseWriter, r *http.Request) {
	a, ok := s.alerterOr503(w)
	if !ok {
		return
	}
	out, err := a.AlertEvents(r.Context(), qInt(r, "limit", 100))
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
}

func (s *Server) alertsRules(w http.ResponseWriter, r *http.Request) {
	a, ok := s.alerterOr503(w)
	if !ok {
		return
	}
	out, err := a.Rules(r.Context())
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, out)
}

func (s *Server) alertGetRule(w http.ResponseWriter, r *http.Request) {
	a, ok := s.alerterOr503(w)
	if !ok {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	rule, err := a.GetRule(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondOK(w, rule)
}

func (s *Server) alertCreateRule(w http.ResponseWriter, r *http.Request) {
	a, ok := s.alerterOr503(w)
	if !ok {
		return
	}
	var rv AlertRuleView
	if err := json.NewDecoder(r.Body).Decode(&rv); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	created, err := a.CreateRule(r.Context(), rv)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(w, created)
}

func (s *Server) alertUpdateRule(w http.ResponseWriter, r *http.Request) {
	a, ok := s.alerterOr503(w)
	if !ok {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	var rv AlertRuleView
	if err := json.NewDecoder(r.Body).Decode(&rv); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	updated, err := a.UpdateRule(r.Context(), id, rv)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(w, updated)
}

func (s *Server) alertDeleteRule(w http.ResponseWriter, r *http.Request) {
	a, ok := s.alerterOr503(w)
	if !ok {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	if err := a.DeleteRule(r.Context(), id); err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
