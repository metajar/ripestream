// Package store writes atlas Records into ClickHouse over its HTTP interface.
package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ripestream/internal/atlas"
)

// Store is a ClickHouse HTTP client that batches and inserts Records.
type Store struct {
	baseURL string
	db      string
	table   string
	user    string
	pass    string
	client  *http.Client
}

// Config configures the Store.
type Config struct {
	BaseURL  string // e.g. http://localhost:8123
	Database string // e.g. ripestream
	Table    string // e.g. atlas_results
	User     string
	Password string
}

// New returns a Store.
func New(cfg Config) *Store {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8123"
	}
	if cfg.Database == "" {
		cfg.Database = "ripestream"
	}
	if cfg.Table == "" {
		cfg.Table = "atlas_results"
	}
	return &Store{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		db:      cfg.Database,
		table:   cfg.Table,
		user:    cfg.User,
		pass:    cfg.Password,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// ApplySchema runs the given DDL (idempotent) split on ';'. The schema is
// supplied by the caller so the canonical schema.sql can live at the repo root.
func (s *Store) ApplySchema(ctx context.Context, schemaSQL string) error {
	for _, stmt := range splitStatements(schemaSQL) {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if err := s.exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply schema statement: %w", err)
		}
	}
	slog.Info("clickhouse schema ensured", "db", s.db, "table", s.table)
	return nil
}

// splitStatements breaks the schema into individual statements on ';'.
// The bundled schema.sql intentionally contains no ';' inside comments/strings.
func splitStatements(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ";") {
		if strings.TrimSpace(part) != "" {
			out = append(out, part)
		}
	}
	return out
}

// Run drains `in`, accumulating Records into batches and flushing them to
// ClickHouse when batchSize is reached or every flushInterval. It returns when
// `in` is closed; on context cancellation it drains anything left and performs
// a final flush before returning.
func (s *Store) Run(ctx context.Context, in <-chan atlas.Record, batchSize int, flushInterval time.Duration) error {
	if batchSize <= 0 {
		batchSize = 1000
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	batch := make([]atlas.Record, 0, batchSize)
	var inserted uint64
	var flushes uint64

	flush := func(fctx context.Context) {
		if len(batch) == 0 {
			return
		}
		start := time.Now()
		rows := batch
		batch = make([]atlas.Record, 0, batchSize)
		if err := s.insertWithRetry(fctx, rows); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				slog.Warn("clickhouse insert skipped (context done)", "rows", len(rows), "err", err)
			} else {
				slog.Error("clickhouse insert failed", "rows", len(rows), "err", err)
			}
			return
		}
		flushes++
		inserted += uint64(len(rows))
		slog.Info("clickhouse insert ok",
			"rows", len(rows), "took", time.Since(start).Round(time.Millisecond),
			"total_rows", inserted, "flushes", flushes)
	}

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Drain anything already buffered without blocking, then flush once
			// against a fresh context so the shutdown batch is not lost.
		drain:
			for {
				select {
				case r, ok := <-in:
					if !ok {
						break drain
					}
					batch = append(batch, r)
				default:
					break drain
				}
			}
			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownFlushTimeout)
			defer cancel()
			flush(shutdownCtx)
			slog.Info("clickhouse writer stopped",
				"total_rows", inserted, "flushes", flushes)
			return nil
		case <-ticker.C:
			flush(ctx)
		case r, ok := <-in:
			if !ok {
				flush(ctx)
				slog.Info("clickhouse writer stopped",
					"total_rows", inserted, "flushes", flushes)
				return nil
			}
			batch = append(batch, r)
			if len(batch) >= batchSize {
				flush(ctx)
			}
		}
	}
}

// shutdownFlushTimeout bounds the final flush on graceful shutdown, which runs
// against a fresh context after the ingest context has been cancelled.
const shutdownFlushTimeout = 30 * time.Second

// insertWithRetry encodes rows as JSONEachRow and POSTs an INSERT, retrying on
// transient failures with capped exponential backoff.
func (s *Store) insertWithRetry(ctx context.Context, rows []atlas.Record) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for i := range rows {
		if err := enc.Encode(&rows[i]); err != nil {
			return fmt.Errorf("encode row %d: %w", i, err)
		}
	}

	query := fmt.Sprintf("INSERT INTO %s.%s FORMAT JSONEachRow", s.db, s.table)
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.post(ctx, query, bytes.NewReader(buf.Bytes()))
		if err == nil {
			return nil
		}
		if !isRetryable(err) {
			return err
		}
		slog.Warn("clickhouse insert retrying",
			"attempt", attempt, "backoff", backoff, "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
	}
}

// post runs a single ClickHouse HTTP request with the query in the URL and the
// body as the data payload.
func (s *Store) post(ctx context.Context, query string, body io.Reader) error {
	u := s.baseURL + "/?query=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if s.user != "" {
		req.SetBasicAuth(s.user, s.pass)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		return nil
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return &clickhouseError{status: resp.StatusCode, body: string(respBody)}
}

// exec runs a statement with no row data.
func (s *Store) exec(ctx context.Context, query string) error {
	return s.post(ctx, query, nil)
}

type clickhouseError struct {
	status int
	body   string
}

func (e *clickhouseError) Error() string {
	return fmt.Sprintf("clickhouse http %d: %s", e.status, e.body)
}

// isRetryable reports whether an error is worth retrying (5xx, transport).
func isRetryable(err error) bool {
	var ce *clickhouseError
	if errors.As(err, &ce) {
		return ce.status >= 500
	}
	return true // network/transport errors are retryable
}
