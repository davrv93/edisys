// Tablero de mantenimiento: estados, transiciones y filtros (en la URL).
// Máquina de estados: copia de api/internal/mantenimiento/estados.go (la que manda).
// reportado → validado → presupuestado → aprobado → en_ejecucion → terminado;
// descartado (desde reportado o validado) y rechazado (la junta, desde presupuestado; se puede re-presupuestar).
// Si el API trae `transiciones` en cada incidencia, se usan esas.

export const COLUMNAS = [
  { estado: 'reportado', etiqueta: 'Reportado' },
  { estado: 'validado', etiqueta: 'Validado' },
  { estado: 'presupuestado', etiqueta: 'Informe y costos' },
  { estado: 'aprobado', etiqueta: 'Aprobado' },
  { estado: 'en_ejecucion', etiqueta: 'En ejecución' },
  { estado: 'terminado', etiqueta: 'Terminado' },
  { estado: 'rechazado', etiqueta: 'Rechazado' },
  { estado: 'descartado', etiqueta: 'Descartado' },
];

export const ESTADOS = COLUMNAS.map((c) => c.estado);

export const TRANSICIONES = {
  reportado: ['validado', 'descartado'],
  validado: ['presupuestado', 'descartado'],
  presupuestado: ['aprobado', 'rechazado'],
  rechazado: ['presupuestado'],
  aprobado: ['en_ejecucion'],
  en_ejecucion: ['terminado'],
  terminado: [],
  descartado: [],
};

/** Estados finales o de salida: no son el «siguiente paso feliz». */
const SALIDAS = ['rechazado', 'descartado'];

/** Verbo del botón que lleva a cada estado. */
export const ACCION_HACIA = {
  validado: 'Validar',
  presupuestado: 'Pasar a informe y costos',
  aprobado: 'Aprobar',
  en_ejecucion: 'Iniciar ejecución',
  terminado: 'Marcar terminado',
  rechazado: 'Rechazar',
  descartado: 'Descartar',
};

/** Qué permiso hace falta para llevar una incidencia a cada estado. */
export const PERMISO_HACIA = {
  validado: 'incidencias.validar',
  descartado: 'incidencias.validar',
  presupuestado: 'trabajos.presupuestar',
  aprobado: 'trabajos.aprobar',
  rechazado: 'trabajos.aprobar',
  en_ejecucion: 'trabajos.ejecutar',
  terminado: 'trabajos.ejecutar',
};

export const esSalida = (estado) => SALIDAS.includes(estado);

/** Transiciones desde un estado: las que trae el API para esa incidencia, o la tabla local. */
function desde(de, propias) {
  return Array.isArray(propias) ? propias : TRANSICIONES[de] || [];
}

export function puedeTransicionar(de, a, propias) {
  return desde(de, propias).includes(a);
}

/** Siguiente paso «feliz» (sin rechazo ni descarte), o null si no hay. */
export function siguienteEstado(de) {
  return (TRANSICIONES[de] || []).find((e) => !esSalida(e)) || null;
}

/** Transiciones que el usuario puede hacer, según sus permisos. */
export function transicionesPermitidas(de, tienePermiso = () => true, propias) {
  return desde(de, propias).filter((a) => tienePermiso(PERMISO_HACIA[a]));
}

export const CRITICIDADES = [
  { valor: 'critica', etiqueta: 'Crítica' },
  { valor: 'media', etiqueta: 'Media' },
  { valor: 'baja', etiqueta: 'Baja' },
];

// Mismas categorías que el CHECK de la tabla incidencia (api/migrations/0006_mantenimiento.sql).
export const CATEGORIAS = [
  { valor: 'gasfiteria', etiqueta: 'Agua o filtración' },
  { valor: 'electricidad', etiqueta: 'Electricidad' },
  { valor: 'ascensores', etiqueta: 'Ascensor' },
  { valor: 'bombas', etiqueta: 'Bombas' },
  { valor: 'estructura', etiqueta: 'Humedad o estructura' },
  { valor: 'limpieza', etiqueta: 'Limpieza' },
  { valor: 'seguridad', etiqueta: 'Seguridad' },
  { valor: 'areas_comunes', etiqueta: 'Áreas comunes' },
  { valor: 'jardineria', etiqueta: 'Jardinería' },
  { valor: 'otros', etiqueta: 'Otro' },
];

export const CLAVES_FILTRO = ['criticidad', 'categoria', 'responsable_id', 'q', 'desde', 'hasta'];

const FECHA = /^\d{4}-\d{2}-\d{2}$/;

