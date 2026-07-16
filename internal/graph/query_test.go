package graph

import (
	"math"
	"testing"
)

func TestAsFloatRejectsNonFiniteValues(t *testing.T) {
	for name, value := range map[string]float64{
		"nan":     math.NaN(),
		"pos_inf": math.Inf(1),
		"neg_inf": math.Inf(-1),
	} {
		t.Run(name, func(t *testing.T) {
			if got := asFloat(value); got != 0 {
				t.Fatalf("asFloat(%s) = %v, want 0", name, got)
			}
		})
	}
}

// TestASNIssueMapping verifies the row → ASNIssue mapping handles both populated
// and nil/missing last_seen values (FalkorDB returns nil for missing aggregates).
func TestASNIssueMapping(t *testing.T) {
	// Populated last_seen.
	r := map[string]any{
		"asn": int64(13335), "org": "Cloudflare", "samples": int64(50),
		"probes": int64(10), "avg_loss_pct": 92.0, "max_loss_pct": 100.0,
		"avg_rtt_ms": 140.0, "last_seen": int64(1784221304),
	}
	issue := ASNIssue{
		ASN: asInt(r["asn"]), Org: asString(r["org"]), Role: "dst",
		Samples: asInt(r["samples"]), Probes: asInt(r["probes"]),
		AvgLossPct: asFloat(r["avg_loss_pct"]), MaxLossPct: asFloat(r["max_loss_pct"]),
		AvgRttMs: asFloat(r["avg_rtt_ms"]),
		LastSeen: asInt(r["last_seen"]),
	}
	if issue.LastSeen != 1784221304 {
		t.Errorf("LastSeen = %d, want 1784221304", issue.LastSeen)
	}
	if issue.AvgLossPct != 92.0 {
		t.Errorf("AvgLossPct = %g, want 92", issue.AvgLossPct)
	}

	// Missing last_seen (nil) maps to 0, not a panic.
	rNil := map[string]any{
		"asn": int64(1), "org": "X", "samples": int64(1),
		"probes": int64(1), "avg_loss_pct": 50.0, "max_loss_pct": 50.0,
		"avg_rtt_ms": 10.0, "last_seen": nil,
	}
	issue2 := ASNIssue{LastSeen: asInt(rNil["last_seen"])}
	if issue2.LastSeen != 0 {
		t.Errorf("nil LastSeen = %d, want 0", issue2.LastSeen)
	}
}

// TestASNPairIssueMapping verifies pair aggregates carry last_seen.
func TestASNPairIssueMapping(t *testing.T) {
	r := map[string]any{
		"sasn": int64(701), "sorg": "Verizon", "dasn": int64(13335), "dorg": "Cloudflare",
		"samples": int64(860), "probes": int64(43),
		"avg_loss_pct": 92.0, "max_loss_pct": 100.0, "avg_rtt_ms": 140.0,
		"last_seen": int64(1784221304),
	}
	p := ASNPairIssue{
		SrcASN: asInt(r["sasn"]), SrcOrg: asString(r["sorg"]),
		DstASN: asInt(r["dasn"]), DstOrg: asString(r["dorg"]),
		Samples: asInt(r["samples"]), Probes: asInt(r["probes"]),
		AvgLossPct: asFloat(r["avg_loss_pct"]), MaxLossPct: asFloat(r["max_loss_pct"]),
		AvgRttMs: asFloat(r["avg_rtt_ms"]),
		LastSeen: asInt(r["last_seen"]),
	}
	if p.LastSeen != 1784221304 {
		t.Errorf("pair LastSeen = %d, want 1784221304", p.LastSeen)
	}
}

// TestAsIntCoercion confirms the scalar extractor handles the FalkorDB value
// types (int64, float64, nil) that appear in query results.
func TestAsIntCoercion(t *testing.T) {
	cases := []struct {
		in   any
		want int64
	}{
		{int64(42), 42},
		{float64(42), 42},
		{nil, 0},
		{int64(0), 0},
	}
	for _, c := range cases {
		got := asInt(c.in)
		if got != c.want {
			t.Errorf("asInt(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
