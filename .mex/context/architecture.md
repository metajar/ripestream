---
name: architecture
description: How the major pieces of this project connect and flow. Load when working on system design, integrations, or understanding how components interact.
triggers:
  - "architecture"
  - "system design"
  - "how does X connect to Y"
  - "integration"
  - "flow"
edges:
  - target: context/stack.md
    condition: when specific technology details are needed
  - target: context/decisions.md
    condition: when understanding why the architecture is structured this way
  - target: context/graph.md
    condition: when working with the FalkorDB graph writer or topology model
  - target: context/alerts.md
    condition: when understanding how the alerting engine integrates with the ingestion pipeline
last_updated: 2026-07-16
---

# Architecture

## System Overview
RIPE Atlas firehose stream → NDJSON lines → atlas.Subscribe (one goroutine per subscription filter) → pipeline.Tee (fans out to multiple sinks) → concurrent writers:
- store.Run → batches and inserts into ClickHouse (all measurement types)
- graph.Run → parses traceroute/ping, enriches with ASN data, batch MERGEs into FalkorDB topology graph

Parallel errgroup runs HTTP API server with embedded React UI, alerting evaluator (SQLite-backed, graph-aware), and overview cache refresh.

## Key Components
- **internal/atlas** — RIPE Atlas stream client with auto-reconnect, exponential backoff, idle timeout watchdog. One connection per subscription filter (msm/prb IDs or firehose). Emits parsed Records on channel.
- **internal/pipeline** — Tee fans each record to multiple sink channels without blocking the firehose. Drops records only when a sink channel is full (backpressure).
- **internal/store** — ClickHouse HTTP client with batching, JSONEachRow inserts, retry logic, and graceful shutdown. Applies schema.sql on startup. Projects PING packet counters and RTT summaries into typed columns so historical health queries do not repeatedly parse the raw JSON firehose under production memory pressure.
- **internal/graph** — FalkorDB graph writer for traceroute paths and ping health. Enriches IPs with ASN from GeoLite2 database, keeps a single current probe location, batch MERGEs nodes/edges, and prunes stale live-state data on a bounded schedule. Separate Reader interface for API queries.
- **internal/alert** — Alerting engine with SQLite store, evaluator that ticks on interval, manager for API CRUD. Rule definitions evaluate against live graph state.
- **internal/api** — HTTP API server serving /api/* routes. Reads from graph.Reader and store.Reader. Returns 503 when dependencies unavailable (graph disabled, alerting disabled). Caches expensive overview queries.
- Transit relationship details combine FalkorDB's current endpoint test cohort and boundary hops with ClickHouse's historical, packet-weighted loss and RTT series for that cohort.
- Route correlation reads responsive replies from a typed ClickHouse hop table populated by a materialized view at traceroute ingestion. Startup performs a guarded 31-hour backfill for existing installations. The query first selects multi-probe candidates from the exact recent window, then compares them with a deterministic one-quarter sample of the preceding baseline. Aggregation and sorting can spill at bounded thresholds before the result is enriched with live FalkorDB ASN identity and graph degree.
- **internal/web** — Embedded React UI via go:embed. Served at / (API at /api/*). Production build via `go build` includes dist files.

## External Dependencies
- **RIPE Atlas Stream** (https://atlas-stream.ripe.net/api/v2/stream/) — live measurement result firehose. Emits NDJSON with event type envelopes. Requires User-Agent header.
- **ClickHouse** — primary time-series database for all atlas_results. HTTP interface only (port 8123). Schema applied via embedded schema.sql.
- **FalkorDB** — graph database (Redis protocol) for live IP/ASN topology. Requires range indexes on Probe.id, IP.addr, AS.asn. Connection pool size 20, read timeout 10s.
- **GeoLite2-ASN** — MaxMind MMDB for IP→ASN mapping. Optional enrichment via internal/asn.LookupASN interface.
- **React UI** — embedded via go:embed web/dist, served at root path. API routes at /api/*.
- Large graph-backed worklists and nested probe/target/IP-hop relationships return bounded offset pages with `has_more` metadata; filters execute in FalkorDB before pagination rather than transferring full result sets to React. Detail relationship counts cover all retained PING or NEXT_HOP edges, while freshness-limited health detection remains a separate concern.
- IP route-graph computation performs cycle-safe, directed breadth-first expansion over retained `NEXT_HOP` edges independently upstream and downstream. Responses are capped at 30 levels, 1,000 nodes, and 10,000 edges and explicitly report whether traversal terminated naturally or hit a safety boundary.

## What Does NOT Exist Here
- No authentication/authorization — API is open, assumes firewall/network-level access control
- No message queue — pipeline uses Go channels with buffered backpressure
- No background worker service — all ingestion runs in the main process
- No separate frontend build step for deployment — UI is embedded at compile time
- No ORM for ClickHouse — raw HTTP INSERT queries with JSONEachRow format
- No Grafana/Prometheus — observability via structured logs and web UI only

## Live Health Semantics
- ClickHouse is the durable history; FalkorDB is a bounded live-state projection (6h default retention).
- Only recent PING observations (30m default) contribute to overview health, issue detection, and alert evaluation.
- Destination-wide incidents require agreement from at least three probes in at least two source ASes.
- Probes failing at least 80% of packets across three or more active targets are excluded from destination-wide detection; their source network can still appear as a source-specific issue.
- The Targets issue page requires recent agreement from at least three probes in two source ASes, excludes broadly failing probes, and only reports targets whose packet loss increased at least 20 points from a prior 24-hour baseline of at most 20% loss.
