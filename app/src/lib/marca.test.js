import { describe, it, expect } from 'vitest';
import { cssDeTokens, contraste, CONTRASTE_MINIMO } from './marca.js';

describe('marca blanca (I2)', () => {
  it('arma las variables del tema claro y del oscuro', () => {
    const css = cssDeTokens({ claro: { '--color-acento': '#7C2D12' }, oscuro: { '--color-acento': '#D9A58C' } });
    expect(css).toBe(':root{--color-acento:#7C2D12;}:root[data-tema="oscuro"]{--color-acento:#D9A58C;}');
  });
  it('no deja pasar CSS arbitrario', () => {
    const css = cssDeTokens({ claro: { '--color-acento': 'red;}body{display:none', 'background': '#000000', '--color-x': '#12345' } });
    expect(css).toBe('');
  });
  it('sin tokens no escribe nada', () => {
    expect(cssDeTokens(null)).toBe('');
  });
  it('contraste igual que en Go', () => {
    expect(contraste('#FFFFFF', '#000000')).toBeCloseTo(21, 1);
    expect(contraste('#155E75', '#FFFFFF')).toBeGreaterThan(CONTRASTE_MINIMO);
    expect(contraste('#FACC15', '#FFFFFF')).toBeLessThan(CONTRASTE_MINIMO);
    expect(contraste('rojo', '#FFFFFF')).toBe(0);
  });
});
