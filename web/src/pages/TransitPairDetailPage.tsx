import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState, ErrorState, Freshness, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { fmtNum, fmtRtt } from "@/lib/utils";

export function TransitPairDetailPage() {
  const { asnA, asnB } = useParams<{ asnA: string; asnB: string }>();
  const a = Number(asnA);
  const b = Number(asnB);

  const { data, isLoading, error } = useQuery({
    queryKey: ["transit-pair", a, b],
    queryFn: () => api.transitPairDetail(a, b),
    enabled: !!a && !!b,
  });

  if (isLoading) return <LoadingState label="Loading transit pair…" />;
  if (error) return <ErrorState message={(error as Error).message} />;
  if (!data) return null;
  const d = data;

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-2">
            <Link to={`/asn/${d.src_asn}`} className="text-lg font-semibold text-text-primary hover:text-brand-300">
              {d.src_org || `AS${d.src_asn}`}
            </Link>
            <span className="text-text-quaternary">→</span>
            <Link to={`/asn/${d.dst_asn}`} className="text-lg font-semibold text-text-primary hover:text-brand-300">
              {d.dst_org || `AS${d.dst_asn}`}
            </Link>
          </div>
          <Link to="/transit" className="text-xs text-text-quaternary hover:text-brand-300">
            ← back to Transit & Hops
          </Link>
        </div>
        <Badge variant="neutral">{fmtNum(d.seen_count)} observations</Badge>
      </div>

      {/* What we see */}
      <div className="rounded-lg border border-border-primary bg-bg-secondary p-4">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-text-quaternary">What we see</h2>
        <p className="mt-1 text-sm text-text-secondary">
          <strong className="text-text-primary">{d.src_org || `AS${d.src_asn}`}</strong> transits to{" "}
          <strong className="text-text-primary">{d.dst_org || `AS${d.dst_asn}`}</strong> — observed{" "}
          {fmtNum(d.seen_count)} time{d.seen_count === 1 ? "" : "s"} in traceroute paths.
          Last observed <Freshness sec={d.last_seen} />.
        </p>
        <p className="mt-1 text-xs text-text-quaternary">
          These are consecutive-hop AS appearances in observed traceroutes, not a claim of a BGP transit relationship.
        </p>
      </div>

      {/* Realizing hop edges */}
      <Card>
        <CardHeader>
          <CardTitle>Hop edges realizing this transit ({d.hops.length})</CardTitle>
        </CardHeader>
        <CardContent>
          {d.hops.length === 0 ? (
            <EmptyState label="No hop edges found" hint="No NEXT_HOP edges cross this AS boundary in the graph." />
          ) : (
            <Table>
              <THead>
                <tr>
                  <Th>From</Th>
                  <Th>To</Th>
                  <Th className="text-right">RTT</Th>
                  <Th className="text-right">Observations</Th>
                  <Th className="text-right">Last observed</Th>
                </tr>
              </THead>
              <TBody>
                {d.hops.map((h, i) => (
                  <Tr key={i}>
                    <Td>
                      <Link to={`/ip/${h.from_addr}`} className="font-mono text-xs text-brand-300 hover:text-brand-500">
                        {h.from_addr}
                      </Link>
                    </Td>
                    <Td>
                      <Link to={`/ip/${h.to_addr}`} className="font-mono text-xs text-brand-300 hover:text-brand-500">
                        {h.to_addr}
                      </Link>
                    </Td>
                    <Td className="text-right font-mono text-xs text-text-tertiary">{fmtRtt(h.last_rtt_ms)}</Td>
                    <Td className="text-right font-mono text-xs">{fmtNum(h.seen_count)}</Td>
                    <Td className="text-right text-xs">
                      <Freshness sec={h.last_seen} />
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
