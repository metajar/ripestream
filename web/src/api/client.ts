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
  const raw = await res.text();
  if (!raw.trim()) {
    throw new Error(`API returned an empty response (HTTP ${res.status})`);
  }

  let body: Envelope<T, M>;
  try {
    body = JSON.parse(raw) as Envelope<T, M>;
  } catch {
    throw new Error(`API returned invalid JSON (HTTP ${res.status})`);
  }

  if (!res.ok) {
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  if (body.error) throw new Error(body.error);
  if (body.data === undefined) throw new Error("API response did not include data");
  return { data: body.data as T, meta: body.meta };
}

export interface PageMeta {
  limit: number;
  offset: number;
  has_more: boolean;
}

export interface Page<T> {
  data: T[];
  meta: PageMeta;
}

async function getPage<T>(path: string): Promise<Page<T>> {
  const response = await getWithMeta<T[], PageMeta>(path);
  return {
    data: response.data,
    meta: response.meta ?? { limit: response.data.length, offset: 0, has_more: false },
  };
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
  source_ases: number;
  targets: number;
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
  metadata?: ProbeMetadata;
  avg_rtt_ms?: number;
  loss_pct?: number;
  last_seen: number;
}

export interface ProbeMetadata {
  display_name: string;
  description?: string;
  probe_type: string;
  country_code?: string;
  latitude?: number;
  longitude?: number;
  is_anchor: boolean;
  is_public: boolean;
  firmware_version?: number;
  status_id: number;
  status_name?: string;
  status_since?: number;
  first_connected?: number;
  last_connected?: number;
  prefix_v4?: string;
  prefix_v6?: string;
  asn_v4?: number;
  asn_v6?: number;
  tags?: string[];
  updated_at: number;
}

