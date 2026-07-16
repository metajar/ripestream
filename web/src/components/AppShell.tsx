import {
  Activity,
  AlertTriangle,
  Globe2,
  Network,
  Route,
  Satellite,
  Target,
  Waypoints,
  Zap,
} from "lucide-react";
import { NavLink, Outlet } from "react-router-dom";
import { GlobalSearch } from "@/components/GlobalSearch";
import { FEATURES } from "@/lib/features";
import { cn } from "@/lib/utils";

// Navigation is grouped by operator workflow: triage first (Attention), then
// the network entities (Network), then expert tools (Explore). All existing
// URLs are preserved; only the visual grouping and labels change.
const NAV_GROUPS = [
  {
    label: "Attention",
    items: [
      { to: "/", label: "Network health", icon: Activity, end: true },
      ...(FEATURES.alerts
        ? [{ to: "/alerts", label: "Alerts", icon: AlertTriangle }]
        : []),
      { to: "/correlation", label: "Route correlation", icon: Waypoints },
    ],
  },
  {
    label: "Network",
    items: [
      { to: "/asns", label: "ASNs", icon: Globe2 },
      { to: "/probes", label: "Probes", icon: Satellite },
      { to: "/targets", label: "Targets", icon: Target },
      { to: "/transit", label: "Transit & Hops", icon: Zap },
    ],
  },
  {
    label: "Explore",
    items: [
      { to: "/path", label: "Path Explorer", icon: Route, advanced: true },
      { to: "/topology", label: "Topology", icon: Network, advanced: true },
    ],
  },
] as const;

export function AppShell() {
  return (
    <div className="flex h-full">
      {/* Sidebar */}
      <aside className="flex w-60 shrink-0 flex-col border-r border-border-primary bg-bg-secondary">
        <div className="flex items-center gap-2 px-5 py-4">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-brand-500/15">
            <Activity className="h-5 w-5 text-brand-300" />
          </div>
          <div className="leading-tight">
            <div className="text-sm font-semibold text-text-primary">RipeStream</div>
            <div className="text-[11px] text-text-quaternary">Internet Observability</div>
          </div>
        </div>
        <nav className="flex-1 overflow-y-auto px-3 py-2">
          {NAV_GROUPS.map((group) => (
            <div key={group.label} className="mb-4">
              <div className="mb-1 px-3 text-[10px] font-semibold uppercase tracking-wider text-text-quaternary">
                {group.label}
                {group.label === "Explore" && (
                  <span className="ml-1 normal-case text-text-quaternary/70">· advanced</span>
                )}
              </div>
              <div className="space-y-0.5">
                {group.items.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    end={"end" in item ? item.end : false}
                    className={({ isActive }) =>
                      cn(
                        "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                        isActive
                          ? "bg-brand-500/10 text-brand-300"
                          : "text-text-tertiary hover:bg-bg-tertiary hover:text-text-primary",
                      )
                    }
                  >
                    <item.icon className="h-4 w-4" />
                    {item.label}
                  </NavLink>
                ))}
              </div>
            </div>
          ))}
        </nav>
        <div className="border-t border-border-primary px-5 py-3 text-[11px] text-text-quaternary">
          Data: RIPE Atlas · FalkorDB · ClickHouse
        </div>
      </aside>

      {/* Main */}
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar />
        <main className="flex-1 overflow-y-auto px-6 py-5">
          <Outlet />
        </main>
      </div>
    </div>
  );
}

function TopBar() {
  return (
    <header className="flex h-14 shrink-0 items-center gap-4 border-b border-border-primary bg-bg-secondary px-6">
      <GlobalSearch />
      <div className="ml-auto flex items-center gap-3 text-xs text-text-quaternary">
        <span className="font-mono">/api</span>
      </div>
    </header>
  );
}
