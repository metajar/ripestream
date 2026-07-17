package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ripestream/internal/graph"
	"ripestream/internal/store"
)

func TestWriteJSONNeverReturnsAnEmptyBodyOnEncodeFailure(t *testing.T) {
	res := httptest.NewRecorder()
	writeJSON(res, http.StatusOK, envelope{Data: math.NaN()})

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusInternalServerError)
	}
	var body envelope
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body.Error == "" {
		t.Fatal("response did not include an error")
	}
}

func TestGraphEndpointsDegradeWhenDisabled(t *testing.T) {
	s := New(nil, nil, nil, time.Minute)
	for _, path := range []string{"/api/overview", "/api/issues", "/api/transit/1/2/series", "/api/hops/commonality", "/api/ip/192.0.2.1/graph"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		s.Handler().ServeHTTP(res, req)
		if res.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s status = %d, want %d", path, res.Code, http.StatusServiceUnavailable)
		}
	}
}

func TestIssueRankingPrefersConfidenceBreadth(t *testing.T) {
	a := Issue{ID: "wide", Severity: "high", SampleCount: 20, LastSeen: 10}
	b := Issue{ID: "narrow", Severity: "high", SampleCount: 3, LastSeen: 20}
	if !issueLess(a, b) {
		t.Fatal("broader evidence should rank ahead of a newer narrow signal")
	}
	alert := Issue{ID: "alert", Source: "alert", Severity: "watch"}
	if !issueLess(alert, a) {
		t.Fatal("an explicit active alert should rank ahead of detected issues")
	}
}

func TestTargetRegressionViewsExcludePersistentNonResponders(t *testing.T) {
	candidates := []graph.TargetInfo{
		{Addr: "new-outage", Probes: 10, SourceASes: 5, LossPct: 100},
		{Addr: "always-silent", Probes: 20, SourceASes: 10, LossPct: 100},
		{Addr: "thin-baseline", Probes: 8, SourceASes: 4, LossPct: 80},
	}
	baselines := map[string]store.TargetBaseline{
		"new-outage":    {Sent: 300, Rcvd: 300, Samples: 100, Probes: 10, LossPct: 0},
		"always-silent": {Sent: 300, Rcvd: 0, Samples: 100, Probes: 10, LossPct: 100},
		"thin-baseline": {Sent: 3, Rcvd: 3, Samples: 1, Probes: 1, LossPct: 0},
	}

	got := targetRegressionViews(candidates, baselines, 50)
	if len(got) != 1 || got[0].Addr != "new-outage" {
		t.Fatalf("target regressions = %#v, want only new-outage", got)
	}
}
