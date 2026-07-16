import { cn } from "@/lib/utils";
import type { OverviewMeta } from "@/api/client";

// DataStatus renders a compact freshness label from overview cache metadata.
// It is the shared version of the inline component originally in OverviewPage.
// Never calls cached data "live". States are text + dot, not color alone.
export function DataStatus({
  meta,
  className,
}: {
  meta?: OverviewMeta;
  className?: string;
}) {
  if (!meta) {
    return (
      <p className={cn("flex items-center gap-1.5 text-xs text-text-tertiary", className)}>
        <span className="h-1.5 w-1.5 rounded-full bg-info-500" aria-hidden />
        Live query
      </p>
    );
  }
  let label: string;
  let dot: string;
  if (!meta.last_ok) {
    label = "Data refresh failed — showing last cached view";
    dot = "bg-error-500";
  } else if (meta.stale) {
    label = "Stale cached view";
    dot = "bg-warning-500";
  } else {
    label = "Current cached view";
    dot = "bg-success-500";
  }
  const title = `Cached overview refreshed every ${meta.ttl_sec}s; last refresh took ${meta.took_ms}ms.`;
  return (
    <p className={cn("flex items-center gap-1.5 text-xs text-text-tertiary", className)} title={title}>
      <span className={cn("h-1.5 w-1.5 rounded-full", dot)} aria-hidden />
      {label}
    </p>
  );
}
