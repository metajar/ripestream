package graph

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/FalkorDB/falkordb-go/v2"
)

const (
	defaultPruneInterval = 15 * time.Minute
	maxPruneBatches      = 100
	pruneQueryTimeoutMS  = 8_000
)

// RunJanitor bounds the live topology graph. ClickHouse remains the durable
// history; FalkorDB intentionally contains only recent, queryable state.
func (s *Store) RunJanitor(ctx context.Context, retention, interval time.Duration, batchSize int) error {
	if retention <= 0 {
		return nil
	}
	if interval <= 0 {
		interval = defaultPruneInterval
	}
	if batchSize < 100 || batchSize > 10_000 {
		batchSize = 1_000
	}

	prune := func() {
		start := time.Now()
		cutoff := time.Now().Add(-retention).Unix()
		stats, err := s.Prune(ctx, cutoff, batchSize)
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("falkordb prune failed", "err", err, "cutoff", cutoff)
			}
			return
		}
		slog.Info("falkordb prune complete",
			"relationships", stats.Relationships, "nodes", stats.Nodes,
			"cutoff", cutoff, "took", time.Since(start).Round(time.Millisecond))
	}

	prune()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			prune()
		}
	}
}

type PruneStats struct {
	Relationships int
	Nodes         int
}

// Prune deletes stale observations in bounded batches, followed by nodes that
// no longer participate in the retained graph. The fixed query list prevents
// user-controlled Cypher from entering this maintenance path.
func (s *Store) Prune(ctx context.Context, cutoff int64, batchSize int) (PruneStats, error) {
	if cutoff <= 0 {
		return PruneStats{}, fmt.Errorf("invalid prune cutoff %d", cutoff)
	}
	if batchSize < 1 || batchSize > 10_000 {
		return PruneStats{}, fmt.Errorf("invalid prune batch size %d", batchSize)
	}
	params := map[string]any{"cutoff": cutoff, "limit": batchSize}
	var out PruneStats

	relQueries := []string{
		`MATCH ()-[e:PING]->() WHERE coalesce(e.last_seen, 0) < $cutoff WITH e LIMIT $limit DELETE e`,
		`MATCH ()-[e:TARGETS]->() WHERE coalesce(e.last_seen, 0) < $cutoff WITH e LIMIT $limit DELETE e`,
		`MATCH ()-[e:NEXT_HOP]->() WHERE coalesce(e.last_seen, 0) < $cutoff WITH e LIMIT $limit DELETE e`,
		`MATCH ()-[e:TRANSITS]->() WHERE coalesce(e.last_seen, 0) < $cutoff WITH e LIMIT $limit DELETE e`,
		`MATCH ()-[e:LOCATED_AT]->(ip:IP) WHERE coalesce(e.last_seen, ip.last_seen, 0) < $cutoff WITH e LIMIT $limit DELETE e`,
	}
	for _, q := range relQueries {
		n, err := s.pruneBatches(ctx, q, params, true, batchSize)
		if err != nil {
			return out, err
		}
		out.Relationships += n
	}

	nodeQueries := []string{
		`MATCH (p:Probe) WHERE coalesce(p.last_seen, 0) < $cutoff AND NOT (p)-[:PING]->() AND NOT (p)-[:TARGETS]->() WITH p LIMIT $limit DETACH DELETE p`,
		`MATCH (ip:IP) WHERE coalesce(ip.last_seen, 0) < $cutoff AND NOT (ip)<-[:PING]-() AND NOT (ip)<-[:TARGETS]-() AND NOT (ip)-[:NEXT_HOP]->() AND NOT (ip)<-[:NEXT_HOP]-() WITH ip LIMIT $limit DETACH DELETE ip`,
		`MATCH (a:AS) WHERE NOT (a)<-[:IN_AS]-() AND NOT (a)-[:TRANSITS]->() AND NOT (a)<-[:TRANSITS]-() WITH a LIMIT $limit DELETE a`,
	}
	for _, q := range nodeQueries {
		n, err := s.pruneBatches(ctx, q, params, false, batchSize)
		if err != nil {
			return out, err
		}
		out.Nodes += n
	}
	return out, nil
}

func (s *Store) pruneBatches(ctx context.Context, query string, params map[string]any, relationships bool, batchSize int) (int, error) {
	total := 0
	options := falkordb.NewQueryOptions().SetTimeout(pruneQueryTimeoutMS)
	for batch := 0; batch < maxPruneBatches; batch++ {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		res, err := s.graph.Query(query, params, options)
		if err != nil {
			return total, fmt.Errorf("prune query: %w", err)
		}
		n := res.NodesDeleted()
		if relationships {
			n = res.RelationshipsDeleted()
		}
		total += n
		if n < batchSize {
			return total, nil
		}
	}
	slog.Warn("falkordb prune category hit batch cap", "batch_size", batchSize, "deleted", total)
	return total, nil
}
