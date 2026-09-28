import { describe, it, expect } from 'vitest';
import { aDiezmilesimas, revisarSuma, esDni, esRuc, esCelular, enmascararDni } from './participacion.js';

describe('participaciones', () => {
  it('convierte a diezmilésimas sin float', () => {
    expect(aDiezmilesimas('4.2000')).toBe(42000);
    expect(aDiezmilesimas(4.2)).toBe(42000);
    expect(aDiezmilesimas('4,20')).toBe(42000);
    expect(aDiezmilesimas('33.3333')).toBe(333333);
    expect(aDiezmilesimas('x')).toBeNull();
  });
  it('la semilla (16 × 4,20 + 6 × 4,00 + 2 × 4,40) suma 100 %', () => {
    const v = [...Array(16).fill('4.2000'), ...Array(6).fill('4.0000'), ...Array(2).fill('4.4000')];
    expect(revisarSuma(v)).toEqual({ ok: true, suma: '100,0000 %', mensaje: null });
  });
  it('99,5 % se rechaza con el mensaje exacto de la guía', () => {
    expect(revisarSuma(['50', '49.5']).mensaje).toBe('Suman 99,5000 %, faltan 0,5000 %');
    expect(revisarSuma(['50', '50.5']).mensaje).toBe('Suman 100,5000 %, sobran 0,5000 %');
  });
  it('tolerancia de 0,0001 % (33,3333 × 3)', () => {
    expect(revisarSuma(['33.3333', '33.3333', '33.3334']).ok).toBe(true);
    expect(revisarSuma(['33.3333', '33.3333', '33.3333']).ok).toBe(true);
  });
});

describe('documentos y celular', () => {
  it('valida formatos peruanos', () => {
    expect(esDni('40000201')).toBe(true);
    expect(esDni('4000020')).toBe(false);
    expect(esRuc('20123456789')).toBe(true);
    expect(esRuc('30123456789')).toBe(false);
    expect(esCelular('900 000 201')).toBe(true);
    expect(esCelular('800000201')).toBe(false);
  });
  it('enmascara el DNI', () => {
    expect(enmascararDni('45123456')).toBe('4512****');
    expect(enmascararDni('4000****')).toBe('4000****');
  });
});
