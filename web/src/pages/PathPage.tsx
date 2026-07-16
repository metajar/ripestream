import { useState } from "react";
import { ArrowRight, Route } from "lucide-react";
import { Link } from "react-router-dom";
import { api, type GraphPath, type SearchResult } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState, ErrorState, LoadingState } from "@/components/ui/states";
import { SearchSelect } from "@/components/SearchSelect";
import { fmtRtt } from "@/lib/utils";

export function PathPage() {
  const [src, setSrc] = useState<SearchResult | null>(null);
  const [dst, setDst] = useState<SearchResult | null>(null);
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
              Trace
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
            <EmptyState label="No path found" hint="These endpoints may not be connected in the graph" />
          )}
          {!loading && !error && result && result.found && (
            <div className="space-y-1">
              {result.hops.map((hop, i) => {
                const lossy = hop.rtt_ms > 200;
                return (
                  <div key={i} className="flex items-center gap-3">
                    <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-bg-tertiary text-xs font-medium text-text-quaternary">
                      {i}
                    </div>
                    <Link to={`/ip/${hop.addr}`} className="font-mono text-sm text-text-primary hover:text-brand-300">
                      {hop.addr}
                    </Link>
                    {hop.asn && (
                      <Link to={`/asn/${hop.asn}`} className="text-xs text-text-tertiary hover:text-brand-300">
                        {hop.org || `AS${hop.asn}`}
                      </Link>
                    )}
                    {i > 0 && (
                      <span className={`ml-auto font-mono text-xs ${lossy ? "text-warning-500" : "text-text-quaternary"}`}>
                        {fmtRtt(hop.rtt_ms)}
                      </span>
                    )}
                  </div>
                );
              })}
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
