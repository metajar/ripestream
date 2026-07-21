package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"ripestream/internal/graph"
)

// Issue is the unified, evidence-backed anomaly schema served by GET /api/issues.
// It normalizes both detected graph patterns and active alerts into one shape so
// the Attention page can rank them by actionability.
type Issue struct {
	ID          string   `json:"id"`       // stable domain key, e.g. "dst-as:13335"
	Kind        string   `json:"kind"`     // destination_wide_loss | source_network_loss | asn_pair_loss | latency_hotspot | alert
	Severity    string   `json:"severity"` // critical | high | watch
	Title       string   `json:"title"`    // plain-language
	Summary     string   `json:"summary"`  // e.g. "92% loss from 43 probes in 8 source ASes"
	LossPct     float64  `json:"loss_pct"`
	AvgRttMs    float64  `json:"avg_rtt_ms,omitempty"`
	ProbeCount  int64    `json:"probe_count"`
	TargetCount int64    `json:"target_count,omitempty"`
	SampleCount int64    `json:"sample_count"`
	LastSeen    int64    `json:"last_seen"`  // unix seconds
	Confidence  string   `json:"confidence"` // high | medium | low
	Href        string   `json:"href"`       // existing detail route
	Evidence    []string `json:"evidence"`   // human-readable evidence lines
	Source      string   `json:"source"`     // detected | alert
}

// Minimum-evidence thresholds. Detected issues below these are suppressed to
// avoid single-probe/single-sample noise. Named constants, not magic numbers.
const (
	minProbesForIssue           = 3  // consensus, not a pair of potentially bad probes
	minSamplesForIssue          = 5  // enough independent current edges to rank
	minSourceASesForDestination = 2  // destination incidents must cross source networks
	minTargetsForSourceIssue    = 3  // distinguish a source outage from one bad endpoint
	criticalLossPct             = 80 // >= this with sufficient evidence is critical
	highLossPct                 = 20 // >= this is high
)

// issues handles GET /api/issues?severity=&limit=&range=1h|24h|7d
// Merges detected graph patterns with active alerts, dedupes by stable key,
// and ranks by severity then evidence breadth. When a range is provided,
// detected destination-AS issues are augmented with a ClickHouse window summary
// (loss over the selected window + baseline change) as additional evidence.
func (s *Server) issues(w http.ResponseWriter, r *http.Request) {
	if s.graph == nil {
		respondError(w, http.StatusServiceUnavailable, "graph is disabled")
		return
	}
	limit := qInt(r, "limit", 10)
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	severityFilter := r.URL.Query().Get("severity")
	rangeStr := r.URL.Query().Get("range")

	ctx := r.Context()
	issues := s.detectIssues(ctx)

	// Augment destination-wide issues with ClickHouse window data when a range
	// is requested. The graph gives latest-value detection; the window gives the
	// selected-period rate and baseline comparison so "increased" claims are backed.
	if rangeStr != "" {
		issues = s.augmentWithWindows(ctx, issues, rangeStr)
	}

	// Merge active alerts (if the alerting subsystem is wired).
	if s.alerter != nil {
		if alerts, err := s.alerter.ActiveAlerts(ctx); err == nil {
			for _, a := range alerts {
				issues = append(issues, alertToIssue(a))
			}
		}
	}

	// Deduplicate by ID, preferring alert source over detected when keys collide.
	issues = dedupeIssues(issues)

	// Rank: critical > high > watch, then by sample/probe breadth, then recency.
	sort.SliceStable(issues, func(i, j int) bool {
		return issueLess(issues[i], issues[j])
	})

	// Apply severity filter after ranking.
	if severityFilter != "" {
		filtered := issues[:0]
		for _, iss := range issues {
			if iss.Severity == severityFilter {
				filtered = append(filtered, iss)
			}
		}
		issues = filtered
	}

	if len(issues) > limit {
		issues = issues[:limit]
	}
	respondOK(w, issues)
}

