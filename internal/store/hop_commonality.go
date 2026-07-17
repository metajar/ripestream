package store

import (
	"context"
	"fmt"
	"math"
	"time"
)

// HopCommonalityFilter bounds the historical route-correlation query.
type HopCommonalityFilter struct {
	Recent    time.Duration
	Baseline  time.Duration
	MinProbes int
	Limit     int
}

// hopBaselineSampleDivisor bounds the expensive raw-JSON expansion in the
// historical window. Hash sampling is deterministic, so repeated requests use
// the same baseline population while the recent incident window remains exact.
const hopBaselineSampleDivisor = 4

// HopCommonality is one transit hop whose RTT changed at the same time across
// otherwise independent traceroutes. It is evidence of shared fate, not proof
// that the router itself caused an incident (ICMP replies may be deprioritized).
type HopCommonality struct {
	Addr                string   `json:"addr"`
	RecentMedianRttMs   float64  `json:"recent_median_rtt_ms"`
	BaselineMedianRttMs float64  `json:"baseline_median_rtt_ms"`
	RttDeltaMs          float64  `json:"rtt_delta_ms"`
	ChangePct           float64  `json:"change_pct"`
	ImpactScore         float64  `json:"impact_score"`
	RecentSamples       int64    `json:"recent_samples"`
	BaselineSamples     int64    `json:"baseline_samples"`
	Probes              int64    `json:"probes"`
	Targets             int64    `json:"targets"`
	Traces              int64    `json:"traces"`
	AffectedProbes      []int64  `json:"affected_probes"`
	AffectedTargets     []string `json:"affected_targets"`
	FirstSeen           int64    `json:"first_seen"`
	LastSeen            int64    `json:"last_seen"`
}

// HopCommonalities reconstructs responsive traceroute hops from raw Atlas JSON
// and compares each hop's recent median RTT with its own preceding baseline.
// Destination replies are excluded so candidates represent shared transit
// infrastructure rather than simply repeating a slow endpoint.
func (s *Store) HopCommonalities(ctx context.Context, now time.Time, f HopCommonalityFilter) ([]HopCommonality, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if f.Recent <= 0 {
		f.Recent = 30 * time.Minute
	}
	if f.Baseline <= 0 {
		f.Baseline = 24 * time.Hour
	}
	if f.MinProbes < 2 {
		f.MinProbes = 3
	}
	if f.MinProbes > 50 {
		f.MinProbes = 50
	}
	if f.Limit <= 0 {
		f.Limit = 40
	}
	if f.Limit > 100 {
		f.Limit = 100
	}

	recentStart := now.Add(-f.Recent).Unix()
	baselineStart := now.Add(-f.Recent - f.Baseline).Unix()
	q := buildHopCommonalityQuery(s.db, s.table, baselineStart, recentStart, now.Unix(), f.MinProbes, f.Limit)
	rows, err := s.QueryJSON(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]HopCommonality, 0, len(rows))
	for _, r := range rows {
		out = append(out, HopCommonality{
			Addr:              asText(r["addr"]),
			RecentMedianRttMs: toF(r["recent_rtt"]), BaselineMedianRttMs: toF(r["baseline_rtt"]),
			RttDeltaMs: toF(r["delta_ms"]), ChangePct: toF(r["change_pct"]),
			ImpactScore: finiteScore(toF(r["impact_score"])), RecentSamples: toI(r["recent_samples"]),
			BaselineSamples: toI(r["baseline_samples"]), Probes: toI(r["probes"]),
			Targets: toI(r["targets"]), Traces: toI(r["traces"]),
			AffectedProbes: int64Slice(r["affected_probes"]), AffectedTargets: stringSlice(r["affected_targets"]),
			FirstSeen: parseClickHouseEpoch(r["first_seen"]), LastSeen: parseClickHouseEpoch(r["last_seen"]),
		})
	}
	return out, nil
}

