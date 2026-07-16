import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "@/api/client";
import { Card, CardContent } from "@/components/ui/card";
import { EmptyState, ErrorState, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { fmtEpoch, fmtPct, fmtRtt, lossColor } from "@/lib/utils";

export function ProbesPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["probes"],
    queryFn: () => api.probes(100),
  });

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-lg font-semibold text-text-primary">Probes</h1>
        <p className="text-sm text-text-quaternary">Currently experiencing loss (actionable set)</p>
      </div>
      <Card>
        <CardContent className="pt-4">
          {isLoading && <LoadingState />}
          {error && <ErrorState message={(error as Error).message} />}
          {data && data.length === 0 && (
            <EmptyState label="No probes with notable observed loss" hint="No probe currently shows greater than 20% loss on its recent PING edges." />
          )}
          {data && data.length > 0 && (
            <Table>
              <THead>
                <tr>
                  <Th>Probe</Th>
                  <Th>Source IP</Th>
                  <Th>AS</Th>
                  <Th className="text-right">Loss</Th>
                  <Th className="text-right">RTT</Th>
                  <Th className="text-right">Last observed</Th>
                </tr>
              </THead>
              <TBody>
                {data.map((p) => (
                  <Tr key={`${p.id}-${p.src_ip}`}>
                    <Td>
                      <Link to={`/probe/${p.id}`} className="font-mono text-xs text-brand-300 hover:text-brand-500">
                        {p.id}
                      </Link>
                    </Td>
                    <Td className="font-mono text-xs text-text-tertiary">{p.src_ip}</Td>
                    <Td>
                      {p.src_asn ? (
                        <Link to={`/asn/${p.src_asn}`} className="text-xs text-text-secondary hover:text-brand-300">
                          {p.src_org || `AS${p.src_asn}`}
                        </Link>
                      ) : (
                        <span className="text-xs text-text-quaternary">—</span>
                      )}
                    </Td>
                    <Td className={`text-right font-mono text-xs font-medium ${lossColor(p.loss_pct)}`}>
                      {fmtPct(p.loss_pct)}
                    </Td>
                    <Td className="text-right font-mono text-xs text-text-tertiary">{fmtRtt(p.avg_rtt_ms)}</Td>
                    <Td className="text-right font-mono text-xs text-text-tertiary">{fmtEpoch(p.last_seen)}</Td>
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
