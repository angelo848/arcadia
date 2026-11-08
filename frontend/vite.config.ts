import { fileURLToPath } from 'node:url'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vitest/config'

// dist/.gitkeep (needed by //go:embed all:dist so `go build` works without Node)
// is restored on every build from public/.gitkeep, which Vite copies after emptyOutDir.
const outDir = fileURLToPath(new URL('./dist', import.meta.url))

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir,
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    globals: true,
  },
})
