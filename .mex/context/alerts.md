---
name: alerts
description: Alerting engine, rule definitions, evaluation logic, and alert state management. Load when working with alerting features, rule CRUD operations, or alert event history.
triggers:
  - "alert"
  - "rule"
  - "notification"
  - "issue"
  - "evaluate"
edges:
  - target: context/graph.md
    condition: when alert rules need to evaluate against graph state
  - target: context/architecture.md
    condition: when understanding how the alerting engine fits into the system
  - target: context/setup.md
    condition: when configuring alerting dependencies (SQLite, evaluator interval)
  - target: context/stack.md
    condition: when working with SQLite for alert state storage
  - target: patterns/add-alert-rule.md
    condition: when implementing a new alert rule type
last_updated: 2026-07-16
---

# Alerting System

## Architecture
The alerting system has three components:
1. **SQLite Store** (`internal/alert/store.go`) — persists rule definitions, alert states, and event history
2. **Evaluator** (`internal/alert/evaluator.go`) — ticks on interval, evaluates rules against live graph state
3. **Manager** (`internal/alert/manager.go`) — implements API interface for CRUD operations

## Evaluation Flow
Every `alert-interval` (default 1 minute):
1. Evaluator queries graph.Reader for relevant state (e.g., paths between ASes, hot hops)
2. For each active rule, evaluate conditions against current state
3. Compare result with previous state (stored in SQLite)
4. If state changed: create AlertEvent, update AlertState, mark as "active" if condition met
5. Manager serves current states, active alerts, and event history to API

## Rule Model
Rules define conditions to evaluate against graph state:
- **id** — auto-increment primary key
- **name** — human-readable rule name
- **description** — what the rule detects
- **enabled** — whether the rule is actively evaluated
- **severity** — impact level (info, warning, critical)
- **condition** — graph query pattern to evaluate (Cypher-based or predefined pattern)
- **threshold** — value that triggers alert when exceeded
- **created_at**, **updated_at** — timestamps

## State Model
AlertState tracks the current evaluation result:
- **rule_id** — foreign key to rules
- **entity_id** — affected entity (ASN, IP, or probe ID)
- **entity_type** — "asn", "ip", or "probe"
- **is_active** — whether the condition is currently met
- **last_evaluated** — timestamp of last evaluation
- **last_value** — current metric value
- **message** — human-readable status message

## Event Model
AlertEvent records state transitions (fire → clear, clear → fire):
- **rule_id** — foreign key to rules
- **entity_id** — affected entity
- **entity_type** — "asn", "ip", or "probe"
- **previous_state** — was_active before transition
- **new_state** — is_active after transition
- **value** — metric value at time of transition
- **message** — explanation of the transition
- **timestamp** — when the transition occurred

## API Endpoints
- **GET /api/alerts/active** — list all currently firing alerts (AlertState where is_active=true)
- **GET /api/alerts/states** — all alert states (active + inactive)
- **GET /api/alerts/events** — event history with optional limit
- **GET /api/alerts/rules** — list all rules
- **GET /api/alerts/rules/{id}** — get single rule
- **POST /api/alerts/rules** — create new rule
- **PUT /api/alerts/rules/{id}** — update existing rule
- **DELETE /api/alerts/rules/{id}** — delete rule

## Graph Integration
Alerts evaluate against the live FalkorDB graph:
- **Transit alerts** — detect when AS A appears in paths between AS B and AS C
- **Hot hop alerts** — detect RTT degradation on specific IP-to-IP links
- **Probe alerts** — detect probe unavailability or packet loss
- **ASN alerts** — detect AS-level changes in topology

Evaluator uses `graph.Reader` interface to query state without coupling to FalkorDB directly.

All alert graph queries use the configured active-health window (30m default), packet-weight loss, and evidence floors for broad scopes: target/destination-AS alerts need at least three probes, destination-AS alerts also need two source ASes, and AS-pair alerts need two probes. Probe-target rules intentionally remain single-probe diagnostics.

## Constraints
- **SQLite only** — alert state is not replicated. Single-instance deployment assumption.
- **No external notifications** — alerts are stored in DB only. UI polling is the notification mechanism.
- **Evaluation interval** — default 1 minute. Lower intervals increase graph query load.
- **Rule persistence** — rules survive service restarts (SQLite). Alert states persist too.
- **Graceful degradation** — if graph is disabled, alerting is also disabled (API returns 503).

## Gotchas
- **Rule state survives restarts** — AlertState table records is_active across restarts. Don't assume clean state.
- **No deduplication** — if the same condition is already firing, no new event is generated. Only transitions create events.
- **Graph query complexity** — rules with complex Cypher queries can timeout. Keep queries focused.
- **No dry-run mode** — new rules are live immediately. Test with enabled=false first.
