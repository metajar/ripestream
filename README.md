# ripestream

A small Go service that reads the **RIPE Atlas** live result stream and writes the
results into **ClickHouse**.

By default it subscribes to the full public *firehose* — every measurement result
RIPE Atlas publishes, across all measurement types (ping, traceroute, dns, http,
sslcert, ntp, …). Each result is stored with a small set of common typed columns
plus the **complete original payload as JSON**, so the same single table works for
every measurement type and nothing is lost.

## How it works

```
RIPE Atlas stream ──NDJSON──▶  ripestream  ──batched INSERT (JSONEachRow)──▶  ClickHouse
 https://atlas-stream.ripe.net                                            atlas_results
```

- **Reader** (`internal/atlas`): opens the streaming endpoint, parses each line
  `["atlas_<event>", {payload}]`, keeps `atlas_result` payloads, and emits them.
  It auto-reconnects with capped exponential backoff and force-reconnects silent
  (low-volume) connections after an idle timeout. Each subscription filter gets
  its own connection; empty filters = the firehose.
- **Writer** (`internal/store`): batches records and inserts them as
  `JSONEachRow` over ClickHouse's HTTP interface (no driver dependency, stdlib
  only). Flushes at `--batch-size` rows or every `--flush-interval`, retries
  transient failures, and flushes a final batch on graceful shutdown.
- **Schema** (`schema.sql`): typed envelope columns + a `result_json String`
  column holding the verbatim payload, partitioned by day, ordered by
  `(type, msm_id, prb_id, timestamp)`.

The firehose mixes types, so type-specific fields (per-packet RTTs, hop lists,
DNS answers, …) are **not** promoted to columns — they live in `result_json` and
are read with ClickHouse's `JSONExtract*` functions (or materialized views).

## Build

```bash
go build -o ripestream .
```

Requires Go 1.21+ (uses `log/slog`, `embed`, `signal.NotifyContext`).

## Run

You need a ClickHouse reachable over HTTP. Quick local one:

```bash
docker run -d --name ripestream-ch -p 8123:8123 -p 9000:9000 \
  clickhouse/clickhouse-server:24.8
```

Then:

```bash
# firehose: ingest all public results
./ripestream --clickhouse http://localhost:8123

# or subscribe to specific measurements/probes (one connection each)
./ripestream --msm 1001,1003 --prb 1,2
```

The schema is applied automatically on startup (`--apply-schema`, idempotent).

### Flags

| flag | default | description |
| --- | --- | --- |
| `--clickhouse` | `http://localhost:8123` | ClickHouse HTTP base URL |
| `--db` | `ripestream` | ClickHouse database |
| `--table` | `atlas_results` | ClickHouse table |
| `--user` / `--password` | `default` / _none_ | ClickHouse credentials |
| `--msm` | _empty_ | comma-separated measurement IDs (empty = firehose) |
| `--prb` | _empty_ | comma-separated probe IDs |
| `--stream-url` | `https://atlas-stream.ripe.net/api/v2/stream/` | stream endpoint |
| `--batch-size` | `1000` | max rows per insert |
| `--flush-interval` | `5s` | max time between flushes |
| `--idle-timeout` | `5m` | reconnect a connection after this long with no data (0 disables) |
| `--apply-schema` | `true` | apply `schema.sql` on startup |
| `--log-level` | `info` | `debug`\|`info`\|`warn`\|`error` |

Each flag also reads an env var of the same upper-snake-cased name prefixed with
`RIPESTREAM_` (e.g. `RIPESTREAM_CLICKHOUSE`, `RIPESTREAM_MSM`,
`RIPESTREAM_PASSWORD`).

## Schema

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

## Project layout

```
main.go                  config, signal handling, orchestration
schema.sql               ClickHouse DDL (embedded into the binary)
internal/atlas/stream.go stream reader + reconnect/idle handling
internal/store/store.go  batched ClickHouse HTTP writer
```
