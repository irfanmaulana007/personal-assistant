// Web binding for the shared, platform-agnostic API client.
//
// The client itself lives in `@personal-assistant/shared/api` so the web app and
// the future React Native app share one implementation. Here we wire in the
// web-specific base URL and re-export the whole client so existing
// `../api/client` imports across the web app keep working unchanged.
//
// `VITE_API_BASE_URL` lets the web app be deployed independently of the backend
// (point it at the backend's public URL). Left unset — the default — every
// request is same-origin (`/api/...`), matching the combined server + static
// build that serves the client today. Token storage (localStorage) and the
// 401 → reload behavior come from the shared client's defaults.
import { configureApiClient } from '@personal-assistant/shared/api';

configureApiClient({
  baseUrl: import.meta.env.VITE_API_BASE_URL ?? '',
});

export * from '@personal-assistant/shared/api';
