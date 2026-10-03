import { describe, it, expect } from 'vitest';
import { menuPara, gruposPara, GRUPOS, ITEMS } from './permisos.js';

// Bloques F1–F3: dónde aparecen Colaboradores, Asistencia y Almacén según el rol (permisos de 0026_personal.sql).
const PERMISOS = {
  administrador: ['personal.ver', 'personal.administrar', 'asistencia.ver', 'almacen.ver', 'almacen.registrar', 'almacen.administrar'],
  junta: ['personal.ver', 'asistencia.ver', 'almacen.ver'],
  propietario: ['personal.ver'],
  operario: ['asistencia.marcar', 'almacen.ver', 'almacen.registrar', 'lecturas.registrar', 'incidencias.reportar'],
  tecnico: ['asistencia.marcar', 'trabajos.ejecutar'],
};
const con = (rol) => (p) => PERMISOS[rol].includes(p);
const ids = (rol) => menuPara(rol, con(rol)).lateral.map((i) => i.id);

describe('menú del personal (F1–F3)', () => {
  it('administración y junta ven las tres pantallas', () => {
    for (const rol of ['administrador', 'junta', 'superadmin']) {
      const p = rol === 'superadmin' ? 'administrador' : rol;
      expect(menuPara(rol, con(p)).lateral.map((i) => i.id)).toEqual(expect.arrayContaining(['personal', 'asistencia', 'almacen']));
    }
  });
  it('el propietario ve colaboradores, no asistencia ni almacén', () => {
    expect(ids('propietario')).toContain('personal');
    expect(ids('propietario')).not.toContain('asistencia');
    expect(ids('propietario')).not.toContain('almacen');
  });
  it('el operario marca asistencia y saca insumos; su barra móvil no cambia', () => {
    expect(ids('operario')).toEqual(expect.arrayContaining(['asistencia', 'almacen']));
    expect(ids('operario')).not.toContain('personal');
    expect(menuPara('operario', con('operario')).movil.map((i) => i.id)).toEqual(['medidores', 'reportar']);
  });
  it('el técnico solo marca asistencia', () => {
    expect(ids('tecnico')).toContain('asistencia');
    expect(ids('tecnico')).not.toContain('almacen');
  });
  it('los tres ítems existen y viven en el grupo Personal', () => {
    const g = GRUPOS.find((x) => x.id === 'equipo');
    expect(g.items).toEqual(['personal', 'asistencia', 'almacen']);
    for (const id of g.items) expect(ITEMS[id]).toBeTruthy();
    expect(gruposPara(menuPara('administrador', con('administrador')).lateral).map((x) => x.id)).toContain('equipo');
  });
});
