// `__APP_VERSION__` is replaced at build time by Vite (see vite.config.ts) with
// the root package.json `version` — the single source of truth bumped by the
// /release command.
declare const __APP_VERSION__: string;

/** The app version, e.g. "1.0.3". */
export const APP_VERSION = __APP_VERSION__;

/** The app version prefixed with "v", e.g. "v1.0.3". */
export const APP_VERSION_LABEL = `v${__APP_VERSION__}`;
