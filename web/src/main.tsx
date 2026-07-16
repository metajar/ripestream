import { QueryClient, QueryClientProvider, keepPreviousData } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, RouterProvider } from "react-router-dom";
import { AppShell } from "@/components/AppShell";
import { OverviewPage } from "@/pages/OverviewPage";
import { ASNsPage } from "@/pages/ASNsPage";
import { ASNDetailPage } from "@/pages/ASNDetailPage";
import { ProbesPage } from "@/pages/ProbesPage";
import { ProbeDetailPage } from "@/pages/ProbeDetailPage";
import { TargetsPage } from "@/pages/TargetsPage";
import { TargetDetailPage } from "@/pages/TargetDetailPage";
import { TransitPage } from "@/pages/TransitPage";
import { TransitPairDetailPage } from "@/pages/TransitPairDetailPage";
import { PathPage } from "@/pages/PathPage";
import { TopologyPage } from "@/pages/TopologyPage";
import { AlertsPage } from "@/pages/AlertsPage";
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
      { index: true, element: <OverviewPage /> },
      { path: "asns", element: <ASNsPage /> },
      { path: "asn/:asn", element: <ASNDetailPage /> },
      { path: "probes", element: <ProbesPage /> },
      { path: "probe/:id", element: <ProbeDetailPage /> },
      { path: "targets", element: <TargetsPage /> },
      { path: "target/:addr", element: <TargetDetailPage /> },
      { path: "transit", element: <TransitPage /> },
      { path: "transit/:asnA/:asnB", element: <TransitPairDetailPage /> },
      { path: "path", element: <PathPage /> },
      { path: "topology", element: <TopologyPage /> },
      { path: "alerts", element: <AlertsPage /> },
    ],
  },
]);

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
