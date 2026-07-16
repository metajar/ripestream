import { useCallback, useMemo } from "react";
import { useSearchParams } from "react-router-dom";

// InvestigationParams are the standardized query parameters that preserve
// operator filters and investigation origin across pages. Only parameters a
// receiving page actually reads should be added here.
export interface InvestigationParams {
  range?: string; // e.g. "1h", "24h", "7d"
  minLoss?: number;
  minProbes?: number;
  sort?: string;
  order?: "asc" | "desc";
  asn?: number;
  target?: string;
  probe?: number;
  fromIssue?: string; // id of the originating issue card
}

// Param keys — single source of truth for the query-string names.
export const PARAM_KEYS = [
  "range", "min_loss", "min_probes", "sort", "order",
  "asn", "target", "probe", "from_issue",
] as const;

// useInvestigation reads and writes the standardized filter/sort/context
// parameters from the URL search string. Writing merges into the existing
// params so unrelated params (e.g. React Router state) are preserved.
export function useInvestigation() {
  const [searchParams, setSearchParams] = useSearchParams();

  const params = useMemo<InvestigationParams>(() => {
    const p: InvestigationParams = {};
    const range = searchParams.get("range");
    if (range) p.range = range;
    const minLoss = searchParams.get("min_loss");
    if (minLoss) p.minLoss = Number(minLoss);
    const minProbes = searchParams.get("min_probes");
    if (minProbes) p.minProbes = Number(minProbes);
    const sort = searchParams.get("sort");
    if (sort) p.sort = sort;
    const order = searchParams.get("order");
    if (order === "asc" || order === "desc") p.order = order;
    const asn = searchParams.get("asn");
    if (asn) p.asn = Number(asn);
    const target = searchParams.get("target");
    if (target) p.target = target;
    const probe = searchParams.get("probe");
    if (probe) p.probe = Number(probe);
    const fromIssue = searchParams.get("from_issue");
    if (fromIssue) p.fromIssue = fromIssue;
    return p;
  }, [searchParams]);

  // setParams merges updates into the current search string.
  const setParams = useCallback(
    (updates: Partial<InvestigationParams>) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          for (const [k, v] of Object.entries(toQueryStrings(updates))) {
            if (v === undefined || v === "") next.delete(k);
            else next.set(k, v);
          }
          return next;
        },
        { replace: false },
      );
    },
    [setSearchParams],
  );

  // clearParams removes all investigation params (one-click reset).
  const clearParams = useCallback(() => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const k of PARAM_KEYS) next.delete(k);
        return next;
      },
      { replace: false },
    );
  }, [setSearchParams]);

  return { params, setParams, clearParams };
}

// toQueryStrings maps InvestigationParams to query-string key/value pairs.
function toQueryStrings(p: Partial<InvestigationParams>): Record<string, string> {
  const out: Record<string, string> = {};
  if (p.range) out.range = p.range;
  if (p.minLoss != null) out.min_loss = String(p.minLoss);
  if (p.minProbes != null) out.min_probes = String(p.minProbes);
  if (p.sort) out.sort = p.sort;
  if (p.order) out.order = p.order;
  if (p.asn != null) out.asn = String(p.asn);
  if (p.target) out.target = p.target;
  if (p.probe != null) out.probe = String(p.probe);
  if (p.fromIssue) out.from_issue = p.fromIssue;
  return out;
}

// buildSearchPath constructs a relative path + query string that routes to a
// detail page while preserving investigation context. Used by global search and
// issue-card CTAs so filters/origin follow the operator.
export function buildDetailPath(
  detail: { route: string },
  context?: InvestigationParams,
): string {
  const qs = context ? toQueryStrings(context) : {};
  const entries = Object.entries(qs).filter(([, v]) => v !== undefined && v !== "");
  if (entries.length === 0) return detail.route;
  return `${detail.route}?${entries.map(([k, v]) => `${k}=${encodeURIComponent(v)}`).join("&")}`;
}
