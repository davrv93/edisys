import { ITEMS, GRUPOS } from './permisos.js';
import { paginaActual, ruta } from './nav.jsx';

// Nombres de los subniveles por query (?id=, ?tab=, ?reportar=).
const SUB_POR_TAB = {
  plan: 'Plan de trabajo',
  importar: 'Importar Excel',
  permisos: 'Permisos por rol',
  junta: 'Junta directiva',
  usuarios: 'Usuarios',
};

/** Ítem del menú que corresponde a la página actual (con su query, como el lateral). */
export function itemActual(pathname, search) {
  const pagina = paginaActual(pathname);
  const q = new URLSearchParams(search || '');
  const exactos = Object.entries(ITEMS).filter(([, it]) => {
    if (it.pagina !== pagina) return false;
    if (!it.query) return true;
    return Object.entries(it.query).every(([k, v]) => q.get(k) === String(v));
  });
  const hallado = exactos.find(([, it]) => it.query) || exactos[0];
  return hallado ? { id: hallado[0], ...hallado[1] } : null;
}

/** Grupo al que pertenece un ítem del menú. */
export function grupoDe(idItem) {
  return GRUPOS.find((g) => g.items.includes(idItem)) || null;
}

/** Migas Grupo › Sección › Subnivel para la URL actual. La sección enlaza; el resto es texto. */
export function migasPara(pathname, search) {
  const it = itemActual(pathname, search);
  const q = new URLSearchParams(search || '');
  if (!it) return [{ etiqueta: 'Resumen', href: ruta('inicio') }];
  const migas = [];
  const g = grupoDe(it.id);
  if (g) migas.push({ etiqueta: g.etiqueta });
  migas.push({ etiqueta: it.etiqueta, href: ruta(it.pagina, it.query) });
  let sub = null;
  if (q.get('reportar')) sub = 'Reportar';
  else if (q.get('id')) sub = 'Detalle';
  else if (q.get('tab') && SUB_POR_TAB[q.get('tab')]) sub = SUB_POR_TAB[q.get('tab')];
  else if (q.get('col')) sub = 'Tablero';
  if (sub) migas.push({ etiqueta: sub });
  return migas;
}