func buildHopCommonalityQuery(db, table string, baselineStart, recentStart, end int64, minProbes, limit int) string {
	return fmt.Sprintf(`
WITH recent_hop_rows AS
(
    SELECT timestamp, prb_id, dst_addr,
           arrayJoin(JSONExtractArrayRaw(result_json, 'result')) AS hop_json
    FROM %s.%s
    PREWHERE type = 'traceroute'
      AND timestamp >= toDateTime(%d)
      AND timestamp < toDateTime(%d)
),
recent_observations AS
(
    SELECT timestamp, prb_id, dst_addr,
           JSONExtractString(reply_json, 'from') AS addr,
           JSONExtractFloat(reply_json, 'rtt') AS rtt
    FROM recent_hop_rows
    ARRAY JOIN JSONExtractArrayRaw(hop_json, 'result') AS reply_json
    WHERE addr != '' AND addr != dst_addr AND rtt > 0 AND rtt < 10000
),
recent_candidates AS
(
    SELECT addr
    FROM recent_observations
    GROUP BY addr
    HAVING uniqCombined64(prb_id) >= %d
       AND count() >= greatest(6, uniqCombined64(prb_id) * 2)
       AND quantileTDigest(0.5)(rtt) >= 15
),
hop_rows AS
(
    SELECT timestamp, msm_id, prb_id, dst_addr,
           arrayJoin(JSONExtractArrayRaw(result_json, 'result')) AS hop_json
    FROM %s.%s
    PREWHERE type = 'traceroute'
      AND timestamp >= toDateTime(%d)
      AND timestamp < toDateTime(%d)
    WHERE timestamp >= toDateTime(%d)
       OR cityHash64(msm_id, prb_id, timestamp) %% %d = 0
),
observations AS
(
    SELECT timestamp, msm_id, prb_id, dst_addr,
           JSONExtractString(reply_json, 'from') AS addr,
           JSONExtractFloat(reply_json, 'rtt') AS rtt
	FROM hop_rows
	ARRAY JOIN JSONExtractArrayRaw(hop_json, 'result') AS reply_json
	WHERE addr != '' AND addr != dst_addr AND rtt > 0 AND rtt < 10000
      AND addr IN (SELECT addr FROM recent_candidates)
)
SELECT addr,
       round(quantileTDigestIf(0.5)(rtt, timestamp >= toDateTime(%d)), 1) AS recent_rtt,
       round(quantileTDigestIf(0.5)(rtt, timestamp < toDateTime(%d)), 1) AS baseline_rtt,
       round(recent_rtt - baseline_rtt, 1) AS delta_ms,
       round(100.0 * delta_ms / nullIf(baseline_rtt, 0), 1) AS change_pct,
       countIf(timestamp >= toDateTime(%d)) AS recent_samples,
       countIf(timestamp < toDateTime(%d)) AS baseline_samples,
       uniqCombined64If(prb_id, timestamp >= toDateTime(%d)) AS probes,
       uniqCombined64If(dst_addr, timestamp >= toDateTime(%d)) AS targets,
       uniqCombined64If(tuple(msm_id, prb_id, timestamp), timestamp >= toDateTime(%d)) AS traces,
       groupUniqArrayIf(12)(prb_id, timestamp >= toDateTime(%d)) AS affected_probes,
       groupUniqArrayIf(12)(dst_addr, timestamp >= toDateTime(%d)) AS affected_targets,
       minIf(timestamp, timestamp >= toDateTime(%d)) AS first_seen,
       maxIf(timestamp, timestamp >= toDateTime(%d)) AS last_seen,
       round(delta_ms * sqrt(probes * greatest(targets, 1)), 1) AS impact_score
FROM observations
GROUP BY addr
HAVING probes >= %d
   AND recent_samples >= greatest(6, probes * 2)
   AND baseline_samples >= 12
   AND baseline_rtt > 0
   AND delta_ms >= 15
   AND recent_rtt >= baseline_rtt * 1.35
ORDER BY impact_score DESC, probes DESC
	LIMIT %d
SETTINGS max_threads = 2,
         max_block_size = 2048,
         max_bytes_before_external_group_by = 268435456,
         max_bytes_before_external_sort = 67108864`,
		db, table, recentStart, end, minProbes,
		db, table, baselineStart, end, recentStart, hopBaselineSampleDivisor,
		recentStart, recentStart, recentStart, recentStart, recentStart, recentStart,
		recentStart, recentStart, recentStart, recentStart, recentStart,
		minProbes, limit)
}

func asText(v any) string {
	s, _ := v.(string)
	return s
}

func int64Slice(v any) []int64 {
	raw, ok := v.([]any)
	if !ok {
		return []int64{}
	}
	out := make([]int64, 0, len(raw))
	for _, item := range raw {
		out = append(out, toI(item))
	}
	return out
}

func stringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parseClickHouseEpoch(v any) int64 {
	s, ok := v.(string)
	if !ok || s == "" {
		return toI(v)
	}
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return 0
	}
	return t.UTC().Unix()
}

// Keep math imported alongside the scoring semantics, and guard future query
// decoding changes from ever leaking a non-finite score into JSON.
func finiteScore(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}
