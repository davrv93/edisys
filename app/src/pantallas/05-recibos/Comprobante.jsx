import { useState } from 'react';
import { api, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { Boton, Insignia, useDialog, useToast } from '../../ui/index.js';

const ESTADO = {
  aceptado: { tono: 'acento', texto: 'Aceptado por SUNAT' },
  rechazado: { tono: 'alerta', texto: 'Rechazado' },
  pendiente: { tono: 'aviso', texto: 'Pendiente' },
  anulado: { tono: 'neutro', texto: 'Anulado' },
};

/** Rótulo del estado de un comprobante; en simulado lo dice (nunca se presenta como aceptado de verdad). */
export function rotuloComprobante(c) {
  const e = ESTADO[c?.estado] || ESTADO.pendiente;
  if (c?.estado === 'aceptado' && c?.modo === 'simulado') return { tono: 'aviso', texto: 'Aceptado (simulado)' };
  return e;
}

/** ¿El recibo ya tiene boleta o factura viva? */
export function tieneComprobanteVivo(lista = []) {
  return lista.some((c) => (c.tipo === '01' || c.tipo === '03') && c.estado !== 'anulado' && c.estado !== 'rechazado');
}

/**
 * Comprobante electrónico SUNAT del recibo: «Emitir comprobante», estado del CDR y descarga de XML, PDF (con QR) y CDR.
 * Boleta si el propietario tiene DNI, factura si tiene RUC.
 */
export default function Comprobante({ eid, recibo }) {
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();
  const [emitiendo, setEmitiendo] = useState(false);
  const c = useCarga(() => api.get(`/edificios/${eid}/recibos/${recibo.id}/comprobante`), [eid, recibo.id], { activo: recibo.estado !== 'borrador' });
  const lista = c.datos?.comprobantes || [];
  const modo = c.datos?.modo;
  if (!c.datos || recibo.origen === 'deuda_inicial') return null;
  if (!lista.length && (!c.datos.puede_emitir || recibo.estado === 'anulado')) return null;

  const emitir = async () => {
    const ok = await dialog.confirm({
      title: 'Emitir comprobante electrónico',
      text: `Se emite boleta (DNI) o factura (RUC) por ${formatearSoles(recibo.total_cts)}.${modo === 'simulado' ? ' Modo simulado: no se envía a SUNAT.' : ''}`,
      okText: 'Emitir',
    });
    if (!ok) return;
    setEmitiendo(true);
    try {
      const r = await api.post(`/edificios/${eid}/recibos/${recibo.id}/comprobante`);
      toast(`${r.numero_completo}: ${r.cdr_descripcion || r.estado}`, { tipo: r.estado === 'aceptado' ? 'exito' : 'error' });
      await c.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo emitir', text: err.message });
    } finally {
      setEmitiendo(false);
    }
  };

  const anular = async (cp) => {
    const motivo = await dialog.prompt({
      title: `Anular ${cp.numero_completo}`,
      text: cp.tipo === '01' ? 'Si la factura tiene hasta 7 días va por comunicación de baja; si no, por nota de crédito.' : 'La boleta se anula con una nota de crédito.',
      label: 'Motivo',
      required: true,
      okText: 'Anular',
    });
    if (!motivo) return;
    try {
      const r = await api.post(`/edificios/${eid}/comprobantes/${cp.id}/anular`, { motivo });
      toast(r.via === 'baja' ? `Comunicación de baja ${r.baja_id} enviada.` : `Nota de crédito ${r.numero_completo} emitida.`, { tipo: 'exito' });
      await c.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo anular', text: err.message });
    }
  };

  return (
    <section className="flex flex-col gap-3 rounded-xl border border-borde p-4" aria-label="Comprobante electrónico">
      {dialogEl}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-base font-semibold">Comprobante electrónico</h3>
        {modo === 'simulado' && <Insignia tono="aviso" texto="SUNAT simulado" />}
      </div>
      {lista.map((cp) => {
        const e = rotuloComprobante(cp);
        return (
          <div key={cp.id} className="flex flex-col gap-2 border-t border-borde pt-3 first:border-t-0 first:pt-0">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="flex flex-col">
                <span className="text-sm font-semibold">
                  {cp.nombre_tipo} {cp.numero_completo}
                </span>
                <span className="text-xs text-texto-apoyo">
                  {formatearFecha(cp.fecha)} · {cp.cliente_nombre} · {formatearSoles(cp.total_cts)}
                  {cp.igv_cts ? ` (IGV ${formatearSoles(cp.igv_cts)})` : ''}
                </span>
              </span>
              <Insignia tono={e.tono} texto={e.texto} />
            </div>
            {cp.cdr_descripcion && <p className="text-xs text-texto-apoyo">CDR {cp.cdr_codigo}: {cp.cdr_descripcion}</p>}
            {cp.anulacion && <p className="text-xs text-alerta">Anulado por {cp.anulacion === 'baja' ? `comunicación de baja ${cp.baja_id}` : 'nota de crédito'}: {cp.anulacion_motivo}</p>}
            <div className="flex flex-wrap gap-2">
              <Boton tamano="sm" variante="secundario" icono="descargar" href={urlApi(`/edificios/${eid}/comprobantes/${cp.id}/pdf`)} target="_blank" rel="noopener">
                PDF con QR
              </Boton>
              <Boton tamano="sm" variante="secundario" icono="documento" href={urlApi(`/edificios/${eid}/comprobantes/${cp.id}/xml`)}>
                XML
              </Boton>
              <Boton tamano="sm" variante="fantasma" href={urlApi(`/edificios/${eid}/comprobantes/${cp.id}/cdr`)}>
                CDR
              </Boton>
              {c.datos.puede_emitir && cp.estado === 'aceptado' && cp.tipo !== '07' && (
                <Boton tamano="sm" variante="fantasma" onClick={() => anular(cp)}>
                  Anular
                </Boton>
              )}
            </div>
          </div>
        );
      })}
      {c.datos.puede_emitir && !tieneComprobanteVivo(lista) && recibo.estado !== 'anulado' && (
        <Boton icono="documento" cargando={emitiendo} onClick={emitir} className="self-start">
          Emitir comprobante
        </Boton>
      )}
    </section>
  );
}
