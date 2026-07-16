// Package alert implements threshold alerting over the ripestream graph. Rules
// are persisted in embedded SQLite; a background evaluator ticks every minute,
// queries the graph for each rule's metric grouped by scope, and emits fire/
// resolve events with a cause-context snapshot that deep-links into the UI.
package alert

import (
	"fmt"
	"strings"
)

// Metric enumerates the signals a rule can threshold on. They map to PING-edge
// properties in FalkorDB.
type Metric string

const (
	MetricLossRatio Metric = "loss_ratio"   // 0..1 packet loss
	MetricAvgRTT    Metric = "avg_rtt_ms"   // average round-trip time, ms
	MetricTargetLost Metric = "target_lost" // rcvd == 0 (full loss)
)

// Scope is the grouping dimension the metric is evaluated over.
type Scope string

const (
	ScopeProbeTarget Scope = "probe_target" // each (probe, target IP) pair
	ScopeASNPair     Scope = "asn_pair"     // each (src AS, dst AS) pair
	ScopeTarget      Scope = "target"       // each destination IP
	ScopeASNDst      Scope = "asn_dst"      // each destination AS
)

// Comparison is the threshold operator.
type Comparison string

const (
	CmpGT  Comparison = ">"
	CmpGTE Comparison = ">="
	CmpLT  Comparison = "<"
	CmpLTE Comparison = "<="
	CmpEQ  Comparison = "=="
)

// Rule is a persisted alert rule.
type Rule struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Metric     Metric   `json:"metric"`
	Comparison Comparison `json:"comparison"`
	Threshold  float64  `json:"threshold"`
	Scope      Scope    `json:"scope"`
	Filters    Filters  `json:"filters"`
	WindowMin  int      `json:"window_min"` // persistence window (0 = current value)
	Enabled    bool     `json:"enabled"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

// Filters narrow the rule's evaluation. Zero values mean "no constraint".
type Filters struct {
	MinSent   int     `json:"min_sent,omitempty"`   // ignore edges with sent < this
	MinLoss   float64 `json:"min_loss,omitempty"`   // pre-filter by minimum loss (0..1)
	SrcASN    int64   `json:"src_asn,omitempty"`    // restrict source AS
	DstASN    int64   `json:"dst_asn,omitempty"`    // restrict destination AS
	ProbeID   int64   `json:"probe_id,omitempty"`   // restrict probe
	TargetIP  string  `json:"target_ip,omitempty"`  // restrict destination IP
}

// State is the current evaluation state of a rule (firing or healthy).
type State struct {
	RuleID     int64          `json:"rule_id"`
	RuleName   string         `json:"rule_name"`
	Status     string         `json:"status"` // firing | ok
	CurrentVal float64        `json:"current_value"`
	ScopeKey   string         `json:"scope_key"`
	LastEvalAt string         `json:"last_eval_at"`
	FiredAt    string         `json:"fired_at,omitempty"`
	Context    map[string]any `json:"context,omitempty"`
}

// Event is one fire/resolve transition.
type Event struct {
	ID       int64          `json:"id"`
	RuleID   int64          `json:"rule_id"`
	RuleName string         `json:"rule_name"`
	Type     string         `json:"type"` // fired | resolved
	ScopeKey string         `json:"scope_key"`
	Value    float64        `json:"value"`
	Context  map[string]any `json:"context,omitempty"`
	Ts       string         `json:"ts"`
}

// Validate checks a rule for internal consistency before persisting.
//
// UNIT CONVENTION (public API + UI):
//   - loss_ratio thresholds are PERCENTAGES (0–100). "50" means 50% loss.
//     The graph stores loss_ratio as a 0..1 fraction; the conversion to a
//     percentage happens at the graph boundary in EvaluateAlert
//     (round(100.0 * avg_loss)). The evaluator and UI work in percentages.
//   - avg_rtt_ms thresholds are milliseconds (>= 0).
//   - target_lost has no meaningful threshold value; the condition is rcvd==0
//     (100% loss). A threshold of 100 with comparison >= is conventional.
func (r *Rule) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	switch r.Metric {
	case MetricLossRatio, MetricAvgRTT, MetricTargetLost:
	default:
		return fmt.Errorf("invalid metric %q", r.Metric)
	}
	switch r.Scope {
	case ScopeProbeTarget, ScopeASNPair, ScopeTarget, ScopeASNDst:
	default:
		return fmt.Errorf("invalid scope %q", r.Scope)
	}
	switch r.Comparison {
	case CmpGT, CmpGTE, CmpLT, CmpLTE, CmpEQ:
	default:
		return fmt.Errorf("invalid comparison %q", r.Comparison)
	}
	// Threshold range validation per metric (public percentage/ms convention).
	switch r.Metric {
	case MetricLossRatio, MetricTargetLost:
		if r.Threshold < 0 || r.Threshold > 100 {
			return fmt.Errorf("loss threshold must be a percentage 0–100, got %g", r.Threshold)
		}
	case MetricAvgRTT:
		if r.Threshold < 0 {
			return fmt.Errorf("rtt threshold must be >= 0 ms, got %g", r.Threshold)
		}
	}
	return nil
}
