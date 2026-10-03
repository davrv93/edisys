#!/usr/bin/env node
// EDISYS · app propia (bloque I4). Construye la PWA con la marca de una administradora
// (nombre, iconos, colores, id) y deja listo el manifiesto de Bubblewrap para empaquetarla
// como app de Android. NO publica en tiendas, NO firma y NO llama a ningún servicio:
// la publicación la hace quien tenga la cuenta de Google Play / Apple (ver docs/APP_PROPIA_Y_DOMINIO.md).
//
// Uso (desde la raíz de EDISYS):
//   node scripts/app-propia.mjs --nombre "Torres Administración" --corto "Torres" \
//     --paquete pe.torres.app --id torres --icono-512 ./marca/icono-512.png [--icono-192 ./marca/icono-192.png] \
//     [--color '#0F172A'] [--fondo '#F8FAFC'] [--host intranet.torres.pe] [--prefijo /edisys] [--salida build-apps/torres]
//
// Resultado en <salida>/:
//   web/                la app estática (servirla igual que app/dist, p. ej. en el borde bajo /app)
//   marca.json          los parámetros usados (para repetir el build)
//   twa-manifest.json   entrada de `bubblewrap init --manifest` (solo si se pasó --host)
//
// --solo-validar revisa parámetros e iconos sin construir.
import { spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { leerMarcaApp, manifiestoPWA, manifiestoTWA } from '../app/src/lib/appPropia.js';

const RAIZ = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const APP = join(RAIZ, 'app');

function args(argv) {
  const out = {};
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (!a.startsWith('--')) continue;
    const k = a.slice(2);
    const sig = argv[i + 1];
    if (sig === undefined || sig.startsWith('--')) out[k] = true;
    else {
      out[k] = sig;
      i++;
    }
  }
  return out;
}

function morir(msg) {
  console.error(`✗ ${msg}`);
  process.exit(1);
}

/** Ancho y alto de un PNG leyendo su cabecera IHDR (sin dependencias). */
function medidasPNG(ruta) {
  const b = readFileSync(ruta);
  const firma = '89504e470d0a1a0a';
  if (b.length < 24 || b.subarray(0, 8).toString('hex') !== firma) return null;
  return { ancho: b.readUInt32BE(16), alto: b.readUInt32BE(20) };
}

/** Redimensiona con la herramienta que haya (sips en macOS, ImageMagick en Linux). */
function redimensionar(origen, destino, lado) {
  const sips = spawnSync('sips', ['-z', String(lado), String(lado), origen, '--out', destino], { stdio: 'ignore' });
  if (sips.status === 0) return true;
  for (const bin of ['magick', 'convert']) {
    const r = spawnSync(bin, [origen, '-resize', `${lado}x${lado}`, destino], { stdio: 'ignore' });
    if (r.status === 0) return true;
  }
  return false;
}

const a = args(process.argv.slice(2));
const env = {
  EDISYS_APP_NOMBRE: a.nombre,
  EDISYS_APP_NOMBRE_CORTO: a.corto,
  EDISYS_APP_DESCRIPCION: a.descripcion,
  EDISYS_APP_COLOR: a.color,
  EDISYS_APP_FONDO: a.fondo,
  EDISYS_APP_PAQUETE: a.paquete,
  EDISYS_APP_ID: a.id,
};
let marca;
try {
  marca = leerMarcaApp(env);
} catch (e) {
  morir(e.message);
}
if (!a.paquete) morir('Falta --paquete (p. ej. pe.torres.app): es el identificador de la app en las tiendas.');
if (!a['icono-512']) morir('Falta --icono-512 (PNG cuadrado de 512×512).');

const icono512 = resolve(a['icono-512']);
if (!existsSync(icono512)) morir(`No existe ${icono512}.`);
const m512 = medidasPNG(icono512);
if (!m512) morir('El icono debe ser PNG.');
if (m512.ancho !== 512 || m512.alto !== 512) morir(`El icono de 512 mide ${m512.ancho}×${m512.alto}; debe ser 512×512.`);
let icono192 = a['icono-192'] ? resolve(a['icono-192']) : null;
if (icono192) {
  const m = existsSync(icono192) && medidasPNG(icono192);
  if (!m || m.ancho !== 192 || m.alto !== 192) morir('El icono de 192 debe ser un PNG de 192×192.');
}

const prefijo = typeof a.prefijo === 'string' ? a.prefijo.replace(/\/+$/, '') : '';
const base = `${prefijo}/app/`;
const host = typeof a.host === 'string' ? a.host.trim().toLowerCase() : '';
const salida = resolve(typeof a.salida === 'string' ? a.salida : join(RAIZ, 'build-apps', marca.idSufijo || marca.packageId));

if (a['solo-validar']) {
  console.log(`✓ Parámetros válidos para «${marca.nombre}» (${marca.packageId}).`);
  console.log(JSON.stringify(manifiestoPWA(marca, base), null, 2));
  process.exit(0);
}

// Estáticos: copia de app/public con los iconos de la marca encima (app/public no se toca).
const tmp = mkdtempSync(join(tmpdir(), 'edisys-app-propia-'));
const publico = join(tmp, 'public');
cpSync(join(APP, 'public'), publico, { recursive: true });
cpSync(icono512, join(publico, 'icon-512.png'));
if (icono192) cpSync(icono192, join(publico, 'icon-192.png'));
else if (!redimensionar(icono512, join(publico, 'icon-192.png'), 192)) {
  rmSync(tmp, { recursive: true, force: true });
  morir('No pude generar el icono de 192 (no hay sips ni ImageMagick). Pásalo con --icono-192.');
}

mkdirSync(salida, { recursive: true });
const web = join(salida, 'web');
console.log(`→ Construyendo «${marca.nombre}» en ${web}`);
const r = spawnSync('npx', ['astro', 'build'], {
  cwd: APP,
  stdio: 'inherit',
  env: {
    ...process.env,
    ...Object.fromEntries(Object.entries(env).filter(([, v]) => typeof v === 'string')),
    EDISYS_PREFIJO: prefijo,
    EDISYS_PUBLIC_DIR: publico,
    EDISYS_OUT_DIR: web,
  },
});
rmSync(tmp, { recursive: true, force: true });
if (r.status !== 0) morir('Falló astro build.');

writeFileSync(join(salida, 'marca.json'), JSON.stringify({ ...marca, prefijo, host, construido: new Date().toISOString() }, null, 2) + '\n');
if (host) {
  writeFileSync(join(salida, 'twa-manifest.json'), JSON.stringify(manifiestoTWA(marca, host, base), null, 2) + '\n');
  console.log(`✓ twa-manifest.json listo: bubblewrap init --manifest https://${host}${base}manifest.webmanifest`);
}
console.log(`✓ Listo: ${salida}`);
