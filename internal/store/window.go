package store

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TargetBaseline is one target's packet-weighted historical PING aggregate.
type TargetBaseline struct {
	Target  string  `json:"target"`
	Sent    int64   `json:"sent"`
	Rcvd    int64   `json:"rcvd"`
	Samples int64   `json:"samples"`
	Probes  int64   `json:"probes"`
	LossPct float64 `json:"loss_pct"`
}

// WindowSummary is the aggregate packet-loss and RTT picture over a time window,
// scoped optionally to a target IP and/or probe. It returns counts (not just
// percentages) so every rate has its denominator. Baseline is the prior
// equal-duration window for comparison; BaselineAvailable is false when there
// isn't enough history to compute it.
type WindowSummary struct {
	// Current window.
	From        time.Time `json:"window_start"`
	To          time.Time `json:"window_end"`
	Sent        int64     `json:"sent"`          // total ping packets sent in window
	Rcvd        int64     `json:"rcvd"`          // total received
	Lost        int64     `json:"lost"`          // sent - rcvd
	LossPct     float64   `json:"loss_pct"`      // 100 * lost / sent (0 if sent==0)
	Samples     int64     `json:"samples"`       // number of ping results in window
	AvgRttMs    float64   `json:"avg_rtt_ms"`    // mean RTT across received replies (0 if none)
	MedianRttMs float64   `json:"median_rtt_ms"` // approximate median RTT (0 if none)
	// Baseline (prior equal-duration window).
	BaselineAvailable bool    `json:"baseline_available"`
	BaselineLossPct   float64 `json:"baseline_loss_pct"`
	BaselineAvgRttMs  float64 `json:"baseline_avg_rtt_ms"`
	ChangeLossPct     float64 `json:"change_loss_pct"` // current - baseline (percentage points)
	ChangeRttPct      float64 `json:"change_rtt_pct"`  // relative RTT change (%)
}

// maxWindowDays bounds the largest selectable window to avoid unbounded scans.
const maxWindowDays = 30

// PingWindowSummary computes loss/RTT aggregates over [from, to) and compares
// against the prior equal-duration window. A target IP and/or probe scope the
// rows. Returns BaselineAvailable=false when the prior window has no data.
func (s *Store) PingWindowSummary(ctx context.Context, target string, probe int64, from, to time.Time) (WindowSummary, error) {
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-24 * time.Hour)
	}
	if !to.After(from) {
		return WindowSummary{}, fmt.Errorf("invalid window: 'to' must be after 'from'")
	}
	dur := to.Sub(from)
	if dur > maxWindowDays*24*time.Hour {
		return WindowSummary{}, fmt.Errorf("window too large: max %d days", maxWindowDays)
	}

	cur, err := s.windowAggregate(ctx, target, probe, from, to)
	if err != nil {
		return WindowSummary{}, err
	}
	ws := WindowSummary{From: from, To: to, Sent: cur.sent, Rcvd: cur.rcvd, Samples: cur.samples,
		AvgRttMs: cur.avgRtt, MedianRttMs: cur.medianRtt}
	ws.Lost = ws.Sent - ws.Rcvd
	if ws.Sent > 0 {
		ws.LossPct = 100.0 * float64(ws.Lost) / float64(ws.Sent)
	}

	// Baseline: prior equal-duration window.
	baseFrom := from.Add(-dur)
	base, err := s.windowAggregate(ctx, target, probe, baseFrom, from)
	if err == nil && base.samples > 0 && base.sent > 0 {
		ws.BaselineAvailable = true
		ws.BaselineLossPct = 100.0 * float64(base.sent-base.rcvd) / float64(base.sent)
		ws.BaselineAvgRttMs = base.avgRtt
		ws.ChangeLossPct = ws.LossPct - ws.BaselineLossPct
		if ws.BaselineAvgRttMs > 0 {
			ws.ChangeRttPct = 100.0 * (ws.AvgRttMs - ws.BaselineAvgRttMs) / ws.BaselineAvgRttMs
		}
	}
	return ws, nil
}

