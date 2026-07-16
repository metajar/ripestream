// This file adds read-only query helpers to *Store. The writer in store.go
// owns connection setup; these methods turn FalkorDB QueryResults into typed Go
// values ready for JSON serialization by the API layer.
//
// All Cypher here was validated against live ingested data. FalkorDB specifics
// to keep in mind:
//   - last_seen / last_rtt_ms / loss_ratio etc. are scalar numbers (epoch int
//     for last_seen).
//   - aggregations used in ORDER BY must be aliased (FalkorDB cannot map them).
//   - relationship property access (e.PROP) goes out of scope after WITH; carry
//     the value through as an aliased bound variable instead.
//   - OPTIONAL MATCH yields <nil> for missing ASNs.
package graph

import (
	"context"
	"fmt"
	"time"
)

// Reader is a read-only handle to the graph. It is the subset of *Store the API
// layer depends on, kept narrow so tests can mock it.
type Reader interface {
	Overview(ctx context.Context) (Overview, error)
	ASNIssues(ctx context.Context, f ASNIssueFilter) ([]ASNIssue, error)
	ASNPairIssues(ctx context.Context, f ASNPairFilter) ([]ASNPairIssue, error)
	ASNDetail(ctx context.Context, asn int64) (ASNDetail, error)
	ASNProbes(ctx context.Context, asn int64, limit int) ([]ProbeInfo, error)
	ASNTargets(ctx context.Context, asn int64, limit int) ([]TargetInfo, error)
	ASNTransit(ctx context.Context, asn int64, limit int) ([]ASNTransitEdge, error)
	ProbeDetail(ctx context.Context, id int64) (ProbeDetail, error)
	Probes(ctx context.Context, f ProbeFilter) ([]ProbeInfo, error)
	TargetDetail(ctx context.Context, addr string) (TargetDetail, error)
	Targets(ctx context.Context, f TargetFilter) ([]TargetInfo, error)
	HotHops(ctx context.Context, f HopFilter) ([]HotHop, error)
	TransitEdges(ctx context.Context, f TransitFilter) ([]ASNTransitEdge, error)
	TransitPairDetail(ctx context.Context, asnA, asnB int64) (TransitPairDetail, error)
	IPDetail(ctx context.Context, addr string) (IPDetail, error)
	Path(ctx context.Context, src, dst string, maxHops int) (Path, error)
	ReachableDestinations(ctx context.Context, src string) ([]ReachableGroup, error)
	Subgraph(ctx context.Context, f SubgraphFilter) (Subgraph, error)
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)
}

// AlertScopeRow is one evaluated scope instance for alerting.
type AlertScopeRow struct {
	ScopeKey string
	Value    float64
	Context  map[string]any
}

// AlertEvaluator is the graph-side surface the alert evaluator needs. It's split
// from Reader so the graph package doesn't import the alert package.
type AlertEvaluator interface {
	EvaluateAlert(ctx context.Context, rule AlertRuleSpec) ([]AlertScopeRow, error)
}

// AlertRuleSpec is the alert-rule projection the graph package needs to build its
// Cypher. It mirrors alert.Rule without importing it (avoids a cycle).
type AlertRuleSpec struct {
	Metric     string  // loss_ratio | avg_rtt_ms | target_lost
	Scope      string  // probe_target | asn_pair | target | asn_dst
	Comparison string
	Threshold  float64
	MinSent    int
	MinLoss    float64
	SrcASN     int64
	DstASN     int64
	ProbeID    int64
	TargetIP   string
}

// Compile-time assertion that *Store satisfies Reader.
var _ Reader = (*Store)(nil)

// ---- Generic query-to-rows helper ------------------------------------------

