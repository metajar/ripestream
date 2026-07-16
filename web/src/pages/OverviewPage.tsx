import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, ArrowRight, Globe2, Network, Satellite, Target } from "lucide-react";
import { Link } from "react-router-dom";
import { api } from "@/api/client";
import { DataStatus, Evidence, SeverityBadge, severityFromLossPct } from "@/components/health";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ErrorState, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { fmtNum, fmtPct, lossColor } from "@/lib/utils";

export function OverviewPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["overview"],
    queryFn: api.overview,
  });

  if (isLoading) return <LoadingState label="Loading internet overview…" />;
  if (error) return <ErrorState message={(error as Error).message} />;
  if (!data) return null;
  const { data: o, meta } = data;

  return (
    <div className="space-y-5">
      {/* Header: title, freshness status, active alerts */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold text-text-primary">Network health</h1>
          <DataStatus meta={meta} />
        </div>
        <Link
          to="/alerts"
          className={`flex items-center gap-2 rounded-lg border px-3 py-1.5 text-sm transition-colors ${
            o.active_alerts > 0
              ? "border-error-500/30 bg-error-500/10 text-error-500"
              : "border-border-primary bg-bg-secondary text-text-tertiary"
          } hover:border-border-secondary`}
        >
          <AlertTriangle className="h-4 w-4" />
          <span className="font-medium">{o.active_alerts}</span>
          <span className="text-xs">{o.active_alerts === 1 ? "active alert" : "active alerts"}</span>
        </Link>
      </div>

      {/* KPI cards */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <Kpi icon={Satellite} label="Probes" value={fmtNum(o.counts.probes)} />
        <Kpi icon={Target} label="Targets" value={fmtNum(o.counts.targets)} />
        <Kpi icon={Globe2} label="ASes" value={fmtNum(o.counts.ases)} />
        <Kpi icon={Network} label="IPs" value={fmtNum(o.counts.ips)} />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        {/* Global loss composition */}
        <Card className="lg:col-span-1">
          <CardHeader>
            <CardTitle>Loss Composition</CardTitle>
            <Badge variant={o.global_loss.avg_loss_pct > 10 ? "danger" : "warning"}>
              avg {fmtPct(o.global_loss.avg_loss_pct, 1)}
            </Badge>
          </CardHeader>
          <CardContent>
            <LossComposition
              healthy={o.global_loss.healthy_edges}
              lossy={o.global_loss.lossy_edges}
              lost={o.global_loss.lost_edges}
            />
            <div className="mt-3 border-t border-border-primary pt-3 text-xs text-text-quaternary">
              Across {fmtNum(o.counts.ping_edges)} active PING edges. Values reflect the latest
              observed PING-edge state, not a historical time-window rate.
            </div>
          </CardContent>
        </Card>

        {/* Top dst AS issues */}
        <ASIssuesCard
          title="Top Problem Destinations"
          subtitle="dst ASes by loss"
          issues={o.top_dst_as}
        />

        {/* Top src AS issues */}
        <ASIssuesCard
          title="Top Problem Sources"
          subtitle="src ASes by loss"
          issues={o.top_src_as}
        />
      </div>

      {/* Top AS pairs */}
      <Card>
        <CardHeader>
          <CardTitle>Lossiest AS Pairs</CardTitle>
          <Link
            to="/transit"
            className="flex items-center gap-1 text-xs font-medium text-brand-300 hover:text-brand-500"
          >
            Transit view <ArrowRight className="h-3 w-3" />
          </Link>
        </CardHeader>
        <CardContent>
          <Table>
            <THead>
              <tr>
                <Th>Source AS</Th>
                <Th>Destination AS</Th>
                <Th className="text-right">Samples</Th>
                <Th className="text-right">Probes</Th>
                <Th className="text-right">Avg Loss</Th>
                <Th className="text-right">RTT</Th>
              </tr>
            </THead>
            <TBody>
              {o.top_as_pairs.map((p) => (
                <Tr key={`${p.src_asn}-${p.dst_asn}`}>
                  <Td>
                    <ASLink asn={p.src_asn} org={p.src_org} />
                  </Td>
                  <Td>
                    <ASLink asn={p.dst_asn} org={p.dst_org} />
                  </Td>
                  <Td className="text-right font-mono text-xs">{fmtNum(p.samples)}</Td>
                  <Td className="text-right font-mono text-xs">{fmtNum(p.probes)}</Td>
                  <Td className={`text-right font-mono text-xs font-medium ${lossColor(p.avg_loss_pct)}`}>
                    {fmtPct(p.avg_loss_pct)}
                  </Td>
                  <Td className="text-right font-mono text-xs text-text-tertiary">
                    {p.avg_rtt_ms >= 0 ? `${Math.round(p.avg_rtt_ms)}ms` : "—"}
                  </Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}

function Kpi({ icon: Icon, label, value }: { icon: React.ComponentType<{ className?: string }>; label: string; value: string }) {
  return (
    <Card className="px-4 py-3">
      <div className="flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-bg-tertiary">
          <Icon className="h-4 w-4 text-brand-300" />
        </div>
        <div>
          <div className="text-lg font-semibold text-text-primary">{value}</div>
          <div className="text-xs text-text-quaternary">{label}</div>
        </div>
      </div>
    </Card>
  );
}

// LossComposition renders a stacked bar of healthy/lossy/lost PING edges, with
// each segment width computed against the total (NOT value/value, which was the
// prior bug that filled every bar to 100%). Each row shows both count and %.
// When there are zero edges it renders an explicit no-data state.
function LossComposition({ healthy, lossy, lost }: { healthy: number; lossy: number; lost: number }) {
  const total = healthy + lossy + lost;
  if (total === 0) {
    return (
      <div className="py-6 text-center text-sm text-text-quaternary">No active PING edges</div>
    );
  }
  const seg = (n: number) => `${(n / total) * 100}%`;
  const row = (label: string, n: number, color: string, def: string) => (
    <div className="flex items-center justify-between gap-3">
      <span className="flex items-center gap-2 text-sm text-text-secondary">
        <span className={`inline-block h-2.5 w-2.5 rounded-sm ${color}`} aria-hidden />
        {label}
      </span>
      <span className="font-mono text-xs text-text-tertiary">
        {fmtNum(n)} <span className="text-text-quaternary">({fmtPct((n / total) * 100, 1)})</span>
      </span>
      <span className="sr-only">{def}</span>
    </div>
  );
  return (
    <div className="space-y-3">
      {/* Stacked composition bar: segment widths are fractions of the total. */}
      <div
        role="img"
        aria-label={`Loss composition of ${total} active PING edges: ${healthy} healthy (${fmtPct(
          (healthy / total) * 100,
          1,
        )}), ${lossy} lossy (${fmtPct((lossy / total) * 100, 1)}), ${lost} target lost (${fmtPct(
          (lost / total) * 100,
          1,
        )}).`}
        className="flex h-2.5 w-full overflow-hidden rounded-full bg-bg-tertiary"
      >
        <div className="h-full bg-success-500" style={{ width: seg(healthy) }} />
        <div className="h-full bg-warning-500" style={{ width: seg(lossy) }} />
        <div className="h-full bg-error-500" style={{ width: seg(lost) }} />
      </div>
      <div className="space-y-1.5">
        {row("Healthy", healthy, "bg-success-500", "Healthy: 0% observed loss")}
        {row("Lossy", lossy, "bg-warning-500", "Lossy: greater than 0% and less than 100% observed loss")}
        {row("Target lost", lost, "bg-error-500", "Target lost: 100% observed loss")}
      </div>
    </div>
  );
}

function ASIssuesCard({ title, subtitle, issues }: { title: string; subtitle: string; issues: import("@/api/client").ASNIssue[] }) {
  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>{title}</CardTitle>
          <p className="text-xs text-text-quaternary">{subtitle}</p>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-1">
          {issues.slice(0, 6).map((a) => (
            <Link
              key={a.asn}
              to={`/asn/${a.asn}`}
              className="flex items-center justify-between gap-3 rounded-lg px-2 py-1.5 hover:bg-bg-tertiary"
            >
              <div className="min-w-0">
                <div className="truncate text-sm font-medium text-text-primary">{a.org || `AS${a.asn}`}</div>
                <div className="text-xs text-text-quaternary">AS{a.asn}</div>
                <Evidence probes={a.probes} samples={a.samples} />
              </div>
              <div className="flex flex-col items-end gap-1">
                <SeverityBadge severity={severityFromLossPct(a.avg_loss_pct)} />
                <span className="font-mono text-xs text-text-tertiary">{fmtPct(a.avg_loss_pct)}</span>
              </div>
            </Link>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

function ASLink({ asn, org }: { asn: number; org: string }) {
  return (
    <Link to={`/asn/${asn}`} className="text-text-primary hover:text-brand-300">
      <span className="font-medium">{org || `AS${asn}`}</span>{" "}
      <span className="text-xs text-text-quaternary">AS{asn}</span>
    </Link>
  );
}
