/// <reference types="vite/client" />

// Typed custom Vite env vars (see `client/src/api/client.ts`). Optional so the
// build works whether or not the deployer sets an explicit backend URL.
interface ImportMetaEnv {
  /** Base URL prefixed to every API request. Unset ⇒ same-origin `/api/...`. */
  readonly VITE_API_BASE_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
