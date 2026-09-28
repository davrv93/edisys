import { describe, it, expect } from 'vitest';
import { ruta, paginaActual } from './nav.jsx';

describe('rutas de las páginas Astro', () => {
  it('construye rutas con barra final y query', () => {
    expect(ruta('inicio')).toBe('/app/');
    expect(ruta('balance')).toBe('/app/balance/');
    expect(ruta('recibos', { id: 9005, vacio: '' })).toBe('/app/recibos/?id=9005');
    expect(ruta('mantenimiento', new URLSearchParams('criticidad=critica'))).toBe('/app/mantenimiento/?criticidad=critica');
  });
  it('reconoce la página actual con o sin barra final', () => {
    expect(paginaActual('/app/')).toBe('inicio');
    expect(paginaActual('/app')).toBe('inicio');
    expect(paginaActual('/app/balance')).toBe('balance');
    expect(paginaActual('/app/balance/')).toBe('balance');
    expect(paginaActual('/app/whatsapp/')).toBe('whatsapp');
    expect(paginaActual('/app/no-existe/')).toBe('inicio');
  });
});
