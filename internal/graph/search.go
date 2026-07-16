package graph

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// Search returns autocomplete hits for a free-text query. It matches ASes by
// organization name (case-insensitive substring) or ASN prefix, plus IPs by
// address prefix and probes by id prefix.
//
// AS hits are returned as a group carrying several concrete IPs (up to ipPerAS)
// and a total count, so the Path Explorer can offer the operator a choice of
// endpoint within the AS rather than a single arbitrary address.
//
// The query is split into cheap, anchored sub-queries rather than one expensive
// full scan; each is bounded by a per-kind limit.
func (s *Store) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 12
	}
	const ipPerAS = 8 // how many concrete IPs to surface per AS hit
	lower := strings.ToLower(query)

	var out []SearchResult

	// --- AS by organization name (substring, case-insensitive) ---
	// Carry up to ipPerAS IPs and a total IP count per AS so callers can pick.
	if asRows, err := s.rows(ctx, `
MATCH (ip:IP)-[:IN_AS]->(a:AS)
WHERE toLower(a.org) CONTAINS $q
WITH a, collect(ip.addr) AS allIps
RETURN a.asn AS asn, a.org AS org, size(allIps) AS ipCount, allIps[0..$n] AS ips
ORDER BY ipCount DESC
LIMIT $limit`,
		map[string]any{"q": lower, "n": ipPerAS, "limit": limit}); err == nil {
		out = append(out, asHitsFromRows(asRows, "name")...)
	}

	// --- AS by ASN prefix (when the query looks numeric) ---
	if isNumericPrefix(query) {
		if asRows, err := s.rows(ctx, `
MATCH (ip:IP)-[:IN_AS]->(a:AS)
WHERE toString(a.asn) STARTS WITH $q
WITH a, collect(ip.addr) AS allIps
RETURN a.asn AS asn, a.org AS org, size(allIps) AS ipCount, allIps[0..$n] AS ips
ORDER BY ipCount DESC
LIMIT $limit`,
			map[string]any{"q": query, "n": ipPerAS, "limit": limit}); err == nil {
			out = append(out, asHitsFromRows(asRows, "asn")...)
		}
	}

	// --- IP by address prefix (only surface as standalone hits when the query
	// looks like an IP; otherwise AS hits already enumerate IPs). ---
	if looksLikeIP(query) {
		if ipRows, err := s.rows(ctx, `
MATCH (ip:IP) WHERE ip.addr STARTS WITH $q
OPTIONAL MATCH (ip)-[:IN_AS]->(as:AS)
RETURN ip.addr AS addr, ip.asn AS asn, as.org AS org LIMIT $limit`,
			map[string]any{"q": query, "limit": limit}); err == nil {
			for _, r := range ipRows {
				sr := SearchResult{Kind: "ip", Label: asString(r["addr"]), Addr: asString(r["addr"])}
				if asn := asInt(r["asn"]); asn > 0 {
					sr.ASN = ptrIf(asn, int64(0))
					sr.Org = asString(r["org"])
					sr.Sub = "AS" + strconv.FormatInt(asn, 10)
					if org := asString(r["org"]); org != "" {
						sr.Sub += " · " + org
					}
				}
				out = append(out, sr)
			}
		}
	}

	// --- Probe by id prefix ---
	if isNumericPrefix(query) {
		if prbRows, err := s.rows(ctx, `
MATCH (p:Probe) WHERE toString(p.id) STARTS WITH $q
OPTIONAL MATCH (p)-[:LOCATED_AT]->(ip:IP)
OPTIONAL MATCH (ip)-[:IN_AS]->(as:AS)
RETURN p.id AS id, ip.addr AS addr, as.asn AS asn, as.org AS org LIMIT $limit`,
			map[string]any{"q": query, "limit": limit}); err == nil {
			for _, r := range prbRows {
				sr := SearchResult{
					Kind: "probe", Label: "probe " + asString(r["id"]),
					Sub: asString(r["addr"]),
				}
				if addr := asString(r["addr"]); addr != "" {
					sr.Addr = addr
				}
				if asn := asInt(r["asn"]); asn > 0 {
					sr.ASN = ptrIf(asn, int64(0))
					sr.Org = asString(r["org"])
				}
				out = append(out, sr)
			}
		}
	}

	// Deduplicate (the AS org-substring and ASN-prefix passes can overlap).
	out = dedupeSearch(out)

	// Order: exact-ish label matches first, then by IP count (busier ASes first).
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rankHit(out[i], lower), rankHit(out[j], lower)
		if ri != rj {
			return ri < rj
		}
		return ipCount(out[i]) > ipCount(out[j])
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// asHitsFromRows builds AS SearchResults from the asn/org/ipCount/ips projection
// used by both AS search passes.
func asHitsFromRows(rows []map[string]any, _ string) []SearchResult {
	out := make([]SearchResult, 0, len(rows))
	for _, r := range rows {
		asn := asInt(r["asn"])
		ipCount := asInt(r["ipCount"])
		var ips []string
		if raw, ok := r["ips"].([]any); ok {
			ips = make([]string, 0, len(raw))
			for _, v := range raw {
				ips = append(ips, asString(v))
			}
		}
		sr := SearchResult{
			Kind: "as", Label: asString(r["org"]),
			ASN: ptrIf(asn, int64(0)), Org: asString(r["org"]),
			Addrs: ips,
			Sub:   "AS" + strconv.FormatInt(asn, 10) + " · " + strconv.FormatInt(ipCount, 10) + " IPs",
		}
		if len(ips) > 0 {
			sr.Addr = ips[0]
		}
		out = append(out, sr)
	}
	return out
}

// ipCount returns the total-IP count parsed out of an AS hit's Sub, or 0.
func ipCount(h SearchResult) int {
	if h.Kind != "as" || len(h.Addrs) == 0 {
		return 0
	}
	return len(h.Addrs)
}

// looksLikeIP reports whether s resembles an IP/v6 prefix (digits, dots, hex, colons).
func looksLikeIP(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isOK := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '.' || c == ':'
		if !isOK {
			return false
		}
	}
	return true
}

// isNumericPrefix reports whether s is a run of digits (an ASN or probe id).
func isNumericPrefix(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// dedupeSearch collapses hits with identical (kind, addr|asn).
func dedupeSearch(in []SearchResult) []SearchResult {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, h := range in {
		key := h.Kind + ":"
		switch h.Kind {
		case "as":
			if h.ASN != nil {
				key += strconv.FormatInt(*h.ASN, 10)
			} else {
				key += h.Label
			}
		default:
			key += h.Addr
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, h)
	}
	return out
}

// rankHit orders results so the most relevant appear first: label starts-with
// the query beats label contains beats everything else.
func rankHit(h SearchResult, q string) int {
	label := strings.ToLower(h.Label)
	switch {
	case label == q:
		return 0
	case strings.HasPrefix(label, q):
		return 1
	case strings.Contains(label, q):
		return 2
	}
	return 3
}
