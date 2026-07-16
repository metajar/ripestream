package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// (time is used by PingSeries via the timeseries.go file and the queryTimeout.)

// Reader is the read-only subset of *Store used by the API layer.
type Reader interface {
	// QueryJSON runs a SELECT and decodes ClickHouse's JSON output format into a
	// slice of row maps. Each row is column-name -> value.
	QueryJSON(ctx context.Context, query string) ([]map[string]any, error)
	// PingSeries returns bucketed ping-metric history for charts.
	PingSeries(ctx context.Context, metric string, probe int64, target string, from, to time.Time, buckets int) ([]SeriesPoint, error)
	// PingWindowSummary returns loss/RTT aggregates over a time window plus a
	// prior equal-duration baseline, scoped to a target/probe.
	PingWindowSummary(ctx context.Context, target string, probe int64, from, to time.Time) (WindowSummary, error)
}

// Compile-time: *Store implements Reader.
var _ Reader = (*Store)(nil)

// chRow is the shape ClickHouse emits for FORMAT JSON (one outer object with a
// "data" array of objects, plus metadata we ignore here).
type chJSONResult struct {
	Data []map[string]any `json:"data"`
}

// queryTimeout bounds read queries so a slow ClickHouse can't hold an API request.
const queryTimeout = 20 * time.Second

// QueryJSON executes a read query against ClickHouse over HTTP and returns the
// decoded rows. The caller must NOT include a FORMAT clause; this method appends
// "FORMAT JSON". Parameterization is the caller's responsibility (interpolate
// safely or use ClickHouse settings as needed).
func (s *Store) QueryJSON(ctx context.Context, query string) ([]map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	full := query + " FORMAT JSON"
	u := s.baseURL + "/?query=" + url.QueryEscape(full)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if s.user != "" {
		req.SetBasicAuth(s.user, s.pass)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clickhouse query: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("clickhouse http %d: %s", resp.StatusCode, body)
	}
	var out chJSONResult
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("clickhouse decode: %w", err)
	}
	return out.Data, nil
}

// EscStr returns s as a ClickHouse single-quoted string literal, doubling any
// embedded single quotes (the SQL standard escape). Used for the few places we
// interpolate values directly rather than via parameters.
func EscStr(s string) string {
	// Pre-size: 2 quotes + up to 2x for worst-case all-quotes.
	out := make([]byte, 0, len(s)*2+2)
	out = append(out, '\'')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' || c == '\\' {
			out = append(out, '\\')
		}
		out = append(out, c)
	}
	out = append(out, '\'')
	return string(out)
}
