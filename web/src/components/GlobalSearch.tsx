import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Search } from "lucide-react";
import { api, type SearchResult } from "@/api/client";
import { cn } from "@/lib/utils";

// GlobalSearch is a debounced search input for the top bar. It queries
// /api/search (min 2 chars), offers keyboard navigation, and routes to the
// matching detail page while preserving existing query parameters.
export function GlobalSearch() {
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [hits, setHits] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [active, setActive] = useState(0);
  const [error, setError] = useState(false);
  const boxRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();

  // Debounced search; require at least 2 characters to avoid noisy short queries.
  useEffect(() => {
    const q = query.trim();
    if (q.length < 2) {
      setHits([]);
      setLoading(false);
      setError(false);
      return;
    }
    setLoading(true);
    setError(false);
    let cancelled = false;
    const t = setTimeout(async () => {
      try {
        const results = await api.search(q, 8);
        if (cancelled) return;
        setHits(results);
        setActive(0);
      } catch {
        if (cancelled) return;
        setHits([]);
        setError(true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }, 200);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [query]);

  // Close on outside click; Escape handled in onKeyDown.
  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (boxRef.current && !boxRef.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, []);

  // Route to the selected result's detail page, preserving current query params.
  function go(hit: SearchResult) {
    const route = detailRoute(hit);
    if (!route) return;
    // Preserve existing search params (e.g. active filters) on navigation.
    const current = window.location.search;
    navigate(current ? `${route}${current.includes("?") ? "&" : "?"}${current.slice(1)}` : route);
    // Actually navigate with preserved params properly merged.
    const params = new URLSearchParams(window.location.search);
    const qs = params.toString();
    navigate(qs ? `${route}?${qs}` : route);
    setQuery("");
    setOpen(false);
    setHits([]);
  }

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown" && hits.length) {
      e.preventDefault();
      setActive((a) => (a + 1) % hits.length);
    } else if (e.key === "ArrowUp" && hits.length) {
      e.preventDefault();
      setActive((a) => (a - 1 + hits.length) % hits.length);
    } else if (e.key === "Enter" && hits[active]) {
      e.preventDefault();
      go(hits[active]);
    } else if (e.key === "Escape") {
      setOpen(false);
      inputRef.current?.blur();
    }
  }

  return (
    <div className="relative w-full max-w-md" ref={boxRef}>
      <div className="relative">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-quaternary" />
        <input
          ref={inputRef}
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setOpen(true);
          }}
          onFocus={() => setOpen(true)}
          onKeyDown={onKeyDown}
          placeholder="Search AS, IP, or probe…"
          aria-label="Global search"
          role="combobox"
          aria-expanded={open}
          aria-controls="global-search-list"
          aria-activedescendant={hits[active] ? `gs-${active}` : undefined}
          className="w-full rounded-lg border border-border-primary bg-bg-tertiary py-1.5 pl-9 pr-3 text-sm text-text-primary placeholder:text-text-quaternary focus:border-brand-500 focus:outline-none"
        />
      </div>

      {open && query.trim().length >= 2 && (
        <ul
          id="global-search-list"
          role="listbox"
          className="absolute z-50 mt-1 max-h-80 w-full overflow-y-auto rounded-lg border border-border-secondary bg-bg-elevated shadow-xl"
        >
          {loading && <li className="px-3 py-2 text-xs text-text-quaternary">Searching…</li>}
          {error && (
            <li className="px-3 py-2 text-xs text-error-500">Search failed. Try again.</li>
          )}
          {!loading && !error && hits.length === 0 && (
            <li className="px-3 py-2 text-xs text-text-quaternary">No matches</li>
          )}
          {!loading &&
            !error &&
            hits.map((h, i) => (
              <li
                key={`${h.kind}-${h.addr || h.label}-${i}`}
                id={`gs-${i}`}
                role="option"
                aria-selected={i === active}
              >
                <button
                  type="button"
                  onMouseEnter={() => setActive(i)}
                  onClick={() => go(h)}
                  className={cn(
                    "flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors",
                    i === active ? "bg-brand-500/10" : "hover:bg-bg-tertiary",
                  )}
                >
                  <KindChip kind={h.kind} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm text-text-primary">{h.label}</div>
                    {h.sub && <div className="truncate font-mono text-xs text-text-tertiary">{h.sub}</div>}
                  </div>
                </button>
              </li>
            ))}
        </ul>
      )}
    </div>
  );
}

// detailRoute maps a SearchResult to its existing detail-page route.
function detailRoute(h: SearchResult): string | null {
  switch (h.kind) {
    case "as":
      return h.asn ? `/asn/${h.asn}` : null;
    case "ip":
      return h.addr ? `/ip/${encodeURIComponent(h.addr)}` : null;
    case "probe":
      // Probes are keyed by label "probe <id>".
      return `/probe/${h.label.replace("probe ", "")}`;
  }
  return null;
}

function KindChip({ kind }: { kind: string }) {
  const map: Record<string, { c: string; g: string }> = {
    as: { c: "bg-brand-500/15 text-brand-300", g: "AS" },
    ip: { c: "bg-info-500/15 text-info-500", g: "IP" },
    probe: { c: "bg-success-500/15 text-success-500", g: "P" },
  };
  const s = map[kind] ?? map.ip;
  return (
    <span className={cn("flex h-5 w-5 shrink-0 items-center justify-center rounded text-[10px] font-bold", s.c)}>
      {s.g}
    </span>
  );
}
