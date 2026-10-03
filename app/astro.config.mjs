// EDISYS · app. Astro es el host de las vistas (una página estática por pantalla)
// y cada pantalla es una isla React + Tailwind. Salida estática en dist/, servida por Caddy en /app/.
import { defineConfig } from 'astro/config';
import react from '@astrojs/react';
import tailwind from '@astrojs/tailwind';
import AstroPWA from '@vite-pwa/astro';
// extras: I4 · app propia (marca del build; sin variables EDISYS_APP_* sale EDISYS tal cual)
import { leerMarcaApp, manifiestoPWA } from './src/lib/appPropia.js';

const EDGE = process.env.EDISYS_EDGE || 'http://localhost:4700';

// Prefijo de URL pública: vacío en local; «/edisys» bajo el alias de boticalima.
// Debe coincidir con el que use el borde (edge) y el login.
const PREFIJO = process.env.EDISYS_PREFIJO || '';
const BASE_APP = `${PREFIJO}/app`;

// extras: I4 · marca, carpeta de estáticos (iconos) y salida, que scripts/app-propia.mjs cambia por cliente.
const MARCA = leerMarcaApp(process.env);
const PUBLIC_DIR = process.env.EDISYS_PUBLIC_DIR || './public';
const OUT_DIR = process.env.EDISYS_OUT_DIR || './dist';

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
  base: BASE_APP,
  trailingSlash: 'ignore',
  output: 'static',
  build: { format: 'directory', assets: '_astro' },
  outDir: OUT_DIR, // extras: I4
  publicDir: PUBLIC_DIR, // extras: I4
  integrations: [
    react(),
    tailwind({ applyBaseStyles: false }),
    muestrario,
    AstroPWA({
      base: `${BASE_APP}/`,
      scope: `${BASE_APP}/`,
      registerType: 'prompt',
      injectRegister: false,
      manifestFilename: 'manifest.webmanifest',
      includeAssets: ['favicon.svg', 'icon-192.png', 'icon-512.png'],
      manifest: manifiestoPWA(MARCA, `${BASE_APP}/`), // extras: I4
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
  devToolbar: { enabled: false },
});
