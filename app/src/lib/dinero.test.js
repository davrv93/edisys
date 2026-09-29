import { describe, it, expect } from 'vitest';
import { formatearSoles, parsearSoles, formatearPct, formatearNumero, ctsATexto, sumarCts, pctDe } from './dinero.js';

describe('formatearSoles', () => {
  it('usa punto de miles y coma decimal, como el lienzo', () => {
    expect(formatearSoles(1946000)).toBe('S/ 19.460,00');
    expect(formatearSoles(99000)).toBe('S/ 990,00');
    expect(formatearSoles(70560)).toBe('S/ 705,60');
    expect(formatearSoles(840)).toBe('S/ 8,40');
    expect(formatearSoles(0)).toBe('S/ 0,00');
    expect(formatearSoles(5)).toBe('S/ 0,05');
    expect(formatearSoles(123456789)).toBe('S/ 1.234.567,89');
  });
  it('negativos, sin decimales y con signo', () => {
    expect(formatearSoles(-51000)).toBe('-S/ 510,00');
    expect(formatearSoles(1946000, { sinDecimales: true })).toBe('S/ 19.460');
    expect(formatearSoles(51000, { conSigno: true })).toBe('+S/ 510,00');
  });
  it('vacío → raya', () => {
    expect(formatearSoles(null)).toBe('—');
    expect(formatearSoles(undefined)).toBe('—');
  });
  it('el recibo del 201 suma S/ 990,00 al céntimo', () => {
    const lineas = [70560, 19600, 840, 8000];
    expect(sumarCts(lineas)).toBe(99000);
    expect(formatearSoles(sumarCts(lineas))).toBe('S/ 990,00');
  });
  it('el emitido de la demo cuadra: 16.800 + 4.800 + 200 + 600 = 22.400', () => {
    expect(formatearSoles(sumarCts([1680000, 480000, 20000, 60000]))).toBe('S/ 22.400,00');
  });
});

describe('parsearSoles', () => {
  it('lee formatos peruanos y con punto decimal', () => {
    expect(parsearSoles('1.234,56')).toBe(123456);
    expect(parsearSoles('1234,56')).toBe(123456);
    expect(parsearSoles('1234.56')).toBe(123456);
    expect(parsearSoles('S/ 80')).toBe(8000);
    expect(parsearSoles('80,5')).toBe(8050);
    expect(parsearSoles('0,05')).toBe(5);
    expect(parsearSoles('5.000')).toBe(500000);
    expect(parsearSoles('1.234.567')).toBe(123456700);
    expect(parsearSoles('-10')).toBe(-1000);
  });
  it('rechaza lo que no es dinero', () => {
    expect(parsearSoles('')).toBeNull();
    expect(parsearSoles('abc')).toBeNull();
    expect(parsearSoles('1,2,3')).toBeNull();
    expect(parsearSoles('12,345')).toBeNull();
    expect(parsearSoles('1.23.4')).toBeNull();
  });
  it('ida y vuelta sin perder céntimos', () => {
    for (const cts of [0, 1, 99, 100, 70560, 99000, 1946000, 123456789]) {
      expect(parsearSoles(ctsATexto(cts))).toBe(cts);
    }
  });
});

describe('porcentajes y números', () => {
  it('formatea con coma decimal', () => {
    expect(formatearPct(13.125)).toBe('13,1 %');
    expect(formatearPct('100.0000', 2)).toBe('100,00 %');
    expect(formatearPct(4.2, 2)).toBe('4,20 %');
    expect(formatearNumero(357.143, 3)).toBe('357,143');
    expect(formatearNumero(1298)).toBe('1.298');
  });
  it('la morosidad de la demo da 13,1 %', () => {
    expect(pctDe(294000, 2240000)).toBe(13.1);
    expect(pctDe(1, 0)).toBe(0);
  });
});

import { formatearSolesCorto } from './dinero.js';
describe('formatearSolesCorto (franja de KPI)', () => {
  it('abrevia desde S/ 10.000 y deja el resto completo', () => {
    expect(formatearSolesCorto(1946000)).toBe('S/ 19,5 mil');
    expect(formatearSolesCorto(3412000)).toBe('S/ 34,1 mil');
    expect(formatearSolesCorto(2000000)).toBe('S/ 20 mil');
    expect(formatearSolesCorto(51000)).toBe('S/ 510,00');
    expect(formatearSolesCorto(-1946000)).toBe('-S/ 19,5 mil');
    expect(formatearSolesCorto(125000000)).toBe('S/ 1,3 mill.');
    expect(formatearSolesCorto(null)).toBe('—');
  });
});
