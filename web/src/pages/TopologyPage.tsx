import { useMemo, useState } from "react";
import { useRef } from "react";
import Dynamic from "./_ForceGraphLazy";
import { api, type GraphNode } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { fmtNum } from "@/lib/utils";

// Visual encoding for node kinds
const KIND_COLOR: Record<string, string> = {
  ip: "#2e90fa",
  as: "#6172f3",
  probe: "#12b76a",
};

export function TopologyPage() {
  const [seed, setSeed] = useState("");
  const [seedType, setSeedType] = useState<"asn" | "target" | "probe">("asn");
  const [nodes, setNodes] = useState<GraphNode[]>([]);
  const [links, setLinks] = useState<{ source: string; target: string }[]>([]);
  const [loading, setLoading] = useState(false);
  const [meta, setMeta] = useState("");
  const fgRef = useRef<unknown>(null);

  async function load(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setMeta("");
    try {
      const params =
        seedType === "asn"
          ? { asn: Number(seed), depth: 2, limit: 80 }
          : seedType === "target"
            ? { target: seed, depth: 2, limit: 80 }
            : { probe: Number(seed), depth: 2, limit: 80 };
      const sg = await api.subgraph(params);
      setNodes(sg.nodes);
      setLinks(sg.edges.map((e) => ({ source: e.from, target: e.to })));
      setMeta(`${sg.nodes.length} nodes · ${sg.edges.length} edges`);
    } finally {
      setLoading(false);
    }
  }

  const graphData = useMemo(() => ({ nodes, links }), [nodes, links]);

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-lg font-semibold text-text-primary">Topology</h1>
        <p className="text-sm text-text-quaternary">Interactive force-directed subgraph around a seed</p>
      </div>

      <Card>
        <CardContent className="pt-4">
          <form onSubmit={load} className="flex items-end gap-3">
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
            <Button type="submit" disabled={loading || !seed}>
              Load
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Subgraph</CardTitle>
          {meta && <span className="text-xs text-text-quaternary">{meta}</span>}
        </CardHeader>
        <CardContent>
          <div className="h-[480px] w-full rounded-lg bg-bg-primary">
            {nodes.length > 0 ? (
              <Dynamic
                graphData={graphData}
                nodeRelSize={6}
                nodeColor={(n: { kind?: string }) => KIND_COLOR[n.kind ?? "ip"] ?? "#667085"}
                nodeLabel="label"
                linkColor={() => "#333b4f"}
                linkDirectionalArrowLength={4}
                backgroundColor="transparent"
                ref={fgRef as never}
              />
            ) : (
              <div className="flex h-full items-center justify-center text-sm text-text-quaternary">
                Load a seed to render the topology
              </div>
            )}
          </div>
          {nodes.length > 0 && (
            <div className="mt-3 flex gap-4 text-xs text-text-quaternary">
              <Legend color={KIND_COLOR.ip} label={`${fmtNum(nodes.filter((n) => n.kind === "ip").length)} IPs`} />
              <Legend color={KIND_COLOR.as} label={`${fmtNum(nodes.filter((n) => n.kind === "as").length)} ASes`} />
              <Legend color={KIND_COLOR.probe} label={`${fmtNum(nodes.filter((n) => n.kind === "probe").length)} Probes`} />
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function Legend({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex items-center gap-1.5">
      <span className="h-2.5 w-2.5 rounded-full" style={{ background: color }} />
      {label}
    </div>
  );
}
