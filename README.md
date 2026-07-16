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
- **Schema** (`schema.sql`): ClickHouse envelope + `result_json`, applied on startup.

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

Start local ClickHouse + FalkorDB:

```bash
docker compose up -d
```

Then:

```bash
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

FalkorDB Browser UI (compose): http://localhost:3000

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
| `--asn-db` | `geolite/GeoLite2-ASN.mmdb` | GeoLite2-ASN `.mmdb` path (empty disables enrichment) |
| `--msm` | _empty_ | comma-separated measurement IDs (empty = firehose) |
| `--prb` | _empty_ | comma-separated probe IDs |
| `--stream-url` | `https://atlas-stream.ripe.net/api/v2/stream/` | stream endpoint |
| `--batch-size` | `1000` | max rows/ops per sink flush |
| `--flush-interval` | `5s` | max time between flushes |
| `--idle-timeout` | `5m` | reconnect a connection after this long with no data (0 disables) |
| `--apply-schema` | `true` | apply `schema.sql` on startup |
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

-- average ping RTT, pulled straight from the JSON payload
SELECT round(avg(toFloat64OrZero(JSONExtractString(result_json,'avg'))),3)
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
main.go                      config, signal handling, dual-sink orchestration
schema.sql                   ClickHouse DDL (embedded into the binary)
docker-compose.yml           ClickHouse + FalkorDB
geolite/GeoLite2-ASN.mmdb    MaxMind ASN DB (local; gitignored)
internal/atlas/stream.go     stream reader + reconnect/idle handling
internal/pipeline/tee.go     non-blocking fan-out to sinks
internal/store/store.go      batched ClickHouse HTTP writer
internal/graph/              FalkorDB traceroute/ping topology writer
internal/asn/                GeoLite2-ASN lookup
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