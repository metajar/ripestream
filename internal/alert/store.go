package alert

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// Store persists alert rules, states, and events in embedded SQLite.
type Store struct {
	db *sql.DB
}

// Open opens (creating if absent) the SQLite file and ensures the schema.
func Open(path string) (*Store, error) {
	if path == "" {
		path = "ripestream.db"
	}
	// cache=shared + busy_timeout avoids "database is locked" under concurrent
	// evaluator/handler access; _txlock=immediate prevents writer starvation.
	dsn := fmt.Sprintf("file:%s?cache=shared&_busy_timeout=5000&_txlock=immediate", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite serializes writes; one conn avoids lock churn.
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS alert_rules (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL,
	metric     TEXT NOT NULL,
	comparison TEXT NOT NULL,
	threshold  REAL NOT NULL,
	scope      TEXT NOT NULL,
	filters    TEXT NOT NULL DEFAULT '{}',
	window_min INTEGER NOT NULL DEFAULT 0,
	enabled    INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS alert_states (
	rule_id      INTEGER NOT NULL,
	scope_key    TEXT NOT NULL,
	status       TEXT NOT NULL,           -- firing | ok
	current_value REAL NOT NULL,
	last_eval_at TEXT NOT NULL,
	fired_at     TEXT,
	context      TEXT NOT NULL DEFAULT '{}',
	PRIMARY KEY (rule_id, scope_key)
);

CREATE TABLE IF NOT EXISTS alert_events (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	rule_id   INTEGER NOT NULL,
	type      TEXT NOT NULL,              -- fired | resolved
	scope_key TEXT NOT NULL,
	value     REAL NOT NULL,
	context   TEXT NOT NULL DEFAULT '{}',
	ts        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_alert_events_ts ON alert_events(ts DESC);
CREATE INDEX IF NOT EXISTS idx_alert_events_rule ON alert_events(rule_id, ts DESC);
`

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339) }

// ---- Rule CRUD --------------------------------------------------------------

// ListRules returns all rules.
func (s *Store) ListRules(ctx context.Context) ([]Rule, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, metric, comparison, threshold, scope, filters, window_min, enabled, created_at, updated_at
FROM alert_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// GetRule returns one rule by id.
func (s *Store) GetRule(ctx context.Context, id int64) (Rule, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, metric, comparison, threshold, scope, filters, window_min, enabled, created_at, updated_at
FROM alert_rules WHERE id = ?`, id)
	r, err := scanRule(row)
	if err != nil {
		return Rule{}, err
	}
	return r, nil
}

// CreateRule persists a new rule.
func (s *Store) CreateRule(ctx context.Context, r Rule) (Rule, error) {
	if err := r.Validate(); err != nil {
		return Rule{}, err
	}
	now := nowISO()
	filtersJSON, _ := json.Marshal(r.Filters)
	res, err := s.db.ExecContext(ctx, `
INSERT INTO alert_rules (name, metric, comparison, threshold, scope, filters, window_min, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Name, r.Metric, r.Comparison, r.Threshold, r.Scope, string(filtersJSON),
		r.WindowMin, boolToInt(r.Enabled), now, now)
	if err != nil {
		return Rule{}, err
	}
	id, _ := res.LastInsertId()
	r.ID = id
	r.CreatedAt = now
	r.UpdatedAt = now
	return r, nil
}

// UpdateRule replaces all mutable fields of an existing rule.
func (s *Store) UpdateRule(ctx context.Context, id int64, r Rule) (Rule, error) {
	if err := r.Validate(); err != nil {
		return Rule{}, err
	}
	now := nowISO()
	filtersJSON, _ := json.Marshal(r.Filters)
	_, err := s.db.ExecContext(ctx, `
UPDATE alert_rules
SET name=?, metric=?, comparison=?, threshold=?, scope=?, filters=?, window_min=?, enabled=?, updated_at=?
WHERE id=?`,
		r.Name, r.Metric, r.Comparison, r.Threshold, r.Scope, string(filtersJSON),
		r.WindowMin, boolToInt(r.Enabled), now, id)
	if err != nil {
		return Rule{}, err
	}
	if r, err := s.GetRule(ctx, id); err == nil {
		return r, nil
	}
	r.ID = id
	r.UpdatedAt = now
	return r, nil
}

// DeleteRule removes a rule and its states/events.
func (s *Store) DeleteRule(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM alert_rules WHERE id = ?`,
		`DELETE FROM alert_states WHERE rule_id = ?`,
		`DELETE FROM alert_events WHERE rule_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ---- State ------------------------------------------------------------------

// ListStates returns the current evaluation state for every (rule, scope).
func (s *Store) ListStates(ctx context.Context) ([]State, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT st.rule_id, r.name, st.scope_key, st.status, st.current_value, st.last_eval_at, st.fired_at, st.context
FROM alert_states st JOIN alert_rules r ON r.id = st.rule_id
ORDER BY st.status DESC, st.last_eval_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []State
	for rows.Next() {
		var st State
		var ctxJSON string
		var firedAt sql.NullString
		if err := rows.Scan(&st.RuleID, &st.RuleName, &st.ScopeKey, &st.Status, &st.CurrentVal,
			&st.LastEvalAt, &firedAt, &ctxJSON); err != nil {
			return nil, err
		}
		st.FiredAt = firedAt.String
		_ = json.Unmarshal([]byte(ctxJSON), &st.Context)
		out = append(out, st)
	}
	return out, nil
}

// upsertState records the latest evaluation for a (rule, scope).
func (s *Store) upsertState(ctx context.Context, st State) error {
	ctxJSON, _ := json.Marshal(st.Context)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO alert_states (rule_id, scope_key, status, current_value, last_eval_at, fired_at, context)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(rule_id, scope_key) DO UPDATE SET
	status=excluded.status, current_value=excluded.current_value,
	last_eval_at=excluded.last_eval_at, fired_at=excluded.fired_at, context=excluded.context`,
		st.RuleID, st.ScopeKey, st.Status, st.CurrentVal, st.LastEvalAt, nullIfEmpty(st.FiredAt), string(ctxJSON))
	return err
}

// ---- Events -----------------------------------------------------------------

// ListEvents returns recent fire/resolve events (most recent first).
func (s *Store) ListEvents(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT e.id, e.rule_id, r.name, e.type, e.scope_key, e.value, e.context, e.ts
FROM alert_events e JOIN alert_rules r ON r.id = e.rule_id
ORDER BY e.ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var ctxJSON string
		if err := rows.Scan(&e.ID, &e.RuleID, &e.RuleName, &e.Type, &e.ScopeKey, &e.Value, &ctxJSON, &e.Ts); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(ctxJSON), &e.Context)
		out = append(out, e)
	}
	return out, nil
}

