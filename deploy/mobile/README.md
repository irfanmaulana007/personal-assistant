# Mobile (React Native) — reserved

The React Native mobile app is **not built yet**. This directory and the
`deploy-mobile` workflow reserve the third service's slot so mobile can ship
independently of web and backend once it exists.

- The app will live under `mobile/` at the repo root and consume the shared
  `@personal-assistant/shared` package (types, utils, platform-agnostic API
  client) — the same package the web app already uses.
- Building the app (auth + navigation, consuming the backend APIs, iOS/Android
  builds, mobile feature-management/RBAC) is tracked separately:
  **https://trello.com/c/TiY5RcSa**
- When the app lands, replace `.github/workflows/deploy-mobile.yml`'s placeholder
  job with the real build+submit steps (e.g. Expo EAS Build / Fastlane).
