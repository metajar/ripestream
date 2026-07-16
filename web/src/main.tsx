import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
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
    },
  },
});

const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [
      { index: true, element: <OverviewPage /> },
      { path: "asns", element: <ASNsPage /> },
      { path: "asn/:asn", element: <ASNDetailPage /> },
      { path: "probes", element: <ProbesPage /> },
      { path: "probe/:id", element: <ProbeDetailPage /> },
      { path: "targets", element: <TargetsPage /> },
      { path: "target/:addr", element: <TargetDetailPage /> },
      { path: "transit", element: <TransitPage /> },
      { path: "path", element: <PathPage /> },
      { path: "topology", element: <TopologyPage /> },
      { path: "alerts", element: <AlertsPage /> },
    ],
  },
]);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
