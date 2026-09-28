// Copia packages/tokens (fuente única) a ./tokens antes de construir.
// En `docker build` el contexto es solo esta carpeta: si ../packages no existe,
// se usa la copia ya versionada en ./tokens.
import { copyFileSync, existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const aqui = dirname(fileURLToPath(import.meta.url));
const origen = join(aqui, "..", "..", "packages", "tokens");
const destino = join(aqui, "..", "tokens");
if (existsSync(origen)) {
  for (const f of ["tokens.css", "tailwind-preset.js"]) copyFileSync(join(origen, f), join(destino, f));
  console.log("tokens: copiados desde packages/tokens");
} else {
  console.log("tokens: sin packages/tokens a la vista, uso la copia local");
}
