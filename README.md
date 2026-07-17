# ripestream

A small Go service that reads the **RIPE Atlas** live result stream and writes
results into **ClickHouse** (full history) and **FalkorDB** (live topology graph).

By default it subscribes to the full public *firehose* — every measurement result
RIPE Atlas publishes, across all measurement types (ping, traceroute, dns, http,
sslcert, ntp, …). Every result is stored in ClickHouse with typed envelope columns
plus the **complete original payload as JSON**. Traceroute and ping also update a
FalkorDB graph you can traverse when investigating public-internet issues.

## How it works

```
RIPE Atlas stream ──NDJSON──▶  ripestream ──tee──▶ ClickHouse (atlas_results)
 https://atlas-stream.ripe.net                  └─▶ FalkorDB (ripestream graph)
```

- **Reader** (`internal/atlas`): opens the streaming endpoint, parses each line
  `["atlas_<event>", {payload}]`, keeps `atlas_result` payloads, and emits them.
  It auto-reconnects with capped exponential backoff and force-reconnects silent
  (low-volume) connections after an idle timeout. Each subscription filter gets
  its own connection; empty filters = the firehose.
- **Tee** (`internal/pipeline`): fans each record to both sinks without blocking
  the firehose (drops for a full sink channel only).
- **ClickHouse writer** (`internal/store`): batches records and inserts them as
  `JSONEachRow` over HTTP. Flushes at `--batch-size` or `--flush-interval`.
- **FalkorDB writer** (`internal/graph`): filters to traceroute + ping, extracts
  hop paths / RTT health, and batch-`MERGE`s nodes and edges into a hybrid IP/ASN
  graph.
- **Schema** (`schema.sql`): ClickHouse envelope, typed PING metrics, and `result_json`, applied on startup.

## Graph model (FalkorDB)

| Kind | Pattern | Source |
| --- | --- | --- |
| Nodes | `(:Probe {id})`, `(:IP {addr, af, asn?, last_seen})`, `(:AS {asn})` | traceroute / ping |
| Path | `(:IP)-[:NEXT_HOP {last_rtt_ms, last_seen, proto, seen_count}]->(:IP)` | traceroute |
| Framing | `(:Probe)-[:LOCATED_AT]->(:IP)`, `(:Probe)-[:TARGETS]->(:IP)` | traceroute |
| Health | `(:Probe)-[:PING {avg_rtt_ms, loss_ratio, …}]->(:IP)` | ping |
| ASN overlay | `(:IP)-[:IN_AS]->(:AS)`, `(:AS)-[:TRANSITS]->(:AS)` | when `asn` is set |

Timeout hops (`*`) are skipped; consecutive responsive hops still get a
`NEXT_HOP` edge across the gap. Hop and endpoint IPs are enriched with ASN +
organization from a local GeoLite2-ASN MaxMind DB (`--asn-db`, default
`geolite/GeoLite2-ASN.mmdb`), which populates `IP.asn`, `(:AS {asn, org})`,
`IN_AS`, and `TRANSITS` edges between consecutive hops in different ASes.

## Build

```bash
go build -o ripestream .
```

Requires Go 1.21+ (uses `log/slog`, `embed`, `signal.NotifyContext`).

## Run

Copy the environment template, fill in the ClickHouse password and private R2
credentials, then build and start the complete stack:

```bash
cp .env.example .env
docker compose up -d --build
```

The embedded UI and API are published on host port `8080`, so a Cloudflare
Tunnel can target `http://localhost:8080`. ClickHouse and FalkorDB are available
only to services inside the Compose network.

For development without the application container, start only the databases
and run the Go binary locally:

```bash
docker compose up -d clickhouse falkordb

# firehose: ingest all public results into both sinks
./ripestream --password ripestream

# ClickHouse only
./ripestream --password ripestream --falkor-enabled=false

# or subscribe to specific measurements/probes (one connection each)
./ripestream --password ripestream --msm 1001,1003 --prb 1,2
```

ClickHouse schema is applied automatically (`--apply-schema`). FalkorDB indexes
on `IP.addr`, `Probe.id`, and `AS.asn` are ensured on startup. Place
`GeoLite2-ASN.mmdb` under `geolite/` (or pass `--asn-db`) for ASN enrichment.

FalkorDB is a bounded live-state store, not the historical archive: by default
only the last 30 minutes contribute to health and a background janitor removes
topology older than 6 hours in small batches. ClickHouse retains the full result
history. On the first production start after upgrading, the janitor may take
several intervals to drain a large stale backlog without blocking ingestion.

## Docker Compose server deployment

The default `docker-compose.yml` builds the embedded React UI and Go service,
downloads GeoLite2-ASN through R2's authenticated S3 API at startup, and runs
the application with persistent ClickHouse, FalkorDB, and SQLite data. Port
`8080` is bound on the host; point the Cloudflare Tunnel at
`http://localhost:8080` (or `http://SERVER_IP:8080` when the connector is on a
different machine).

