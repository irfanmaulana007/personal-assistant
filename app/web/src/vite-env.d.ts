/// <reference types="vite/client" />

// Typed custom Vite env vars (see `client/src/api/client.ts`). Optional so the
// build works whether or not the deployer sets an explicit backend URL.
interface ImportMetaEnv {
  /** Base URL prefixed to every API request. Unset ⇒ same-origin `/api/...`. */
  readonly VITE_API_BASE_URL?: string;
  /** Sentry project DSN. Unset ⇒ Sentry disabled (see `lib/sentry.ts`). */
  readonly VITE_SENTRY_DSN?: string;
  /** Sentry environment label. Unset ⇒ falls back to Vite's `MODE`. */
  readonly VITE_SENTRY_ENVIRONMENT?: string;
  /** Fraction of transactions traced for performance (default `1.0`). */
  readonly VITE_SENTRY_TRACES_SAMPLE_RATE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
