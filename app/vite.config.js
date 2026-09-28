import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { VitePWA } from 'vite-plugin-pwa';

// La app vive en /app/ detrás de Caddy (http://localhost:4700 en local).
// En desarrollo, /api y /login se reenvían a Caddy para trabajar con el mismo origen.
const EDGE = process.env.EDISYS_EDGE || 'http://localhost:4700';

export default defineConfig({
  base: '/app/',
  plugins: [
    react(),
    VitePWA({
      registerType: 'prompt',
      injectRegister: false,
      manifestFilename: 'manifest.webmanifest',
      includeAssets: ['favicon.svg', 'icon-192.png', 'icon-512.png'],
      manifest: {
        name: 'EDISYS',
        short_name: 'EDISYS',
        description: 'Administración de edificios: cuotas, recibos, balance, reservas y mantenimiento.',
        lang: 'es-PE',
        start_url: '/app/',
        scope: '/app/',
        display: 'standalone',
        background_color: '#F8FAFC',
        theme_color: '#0F172A',
        icons: [
          { src: 'icon-192.png', sizes: '192x192', type: 'image/png' },
          { src: 'icon-512.png', sizes: '512x512', type: 'image/png' },
          { src: 'icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
      workbox: {
        // Solo estáticos: las respuestas del API NUNCA se cachean (un saldo viejo es peor que un error).
        globPatterns: ['**/*.{js,css,html,svg,png,webmanifest}'],
        navigateFallback: '/app/index.html',
        navigateFallbackDenylist: [/^\/api\//, /^\/login/],
        runtimeCaching: [],
      },
    }),
  ],
  server: {
    port: 5173,
    fs: { allow: ['..'] },
    proxy: {
      '/api': { target: EDGE, changeOrigin: false },
      '/login': { target: EDGE, changeOrigin: false },
    },
  },
  build: { outDir: 'dist', sourcemap: false },
  test: {
    environment: 'node',
    include: ['src/**/*.test.js'],
  },
});
