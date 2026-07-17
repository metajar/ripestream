package graph

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

const probeMetadataProjection = `p.display_name AS display_name, p.description AS description,
       p.probe_type AS probe_type, p.country_code AS country_code,
       p.latitude AS latitude, p.longitude AS longitude,
       p.is_anchor AS is_anchor, p.is_public AS is_public,
       p.firmware_version AS firmware_version, p.status_id AS status_id,
       p.status_name AS status_name, p.status_since AS status_since,
       p.first_connected AS first_connected, p.last_connected AS last_connected,
       p.prefix_v4 AS prefix_v4, p.prefix_v6 AS prefix_v6,
       p.asn_v4 AS asn_v4, p.asn_v6 AS asn_v6,
       p.tag_slugs AS tag_slugs, p.metadata_updated_at AS metadata_updated_at`

// ---- Overview ---------------------------------------------------------------

func (s *Store) Overview(ctx context.Context) (Overview, error) {
	var o Overview
	cutoff := s.activeCutoff()

	// Counts are fetched as independent single-purpose queries. The original
	// version chained them with WITH/count, which re-traverses the graph at each
	// step and blows past FalkorDB's query timeout as the graph grows. These
	// simple count() calls each complete in ~1ms.
	o.Counts = OverviewCounts{
		IPs:          s.countOne(ctx, "MATCH (n:IP) RETURN count(n)"),
		Probes:       s.countOne(ctx, "MATCH (n:Probe) RETURN count(n)"),
		ASes:         s.countOne(ctx, "MATCH (n:AS) RETURN count(n)"),
		Targets:      s.countOne(ctx, "MATCH ()-[e:TARGETS]->() RETURN count(e)"),
		NextHopEdges: s.countOne(ctx, "MATCH ()-[e:NEXT_HOP]->() RETURN count(e)"),
		PingEdges:    s.countOne(ctx, "MATCH ()-[e:PING]->() RETURN count(e)"),
		TransitEdges: s.countOne(ctx, "MATCH ()-[e:TRANSITS]->() RETURN count(e)"),
		LossyPings:   s.countOneParams(ctx, "MATCH ()-[e:PING]->() WHERE e.loss_ratio > 0.2 AND e.sent > 0 AND e.last_seen >= $cutoff RETURN count(e)", map[string]any{"cutoff": cutoff}),
	}

	g, err := s.rows(ctx, `
MATCH ()-[e:PING]->() WHERE e.sent > 0 AND e.last_seen >= $cutoff
WITH count(CASE WHEN e.loss_ratio = 0 THEN 1 END) AS healthy,
     count(CASE WHEN e.loss_ratio > 0 AND e.loss_ratio < 1 THEN 1 END) AS lossy,
     count(CASE WHEN e.loss_ratio >= 1 THEN 1 END) AS lost,
     sum(e.sent) AS sent, sum(e.rcvd) AS rcvd
RETURN healthy, lossy, lost, (1.0 * (sent - rcvd) / sent) AS avg_loss`, map[string]any{"cutoff": cutoff})
	if err != nil {
		// Best-effort: counts alone are still useful; don't fail the whole overview.
		slog.Warn("overview: global loss query failed (serving counts only)", "err", err)
	} else if len(g) > 0 {
		r := g[0]
		o.GlobalLoss = GlobalLoss{
			HealthyEdges: asInt(r["healthy"]), LossyEdges: asInt(r["lossy"]),
			LostEdges: asInt(r["lost"]), AvgLossPct: asFloat(r["avg_loss"]) * 100,
		}
	}

	// AS rankings are best-effort too — a slow graph shouldn't blank the page.
	// Default to empty (not nil) slices so a failed sub-query yields JSON `[]`
	// rather than `null`, which the UI can always .slice()/map over safely.
	o.TopSrcAS = []ASNIssue{}
	o.TopDstAS = []ASNIssue{}
	o.TopASPairs = []ASNPairIssue{}
	if res, err := s.ASNIssues(ctx, ASNIssueFilter{Role: "src", MinLoss: 0.1, MinProbes: 3, Limit: 8}); err == nil {
		o.TopSrcAS = res
	} else {
		slog.Warn("overview: src AS issues query failed", "err", err)
	}
	if res, err := s.ASNIssues(ctx, ASNIssueFilter{Role: "dst", MinLoss: 0.1, MinProbes: 3, MinSourceASes: 2, Limit: 8}); err == nil {
		o.TopDstAS = res
	} else {
		slog.Warn("overview: dst AS issues query failed", "err", err)
	}
	if res, err := s.ASNPairIssues(ctx, ASNPairFilter{MinLoss: 0.2, MinProbes: 2, Limit: 8}); err == nil {
		o.TopASPairs = res
	} else {
		slog.Warn("overview: AS pair issues query failed", "err", err)
	}
	return o, nil
}

// countOne runs a single-row count query and returns the integer, tolerating
// errors (returns 0) so one slow/failed count can't abort the whole overview.
func (s *Store) countOne(ctx context.Context, cypher string) int64 {
	return s.countOneParams(ctx, cypher, nil)
}

func (s *Store) countOneParams(ctx context.Context, cypher string, params map[string]any) int64 {
	rows, err := s.rows(ctx, cypher, params)
	if err != nil || len(rows) == 0 {
		return 0
	}
	for _, v := range rows[0] {
		return asInt(v)
	}
	return 0
}

// ---- ASN issues (single-AS ranking) ----------------------------------------

