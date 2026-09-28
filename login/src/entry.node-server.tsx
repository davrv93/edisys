/**
 * Servidor Node de producción (sin Express): SSR de Qwik City en el puerto 3000.
 */
import { createQwikCity } from "@builder.io/qwik-city/middleware/node";
import qwikCityPlan from "@qwik-city-plan";
import { manifest } from "@qwik-client-manifest";
import { createReadStream, statSync } from "node:fs";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";
import render from "./entry.ssr";

// Estáticos propios: el staticFile de Qwik duplica la base («/login/login/…»)
// cuando `base` no es «/», así que servimos dist/ nosotros mismos.
const DIST = join(fileURLToPath(import.meta.url), "..", "..", "dist");
const TIPOS: Record<string, string> = {
  js: "text/javascript; charset=utf-8",
  css: "text/css; charset=utf-8",
  json: "application/json",
  svg: "image/svg+xml",
  woff: "font/woff",
  woff2: "font/woff2",
  txt: "text/plain; charset=utf-8",
  png: "image/png",
  ico: "image/x-icon",
  webmanifest: "application/manifest+json",
};

function servirEstatico(req: IncomingMessage, res: ServerResponse): boolean {
  if (req.method !== "GET" && req.method !== "HEAD") return false;
  const ruta = decodeURIComponent((req.url ?? "/").split("?")[0]);
  if (!ruta.startsWith("/login/")) return false;
  const ext = extname(ruta).slice(1);
  if (!ext || !TIPOS[ext]) return false;
  const archivo = normalize(join(DIST, ruta));
  if (!archivo.startsWith(DIST)) return false;
  let tam: number;
  try {
    const st = statSync(archivo);
    if (!st.isFile()) return false;
    tam = st.size;
  } catch {
    return false;
  }
  const inmutable = ruta.startsWith("/login/build/") || ruta.startsWith("/login/assets/");
  res.writeHead(200, {
    "Content-Type": TIPOS[ext],
    "Content-Length": tam,
    "Cache-Control": inmutable ? "public, max-age=31536000, immutable" : "public, max-age=3600",
    "X-Content-Type-Options": "nosniff",
  });
  if (req.method === "HEAD") res.end();
  else createReadStream(archivo).pipe(res);
  return true;
}

const PORT = Number(process.env.PORT ?? 3000);
const HOST = process.env.HOST ?? "0.0.0.0";

const { router, notFound } = createQwikCity({
  render,
  qwikCityPlan,
  manifest,
  // Detrás de Caddy: el origen real llega en X-Forwarded-*.
  getOrigin(req) {
    const proto = (req.headers["x-forwarded-proto"] as string) ?? "http";
    const host = (req.headers["x-forwarded-host"] as string) ?? req.headers.host;
    return `${proto.split(",")[0]}://${host}`;
  },
});

const server = createServer((req, res) => {
  // Salud para el healthcheck del compose.
  if (req.url === "/login/salud") {
    res.writeHead(200, { "content-type": "application/json" });
    res.end('{"ok":true}');
    return;
  }
  if (servirEstatico(req, res)) return;
  router(req, res, () => {
    notFound(req, res, () => {});
  });
});

server.listen(PORT, HOST, () => {
  console.log(`edisys-login escuchando en http://${HOST}:${PORT}/login/`);
});

const apagar = () => server.close(() => process.exit(0));
process.on("SIGTERM", apagar);
process.on("SIGINT", apagar);
