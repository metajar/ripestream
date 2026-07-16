import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api } from "@/api/client";
import { SeverityBadge, severityFromLossPct } from "@/components/health";
import { DEFAULT_WORKLIST, WorklistToolbar, type WorklistState } from "@/components/WorklistToolbar";
import { Card, CardContent } from "@/components/ui/card";
import { EmptyState, ErrorState, Freshness, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { fmtNum, fmtPct } from "@/lib/utils";

const SORT_OPTIONS: [string, string][] = [
  ["impact", "Impact"],
  ["loss", "Loss %"],
  ["probes", "Probes"],
  ["samples", "Samples"],
  ["last_seen", "Last observed"],
];

export function ASNsPage() {
  const [role, setRole] = useState<"src" | "dst">("dst");
  const [searchParams, setSearchParams] = useSearchParams();

  // Read worklist state from URL params.
  const state: WorklistState = {
    sort: searchParams.get("sort") || DEFAULT_WORKLIST.sort,
    order: (searchParams.get("order") as "asc" | "desc") || DEFAULT_WORKLIST.order,
    minProbes: Number(searchParams.get("min_probes")) || DEFAULT_WORKLIST.minProbes,
  };

  function patchState(p: Partial<WorklistState>) {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (p.sort) next.set("sort", p.sort);
        if (p.order) next.set("order", p.order);
        if (p.minProbes != null) next.set("min_probes", String(p.minProbes));
        return next;
      },
      { replace: true },
    );
  }
  function clearState() {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.delete("sort");
      next.delete("order");
      next.delete("min_probes");
      return next;
    }, { replace: true });
  }

  const { data, isLoading, error } = useQuery({
    queryKey: ["asn-issues", role, state.sort, state.order, state.minProbes],
    queryFn: () => api.asnIssues(role, 50, state.sort, state.order, state.minProbes),
  });

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold text-text-primary">Autonomous Systems</h1>
          <p className="text-sm text-text-quaternary">
            Ranked by impact (loss × probes). Sorted server-side.
          </p>
        </div>
        <div className="flex gap-1 rounded-lg border border-border-primary bg-bg-secondary p-1">
          {(["dst", "src"] as const).map((r) => (
            <button
              key={r}
              onClick={() => setRole(r)}
              className={`rounded-md px-3 py-1 text-xs font-medium transition-colors ${
                role === r ? "bg-brand-500 text-white" : "text-text-tertiary hover:text-text-primary"
              }`}
            >
              {r === "dst" ? "Destinations" : "Sources"}
            </button>
          ))}
        </div>
      </div>

      <Card>
        <CardContent className="pt-4">
          <WorklistToolbar
            state={state}
            onChange={patchState}
            onClear={clearState}
            sortOptions={SORT_OPTIONS}
            resultCount={data?.length}
          />
          {isLoading && <LoadingState />}
          {error && <ErrorState message={(error as Error).message} />}
          {data && data.length === 0 && (
            <EmptyState
              label="No ASes with notable observed loss"
              hint={`No AS meets the current threshold (min ${state.minProbes} probes). Try lowering the minimum or this may indicate a healthy network.`}
            />
          )}
          {data && data.length > 0 && (
            <Table>
              <THead>
                <tr>
                  <Th>AS</Th>
                  <Th>Organization</Th>
                  <Th>Severity</Th>
                  <Th className="text-right">Samples</Th>
                  <Th className="text-right">Probes</Th>
                  <Th className="text-right">Avg Loss</Th>
                  <Th className="text-right">Last observed</Th>
                </tr>
              </THead>
              <TBody>
                {data.map((a) => (
                  <Tr key={a.asn}>
                    <Td>
                      <Link
                        to={`/asn/${a.asn}`}
                        className="font-mono text-xs text-brand-300 hover:text-brand-500"
                      >
                        AS{a.asn}
                      </Link>
                    </Td>
                    <Td className="text-text-primary">{a.org || "—"}</Td>
                    <Td>
                      <SeverityBadge severity={severityFromLossPct(a.avg_loss_pct)} />
                    </Td>
                    <Td className="text-right font-mono text-xs">{fmtNum(a.samples)}</Td>
                    <Td className="text-right font-mono text-xs">{fmtNum(a.probes)}</Td>
                    <Td className="text-right font-mono text-xs font-medium">
                      {fmtPct(a.avg_loss_pct)}
                    </Td>
                    <Td className="text-right text-xs">
                      <Freshness sec={a.last_seen} />
                    </Td>
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
