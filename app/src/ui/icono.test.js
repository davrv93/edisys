import { describe, expect, it } from 'vitest';
import { existeIcono } from './Icono.jsx';

describe('Icono (Lucide)', () => {
  it('acepta todos los nombres de la v1 que siguen en uso', () => {
    const v1 = [
      'inicio', 'balance', 'recibo', 'edificio', 'calendario', 'medidor', 'herramienta', 'camara', 'mensaje', 'grafico', 'llave',
      'mas', 'menu', 'salir', 'volver', 'izq', 'der', 'abajo', 'arriba', 'cerrar', 'check', 'alerta', 'info', 'buscar',
      'subir', 'descargar', 'imprimir', 'enviar', 'candado', 'documento', 'usuario', 'ver', 'mas_signo', 'arrastrar', 'reloj', 'whatsapp', 'robot', 'engranaje',
    ];
    for (const n of v1) expect(existeIcono(n), n).toBe(true);
  });

  it('acepta Conciliación (aviso del balance), en español y con el nombre de Lucide', () => {
    for (const n of ['conciliacion', 'Landmark']) expect(existeIcono(n), n).toBe(true);
  });

  it('no trae iconos sin uso (filtro, foto, clave, celular, facturación, copiar)', () => {
    for (const n of ['filtro', 'foto', 'clave', 'celular', 'facturacion', 'FileCheck2', 'Image', 'KeyRound', 'Smartphone', 'ListFilter', 'Copy']) {
      expect(existeIcono(n), n).toBe(false);
    }
  });

  it('no inventa iconos', () => {
    expect(existeIcono('no-existe')).toBe(false);
  });
});
