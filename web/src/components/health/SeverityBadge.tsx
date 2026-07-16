import { AlertOctagon, AlertTriangle, Activity, CheckCircle2, HelpCircle } from "lucide-react";
import { cn } from "@/lib/utils";
import { type SeverityInfo } from "./severity";

// SeverityBadge renders a severity level as a labeled badge with an icon shape
// (not color alone): critical=octagon, high=triangle, watch=activity,
// normal=check, unknown=help. Understandable in grayscale and by screen reader.
export function SeverityBadge({
  severity,
  className,
}: {
  severity: SeverityInfo;
  className?: string;
}) {
  const Icon = iconFor(severity.level);
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium",
        severity.badge,
        className,
      )}
      title={severity.def}
    >
      <Icon className="h-3 w-3" aria-hidden />
      {severity.label}
    </span>
  );
}

function iconFor(level: string) {
  switch (level) {
    case "critical":
      return AlertOctagon;
    case "high":
      return AlertTriangle;
    case "watch":
      return Activity;
    case "normal":
      return CheckCircle2;
    default:
      return HelpCircle;
  }
}