// PingTargetBaselines aggregates a bounded target cohort in one ClickHouse
// query. The caller chooses a completed historical window that does not overlap
// the live graph window.
func (s *Store) PingTargetBaselines(ctx context.Context, targets []string, from, to time.Time) (map[string]TargetBaseline, error) {
	q, targetList, err := buildPingTargetBaselinesQuery(s.db, s.table, targets, from, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.QueryJSON(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make(map[string]TargetBaseline, len(targetList))
	for _, r := range rows {
		target, _ := r["target"].(string)
		if target == "" {
			continue
		}
		baseline := TargetBaseline{
			Target: target, Sent: toI(r["sent"]), Rcvd: toI(r["rcvd"]),
			Samples: toI(r["samples"]), Probes: toI(r["probes"]),
		}
		if baseline.Sent > 0 {
			baseline.LossPct = 100 * float64(baseline.Sent-baseline.Rcvd) / float64(baseline.Sent)
		}
		out[target] = baseline
	}
	return out, nil
}

func buildPingTargetBaselinesQuery(db, table string, targets []string, from, to time.Time) (string, []string, error) {
	if !from.Before(to) {
		return "", nil, fmt.Errorf("baseline start must be before end")
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
	if len(targetList) == 0 {
		return "", nil, fmt.Errorf("at least one target is required")
	}
	sort.Strings(targetList)
	targetSQL := make([]string, len(targetList))
	for i, target := range targetList {
		targetSQL[i] = EscStr(target)
	}

	q := fmt.Sprintf(`
SELECT dst_addr AS target,
       sum(toInt64(sent)) AS sent,
       sum(toInt64(rcvd)) AS rcvd,
       count() AS samples,
       uniqExact(prb_id) AS probes
FROM %s.%s
WHERE type = 'ping'
  AND timestamp >= toDateTime(%d)
  AND timestamp < toDateTime(%d)
  AND dst_addr IN (%s)
GROUP BY target`, db, table, from.Unix(), to.Unix(), strings.Join(targetSQL, ","))
	return q, targetList, nil
}

// windowAgg holds the raw aggregate values from one window's ClickHouse query.
type windowAgg struct {
	sent      int64
	rcvd      int64
	samples   int64
	avgRtt    float64
	medianRtt float64
}

// windowAggregate runs the loss/RTT aggregation query for a single window.
// RTT is pulled from the per-reply result array via JSONExtract; loss from
// sent/rcvd. Median uses ClickHouse's quantile(0.5).
func (s *Store) windowAggregate(ctx context.Context, target string, probe int64, from, to time.Time) (windowAgg, error) {
	q := fmt.Sprintf(`
SELECT
  sum(toInt64(sent)) AS sent,
  sum(toInt64(rcvd)) AS rcvd,
  count() AS samples,
  avgIf(avg_rtt_ms, rcvd > 0) AS avg_rtt,
  quantileIf(0.5)(avg_rtt_ms, rcvd > 0) AS med_rtt
FROM %s.%s
WHERE type = 'ping'
  AND timestamp >= toDateTime(%d)
  AND timestamp <  toDateTime(%d)`,
		s.db, s.table, from.Unix(), to.Unix())
	if probe > 0 {
		q += fmt.Sprintf(" AND prb_id = %d", probe)
	}
	if target != "" {
		q += " AND dst_addr = " + EscStr(target)
	}

	rows, err := s.QueryJSON(ctx, q)
	if err != nil {
		return windowAgg{}, err
	}
	if len(rows) == 0 {
		return windowAgg{}, nil
	}
	r := rows[0]
	return windowAgg{
		sent:      toI(r["sent"]),
		rcvd:      toI(r["rcvd"]),
		samples:   toI(r["samples"]),
		avgRtt:    toF(r["avg_rtt"]),
		medianRtt: toF(r["med_rtt"]),
	}, nil
}

// _ keeps strconv referenced (used by other files in this package).
var _ = strconv.Itoa
