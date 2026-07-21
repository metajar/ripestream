---
name: router
description: Session bootstrap and navigation hub. Read at the start of every session before any task. Contains project state, routing table, and behavioural contract.
edges:
  - target: context/architecture.md
    condition: when working on system design, integrations, or understanding how components connect
  - target: context/stack.md
    condition: when working with specific technologies, libraries, or making tech decisions
  - target: context/conventions.md
    condition: when writing new code, reviewing code, or unsure about project patterns
  - target: context/decisions.md
    condition: when making architectural choices or understanding why something is built a certain way
  - target: context/setup.md
    condition: when setting up the dev environment or running the project for the first time
  - target: context/graph.md
    condition: when working with FalkorDB graph model, topology queries, or graph operations
  - target: context/alerts.md
    condition: when working with alerting engine, rule CRUD, or alert state management
  - target: patterns/INDEX.md
    condition: when starting a task — check the pattern index for a matching pattern file
last_updated: 2026-07-21
---

# Session Bootstrap

If you haven't already read `AGENTS.md`, read it now — it contains the project identity, non-negotiables, and commands.

Then read this file fully before doing anything else in this session.

## Current Project State
**Working:**
- Memory-bounded production history queries using typed ClickHouse PING metrics and ingest-time typed traceroute-hop extraction, with recent-candidate-prefiltered, deterministically sampled route correlation and an env-configurable 8 GiB ClickHouse container limit
- Rolling 24-hour retention for ClickHouse measurement tables and 6-hour retention for the FalkorDB live graph
- Direct Docker Compose server deployment with authenticated runtime GeoLite2-ASN download from private Cloudflare R2, the embedded dashboard published on host port 8080 for a Cloudflare Tunnel, live ingestion, and persistent private ClickHouse/FalkorDB services
- RIPE Atlas stream ingestion with auto-reconnect and exponential backoff
- ClickHouse batch writes with JSONEachRow format and retry logic
- FalkorDB graph topology for traceroute paths and ping health
- ASN enrichment via GeoLite2-ASN MaxMind database
- HTTP API server with embedded React UI
- Alerting engine with SQLite store and graph-based evaluation
- Overview page with network health metrics
- ASN detail pages with probe/target/transit views
- Transit pair analysis and topology visualization
- Click-through transit relationship evidence with active RIPE tests and historical loss/RTT charts
- Valid JSON API responses for empty graph aggregates, including ASN pages with no PING evidence
- Destination ASN worklists use a single-pass FalkorDB aggregate with cross-network `MinSourceASes` consensus (avoids Cloudflare Tunnel plain-text 502s from the previous double-scan timeout)
- Path finding and subgraph queries
- Alert rule CRUD and event history
- Issue detection and worklist views
- Freshness-bounded, consensus-based incident detection with noisy-probe suppression
- Target regressions verified against a healthy 24-hour ClickHouse baseline
- Bounded FalkorDB retention with batched stale-edge and orphan-node pruning
- Cached RIPE Atlas probe names, types, locations, statuses, tags, and connection details with UI filters and search
- Click-through IP detail pages showing ASN identity and observed incoming/outgoing traceroute hops
- On-demand IP route graphs that traverse observed NEXT_HOP relationships upstream and downstream, with explicit completeness limits and a full-page source/destination IP/ASN-filterable explorer
- Route Correlation page detecting shared-hop RTT regressions across independent traceroutes against a 24-hour baseline, with live ASN and topology context
- Server-paginated, server-filtered AS, probe, transit, and hop worklists with stable ordering, including complete Probe→Targets, Target→Probes, and IP incoming/outgoing hop relationships; paged probe lists do not poll while open
- Networking-themed RJ45/Cat5e loading spinner shared via `LoadingState` / `FetchingOverlay`; paginated worklists overlay it on query-key changes without blanking prior rows
- Live sidebar ingestion telemetry showing a five-second-smoothed tests/second rate, a 60-second sparkline, and total RIPE Atlas results read since process start

**Not yet built:**
- Email or external notification delivery (alerts are in-UI only)
- User authentication/authorization
- Multi-instance alert state synchronization
- Historical alert reporting beyond event log
- Export functionality for graph or time-series data
- Real-time WebSocket updates (UI polls via React Query)

**Known issues:**
- Overview inventory counts still scale with retained graph size (cached with 60s TTL)
- FalkorDB ReadTimeout must be above `readQueryTimeoutMS` (20s socket / 15s query) to avoid socket timeouts on aggregate queries
- Destination ASN worklists use a single-pass PING aggregate with `MinSourceASes` consensus; the previous double-scan noisy-probe prefilter timed out behind Cloudflare Tunnel as a plain-text 502
- Alert states persist across restarts (no clean-state mechanism)
- No dry-run mode for new alert rules (live immediately)

## Routing Table

Load the relevant file based on the current task. Always load `context/architecture.md` first if not already in context this session.

| Task type | Load |
|-----------|------|
| Understanding how the system works | `context/architecture.md` |
| Working with a specific technology | `context/stack.md` |
| Writing or reviewing code | `context/conventions.md` |
| Making a design decision | `context/decisions.md` |
| Setting up or running the project | `context/setup.md` |
| Working with FalkorDB graph or topology | `context/graph.md` |
| Working with alerting features or rules | `context/alerts.md` |
| Any specific task | Check `patterns/INDEX.md` for a matching pattern |

## Behavioural Contract

For every task, follow this loop:

1. **CONTEXT** — Load the relevant context file(s) from the routing table above. Check `patterns/INDEX.md` for a matching pattern. If one exists, follow it. Narrate what you load: "Loading architecture context..."
2. **BUILD** — Do the work. If a pattern exists, follow its Steps. If you are about to deviate from an established pattern, say so before writing any code — state the deviation and why.
3. **VERIFY** — Load `context/conventions.md` and run the Verify Checklist item by item. State each item and whether the output passes. Do not summarise — enumerate explicitly.
4. **DEBUG** — If verification fails or something breaks, check `patterns/INDEX.md` for a debug pattern. Follow it. Fix the issue and re-run VERIFY.
5. **GROW** — After meaningful work, run this binary checklist:
   - **Ground:** What changed in reality? Name the changed behavior, system, command, dependency, or workflow.
   - **Record:** If project state changed, update the "Current Project State" section above. If documented facts changed, update the relevant `context/` file surgically.
   - **Orient:** If this task can recur and no pattern exists, create one in `patterns/` using `patterns/README.md`, then add it to `patterns/INDEX.md`. If a pattern exists but you learned a gotcha, update it.
   - **Write:** Bump `last_updated` in every scaffold file you changed. If the why matters, run `mex log --type decision "<what changed and why>"` or `mex log "<note>"`.
