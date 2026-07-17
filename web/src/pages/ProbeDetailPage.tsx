import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PaginationControls } from "@/components/PaginationControls";
import { EmptyState, ErrorState, FetchingOverlay, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { probeName } from "@/lib/probes";
import { fmtEpoch, fmtPct, fmtRtt, lossColor } from "@/lib/utils";

export function ProbeDetailPage() {
  const { id } = useParams<{ id: string }>();
  const probeId = Number(id);
  const [targetOffset, setTargetOffset] = useState(0);
  const [targetQuery, setTargetQuery] = useState("");
  const { data, isLoading, error } = useQuery({
    queryKey: ["probe", probeId],
    queryFn: () => api.probeDetail(probeId),
    enabled: !!probeId,
  });
  const targets = useQuery({
    queryKey: ["probe-targets", probeId, targetOffset, targetQuery],
    queryFn: () => api.probeTargets(probeId, 25, targetOffset, targetQuery.trim()),
    enabled: !!probeId,
    refetchInterval: false,
    refetchOnWindowFocus: false,
  });
  const targetsBusy = targets.isFetching && targets.isPlaceholderData;

  if (isLoading) return <LoadingState label={`Loading probe ${id}…`} />;
  if (error) return <ErrorState message={(error as Error).message} />;
  if (!data) return null;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-lg font-semibold text-text-primary">{probeName(data.id, data.metadata)}</h1>
        <Badge>Probe {data.id}</Badge>
        {data.metadata?.probe_type && <Badge variant="info">{data.metadata.probe_type}</Badge>}
        {data.src_asn && (
          <Link to={`/asn/${data.src_asn}`}>
            <Badge variant="brand">{data.src_org || `AS${data.src_asn}`}</Badge>
          </Link>
        )}
      </div>

      {/* What we see — plain-language summary */}
      <div className="rounded-lg border border-border-primary bg-bg-secondary p-4">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-text-quaternary">What we see</h2>
        <p className="mt-1 text-sm text-text-secondary">
          <strong className="text-text-primary">{probeName(data.id, data.metadata)}</strong> at{" "}
          <span className="font-mono text-text-primary">{data.src_ip}</span>
          {data.src_asn ? (
            <> in {data.src_org || `AS${data.src_asn}`}</>
          ) : null}{" "}
          measures <strong className="text-text-primary">{data.target_count} {data.target_count === 1 ? "target" : "targets"}</strong>.
        </p>
        <p className="mt-1 text-sm text-text-quaternary">
          Source IP: <span className="font-mono">{data.src_ip}</span>. Last seen {fmtEpoch(data.last_seen)}.
        </p>
      </div>

      {data.metadata && (
        <Card>
          <CardHeader><CardTitle>RIPE Atlas probe details</CardTitle></CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-4 text-xs md:grid-cols-4">
              <Meta label="Type" value={data.metadata.probe_type} />
              <Meta label="Location" value={data.metadata.country_code || "Unknown"} />
              <Meta label="Status" value={data.metadata.status_name || "Unknown"} />
              <Meta label="Firmware" value={data.metadata.firmware_version ? String(data.metadata.firmware_version) : "Unknown"} />
              <Meta label="First connected" value={fmtEpoch(data.metadata.first_connected)} />
              <Meta label="Last connected" value={fmtEpoch(data.metadata.last_connected)} />
              <Meta label="IPv4 prefix" value={data.metadata.prefix_v4 || "—"} mono />
              <Meta label="IPv6 prefix" value={data.metadata.prefix_v6 || "—"} mono />
            </div>
            {data.metadata.description && <p className="text-xs text-text-tertiary">{data.metadata.description}</p>}
            {(data.metadata.latitude || data.metadata.longitude) && (
              <p className="text-xs text-text-quaternary">Approximate coordinates: {data.metadata.latitude?.toFixed(3)}, {data.metadata.longitude?.toFixed(3)}; RIPE Atlas obscures probe coordinates for privacy.</p>
            )}
            {data.metadata.tags && data.metadata.tags.length > 0 && <div className="flex flex-wrap gap-1">{data.metadata.tags.map((tag) => <Badge key={tag}>{tag}</Badge>)}</div>}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Targets ({data.target_count})</CardTitle>
        </CardHeader>
        <CardContent className="relative">
          <label className="mb-3 block text-xs text-text-quaternary">
            Filter targets
            <input
              value={targetQuery}
              onChange={(event) => { setTargetQuery(event.target.value); setTargetOffset(0); }}
              placeholder="IP, ASN, organization, loss, RTT…"
              className="mt-1 block w-full rounded-md border border-border-primary bg-bg-tertiary px-3 py-2 text-xs text-text-primary outline-none focus:border-brand-500"
            />
          </label>
          {targets.isLoading && <LoadingState label="Loading targets…" />}
          {targets.error && <ErrorState message={(targets.error as Error).message} />}
          {targets.data && targets.data.data.length === 0 && (
            <EmptyState label="No matching targets" hint="Try clearing the target filter." />
          )}
          {targets.data && targets.data.data.length > 0 && <>
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
              {targets.data.data.map((t) => (
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
          <PaginationControls
            meta={targets.data.meta}
            rowCount={targets.data.data.length}
            noun="targets"
            onPage={setTargetOffset}
          />
          </>}
          <FetchingOverlay active={targetsBusy} label="Loading targets…" />
        </CardContent>
      </Card>
    </div>
  );
}

function Meta({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div><div className="text-text-quaternary">{label}</div><div className={`mt-1 text-text-secondary ${mono ? "font-mono" : ""}`}>{value}</div></div>;
}
