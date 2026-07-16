// Feature flags. Each flag defaults to off unless explicitly enabled via its
// VITE_FEATURE_* env var (set to "true" in a .env file or the shell environment
// at build time). Reads happen once at module load; toggling requires a rebuild.
//
// To enable a flag locally, create web/.env (see web/.env.example):
//   VITE_FEATURE_ALERTS=true

function boolEnv(value: string | undefined): boolean {
  return value === "true";
}

export const FEATURES = {
  // Alerting UI (rule builder, active alerts, events). Disabled until ready.
  alerts: boolEnv(import.meta.env.VITE_FEATURE_ALERTS),
} as const;

export type FeatureKey = keyof typeof FEATURES;
