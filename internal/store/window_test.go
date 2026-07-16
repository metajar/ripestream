package store

import (
	"strings"
	"testing"
	"time"
)

func TestBuildPingTargetBaselinesQueryScopesAndEscapes(t *testing.T) {
	from := time.Unix(100, 0).UTC()
	to := time.Unix(200, 0).UTC()
	q, targets, err := buildPingTargetBaselinesQuery("ripestream", "atlas_results",
		[]string{"192.0.2.1", "bad'addr", "192.0.2.1"}, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %#v, want 2 unique values", targets)
	}
	for _, want := range []string{"FROM ripestream.atlas_results", "timestamp >= toDateTime(100)", "timestamp < toDateTime(200)", "'bad\\'addr'"} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q:\n%s", want, q)
		}
	}
}

func TestBuildPingTargetBaselinesQueryRequiresScope(t *testing.T) {
	if _, _, err := buildPingTargetBaselinesQuery("db", "table", nil, time.Unix(100, 0), time.Unix(200, 0)); err == nil {
		t.Fatal("expected empty target scope to fail")
	}
}
