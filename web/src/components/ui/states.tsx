import { AlertCircle, Inbox } from "lucide-react";
import { CableSpinner } from "@/components/ui/CableSpinner";
import { cn, fmtRelative, fmtUtc } from "@/lib/utils";

// Freshness renders a relative "last observed" time with the exact UTC timestamp
// in a native title tooltip. Renders an explicit "no observation" state rather
// than a fake "0s ago" when sec is missing/zero.
export function Freshness({ sec }: { sec: number | null | undefined }) {
  if (!sec || sec <= 0) {
    return <span className="text-text-quaternary">no observation</span>;
  }
  return (
    <span title={`Last observed ${fmtUtc(sec)}`} className="cursor-help">
      {fmtRelative(sec)}
    </span>
  );
}

export function Spinner({ className, size }: { className?: string; size?: number }) {
  return <CableSpinner className={className} size={size} />;
}

export function LoadingState({
  label = "Loading…",
  className,
}: {
  label?: string;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex items-center justify-center gap-2.5 py-12 text-text-tertiary",
        className,
      )}
      role="status"
      aria-live="polite"
      aria-busy="true"
    >
      <Spinner size={22} />
      <span className="text-sm">{label}</span>
    </div>
  );
}

/** Dimmed overlay while a query key change is in flight (pagination / filters).
 *  Skips background refetches — callers should pass `isFetching && isPlaceholderData`. */
export function FetchingOverlay({
  active,
  label = "Loading…",
}: {
  active: boolean;
  label?: string;
}) {
  if (!active) return null;
  return (
    <div
      className="absolute inset-0 z-10 flex items-center justify-center rounded-md bg-bg-secondary/70 backdrop-blur-[1px]"
      role="status"
      aria-live="polite"
      aria-busy="true"
    >
      <div className="flex items-center gap-2.5 rounded-lg border border-border-primary bg-bg-elevated px-4 py-2.5 shadow-lg">
        <Spinner size={20} />
        <span className="text-sm text-text-secondary">{label}</span>
      </div>
    </div>
  );
}

export function ErrorState({ message }: { message: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-12 text-error-500">
      <AlertCircle className="h-5 w-5" />
      <span className="text-sm">{message}</span>
    </div>
  );
}

export function EmptyState({
  label = "No data",
  hint,
}: {
  label?: string;
  hint?: string;
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-1 py-12 text-text-tertiary">
      <Inbox className="h-5 w-5" />
      <span className="text-sm font-medium">{label}</span>
      {hint && <span className="text-xs text-text-quaternary">{hint}</span>}
    </div>
  );
}
