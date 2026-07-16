package store

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SeriesPoint is one bucketed time-series sample for charts.
type SeriesPoint struct {
	Ts      time.Time `json:"ts"`
	Value   float64   `json:"value"`
	Samples int64     `json:"samples"`
}

// PairSeriesPoint carries both health signals for one transit-pair time bucket.
type PairSeriesPoint struct {
	Ts       time.Time `json:"ts"`
	LossPct  float64   `json:"loss_pct"`
	AvgRttMs float64   `json:"avg_rtt_ms"`
	Samples  int64     `json:"samples"`
	Probes   int64     `json:"probes"`
	Targets  int64     `json:"targets"`
}

// PingSeries pulls aggregated ping metrics from the raw atlas_results table,
// bucketed into N intervals over [from, to]. Because all metrics live in the
// result_json payload, we use ClickHouse's JSONExtract* functions.
//
// metric ∈ {"loss_pct", "avg_rtt_ms", "min_rtt_ms", "max_rtt_ms"}.
// probe (0 = any), target ("" = any) scope the rows. buckets defaults to 60.
func (s *Store) PingSeries(ctx context.Context, metric string, probe int64, target string, from, to time.Time, buckets int) ([]SeriesPoint, error) {
	if buckets <= 0 {
		buckets = 60
	}
	if buckets > 1000 {
		buckets = 1000
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-24 * time.Hour)
	}

	var expr string
	switch metric {
	case "loss_pct":
		// loss = (sent-rcvd)/sent * 100. Guard divide-by-zero.
		expr = "round(100.0 * (toFloat64OrZero(JSONExtractString(result_json,'sent')) - " +
			"toFloat64OrZero(JSONExtractString(result_json,'rcvd'))) / " +
			"nullIf(toFloat64OrZero(JSONExtractString(result_json,'sent')), 0))"
	case "avg_rtt_ms", "min_rtt_ms", "max_rtt_ms":
		// RIPE Atlas payload keys are avg/min/max; the API uses explicit metric
		// names so callers know these values are RTT milliseconds.
		jsonKey := strings.TrimSuffix(metric, "_rtt_ms")
		expr = "toFloat64OrZero(JSONExtractString(result_json, " + EscStr(jsonKey) + "))"
	default:
		return nil, fmt.Errorf("unknown metric %q", metric)
	}

	q := fmt.Sprintf(`
SELECT toStartOfInterval(timestamp, INTERVAL %s SECOND) AS bkt,
       avg(%s) AS val,
       count() AS samples
FROM %s.%s
WHERE type = 'ping'
  AND timestamp >= toDateTime(%d)
  AND timestamp <  toDateTime(%d)`,
		strconv.Itoa(int(to.Sub(from).Seconds()/float64(buckets))), expr, s.db, s.table,
		from.Unix(), to.Unix())

	if probe > 0 {
		q += fmt.Sprintf(" AND prb_id = %d", probe)
	}
	if target != "" {
		q += " AND dst_addr = " + EscStr(target)
	}
	q += " GROUP BY bkt ORDER BY bkt"

	rows, err := s.QueryJSON(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]SeriesPoint, 0, len(rows))
	for _, r := range rows {
		ts, ok := r["bkt"].(string)
		if !ok {
			continue
		}
		t, err := time.Parse("2006-01-02 15:04:05", ts)
		if err != nil {
			continue
		}
		out = append(out, SeriesPoint{
			Ts:      t.UTC(),
			Value:   toF(r["val"]),
			Samples: toI(r["samples"]),
		})
	}
	return out, nil
}

