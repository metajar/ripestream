import { useQuery } from "@tanstack/react-query";
import { ArrowUpRight, Check, CircleDot, Clock3, GitFork, Radar, Route } from "lucide-react";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api, type HopCommonality } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { EmptyState, ErrorState, LoadingState } from "@/components/ui/states";
import { fmtEpoch, fmtNum, fmtRtt } from "@/lib/utils";

type Range = "15m" | "30m" | "1h" | "6h";

export function RouteCorrelationPage() {
  const [range, setRange] = useState<Range>("30m");
  const [minProbes, setMinProbes] = useState(3);
  const [selectedAddr, setSelectedAddr] = useState<string>();
  const query = useQuery({
    queryKey: ["hop-commonality", range, minProbes],
    queryFn: () => api.hopCommonalities(range, minProbes),
    retry: false,
    refetchInterval: 60_000,
  });
  const candidates = query.data?.candidates ?? [];
  const selected = candidates.find((hop) => hop.addr === selectedAddr) ?? candidates[0];

  const totals = useMemo(() => ({
    probes: Math.max(0, ...candidates.map((c) => c.probes)),
    targets: Math.max(0, ...candidates.map((c) => c.targets)),
    traces: candidates.reduce((sum, c) => sum + c.traces, 0),
  }), [candidates]);

  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div className="max-w-2xl">
          <div className="mb-1 flex items-center gap-2">
            <Radar className="h-5 w-5 text-brand-300" />
            <h1 className="text-xl font-semibold text-text-primary">Route Correlation</h1>
            {candidates.length > 0 && <Badge variant="warning">{candidates.length} shared signals</Badge>}
          </div>
          <p className="text-sm text-text-quaternary">
            Find hops where independent traceroutes changed together — shared-fate evidence across probes and destinations.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <label className="text-xs text-text-quaternary" htmlFor="correlation-probes">Agreement</label>
          <select
            id="correlation-probes"
            value={minProbes}
            onChange={(event) => setMinProbes(Number(event.target.value))}
            className="rounded-lg border border-border-primary bg-bg-secondary px-2.5 py-1.5 text-xs text-text-secondary outline-none focus:border-brand-500"
          >
            <option value={2}>2+ probes</option>
            <option value={3}>3+ probes</option>
            <option value={5}>5+ probes</option>
          </select>
          <div className="flex rounded-lg border border-border-primary bg-bg-secondary p-1">
            {(["15m", "30m", "1h", "6h"] as Range[]).map((value) => (
              <button
                key={value}
                type="button"
                onClick={() => setRange(value)}
                className={`rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
                  range === value ? "bg-brand-500 text-white" : "text-text-tertiary hover:text-text-primary"
                }`}
              >{value}</button>
            ))}
          </div>
        </div>
      </header>

      {query.isLoading && <Card><LoadingState label="Reconstructing shared route changes…" /></Card>}
      {query.error && <Card><ErrorState message={(query.error as Error).message} /></Card>}
      {query.data && candidates.length === 0 && (
        <Card>
          <EmptyState
            label="No corroborated shared-hop regressions"
            hint={`No transit hop rose at least 15 ms and 35% above its own ${query.data.baseline_hours}-hour baseline with ${minProbes}+ probes in the last ${query.data.recent_minutes} minutes.`}
          />
        </Card>
      )}

      {query.data && candidates.length > 0 && (
        <>
          <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <SignalStat label="Shared hop candidates" value={fmtNum(candidates.length)} icon={CircleDot} />
            <SignalStat label="Widest probe agreement" value={fmtNum(totals.probes)} icon={Radar} />
            <SignalStat label="Widest destination reach" value={fmtNum(totals.targets)} icon={Route} />
            <SignalStat label="Hop–trace matches" value={fmtNum(totals.traces)} icon={GitFork} />
          </section>

          <section className="grid gap-4 xl:grid-cols-[minmax(0,1.65fr)_minmax(320px,0.75fr)]">
            <Card className="overflow-hidden">
              <div className="flex items-start justify-between gap-4 border-b border-border-primary px-5 py-4">
                <div>
                  <h2 className="text-sm font-semibold text-text-primary">Correlation field</h2>
                  <p className="mt-0.5 text-xs text-text-quaternary">Higher and farther right means a larger, more widely corroborated change. Bubble size is destination breadth.</p>
                </div>
                <span className="shrink-0 text-[11px] text-text-quaternary">vs {query.data.baseline_hours}h baseline</span>
              </div>
              <CardContent className="pt-4">
                <CorrelationField candidates={candidates} selected={selected?.addr} onSelect={setSelectedAddr} />
              </CardContent>
            </Card>

            {selected && <EvidenceRail hop={selected} recentMinutes={query.data.recent_minutes} />}
          </section>

          <Card>
            <div className="border-b border-border-primary px-5 py-4">
              <h2 className="text-sm font-semibold text-text-primary">Ranked shared-fate candidates</h2>
              <p className="mt-0.5 text-xs text-text-quaternary">Click a row to focus its evidence above.</p>
            </div>
            <div className="divide-y divide-border-primary">
              {candidates.map((hop, index) => (
                <button
                  key={hop.addr}
                  type="button"
                  onClick={() => setSelectedAddr(hop.addr)}
                  className={`grid w-full grid-cols-[32px_minmax(140px,1fr)_repeat(2,minmax(68px,auto))] items-center gap-3 px-5 py-3 text-left transition-colors hover:bg-bg-tertiary xl:grid-cols-[32px_minmax(160px,1fr)_repeat(3,minmax(72px,auto))] xl:gap-4 2xl:grid-cols-[32px_minmax(180px,1fr)_repeat(5,minmax(72px,auto))] ${selected?.addr === hop.addr ? "bg-brand-500/5" : ""}`}
                >
                  <span className="font-mono text-xs text-text-quaternary">{String(index + 1).padStart(2, "0")}</span>
                  <span className="min-w-0">
                    <span className="block truncate font-mono text-xs text-brand-300">{hop.addr}</span>
                    <span className="block truncate text-[11px] text-text-quaternary">{hop.org || (hop.asn ? `AS${hop.asn}` : "ASN unknown")}</span>
                  </span>
                  <RowMetric label="RTT shift" value={`+${fmtRtt(hop.rtt_delta_ms)}`} tone="text-warning-500" />
                  <RowMetric label="Probes" value={fmtNum(hop.probes)} />
                  <RowMetric label="Targets" value={fmtNum(hop.targets)} className="hidden xl:block" />
                  <RowMetric label="Traces" value={fmtNum(hop.traces)} className="hidden 2xl:block" />
                  <RowMetric label="Last seen" value={fmtEpoch(hop.last_seen)} className="hidden 2xl:block" />
                </button>
              ))}
            </div>
          </Card>

          <p className="flex items-start gap-2 text-xs text-text-quaternary">
            <CircleDot className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            A traceroute hop can delay or deprioritize ICMP replies without delaying forwarded traffic. Treat this as a high-value investigation lead, then confirm with endpoint loss, adjacent hops, and provider evidence.
          </p>
        </>
      )}
    </div>
  );
}

