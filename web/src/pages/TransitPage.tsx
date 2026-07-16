import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ErrorState, LoadingState } from "@/components/ui/states";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { fmtEpoch, fmtNum, fmtRtt } from "@/lib/utils";

export function TransitPage() {
  const navigate = useNavigate();
  const [tab, setTab] = useState<"transit" | "hops">("transit");
  const transit = useQuery({ queryKey: ["transit-edges"], queryFn: () => api.transitEdges(100) });
  const hops = useQuery({ queryKey: ["hot-hops"], queryFn: () => api.hotHops(100, 100) });

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold text-text-primary">Transit & Hops</h1>
          <p className="text-sm text-text-quaternary">Common transit paths and hotspot edges</p>
        </div>
        <div className="flex gap-1 rounded-lg border border-border-primary bg-bg-secondary p-1">
          {(["transit", "hops"] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`rounded-md px-3 py-1 text-xs font-medium transition-colors ${
                tab === t ? "bg-brand-500 text-white" : "text-text-tertiary hover:text-text-primary"
              }`}
            >
              {t === "transit" ? "Common paths" : "Latency hotspots"}
            </button>
          ))}
        </div>
      </div>
      <p className="text-xs text-text-quaternary">
        {tab === "transit"
          ? "Common paths: how often two ASes appear consecutively in observed traceroutes. Higher = more frequently traversed."
          : "Latency hotspots: NEXT_HOP edges with high latest observed RTT. High RTT alone is not packet loss or proof a hop is at fault."}
      </p>

      <Card>
        <CardHeader>
          <CardTitle>{tab === "transit" ? "AS → AS transit frequency" : "High-latency hop edges"}</CardTitle>
        </CardHeader>
        <CardContent>
          {tab === "transit" ? (
            <>
              {transit.isLoading && <LoadingState />}
              {transit.error && <ErrorState message={(transit.error as Error).message} />}
              {transit.data && (
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
                    {transit.data.map((t, i) => (
                      <Tr
                        key={i}
                        role="link"
                        tabIndex={0}
                        aria-label={`View ${t.src_org || `AS${t.src_asn}`} to ${t.dst_org || `AS${t.dst_asn}`} relationship`}
                        className="cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand-500"
                        onClick={() => navigate(`/transit/${t.src_asn}/${t.dst_asn}`)}
                        onKeyDown={(event) => {
                          if (event.key === "Enter" || event.key === " ") {
                            event.preventDefault();
                            navigate(`/transit/${t.src_asn}/${t.dst_asn}`);
                          }
                        }}
                      >
                        <Td>
                          <Link to={`/transit/${t.src_asn}/${t.dst_asn}`} className="hover:text-brand-300">
                            <span className="text-text-primary">{t.src_org || `AS${t.src_asn}`}</span>{" "}
                            <span className="text-xs text-text-quaternary">AS{t.src_asn}</span>
                          </Link>
                        </Td>
                        <Td>
                          <Link to={`/transit/${t.src_asn}/${t.dst_asn}`} className="hover:text-brand-300">
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
            </>
          ) : (
            <>
              {hops.isLoading && <LoadingState />}
              {hops.error && <ErrorState message={(hops.error as Error).message} />}
              {hops.data && (
                <Table>
                  <THead>
                    <tr>
                      <Th>From</Th>
                      <Th>To</Th>
                      <Th>From AS</Th>
                      <Th>To AS</Th>
                      <Th className="text-right">RTT</Th>
                      <Th className="text-right">Observations</Th>
                    </tr>
                  </THead>
                  <TBody>
                    {hops.data.map((h, i) => (
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
                        <Td className="text-xs text-text-tertiary">{h.from_org || "—"}</Td>
                        <Td className="text-xs text-text-tertiary">{h.to_org || "—"}</Td>
                        <Td className="text-right font-mono text-xs text-warning-500">{fmtRtt(h.last_rtt_ms)}</Td>
                        <Td className="text-right font-mono text-xs text-text-quaternary">{h.seen_count}</Td>
                      </Tr>
                    ))}
                  </TBody>
                </Table>
              )}
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
