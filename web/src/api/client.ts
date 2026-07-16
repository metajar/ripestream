// Typed API client for the ripestream backend. Types mirror the Go structs in
// internal/graph (query.go) and internal/api (views.go).

const BASE = import.meta.env.VITE_API_BASE ?? "";

// ---- envelope ---------------------------------------------------------------
type Envelope<T, M = unknown> = { data?: T; error?: string; meta?: M };

async function get<T>(path: string): Promise<T> {
  return (await getWithMeta<T>(path)).data;
}

// getWithMeta returns both data and the response meta block. Use for endpoints
// that carry meaningful metadata (e.g. /api/overview cache freshness).
async function getWithMeta<T, M = unknown>(path: string): Promise<{ data: T; meta?: M }> {
  const res = await fetch(`${BASE}${path}`);
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try {
      const body = (await res.json()) as Envelope<never>;
      if (body.error) msg = body.error;
    } catch {
      /* keep default */
    }
    throw new Error(msg);
  }
  const body = (await res.json()) as Envelope<T, M>;
  if (body.error) throw new Error(body.error);
  return { data: body.data as T, meta: body.meta };
}

// OverviewMeta mirrors internal/api overviewMeta: cache freshness for the
// overview endpoint. last_ok is a boolean (last refresh succeeded), NOT a
// timestamp — do not label it "last refreshed at".
export interface OverviewMeta {
  cached: boolean;
  stale: boolean;
  took_ms: number;
  ttl_sec: number;
  last_ok: boolean;
}

// ---- types ------------------------------------------------------------------

export interface OverviewCounts {
  probes: number;
  ips: number;
  ases: number;
  targets: number;
  next_hop_edges: number;
  ping_edges: number;
  transit_edges: number;
  lossy_pings: number;
}
export interface GlobalLoss {
  healthy_edges: number;
  lossy_edges: number;
  lost_edges: number;
  avg_loss_pct: number;
}
export interface ASNIssue {
  asn: number;
  org: string;
  role: "src" | "dst";
  samples: number;
  probes: number;
  avg_loss_pct: number;
  max_loss_pct: number;
  avg_rtt_ms: number;
  // Unix-seconds epoch of the most recent PING edge in this aggregate.
  last_seen: number;
}
export interface ASNPairIssue {
  src_asn: number;
  src_org: string;
  dst_asn: number;
  dst_org: string;
  samples: number;
  probes: number;
  avg_loss_pct: number;
  max_loss_pct: number;
  avg_rtt_ms: number;
  // Unix-seconds epoch of the most recent PING edge in this pair aggregate.
  last_seen: number;
}
export interface Overview {
  counts: OverviewCounts;
  global_loss: GlobalLoss;
  top_src_as: ASNIssue[];
  top_dst_as: ASNIssue[];
  top_as_pairs: ASNPairIssue[];
  active_alerts: number;
}

export interface ASNDetail {
  asn: number;
  org: string;
  probe_count: number;
  ip_count: number;
  target_count: number;
  transit_in: number;
  transit_out: number;
  avg_loss_pct: number;
  lossy_edges: number;
}

// Issue mirrors internal/api.Issue: the unified evidence-backed anomaly schema.
export interface Issue {
  id: string;
  kind: string;
  severity: "critical" | "high" | "watch";
  title: string;
  summary: string;
  loss_pct: number;
  avg_rtt_ms?: number;
  probe_count: number;
  target_count?: number;
  sample_count: number;
  last_seen: number; // unix seconds
  confidence: "high" | "medium" | "low";
  href: string;
  evidence: string[];
  source: "detected" | "alert";
}

export interface ProbeInfo {
  id: number;
  src_ip: string;
  src_asn?: number;
  src_org?: string;
  avg_rtt_ms?: number;
  loss_pct?: number;
  last_seen: number;
}

export interface TargetInfo {
  addr: string;
  asn?: number;
  org?: string;
  probes: number;
  avg_rtt_ms?: number;
  loss_pct?: number;
  last_seen: number;
}

export interface ASNTransitEdge {
  src_asn: number;
  src_org: string;
  dst_asn: number;
  dst_org: string;
  seen_count: number;
  last_seen: number;
}

export interface ProbeDetail {
  id: number;
  src_ip: string;
  src_asn?: number;
  src_org?: string;
  targets: TargetInfo[];
  last_seen: number;
}

export interface HotHop {
  from_addr: string;
  to_addr: string;
  from_asn?: number;
  from_org?: string;
  to_asn?: number;
  to_org?: string;
  last_rtt_ms: number;
  seen_count: number;
  last_seen: number;
}

export interface TargetDetail {
  addr: string;
  asn?: number;
  org?: string;
  probes: ProbeInfo[];
  nearby_hops: HotHop[];
  last_seen: number;
}

export interface TransitPairDetail {
  src_asn: number;
  src_org: string;
  dst_asn: number;
  dst_org: string;
  seen_count: number;
  last_seen: number;
  hops: HotHop[];
}

export interface IPDetail {
  addr: string;
  asn?: number;
  org?: string;
  af: number;
  last_seen: number;
  in_hops: HotHop[];
  out_hops: HotHop[];
}