function SignalStat({ label, value, icon: Icon }: { label: string; value: string; icon: typeof Radar }) {
  return (
    <Card className="px-4 py-3">
      <div className="flex items-center gap-3">
        <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-brand-500/10"><Icon className="h-4 w-4 text-brand-300" /></span>
        <span><span className="block text-xl font-semibold text-text-primary">{value}</span><span className="block text-xs text-text-quaternary">{label}</span></span>
      </div>
    </Card>
  );
}

function CorrelationField({ candidates, selected, onSelect }: { candidates: HopCommonality[]; selected?: string; onSelect: (addr: string) => void }) {
  const width = 760, height = 330, left = 56, right = 24, top = 20, bottom = 46;
  const maxX = Math.max(5, ...candidates.map((c) => c.probes));
  const maxY = Math.max(50, ...candidates.map((c) => c.rtt_delta_ms)) * 1.1;
  const x = (value: number) => left + (value / maxX) * (width - left - right);
  const y = (value: number) => top + (1 - value / maxY) * (height - top - bottom);
  const yTicks = [0, .25, .5, .75, 1].map((p) => Math.round(maxY * p));
  const xTicks = Array.from(new Set([0, Math.ceil(maxX / 4), Math.ceil(maxX / 2), Math.ceil(maxX * .75), maxX])).sort((a, b) => a - b);
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="w-full" role="img" aria-label="Shared hop correlation plot">
      {yTicks.map((tick) => <g key={tick}><line x1={left} x2={width - right} y1={y(tick)} y2={y(tick)} style={{ stroke: "var(--color-border-secondary)" }} /><text x={left - 10} y={y(tick) + 4} textAnchor="end" className="text-[10px]" style={{ fill: "var(--color-text-quaternary)" }}>{tick}</text></g>)}
      {xTicks.map((tick) => <g key={tick}><line x1={x(tick)} x2={x(tick)} y1={top} y2={height - bottom} style={{ stroke: "var(--color-border-primary)", opacity: 0.55 }} /><text x={x(tick)} y={height - 22} textAnchor="middle" className="text-[10px]" style={{ fill: "var(--color-text-quaternary)" }}>{tick}</text></g>)}
      <text x={16} y={height / 2} transform={`rotate(-90 16 ${height / 2})`} textAnchor="middle" className="text-[10px]" style={{ fill: "var(--color-text-tertiary)" }}>Median RTT increase (ms)</text>
      <text x={(left + width - right) / 2} y={height - 3} textAnchor="middle" className="text-[10px]" style={{ fill: "var(--color-text-tertiary)" }}>Independent probes observing the same change</text>
      {candidates.map((hop) => {
        const radius = Math.min(20, 6 + Math.sqrt(hop.targets) * 3);
        const active = hop.addr === selected;
        return <g key={hop.addr} onClick={() => onSelect(hop.addr)} className="cursor-pointer">
          {active && <circle cx={x(hop.probes)} cy={y(hop.rtt_delta_ms)} r={radius + 6} fill="var(--color-brand-500)" fillOpacity="0.18" stroke="var(--color-brand-300)" strokeWidth="1.5" />}
          <circle
            cx={x(hop.probes)}
            cy={y(hop.rtt_delta_ms)}
            r={radius}
            fill={active ? "var(--color-brand-500)" : "var(--color-warning-500)"}
            fillOpacity={active ? 1 : 0.78}
            stroke={active ? "var(--color-brand-300)" : "var(--color-warning-500)"}
            strokeWidth="1.5"
          >
            <title>{`${hop.addr}: ${hop.probes} probes, ${hop.targets} targets, +${Math.round(hop.rtt_delta_ms)} ms`}</title>
          </circle>
        </g>;
      })}
    </svg>
  );
}

