---
name: tune-target-issues
description: Change target loss detection without reintroducing noisy probes, biased aggregates, or persistent non-responder false positives.
triggers:
  - "target loss"
  - "target issue"
  - "100% loss"
edges:
  - target: ../context/architecture.md
    condition: when changing incident semantics
  - target: ../context/graph.md
    condition: when changing FalkorDB target queries
last_updated: 2026-07-16
---

# Tune Target Issues

## Steps
1. Qualify probes from all active targets and exclude probes broadly failing at least 80% of packets across three or more targets.
2. Aggregate all recent PING edges from remaining probes using packet-weighted sent/received totals.
3. Apply loss, probe-count, and distinct-source-AS thresholds only after aggregation.
4. Pull a wider candidate cohort before filtering so persistent non-responders cannot crowd out regressions.
5. Compare candidates with a completed ClickHouse baseline window and report only material changes from previously healthy behavior.
6. Explain in the UI that PING non-response alone does not prove a host or AS outage.

## Verify
- Unit-test persistent non-responder exclusion and baseline query scoping/escaping.
- Run the graph/API/store tests and frontend production build.
- Compare the old and new loss distributions against live local data.
- Verify the page in a browser includes current loss, baseline, change, probe count, and source-AS count.

## Gotchas
- Filtering lossy edges before aggregation makes every remaining average trend toward 100%.
- A target that has always ignored ICMP is not a new incident even when hundreds of probes agree.
- Baseline filtering must happen after fetching a wider live candidate set.
