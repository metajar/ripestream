import { cn, fmtNum } from "@/lib/utils";

// Evidence renders a compact "N probes · N samples · last observed …" line.
// Omit unavailable fields rather than showing a fabricated zero denominator.
export function Evidence({
  probes,
  samples,
  lastSeenSec,
  className,
}: {
  probes?: number | null;
  samples?: number | null;
  lastSeenSec?: number | null;
  className?: string;
}) {
  const parts: string[] = [];
  if (probes != null && probes > 0) parts.push(`${fmtNum(probes)} ${probes === 1 ? "probe" : "probes"}`);
  if (samples != null && samples > 0) parts.push(`${fmtNum(samples)} ${samples === 1 ? "sample" : "samples"}`);
  if (lastSeenSec && lastSeenSec > 0) {
    const diff = Date.now() / 1000 - lastSeenSec;
    if (diff < 60) parts.push(`last observed ${Math.round(diff)}s ago`);
    else if (diff < 3600) parts.push(`last observed ${Math.round(diff / 60)}m ago`);
    else parts.push(`last observed ${new Date(lastSeenSec * 1000).toISOString().slice(11, 16)} UTC`);
  }
  if (parts.length === 0) return null;
  return <span className={cn("text-xs text-text-quaternary", className)}>{parts.join(" · ")}</span>;
}
