import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Modal, Vacio, useDialog, useToast } from '../../ui/index.js';

const FORM_RE = { concepto: '', tercero: '', monto_cts: null, fecha_emision: '', fecha_vencimiento: '', recurrente: false };
const FORM_IE = { descripcion: '', monto_cts: null, fecha: '', medio: 'efectivo', fondo_id: '' };

/** Bloque B4 · Recibos externos e ingresos externos (lo que no es la cuota). */
export default function Externos() {
  const eid = useEid();
  const [periodo] = usePeriodo();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [vista, setVista] = useState('recibos');

  const re = useCarga(() => api.get(`/edificios/${eid}/recibos-externos`, { periodo }), [eid, periodo]);
  const ie = useCarga(() => api.get(`/edificios/${eid}/ingresos-externos`, { periodo }), [eid, periodo]);
  const fondos = useCarga(() => api.get(`/edificios/${eid}/fondos`), [eid]);

  const [formRe, setFormRe] = useState(null);
  const [formIe, setFormIe] = useState(null);
  const [ocupado, setOcupado] = useState(false);

  const listaRe = lista(re.datos);
  const listaIe = lista(ie.datos);
  const listaFondos = lista(fondos.datos);

  const guardarRe = async (e) => {
    e.preventDefault();
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/recibos-externos`, { ...formRe, monto_cts: formRe.monto_cts || 0 });
      toast('Recibo externo registrado.', { tipo: 'exito' });
      setFormRe(null);
      await re.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const pagar = async (r) => {
    try {
      await api.post(`/edificios/${eid}/recibos-externos/${r.id}/pagar`, {});
      toast('Recibo externo cobrado.', { tipo: 'exito' });
      await re.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo cobrar', text: err.message });
    }
  };

  const guardarIe = async (e) => {
    e.preventDefault();
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/ingresos-externos`, { ...formIe, monto_cts: formIe.monto_cts || 0, fondo_id: formIe.fondo_id ? Number(formIe.fondo_id) : 0 });
      toast('Ingreso registrado.', { tipo: 'exito' });
      setFormIe(null);
      await ie.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const acciones = vista === 'recibos' ? (
    <Boton icono="mas_signo" onClick={() => setFormRe({ ...FORM_RE })}>
      Nuevo recibo externo
    </Boton>
  ) : (
    <Boton icono="mas_signo" onClick={() => setFormIe({ ...FORM_IE, fecha: `${periodo}-01` })}>
      Nuevo ingreso
    </Boton>
  );

  return (
    <>
      {dialogEl}
      <Encabezado titulo="Recibos e ingresos externos" subtitulo={`Periodo ${periodo}`} ayuda="Documentos que no son la cuota de mantenimiento (alquileres a terceros) y los ingresos que no vienen de un recibo." acciones={acciones} />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'recibos'} icono="recibo" onClick={() => setVista('recibos')} contador={listaRe.length}>
            Recibos externos
          </Chip>
          <Chip activo={vista === 'ingresos'} icono="entrante" onClick={() => setVista('ingresos')} contador={listaIe.length}>
            Ingresos externos
          </Chip>
        </div>

        {vista === 'recibos' ? (
          <Seccion titulo="Recibos externos">
            {re.error ? (
              <ErrorCarga error={re.error} onReintentar={re.recargar} />
            ) : !re.datos ? (
              <Esqueleto className="h-40 w-full" />
            ) : listaRe.length === 0 ? (
              <Vacio icono="recibo" titulo="Sin recibos externos" texto="Registra alquileres o servicios a terceros que no son la cuota." />
            ) : (
              <ul className="divide-y divide-borde">
                {listaRe.map((r) => (
                  <li key={r.id} className="flex items-center gap-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-tinta">{r.concepto}</p>
                      <p className="truncate text-xs text-texto-apoyo">{[r.tercero || r.cliente, formatearFecha(r.fecha_vencimiento)].filter(Boolean).join(' · ')}</p>
                    </div>
                    <span className="shrink-0 text-sm font-semibold tabular-nums">{formatearSoles(r.monto_cts)}</span>
                    <Insignia estado={r.estado === 'pagado' ? 'pagado' : 'pendiente'} />
                    {r.estado === 'emitido' && (
                      <Boton tamano="sm" variante="secundario" onClick={() => pagar(r)}>
                        Cobrar
                      </Boton>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        ) : (
          <Seccion titulo="Ingresos externos">
            {ie.error ? (
              <ErrorCarga error={ie.error} onReintentar={ie.recargar} />
            ) : !ie.datos ? (
              <Esqueleto className="h-40 w-full" />
            ) : listaIe.length === 0 ? (
              <Vacio icono="entrante" titulo="Sin ingresos externos" texto="Registra el dinero que entra y no viene de un recibo (reciclaje, donaciones…)." />
            ) : (
              <ul className="divide-y divide-borde">
                {listaIe.map((r) => (
                  <li key={r.id} className="flex items-center gap-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-tinta">{r.descripcion}</p>
                      <p className="truncate text-xs text-texto-apoyo">{[formatearFecha(r.fecha), r.medio, r.fondo].filter(Boolean).join(' · ')}</p>
                    </div>
                    <span className="shrink-0 text-sm font-semibold tabular-nums text-acento-hover">{formatearSoles(r.monto_cts)}</span>
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        )}
      </Contenido>

      <Modal abierto={!!formRe} onCerrar={() => setFormRe(null)} titulo="Nuevo recibo externo" ancho="max-w-lg" pie={<><Boton variante="fantasma" onClick={() => setFormRe(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarRe}>Registrar</Boton></>}>
        {formRe && (
          <form className="flex flex-col gap-3" onSubmit={guardarRe}>
            <Campo etiqueta="Concepto" valor={formRe.concepto} onCambio={(v) => setFormRe({ ...formRe, concepto: v })} />
            <Campo etiqueta="Tercero" valor={formRe.tercero} onCambio={(v) => setFormRe({ ...formRe, tercero: v })} ayuda="Persona o empresa a la que se cobra." />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Monto" tipo="dinero" valor={formRe.monto_cts} onCambio={(v) => setFormRe({ ...formRe, monto_cts: v })} />
              <Campo etiqueta="Emisión" tipo="fecha" valor={formRe.fecha_emision} onCambio={(v) => setFormRe({ ...formRe, fecha_emision: v })} />
            </div>
            <Campo etiqueta="Vencimiento" tipo="fecha" valor={formRe.fecha_vencimiento} onCambio={(v) => setFormRe({ ...formRe, fecha_vencimiento: v })} />
          </form>
        )}
      </Modal>

      <Modal abierto={!!formIe} onCerrar={() => setFormIe(null)} titulo="Nuevo ingreso externo" ancho="max-w-md" pie={<><Boton variante="fantasma" onClick={() => setFormIe(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarIe}>Registrar</Boton></>}>
        {formIe && (
          <form className="flex flex-col gap-3" onSubmit={guardarIe}>
            <Campo etiqueta="Descripción" valor={formIe.descripcion} onCambio={(v) => setFormIe({ ...formIe, descripcion: v })} />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Monto" tipo="dinero" valor={formIe.monto_cts} onCambio={(v) => setFormIe({ ...formIe, monto_cts: v })} />
              <Campo etiqueta="Fecha" tipo="fecha" valor={formIe.fecha} onCambio={(v) => setFormIe({ ...formIe, fecha: v })} />
            </div>
            <Campo etiqueta="Fondo" tipo="select" valor={formIe.fondo_id} onCambio={(v) => setFormIe({ ...formIe, fondo_id: v })} opciones={[{ valor: '', etiqueta: 'Sin fondo' }, ...listaFondos.map((f) => ({ valor: String(f.id), etiqueta: f.nombre }))]} ayuda="Si eliges un fondo, el ingreso entra en su trazabilidad." />
          </form>
        )}
      </Modal>
    </>
  );
}
