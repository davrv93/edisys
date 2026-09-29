import { useMemo, useState } from 'react';
import { api } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { diaLima, diasDeSemana, etiquetaDia, formatearHora, inicioSemana, rangoSemana, sumarDias } from '../../lib/fechas.js';
import { ruta, useQuery } from '../../lib/nav.jsx';
import { useEid, useSesion, Guarda } from '../../layout/Sesion.jsx';
import { veCalendarioReservas } from '../../lib/permisos.js';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton, Calendario, LeyendaCalendario, ErrorCarga, Esqueleto, Icono, Insignia, Modal, Vacio, useDialog, useToast, BotonIcono } from '../../ui/index.js';
import NuevaReserva from './NuevaReserva.jsx';
import { nombreUnidad } from '../../lib/unidad.js';

/** Cómo se cobra una reserva, con la forma del API (modo_cobro) o la del mock (medio). */
export function textoCobro(r) {
  if (r.estado === 'pendiente_pago') return 'Esperando pago';
  if (r.medio) return r.medio;
  if (r.modo_cobro === 'cargo_recibo') return 'Cargo al recibo';
  if (r.modo_cobro === 'pago_inmediato') return r.pago_validado ? 'Pagado' : 'Pago por validar';
  return '';
}

/** 07 · Reservas. Administración: calendario semanal. Propietario/inquilino (o ?nueva=1): reservar desde el celular. */
export default function Reservas() {
  const s = useSesion();
  const [q] = useQuery();
  if (q.get('nueva') || !veCalendarioReservas(s.tiene)) return <NuevaReserva />;
  return <CalendarioAdmin />;
}