Create the server environment file and start the stack:

```bash
cp .env.example .env
# Edit .env before continuing.
docker compose config
docker compose up -d --build
```

Upload the raw, uncompressed `GeoLite2-ASN.mmdb` file to the private R2 bucket,
then configure these variables in `.env`:

| Variable | Required | Purpose |
| --- | --- | --- |
| `R2_ACCOUNT_ID` | yes | Cloudflare account ID used to construct the R2 S3 endpoint |
| `R2_ACCESS_KEY_ID` | yes | Access Key ID from an R2 Object Read API token |
| `R2_SECRET_ACCESS_KEY` | yes | Secret Access Key from that R2 API token |
| `R2_BUCKET` | yes | Private bucket containing the database |
| `R2_OBJECT_KEY` | no | Object key; defaults to `GeoLite2-ASN.mmdb` |
| `R2_JURISDICTION` | no | `default`, `eu`, or `fedramp`; defaults to `default` |
| `GEOLITE_DB_SHA256` | no | Expected SHA-256 checksum; startup fails on mismatch |
| `CLICKHOUSE_MEMORY_LIMIT` | no | ClickHouse container memory limit; defaults to `8g` |

Do not set the displayed Cloudflare API token value; the S3 client
uses only its generated Access Key ID and Secret Access Key. Scope the token to
Object Read on this bucket. The entrypoint downloads to a temporary file,
optionally verifies it, atomically replaces the copy under the persistent `/data`
volume, and only then starts the application. An authentication, download, or
validation failure prevents ingestion from starting.

## Web UI & API

The binary also serves an observability dashboard and JSON API (dark-mode,
Untitled-UI-styled) at the `--http-addr` (default `:8080`). With the UI enabled,
both ship from one process — the built Vite bundle is embedded via `//go:embed`.

```
http://localhost:8080/          # web UI (SPA)
http://localhost:8080/api/...   # JSON API
```

### Running the UI in development

Hot-reloading frontend dev server that proxies `/api` to the Go server:

```bash
cd web && npm install && npm run dev    # Vite on :5173
./ripestream --password ripestream      # Go API on :8080 (in another shell)
```

To embed a production build into the binary:

```bash
cd web && npm run build                 # emits web/dist/
go build -o ripestream .                # embeds web/dist/ into the binary
./ripestream --password ripestream      # serves UI + API from :8080
```

### Dashboard pages

| Page | What it shows |
| --- | --- |
| **Overview** | KPIs (probes/targets/ASes/IPs), global loss breakdown, top problematic source & destination ASes, lossiest AS pairs |
| **ASNs** → **ASN detail** | Per-AS ranking by loss; drill into probes, targets, transit relationships |
| **Probes** → **Probe detail** | Lossy probes; per-probe target health |
| **Targets** → **Target detail** | Lossy targets; health trend from ClickHouse, probes, nearby hops |
| **Transit & Hops** | Clickable AS→AS relationships with probe/measurement evidence, loss + RTT history, boundary hops, and high-latency hotspot exploration |
| **Path Explorer** | Network path between two IPs with per-hop RTT + ASN |
| **Topology** | Interactive force-directed subgraph around a seed AS/probe/target |
| **Alerts** | Rule builder, firing alerts, event history (see below) |

Every element drills down with context-preserving filters.

### JSON API

All endpoints return `{data, error, meta}` envelopes. Full surface:

> **Note:** `GET /api/overview` uses recent observations for health while keeping
> inventory counts separate. It is served from a background-refreshed
> in-memory cache (`--overview-refresh`, default 1m): the page returns instantly,
> and each background refresh logs its elapsed time (`cache refresh ok took_ms=…`).
> The response's `meta` block reports `cached`, `stale`, `took_ms`, and `last_ok`.

```
GET /api/overview                         # internet-state summary
GET /api/asn/issues?role=src|dst           # ranked ASes by loss
GET /api/asn/{asn}                         # AS detail (probes, IPs, transit, loss)
GET /api/asn/{asn}/probes|targets|transit
GET /api/probes | /api/probe/{id}
GET /api/targets | /api/target/{addr}
GET /api/ip/{addr}                         # IP node + incident NEXT_HOP edges
GET /api/hops/hotspots?min_rtt=100         # transit hotspot edges
GET /api/transit | /api/transit/{a}/{b}    # AS→AS transit edges + pair detail
GET /api/transit/{a}/{b}/series             # pair-scoped historical loss + RTT cohort
GET /api/path?src=&dst=                    # traceroute path between IPs
GET /api/path/destinations?src=            # destinations reachable from src (constrains the picker)
GET /api/search?q=&limit=                  # typeahead: AS by org/ASN, IP prefix, probe id
GET /api/graph/subgraph?asn=&depth=        # nodes+edges for topology viz
GET /api/timeseries?target=&metric=        # bucketed history from ClickHouse
GET /api/health | /api/schema              # liveness + graph model docs
```

