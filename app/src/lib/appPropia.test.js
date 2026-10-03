import { describe, it, expect } from 'vitest';
import { leerMarcaApp, manifiestoPWA, manifiestoTWA, MARCA_POR_DEFECTO } from './appPropia.js';

describe('app propia (I4): marca del build', () => {
  it('sin variables sale EDISYS tal cual', () => {
    const m = leerMarcaApp({});
    expect(m.nombre).toBe('EDISYS');
    const man = manifiestoPWA(m, '/app');
    expect(man.id).toBe('/app/');
    expect(man.theme_color).toBe(MARCA_POR_DEFECTO.colorTema);
    expect(man.icons[0].src).toBe('/app/icon-192.png');
  });
  it('el nombre corto hereda del nombre si cabe', () => {
    expect(leerMarcaApp({ EDISYS_APP_NOMBRE: 'Torres' }).nombreCorto).toBe('Torres');
  });
  it('junta todos los errores en uno', () => {
    expect(() =>
      leerMarcaApp({ EDISYS_APP_NOMBRE_CORTO: 'Administradora Larga', EDISYS_APP_COLOR: 'rojo', EDISYS_APP_PAQUETE: 'MiApp' }),
    ).toThrow(/nombre corto.*color de tema.*paquete/);
  });
  it('el sufijo separa el id de instalación', () => {
    const m = leerMarcaApp({ EDISYS_APP_NOMBRE: 'Torres SAC', EDISYS_APP_NOMBRE_CORTO: 'Torres', EDISYS_APP_ID: 'torres' });
    expect(manifiestoPWA(m, '/edisys/app/').id).toBe('/edisys/app/?app=torres');
  });
  it('manifiesto TWA para Bubblewrap', () => {
    const m = leerMarcaApp({ EDISYS_APP_PAQUETE: 'pe.torres.app', EDISYS_APP_NOMBRE: 'Torres' });
    const t = manifiestoTWA(m, 'intranet.torres.pe', '/app');
    expect(t.packageId).toBe('pe.torres.app');
    expect(t.webManifestUrl).toBe('https://intranet.torres.pe/app/manifest.webmanifest');
    expect(() => manifiestoTWA(m, '', '/app')).toThrow(/dominio/);
  });
});
