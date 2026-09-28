import { describe, it, expect } from 'vitest';
import { medidasDestino, validarArchivo, leerFechaExif } from './imagen.js';

describe('imagen', () => {
  it('reduce el lado mayor a 1600 px manteniendo proporción', () => {
    expect(medidasDestino(4000, 3000)).toEqual({ ancho: 1600, alto: 1200 });
    expect(medidasDestino(3000, 4000)).toEqual({ ancho: 1200, alto: 1600 });
    expect(medidasDestino(800, 600)).toEqual({ ancho: 800, alto: 600 });
  });
  it('valida tipo y tope de tamaño', () => {
    const mb = 1024 * 1024;
    expect(validarArchivo({ type: 'application/pdf', size: 2 * mb })).toBeNull();
    expect(validarArchivo({ type: 'application/pdf', size: 11 * mb })).toMatch(/10 MB/);
    expect(validarArchivo({ type: 'application/zip', size: 1 })).toMatch(/tipo/);
    expect(validarArchivo({ type: 'video/mp4', size: 40 * mb }, { tipos: ['video/'] })).toBeNull();
    expect(validarArchivo(null)).toMatch(/Elige/);
    expect(validarArchivo({ type: '', name: 'Padron.XLSX', size: 1 }, { tipos: [], extensiones: ['.xlsx'] })).toBeNull();
  });
  it('sin EXIF devuelve null sin romperse', () => {
    expect(leerFechaExif(new Uint8Array([0xff, 0xd8, 0xff, 0xd9, 0, 0]).buffer)).toBeNull();
    expect(leerFechaExif(new Uint8Array([1, 2, 3]).buffer)).toBeNull();
  });
});
