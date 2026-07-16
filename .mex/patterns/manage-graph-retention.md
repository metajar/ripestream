---
name: manage-graph-retention
description: Safely change FalkorDB live-state retention, pruning, or freshness semantics.
last_updated: 2026-07-16
---

# Manage FalkorDB retention

1. Keep ClickHouse as durable history and FalkorDB as bounded live state.
2. Apply health freshness through `Store.activeCutoff()` independently of the longer storage-retention interval.
3. Delete stale observation relationships in bounded batches before deleting orphan Probe, IP, and AS nodes.
4. Preserve the single-current-location invariant for `Probe-[:LOCATED_AT]->IP`.
5. Run `TestLivePruneRemovesOnlyStaleObservations` against FalkorDB and the full Go suite after query changes.
6. Document changed defaults in `README.md`, `context/graph.md`, and `context/architecture.md`.
