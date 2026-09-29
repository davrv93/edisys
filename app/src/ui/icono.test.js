import { describe, expect, it } from 'vitest';
import { existeIcono } from './Icono.jsx';

describe('Icono (Lucide)', () => {
  it('acepta todos los nombres de la v1', () => {
    const v1 = [
      'inicio', 'balance', 'recibo', 'edificio', 'calendario', 'medidor', 'herramienta', 'camara', 'mensaje', 'grafico', 'llave',
      'mas', 'menu', 'salir', 'volver', 'izq', 'der', 'abajo', 'arriba', 'cerrar', 'check', 'alerta', 'info', 'buscar', 'filtro',
      'subir', 'descargar', 'enviar', 'candado', 'documento', 'usuario', 'mas_signo', 'arrastrar', 'reloj', 'whatsapp', 'robot', 'engranaje',
    ];
    for (const n of v1) expect(existeIcono(n), n).toBe(true);
  });

  it('acepta Conciliación y Facturación electrónica, en español y con el nombre de Lucide', () => {
    for (const n of ['conciliacion', 'facturacion', 'Landmark', 'FileCheck2']) expect(existeIcono(n), n).toBe(true);
  });

  it('no inventa iconos', () => {
    expect(existeIcono('no-existe')).toBe(false);
  });
});
