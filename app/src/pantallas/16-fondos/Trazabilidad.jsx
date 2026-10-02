import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { useEid } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, ErrorCarga, Esqueleto, Modal, Vacio, useDialog, useToast } from '../../ui/index.js';

const FORM_MOV = { monto_cts: null, tipo: 'egreso', fecha: '', descripcion: '' };
const FORM_TRANS = { origen_id: '', destino_id: '', monto_cts: null, fecha: '', motivo: '' };

function Monto({ cts, className = '' }) {
  return <span className={`tabular-nums ${cts < 0 ? 'text-alerta' : ''} ${className}`}>{formatearSoles(cts)}</span>;
}

/** Bloque C · Trazabilidad de fondos: cuánto entra y sale de cada servicio, con transferencias. */
export default function Trazabilidad() {
  const eid = useEid();
  const [periodo] = usePeriodo();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const desde = periodo;

  const t = useCarga(() => api.get(`/edificios/${eid}/fondos/trazabilidad`, { desde, hasta: desde }), [eid, desde]);
  const [movFondo, setMovFondo] = useState(null); // {fondo, movimientos}
  const [formMov, setFormMov] = useState(null);
  const [formTrans, setFormTrans] = useState(null);
  const [ocupado, setOcupado] = useState(false);

  const filas = lista(t.datos);
  const total = t.datos?.saldo_cts ?? 0;

  const abrirFondo = async (f) => {
    try {
      const d = await api.get(`/edificios/${eid}/fondos/${f.id}/movimientos`, { desde, hasta: desde });
      setMovFondo({ fondo: f, movimientos: lista(d) });
    } catch (err) {
      await dialog.alert({ title: 'No se pudo abrir', text: err.message });
    }
  };

  const guardarMov = async (e) => {
    e.preventDefault();
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/fondos/${formMov.fondo_id}/movimientos`, { ...formMov, monto_cts: formMov.monto_cts || 0 });
      toast('Movimiento registrado.', { tipo: 'exito' });
      setFormMov(null);
      await t.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const transferir = async (e) => {
    e.preventDefault();
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/fondos/transferencia`, {
        ...formTrans,
        origen_id: Number(formTrans.origen_id),
        destino_id: Number(formTrans.destino_id),
        monto_cts: formTrans.monto_cts || 0,
      });
      toast('Transferencia registrada.', { tipo: 'exito' });
      setFormTrans(null);
      await t.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo transferir', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const acciones = (
    <>
      <Boton variante="secundario" onClick={() => setFormTrans({ ...FORM_TRANS, origen_id: filas[0] ? String(filas[0].id) : '', destino_id: filas[1] ? String(filas[1].id) : '' })}>
        Transferir
      </Boton>
      <Boton icono="mas_signo" onClick={() => setFormMov({ ...FORM_MOV, fondo_id: filas[0] ? String(filas[0].id) : '', fecha: `${desde}-01` })}>
        Movimiento
      </Boton>
    </>
  );

  return (
    <>
      {dialogEl}
      <Encabezado titulo="Trazabilidad de fondos" subtitulo="Cuánto entra y sale de cada servicio" ayuda="Cada sol cobrado y cada egreso se asigna a un fondo (servicio). Toca un fondo para ver sus movimientos del periodo." acciones={acciones} />
      <Contenido>
        <Seccion titulo={`Saldos de ${desde}`} extra={<span className="text-sm text-texto-apoyo">Saldo total: <b className="tabular-nums text-tinta">{formatearSoles(total)}</b></span>}>
          {t.error ? (
            <ErrorCarga error={t.error} onReintentar={t.recargar} />
          ) : !t.datos ? (
            <Esqueleto className="h-56 w-full" />
          ) : filas.length === 0 ? (
            <Vacio icono="balance" titulo="Sin fondos" texto="Los fondos se crean solos al registrar el primer cobro del edificio." />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-borde text-left text-xs text-texto-apoyo">
                    <th className="py-2 pr-2 font-semibold">Fondo</th>
                    <th className="py-2 px-2 text-right font-semibold">Saldo anterior</th>
                    <th className="py-2 px-2 text-right font-semibold">Ingreso</th>
                    <th className="py-2 px-2 text-right font-semibold">Egreso</th>
                    <th className="py-2 pl-2 text-right font-semibold">Saldo</th>
                  </tr>
                </thead>
                <tbody>
                  {filas.map((f) => (
                    <tr key={f.id} className="cursor-pointer border-b border-borde last:border-0 hover:bg-fondo" onClick={() => abrirFondo(f)}>
                      <td className="py-2 pr-2">
                        <span className="font-medium text-tinta">{f.nombre}</span>
                        {f.categoria && <span className="ml-2 text-xs text-texto-apoyo">{f.categoria}</span>}
                      </td>
                      <td className="py-2 px-2 text-right"><Monto cts={f.anterior_cts} /></td>
                      <td className="py-2 px-2 text-right"><Monto cts={f.ingreso_cts} /></td>
                      <td className="py-2 px-2 text-right"><Monto cts={f.egreso_cts} /></td>
                      <td className="py-2 pl-2 text-right font-semibold"><Monto cts={f.saldo_cts} /></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Seccion>
      </Contenido>

      {/* Movimientos de un fondo */}
      <Modal abierto={!!movFondo} onCerrar={() => setMovFondo(null)} titulo={movFondo ? `Movimientos · ${movFondo.fondo.nombre}` : ''} ancho="max-w-xl" lateral>
        {movFondo && (
          movFondo.movimientos.length === 0 ? (
            <p className="text-sm text-texto-apoyo">Sin movimientos en el rango.</p>
          ) : (
            <ul className="divide-y divide-borde">
              {movFondo.movimientos.map((m) => (
                <li key={m.id} className="flex items-center justify-between gap-3 py-2 text-sm">
                  <span className="min-w-0">
                    <span className="text-texto-suave">{m.fecha?.slice(0, 10)}</span> · {m.descripcion || m.tipo}
                    <span className="ml-2 text-xs text-texto-apoyo">{m.origen}</span>
                  </span>
                  <Monto cts={m.monto_cts} className="shrink-0 font-semibold" />
                </li>
              ))}
            </ul>
          )
        )}
      </Modal>

      {/* Movimiento manual */}
      <Modal abierto={!!formMov} onCerrar={() => setFormMov(null)} titulo="Movimiento manual" ancho="max-w-md" pie={<><Boton variante="fantasma" onClick={() => setFormMov(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarMov}>Registrar</Boton></>}>
        {formMov && (
          <form className="flex flex-col gap-3" onSubmit={guardarMov}>
            <Campo etiqueta="Fondo" tipo="select" valor={formMov.fondo_id} onCambio={(v) => setFormMov({ ...formMov, fondo_id: v })} opciones={filas.map((f) => ({ valor: String(f.id), etiqueta: f.nombre }))} />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Tipo" tipo="select" valor={formMov.tipo} onCambio={(v) => setFormMov({ ...formMov, tipo: v })} opciones={[{ valor: 'ingreso', etiqueta: 'Ingreso' }, { valor: 'egreso', etiqueta: 'Egreso' }]} />
              <Campo etiqueta="Monto" tipo="dinero" valor={formMov.monto_cts} onCambio={(v) => setFormMov({ ...formMov, monto_cts: v })} />
            </div>
            <Campo etiqueta="Fecha" tipo="fecha" valor={formMov.fecha} onCambio={(v) => setFormMov({ ...formMov, fecha: v })} />
            <Campo etiqueta="Descripción" valor={formMov.descripcion} onCambio={(v) => setFormMov({ ...formMov, descripcion: v })} />
          </form>
        )}
      </Modal>

      {/* Transferencia */}
      <Modal abierto={!!formTrans} onCerrar={() => setFormTrans(null)} titulo="Transferir entre fondos" ancho="max-w-md" pie={<><Boton variante="fantasma" onClick={() => setFormTrans(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={transferir}>Transferir</Boton></>}>
        {formTrans && (
          <form className="flex flex-col gap-3" onSubmit={transferir}>
            <Campo etiqueta="Desde" tipo="select" valor={formTrans.origen_id} onCambio={(v) => setFormTrans({ ...formTrans, origen_id: v })} opciones={filas.map((f) => ({ valor: String(f.id), etiqueta: f.nombre }))} />
            <Campo etiqueta="Hacia" tipo="select" valor={formTrans.destino_id} onCambio={(v) => setFormTrans({ ...formTrans, destino_id: v })} opciones={filas.map((f) => ({ valor: String(f.id), etiqueta: f.nombre }))} />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Monto" tipo="dinero" valor={formTrans.monto_cts} onCambio={(v) => setFormTrans({ ...formTrans, monto_cts: v })} />
              <Campo etiqueta="Fecha" tipo="fecha" valor={formTrans.fecha} onCambio={(v) => setFormTrans({ ...formTrans, fecha: v })} />
            </div>
            <Campo etiqueta="Motivo" valor={formTrans.motivo} onCambio={(v) => setFormTrans({ ...formTrans, motivo: v })} />
          </form>
        )}
      </Modal>
    </>
  );
}
