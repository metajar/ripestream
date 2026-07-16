package graph

import (
	"strings"
	"testing"
)

func TestBuildAlertCypherUsesFreshConsensusEvidence(t *testing.T) {
	q, err := buildAlertCypher(AlertRuleSpec{Metric: "loss_ratio", Scope: "asn_dst"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"e.last_seen >= $cutoff",
		"sum(e.sent) - sum(e.rcvd)",
		"probes >= 3 AND source_ases >= 2",
	} {
		if !strings.Contains(q.cypher, want) {
			t.Errorf("query missing %q:\n%s", want, q.cypher)
		}
	}
	if strings.Contains(q.cypher, "e.loss_ratio >= $minLoss") {
		t.Fatal("default aggregate must retain healthy edges in its denominator")
	}
}

func TestBuildAlertCypherHonorsExplicitMinimumLoss(t *testing.T) {
	q, err := buildAlertCypher(AlertRuleSpec{Metric: "loss_ratio", Scope: "target", MinLoss: 0.2})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q.cypher, "e.loss_ratio >= $minLoss") {
		t.Fatal("explicit edge-level minimum loss filter is missing")
	}
}