// detectIssues queries the graph for the supported detected patterns and maps
// them into Issue values with evidence and severity. Failures in any single
// source query degrade gracefully (the pattern is skipped), matching the
// "partial data" principle.
func (s *Server) detectIssues(ctx context.Context) []Issue {
	var issues []Issue

	// Destination-wide loss: many sources failing to one dst AS.
	if dst, err := s.graph.ASNIssues(ctx, graph.ASNIssueFilter{
		Role: "dst", MinLoss: 0.2, MinProbes: minProbesForIssue,
		MinSourceASes: minSourceASesForDestination,
		Limit:         10, Sort: "impact", Order: "desc",
	}); err == nil {
		for _, a := range dst {
			if a.Probes < minProbesForIssue || a.Samples < minSamplesForIssue || a.SourceASes < minSourceASesForDestination {
				continue
			}
			issues = append(issues, Issue{
				ID:          fmt.Sprintf("dst-as:%d", a.ASN),
				Kind:        "destination_wide_loss",
				Severity:    severityFromLoss(a.AvgLossPct, a.Probes),
				Title:       fmt.Sprintf("High loss to AS%d (%s)", a.ASN, orgOr(a.Org, a.ASN)),
				Summary:     fmt.Sprintf("%.0f%% loss from %d probes in %d source networks", a.AvgLossPct, a.Probes, a.SourceASes),
				LossPct:     a.AvgLossPct,
				AvgRttMs:    a.AvgRttMs,
				ProbeCount:  a.Probes,
				SampleCount: a.Samples,
				LastSeen:    a.LastSeen,
				Confidence:  confidenceFrom(a.Probes, a.Samples),
				Href:        fmt.Sprintf("/asn/%d", a.ASN),
				Evidence: []string{
					fmt.Sprintf("%d probes across %d source ASes agree", a.Probes, a.SourceASes),
					fmt.Sprintf("%.0f%% average, %.0f%% max loss across %d samples", a.AvgLossPct, a.MaxLossPct, a.Samples),
					"Destination-wide detection requires agreement from at least two source networks",
				},
				Source: "detected",
			})
		}
	} else {
		slog.Debug("issues: dst-as query failed", "err", err)
	}

	// Source-network loss: one source AS failing across destinations.
	if src, err := s.graph.ASNIssues(ctx, graph.ASNIssueFilter{
		Role: "src", MinLoss: 0.2, MinProbes: minProbesForIssue,
		Limit: 5, Sort: "impact", Order: "desc",
	}); err == nil {
		for _, a := range src {
			if a.Probes < minProbesForIssue || a.Samples < minSamplesForIssue || a.Targets < minTargetsForSourceIssue {
				continue
			}
			issues = append(issues, Issue{
				ID:          fmt.Sprintf("src-as:%d", a.ASN),
				Kind:        "source_network_loss",
				Severity:    severityFromLoss(a.AvgLossPct, a.Probes),
				Title:       fmt.Sprintf("Loss from AS%d (%s)", a.ASN, orgOr(a.Org, a.ASN)),
				Summary:     fmt.Sprintf("%.0f%% loss across %d targets from this source network", a.AvgLossPct, a.Targets),
				LossPct:     a.AvgLossPct,
				ProbeCount:  a.Probes,
				SampleCount: a.Samples,
				LastSeen:    a.LastSeen,
				Confidence:  confidenceFrom(a.Probes, a.Samples),
				Href:        fmt.Sprintf("/asn/%d", a.ASN),
				Evidence: []string{
					fmt.Sprintf("%d probes in this source AS observe loss", a.Probes),
					fmt.Sprintf("%.0f%% average loss across %d targets", a.AvgLossPct, a.Targets),
				},
				Source: "detected",
			})
		}
	} else {
		slog.Debug("issues: src-as query failed", "err", err)
	}

	return issues
}

// augmentWithWindows adds ClickHouse window/baseline evidence to detected
// issues when a range is selected. It resolves each destination-AS issue to a
// representative target IP via the graph, then fetches the window summary for
// that target over the selected range. Issues without a resolvable target are
// left unchanged. Failures degrade gracefully (no augmentation).
func (s *Server) augmentWithWindows(ctx context.Context, issues []Issue, rangeStr string) []Issue {
	if s.ch == nil {
		return issues
	}
	dur, ok := parseRange(rangeStr)
	if !ok {
		return issues
	}
	to := time.Now().UTC()
	from := to.Add(-dur)
	for i := range issues {
		if issues[i].Kind != "destination_wide_loss" {
			continue
		}
		// Resolve the ASN from the href (format "/asn/{asn}").
		asnStr := strings.TrimPrefix(issues[i].Href, "/asn/")
		asn, err := strconv.ParseInt(asnStr, 10, 64)
		if err != nil || asn <= 0 {
			continue
		}
		// Find a representative target IP for this ASN from the graph.
		targets, err := s.graph.ASNTargets(ctx, asn, 1)
		if err != nil || len(targets) == 0 {
			continue
		}
		ws, err := s.ch.PingWindowSummary(ctx, targets[0].Addr, 0, from, to)
		if err != nil {
			continue
		}
		// Add window evidence without overwriting the graph-derived severity.
		issues[i].Evidence = append(issues[i].Evidence,
			fmt.Sprintf("Selected window (%s): %.1f%% loss, %.0fms avg RTT, %d samples",
				rangeStr, ws.LossPct, ws.AvgRttMs, ws.Samples))
		if ws.BaselineAvailable {
			issues[i].Evidence = append(issues[i].Evidence,
				fmt.Sprintf("Baseline: %.1f%% loss (change %+.1f pts), RTT change %+.0f%%",
					ws.BaselineLossPct, ws.ChangeLossPct, ws.ChangeRttPct))
		} else {
			issues[i].Evidence = append(issues[i].Evidence, "Baseline: unavailable (no prior-window data)")
		}
	}
	return issues
}