func (s *Store) ASNIssues(ctx context.Context, f ASNIssueFilter) ([]ASNIssue, error) {
	f.Limit = clampLimit(f.Limit, 20, 101)
	if f.Offset < 0 {
		f.Offset = 0
	}
	if f.MinLoss <= 0 {
		f.MinLoss = 0.1
	}
	role := f.Role
	if role != "src" && role != "dst" {
		role = "dst"
	}
	// Whitelisted sort: validatedSort guarantees only safe identifiers reach Cypher.
	// "impact" = avg_loss × probes (breadth-weighted); default to impact.
	sortExpr := validatedSort(f.Sort, "impact", asnIssueSortExpr)
	dir := sortDir(f.Order)
	// min_probes filters out single-probe noise before ranking.
	minProbes := f.MinProbes
	if minProbes < 1 {
		minProbes = 1
	}
	minSourceASes := f.MinSourceASes
	if minSourceASes < 1 {
		minSourceASes = 1
	}
	query := strings.ToLower(strings.TrimSpace(f.Query))

	var q string
	if role == "src" {
		q = `
MATCH (p:Probe)-[:LOCATED_AT]->(src:IP)-[:IN_AS]->(as:AS)
MATCH (p)-[e:PING]->(t:IP)
WHERE e.sent > 0 AND e.last_seen >= $cutoff
WITH as.asn AS asn, as.org AS org,
	 count(*) AS samples, sum(e.sent) AS sent, sum(e.rcvd) AS rcvd,
	 max(e.loss_ratio) AS max_loss, avg(e.avg_rtt_ms) AS avg_rtt,
	 count(DISTINCT p.id) AS probes, count(DISTINCT t.addr) AS targets,
	 max(e.last_seen) AS last_seen
WITH asn, org, samples, max_loss, avg_rtt, probes, targets, last_seen,
	 (1.0 * (sent - rcvd) / sent) AS avg_loss
WHERE probes >= $minProbes AND avg_loss >= $minLoss
  AND ($query = '' OR toString(asn) CONTAINS $query OR ('as' + toString(asn)) CONTAINS $query OR toLower(coalesce(org, '')) CONTAINS $query
       OR toString(samples) CONTAINS $query OR toString(probes) CONTAINS $query
       OR toString(targets) CONTAINS $query OR toString(round(100.0*avg_loss)) CONTAINS $query
       OR toString(round(avg_rtt)) CONTAINS $query OR toString(last_seen) CONTAINS $query
       OR ($query = 'critical' AND avg_loss >= 0.8)
       OR ($query = 'high' AND avg_loss >= 0.2 AND avg_loss < 0.8)
       OR ($query = 'watch' AND avg_loss > 0 AND avg_loss < 0.2))
WITH asn, org, samples, avg_loss, max_loss, avg_rtt, probes, targets, last_seen,
	 avg_loss * probes AS impact
RETURN asn, org, samples, round(100.0*avg_loss) AS avg_loss_pct,
	   round(100.0*max_loss) AS max_loss_pct, round(avg_rtt) AS avg_rtt_ms,
	   probes, 1 AS source_ases, targets, last_seen
ORDER BY ` + sortExpr + ` ` + dir + `, samples DESC, asn ASC SKIP $offset LIMIT $limit`
	} else {
		q = `
MATCH (p:Probe)-[allPing:PING]->()
WHERE allPing.sent > 0 AND allPing.last_seen >= $cutoff
WITH p, count(allPing) AS probe_targets, sum(allPing.sent) AS probe_sent, sum(allPing.rcvd) AS probe_rcvd
WITH p, probe_targets, (1.0 * (probe_sent - probe_rcvd) / probe_sent) AS probe_loss
WHERE probe_targets < $qualityTargets OR probe_loss < $maxProbeLoss
MATCH (p:Probe)-[e:PING]->(t:IP)-[:IN_AS]->(as:AS)
WHERE e.sent > 0 AND e.last_seen >= $cutoff
WITH as.asn AS asn, as.org AS org,
	 count(*) AS samples, sum(e.sent) AS sent, sum(e.rcvd) AS rcvd,
	 max(e.loss_ratio) AS max_loss, avg(e.avg_rtt_ms) AS avg_rtt,
	 count(DISTINCT p.id) AS probes, count(DISTINCT p.source_asn) AS source_ases,
	 count(DISTINCT t.addr) AS targets, max(e.last_seen) AS last_seen
WITH asn, org, samples, max_loss, avg_rtt, probes, source_ases, targets, last_seen,
	 (1.0 * (sent - rcvd) / sent) AS avg_loss
WHERE probes >= $minProbes AND source_ases >= $minSourceASes AND avg_loss >= $minLoss
  AND ($query = '' OR toString(asn) CONTAINS $query OR ('as' + toString(asn)) CONTAINS $query OR toLower(coalesce(org, '')) CONTAINS $query
       OR toString(samples) CONTAINS $query OR toString(probes) CONTAINS $query
       OR toString(source_ases) CONTAINS $query OR toString(targets) CONTAINS $query
       OR toString(round(100.0*avg_loss)) CONTAINS $query OR toString(round(avg_rtt)) CONTAINS $query
       OR toString(last_seen) CONTAINS $query
       OR ($query = 'critical' AND avg_loss >= 0.8)
       OR ($query = 'high' AND avg_loss >= 0.2 AND avg_loss < 0.8)
       OR ($query = 'watch' AND avg_loss > 0 AND avg_loss < 0.2))
WITH asn, org, samples, avg_loss, max_loss, avg_rtt, probes, source_ases, targets, last_seen,
	 avg_loss * probes AS impact
RETURN asn, org, samples, round(100.0*avg_loss) AS avg_loss_pct,
	   round(100.0*max_loss) AS max_loss_pct, round(avg_rtt) AS avg_rtt_ms,
	   probes, source_ases, targets, last_seen
ORDER BY ` + sortExpr + ` ` + dir + `, samples DESC, asn ASC SKIP $offset LIMIT $limit`
	}
	rows, err := s.rows(ctx, q, map[string]any{
		"minLoss": f.MinLoss, "limit": f.Limit, "minProbes": minProbes, "offset": f.Offset,
		"cutoff": s.activeCutoff(), "qualityTargets": 3, "maxProbeLoss": 0.8,
		"minSourceASes": minSourceASes,
		"query":         query,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ASNIssue, 0, len(rows))
	for _, r := range rows {
		out = append(out, ASNIssue{
			ASN: asInt(r["asn"]), Org: asString(r["org"]), Role: role,
			Samples: asInt(r["samples"]), Probes: asInt(r["probes"]),
			SourceASes: asInt(r["source_ases"]), Targets: asInt(r["targets"]),
			AvgLossPct: asFloat(r["avg_loss_pct"]), MaxLossPct: asFloat(r["max_loss_pct"]),
			AvgRttMs: asFloat(r["avg_rtt_ms"]),
			LastSeen: asInt(r["last_seen"]),
		})
	}
	return out, nil
}

// ---- ASN pair issues --------------------------------------------------------

func (s *Store) ASNPairIssues(ctx context.Context, f ASNPairFilter) ([]ASNPairIssue, error) {
	f.Limit = clampLimit(f.Limit, 20, 100)
	if f.MinLoss <= 0 {
		f.MinLoss = 0.2
	}
	minProbes := f.MinProbes
	if minProbes < 1 {
		minProbes = 1
	}
	rows, err := s.rows(ctx, `
MATCH (p:Probe)-[:LOCATED_AT]->(src:IP)-[:IN_AS]->(srcAS:AS)
MATCH (p)-[e:PING]->(t:IP)-[:IN_AS]->(dstAS:AS)
WHERE e.sent > 0 AND e.last_seen >= $cutoff
WITH srcAS.asn AS sasn, srcAS.org AS sorg, dstAS.asn AS dasn, dstAS.org AS dorg,
     count(*) AS samples, sum(e.sent) AS sent, sum(e.rcvd) AS rcvd,
     max(e.loss_ratio) AS max_loss, avg(e.avg_rtt_ms) AS avg_rtt,
     count(DISTINCT p.id) AS probes, max(e.last_seen) AS last_seen
WITH sasn, sorg, dasn, dorg, samples, max_loss, avg_rtt, probes, last_seen,
     (1.0 * (sent - rcvd) / sent) AS avg_loss
WHERE avg_loss >= $minLoss AND probes >= $minProbes
RETURN sasn, sorg, dasn, dorg, samples,
       round(100.0*avg_loss) AS avg_loss_pct, round(100.0*max_loss) AS max_loss_pct,
       round(avg_rtt) AS avg_rtt_ms, probes, last_seen
ORDER BY avg_loss DESC, samples DESC LIMIT $limit`,
		map[string]any{"minLoss": f.MinLoss, "minProbes": minProbes, "limit": f.Limit, "cutoff": s.activeCutoff()})
	if err != nil {
		return nil, err
	}
	out := make([]ASNPairIssue, 0, len(rows))
	for _, r := range rows {
		out = append(out, ASNPairIssue{
			SrcASN: asInt(r["sasn"]), SrcOrg: asString(r["sorg"]),
			DstASN: asInt(r["dasn"]), DstOrg: asString(r["dorg"]),
			Samples: asInt(r["samples"]), Probes: asInt(r["probes"]),
			AvgLossPct: asFloat(r["avg_loss_pct"]), MaxLossPct: asFloat(r["max_loss_pct"]),
			AvgRttMs: asFloat(r["avg_rtt_ms"]),
			LastSeen: asInt(r["last_seen"]),
		})
	}
	return out, nil
}

// ---- ASN detail -------------------------------------------------------------

func (s *Store) ASNDetail(ctx context.Context, asn int64) (ASNDetail, error) {
	var d ASNDetail
	// Each count is its own anchored query; combining OPTIONAL MATCHes across
	// unrelated patterns produces a cross-product that times out on large graphs.
	p := map[string]any{"asn": asn}

	// Existence + org.
	basic, err := s.rows(ctx, `MATCH (as:AS {asn: $asn}) RETURN as.org AS org`, p)
	if err != nil {
		return d, err
	}
	if len(basic) == 0 {
		return d, fmt.Errorf("asn %d not found", asn)
	}
	d.ASN = asn
	d.Org = asString(basic[0]["org"])

	// Probes located in this AS.
	pr, _ := s.rows(ctx, `
MATCH (pr:Probe)-[:LOCATED_AT]->(:IP)-[:IN_AS]->(:AS {asn: $asn})
RETURN count(DISTINCT pr) AS probes`, p)
	if len(pr) > 0 {
		d.ProbeCount = asInt(pr[0]["probes"])
	}
	// IPs in this AS.
	ip, _ := s.rows(ctx, `MATCH (:IP)-[:IN_AS]->(:AS {asn: $asn}) RETURN count(*) AS ips`, p)
	if len(ip) > 0 {
		d.IPCount = asInt(ip[0]["ips"])
	}
	// Targets in this AS (destination IPs with PING edges).
	tg, _ := s.rows(ctx, `
MATCH (:Probe)-[:PING]->(t:IP)-[:IN_AS]->(:AS {asn: $asn}) RETURN count(DISTINCT t) AS tgts`, p)
	if len(tg) > 0 {
		d.TargetCount = asInt(tg[0]["tgts"])
	}
	// Transit in/out.
	tr, _ := s.rows(ctx, `
MATCH (a:AS {asn: $asn})
OPTIONAL MATCH (a)-[out:TRANSITS]->()
WITH a, count(out) AS tout
OPTIONAL MATCH ()-[inc:TRANSITS]->(a)
RETURN tout, count(inc) AS tin`, p)
	if len(tr) > 0 {
		d.TransitOut = asInt(tr[0]["tout"])
		d.TransitIn = asInt(tr[0]["tin"])
	}
	// Loss summary over PING edges into this AS.
	ls, _ := s.rows(ctx, `
MATCH (p:Probe)-[e:PING]->(t:IP)-[:IN_AS]->(:AS {asn: $asn})
WHERE e.sent > 0
WITH sum(e.sent) AS sent, sum(e.rcvd) AS rcvd,
     count(CASE WHEN e.loss_ratio > 0.2 THEN 1 END) AS lossy
RETURN CASE WHEN sent > 0 THEN round(100.0 * (sent - rcvd) / sent) ELSE 0 END AS avg_loss_pct,
       lossy`, p)
	if len(ls) > 0 {
		d.AvgLossPct = asFloat(ls[0]["avg_loss_pct"])
		d.LossyEdges = asInt(ls[0]["lossy"])
	}
	return d, nil
}

func (s *Store) ASNProbes(ctx context.Context, asn int64, limit int) ([]ProbeInfo, error) {
	limit = clampLimit(limit, 50, 500)
	rows, err := s.rows(ctx, `
MATCH (p:Probe)-[:LOCATED_AT]->(src:IP)-[:IN_AS]->(as:AS {asn: $asn})
OPTIONAL MATCH (p)-[e:PING]->()
WHERE e.sent > 0
WITH p, src, as, avg(e.loss_ratio) AS avg_loss, avg(e.avg_rtt_ms) AS avg_rtt,
     max(src.last_seen) AS last_seen
RETURN p.id AS id, src.addr AS src_ip, as.asn AS asn, as.org AS org,
       round(100.0*avg_loss) AS loss_pct, round(avg_rtt) AS rtt, last_seen,
       `+probeMetadataProjection+`
ORDER BY avg_loss DESC LIMIT $limit`, map[string]any{"asn": asn, "limit": limit})
	if err != nil {
		return nil, err
	}
	out := make([]ProbeInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, probeInfoFromRow(r))
	}
	return out, nil
}

