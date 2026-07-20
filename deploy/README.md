# Deployment

The project is an **npm-workspaces monorepo** heading toward three services that
build and deploy **independently**:

| Service     | Source                     | Image (Dockerfile)          | Pipeline                          |
| ----------- | -------------------------- | --------------------------- | --------------------------------- |
| **Backend** | `app/api/` (Go API)         | `deploy/backend.Dockerfile` | `.github/workflows/deploy-backend.yml` |
| **Web**     | `app/web/` (React/Vite)     | `deploy/web.Dockerfile`     | `.github/workflows/deploy-web.yml` |
| **Mobile**  | `app/mobile/` (React Native)*  | — (EAS/Fastlane)*           | `.github/workflows/deploy-mobile.yml`* |

`packages/shared` (`@personal-assistant/shared`) holds the code shared across the
clients — TypeScript types, framework-agnostic utils, and the platform-agnostic
API client. The web app consumes it today; the mobile app will consume the same
package.

\* Mobile is a **reserved slot** — the app is not built yet. See
[`deploy/mobile/README.md`](./mobile/README.md) and the tracking card
<https://trello.com/c/TiY5RcSa>.

## Independent pipelines

Each deploy workflow is **path-filtered**, so a change to one service ships only
that service:

- `deploy-backend` runs on changes under `app/api/**`, `go.mod`, `go.sum`, or its
  Dockerfile.
- `deploy-web` runs on changes under `app/web/**`, `packages/**` (the web bundle
  embeds the shared code), or its Dockerfile / nginx template.
- `deploy-mobile` runs on changes under `app/mobile/**` or `packages/**`.

Deploy workflows build and push per-service images to GHCR
(`ghcr.io/<owner>/personal-assistant/{backend,web}`) using the built-in
`GITHUB_TOKEN`. The final push-to-host step is infra-specific and marked as a
TODO in each workflow.

`ci.yml` validates both the web workspace (shared typecheck + client lint +
build) and the backend (vet + build) on every PR to `staging`/`main`.

## Deploying to a host (Dokploy)

The deploy workflows push images to GHCR but leave the final push-to-host step as
a TODO. See [`DOKPLOY.md`](./DOKPLOY.md) for a step-by-step guide to running the
split stack on [Dokploy](https://dokploy.com) — provisioning the databases, the
backend + web applications from the GHCR images, and wiring the workflows to
trigger per-service redeploys.

## Building locally

```sh
# Per-service images
make docker-build-backend
make docker-build-web VITE_API_BASE_URL=https://api.example.com   # empty ⇒ same-origin proxy

# Run the split stack (backend + web + Postgres + Mongo) — web on :8080
docker compose -f deploy/docker-compose.split.yml --env-file .env up --build
```

The repo-root `Dockerfile` + `docker-compose.yml` still build and run the
**combined** all-in-one image (Go server serving the bundled web assets) for
quick local dev — unchanged by the split.

## Web ↔ backend wiring

- **Same-origin (default):** build the web image with `VITE_API_BASE_URL` empty.
  The web app requests `/api/...`, and nginx proxies to `BACKEND_URL`
  (default `http://backend:8090`). This matches how the combined server works.
- **Cross-origin:** build with `VITE_API_BASE_URL=https://api.example.com`. The
  web app calls the backend directly; the nginx `/api` proxy is unused. (The
  backend must allow CORS from the web origin in that mode.)
