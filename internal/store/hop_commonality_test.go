package store

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBuildHopCommonalityQueryIsBoundedAndExcludesDestination(t *testing.T) {
	q := buildHopCommonalityQuery("ripestream", "atlas_results", 100, 200, 300, 3, 40)
	for _, want := range []string{
		"timestamp >= toDateTime(100)",
		"timestamp < toDateTime(300)",
		"addr != dst_addr",
		"HAVING probes >= 3",
		"baseline_samples >= 12",
		"LIMIT 40",
		"uniqCombined64If(prb_id",
		"SETTINGS max_threads = 2, max_block_size = 2048",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q", want)
		}
	}
}

func TestHopCommonalitiesDecodesEvidence(t *testing.T) {
	s := New(Config{BaseURL: "http://clickhouse.test"})
	s.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		query, err := url.QueryUnescape(r.URL.Query().Get("query"))
		if err != nil || !strings.Contains(query, "FORMAT JSON") {
			t.Errorf("unexpected query: %q (%v)", query, err)
		}
		body := `{"data":[{"addr":"203.0.113.9","recent_rtt":91.2,"baseline_rtt":40.1,"delta_ms":51.1,"change_pct":127.4,"impact_score":250.3,"recent_samples":42,"baseline_samples":180,"probes":5,"targets":3,"traces":19,"affected_probes":[10,11,12],"affected_targets":["1.1.1.1","8.8.8.8"],"first_seen":"2026-07-16 12:00:00","last_seen":"2026-07-16 12:29:00"}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	rows, err := s.HopCommonalities(context.Background(), time.Date(2026, 7, 16, 12, 30, 0, 0, time.UTC), HopCommonalityFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	got := rows[0]
	if got.Addr != "203.0.113.9" || got.Probes != 5 || got.RttDeltaMs != 51.1 {
		t.Fatalf("decoded row = %#v", got)
	}
	if len(got.AffectedProbes) != 3 || len(got.AffectedTargets) != 2 {
		t.Fatalf("decoded evidence = probes %#v targets %#v", got.AffectedProbes, got.AffectedTargets)
	}
	if got.LastSeen <= got.FirstSeen {
		t.Fatalf("time range = %d..%d", got.FirstSeen, got.LastSeen)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
