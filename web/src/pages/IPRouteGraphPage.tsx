import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, CheckCircle2, Filter, Focus, TriangleAlert } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import Dynamic from "./_ForceGraphLazy";
import { api, type GraphEdge, type GraphNode } from "@/api/client";
import { buttonVariants } from "@/components/ui/button";
import { ErrorState, LoadingState } from "@/components/ui/states";
import { fmtNum, fmtRtt } from "@/lib/utils";

const ROLE_COLOR: Record<string, string> = {
  seed: "#fdb022",
  upstream: "#2e90fa",
  downstream: "#12b76a",
  both: "#ee46bc",
};

type GraphRef = {
  zoomToFit?: (ms?: number, padding?: number) => void;
  centerAt?: (x?: number, y?: number, ms?: number) => void;
  zoom?: (scale?: number, ms?: number) => number | void;
};

function fitGraph(ref: GraphRef | null) {
  ref?.zoomToFit?.(400, 60);
  window.setTimeout(() => {
    const scale = ref?.zoom?.();
    if (typeof scale === "number" && scale > 2.5) ref?.zoom?.(2.5, 200);
  }, 450);
}

export function IPRouteGraphPage() {
  const { addr } = useParams<{ addr: string }>();
  const seed = decodeURIComponent(addr ?? "");
  const navigate = useNavigate();
  const graphRef = useRef<GraphRef | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [dims, setDims] = useState({ width: 900, height: 640 });
  const [sourceIP, setSourceIP] = useState("");
  const [sourceASN, setSourceASN] = useState("");
  const [destinationIP, setDestinationIP] = useState("");
  const [destinationASN, setDestinationASN] = useState("");
  const [selected, setSelected] = useState<GraphNode | null>(null);

  const graph = useQuery({
    queryKey: ["ip-route-graph", seed],
    queryFn: () => api.ipRouteGraph(seed),
    enabled: seed.length > 0,
    refetchInterval: false,
    refetchOnWindowFocus: false,
  });

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const resize = () => {
      const rect = el.getBoundingClientRect();
      if (rect.width > 0 && rect.height > 0) {
        setDims({ width: Math.round(rect.width), height: Math.round(rect.height) });
      }
    };
    const observer = new ResizeObserver(resize);
    observer.observe(el);
    resize();
    return () => observer.disconnect();
  }, []);

  const filtered = useMemo(() => {
    const data = graph.data;
    if (!data) return { nodes: [] as GraphNode[], links: [] as Array<GraphEdge & { source: string; target: string }> };
    const byID = new Map(data.nodes.map((node) => [node.id, node]));
    const normalizedASN = (value: string) => value.trim().toLowerCase().replace(/^as/, "");
    const sourceASNValue = normalizedASN(sourceASN);
    const destinationASNValue = normalizedASN(destinationASN);
    const edges = data.edges.filter((edge) => {
      const source = byID.get(edge.from);
      const destination = byID.get(edge.to);
      return (!sourceIP.trim() || edge.from.toLowerCase().includes(sourceIP.trim().toLowerCase()))
        && (!destinationIP.trim() || edge.to.toLowerCase().includes(destinationIP.trim().toLowerCase()))
        && (!sourceASNValue || String(source?.asn ?? "") === sourceASNValue)
        && (!destinationASNValue || String(destination?.asn ?? "") === destinationASNValue);
    });
    const visibleIDs = new Set<string>([data.seed]);
    for (const edge of edges) {
      visibleIDs.add(edge.from);
      visibleIDs.add(edge.to);
    }
    return {
      nodes: data.nodes.filter((node) => visibleIDs.has(node.id)).map((node) => ({ ...node })),
      links: edges.map((edge) => ({ ...edge, source: edge.from, target: edge.to })),
    };
  }, [graph.data, sourceIP, sourceASN, destinationIP, destinationASN]);

  useEffect(() => {
    if (filtered.nodes.length === 0) return;
    const timer = window.setTimeout(() => fitGraph(graphRef.current), 600);
    return () => window.clearTimeout(timer);
  }, [filtered.nodes.length, filtered.links.length]);

  if (graph.isLoading) return <LoadingState label={`Computing the observed route graph around ${seed}…`} />;
  if (graph.error) return <ErrorState message={(graph.error as Error).message} />;
  if (!graph.data) return null;

  const hasFilters = Boolean(sourceIP || sourceASN || destinationIP || destinationASN);

  return (
    <div className="flex h-[calc(100vh-6.5rem)] min-h-[620px] flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <Link to={`/ip/${encodeURIComponent(seed)}`} className={buttonVariants({ variant: "ghost", size: "sm" })}>
          <ArrowLeft className="h-4 w-4" /> IP details
        </Link>
        <div className="min-w-0">
          <h1 className="truncate font-mono text-lg font-semibold text-text-primary">Route graph · {seed}</h1>
          <p className="text-xs text-text-quaternary">Observed directed traceroute adjacencies, traversed toward both route beginnings and endings.</p>
        </div>
        <div className="ml-auto flex items-center gap-2 rounded-lg border border-border-primary bg-bg-secondary px-3 py-2 text-xs">
          {graph.data.complete ? (
            <><CheckCircle2 className="h-4 w-4 text-success-500" /><span className="text-text-secondary">Traversal completed</span></>
          ) : (
            <><TriangleAlert className="h-4 w-4 text-warning-500" /><span className="text-text-secondary">Partial · {graph.data.truncation_reason}</span></>
          )}
        </div>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 xl:grid-cols-[260px_minmax(0,1fr)]">
        <aside className="overflow-y-auto rounded-xl border border-border-primary bg-bg-secondary p-4">
          <div className="mb-4 flex items-center gap-2">
            <Filter className="h-4 w-4 text-brand-300" />
            <h2 className="text-sm font-semibold text-text-primary">Relationship filters</h2>
          </div>
          <p className="mb-4 text-xs text-text-quaternary">Filters apply to each directed edge. ASN matches are exact; IP matches accept partial addresses.</p>
          <FilterInput label="Source IP" value={sourceIP} onChange={setSourceIP} placeholder="131.108…" />
          <FilterInput label="Source ASN" value={sourceASN} onChange={setSourceASN} placeholder="AS1234" />
          <FilterInput label="Destination IP" value={destinationIP} onChange={setDestinationIP} placeholder="8.8.8.8" />
          <FilterInput label="Destination ASN" value={destinationASN} onChange={setDestinationASN} placeholder="AS15169" />
          {hasFilters && (
            <button
              type="button"
              onClick={() => { setSourceIP(""); setSourceASN(""); setDestinationIP(""); setDestinationASN(""); }}
              className="mb-4 text-xs font-medium text-brand-300 hover:text-brand-500"
            >
              Clear all filters
            </button>
          )}

          <div className="space-y-2 border-t border-border-primary pt-4 text-xs text-text-quaternary">
            <div className="flex justify-between"><span>Visible nodes</span><span className="font-mono text-text-secondary">{fmtNum(filtered.nodes.length)} / {fmtNum(graph.data.nodes.length)}</span></div>
            <div className="flex justify-between"><span>Visible edges</span><span className="font-mono text-text-secondary">{fmtNum(filtered.links.length)} / {fmtNum(graph.data.edges.length)}</span></div>
            <div className="flex justify-between"><span>Traversal depth</span><span className="font-mono text-text-secondary">≤ {graph.data.max_depth}</span></div>
          </div>

          <div className="mt-5 space-y-2 border-t border-border-primary pt-4 text-xs text-text-quaternary">
            <Legend color={ROLE_COLOR.seed} label="Selected IP" />
            <Legend color={ROLE_COLOR.upstream} label="Upstream" />
            <Legend color={ROLE_COLOR.downstream} label="Downstream" />
            <Legend color={ROLE_COLOR.both} label="Seen on both sides" />
          </div>

          {selected && (
            <div className="mt-5 border-t border-border-primary pt-4">
              <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-text-quaternary">Selected node</h3>
              <Link to={`/ip/${encodeURIComponent(selected.id)}`} className="break-all font-mono text-sm text-brand-300 hover:text-brand-500">{selected.id}</Link>
              <div className="mt-1 text-xs text-text-quaternary">{selected.asn ? `AS${selected.asn} · ${selected.org || "Unknown organization"}` : "ASN unknown"}</div>
              <div className="mt-1 text-xs text-text-quaternary">{selected.traversal_role} · depth {selected.depth ?? 0}</div>
            </div>
          )}
        </aside>

        <div ref={containerRef} className="relative min-h-[520px] overflow-hidden rounded-xl border border-border-primary bg-bg-primary">
          <button
            type="button"
            title="Fit graph to view"
            onClick={() => fitGraph(graphRef.current)}
            className="absolute right-3 top-3 z-10 rounded-lg border border-border-primary bg-bg-secondary/90 p-2 text-text-tertiary hover:text-text-primary"
          >
            <Focus className="h-4 w-4" />
          </button>
          {filtered.links.length === 0 && hasFilters ? (
            <div className="flex h-full items-center justify-center px-6 text-center text-sm text-text-quaternary">No relationships match these source and destination filters.</div>
          ) : (
            <Dynamic
              ref={graphRef as never}
              graphData={{ nodes: filtered.nodes, links: filtered.links }}
              width={dims.width}
              height={dims.height}
              backgroundColor="transparent"
              nodeRelSize={6}
              nodeColor={(node: GraphNode) => ROLE_COLOR[node.traversal_role ?? ""] ?? "#667085"}
              nodeVal={(node: GraphNode) => node.id === seed ? 3 : 1}
              nodeLabel={(node: GraphNode) => `${node.id}${node.asn ? ` · AS${node.asn}` : ""}${node.org ? ` · ${node.org}` : ""}`}
              linkColor={() => "rgba(152, 162, 179, 0.55)"}
              linkWidth={(edge: GraphEdge) => Math.max(1, Math.min(4, Math.log10((edge.seen_count ?? 0) + 1)))}
              linkLabel={(edge: GraphEdge) => `${edge.from} → ${edge.to} · ${fmtRtt(edge.last_rtt_ms ?? 0)} · ${fmtNum(edge.seen_count ?? 0)} observations`}
              linkDirectionalArrowLength={5}
              linkDirectionalArrowRelPos={1}
              onNodeClick={(node: GraphNode) => setSelected(node)}
              onNodeRightClick={(node: GraphNode) => navigate(`/ip/${encodeURIComponent(node.id)}`)}
              cooldownTicks={100}
            />
          )}
        </div>
      </div>
    </div>
  );
}

function FilterInput({ label, value, onChange, placeholder }: { label: string; value: string; onChange: (value: string) => void; placeholder: string }) {
  return (
    <label className="mb-3 block text-xs font-medium text-text-quaternary">
      {label}
      <input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        className="mt-1 block w-full rounded-lg border border-border-primary bg-bg-tertiary px-3 py-2 font-mono text-xs text-text-primary placeholder:text-text-quaternary focus:border-brand-500 focus:outline-none"
      />
    </label>
  );
}

function Legend({ color, label }: { color: string; label: string }) {
  return <div className="flex items-center gap-2"><span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: color }} />{label}</div>;
}
