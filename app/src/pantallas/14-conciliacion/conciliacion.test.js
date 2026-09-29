import { describe, expect, it } from 'vitest';
import { estadoMovimiento, pendientes } from './Conciliacion.jsx';

describe('conciliación', () => {
  it('rotula cada estado con texto (el color nunca va solo)', () => {
    expect(estadoMovimiento('conciliado').texto).toBe('Conciliado');
    expect(estadoMovimiento('sugerido').texto).toBe('Por confirmar');
    expect(estadoMovimiento('raro').texto).toBe('Sin pareja');
  });
  it('cuenta lo que falta resolver', () => {
    expect(pendientes([{ estado: 'conciliado' }, { estado: 'sugerido' }, { estado: 'sin_pareja' }])).toBe(2);
  });
});
