import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Bell, CheckCircle2, Plus, Trash2, Zap } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState, ErrorState, LoadingState } from "@/components/ui/states";
import { fmtRelative } from "@/lib/utils";

const API = "/api/alerts";

// ---- minimal typed fetch for alerts (no shared client types yet) -----------
async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(API + path);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const body = await res.json();
  return body.data as T;
}

interface RuleView {
  id: number;
  name: string;
  metric: string;
  comparison: string;
  threshold: number;
  scope: string;
  enabled: boolean;
}
interface ActiveAlert {
  rule_id: number;
  rule_name: string;
  scope_key: string;
  value: number;
  fired_at: string;
}
interface EventView {
  id: number;
  rule_name: string;
  type: string;
  scope_key: string;
  value: number;
  ts: string;
}

export function AlertsPage() {
  const qc = useQueryClient();
  const rules = useQuery({ queryKey: ["alert-rules"], queryFn: () => fetchJSON<RuleView[]>("/rules") });
  const active = useQuery({ queryKey: ["alert-active"], queryFn: () => fetchJSON<ActiveAlert[]>("/active") });
  const events = useQuery({ queryKey: ["alert-events"], queryFn: () => fetchJSON<EventView[]>("/events?limit=50") });

  const del = useMutation({
    mutationFn: async (id: number) => {
      const res = await fetch(`${API}/rules/${id}`, { method: "DELETE" });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alert-rules"] }),
  });

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold text-text-primary">Alerts</h1>
          <p className="text-sm text-text-quaternary">Threshold rules evaluated against the live graph every minute</p>
        </div>
      </div>

      {/* Active firing */}
      <Card>
        <CardHeader>
          <CardTitle>
            <span className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4 text-error-500" />
              Firing Now
            </span>
          </CardTitle>
          <Badge variant={active.data?.length ? "danger" : "success"}>
            {active.data?.length ?? 0}
          </Badge>
        </CardHeader>
        <CardContent>
          {active.isLoading ? (
            <LoadingState />
          ) : active.data && active.data.length > 0 ? (
            <div className="space-y-1.5">
              {active.data.map((a, i) => {
                const href = scopeKeyToHref(a.scope_key);
                const scopeLabel = scopeKeyLabel(a.scope_key);
                return (
                  <div key={i} className="flex items-center justify-between rounded-lg bg-error-500/5 px-3 py-2">
                    <div className="min-w-0">
                      <div className="text-sm font-medium text-text-primary">{a.rule_name}</div>
                      <div className="text-xs text-text-tertiary">{scopeLabel}</div>
                    </div>
                    <div className="flex items-center gap-3">
                      <span className="font-mono text-sm font-semibold text-error-500">{a.value.toFixed(0)}</span>
                      <span className="text-xs text-text-quaternary">{fmtRelative(Date.parse(a.fired_at) / 1000)}</span>
                      {href && (
                        <Link
                          to={href}
                          className="rounded-md border border-border-primary px-2 py-0.5 text-xs text-brand-300 hover:border-border-secondary hover:text-brand-500"
                        >
                          Investigate
                        </Link>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          ) : (
            <EmptyState label="No alerts firing" hint="All clear" />
          )}
        </CardContent>
      </Card>

      {/* Rule builder */}
      <RuleBuilder onCreated={() => qc.invalidateQueries({ queryKey: ["alert-rules"] })} />

      {/* Saved rules list */}
      <Card>
        <CardHeader>
          <CardTitle>Saved rules</CardTitle>
        </CardHeader>
        <CardContent>
          {rules.isLoading && <LoadingState />}
          {rules.error && <ErrorState message={(rules.error as Error).message} />}
          {rules.data && rules.data.length === 0 && (
            <EmptyState label="No rules yet" hint="Create one above" />
          )}
          {rules.data && rules.data.length > 0 && (
            <div className="space-y-1.5">
              {rules.data.map((r) => (
                <div key={r.id} className="flex items-center justify-between rounded-lg border border-border-primary px-3 py-2.5">
                  <div className="flex items-center gap-3">
                    {r.enabled ? (
                      <Zap className="h-4 w-4 text-success-500" />
                    ) : (
                      <Zap className="h-4 w-4 text-text-quaternary" />
                    )}
                    <div>
                      <div className="text-sm font-medium text-text-primary">{r.name}</div>
                      <div className="text-xs text-text-quaternary">
                        {describeRule(r.metric, r.comparison, String(r.threshold), r.scope)}
                      </div>
                    </div>
                  </div>
                  <Button variant="ghost" size="icon" onClick={() => del.mutate(r.id)}>
                    <Trash2 className="h-4 w-4 text-text-quaternary hover:text-error-500" />
                  </Button>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Event history */}
      <Card>
        <CardHeader>
          <CardTitle>
            <span className="flex items-center gap-2">
              <Bell className="h-4 w-4 text-text-tertiary" />
              Recent Events
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {events.isLoading ? (
            <LoadingState />
          ) : events.data && events.data.length > 0 ? (
            <div className="space-y-1">
              {events.data.map((e) => (
                <div key={e.id} className="flex items-center gap-3 px-2 py-1.5 text-sm">
                  {e.type === "fired" ? (
                    <AlertTriangle className="h-3.5 w-3.5 text-error-500" />
                  ) : (
                    <CheckCircle2 className="h-3.5 w-3.5 text-success-500" />
                  )}
                  <span className="font-medium text-text-secondary">{e.rule_name}</span>
                  <span className="font-mono text-xs text-text-tertiary">{e.scope_key}</span>
                  <span className="ml-auto text-xs text-text-quaternary">{fmtRelative(Date.parse(e.ts) / 1000)}</span>
                </div>
              ))}
            </div>
          ) : (
            <EmptyState label="No events yet" />
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function RuleBuilder({ onCreated }: { onCreated: () => void }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [metric, setMetric] = useState("loss_ratio");
  const [comparison, setComparison] = useState(">=");
  const [threshold, setThreshold] = useState("50");
  const [scope, setScope] = useState("asn_dst");
  const [error, setError] = useState<string | null>(null);
  const qc = useQueryClient();

  async function create(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const res = await fetch(`${API}/rules`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        name,
        metric,
        comparison,
        threshold: Number(threshold),
        scope,
        enabled: true,
      }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      setError(body.error || `HTTP ${res.status}`);
      return;
    }
    setName("");
    setOpen(false);
    qc.invalidateQueries({ queryKey: ["alert-rules"] });
    onCreated();
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Create rule</CardTitle>
        {!open && (
          <Button variant="secondary" size="sm" onClick={() => setOpen(true)}>
            <Plus className="h-4 w-4" /> New rule
          </Button>
        )}
      </CardHeader>
      {open && (
        <CardContent>
          <form onSubmit={create} className="space-y-3">
            {/* Templates — fill metric/scope/threshold, user still edits before saving */}
            <div>
              <label className="mb-1 block text-xs font-medium text-text-quaternary">Templates (optional)</label>
              <div className="flex flex-wrap gap-2">
                {TEMPLATES.map((t) => (
                  <button
                    key={t.name}
                    type="button"
                    onClick={() => {
                      setMetric(t.metric);
                      setComparison(t.comparison);
                      setThreshold(String(t.threshold));
                      setScope(t.scope);
                      if (!name) setName(t.name);
                    }}
                    className="rounded-lg border border-border-primary bg-bg-tertiary px-2.5 py-1 text-xs text-text-secondary hover:border-brand-500 hover:text-brand-300"
                  >
                    {t.name}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-text-quaternary">Name</label>
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. High loss to Cloudflare"
                className="w-full rounded-lg border border-border-primary bg-bg-tertiary px-3 py-2 text-sm text-text-primary placeholder:text-text-quaternary focus:border-brand-500 focus:outline-none"
              />
            </div>
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
              <Field label="Metric">
                <Select value={metric} onChange={setMetric} options={[
                  ["loss_ratio", "Loss %"], ["avg_rtt_ms", "RTT (ms)"], ["target_lost", "Target lost"],
                ]} />
              </Field>
              <Field label="When">
                <Select value={comparison} onChange={setComparison} options={[
                  [">=", "≥"], [">", ">"], ["<", "<"], ["<=", "≤"], ["==", "="],
                ]} />
              </Field>
              <Field label={`Threshold (${thresholdUnit(metric)})`}>
                <input
                  type="number"
                  value={threshold}
                  onChange={(e) => setThreshold(e.target.value)}
                  className="w-full rounded-lg border border-border-primary bg-bg-tertiary px-3 py-2 text-sm text-text-primary placeholder:text-text-quaternary focus:border-brand-500 focus:outline-none"
                />
              </Field>
              <Field label="Scope">
                <Select value={scope} onChange={setScope} options={[
                  ["asn_dst", "Per dst AS"], ["asn_pair", "Per AS pair"], ["target", "Per target IP"], ["probe_target", "Per probe→target"],
                ]} />
              </Field>
            </div>
            {/* Human-readable preview of the rule condition */}
            <div className="rounded-lg border border-border-primary bg-bg-tertiary px-3 py-2 text-sm text-text-secondary">
              {describeRule(metric, comparison, threshold, scope)}
            </div>
            {error && <p className="text-sm text-error-500">{error}</p>}
            <div className="flex justify-end gap-2">
              <Button type="button" variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={!name}>Create rule</Button>
            </div>
          </form>
        </CardContent>
      )}
    </Card>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-text-quaternary">{label}</label>
      {children}
    </div>
  );
}

function Select({ value, onChange, options }: { value: string; onChange: (v: string) => void; options: [string, string][] }) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="w-full rounded-lg border border-border-primary bg-bg-tertiary px-3 py-2 text-sm text-text-primary focus:border-brand-500 focus:outline-none"
    >
      {options.map(([v, l]) => (
        <option key={v} value={v}>{l}</option>
      ))}
    </select>
  );
}

// thresholdUnit returns the display unit for a metric's threshold field.
function thresholdUnit(metric: string): string {
  switch (metric) {
    case "loss_ratio":
      return "% loss";
    case "avg_rtt_ms":
      return "ms";
    case "target_lost":
      return "% loss (use 100)";
    default:
      return "";
  }
}

// TEMPLATES provide one-click rule presets for common alerting cases. The user
// still sees/edits the human-readable condition before saving.
const TEMPLATES: { name: string; metric: string; comparison: string; threshold: number; scope: string }[] = [
  { name: "Broad destination loss", metric: "loss_ratio", comparison: ">=", threshold: 50, scope: "asn_dst" },
  { name: "Source network loss", metric: "loss_ratio", comparison: ">=", threshold: 50, scope: "asn_dst" },
  { name: "High latency on target", metric: "avg_rtt_ms", comparison: ">=", threshold: 250, scope: "target" },
  { name: "Target unreachable", metric: "target_lost", comparison: ">=", threshold: 100, scope: "target" },
  { name: "Probe-to-target degradation", metric: "loss_ratio", comparison: ">=", threshold: 50, scope: "probe_target" },
];

// scopeKeyToHref maps an alert scope_key to an existing detail-page route.
// Returns "" when no safe drill-down exists (the UI shows a disabled state).
function scopeKeyToHref(scopeKey: string): string {
  const { href } = parseScopeKey(scopeKey);
  return href;
}

// scopeKeyLabel renders a human-readable label for an alert scope.
function scopeKeyLabel(scopeKey: string): string {
  const { label } = parseScopeKey(scopeKey);
  return label;
}

// parseScopeKey converts a scope_key into a readable label + href.
// asn_dst scopes are bare ASN numbers; asn_pair look like "123→456".
function parseScopeKey(scopeKey: string): { label: string; href: string } {
  const n = Number(scopeKey);
  if (!Number.isNaN(n) && n > 0) {
    return { label: `AS${n}`, href: `/asn/${n}` };
  }
  const parts = scopeKey.split("→");
  if (parts.length === 2) {
    return { label: `AS pair ${scopeKey}`, href: "" };
  }
  return { label: scopeKey, href: "" };
}

// describeRule produces a human-readable condition string for the rule builder
// preview and (later) saved-rule display. Keeps the unit convention explicit:
// loss is a percentage, RTT is ms, target_lost is a 100% condition.
function describeRule(metric: string, comparison: string, threshold: string, scope: string): string {
  const cmpWord: Record<string, string> = {
    ">=": "at least", ">": "more than", "<": "under", "<=": "at most", "==": "exactly",
  };
  const scopeWord: Record<string, string> = {
    asn_dst: "a destination AS", asn_pair: "an AS pair",
    target: "a target IP", probe_target: "a probe→target pair",
  };
  const cw = cmpWord[comparison] ?? comparison;
  const sw = scopeWord[scope] ?? scope;
  switch (metric) {
    case "loss_ratio":
      return `Alert when ${sw} has observed packet loss of ${cw} ${threshold}% (0–100).`;
    case "avg_rtt_ms":
      return `Alert when ${sw} has average RTT of ${cw} ${threshold} ms.`;
    case "target_lost":
      return `Alert when ${sw} has no replies (target lost, 100% loss).`;
    default:
      return "";
  }
}
