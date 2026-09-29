import { describe, it, expect } from 'vitest';
import { normalizarYo, rolPrincipal, tienePermiso, iniciales } from './sesion.js';

describe('normalizarYo', () => {
  it('forma de la guía: roles_por_edificio como arreglo', () => {
    const s = normalizarYo({
      usuario: { id: 1, nombre: 'Administración Demo', correo: 'admin@demo.pe' },
      roles_por_edificio: [{ edificio_id: 1, edificio_nombre: 'Edificio Demo', rol: 'administrador', unidades_total: 24 }],
      permisos: ['balance.ver', 'recibos.emitir'],
      edificio_actual: { id: 1, nombre: 'Edificio Demo' },
    });
    expect(s.rol).toBe('administrador');
    expect(s.edificio).toMatchObject({ id: 1, nombre: 'Edificio Demo', unidades: 24 });
    expect(tienePermiso(s, 'recibos.emitir')).toBe(true);
    expect(tienePermiso(s, 'roles.administrar')).toBe(false);
    expect(s.usuario.iniciales).toBe('AD');
  });
  it('roles_por_edificio como objeto y permisos por edificio', () => {
    const s = normalizarYo({ usuario: { nombre: 'María Demo' }, roles_por_edificio: { 1: ['propietario'], 2: 'junta' }, permisos: { 1: ['portal.ver'], 2: ['balance.ver'] } }, '2');
    expect(s.edificios).toHaveLength(2);
    expect(s.rol).toBe('junta');
    expect(tienePermiso(s, 'balance.ver')).toBe(true);
    expect(tienePermiso(s, 'portal.ver')).toBe(false);
  });
  it('sin permisos en /yo usa los del rol', () => {
    const s = normalizarYo({ usuario: { nombre: 'Conserje', rol: 'operario' }, edificio_actual: { id: 1, nombre: 'Edificio Demo' } });
    expect(s.rol).toBe('operario');
    expect(tienePermiso(s, 'lecturas.registrar')).toBe(true);
  });
  it('elige el rol de más rango', () => {
    expect(rolPrincipal(['propietario', 'junta'])).toBe('junta');
    expect(rolPrincipal([])).toBe('propietario');
  });
  it('iniciales', () => {
    expect(iniciales('María Demo')).toBe('MD');
    expect(iniciales('')).toBe('U');
  });
});

describe('forma real del API (api/internal/app/sesion.go)', () => {
  it('roles_por_edificio con «edificio» como nombre', () => {
    const s = normalizarYo({
      usuario: { id: 1, nombre: 'Administración Demo', roles_por_edificio: [{ edificio_id: 1, edificio: 'Edificio Demo', rol: 'administrador' }] },
      roles_por_edificio: [{ edificio_id: 1, edificio: 'Edificio Demo', rol: 'administrador' }],
      edificio_actual: { id: 1, nombre: 'Edificio Demo', rol: 'administrador' },
      permisos: ['dashboard.ver'],
      unidades: [],
    });
    expect(s.edificio.nombre).toBe('Edificio Demo');
    expect(s.edificios).toHaveLength(1);
    expect(s.rol).toBe('administrador');
  });
});

import { nombreUnidad } from './unidad.js';
describe('nombreUnidad', () => {
  it('antepone Dpto al código pelado', () => {
    expect(nombreUnidad('201')).toBe('Dpto 201');
    expect(nombreUnidad('Dpto 201')).toBe('Dpto 201');
    expect(nombreUnidad(null)).toBe('');
  });
});