export interface TargetInfo {
  addr: string;
  asn?: number;
  org?: string;
  probes: number;
  source_ases?: number;
  avg_rtt_ms?: number;
  loss_pct?: number;
  baseline_loss_pct?: number;
  loss_change_pct?: number;
  baseline_samples?: number;
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
  metadata?: ProbeMetadata;
  target_count: number;
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

export interface HopCommonality {
  addr: string;
  asn?: number;
  org?: string;
  incoming: number;
  outgoing: number;
  recent_median_rtt_ms: number;
  baseline_median_rtt_ms: number;
  rtt_delta_ms: number;
  change_pct: number;
  impact_score: number;
  recent_samples: number;
  baseline_samples: number;
  probes: number;
  targets: number;
  traces: number;
  affected_probes: number[];
  affected_targets: string[];
  first_seen: number;
  last_seen: number;
}

export interface HopCommonalityResponse {
  generated_at: number;
  recent_minutes: number;
  baseline_hours: number;
  candidates: HopCommonality[];
}

export interface TargetDetail {
  addr: string;
  asn?: number;
  org?: string;
  probe_count: number;
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
  tests: TransitTest[];
}

export interface TransitTest {
  probe_id: number;
  probe_metadata?: ProbeMetadata;
  msm_id: number;
  source_ip: string;
  target_ip: string;
  sent: number;
  received: number;
  loss_pct: number;
  avg_rtt_ms: number;
  min_rtt_ms: number;
  max_rtt_ms: number;
  last_seen: number;
}

export interface TransitPairSeriesPoint {
  ts: string;
  loss_pct: number;
  avg_rtt_ms: number;
  samples: number;
  probes: number;
  targets: number;
}

export interface IPDetail {
  addr: string;
  asn?: number;
  org?: string;
  af: number;
  last_seen: number;
  incoming_hops: number;
  outgoing_hops: number;
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
  probe_id?: number;
  org?: string;
  sub?: string;
}

export interface GraphNode {
  id: string;
  label: string;
  kind: "ip" | "probe" | "as";
  asn?: number;
  org?: string;
  probe_metadata?: ProbeMetadata;
  traversal_role?: "seed" | "upstream" | "downstream" | "both";
  depth?: number;
  last_seen?: number;
}
export interface GraphEdge {
  from: string;
  to: string;
  kind: string;
  last_rtt_ms?: number;
  loss_ratio?: number;
  seen_count?: number;
  last_seen?: number;
}
export interface Subgraph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface IPRouteGraph {
  seed: string;
  nodes: GraphNode[];
  edges: GraphEdge[];
  complete: boolean;
  max_depth: number;
  node_limit: number;
  truncation_reason?: string;
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
  asnIssues: (role: "src" | "dst", limit = 25, offset = 0, sort = "impact", order = "desc", minProbes = 2, query = "") => {
    const q = new URLSearchParams({
      role, limit: String(limit), offset: String(offset), sort, order, min_probes: String(minProbes),
    });
    if (query) q.set("q", query);
    return getPage<ASNIssue>(`/api/asn/issues?${q}`);
  },
  asnDetail: (asn: number) => get<ASNDetail>(`/api/asn/${asn}`),
  asnProbes: (asn: number, limit = 50) =>
    get<ProbeInfo[]>(`/api/asn/${asn}/probes?limit=${limit}`),
  asnTargets: (asn: number, limit = 50) =>
    get<TargetInfo[]>(`/api/asn/${asn}/targets?limit=${limit}`),
  asnTransit: (asn: number, limit = 50) =>
    get<ASNTransitEdge[]>(`/api/asn/${asn}/transit?limit=${limit}`),
  probes: (limit = 25, offset = 0, filters: { country?: string; type?: string; status?: string; query?: string } = {}) => {
    const q = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    if (filters.country) q.set("country", filters.country);
    if (filters.type) q.set("type", filters.type);
    if (filters.status) q.set("status", filters.status);
    if (filters.query) q.set("q", filters.query);
    return getPage<ProbeInfo>(`/api/probes?${q}`);
  },
  probeDetail: (id: number) => get<ProbeDetail>(`/api/probe/${id}`),
  probeTargets: (id: number, limit = 25, offset = 0, query = "", sort = "loss", order = "desc") => {
    const q = new URLSearchParams({ limit: String(limit), offset: String(offset), sort, order });
    if (query) q.set("q", query);
    return getPage<TargetInfo>(`/api/probe/${id}/targets?${q}`);
  },
  targets: (limit = 50) => get<TargetInfo[]>(`/api/targets?limit=${limit}`),
  targetDetail: (addr: string) =>
    get<TargetDetail>(`/api/target/${encodeURIComponent(addr)}`),
  targetProbes: (addr: string, limit = 25, offset = 0, query = "", sort = "loss", order = "desc") => {
    const q = new URLSearchParams({ limit: String(limit), offset: String(offset), sort, order });
    if (query) q.set("q", query);
    return getPage<ProbeInfo>(`/api/target/${encodeURIComponent(addr)}/probes?${q}`);
  },
  ipDetail: (addr: string) => get<IPDetail>(`/api/ip/${encodeURIComponent(addr)}`),
  ipRouteGraph: (addr: string, maxDepth = 15, nodeLimit = 400) =>
    get<IPRouteGraph>(
      `/api/ip/${encodeURIComponent(addr)}/graph?max_depth=${maxDepth}&node_limit=${nodeLimit}`,
    ),
  ipHops: (addr: string, direction: "in" | "out", limit = 25, offset = 0, query = "", sort = "rtt", order = "desc") => {
    const q = new URLSearchParams({
      direction, limit: String(limit), offset: String(offset), sort, order,
    });
    if (query) q.set("q", query);
    return getPage<HotHop>(`/api/ip/${encodeURIComponent(addr)}/hops?${q}`);
  },
  hotHops: (limit = 25, offset = 0, minRtt = 100, query = "", sort = "rtt", order = "desc") => {
    const q = new URLSearchParams({
      limit: String(limit), offset: String(offset), min_rtt: String(minRtt), sort, order,
    });
    if (query) q.set("q", query);
    return getPage<HotHop>(`/api/hops/hotspots?${q}`);
  },
  hopCommonalities: (range = "30m", minProbes = 3, limit = 40) =>
    get<HopCommonalityResponse>(
      `/api/hops/commonality?range=${encodeURIComponent(range)}&min_probes=${minProbes}&limit=${limit}`,
    ),
  transitEdges: (limit = 25, offset = 0, query = "", sort = "observations", order = "desc") => {
    const q = new URLSearchParams({ limit: String(limit), offset: String(offset), sort, order });
    if (query) q.set("q", query);
    return getPage<ASNTransitEdge>(`/api/transit?${q}`);
  },
  transitPairDetail: (a: number, b: number) =>
    get<TransitPairDetail>(`/api/transit/${a}/${b}`),
  transitPairSeries: (a: number, b: number, range = "24h", buckets = 60) =>
    get<TransitPairSeriesPoint[]>(
      `/api/transit/${a}/${b}/series?range=${encodeURIComponent(range)}&buckets=${buckets}`,
    ),
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