function CalendarioAdmin() {
  const eid = useEid();
  const s = useSesion();
  const [q, setQuery] = useQuery();
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();
  const hoy = diaLima(new Date());
  const lunes = /^\d{4}-\d{2}-\d{2}$/.test(q.get('semana') || '') ? inicioSemana(q.get('semana')) : inicioSemana(hoy);
  const dias = diasDeSemana(lunes);
  const [diaMovil, setDiaMovil] = useState(dias.includes(hoy) ? hoy : lunes);
  const [evento, setEvento] = useState(null);
  const [trabajando, setTrabajando] = useState(false);

  const areas = useCarga(() => api.get(`/edificios/${eid}/areas`), [eid]);
  const reservas = useCarga(() => api.get(`/edificios/${eid}/reservas`, { desde: dias[0], hasta: dias[6] }), [eid, lunes]);

  const listaAreas = Array.isArray(areas.datos) ? areas.datos : areas.datos?.datos || [];
  const recursos = listaAreas.flatMap((a) =>
    (a.recursos || []).filter((r) => r.activo !== false).map((r) => ({ id: r.id, nombre: r.nombre, area: a, detalle: `${a.tarifa_cts ? formatearSoles(a.tarifa_cts, { sinDecimales: true }) : 'Sin costo'}${a.duracion_h ? ` · ${a.duracion_h} h` : ''}` })),
  );
  const listaRes = useMemo(() => (Array.isArray(reservas.datos) ? reservas.datos : reservas.datos?.datos || []), [reservas.datos]);
  const eventos = listaRes
    .filter((r) => !['cancelada', 'vencida'].includes(r.estado))
    .map((r) => ({
      ...r,
      dia: diaLima(r.inicio),
      desde: formatearHora(r.inicio),
      hasta: formatearHora(r.fin),
      unidad: nombreUnidad(r.unidad),
      titulo: r.estado === 'bloqueo' ? 'Cerrada' : nombreUnidad(r.unidad),
      sub: textoCobro(r),
      medio: textoCobro(r),
    }));

  const resumen = useMemo(() => {
    const vivas = listaRes.filter((r) => ['confirmada', 'pendiente_pago'].includes(r.estado));
    const suma = (f) => vivas.filter(f).reduce((a, r) => a + (r.total_cts || 0), 0);
    return {
      cantidad: vivas.length,
      enLinea: suma((r) => r.estado === 'confirmada' && !/recibo/i.test(textoCobro(r))),
      alRecibo: suma((r) => r.estado === 'confirmada' && /recibo/i.test(textoCobro(r))),
      porConfirmar: suma((r) => r.estado === 'pendiente_pago'),
    };
  }, [listaRes]);

  const cambiarEstado = async (estado, texto) => {
    let motivo = null;
    if (estado !== 'confirmada') {
      motivo = await dialog.prompt({ title: texto, label: 'Motivo', required: true, okText: texto, text: 'Queda registrado en la auditoría.' });
      if (motivo === null) return;
    }
    setTrabajando(true);
    try {
      await api.patch(`/edificios/${eid}/reservas/${evento.id}`, { estado, motivo });
      toast(estado === 'confirmada' ? `Reserva ${evento.codigo} confirmada. Su ingreso entra solo al balance.` : `Reserva ${evento.codigo} actualizada.`, { tipo: 'exito' });
      setEvento(null);
      reservas.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo actualizar', text: err.message });
      reservas.recargar();
    } finally {
      setTrabajando(false);
    }
  };

  const semana = (n) => {
    const nuevo = sumarDias(lunes, 7 * n);
    setQuery({ semana: nuevo }, { reemplazar: true });
    setDiaMovil(nuevo);
  };

  const acciones = (
    <>
      <div className="flex items-center gap-2">
        <BotonIcono variante="secundario" icono="izq" etiqueta="Semana anterior" onClick={() => semana(-1)} />
        <span className="min-w-[150px] text-center text-base font-semibold" aria-live="polite">
          {rangoSemana(lunes)}
        </span>
        <BotonIcono variante="secundario" icono="der" etiqueta="Semana siguiente" onClick={() => semana(1)} />
      </div>
      <Guarda permiso="reservas.administrar">
        <Boton icono="mas_signo" href={ruta('reservas', { nueva: 1 })}>
          Nueva reserva
        </Boton>
      </Guarda>
    </>
  );

  const area0 = listaAreas[0];

  return (
    <>
      {dialogEl}
      <Encabezado titulo="Reservas de áreas comunes" acciones={acciones} />
      <Contenido>
        {areas.error || reservas.error ? (
          <ErrorCarga error={areas.error || reservas.error} onReintentar={() => (areas.recargar(), reservas.recargar())} />
        ) : areas.datos && recursos.length === 0 ? (
          <Vacio titulo="Configura tu primera área" texto="Parrillas, SUM, piscina… con su tarifa, horario, aforo y normas." icono="calendario" />
        ) : (
          <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:gap-6">
            <section className="min-w-0 flex-1 overflow-hidden rounded-tarjeta border border-borde bg-superficie">
              {!areas.datos || !reservas.datos ? (
                <div className="flex flex-col gap-2 p-4">
                  {Array.from({ length: 5 }, (_, i) => (
                    <Esqueleto key={i} className="h-16 w-full" />
                  ))}
                </div>
              ) : (
                <Calendario
                  dias={dias}
                  recursos={recursos}
                  eventos={eventos}
                  hoy={hoy}
                  onEvento={setEvento}
                  onCelda={s.tiene('reservas.administrar') ? (r, d) => (window.location.href = ruta('reservas', { nueva: 1, recurso: r.id, dia: d })) : undefined}
                  diaMovil={diaMovil}
                  onDiaMovil={setDiaMovil}
                />
              )}
              <LeyendaCalendario />
            </section>

            <aside className="flex w-full flex-col gap-4 xl:w-[300px] xl:shrink-0">
              <section className="flex flex-col gap-3 rounded-tarjeta border border-borde bg-superficie p-5">
                <h2 className="text-base font-semibold">Esta semana</h2>
                {[
                  ['Reservas', resumen.cantidad],
                  ['Cobrado en línea', formatearSoles(resumen.enLinea)],
                  ['Cargo al recibo', formatearSoles(resumen.alRecibo)],
                ].map(([t, v]) => (
                  <div key={t} className="flex justify-between text-sm">
                    <span className="text-texto-suave">{t}</span>
                    <b className="tabular-nums">{v}</b>
                  </div>
                ))}
                <div className="flex justify-between text-sm">
                  <span className="text-texto-suave">Por confirmar</span>
                  <b className="tabular-nums text-aviso">{formatearSoles(resumen.porConfirmar)}</b>
                </div>
                <span className="text-xs text-texto-apoyo">Cada pago validado entra solo a Ingresos › Reservas de áreas con su código de reserva.</span>
              </section>
              {area0 && (
                <section className="flex flex-col gap-2 rounded-tarjeta border border-borde bg-superficie p-5 text-sm">
                  <h2 className="text-base font-semibold">Reglas de {area0.nombre}</h2>
                  <span className="text-texto-suave">
                    Turnos: {area0.horario || (area0.franjas || []).map((f) => `${f.inicio}–${f.fin}`).join(' · ') || '—'}
                  </span>
                  {area0.aforo ? (
                    <span className="text-texto-suave">
                      Aforo {area0.aforo} personas{area0.incluye ? ` · ${area0.incluye}` : ''}
                    </span>
                  ) : null}
                  <span className="text-texto-suave">Cobro: {area0.cobro || (areas.datos?.modo_cobro === 'pago_inmediato' ? 'pago inmediato con voucher' : areas.datos?.modo_cobro === 'cargo_recibo' ? 'cargo al recibo' : '—')}</span>
                  <span className="text-texto-suave">Unidades morosas: no pueden reservar</span>
                </section>
              )}
            </aside>
          </div>
        )}
      </Contenido>

      <Modal
        abierto={!!evento}
        onCerrar={() => setEvento(null)}
        titulo={evento ? `${evento.codigo || 'Reserva'} · ${recursos.find((r) => r.id === evento.recurso_id)?.nombre || ''}` : ''}
        pie={
          evento &&
          s.tiene('reservas.administrar') &&
          evento.estado !== 'bloqueo' && (
            <>
              {evento.estado === 'confirmada' && (
                <Boton variante="secundario" onClick={() => cambiarEstado('no_show', 'Marcar no se presentó')} disabled={trabajando}>
                  No se presentó
                </Boton>
              )}
              <Boton variante="secundario" onClick={() => cambiarEstado('cancelada', 'Cancelar reserva')} disabled={trabajando}>
                Cancelar reserva
              </Boton>
              {evento.estado === 'pendiente_pago' && (
                <Boton onClick={() => cambiarEstado('confirmada')} cargando={trabajando}>
                  Validar pago y confirmar
                </Boton>
              )}
            </>
          )
        }
      >
        {evento && (
          <dl className="grid grid-cols-[110px_1fr] gap-x-3 gap-y-2 text-sm">
            <dt className="text-texto-apoyo">Estado</dt>
            <dd>
              <Insignia estado={evento.estado} />
            </dd>
            <dt className="text-texto-apoyo">Unidad</dt>
            <dd>{evento.unidad || '—'}</dd>
            <dt className="text-texto-apoyo">Día</dt>
            <dd>
              {etiquetaDia(evento.dia)} · {evento.desde}–{evento.hasta}
            </dd>
            <dt className="text-texto-apoyo">Cobro</dt>
            <dd>
              {formatearSoles(evento.total_cts)} · {evento.medio}
            </dd>
          </dl>
        )}
      </Modal>
    </>
  );
}
