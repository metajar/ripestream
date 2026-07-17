import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "@/api/client";
import { Card, CardContent } from "@/components/ui/card";
import { EmptyState, ErrorState, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { fmtEpoch, fmtNum, fmtPct, fmtRtt, lossColor } from "@/lib/utils";

export function TargetsPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["targets"],
    queryFn: () => api.targets(100),
  });

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-lg font-semibold text-text-primary">Targets with new corroborated loss</h1>
        <p className="text-sm text-text-quaternary">
          Last 30 minutes versus the prior 24 hours · at least 3 probes across 2 source networks
        </p>
      </div>
      <div className="rounded-lg border border-border-primary bg-bg-secondary px-4 py-3 text-xs text-text-tertiary">
        Persistent non-responders and broadly failing probes are excluded. A 100% current value means none of the
        included recent PING packets received a reply after the target previously had 20% loss or less.
      </div>
      <Card>
        <CardContent className="pt-4">
          {isLoading && <LoadingState label="Loading targets…" />}
          {error && <ErrorState message={(error as Error).message} />}
          {data && data.length === 0 && (
            <EmptyState
              label="No new corroborated target regressions"
              hint="Silent targets without a healthy historical baseline are intentionally excluded because non-response alone does not prove an outage."
            />
          )}
          {data && data.length > 0 && (
            <Table>
              <THead>
                <tr>
                  <Th>Target</Th>
                  <Th>AS</Th>
                  <Th className="text-right">Probes</Th>
                  <Th className="text-right">Source ASes</Th>
                  <Th className="text-right">Current loss</Th>
                  <Th className="text-right">24h baseline</Th>
                  <Th className="text-right">Change</Th>
                  <Th className="text-right">Avg RTT</Th>
                  <Th className="text-right">Last observed</Th>
                </tr>
              </THead>
              <TBody>
                {data.map((t) => (
                  <Tr key={t.addr}>
                    <Td>
                      <Link to={`/target/${t.addr}`} className="font-mono text-xs text-brand-300 hover:text-brand-500">
                        {t.addr}
                      </Link>
                    </Td>
                    <Td>
                      {t.asn ? (
                        <Link to={`/asn/${t.asn}`} className="text-xs text-text-secondary hover:text-brand-300">
                          {t.org || `AS${t.asn}`}
                        </Link>
                      ) : (
                        <span className="text-xs text-text-quaternary">—</span>
                      )}
                    </Td>
                    <Td className="text-right font-mono text-xs">{fmtNum(t.probes)}</Td>
                    <Td className="text-right font-mono text-xs">{fmtNum(t.source_ases ?? 0)}</Td>
                    <Td className={`text-right font-mono text-xs font-medium ${lossColor(t.loss_pct)}`}>
                      {fmtPct(t.loss_pct)}
                    </Td>
                    <Td className="text-right font-mono text-xs text-text-tertiary">
                      {fmtPct(t.baseline_loss_pct)}
                    </Td>
                    <Td className="text-right font-mono text-xs text-error-500">
                      +{fmtPct(t.loss_change_pct)}
                    </Td>
                    <Td className="text-right font-mono text-xs text-text-tertiary">{fmtRtt(t.avg_rtt_ms)}</Td>
                    <Td className="text-right font-mono text-xs text-text-tertiary">{fmtEpoch(t.last_seen)}</Td>
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