export interface PathHop {
  addr: string;
  asn?: number;
  org?: string;
  rtt_ms: number;
  seen_count: number;
}
export interface GraphPath {
  found: boolean;
  hops: PathHop[];
}

export interface ReachableGroup {
  asn?: number;
  org: string;
  ip_count: number;
  sample_ips: string[];
  basis: "probe" | "transit";
}

export interface SearchResult {
  kind: "as" | "ip" | "probe";
  label: string;
  addr?: string;
  addrs?: string[];
  asn?: number;
  org?: string;
  sub?: string;
}

export interface GraphNode {
  id: string;
  label: string;
  kind: "ip" | "probe" | "as";
  asn?: number;
  org?: string;
}
export interface GraphEdge {
  from: string;
  to: string;
  kind: string;
  last_rtt_ms?: number;
  loss_ratio?: number;
  seen_count?: number;
}
export interface Subgraph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface SeriesPoint {
  ts: string;
  value: number;
  samples: number;
}

// ---- alert types (used once Phase 4 wires the engine) -----------------------
export interface AlertRuleView {
  id: number;
  name: string;
  metric: string;
  comparison: string;
  threshold: number;
  scope: string;
  filters?: Record<string, unknown>;
  window_min: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

// ---- endpoint functions -----------------------------------------------------

export const api = {
  overview: () => getWithMeta<Overview, OverviewMeta>("/api/overview"),
  asnIssues: (role: "src" | "dst", limit = 20, sort = "impact", order = "desc", minProbes = 2) =>
    get<ASNIssue[]>(
      `/api/asn/issues?role=${role}&limit=${limit}&sort=${sort}&order=${order}&min_probes=${minProbes}`,
    ),
  asnDetail: (asn: number) => get<ASNDetail>(`/api/asn/${asn}`),
  asnProbes: (asn: number, limit = 50) =>
    get<ProbeInfo[]>(`/api/asn/${asn}/probes?limit=${limit}`),
  asnTargets: (asn: number, limit = 50) =>
    get<TargetInfo[]>(`/api/asn/${asn}/targets?limit=${limit}`),
  asnTransit: (asn: number, limit = 50) =>
    get<ASNTransitEdge[]>(`/api/asn/${asn}/transit?limit=${limit}`),
  probes: (limit = 50) => get<ProbeInfo[]>(`/api/probes?limit=${limit}`),
  probeDetail: (id: number) => get<ProbeDetail>(`/api/probe/${id}`),
  targets: (limit = 50) => get<TargetInfo[]>(`/api/targets?limit=${limit}`),
  targetDetail: (addr: string) =>
    get<TargetDetail>(`/api/target/${encodeURIComponent(addr)}`),
  ipDetail: (addr: string) => get<IPDetail>(`/api/ip/${encodeURIComponent(addr)}`),
  hotHops: (limit = 50, minRtt = 100) =>
    get<HotHop[]>(`/api/hops/hotspots?limit=${limit}&min_rtt=${minRtt}`),
  transitEdges: (limit = 50) =>
    get<ASNTransitEdge[]>(`/api/transit?limit=${limit}`),
  transitPairDetail: (a: number, b: number) =>
    get<TransitPairDetail>(`/api/transit/${a}/${b}`),
  path: (src: string, dst: string, maxHops = 15) =>
    get<GraphPath>(
      `/api/path?src=${encodeURIComponent(src)}&dst=${encodeURIComponent(dst)}&max_hops=${maxHops}`,
    ),
  subgraph: (params: {
    asn?: number;
    probe?: number;
    target?: string;
    depth?: number;
    limit?: number;
  }) => {
    const q = new URLSearchParams();
    if (params.asn) q.set("asn", String(params.asn));
    if (params.probe) q.set("probe", String(params.probe));
    if (params.target) q.set("target", params.target);
    q.set("depth", String(params.depth ?? 2));
    q.set("limit", String(params.limit ?? 80));
    return get<Subgraph>(`/api/graph/subgraph?${q}`);
  },
  timeseries: (params: {
    metric?: string;
    probe?: number;
    target?: string;
    from?: number;
    to?: number;
    buckets?: number;
  }) => {
    const q = new URLSearchParams();
    if (params.metric) q.set("metric", params.metric);
    if (params.probe) q.set("probe", String(params.probe));
    if (params.target) q.set("target", params.target);
    if (params.from) q.set("from", String(params.from));
    if (params.to) q.set("to", String(params.to));
    q.set("buckets", String(params.buckets ?? 60));
    return get<SeriesPoint[]>(`/api/timeseries?${q}`);
  },
  alertsRules: () => get<AlertRuleView[]>("/api/alerts/rules"),
  search: (q: string, limit = 12) =>
    get<SearchResult[]>(`/api/search?q=${encodeURIComponent(q)}&limit=${limit}`),
  issues: (limit = 10, severity?: string) => {
    const q = new URLSearchParams();
    q.set("limit", String(limit));
    if (severity) q.set("severity", severity);
    return get<Issue[]>(`/api/issues?${q}`);
  },
  reachableDestinations: (src: string) =>
    get<ReachableGroup[]>(`/api/path/destinations?src=${encodeURIComponent(src)}`),
};