// addEvent inserts an event.
func (s *Store) addEvent(ctx context.Context, e Event) error {
	ctxJSON, _ := json.Marshal(e.Context)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO alert_events (rule_id, type, scope_key, value, context, ts)
VALUES (?, ?, ?, ?, ?, ?)`,
		e.RuleID, e.Type, e.ScopeKey, e.Value, string(ctxJSON), e.Ts)
	return err
}

// ListActiveEvents returns the firing scopes (the active-alert ticker).
func (s *Store) ListActiveEvents(ctx context.Context) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT st.rule_id, r.name, st.scope_key, st.current_value, st.fired_at
FROM alert_states st
JOIN alert_rules r ON r.id = st.rule_id
WHERE st.status = 'firing'
ORDER BY st.fired_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var firedAt sql.NullString
		if err := rows.Scan(&e.RuleID, &e.RuleName, &e.ScopeKey, &e.Value, &firedAt); err != nil {
			return nil, err
		}
		e.Ts = firedAt.String
		e.Type = "fired"
		out = append(out, e)
	}
	return out, nil
}

// ---- scan helpers -----------------------------------------------------------

// scanner abstracts *sql.Row and *sql.Rows for shared scan logic.
type scanner interface {
	Scan(dest ...any) error
}

func scanRule(s scanner) (Rule, error) {
	var r Rule
	var filtersJSON string
	var enabled int
	if err := s.Scan(&r.ID, &r.Name, &r.Metric, &r.Comparison, &r.Threshold, &r.Scope,
		&filtersJSON, &r.WindowMin, &enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return Rule{}, fmt.Errorf("rule not found")
		}
		return Rule{}, err
	}
	r.Enabled = enabled != 0
	_ = json.Unmarshal([]byte(filtersJSON), &r.Filters)
	return r, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
