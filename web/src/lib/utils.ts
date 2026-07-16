import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

// cn merges Tailwind classes with conditional logic (shadcn convention).
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// fmtNum formats integers with thousands separators.
export function fmtNum(n: number | null | undefined): string {
  if (n == null) return "—";
  return new Intl.NumberFormat("en-US").format(n);
}

// fmtPct formats a 0-100 float to a percentage string.
export function fmtPct(n: number | null | undefined, digits = 0): string {
  if (n == null || Number.isNaN(n)) return "—";
  return `${n.toFixed(digits)}%`;
}

// fmtRtt formats RTT in ms; -1 means "target lost / no reply" (FalkorDB sentinel).
export function fmtRtt(ms: number | null | undefined): string {
  if (ms == null) return "—";
  if (ms < 0) return "lost";
  if (ms < 10) return `${ms.toFixed(1)} ms`;
  return `${Math.round(ms)} ms`;
}

// fmtEpoch converts a unix-seconds epoch to a compact local time string.
export function fmtEpoch(sec: number | null | undefined): string {
  if (!sec || sec <= 0) return "—";
  return new Date(sec * 1000).toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

// fmtRelative converts a unix-seconds epoch to "3m ago" style relative time.
export function fmtRelative(sec: number | null | undefined): string {
  if (!sec || sec <= 0) return "—";
  const diff = Date.now() / 1000 - sec;
  if (diff < 60) return `${Math.round(diff)}s ago`;
  if (diff < 3600) return `${Math.round(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.round(diff / 3600)}h ago`;
  return `${Math.round(diff / 86400)}d ago`;
}

// lossColor returns a Tailwind text-color class by loss severity.
export function lossColor(pct: number | null | undefined): string {
  if (pct == null) return "text-text-tertiary";
  if (pct >= 100) return "text-error-500";
  if (pct >= 50) return "text-warning-500";
  if (pct > 0) return "text-warning-600";
  return "text-success-500";
}

// fmtUtc renders a unix-seconds epoch as an explicit UTC timestamp string,
// for use in tooltips/titles alongside a relative time.
export function fmtUtc(sec: number | null | undefined): string {
  if (!sec || sec <= 0) return "—";
  return new Date(sec * 1000).toISOString().replace("T", " ").replace(".000Z", " UTC");
}

// fmtPctOf renders "n (pct%)" where pct = n/total*100, used for count+share
// displays that need both numerator and denominator context.
export function fmtPctOf(n: number, total: number, digits = 1): string {
  if (total <= 0) return `${fmtNum(n)} (—)`;
  return `${fmtNum(n)} (${fmtPct((n / total) * 100, digits)})`;
}

// FreshnessProps is the input to the Freshness component.
export interface FreshnessProps {
  // Unix seconds; 0/null means "no observation available".
  sec: number | null | undefined;
  // When true, show as "no observation" rather than a relative time. Default true.
  showUnknown?: boolean;
}

// severityFromLoss returns a severity label string for a loss percentage using
// the documented thresholds (critical >= 80, high >= 20, watch > 0, else normal).
// Returns "unknown" when loss is null/undefined. NOTE: temporary client-side
// thresholds; /api/issues will own severity once Milestone 2 lands.
export function severityFromLoss(lossPct: number | null | undefined): {
  level: "critical" | "high" | "watch" | "normal" | "unknown";
  label: string;
} {
  if (lossPct == null) return { level: "unknown", label: "Unknown" };
  if (lossPct >= 80) return { level: "critical", label: "Critical" };
  if (lossPct >= 20) return { level: "high", label: "High" };
  if (lossPct > 0) return { level: "watch", label: "Watch" };
  return { level: "normal", label: "Normal" };
}
