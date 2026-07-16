import { AlertCircle, Inbox, Loader2 } from "lucide-react";
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

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={cn("h-4 w-4 animate-spin", className)} />;
}

export function LoadingState({ label = "Loading…" }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-2 py-12 text-text-tertiary">
      <Spinner className="text-brand-500" />
      <span className="text-sm">{label}</span>
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
