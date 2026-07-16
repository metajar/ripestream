# UI Improvements: From Network Noise to Useful Decisions

## Goal

RipeStream should answer an operator's first questions in seconds:

1. **Is something worth investigating right now?**
2. **What is affected, how severe is it, and how broadly is it observed?**
3. **Where is the likely fault domain?**
4. **What should I inspect next?**

The product has the data and drill-down routes to support this, but the current UI leads with inventory counts, long rankings, and a graph visualization. Those are useful *after* triage, not during it. This plan makes the UI opinionated about priority, evidence, and next actions while keeping the existing API and pages useful for exploration.

---

## Product principles

- **Exceptions before inventory.** Surface active, recent, well-supported anomalies before counts of probes, IPs, ASes, or edges.
- **Evidence beside every conclusion.** A claim such as “high loss” must include its time window, number of probes/samples, last observation, and comparison baseline where available.
- **Progressive disclosure.** The default view is a short actionable queue. Tables, raw metrics, topology, and history are one click deeper.
- **One visual language for severity.** Use the same severity thresholds, labels, colors, and ordering across Overview, ASNs, Targets, Transit, and Alerts. Never rely on color alone.
- **Explain terms in context.** “Source AS,” “target lost,” “seen,” and “last RTT” are domain-specific. Use plain-language labels and concise tooltips.
- **Make freshness explicit.** The graph is live-ish and the overview is cached. Every decision surface needs “observed at,” “data through,” and a visible stale/error state.
- **Design for an operator’s workflow.** Identify → validate → isolate → act/watch. Navigation and drill-downs should preserve filters and provide a clear return path.

## What is currently creating noise

| Area | Current behavior | Why it is hard to use | Direction |
| --- | --- | --- | --- |
| Overview | Starts with four entity-count cards and multiple “top” lists | Counts do not indicate health; top lists have no confidence or recency context | Start with an incident/attention queue and a concise health summary |
| Loss display | Healthy/lossy/lost are raw counts; the progress bar in `LossRow` always renders at 100% | Relative magnitude cannot be read and labels do not explain thresholds | Show percentages plus counts, correctly scaled bars, and definitions |
| Rankings | AS, target, and transit tables are mostly fixed top-N lists | A single sample can outrank a broad, persistent problem; users cannot narrow or sort | Rank by a transparent impact score, expose filters and sort controls |
| Data meaning | Many values lack time range, unit, sample size, or last-seen context | Operators cannot distinguish a fresh incident from stale or weak evidence | Put context in every row and header |
| Navigation | Eight peers in the sidebar, including advanced topology | Users must know the data model before they can diagnose an issue | Group navigation by workflow; treat exploration tools as secondary |
| Topology | A force-directed graph is a primary destination with generic nodes/edges | It is visually dense and does not communicate health, direction, or a recommended next step | Make it an investigation aid launched with a selected issue and focused path |
| Alerts | Alerts are separate from the operational overview; the rule builder is technical and duplicated “Rules” headings | The highest-priority state is easy to miss; creating a useful rule requires knowing raw field names | Promote active alerts, add guided templates and human-readable summaries |
| Empty/error states | Pages can show empty tables without explaining whether data is healthy, filtered out, stale, or unavailable | Absence of rows is ambiguous | Give outcome-specific empty states and recovery actions |

---

## Recommended information architecture

### Primary navigation

Use these top-level sections, in this order:

1. **Attention** (default route; replaces the current Overview emphasis)
2. **Investigate** (search and saved/current investigations)
3. **Network** (ASNs, Targets, Probes, Transit)
4. **Alerts**
5. **Explore** (Path Explorer and Topology)

Keep a persistent global search in the top bar for AS number/name, IP/target, and probe ID. Search results should say what the item is and its current health, e.g. `Cloudflare — AS13335 — 3 affected targets` rather than just an identifier.

### Global context bar

Replace “Live topology · refreshing from FalkorDB” with an operational status bar:

- **Data status:** `Live`, `Delayed`, `Stale`, or `Unavailable`; include last successful graph and history refresh times.
- **Time range:** a global default (for example, Last 1h / 24h / 7d) that drives history views and is shown in every aggregate label.
- **Scope/filter summary:** active AS, target, probe, address-family, and severity filters, with a one-click clear action.
- **Alerts:** a visible count of firing alerts linking to the attention queue.

Do not call data “live” when the relevant value is cached. The existing overview API already reports cache metadata (`cached`, `stale`, `took_ms`, `last_ok`); expose it in the UI.

---

## Page-by-page changes

### 1. Attention / Overview

Make this a scan-friendly command center, ordered by actionability.

**Header**

- Title: **Network health**.
- A single sentence of plain-language status, such as: “3 high-confidence issues are affecting 18 targets; data current to 14:32 UTC.”
- Show the selected time range and refresh/freshness status.

**First panel: Needs attention**

Show at most 5–10 issue cards, not several unrelated rankings. Each card should contain:

- severity label (`Critical`, `High`, `Watch`) and an icon in addition to color;
- plain-language title: “High loss to AS13335 (Cloudflare) from 12 probe networks”;
- impact: affected probes, targets, AS pairs, and/or destinations;
- evidence: loss percentage, median/average RTT where relevant, sample count, and last observed time;
- why it is ranked: e.g. “92% loss across 43 probes in 8 source ASes”;
- a direct CTA: **Investigate destination**, **Compare paths**, or **View alert**.

Initially derive this queue from existing active alerts, `top_dst_as`, `top_src_as`, `top_as_pairs`, targets, and hop hotspots. Do not imply root cause; label recommendations as observations or suspected fault domains.

**Second panel: Health at a glance**

Replace the four inventory KPIs with 3–4 health metrics:

- affected targets / monitored targets;
- probes seeing loss / active probes;
- target-lost rate;
- median RTT and change versus the prior comparable window (once baseline data exists).

