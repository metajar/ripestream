package api

// AlertRuleView is the API representation of an alert rule. Shared with the
// internal/alert package so the HTTP layer doesn't import alert directly.
type AlertRuleView struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Metric      string            `json:"metric"`       // loss_ratio | avg_rtt_ms | target_lost
	Comparison  string            `json:"comparison"`   // > | >= | < | <= | ==
	Threshold   float64           `json:"threshold"`
	Scope       string            `json:"scope"`        // probe_target | asn_pair | target | asn_dst
	Filters     map[string]any    `json:"filters,omitempty"`
	WindowMin   int               `json:"window_min"`   // persistence window (0 = current-value only)
	Enabled     bool              `json:"enabled"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

// AlertStateView is the current evaluation state of a rule (firing or not).
type AlertStateView struct {
	RuleID      int64           `json:"rule_id"`
	RuleName    string          `json:"rule_name"`
	Status      string          `json:"status"` // firing | ok
	CurrentVal  float64         `json:"current_value"`
	ScopeKey    string          `json:"scope_key"`
	LastEvalAt  string          `json:"last_eval_at"`
	FiredAt     string          `json:"fired_at,omitempty"`
	Context     map[string]any  `json:"context,omitempty"`
}

// AlertEventView is one fire/resolve transition in the alert history.
type AlertEventView struct {
	ID        int64          `json:"id"`
	RuleID    int64          `json:"rule_id"`
	RuleName  string         `json:"rule_name"`
	Type      string         `json:"type"` // fired | resolved
	ScopeKey  string         `json:"scope_key"`
	Value     float64        `json:"value"`
	Context   map[string]any `json:"context,omitempty"`
	Ts        string         `json:"ts"`
}

// AlertView is the active-alert summary shown on the overview ticker.
type AlertView struct {
	RuleID     int64  `json:"rule_id"`
	RuleName   string `json:"rule_name"`
	ScopeKey   string `json:"scope_key"`
	Value      float64 `json:"value"`
	FiredAt    string `json:"fired_at"`
}