function EvidenceRail({ hop, recentMinutes }: { hop: HopCommonality; recentMinutes: number }) {
  const confidence = hop.probes >= 5 && hop.targets >= 3 && hop.baseline_samples >= 50 ? "High confidence" : hop.probes >= 3 ? "Corroborated" : "Early signal";
  const variant = confidence === "High confidence" ? "danger" : "warning";
  const maxRtt = Math.max(hop.recent_median_rtt_ms, hop.baseline_median_rtt_ms, 1);
  return (
    <Card className="overflow-hidden">
      <div className="border-b border-border-primary bg-bg-tertiary/40 px-5 py-4">
        <div className="mb-3 flex items-center justify-between"><Badge variant={variant}>{confidence}</Badge><span className="text-[11px] text-text-quaternary">impact {Math.round(hop.impact_score)}</span></div>
        <Link to={`/ip/${hop.addr}`} className="group flex items-center gap-2 font-mono text-base text-brand-300 hover:text-brand-500">
          {hop.addr}<ArrowUpRight className="h-4 w-4 transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" />
        </Link>
        <p className="mt-1 truncate text-xs text-text-tertiary">{hop.org || "Unidentified network"}{hop.asn ? ` · AS${hop.asn}` : ""}</p>
      </div>
      <CardContent className="space-y-5 pt-4">
        <div>
          <div className="mb-2 flex items-baseline justify-between"><span className="text-xs text-text-quaternary">Median RTT</span><span className="font-mono text-sm font-medium text-warning-500">+{fmtRtt(hop.rtt_delta_ms)} <span className="text-[11px] text-text-quaternary">({Math.round(hop.change_pct)}%)</span></span></div>
          <RttBar label={`${recentMinutes}m now`} value={hop.recent_median_rtt_ms} max={maxRtt} active />
          <RttBar label="24h baseline" value={hop.baseline_median_rtt_ms} max={maxRtt} />
        </div>
        <div className="grid grid-cols-3 gap-2 border-y border-border-primary py-3 text-center">
          <MiniMetric value={hop.probes} label="probes" /><MiniMetric value={hop.targets} label="targets" /><MiniMetric value={hop.traces} label="traces" />
        </div>
        <div>
          <h3 className="mb-2 text-xs font-medium text-text-secondary">Why it surfaced</h3>
          <EvidenceCheck text={`${hop.probes} independent probes agree`} />
          <EvidenceCheck text={`${hop.recent_samples} recent replies vs ${hop.baseline_samples} baseline`} />
          <EvidenceCheck text={`${hop.incoming} incoming / ${hop.outgoing} outgoing live paths`} />
        </div>
        <div>
          <h3 className="mb-2 text-xs font-medium text-text-secondary">Affected destinations</h3>
          <div className="flex flex-wrap gap-1.5">{hop.affected_targets.slice(0, 6).map((target) => <Link key={target} to={`/target/${target}`} className="rounded-md bg-bg-tertiary px-2 py-1 font-mono text-[10px] text-text-tertiary hover:text-brand-300">{target}</Link>)}</div>
        </div>
        <div className="flex items-center gap-2 text-[11px] text-text-quaternary"><Clock3 className="h-3.5 w-3.5" />Active {fmtEpoch(hop.first_seen)} — {fmtEpoch(hop.last_seen)}</div>
      </CardContent>
    </Card>
  );
}