func (s *Store) ASNTargets(ctx context.Context, asn int64, limit int) ([]TargetInfo, error) {
	limit = clampLimit(limit, 50, 500)
	rows, err := s.rows(ctx, `
MATCH (p:Probe)-[e:PING]->(t:IP)-[:IN_AS]->(as:AS {asn: $asn})
WHERE e.sent > 0
WITH t, as, avg(e.loss_ratio) AS avg_loss, avg(e.avg_rtt_ms) AS avg_rtt,
     count(DISTINCT p.id) AS probes, max(e.last_seen) AS last_seen
RETURN t.addr AS addr, as.org AS org, probes,
       round(100.0*avg_loss) AS loss_pct, round(avg_rtt) AS rtt, last_seen
ORDER BY avg_loss DESC LIMIT $limit`, map[string]any{"asn": asn, "limit": limit})
	if err != nil {
		return nil, err
	}
	out := make([]TargetInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, TargetInfo{
			Addr: asString(r["addr"]), ASN: ptrIf(asn, int64(0)), Org: asString(r["org"]),
			Probes: asInt(r["probes"]), LossPct: asFloat(r["loss_pct"]),
			AvgRttMs: asFloat(r["rtt"]), LastSeen: asInt(r["last_seen"]),
		})
	}
	return out, nil
}

func (s *Store) ASNTransit(ctx context.Context, asn int64, limit int) ([]ASNTransitEdge, error) {
	limit = clampLimit(limit, 50, 500)
	rows, err := s.rows(ctx, `
MATCH (a:AS {asn: $asn})-[e:TRANSITS]->(b:AS)
RETURN a.asn AS sasn, a.org AS sorg, b.asn AS dasn, b.org AS dorg,
       e.seen_count AS sc, e.last_seen AS ls
ORDER BY sc DESC LIMIT $limit
UNION ALL
MATCH (b:AS)-[e:TRANSITS]->(a:AS {asn: $asn})
RETURN b.asn AS sasn, b.org AS sorg, a.asn AS dasn, a.org AS dorg,
       e.seen_count AS sc, e.last_seen AS ls
ORDER BY sc DESC LIMIT $limit`, map[string]any{"asn": asn, "limit": limit})
	if err != nil {
		return nil, err
	}
	out := make([]ASNTransitEdge, 0, len(rows))
	for _, r := range rows {
		out = append(out, ASNTransitEdge{
			SrcASN: asInt(r["sasn"]), SrcOrg: asString(r["sorg"]),
			DstASN: asInt(r["dasn"]), DstOrg: asString(r["dorg"]),
			SeenCount: asInt(r["sc"]), LastSeen: asInt(r["ls"]),
		})
	}
	return out, nil
}

// ---- Probe ------------------------------------------------------------------

