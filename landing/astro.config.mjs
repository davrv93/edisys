import { defineConfig } from "astro/config";
import sitemap from "@astrojs/sitemap";

// Página 100 % estática: `npm run build` deja todo en dist/ para que Caddy lo sirva en «/».
export default defineConfig({
  site: process.env.EDISYS_SITIO ?? "https://edisys.pe",
  output: "static",
  trailingSlash: "ignore",
  integrations: [sitemap()],
  build: { inlineStylesheets: "auto" },
});