function RttBar({ label, value, max, active = false }: { label: string; value: number; max: number; active?: boolean }) {
  return <div className="mb-2 grid grid-cols-[76px_1fr_58px] items-center gap-2"><span className="text-[10px] text-text-quaternary">{label}</span><span className="h-1.5 overflow-hidden rounded-full bg-bg-tertiary"><span className={`block h-full rounded-full ${active ? "bg-warning-500" : "bg-brand-500/60"}`} style={{ width: `${Math.max(4, value / max * 100)}%` }} /></span><span className="text-right font-mono text-[10px] text-text-tertiary">{fmtRtt(value)}</span></div>;
}

function MiniMetric({ value, label }: { value: number; label: string }) { return <span><strong className="block font-mono text-sm font-medium text-text-primary">{fmtNum(value)}</strong><span className="text-[10px] text-text-quaternary">{label}</span></span>; }
function EvidenceCheck({ text }: { text: string }) { return <div className="flex items-center gap-2 py-1 text-xs text-text-tertiary"><span className="flex h-4 w-4 items-center justify-center rounded-full bg-success-500/10"><Check className="h-2.5 w-2.5 text-success-500" /></span>{text}</div>; }
function RowMetric({ label, value, tone = "text-text-secondary", className = "" }: { label: string; value: string; tone?: string; className?: string }) { return <span className={className}><span className="block text-[10px] text-text-quaternary">{label}</span><span className={`block whitespace-nowrap font-mono text-xs ${tone}`}>{value}</span></span>; }