// rows runs a parameterized Cypher read query and yields one row at a time as a
// column-name → value map. Values are the Go scalars FalkorDB's client decodes
// (string, int64, float64, bool, nil, plus Node/Edge/Path for graph projections).
func (s *Store) rows(ctx context.Context, cypher string, params map[string]any) ([]map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	res, err := s.graph.Query(cypher, params, nil)
	if err != nil {
		return nil, fmt.Errorf("cypher: %w", err)
	}
	out := make([]map[string]any, 0, 16)
	for res.Next() {
		r := res.Record()
		keys := r.Keys()
		vals := r.Values()
		row := make(map[string]any, len(keys))
		for i := 0; i < len(keys) && i < len(vals); i++ {
			row[keys[i]] = vals[i]
		}
		out = append(out, row)
	}
	return out, nil
}

// ---- typed scalar extractors (handle FalkorDB's nil for missing values) -----

func asInt(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	}
	return 0
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int64:
		return float64(t)
	case int:
		return float64(t)
	}
	return 0
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// asTime converts a FalkorDB epoch-seconds scalar into UTC time, mapping the
// zero value / nil to the Go zero time.
func asTime(v any) time.Time {
	sec := asInt(v)
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

// asEpoch returns the epoch-seconds int (0 if nil/missing) without converting.
func asEpoch(v any) int64 { return asInt(v) }

// ptrIf returns a pointer to v when v is non-zero, else nil. Used for optional
// fields (e.g. an ASN that may be unknown for an IP).
func ptrIf[T any](v T, zero T) *T {
	if any(v) == any(zero) {
		return nil
	}
	return &v
}

// ---- Response structs -------------------------------------------------------

// Overview is the internet-state summary shown on the landing page.
type Overview struct {
	Counts      OverviewCounts `json:"counts"`
	GlobalLoss  GlobalLoss     `json:"global_loss"`
	TopSrcAS    []ASNIssue     `json:"top_src_as"`
	TopDstAS    []ASNIssue     `json:"top_dst_as"`
	TopASPairs  []ASNPairIssue `json:"top_as_pairs"`
	ActiveAlert int            `json:"active_alerts"` // filled by API layer
}

type OverviewCounts struct {
	Probes       int64 `json:"probes"`
	IPs          int64 `json:"ips"`
	ASes         int64 `json:"ases"`
	Targets      int64 `json:"targets"`
	NextHopEdges int64 `json:"next_hop_edges"`
	PingEdges    int64 `json:"ping_edges"`
	TransitEdges int64 `json:"transit_edges"`
	LossyPings   int64 `json:"lossy_pings"`
}

// GlobalLoss aggregates across all current PING edges with sent>0.
type GlobalLoss struct {
	HealthyEdges int64   `json:"healthy_edges"` // loss_ratio == 0
	LossyEdges   int64   `json:"lossy_edges"`   // loss_ratio > 0
	LostEdges    int64   `json:"lost_edges"`    // loss_ratio >= 1 (target lost)
	AvgLossPct   float64 `json:"avg_loss_pct"`  // over all edges with sent>0
}

// ASNIssue ranks a single AS by aggregate loss it is involved in, as either
// source or destination of lossy PING edges. Fields are derived from current
// PING-edge aggregates: Samples = count of lossy PING edges; LastSeen = the
// most recent last_seen among those edges (unix seconds).
type ASNIssue struct {
	ASN        int64   `json:"asn"`
	Org        string  `json:"org"`
	Role       string  `json:"role"` // "src" | "dst"
	Samples    int64   `json:"samples"`
	Probes     int64   `json:"probes"`
	AvgLossPct float64 `json:"avg_loss_pct"`
	MaxLossPct float64 `json:"max_loss_pct"`
	AvgRttMs   float64 `json:"avg_rtt_ms"`
	LastSeen   int64   `json:"last_seen"`
}

// ASNPairIssue ranks a (src AS → dst AS) pair by aggregate loss.
type ASNPairIssue struct {
	SrcASN     int64   `json:"src_asn"`
	SrcOrg     string  `json:"src_org"`
	DstASN     int64   `json:"dst_asn"`
	DstOrg     string  `json:"dst_org"`
	Samples    int64   `json:"samples"`
	Probes     int64   `json:"probes"`
	AvgLossPct float64 `json:"avg_loss_pct"`
	MaxLossPct float64 `json:"max_loss_pct"`
	AvgRttMs   float64 `json:"avg_rtt_ms"`
	LastSeen   int64   `json:"last_seen"`
}

// ASNDetail is the per-AS overview for the drill-down page.
type ASNDetail struct {
	ASN         int64   `json:"asn"`
	Org         string  `json:"org"`
	ProbeCount  int64   `json:"probe_count"`
	IPCount     int64   `json:"ip_count"`
	TargetCount int64   `json:"target_count"`
	TransitIn   int64   `json:"transit_in"`
	TransitOut  int64   `json:"transit_out"`
	AvgLossPct  float64 `json:"avg_loss_pct"`
	LossyEdges  int64   `json:"lossy_edges"`
}

type ProbeInfo struct {
	ID       int64  `json:"id"`
	SrcIP    string `json:"src_ip"`
	SrcASN   *int64 `json:"src_asn,omitempty"`
	SrcOrg   string `json:"src_org,omitempty"`
	AvgRttMs float64 `json:"avg_rtt_ms,omitempty"`
	LossPct  float64 `json:"loss_pct,omitempty"`
	LastSeen int64  `json:"last_seen"`
}

type TargetInfo struct {
	Addr     string  `json:"addr"`
	ASN      *int64  `json:"asn,omitempty"`
	Org      string  `json:"org,omitempty"`
	Probes   int64   `json:"probes"`
	AvgRttMs float64 `json:"avg_rtt_ms,omitempty"`
	LossPct  float64 `json:"loss_pct,omitempty"`
	LastSeen int64   `json:"last_seen"`
}

// ASNTransitEdge is one AS→AS transits link.
type ASNTransitEdge struct {
	SrcASN    int64  `json:"src_asn"`
	SrcOrg    string `json:"src_org"`
	DstASN    int64  `json:"dst_asn"`
	DstOrg    string `json:"dst_org"`
	SeenCount int64  `json:"seen_count"`
	LastSeen  int64  `json:"last_seen"`
}

type ProbeDetail struct {
	ID         int64        `json:"id"`
	SrcIP      string       `json:"src_ip"`
	SrcASN     *int64       `json:"src_asn,omitempty"`
	SrcOrg     string       `json:"src_org,omitempty"`
	Targets    []TargetInfo `json:"targets"`
	LastSeen   int64        `json:"last_seen"`
}

type TargetDetail struct {
	Addr      string       `json:"addr"`
	ASN       *int64       `json:"asn,omitempty"`
	Org       string       `json:"org,omitempty"`
	Probes    []ProbeInfo  `json:"probes"`
	NearbyHops []HotHop    `json:"nearby_hops"`
	LastSeen  int64        `json:"last_seen"`
}

// HotHop is a NEXT_HOP edge flagged as a potential transit hotspot.
type HotHop struct {
	FromAddr  string `json:"from_addr"`
	ToAddr    string `json:"to_addr"`
	FromASN   *int64 `json:"from_asn,omitempty"`
	FromOrg   string `json:"from_org,omitempty"`
	ToASN     *int64 `json:"to_asn,omitempty"`
	ToOrg     string `json:"to_org,omitempty"`
	LastRttMs float64 `json:"last_rtt_ms"`
	SeenCount int64   `json:"seen_count"`
	LastSeen  int64   `json:"last_seen"`
}

type TransitPairDetail struct {
	SrcASN    int64   `json:"src_asn"`
	SrcOrg    string  `json:"src_org"`
	DstASN    int64   `json:"dst_asn"`
	DstOrg    string  `json:"dst_org"`
	SeenCount int64   `json:"seen_count"`
	LastSeen  int64   `json:"last_seen"`
	Hops      []HotHop `json:"hops"`
}

// IPDetail describes a single :IP node and its incident NEXT_HOP edges.
type IPDetail struct {
	Addr     string   `json:"addr"`
	ASN      *int64   `json:"asn,omitempty"`
	Org      string   `json:"org,omitempty"`
	AF       int64    `json:"af"`
	LastSeen int64    `json:"last_seen"`
	InHops   []HotHop `json:"in_hops"`
	OutHops  []HotHop `json:"out_hops"`
}

// Path is a traceroute-style node/edge sequence between two IPs.
type Path struct {
	Found bool      `json:"found"`
	Hops  []PathHop `json:"hops"`
}

// ReachableGroup is one destination AS (or "Direct targets" for un-AS'd IPs)
// reachable from a given source, with a sample of concrete endpoint IPs.
type ReachableGroup struct {
	ASN      *int64  `json:"asn,omitempty"`
	Org      string  `json:"org"`
	IPCount  int64   `json:"ip_count"`
	SampleIP []string `json:"sample_ips"`
	Basis    string  `json:"basis"` // "probe" | "transit"
}

// SearchResult is one autocomplete hit. Kind is "as" | "ip" | "probe". For AS
// hits, Addrs holds several concrete IPs in that AS so the caller can pick one
// as a path endpoint (Addr is the first of these for back-compat). For ip/probe
// hits, Addr is the single resolved address.
type SearchResult struct {
	Kind  string   `json:"kind"`            // as | ip | probe
	Label string   `json:"label"`           // primary display text
	Addr  string   `json:"addr,omitempty"`  // concrete IP for path src/dst
	Addrs []string `json:"addrs,omitempty"` // for AS hits: several IPs in the AS
	ASN   *int64   `json:"asn,omitempty"`
	Org   string   `json:"org,omitempty"`
	Sub   string   `json:"sub,omitempty"`   // secondary text (e.g. "AS13335 · 147 IPs")
}

type PathHop struct {
	Addr     string  `json:"addr"`
	ASN      *int64  `json:"asn,omitempty"`
	Org      string  `json:"org,omitempty"`
	RttMs    float64 `json:"rtt_ms"`
	SeenCount int64  `json:"seen_count"`
}

// Subgraph is the nodes+edges payload for the topology force-directed view.
type Subgraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type GraphNode struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Kind  string  `json:"kind"` // ip | probe | as
	ASN   *int64  `json:"asn,omitempty"`
	Org   string  `json:"org,omitempty"`
}

