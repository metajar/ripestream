package alert

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"ripestream/internal/graph"
)

// GraphReader is the narrow graph read surface the evaluator needs. *graph.Store
// implements graph.AlertEvaluator.
type GraphReader = graph.AlertEvaluator

// Evaluator ticks on an interval, evaluating every enabled rule against the
// graph and recording fire/resolve transitions.
type Evaluator struct {
	store    *Store
	graph    GraphReader
	interval time.Duration
}

// NewEvaluator builds an Evaluator. interval defaults to 1 minute if <= 0.
func NewEvaluator(s *Store, g GraphReader, interval time.Duration) *Evaluator {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Evaluator{store: s, graph: g, interval: interval}
}

// Run blocks until ctx is done, ticking the evaluation loop.
func (e *Evaluator) Run(ctx context.Context) {
	slog.Info("alert evaluator started", "interval", e.interval)
	t := time.NewTicker(e.interval)
	defer t.Stop()
	e.tick(ctx) // run once immediately so alerts don't wait a full interval on boot.
	for {
		select {
		case <-ctx.Done():
			slog.Info("alert evaluator stopped")
			return
		case <-t.C:
			e.tick(ctx)
		}
	}
}

// tick evaluates all enabled rules once.
func (e *Evaluator) tick(ctx context.Context) {
	rules, err := e.store.ListRules(ctx)
	if err != nil {
		slog.Warn("alert: list rules failed", "err", err)
		return
	}
	evaluated := 0
	for _, r := range rules {
		if ctx.Err() != nil {
			return
		}
		if !r.Enabled {
			continue
		}
		n, err := e.evaluateRule(ctx, r)
		if err != nil {
			slog.Warn("alert: evaluate rule failed", "rule", r.Name, "err", err)
			continue
		}
		evaluated += n
	}
	if evaluated > 0 {
		slog.Info("alert tick", "rules_evaluated", evaluated)
	}
}

// scopeRow is one evaluated scope instance with its metric value and a cause
// context snapshot for the UI.
type scopeRow struct {
	scopeKey string
	value    float64
	context  map[string]any
}

// evaluateRule runs the rule's Cypher, compares each scope instance against the
// threshold, and records state transitions (fire/resolve) + events.
func (e *Evaluator) evaluateRule(ctx context.Context, r Rule) (int, error) {
	spec := graph.AlertRuleSpec{
		Metric:     string(r.Metric),
		Scope:      string(r.Scope),
		Comparison: string(r.Comparison),
		Threshold:  r.Threshold,
		MinSent:    r.Filters.MinSent,
		MinLoss:    r.Filters.MinLoss,
		SrcASN:     r.Filters.SrcASN,
		DstASN:     r.Filters.DstASN,
		ProbeID:    r.Filters.ProbeID,
		TargetIP:   r.Filters.TargetIP,
	}
	graphRows, err := e.graph.EvaluateAlert(ctx, spec)
	if err != nil {
		return 0, fmt.Errorf("evaluate alert %q: %w", r.Name, err)
	}
	rows := make([]scopeRow, 0, len(graphRows))
	for _, gr := range graphRows {
		rows = append(rows, scopeRow{scopeKey: gr.ScopeKey, value: gr.Value, context: gr.Context})
	}

	now := nowISO()
	// Build the set of currently-firing scope keys.
	firing := make(map[string]scopeRow, len(rows))
	for _, row := range rows {
		if compare(r.Comparison, row.value, r.Threshold) {
			firing[row.scopeKey] = row
		}
	}

	// Load prior states to detect transitions.
	prior := make(map[string]string) // scopeKey -> "firing" | "ok"
	existing, _ := statesForRule(ctx, e.store, r.ID)
	for _, st := range existing {
		prior[st.ScopeKey] = st.Status
	}

	evaluated := 0
	// Fire transitions: firing now, not firing before.
	for key, row := range firing {
		evaluated++
		wasFiring := prior[key] == "firing"
		st := State{
			RuleID: r.ID, Status: "firing", CurrentVal: row.value,
			ScopeKey: key, LastEvalAt: now, Context: row.context,
		}
		if wasFiring {
			// Carry over original fired_at.
			for _, p := range existing {
				if p.ScopeKey == key {
					st.FiredAt = p.FiredAt
				}
			}
		} else {
			st.FiredAt = now
			_ = e.store.addEvent(ctx, Event{
				RuleID: r.ID, Type: "fired", ScopeKey: key,
				Value: row.value, Context: row.context, Ts: now,
			})
			slog.Warn("alert fired", "rule", r.Name, "scope", key,
				"value", row.value, "threshold", r.Threshold)
		}
		if err := e.store.upsertState(ctx, st); err != nil {
			slog.Warn("alert: upsert firing state failed", "err", err)
		}
	}

	// Resolve transitions: firing before, not firing now.
	for key, prevStatus := range prior {
		if prevStatus != "firing" {
			continue
		}
		if _, still := firing[key]; still {
			continue
		}
		evaluated++
		var val float64
		for _, row := range rows {
			if row.scopeKey == key {
				val = row.value
				break
			}
		}
		_ = e.store.addEvent(ctx, Event{
			RuleID: r.ID, Type: "resolved", ScopeKey: key,
			Value: val, Ts: now,
		})
		_ = e.store.upsertState(ctx, State{
			RuleID: r.ID, Status: "ok", CurrentVal: val,
			ScopeKey: key, LastEvalAt: now,
		})
		slog.Info("alert resolved", "rule", r.Name, "scope", key, "value", val)
	}
	return evaluated, nil
}

// statesForRule is a thin helper over ListStates filtering by rule id.
func statesForRule(ctx context.Context, s *Store, ruleID int64) ([]State, error) {
	all, err := s.ListStates(ctx)
	if err != nil {
		return nil, err
	}
	var out []State
	for _, st := range all {
		if st.RuleID == ruleID {
			out = append(out, st)
		}
	}
	return out, nil
}

// compare applies the threshold operator.
func compare(cmp Comparison, value, threshold float64) bool {
	switch cmp {
	case CmpGT:
		return value > threshold
	case CmpGTE:
		return value >= threshold
	case CmpLT:
		return value < threshold
	case CmpLTE:
		return value <= threshold
	case CmpEQ:
		return value == threshold
	}
	return false
}
