---
name: stack
description: Technology stack, library choices, and the reasoning behind them. Load when working with specific technologies or making decisions about libraries and tools.
triggers:
  - "library"
  - "package"
  - "dependency"
  - "which tool"
  - "technology"
edges:
  - target: context/decisions.md
    condition: when the reasoning behind a tech choice is needed
  - target: context/conventions.md
    condition: when understanding how to use a technology in this codebase
  - target: context/graph.md
    condition: when working with FalkorDB-specific configuration and queries
  - target: context/alerts.md
    condition: when working with SQLite for alert state storage
last_updated: 2026-07-17
---

# Stack

## Core Technologies
- **Go 1.25** — primary language for ingestion service. Uses log/slog, embed, signal.NotifyContext.
- **ClickHouse 24.8** — time-series database for a 24-hour rolling `atlas_results` history. HTTP interface only (not native TCP).
- **FalkorDB** — graph database for live IP/ASN topology (runs on Redis protocol, port 6379). Browser UI at port 3000.
- **React 19 + TypeScript** — frontend UI. Vite for dev/build, Tailwind CSS v4 for styling.
- **Docker Compose** — local development stack (ClickHouse + FalkorDB containers).

## Key Libraries
- **FalkorDB Go Driver v2** (github.com/FalkorDB/falkordb-go/v2) — graph client. Requires ReadTimeout > server query timeout (5s default).
- **TanStack React Query v5** — data fetching, caching, and state management in React UI. 30s refetch interval, 15s stale time.
- **React Router v7** — client-side routing with error boundary.
- **React Force Graph 2D** — topology visualization component.
- **Recharts** — time-series and metric charts.
- **golang.org/x/sync/errgroup** — goroutine coordination for concurrent writers and HTTP server shutdown.

## What We Deliberately Do NOT Use
- No ClickHouse native TCP driver — HTTP interface only for simplicity (no native protocol dependency)
- No separate Go framework (gin, echo, etc.) — use stdlib net/http with ServeMux
- No state management library (Redux, Zustand) — React Query handles server state, component state is local
- No CSS-in-JS — Tailwind utility classes only
- No ORMs — raw SQL queries for ClickHouse, Cypher queries for FalkorDB
- No separate frontend server — UI embedded in Go binary and served at /

## Version Constraints
- **ClickHouse**: 24.8 (latest in Docker Compose). HTTP query format stable.
- **Go**: 1.25+ required for log/slog, embed (Go 1.16+), signal.NotifyContext (Go 1.20+).
- **React**: 19.0+ (using React 19 features).
- **FalkorDB**: latest Docker image. Read timeout must be >= 10s to avoid client-side socket timeouts.
