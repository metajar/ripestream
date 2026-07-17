import { useQuery } from "@tanstack/react-query";
import { Activity } from "lucide-react";
import { Area, AreaChart, ResponsiveContainer } from "recharts";
import { api } from "@/api/client";

const compactNumber = new Intl.NumberFormat("en", {
  notation: "compact",
  maximumFractionDigits: 1,
});

const rateNumber = new Intl.NumberFormat("en", {
  maximumFractionDigits: 0,
});

export function IngestionRate() {
  const { data, error } = useQuery({
    queryKey: ["ingestion"],
    queryFn: api.ingestion,
    refetchInterval: 2_000,
    staleTime: 0,
  });

  return (
    <section
      className="mx-3 mb-3 rounded-lg border border-border-primary bg-bg-primary/55 px-3 py-2.5"
      aria-label="Live stream ingestion"
      title="RIPE Atlas measurement results read from the stream. The rate is averaged over up to five completed seconds."
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-text-quaternary">
          <Activity className="h-3 w-3 text-success-500" />
          Live ingestion
        </div>
        <span
          className={`flex items-center gap-1 text-[10px] ${
            data ? "text-success-500" : error ? "text-error-500" : "text-text-quaternary"
          }`}
        >
          <span
            className={`h-1.5 w-1.5 rounded-full ${
              data ? "bg-success-500" : error ? "bg-error-500" : "bg-text-quaternary"
            }`}
          />
          {data ? "Live" : error ? "Offline" : "Connecting"}
        </span>
      </div>

      {data ? (
        <>
          <div className="mt-1.5 flex items-baseline gap-1.5">
            <span className="font-mono text-xl font-semibold tabular-nums text-text-primary">
              {rateNumber.format(data.tests_per_second)}
            </span>
            <span className="text-[11px] text-text-quaternary">tests / sec</span>
          </div>
          <div className="mt-1 h-12 w-full" aria-hidden="true">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={data.samples} margin={{ top: 3, right: 0, bottom: 0, left: 0 }}>
                <defs>
                  <linearGradient id="ingestionFill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="var(--color-success-500)" stopOpacity={0.45} />
                    <stop offset="100%" stopColor="var(--color-success-500)" stopOpacity={0.02} />
                  </linearGradient>
                </defs>
                <Area
                  type="monotone"
                  dataKey="tests"
                  stroke="var(--color-success-500)"
                  strokeWidth={1.5}
                  fill="url(#ingestionFill)"
                  isAnimationActive={false}
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-0.5 flex items-center justify-between text-[10px] text-text-quaternary">
            <span>Last {data.window_seconds}s</span>
            <span className="font-mono tabular-nums">{compactNumber.format(data.total)} read this run</span>
          </div>
        </>
      ) : (
        <div className="flex h-[76px] items-center text-[11px] text-text-quaternary">
          {error ? "Ingestion metrics unavailable" : "Reading stream rate…"}
        </div>
      )}
    </section>
  );
}
