---
name: add-alert-rule
description: Adding a new alert rule type or evaluation logic to the alerting engine. Use when extending alerting to detect new network conditions or adding rule evaluation patterns.
triggers:
  - "alert rule"
  - "alerting"
  - "rule type"
  - "evaluation"
edges:
  - target: context/alerts.md
    condition: when understanding alert architecture, rule model, and evaluation flow
  - target: context/graph.md
    condition: when alert rules need to query the FalkorDB graph
  - target: context/stack.md
    condition: when working with SQLite for alert state storage
  - target: context/conventions.md
    condition: when writing Go code for alert evaluation logic
  - target: patterns/add-graph-query.md
    condition: when the alert rule requires a new graph query
  - target: patterns/add-api-endpoint.md
    condition: when the alert rule needs custom API endpoints
last_updated: 2025-01-16
---

# Add Alert Rule Type

## Context
Alerts are defined in SQLite (`internal/alert/`), evaluated against live graph state via `graph.Reader`, and served via API. Rules define conditions, states track current evaluation, events record transitions.

## Steps

### 1. Define Rule Condition
Add rule type to `internal/alert/model.go` if new pattern needed:

```go
const (
    RuleTypePath      = "path"      // existing: AS appears in path
    RuleTypeHotHop   = "hothop"    // existing: RTT degradation
    RuleTypeMyFeature = "myfeature" // new: your rule type
)
```

### 2. Extend AlertState/Event if Needed
If new rule type needs different state tracking, extend model structs:

```go
type AlertState struct {
    // ... existing fields ...
    MyFeatureField string `json:"my_feature_field,omitempty"` // type-specific data
}
```

### 3. Implement Evaluation Logic
Add evaluation method in `internal/alert/evaluator.go`:

```go
func (ev *Evaluator) evaluateMyFeatureRule(ctx context.Context, rule Rule) ([]AlertState, []AlertEvent, error) {
    // Query graph for relevant state
    results, err := ev.graph.QueryForMyFeature(ctx, rule.Threshold)
    if err != nil {
        return nil, nil, fmt.Errorf("query myfeature: %w", err)
    }

    var states []AlertState
    var events []AlertEvent

    for _, result := range results {
        // Get previous state from SQLite
        prevState, err := ev.store.GetState(rule.ID, result.EntityID)
        if err != nil && !errors.Is(err, sql.ErrNoRows) {
            return nil, nil, err
        }

        // Determine if condition is met
        isActive := evaluateMyFeatureCondition(result, rule.Threshold)

        // Check for state transition
        if prevState == nil || prevState.IsActive != isActive {
            events = append(events, AlertEvent{
                RuleID:       rule.ID,
                EntityID:     result.EntityID,
                EntityType:   "asn", // or ip, probe
                PreviousState: prevState != nil && prevState.IsActive,
                NewState:     isActive,
                Value:        result.Value,
                Message:      formatMyFeatureMessage(result, isActive),
                Timestamp:    time.Now(),
            })
        }

        // Update or create state
        states = append(states, AlertState{
            RuleID:       rule.ID,
            EntityID:     result.EntityID,
            EntityType:   "asn",
            IsActive:     isActive,
            LastEvaluated: time.Now(),
            LastValue:    result.Value,
            Message:      formatMyFeatureMessage(result, isActive),
        })
    }

    return states, events, nil
}
```

### 4. Add Graph Query (if new)
Add query method to `graph.Reader` interface and implementation (see `add-graph-query.md` pattern):

```go
// QueryForMyFeature finds entities meeting the alert condition
func (s *Store) QueryForMyFeature(ctx context.Context, threshold float64) ([]MyFeatureAlertResult, error) {
    q := `
MATCH (a:AS)-[:TRANSITS]->(b:AS)
WHERE a.asn = $asn
RETURN a.asn, b.asn, COUNT(*) AS path_count
HAVING path_count > $threshold
`
    // ... implementation ...
}
```

### 5. Wire into Evaluator
Add case to evaluator tick function in `internal/alert/evaluator.go`:

```go
func (ev *Evaluator) tick(ctx context.Context) error {
    rules, err := ev.store.GetActiveRules()
    // ... existing rule types ...

    for _, rule := range rules {
        switch rule.Type {
        // ... existing cases ...
        case RuleTypeMyFeature:
            ruleStates, ruleEvents, err := ev.evaluateMyFeatureRule(ctx, rule)
            if err != nil {
                slog.Error("myfeature rule eval failed", "rule", rule.ID, "err", err)
                continue
            }
            states = append(states, ruleStates...)
            events = append(events, ruleEvents...)
        }
    }
    // ... persist states and events ...
}
```

### 6. Add API CRUD Support
Manager already handles generic CRUD. If rule type needs special handling, add to `internal/alert/manager.go`.

### 7. Test Rule Creation
Create rule via API:
```bash
curl -X POST http://localhost:8080/api/alerts/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Feature Alert",
    "description": "Detects my feature condition",
    "type": "myfeature",
    "enabled": true,
    "severity": "warning",
    "threshold": 100,
    "condition": {"asn": 12345}
  }'
```

## Gotchas
- **Rule persistence**: Rule definitions survive restarts in SQLite. Test with enabled=false first.
- **State transitions**: Only create events when state actually changes (fire → clear, clear → fire)
- **Graph query complexity**: Alert queries run every interval (default 1m). Keep queries efficient with proper indexes.
- **No dry-run mode**: New rules are live immediately when enabled=true
- **State deduplication**: Check previous state before creating new AlertState for same entity
- **Message formatting**: Include enough context in messages for UI display

## Verify
Before considering the alert rule type complete:
- [ ] Evaluation logic handles both fire and clear transitions
- [ ] Graph query uses indexes and has LIMIT
- [ ] Events only created on state changes (not every tick)
- [ ] Alert states persisted to SQLite correctly
- [ ] Manager CRUD operations work for new rule type
- [ ] UI displays new alert type correctly
- [ ] Test rule with enabled=false before enabling

## Debug
If alert evaluation fails:
- **No alerts firing**: Check rule is enabled=true and evaluator is running (logs show "alerting enabled")
- **Query errors**: Test graph query in FalkorDB Browser UI first
- **State not persisting**: Check SQLite database connectivity and table schema
- **No events**: Verify state transitions are actually happening (condition results changing)
- **Slow evaluation**: Check query performance in FalkorDB, add LIMIT if missing

## Update Scaffold
- [ ] Update `.mex/ROUTER.md` "Current Project State" if new alert type is working
- [ ] Update `.mex/context/alerts.md` if adding new rule patterns or evaluation logic
- [ ] If this task recurs without a pattern, create one in `.mex/patterns/`
