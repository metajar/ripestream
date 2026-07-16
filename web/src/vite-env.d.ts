/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_FEATURE_ALERTS?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
