import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, ArrowRight, ChevronDown } from "lucide-react";
import { Link } from "react-router-dom";
import { useState } from "react";
import { api, type Issue } from "@/api/client";
import { DataStatus, Evidence, SEVERITY, SeverityBadge } from "@/components/health";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState, ErrorState, LoadingState } from "@/components/ui/states";
import { FEATURES } from "@/lib/features";
import { fmtNum, fmtPct } from "@/lib/utils";

export function OverviewPage() {
  const overview = useQuery({ queryKey: ["overview"], queryFn: api.overview });
  const issues = useQuery({ queryKey: ["issues"], queryFn: () => api.issues(10) });

  if (overview.isLoading) return <LoadingState label="Loading network health…" />;
  if (overview.error) return <ErrorState message={(overview.error as Error).message} />;
  if (!overview.data) return null;
  const { data: o, meta } = overview.data;

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold text-text-primary">Network health</h1>
          <DataStatus meta={meta} />
        </div>
        {FEATURES.alerts && (
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
        )}
      </div>

      {/* First: Needs attention queue */}
      <NeedsAttention issues={issues} />

      {/* Second: Health at a glance */}
      <HealthAtAGlance overview={o} />

      {/* Third: Diagnose by pattern */}
      <DiagnoseByPattern overview={o} />
    </div>
  );
}

// ---- Needs attention --------------------------------------------------------

function NeedsAttention({
  issues,
}: {
  issues: ReturnType<typeof useQuery<Issue[]>>;
}) {
  return (
    <section aria-labelledby="attention-heading">
      <div className="mb-2 flex items-center justify-between">
        <h2 id="attention-heading" className="text-sm font-semibold text-text-primary">
          Needs attention
        </h2>
        {issues.data && issues.data.length > 0 && (
          <Badge variant="neutral">{issues.data.length}</Badge>
        )}
      </div>
      {issues.isLoading && <LoadingState label="Detecting issues…" />}
      {issues.error && <ErrorState message={(issues.error as Error).message} />}
      {issues.data && issues.data.length === 0 && (
        <EmptyState
          label="No issues meet the current evidence threshold"
          hint="The network may be healthy, or issues lack sufficient probes/samples to rank."
        />
      )}
      {issues.data && issues.data.length > 0 && (
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          {issues.data.map((issue) => (
            <IssueCard key={issue.id} issue={issue} />
          ))}
        </div>
      )}
    </section>
  );
}

function IssueCard({ issue }: { issue: Issue }) {
  return (
    <Card className="flex flex-col gap-3 p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <SeverityBadge severity={SEVERITY[issue.severity]} />
          <h3 className="mt-1.5 text-sm font-medium text-text-primary">{issue.title}</h3>
        </div>
        {issue.source === "alert" && (
          <Badge variant="danger">Alert</Badge>
        )}
      </div>
      <p className="text-sm text-text-secondary">{issue.summary}</p>
      <Evidence probes={issue.probe_count} samples={issue.sample_count} lastSeenSec={issue.last_seen} />
      {issue.evidence.length > 0 && (
        <ul className="space-y-0.5 text-xs text-text-quaternary">
          {issue.evidence.slice(0, 2).map((e, i) => (
            <li key={i} className="flex gap-1.5">
              <span className="text-text-quaternary">•</span> {e}
            </li>
          ))}
        </ul>
      )}
      <Link
        to={issue.href}
        className="mt-auto flex items-center gap-1 text-xs font-medium text-brand-300 hover:text-brand-500"
      >
        Investigate <ArrowRight className="h-3 w-3" />
      </Link>
    </Card>
  );
}

// ---- Health at a glance -----------------------------------------------------

