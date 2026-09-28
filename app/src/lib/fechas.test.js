import { describe, it, expect } from 'vitest';
import {
  nombrePeriodo, sumarMeses, esPeriodo, formatearFecha, formatearFechaHora, periodoActual,
  inicioSemana, rangoSemana, diasDeSemana, etiquetaDia, haceCuanto, diaLima, mesDePeriodo,
} from './fechas.js';

describe('periodos', () => {
  it('nombres en español de Perú (setiembre)', () => {
    expect(nombrePeriodo('2026-09')).toBe('Setiembre 2026');
    expect(nombrePeriodo('2026-09', { corto: true })).toBe('set-2026');
    expect(mesDePeriodo('2026-10')).toBe('octubre');
  });
  it('suma y resta meses cruzando años', () => {
    expect(sumarMeses('2026-09', 1)).toBe('2026-10');
    expect(sumarMeses('2026-12', 1)).toBe('2027-01');
    expect(sumarMeses('2026-01', -1)).toBe('2025-12');
    expect(sumarMeses('2026-09', -12)).toBe('2025-09');
  });
  it('valida el formato AAAA-MM', () => {
    expect(esPeriodo('2026-09')).toBe(true);
    expect(esPeriodo('2026-13')).toBe(false);
    expect(esPeriodo('2026-9')).toBe(false);
  });
});

describe('hora de Lima', () => {
  it('convierte UTC a Lima (UTC−5)', () => {
    // 01:30 UTC del 1 de octubre = 20:30 del 30 de setiembre en Lima
    expect(formatearFecha('2026-10-01T01:30:00Z')).toBe('30-09-2026');
    expect(formatearFechaHora('2026-09-30T13:14:00Z')).toBe('30-09-2026 08:14');
    expect(diaLima('2026-10-01T01:30:00Z')).toBe('2026-09-30');
    expect(periodoActual(new Date('2026-10-01T03:00:00Z'))).toBe('2026-09');
  });
  it('una fecha pura no se corre de día', () => {
    expect(formatearFecha('2026-09-15')).toBe('15-09-2026');
  });
  it('tiempo relativo', () => {
    const ahora = new Date('2026-09-28T17:00:00Z');
    expect(haceCuanto('2026-09-28T16:00:00Z', ahora)).toBe('hace 1 h');
    expect(haceCuanto('2026-09-27T16:00:00Z', ahora)).toBe('ayer');
    expect(haceCuanto('2026-09-28T16:58:00Z', ahora)).toBe('hace 2 min');
  });
});

describe('semanas del calendario', () => {
  it('empieza en lunes y rotula como el lienzo', () => {
    expect(inicioSemana('2026-10-03')).toBe('2026-09-28');
    expect(inicioSemana('2026-10-04')).toBe('2026-09-28');
    expect(inicioSemana('2026-09-28')).toBe('2026-09-28');
    expect(rangoSemana('2026-09-28')).toBe('28 set – 4 oct 2026');
    expect(diasDeSemana('2026-09-28')).toHaveLength(7);
    expect(etiquetaDia('2026-10-03')).toBe('Sáb 3 oct');
  });
});
