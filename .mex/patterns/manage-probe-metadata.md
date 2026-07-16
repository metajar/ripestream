---
name: manage-probe-metadata
description: Safely change RIPE Atlas probe inventory enrichment, caching, and UI exposure.
triggers:
  - "probe metadata"
  - "probe inventory"
  - "probe type"
  - "probe location"
edges:
  - target: ../context/graph.md
    condition: when changing stored probe properties or graph queries
  - target: ../context/conventions.md
    condition: before implementation and verification
last_updated: 2026-07-16
---

# Manage Probe Metadata

## Context

Probe nodes are created by live ingestion; metadata enrichment is an asynchronous, best-effort cache of the public RIPE Atlas probe inventory and must never block that write path.

## Steps

1. Extend the typed record and normalization in `internal/atlas/probes.go`.
2. Persist only scalar FalkorDB properties in `internal/graph/probe_metadata.go`; use comma-separated tag slugs for the public API projection.
3. Keep the list request batched to 500 probe IDs and preserve the refresh interval.
4. Add response fields to the Go and TypeScript types, then expose them consistently on lists, details, search, and topology.
5. Keep the numeric RIPE probe ID visible even when a friendly name is available.

## Gotchas

- RIPE Atlas descriptions are operator-supplied and can be empty; always retain a deterministic type/location fallback.
- Coordinates are deliberately obscured and must be labelled approximate.
- Stamp `metadata_checked_at` for every requested ID, but stamp `metadata_updated_at` only when a usable record was returned.
- Do not create a second graph node or relationship for inventory data; properties keep growth bounded.
- Filters must use fixed validated Cypher fragments, never raw query-string interpolation.

## Verify

- Run `GOCACHE=/private/tmp/ripestream-go-cache go test ./...`.
- Run `npm run build` from `web/`.
- Start the local app and confirm one backfill batch reports requested/received counts.
- Verify a friendly-name row, probe detail fields, country/type filters, search navigation, and browser console errors.

## Update Scaffold

- [ ] Update `.mex/ROUTER.md` when visible enrichment behavior changes
- [ ] Update `.mex/context/graph.md` when stored properties or refresh semantics change
- [ ] Run `mex log` when cache or privacy semantics change
