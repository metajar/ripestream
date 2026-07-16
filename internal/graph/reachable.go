package graph

import (
	"context"
	"strconv"
)

// ReachableDestinations returns the endpoints reachable from src, grouped by AS,
// for constraining the Path Explorer's destination picker. It uses two signals:
//
//  1. Probe reachability (authoritative): if a probe is located at src, its
//     PING edges name every target RIPE Atlas measures from there. This is the
//     complete, real measurement set.
//  2. Transit reachability (fallback): for an IP that isn't a probe location
//     (e.g. an intermediate router), a bounded NEXT_HOP traversal (depth 1..4)
//     finds downstream IPs on paths through it. FalkorDB can't do unbounded
//     reachability on this graph, so the depth is kept small.
//
// Each group carries the AS, a total IP count, and a sample of concrete IPs the
// caller can offer as the destination.
func (s *Store) ReachableDestinations(ctx context.Context, src string) ([]ReachableGroup, error) {
	if src == "" {
		return nil, nil
	}

	// 1. Probe reachability. One or more probes may share this source IP.
	probeRows, err := s.rows(ctx, `
MATCH (p:Probe)-[:LOCATED_AT]->(src:IP {addr: $src})
MATCH (p)-[e:PING]->(t:IP)
WHERE e.sent > 0
OPTIONAL MATCH (t)-[:IN_AS]->(as:AS)
WITH COALESCE(as.asn, -1) AS asn, COALESCE(as.org, "Direct targets") AS org, t.addr AS addr
WITH asn, org, collect(addr) AS ips
RETURN asn, org, size(ips) AS n, ips[0..8] AS sample
ORDER BY n DESC`, map[string]any{"src": src})
	if err == nil && len(probeRows) > 0 {
		return groupsFromRows(probeRows, "probe"), nil
	}

	// 2. Transit reachability (bounded NEXT_HOP depth 1..4).
	hopRows, err := s.rows(ctx, `
MATCH (src:IP {addr: $src})-[:NEXT_HOP*1..4]->(r:IP)
OPTIONAL MATCH (r)-[:IN_AS]->(as:AS)
WITH COALESCE(as.asn, -1) AS asn, COALESCE(as.org, "Transit IPs") AS org, r.addr AS addr
WITH asn, org, collect(DISTINCT addr) AS ips
RETURN asn, org, size(ips) AS n, ips[0..8] AS sample
ORDER BY n DESC`, map[string]any{"src": src})
	if err != nil {
		return nil, err
	}
	return groupsFromRows(hopRows, "transit"), nil
}

// groupsFromRows normalizes the asn/org/n/sample projection into ReachableGroup
// values, mapping the sentinel -1 ASN to nil (unattributed IPs).
func groupsFromRows(rows []map[string]any, basis string) []ReachableGroup {
	out := make([]ReachableGroup, 0, len(rows))
	for _, r := range rows {
		var sample []string
		if raw, ok := r["sample"].([]any); ok {
			sample = make([]string, 0, len(raw))
			for _, v := range raw {
				sample = append(sample, asString(v))
			}
		}
		asn := asInt(r["asn"])
		g := ReachableGroup{
			Org:      asString(r["org"]),
			IPCount:  asInt(r["n"]),
			SampleIP: sample,
			Basis:    basis,
		}
		if asn > 0 {
			g.ASN = ptrIf(asn, int64(0))
		}
		out = append(out, g)
	}
	return out
}

// asnStr renders an ASN (or "-") for logging/debug.
func asnStr(a *int64) string {
	if a == nil {
		return "-"
	}
	return strconv.FormatInt(*a, 10)
}
