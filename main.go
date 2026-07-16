// Command ripestream reads the RIPE Atlas live result stream and writes the
// results into ClickHouse.
//
// By default it subscribes to the full public firehose (all measurement types:
// ping, traceroute, dns, http, ...). Use --msm / --prb to subscribe to specific
// measurements or probes; each value opens its own connection. The full payload
// of every result is stored verbatim in the result_json column alongside a set
// of common typed columns, so the schema works for every measurement type.
package main

import (
	"context"
	_ "embed"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ripestream/internal/atlas"
	"ripestream/internal/store"
)

//go:embed schema.sql
var schemaSQL string

func main() {
	var (
		chURL         = flag.String("clickhouse", envDefault("RIPESTREAM_CLICKHOUSE", "http://localhost:8123"), "ClickHouse HTTP base URL")
		chDB          = flag.String("db", envDefault("RIPESTREAM_DB", "ripestream"), "ClickHouse database")
		chTable       = flag.String("table", envDefault("RIPESTREAM_TABLE", "atlas_results"), "ClickHouse table")
		chUser        = flag.String("user", envDefault("RIPESTREAM_USER", "default"), "ClickHouse user")
		chPass        = flag.String("password", os.Getenv("RIPESTREAM_PASSWORD"), "ClickHouse password")
		msmCSV        = flag.String("msm", envDefault("RIPESTREAM_MSM", ""), "comma-separated measurement IDs to subscribe to (empty = firehose)")
		prbCSV        = flag.String("prb", envDefault("RIPESTREAM_PRB", ""), "comma-separated probe IDs to subscribe to")
		streamURL     = flag.String("stream-url", atlas.DefaultBaseURL, "RIPE Atlas stream endpoint")
		batchSize     = flag.Int("batch-size", 1000, "max rows per ClickHouse insert")
		flushInterval = flag.Duration("flush-interval", 5*time.Second, "max time between flushes")
		idleTimeout   = flag.Duration("idle-timeout", 5*time.Minute, "reconnect a connection after this long with no data (0 disables)")
		applySchema   = flag.Bool("apply-schema", true, "apply schema.sql on startup if true")
		logLevel      = flag.String("log-level", envDefault("RIPESTREAM_LOG_LEVEL", "info"), "log level: debug|info|warn|error")
	)
	flag.Parse()

	setupLogger(*logLevel)

	params := atlas.Params{
		MSM: parseIntCSV(*msmCSV),
		PRB: parseIntCSV(*prbCSV),
	}
	opts := atlas.Options{
		BaseURL:     *streamURL,
		IdleTimeout: *idleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st := store.New(store.Config{
		BaseURL:  *chURL,
		Database: *chDB,
		Table:    *chTable,
		User:     *chUser,
		Password: *chPass,
	})

	if *applySchema {
		if err := st.ApplySchema(ctx, schemaSQL); err != nil {
			slog.Error("failed to apply schema", "err", err)
			os.Exit(1)
		}
	}

	slog.Info("starting ripestream",
		"sub", subSummary(params),
		"clickhouse", *chURL, "db", *chDB, "table", *chTable,
		"batch_size", *batchSize, "flush_interval", *flushInterval)

	records := atlas.Subscribe(ctx, params, opts)

	if err := st.Run(ctx, records, *batchSize, *flushInterval); err != nil &&
		err != context.Canceled {
		slog.Error("writer exited with error", "err", err)
		os.Exit(1)
	}
	slog.Info("ripestream stopped cleanly")
}

func setupLogger(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))
}

func subSummary(p atlas.Params) string {
	if len(p.MSM) == 0 && len(p.PRB) == 0 {
		return "firehose"
	}
	out := ""
	if len(p.MSM) > 0 {
		out += "msm"
	}
	if len(p.PRB) > 0 {
		if out != "" {
			out += "+"
		}
		out += "prb"
	}
	return out
}

func parseIntCSV(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			slog.Warn("ignoring non-numeric id", "value", part, "err", err)
			continue
		}
		if n <= 0 {
			slog.Warn("ignoring non-positive id", "value", n)
			continue
		}
		out = append(out, n)
	}
	return out
}

// envDefault returns the env var value when set, otherwise def.
func envDefault(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}
