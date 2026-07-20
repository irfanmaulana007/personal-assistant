# Web app image — builds the React/Vite client (with the `@personal-assistant/shared`
# workspace package) into static assets and serves them with nginx. Deploys
# independently of the backend.
#
# Build from the repo root:
#   docker build -f app/web/Dockerfile \
#     --build-arg VITE_API_BASE_URL=https://api.example.com -t pa-web .
#
# VITE_API_BASE_URL (build-time) is baked into the bundle as the API origin. Leave
# it empty to call the backend same-origin (`/api/...`) through the nginx proxy
# below — which is the model the combined server uses today.
FROM node:22-alpine AS builder
RUN corepack enable
WORKDIR /app
# Workspace manifests + lockfile first, for cacheable dependency layers.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages/shared/package.json ./packages/shared/package.json
COPY app/web/package.json ./app/web/package.json
# pnpm's lockfile records optional native deps for every platform, so a frozen
# install resolves the Linux/musl-specific binaries even though the lockfile is
# generated on a different platform.
RUN pnpm install --frozen-lockfile
# Sources for the shared package and the web app.
COPY packages/ ./packages/
COPY app/web/ ./app/web/
ARG VITE_API_BASE_URL=""
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
# vite.config.ts reads the app version from the root package.json (already copied).
RUN pnpm --filter web build

FROM nginx:1.27-alpine
# nginx:alpine runs envsubst over /etc/nginx/templates/*.template on startup,
# substituting only variables present in the environment (so nginx's own $uri,
# $host, … are left intact). BACKEND_URL points the /api proxy at the backend.
ENV BACKEND_URL=http://backend:8090
COPY app/web/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=builder /app/app/web/dist /usr/share/nginx/html
EXPOSE 80
