// EDISYS · app. Astro es el host de las vistas (una página estática por pantalla)
// y cada pantalla es una isla React + Tailwind. Salida estática en dist/, servida por Caddy en /app/.
import { defineConfig } from 'astro/config';
import react from '@astrojs/react';
import tailwind from '@astrojs/tailwind';
import AstroPWA from '@vite-pwa/astro';

const EDGE = process.env.EDISYS_EDGE || 'http://localhost:4700';

/** El muestrario de componentes (/app/ui) solo existe en desarrollo (§2.2). */
const muestrario = {
  name: 'edisys-muestrario',
  hooks: {
    'astro:config:setup': ({ command, injectRoute }) => {
      if (command === 'dev') injectRoute({ pattern: '/ui', entrypoint: './src/muestrario/ui.astro' });
    },
  },
};

export default defineConfig({
  base: '/app',
  trailingSlash: 'ignore',
  output: 'static',
  build: { format: 'directory', assets: '_astro' },
  outDir: './dist',
  integrations: [
    react(),
    tailwind({ applyBaseStyles: false }),
    muestrario,
    AstroPWA({
      base: '/app/',
      scope: '/app/',
      registerType: 'prompt',
      injectRegister: false,
      manifestFilename: 'manifest.webmanifest',
      includeAssets: ['favicon.svg', 'icon-192.png', 'icon-512.png'],
      manifest: {
        name: 'EDISYS',
        short_name: 'EDISYS',
        description: 'Administración de edificios: cuotas, recibos, balance, reservas y mantenimiento.',
        lang: 'es-PE',
        id: '/app/',
        start_url: '/app/',
        scope: '/app/',
        display: 'standalone',
        background_color: '#F8FAFC',
        theme_color: '#0F172A',
        icons: [
          { src: '/app/icon-192.png', sizes: '192x192', type: 'image/png' },
          { src: '/app/icon-512.png', sizes: '512x512', type: 'image/png' },
          { src: '/app/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
      workbox: {
        // Solo estáticos: las respuestas del API NUNCA se cachean (un saldo viejo es peor que un error).
        globPatterns: ['**/*.{js,css,html,svg,png,webmanifest}'],
        navigateFallback: null,
        directoryIndex: 'index.html',
        runtimeCaching: [],
      },
      devOptions: { enabled: false },
    }),
  ],
  vite: {
    envPrefix: ['PUBLIC_', 'VITE_'],
    server: {
      proxy: {
        '/api': { target: EDGE, changeOrigin: false },
        '/login': { target: EDGE, changeOrigin: false },
      },
    },
  },
  server: { port: 5173 },
});
