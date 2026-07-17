package graph

import "testing"

// TestValidatedSort_InjectionProof verifies that arbitrary/untrusted sort input
// can never reach the Cypher ORDER BY clause — it must fall back to the default.
func TestValidatedSort_InjectionProof(t *testing.T) {
	cases := []struct {
		sort string
		want string
	}{
		{"loss", "avg_loss"},
		{"probes", "probes"},
		{"samples", "samples"},
		{"last_seen", "last_seen"},
		{"impact", "impact"},
		// Injection attempts must fall back to the default ("impact" here).
		{"avg_loss; DROP", "impact"},
		{"1=1", "impact"},
		{"", "impact"},
		{"bogus", "impact"},
	}
	for _, c := range cases {
		got := validatedSort(c.sort, "impact", asnIssueSortExpr)
		if got != c.want {
			t.Errorf("validatedSort(%q) = %q, want %q", c.sort, got, c.want)
		}
	}
}

// TestSortDir verifies direction normalization.
func TestSortDir(t *testing.T) {
	if sortDir("asc") != "ASC" {
		t.Error(`sortDir("asc") should be ASC`)
	}
	if sortDir("desc") != "DESC" {
		t.Error(`sortDir("desc") should be DESC`)
	}
	if sortDir("") != "DESC" {
		t.Error(`sortDir("") should default to DESC`)
	}
	if sortDir("malicious") != "DESC" {
		t.Error(`sortDir("malicious") should fall back to DESC`)
	}
}

func TestTransitAndHopSortsAreWhitelisted(t *testing.T) {
	if got := validatedSort("observations", "sc", transitSortExpr); got != "sc" {
		t.Fatalf("transit observations sort = %q", got)
	}
	if got := validatedSort("rtt", "rtt", hopSortExpr); got != "rtt" {
		t.Fatalf("hop RTT sort = %q", got)
	}
	if got := validatedSort("rtt; DELETE", "sc", transitSortExpr); got != "sc" {
		t.Fatalf("unsafe transit sort did not fall back: %q", got)
	}
}
