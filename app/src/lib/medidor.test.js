import { describe, it, expect } from 'vitest';
import { parsearLectura, calcularConsumo, alertaConsumo, cargoAgua, siguientePendiente } from './medidor.js';

describe('lecturas', () => {
  it('lee con coma o punto y hasta 3 decimales', () => {
    expect(parsearLectura('876')).toBe(876);
    expect(parsearLectura('876,5')).toBe(876.5);
    expect(parsearLectura('876.125')).toBe(876.125);
    expect(parsearLectura('1.298,125')).toBe(1298.125);
    expect(parsearLectura('876,1234')).toBeNull();
    expect(parsearLectura('abc')).toBeNull();
    expect(parsearLectura('')).toBeNull();
  });
  it('consumo exacto en milésimas', () => {
    expect(calcularConsumo(861, 876)).toBe(15);
    expect(calcularConsumo(1284.1, 1298.1)).toBe(14);
    expect(calcularConsumo(0.1, 0.3)).toBe(0.2);
  });
  it('alertas: negativo y pico', () => {
    expect(alertaConsumo(-2, 14)).toBe('NEGATIVO');
    expect(alertaConsumo(29, 14)).toBe('PICO');
    expect(alertaConsumo(28, 14)).toBeNull();
    expect(alertaConsumo(15, null)).toBeNull();
  });
  it('caso del dueño: 14,000 m³ × S/ 14,00 = S/ 196,00; 15 m³ = S/ 210,00', () => {
    expect(cargoAgua(14, 1400)).toBe(19600);
    expect(cargoAgua(15, 1400)).toBe(21000);
    expect(cargoAgua(0.001, 1400)).toBe(1);
  });
  it('siguiente pendiente en orden de ronda', () => {
    const l = [
      { medidor_id: 1, orden_ronda: 1, estado: 'leida' },
      { medidor_id: 2, orden_ronda: 2, estado: 'pendiente' },
      { medidor_id: 3, orden_ronda: 3, estado: 'pendiente' },
    ];
    expect(siguientePendiente(l, 2).medidor_id).toBe(3);
    expect(siguientePendiente(l, 3).medidor_id).toBe(2);
    expect(siguientePendiente([{ medidor_id: 1, estado: 'leida' }], 1)).toBeNull();
  });
});
