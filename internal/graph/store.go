// Package graph writes traceroute paths and ping health into FalkorDB as a
// hybrid IP/ASN topology graph.
package graph

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/FalkorDB/falkordb-go/v2"

	"ripestream/internal/atlas"
)

// LookupASN resolves an IP to an ASN number and organization name.
// Implementations should return ok=false when the address has no mapping.
type LookupASN interface {
	Lookup(ip string) (number int64, org string, ok bool)
}

// Config configures the FalkorDB graph writer.
type Config struct {
	Addr     string // e.g. localhost:6379
	Password string
	Graph    string // graph name, default ripestream
	ASN      LookupASN
	// ActiveWindow limits health reads to recent measurements. Topology can be
	// retained longer without allowing stale probe results to look like a live
	// internet incident.
	ActiveWindow time.Duration
}

// Store is a FalkorDB client that batches topology upserts.
type Store struct {
	db           *falkordb.FalkorDB
	graph        *falkordb.Graph
	name         string
	asn          LookupASN
	activeWindow time.Duration
}

// New connects to FalkorDB and selects the named graph.
func New(cfg Config) (*Store, error) {
	if cfg.Addr == "" {
		cfg.Addr = "localhost:6379"
	}
	if cfg.Graph == "" {
		cfg.Graph = "ripestream"
	}
	if cfg.ActiveWindow <= 0 {
		cfg.ActiveWindow = 30 * time.Minute
	}
	// go-redis defaults ReadTimeout to 3s, which is SHORTER than the graph's
	// server-side query timeout (readQueryTimeoutMS, 5s). Aggregate read queries
	// over the live firehose graph regularly take 2–4s; without raising the
	// socket read timeout the connection is abandoned with "i/o timeout" before
	// the query returns a result (or its own server-side timeout). Set read/dial
	// timeouts comfortably above the query timeout, and give the pool enough
	// connections to keep reads from starving under write load.
	opt := &falkordb.ConnectionOption{
		Addr:         cfg.Addr,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		PoolSize:     20,
	}
	if cfg.Password != "" {
		opt.Password = cfg.Password
	}
	db, err := falkordb.FalkorDBNew(opt)
	if err != nil {
		return nil, fmt.Errorf("falkordb connect: %w", err)
	}
	g := db.SelectGraph(cfg.Graph)
	return &Store{
		db: db, graph: g, name: cfg.Graph, asn: cfg.ASN,
		activeWindow: cfg.ActiveWindow,
	}, nil
}

func (s *Store) activeCutoff() int64 {
	window := s.activeWindow
	if window <= 0 {
		window = 30 * time.Minute
	}
	return time.Now().Add(-window).Unix()
}

// EnsureIndexes creates range indexes used by MERGE lookups.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	queries := []string{
		"CREATE INDEX FOR (n:IP) ON (n.addr)",
		"CREATE INDEX FOR (n:Probe) ON (n.id)",
		"CREATE INDEX FOR (n:AS) ON (n.asn)",
	}
	for _, q := range queries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := s.graph.Query(q, nil, nil); err != nil {
			// Index may already exist; FalkorDB returns an error in that case.
			slog.Debug("falkordb index ensure", "query", q, "err", err)
		}
	}
	slog.Info("falkordb indexes ensured", "graph", s.name)
	return nil
}

// Ping verifies connectivity with a trivial query.
func (s *Store) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := s.graph.Query("RETURN 1", nil, nil)
	if err != nil {
		return fmt.Errorf("falkordb ping: %w", err)
	}
	return nil
}

func (s *Store) lookupASN(ip string) (number int64, org string) {
	if s == nil || s.asn == nil || ip == "" {
		return 0, ""
	}
	n, org, ok := s.asn.Lookup(ip)
	if !ok {
		return 0, ""
	}
	return n, org
}

// batch holds pending MERGE parameter rows until flush.
type batch struct {
	nextHops []map[string]any
	asLinks  []map[string]any
	probes   []map[string]any
	pings    []map[string]any
}

func (b *batch) len() int {
	return len(b.nextHops) + len(b.asLinks) + len(b.probes) + len(b.pings)
}

