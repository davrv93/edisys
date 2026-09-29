import { useEffect, useState } from 'react';
import { api, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearPct } from '../../lib/dinero.js';
import { formatearFecha, formatearDiaMes, formatearFechaHora, mesDePeriodo, nombrePeriodo, sumarMeses } from '../../lib/fechas.js';
import { useQuery } from '../../lib/nav.jsx';
import { useEid, useSesion, Guarda } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton, Tabla, Insignia, SelectorPeriodo, ErrorCarga, Vacio, Esqueleto, Icono, useDialog, useToast } from '../../ui/index.js';
import PagoModal from './PagoModal.jsx';
import { nombreUnidad } from '../../lib/unidad.js';

const POR_PAGINA = 25;
const MEDIO = { yape: 'Yape', transferencia: 'Transferencia', deposito: 'Depósito', efectivo: 'Efectivo' };

/** 05 · Recibos y cobranza. Lista a la izquierda, recibo a la derecha (en móvil, uno tras otro). */
export default function Recibos() {
  const eid = useEid();
  const s = useSesion();
  const esAdmin = s.tiene('pagos.registrar');
  const propio = !s.tiene('recibos.emitir') && s.tiene('portal.ver');
  const [periodo] = usePeriodo();
  const [q, setQuery] = useQuery();
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();
  const estado = q.get('estado') || '';
  const buscar = q.get('buscar') || '';
  const pagina = Number(q.get('pagina') || 1);
  const idSel = q.get('id');
  const [texto, setTexto] = useState(buscar);
  const [pagoAbierto, setPagoAbierto] = useState(false);
  const [trabajando, setTrabajando] = useState(null);

  // Búsqueda con pausa: no se pide al API en cada tecla.
  useEffect(() => {
    const t = setTimeout(() => texto !== buscar && setQuery({ buscar: texto, pagina: null }, { reemplazar: true }), 350);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [texto]);

  const listaQ = propio ? { mios: 1, pagina, por_pagina: POR_PAGINA } : { periodo, estado, buscar, pagina, por_pagina: POR_PAGINA };
  const lista = useCarga(() => api.get(`/edificios/${eid}/recibos`, listaQ), [eid, periodo, estado, buscar, pagina, propio]);
  const detalle = useCarga(() => api.get(`/edificios/${eid}/recibos/${idSel}`), [eid, idSel], { activo: !!idSel });

  // El portal abre el pago directo con ?pagar=1
  useEffect(() => {
    if (q.get('pagar') && detalle.datos) {
      setPagoAbierto(true);
      setQuery({ pagar: null }, { reemplazar: true });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detalle.datos]);

  const filas = (lista.datos?.datos || (Array.isArray(lista.datos) ? lista.datos : [])).map((f) => ({ ...f, unidad: nombreUnidad(f.unidad), estado: f.vencido ? 'vencido' : f.estado }));
  const total = lista.datos?.total ?? filas.length;
  const conteos = lista.datos?.conteos;
  const r0 = idSel ? detalle.datos : null;
  const r = r0 && {
    ...r0,
    unidad: nombreUnidad(r0.unidad),
    emitido: r0.emitido ?? r0.emitido_en,
    lineas: (r0.lineas || []).map((l) => ({ ...l, concepto: l.concepto || l.descripcion, detalle: l.detalle })),
    foto_medidor: r0.foto_medidor || (r0.medidor ? { url: r0.foto_medidor_url, medidor: r0.medidor.serie, lectura: r0.medidor.lectura_actual, tomada_en: r0.medidor.tomada_en } : null),
  };

  const seleccionar = (fila) => setQuery({ id: fila.id });

  const emitir = async () => {
    const ok = await dialog.confirm({ title: `¿Emitir los recibos de ${mesDePeriodo(periodo)}?`, text: 'Después de emitirlos ya no se editan; solo se anulan con motivo.', okText: 'Emitir recibos' });
    if (!ok) return;
    setTrabajando('emitir');
    try {
      const res = await api.post(`/edificios/${eid}/periodos/${periodo}/recibos/emitir`, {});
      toast(`Se emitieron ${res?.emitidos ?? ''} recibos de ${mesDePeriodo(periodo)}.`, { tipo: 'exito' });
      lista.recargar();
    } catch (err) {
      await dialog.alert({ title: err.codigo === 'YA_EMITIDO' ? 'Ese periodo ya se emitió' : 'No se pudieron emitir', text: err.message });
    } finally {
      setTrabajando(null);
    }
  };

  const generar = async () => {
    setTrabajando('generar');
    try {
      const res = await api.post(`/edificios/${eid}/periodos/${periodo}/recibos/generar`, {});
      const adv = res?.advertencias || [];
      await dialog.alert({
        title: `Borradores de ${mesDePeriodo(periodo)} listos`,
        text: `Total del periodo: ${formatearSoles(res?.total_cts)}.` + (adv.length ? `\n\nRevisa antes de emitir:\n• ${adv.join('\n• ')}` : '\n\nSin advertencias.'),
      });
      lista.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudieron generar', text: err.message });
    } finally {
      setTrabajando(null);
    }
  };

  const enviarCorreo = async () => {
    try {
      const res = await api.post(`/edificios/${eid}/recibos/enviar`, { recibo_ids: [r.id] });
      toast(`Recibo en cola de envío por correo (${res?.en_cola ?? 1}).`, { tipo: 'exito' });
    } catch (err) {
      dialog.alert({ title: 'No se pudo enviar', text: err.message });
    }
  };

  const enviarWhatsApp = async () => {
    try {
      const res = await api.post('/whatsapp/enviar', {
        unidad_id: r.unidad_id,
        plantilla: 'recibo',
        variables: { nombre: r.propietario, periodo: nombrePeriodo(r.periodo), unidad: String(r0.unidad), total: formatearSoles(r.total_cts), vence: formatearFecha(r.vence) },
      });
      const sim = res?.simulado || res?.estado === 'simulado';
      toast(sim ? 'Enviado en modo SIMULADO: no salió ningún WhatsApp real.' : 'Recibo enviado por WhatsApp.', { tipo: sim ? 'aviso' : 'exito' });
    } catch (err) {
      dialog.alert({ title: 'No se pudo enviar por WhatsApp', text: err.message });
    }
  };

  const anular = async () => {
    const motivo = await dialog.prompt({ title: `Anular recibo ${r.numero}`, text: 'Solo se anulan recibos sin pagos. Queda en la auditoría.', label: 'Motivo', required: true, okText: 'Anular' });
    if (motivo === null) return;
    try {
      await api.post(`/edificios/${eid}/recibos/${r.id}/anular`, { motivo });
      toast('Recibo anulado.', { tipo: 'exito' });
      detalle.recargar();
      lista.recargar();
    } catch (err) {
      dialog.alert({ title: 'No se pudo anular', text: err.message });
    }
  };

  const acciones = propio ? null : (
    <>
      <SelectorPeriodo periodo={periodo} onCambio={(p) => setQuery({ periodo: p, pagina: null, id: null }, { reemplazar: true })} />
      <Guarda permiso="recibos.emitir">
        <Boton variante="secundario" onClick={generar} cargando={trabajando === 'generar'}>
          Generar borradores
        </Boton>
        <Boton onClick={emitir} cargando={trabajando === 'emitir'} icono="recibo">
          Emitir {mesDePeriodo(periodo)}
        </Boton>
      </Guarda>
    </>
  );

  const columnas = [
    { clave: 'unidad', titulo: 'Unidad', movil: 'titulo' },
    { clave: 'propietario', titulo: 'Propietario', movil: 'sub' },
    ...(propio ? [{ clave: 'periodo', titulo: 'Periodo', render: (f) => nombrePeriodo(f.periodo) }] : []),
    { clave: 'total_cts', titulo: 'Total', alinear: 'der', render: (f) => formatearSoles(f.total_cts), movil: 'valor' },
    { clave: 'estado', titulo: 'Estado', render: (f) => <Insignia estado={f.estado} /> },
  ];

  const chip = (valor, etiqueta, n, tono) => {
    const activo = estado === valor;
    const base = tono === 'alerta' && !activo ? 'bg-alerta-suave text-alerta border-alerta-borde' : activo ? 'bg-tinta text-white border-tinta' : 'bg-superficie border-borde-fuerte text-tinta';
    return (
      <button type="button" aria-pressed={activo} onClick={() => setQuery({ estado: valor || null, pagina: null }, { reemplazar: true })} className={`h-11 rounded-lg border px-3 text-sm font-semibold sm:h-10 ${base}`}>
        {etiqueta}
        {n != null ? ` · ${n}` : ''}
      </button>
    );
  };

  const vistaLista = (
    <section className={`flex min-w-0 flex-col gap-3 lg:w-[440px] lg:shrink-0 xl:w-[480px] ${idSel ? 'hidden lg:flex' : 'flex'}`}>
      {!propio && (
        <div className="flex flex-wrap items-center gap-2">
          {chip('', 'Todos', conteos?.todos)}
          {chip('pagado', 'Pagados', conteos?.pagado)}
          {chip('vencido', 'Vencidos', conteos?.vencido, 'alerta')}
          <label className="relative min-w-[160px] flex-1">
            <span className="sr-only">Buscar unidad o propietario</span>
            <Icono nombre="buscar" tam={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-texto-apoyo" />
            <input type="search" value={texto} onChange={(e) => setTexto(e.target.value)} placeholder="Buscar unidad" className="h-11 w-full rounded-lg border border-borde-fuerte bg-superficie pl-9 pr-3 text-base focus:outline-none focus:ring-2 focus:ring-acento sm:h-10 sm:text-sm" />
          </label>
        </div>
      )}
      <div className="overflow-hidden rounded-xl border border-borde bg-superficie">
        <Tabla
          etiqueta="Recibos"
          columnas={columnas}
          filas={filas}
          cargando={lista.cargando}
          error={lista.error}
          onReintentar={lista.recargar}
          onFila={seleccionar}
          seleccionada={idSel}
          vacio={
            propio ? (
              <Vacio titulo="Aún no tienes recibos" texto={`Tu primer recibo llega el 1 de ${mesDePeriodo(sumarMeses(periodo, 1))}.`} compacto />
            ) : (
              <Vacio titulo={estado || buscar ? 'Ningún recibo coincide' : `Aún no hay recibos de ${mesDePeriodo(periodo)}`} texto={estado || buscar ? 'Prueba con otro filtro.' : 'Genera los borradores, revísalos y emítelos.'} compacto />
            )
          }
          paginacion={total > POR_PAGINA ? { pagina, porPagina: POR_PAGINA, total, onPagina: (p) => setQuery({ pagina: p }), unidad: 'recibos' } : undefined}
        />
        {total <= POR_PAGINA && filas.length > 0 && (
          <div className="border-t border-borde px-4 py-3 text-sm text-texto-apoyo">
            {filas.length} de {total} recibos
          </div>
        )}
      </div>
    </section>
  );

  const vistaDetalle = idSel && (
    <section className="flex min-w-0 flex-1 flex-col gap-4 rounded-xl border border-borde bg-superficie p-4 lg:p-8">
      <button type="button" onClick={() => setQuery({ id: null })} className="-ml-1 flex h-11 items-center gap-1 self-start text-sm font-semibold text-acento lg:hidden">
        <Icono nombre="volver" tam={18} /> Volver a la lista
      </button>
      {detalle.error ? (
        <ErrorCarga error={detalle.error} onReintentar={detalle.recargar} compacto />
      ) : !r ? (
        <div className="flex flex-col gap-3">
          <Esqueleto className="h-8 w-2/3" />
          <Esqueleto className="h-48 w-full" />
        </div>
      ) : (
        <DetalleRecibo
          r={r}
          edificio={s.edificio.nombre}
          acciones={
            <>
              <Boton variante="secundario" icono="descargar" href={urlApi(`/edificios/${eid}/recibos/${r.id}/pdf`)} target="_blank" rel="noopener">
                Descargar PDF
              </Boton>
              <Guarda permiso="recibos.emitir">
                <Boton variante="secundario" icono="enviar" onClick={enviarCorreo}>
                  Enviar por correo
                </Boton>
              </Guarda>
              <Guarda permiso="whatsapp.enviar">
                <Boton icono="whatsapp" onClick={enviarWhatsApp}>
                  Enviar por WhatsApp
                </Boton>
              </Guarda>
              {r.saldo_cts > 0 && r.estado !== 'anulado' && (
                <Guarda permiso={['pagos.registrar', 'pagos.informar']}>
                  <Boton icono="mas_signo" onClick={() => setPagoAbierto(true)}>
                    {esAdmin ? 'Registrar pago' : 'Pagar'}
                  </Boton>
                </Guarda>
              )}
              {!r.pagos?.length && r.estado !== 'anulado' && (
                <Guarda permiso="recibos.emitir">
                  <Boton variante="fantasma" onClick={anular}>
                    Anular
                  </Boton>
                </Guarda>
              )}
            </>
          }
        />
      )}
    </section>
  );

  return (
    <>
      {dialogEl}
      <Encabezado titulo={propio ? 'Mis recibos' : `Recibos · ${nombrePeriodo(periodo)}`} acciones={acciones} />
      <Contenido>
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:gap-6">
          {vistaLista}
          {vistaDetalle || (
            <div className="hidden flex-1 rounded-xl border border-dashed border-borde-fuerte lg:block">
              <Vacio titulo="Elige un recibo" texto="Verás su desglose, la foto del medidor y sus pagos." icono="recibo" />
            </div>
          )}
        </div>
      </Contenido>
      <PagoModal
        abierto={pagoAbierto}
        onCerrar={() => setPagoAbierto(false)}
        eid={eid}
        recibo={r}
        esAdmin={esAdmin}
        yape={s.edificio.yape || (r?.yape_numero ? { numero: r.yape_numero, titular: r.edificio || s.edificio.nombre } : { numero: 'el Yape del edificio', titular: s.edificio.nombre })}
        dialog={dialog}
        onListo={() => {
          setPagoAbierto(false);
          toast(esAdmin ? 'Pago registrado.' : 'Pago enviado, en revisión.', { tipo: 'exito' });
          detalle.recargar();
          lista.recargar();
        }}
      />
    </>
  );
}

export function DetalleRecibo({ r, edificio, acciones }) {
  const pago = r.pagos?.find((p) => p.estado === 'validado');
  const enRevision = r.pagos?.some((p) => p.estado === 'pendiente_validacion');
  const textoEstado =
    r.estado === 'pagado' && pago
      ? `Pagado ${formatearDiaMes(pago.fecha)} · ${MEDIO[pago.medio] || pago.medio}${pago.codigo_operacion ? ` · Op. ${pago.codigo_operacion}` : ''}`
      : enRevision
        ? 'Pago enviado, en revisión'
        : undefined;
  const lineas = r.lineas || [];
  const foto = r.foto_medidor || (r.foto_medidor_url ? { url: r.foto_medidor_url } : null);
  return (
    <>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 flex-col gap-1">
          <span className="text-sm text-texto-apoyo">
            Recibo N.º {r.numero} · {edificio}
          </span>
          <h2 className="font-titulo text-2xl font-semibold lg:text-3xl">
            {r.unidad} · {r.propietario}
          </h2>
          <span className="text-sm text-texto-suave">
            {r.participacion_pct != null && `Participación ${formatearPct(r.participacion_pct, 2)} · `}Emitido {formatearFecha(r.emitido)} · Vence {formatearFecha(r.vence)}
          </span>
        </div>
        <Insignia estado={enRevision && r.estado !== 'pagado' ? 'pendiente_validacion' : r.estado} texto={textoEstado} tam="md" className="self-start whitespace-normal" />
      </div>

      <div className="flex flex-col gap-6 2xl:flex-row 2xl:items-start">
        <table className="w-full flex-1 text-sm">
          <caption className="sr-only">Desglose del recibo</caption>
          <tbody>
            {lineas.map((l, i) => (
              <tr key={i} className="border-b border-superficie-2">
                <td className="py-3 pr-3">
                  {l.concepto}
                  {l.detalle && <div className="text-xs text-texto-apoyo">{l.detalle}</div>}
                </td>
                <td className="py-3 text-right font-semibold tabular-nums">{formatearSoles(l.monto_cts)}</td>
              </tr>
            ))}
            <tr>
              <td className="pt-4 text-lg font-semibold">Total</td>
              <td className="pt-4 whitespace-nowrap text-right font-titulo text-3xl font-semibold tabular-nums">{formatearSoles(r.total_cts)}</td>
            </tr>
            {r.saldo_cts > 0 && r.saldo_cts !== r.total_cts && (
              <tr>
                <td className="pt-1 text-sm text-alerta">Saldo pendiente</td>
                <td className="pt-1 text-right font-semibold tabular-nums text-alerta">{formatearSoles(r.saldo_cts)}</td>
              </tr>
            )}
          </tbody>
        </table>
        {foto && (
          <figure className="flex w-full flex-col gap-2 sm:max-w-[260px] 2xl:w-[220px] 2xl:shrink-0">
            <div className="flex h-40 flex-col items-center justify-center gap-1 overflow-hidden rounded-xl bg-superficie-oscura text-xs text-texto-claro">
              {foto.url ? (
                <img src={foto.url} alt={`Foto del medidor ${foto.medidor || ''}`} className="h-full w-full object-cover" />
              ) : (
                <>
                  <Icono nombre="camara" tam={24} />
                  <span>[Foto del medidor]</span>
                  {foto.lectura != null && <span className="font-titulo text-2xl text-white">{String(foto.lectura).padStart(5, '0')}</span>}
                </>
              )}
            </div>
            <figcaption className="text-xs text-texto-apoyo">
              Medidor agua {foto.medidor} · {formatearFechaHora(foto.tomada_en)}
              {foto.operario ? ` · ${foto.operario}` : ''}
            </figcaption>
          </figure>
        )}
      </div>

      {r.pagos?.length > 0 && (
        <div className="flex flex-col gap-2">
          <h3 className="text-sm font-semibold">Pagos</h3>
          <ul className="flex flex-col gap-2">
            {r.pagos.map((p) => (
              <li key={p.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-borde px-3 py-2 text-sm">
                <span>
                  {formatearFecha(p.fecha)} · {MEDIO[p.medio] || p.medio}
                  {p.codigo_operacion ? ` · Op. ${p.codigo_operacion}` : ''}
                </span>
                <span className="flex items-center gap-2">
                  <b className="tabular-nums">{formatearSoles(p.monto_cts)}</b>
                  <Insignia estado={p.estado === 'validado' ? 'validado_pago' : p.estado} />
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}

      <p className="rounded-lg bg-fondo p-3 text-xs text-texto-apoyo">Recibo interno de mantenimiento; no es un comprobante SUNAT.</p>
      {acciones && <div className="flex flex-col gap-2 border-t border-borde pt-4 sm:flex-row sm:flex-wrap sm:justify-end">{acciones}</div>}
    </>
  );
}
