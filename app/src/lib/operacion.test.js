import { describe, it, expect } from 'vitest';
import { infoSemaforo, duracion, restanteSLA, trazoQR, normalizarPlaca, montoParking } from './operacion.js';

describe('semáforo del SLA', () => {
  it('lleva tono y texto', () => {
    expect(infoSemaforo('rojo')).toEqual({ tono: 'alerta', texto: 'Vencido' });
    expect(infoSemaforo('ambar').tono).toBe('aviso');
    expect(infoSemaforo('cerrado', true).texto).toBe('Cerrado en plazo');
    expect(infoSemaforo('cerrado', false).tono).toBe('alerta');
    expect(infoSemaforo(null).texto).toBe('Sin SLA');
  });
  it('dice cuánto falta o cuánto pasó', () => {
    expect(restanteSLA(135)).toBe('vence en 2 h 15 min');
    expect(restanteSLA(-2 * 1440)).toBe('venció hace 2 d');
    expect(duracion(45)).toBe('45 min');
    expect(duracion(1500)).toBe('1 d 1 h');
  });
});

describe('QR', () => {
  it('junta los módulos negros de cada fila', () => {
    expect(trazoQR(['110', '001'])).toBe('M0 0h2v1h-2zM2 1h1v1h-1z');
    expect(trazoQR([])).toBe('');
  });
});

describe('parking', () => {
  it('normaliza la placa como el API', () => {
    expect(normalizarPlaca('abc-123 ')).toBe('ABC123');
  });
  it('cobra igual que el API (fracción empezada, tolerancia, redondeo)', () => {
    expect(montoParking(10, 500, 60, 15)).toBe(0);
    expect(montoParking(16, 500, 60, 15)).toBe(500);
    expect(montoParking(135, 400, 30, 0)).toBe(1000);
    expect(montoParking(20, 333, 15, 0)).toBe(167);
  });
});