Entity totals can move into a compact “Network coverage” disclosure. `active_alerts`, already returned by `/api/overview`, should be displayed here and linked to Alerts.

**Third panel: Diagnose by pattern**

Provide small, mutually exclusive summaries with definitions:

- **Destination-wide:** many source networks failing to one target/AS;
- **Source-specific:** one source network failing across destinations;
- **Path hotspot:** elevated RTT on a hop shared by affected paths;
- **Isolated:** limited to a probe→target pair.

This helps users choose the right drill-down rather than interpret a list of AS pair rows unaided.

**Fix immediately:** `LossRow` calculates its bar width with `value / Math.max(1, value)`, which is always 100% for positive values. Compute each segment against the total active PING edges (or display a stacked 100% bar) and show both `n` and `%`.

### 2. ASNs, Targets, and Probes

Keep tables for investigation, but turn them into usable worklists.

- Add a compact filter row: severity, minimum probes, minimum samples, freshness, loss threshold, and address family where available.
- Support sorting by impact, loss, affected probes, last seen, and RTT; show the active sort in the UI.
- Default to **Impact** rather than raw loss. A transparent initial formula can be `loss % × affected probes`, with a minimum sample/probe threshold. Document the formula in a tooltip.
- Every row should include `last observed`, samples, and a severity label. Current AS rows include samples/probes but omit recency; Target rows omit samples.
- Use meaningful empty states: “No targets exceed 5% loss in the last hour” is different from “No recent measurements are available.”
- Permit search and pagination/virtualization rather than silently limiting results to 50 or 100.

On detail pages, begin with a **What we see** summary and a **Scope** statement before the raw lists. Example: “Loss to 1.1.1.1 is observed by 27 of 30 reporting probes, across 9 source ASes, last seen 2 minutes ago.” Then offer tabs for Health history, Affected probes, Related paths/hops, and Raw observations.

### 3. Transit & Hops

Separate topology frequency from performance diagnosis.

- Rename the tabs to **Common paths** and **Latency hotspots**, with a short definition of each.
- “Seen” must say whether it is observations, paths, or edges and over what period; show `Last observed` in both tabs.
- The 100 ms hotspot threshold is currently implicit and fixed in the page. Make it a visible, adjustable threshold and explain that high RTT alone is not packet loss or proof that a hop is at fault.
- Add impact fields to hotspots: affected paths/probes/targets, baseline RTT or change, and source/destination context. Rank by breadth and recency, not `last_rtt_ms` alone.
- Add a direct **Inspect paths through this hop** action. Avoid presenting a hop as the fault location unless corroborating evidence exists.

### 4. Path Explorer and Topology

These are expert tools, not the primary way to learn whether there is a problem.

**Path Explorer**

- Open prefilled from an issue card, target, probe, or AS pair.
- Show a path as an ordered, readable sequence with hop number, IP, ASN/org, RTT, and missing/timeout hops—not only as a graph.
- Highlight the first material RTT change or loss-related context, with language such as “RTT rises after this boundary,” not “this router is broken.”
- Offer alternative observed paths and show when each was last seen.

**Topology**

- Start with a selected investigation context rather than an empty seed form whenever launched from another page.
- Add depth, node-limit, relationship-type, and time/freshness controls; warn before rendering a broad query.
- Encode node and edge meaning: relation type, direction, recency, and performance signals. The current graph maps all edges to unlabeled links, so `PING`, `NEXT_HOP`, `IN_AS`, and `TRANSITS` are indistinguishable.
- Provide click selection details and a side panel with a readable explanation and links to the appropriate detail page.
- Preserve the force graph as an optional visual layer; pair it with an accessible list/tree or path table. Do not use color alone to distinguish IP, AS, and probe nodes.

### 5. Alerts

- Surface firing alerts in Attention and make every alert link to its scoped AS, AS pair, target, or probe→target investigation.
- Replace raw summaries such as `loss_ratio >= 50 · asn_dst` with “Alert when a destination AS has at least 50% packet loss.” Keep the raw expression in an advanced disclosure.
- Add templates for common cases: broad destination loss, source-network loss, high latency on a target, target unreachable, and probe-to-target degradation.
- Make the threshold unit unambiguous. `loss_ratio` is described as “Loss %” in the builder but the backend model may use a fractional ratio internally; validate and display the accepted unit beside the field.
- Let users edit, enable/disable, duplicate, and test rules against current matching entities before saving. Deletion needs a confirmation/undo.
- Show firing value, threshold, sample/probe count, started time, last evaluated time, and a direct investigation CTA. Add filters for active/resolved and severity.
- Remove or rename the duplicate “Rules” card headings so the builder and saved-rule list are visually distinct.

---

## Data and API work required

The current APIs are a strong start, but useful triage requires more context than a point-in-time graph aggregate.

### Add to list and issue responses

For every aggregate or candidate issue return:

- `observed_from`, `observed_to`, and `last_seen` in ISO 8601 UTC;
- `sample_count`, `probe_count`, `target_count` as applicable;
- numerator and denominator for every rate;
- severity and/or the inputs needed for a client-consistent severity calculation;
- optional `baseline_value`, `change_pct`, and baseline window;
- stable pagination (`cursor`, `next_cursor`, `total` where inexpensive);
- explicit sort and filter metadata.

Prefer returning integer loss counts plus total sent/measurements alongside `loss_pct`; a percentage without a denominator is not actionable.

### Create an issue-oriented endpoint

Add a backend-owned endpoint such as `GET /api/issues?window=1h&severity=high&limit=...`. It should normalize active alerts and detected patterns into one schema:

