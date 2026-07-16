import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ErrorState, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { fmtEpoch, fmtNum, fmtPct, fmtRtt, lossColor } from "@/lib/utils";

export function ASNDetailPage() {
  const { asn } = useParams<{ asn: string }>();
  const asnNum = Number(asn);

  const detail = useQuery({
    queryKey: ["asn-detail", asnNum],
    queryFn: () => api.asnDetail(asnNum),
    enabled: !!asnNum,
  });
  const probes = useQuery({
    queryKey: ["asn-probes", asnNum],
    queryFn: () => api.asnProbes(asnNum, 50),
    enabled: !!asnNum,
  });
  const targets = useQuery({
    queryKey: ["asn-targets", asnNum],
    queryFn: () => api.asnTargets(asnNum, 50),
    enabled: !!asnNum,
  });
  const transit = useQuery({
    queryKey: ["asn-transit", asnNum],
    queryFn: () => api.asnTransit(asnNum, 50),
    enabled: !!asnNum,
  });

  if (detail.isLoading) return <LoadingState label={`Loading AS${asn}…`} />;
  if (detail.error) return <ErrorState message={(detail.error as Error).message} />;
  if (!detail.data) return null;

  const d = detail.data;

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-lg font-semibold text-text-primary">{d.org}</h1>
            <Badge variant="brand">AS{d.asn}</Badge>
          </div>
          <Link to="/asns" className="text-xs text-text-quaternary hover:text-brand-300">
            ← back to ASes
          </Link>
        </div>
        <Badge variant={d.avg_loss_pct > 10 ? "danger" : d.avg_loss_pct > 0 ? "warning" : "success"}>
          avg {fmtPct(d.avg_loss_pct, 1)} loss
        </Badge>
      </div>

      {/* What we see — plain-language summary before the raw lists */}
      <div className="rounded-lg border border-border-primary bg-bg-secondary p-4">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-text-quaternary">What we see</h2>
        <p className="mt-1 text-sm text-text-secondary">
          {d.org || `AS${d.asn}`} is a <strong className="text-text-primary">destination AS</strong> with{" "}
          <strong className="text-text-primary">{fmtPct(d.avg_loss_pct, 1)} average observed loss</strong>{" "}
          across {fmtNum(d.lossy_edges)} lossy PING {d.lossy_edges === 1 ? "edge" : "edges"}.
        </p>
        <p className="mt-1 text-sm text-text-quaternary">
          Scope: {fmtNum(d.probe_count)} {d.probe_count === 1 ? "probe" : "probes"},{" "}
          {fmtNum(d.ip_count)} IPs, {fmtNum(d.target_count)} targets,{" "}
          {fmtNum(d.transit_in + d.transit_out)} transit relationships.
        </p>
      </div>

      {/* Stat cards */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
        <Stat label="Probes" value={fmtNum(d.probe_count)} />
        <Stat label="IPs" value={fmtNum(d.ip_count)} />
        <Stat label="Targets" value={fmtNum(d.target_count)} />
        <Stat label="Transit In" value={fmtNum(d.transit_in)} />
        <Stat label="Transit Out" value={fmtNum(d.transit_out)} />
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        {/* Probes */}
        <Card>
          <CardHeader>
            <CardTitle>Probes in this AS</CardTitle>
          </CardHeader>
          <CardContent>
            {probes.isLoading ? <LoadingState /> : (
              <Table>
                <THead>
                  <tr>
                    <Th>Probe</Th>
                    <Th>Source IP</Th>
                    <Th className="text-right">Loss</Th>
                    <Th className="text-right">RTT</Th>
                  </tr>
                </THead>
                <TBody>
                  {(probes.data ?? []).map((p) => (
                    <Tr key={p.id}>
                      <Td>
                        <Link to={`/probe/${p.id}`} className="font-mono text-xs text-brand-300 hover:text-brand-500">
                          {p.id}
                        </Link>
                      </Td>
                      <Td className="font-mono text-xs text-text-tertiary">{p.src_ip}</Td>
                      <Td className={`text-right font-mono text-xs ${lossColor(p.loss_pct)}`}>
                        {fmtPct(p.loss_pct)}
                      </Td>
                      <Td className="text-right font-mono text-xs text-text-tertiary">{fmtRtt(p.avg_rtt_ms)}</Td>
                    </Tr>
                  ))}
                </TBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Targets */}
        <Card>
          <CardHeader>
            <CardTitle>Targets in this AS</CardTitle>
          </CardHeader>
          <CardContent>
            {targets.isLoading ? <LoadingState /> : (
              <Table>
                <THead>
                  <tr>
                    <Th>Target</Th>
                    <Th className="text-right">Probes</Th>
                    <Th className="text-right">Loss</Th>
                    <Th className="text-right">RTT</Th>
                  </tr>
                </THead>
                <TBody>
                  {(targets.data ?? []).map((t) => (
                    <Tr key={t.addr}>
                      <Td>
                        <Link to={`/target/${t.addr}`} className="font-mono text-xs text-brand-300 hover:text-brand-500">
                          {t.addr}
                        </Link>
                      </Td>
                      <Td className="text-right font-mono text-xs">{fmtNum(t.probes)}</Td>
                      <Td className={`text-right font-mono text-xs ${lossColor(t.loss_pct)}`}>
                        {fmtPct(t.loss_pct)}
                      </Td>
                      <Td className="text-right font-mono text-xs text-text-tertiary">{fmtRtt(t.avg_rtt_ms)}</Td>
                    </Tr>
                  ))}
                </TBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Transit */}
      <Card>
        <CardHeader>
          <CardTitle>Transit Relationships</CardTitle>
        </CardHeader>
        <CardContent>
          {transit.isLoading ? <LoadingState /> : (
            <Table>
              <THead>
                <tr>
                  <Th>From</Th>
                  <Th>To</Th>
                  <Th className="text-right">Observations</Th>
                  <Th className="text-right">Last observed</Th>
                </tr>
              </THead>
              <TBody>
                {(transit.data ?? []).map((t, i) => (
                  <Tr key={i}>
                    <Td>
                      <Link to={`/asn/${t.src_asn}`} className="hover:text-brand-300">
                        <span className="text-text-primary">{t.src_org || `AS${t.src_asn}`}</span>{" "}
                        <span className="text-xs text-text-quaternary">AS{t.src_asn}</span>
                      </Link>
                    </Td>
                    <Td>
                      <Link to={`/asn/${t.dst_asn}`} className="hover:text-brand-300">
                        <span className="text-text-primary">{t.dst_org || `AS${t.dst_asn}`}</span>{" "}
                        <span className="text-xs text-text-quaternary">AS{t.dst_asn}</span>
                      </Link>
                    </Td>
                    <Td className="text-right font-mono text-xs">{fmtNum(t.seen_count)}</Td>
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

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card className="px-4 py-3">
      <div className="text-xl font-semibold text-text-primary">{value}</div>
      <div className="text-xs text-text-quaternary">{label}</div>
    </Card>
  );
}
