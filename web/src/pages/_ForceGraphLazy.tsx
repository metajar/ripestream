import { Suspense, forwardRef, lazy } from "react";
import { LoadingState } from "@/components/ui/states";

// react-force-graph-2d depends on canvas/DOM globals only present in the browser,
// so we lazy-load it to keep the initial bundle light and avoid import-time issues.
const ForceGraph2D = lazy(async () => {
  const mod = await import("react-force-graph-2d");
  return { default: mod.default };
});

// Default export wraps the lazy component in a Suspense boundary with a fallback,
// so callers can use <Dynamic {...props} /> without their own Suspense.
const Dynamic = forwardRef<unknown, Record<string, unknown>>(function Dynamic(props, ref) {
  return (
    <Suspense fallback={<LoadingState label="Loading graph…" className="h-full py-0" />}>
      {/* eslint-disable-next-line @typescript-eslint/no-explicit-any */}
      <ForceGraph2D {...(props as any)} ref={ref as any} />
    </Suspense>
  );
});

export default Dynamic;