```json
{
  "id": "dst-as:13335",
  "kind": "destination_wide_loss",
  "severity": "high",
  "title": "High loss to AS13335 (Cloudflare)",
  "summary": "92% loss from 43 probes in 8 source ASes",
  "loss_pct": 92.0,
  "avg_rtt_ms": 140,
  "probe_count": 43,
  "target_count": 6,
  "sample_count": 860,
  "observed_from": "2025-...Z",
  "last_seen": "2025-...Z",
  "confidence": "high",
  "href": "/asn/13335",
  "evidence": ["..."],
  "source": "detected"
}
```

Use minimum evidence requirements to suppress noise (for example, a minimum number of recent probes and samples). The exact thresholds should be configurable and documented. Active alert state should augment this endpoint, not be the only source of priority.

### Time windows and baselines

The graph stores latest edge attributes while ClickHouse contains history. Use ClickHouse for selected-window aggregation and baseline comparison, especially for user-facing claims of “current,” “increased,” or “degraded.” Clearly distinguish **last observed value** from **window average/median**. For very large queries, precompute or cache rollups; preserve the current fast overview cache but display its freshness.

### URL state and investigation handoff

Persist time range, filters, sort, and selected entity in query parameters. Links from an issue to target/ASN/transit/path/topology must carry this context. This makes browser Back, sharing, and incident handoff reliable.

---

## Shared design-system improvements

- Establish written severity thresholds and reusable `SeverityBadge`, `Metric`, `Freshness`, `Evidence`, and `DataStatus` components.
- Use semantic color plus text/icon/pattern. Meet contrast requirements, including color-blind-safe loss and warning states.
- Standardize number formatting: percent precision, `ms`, UTC/local timestamp policy, relative time with an exact-time tooltip, and `—` only when paired with a reason where possible.
- Make table headers sticky where useful, rows keyboard-focusable, controls labeled, and graphs accompanied by text alternatives.
- On narrow screens, turn dense tables into a stacked issue/metric card rather than horizontal overflow as the only option.
- Use loading skeletons that preserve layout; preserve prior data while refreshing and mark it as refreshing instead of blanking the page.

---

## Delivery plan

### Phase 0 — Correctness and clarity (small, high confidence)

1. Fix the Overview loss-bar calculation.
2. Display `active_alerts` and cache freshness on Overview.
3. Add units, tooltips, last-seen labels, sample denominators where already available, and meaningful zero/empty/error states.
4. Rename ambiguous labels (`Seen`, raw alert fields, transit/hops tabs) and remove duplicated Alert “Rules” headings.
5. Verify that loss ratio units are consistent end-to-end.

**Success:** an operator can tell whether data is fresh, whether a count/rate is meaningful, and what each primary metric means without reading source code.

### Phase 1 — Make the default workflow actionable

1. Build the Attention queue from existing overview data and active alerts.
2. Move inventory KPIs behind a coverage disclosure; make health/impact the initial overview.
3. Add consistent severity scoring, min-evidence thresholds, filters, sorting, and URL-persisted state to ASNs/Targets/Probes.
4. Add direct, context-preserving CTAs into entity detail and path investigation.

**Success:** a user can identify and open the most impactful current issue in fewer than three interactions.

### Phase 2 — Add evidence and historical context

1. Extend API responses with windows, denominators, freshness, pagination, and issue evidence.
2. Implement `/api/issues` and baseline/selected-window summaries using ClickHouse.
3. Rework detail pages into summary → trend → contributing entities → raw evidence.
4. Add alert templates, human-readable rule descriptions, test-before-save, and scoped drill-downs.

**Success:** users can distinguish a broad recent degradation from a single stale or low-sample observation.

### Phase 3 — Improve expert exploration

1. Make Path Explorer issue-aware and present ordered path data.
2. Add topology filtering, relationship/performance encoding, selection details, and an accessible non-graph alternative.
3. Add saved investigations and shareable URLs.

**Success:** the graph helps validate a specific hypothesis instead of being an unexplained visual destination.

---

## Measurement and validation

Before implementation, interview or observe 3–5 intended operators using realistic questions: “What is broken?”, “Is this broad?”, “Where should I look next?”, and “Has this changed?” Capture the current time and error rate.

Instrument privacy-conscious UI events for: issue queue impressions/opened, filter use, drill-down path, time to first investigation, empty states, search failures, and alert-rule creation/test outcomes. Do not send raw IPs or other sensitive identifiers in analytics.

Validate each release with:

- usability tests: users correctly identify the highest-impact issue and explain the evidence;
- visual/accessibility review: keyboard navigation, screen-reader labels, contrast, non-color severity cues, responsive tables;
- data-contract tests: percentages always have denominators, timestamps parse consistently, rates/units agree with backend semantics;
- performance budgets: Attention content remains useful under cache refresh, and exploration queries never silently render an unbounded graph.

Primary outcome metrics: median time to identify an actionable issue, successful drill-down completion rate, percentage of investigations started from an evidence-backed issue, and reduction in dead-end/empty-state sessions.

---

# Agent-Ready Implementation Backlog

This is the handoff checklist for coding agents. Work items are intentionally small, reviewable, and ordered by dependency. Do not start a later phase until its listed acceptance criteria and tests pass. Preserve the existing Go API envelope (`{data, error, meta}`), React + TypeScript frontend, and existing routes unless a task explicitly changes them.

## Operating rules for every task

- Read the affected backend handler, graph/store query, frontend page, client type, and existing tests before changing them.
- Do not fabricate time windows, baselines, sample counts, or incident/root-cause claims from the latest-value FalkorDB graph. If the data is not available, render the field as unavailable and explain why.
- Keep all network-health language observational: “observed loss,” “affected,” and “possible hotspot,” never a claim that a router/AS caused an outage.
- Use the helpers in `web/src/lib/utils.ts` for formatting; extend those helpers instead of duplicating date, percentage, or RTT formatting in pages.
- Keep API fields snake_case and TypeScript fields consistent with the API. Add/adjust Go and TypeScript types in the same change.
- Retain keyboard operation, visible focus, text alternatives, and non-color severity indicators for every new control or visualization.
- Each task must include tests appropriate to the layer changed: Go unit/handler tests, TypeScript component tests if a test harness is introduced, and at minimum `go test ./...`, `cd web && npm run build` before handoff.
- Update the README Dashboard pages/API sections when an endpoint, route, or user-visible behavior changes.

