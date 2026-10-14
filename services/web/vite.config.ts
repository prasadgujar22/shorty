import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

// `npm run dev` proxies the API to the docker-compose stack
// (deploy/compose/dev.yml) or any gateway via API_URL.
const links = process.env.LINKS_URL ?? process.env.API_URL ?? 'http://localhost:18081';
const stats = process.env.STATS_URL ?? process.env.API_URL ?? 'http://localhost:18083';

export default defineConfig({
  plugins: [
    sveltekit({
      // Pure SPA: Nginx serves index.html for /s/* (see nginx.conf).
      adapter: adapter({ fallback: 'index.html', precompress: true }),
      // One JS and one CSS file: fewer requests, simpler budget accounting.
      output: { bundleStrategy: 'single' }
    })
  ],
  server: {
    proxy: {
      '/api/links': links,
      '/api/stats': stats
    }
  },
  build: { target: 'es2022', cssMinify: true },
  test: { include: ['src/**/*.test.ts'], environment: 'node' }
});