// PingPairSeries aggregates historical pings between graph-resolved source
// probes and destination targets. The scope is endpoint-to-endpoint evidence;
// it does not claim every ping traversed the selected traceroute relationship.
func (s *Store) PingPairSeries(ctx context.Context, probes []int64, targets []string, from, to time.Time, buckets int) ([]PairSeriesPoint, error) {
	q, err := buildPingPairSeriesQuery(s.db, s.table, probes, targets, from, to, buckets)
	if err != nil {
		return nil, err
	}
	rows, err := s.QueryJSON(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]PairSeriesPoint, 0, len(rows))
	for _, r := range rows {
		ts, ok := r["bkt"].(string)
		if !ok {
			continue
		}
		t, err := time.Parse("2006-01-02 15:04:05", ts)
		if err != nil {
			continue
		}
		out = append(out, PairSeriesPoint{
			Ts: t.UTC(), LossPct: toF(r["loss_pct"]), AvgRttMs: toF(r["avg_rtt_ms"]),
			Samples: toI(r["samples"]), Probes: toI(r["probes"]), Targets: toI(r["targets"]),
		})
	}
	return out, nil
}

func buildPingPairSeriesQuery(db, table string, probes []int64, targets []string, from, to time.Time, buckets int) (string, error) {
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-24 * time.Hour)
	}
	if !from.Before(to) {
		return "", fmt.Errorf("series start must be before end")
	}
	if buckets <= 0 {
		buckets = 60
	}
	if buckets > 500 {
		buckets = 500
	}

	probeSet := make(map[int64]struct{}, len(probes))
	for _, id := range probes {
		if id > 0 && len(probeSet) < 500 {
			probeSet[id] = struct{}{}
		}
	}
	probeList := make([]int64, 0, len(probeSet))
	for id := range probeSet {
		probeList = append(probeList, id)
	}
	sort.Slice(probeList, func(i, j int) bool { return probeList[i] < probeList[j] })
	if len(probeList) == 0 {
		return "", fmt.Errorf("at least one probe is required")
	}
	probeSQL := make([]string, len(probeList))
	for i, id := range probeList {
		probeSQL[i] = strconv.FormatInt(id, 10)
	}

	targetSet := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if target != "" && len(targetSet) < 500 {
			targetSet[target] = struct{}{}
		}
	}
	targetList := make([]string, 0, len(targetSet))
	for target := range targetSet {
		targetList = append(targetList, target)
	}
	sort.Strings(targetList)
	if len(targetList) == 0 {
		return "", fmt.Errorf("at least one target is required")
	}
	targetSQL := make([]string, len(targetList))
	for i, target := range targetList {
		targetSQL[i] = EscStr(target)
	}

	interval := int(to.Sub(from).Seconds() / float64(buckets))
	if interval < 1 {
		interval = 1
	}
	sent := "toFloat64OrZero(JSONExtractString(result_json,'sent'))"
	rcvd := "toFloat64OrZero(JSONExtractString(result_json,'rcvd'))"
	avg := "toFloat64OrZero(JSONExtractString(result_json,'avg'))"
	return fmt.Sprintf(`
SELECT toStartOfInterval(timestamp, INTERVAL %d SECOND) AS bkt,
       round(100.0 * (sum(%s) - sum(%s)) / nullIf(sum(%s), 0), 1) AS loss_pct,
       round(avgIf(%s, %s > 0), 1) AS avg_rtt_ms,
       count() AS samples, uniqExact(prb_id) AS probes, uniqExact(dst_addr) AS targets
FROM %s.%s
WHERE type = 'ping'
  AND timestamp >= toDateTime(%d)
  AND timestamp < toDateTime(%d)
  AND prb_id IN (%s)
  AND dst_addr IN (%s)
GROUP BY bkt ORDER BY bkt`, interval, sent, rcvd, sent, avg, rcvd,
		db, table, from.Unix(), to.Unix(), strings.Join(probeSQL, ","), strings.Join(targetSQL, ",")), nil
}

// toF coerces a ClickHouse JSON scalar (number or string) to float64.
func toF(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int64:
		return float64(t)
	case int:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

// toI coerces a ClickHouse JSON scalar to int64.
func toI(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case string:
		i, _ := strconv.ParseInt(t, 10, 64)
		return i
	}
	return 0
}
