import { describe, expect, it } from 'vitest';
import { ESTADOS, ESTADOS_DEL_PLAN, TONO_CLASES, TONO_PUNTO, TONO_TEXTO, infoEstado, tonoDe } from './estados.js';
import { existeIcono } from './Icono.jsx';
import { COLUMNAS } from '../lib/kanban.js';

describe('mapa de estados', () => {
  it('cubre todos los estados del plan', () => {
    for (const e of ESTADOS_DEL_PLAN) expect(ESTADOS[e], e).toBeTruthy();
  });

  it('cada estado tiene color, icono y texto', () => {
    for (const [clave, info] of Object.entries(ESTADOS)) {
      expect(TONO_CLASES[info.tono], `${clave}: tono`).toBeTruthy();
      expect(TONO_PUNTO[info.tono], `${clave}: punto`).toBeTruthy();
      expect(TONO_TEXTO[info.tono], `${clave}: texto de tono`).toBeTruthy();
      expect(existeIcono(info.icono), `${clave}: icono «${info.icono}»`).toBe(true);
      expect(typeof info.texto === 'string' && info.texto.length > 0, `${clave}: texto`).toBe(true);
    }
  });

  it('cada columna del kanban tiene su estado en el mapa', () => {
    for (const c of COLUMNAS) expect(ESTADOS[c.estado], c.estado).toBeTruthy();
  });

  it('sigue la regla de color: cobrado petróleo, deuda roja, pendiente ámbar, en curso índigo, hecho slate', () => {
    expect(tonoDe('pagado')).toBe('acento');
    expect(tonoDe('vencido')).toBe('alerta');
    expect(tonoDe('moroso')).toBe('alerta');
    expect(tonoDe('presupuestado')).toBe('aviso');
    expect(tonoDe('en_ejecucion')).toBe('curso');
    expect(tonoDe('terminado')).toBe('hecho');
  });

  it('un estado desconocido cae en neutro con su nombre legible', () => {
    expect(infoEstado('algo_raro')).toMatchObject({ tono: 'neutro', texto: 'algo raro' });
    expect(infoEstado(null).texto).toBe('—');
  });
});
