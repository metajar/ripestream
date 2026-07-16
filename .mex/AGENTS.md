---
name: agents
description: Always-loaded project anchor. Read this first. Contains project identity, non-negotiables, commands, and pointer to ROUTER.md for full context.
last_updated: 2025-01-16
---

# Ripestream

## What This Is
A Go service that reads the RIPE Atlas live result stream and writes results into ClickHouse (full history) and FalkorDB (live topology graph).

## Non-Negotiables
- Never block the firehose — sink channels must be buffered, never block on slow writes
- Always propagate context cancellation — check ctx.Err() before blocking operations
- Never panic in production code — explicit error handling only, use errors.Is/As for type checking
- Never hardcode credentials — use environment variables or flags for all secrets
- Never write graph queries without proper indexes — MERGE requires indexes on Probe.id, IP.addr, AS.asn
- Always use Go 1.25+ ServeMux patterns for HTTP routes — "GET /api/overview" syntax
- Always use TanStack Query for data fetching in React UI — no manual fetch/state management

## Commands

**Go Service:**
- Dev: `go build -o ripestream . && ./ripestream --password ripestream`
- Build: `go build -o ripestream .`
- Test: `go test ./...`
- Docker Compose: `docker compose up -d` (ClickHouse + FalkorDB)

**React UI:**
- Dev: `cd web && npm run dev` (Vite dev server, separate from Go service)
- Build: `cd web && npm run build` (required before Go build for embedding)
- Install: `cd web && npm install`

**Combined Dev Workflow:**
1. Terminal 1: `docker compose up -d`
2. Terminal 2: `cd web && npm run dev` (UI on :5173)
3. Terminal 3: `go run . --password ripestream --http-addr :8080` (Go service on :8080)

**Production Build:**
1. `cd web && npm run build`
2. `go build -o ripestream .`
3. `./ripestream --password ripestream --ui-enabled`

## Scaffold Growth
After meaningful work, run GROW:
- Ground: what changed in reality?
- Record: update `ROUTER.md` and relevant `context/` files
- Orient: create or update a `patterns/` runbook if this can recur
- Write: bump `last_updated` on changed scaffold files and run `mex log` when rationale matters

The scaffold grows from real work, not just setup. See the GROW step in `ROUTER.md` for details.

## Navigation
At the start of every session, read `ROUTER.md` before doing anything else.
For full project context, patterns, and task guidance — everything is there.
