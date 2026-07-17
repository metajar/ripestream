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
	"math"
	"strings"
	"time"

	"github.com/FalkorDB/falkordb-go/v2"
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
	HopContexts(ctx context.Context, addrs []string) (map[string]HopContext, error)
	TransitEdges(ctx context.Context, f TransitFilter) ([]ASNTransitEdge, error)
	TransitPairDetail(ctx context.Context, asnA, asnB int64) (TransitPairDetail, error)
	TransitPairTests(ctx context.Context, asnA, asnB int64, limit int) ([]TransitTest, error)
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
	Metric     string // loss_ratio | avg_rtt_ms | target_lost
	Scope      string // probe_target | asn_pair | target | asn_dst
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

// readQueryTimeoutMS is deliberately above FalkorDB's one-second default.
// Aggregate read queries over the live graph (notably the ASN worklist) need
// a few seconds on a populated graph, but remain bounded so a UI request never
// runs indefinitely.
const readQueryTimeoutMS = 5_000

// rows runs a parameterized, read-only Cypher query and yields one row at a
// time as a column-name → value map. Values are the Go scalars FalkorDB's
// client decodes (string, int64, float64, bool, nil, plus Node/Edge/Path for
// graph projections).
func (s *Store) rows(ctx context.Context, cypher string, params map[string]any) ([]map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	options := falkordb.NewQueryOptions().SetTimeout(readQueryTimeoutMS)
	res, err := s.graph.ROQuery(cypher, params, options)
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
	var value float64
	switch t := v.(type) {
	case float64:
		value = t
	case int64:
		value = float64(t)
	case int:
		value = float64(t)
	default:
		return 0
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
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

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
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

// ASNIssue ranks a single AS by packet loss across its current PING edges.
// AvgLossPct is packet-weighted: sum(sent - rcvd) / sum(sent), rather than an
// average limited to already-lossy edges. Samples is the count of current
// PING edges included in that aggregate; LastSeen is the most recent edge
// timestamp (unix seconds).
type ASNIssue struct {
	ASN        int64   `json:"asn"`
	Org        string  `json:"org"`
	Role       string  `json:"role"` // "src" | "dst"
	Samples    int64   `json:"samples"`
	Probes     int64   `json:"probes"`
	SourceASes int64   `json:"source_ases"`
	Targets    int64   `json:"targets"`
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
	ID       int64          `json:"id"`
	SrcIP    string         `json:"src_ip"`
	SrcASN   *int64         `json:"src_asn,omitempty"`
	SrcOrg   string         `json:"src_org,omitempty"`
	Metadata *ProbeMetadata `json:"metadata,omitempty"`
	AvgRttMs float64        `json:"avg_rtt_ms,omitempty"`
	LossPct  float64        `json:"loss_pct,omitempty"`
	LastSeen int64          `json:"last_seen"`
}

// ProbeMetadata is the cached public RIPE Atlas inventory attached to a probe
// graph node. Latitude/longitude are privacy-obfuscated by RIPE Atlas.
type ProbeMetadata struct {
	DisplayName     string   `json:"display_name"`
	Description     string   `json:"description,omitempty"`
	ProbeType       string   `json:"probe_type"`
	CountryCode     string   `json:"country_code,omitempty"`
	Latitude        float64  `json:"latitude,omitempty"`
	Longitude       float64  `json:"longitude,omitempty"`
	IsAnchor        bool     `json:"is_anchor"`
	IsPublic        bool     `json:"is_public"`
	FirmwareVersion int64    `json:"firmware_version,omitempty"`
	StatusID        int64    `json:"status_id"`
	StatusName      string   `json:"status_name,omitempty"`
	StatusSince     int64    `json:"status_since,omitempty"`
	FirstConnected  int64    `json:"first_connected,omitempty"`
	LastConnected   int64    `json:"last_connected,omitempty"`
	PrefixV4        string   `json:"prefix_v4,omitempty"`
	PrefixV6        string   `json:"prefix_v6,omitempty"`
	ASNv4           int64    `json:"asn_v4,omitempty"`
	ASNv6           int64    `json:"asn_v6,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	UpdatedAt       int64    `json:"updated_at"`
}

func probeMetadataFromRow(r map[string]any) *ProbeMetadata {
	updatedAt := asInt(r["metadata_updated_at"])
	if updatedAt <= 0 {
		return nil
	}
	var tags []string
	if raw := asString(r["tag_slugs"]); raw != "" {
		for _, tag := range strings.Split(raw, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	return &ProbeMetadata{
		DisplayName: asString(r["display_name"]), Description: asString(r["description"]),
		ProbeType: asString(r["probe_type"]), CountryCode: asString(r["country_code"]),
		Latitude: asFloat(r["latitude"]), Longitude: asFloat(r["longitude"]),
		IsAnchor: asBool(r["is_anchor"]), IsPublic: asBool(r["is_public"]),
		FirmwareVersion: asInt(r["firmware_version"]), StatusID: asInt(r["status_id"]),
		StatusName: asString(r["status_name"]), StatusSince: asInt(r["status_since"]),
		FirstConnected: asInt(r["first_connected"]), LastConnected: asInt(r["last_connected"]),
		PrefixV4: asString(r["prefix_v4"]), PrefixV6: asString(r["prefix_v6"]),
		ASNv4: asInt(r["asn_v4"]), ASNv6: asInt(r["asn_v6"]), Tags: tags, UpdatedAt: updatedAt,
	}
}

func probeInfoFromRow(r map[string]any) ProbeInfo {
	return ProbeInfo{
		ID: asInt(r["id"]), SrcIP: asString(r["src_ip"]),
		SrcASN: ptrIf(asInt(r["asn"]), int64(0)), SrcOrg: asString(r["org"]),
		Metadata: probeMetadataFromRow(r), LossPct: asFloat(r["loss_pct"]),
		AvgRttMs: asFloat(r["rtt"]), LastSeen: asInt(r["last_seen"]),
	}
}

type TargetInfo struct {
	Addr       string  `json:"addr"`
	ASN        *int64  `json:"asn,omitempty"`
	Org        string  `json:"org,omitempty"`
	Probes     int64   `json:"probes"`
	SourceASes int64   `json:"source_ases,omitempty"`
	AvgRttMs   float64 `json:"avg_rtt_ms,omitempty"`
	LossPct    float64 `json:"loss_pct,omitempty"`
	LastSeen   int64   `json:"last_seen"`
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

// HopContext supplies live graph identity and topology breadth for a hop found
// by historical route correlation.
type HopContext struct {
	Addr     string `json:"addr"`
	ASN      *int64 `json:"asn,omitempty"`
	Org      string `json:"org,omitempty"`
	Incoming int64  `json:"incoming"`
	Outgoing int64  `json:"outgoing"`
}

type ProbeDetail struct {
	ID       int64          `json:"id"`
	SrcIP    string         `json:"src_ip"`
	SrcASN   *int64         `json:"src_asn,omitempty"`
	SrcOrg   string         `json:"src_org,omitempty"`
	Metadata *ProbeMetadata `json:"metadata,omitempty"`
	Targets  []TargetInfo   `json:"targets"`
	LastSeen int64          `json:"last_seen"`
}

type TargetDetail struct {
	Addr       string      `json:"addr"`
	ASN        *int64      `json:"asn,omitempty"`
	Org        string      `json:"org,omitempty"`
	Probes     []ProbeInfo `json:"probes"`
	NearbyHops []HotHop    `json:"nearby_hops"`
	LastSeen   int64       `json:"last_seen"`
}

// HotHop is a NEXT_HOP edge flagged as a potential transit hotspot.
type HotHop struct {
	FromAddr  string  `json:"from_addr"`
	ToAddr    string  `json:"to_addr"`
	FromASN   *int64  `json:"from_asn,omitempty"`
	FromOrg   string  `json:"from_org,omitempty"`
	ToASN     *int64  `json:"to_asn,omitempty"`
	ToOrg     string  `json:"to_org,omitempty"`
	LastRttMs float64 `json:"last_rtt_ms"`
	SeenCount int64   `json:"seen_count"`
	LastSeen  int64   `json:"last_seen"`
}

type TransitPairDetail struct {
	SrcASN    int64         `json:"src_asn"`
	SrcOrg    string        `json:"src_org"`
	DstASN    int64         `json:"dst_asn"`
	DstOrg    string        `json:"dst_org"`
	SeenCount int64         `json:"seen_count"`
	LastSeen  int64         `json:"last_seen"`
	Hops      []HotHop      `json:"hops"`
	Tests     []TransitTest `json:"tests"`
}

// TransitTest is the latest PING observation from a probe in the source AS to
// a target in the destination AS. It is endpoint evidence for the AS pair, not
// proof that the ping traversed the specific TRANSITS edge.
type TransitTest struct {
	ProbeID       int64          `json:"probe_id"`
	ProbeMetadata *ProbeMetadata `json:"probe_metadata,omitempty"`
	MsmID         int64          `json:"msm_id"`
	SourceIP      string         `json:"source_ip"`
	TargetIP      string         `json:"target_ip"`
	Sent          int64          `json:"sent"`
	Received      int64          `json:"received"`
	LossPct       float64        `json:"loss_pct"`
	AvgRttMs      float64        `json:"avg_rtt_ms"`
	MinRttMs      float64        `json:"min_rtt_ms"`
	MaxRttMs      float64        `json:"max_rtt_ms"`
	LastSeen      int64          `json:"last_seen"`
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
	ASN      *int64   `json:"asn,omitempty"`
	Org      string   `json:"org"`
	IPCount  int64    `json:"ip_count"`
	SampleIP []string `json:"sample_ips"`
	Basis    string   `json:"basis"` // "probe" | "transit"
}

// SearchResult is one autocomplete hit. Kind is "as" | "ip" | "probe". For AS
// hits, Addrs holds several concrete IPs in that AS so the caller can pick one
// as a path endpoint (Addr is the first of these for back-compat). For ip/probe
// hits, Addr is the single resolved address.
type SearchResult struct {
	Kind    string   `json:"kind"`            // as | ip | probe
	Label   string   `json:"label"`           // primary display text
	Addr    string   `json:"addr,omitempty"`  // concrete IP for path src/dst
	Addrs   []string `json:"addrs,omitempty"` // for AS hits: several IPs in the AS
	ASN     *int64   `json:"asn,omitempty"`
	ProbeID *int64   `json:"probe_id,omitempty"`
	Org     string   `json:"org,omitempty"`
	Sub     string   `json:"sub,omitempty"` // secondary text (e.g. "AS13335 · 147 IPs")
}

type PathHop struct {
	Addr      string  `json:"addr"`
	ASN       *int64  `json:"asn,omitempty"`
	Org       string  `json:"org,omitempty"`
	RttMs     float64 `json:"rtt_ms"`
	SeenCount int64   `json:"seen_count"`
}

// Subgraph is the nodes+edges payload for the topology force-directed view.
type Subgraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type GraphNode struct {
	ID            string         `json:"id"`
	Label         string         `json:"label"`
	Kind          string         `json:"kind"` // ip | probe | as
	ASN           *int64         `json:"asn,omitempty"`
	Org           string         `json:"org,omitempty"`
	ProbeMetadata *ProbeMetadata `json:"probe_metadata,omitempty"`
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
	Role          string // "src" | "dst"
	MinLoss       float64
	MinProbes     int64
	MinSourceASes int64
	Limit         int
	Offset        int
	Query         string // case-insensitive match across every returned field
	Sort          string // loss | probes | samples | last_seen | impact
	Order         string // desc | asc
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
	Country   string
	Type      string // anchor | software | hardware
	Status    string // RIPE Atlas status name
	Limit     int
	Offset    int
	Query     string // case-insensitive match across every returned field
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
	Offset int
	Query  string // case-insensitive match across every returned field
	Sort   string // rtt | observations | last_seen
	Order  string
}

type TransitFilter struct {
	Limit  int
	Offset int
	Query  string // case-insensitive match across every returned field
	Sort   string // observations | last_seen
	Order  string
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
