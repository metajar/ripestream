import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronRight, Globe2 } from "lucide-react";
import { api, type ReachableGroup, type SearchResult } from "@/api/client";
import { cn } from "@/lib/utils";

interface SearchSelectProps {
  label: string;
  placeholder: string;
  value: SearchResult | null;
  onChange: (hit: SearchResult | null) => void;
  // When set, the picker operates in constrained mode: it shows ONLY endpoints
  // reachable from this source address (queried once via /api/path/destinations),
  // instead of free-text search. Typing filters the reachable set.
  reachableFrom?: string;
}

// SearchSelect is a debounced autocomplete picker for graph entities. Typing
// queries /api/search (org name, ASN, IP prefix, probe id). AS hits render as
// expandable groups whose individual IPs are each selectable as the endpoint.
//
// In constrained mode (reachableFrom set) it instead lists only destinations
// proven reachable from the given source, so the operator can never pick a
// pair with no path between them.
export function SearchSelect({ label, placeholder, value, onChange, reachableFrom }: SearchSelectProps) {
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [hits, setHits] = useState<SearchResult[]>([]);
  const [groups, setGroups] = useState<ReachableGroup[]>([]);
  const [loading, setLoading] = useState(false);
  const [expandedAS, setExpandedAS] = useState<string | null>(null);
  const boxRef = useRef<HTMLDivElement>(null);
  const constrained = !!reachableFrom;

  // Constrained mode: fetch reachable destinations once when the source changes.
  useEffect(() => {
    if (!reachableFrom) {
      setGroups([]);
      return;
    }
    setLoading(true);
    let cancelled = false;
    api
      .reachableDestinations(reachableFrom)
      .then((g) => {
        if (cancelled) return;
        setGroups(g);
        setExpandedAS(g.length > 0 ? groupKey(g[0]) : null); // expand the first group
      })
      .catch(() => !cancelled && setGroups([]))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [reachableFrom]);

  // Free-text mode: debounced search.
  useEffect(() => {
    if (constrained) return; // constrained mode filters locally, no search call.
    const q = query.trim();
    if (q.length < 1) {
      setHits([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    const t = setTimeout(async () => {
      try {
        const results = await api.search(q, 12);
        setHits(results);
        const firstAS = results.find((r) => r.kind === "as" && r.addrs && r.addrs.length > 0);
        setExpandedAS(firstAS ? asKey(firstAS) : null);
      } catch {
        setHits([]);
      } finally {
        setLoading(false);
      }
    }, 200);
    return () => clearTimeout(t);
  }, [query, constrained]);

  // Constrained mode: filter the reachable groups by the typed query (org/asn/ip).
  const filteredGroups = useMemo(() => {
    if (!constrained) return [];
    const q = query.trim().toLowerCase();
    if (!q) return groups;
    return groups
      .map((g) => {
        const asn = g.asn ? `as${g.asn}` : "";
        const ipMatch = (g.sample_ips || []).some((ip) => ip.toLowerCase().includes(q));
        const orgMatch = g.org.toLowerCase().includes(q) || asn.includes(q);
        if (orgMatch) return g; // whole group matches -> keep all its IPs
        if (!ipMatch) return null; // no match at all
        // Partial: keep only the matching IPs in this group.
        return { ...g, sample_ips: (g.sample_ips || []).filter((ip) => ip.toLowerCase().includes(q)) };
      })
      .filter((g): g is ReachableGroup => g !== null && g.sample_ips.length > 0);
  }, [groups, query, constrained]);

  // Close the dropdown on outside click.
  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (boxRef.current && !boxRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, []);

  function pickConcreteAddr(addr: string, from: SearchResult) {
    // Select a specific IP, carrying the parent AS context for the chip label.
    const sub = from.asn ? `${from.label} · AS${from.asn}` : from.label;
    onChange({
      kind: "ip",
      label: addr,
      addr,
      asn: from.asn,
      org: from.org,
      sub,
    });
    setQuery("");
    setOpen(false);
    setHits([]);
    setExpandedAS(null);
  }

  function pickSingle(hit: SearchResult) {
    onChange(hit);
    setQuery("");
    setOpen(false);
    setHits([]);
    setExpandedAS(null);
  }

  return (
    <div className="flex-1" ref={boxRef}>
      <label className="mb-1 block text-xs font-medium text-text-quaternary">{label}</label>

      {/* Selected chip or input */}
      {value ? (
        <div className="flex items-center justify-between rounded-lg border border-brand-500/40 bg-brand-500/5 px-3 py-2">
          <div className="flex min-w-0 items-center gap-2">
            <KindIcon kind={value.kind} />
            <div className="min-w-0">
              <div className="truncate text-sm font-medium text-text-primary">{value.label}</div>
              {(value.sub || value.addr) && (
                <div className="truncate font-mono text-xs text-text-tertiary">
                  {value.sub || value.addr}
                </div>
              )}
            </div>
          </div>
          <button
            type="button"
            onClick={() => onChange(null)}
            className="ml-2 shrink-0 rounded text-text-quaternary hover:text-error-500"
            aria-label="clear"
          >
            ✕
          </button>
        </div>
      ) : (
        <input
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setOpen(true);
          }}
          onFocus={() => setOpen(true)}
          placeholder={placeholder}
          className="w-full rounded-lg border border-border-primary bg-bg-tertiary px-3 py-2 font-mono text-sm text-text-primary placeholder:text-text-quaternary focus:border-brand-500 focus:outline-none"
        />
      )}

      {/* Dropdown — constrained mode shows reachable destinations; free-text mode
          shows search hits (and only after the user types). */}
      {open && !value && (constrained || query.trim().length > 0) && (
        <div className="relative z-10 mt-1">
          <div className="max-h-80 overflow-y-auto rounded-lg border border-border-secondary bg-bg-elevated shadow-xl">
            {loading && (
              <div className="px-3 py-2 text-xs text-text-quaternary">Finding reachable destinations…</div>
            )}

            {constrained && !loading && filteredGroups.length === 0 && (
              <div className="px-3 py-2 text-xs text-text-quaternary">
                {groups.length === 0 ? "No destinations reachable from this source" : "No matches in reachable set"}
              </div>
            )}

            {!constrained && !loading && hits.length === 0 && (
              <div className="px-3 py-2 text-xs text-text-quaternary">No matches</div>
            )}

            {/* Constrained: reachable destination groups */}
            {constrained && !loading &&
              filteredGroups.map((g) => {
                const key = groupKey(g);
                const expanded = expandedAS === key;
                const ips = g.sample_ips || [];
                return (
                  <div key={key} className="border-b border-border-primary/50 last:border-0">
                    <button
                      type="button"
                      onClick={() => setExpandedAS(expanded ? null : key)}
                      className="flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors hover:bg-bg-tertiary"
                    >
                      <KindIcon kind="as" />
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm text-text-primary">{g.org}</div>
                        <div className="truncate font-mono text-xs text-text-tertiary">
                          {g.asn ? `AS${g.asn}` : "—"} · {g.ip_count} IP{g.ip_count === 1 ? "" : "s"} · {g.basis}
                        </div>
                      </div>
                      {ips.length > 0 && (
                        <ChevronRight
                          className={cn(
                            "h-4 w-4 shrink-0 text-text-quaternary transition-transform",
                            expanded && "rotate-90",
                          )}
                        />
                      )}
                    </button>
                    {expanded && ips.length > 0 && (
                      <div className="bg-bg-primary/40 pb-1">
                        {ips.map((ip) => (
                          <button
                            key={ip}
                            type="button"
                            onClick={() =>
                              pickConcreteAddr(ip, {
                                kind: "ip",
                                asn: g.asn,
                                org: g.org,
                                label: g.org,
                              })
                            }
                            className="flex w-full items-center gap-2 py-1.5 pl-11 pr-3 text-left font-mono text-xs text-text-secondary transition-colors hover:bg-brand-500/10 hover:text-brand-300"
                          >
                            <Globe2 className="h-3 w-3 shrink-0 text-text-quaternary" />
                            <span className="truncate">{ip}</span>
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                );
              })}

            {/* Free-text: search hits */}
            {!constrained && !loading &&
              hits.map((h) => {
                const key = asKey(h);
                const isAS = h.kind === "as";
                const hasIPs = isAS && h.addrs && h.addrs.length > 0;
                const expanded = expandedAS === key;
                return (
                  <div key={key} className="border-b border-border-primary/50 last:border-0">
                    {isAS ? (
                      <button
                        type="button"
                        onClick={() => setExpandedAS(expanded ? null : key)}
                        className="flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors hover:bg-bg-tertiary"
                      >
                        <KindIcon kind="as" />
                        <div className="min-w-0 flex-1">
                          <div className="truncate text-sm text-text-primary">{h.label}</div>
                          <div className="truncate font-mono text-xs text-text-tertiary">{h.sub}</div>
                        </div>
                        {hasIPs && (
                          <ChevronRight
                            className={cn(
                              "h-4 w-4 shrink-0 text-text-quaternary transition-transform",
                              expanded && "rotate-90",
                            )}
                          />
                        )}
                      </button>
                    ) : (
                      <button
                        type="button"
                        onClick={() => pickSingle(h)}
                        className="flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors hover:bg-bg-tertiary"
                      >
                        <KindIcon kind={h.kind} />
                        <div className="min-w-0 flex-1">
                          <div className="truncate text-sm text-text-primary">{h.label}</div>
                          {h.sub && (
                            <div className="truncate font-mono text-xs text-text-tertiary">{h.sub}</div>
                          )}
                        </div>
                      </button>
                    )}
                    {isAS && expanded && hasIPs && (
                      <div className="bg-bg-primary/40 pb-1">
                        {h.addrs!.map((ip) => (
                          <button
                            key={ip}
                            type="button"
                            onClick={() => pickConcreteAddr(ip, h)}
                            className="flex w-full items-center gap-2 py-1.5 pl-11 pr-3 text-left font-mono text-xs text-text-secondary transition-colors hover:bg-brand-500/10 hover:text-brand-300"
                          >
                            <Globe2 className="h-3 w-3 shrink-0 text-text-quaternary" />
                            <span className="truncate">{ip}</span>
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                );
              })}
          </div>
        </div>
      )}
    </div>
  );
}

function groupKey(g: ReachableGroup): string {
  return g.asn ? `as:${g.asn}` : `org:${g.org}`;
}

function asKey(h: SearchResult): string {
  if (h.kind === "as" && h.asn) return `as:${h.asn}`;
  return `${h.kind}:${h.label}`;
}

function KindIcon({ kind }: { kind: string }) {
  const map: Record<string, { c: string; g: string }> = {
    as: { c: "bg-brand-500/15 text-brand-300", g: "AS" },
    ip: { c: "bg-info-500/15 text-info-500", g: "IP" },
    probe: { c: "bg-success-500/15 text-success-500", g: "P" },
  };
  const s = map[kind] ?? map.ip;
  return (
    <span className={cn("flex h-6 w-6 shrink-0 items-center justify-center rounded text-[10px] font-bold", s.c)}>
      {s.g}
    </span>
  );
}
