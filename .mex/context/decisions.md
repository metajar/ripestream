---
name: decisions
description: Key architectural and technical decisions with reasoning. Load when making design choices or understanding why something is built a certain way.
triggers:
  - "why do we"
  - "why is it"
  - "decision"
  - "alternative"
  - "we chose"
edges:
  - target: context/architecture.md
    condition: when a decision relates to system structure
  - target: context/stack.md
    condition: when a decision relates to technology choice
last_updated: 2026-07-16
---

# Decisions

<!-- HOW TO USE THIS FILE:
     Each decision follows the format below.
     When a decision changes: DO NOT delete the old entry.
     Mark it as superseded, add the new entry above it.
     The history must be preserved — this is the event clock. -->

## Decision Log

### Treat correlated hop RTT shifts as investigation evidence, not fault proof
**Date:** 2026-07-16
**Status:** Active
**Decision:** Detect shared-fate candidates from route-level ClickHouse history using per-hop baselines and multi-probe agreement, enrich them from FalkorDB, and explicitly label the result as an investigation lead.
**Reasoning:** The live graph merges paths globally and cannot preserve which probe-target trace traversed a hop, while router ICMP response latency can change without affecting forwarded traffic. Route-level history establishes commonality, but responsible attribution still requires endpoint and adjacent-hop evidence.
**Alternatives considered:** Rank only the latest `NEXT_HOP.last_rtt_ms` values (rejected — no route cohort or baseline), declare the highest-latency hop the cause (rejected — ICMP de-prioritization makes that unsafe), or omit hop-level correlation entirely (rejected — it leaves valuable shared-fate evidence unused).
**Consequences:** Candidates require a 15 ms and 35% median RTT regression, historical samples, and at least two configurable agreeing probes; destination replies are excluded, results are bounded, and the UI carries a causality caveat.

### Bound FalkorDB as live state and require consensus for broad incidents
**Date:** 2026-07-16
**Status:** Active
**Decision:** Keep full measurement history in ClickHouse, prune FalkorDB observations after 6 hours by default, use a 30-minute active-health window, and require multi-probe/multi-AS agreement for destination-wide incidents.
**Reasoning:** An unbounded graph made old and unreliable probe results look current, degraded query latency, and allowed individual probes to dominate the incident view.
**Alternatives considered:** Retain the full graph forever (rejected — unbounded operational growth), hide noisy results only in the UI (rejected — alerts and APIs would remain wrong), maintain a manual probe denylist (rejected — high maintenance and slow to adapt).
**Consequences:** FalkorDB is explicitly a disposable live projection; historical investigation uses ClickHouse, graph retention and freshness are independently configurable, and source-specific failures remain visible even when those probes are excluded from destination-wide consensus.

### Use HTTP interface for ClickHouse (not native TCP)
**Date:** 2025-01-16
**Status:** Active
**Decision:** All ClickHouse communication uses HTTP POST on port 8123, not the native TCP protocol.
**Reasoning:** HTTP is simpler — no protocol driver dependency, works through proxies, easier debugging with curl. JSONEachRow format aligns with Atlas stream JSON nature. Performance is adequate for our write volume (batch inserts, high latency tolerance).
**Alternatives considered:** Native TCP driver (rejected — adds dependency without meaningful benefit), clickhouse-go HTTP driver (rejected — we only need simple INSERT).
**Consequences:** All queries are URL-encoded in POST bodies. Authentication via Basic Auth header. Connection pooling handled by http.Client.

### Use FalkorDB for live topology (not Neo4j, not TigerGraph)
**Date:** 2025-01-16
**Status:** Active
**Decision:** FalkorDB stores the live IP/ASN graph used by the UI for topology queries and alert evaluation.
**Reasoning:** Redis protocol means minimal dependency overhead. MERGE operations are batch-friendly. Browser UI (port 3000) aids development. Read timeout issues are well-documented and configurable.
**Alternatives considered:** Neo4j (rejected — heavy operational overhead), separate graph service (rejected — adds latency), materialized ClickHouse views (rejected — Cypher queries are more expressive for path operations).
**Consequences:** Must set ReadTimeout >= 10s to avoid socket timeouts on aggregate queries. Range indexes required on Probe.id, IP.addr, AS.asn.

### Embed React UI in Go binary (not separate server)
**Date:** 2025-01-16
**Status:** Active
**Decision:** Production builds embed `web/dist` via go:embed and serve at `/`. API routes at `/api/*`.
**Reasoning:** Single binary deployment — no separate frontend build/deploy step. Reduced operational complexity. Development still uses separate Vite dev server.
**Alternatives considered:** Separate frontend server (rejected — doubles deployment complexity), CDN hosting (rejected — adds external dependency), static file serving from disk (rejected — requires file management).
**Consequences:** UI rebuild required for each frontend change. `go build` must be run after `npm run build`. Dev workflow: run Go service separately from Vite dev server.

### Use pipeline.Tee with backpressure (not fan-out library)
**Date:** 2025-01-16
**Status:** Active
**Decision:** Custom Tee function fans each record to multiple sink channels (ClickHouse, FalkorDB). Drops records only when sink channel is full.
**Reasoning:** Firehose cannot block for any single sink. Backpressure via channel buffers prevents slow sinks from stalling the stream. Simple enough to not warrant external dependency.
**Alternatives considered:** Kafka/Redis Streams (rejected — adds infrastructure), separate fan-out service (rejected — adds latency), unlimited buffering (rejected — unbounded memory risk).
**Consequences:** Sink channels must be buffered (8192 default). Monitor "drops" metrics. Slow sinks cause data loss for their sink only, not the firehose.

### Use embedded schema.sql for ClickHouse DDL (not migration tool)
**Date:** 2025-01-16
**Status:** Active
**Decision:** Schema is embedded as string in Go binary and applied on startup. No versioned migrations.
**Reasoning:** Single table with idempotent CREATE TABLE IF NOT EXISTS. No schema evolution complexity. Simplicity over migration infrastructure.
**Alternatives considered:** Versioned migration files (rejected — overkill for single-table schema), manual schema application (rejected — automation is safer).
**Consequences:** Schema changes require code rebuild. No rollback mechanism. Manual intervention required for breaking schema changes.

### Alert evaluation against live graph (not historical ClickHouse queries)
**Date:** 2025-01-16
**Status:** Active
**Decision:** Alerting engine evaluates rules against FalkorDB graph state, not ClickHouse historical data. State persisted in SQLite.
**Reasoning:** Alerts need real-time topology visibility (current paths, current transit relationships). Graph queries are more expressive for "AS X appears in path between Y and Z" type rules. SQLite keeps rule definitions and evaluation state separate from operational databases.
**Alternatives considered:** ClickHouse-based alerting (rejected — complex Cypher-to-SQL translation, slower for topology queries), separate alert DB (rejected — operational complexity).
**Consequences:** Alert visibility limited to what's in the graph (traceroute + ping only). Historical alert states in SQLite, not cross-server replicable.
