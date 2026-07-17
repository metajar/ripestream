---
name: setup
description: Dev environment setup and commands. Load when setting up the project for the first time or when environment issues arise.
triggers:
  - "setup"
  - "install"
  - "environment"
  - "getting started"
  - "how do I run"
  - "local development"
edges:
  - target: context/stack.md
    condition: when specific technology versions or library details are needed
  - target: context/architecture.md
    condition: when understanding how components connect during setup
  - target: context/graph.md
    condition: when setting up FalkorDB or troubleshooting graph connectivity
  - target: context/alerts.md
    condition: when configuring alerting engine or SQLite database
last_updated: 2026-07-16
---

# Setup

## Prerequisites
- **Go 1.25+** — required for log/slog, embed (Go 1.16+), signal.NotifyContext (Go 1.20+)
- **Node.js 20+** — for building the React UI
- **Docker & Docker Compose** — for ClickHouse and FalkorDB containers
- **Make** (optional) — for convenience targets if present

## First-time Setup
1. `git clone <repo>` && `cd ripestream`
2. `docker compose up -d` — starts ClickHouse (port 8123) and FalkorDB (ports 6379, 3000)
3. `go build -o ripestream .` — builds the Go binary
4. Download GeoLite2-ASN database: `mkdir -p geolite && curl -L https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-ASN.mmdb -o geolite/GeoLite2-ASN.mmdb` (optional, for ASN enrichment)
5. `./ripestream --password ripestream` — starts firehose ingestion (requires --password for ClickHouse auth)
6. Open http://localhost:8080 for the web UI (default HTTP addr :8080)

## Environment Variables
- `RIPESTREAM_CLICKHOUSE` (default: http://localhost:8123) — ClickHouse HTTP base URL
- `RIPESTREAM_PASSWORD` (required) — ClickHouse password (default: ripestream in Docker Compose)
- `RIPESTREAM_DB` (default: ripestream) — ClickHouse database name
- `RIPESTREAM_TABLE` (default: atlas_results) — ClickHouse table name
- `RIPESTREAM_USER` (default: default) — ClickHouse user
- `RIPESTREAM_FALKOR_ADDR` (default: localhost:6379) — FalkorDB address
- `RIPESTREAM_FALKOR_PASSWORD` (optional) — FalkorDB password (none in Docker Compose)
- `RIPESTREAM_FALKOR_GRAPH` (default: ripestream) — FalkorDB graph name
- `RIPESTREAM_FALKOR_ENABLED` (default: true) — enable/disable graph writer
- `RIPESTREAM_ASN_DB` (default: geolite/GeoLite2-ASN.mmdb) — GeoLite2 ASN database path
- `RIPESTREAM_HTTP_ADDR` (default: :8080) — HTTP server address (empty to disable)
- `RIPESTREAM_UI_ENABLED` (default: true) — serve embedded UI at /
- `RIPESTREAM_ALERT_ENABLED` (default: true) — run alerting evaluator
- `RIPESTREAM_ALERT_INTERVAL` (default: 1m) — alert evaluation tick interval
- `RIPESTREAM_DB_PATH` (default: ripestream.db) — SQLite path for alert state
- `RIPESTREAM_LOG_LEVEL` (default: info) — log level: debug|info|warn|error

## Common Commands
- `docker compose up -d` — start ClickHouse + FalkorDB containers
- `CLICKHOUSE_PASSWORD='<secret>' R2_ACCOUNT_ID='<id>' R2_ACCESS_KEY_ID='<key>' R2_SECRET_ACCESS_KEY='<secret>' R2_BUCKET='<bucket>' docker compose -f docker-compose.prod.yml up -d --build` — build and start the full production stack (dashboard on internal port 8080)
- `docker compose down` — stop containers
- `go build -o ripestream .` — build Go binary
- `./ripestream --password ripestream` — run with default settings (firehose to both sinks)
- `./ripestream --password ripestream --falkor-enabled=false` — ClickHouse only
- `./ripestream --password ripestream --msm 1001 --msm 1002` — subscribe to specific measurements
- `go test ./...` — run Go tests
- `go test -v ./internal/graph` — run specific package tests
- `cd web && npm install` — install UI dependencies
- `cd web && npm run dev` — start Vite dev server (separate from Go service)
- `cd web && npm run build` — build UI for embedding

## Common Issues
- **Dokploy deployment**: Select `docker-compose.prod.yml`, set the required `CLICKHOUSE_PASSWORD`, `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, and `R2_BUCKET`, then route the domain to service `ripestream` on container port `8080`. No host port is bound; the dashboard and databases remain private except through Dokploy's domain proxy.
- **Production ASN download fails**: The R2 token needs Object Read access to the configured private bucket. Match `R2_JURISDICTION` to the bucket (`default`, `eu`, or `fedramp`) and set `R2_OBJECT_KEY` when the raw MMDB is not stored as `GeoLite2-ASN.mmdb`. An optional `GEOLITE_DB_SHA256` rejects a corrupt or unexpected object.
- **ClickHouse connection refused**: Ensure Docker Compose is running (`docker compose ps`). Check ClickHouse container logs (`docker compose logs clickhouse`).
- **FalkorDB timeout errors**: Increase ReadTimeout in `internal/graph/store.go` (currently 10s). Check FalkorDB container is healthy (`docker compose ps falkordb`).
- **Port already in use**: `lsof -i :8080` to find process, `kill -9 [PID]` or change `RIPESTREAM_HTTP_ADDR`.
- **UI not loading**: Ensure UI was built (`cd web && npm run build`) before `go build`. Check `--ui-enabled` flag (default true).
- **GeoLite2 ASN lookup fails**: Download MMDB file or set `--asn-db=""` to disable ASN enrichment (warnings only, not fatal).
- **Schema not applied**: Check `--apply-schema` flag (default true). Verify ClickHouse credentials are correct.
- **Tests failing**: Ensure Docker Compose services are running for integration tests that hit real databases.
