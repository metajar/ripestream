---
name: add-server-paginated-worklist
description: Add or extend a large graph-backed list with server-side filtering, stable offset pagination, and bounded page metadata.
last_updated: 2026-07-17
---

# Add a Server-Paginated Worklist

## Contract

- Accept `limit`, `offset`, and `q`; validate any `sort` and `order` values through explicit whitelists.
- Clamp the public page size to 100 and normalize negative offsets to zero.
- Ask the graph query for `limit + 1`, return only `limit` rows, and set `meta.has_more` from the sentinel row. Do not run an expensive count merely to claim an exact total.
- Apply search and field filters in FalkorDB before `SKIP` and `LIMIT`; never fetch an unbounded set for client-side filtering.
- End every ordering with stable entity keys so unchanged data does not duplicate or skip rows at page boundaries.
- For nested relationship lists, keep the entity summary/count separate from the page endpoint. Ensure reciprocal pages use the same retention/freshness semantics so an edge visible from one entity is discoverable from the other.

## Frontend

- Include the offset and all filters in the TanStack Query key and request.
- Reset the offset to zero whenever a filter, tab, role, sort, or direction changes.
- Preserve the previous page while the next request loads, then render Previous/Next from response metadata.
- While `isFetching && isPlaceholderData`, show `FetchingOverlay` (RJ45 cable spinner) over the preserved rows — do not blank the table, and do not spin on background same-key refetches.
- Disable polling on worklists where live reorderings would disrupt an active investigation, especially Probes.

## Verify

- Test page-size clamping and sentinel trimming.
- Test sort whitelist fallbacks with hostile input.
- Confirm pagination/filter changes show the networking spinner overlay without wiping the prior page.
- Run `go test ./...` and `npm run build` in `web/`.
