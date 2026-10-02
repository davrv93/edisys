/**
 * Configuración base de Vite. El adaptador (adapters/node-server) la extiende al construir.
 * Todo el servicio vive bajo /login/ (Caddy manda /login* a login:3000 sin quitar el prefijo).
 */
import { defineConfig, type UserConfig } from "vite";
import { qwikVite } from "@builder.io/qwik/optimizer";
import { qwikCity } from "@builder.io/qwik-city/vite";
import tsconfigPaths from "vite-tsconfig-paths";

// Prefijo de URL pública: vacío en local; «/edisys» bajo el alias de boticalima.
const PREFIJO = process.env.EDISYS_PREFIJO || "";

export default defineConfig((): UserConfig => {
  return {
    base: `${PREFIJO}/login/`,
    plugins: [qwikCity(), qwikVite(), tsconfigPaths({ root: "." })],
    server: {
      headers: { "Cache-Control": "public, max-age=0" },
      // En desarrollo, el API se alcanza por el mismo origen.
      proxy: {
        "/api": process.env.EDISYS_API_URL ?? "http://localhost:4700",
      },
    },
  };
});
