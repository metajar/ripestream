---
name: pattern-index
description: Master index of all pattern files. Load before starting any task to check if a pattern exists.
triggers:
  - "pattern"
  - "task"
  - "how do I"
edges:
  - target: ../ROUTER.md
    condition: when starting a new session or needing context bootstrap
  - target: ../context/architecture.md
    condition: when understanding what components patterns might reference
---

# Pattern Index

Lookup table for all pattern files in this directory. Check here before starting any task — if a pattern exists, follow it.

<!-- This file is populated during setup (Pass 2) and updated whenever patterns are added.
     Each row maps a pattern file (or section) to its trigger — when should the agent load it?

     Format — simple (one task per file):
     | [filename.md](filename.md) | One-line description of when to use this pattern |

     Format — anchored (multi-section file, one row per task):
     | [filename.md#task-first-task](filename.md#task-first-task) | When doing the first task |
     | [filename.md#task-second-task](filename.md#task-second-task) | When doing the second task |

     Example (from a Flask API project):
     | [add-api-client.md](add-api-client.md) | Adding a new external service integration |
     | [debug-pipeline.md](debug-pipeline.md) | Diagnosing failures in the request pipeline |
     | [crud-operations.md#task-add-endpoint](crud-operations.md#task-add-endpoint) | Adding a new API route with validation |
     | [crud-operations.md#task-add-model](crud-operations.md#task-add-model) | Adding a new database model |

     Keep this table sorted alphabetically. One row per task (not per file).
     If you create a new pattern, add it here. If you delete one, remove it. -->

| Pattern | Use when |
|---------|----------|
| [add-alert-rule.md](add-alert-rule.md) | Adding a new alert rule type or evaluation logic |
| [add-api-endpoint.md](add-api-endpoint.md) | Adding a new API endpoint with Go handler and React route |
| [add-graph-query.md](add-graph-query.md) | Adding a new graph query feature or topology analysis |
| [debug-api-json.md](debug-api-json.md) | Diagnosing empty or invalid API JSON responses from graph-backed pages |
| [debug-stream-ingestion.md](debug-stream-ingestion.md) | Diagnosing failures in the RIPE Atlas stream ingestion pipeline |
| [manage-graph-retention.md](manage-graph-retention.md) | Changing FalkorDB retention, pruning, or live-health freshness semantics |
| [manage-probe-metadata.md](manage-probe-metadata.md) | Changing RIPE Atlas probe inventory enrichment, caching, or filters |
| [tune-target-issues.md](tune-target-issues.md) | Changing target loss detection, probe consensus, or historical baseline rules |