func (b *batch) clear() {
	b.nextHops = b.nextHops[:0]
	b.asLinks = b.asLinks[:0]
	b.probes = b.probes[:0]
	b.pings = b.pings[:0]
}

// Run drains in, converting traceroute/ping records into graph upserts and
// flushing when batchSize ops are reached or every flushInterval.
func (s *Store) Run(ctx context.Context, in <-chan atlas.Record, batchSize int, flushInterval time.Duration) error {
	if batchSize <= 0 {
		batchSize = 500
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	var b batch
	var upserted uint64
	var flushes uint64
	var skipped uint64

	flush := func(fctx context.Context) {
		if b.len() == 0 {
			return
		}
		start := time.Now()
		n := b.len()
		if err := s.flushBatch(fctx, &b); err != nil {
			if fctx.Err() != nil {
				slog.Warn("falkordb upsert skipped (context done)", "ops", n, "err", err)
			} else {
				slog.Error("falkordb upsert failed", "ops", n, "err", err)
			}
			b.clear()
			return
		}
		flushes++
		upserted += uint64(n)
		slog.Info("falkordb upsert ok",
			"ops", n, "took", time.Since(start).Round(time.Millisecond),
			"total_ops", upserted, "flushes", flushes)
		b.clear()
	}

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
		drain:
			for {
				select {
				case r, ok := <-in:
					if !ok {
						break drain
					}
					if !s.ingest(r, &b, &skipped) {
						continue
					}
				default:
					break drain
				}
			}
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			flush(shutdownCtx)
			cancel()
			slog.Info("falkordb writer stopped",
				"total_ops", upserted, "flushes", flushes, "skipped", skipped)
			return nil
		case <-ticker.C:
			flush(ctx)
		case r, ok := <-in:
			if !ok {
				flush(ctx)
				slog.Info("falkordb writer stopped",
					"total_ops", upserted, "flushes", flushes, "skipped", skipped)
				return nil
			}
			if !s.ingest(r, &b, &skipped) {
				continue
			}
			if b.len() >= batchSize {
				flush(ctx)
			}
		}
	}
}

func (s *Store) ingest(r atlas.Record, b *batch, skipped *uint64) bool {
	if !isGraphType(r) {
		*skipped++
		return false
	}
	switch r.Type {
	case "traceroute":
		if err := s.appendTraceroute(r, b); err != nil {
			slog.Debug("falkordb skip traceroute", "err", err, "msm_id", r.MsmID, "prb_id", r.PrbID)
			*skipped++
			return false
		}
	case "ping":
		if err := s.appendPing(r, b); err != nil {
			slog.Debug("falkordb skip ping", "err", err, "msm_id", r.MsmID, "prb_id", r.PrbID)
			*skipped++
			return false
		}
	}
	return true
}

func (s *Store) appendTraceroute(r atlas.Record, b *batch) error {
	hops, err := extractResponsiveHops(r.ResultJSON)
	if err != nil {
		return err
	}
	for i := range hops {
		hops[i].ASN, hops[i].Org = s.lookupASN(hops[i].Addr)
	}
	ts := r.Timestamp
	af := int(r.AF)
	proto := r.Proto

	for i := 0; i+1 < len(hops); i++ {
		a, c := hops[i], hops[i+1]
		if a.Addr == "" || c.Addr == "" || a.Addr == c.Addr {
			continue
		}
		b.nextHops = append(b.nextHops, map[string]any{
			"from":  a.Addr,
			"to":    c.Addr,
			"rtt":   c.RTT,
			"ts":    ts,
			"proto": proto,
			"af":    af,
			"asn_a": a.ASN,
			"org_a": a.Org,
			"asn_b": c.ASN,
			"org_b": c.Org,
		})
		if a.ASN > 0 && c.ASN > 0 && a.ASN != c.ASN {
			b.asLinks = append(b.asLinks, map[string]any{
				"asn_a": a.ASN,
				"org_a": a.Org,
				"asn_b": c.ASN,
				"org_b": c.Org,
				"ts":    ts,
			})
		}
	}

	if r.FromIP != "" || r.DstAddr != "" {
		fromASN, fromOrg := s.lookupASN(r.FromIP)
		dstASN, dstOrg := s.lookupASN(r.DstAddr)
		b.probes = append(b.probes, map[string]any{
			"prb_id":   int64(r.PrbID),
			"from_ip":  r.FromIP,
			"dst_addr": r.DstAddr,
			"msm_id":   int64(r.MsmID),
			"ts":       ts,
			"af":       af,
			"from_asn": fromASN,
			"from_org": fromOrg,
			"dst_asn":  dstASN,
			"dst_org":  dstOrg,
		})
	}
	return nil
}

