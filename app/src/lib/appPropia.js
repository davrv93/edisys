// App propia (bloque I4): parámetros de marca para construir la PWA con el nombre, el icono y el
// identificador de una administradora. Lo usan astro.config.mjs (manifiesto) y scripts/app-propia.mjs
// (build + manifiesto TWA para empaquetar con Bubblewrap). Sin conexión en runtime: todo es build.
// Sin variables de entorno, sale EDISYS tal cual (el build normal no cambia).

export const MARCA_POR_DEFECTO = {
  nombre: 'EDISYS',
  nombreCorto: 'EDISYS',
  descripcion: 'Administración de edificios: cuotas, recibos, balance, reservas y mantenimiento.',
  colorTema: '#0F172A',
  colorFondo: '#F8FAFC',
  packageId: 'pe.edisys.app',
  idSufijo: '',
};

const RE_COLOR = /^#[0-9a-fA-F]{6}$/;
// Identificador de paquete Android / bundle id de iOS: dominio invertido, al menos dos segmentos.
const RE_PAQUETE = /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/;
const RE_SUFIJO = /^[a-z0-9-]{0,40}$/;

/**
 * Lee la marca de las variables EDISYS_APP_* y la valida. Lanza un Error con todos los problemas juntos.
 *   EDISYS_APP_NOMBRE, EDISYS_APP_NOMBRE_CORTO, EDISYS_APP_DESCRIPCION,
 *   EDISYS_APP_COLOR (tema), EDISYS_APP_FONDO, EDISYS_APP_PAQUETE, EDISYS_APP_ID (sufijo del id del manifiesto)
 */
export function leerMarcaApp(env = {}) {
  const v = (k, def) => (env[k] != null && String(env[k]).trim() !== '' ? String(env[k]).trim() : def);
  const d = MARCA_POR_DEFECTO;
  const nombre = v('EDISYS_APP_NOMBRE', d.nombre);
  const marca = {
    nombre,
    nombreCorto: v('EDISYS_APP_NOMBRE_CORTO', nombre === d.nombre ? d.nombreCorto : nombre),
    descripcion: v('EDISYS_APP_DESCRIPCION', d.descripcion),
    colorTema: v('EDISYS_APP_COLOR', d.colorTema),
    colorFondo: v('EDISYS_APP_FONDO', d.colorFondo),
    packageId: v('EDISYS_APP_PAQUETE', d.packageId),
    idSufijo: v('EDISYS_APP_ID', d.idSufijo),
  };
  const errores = [];
  if (marca.nombre.length > 45) errores.push('El nombre tiene hasta 45 caracteres.');
  // Android corta el nombre bajo el icono a ~12 caracteres; más largo se ve «Mi Adminis…».
  if (marca.nombreCorto.length > 12) errores.push('El nombre corto tiene hasta 12 caracteres (es el que sale bajo el icono).');
  if (!RE_COLOR.test(marca.colorTema)) errores.push('El color de tema va como #RRGGBB.');
  if (!RE_COLOR.test(marca.colorFondo)) errores.push('El color de fondo va como #RRGGBB.');
  if (!RE_PAQUETE.test(marca.packageId)) errores.push('El paquete va como dominio invertido en minúsculas, p. ej. pe.miadministradora.app.');
  if (!RE_SUFIJO.test(marca.idSufijo)) errores.push('El id usa minúsculas, números y guiones.');
  if (errores.length) throw new Error(errores.join(' '));
  return marca;
}

/** Manifiesto web de la PWA. `base` es la base de la app con barra final (p. ej. «/app/»). */
export function manifiestoPWA(marca, base) {
  const b = base.endsWith('/') ? base : `${base}/`;
  // El id distingue instalaciones: dos marcas en el mismo origen no se pisan entre sí.
  const id = marca.idSufijo ? `${b}?app=${marca.idSufijo}` : b;
  return {
    name: marca.nombre,
    short_name: marca.nombreCorto,
    description: marca.descripcion,
    lang: 'es-PE',
    id,
    start_url: b,
    scope: b,
    display: 'standalone',
    background_color: marca.colorFondo,
    theme_color: marca.colorTema,
    icons: [
      { src: `${b}icon-192.png`, sizes: '192x192', type: 'image/png' },
      { src: `${b}icon-512.png`, sizes: '512x512', type: 'image/png' },
      { src: `${b}icon-512.png`, sizes: '512x512', type: 'image/png', purpose: 'maskable' },
    ],
  };
}

/**
 * Manifiesto de Bubblewrap (Trusted Web Activity) para empaquetar la PWA como app de Android.
 * EDISYS no publica: esto es la entrada de `bubblewrap build` que corre quien tenga la cuenta de Play.
 */
export function manifiestoTWA(marca, host, base) {
  const b = base.endsWith('/') ? base : `${base}/`;
  if (!host) throw new Error('Falta el dominio donde se sirve la app (--host).');
  return {
    packageId: marca.packageId,
    host,
    name: marca.nombre,
    launcherName: marca.nombreCorto,
    display: 'standalone',
    themeColor: marca.colorTema,
    navigationColor: marca.colorTema,
    backgroundColor: marca.colorFondo,
    startUrl: b,
    iconUrl: `https://${host}${b}icon-512.png`,
    maskableIconUrl: `https://${host}${b}icon-512.png`,
    webManifestUrl: `https://${host}${b}manifest.webmanifest`,
    enableNotifications: false,
    appVersionCode: 1,
    appVersionName: '1.0.0',
    signingKey: { path: './llave-de-firma.keystore', alias: 'edisys' },
  };
}
