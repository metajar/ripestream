import { useState } from "react";
import { ArrowRight, Info, Route } from "lucide-react";
import { Link, useSearchParams } from "react-router-dom";
import { api, type GraphPath, type SearchResult } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState, ErrorState, LoadingState, Spinner } from "@/components/ui/states";
import { SearchSelect } from "@/components/SearchSelect";
import { fmtRtt } from "@/lib/utils";

export function PathPage() {
  const [params] = useSearchParams();
  // Prefill from URL params (e.g. links from issue cards / detail pages).
  const [src, setSrc] = useState<SearchResult | null>(
    params.get("src") ? { kind: "ip", label: params.get("src")!, addr: params.get("src")! } : null,
  );
  const [dst, setDst] = useState<SearchResult | null>(
    params.get("dst") ? { kind: "ip", label: params.get("dst")!, addr: params.get("dst")! } : null,
  );
  const [result, setResult] = useState<GraphPath | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Changing the source must clear the destination, since the reachable set
  // changes with it (the destination picker is constrained to the source).
  function changeSource(hit: SearchResult | null) {
    setSrc(hit);
    setDst(null);
    setResult(null);
    setError(null);
  }

  async function run(e: React.FormEvent) {
    e.preventDefault();
    if (!src?.addr || !dst?.addr) return;
    setLoading(true);
    setError(null);
    setResult(null);
    try {
      setResult(await api.path(src.addr, dst.addr, 20));
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }

  const ready = !!src?.addr && !!dst?.addr;

  // Find the largest RTT step for an observational note (not a root-cause claim).
  const maxStepIdx = (() => {
    if (!result?.found || result.hops.length < 2) return -1;
    let maxIdx = 1;
    let maxDelta = -1;
    for (let i = 1; i < result.hops.length; i++) {
      const prev = result.hops[i - 1].rtt_ms;
      const cur = result.hops[i].rtt_ms;
      if (prev < 0 || cur < 0) continue;
      const delta = cur - prev;
      if (delta > maxDelta) {
        maxDelta = delta;
        maxIdx = i;
      }
    }
    return maxDelta > 50 ? maxIdx : -1; // only flag if > 50ms jump
  })();

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-lg font-semibold text-text-primary">Path Explorer</h1>
        <p className="text-sm text-text-quaternary">
          Trace the network path between two endpoints — search by org name, ASN, IP, or probe
        </p>
      </div>

      <Card>
        <CardContent className="pt-4">
          <form onSubmit={run} className="flex items-end gap-3">
            <SearchSelect
              label="Source"
              placeholder="e.g. DigitalOcean, AS13335, 1.2.3.4"
              value={src}
              onChange={changeSource}
            />
            <ArrowRight className="mb-2.5 h-4 w-4 shrink-0 text-text-quaternary" />
            <SearchSelect
              label="Destination"
              placeholder={src?.addr ? "Only reachable destinations shown" : "Pick a source first"}
              value={dst}
              onChange={setDst}
              reachableFrom={src?.addr}
            />
            <Button type="submit" disabled={loading || !ready} className="shrink-0">
              {loading ? <><Spinner size={14} /> Tracing</> : "Trace"}
            </Button>
          </form>
          {src?.addr && (
            <div className="mt-2 flex items-center gap-1.5 text-xs text-text-quaternary">
              <Route className="h-3 w-3" />
              Destination is constrained to endpoints reachable from{" "}
              <span className="font-mono text-text-tertiary">{src.addr}</span>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Path</CardTitle>
        </CardHeader>
        <CardContent>
          {loading && <LoadingState label="Finding path…" />}
          {error && <ErrorState message={error} />}
          {!loading && !error && result && !result.found && (
            <EmptyState
              label="No path found between these endpoints"
              hint="The graph only contains paths that observed traceroutes actually traversed. Try selecting a reachable destination from the picker above, or inspect the source/target detail pages."
            />
          )}
          {!loading && !error && result && result.found && (
            <div className="space-y-3">
              {/* Explanation: derived from observed topology */}
              <p className="flex items-center gap-1.5 text-xs text-text-quaternary">
                <Info className="h-3 w-3" />
                Path derived from observed NEXT_HOP relationships; may not reflect the current live path.
              </p>

              {/* Largest RTT step note (observational, not root-cause) */}
              {maxStepIdx > 0 && (
                <p className="rounded-lg border border-warning-500/20 bg-warning-500/5 px-3 py-1.5 text-xs text-warning-500">
                  Largest observed RTT increase after hop {maxStepIdx - 1} → {maxStepIdx}:{" "}
                  {fmtRtt(result.hops[maxStepIdx].rtt_ms)}.
                </p>
              )}

              {/* Ordered hop table */}
              <div className="space-y-1">
                {result.hops.map((hop, i) => {
                  const lossy = hop.rtt_ms > 200;
                  const isMaxStep = i === maxStepIdx;
                  return (
                    <div
                      key={i}
                      className={`flex items-center gap-3 rounded-lg px-2 py-1.5 ${
                        isMaxStep ? "bg-warning-500/5" : ""
                      }`}
                    >
                      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-bg-tertiary text-xs font-medium text-text-quaternary">
                        {i}
                      </div>
                      <Link
                        to={`/ip/${hop.addr}`}
                        className="font-mono text-sm text-text-primary hover:text-brand-300"
                      >
                        {hop.addr}
                      </Link>
                      {hop.asn && (
                        <Link
                          to={`/asn/${hop.asn}`}
                          className="text-xs text-text-tertiary hover:text-brand-300"
                        >
                          {hop.org || `AS${hop.asn}`}
                        </Link>
                      )}
                      <div className="ml-auto flex items-center gap-3">
                        {hop.seen_count > 0 && (
                          <span className="text-xs text-text-quaternary">{hop.seen_count} obs</span>
                        )}
                        {i > 0 && (
                          <span
                            className={`font-mono text-xs ${
                              lossy ? "text-warning-500" : "text-text-quaternary"
                            }`}
                          >
                            {fmtRtt(hop.rtt_ms)}
                          </span>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}
          {!loading && !error && !result && (
            <EmptyState label="Select a source and destination to trace a path" />
          )}
        </CardContent>
      </Card>
    </div>
  );
}