/** URLSearchParams → objeto de filtros limpio (descarta valores inválidos). */
export function filtrosDesdeURL(params) {
  const sp = params instanceof URLSearchParams ? params : new URLSearchParams(params);
  const f = {};
  for (const k of CLAVES_FILTRO) {
    const v = (sp.get(k) || '').trim();
    if (!v) continue;
    if (k === 'criticidad' && !CRITICIDADES.some((c) => c.valor === v)) continue;
    if ((k === 'desde' || k === 'hasta') && !FECHA.test(v)) continue;
    f[k] = v;
  }
  if (f.desde && f.hasta && f.desde > f.hasta) {
    [f.desde, f.hasta] = [f.hasta, f.desde];
  }
  return f;
}

/** Objeto de filtros → URLSearchParams sin claves vacías (orden estable). */
export function filtrosAURL(filtros, base) {
  const sp = new URLSearchParams(base || undefined);
  for (const k of CLAVES_FILTRO) sp.delete(k);
  for (const k of CLAVES_FILTRO) {
    const v = filtros[k];
    if (v !== undefined && v !== null && String(v).trim() !== '') sp.set(k, String(v).trim());
  }
  return sp;
}

export function hayFiltros(filtros) {
  return CLAVES_FILTRO.some((k) => filtros[k]);
}

const sinTildes = (s) => String(s || '').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();

/**
 * Filtra en el cliente (lo usa el modo mock y sirve de red si el API ignora un filtro).
 * Las fechas comparan el día en que se reportó («AAAA-MM-DD»), con los dos extremos incluidos.
 */
export function filtrarIncidencias(lista, filtros) {
  const q = sinTildes(filtros.q).trim();
  return lista.filter((i) => {
    if (filtros.criticidad && i.criticidad !== filtros.criticidad) return false;
    if (filtros.categoria && i.categoria !== filtros.categoria) return false;
    if (filtros.responsable_id && String(i.responsable_id ?? '') !== String(filtros.responsable_id)) return false;
    const dia = (i.reportado_en || '').slice(0, 10);
    if (filtros.desde && (!dia || dia < filtros.desde)) return false;
    if (filtros.hasta && (!dia || dia > filtros.hasta)) return false;
    if (q) {
      const texto = sinTildes([i.codigo, i.titulo, i.descripcion, i.ubicacion, i.unidad, i.reportado_por, i.responsable_nombre].join(' '));
      if (!q.split(/\s+/).every((p) => texto.includes(p))) return false;
    }
    return true;
  });
}

export function agruparPorEstado(lista) {
  const g = Object.fromEntries(ESTADOS.map((e) => [e, []]));
  for (const i of lista) (g[i.estado] || (g[i.estado] = [])).push(i);
  return g;
}

/** Acepta las variantes razonables del API y deja una forma única para la pantalla. */
export function normalizarIncidencia(r) {
  const resp = r.responsable && typeof r.responsable === 'object' ? r.responsable : null;
  return {
    id: r.id,
    codigo: r.codigo || (r.id != null ? `INC-${String(r.id).padStart(3, '0')}` : ''),
    titulo: r.titulo || r.descripcion || '',
    descripcion: r.descripcion || '',
    estado: r.estado || 'reportado',
    criticidad: r.criticidad || null,
    categoria: r.categoria || r.tipo || null,
    ubicacion: r.ubicacion || '',
    unidad: r.unidad ? (/^\d+$/.test(String(r.unidad)) ? `Dpto ${r.unidad}` : r.unidad) : r.unidad_codigo || '',
    reportado_por: r.reportado_por_nombre || (typeof r.reportado_por === 'string' ? r.reportado_por : r.reportado_por?.nombre) || '',
    reportado_en: r.reportado_en || r.creado_en || r.created_at || '',
    responsable_id: resp ? resp.id : r.responsable_id ?? null,
    responsable_nombre: resp ? resp.nombre : r.responsable_nombre || '',
    monto_cts: r.monto_cts ?? r.monto_presupuesto_cts ?? r.presupuesto_cts ?? null,
    costo_real_cts: r.costo_real_cts || null,
    proveedor: r.proveedor || '',
    foto_url: r.foto_url || null,
    n_fotos: r.n_fotos ?? (typeof r.evidencias === 'number' ? r.evidencias : Array.isArray(r.fotos) ? r.fotos.length : 0),
    votos: r.votos || (r.requiere_junta ? { a_favor: r.votos_a_favor ?? 0, necesarios: r.votos_necesarios ?? null } : null),
    transiciones: Array.isArray(r.transiciones) ? r.transiciones : null,
    motivo: r.motivo || '',
    avance_pct: r.avance_pct ?? null,
  };
}