func (s *Store) appendPing(r atlas.Record, b *batch) error {
	pl, err := parsePing(r.ResultJSON)
	if err != nil {
		return err
	}
	if r.DstAddr == "" {
		return fmt.Errorf("missing dst_addr")
	}
	fromASN, fromOrg := s.lookupASN(r.FromIP)
	dstASN, dstOrg := s.lookupASN(r.DstAddr)
	b.pings = append(b.pings, map[string]any{
		"prb_id":   int64(r.PrbID),
		"dst":      r.DstAddr,
		"from":     r.FromIP,
		"avg":      pl.Avg,
		"min":      pl.Min,
		"max":      pl.Max,
		"sent":     int64(pl.Sent),
		"rcvd":     int64(pl.Rcvd),
		"loss":     lossRatio(pl.Sent, pl.Rcvd),
		"ts":       r.Timestamp,
		"msm_id":   int64(r.MsmID),
		"af":       int(r.AF),
		"from_asn": fromASN,
		"from_org": fromOrg,
		"dst_asn":  dstASN,
		"dst_org":  dstOrg,
	})
	return nil
}

func mapsToIface(rows []map[string]any) []any {
	out := make([]any, len(rows))
	for i := range rows {
		out[i] = rows[i]
	}
	return out
}

func appendInAS(dst *[]map[string]any, addr string, asn int64, org string) {
	if addr == "" || asn <= 0 {
		return
	}
	*dst = append(*dst, map[string]any{
		"addr": addr,
		"asn":  asn,
		"org":  org,
	})
}

func (s *Store) flushInAS(ops []map[string]any) error {
	if len(ops) == 0 {
		return nil
	}
	q := `
UNWIND $ops AS op
MERGE (ip:IP {addr: op.addr})
SET ip.asn = op.asn
MERGE (asnNode:AS {asn: op.asn})
  ON CREATE SET asnNode.org = op.org
  ON MATCH SET asnNode.org = CASE WHEN op.org <> '' THEN op.org ELSE asnNode.org END
MERGE (ip)-[:IN_AS]->(asnNode)
`
	if _, err := s.graph.Query(q, map[string]any{"ops": mapsToIface(ops)}, nil); err != nil {
		return fmt.Errorf("in_as upsert: %w", err)
	}
	return nil
}