// parseRange maps a range shorthand to a duration.
func parseRange(s string) (time.Duration, bool) {
	switch s {
	case "15m":
		return 15 * time.Minute, true
	case "30m":
		return 30 * time.Minute, true
	case "1h":
		return time.Hour, true
	case "6h":
		return 6 * time.Hour, true
	case "24h":
		return 24 * time.Hour, true
	case "7d":
		return 7 * 24 * time.Hour, true
	}
	return 0, false
}
func alertToIssue(a AlertView) Issue {
	scope, href := parseAlertScope(a.ScopeKey)
	return Issue{
		ID:          "alert:" + strconv.FormatInt(a.RuleID, 10) + ":" + a.ScopeKey,
		Kind:        "alert",
		Severity:    severityFromLoss(a.Value, 0), // value is a percentage
		Title:       a.RuleName,
		Summary:     fmt.Sprintf("Alert firing: %.0f on %s", a.Value, scope),
		LossPct:     a.Value,
		Href:        href,
		Source:      "alert",
		Confidence:  "high",
		SampleCount: 1,
		LastSeen:    alertFiredEpoch(a.FiredAt),
		Evidence:    []string{fmt.Sprintf("Rule %q is firing at %.0f", a.RuleName, a.Value)},
	}
}

// parseAlertScope converts a scope_key (e.g. "13335" for asn_dst) into a
// readable label and an existing detail-route href. Returns ("unknown","")
// when the scope can't be parsed — the UI shows an explanatory state.
func parseAlertScope(scopeKey string) (label, href string) {
	// asn_dst scopes are bare ASN numbers.
	if n, err := strconv.ParseInt(scopeKey, 10, 64); err == nil && n > 0 {
		return fmt.Sprintf("AS%d", n), fmt.Sprintf("/asn/%d", n)
	}
	// asn_pair scopes look like "123→456".
	if parts := strings.Split(scopeKey, "→"); len(parts) == 2 {
		return scopeKey, ""
	}
	return scopeKey, ""
}

func alertFiredEpoch(iso string) int64 {
	if t, err := parseISO(iso); err == nil {
		return t
	}
	return 0
}

// --- ranking + severity helpers ---

func severityFromLoss(lossPct float64, probes int64) string {
	if lossPct >= criticalLossPct && probes >= minProbesForIssue {
		return "critical"
	}
	if lossPct >= highLossPct {
		return "high"
	}
	if lossPct > 0 {
		return "watch"
	}
	return "watch"
}

func confidenceFrom(probes, samples int64) string {
	if probes >= 5 && samples >= 20 {
		return "high"
	}
	if probes >= minProbesForIssue && samples >= minSamplesForIssue {
		return "medium"
	}
	return "low"
}

// issueLess defines the deterministic ranking: alerts first, then severity,
// then evidence breadth (samples), then recency, then stable ID.
func issueLess(a, b Issue) bool {
	if a.Source == "alert" && b.Source != "alert" {
		return true
	}
	if a.Source != "alert" && b.Source == "alert" {
		return false
	}
	if a.Severity != b.Severity {
		return sevRank(a.Severity) < sevRank(b.Severity)
	}
	if a.SampleCount != b.SampleCount {
		return a.SampleCount > b.SampleCount
	}
	if a.LastSeen != b.LastSeen {
		return a.LastSeen > b.LastSeen
	}
	return a.ID < b.ID
}

func sevRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "high":
		return 1
	case "watch":
		return 2
	}
	return 3
}

// dedupeIssues collapses issues with the same ID, preferring alert source.
func dedupeIssues(in []Issue) []Issue {
	seen := make(map[string]int, len(in)) // id -> index in out
	out := make([]Issue, 0, len(in))
	for _, iss := range in {
		if idx, ok := seen[iss.ID]; ok {
			// Prefer alert source over detected when keys collide.
			if iss.Source == "alert" && out[idx].Source != "alert" {
				out[idx] = iss
			}
			continue
		}
		seen[iss.ID] = len(out)
		out = append(out, iss)
	}
	return out
}

func orgOr(org string, asn int64) string {
	if org != "" {
		return org
	}
	return fmt.Sprintf("AS%d", asn)
}

// parseISO parses an RFC3339 timestamp to unix seconds.
func parseISO(s string) (int64, error) {
	t, err := parseISOTime(s)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}