## Definition of done for the whole program

The work is complete only when an operator can open `/`, identify a fresh, sufficiently supported issue, see the evidence and its scope, and reach a relevant ASN/target/path view in two clicks or fewer. Every aggregate shown on the default page must state or expose its freshness and denominator/sample basis. A healthy/empty network and an unavailable/stale data source must be visually and semantically distinct.

---

## Milestone 0 — Baseline, contracts, and immediate correctness

### T00.1 — Record current behavior and establish build gates

**Owner:** full-stack agent  
**Files:** `README.md`, project root CI configuration if present/new, `web/package.json`

- [x] Run `go test ./...` and `cd web && npm run build`; fix pre-existing failures only if they block the work, and report unrelated failures separately.
- [x] Capture screenshots or a short written baseline for `/`, `/asns`, `/targets`, `/transit`, `/topology`, and `/alerts` with representative data and with no data.
- [x] Add a root-level `make verify-ui` target or documented equivalent only if the repository already uses Make/automation; it must run Go tests and the web production build.
- [x] Document the expected local run command and seed/data prerequisites in the implementation PR description or README; do not add fake production data to the app.

**Acceptance criteria**

- `go test ./...` and the production Vite build are reproducible from a clean checkout.
- Reviewers have before/after evidence for the default Overview and an empty-data state.

### T00.2 — Fix Overview loss composition and make its semantics explicit

**Owner:** frontend agent  
**Files:** `web/src/pages/OverviewPage.tsx`, `web/src/lib/utils.ts` if helpers are needed

- [x] Replace the current `LossRow` bar formula. It currently uses `value / Math.max(1, value)`, so every positive category fills the entire bar.
- [x] Render one stacked composition bar for `healthy_edges`, `lossy_edges`, and `lost_edges`, each width calculated against `healthy + lossy + lost`.
- [x] For each category display both count and percentage of active PING edges. If the total is zero, render an explicit no-data state and no filled segments.
- [x] Add an inline tooltip or help text defining: Healthy = 0% observed loss; Lossy = >0% and <100%; Target lost = 100% observed loss. State that these are latest PING-edge values, not a historical time-window rate.
- [x] Ensure the bar has an accessible label containing all three counts and percentages; do not rely only on red/yellow/green color.

**Acceptance criteria**

- With values 50/30/20, the segments are 50%/30%/20%, not 100% each.
- With all zero values, the UI says “No active PING edges” and does not divide by zero.
- Screen-reader text communicates the composition without requiring visual color.

### T00.3 — Surface overview cache freshness correctly

**Owner:** backend + frontend agent  
**Files:** `internal/api/handlers_overview.go`, `web/src/api/client.ts`, `web/src/pages/OverviewPage.tsx`, optionally new shared UI component under `web/src/components/`

- [x] Change the API client `get<T>` helper or add `getWithMeta<T>` so `/api/overview` returns both `data` and typed `meta`; do not discard response metadata.
- [x] Define a TypeScript `OverviewMeta` matching `overviewMeta`: `cached`, `stale`, `took_ms`, `ttl_sec`, `last_ok`. Confirm whether `last_ok` is a boolean or timestamp; it is currently a boolean in Go, so do not label it “last refreshed at.”
- [x] On Overview, render a compact status label:
  - `Current cached view` when `cached && !stale && last_ok`;
  - `Stale cached view` when `stale`;
  - `Live query` when no cache metadata was returned;
  - `Data refresh failed` when `last_ok` is false.
- [x] Include the configured cache interval (`ttl_sec`) in a tooltip/help text. Do not use the word “live” for cached graph aggregates.
- [x] Add `active_alerts` to the Overview header/health summary with a link to `/alerts`. It must display zero explicitly.

**Acceptance criteria**

- Cached, stale, direct-query, and failed-refresh states have distinct text and non-color visual treatment.
- The page never calls stale cached data “live.”
- The existing `/api/overview` payload remains backward compatible for clients that only read `data`.

### T00.4 — Standardize freshness, labels, and empty states on existing list pages

**Owner:** frontend agent  
**Files:** `web/src/lib/utils.ts`, `web/src/components/ui/states.tsx`, `web/src/pages/ASNsPage.tsx`, `ProbesPage.tsx`, `TargetsPage.tsx`, `TransitPage.tsx`, related detail pages

- [x] Add one shared `Freshness` presentation component or helper that takes Unix seconds and renders relative time plus an exact UTC timestamp in a native title/tooltip.
- [x] Replace ambiguous `Seen` table headers with `Observations` where the source is `seen_count`; replace `Last Seen` with `Last observed` everywhere.
- [x] On AS rows, add last-observed information only after the backend task that supplies it; until then do not show a fake value.
- [x] Ensure empty result states distinguish: no qualifying issues, no data received, invalid/unavailable entity, and request failure. Reuse/extend `EmptyState` rather than showing empty table bodies.
- [x] Rename Transit tabs to `Common paths` and `Latency hotspots`; add one-sentence descriptions that distinguish frequency from measured latest RTT.
- [x] In Alerts, rename the rule-creation card to `Create rule` and the saved list to `Saved rules`.

**Acceptance criteria**

- No primary table uses an unexplained `Seen` label.
- Empty lists tell the user whether the system is healthy/filter-empty versus data-unavailable.
- Timestamp formatting is consistent and accessible across all changed pages.

### T00.5 — Verify and enforce alert threshold units

**Owner:** backend + frontend agent  
**Files:** `internal/alert/model.go`, `internal/alert/evaluator.go`, `internal/api/handlers_alerts.go`, `web/src/pages/AlertsPage.tsx`, `README.md`

