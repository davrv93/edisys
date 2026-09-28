import { useEffect, useState } from 'react';
import { api } from '../../lib/api.js';
import { formatearSoles } from '../../lib/dinero.js';
import { diaLima } from '../../lib/fechas.js';
import { Boton, Campo, Modal, SubirArchivo } from '../../ui/index.js';

const MEDIOS = [
  { valor: 'yape', etiqueta: 'Yape' },
  { valor: 'transferencia', etiqueta: 'Transferencia' },
  { valor: 'deposito', etiqueta: 'Depósito' },
  { valor: 'efectivo', etiqueta: 'Efectivo' },
];

/**
 * Registrar (admin) o informar (propietario) un pago con voucher.
 * Lo que informa el propietario entra «en revisión» y no baja la deuda hasta que se valida.
 */
export default function PagoModal({ abierto, onCerrar, eid, recibo, esAdmin, yape, onListo, dialog }) {
  const [f, setF] = useState({});
  const [errores, setErrores] = useState({});
  const [enviando, setEnviando] = useState(false);

  useEffect(() => {
    if (abierto && recibo) {
      setF({ monto_cts: recibo.saldo_cts || recibo.total_cts, medio: 'yape', codigo_operacion: '', fecha: diaLima(new Date()), voucher: null });
      setErrores({});
    }
  }, [abierto, recibo]);

  if (!recibo) return null;

  const enviar = async () => {
    const e = {};
    if (!f.monto_cts || f.monto_cts <= 0) e.monto_cts = 'El monto debe ser mayor a cero.';
    if (f.medio !== 'efectivo' && !f.codigo_operacion.trim()) e.codigo_operacion = 'Escribe el código de operación del voucher.';
    if (!esAdmin && !f.voucher) e.voucher = 'Sube la captura del voucher.';
    setErrores(e);
    if (Object.keys(e).length) return;
    setEnviando(true);
    try {
      const form = new FormData();
      form.set('monto_cts', String(f.monto_cts));
      form.set('medio', f.medio);
      form.set('codigo_operacion', f.codigo_operacion.trim());
      form.set('fecha', f.fecha);
      if (f.voucher) form.set('voucher', f.voucher);
      await api.form('POST', `/edificios/${eid}/recibos/${recibo.id}/pagos`, form);
      onListo?.();
    } catch (err) {
      if (err.status === 422 && Object.keys(err.campos).length) setErrores(err.campos);
      else await dialog.alert({ title: err.status === 409 ? 'Ese pago ya está registrado' : 'No se pudo registrar el pago', text: err.message });
    } finally {
      setEnviando(false);
    }
  };

  return (
    <Modal
      abierto={abierto}
      onCerrar={onCerrar}
      titulo={esAdmin ? `Registrar pago · ${recibo.unidad}` : `Pagar recibo · ${recibo.unidad}`}
      pie={
        <>
          <Boton variante="secundario" onClick={onCerrar}>
            Cancelar
          </Boton>
          <Boton onClick={enviar} cargando={enviando}>
            {esAdmin ? 'Registrar pago' : 'Enviar voucher'}
          </Boton>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {!esAdmin && yape && (
          <div className="flex flex-col gap-1 rounded-xl border border-acento-borde bg-acento-suave p-4 text-sm text-acento-hover">
            <span className="text-xs font-bold">PAGA CON YAPE</span>
            <span className="font-titulo text-2xl font-semibold text-acento">{yape.numero}</span>
            <span>{yape.titular}</span>
            <span>
              Monto exacto <b className="tabular-nums">{formatearSoles(recibo.saldo_cts || recibo.total_cts)}</b> · concepto «{recibo.unidad} {recibo.periodo}»
            </span>
          </div>
        )}
        <div className="grid grid-cols-2 gap-3">
          <Campo etiqueta="Monto" tipo="dinero" valor={f.monto_cts} onCambio={(v) => setF({ ...f, monto_cts: v })} error={errores.monto_cts} ayuda={`Saldo: ${formatearSoles(recibo.saldo_cts)}`} />
          <Campo etiqueta="Fecha" tipo="fecha" valor={f.fecha} onCambio={(v) => setF({ ...f, fecha: v })} error={errores.fecha} />
        </div>
        <Campo etiqueta="Medio" tipo="select" valor={f.medio} onCambio={(v) => setF({ ...f, medio: v })} opciones={esAdmin ? MEDIOS : MEDIOS.filter((m) => m.valor !== 'efectivo')} />
        <Campo etiqueta="Código de operación" valor={f.codigo_operacion} onCambio={(v) => setF({ ...f, codigo_operacion: v })} error={errores.codigo_operacion} placeholder="Ej. 000184" inputMode="numeric" />
        <SubirArchivo etiqueta={esAdmin ? 'Voucher (opcional)' : 'Captura del voucher'} ayuda="Imagen o PDF, hasta 10 MB" archivo={f.voucher} onArchivo={(a) => setF({ ...f, voucher: a })} error={errores.voucher} />
        {!esAdmin && <p className="text-sm text-texto-suave">Verás «Pago enviado, en revisión» hasta que la administración lo valide.</p>}
      </div>
    </Modal>
  );
}
