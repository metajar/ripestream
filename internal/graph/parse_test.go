package graph

import (
	"testing"

	"ripestream/internal/atlas"
)

func TestExtractResponsiveHopsSkipsTimeouts(t *testing.T) {
	raw := `{
		"result": [
			{"hop":1,"result":[{"from":"10.0.0.1","rtt":1.0},{"from":"10.0.0.1","rtt":2.0}]},
			{"hop":2,"result":[{"x":"*"},{"x":"*"}]},
			{"hop":3,"result":[{"from":"10.0.0.3","rtt":5.0}]}
		]
	}`
	hops, err := extractResponsiveHops(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(hops) != 2 {
		t.Fatalf("got %d hops, want 2", len(hops))
	}
	if hops[0].Addr != "10.0.0.1" || hops[1].Addr != "10.0.0.3" {
		t.Fatalf("addrs = %+v", hops)
	}
	if hops[0].RTT != 1.5 {
		t.Fatalf("avg rtt = %v, want 1.5", hops[0].RTT)
	}
}

func TestAppendTracerouteBuildsNextHops(t *testing.T) {
	rec := atlas.Record{
		Type: "traceroute", Timestamp: 100, MsmID: 1, PrbID: 2,
		FromIP: "10.0.0.1", DstAddr: "10.0.0.3", AF: 4, Proto: "ICMP",
		ResultJSON: `{
			"result": [
				{"hop":1,"result":[{"from":"10.0.0.1","rtt":1.0}]},
				{"hop":2,"result":[{"from":"10.0.0.2","rtt":2.0}]},
				{"hop":3,"result":[{"from":"10.0.0.3","rtt":3.0}]}
			]
		}`,
	}
	s := &Store{}
	var b batch
	if err := s.appendTraceroute(rec, &b); err != nil {
		t.Fatal(err)
	}
	if len(b.nextHops) != 2 {
		t.Fatalf("nextHops=%d want 2", len(b.nextHops))
	}
	if len(b.probes) != 1 {
		t.Fatalf("probes=%d want 1", len(b.probes))
	}
}

func TestAppendPing(t *testing.T) {
	rec := atlas.Record{
		Type: "ping", Timestamp: 100, MsmID: 1, PrbID: 2,
		FromIP: "10.0.0.1", DstAddr: "1.2.3.4", AF: 4,
		ResultJSON: `{"avg":10.5,"min":10,"max":11,"sent":3,"rcvd":2}`,
	}
	s := &Store{}
	var b batch
	if err := s.appendPing(rec, &b); err != nil {
		t.Fatal(err)
	}
	if len(b.pings) != 1 {
		t.Fatalf("pings=%d", len(b.pings))
	}
	loss, _ := b.pings[0]["loss"].(float64)
	if loss < 0.33 || loss > 0.34 {
		t.Fatalf("loss=%v want ~0.333", loss)
	}
}

func TestAppendTracerouteEnrichesASN(t *testing.T) {
	s := &Store{asn: stubASN{m: map[string]asnHit{
		"8.8.8.8": {15169, "Google"},
		"1.1.1.1": {13335, "Cloudflare"},
	}}}
	rec := atlas.Record{
		Type: "traceroute", Timestamp: 100, MsmID: 1, PrbID: 2,
		FromIP: "8.8.8.8", DstAddr: "1.1.1.1", AF: 4, Proto: "ICMP",
		ResultJSON: `{
			"result": [
				{"hop":1,"result":[{"from":"8.8.8.8","rtt":1.0}]},
				{"hop":2,"result":[{"from":"1.1.1.1","rtt":2.0}]}
			]
		}`,
	}
	var b batch
	if err := s.appendTraceroute(rec, &b); err != nil {
		t.Fatal(err)
	}
	if len(b.nextHops) != 1 {
		t.Fatalf("nextHops=%d", len(b.nextHops))
	}
	if b.nextHops[0]["asn_a"] != int64(15169) || b.nextHops[0]["asn_b"] != int64(13335) {
		t.Fatalf("asns=%v", b.nextHops[0])
	}
	if len(b.asLinks) != 1 {
		t.Fatalf("asLinks=%d want 1", len(b.asLinks))
	}
}

type asnHit struct {
	n   int64
	org string
}

type stubASN struct {
	m map[string]asnHit
}

func (s stubASN) Lookup(ip string) (int64, string, bool) {
	h, ok := s.m[ip]
	return h.n, h.org, ok
}

func TestLossRatio(t *testing.T) {
	if lossRatio(0, 0) != 0 {
		t.Fatal("sent=0")
	}
	if lossRatio(4, 4) != 0 {
		t.Fatal("no loss")
	}
	if lossRatio(4, 1) != 0.75 {
		t.Fatal("expected 0.75")
	}
}

func TestIsGraphType(t *testing.T) {
	if !isGraphType(atlas.Record{Type: "ping"}) || !isGraphType(atlas.Record{Type: "traceroute"}) {
		t.Fatal("expected ping/traceroute")
	}
	if isGraphType(atlas.Record{Type: "dns"}) {
		t.Fatal("dns should be skipped")
	}
}