- [x] Trace `loss_ratio` from graph storage through evaluator comparison and alert API response. Write down whether a rule threshold is a fraction (`0.5`) or percent (`50`).
- [x] Choose one public API/UI convention. Recommended: public thresholds use percentages for `loss_ratio` (`50` means 50%), while the evaluator converts at the API boundary if its graph value remains fractional.
- [x] Validate range and units server-side with useful 4xx messages. Reject impossible values (for example, percentage loss outside 0–100).
- [x] Render the unit beside the threshold input and in human-readable rule summaries. RTT must be `ms`; target-lost must state its valid value/condition.
- [x] Update the README alert example and API docs to match the implemented convention.

**Acceptance criteria**

- A UI rule labeled “50% loss” fires at the same threshold the user expects.
- Unit tests cover threshold conversion/validation for each metric.
- No page displays `loss_ratio` as “Loss %” while submitting a contradictory value.

---

## Milestone 1 — Shared UI primitives and context-preserving navigation

### T01.1 — Create reusable health presentation components

**Owner:** frontend agent  
**New files:** `web/src/components/health/SeverityBadge.tsx`, `Metric.tsx`, `Freshness.tsx`, `Evidence.tsx`, `DataStatus.tsx` (or equivalent colocated structure)

- [x] Define a single severity model: `critical`, `high`, `watch`, `normal`, `unknown`. Every rendered severity includes text and an icon/shape in addition to semantic color.
- [x] Define initial client-side thresholds in one exported constant, with a comment that they are temporary until `/api/issues` owns severity. Suggested starting thresholds: critical ≥80% loss with sufficient evidence; high ≥20%; watch >0%. Do not infer severity when evidence is insufficient.
- [x] Implement `Metric` with label, formatted value, unit, optional tooltip, and optional denominator/secondary text.
- [x] Implement `Evidence` to render `N probes · N samples · last observed …`; omit unavailable fields rather than showing zero as a fabricated denominator.
- [x] Implement `DataStatus` for current/stale/error/direct-query states from T00.3.
- [x] Use these primitives on Overview and at least one list/detail page in the same task to prove they are practical.

**Acceptance criteria**

- Severity colors and labels are not reimplemented ad hoc in pages.
- A severity badge remains understandable in grayscale and by screen reader.
- Components have typed props and no `any` data contracts.

### T01.2 — Add global search and URL-backed investigation context

**Owner:** frontend agent  
**Files:** `web/src/components/AppShell.tsx`, `web/src/components/SearchSelect.tsx`, `web/src/main.tsx`, relevant page files

- [x] Put a debounced global search input in the top bar using the existing `/api/search` endpoint and `SearchSelect` component where possible.
- [x] Require at least 2–3 characters before querying; cancel/ignore stale requests; provide keyboard navigation, Escape to close, and an accessible result list.
- [x] Result selection must route to the matching existing detail page (`/asn/:asn`, `/ip/:addr`, `/probe/:id`, and target when distinguishable) and preserve current query parameters.
- [x] Standardize query parameters: `range`, `min_loss`, `min_probes`, `sort`, `order`, `asn`, `target`, `probe`, and `from_issue`. Only add parameters that the receiving page actually reads.
- [x] Add a small `InvestigationContext` helper/hook that reads/writes these parameters; do not use global mutable state for shareable filters.

**Acceptance criteria**

- A URL copied after setting a supported filter restores the same UI state on reload.
- Search is fully usable without a mouse.
- Search no-result and backend-error states are distinguishable.

### T01.3 — Reorganize navigation without breaking routes

**Owner:** frontend agent  
**Files:** `web/src/components/AppShell.tsx`, `web/src/main.tsx`

- [x] Keep all current URLs working.
- [x] Group the sidebar visually into `Attention`, `Network`, and `Explore`, with Alerts visible near Attention. Do not create placeholder pages.
- [x] Change the default route label from `Overview` to `Attention` or `Network health` while retaining `/` as its URL.
- [x] Move Topology and Path Explorer under an `Explore` label and add a concise “advanced investigation” affordance.
- [x] Replace the fixed top-bar “Live topology” claim with the `DataStatus` component and global search.

**Acceptance criteria**

- A new user can find the default triage view before topology exploration.
- Existing bookmarks to every route continue to resolve.

---

## Milestone 2 — Backend evidence contracts

### T02.1 — Extend graph list models with truthful recency and evidence fields

**Owner:** backend agent  
**Files:** graph model definitions (locate `Overview`, `ASNIssue`, `TargetInfo`, `ProbeInfo`), `internal/graph/queries_read.go`, `internal/api/handlers_*.go`, `web/src/api/client.ts`

- [x] Add only fields that can be correctly derived from current FalkorDB data: `last_seen`, `sample_count` where it is a count of graph PING edges/observations, and `probe_count`/`target_count` where available.
- [x] Update `ASNIssues` queries to select `max(e.last_seen) AS last_seen` and map it to the response model.
- [x] Update unscoped/scoped target and probe queries to expose a clearly named `sample_count` when the query already computes `count(*)`; do not substitute `seen_count` from unrelated NEXT_HOP edges.
- [x] Add `last_seen` to the ASN pair issue model/query.
- [x] Preserve existing JSON field names; adding optional fields is compatible. Use Unix seconds consistently with existing graph endpoints, and document that convention in the client types.
- [x] Add Go tests with mocked/query fixture rows for mapping null/missing values and populated values.

**Acceptance criteria**

- Every newly shown recency/sample field maps to a specific query expression documented in code comments or tests.
- AS issue, target, probe, and AS-pair list responses expose enough evidence for the frontend to avoid a percentage-only row.
- No expensive unbounded aggregation is introduced; existing limit clamps remain enforced.

### T02.2 — Add explicit list filtering and sorting contracts

