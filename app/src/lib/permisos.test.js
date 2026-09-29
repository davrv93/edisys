import { describe, it, expect } from 'vitest';
import { menuPara, destinoPorRol, PERMISOS_POR_ROL } from './permisos.js';

const con = (rol) => (p) => PERMISOS_POR_ROL[rol].includes(p);

describe('menú por rol (§2.5)', () => {
  it('administrador: Resumen · Recibos · Mantenimiento · WhatsApp + Más (plan de la segunda pasada)', () => {
    const m = menuPara('administrador', con('administrador'));
    expect(m.movil.map((i) => i.corta)).toEqual(['Resumen', 'Recibos', 'Mantenim.', 'WhatsApp']);
    expect(m.mas.map((i) => i.id)).toContain('balance');
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

import { veCalendarioReservas, veTableroMantenimiento } from './permisos.js';
describe('qué vista ve cada rol con los permisos reales', () => {
  it('reservas: staff ve el calendario; residentes reservan', () => {
    expect(veCalendarioReservas(con('administrador'))).toBe(true);
    expect(veCalendarioReservas(con('junta'))).toBe(true);
    expect(veCalendarioReservas(con('operario'))).toBe(true);
    expect(veCalendarioReservas(con('propietario'))).toBe(false);
    expect(veCalendarioReservas(con('inquilino'))).toBe(false);
  });
  it('mantenimiento: tablero para quien gestiona; reportar para el resto', () => {
    expect(veTableroMantenimiento(con('administrador'))).toBe(true);
    expect(veTableroMantenimiento(con('junta'))).toBe(true);
    expect(veTableroMantenimiento(con('tecnico'))).toBe(true);
    expect(veTableroMantenimiento(con('propietario'))).toBe(false);
    expect(veTableroMantenimiento(con('operario'))).toBe(false);
  });
});
