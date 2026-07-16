// Shared health-presentation primitives. These establish one visual language
// for severity, metrics, freshness, evidence, and data status across every page.
//
// Severity is text + icon + color (never color alone). The thresholds here are
// temporary client-side values until /api/issues owns severity (Milestone 2).

export type SeverityLevel = "critical" | "high" | "watch" | "normal" | "unknown";

export interface SeverityInfo {
  level: SeverityLevel;
  label: string;
  // Tailwind classes for badge background/text and the indicator dot.
  badge: string;
  dot: string;
  // Short definition shown in tooltips.
  def: string;
}

// SEVERITY defines the single source of truth for severity presentation.
// Temporary thresholds (documented): critical >= 80% loss, high >= 20%, watch > 0%.
// Do not infer severity when evidence (probe/sample count) is insufficient.
export const SEVERITY: Record<SeverityLevel, SeverityInfo> = {
  critical: {
    level: "critical",
    label: "Critical",
    badge: "bg-error-500/15 text-error-500 border-error-500/30",
    dot: "bg-error-500",
    def: "Critical: 80% or greater observed loss with sufficient evidence.",
  },
  high: {
    level: "high",
    label: "High",
    badge: "bg-warning-500/15 text-warning-500 border-warning-500/30",
    dot: "bg-warning-500",
    def: "High: 20%–79% observed loss with sufficient evidence.",
  },
  watch: {
    level: "watch",
    label: "Watch",
    badge: "bg-info-500/15 text-info-500 border-info-500/30",
    dot: "bg-info-500",
    def: "Watch: greater than 0% and below 20% observed loss.",
  },
  normal: {
    level: "normal",
    label: "Normal",
    badge: "bg-success-500/15 text-success-500 border-success-500/30",
    dot: "bg-success-500",
    def: "Normal: no observed loss.",
  },
  unknown: {
    level: "unknown",
    label: "Unknown",
    badge: "bg-bg-tertiary text-text-quaternary border-border-primary",
    dot: "bg-text-quaternary",
    def: "Unknown: insufficient data to assess.",
  },
};

// severityFromLossPct maps a loss percentage to a severity level. NOTE: this is
// a temporary heuristic; pass insufficient evidence through as "unknown".
export function severityFromLossPct(
  lossPct: number | null | undefined,
): SeverityInfo {
  if (lossPct == null) return SEVERITY.unknown;
  if (lossPct >= 80) return SEVERITY.critical;
  if (lossPct >= 20) return SEVERITY.high;
  if (lossPct > 0) return SEVERITY.watch;
  return SEVERITY.normal;
}
