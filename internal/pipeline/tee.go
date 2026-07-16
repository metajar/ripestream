// Package pipeline provides small helpers for wiring Atlas record streams
// between sources and sinks.
package pipeline

import (
	"context"
	"log/slog"
	"sync/atomic"

	"ripestream/internal/atlas"
)

// Tee copies records from in to each of outs. If a sink channel is full the
// record is dropped for that sink only (other sinks still receive it) so a
// slow consumer cannot stall the firehose. The out channels are closed when
// in is closed or ctx is cancelled.
func Tee(ctx context.Context, in <-chan atlas.Record, outs ...chan<- atlas.Record) {
	var dropped atomic.Uint64
	go func() {
		defer func() {
			for _, out := range outs {
				close(out)
			}
			if n := dropped.Load(); n > 0 {
				slog.Warn("pipeline tee finished with drops", "dropped", n)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case r, ok := <-in:
				if !ok {
					return
				}
				for _, out := range outs {
					select {
					case out <- r:
					default:
						n := dropped.Add(1)
						if n == 1 || n%10000 == 0 {
							slog.Warn("pipeline tee dropping record (sink full)",
								"dropped_total", n)
						}
					}
				}
			}
		}
	}()
}
