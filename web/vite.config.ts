import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// The Go server embeds dist/ (see web/embed.go) and serves it with the API on
// one origin. In development, `npm run dev` proxies the API to a running
// `servly serve` on :8080.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
      '/readyz': 'http://localhost:8080',
    },
  },
});
