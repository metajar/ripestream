---
name: graph
description: FalkorDB graph model, topology queries, and live graph operations. Load when working with the IP/ASN topology graph, path queries, or graph-based features.
triggers:
  - "graph"
  - "topology"
  - "falkor"
  - "path"
  - "transit"
  - "asn"
  - "hop"
edges:
  - target: context/architecture.md
    condition: when understanding how the graph writer fits into the ingestion pipeline
  - target: context/stack.md
    condition: when working with FalkorDB-specific features or configuration
  - target: context/alerts.md
    condition: when alert rules need to evaluate against graph state
  - target: context/conventions.md
    condition: when writing Cypher queries or working with graph operations
  - target: patterns/add-graph-query.md
    condition: when implementing a new graph query feature
last_updated: 2026-07-17
---

# Graph & Topology

## Graph Model (FalkorDB)

### Node Types
- **(:Probe {id, display_name?, probe_type?, country_code?, status_name?, metadata_updated_at?})** — RIPE Atlas probe identifier plus asynchronously cached public inventory metadata.
- **(:IP {addr, af, asn?, last_seen})** — IPv4/IPv6 address. `addr` is string key, `af` is address family (4/6). Optional `asn` from enrichment.
- **(:AS {asn, org})** — Autonomous system. `asn` is integer key, `org` is organization name from MaxMind.

### Edge Types
- **(:IP)-[:NEXT_HOP {last_rtt_ms, last_seen, proto, seen_count}]->(:IP)** — Traceroute hop relationship. RTT in milliseconds, protocol (ICMP/UDP), last seen timestamp.
- **(:IP)-[:IN_AS]->(:AS)** — IP belongs to AS (from MaxMind enrichment).
- **(:AS)-[:TRANSITS {last_seen, seen_count}]->(:AS)** — AS-to-AS transit (consecutive hops in different ASes).
- **(:Probe)-[:LOCATED_AT {last_seen}]->(:IP)** — Probe's single current source IP address; older locations are removed during upsert.
- **(:Probe)-[:TARGETS {msm_id, last_seen}]->(:IP)** — Probe targeting an IP (traceroute destination).
- **(:Probe)-[:PING {avg_rtt_ms, min_rtt_ms, max_rtt_ms, loss_ratio, sent, rcvd, last_seen, msm_id}]->(:IP)** — Ping health relationship.

## Graph Writer Operations
- **Batch MERGE** — Every flushInterval (default 5s) or batchSize ops (default 500).
- **MERGE semantics** — CREATE nodes/edges if missing, MATCH if existing. Update `last_seen` and increment `seen_count`.
- **Timeout hops** — `*` responses are skipped entirely. Consecutive responsive hops still get NEXT_HOP across the gap.
- **ASN enrichment** — LookupASN interface resolves IPs to (asn, org). MaxMind GeoLite2-ASN database via internal/asn package.
- **Probe inventory enrichment** — `RunProbeMetadata` scans only stale/missing probe nodes, fetches up to 500 IDs per RIPE Atlas request, and updates properties independently of the ingestion path so an API outage cannot block measurement writes.