## Alerting

The alerting subsystem evaluates threshold rules against the live FalkorDB
graph on a configurable interval (default every minute) and emits fire/resolve
events. Rule definitions, evaluation state, and event history persist in an
embedded SQLite database (`--db-path`, default `ripestream.db`).

A rule combines a **metric** (`loss_ratio`, `avg_rtt_ms`, or `target_lost`), a
**comparison** + **threshold**, a **scope** (`asn_dst`, `asn_pair`, `target`, or
`probe_target`), and optional **filters** (probe, ASN, target IP, min sent/loss).
At each tick the evaluator runs one Cypher per rule, grouped by scope, and:

- **fires** when a scope instance crosses the threshold (records an event with a
  cause-context snapshot: the ASN/org, avg loss/RTT, probe & sample counts — so
  the operator can jump straight into the relevant ASN/target/hop view);
- **resolves** when a previously-firing instance drops back under threshold.

No notifications are sent yet — alerts surface only on the **Alerts** dashboard
page (active list + event history). Rules are managed there or via the API:

```
GET    /api/alerts/rules              POST   /api/alerts/rules
GET    /api/alerts/rules/{id}         PUT    /api/alerts/rules/{id}
DELETE /api/alerts/rules/{id}
GET    /api/alerts/active             # currently firing
GET    /api/alerts/states             # per-scope evaluation state
GET    /api/alerts/events             # fire/resolve history
```

Example: alert when any destination AS averages ≥ 80% loss:

```bash
curl -X POST localhost:8080/api/alerts/rules -H 'Content-Type: application/json' -d '{
  "name": "High dst-AS loss",
  "metric": "loss_ratio",
  "comparison": ">=",
  "threshold": 80,
  "scope": "asn_dst",
  "enabled": true
}'
```


### Flags

| flag | default | description |
| --- | --- | --- |
| `--clickhouse` | `http://localhost:8123` | ClickHouse HTTP base URL |
| `--db` | `ripestream` | ClickHouse database |
| `--table` | `atlas_results` | ClickHouse table |
| `--user` / `--password` | `default` / _none_ | ClickHouse credentials |
| `--falkor-enabled` | `true` | write traceroute/ping topology to FalkorDB |
| `--falkor-addr` | `localhost:6379` | FalkorDB `host:port` |
| `--falkor-graph` | `ripestream` | FalkorDB graph name |
| `--falkor-password` | _none_ | FalkorDB password |
| `--graph-active-window` | `30m` | recent interval that contributes to live health and alerts |
| `--graph-retention` | `6h` | prune older FalkorDB observations (`0` disables) |
| `--graph-prune-interval` | `15m` | how often to prune stale graph data in bounded batches |
| `--probe-metadata-enabled` | `true` | cache public RIPE Atlas inventory details on probe nodes |
| `--probe-api-url` | RIPE Atlas probes API | probe inventory endpoint |
| `--probe-metadata-refresh` | `24h` | how often cached probe inventory is refreshed |
| `--asn-db` | `geolite/GeoLite2-ASN.mmdb` | GeoLite2-ASN `.mmdb` path (empty disables enrichment) |
| `--msm` | _empty_ | comma-separated measurement IDs (empty = firehose) |
| `--prb` | _empty_ | comma-separated probe IDs |
| `--stream-url` | `https://atlas-stream.ripe.net/api/v2/stream/` | stream endpoint |
| `--batch-size` | `1000` | max rows/ops per sink flush |
| `--flush-interval` | `5s` | max time between flushes |
| `--idle-timeout` | `5m` | reconnect a connection after this long with no data (0 disables) |
| `--apply-schema` | `true` | apply `schema.sql` on startup |
| `--http-addr` | `:8080` | HTTP address for the API + UI server (empty disables) |
| `--ui-enabled` | `true` | serve the embedded web UI at `/` |
| `--db-path` | `ripestream.db` | SQLite path for alert state (rules, states, events) |
| `--alert-enabled` | `true` | run the alerting evaluator (needs FalkorDB) |
| `--alert-interval` | `1m` | how often the alert evaluator ticks |
| `--overview-refresh` | `1m` | how often to recompute the cached overview query |
| `--log-level` | `info` | `debug`\|`info`\|`warn`\|`error` |

Each flag also reads an env var of the same upper-snake-cased name prefixed with
`RIPESTREAM_` (e.g. `RIPESTREAM_CLICKHOUSE`, `RIPESTREAM_FALKOR_ADDR`,
`RIPESTREAM_PASSWORD`).

