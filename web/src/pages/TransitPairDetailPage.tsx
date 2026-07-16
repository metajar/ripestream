import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { api } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState, ErrorState, Freshness, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { fmtNum, fmtPct, fmtRtt, lossColor } from "@/lib/utils";
import { probeName, probeSubtitle } from "@/lib/probes";

type Range = "1h" | "24h" | "7d";

export function TransitPairDetailPage() {
  const { asnA, asnB } = useParams<{ asnA: string; asnB: string }>();
  const a = Number(asnA);
  const b = Number(asnB);
  const [range, setRange] = useState<Range>("24h");

  const detail = useQuery({
    queryKey: ["transit-pair", a, b],
    queryFn: () => api.transitPairDetail(a, b),
    enabled: a > 0 && b > 0,
  });
  const series = useQuery({
    queryKey: ["transit-pair-series", a, b, range],
    queryFn: () => api.transitPairSeries(a, b, range, range === "7d" ? 84 : 60),
    enabled: a > 0 && b > 0,
  });

  const summary = useMemo(() => summarizeTests(detail.data?.tests ?? []), [detail.data?.tests]);

  if (detail.isLoading) return <LoadingState label="Loading transit relationship…" />;
  if (detail.error) return <ErrorState message={(detail.error as Error).message} />;
  if (!detail.data) return null;
  const d = detail.data;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <Link to={`/asn/${d.src_asn}`} className="text-lg font-semibold text-text-primary hover:text-brand-300">
              {d.src_org || `AS${d.src_asn}`}
            </Link>
            <span className="text-text-quaternary">→</span>
            <Link to={`/asn/${d.dst_asn}`} className="text-lg font-semibold text-text-primary hover:text-brand-300">
              {d.dst_org || `AS${d.dst_asn}`}
            </Link>
          </div>
          <Link to="/transit" className="text-xs text-text-quaternary hover:text-brand-300">
            ← back to Transit Relationships
          </Link>
        </div>
        <Badge variant="neutral">{fmtNum(d.seen_count)} traceroute observations</Badge>
      </div>

      <div className="rounded-lg border border-border-primary bg-bg-secondary p-4">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-text-quaternary">Relationship evidence</h2>
        <p className="mt-1 text-sm text-text-secondary">
          This AS boundary appeared {fmtNum(d.seen_count)} time{d.seen_count === 1 ? "" : "s"} in traceroutes,
          last observed <Freshness sec={d.last_seen} />; {fmtNum(d.tests.length)} active PING tests between probes in
          the source AS and targets in the destination AS provide the health evidence below.
        </p>
        <p className="mt-1 text-xs text-text-quaternary">
          PING evidence is scoped to the endpoint ASes and does not prove every test traversed this exact relationship.
        </p>
      </div>

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Metric label="Current loss" value={fmtPct(summary.lossPct, 1)} detail={`${fmtNum(summary.received)} / ${fmtNum(summary.sent)} replies`} />
        <Metric label="Current RTT" value={fmtRtt(summary.avgRttMs)} detail="average of responding tests" />
        <Metric label="Source probes" value={fmtNum(summary.probes)} detail={`${fmtNum(d.tests.length)} active tests`} />
        <Metric label="Destination targets" value={fmtNum(summary.targets)} detail={`${fmtNum(d.hops.length)} boundary hop edges`} />
      </div>

      <Card>
        <CardHeader className="flex-wrap gap-3">
          <div>
            <CardTitle>Loss and RTT history</CardTitle>
            <p className="text-xs text-text-quaternary">ClickHouse history for the active probe and target cohort</p>
          </div>
          <div className="flex gap-1 rounded-lg border border-border-primary bg-bg-tertiary p-1">
            {(["1h", "24h", "7d"] as const).map((value) => (
              <button
                key={value}
                type="button"
                onClick={() => setRange(value)}
                className={`rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
                  range === value ? "bg-brand-500 text-white" : "text-text-tertiary hover:text-text-primary"
                }`}
              >
                {value}
              </button>
            ))}
          </div>
        </CardHeader>
        <CardContent>
          {series.isLoading && <LoadingState label="Loading relationship history…" />}
          {series.error && <ErrorState message={(series.error as Error).message} />}
          {series.data && series.data.length === 0 && (
            <EmptyState label="No historical PING data for this cohort" hint={`No matching samples were stored during the last ${range}.`} />
          )}
          {series.data && series.data.length > 0 && (
            <ResponsiveContainer width="100%" height={280}>
              <LineChart data={series.data} margin={{ left: 4, right: 4, top: 8, bottom: 0 }}>
                <CartesianGrid stroke="var(--color-border-primary)" strokeDasharray="3 3" vertical={false} />
                <XAxis
                  dataKey="ts"
                  tickFormatter={(value) => formatChartTime(value, range)}
                  tick={{ fill: "var(--color-text-quaternary)", fontSize: 11 }}
                  stroke="var(--color-border-primary)"
                />
                <YAxis
                  yAxisId="loss"
                  domain={[0, 100]}
                  tickFormatter={(value) => `${value}%`}
                  tick={{ fill: "var(--color-text-quaternary)", fontSize: 11 }}
                  stroke="var(--color-border-primary)"
                  width={42}
                />
                <YAxis
                  yAxisId="rtt"
                  orientation="right"
                  tickFormatter={(value) => `${value}ms`}
                  tick={{ fill: "var(--color-text-quaternary)", fontSize: 11 }}
                  stroke="var(--color-border-primary)"
                  width={54}
                />
                <Tooltip
                  contentStyle={{
                    background: "var(--color-bg-elevated)",
                    border: "1px solid var(--color-border-secondary)",
                    borderRadius: 8,
                    fontSize: 12,
                  }}
                  labelFormatter={(value) => new Date(value).toLocaleString()}
                />
                <Line yAxisId="loss" type="monotone" dataKey="loss_pct" name="Loss %" stroke="var(--color-error-500)" strokeWidth={2} dot={false} />
                <Line yAxisId="rtt" type="monotone" dataKey="avg_rtt_ms" name="RTT (ms)" stroke="var(--color-info-500)" strokeWidth={2} dot={false} />
              </LineChart>
            </ResponsiveContainer>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div>
            <CardTitle>Active RIPE Atlas tests ({d.tests.length})</CardTitle>
            <p className="text-xs text-text-quaternary">Latest PING result per probe and destination target</p>
          </div>
        </CardHeader>
        <CardContent>
          {d.tests.length === 0 ? (
            <EmptyState label="No active endpoint tests" hint="The traceroute relationship exists, but no recent PING cohort connects these endpoint ASes." />
          ) : (
            <Table>
              <THead>
                <tr>
                  <Th>Probe / measurement</Th>
                  <Th>Source</Th>
                  <Th>Target</Th>
                  <Th className="text-right">Replies</Th>
                  <Th className="text-right">Loss</Th>
                  <Th className="text-right">RTT avg</Th>
                  <Th className="text-right">RTT range</Th>
                  <Th className="text-right">Observed</Th>
                </tr>
              </THead>
              <TBody>
                {d.tests.map((test) => (
                  <Tr key={`${test.probe_id}:${test.target_ip}`}>
                    <Td>
                      <Link to={`/probe/${test.probe_id}`} className="text-xs text-brand-300 hover:text-brand-500">
                        <span className="block">{probeName(test.probe_id, test.probe_metadata)}</span>
                        <span className="font-mono text-[11px] text-text-quaternary">{probeSubtitle(test.probe_id, test.probe_metadata)}</span>
                      </Link>
                      <div className="font-mono text-[11px] text-text-quaternary">MSM {test.msm_id}</div>
                    </Td>
                    <Td className="font-mono text-xs text-text-tertiary">{test.source_ip}</Td>
                    <Td>
                      <Link to={`/target/${encodeURIComponent(test.target_ip)}`} className="font-mono text-xs text-brand-300 hover:text-brand-500">
                        {test.target_ip}
                      </Link>
                    </Td>
                    <Td className="text-right font-mono text-xs">{test.received}/{test.sent}</Td>
                    <Td className={`text-right font-mono text-xs ${lossColor(test.loss_pct)}`}>{fmtPct(test.loss_pct)}</Td>
                    <Td className="text-right font-mono text-xs text-text-tertiary">{fmtRtt(test.avg_rtt_ms)}</Td>
                    <Td className="text-right font-mono text-xs text-text-quaternary">
                      {test.received > 0 ? `${fmtRtt(test.min_rtt_ms)}–${fmtRtt(test.max_rtt_ms)}` : "—"}
                    </Td>
                    <Td className="text-right text-xs"><Freshness sec={test.last_seen} /></Td>
                  </Tr>
                ))}
              </TBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>Traceroute hop edges crossing this boundary ({d.hops.length})</CardTitle></CardHeader>
        <CardContent>
          {d.hops.length === 0 ? (
            <EmptyState label="No retained hop edges" hint="The aggregate relationship remains, but no active NEXT_HOP edge currently realizes this boundary." />
          ) : (
            <Table>
              <THead><tr><Th>From</Th><Th>To</Th><Th className="text-right">RTT</Th><Th className="text-right">Observations</Th><Th className="text-right">Observed</Th></tr></THead>
              <TBody>
                {d.hops.map((hop) => (
                  <Tr key={`${hop.from_addr}:${hop.to_addr}`}>
                    <Td><Link to={`/ip/${hop.from_addr}`} className="font-mono text-xs text-brand-300 hover:text-brand-500">{hop.from_addr}</Link></Td>
                    <Td><Link to={`/ip/${hop.to_addr}`} className="font-mono text-xs text-brand-300 hover:text-brand-500">{hop.to_addr}</Link></Td>
                    <Td className="text-right font-mono text-xs text-text-tertiary">{fmtRtt(hop.last_rtt_ms)}</Td>
                    <Td className="text-right font-mono text-xs">{fmtNum(hop.seen_count)}</Td>
                    <Td className="text-right text-xs"><Freshness sec={hop.last_seen} /></Td>
                  </Tr>
                ))}
              </TBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function Metric({ label, value, detail }: { label: string; value: string; detail: string }) {
  return <Card className="px-4 py-3"><div className="text-xs text-text-quaternary">{label}</div><div className="text-lg font-semibold text-text-primary">{value}</div><div className="text-[11px] text-text-quaternary">{detail}</div></Card>;
}

function summarizeTests(tests: import("@/api/client").TransitTest[]) {
  const sent = tests.reduce((sum, test) => sum + test.sent, 0);
  const received = tests.reduce((sum, test) => sum + test.received, 0);
  const responding = tests.filter((test) => test.received > 0);
  return {
    sent,
    received,
    lossPct: sent > 0 ? ((sent - received) / sent) * 100 : 0,
    avgRttMs: responding.length > 0 ? responding.reduce((sum, test) => sum + test.avg_rtt_ms, 0) / responding.length : -1,
    probes: new Set(tests.map((test) => test.probe_id)).size,
    targets: new Set(tests.map((test) => test.target_ip)).size,
  };
}

function formatChartTime(value: string, range: Range) {
  const date = new Date(value);
  return range === "7d"
    ? date.toLocaleDateString("en-US", { month: "short", day: "numeric" })
    : date.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" });
}
