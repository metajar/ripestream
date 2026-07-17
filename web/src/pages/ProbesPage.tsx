import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { PaginationControls } from "@/components/PaginationControls";
import { EmptyState, ErrorState, FetchingOverlay, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { probeName, probeSubtitle } from "@/lib/probes";
import { fmtEpoch, fmtPct, fmtRtt, lossColor } from "@/lib/utils";

const controlClass = "rounded-md border border-border-primary bg-bg-tertiary px-2.5 py-1.5 text-xs text-text-secondary outline-none focus:border-brand-500";

export function ProbesPage() {
  const [type, setType] = useState("");
  const [status, setStatus] = useState("");
  const [country, setCountry] = useState("");
  const [query, setQuery] = useState("");
  const [offset, setOffset] = useState(0);
  const filters = { type, status, country: country.trim().toUpperCase(), query: query.trim() };
  const { data, isLoading, isFetching, isPlaceholderData, error } = useQuery({
    queryKey: ["probes", offset, filters],
    queryFn: () => api.probes(25, offset, filters),
    // Probe rows move frequently as measurements arrive. Refreshing while a
    // user pages or filters can shift rows between pages and lose their place.
    refetchInterval: false,
    refetchOnWindowFocus: false,
  });
  const pageBusy = isFetching && isPlaceholderData;

  function resetPage(action: () => void) {
    action();
    setOffset(0);
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold text-text-primary">Probes experiencing loss</h1>
          <p className="text-sm text-text-quaternary">Packet-weighted recent observations, enriched from the RIPE Atlas probe inventory.</p>
        </div>
        <div className="flex flex-wrap gap-2" aria-label="Probe filters">
          <select aria-label="Probe type" value={type} onChange={(e) => resetPage(() => setType(e.target.value))} className={controlClass}>
            <option value="">All probe types</option>
            <option value="anchor">Anchors</option>
            <option value="software">Software</option>
            <option value="hardware">Hardware</option>
          </select>
          <select aria-label="Probe status" value={status} onChange={(e) => resetPage(() => setStatus(e.target.value))} className={controlClass}>
            <option value="">All statuses</option>
            <option value="connected">Connected</option>
            <option value="disconnected">Disconnected</option>
            <option value="never connected">Never connected</option>
            <option value="abandoned">Abandoned</option>
            <option value="written off">Written off</option>
          </select>
          <input aria-label="Country code" value={country} onChange={(e) => resetPage(() => setCountry(e.target.value.slice(0, 2)))} placeholder="Country" className={`${controlClass} w-20 uppercase`} />
        </div>
      </div>
      <Card>
        <CardContent className="pt-4">
          <label className="mb-3 block text-xs text-text-quaternary">
            Filter any field
            <input
              value={query}
              onChange={(event) => resetPage(() => setQuery(event.target.value))}
              placeholder="Probe ID or name, IP, ASN, org, type, country, status, loss, RTT…"
              className="mt-1 block w-full rounded-md border border-border-primary bg-bg-tertiary px-3 py-2 text-xs text-text-primary outline-none focus:border-brand-500"
            />
          </label>
          {isLoading && <LoadingState label="Loading probes…" />}
          {error && <ErrorState message={(error as Error).message} />}
          {data && (
            <div className="relative">
              <FetchingOverlay active={pageBusy} label="Loading page…" />
              {data.data.length === 0 ? (
                <EmptyState label="No matching probes with notable loss" hint="Try broadening the metadata filters or wait for current measurements." />
              ) : (
                <>
                  <Table>
                    <THead><tr><Th>Probe</Th><Th>Type / location</Th><Th>Status</Th><Th>Source AS</Th><Th className="text-right">Loss</Th><Th className="text-right">RTT</Th><Th className="text-right">Observed</Th></tr></THead>
                    <TBody>
                      {data.data.map((p) => (
                        <Tr key={`${p.id}-${p.src_ip}`}>
                          <Td>
                            <Link to={`/probe/${p.id}`} className="text-xs text-brand-300 hover:text-brand-500">
                              <span className="block font-medium">{probeName(p.id, p.metadata)}</span>
                              <span className="font-mono text-[11px] text-text-quaternary">{probeSubtitle(p.id, undefined)}</span>
                            </Link>
                          </Td>
                          <Td className="text-xs text-text-tertiary">{p.metadata ? [p.metadata.probe_type, p.metadata.country_code].filter(Boolean).join(" · ") : "Metadata pending"}</Td>
                          <Td>{p.metadata?.status_name ? <Badge variant={p.metadata.status_name === "Connected" ? "success" : "neutral"}>{p.metadata.status_name}</Badge> : <span className="text-xs text-text-quaternary">—</span>}</Td>
                          <Td>{p.src_asn ? <Link to={`/asn/${p.src_asn}`} className="text-xs text-text-secondary hover:text-brand-300">{p.src_org || `AS${p.src_asn}`}</Link> : <span className="text-xs text-text-quaternary">—</span>}</Td>
                          <Td className={`text-right font-mono text-xs font-medium ${lossColor(p.loss_pct)}`}>{fmtPct(p.loss_pct)}</Td>
                          <Td className="text-right font-mono text-xs text-text-tertiary">{fmtRtt(p.avg_rtt_ms)}</Td>
                          <Td className="text-right font-mono text-xs text-text-tertiary">{fmtEpoch(p.last_seen)}</Td>
                        </Tr>
                      ))}
                    </TBody>
                  </Table>
                  <PaginationControls meta={data.meta} rowCount={data.data.length} noun="probes" onPage={setOffset} />
                </>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
