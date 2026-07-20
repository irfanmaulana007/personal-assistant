import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// The single source of truth for the app version is the root package.json
// `version` field (bumped by the /release command). Inject it into the client
// bundle at build time so the UI and the run-detail copy can show it. Fall back
// to '0.0.0' if the file is unreachable (e.g. an isolated build context) rather
// than failing the whole build.
let appVersion = '0.0.0'
try {
  const rootPkg = JSON.parse(
    readFileSync(fileURLToPath(new URL('../../package.json', import.meta.url)), 'utf-8'),
  )
  appVersion = rootPkg.version ?? appVersion
} catch {
  console.warn('[vite] root package.json not found; defaulting __APP_VERSION__ to 0.0.0')
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
  },
  server: {
    port: 5273,
    strictPort: true,
    proxy: {
      '/api': {
        target: 'http://localhost:8090',
        changeOrigin: true,
      },
    },
  },
})
