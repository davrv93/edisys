import { describe, expect, it } from 'vitest';
import { migasPara, grupoDe } from './migas.js';
import { ITEMS, GRUPOS } from './permisos.js';

describe('migas', () => {
  it('sección con su grupo', () => {
    expect(migasPara('/app/recibos', '').map((m) => m.etiqueta)).toEqual(['Finanzas', 'Recibos y cobranza']);
  });
  it('detalle con id', () => {
    expect(migasPara('/app/recibos', '?id=1').map((m) => m.etiqueta)).toEqual(['Finanzas', 'Recibos y cobranza', 'Detalle']);
  });
  it('plan de trabajo y reportar', () => {
    expect(migasPara('/app/mantenimiento', '?tab=plan').map((m) => m.etiqueta)).toEqual(['Operación', 'Mantenimiento', 'Plan de trabajo']);
    expect(migasPara('/app/mantenimiento', '?reportar=1')[2].etiqueta).toBe('Reportar');
  });
  it('la sección enlaza y el resto es texto', () => {
    const m = migasPara('/app/unidades', '?tab=importar');
    expect(m[1].href).toMatch(/unidades/);
    expect(m[2]).toEqual({ etiqueta: 'Importar Excel' });
  });
  it('todo ítem del menú vive en un grupo', () => {
    const enGrupos = new Set(GRUPOS.flatMap((g) => g.items));
    for (const id of Object.keys(ITEMS)) expect(enGrupos.has(id), id).toBe(true);
  });
  it('grupoDe ubica motor con comunicación', () => {
    expect(grupoDe('motor').id).toBe('comunicacion');
  });
});