function HealthAtAGlance({ overview: o }: { overview: import("@/api/client").Overview }) {
  const totalEdges = o.global_loss.healthy_edges + o.global_loss.lossy_edges + o.global_loss.lost_edges;
  const [showCoverage, setShowCoverage] = useState(false);

  return (
    <section aria-labelledby="health-heading">
      <h2 id="health-heading" className="mb-2 text-sm font-semibold text-text-primary">
        Health at a glance
      </h2>
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <HealthMetric
          label="Affected edges"
          value={fmtNum(o.global_loss.lossy_edges + o.global_loss.lost_edges)}
          secondary={`of ${fmtNum(totalEdges)} active PING`}
        />
        <HealthMetric
          label="Target-lost rate"
          value={fmtPct(totalEdges > 0 ? (o.global_loss.lost_edges / totalEdges) * 100 : 0, 1)}
          secondary={`${fmtNum(o.global_loss.lost_edges)} edges`}
        />
        <HealthMetric
          label="Avg loss"
          value={fmtPct(o.global_loss.avg_loss_pct, 1)}
          secondary="across all PING edges"
        />
        <HealthMetric
          label="Lossy edges"
          value={fmtNum(o.global_loss.lossy_edges)}
          secondary="0%–100% partial loss"
        />
      </div>

      {/* Collapsible network coverage (inventory counts) */}
      <button
        type="button"
        onClick={() => setShowCoverage(!showCoverage)}
        className="mt-2 flex items-center gap-1 text-xs text-text-quaternary hover:text-text-secondary"
      >
        <ChevronDown className={`h-3 w-3 transition-transform ${showCoverage ? "rotate-180" : ""}`} />
        Network coverage
      </button>
      {showCoverage && (
        <div className="mt-2 grid grid-cols-2 gap-2 rounded-lg border border-border-primary bg-bg-secondary p-3 text-xs md:grid-cols-4">
          <CoverageStat label="Probes" value={fmtNum(o.counts.probes)} />
          <CoverageStat label="Targets" value={fmtNum(o.counts.targets)} />
          <CoverageStat label="ASes" value={fmtNum(o.counts.ases)} />
          <CoverageStat label="IPs" value={fmtNum(o.counts.ips)} />
          <CoverageStat label="NEXT_HOP" value={fmtNum(o.counts.next_hop_edges)} />
          <CoverageStat label="PING edges" value={fmtNum(o.counts.ping_edges)} />
          <CoverageStat label="Transit" value={fmtNum(o.counts.transit_edges)} />
          <CoverageStat label="Lossy PING" value={fmtNum(o.counts.lossy_pings)} />
        </div>
      )}
      {showCoverage && (
        <p className="mt-1 text-[11px] text-text-quaternary">
          Graph inventory counts, not current health indicators.
        </p>
      )}
    </section>
  );
}

function HealthMetric({ label, value, secondary }: { label: string; value: string; secondary: string }) {
  return (
    <Card className="px-4 py-3">
      <div className="text-xs text-text-quaternary">{label}</div>
      <div className="text-lg font-semibold text-text-primary">{value}</div>
      <div className="text-[11px] text-text-quaternary">{secondary}</div>
    </Card>
  );
}

function CoverageStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col">
      <span className="text-text-quaternary">{label}</span>
      <span className="font-mono text-text-secondary">{value}</span>
    </div>
  );
}

// ---- Diagnose by pattern ----------------------------------------------------

function DiagnoseByPattern({ overview: o }: { overview: import("@/api/client").Overview }) {
  const dst = (o.top_dst_as ?? []).slice(0, 5);
  const src = (o.top_src_as ?? []).slice(0, 5);
  const pairs = (o.top_as_pairs ?? []).slice(0, 5);

  return (
    <section aria-labelledby="pattern-heading">
      <h2 id="pattern-heading" className="mb-2 text-sm font-semibold text-text-primary">
        Diagnose by pattern
      </h2>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <PatternCard
          title="Destination-wide"
          def="Many source networks failing to reach one destination AS."
          issues={dst}
          linkPrefix="/asn/"
          linkKey="asn"
        />
        <PatternCard
          title="Source-specific"
          def="One source network failing across many destinations."
          issues={src}
          linkPrefix="/asn/"
          linkKey="asn"
        />
        <PatternCard
          title="AS pairs"
          def="Loss concentrated on a specific source→destination AS pair."
          issues={pairs}
          linkPrefix="/transit/"
          linkKey="pair"
        />
      </div>
    </section>
  );
}

function PatternCard({
  title,
  def,
  issues,
  linkPrefix,
  linkKey,
}: {
  title: string;
  def: string;
  issues: (import("@/api/client").ASNIssue | import("@/api/client").ASNPairIssue)[];
  linkPrefix: string;
  linkKey: "asn" | "pair";
}) {
  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>{title}</CardTitle>
          <p className="text-xs text-text-quaternary">{def}</p>
        </div>
      </CardHeader>
      <CardContent>
        {issues.length === 0 ? (
          <p className="py-3 text-center text-xs text-text-quaternary">No qualifying patterns</p>
        ) : (
          <div className="space-y-1">
            {issues.map((a, i) => {
              const asn = "asn" in a ? a.asn : 0;
              const org = "org" in a
                ? a.org
                : `${a.src_org || `AS${a.src_asn}`} → ${a.dst_org || `AS${a.dst_asn}`}`;
              const href = linkKey === "pair" && "src_asn" in a ? `/transit/${a.src_asn}/${a.dst_asn}` : `${linkPrefix}${asn}`;
              return (
                <Link
                  key={i}
                  to={href}
                  className="flex items-center justify-between gap-2 rounded-lg px-2 py-1.5 hover:bg-bg-tertiary"
                >
                  <span className="truncate text-sm text-text-secondary">{org || `AS${asn}`}</span>
                  <span className="font-mono text-xs text-text-tertiary">{fmtPct(a.avg_loss_pct)}</span>
                </Link>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
