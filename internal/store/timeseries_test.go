package store

import (
	"strings"
	"testing"
	"time"
)

func TestBuildPingPairSeriesQueryScopesAndEscapes(t *testing.T) {
	from := time.Unix(1000, 0).UTC()
	to := from.Add(time.Hour)
	q, err := buildPingPairSeriesQuery("ripestream", "atlas_results",
		[]int64{9, 2, 9, -1}, []string{"192.0.2.1", "x' OR 1=1 --"}, from, to, 60)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"prb_id IN (2,9)",
		"dst_addr IN ('192.0.2.1','x\\' OR 1=1 --')",
		"JSONExtractString(result_json,'avg')",
		"uniqExact(prb_id)",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q:\n%s", want, q)
		}
	}
}

func TestBuildPingPairSeriesQueryRequiresScope(t *testing.T) {
	if _, err := buildPingPairSeriesQuery("db", "table", nil, []string{"192.0.2.1"}, time.Now().Add(-time.Hour), time.Now(), 60); err == nil {
		t.Fatal("expected missing probes to fail")
	}
}
