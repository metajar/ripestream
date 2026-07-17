import { QueryClient, QueryClientProvider, keepPreviousData } from "@tanstack/react-query";
import { StrictMode, type ComponentType } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, RouterProvider } from "react-router-dom";
import { AppShell } from "@/components/AppShell";
import { FEATURES } from "@/lib/features";
import "@/index.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchInterval: 30_000, // live-ish; topology updates every few seconds
      staleTime: 15_000,
      retry: 1,
      // Preserve prior data during refetch so a failed/slow refresh never
      // blanks the page — the UI shows stale data rather than nothing.
      placeholderData: keepPreviousData,
    },
  },
});

const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    errorElement: <RouteError />,
    children: [
      { index: true, lazy: route(() => import("@/pages/OverviewPage"), "OverviewPage") },
      { path: "asns", lazy: route(() => import("@/pages/ASNsPage"), "ASNsPage") },
      { path: "asn/:asn", lazy: route(() => import("@/pages/ASNDetailPage"), "ASNDetailPage") },
      { path: "probes", lazy: route(() => import("@/pages/ProbesPage"), "ProbesPage") },
      { path: "probe/:id", lazy: route(() => import("@/pages/ProbeDetailPage"), "ProbeDetailPage") },
      { path: "targets", lazy: route(() => import("@/pages/TargetsPage"), "TargetsPage") },
      { path: "target/:addr", lazy: route(() => import("@/pages/TargetDetailPage"), "TargetDetailPage") },
      { path: "ip/:addr", lazy: route(() => import("@/pages/IPDetailPage"), "IPDetailPage") },
      { path: "ip/:addr/graph", lazy: route(() => import("@/pages/IPRouteGraphPage"), "IPRouteGraphPage") },
      { path: "transit", lazy: route(() => import("@/pages/TransitPage"), "TransitPage") },
      { path: "correlation", lazy: route(() => import("@/pages/RouteCorrelationPage"), "RouteCorrelationPage") },
      { path: "transit/:asnA/:asnB", lazy: route(() => import("@/pages/TransitPairDetailPage"), "TransitPairDetailPage") },
      { path: "path", lazy: route(() => import("@/pages/PathPage"), "PathPage") },
      { path: "topology", lazy: route(() => import("@/pages/TopologyPage"), "TopologyPage") },
      // Alerting UI is behind a feature flag; the route is registered only
      // when VITE_FEATURE_ALERTS=true. Direct navigation to /alerts otherwise
      // falls through to the router's errorElement (RouteError / "not found").
      ...(FEATURES.alerts
        ? [{ path: "alerts", lazy: route(() => import("@/pages/AlertsPage"), "AlertsPage") }]
        : []),
    ],
  },
]);

function route(load: () => Promise<unknown>, name: string) {
  return async () => {
    const module = (await load()) as Record<string, ComponentType>;
    return { Component: module[name] };
  };
}

// RouteError is the top-level error boundary for the router. It catches render
// errors and unmatched routes (404), showing a graceful message with a link
// back to safety instead of the raw "Unexpected Application Error!" dev screen.
function RouteError() {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
      <div className="text-2xl">🤔</div>
      <h1 className="text-lg font-semibold text-text-primary">Page not found</h1>
      <p className="max-w-md text-sm text-text-quaternary">
        This page doesn't exist or couldn't be loaded. The link may be broken or the route may have changed.
      </p>
      <a
        href="/"
        className="mt-2 rounded-lg bg-brand-500 px-4 py-2 text-sm font-medium text-white hover:bg-brand-600"
      >
        Back to Network health
      </a>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
