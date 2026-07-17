---
name: tune-clickhouse-history
description: Diagnose and change ClickHouse history queries that approach API deadlines or production memory limits.
triggers:
  - "ClickHouse query timeout"
  - "context deadline exceeded"
  - "ClickHouse memory"
  - "historical query"
edges:
  - target: ../context/architecture.md
    condition: when deciding whether data belongs in durable history or the live graph
  - target: ../context/conventions.md
    condition: before changing Go query code
last_updated: 2026-07-17
---

# Tune ClickHouse History Queries

## Context
Read `internal/store/query.go` for the client deadline, `main.go` for the HTTP server deadline, and the relevant query builder. Production ClickHouse has a container memory limit, but a larger limit is not a substitute for bounding query state.

## Steps
1. Confirm whether the failure is the Go query deadline, HTTP write deadline, or a ClickHouse memory exception.
2. Reduce rows before JSON expansion and reduce grouping keys before historical aggregation. For recurring reads over nested payloads, extract the required fields into a typed table at ingestion. For recent-vs-baseline detection, select candidates from the exact recent window first.
3. Use deterministic hash sampling for large baselines when approximate aggregates are already acceptable; never sample the incident window.
4. Prefer approximate distinct functions and bounded arrays. Add external group/sort spill thresholds for large aggregations.
5. Keep time, result, thread, and block-size bounds explicit in the generated SQL.

## Gotchas
- `QueryJSON` has its own deadline, and both API server constructors have a write deadline. Increasing only the ClickHouse or HTTP client timeout does not make the request viable.
- `url.URL.Query().Get` is already decoded; tests must not call `url.QueryUnescape` on it again. Literal SQL modulus operators expose this bug.
- Baseline sampling changes `baseline_samples` to the sampled count. Minimum-sample gates must apply after sampling.
- Raw nested traceroute JSON costs CPU even when aggregate memory is bounded. If candidate filtering and sampling are insufficient, add a typed hop-observation table rather than continuing to raise limits.
- ClickHouse materialized views only process new inserts. Existing installations need a bounded, idempotently guarded backfill that covers the maximum query window, and its extraction SQL must stay identical to the view.
- Multiple `arrayJoin` function calls produce a Cartesian product. Expand array indexes once and use `arrayElement` to retain aligned hop/reply positions.
- `ALTER TABLE ... MODIFY TTL` can materialize existing parts and outlive the schema client's deadline. Startup TTL migrations must use `SETTINGS materialize_ttl_after_modify=0`; schedule `MATERIALIZE TTL` or partition cleanup separately when parts predating the TTL must be purged.

## Verify
- Assert recent and baseline time bounds, candidate filtering, sampling, spill settings, and the final result limit in query-builder tests.
- Run `env GOCACHE=/tmp/ripestream-go-cache go test ./...`.
- Run `npm --prefix web run build` when API behavior shown by the embedded UI changes.
- Execute the SQL against ClickHouse with representative production volume when that service is available.

## Update Scaffold
- [ ] Update `.mex/ROUTER.md` if the production query behavior changed
- [ ] Update `.mex/context/architecture.md` or decisions when history semantics changed
- [ ] Record important query tradeoffs with `mex log --type decision`
