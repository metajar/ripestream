import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
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
import { ErrorState, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { fmtPct, fmtRtt, lossColor } from "@/lib/utils";

const CHART_COLORS = ["var(--color-chart-4)", "var(--color-chart-3)", "var(--color-chart-5)"];

export function TargetDetailPage() {
  const { addr } = useParams<{ addr: string }>();
  const decoded = decodeURIComponent(addr ?? "");
  const [metric, setMetric] = useState<"loss_pct" | "avg_rtt_ms">("loss_pct");

  const detail = useQuery({
    queryKey: ["target", decoded],
    queryFn: () => api.targetDetail(decoded),
    enabled: !!decoded,
  });
  const series = useQuery({
    queryKey: ["timeseries", decoded, metric],
    queryFn: () => api.timeseries({ target: decoded, metric, buckets: 60 }),
    enabled: !!decoded,
  });

  if (detail.isLoading) return <LoadingState label={`Loading ${decoded}…`} />;
  if (detail.error) return <ErrorState message={(detail.error as Error).message} />;
  if (!detail.data) return null;
  const d = detail.data;

  return (
    <div className="space-y-5">
      <div className="flex items-center gap-2">
        <h1 className="font-mono text-base font-semibold text-text-primary">{d.addr}</h1>
        {d.asn && (
          <Link to={`/asn/${d.asn}`}>
            <Badge variant="brand">{d.org || `AS${d.asn}`}</Badge>
          </Link>
        )}
      </div>

      {/* Trend chart */}
      <Card>
        <CardHeader>
          <CardTitle>Health Trend</CardTitle>
          <div className="flex gap-1 rounded-lg border border-border-primary bg-bg-tertiary p-1">
            {(["loss_pct", "avg_rtt_ms"] as const).map((m) => (
              <button
                key={m}
                onClick={() => setMetric(m)}
                className={`rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
                  metric === m ? "bg-brand-500 text-white" : "text-text-tertiary hover:text-text-primary"
                }`}
              >
                {m === "loss_pct" ? "Loss %" : "RTT"}
              </button>
            ))}
          </div>
        </CardHeader>
        <CardContent>
          {series.isLoading ? (
            <LoadingState />
          ) : (
            <ResponsiveContainer width="100%" height={220}>
              <LineChart data={series.data ?? []}>
                <CartesianGrid stroke="var(--color-border-primary)" strokeDasharray="3 3" vertical={false} />
                <XAxis
                  dataKey="ts"
                  tickFormatter={(t) => new Date(t).toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" })}
                  tick={{ fill: "var(--color-text-quaternary)", fontSize: 11 }}
                  stroke="var(--color-border-primary)"
                />
                <YAxis
                  tick={{ fill: "var(--color-text-quaternary)", fontSize: 11 }}
                  stroke="var(--color-border-primary)"
                  width={40}
                />
                <Tooltip
                  contentStyle={{
                    background: "var(--color-bg-elevated)",
                    border: "1px solid var(--color-border-secondary)",
                    borderRadius: 8,
                    fontSize: 12,
                  }}
                  labelStyle={{ color: "var(--color-text-tertiary)" }}
                />
                <Line
                  type="monotone"
                  dataKey="value"
                  stroke={CHART_COLORS[0]}
                  strokeWidth={2}
                  dot={false}
                  name={metric === "loss_pct" ? "Loss %" : "RTT (ms)"}
                />
              </LineChart>
            </ResponsiveContainer>
          )}
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        {/* Probes reaching this target */}
        <Card>
          <CardHeader>
            <CardTitle>Probes ({d.probes.length})</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <THead>
                <tr>
                  <Th>Probe</Th>
                  <Th>Source AS</Th>
                  <Th className="text-right">Loss</Th>
                  <Th className="text-right">RTT</Th>
                </tr>
              </THead>
              <TBody>
                {d.probes.map((p) => (
                  <Tr key={p.id}>
                    <Td>
                      <Link to={`/probe/${p.id}`} className="font-mono text-xs text-brand-300 hover:text-brand-500">
                        {p.id}
                      </Link>
                    </Td>
                    <Td>
                      {p.src_asn ? (
                        <Link to={`/asn/${p.src_asn}`} className="text-xs text-text-secondary hover:text-brand-300">
                          {p.src_org || `AS${p.src_asn}`}
                        </Link>
                      ) : (
                        <span className="text-xs text-text-quaternary">—</span>
                      )}
                    </Td>
                    <Td className={`text-right font-mono text-xs ${lossColor(p.loss_pct)}`}>
                      {fmtPct(p.loss_pct)}
                    </Td>
                    <Td className="text-right font-mono text-xs text-text-tertiary">{fmtRtt(p.avg_rtt_ms)}</Td>
                  </Tr>
                ))}
              </TBody>
            </Table>
          </CardContent>
        </Card>

        {/* Nearby hops */}
        <Card>
          <CardHeader>
            <CardTitle>Nearby Hops ({d.nearby_hops.length})</CardTitle>
          </CardHeader>
          <CardContent>
            {d.nearby_hops.length === 0 ? (
              <p className="py-6 text-center text-xs text-text-quaternary">No adjacent NEXT_HOP edges</p>
            ) : (
              <Table>
                <THead>
                  <tr>
                    <Th>From → To</Th>
                    <Th className="text-right">RTT</Th>
                    <Th className="text-right">Observations</Th>
                  </tr>
                </THead>
                <TBody>
                  {d.nearby_hops.map((h, i) => (
                    <Tr key={i}>
                      <Td className="font-mono text-xs text-text-tertiary">
                        <Link to={`/ip/${h.from_addr}`} className="hover:text-brand-300">{h.from_addr}</Link>
                        {" → "}
                        <Link to={`/ip/${h.to_addr}`} className="hover:text-brand-300">{h.to_addr}</Link>
                      </Td>
                      <Td className="text-right font-mono text-xs text-text-tertiary">{fmtRtt(h.last_rtt_ms)}</Td>
                      <Td className="text-right font-mono text-xs text-text-quaternary">{h.seen_count}</Td>
                    </Tr>
                  ))}
                </TBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