func (s *Store) Probes(ctx context.Context, f ProbeFilter) ([]ProbeInfo, error) {
	f.Limit = clampLimit(f.Limit, 50, 501)
	if f.Offset < 0 {
		f.Offset = 0
	}
	if f.ASN > 0 {
		return s.ASNProbes(ctx, f.ASN, f.Limit)
	}
	conditions := []string{"avg_loss > $minLoss"}
	params := map[string]any{
		"limit": f.Limit, "offset": f.Offset, "cutoff": s.activeCutoff(), "minLoss": 0.2,
		"query": strings.ToLower(strings.TrimSpace(f.Query)),
	}
	if country := strings.ToUpper(strings.TrimSpace(f.Country)); len(country) == 2 {
		conditions = append(conditions, "p.country_code = $country")
		params["country"] = country
	}
	switch strings.ToLower(strings.TrimSpace(f.Type)) {
	case "anchor":
		conditions = append(conditions, "p.is_anchor = true")
	case "software":
		conditions = append(conditions, "p.probe_type = 'Software probe'")
	case "hardware":
		conditions = append(conditions, "p.probe_type STARTS WITH 'Hardware'")
	}
	statusNames := map[string]string{
		"connected": "Connected", "disconnected": "Disconnected",
		"never connected": "Never Connected", "abandoned": "Abandoned", "written off": "Written Off",
	}
	if status, ok := statusNames[strings.ToLower(strings.TrimSpace(f.Status))]; ok {
		conditions = append(conditions, "p.status_name = $status")
		params["status"] = status
	}
	sortExpr := validatedSort(f.Sort, "avg_loss", probeSortExpr)
	dir := sortDir(f.Order)
	conditions = append(conditions, `($query = '' OR toString(p.id) CONTAINS $query
       OR toLower(coalesce(p.display_name, '')) CONTAINS $query
       OR toLower(coalesce(p.description, '')) CONTAINS $query
       OR toLower(coalesce(p.probe_type, '')) CONTAINS $query
       OR toLower(coalesce(p.country_code, '')) CONTAINS $query
       OR toLower(coalesce(p.status_name, '')) CONTAINS $query
       OR toLower(coalesce(p.source_ip, '')) CONTAINS $query
       OR toString(coalesce(p.source_asn, 0)) CONTAINS $query
       OR ('as' + toString(coalesce(p.source_asn, 0))) CONTAINS $query
       OR toLower(coalesce(p.source_org, '')) CONTAINS $query
       OR toLower(coalesce(p.tag_slugs, '')) CONTAINS $query
       OR toString(round(100.0*avg_loss)) CONTAINS $query
       OR toString(round(avg_rtt)) CONTAINS $query OR toString(last_seen) CONTAINS $query)`)
	rows, err := s.rows(ctx, `
MATCH (p:Probe)-[e:PING]->()
WHERE e.sent > 0 AND e.last_seen >= $cutoff
WITH p, sum(e.sent) AS sent, sum(e.rcvd) AS rcvd,
     avg(e.avg_rtt_ms) AS avg_rtt, max(e.last_seen) AS last_seen
WITH p, avg_rtt, last_seen, (1.0 * (sent - rcvd) / sent) AS avg_loss
WHERE `+strings.Join(conditions, " AND ")+`
RETURN p.id AS id, p.source_ip AS src_ip, p.source_asn AS asn, p.source_org AS org,
       round(100.0*avg_loss) AS loss_pct, round(avg_rtt) AS rtt, last_seen,
       `+probeMetadataProjection+`
ORDER BY `+sortExpr+` `+dir+`, last_seen DESC, id ASC SKIP $offset LIMIT $limit`, params)
	if err != nil {
		return nil, err
	}
	out := make([]ProbeInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, probeInfoFromRow(r))
	}
	return out, nil
}

func (s *Store) ProbeDetail(ctx context.Context, id int64) (ProbeDetail, error) {
	var d ProbeDetail
	rows, err := s.rows(ctx, `
MATCH (p:Probe {id: $id})-[:LOCATED_AT]->(src:IP)
OPTIONAL MATCH (src)-[:IN_AS]->(as:AS)
RETURN src.addr AS src_ip, as.asn AS asn, as.org AS org, src.last_seen AS last_seen,
       `+probeMetadataProjection,
		map[string]any{"id": id})
	if err != nil {
		return d, err
	}
	if len(rows) == 0 {
		return d, fmt.Errorf("probe %d not found", id)
	}
	r := rows[0]
	d.ID = id
	d.SrcIP = asString(r["src_ip"])
	d.SrcASN = ptrIf(asInt(r["asn"]), int64(0))
	d.SrcOrg = asString(r["org"])
	d.Metadata = probeMetadataFromRow(r)
	d.LastSeen = asInt(r["last_seen"])
	counts, err := s.rows(ctx, `MATCH (:Probe {id: $id})-[e:PING]->() WHERE e.sent > 0 RETURN count(e) AS count`, map[string]any{"id": id})
	if err != nil {
		return d, err
	}
	if len(counts) > 0 {
		d.TargetCount = asInt(counts[0]["count"])
	}
	return d, nil
}

func (s *Store) ProbeTargets(ctx context.Context, id int64, f TargetFilter) ([]TargetInfo, error) {
	f.Limit = clampLimit(f.Limit, 25, 501)
	if f.Offset < 0 {
		f.Offset = 0
	}
	sortExpr := validatedSort(f.Sort, "loss", detailTargetSortExpr)
	dir := sortDir(f.Order)
	tgts, err := s.rows(ctx, `
MATCH (p:Probe {id: $id})-[e:PING]->(t:IP)
WHERE e.sent > 0
OPTIONAL MATCH (t)-[:IN_AS]->(as:AS)
WITH t, as, e, e.loss_ratio AS loss, e.avg_rtt_ms AS rtt, e.last_seen AS ls
WHERE $query = '' OR toLower(t.addr) CONTAINS $query
   OR toString(coalesce(as.asn, 0)) CONTAINS $query OR ('as' + toString(coalesce(as.asn, 0))) CONTAINS $query
   OR toLower(coalesce(as.org, '')) CONTAINS $query OR toString(round(100.0*loss)) CONTAINS $query
   OR toString(round(rtt)) CONTAINS $query OR toString(ls) CONTAINS $query
RETURN t.addr AS addr, as.asn AS asn, as.org AS org,
       round(100.0*loss) AS loss_pct, round(rtt) AS rtt, ls AS last_seen, loss
ORDER BY `+sortExpr+` `+dir+`, last_seen DESC, addr ASC SKIP $offset LIMIT $limit`, map[string]any{
		"id": id, "limit": f.Limit, "offset": f.Offset,
		"query": strings.ToLower(strings.TrimSpace(f.Query)),
	})
	if err != nil {
		return nil, err
	}
	out := make([]TargetInfo, 0, len(tgts))
	for _, r := range tgts {
		out = append(out, TargetInfo{
			Addr: asString(r["addr"]), ASN: ptrIf(asInt(r["asn"]), int64(0)),
			Org: asString(r["org"]), Probes: 1,
			LossPct: asFloat(r["loss_pct"]), AvgRttMs: asFloat(r["rtt"]),
			LastSeen: asInt(r["last_seen"]),
		})
	}
	return out, nil
}

// ---- Target -----------------------------------------------------------------

func (s *Store) Targets(ctx context.Context, f TargetFilter) ([]TargetInfo, error) {
	f.Limit = clampLimit(f.Limit, 50, 500)
	if f.ASN > 0 {
		return s.ASNTargets(ctx, f.ASN, f.Limit)
	}
	minProbes := f.MinProbes
	if minProbes < 3 {
		minProbes = 3
	}
	sortExpr := validatedSort(f.Sort, "impact", targetSortExpr)
	dir := sortDir(f.Order)

	// Qualify probes before calculating target health so a broadly failing probe
	// cannot make every destination look down. Aggregate every recent observation
	// from the remaining probes; filtering lossy edges before avg_loss would turn
	// a mixed healthy/lossy target into a misleading 100% row.
	rows, err := s.rows(ctx, `
MATCH (p:Probe)-[allPing:PING]->()
WHERE allPing.sent > 0 AND allPing.last_seen >= $cutoff
WITH p, count(allPing) AS probe_targets, sum(allPing.sent) AS probe_sent,
     sum(allPing.rcvd) AS probe_rcvd
WITH p, probe_targets, (1.0 * (probe_sent - probe_rcvd) / probe_sent) AS probe_loss
WHERE probe_targets < $qualityTargets OR probe_loss < $maxProbeLoss
MATCH (p)-[e:PING]->(t:IP)
WHERE e.sent > 0 AND e.last_seen >= $cutoff
WITH t, sum(e.sent) AS sent, sum(e.rcvd) AS rcvd, avg(e.avg_rtt_ms) AS avg_rtt,
     count(DISTINCT p.id) AS probes, count(DISTINCT p.source_asn) AS source_ases,
     max(e.last_seen) AS last_seen
WITH t, avg_rtt, probes, source_ases, last_seen,
     (1.0 * (sent - rcvd) / sent) AS avg_loss,
     (1.0 * (sent - rcvd) / sent) * probes AS impact
WHERE avg_loss > $minLoss AND probes >= $minProbes AND source_ases >= $minSourceASes
OPTIONAL MATCH (t)-[:IN_AS]->(as:AS)
WHERE as.asn = t.asn
RETURN t.addr AS addr, as.asn AS asn, as.org AS org, probes, source_ases,
       round(100.0*avg_loss) AS loss_pct, round(avg_rtt) AS rtt, last_seen, impact
ORDER BY `+sortExpr+` `+dir+`, last_seen DESC LIMIT $limit`, map[string]any{
		"limit": f.Limit, "cutoff": s.activeCutoff(), "minLoss": 0.2,
		"minProbes": minProbes, "minSourceASes": 2,
		"qualityTargets": 3, "maxProbeLoss": 0.8,
	})
	if err != nil {
		return nil, err
	}
	out := make([]TargetInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, TargetInfo{
			Addr: asString(r["addr"]), ASN: ptrIf(asInt(r["asn"]), int64(0)),
			Org: asString(r["org"]), Probes: asInt(r["probes"]), SourceASes: asInt(r["source_ases"]),
			LossPct: asFloat(r["loss_pct"]), AvgRttMs: asFloat(r["rtt"]),
			LastSeen: asInt(r["last_seen"]),
		})
	}
	return out, nil
}