## ClickHouse schema

```sql
CREATE TABLE ripestream.atlas_results (
    received_at DateTime64(3,'UTC') DEFAULT now64(3),
    timestamp   DateTime('UTC'),
    msm_id      UInt32,
    prb_id      UInt32,
    type        LowCardinality(String),   -- ping | traceroute | dns | ...
    msm_name    LowCardinality(String),
    from_ip     String,                    -- probe public IP (v4 or v6)
    af          UInt8,                     -- 4 | 6
    proto       LowCardinality(String),    -- ICMP | UDP | TCP
    dst_name    String,
    dst_addr    String,
    src_addr    String,
    fw          UInt32,                    -- probe firmware
    sent        UInt32,                    -- typed PING counters avoid historical JSON scans
    rcvd        UInt32,
    avg_rtt_ms  Float64,
    min_rtt_ms  Float64,
    max_rtt_ms  Float64,
    result_json String,                    -- complete original payload
    INDEX idx_result_json result_json TYPE tokenbf_v1(30720,3,0) GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (type, msm_id, prb_id, timestamp);
```

## Example queries

### ClickHouse

```sql
-- volume by measurement type
SELECT type, count() FROM ripestream.atlas_results GROUP BY type ORDER BY 2 DESC;

-- average ping RTT from the typed ingestion projection
SELECT round(avgIf(avg_rtt_ms, rcvd > 0),3)
FROM ripestream.atlas_results WHERE type = 'ping';

-- top destinations seen by traceroute
SELECT dst_addr, count() FROM ripestream.atlas_results
WHERE type = 'traceroute' GROUP BY dst_addr ORDER BY 2 DESC LIMIT 10;
```

### FalkorDB (Cypher)

```cypher
# Path between two IPs
MATCH p = (a:IP {addr:$src})-[:NEXT_HOP*1..30]->(b:IP {addr:$dst})
RETURN p LIMIT 5

# Hot / recently seen edges near a target
MATCH (x:IP)-[e:NEXT_HOP]->(t:IP {addr:$dst})
WHERE e.last_seen > $since
RETURN x.addr, e.last_rtt_ms, e.seen_count
ORDER BY e.last_rtt_ms DESC LIMIT 50

# Probe ping health to a destination
MATCH (p:Probe)-[e:PING]->(t:IP {addr:$dst})
WHERE e.loss_ratio > 0.2
RETURN p.id, e.avg_rtt_ms, e.loss_ratio, e.last_seen

# AS-level transit edges
MATCH (a:AS)-[e:TRANSITS]->(b:AS)
RETURN a.asn, a.org, b.asn, b.org, e.seen_count
ORDER BY e.seen_count DESC LIMIT 50

# IPs in an AS
MATCH (ip:IP)-[:IN_AS]->(a:AS {asn:$asn})
RETURN ip.addr, ip.last_seen LIMIT 100
```

## Project layout

```
main.go                      config, signal handling, dual-sink + API/UI orchestration
schema.sql                   ClickHouse DDL (embedded into the binary)
docker-compose.yml           ClickHouse + FalkorDB
geolite/GeoLite2-ASN.mmdb    MaxMind ASN DB (local; gitignored)
web/                         Vite + React + Tailwind v4 frontend (embedded via //go:embed)
internal/atlas/stream.go     stream reader + reconnect/idle handling
internal/pipeline/tee.go     non-blocking fan-out to sinks
internal/store/              batched ClickHouse HTTP writer + read queries (timeseries)
internal/graph/              FalkorDB traceroute/ping topology writer + read queries
internal/asn/                GeoLite2-ASN lookup
internal/api/                HTTP API server (stdlib net/http, Go 1.25 ServeMux)
internal/alert/              alerting engine (SQLite store + evaluator)
internal/web/                embedded UI SPA handler
```
# Some Graph Queries taht are super useful

### Who can't talk to who

```   
MATCH (p:Probe)-[:LOCATED_AT]->(src:IP)-[:IN_AS]->(srcAS:AS)
   MATCH (p)-[e:PING]->(t:IP)-[:IN_AS]->(dstAS:AS)
   WHERE e.loss_ratio > 0.2
     AND e.sent > 0
   RETURN
     srcAS.org                       AS src_org,
     srcAS.asn                       AS src_asn,
     dstAS.org                       AS dst_org,
     dstAS.asn                       AS dst_asn,
     count(*)                        AS samples,
     round(100.0 * avg(e.loss_ratio)) AS avg_loss_pct,
     round(100.0 * max(e.loss_ratio)) AS max_loss_pct,
     round(avg(e.avg_rtt_ms))         AS avg_rtt_ms,
     count(DISTINCT p.id)            AS probes
   ORDER BY avg_loss_pct DESC, samples DESC
   LIMIT 50;
```
