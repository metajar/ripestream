package pipeline

import (
	"context"
	"sync/atomic"
	"testing"

	"ripestream/internal/atlas"
)

func TestTeeObservedCountsInputOnceAcrossMultipleSinks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan atlas.Record, 1)
	outA := make(chan atlas.Record, 1)
	outB := make(chan atlas.Record, 1)
	var observed atomic.Int64

	TeeObserved(ctx, in, func() { observed.Add(1) }, outA, outB)
	in <- atlas.Record{MsmID: 123}
	close(in)

	if got := (<-outA).MsmID; got != 123 {
		t.Fatalf("sink A measurement = %d, want 123", got)
	}
	if got := (<-outB).MsmID; got != 123 {
		t.Fatalf("sink B measurement = %d, want 123", got)
	}
	if got := observed.Load(); got != 1 {
		t.Fatalf("observations = %d, want 1", got)
	}
}
