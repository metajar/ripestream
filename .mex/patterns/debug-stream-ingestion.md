---
name: debug-stream-ingestion
description: Diagnosing failures in the RIPE Atlas stream ingestion pipeline. Use when ingestion stops, records drop, or stream connections fail.
triggers:
  - "stream"
  - "ingestion"
  - "firehose"
  - "atlas"
  - "connection"
edges:
  - target: context/architecture.md
    condition: when understanding the ingestion pipeline flow and failure boundaries
  - target: context/setup.md
    condition: when checking Docker Compose and environment configuration
  - target: context/stack.md
    condition: when troubleshooting ClickHouse or FalkorDB connectivity issues
  - target: context/graph.md
    condition: when debugging FalkorDB-specific query or timeout issues
last_updated: 2025-01-16
---

# Debug Stream Ingestion Failures

## Context
The RIPE Atlas stream flows through: `atlas.Subscribe` → `pipeline.Tee` → concurrent writers (store.Run, graph.Run). Failures can occur at stream connection, tee, or sink stages.

## Diagnosis Steps

### 1. Check Stream Connection
Logs should show "atlas connecting" and "atlas subscribed":
```bash
# Check logs for stream status
grep "atlas" logs

# Expected:
# "atlas connecting" sub=firehose
# "atlas subscribed" ack=...
```

**If missing or error:**
- Verify internet connectivity (RIPE Atlas endpoint is external)
- Check User-Agent header is set (required by RIPE API)
- Verify stream URL: `https://atlas-stream.ripe.net/api/v2/stream/`
- Test manually: `curl -H "Accept: application/x-ndjson" "https://atlas-stream.ripe.net/api/v2/stream/?streamType=result"`

### 2. Check Record Flow
Monitor if records are being produced:
```bash
# Logs should show periodic clickhouse/falkordb insert ok messages
grep "insert ok\|upsert ok" logs
```

**If no insert messages:**
- Check channel buffers are not full (Tee drops when sinks block)
- Verify batchSize and flushInterval are reasonable (defaults: 1000, 5s)
- Check for "skipped" messages indicating non-traceroute/ping records

### 3. Check ClickHouse Sink
```bash
# ClickHouse logs
docker compose logs clickhouse | tail -50

# Test ClickHouse connectivity
curl "http://localhost:8123/ping"
# Expected: Ok

# Verify table exists
curl "http://localhost:8123/?query=SELECT+count()+FROM+ripestream.atlas_results"
# Returns row count
```

**If ClickHouse errors:**
- Verify password matches Docker Compose (default: ripestream)
- Check schema was applied: `--apply-schema` flag (default true)
- Verify table exists: `SELECT * FROM system.tables WHERE database = 'ripestream'`
- Check disk space: `docker compose exec clickhouse df -h`

### 4. Check FalkorDB Sink
```bash
# FalkorDB logs
docker compose logs falkordb | tail -50

# Test FalkorDB connectivity
docker compose exec falkordb redis-cli ping
# Expected: PONG

# Check graph exists
docker compose exec falkordb redis-cli GRAPH.QUERY ripestream "RETURN 1"
```

**If FalkorDB errors:**
- Verify ReadTimeout >= 10s in `internal/graph/store.go`
- Check indexes exist: `CALL db.indexes()` in FalkorDB Browser UI
- Verify connection pool size >= 20
- Check for "upsert failed" errors with query syntax issues

### 5. Check Tee Backpressure
```bash
# Check for dropped records
grep "dropped\|skipped" logs
```

**If drops occurring:**
- Sink channels are full (8192 buffer default)
- ClickHouse or FalkorDB writes are too slow
- Increase batchSize or flushInterval
- Check for network latency to databases

## Common Failure Modes

**Stream reconnect loop:**
- Symptom: Repeated "atlas connecting" without "subscribed"
- Cause: Network issue or authentication problem
- Fix: Check external connectivity, verify User-Agent header

**ClickHouse 401 errors:**
- Symptom: "clickhouse http 401" in logs
- Cause: Password mismatch
- Fix: Verify `RIPESTREAM_PASSWORD` matches Docker Compose CLICKHOUSE_PASSWORD

**FalkorDB i/o timeout:**
- Symptom: "i/o timeout" or context deadline exceeded
- Cause: ReadTimeout < query time (aggregate queries take 2-4s)
- Fix: Increase ReadTimeout to 10s in `internal/graph/store.go`

**No traceroute/ping records:**
- Symptom: "skipped" count increasing, no inserts
- Cause: Subscribed to DNS/HTTP/SSL measurements only
- Fix: Verify subscription includes msm/prb IDs that produce traceroute/ping data

**Schema not applied:**
- Symptom: "table not found" errors
- Cause: Schema application failed
- Fix: Manually apply schema.sql: `docker compose exec clickhouse clickhouse-client --password ripestream < schema.sql`

## Verify Checklist
After fixing ingestion issues:
- [ ] Stream connection shows "atlas subscribed" in logs
- [ ] Records are being produced (not skipped/dropped)
- [ ] ClickHouse inserts succeed ("insert ok" logs)
- [ ] FalkorDB upserts succeed ("upsert ok" logs)
- [ ] No authentication or timeout errors in logs
- [ ] Row count increasing in ClickHouse: `SELECT count() FROM ripestream.atlas_results`
- [ ] Graph nodes/edges increasing: FalkorDB Browser UI shows topology

## Update Scaffold
- [ ] Update `.mex/ROUTER.md` "Current Project State" "Known issues" if this is a recurring problem
- [ ] Update `.mex/context/architecture.md` if failure boundary was not documented
- [ ] If this debug pattern helped, update `.mex/patterns/debug-stream-ingestion.md` with new gotchas
