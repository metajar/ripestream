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
