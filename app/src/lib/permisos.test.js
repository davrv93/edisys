import { describe, it, expect } from 'vitest';
import { menuPara, destinoPorRol, PERMISOS_POR_ROL } from './permisos.js';

const con = (rol) => (p) => PERMISOS_POR_ROL[rol].includes(p);

describe('menú por rol (§2.5)', () => {
  it('administrador: Inicio · Balance · Recibos · Mantenimiento + Más', () => {
    const m = menuPara('administrador', con('administrador'));
    expect(m.movil.map((i) => i.corta)).toEqual(['Inicio', 'Balance', 'Recibos', 'Mantenim.']);
    expect(m.mas.map((i) => i.id)).toContain('whatsapp');
    expect(m.lateral.map((i) => i.id)).toContain('analitica');
  });
  it('propietario: Portal · Recibos · Reservas · Reportar', () => {
    const m = menuPara('propietario', con('propietario'));
    expect(m.movil.map((i) => i.id)).toEqual(['portal', 'recibos', 'reservar', 'reportar']);
    expect(m.lateral.map((i) => i.id)).not.toContain('roles');
  });
  it('operario: Lecturas · Reportar', () => {
    expect(menuPara('operario', con('operario')).movil.map((i) => i.id)).toEqual(['medidores', 'reportar']);
  });
  it('quita lo que no permite el rol', () => {
    const m = menuPara('administrador', (p) => p !== 'roles.administrar');
    expect(m.lateral.map((i) => i.id)).not.toContain('roles');
  });
  it('destino por rol', () => {
    expect(destinoPorRol('administrador')).toBe('inicio');
    expect(destinoPorRol('junta')).toBe('inicio');
    expect(destinoPorRol('propietario')).toBe('portal');
    expect(destinoPorRol('operario')).toBe('medidores');
    expect(destinoPorRol('tecnico')).toBe('mantenimiento');
  });
});
