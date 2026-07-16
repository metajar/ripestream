import { Suspense, lazy } from "react";

// react-force-graph-2d depends on canvas/DOM globals only present in the browser,
// so we lazy-load it to keep the initial bundle light and avoid import-time issues.
const ForceGraph2D = lazy(async () => {
  const mod = await import("react-force-graph-2d");
  return { default: mod.default };
});

// Default export wraps the lazy component in a Suspense boundary with a fallback,
// so callers can use <Dynamic {...props} /> without their own Suspense.
export default function Dynamic(props: Record<string, unknown>) {
  return (
    <Suspense fallback={<div className="flex h-full items-center justify-center text-sm text-text-quaternary">Loading graph…</div>}>
      {/* eslint-disable-next-line @typescript-eslint/no-explicit-any */}
      <ForceGraph2D {...(props as any)} />
    </Suspense>
  );
}
