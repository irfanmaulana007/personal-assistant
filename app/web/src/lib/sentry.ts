// Sentry wiring for the web client — error and performance monitoring, plus
// session replay on errors. Initialization is best-effort: when
// `VITE_SENTRY_DSN` is unset (the default for local development) the SDK stays
// disabled and the app runs unchanged.
import * as Sentry from '@sentry/react';
import { APP_VERSION } from '../appVersion';

/**
 * Initialize Sentry from build-time env vars. A no-op when `VITE_SENTRY_DSN` is
 * empty. Call once, before the app renders (see `main.tsx`).
 *
 * - `VITE_SENTRY_DSN` — the project DSN; unset disables Sentry.
 * - `VITE_SENTRY_ENVIRONMENT` — environment label; falls back to Vite's `MODE`
 *   (`development` / `production`).
 * - `VITE_SENTRY_TRACES_SAMPLE_RATE` — fraction of transactions traced for
 *   performance (default `1.0`; errors are always captured).
 */
export function initSentry(): void {
  const dsn = import.meta.env.VITE_SENTRY_DSN;
  if (!dsn) return;

  const tracesSampleRate = Number(import.meta.env.VITE_SENTRY_TRACES_SAMPLE_RATE ?? '1');

  Sentry.init({
    dsn,
    environment: import.meta.env.VITE_SENTRY_ENVIRONMENT ?? import.meta.env.MODE,
    release: APP_VERSION,
    integrations: [
      Sentry.browserTracingIntegration(),
      // Record a replay only when an error occurs — high signal, low volume.
      Sentry.replayIntegration({ maskAllText: false, blockAllMedia: false }),
    ],
    // Performance tracing sample rate (errors are captured regardless).
    tracesSampleRate: Number.isFinite(tracesSampleRate) ? tracesSampleRate : 1,
    // Session replay: none for ordinary sessions, always on when an error fires.
    replaysSessionSampleRate: 0,
    replaysOnErrorSampleRate: 1.0,
  });
}
