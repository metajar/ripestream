import { cn } from "@/lib/utils";

// Metric renders a labeled value with optional unit, tooltip, and secondary/
// denominator text. Used for KPI-style displays and inline metric rows.
export function Metric({
  label,
  value,
  unit,
  tooltip,
  secondary,
  className,
}: {
  label: string;
  value: string;
  unit?: string;
  tooltip?: string;
  secondary?: string;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col gap-0.5", className)} title={tooltip}>
      <span className="text-xs text-text-quaternary">{label}</span>
      <span className="text-lg font-semibold text-text-primary">
        {value}
        {unit && <span className="ml-1 text-sm font-normal text-text-tertiary">{unit}</span>}
      </span>
      {secondary && <span className="text-xs text-text-quaternary">{secondary}</span>}
    </div>
  );
}