func (s *Store) TargetDetail(ctx context.Context, addr string) (TargetDetail, error) {
	var d TargetDetail
	rows, err := s.rows(ctx, `
MATCH (t:IP {addr: $addr})
OPTIONAL MATCH (t)-[:IN_AS]->(as:AS)
RETURN t.last_seen AS last_seen, as.asn AS asn, as.org AS org`,
		map[string]any{"addr": addr})
	if err != nil {
		return d, err
	}
	if len(rows) == 0 {
		return d, fmt.Errorf("target %s not found", addr)
	}
	r := rows[0]
	d.Addr = addr
	d.ASN = ptrIf(asInt(r["asn"]), int64(0))
	d.Org = asString(r["org"])
	d.LastSeen = asInt(r["last_seen"])
	counts, err := s.rows(ctx, `MATCH ()-[e:PING]->(:IP {addr: $addr}) WHERE e.sent > 0 RETURN count(e) AS count`, map[string]any{"addr": addr})
	if err != nil {
		return d, err
	}
	if len(counts) > 0 {
		d.ProbeCount = asInt(counts[0]["count"])
	}

	// nearby hops: edges incident to this target's IP.
	hops, err := s.nearbyHops(ctx, addr, 20)
	if err == nil {
		d.NearbyHops = hops
	}
	return d, nil
}

func (s *Store) TargetProbes(ctx context.Context, addr string, f ProbeFilter) ([]ProbeInfo, error) {
	f.Limit = clampLimit(f.Limit, 25, 501)
	if f.Offset < 0 {
		f.Offset = 0
	}
	sortExpr := validatedSort(f.Sort, "loss", detailProbeSortExpr)
	dir := sortDir(f.Order)
	probes, err := s.rows(ctx, `
MATCH (p:Probe)-[e:PING]->(t:IP {addr: $addr})
WHERE e.sent > 0
WITH p, e, e.loss_ratio AS loss, e.avg_rtt_ms AS rtt, e.last_seen AS last_seen
WHERE $query = '' OR toString(p.id) CONTAINS $query
   OR toLower(coalesce(p.display_name, '')) CONTAINS $query
   OR toLower(coalesce(p.probe_type, '')) CONTAINS $query
   OR toLower(coalesce(p.country_code, '')) CONTAINS $query
   OR toLower(coalesce(p.source_ip, '')) CONTAINS $query
   OR toString(coalesce(p.source_asn, 0)) CONTAINS $query OR ('as' + toString(coalesce(p.source_asn, 0))) CONTAINS $query
   OR toLower(coalesce(p.source_org, '')) CONTAINS $query
   OR toString(round(100.0*loss)) CONTAINS $query OR toString(round(rtt)) CONTAINS $query
   OR toString(last_seen) CONTAINS $query
RETURN p.id AS id, p.source_ip AS src_ip, p.source_asn AS asn, p.source_org AS org,
       round(100.0*loss) AS loss_pct, round(rtt) AS rtt,
       last_seen, loss, `+probeMetadataProjection+`
ORDER BY `+sortExpr+` `+dir+`, last_seen DESC, id ASC SKIP $offset LIMIT $limit`, map[string]any{
		"addr": addr, "limit": f.Limit, "offset": f.Offset,
		"query": strings.ToLower(strings.TrimSpace(f.Query)),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ProbeInfo, 0, len(probes))
	for _, r := range probes {
		out = append(out, probeInfoFromRow(r))
	}
	return out, nil
}

// ---- Hops / Transit ---------------------------------------------------------

func (s *Store) HotHops(ctx context.Context, f HopFilter) ([]HotHop, error) {
	f.Limit = clampLimit(f.Limit, 50, 501)
	if f.Offset < 0 {
		f.Offset = 0
	}
	if f.MinRtt <= 0 {
		f.MinRtt = 100
	}
	sortExpr := validatedSort(f.Sort, "rtt", hopSortExpr)
	dir := sortDir(f.Order)
	rows, err := s.rows(ctx, `
MATCH (a:IP)-[e:NEXT_HOP]->(b:IP)
WHERE e.last_rtt_ms >= $minRtt
OPTIONAL MATCH (a)-[:IN_AS]->(aas:AS)
OPTIONAL MATCH (b)-[:IN_AS]->(bas:AS)
WITH a.addr AS fa, b.addr AS ta, e.last_rtt_ms AS rtt, e.seen_count AS sc, e.last_seen AS ls,
     aas.asn AS faasn, aas.org AS faorg, bas.asn AS taasn, bas.org AS taorg
WHERE $query = '' OR toLower(fa) CONTAINS $query OR toLower(ta) CONTAINS $query
   OR toString(coalesce(faasn, 0)) CONTAINS $query OR ('as' + toString(coalesce(faasn, 0))) CONTAINS $query OR toLower(coalesce(faorg, '')) CONTAINS $query
   OR toString(coalesce(taasn, 0)) CONTAINS $query OR ('as' + toString(coalesce(taasn, 0))) CONTAINS $query OR toLower(coalesce(taorg, '')) CONTAINS $query
   OR toString(round(rtt)) CONTAINS $query OR toString(sc) CONTAINS $query OR toString(ls) CONTAINS $query
RETURN fa, ta, rtt, sc, ls, faasn, faorg, taasn, taorg
ORDER BY `+sortExpr+` `+dir+`, fa ASC, ta ASC SKIP $offset LIMIT $limit`,
		map[string]any{"minRtt": f.MinRtt, "limit": f.Limit, "offset": f.Offset, "query": strings.ToLower(strings.TrimSpace(f.Query))})
	if err != nil {
		return nil, err
	}
	return scanHotHops(rows), nil
}

// HopContexts enriches a bounded list of historically correlated hop IPs with
// their live ASN identity and observed graph degree in one query.
func (s *Store) HopContexts(ctx context.Context, addrs []string) (map[string]HopContext, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return map[string]HopContext{}, nil
	}
	if len(addrs) > 100 {
		addrs = addrs[:100]
	}
	rows, err := s.rows(ctx, `
MATCH (ip:IP)
WHERE ip.addr IN $addrs
OPTIONAL MATCH (ip)-[:IN_AS]->(as:AS)
OPTIONAL MATCH (prev:IP)-[:NEXT_HOP]->(ip)
WITH ip, as, count(DISTINCT prev) AS incoming
OPTIONAL MATCH (ip)-[:NEXT_HOP]->(next:IP)
RETURN ip.addr AS addr, as.asn AS asn, as.org AS org,
       incoming, count(DISTINCT next) AS outgoing
LIMIT 100`, map[string]any{"addrs": addrs})
	if err != nil {
		return nil, err
	}
	out := make(map[string]HopContext, len(rows))
	for _, r := range rows {
		addr := asString(r["addr"])
		out[addr] = HopContext{
			Addr: addr, ASN: ptrIf(asInt(r["asn"]), int64(0)), Org: asString(r["org"]),
			Incoming: asInt(r["incoming"]), Outgoing: asInt(r["outgoing"]),
		}
	}
	return out, nil
}

