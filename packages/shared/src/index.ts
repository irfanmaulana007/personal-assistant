// @personal-assistant/shared — code shared across the platform's clients (the
// existing web app and the future React Native mobile app):
//   - `types`  the API/domain TypeScript types
//   - `utils`  framework-agnostic helpers (formatting, filters, …)
//   - `api`    the platform-agnostic API client (see `./api/client`)
//
// Subpath entry points (`@personal-assistant/shared/types`, `/utils`, `/api`)
// are also exported for callers that want a narrower import.
export * from './types';
export * from './utils';
export * from './api';