**Owner:** backend agent  
**Files:** `internal/api/filters.go`, `handlers_overview.go`/relevant list handlers, `internal/graph/queries_read.go`, API tests

- [x] For `/api/asn/issues`, `/api/probes`, and `/api/targets`, support validated query parameters: `limit`, `min_loss`, `min_probes` (where meaningful), `sort`, and `order`.
- [x] Whitelist sort values; never interpolate unvalidated query input into Cypher. Initial supported values: `impact`, `loss`, `probes`, `samples`, `last_seen`, `rtt` where the endpoint can provide them.
- [x] Define and document `impact`. Initial implementation: `avg_loss × distinct_probe_count`, sorted descending, with `min_probes` applied before ranking. If the API reports percentage points rather than fraction, use a consistent scale; the relative ordering is what matters.
- [x] Return effective filters/sort in response `meta`, or add them to a typed response wrapper, so the frontend can show what is applied.
- [x] Keep defaults compatible with today’s behavior where possible, except where the product decision explicitly changes the default to impact.
- [x] Reject invalid values with 400 and a useful error message; clamp numeric limits with documented max values.

**Acceptance criteria**

- Query injection is impossible through sorting/filter query parameters.
- A request can reproduce a displayed order using only URL parameters.
- Tests cover defaults, each valid sort, invalid sort/order, min evidence filters, and limit bounds.

### T02.3 — Add time-window and baseline capability from ClickHouse (discovery then implementation)

**Owner:** backend agent  
**Files:** `internal/store/`, `internal/api/handlers_timeseries.go`, `schema.sql` only if necessary, design note in this file/README

- [x] First inspect the stored RIPE payload shapes and current `timeseries` implementation to establish which measurement types and fields can produce accurate packet-loss and RTT aggregates.
- [x] Write a short design note before implementation defining supported dimensions (target, probe, destination ASN if feasible), bucket rules, timezone, and what “sample” means.
- [x] Add a validated `from`/`to` range to history endpoints, with sane defaults and maximum range/bucket limits. Return `window_start`, `window_end`, and sample count with each aggregate or response meta.
- [x] Add a comparison endpoint or fields for a prior equal-duration baseline only when the underlying payload gives comparable metrics. Return `baseline_unavailable` rather than a misleading zero baseline.
- [x] Add ClickHouse query tests or integration tests using fixtures. Confirm query plans/limits do not cause full unbounded scans in the default UI path.

**Acceptance criteria**

- Any UI phrase such as “in the last hour” or “increased” is backed by this API, not latest graph state.
- Time ranges are validated and included in response metadata.
- Baseline absence is explicit, not represented as 0% change.

### T02.4 — Implement a bounded, evidence-backed issues endpoint

**Owner:** backend agent  
**New/changed files:** `internal/api/handlers_issues.go`, `internal/api/server.go`, graph/store query code, `web/src/api/client.ts`, tests, `README.md`

- [x] Add `GET /api/issues` with parameters: `severity`, `limit`, and, only after T02.3, `from`/`to` or `range`.
- [x] Define a typed `Issue` response with at minimum: `id`, `kind`, `severity`, `title`, `summary`, `loss_pct`, `avg_rtt_ms` when applicable, `probe_count`, `target_count`, `sample_count`, `last_seen`, `confidence`, `href`, `evidence`, and `source` (`alert` or `detected`).
- [x] Implement initial detected kinds from bounded existing queries: `destination_wide_loss`, `source_network_loss`, `asn_pair_loss`, `latency_hotspot`. Merge active alerts as `source: alert`; deduplicate by a stable domain key (for example, `dst-as:13335`).
- [x] Apply explicit minimum-evidence thresholds before emitting a detected issue. Put values in named server configuration/constants and tests, not magic numbers in Cypher.
- [x] Rank deterministically: active critical alerts first, then severity, confidence/evidence breadth, recency, and stable ID as final tie-breaker.
- [x] Make `href` point only to existing routes. Encode required context safely.
- [x] Return an empty array for no issues; reserve errors for dependency/query failure. If only one source query fails, decide and document whether to return partial data with `meta.partial=true` or fail; do not silently pretend complete coverage.

**Acceptance criteria**

- The endpoint never returns an issue supported by fewer observations than its documented minimum.
- Each issue has a valid `href`, evidence text, and a test fixture covering its mapping/ranking.
- Alert and detected duplicates appear once with alert state retained in the response.
- README documents endpoint schema, units, freshness limits, and detection caveats.

---

## Milestone 3 — Actionable Attention page and worklists

### T03.1 — Replace the default Overview layout with an Attention queue

**Owner:** frontend agent  
**Files:** `web/src/pages/OverviewPage.tsx`, `web/src/api/client.ts`, shared health components

- [ ] Consume `/api/issues` once T02.4 is available. Until then, use a clearly marked adapter over existing Overview lists; do not block the whole UI redesign on the endpoint.
- [ ] Build a `Needs attention` section with a maximum of 10 cards and an explicit empty state: “No issues meet the current evidence threshold.”
- [ ] Each card must show: severity, title, plain-language summary, evidence (`probes`, `samples`, `last observed`), source (alert/detected), and one CTA to its `href`.
- [ ] Use semantic heading structure (`h1`, `h2`) and a list pattern accessible by keyboard.
- [ ] Place firing alert count near the page title and link it to `/alerts`.
- [ ] Do not display entity inventory cards above the attention queue.

**Acceptance criteria**

- At common desktop width, a user sees the top actionable issue and its evidence without scrolling.
- A card click and CTA both reach the intended detail page with context retained.
- Zero issues, stale data, partial results, loading, and error have distinct visual states.

### T03.2 — Build health and coverage summaries below the queue

**Owner:** frontend agent  
**Files:** `web/src/pages/OverviewPage.tsx`, shared components

