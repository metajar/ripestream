package atlas

import (
	"context"
	"testing"
)

func TestHandleLineProjectsTypedPingMetrics(t *testing.T) {
	line := `["atlas_result",{"timestamp":123,"msm_id":456,"prb_id":789,"type":"ping","sent":3,"rcvd":2,"avg":12.5,"min":10.25,"max":15.75}]`
	out := make(chan Record, 1)
	n, err := handleLine(line, out, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("records sent = %d, want 1", n)
	}
	rec := <-out
	if rec.Sent != 3 || rec.Rcvd != 2 || rec.AvgRttMs != 12.5 || rec.MinRttMs != 10.25 || rec.MaxRttMs != 15.75 {
		t.Fatalf("typed ping projection = %+v", rec)
	}
}
