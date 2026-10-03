import { describe, it, expect } from 'vitest';
import { alternarOpcion, cuerpoEncuesta, cuerpoRespuestas, formularioDeEncuesta, opcionesDeTexto } from './encuesta.js';

describe('encuestas (J1): formulario → API', () => {
  it('opciones: una por línea, sin vacías ni repetidas', () => {
    expect(opcionesDeTexto(' Sí \n\nsí\nNo\n')).toEqual(['Sí', 'No']);
  });
  it('rechaza preguntas de opción con menos de dos opciones distintas', () => {
    const { error } = cuerpoEncuesta({ titulo: 'T', preguntas: [{ texto: '¿?', tipo: 'unica', opciones: 'Sí\nsí' }] });
    expect(error).toMatch(/dos opciones/);
  });
  it('las preguntas de texto no mandan opciones', () => {
    const { cuerpo } = cuerpoEncuesta({ titulo: ' T ', preguntas: [{ texto: 'Opinión', tipo: 'texto', obligatoria: false, opciones: 'x' }] });
    expect(cuerpo.titulo).toBe('T');
    expect(cuerpo.preguntas[0]).toEqual({ texto: 'Opinión', tipo: 'texto', obligatoria: false });
  });
  it('ida y vuelta con el detalle del API', () => {
    const f = formularioDeEncuesta({ titulo: 'A', anonima: false, preguntas: [{ texto: 'P', tipo: 'multiple', obligatoria: true, opciones: [{ id: 1, texto: 'x' }, { id: 2, texto: 'y' }] }] });
    expect(cuerpoEncuesta(f).cuerpo.preguntas[0].opciones).toEqual(['x', 'y']);
  });
});

describe('encuestas (J1): responder', () => {
  const pregs = [
    { id: 1, tipo: 'unica', obligatoria: true, texto: 'Color' },
    { id: 2, tipo: 'multiple', obligatoria: false, texto: 'Áreas' },
    { id: 3, tipo: 'texto', obligatoria: true, texto: 'Comentario' },
  ];
  it('única reemplaza, múltiple alterna', () => {
    let m = {};
    m = alternarOpcion(m, pregs[0], 10);
    m = alternarOpcion(m, pregs[0], 11);
    expect(m[1]).toEqual([11]);
    m = alternarOpcion(m, pregs[1], 20);
    m = alternarOpcion(m, pregs[1], 21);
    m = alternarOpcion(m, pregs[1], 20);
    expect(m[2]).toEqual([21]);
  });
  it('lista las obligatorias que faltan', () => {
    const { cuerpo, faltan } = cuerpoRespuestas(pregs, { 1: [10], 3: '  ' });
    expect(faltan).toEqual(['Comentario']);
    expect(cuerpo.respuestas).toEqual([{ pregunta_id: 1, opcion_ids: [10] }]);
  });
});
