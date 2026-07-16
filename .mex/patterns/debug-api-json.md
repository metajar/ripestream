---
name: debug-api-json
description: Diagnose empty or invalid JSON responses from graph-backed API endpoints.
triggers:
  - "Unexpected end of JSON input"
  - "empty response"
  - "invalid JSON"
edges:
  - target: ../context/graph.md
    condition: when the failing endpoint reads FalkorDB aggregates
  - target: ../context/conventions.md
    condition: when changing API response handling
last_updated: 2026-07-16
---

# Debug API JSON

## Context
Confirm the failing page, then request each API endpoint it loads directly and inspect both the HTTP status and body length.

## Steps
1. Reproduce the page error and identify its API calls from the page component.
2. Request each endpoint directly; a `200` with a zero-byte body usually means JSON encoding failed after headers were written.
3. Inspect graph aggregates for division over empty sets and guard denominators with a Cypher `CASE` expression.
4. Ensure scalar conversion maps `NaN` and infinities to a finite domain value.
5. Encode API payloads to a buffer before writing headers so encoding failures become valid JSON `500` responses.
6. Parse response text defensively in the web client so empty or malformed payloads produce an actionable error.

## Gotchas
- FalkorDB aggregate queries may return one row even when the preceding `MATCH` produced no rows.
- Go's `encoding/json` rejects `NaN` and infinities.
- Writing `200` before encoding prevents the server from changing the status when encoding fails.

## Verify
- Add a unit test covering a non-finite scalar and an API encoding failure.
- Run focused Go tests and the frontend production build.
- Run the fixed binary against live local data and verify the exact endpoint returns non-empty valid JSON.
- Load the original page in a browser and confirm its content renders.

## Update Scaffold
- [ ] Update `.mex/ROUTER.md` if supported behavior changed
- [ ] Update `.mex/context/graph.md` with newly discovered FalkorDB behavior
- [ ] Run `mex log` when the failure mode or rationale is non-obvious
