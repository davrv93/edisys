// Navegación entre las páginas Astro (una por pantalla). No hay router de cliente:
// cambiar de pantalla es cargar otra página estática; los detalles van en la query (?id=…).
import { useCallback, useEffect, useState } from 'react';
import { BASE_APP } from './base.js';

export { BASE_APP };

/** Página → ruta pública (cada una se sirve como /app/<ruta>/index.html). */
export const PAGINAS = {
  inicio: '/',
  balance: '/balance/',
  recibos: '/recibos/',
  unidades: '/unidades/',
  reservas: '/reservas/',
  medidores: '/medidores/',
  mantenimiento: '/mantenimiento/',
  portal: '/portal/',
  roles: '/roles/',
  whatsapp: '/whatsapp/',
  chatbot: '/chatbot/',
  motor: '/motor/',
  analitica: '/analitica/',
  conciliacion: '/conciliacion/',
  proveedores: '/proveedores/',
  fondos: '/fondos/',
  informes: '/informes/',
  externos: '/externos/',
  vouchers: '/vouchers/',
  cobranzas: '/cobranzas/',
  documentos: '/documentos/',
  // recaudacion: A1, A2
  recaudadora: '/recaudadora/',
  'cobranza-masiva': '/cobranza-masiva/',
  // deuda: B3, D1, D2 y D3
  'cuentas-cobrar': '/cuentas-cobrar/',
  acuerdos: '/acuerdos/',
  morosos: '/morosos/',
  configuracion: '/configuracion/',
  // reservas: H1 / H3
  checkin: '/checkin/',
  areascomunes: '/areas-comunes/',
  marca: '/marca/', // marca: I1/I2
  ajustes: '/ajustes/', // marca: I5
};
// extras: J1/J2/I3
PAGINAS.encuestas = '/encuestas/';
PAGINAS.videollamadas = '/videollamadas/';

function qs(query) {
  if (!query) return '';
  const sp = query instanceof URLSearchParams ? query : new URLSearchParams();
  if (!(query instanceof URLSearchParams)) {
    for (const [k, v] of Object.entries(query)) if (v !== undefined && v !== null && v !== '') sp.set(k, String(v));
  }
  const s = sp.toString();
  return s ? `?${s}` : '';
}

/** ruta('recibos', { id: 9005 }) → «/app/recibos/?id=9005». */
export function ruta(pagina, query) {
  return BASE_APP + (PAGINAS[pagina] ?? `/${pagina}/`) + qs(query);
}

/** Qué página es la actual, a partir de la URL. */
export function paginaActual(pathname = typeof window !== 'undefined' ? window.location.pathname : BASE_APP + '/') {
  const sinBase = pathname.startsWith(BASE_APP) ? pathname.slice(BASE_APP.length) : pathname;
  const resto = sinBase.replace(/\/+$/, '/') || '/';
  const hallada = Object.entries(PAGINAS).find(([, r]) => r === resto || r === resto + '/');
  return hallada ? hallada[0] : 'inicio';
}

export function navegar(url, { reemplazar = false } = {}) {
  if (reemplazar) window.location.replace(url);
  else window.location.assign(url);
}

const EVENTO = 'edisys:query';

/**
 * Parámetros de la query de la página actual, con setter que usa history (sin recargar).
 * setQuery({ id: 5 }) fusiona; setQuery(sp => …) recibe una copia; { reemplazar: true } no deja historial.
 */
export function useQuery() {
  const leer = () => new URLSearchParams(typeof window !== 'undefined' ? window.location.search : '');
  const [sp, setSp] = useState(leer);
  useEffect(() => {
    const sync = () => setSp(leer());
    window.addEventListener('popstate', sync);
    window.addEventListener(EVENTO, sync);
    return () => {
      window.removeEventListener('popstate', sync);
      window.removeEventListener(EVENTO, sync);
    };
  }, []);
  const setQuery = useCallback((cambio, { reemplazar = false } = {}) => {
    const actual = new URLSearchParams(window.location.search);
    let nueva;
    if (typeof cambio === 'function') nueva = cambio(new URLSearchParams(actual));
    else if (cambio instanceof URLSearchParams) nueva = cambio;
    else {
      nueva = new URLSearchParams(actual);
      for (const [k, v] of Object.entries(cambio)) {
        if (v === undefined || v === null || v === '') nueva.delete(k);
        else nueva.set(k, String(v));
      }
    }
    const s = nueva.toString();
    const url = window.location.pathname + (s ? `?${s}` : '');
    if (reemplazar) window.history.replaceState(null, '', url);
    else window.history.pushState(null, '', url);
    window.dispatchEvent(new Event(EVENTO));
  }, []);
  return [sp, setQuery];
}

/** Enlace a otra pantalla (recarga la página Astro correspondiente). */
export function Enlace({ pagina, query, href, children, ...resto }) {
  return (
    <a href={href || ruta(pagina, query)} {...resto}>
      {children}
    </a>
  );
}
