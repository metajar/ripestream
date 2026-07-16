package store

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// SeriesPoint is one bucketed time-series sample for charts.
type SeriesPoint struct {
	Ts     time.Time `json:"ts"`
	Value  float64   `json:"value"`
	Samples int64    `json:"samples"`
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
		expr = "toFloat64OrZero(JSONExtractString(result_json, " + EscStr(metric) + "))"
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
