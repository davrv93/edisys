import { describe, expect, it } from 'vitest';
import { dmyAIso, isoADmy, semanasDelMes } from './SelectorFecha.jsx';

describe('SelectorFecha (dd/mm/aaaa, lunes primero)', () => {
  it('convierte entre ISO y dd/mm/aaaa', () => {
    expect(isoADmy('2026-09-05')).toBe('05/09/2026');
    expect(isoADmy('')).toBe('');
    expect(dmyAIso('5/9/2026')).toBe('2026-09-05');
    expect(dmyAIso('05-09-26')).toBe('2026-09-05');
    expect(dmyAIso('31/02/2026')).toBe(null);
    expect(dmyAIso('09/31/2026')).toBe(null); // el mes 31 no existe: no se confunde día con mes
    expect(dmyAIso('hola')).toBe(null);
  });
  it('arma el mes con el lunes primero', () => {
    const s = semanasDelMes('2026-09'); // 1 de setiembre de 2026 es martes
    expect(s[0][0]).toBe(null);
    expect(s[0][1]).toBe('2026-09-01');
    expect(s.every((sem) => sem.length === 7)).toBe(true);
    expect(s.flat().filter(Boolean)).toHaveLength(30);
  });
});