## Important Query Patterns
- **Path between ASes**: `MATCH path = (a:AS {asn: $asnA})-[:TRANSITS*]->(:AS {asn: $asnB})` — returns transit AS sequence.
- **IP details with ASN**: `MATCH (ip:IP {addr: $addr}) OPTIONAL MATCH (ip)-[:IN_AS]->(as:AS) RETURN ip, as`
- **Hot hops**: Aggregation on NEXT_HOP edges grouped by (from, to) with high RTT or low seen_count.
- **Probe reachability**: `MATCH (p:Probe {id: $id})-[:TARGETS|LOCATED_AT]->()` — find all IPs connected to a probe.
- **Probe/target detail relationships**: paginate retained `PING` edges symmetrically in both directions. Do not apply the 30-minute health cutoff to only one direction; the cutoff belongs to health/incident views, while entity detail pages describe the retained live graph.
- **IP hop detail relationships**: keep anchored incoming/outgoing NEXT_HOP counts on the IP summary and page each direction independently so high-degree hops remain completely traceable without transferring every edge.
- **IP route graph**: expand incoming and outgoing `NEXT_HOP` frontiers independently with per-direction visited sets so cycles terminate safely and neither side can consume the entire shared node budget. Return all retained edges between selected nodes, plus explicit completion/truncation metadata; never imply a safety-bounded response is complete.
- **Transit-pair endpoint tests**: anchor source and destination AS nodes, then match source-AS probes through current `PING` edges to destination-AS targets; present this as endpoint evidence, never proof that each ping crossed the selected `TRANSITS` edge.
- **Target regressions**: aggregate every recent edge from quality-qualified probes before filtering by loss, require cross-network agreement, then compare candidates with ClickHouse history; persistent PING non-responders are not new incidents.
- **Shared-hop regressions**: reconstruct route-level observations from ClickHouse history first, then batch-enrich the bounded candidate IPs from FalkorDB; never infer route commonality from the globally merged `NEXT_HOP` graph alone.

## Index Requirements
FalkorDB requires range indexes for MERGE lookups:
- `CREATE INDEX FOR (n:IP) ON (n.addr)` — IP address lookups
- `CREATE INDEX FOR (n:Probe) ON (n.id)` — probe ID lookups
- `CREATE INDEX FOR (n:AS) ON (n.asn)` — ASN lookups

## Reader Interface
`internal/graph.Reader` defines read operations used by the API:
- `Overview()` — aggregate stats for the dashboard (probe count, IP count, AS count, active measurements)
- `ProbeDetail()`, `ASNDetail()`, `IPDetail()` — entity-specific views with connections
- `IPRouteGraph()` — cycle-safe directed traversal from an IP toward observed route beginnings and endings
- `TransitPairs()`, `TransitDetail()` — AS-to-AS transit relationships
- `TransitPairTests()` — active probe/measurement/target health evidence between a relationship's endpoint ASes
- `HopContexts()` — batch ASN identity and incoming/outgoing topology breadth for historically correlated hop IPs
- `PathQuery()` — pathfinding between ASNs or IPs
- `Search()` — fuzzy search across entities

## Gotchas
- **Empty aggregates**: FalkorDB can return `NaN` for arithmetic over an empty aggregate row; guard divisors in Cypher with `CASE WHEN total > 0` and keep scalar conversion non-finite-safe so JSON encoding cannot fail.
- **Loss-filter bias**: never filter `PING` edges by `loss_ratio` before calculating a target aggregate; doing so removes healthy evidence and can turn a mixed target into a false 100% loss result.
- **Read timeout**: FalkorDB go-redis client defaults ReadTimeout to 3s, shorter than server-side query timeout (5s). Aggregate queries (especially Overview) can take 2-4s. Must set ReadTimeout >= 10s to avoid "i/o timeout" errors.
- **Connection pool**: Default pool size is too small for concurrent read + write load. Set PoolSize >= 20.
- **Idempotent MERGE**: Running the same MERGE twice is safe. Edge properties get updated, nodes get merged. No duplicates.
- **Schema enforcement**: FalkorDB is schemaless for properties, but indexes must exist for MERGE performance.
- **Graph size**: Live firehose creates millions of nodes/edges. Queries without proper filters can timeout. Always use index-backed filters.
- **Retention**: `RunJanitor` removes stale observation edges and then orphan nodes in batches (6h default). Never use FalkorDB as the historical archive; query ClickHouse for history.
- **Freshness**: Health and alert queries use `Store.activeCutoff()` (30m default) so retained but stale observations do not appear current.
- **Probe metadata cache**: use `metadata_checked_at` to pace missing/private IDs and `metadata_updated_at` only for usable inventory records; otherwise unavailable probes look falsely enriched and are retried too aggressively.
