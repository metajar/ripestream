import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import Dynamic from "./_ForceGraphLazy";
import { api, type GraphEdge, type GraphNode } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { fmtNum } from "@/lib/utils";

// Visual encoding for node kinds (color + shape label)
const KIND_COLOR: Record<string, string> = {
  ip: "#2e90fa",
  as: "#6172f3",
  probe: "#12b76a",
};

// Visual encoding for edge kinds (distinct colors so PING/NEXT_HOP/IN_AS/etc.
// are distinguishable, not all the same gray link)
const EDGE_COLOR: Record<string, string> = {
  next_hop: "#2e90fa",
  ping: "#f04438",
  in_as: "#6172f3",
  transits: "#fdb022",
  targets: "#12b76a",
  located_at: "#667085",
};

const EDGE_LABEL: Record<string, string> = {
  next_hop: "NEXT_HOP (traceroute hop)",
  ping: "PING (probe→target health)",
  in_as: "IN_AS (IP belongs to AS)",
  transits: "TRANSITS (AS→AS path)",
  targets: "TARGETS (probe→destination)",
  located_at: "LOCATED_AT (probe→source IP)",
};

export function TopologyPage() {
  const [params] = useSearchParams();
  // Prefill seed from URL params (links from detail pages).
  const [seedType, setSeedType] = useState<"asn" | "target" | "probe">(
    (params.get("seed_type") as "asn" | "target" | "probe") || "asn",
  );
  const [seed, setSeed] = useState(
    params.get("asn") || params.get("target") || params.get("probe") || "",
  );
  const [depth, setDepth] = useState(Number(params.get("depth")) || 2);
  const [limit, setLimit] = useState(Number(params.get("limit")) || 80);
  const [showList, setShowList] = useState(false);

  const [nodes, setNodes] = useState<GraphNode[]>([]);
  const [edges, setEdges] = useState<GraphEdge[]>([]);
  const [loading, setLoading] = useState(false);
  const [meta, setMeta] = useState("");
  const fgRef = useRef<unknown>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [dims, setDims] = useState({ width: 800, height: 480 });

  // Measure the container so react-force-graph renders at the right canvas size
  // (it doesn't reliably auto-size to a CSS container, causing overflow/offset).
  useEffect(() => {
    if (!containerRef.current) return;
    const el = containerRef.current;
    const ro = new ResizeObserver((entries) => {
      const cr = entries[0]?.contentRect;
      if (cr && cr.width > 0) setDims({ width: Math.round(cr.width), height: 480 });
    });
    ro.observe(el);
    // Initial measurement.
    const r = el.getBoundingClientRect();
    if (r.width > 0) setDims({ width: Math.round(r.width), height: 480 });
    return () => ro.disconnect();
  }, []);

  // After data loads, zoom-to-fit so the graph is centered and visible.
  useEffect(() => {
    if (nodes.length === 0 || showList) return;
    // The force graph needs a tick to settle before zoomToFit can find bounds.
    const t = setTimeout(() => {
      const fg = fgRef.current as { zoomToFit?: (ms?: number, pad?: number) => void } | null;
      fg?.zoomToFit?.(400, 40);
    }, 500);
    return () => clearTimeout(t);
  }, [nodes, showList]);

  async function load(e: React.FormEvent) {
    e.preventDefault();
    if (!seed) return;
    setLoading(true);
    setMeta("");
    try {
      const base = { depth, limit };
      const p =
        seedType === "asn"
          ? { ...base, asn: Number(seed) }
          : seedType === "target"
            ? { ...base, target: seed }
            : { ...base, probe: Number(seed) };
      const sg = await api.subgraph(p);
      setNodes(sg.nodes);
      setEdges(sg.edges);
      setMeta(`${sg.nodes.length} nodes · ${sg.edges.length} edges (depth ${depth}, limit ${limit})`);
    } finally {
      setLoading(false);
    }
  }

  // Links preserve edge kind for color-coding.
  const graphData = useMemo(
    () => ({
      nodes: nodes.map((n) => ({ ...n })),
      links: edges.map((e) => ({ source: e.from, target: e.to, kind: e.kind })),
    }),
    [nodes, edges],
  );

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-lg font-semibold text-text-primary">Topology</h1>
        <p className="text-sm text-text-quaternary">
          Interactive force-directed subgraph around a seed. Advanced investigation tool.
        </p>
      </div>

      {/* Seed + controls */}
      <Card>
        <CardContent className="pt-4">
          <form onSubmit={load} className="flex flex-wrap items-end gap-3">
            <div className="w-32">
              <label className="mb-1 block text-xs font-medium text-text-quaternary">Seed type</label>
              <select
                value={seedType}
                onChange={(e) => setSeedType(e.target.value as "asn" | "target" | "probe")}
                className="w-full rounded-lg border border-border-primary bg-bg-tertiary px-3 py-2 text-sm text-text-primary focus:border-brand-500 focus:outline-none"
              >
                <option value="asn">AS number</option>
                <option value="target">Target IP</option>
                <option value="probe">Probe ID</option>
              </select>
            </div>
            <div className="flex-1">
              <label className="mb-1 block text-xs font-medium text-text-quaternary">Seed value</label>
              <input
                value={seed}
                onChange={(e) => setSeed(e.target.value)}
                placeholder={seedType === "asn" ? "13335" : seedType === "probe" ? "1004501" : "1.1.1.1"}
                className="w-full rounded-lg border border-border-primary bg-bg-tertiary px-3 py-2 font-mono text-sm text-text-primary placeholder:text-text-quaternary focus:border-brand-500 focus:outline-none"
              />
            </div>
            <div className="w-20">
              <label className="mb-1 block text-xs font-medium text-text-quaternary" title="Max traversal depth (backend max: 4)">Depth</label>
              <input
                type="number" min={1} max={4} value={depth}
                onChange={(e) => setDepth(Math.min(4, Math.max(1, Number(e.target.value) || 2)))}
                className="w-full rounded-lg border border-border-primary bg-bg-tertiary px-2 py-2 text-sm text-text-primary focus:border-brand-500 focus:outline-none"
              />
            </div>
            <div className="w-20">
              <label className="mb-1 block text-xs font-medium text-text-quaternary" title="Max nodes returned (backend max: 300)">Limit</label>
              <input
                type="number" min={10} max={300} value={limit}
                onChange={(e) => setLimit(Math.min(300, Math.max(10, Number(e.target.value) || 80)))}
                className="w-full rounded-lg border border-border-primary bg-bg-tertiary px-2 py-2 text-sm text-text-primary focus:border-brand-500 focus:outline-none"
              />
            </div>
            <Button type="submit" disabled={loading || !seed}>Load</Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Subgraph</CardTitle>
          <div className="flex items-center gap-3">
            {meta && <span className="text-xs text-text-quaternary">{meta}</span>}
            {nodes.length > 0 && (
              <button
                type="button"
                onClick={() => setShowList(!showList)}
                className="text-xs text-brand-300 hover:text-brand-500"
              >
                {showList ? "Show graph" : "Show text list"}
              </button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          {/* Bounded limits shown before/during load */}
          {loading && <p className="mb-2 text-xs text-text-quaternary">Loading (depth {depth}, max {limit} nodes)…</p>}

          {/* Text/list alternative for accessibility */}
          {showList && nodes.length > 0 ? (
            <div className="space-y-3">
              <div>
                <h3 className="mb-1 text-xs font-semibold uppercase text-text-quaternary">Nodes ({nodes.length})</h3>
                <ul className="space-y-0.5 text-sm">
                  {nodes.map((n) => (
                    <li key={n.id} className="flex gap-2">
                      <span className="rounded bg-bg-tertiary px-1 text-xs uppercase text-text-quaternary">{n.kind}</span>
                      <span className="text-text-secondary">{n.label}</span>
                      {n.asn && (
                        <Link to={`/asn/${n.asn}`} className="text-xs text-brand-300 hover:text-brand-500">AS{n.asn}</Link>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
              <div>
                <h3 className="mb-1 text-xs font-semibold uppercase text-text-quaternary">Edges ({edges.length})</h3>
                <ul className="space-y-0.5 text-xs">
                  {edges.slice(0, 100).map((e, i) => (
                    <li key={i} className="font-mono text-text-tertiary">
                      <span className="text-text-quaternary">{e.kind}</span>: {e.from} → {e.to}
                    </li>
                  ))}
                  {edges.length > 100 && <li className="text-text-quaternary">…and {edges.length - 100} more</li>}
                </ul>
              </div>
            </div>
          ) : (
            <div ref={containerRef} className="h-[480px] w-full overflow-hidden rounded-lg bg-bg-primary">
              {nodes.length > 0 ? (
                <Dynamic
                  graphData={graphData}
                  width={dims.width}
                  height={dims.height}
                  nodeRelSize={6}
                  nodeColor={(n: { kind?: string }) => KIND_COLOR[n.kind ?? "ip"] ?? "#667085"}
                  nodeLabel="label"
                  linkColor={(l: { kind?: string }) => EDGE_COLOR[l.kind ?? ""] ?? "#333b4f"}
                  linkDirectionalArrowLength={4}
                  backgroundColor="transparent"
                  ref={fgRef as never}
                />
              ) : (
                <div className="flex h-full items-center justify-center text-sm text-text-quaternary">
                  {loading ? "Loading…" : "Load a seed to render the topology"}
                </div>
              )}
            </div>
          )}

          {/* Legends: node kinds + edge kinds */}
          {nodes.length > 0 && (
            <div className="mt-3 flex flex-wrap gap-4 text-xs text-text-quaternary">
              <span className="font-medium text-text-tertiary">Nodes:</span>
              <Legend color={KIND_COLOR.ip} label={`${fmtNum(nodes.filter((n) => n.kind === "ip").length)} IPs`} />
              <Legend color={KIND_COLOR.as} label={`${fmtNum(nodes.filter((n) => n.kind === "as").length)} ASes`} />
              <Legend color={KIND_COLOR.probe} label={`${fmtNum(nodes.filter((n) => n.kind === "probe").length)} Probes`} />
              <span className="ml-2 font-medium text-text-tertiary">Edges:</span>
              {Object.entries(EDGE_LABEL).map(([k, label]) => {
                const count = edges.filter((e) => e.kind === k).length;
                if (count === 0) return null;
                return <Legend key={k} color={EDGE_COLOR[k]} label={`${count} ${label.split(" ")[0]}`} title={label} />;
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function Legend({ color, label, title }: { color: string; label: string; title?: string }) {
  return (
    <div className="flex items-center gap-1.5" title={title}>
      <span className="h-2.5 w-2.5 rounded-full" style={{ background: color }} />
      {label}
    </div>
  );
}