- [ ] Replace the primary count-card row with health metrics that the current API can truthfully support: PING-edge loss composition, affected/lossy PING edges, and alert count.
- [ ] Keep probes/targets/ASes/IPs in a collapsed `Network coverage` disclosure. State that these are graph inventory counts, not current health indicators.
- [ ] If T02.3 is complete, add selected-window rates and baseline change; otherwise retain latest-value wording and do not show a time-range selector that implies historical aggregation.
- [ ] Convert AS source/destination and pair rankings into a `Diagnose by pattern` section with explanatory labels and direct links. Limit to 3–5 entries each.

**Acceptance criteria**

- The default page visually prioritizes issues and health over inventory.
- Every rate has a denominator or a tooltip pointing to it.
- Summary language matches the actual data source (latest graph vs selected historical window).

### T03.3 — Turn ASNs, Targets, and Probes into filterable worklists

**Owner:** frontend agent  
**Files:** `ASNsPage.tsx`, `TargetsPage.tsx`, `ProbesPage.tsx`, `web/src/api/client.ts`, new filter/sort components

- [ ] Add a shared worklist toolbar: loss threshold, minimum probes (where supported), sort field, sort direction, result count, and clear filters.
- [ ] Synchronize toolbar state with URL parameters using T01.2.
- [ ] Request server-side sort/filter values from T02.2; do not sort only the already limited page client-side.
- [ ] Default sort to `impact` once T02.2 is released. Show a tooltip defining the score and minimum-evidence rule.
- [ ] Add `Samples` and `Last observed` columns once T02.1 fields exist. Preserve a mobile card layout or accessible horizontal-scroll alternative.
- [ ] Add explicit empty copy based on current filters, for example: “No targets with at least 20% observed loss from 3 or more probes.”

**Acceptance criteria**

- Reloading or sharing a filtered URL produces the same API request and visible table state.
- Users can sort by impact, loss, recency, and available evidence fields.
- Tables never imply they represent all entities when a server limit/pagination is in effect.

### T03.4 — Add summary-first entity detail pages

**Owner:** frontend agent, with backend support only where fields are missing  
**Files:** `ASNDetailPage.tsx`, `TargetDetailPage.tsx`, `ProbeDetailPage.tsx`, detail API types/endpoints

- [ ] Add a `What we see` summary at the top of each detail page using currently available values. It must name the entity, loss/RTT observation, breadth, and last observed time where available.
- [ ] Add a `Scope` line: e.g., affected probes, targets, source ASes. Do not claim unavailable counts.
- [ ] Ensure target/probe tables include `Last observed`; use existing `last_seen` fields.
- [ ] Add CTAs from target and probe details to a prefilled Path Explorer and filtered Topology view, only when enough identifiers are known.
- [ ] Make the existing health trend explicitly state its selected/default time range and sample/bucket semantics once T02.3 provides them. Until then, label it as historical observations and handle no points cleanly.

**Acceptance criteria**

- Each detail page answers “what is observed here?” before presenting tables/charts.
- No detail page emits a blank chart/table without explanation.
- Investigation links preserve the originating entity in the URL.

---

## Milestone 4 — Alerts and investigation tools

### T04.1 — Make alerts readable and actionable

**Owner:** frontend + backend agent  
**Files:** `web/src/pages/AlertsPage.tsx`, alert handlers/views, client types

- [ ] Replace raw metric/scope rule text with a tested `describeRule(rule)` function. Example: “Alert when a destination AS has observed packet loss of 50% or more.”
- [ ] Extend active-alert responses with the rule threshold/unit, `last_eval_at`, and context/evidence already captured by the evaluator where available.
- [ ] Map alert `scope_key` to a safe existing investigation link. Implement a parser with tests for every supported scope; if parsing is impossible, provide a disabled explanatory state rather than a broken link.
- [ ] Add rule enable/disable and edit APIs/UI only after verifying existing alert store semantics. Require a confirmation or undo for destructive deletion.
- [ ] Add create-rule templates that fill metric, threshold, and scope; the user must still see/edit the final human-readable condition before saving.
- [ ] Add form validation and an inline preview of the submitted API payload/rule description. Do not silently coerce invalid values.

**Acceptance criteria**

- Every firing alert can either open its scoped investigation or explicitly explains why no direct drill-down is available.
- Rules are readable without knowing backend metric names.
- Create/edit/delete/enable changes invalidate all relevant React Query keys (`rules`, `active`, `events`, and Overview/Issues when appropriate).

### T04.2 — Make Path Explorer investigation-led

**Owner:** frontend agent; backend agent only if needed for safe query parameters  
**Files:** `web/src/pages/PathPage.tsx`, `web/src/api/client.ts`, `internal/graph/queries_read.go` if enriching path data

- [ ] Read `src`, `dst`, and origin context from URL parameters and prefill controls. Validate IP input before requesting a path.
- [ ] Render a readable ordered hop table in addition to any existing visual presentation: hop number, IP, ASN/org, edge RTT, observation count, and last observed if available.
- [ ] Clearly explain that shortest path is derived from observed `NEXT_HOP` relationships and may not be the current path.
- [ ] When data supports it, flag the largest RTT step as “largest observed RTT increase,” not a failure/root-cause assertion.
- [ ] Show an explicit no-path state with next actions (choose a reachable destination, inspect source/target) instead of a blank result.

**Acceptance criteria**

- A link from a target/probe/issue opens a prefilled, understandable path investigation.
- The route handles malformed/missing inputs without issuing unsafe or confusing requests.

### T04.3 — Make Topology a bounded, interpretable secondary tool

**Owner:** frontend + backend agent  
**Files:** `web/src/pages/TopologyPage.tsx`, `web/src/api/client.ts`, `internal/graph/queries_read.go`

