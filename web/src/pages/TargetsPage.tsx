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
        <h1 className="text-lg font-semibold text-text-primary">Targets</h1>
        <p className="text-sm text-text-quaternary">Currently experiencing loss (actionable set)</p>
      </div>
      <Card>
        <CardContent className="pt-4">
          {isLoading && <LoadingState />}
          {error && <ErrorState message={(error as Error).message} />}
          {data && data.length === 0 && (
            <EmptyState
              label="No targets with notable observed loss"
              hint="No destination IP currently shows greater than 20% loss across recent PING edges. This may indicate a healthy network."
            />
          )}
          {data && data.length > 0 && (
            <Table>
              <THead>
                <tr>
                  <Th>Target</Th>
                  <Th>AS</Th>
                  <Th className="text-right">Probes</Th>
                  <Th className="text-right">Loss</Th>
                  <Th className="text-right">RTT</Th>
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
                    <Td className={`text-right font-mono text-xs font-medium ${lossColor(t.loss_pct)}`}>
                      {fmtPct(t.loss_pct)}
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
