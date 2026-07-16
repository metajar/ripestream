// Package api exposes the ripestream observability API over HTTP. It reads live
// topology from FalkorDB and historical metrics from ClickHouse, serving JSON to
// the embedded web UI (and any other client).
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"ripestream/internal/graph"
	"ripestream/internal/store"
)

// Alerter is the alerting subsystem the API calls into for the alerts endpoints.
// Implemented by internal/alert.Manager; defined here to avoid an import cycle.
type Alerter interface {
	ActiveAlerts(ctx context.Context) ([]AlertView, error)
	AlertStates(ctx context.Context) ([]AlertStateView, error)
	AlertEvents(ctx context.Context, limit int) ([]AlertEventView, error)
	Rules(ctx context.Context) ([]AlertRuleView, error)
	GetRule(ctx context.Context, id int64) (AlertRuleView, error)
	CreateRule(ctx context.Context, r AlertRuleView) (AlertRuleView, error)
	UpdateRule(ctx context.Context, id int64, r AlertRuleView) (AlertRuleView, error)
	DeleteRule(ctx context.Context, id int64) error
}

// Server holds the read-side dependencies wired by main and serves /api/*.
type Server struct {
	graph   graph.Reader
	ch      store.Reader
	alerter Alerter
	mux     *http.ServeMux

	// overviewCache serves the /api/overview result from memory; a background
	// goroutine recomputes it on overviewTTL so the page load no longer depends
	// on (growing) full-graph aggregation latency.
	overviewCache *refreshCache[graph.Overview]
	overviewTTL   time.Duration
}

// New builds a Server with the given dependencies. alerter may be nil during
// Phase 1 (the alerts routes are registered but return 503 until wired).
// overviewTTL is the refresh interval for the cached overview; if the graph
// reader is nil the cache is skipped and /api/overview returns 503.
func New(g graph.Reader, ch store.Reader, alerter Alerter, overviewTTL time.Duration) *Server {
	if overviewTTL <= 0 {
		overviewTTL = 60 * time.Second
	}
	s := &Server{graph: g, ch: ch, alerter: alerter, overviewTTL: overviewTTL}
	s.mux = http.NewServeMux()
	s.register()
	// Only cache when there's a graph to read from.
	if g != nil {
		s.overviewCache = newRefreshCache(g.Overview, overviewTTL)
	}
	return s
}

// StartCache launches the background overview refresher against ctx. Safe to
// call when no cache exists (graph disabled); it's a no-op then.
func (s *Server) StartCache(ctx context.Context) {
	if s.overviewCache != nil {
		s.overviewCache.start(ctx)
	}
}

// StopCache halts the background refresher.
func (s *Server) StopCache() {
	if s.overviewCache != nil {
		s.overviewCache.stop()
	}
}

// Handler returns the configured mux, wrapped with logging/recovery/CORS.
func (s *Server) Handler() http.Handler {
	return recoverPanic(cors(s.mux))
}

// ListenAndServe starts the HTTP server, blocking until ctx is done.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("api server listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// register wires every route. Method+path patterns use Go 1.25 ServeMux syntax.
func (s *Server) register() {
	m := s.mux

	// Health / meta
	m.HandleFunc("GET /api/health", s.health)
	m.HandleFunc("GET /api/schema", s.schema)

	// Overview (UC1)
	m.HandleFunc("GET /api/overview", s.overview)
	m.HandleFunc("GET /api/issues", s.issues)
	m.HandleFunc("GET /api/asn/issues", s.asnIssues)

	// Drill-down (UC2)
	m.HandleFunc("GET /api/asn/{asn}", s.asnDetail)
	m.HandleFunc("GET /api/asn/{asn}/probes", s.asnProbes)
	m.HandleFunc("GET /api/asn/{asn}/targets", s.asnTargets)
	m.HandleFunc("GET /api/asn/{asn}/transit", s.asnTransit)
	m.HandleFunc("GET /api/probes", s.probes)
	m.HandleFunc("GET /api/probe/{id}", s.probeDetail)
	m.HandleFunc("GET /api/targets", s.targets)
	m.HandleFunc("GET /api/target/{addr}", s.targetDetail)
	m.HandleFunc("GET /api/ip/{addr}", s.ipDetail)

	// Hops / Transit (UC3)
	m.HandleFunc("GET /api/hops/commonality", s.hopCommonalities)
	m.HandleFunc("GET /api/hops/hotspots", s.hotHops)
	m.HandleFunc("GET /api/transit", s.transitEdges)
	m.HandleFunc("GET /api/transit/{asnA}/{asnB}", s.transitPairDetail)
	m.HandleFunc("GET /api/transit/{asnA}/{asnB}/series", s.transitPairSeries)

	// Compete-with-TE views
	m.HandleFunc("GET /api/path", s.path)
	m.HandleFunc("GET /api/path/destinations", s.pathDestinations)
	m.HandleFunc("GET /api/graph/subgraph", s.subgraph)
	m.HandleFunc("GET /api/timeseries", s.timeseries)
	m.HandleFunc("GET /api/window", s.windowSummary)
	m.HandleFunc("GET /api/search", s.search)

	// Alerts (UC4) — registered always; handlers 503 when alerter is nil.
	m.HandleFunc("GET /api/alerts/active", s.alertsActive)
	m.HandleFunc("GET /api/alerts/states", s.alertsStates)
	m.HandleFunc("GET /api/alerts/events", s.alertsEvents)
	m.HandleFunc("GET /api/alerts/rules", s.alertsRules)
	m.HandleFunc("POST /api/alerts/rules", s.alertCreateRule)
	m.HandleFunc("GET /api/alerts/rules/{id}", s.alertGetRule)
	m.HandleFunc("PUT /api/alerts/rules/{id}", s.alertUpdateRule)
	m.HandleFunc("DELETE /api/alerts/rules/{id}", s.alertDeleteRule)
}
