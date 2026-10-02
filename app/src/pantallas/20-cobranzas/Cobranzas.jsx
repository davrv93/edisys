import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Modal, Vacio, useDialog, useToast } from '../../ui/index.js';

const FORM = { monto_cts: null, medio: 'transferencia', codigo_operacion: '', fecha: '', descripcion: '' };

/** Bloque A4 · Cobranzas sin identificar y devoluciones. */
export default function Cobranzas() {
  const eid = useEid();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [vista, setVista] = useState('pendientes');

  const cob = useCarga(() => api.get(`/edificios/${eid}/cobranzas-sin-identificar`, { estado: 'pendiente' }), [eid]);
  const dev = useCarga(() => api.get(`/edificios/${eid}/devoluciones`), [eid]);
  const [form, setForm] = useState(null);
  const [imputar, setImputar] = useState(null); // {cobranza, codigo}
  const [devolver, setDevolver] = useState(null); // {cobranza, motivo}
  const [ocupado, setOcupado] = useState(false);

  const listaCob = lista(cob.datos);
  const listaDev = lista(dev.datos);

  const guardar = async (e) => {
    e.preventDefault();
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/cobranzas-sin-identificar`, { ...form, monto_cts: form.monto_cts || 0 });
      toast('Cobranza registrada.', { tipo: 'exito' });
      setForm(null);
      await cob.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const imputarAhora = async () => {
    setOcupado(true);
    try {
      const r = await api.get(`/edificios/${eid}/unidades`, { buscar: imputar.codigo });
      const u = lista(r)[0];
      if (!u) throw new Error(`No encontré la unidad «${imputar.codigo}».`);
      await api.post(`/edificios/${eid}/cobranzas-sin-identificar/${imputar.cobranza.id}/imputar`, { unidad_id: u.id });
      toast('Cobranza imputada a la unidad.', { tipo: 'exito' });
      setImputar(null);
      await cob.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo imputar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const devolverAhora = async () => {
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/cobranzas-sin-identificar/${devolver.cobranza.id}/devolver`, { motivo: devolver.motivo });
      toast('Devolución registrada.', { tipo: 'exito' });
      setDevolver(null);
      await Promise.all([cob.recargar(), dev.recargar()]);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo devolver', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  return (
    <>
      {dialogEl}
      <Encabezado titulo="Cobranzas sin identificar" subtitulo="Dinero que entró sin recibo" ayuda="Registra el dinero que llega sin recibo, ímputalo a una unidad (se reparte del más antiguo al más nuevo) o regístralo como devolución." acciones={<Boton icono="mas_signo" onClick={() => setForm({ ...FORM })}>Nueva cobranza</Boton>} />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'pendientes'} icono="entrante" onClick={() => setVista('pendientes')} contador={listaCob.length}>
            Pendientes
          </Chip>
          <Chip activo={vista === 'devoluciones'} icono="salir" onClick={() => setVista('devoluciones')} contador={listaDev.length}>
            Devoluciones
          </Chip>
        </div>

        {vista === 'pendientes' ? (
          <Seccion titulo="Por identificar">
            {cob.error ? (
              <ErrorCarga error={cob.error} onReintentar={cob.recargar} />
            ) : !cob.datos ? (
              <Esqueleto className="h-40 w-full" />
            ) : listaCob.length === 0 ? (
              <Vacio icono="entrante" titulo="Nada por identificar" texto="Cuando entre dinero sin recibo, regístralo aquí para imputarlo después." />
            ) : (
              <ul className="divide-y divide-borde">
                {listaCob.map((c) => (
                  <li key={c.id} className="flex items-center gap-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-tinta">{formatearSoles(c.monto_cts)} · {c.medio}</p>
                      <p className="truncate text-xs text-texto-apoyo">{[formatearFecha(c.fecha), c.codigo_operacion, c.descripcion].filter(Boolean).join(' · ')}</p>
                    </div>
                    <Insignia estado="pendiente" texto="Pendiente" />
                    <Boton tamano="sm" variante="secundario" onClick={() => setImputar({ cobranza: c, codigo: '' })}>Imputar</Boton>
                    <Boton tamano="sm" variante="fantasma" className="!text-alerta" onClick={() => setDevolver({ cobranza: c, motivo: '' })}>Devolver</Boton>
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        ) : (
          <Seccion titulo="Devoluciones">
            {dev.error ? (
              <ErrorCarga error={dev.error} onReintentar={dev.recargar} />
            ) : !dev.datos ? (
              <Esqueleto className="h-40 w-full" />
            ) : listaDev.length === 0 ? (
              <Vacio icono="salir" titulo="Sin devoluciones" texto="Aquí quedan las devoluciones de dinero registradas." />
            ) : (
              <ul className="divide-y divide-borde">
                {listaDev.map((d) => (
                  <li key={d.id} className="flex items-center gap-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-tinta">{formatearSoles(d.monto_cts)}</p>
                      <p className="truncate text-xs text-texto-apoyo">{[formatearFecha(d.fecha), d.unidad && `Dpto ${d.unidad}`, d.motivo].filter(Boolean).join(' · ')}</p>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        )}
      </Contenido>

      <Modal abierto={!!form} onCerrar={() => setForm(null)} titulo="Nueva cobranza sin identificar" ancho="max-w-md" pie={<><Boton variante="fantasma" onClick={() => setForm(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardar}>Registrar</Boton></>}>
        {form && (
          <form className="flex flex-col gap-3" onSubmit={guardar}>
            <Campo etiqueta="Monto" tipo="dinero" valor={form.monto_cts} onCambio={(v) => setForm({ ...form, monto_cts: v })} />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Medio" tipo="select" valor={form.medio} onCambio={(v) => setForm({ ...form, medio: v })} opciones={[{ valor: 'transferencia', etiqueta: 'Transferencia' }, { valor: 'deposito', etiqueta: 'Depósito' }, { valor: 'efectivo', etiqueta: 'Efectivo' }, { valor: 'yape', etiqueta: 'Yape' }]} />
              <Campo etiqueta="Fecha" tipo="fecha" valor={form.fecha} onCambio={(v) => setForm({ ...form, fecha: v })} />
            </div>
            <Campo etiqueta="Código de operación" valor={form.codigo_operacion} onCambio={(v) => setForm({ ...form, codigo_operacion: v })} />
            <Campo etiqueta="Descripción" valor={form.descripcion} onCambio={(v) => setForm({ ...form, descripcion: v })} />
          </form>
        )}
      </Modal>

      <Modal abierto={!!imputar} onCerrar={() => setImputar(null)} titulo="Imputar a una unidad" ancho="max-w-sm" pie={<><Boton variante="fantasma" onClick={() => setImputar(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={imputarAhora}>Imputar</Boton></>}>
        {imputar && (
          <div className="flex flex-col gap-3">
            <p className="text-sm text-texto-apoyo">Se repartirá {formatearSoles(imputar.cobranza.monto_cts)} entre los recibos de la unidad, del más antiguo al más nuevo.</p>
            <Campo etiqueta="Código de unidad" valor={imputar.codigo} onCambio={(v) => setImputar({ ...imputar, codigo: v })} />
          </div>
        )}
      </Modal>

      <Modal abierto={!!devolver} onCerrar={() => setDevolver(null)} titulo="Registrar devolución" ancho="max-w-sm" pie={<><Boton variante="fantasma" onClick={() => setDevolver(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={devolverAhora}>Devolver</Boton></>}>
        {devolver && (
          <Campo etiqueta="Motivo" valor={devolver.motivo} onCambio={(v) => setDevolver({ ...devolver, motivo: v })} />
        )}
      </Modal>
    </>
  );
}
