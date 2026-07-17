import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api, type HotHop } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PaginationControls } from "@/components/PaginationControls";
import { EmptyState, ErrorState, FetchingOverlay, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { fmtEpoch, fmtNum, fmtRtt } from "@/lib/utils";

export function IPDetailPage() {
  const { addr } = useParams<{ addr: string }>();
  const decoded = decodeURIComponent(addr ?? "");
  const { data, isLoading, error } = useQuery({
    queryKey: ["ip", decoded],
    queryFn: () => api.ipDetail(decoded),
    enabled: decoded.length > 0,
  });

  if (isLoading) return <LoadingState label={`Loading ${decoded}…`} />;
  if (error) return <ErrorState message={(error as Error).message} />;
  if (!data) return null;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="break-all font-mono text-lg font-semibold text-text-primary">{data.addr}</h1>
        <Badge variant="info">IPv{data.af || (data.addr.includes(":") ? 6 : 4)}</Badge>
        {data.asn && (
          <Link to={`/asn/${data.asn}`}>
            <Badge variant="brand">{data.org || `AS${data.asn}`} · AS{data.asn}</Badge>
          </Link>
        )}
      </div>

      <div className="rounded-lg border border-border-primary bg-bg-secondary p-4">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-text-quaternary">What we see</h2>
        <p className="mt-1 text-sm text-text-secondary">
          This address appears in observed RIPE Atlas traceroutes with {fmtNum(data.incoming_hops)} incoming and {fmtNum(data.outgoing_hops)} outgoing hop relationships.
        </p>
        <p className="mt-1 text-xs text-text-quaternary">
          These are observed path adjacencies, not proof that this device caused latency or packet loss; last observed {fmtEpoch(data.last_seen)}.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <HopCard current={data.addr} total={data.incoming_hops} direction="in" />
        <HopCard current={data.addr} total={data.outgoing_hops} direction="out" />
      </div>
    </div>
  );
}

function HopCard({ current, total, direction }: { current: string; total: number; direction: "in" | "out" }) {
  const [offset, setOffset] = useState(0);
  const [query, setQuery] = useState("");
  const hops = useQuery({
    queryKey: ["ip-hops", current, direction, offset, query],
    queryFn: () => api.ipHops(current, direction, 25, offset, query.trim()),
    enabled: current.length > 0,
    refetchInterval: false,
    refetchOnWindowFocus: false,
  });
  const pageBusy = hops.isFetching && hops.isPlaceholderData;
  const label = direction === "in" ? "incoming" : "outgoing";

  return (
    <Card>
      <CardHeader><CardTitle>{direction === "in" ? "Incoming" : "Outgoing"} hops ({fmtNum(total)})</CardTitle></CardHeader>
      <CardContent className="relative">
        <label className="mb-3 block text-xs text-text-quaternary">
          Filter {label} hops
          <input
            value={query}
            onChange={(event) => { setQuery(event.target.value); setOffset(0); }}
            placeholder="Peer IP, ASN, organization, RTT, observations…"
            className="mt-1 block w-full rounded-md border border-border-primary bg-bg-tertiary px-3 py-2 text-xs text-text-primary outline-none focus:border-brand-500"
          />
        </label>
        {hops.isLoading && <LoadingState label={`Loading ${label} hops…`} />}
        {hops.error && <ErrorState message={(hops.error as Error).message} />}
        {hops.data && hops.data.data.length === 0 && (
          <EmptyState label={`No matching ${label} hops`} hint="Try clearing the hop filter." />
        )}
        {hops.data && hops.data.data.length > 0 && <>
          <Table>
            <THead><tr><Th>{direction === "in" ? "Previous hop" : "Next hop"}</Th><Th>AS</Th><Th className="text-right">RTT</Th><Th className="text-right">Seen</Th><Th className="text-right">Observed</Th></tr></THead>
            <TBody>
              {hops.data.data.map((hop: HotHop) => {
                const peer = direction === "in" ? hop.from_addr : hop.to_addr;
                const asn = direction === "in" ? hop.from_asn : hop.to_asn;
                const org = direction === "in" ? hop.from_org : hop.to_org;
                return (
                  <Tr key={peer || current}>
                    <Td><Link to={`/ip/${encodeURIComponent(peer)}`} className="font-mono text-xs text-brand-300 hover:text-brand-500">{peer || "Unknown"}</Link></Td>
                    <Td>{asn ? <Link to={`/asn/${asn}`} className="text-xs text-text-secondary hover:text-brand-300">{org || `AS${asn}`}</Link> : <span className="text-xs text-text-quaternary">—</span>}</Td>
                    <Td className="text-right font-mono text-xs text-text-tertiary">{fmtRtt(hop.last_rtt_ms)}</Td>
                    <Td className="text-right font-mono text-xs text-text-tertiary">{fmtNum(hop.seen_count)}</Td>
                    <Td className="text-right font-mono text-xs text-text-quaternary">{fmtEpoch(hop.last_seen)}</Td>
                  </Tr>
                );
              })}
            </TBody>
          </Table>
          <PaginationControls
            meta={hops.data.meta}
            rowCount={hops.data.data.length}
            noun={`${label} hops`}
            onPage={setOffset}
          />
        </>}
        <FetchingOverlay active={pageBusy} label={`Loading ${label} hops…`} />
      </CardContent>
    </Card>
  );
}