- [ ] Read `asn`, `target`, `probe`, `depth`, and `limit` from URL query parameters; initialize the existing seed controls from them.
- [ ] Add visible controls for depth (bounded by backend), node limit (bounded by backend), and relationship types. If server-side relationship filtering is added, validate/whitelist types.
- [ ] Encode edge kind distinctly: at minimum different line style/color and legend labels for `next_hop`, `ping`, `in_as`, `transits`, `targets`, and `located_at`. Use `GraphEdge.kind`; the current implementation discards it when forming ForceGraph links.
- [ ] On node/edge selection, show a keyboard-accessible side/details panel with entity name, kind, ASN/org, metric values, relationship type/direction, and a relevant detail-page link.
- [ ] Add a text/list alternative summarizing nodes and edges for accessibility and for users who do not need the force graph.
- [ ] Before loading a large graph, show the requested bounded limits. Never remove existing backend clamps (`depth <= 4`, `limit <= 300`).
- [ ] Remove or revise the backend’s implicit “most-connected AS” default seed; an empty seed should request user input or clearly announce the selected default before querying.

**Acceptance criteria**

- Users can identify edge type and direction without inspecting source code.
- A selected graph item has a usable text detail and drill-down route.
- Topology opened from an issue/entity preserves its seed and does not begin as an unexplained generic graph.

---

## Milestone 5 — Historical intelligence, pagination, and hardening

### T05.1 — Add real pagination and result transparency

**Owner:** backend + frontend agent  
**Files:** list handlers, graph/store queries, `web/src/api/client.ts`, worklist pages

- [ ] Choose cursor pagination or offset pagination based on FalkorDB query capability; document ordering stability. Prefer cursor pagination for large, mutable lists.
- [ ] Return `next_cursor` and the effective limit/order in response metadata. Do not claim an exact total unless it is cheap and accurate.
- [ ] Add `Load more`/pagination controls with focus management and an announcement of newly loaded rows.
- [ ] Retain filters and sort when loading subsequent pages.

**Acceptance criteria**

- A user can discover that the first 50/100 rows are not the complete result set.
- Page boundaries do not duplicate or omit rows under a stable snapshot/order as far as the data store permits.

### T05.2 — Implement selected-window issue detection and baselines

**Owner:** backend + frontend agent  
**Depends on:** T02.3 and T02.4

- [ ] Add a global time-range control with a small supported set (for example 1h, 24h, 7d) and an explicit custom range only if API performance is proven.
- [ ] Pass selected ranges through Attention, detail trends, and issue links. Do not apply it to graph-only topology unless the backend can honor it; label graph freshness separately.
- [ ] Update issue ranking to use selected-window aggregates and prior equal-window baseline where available.
- [ ] Show change values with direction, baseline window, and `baseline unavailable` behavior.
- [ ] Add performance tests/observability for expensive ClickHouse aggregates and caching/preaggregation where required.

**Acceptance criteria**

- Changing range visibly changes only views backed by selected-window data.
- “Regression”/“increase” claims have a visible comparison basis.
- Default range queries meet a documented response-time budget.

### T05.3 — Accessibility, responsive behavior, and resilience pass

**Owner:** frontend QA/accessibility agent  
**Files:** all changed frontend components/pages, `web/src/index.css`

- [ ] Test all pages at keyboard-only, screen-reader semantic, 200% zoom, narrow mobile, and common desktop widths.
- [ ] Ensure focus is visible after route changes, modal/dropdown interactions, search selection, loading more rows, and graph selection.
- [ ] Add accessible names/descriptions to icons, charts, progress bars, tooltips, buttons, and all form controls.
- [ ] Verify status messages use appropriate live regions without announcing every 30-second background refresh excessively.
- [ ] Verify color contrast and non-color severity distinctions against the actual dark theme tokens in `web/src/index.css`.
- [ ] Handle slow/failing API calls by retaining prior useful data during refetch, marking it refreshing/stale, and offering retry where appropriate.

**Acceptance criteria**

- Core task—open Attention, understand top issue, open its detail—is possible without a mouse and without color perception.
- No critical content is clipped or inaccessible on a 320px-wide viewport.
- A failed refresh does not erase previously visible data without explanation.

### T05.4 — Instrument and validate the operator workflow

**Owner:** product/full-stack agent  
**Files:** analytics/telemetry integration if approved, tests/docs

- [ ] Before adding analytics, document approved telemetry destination, retention, and prohibited fields. Never emit raw IPs, targets, probe IDs, organizations, or full search queries unless explicitly approved.
- [ ] Instrument aggregate, privacy-safe events: Attention loaded, issue opened (kind/severity only), filter changed, search no-result, path requested, and rule template used.
- [ ] Add an operator test script with four tasks: identify highest-priority issue, assess breadth/freshness, inspect an affected entity, and create/test an alert rule.
- [ ] Compare completion rate, errors, and time-to-first-investigation against T00.1 baseline.

**Acceptance criteria**

- Telemetry is optional/configurable and privacy reviewed.
- The final report includes observed usability outcomes, not only implementation completion.

---

## Suggested agent sequencing

1. **Agent A:** T00.2, T00.3, T00.4 frontend portions, T01.1, T01.3.
2. **Agent B:** T00.5, T02.1, T02.2, with API tests and README updates.
3. **Agent C:** T01.2 and then T03.3 after Agent B’s contract lands.
4. **Agent D:** T02.3 discovery/implementation, then T02.4. These tasks require careful data semantics and should not be parallelized with incompatible API design.
5. **Agent E:** T03.1, T03.2, T03.4 once issue/evidence contracts are available.
6. **Agent F:** T04.1, T04.2, T04.3 after URL context/shared primitives are stable.
7. **Agent G:** T05.1–T05.4 as integration, quality, and measurement work.

When parallelizing, agents must not simultaneously edit `web/src/api/client.ts`, `web/src/pages/OverviewPage.tsx`, `internal/graph/queries_read.go`, or `README.md` without agreeing on interfaces first. The agent implementing an API contract owns its typed Go view, endpoint tests, TypeScript client type, and README schema update in the same change.
