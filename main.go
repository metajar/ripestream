// Command ripestream reads the RIPE Atlas live result stream and writes the
// results into ClickHouse and FalkorDB.
//
// By default it subscribes to the full public firehose (all measurement types:
// ping, traceroute, dns, http, ...). Use --msm / --prb to subscribe to specific
// measurements or probes; each value opens its own connection. Every result is
// stored verbatim in ClickHouse. Traceroute and ping also update a FalkorDB
// topology graph (IP hops, probe framing, ping health).
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

	"golang.org/x/sync/errgroup"

	"ripestream/internal/asn"
	"ripestream/internal/atlas"
	"ripestream/internal/graph"
	"ripestream/internal/pipeline"
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
		falkorAddr    = flag.String("falkor-addr", envDefault("RIPESTREAM_FALKOR_ADDR", "localhost:6379"), "FalkorDB address host:port")
		falkorGraph   = flag.String("falkor-graph", envDefault("RIPESTREAM_FALKOR_GRAPH", "ripestream"), "FalkorDB graph name")
		falkorPass    = flag.String("falkor-password", os.Getenv("RIPESTREAM_FALKOR_PASSWORD"), "FalkorDB password")
		falkorEnabled = flag.Bool("falkor-enabled", envBoolDefault("RIPESTREAM_FALKOR_ENABLED", true), "write traceroute/ping topology to FalkorDB")
		asnDBPath     = flag.String("asn-db", envDefault("RIPESTREAM_ASN_DB", asn.DefaultDBPath), "GeoLite2-ASN .mmdb path (empty disables ASN enrichment)")
		msmCSV        = flag.String("msm", envDefault("RIPESTREAM_MSM", ""), "comma-separated measurement IDs to subscribe to (empty = firehose)")
		prbCSV        = flag.String("prb", envDefault("RIPESTREAM_PRB", ""), "comma-separated probe IDs to subscribe to")
		streamURL     = flag.String("stream-url", atlas.DefaultBaseURL, "RIPE Atlas stream endpoint")
		batchSize     = flag.Int("batch-size", 1000, "max rows/ops per sink flush")
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

	var gs *graph.Store
	var asnDB *asn.DB
	if *falkorEnabled {
		var lookup graph.LookupASN
		if strings.TrimSpace(*asnDBPath) != "" {
			db, err := asn.Open(*asnDBPath)
			if err != nil {
				slog.Warn("asn enrichment disabled (failed to open db)", "path", *asnDBPath, "err", err)
			} else {
				asnDB = db
				lookup = db
				slog.Info("asn enrichment enabled", "path", *asnDBPath)
			}
		}
		if asnDB != nil {
			defer asnDB.Close()
		}

		var err error
		gs, err = graph.New(graph.Config{
			Addr:     *falkorAddr,
			Password: *falkorPass,
			Graph:    *falkorGraph,
			ASN:      lookup,
		})
		if err != nil {
			slog.Error("failed to connect to falkordb", "err", err)
			os.Exit(1)
		}
		if err := gs.Ping(ctx); err != nil {
			slog.Error("falkordb ping failed", "err", err)
			os.Exit(1)
		}
		if err := gs.EnsureIndexes(ctx); err != nil {
			slog.Error("failed to ensure falkordb indexes", "err", err)
			os.Exit(1)
		}
	}

	slog.Info("starting ripestream",
		"sub", subSummary(params),
		"clickhouse", *chURL, "db", *chDB, "table", *chTable,
		"falkor_enabled", *falkorEnabled, "falkor_addr", *falkorAddr, "falkor_graph", *falkorGraph,
		"asn_db", *asnDBPath,
		"batch_size", *batchSize, "flush_interval", *flushInterval)

	records := atlas.Subscribe(ctx, params, opts)

	chIn := make(chan atlas.Record, 8192)
	outs := []chan<- atlas.Record{chIn}
	var graphIn chan atlas.Record
	if gs != nil {
		graphIn = make(chan atlas.Record, 8192)
		outs = append(outs, graphIn)
	}
	pipeline.Tee(ctx, records, outs...)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return st.Run(gctx, chIn, *batchSize, *flushInterval)
	})
	if gs != nil {
		g.Go(func() error {
			return gs.Run(gctx, graphIn, *batchSize, *flushInterval)
		})
	}

	if err := g.Wait(); err != nil && err != context.Canceled {
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

func envBoolDefault(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
