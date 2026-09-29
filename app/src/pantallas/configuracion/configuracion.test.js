import { describe, expect, it } from 'vitest';
import { rucValido } from './FacturacionElectronica.jsx';
import { rotuloComprobante, tieneComprobanteVivo } from '../05-recibos/Comprobante.jsx';

describe('facturación electrónica', () => {
  it('valida el RUC con su dígito verificador', () => {
    expect(rucValido('20600000005')).toBe(true);
    expect(rucValido('20600000001')).toBe(false);
    expect(rucValido('40000201')).toBe(false);
  });
  it('en simulado no presenta el comprobante como aceptado de verdad', () => {
    expect(rotuloComprobante({ estado: 'aceptado', modo: 'simulado' }).texto).toBe('Aceptado (simulado)');
    expect(rotuloComprobante({ estado: 'aceptado', modo: 'beta' }).texto).toBe('Aceptado por SUNAT');
  });
  it('una boleta anulada deja volver a emitir', () => {
    expect(tieneComprobanteVivo([{ tipo: '03', estado: 'anulado' }, { tipo: '07', estado: 'aceptado' }])).toBe(false);
    expect(tieneComprobanteVivo([{ tipo: '03', estado: 'aceptado' }])).toBe(true);
  });
});
