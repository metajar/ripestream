import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState, ErrorState, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { fmtNum, fmtPct, lossColor } from "@/lib/utils";

export function ASNsPage() {
  const [role, setRole] = useState<"src" | "dst">("dst");
  const { data, isLoading, error } = useQuery({
    queryKey: ["asn-issues", role],
    queryFn: () => api.asnIssues(role, 50),
  });

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold text-text-primary">Autonomous Systems</h1>
          <p className="text-sm text-text-quaternary">Ranked by aggregate packet loss</p>
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
        <CardHeader>
          <CardTitle>{role === "dst" ? "Destination ASes" : "Source ASes"}</CardTitle>
          <Badge variant="neutral">{data?.length ?? 0} shown</Badge>
        </CardHeader>
        <CardContent>
          {isLoading && <LoadingState />}
          {error && <ErrorState message={(error as Error).message} />}
          {data && data.length === 0 && (
            <EmptyState label="No ASes with notable observed loss" hint="No autonomous system currently shows greater than 10% aggregate loss." />
          )}
          {data && data.length > 0 && (
            <Table>
              <THead>
                <tr>
                  <Th>AS</Th>
                  <Th>Organization</Th>
                  <Th className="text-right">Samples</Th>
                  <Th className="text-right">Probes</Th>
                  <Th className="text-right">Avg Loss</Th>
                  <Th className="text-right">Max Loss</Th>
                </tr>
              </THead>
              <TBody>
                {data.map((a) => (
                  <Tr key={a.asn}>
                    <Td>
                      <Link to={`/asn/${a.asn}`} className="font-mono text-xs text-brand-300 hover:text-brand-500">
                        AS{a.asn}
                      </Link>
                    </Td>
                    <Td className="text-text-primary">{a.org || "—"}</Td>
                    <Td className="text-right font-mono text-xs">{fmtNum(a.samples)}</Td>
                    <Td className="text-right font-mono text-xs">{fmtNum(a.probes)}</Td>
                    <Td className={`text-right font-mono text-xs font-medium ${lossColor(a.avg_loss_pct)}`}>
                      {fmtPct(a.avg_loss_pct)}
                    </Td>
                    <Td className={`text-right font-mono text-xs ${lossColor(a.max_loss_pct)}`}>
                      {fmtPct(a.max_loss_pct)}
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