func (s *Store) TransitEdges(ctx context.Context, f TransitFilter) ([]ASNTransitEdge, error) {
	f.Limit = clampLimit(f.Limit, 50, 501)
	if f.Offset < 0 {
		f.Offset = 0
	}
	sortExpr := validatedSort(f.Sort, "sc", transitSortExpr)
	dir := sortDir(f.Order)
	rows, err := s.rows(ctx, `
MATCH (a:AS)-[e:TRANSITS]->(b:AS)
WITH a.asn AS sasn, a.org AS sorg, b.asn AS dasn, b.org AS dorg,
     e.seen_count AS sc, e.last_seen AS ls
WHERE $query = '' OR toString(sasn) CONTAINS $query OR ('as' + toString(sasn)) CONTAINS $query OR toLower(coalesce(sorg, '')) CONTAINS $query
   OR toString(dasn) CONTAINS $query OR ('as' + toString(dasn)) CONTAINS $query OR toLower(coalesce(dorg, '')) CONTAINS $query
   OR toString(sc) CONTAINS $query OR toString(ls) CONTAINS $query
RETURN sasn, sorg, dasn, dorg, sc, ls
ORDER BY `+sortExpr+` `+dir+`, sasn ASC, dasn ASC SKIP $offset LIMIT $limit`, map[string]any{
		"limit": f.Limit, "offset": f.Offset, "query": strings.ToLower(strings.TrimSpace(f.Query)),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ASNTransitEdge, 0, len(rows))
	for _, r := range rows {
		out = append(out, ASNTransitEdge{
			SrcASN: asInt(r["sasn"]), SrcOrg: asString(r["sorg"]),
			DstASN: asInt(r["dasn"]), DstOrg: asString(r["dorg"]),
			SeenCount: asInt(r["sc"]), LastSeen: asInt(r["ls"]),
		})
	}
	return out, nil
}

func (s *Store) TransitPairDetail(ctx context.Context, asnA, asnB int64) (TransitPairDetail, error) {
	var d TransitPairDetail
	rows, err := s.rows(ctx, `
MATCH (a:AS {asn: $a})-[e:TRANSITS]->(b:AS {asn: $b})
RETURN a.org AS sorg, b.org AS dorg, e.seen_count AS sc, e.last_seen AS ls`,
		map[string]any{"a": asnA, "b": asnB})
	if err != nil {
		return d, err
	}
	if len(rows) == 0 {
		return d, fmt.Errorf("transit pair %d->%d not found", asnA, asnB)
	}
	r := rows[0]
	d.SrcASN, d.DstASN = asnA, asnB
	d.SrcOrg = asString(r["sorg"])
	d.DstOrg = asString(r["dorg"])
	d.SeenCount = asInt(r["sc"])
	d.LastSeen = asInt(r["ls"])
	d.Hops = []HotHop{}

	// hops realizing this transit: NEXT_HOP edges whose endpoints are in A and B.
	hops, err := s.rows(ctx, `
MATCH (a:IP)-[:IN_AS]->(asA:AS {asn: $a})
MATCH (a)-[e:NEXT_HOP]->(b:IP)-[:IN_AS]->(asB:AS {asn: $b})
WITH a.addr AS fa, b.addr AS ta, e.last_rtt_ms AS rtt, e.seen_count AS sc, e.last_seen AS ls
RETURN fa, ta, rtt, sc, ls
ORDER BY rtt DESC LIMIT 50`, map[string]any{"a": asnA, "b": asnB})
	if err != nil {
		return d, err
	}
	for _, rr := range hops {
		d.Hops = append(d.Hops, HotHop{
			FromAddr: asString(rr["fa"]), ToAddr: asString(rr["ta"]),
			FromASN: ptrIf(asnA, int64(0)), ToASN: ptrIf(asnB, int64(0)),
			LastRttMs: asFloat(rr["rtt"]), SeenCount: asInt(rr["sc"]),
			LastSeen: asInt(rr["ls"]),
		})
	}
	d.Tests, err = s.TransitPairTests(ctx, asnA, asnB, 200)
	if err != nil {
		return d, err
	}
	return d, nil
}

func (s *Store) TransitPairTests(ctx context.Context, asnA, asnB int64, limit int) ([]TransitTest, error) {
	limit = clampLimit(limit, 100, 500)
	rows, err := s.rows(ctx, `
MATCH (srcAS:AS {asn: $a})<-[:IN_AS]-(src:IP)<-[:LOCATED_AT]-(p:Probe)-[e:PING]->(t:IP)-[:IN_AS]->(dstAS:AS {asn: $b})
WHERE e.sent > 0 AND e.last_seen >= $cutoff
RETURN p.id AS probe_id, e.msm_id AS msm_id, src.addr AS source_ip, t.addr AS target_ip,
       e.sent AS sent, e.rcvd AS received, round(100.0 * e.loss_ratio) AS loss_pct,
       e.avg_rtt_ms AS avg_rtt_ms, e.min_rtt_ms AS min_rtt_ms, e.max_rtt_ms AS max_rtt_ms,
       e.last_seen AS last_seen, `+probeMetadataProjection+`
ORDER BY e.loss_ratio DESC, e.avg_rtt_ms DESC, e.last_seen DESC
LIMIT $limit`, map[string]any{
		"a": asnA, "b": asnB, "cutoff": s.activeCutoff(), "limit": limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]TransitTest, 0, len(rows))
	for _, r := range rows {
		out = append(out, TransitTest{
			ProbeID: asInt(r["probe_id"]), ProbeMetadata: probeMetadataFromRow(r), MsmID: asInt(r["msm_id"]),
			SourceIP: asString(r["source_ip"]), TargetIP: asString(r["target_ip"]),
			Sent: asInt(r["sent"]), Received: asInt(r["received"]),
			LossPct: asFloat(r["loss_pct"]), AvgRttMs: asFloat(r["avg_rtt_ms"]),
			MinRttMs: asFloat(r["min_rtt_ms"]), MaxRttMs: asFloat(r["max_rtt_ms"]),
			LastSeen: asInt(r["last_seen"]),
		})
	}
	return out, nil
}

// ---- IP detail --------------------------------------------------------------

func (s *Store) IPDetail(ctx context.Context, addr string) (IPDetail, error) {
	var d IPDetail
	rows, err := s.rows(ctx, `
MATCH (ip:IP {addr: $addr})
OPTIONAL MATCH (ip)-[:IN_AS]->(as:AS)
RETURN ip.af AS af, ip.last_seen AS ls, as.asn AS asn, as.org AS org`,
		map[string]any{"addr": addr})
	if err != nil {
		return d, err
	}
	if len(rows) == 0 {
		return d, fmt.Errorf("ip %s not found", addr)
	}
	r := rows[0]
	d.Addr = addr
	d.ASN = ptrIf(asInt(r["asn"]), int64(0))
	d.Org = asString(r["org"])
	d.AF = asInt(r["af"])
	d.LastSeen = asInt(r["ls"])

	inCounts, err := s.rows(ctx, `MATCH ()-[e:NEXT_HOP]->(:IP {addr: $addr}) RETURN count(e) AS count`, map[string]any{"addr": addr})
	if err != nil {
		return d, err
	}
	if len(inCounts) > 0 {
		d.IncomingHops = asInt(inCounts[0]["count"])
	}
	outCounts, err := s.rows(ctx, `MATCH (:IP {addr: $addr})-[e:NEXT_HOP]->() RETURN count(e) AS count`, map[string]any{"addr": addr})
	if err != nil {
		return d, err
	}
	if len(outCounts) > 0 {
		d.OutgoingHops = asInt(outCounts[0]["count"])
	}
	return d, nil
}

func (s *Store) IPHops(ctx context.Context, addr, direction string, f HopFilter) ([]HotHop, error) {
	f.Limit = clampLimit(f.Limit, 25, 501)
	if f.Offset < 0 {
		f.Offset = 0
	}
	sortExpr := validatedSort(f.Sort, "rtt", hopSortExpr)
	dir := sortDir(f.Order)
	params := map[string]any{
		"addr": addr, "limit": f.Limit, "offset": f.Offset,
		"query": strings.ToLower(strings.TrimSpace(f.Query)),
	}
	var q string
	switch direction {
	case "in":
		q = `
MATCH (a:IP)-[e:NEXT_HOP]->(b:IP {addr: $addr})
OPTIONAL MATCH (a)-[:IN_AS]->(aas:AS)
WITH a.addr AS fa, b.addr AS ta, e.last_rtt_ms AS rtt, e.seen_count AS sc, e.last_seen AS ls,
     aas.asn AS faasn, aas.org AS faorg
WHERE $query = '' OR toLower(fa) CONTAINS $query
   OR toString(coalesce(faasn, 0)) CONTAINS $query OR ('as' + toString(coalesce(faasn, 0))) CONTAINS $query
   OR toLower(coalesce(faorg, '')) CONTAINS $query OR toString(round(rtt)) CONTAINS $query
   OR toString(sc) CONTAINS $query OR toString(ls) CONTAINS $query
RETURN fa, ta, rtt, sc, ls, faasn, faorg
ORDER BY ` + sortExpr + ` ` + dir + `, fa ASC SKIP $offset LIMIT $limit`
	case "out":
		q = `
MATCH (a:IP {addr: $addr})-[e:NEXT_HOP]->(b:IP)
OPTIONAL MATCH (b)-[:IN_AS]->(bas:AS)
WITH a.addr AS fa, b.addr AS ta, e.last_rtt_ms AS rtt, e.seen_count AS sc, e.last_seen AS ls,
     bas.asn AS taasn, bas.org AS taorg
WHERE $query = '' OR toLower(ta) CONTAINS $query
   OR toString(coalesce(taasn, 0)) CONTAINS $query OR ('as' + toString(coalesce(taasn, 0))) CONTAINS $query
   OR toLower(coalesce(taorg, '')) CONTAINS $query OR toString(round(rtt)) CONTAINS $query
   OR toString(sc) CONTAINS $query OR toString(ls) CONTAINS $query
RETURN fa, ta, rtt, sc, ls, taasn, taorg
ORDER BY ` + sortExpr + ` ` + dir + `, ta ASC SKIP $offset LIMIT $limit`
	default:
		return nil, fmt.Errorf("invalid hop direction %q", direction)
	}
	rows, err := s.rows(ctx, q, params)
	if err != nil {
		return nil, err
	}
	return scanHotHops(rows), nil
}

// nearbyHops returns NEXT_HOP edges on either side of addr (the union of in/out).
func (s *Store) nearbyHops(ctx context.Context, addr string, limit int) ([]HotHop, error) {
	limit = clampLimit(limit, 20, 200)
	rows, err := s.rows(ctx, `
MATCH (a:IP)-[e:NEXT_HOP]->(b:IP)
WHERE a.addr = $addr OR b.addr = $addr
OPTIONAL MATCH (a)-[:IN_AS]->(aas:AS)
OPTIONAL MATCH (b)-[:IN_AS]->(bas:AS)
WITH a.addr AS fa, b.addr AS ta, e.last_rtt_ms AS rtt, e.seen_count AS sc, e.last_seen AS ls,
     aas.asn AS faasn, aas.org AS faorg, bas.asn AS taasn, bas.org AS taorg
RETURN fa, ta, rtt, sc, ls, faasn, faorg, taasn, taorg
ORDER BY rtt DESC LIMIT $limit`,
		map[string]any{"addr": addr, "limit": limit})
	if err != nil {
		return nil, err
	}
	return scanHotHops(rows), nil
}

// scanHotHops normalizes a row set with the fa/ta/rtt/sc/ls/faasn/faorg/taasn/taorg
// columns used by HotHops, nearbyHops and IPDetail queries.
func scanHotHops(rows []map[string]any) []HotHop {
	out := make([]HotHop, 0, len(rows))
	for _, r := range rows {
		out = append(out, HotHop{
			FromAddr: asString(r["fa"]), ToAddr: asString(r["ta"]),
			FromASN: ptrIf(asInt(r["faasn"]), int64(0)), FromOrg: asString(r["faorg"]),
			ToASN: ptrIf(asInt(r["taasn"]), int64(0)), ToOrg: asString(r["taorg"]),
			LastRttMs: asFloat(r["rtt"]), SeenCount: asInt(r["sc"]),
			LastSeen: asInt(r["ls"]),
		})
	}
	return out
}

// ---- Path between two IPs ---------------------------------------------------

func (s *Store) Path(ctx context.Context, src, dst string, maxHops int) (Path, error) {
	var p Path
	if maxHops <= 0 || maxHops > 30 {
		maxHops = 15
	}
	// FalkorDB requires shortestPath() to appear in a WITH/RETURN clause (not a
	// bare MATCH). We bind it via WITH, then project nodes()/relationships().
	rows, err := s.rows(ctx, fmt.Sprintf(`
MATCH (src:IP {addr: $src}), (dst:IP {addr: $dst})
WITH shortestPath((src)-[:NEXT_HOP*1..%d]->(dst)) AS pth
RETURN [n IN nodes(pth) | n.addr] AS addrs,
       [r IN relationships(pth) | r.last_rtt_ms] AS rtts,
       [r IN relationships(pth) | r.seen_count] AS scs`, maxHops),
		map[string]any{"src": src, "dst": dst})
	if err != nil {
		return p, err
	}
	if len(rows) == 0 {
		return p, nil
	}
	r := rows[0]
	addrs, _ := r["addrs"].([]any)
	rtts, _ := r["rtts"].([]any)
	scs, _ := r["scs"].([]any)
	if len(addrs) == 0 {
		return p, nil
	}
	p.Found = true
	// Enrich each hop address with ASN (single round-trip batch).
	asnMap, _ := s.asnBatch(ctx, addrs)
	for i, a := range addrs {
		addr := asString(a)
		hop := PathHop{Addr: addr, RttMs: -1}
		if i > 0 {
			hop.RttMs = asFloat(rtts[i-1])
			hop.SeenCount = asInt(scs[i-1])
		}
		if asn, ok := asnMap[addr]; ok {
			hop.ASN = ptrIf(asn.asn, int64(0))
			hop.Org = asn.org
		}
		p.Hops = append(p.Hops, hop)
	}
	return p, nil
}

// asnInfo holds the AS enrichment for a single IP.
type asnInfo struct {
	asn int64
	org string
}

// asnBatch resolves ASN+org for a list of addresses in one query.
func (s *Store) asnBatch(ctx context.Context, addrs []any) (map[string]asnInfo, error) {
	if len(addrs) == 0 {
		return nil, nil
	}
	rows, err := s.rows(ctx, `
UNWIND $addrs AS addr
MATCH (ip:IP {addr: addr})-[:IN_AS]->(as:AS)
RETURN ip.addr AS addr, as.asn AS asn, as.org AS org`,
		map[string]any{"addrs": addrs})
	if err != nil {
		return nil, err
	}
	m := make(map[string]asnInfo, len(rows))
	for _, r := range rows {
		m[asString(r["addr"])] = asnInfo{asn: asInt(r["asn"]), org: asString(r["org"])}
	}
	return m, nil
}

// ---- Subgraph for topology view --------------------------------------------
//
// The topology view needs nodes keyed by natural identifiers (IP addr, ASN,
// probe id) with edges referencing those same keys. Rather than round-tripping
// FalkorDB *Node/*Edge objects and reconciling internal node IDs afterward, we
// project nodes and each relationship type out to typed rows in two passes:
// pass 1 collects the bounded set of IP/AS/Probe node ids; pass 2 emits edges
// by type with endpoint addresses/ASNs. This keeps the JSON payload small and
// the endpoint keys consistent.

func (s *Store) Subgraph(ctx context.Context, f SubgraphFilter) (Subgraph, error) {
	f.Depth = clampLimit(f.Depth, 2, 4)
	f.Limit = clampLimit(f.Limit, 80, 300)

	var seed string
	params := map[string]any{"limit": f.Limit}
	switch {
	case f.Target != "":
		seed = "MATCH (seed:IP {addr: $seed})"
		params["seed"] = f.Target
	case f.Probe > 0:
		seed = "MATCH (seed:Probe {id: $seed})"
		params["seed"] = f.Probe
	case f.ASN > 0:
		seed = "MATCH (seed:AS {asn: $seed})"
		params["seed"] = f.ASN
	default:
		// Default seed: the most-connected AS, so the view always shows something.
		seed = "MATCH (seed:AS) WITH seed, size((seed)--()) AS deg ORDER BY deg DESC LIMIT 1 WITH seed"
	}

	// Collect a bounded neighborhood of node ids (FalkorDB internal IDs).
	idRows, err := s.rows(ctx, fmt.Sprintf(`
%s
MATCH pth = (seed)-[*1..%d]-(related)
UNWIND [n IN nodes(pth) | id(n)] AS nid
RETURN collect(DISTINCT nid)[0..$limit] AS ids`, seed, f.Depth), params)
	if err != nil {
		return Subgraph{}, err
	}
	if len(idRows) == 0 {
		return Subgraph{}, nil
	}
	ids, _ := idRows[0]["ids"].([]any)
	if len(ids) == 0 {
		return Subgraph{}, nil
	}

	var sg Subgraph
	// Nodes by label.
	ipRows, _ := s.rows(ctx, `
MATCH (n:IP) WHERE id(n) IN $ids
OPTIONAL MATCH (n)-[:IN_AS]->(as:AS)
RETURN n.addr AS addr, n.asn AS asn, as.org AS org`,
		map[string]any{"ids": ids})
	for _, r := range ipRows {
		gn := GraphNode{ID: asString(r["addr"]), Kind: "ip", Label: asString(r["addr"])}
		if asn := asInt(r["asn"]); asn > 0 {
			gn.ASN = ptrIf(asn, int64(0))
		}
		gn.Org = asString(r["org"])
		sg.Nodes = append(sg.Nodes, gn)
	}
	asRows, _ := s.rows(ctx, `
MATCH (n:AS) WHERE id(n) IN $ids
RETURN n.asn AS asn, n.org AS org`, map[string]any{"ids": ids})
	for _, r := range asRows {
		asn := asInt(r["asn"])
		sg.Nodes = append(sg.Nodes, GraphNode{
			ID: "AS" + asString(r["asn"]), Kind: "as", Label: "AS" + asString(r["asn"]),
			ASN: ptrIf(asn, int64(0)), Org: asString(r["org"]),
		})
	}
	prbRows, _ := s.rows(ctx, `
MATCH (n:Probe) WHERE id(n) IN $ids
WITH n AS p
RETURN p.id AS id, `+probeMetadataProjection, map[string]any{"ids": ids})
	for _, r := range prbRows {
		id := asString(r["id"])
		metadata := probeMetadataFromRow(r)
		label := "Probe " + id
		if metadata != nil && metadata.DisplayName != "" {
			label = metadata.DisplayName
		}
		sg.Nodes = append(sg.Nodes, GraphNode{
			ID: "P" + id, Kind: "probe", Label: label, ProbeMetadata: metadata,
		})
	}

	// Edges by type. Each query restricts both endpoints to the collected id set
	// so the subgraph stays bounded; endpoint keys use node properties directly.
	add := func(rows []map[string]any, kind string, props func(r map[string]any) GraphEdge) {
		for _, r := range rows {
			ge := props(r)
			ge.Kind = kind
			sg.Edges = append(sg.Edges, ge)
		}
	}
	p := map[string]any{"ids": ids}

	if rows, err := s.rows(ctx, `
MATCH (a:IP)-[e:NEXT_HOP]->(b:IP) WHERE id(a) IN $ids AND id(b) IN $ids
RETURN a.addr AS f, b.addr AS t, e.last_rtt_ms AS rtt, e.seen_count AS sc`, p); err == nil {
		add(rows, "next_hop", func(r map[string]any) GraphEdge {
			return GraphEdge{From: asString(r["f"]), To: asString(r["t"]),
				LastRttMs: asFloat(r["rtt"]), SeenCount: asInt(r["sc"])}
		})
	}
	if rows, err := s.rows(ctx, `
MATCH (a:IP)-[e:IN_AS]->(b:AS) WHERE id(a) IN $ids AND id(b) IN $ids
RETURN a.addr AS f, b.asn AS t`, p); err == nil {
		add(rows, "in_as", func(r map[string]any) GraphEdge {
			return GraphEdge{From: asString(r["f"]), To: "AS" + asString(r["t"])}
		})
	}
	if rows, err := s.rows(ctx, `
MATCH (a:AS)-[e:TRANSITS]->(b:AS) WHERE id(a) IN $ids AND id(b) IN $ids
RETURN a.asn AS f, b.asn AS t, e.seen_count AS sc`, p); err == nil {
		add(rows, "transits", func(r map[string]any) GraphEdge {
			return GraphEdge{From: "AS" + asString(r["f"]), To: "AS" + asString(r["t"]),
				SeenCount: asInt(r["sc"])}
		})
	}
	if rows, err := s.rows(ctx, `
MATCH (a:Probe)-[e:PING]->(b:IP) WHERE id(a) IN $ids AND id(b) IN $ids
RETURN a.id AS f, b.addr AS t, e.loss_ratio AS loss, e.avg_rtt_ms AS rtt`, p); err == nil {
		add(rows, "ping", func(r map[string]any) GraphEdge {
			return GraphEdge{From: "P" + asString(r["f"]), To: asString(r["t"]),
				LossRatio: asFloat(r["loss"]), LastRttMs: asFloat(r["rtt"])}
		})
	}
	if rows, err := s.rows(ctx, `
MATCH (a:Probe)-[e:TARGETS]->(b:IP) WHERE id(a) IN $ids AND id(b) IN $ids
RETURN a.id AS f, b.addr AS t`, p); err == nil {
		add(rows, "targets", func(r map[string]any) GraphEdge {
			return GraphEdge{From: "P" + asString(r["f"]), To: asString(r["t"])}
		})
	}
	if rows, err := s.rows(ctx, `
MATCH (a:Probe)-[e:LOCATED_AT]->(b:IP) WHERE id(a) IN $ids AND id(b) IN $ids
RETURN a.id AS f, b.addr AS t`, p); err == nil {
		add(rows, "located_at", func(r map[string]any) GraphEdge {
			return GraphEdge{From: "P" + asString(r["f"]), To: asString(r["t"])}
		})
	}
	return sg, nil
}

// ---- Unused but kept to satisfy Reader API surface -------------------------
// (no-op; methods above implement the full Reader interface)
