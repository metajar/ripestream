package alert

import (
	"context"

	"ripestream/internal/api"
)

// Manager wires the SQLite store + evaluator and adapts them to the api.Alerter
// interface. It's the single object main.go constructs and hands to the API.
type Manager struct {
	store *Store
}

// NewManager wraps a store. The evaluator runs separately (its own goroutine).
func NewManager(s *Store) *Manager {
	return &Manager{store: s}
}

// ---- api.Alerter implementation --------------------------------------------

func (m *Manager) ActiveAlerts(ctx context.Context) ([]api.AlertView, error) {
	events, err := m.store.ListActiveEvents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.AlertView, 0, len(events))
	for _, e := range events {
		out = append(out, api.AlertView{
			RuleID: e.RuleID, RuleName: e.RuleName, ScopeKey: e.ScopeKey,
			Value: e.Value, FiredAt: e.Ts,
		})
	}
	return out, nil
}

func (m *Manager) AlertStates(ctx context.Context) ([]api.AlertStateView, error) {
	states, err := m.store.ListStates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.AlertStateView, 0, len(states))
	for _, st := range states {
		out = append(out, api.AlertStateView{
			RuleID: st.RuleID, RuleName: st.RuleName, Status: st.Status,
			CurrentVal: st.CurrentVal, ScopeKey: st.ScopeKey,
			LastEvalAt: st.LastEvalAt, FiredAt: st.FiredAt, Context: st.Context,
		})
	}
	return out, nil
}

func (m *Manager) AlertEvents(ctx context.Context, limit int) ([]api.AlertEventView, error) {
	events, err := m.store.ListEvents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]api.AlertEventView, 0, len(events))
	for _, e := range events {
		out = append(out, api.AlertEventView{
			ID: e.ID, RuleID: e.RuleID, RuleName: e.RuleName, Type: e.Type,
			ScopeKey: e.ScopeKey, Value: e.Value, Context: e.Context, Ts: e.Ts,
		})
	}
	return out, nil
}

func (m *Manager) Rules(ctx context.Context) ([]api.AlertRuleView, error) {
	rules, err := m.store.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.AlertRuleView, 0, len(rules))
	for _, r := range rules {
		out = append(out, ruleToView(r))
	}
	return out, nil
}

func (m *Manager) GetRule(ctx context.Context, id int64) (api.AlertRuleView, error) {
	r, err := m.store.GetRule(ctx, id)
	if err != nil {
		return api.AlertRuleView{}, err
	}
	return ruleToView(r), nil
}

func (m *Manager) CreateRule(ctx context.Context, v api.AlertRuleView) (api.AlertRuleView, error) {
	r, err := m.store.CreateRule(ctx, viewToRule(0, v))
	if err != nil {
		return api.AlertRuleView{}, err
	}
	return ruleToView(r), nil
}

func (m *Manager) UpdateRule(ctx context.Context, id int64, v api.AlertRuleView) (api.AlertRuleView, error) {
	r, err := m.store.UpdateRule(ctx, id, viewToRule(id, v))
	if err != nil {
		return api.AlertRuleView{}, err
	}
	return ruleToView(r), nil
}

func (m *Manager) DeleteRule(ctx context.Context, id int64) error {
	return m.store.DeleteRule(ctx, id)
}

// ---- view <-> model conversion ---------------------------------------------

func ruleToView(r Rule) api.AlertRuleView {
	filters := map[string]any{}
	if r.Filters.MinSent != 0 {
		filters["min_sent"] = r.Filters.MinSent
	}
	if r.Filters.MinLoss != 0 {
		filters["min_loss"] = r.Filters.MinLoss
	}
	if r.Filters.SrcASN != 0 {
		filters["src_asn"] = r.Filters.SrcASN
	}
	if r.Filters.DstASN != 0 {
		filters["dst_asn"] = r.Filters.DstASN
	}
	if r.Filters.ProbeID != 0 {
		filters["probe_id"] = r.Filters.ProbeID
	}
	if r.Filters.TargetIP != "" {
		filters["target_ip"] = r.Filters.TargetIP
	}
	return api.AlertRuleView{
		ID: r.ID, Name: r.Name, Metric: string(r.Metric),
		Comparison: string(r.Comparison), Threshold: r.Threshold,
		Scope: string(r.Scope), Filters: filters, WindowMin: r.WindowMin,
		Enabled: r.Enabled, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func viewToRule(id int64, v api.AlertRuleView) Rule {
	f := Filters{}
	if v.Filters != nil {
		if x, ok := v.Filters["min_sent"].(float64); ok {
			f.MinSent = int(x)
		}
		if x, ok := v.Filters["min_loss"].(float64); ok {
			f.MinLoss = x
		}
		if x, ok := v.Filters["src_asn"].(float64); ok {
			f.SrcASN = int64(x)
		}
		if x, ok := v.Filters["dst_asn"].(float64); ok {
			f.DstASN = int64(x)
		}
		if x, ok := v.Filters["probe_id"].(float64); ok {
			f.ProbeID = int64(x)
		}
		if x, ok := v.Filters["target_ip"].(string); ok {
			f.TargetIP = x
		}
	}
	return Rule{
		ID: id, Name: v.Name, Metric: Metric(v.Metric),
		Comparison: Comparison(v.Comparison), Threshold: v.Threshold,
		Scope: Scope(v.Scope), Filters: f, WindowMin: v.WindowMin,
		Enabled: v.Enabled,
	}
}

// Compile-time assertion that Manager satisfies api.Alerter.
var _ api.Alerter = (*Manager)(nil)
