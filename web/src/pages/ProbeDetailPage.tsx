import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ErrorState, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { fmtEpoch, fmtPct, fmtRtt, lossColor } from "@/lib/utils";

export function ProbeDetailPage() {
  const { id } = useParams<{ id: string }>();
  const probeId = Number(id);
  const { data, isLoading, error } = useQuery({
    queryKey: ["probe", probeId],
    queryFn: () => api.probeDetail(probeId),
    enabled: !!probeId,
  });

  if (isLoading) return <LoadingState label={`Loading probe ${id}…`} />;
  if (error) return <ErrorState message={(error as Error).message} />;
  if (!data) return null;

  return (
    <div className="space-y-5">
      <div className="flex items-center gap-2">
        <h1 className="text-lg font-semibold text-text-primary">Probe {data.id}</h1>
        {data.src_asn && (
          <Link to={`/asn/${data.src_asn}`}>
            <Badge variant="brand">{data.src_org || `AS${data.src_asn}`}</Badge>
          </Link>
        )}
      </div>
      <div className="text-sm text-text-tertiary">
        Source IP: <span className="font-mono text-text-secondary">{data.src_ip}</span>
        {" · "}Last seen {fmtEpoch(data.last_seen)}
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Targets ({data.targets.length})</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <THead>
              <tr>
                <Th>Target</Th>
                <Th>AS</Th>
                <Th className="text-right">Loss</Th>
                <Th className="text-right">RTT</Th>
                <Th className="text-right">Last observed</Th>
              </tr>
            </THead>
            <TBody>
              {data.targets.map((t) => (
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
                  <Td className={`text-right font-mono text-xs font-medium ${lossColor(t.loss_pct)}`}>
                    {fmtPct(t.loss_pct)}
                  </Td>
                  <Td className="text-right font-mono text-xs text-text-tertiary">{fmtRtt(t.avg_rtt_ms)}</Td>
                  <Td className="text-right font-mono text-xs text-text-tertiary">{fmtEpoch(t.last_seen)}</Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
