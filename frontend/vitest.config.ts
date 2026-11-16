import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// Unit and component tests, run in jsdom (a browser-like DOM inside Node; no real browser or server).
export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['src/test/setup.ts'],
  },
})