func (s *Store) flushBatch(ctx context.Context, b *batch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(b.nextHops) > 0 {
		q := `
UNWIND $ops AS op
MERGE (a:IP {addr: op.from})
  ON CREATE SET a.af = op.af, a.last_seen = op.ts
  ON MATCH SET a.last_seen = op.ts
MERGE (c:IP {addr: op.to})
  ON CREATE SET c.af = op.af, c.last_seen = op.ts
  ON MATCH SET c.last_seen = op.ts
MERGE (a)-[e:NEXT_HOP]->(c)
  ON CREATE SET e.last_rtt_ms = op.rtt, e.last_seen = op.ts, e.proto = op.proto, e.seen_count = 1
  ON MATCH SET e.last_rtt_ms = op.rtt, e.last_seen = op.ts, e.proto = op.proto, e.seen_count = coalesce(e.seen_count, 0) + 1
`
		if _, err := s.graph.Query(q, map[string]any{"ops": mapsToIface(b.nextHops)}, nil); err != nil {
			return fmt.Errorf("next_hop upsert: %w", err)
		}
		inAS := make([]map[string]any, 0, len(b.nextHops)*2)
		for _, op := range b.nextHops {
			asnA, _ := op["asn_a"].(int64)
			orgA, _ := op["org_a"].(string)
			asnB, _ := op["asn_b"].(int64)
			orgB, _ := op["org_b"].(string)
			from, _ := op["from"].(string)
			to, _ := op["to"].(string)
			appendInAS(&inAS, from, asnA, orgA)
			appendInAS(&inAS, to, asnB, orgB)
		}
		if err := s.flushInAS(inAS); err != nil {
			return err
		}
	}
	if len(b.asLinks) > 0 {
		q := `
UNWIND $ops AS op
MERGE (a:AS {asn: op.asn_a})
  ON CREATE SET a.org = op.org_a
  ON MATCH SET a.org = CASE WHEN op.org_a <> '' THEN op.org_a ELSE a.org END
MERGE (b:AS {asn: op.asn_b})
  ON CREATE SET b.org = op.org_b
  ON MATCH SET b.org = CASE WHEN op.org_b <> '' THEN op.org_b ELSE b.org END
MERGE (a)-[e:TRANSITS]->(b)
  ON CREATE SET e.last_seen = op.ts, e.seen_count = 1
  ON MATCH SET e.last_seen = op.ts, e.seen_count = coalesce(e.seen_count, 0) + 1
`
		if _, err := s.graph.Query(q, map[string]any{"ops": mapsToIface(b.asLinks)}, nil); err != nil {
			return fmt.Errorf("transits upsert: %w", err)
		}
	}
	if len(b.probes) > 0 {
		located := make([]map[string]any, 0, len(b.probes))
		targets := make([]map[string]any, 0, len(b.probes))
		inAS := make([]map[string]any, 0, len(b.probes)*2)
		for _, op := range b.probes {
			fromIP, _ := op["from_ip"].(string)
			dst, _ := op["dst_addr"].(string)
			fromASN, _ := op["from_asn"].(int64)
			fromOrg, _ := op["from_org"].(string)
			dstASN, _ := op["dst_asn"].(int64)
			dstOrg, _ := op["dst_org"].(string)
			if fromIP != "" {
				located = append(located, map[string]any{
					"prb_id": op["prb_id"], "from_ip": fromIP,
					"ts": op["ts"], "af": op["af"],
					"from_asn": fromASN, "from_org": fromOrg,
				})
				appendInAS(&inAS, fromIP, fromASN, fromOrg)
			}
			if dst != "" {
				targets = append(targets, map[string]any{
					"prb_id": op["prb_id"], "dst_addr": dst,
					"msm_id": op["msm_id"], "ts": op["ts"], "af": op["af"],
				})
				appendInAS(&inAS, dst, dstASN, dstOrg)
			}
		}
		if len(located) > 0 {
			q := `
UNWIND $ops AS op
MERGE (p:Probe {id: op.prb_id})
SET p.last_seen = CASE WHEN coalesce(p.last_seen, 0) > op.ts THEN p.last_seen ELSE op.ts END
SET p.source_ip = op.from_ip, p.source_asn = op.from_asn, p.source_org = op.from_org
WITH p, op
OPTIONAL MATCH (p)-[old:LOCATED_AT]->(oldSrc:IP)
WHERE oldSrc.addr <> op.from_ip
DELETE old
WITH p, op
MERGE (src:IP {addr: op.from_ip})
  ON CREATE SET src.af = op.af, src.last_seen = op.ts
  ON MATCH SET src.last_seen = CASE WHEN src.last_seen > op.ts THEN src.last_seen ELSE op.ts END
MERGE (p)-[loc:LOCATED_AT]->(src)
SET loc.last_seen = CASE WHEN coalesce(loc.last_seen, 0) > op.ts THEN loc.last_seen ELSE op.ts END
`
			if _, err := s.graph.Query(q, map[string]any{"ops": mapsToIface(located)}, nil); err != nil {
				return fmt.Errorf("located_at upsert: %w", err)
			}
		}
		if len(targets) > 0 {
			q := `
UNWIND $ops AS op
MERGE (p:Probe {id: op.prb_id})
SET p.last_seen = CASE WHEN coalesce(p.last_seen, 0) > op.ts THEN p.last_seen ELSE op.ts END
MERGE (dst:IP {addr: op.dst_addr})
  ON CREATE SET dst.af = op.af, dst.last_seen = op.ts
  ON MATCH SET dst.last_seen = op.ts
MERGE (p)-[t:TARGETS]->(dst)
  ON CREATE SET t.msm_id = op.msm_id, t.last_seen = op.ts
  ON MATCH SET t.msm_id = op.msm_id, t.last_seen = op.ts
`
			if _, err := s.graph.Query(q, map[string]any{"ops": mapsToIface(targets)}, nil); err != nil {
				return fmt.Errorf("targets upsert: %w", err)
			}
		}
		if err := s.flushInAS(inAS); err != nil {
			return err
		}
	}
	if len(b.pings) > 0 {
		q := `
UNWIND $ops AS op
MERGE (p:Probe {id: op.prb_id})
SET p.last_seen = CASE WHEN coalesce(p.last_seen, 0) > op.ts THEN p.last_seen ELSE op.ts END
MERGE (t:IP {addr: op.dst})
  ON CREATE SET t.af = op.af, t.last_seen = op.ts
  ON MATCH SET t.last_seen = op.ts
MERGE (p)-[e:PING]->(t)
  ON CREATE SET
    e.avg_rtt_ms = op.avg, e.min_rtt_ms = op.min, e.max_rtt_ms = op.max,
    e.loss_ratio = op.loss, e.sent = op.sent, e.rcvd = op.rcvd,
    e.last_seen = op.ts, e.msm_id = op.msm_id
  ON MATCH SET
    e.avg_rtt_ms = op.avg, e.min_rtt_ms = op.min, e.max_rtt_ms = op.max,
    e.loss_ratio = op.loss, e.sent = op.sent, e.rcvd = op.rcvd,
    e.last_seen = op.ts, e.msm_id = op.msm_id
`
		if _, err := s.graph.Query(q, map[string]any{"ops": mapsToIface(b.pings)}, nil); err != nil {
			return fmt.Errorf("ping upsert: %w", err)
		}
		located := make([]map[string]any, 0, len(b.pings))
		inAS := make([]map[string]any, 0, len(b.pings)*2)
		for _, op := range b.pings {
			from, _ := op["from"].(string)
			dst, _ := op["dst"].(string)
			fromASN, _ := op["from_asn"].(int64)
			fromOrg, _ := op["from_org"].(string)
			dstASN, _ := op["dst_asn"].(int64)
			dstOrg, _ := op["dst_org"].(string)
			appendInAS(&inAS, dst, dstASN, dstOrg)
			if from == "" {
				continue
			}
			located = append(located, map[string]any{
				"prb_id": op["prb_id"], "from_ip": from,
				"ts": op["ts"], "af": op["af"],
				"from_asn": fromASN, "from_org": fromOrg,
			})
			appendInAS(&inAS, from, fromASN, fromOrg)
		}
		if len(located) > 0 {
			q = `
UNWIND $ops AS op
MERGE (p:Probe {id: op.prb_id})
SET p.last_seen = CASE WHEN coalesce(p.last_seen, 0) > op.ts THEN p.last_seen ELSE op.ts END
SET p.source_ip = op.from_ip, p.source_asn = op.from_asn, p.source_org = op.from_org
WITH p, op
OPTIONAL MATCH (p)-[old:LOCATED_AT]->(oldSrc:IP)
WHERE oldSrc.addr <> op.from_ip
DELETE old
WITH p, op
MERGE (src:IP {addr: op.from_ip})
  ON CREATE SET src.af = op.af, src.last_seen = op.ts
  ON MATCH SET src.last_seen = CASE WHEN src.last_seen > op.ts THEN src.last_seen ELSE op.ts END
MERGE (p)-[loc:LOCATED_AT]->(src)
SET loc.last_seen = CASE WHEN coalesce(loc.last_seen, 0) > op.ts THEN loc.last_seen ELSE op.ts END
`
			if _, err := s.graph.Query(q, map[string]any{"ops": mapsToIface(located)}, nil); err != nil {
				return fmt.Errorf("ping located_at upsert: %w", err)
			}
		}
		if err := s.flushInAS(inAS); err != nil {
			return err
		}
	}
	return nil
}