type GraphEdge struct {
	From      string  `json:"from"`
	To        string  `json:"to"`
	Kind      string  `json:"kind"` // next_hop | ping | transits | targets | located_at | in_as
	LastRttMs float64 `json:"last_rtt_ms,omitempty"`
	LossRatio float64 `json:"loss_ratio,omitempty"`
	SeenCount int64   `json:"seen_count,omitempty"`
}

// ---- Filters ----------------------------------------------------------------

type ASNIssueFilter struct {
	Role      string // "src" | "dst"
	MinLoss   float64
	MinProbes int64
	Limit     int
	Sort      string // loss | probes | samples | last_seen | impact
	Order     string // desc | asc
}

type ASNPairFilter struct {
	MinLoss   float64
	MinProbes int64
	Limit     int
	Sort      string
	Order     string
}

type ProbeFilter struct {
	ASN       int64 // source ASN, 0 = any
	Limit     int
	MinProbes int64
	Sort      string // loss | rtt | last_seen
	Order     string
}

type TargetFilter struct {
	ASN       int64 // destination ASN, 0 = any
	Limit     int
	MinProbes int64
	Sort      string // loss | rtt | probes | last_seen
	Order     string
}

type HopFilter struct {
	MinRtt float64
	Limit  int
}

type TransitFilter struct {
	Limit int
}

type SubgraphFilter struct {
	ASN    int64
	Probe  int64
	Target string
	Depth  int
	Limit  int
}

// ---- Filters: clamping ------------------------------------------------------

func clampLimit(n, def, max int) int {
	if n <= 0 {
		n = def
	}
	if n > max {
		n = max
	}
	return n
}
