package graph

import (
	"context"
	"os"
	"testing"
	"time"

	"ripestream/internal/asn"
	"ripestream/internal/atlas"
)

func TestLiveUpsert(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg := Config{Addr: "localhost:6379", Graph: "ripestream_test"}
	if db := openASN(t); db != nil {
		defer db.Close()
		cfg.ASN = db
	}

	s, err := New(cfg)
	if err != nil {
		t.Skipf("falkordb unavailable: %v", err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Skipf("falkordb ping failed: %v", err)
	}
	if err := s.EnsureIndexes(ctx); err != nil {
		t.Fatalf("indexes: %v", err)
	}

	in := make(chan atlas.Record, 4)
	in <- atlas.Record{
		Type: "traceroute", Timestamp: time.Now().Unix(), MsmID: 1, PrbID: 42,
		FromIP: "8.8.8.8", DstAddr: "1.1.1.1", AF: 4, Proto: "ICMP",
		ResultJSON: `{"result":[
			{"hop":1,"result":[{"from":"8.8.8.8","rtt":1.0}]},
			{"hop":2,"result":[{"x":"*"}]},
			{"hop":3,"result":[{"from":"1.0.0.1","rtt":2.0}]},
			{"hop":4,"result":[{"from":"1.1.1.1","rtt":3.0}]}
		]}`,
	}
	in <- atlas.Record{
		Type: "ping", Timestamp: time.Now().Unix(), MsmID: 2, PrbID: 42,
		FromIP: "8.8.8.8", DstAddr: "1.1.1.1", AF: 4,
		ResultJSON: `{"avg":3.1,"min":3,"max":3.2,"sent":3,"rcvd":3}`,
	}
	close(in)

	if err := s.Run(ctx, in, 10, time.Second); err != nil {
		t.Fatal(err)
	}

	res, err := s.graph.Query(
		"MATCH (a:IP)-[:NEXT_HOP]->(b:IP) RETURN a.addr, b.addr ORDER BY a.addr",
		nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var edges int
	for res.Next() {
		edges++
	}
	if edges < 2 {
		t.Fatalf("expected >=2 NEXT_HOP edges, got %d", edges)
	}

	if cfg.ASN != nil {
		res, err = s.graph.Query(
			"MATCH (a:AS)-[:TRANSITS]->(b:AS) RETURN a.asn, a.org, b.asn, b.org",
			nil, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Next() {
			t.Fatal("expected at least one TRANSITS edge with ASN enrichment")
		}
		if tests, err := s.TransitPairTests(ctx, 15169, 13335, 10); err != nil {
			t.Fatalf("transit pair tests: %v", err)
		} else if len(tests) == 0 {
			t.Fatal("expected endpoint PING evidence for enriched transit pair")
		}
	}

	// Exercise the production health queries against FalkorDB so Cypher changes
	// for freshness and probe-quality filtering are integration-tested.
	if _, err := s.ASNIssues(ctx, ASNIssueFilter{Role: "src", Limit: 5, Offset: 1, Query: "as15169"}); err != nil {
		t.Fatalf("source AS issues: %v", err)
	}
	if _, err := s.ASNIssues(ctx, ASNIssueFilter{Role: "dst", Limit: 5, Query: "google"}); err != nil {
		t.Fatalf("destination AS issues: %v", err)
	}
	if _, err := s.Probes(ctx, ProbeFilter{Limit: 5, Offset: 1, Query: "as15169"}); err != nil {
		t.Fatalf("filtered probes: %v", err)
	}
	if _, err := s.HotHops(ctx, HopFilter{Limit: 5, Offset: 1, Query: "as15169", Sort: "observations"}); err != nil {
		t.Fatalf("filtered hot hops: %v", err)
	}
	if _, err := s.TransitEdges(ctx, TransitFilter{Limit: 5, Offset: 1, Query: "as15169", Sort: "last_seen"}); err != nil {
		t.Fatalf("filtered transit edges: %v", err)
	}
	if _, err := s.Overview(ctx); err != nil {
		t.Fatalf("overview: %v", err)
	}
	if _, err := s.EvaluateAlert(ctx, AlertRuleSpec{Metric: "loss_ratio", Scope: "asn_dst"}); err != nil {
		t.Fatalf("destination alert evaluation: %v", err)
	}
}

func TestLivePruneRemovesOnlyStaleObservations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	s, err := New(Config{Addr: "localhost:6379", Graph: "ripestream_test"})
	if err != nil {
		t.Skipf("falkordb unavailable: %v", err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Skipf("falkordb ping failed: %v", err)
	}
	if err := s.EnsureIndexes(ctx); err != nil {
		t.Fatalf("indexes: %v", err)
	}

	now := time.Now().Unix()
	in := make(chan atlas.Record, 2)
	in <- atlas.Record{Type: "ping", Timestamp: now - int64((48 * time.Hour).Seconds()), MsmID: 990001, PrbID: 990001,
		FromIP: "192.0.2.1", DstAddr: "192.0.2.2", AF: 4,
		ResultJSON: `{"avg":10,"min":9,"max":11,"sent":3,"rcvd":0}`}
	in <- atlas.Record{Type: "ping", Timestamp: now, MsmID: 990002, PrbID: 990002,
		FromIP: "198.51.100.1", DstAddr: "198.51.100.2", AF: 4,
		ResultJSON: `{"avg":10,"min":9,"max":11,"sent":3,"rcvd":3}`}
	close(in)
	if err := s.Run(ctx, in, 10, time.Second); err != nil {
		t.Fatal(err)
	}

	stats, err := s.Prune(ctx, now-int64(time.Hour.Seconds()), 100)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Relationships == 0 {
		t.Fatal("expected at least one stale relationship to be pruned")
	}
	rows, err := s.rows(ctx, `MATCH (p:Probe)-[:PING]->() WHERE p.id IN [990001, 990002] RETURN p.id AS id ORDER BY id`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || asInt(rows[0]["id"]) != 990002 {
		t.Fatalf("remaining ping probes = %#v, want only 990002", rows)
	}
}

func openASN(t *testing.T) *asn.DB {
	t.Helper()
	for _, p := range []string{asn.DefaultDBPath, "../../" + asn.DefaultDBPath} {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		db, err := asn.Open(p)
		if err != nil {
			t.Fatalf("open asn db: %v", err)
		}
		return db
	}
	return nil
}
