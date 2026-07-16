import { ChevronDown, ChevronUp, X } from "lucide-react";
import { cn } from "@/lib/utils";

// WorklistToolbar provides the shared filter/sort controls for ASNs, Targets,
// and Probes worklists. State is URL-backed via the onChange callbacks so the
// page can synchronize with useSearchParams.
export interface WorklistState {
  sort: string;
  order: "asc" | "desc";
  minProbes: number;
}

export const DEFAULT_WORKLIST: WorklistState = {
  sort: "impact",
  order: "desc",
  minProbes: 2,
};

export function WorklistToolbar({
  state,
  onChange,
  onClear,
  sortOptions,
  resultCount,
  showMinProbes = true,
}: {
  state: WorklistState;
  onChange: (patch: Partial<WorklistState>) => void;
  onClear: () => void;
  sortOptions: [string, string][];
  resultCount?: number;
  showMinProbes?: boolean;
}) {
  const isDefault =
    state.sort === DEFAULT_WORKLIST.sort &&
    state.order === DEFAULT_WORKLIST.order &&
    state.minProbes === DEFAULT_WORKLIST.minProbes;

  return (
    <div className="flex flex-wrap items-center gap-2 border-b border-border-primary pb-3">
      {/* Sort field */}
      <label className="flex items-center gap-1.5 text-xs text-text-quaternary">
        Sort
        <select
          value={state.sort}
          onChange={(e) => onChange({ sort: e.target.value })}
          className="rounded-md border border-border-primary bg-bg-tertiary px-2 py-1 text-xs text-text-primary focus:border-brand-500 focus:outline-none"
        >
          {sortOptions.map(([v, l]) => (
            <option key={v} value={v}>{l}</option>
          ))}
        </select>
      </label>

      {/* Sort direction toggle */}
      <button
        type="button"
        onClick={() => onChange({ order: state.order === "desc" ? "asc" : "desc" })}
        className="flex items-center gap-1 rounded-md border border-border-primary bg-bg-tertiary px-2 py-1 text-xs text-text-secondary hover:border-border-secondary"
        title={state.order === "desc" ? "Descending (worst first)" : "Ascending"}
      >
        {state.order === "desc" ? <ChevronDown className="h-3 w-3" /> : <ChevronUp className="h-3 w-3" />}
        {state.order === "desc" ? "Worst first" : "Best first"}
      </button>

      {/* Min probes filter */}
      {showMinProbes && (
        <label className="flex items-center gap-1.5 text-xs text-text-quaternary" title="Minimum probes required for an item to rank (suppresses single-probe noise)">
          Min probes
          <input
            type="number"
            min={1}
            max={50}
            value={state.minProbes}
            onChange={(e) => onChange({ minProbes: Math.max(1, Number(e.target.value) || 1) })}
            className="w-14 rounded-md border border-border-primary bg-bg-tertiary px-2 py-1 text-xs text-text-primary focus:border-brand-500 focus:outline-none"
          />
        </label>
      )}

      {/* Result count */}
      {resultCount != null && (
        <span className="ml-auto text-xs text-text-quaternary">{resultCount} results</span>
      )}

      {/* Clear filters */}
      {!isDefault && (
        <button
          type="button"
          onClick={onClear}
          className={cn("flex items-center gap-1 rounded-md px-2 py-1 text-xs text-text-quaternary hover:text-text-secondary")}
        >
          <X className="h-3 w-3" /> Clear
        </button>
      )}
    </div>
  );
}
