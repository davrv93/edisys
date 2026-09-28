import { describe, it, expect } from 'vitest';
import {
  puedeTransicionar, siguienteEstado, transicionesPermitidas, filtrosDesdeURL, filtrosAURL,
  filtrarIncidencias, agruparPorEstado, normalizarIncidencia, hayFiltros,
} from './kanban.js';

describe('transiciones', () => {
  it('solo avanza un paso por el camino feliz', () => {
    expect(puedeTransicionar('reportado', 'validado')).toBe(true);
    expect(puedeTransicionar('validado', 'presupuestado')).toBe(true);
    expect(puedeTransicionar('presupuestado', 'aprobado')).toBe(true);
    expect(puedeTransicionar('aprobado', 'en_ejecucion')).toBe(true);
    expect(puedeTransicionar('en_ejecucion', 'terminado')).toBe(true);
  });
  it('prohíbe saltos, retrocesos y salir de estados finales', () => {
    expect(puedeTransicionar('reportado', 'aprobado')).toBe(false);
    expect(puedeTransicionar('validado', 'en_ejecucion')).toBe(false); // sin aprobación
    expect(puedeTransicionar('aprobado', 'validado')).toBe(false);
    expect(puedeTransicionar('terminado', 'reportado')).toBe(false);
    expect(puedeTransicionar('rechazado', 'validado')).toBe(false);
    expect(puedeTransicionar('aprobado', 'rechazado')).toBe(false);
    expect(puedeTransicionar('x', 'validado')).toBe(false);
  });
  it('rechazar se puede antes de aprobar', () => {
    for (const e of ['reportado', 'validado', 'presupuestado']) expect(puedeTransicionar(e, 'rechazado')).toBe(true);
  });
  it('siguiente estado', () => {
    expect(siguienteEstado('reportado')).toBe('validado');
    expect(siguienteEstado('en_ejecucion')).toBe('terminado');
    expect(siguienteEstado('terminado')).toBeNull();
  });
  it('filtra por permisos: la junta solo aprueba o rechaza lo presupuestado', () => {
    const junta = (p) => p === 'trabajos.aprobar';
    expect(transicionesPermitidas('presupuestado', junta)).toEqual(['aprobado']);
    expect(transicionesPermitidas('reportado', junta)).toEqual([]);
  });
});

describe('filtros en la URL', () => {
  it('lee, limpia y ordena', () => {
    const f = filtrosDesdeURL('?criticidad=critica&q=%20bomba%20&desde=2026-09-30&hasta=2026-09-01&x=1');
    expect(f).toEqual({ criticidad: 'critica', q: 'bomba', desde: '2026-09-01', hasta: '2026-09-30' });
  });
  it('descarta valores inválidos', () => {
    expect(filtrosDesdeURL('?criticidad=urgente&desde=30/09/2026')).toEqual({});
  });
  it('escribe sin claves vacías y conserva otras claves', () => {
    const sp = filtrosAURL({ criticidad: 'media', q: '', categoria: 'electricidad' }, 'ver=5');
    expect(sp.toString()).toBe('ver=5&criticidad=media&categoria=electricidad');
    expect(filtrosDesdeURL(sp)).toEqual({ criticidad: 'media', categoria: 'electricidad' });
  });
  it('hayFiltros', () => {
    expect(hayFiltros({})).toBe(false);
    expect(hayFiltros({ q: 'x' })).toBe(true);
  });
});

describe('filtrado del tablero', () => {
  const lista = [
    { codigo: 'INC-014', titulo: 'Bomba de agua N.º 2', estado: 'presupuestado', criticidad: 'critica', categoria: 'gasfiteria', responsable_id: 3, reportado_en: '2026-09-20T12:00:00Z', ubicacion: 'Cuarto de bombas' },
    { codigo: 'INC-018', titulo: 'Luz quemada en pasillo', estado: 'reportado', criticidad: null, categoria: 'electricidad', responsable_id: null, reportado_en: '2026-09-27T12:00:00Z' },
    { codigo: 'INC-012', titulo: 'Pintura de escalera', estado: 'en_ejecucion', criticidad: 'baja', categoria: 'otro', responsable_id: 7, reportado_en: '2026-09-05T12:00:00Z' },
  ];
  it('combina criterios', () => {
    expect(filtrarIncidencias(lista, { criticidad: 'critica' }).map((i) => i.codigo)).toEqual(['INC-014']);
    expect(filtrarIncidencias(lista, { categoria: 'electricidad' }).map((i) => i.codigo)).toEqual(['INC-018']);
    expect(filtrarIncidencias(lista, { responsable_id: '7' }).map((i) => i.codigo)).toEqual(['INC-012']);
  });
  it('texto sin tildes ni mayúsculas, en varios campos', () => {
    expect(filtrarIncidencias(lista, { q: 'BOMBAS' }).map((i) => i.codigo)).toEqual(['INC-014']);
    expect(filtrarIncidencias(lista, { q: 'inc-012' }).map((i) => i.codigo)).toEqual(['INC-012']);
    expect(filtrarIncidencias(lista, { q: 'pasillo luz' }).map((i) => i.codigo)).toEqual(['INC-018']);
  });
  it('rango de fechas con extremos incluidos', () => {
    expect(filtrarIncidencias(lista, { desde: '2026-09-20', hasta: '2026-09-27' }).map((i) => i.codigo)).toEqual(['INC-014', 'INC-018']);
    expect(filtrarIncidencias(lista, { hasta: '2026-09-05' }).map((i) => i.codigo)).toEqual(['INC-012']);
  });
  it('agrupa por estado con todas las columnas', () => {
    const g = agruparPorEstado(lista);
    expect(Object.keys(g)).toContain('rechazado');
    expect(g.presupuestado).toHaveLength(1);
    expect(g.terminado).toHaveLength(0);
  });
  it('normaliza variantes del API', () => {
    const n = normalizarIncidencia({ id: 14, descripcion: 'Bomba', responsable: { id: 3, nombre: 'Bombas Demo SAC' }, presupuesto_cts: 185000, creado_en: '2026-09-20' });
    expect(n.codigo).toBe('INC-014');
    expect(n.titulo).toBe('Bomba');
    expect(n.responsable_id).toBe(3);
    expect(n.monto_cts).toBe(185000);
    expect(n.estado).toBe('reportado');
    expect(n.reportado_en).toBe('2026-09-20');
  });
});
