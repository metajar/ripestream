import { ChevronLeft, ChevronRight } from "lucide-react";
import type { PageMeta } from "@/api/client";

export function PaginationControls({
  meta,
  onPage,
  noun = "results",
  rowCount,
}: {
  meta: PageMeta;
  onPage: (offset: number) => void;
  noun?: string;
  rowCount: number;
}) {
  const page = Math.floor(meta.offset / meta.limit) + 1;
  const first = meta.offset + 1;
  const last = meta.offset + rowCount;

  return (
    <nav className="flex flex-wrap items-center justify-between gap-3 border-t border-border-primary pt-3" aria-label={`${noun} pagination`}>
      <span className="text-xs text-text-quaternary">
        Page {page} · showing {first}–{last}{meta.has_more ? "+" : ""} filtered {noun}
      </span>
      <div className="flex gap-2">
        <button
          type="button"
          disabled={meta.offset === 0}
          onClick={() => onPage(Math.max(0, meta.offset - meta.limit))}
          className="flex items-center gap-1 rounded-md border border-border-primary px-2.5 py-1.5 text-xs text-text-secondary hover:border-border-secondary disabled:cursor-not-allowed disabled:opacity-40"
        >
          <ChevronLeft className="h-3.5 w-3.5" /> Previous
        </button>
        <button
          type="button"
          disabled={!meta.has_more}
          onClick={() => onPage(meta.offset + meta.limit)}
          className="flex items-center gap-1 rounded-md border border-border-primary px-2.5 py-1.5 text-xs text-text-secondary hover:border-border-secondary disabled:cursor-not-allowed disabled:opacity-40"
        >
          Next <ChevronRight className="h-3.5 w-3.5" />
        </button>
      </div>
    </nav>
  );
}
