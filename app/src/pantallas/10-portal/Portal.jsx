import { api } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearPct } from '../../lib/dinero.js';
import { etiquetaDia, formatearDiaMes, formatearFecha, formatearHora, diaLima, mesDePeriodo, nombrePeriodo, sumarMeses } from '../../lib/fechas.js';
import { ruta } from '../../lib/nav.jsx';
import { useEid, useSesion, Guarda } from '../../layout/Sesion.jsx';
import { useModoTarea } from '../../layout/Armazon.jsx';
import { Boton, ErrorCarga, Esqueleto, Icono, Insignia } from '../../ui/index.js';
import { nombreUnidad } from '../../lib/unidad.js';

const PASOS = ['reportado', 'validado', 'presupuestado', 'aprobado', 'en_ejecucion', 'terminado'];
const TEXTO_PASO = {
  reportado: 'Reportado · la administración lo revisará',
  validado: 'Validado · se está preparando el informe y el costo',
  presupuestado: 'Con presupuesto · esperando aprobación',
  aprobado: 'Aprobado · pronto empieza el trabajo',
  en_ejecucion: 'En ejecución',
  terminado: 'Terminado',
  rechazado: 'No aprobado por la junta',
  descartado: 'Descartado por la administración',
};

/** 10 · Portal del propietario (móvil primero): cuánto debo, pagar, transparencia, reservar y reportar. */
export default function Portal() {
  useModoTarea(true, 'cabecera');
  const eid = useEid();
  const s = useSesion();
  const { datos: d, error, recargar } = useCarga(() => api.get(`/edificios/${eid}/portal`), [eid]);
  const nombre = s.usuario.nombre.split(' ')[0];
  const unidad = d?.unidades?.[0];
  const r = d?.recibo_actual;
  const deuda = d?.deuda?.total_cts || 0;
  const pago = r?.pagos?.find((p) => p.estado === 'validado');
  const k = d?.kpis_edificio;
  const reservaPendiente = d?.proximas_reservas?.find((x) => x.estado === 'pendiente_pago');
  const verRecibos = s.tiene(['recibos.ver', 'portal.ver']) && s.rol !== 'inquilino';

  let estadoCuenta;
  if (!d) estadoCuenta = null;
  else if (!r) estadoCuenta = { titulo: 'Sin recibos aún', tono: 'text-texto-claro', detalle: `Tu primer recibo llega el 1 de ${mesDePeriodo(sumarMeses(s.edificio.periodo_abierto || '2026-09', 1))}.` };
  else if (deuda > 0)
    estadoCuenta = {
      titulo: `Debes ${formatearSoles(deuda)}`,
      tono: 'text-alerta-borde',
      detalle: `${nombrePeriodo(r.periodo)} · vence el ${formatearDiaMes(r.vence)}`,
    };
  else if (d.pago_en_revision || r.pagos_en_revision > 0) estadoCuenta = { titulo: 'Pago en revisión', tono: 'text-aviso-borde', detalle: 'La administración está validando tu voucher.' };
  else estadoCuenta = { titulo: 'Al día', tono: 'text-acento-oscuro', detalle: `${mesDePeriodo(r.periodo).replace(/^./, (c) => c.toUpperCase())} ${formatearSoles(r.total_cts)}${pago ? ` · pagado ${formatearDiaMes(pago.fecha)}` : ''}` };

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col lg:gap-6 lg:p-8">
      <header className="flex flex-col gap-4 bg-tinta px-4 pb-4 pt-5 text-white lg:rounded-xl lg:p-6">
        <div className="flex items-center justify-between gap-3">
          <div className="flex flex-col">
            <span className="text-xs text-texto-tenue">
              {s.edificio.nombre}
              {unidad ? ` · ${unidad.nombre || `Dpto ${unidad.codigo}`}` : ''}
            </span>
            <span className="font-titulo text-2xl font-semibold">Hola, {nombre}</span>
          </div>
          <span className="flex h-10 w-10 items-center justify-center rounded-full bg-superficie-oscura-2 text-sm font-semibold">{s.usuario.iniciales}</span>
        </div>
        {verRecibos && (
          <div className="flex items-center justify-between gap-3 rounded-xl bg-superficie-oscura p-4">
            {!estadoCuenta ? (
              <Esqueleto className="h-14 w-2/3 bg-superficie-oscura-2" />
            ) : (
              <div className="flex min-w-0 flex-col gap-0.5">
                <span className="text-xs text-texto-tenue">Tu estado de cuenta</span>
                <span className={`text-lg font-semibold ${estadoCuenta.tono}`}>{estadoCuenta.titulo}</span>
                <span className="text-xs text-texto-claro">{estadoCuenta.detalle}</span>
              </div>
            )}
            {r && (
              <a href={ruta('recibos', deuda > 0 ? { id: r.id, pagar: 1 } : { id: r.id })} className="flex h-11 shrink-0 items-center rounded-lg bg-superficie px-4 text-sm font-semibold text-tinta hover:text-tinta">
                {deuda > 0 ? 'Pagar' : 'Ver recibo'}
              </a>
            )}
          </div>
        )}
      </header>

      <div className="flex flex-col gap-4 p-4 lg:p-0">
        {error && <ErrorCarga error={error} onReintentar={recargar} compacto />}

        {reservaPendiente && (
          <a href={ruta('reservas', { nueva: 1 })} className="flex items-center justify-between gap-3 rounded-xl border border-aviso-borde bg-aviso-suave p-4 text-sm text-aviso-texto hover:text-aviso-texto">
            <span className="flex flex-col">
              <b>
                {reservaPendiente.recurso || 'Reserva'} · {etiquetaDia(diaLima(reservaPendiente.inicio))}, {formatearHora(reservaPendiente.inicio)}
              </b>
              <span>Falta pagar {formatearSoles(reservaPendiente.total_cts)}</span>
            </span>
            <span className="font-semibold">Pagar</span>
          </a>
        )}

        {deuda > 0 && (d?.deuda?.meses?.length > 0 || d?.deuda?.por_unidad?.length > 0) && (
          <section className="flex flex-col gap-2 rounded-xl border border-alerta-borde bg-alerta-suave p-4 text-alerta-texto">
            <h2 className="text-base font-semibold">Detalle de tu deuda</h2>
            {(d.deuda.meses || []).map((m) => (
              <div key={m.periodo} className="flex justify-between text-base">
                <span>{nombrePeriodo(m.periodo)}</span>
                <b className="tabular-nums">{formatearSoles(m.saldo_cts)}</b>
              </div>
            ))}
            {!d.deuda.meses &&
              d.deuda.por_unidad.flatMap((u) =>
                Array.isArray(u.meses) && u.meses.length
                  ? u.meses.map((m) => (
                      <div key={`${u.unidad_id ?? u.unidad}-${m.periodo}`} className="flex justify-between text-base">
                        <span>
                          {nombreUnidad(u.codigo || u.unidad)} · {nombrePeriodo(m.periodo)}
                        </span>
                        <b className="tabular-nums">{formatearSoles(m.saldo_cts)}</b>
                      </div>
                    ))
                  : [
                      <div key={u.unidad_id ?? u.unidad} className="flex justify-between text-base">
                        <span>{nombreUnidad(u.codigo || u.unidad)}</span>
                        <b className="tabular-nums">{formatearSoles(u.deuda_cts)}</b>
                      </div>,
                    ],
              )}
            <span className="text-sm">Mientras haya un recibo vencido no se pueden reservar áreas comunes.</span>
          </section>
        )}

        <div className="grid grid-cols-2 gap-3">
          <Guarda permiso="reservas.crear">
            <Boton href={ruta('reservas', { nueva: 1 })} tamano="lg" icono="calendario">
              Reservar
            </Boton>
          </Guarda>
          <Guarda permiso="incidencias.reportar">
            <Boton href={ruta('mantenimiento', { reportar: 1 })} tamano="lg" variante="secundario" icono="camara">
              Reportar
            </Boton>
          </Guarda>
        </div>

        {k && (
          <section className="flex flex-col gap-3">
            <div className="flex items-baseline justify-between">
              <h2 className="text-base font-semibold">Balance de {mesDePeriodo(d.periodo_kpis || s.edificio.periodo_abierto || r?.periodo || '')}</h2>
              <Guarda permiso="balance.ver">
                <a href={ruta('balance')} className="text-sm">
                  Ver detalle
                </a>
              </Guarda>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <MiniKpi titulo="Ingresos" valor={formatearSoles(k.ingresos_cts, { sinDecimales: true })} />
              <MiniKpi titulo="Egresos" valor={formatearSoles(k.egresos_cts, { sinDecimales: true })} />
              <MiniKpi titulo="Saldo del mes" valor={formatearSoles(k.saldo_cts, { sinDecimales: true })} tono="acento" />
              <MiniKpi titulo="Morosidad" valor={formatearPct(k.morosidad?.pct)} tono="alerta" />
            </div>
          </section>
        )}

        {d?.mis_incidencias?.map((inc) => {
          const i = PASOS.indexOf(inc.estado);
          return (
            <section key={inc.id} className="flex flex-col gap-2 rounded-xl border border-borde bg-superficie p-4">
              <span className="text-xs text-texto-apoyo">Tu reporte {inc.codigo}</span>
              <b className="text-base">{inc.titulo}</b>
              <div className="flex gap-1" aria-label={`Estado: ${TEXTO_PASO[inc.estado]}`}>
                {PASOS.map((p, j) => (
                  <span key={p} className={`h-1.5 flex-1 rounded-full ${inc.estado === 'rechazado' ? 'bg-alerta-borde' : j <= i ? 'bg-acento' : 'bg-borde'}`} />
                ))}
              </div>
              <span className="text-sm text-texto-suave">{TEXTO_PASO[inc.estado] || inc.estado}</span>
            </section>
          );
        })}

        {d?.proximas_reservas?.length > 0 && (
          <section className="flex flex-col gap-2 rounded-xl border border-borde bg-superficie p-4">
            <h2 className="text-base font-semibold">Mis reservas</h2>
            {d.proximas_reservas.map((x) => (
              <div key={x.id} className="flex items-center justify-between gap-2 border-t border-superficie-2 pt-2 text-sm first:border-0 first:pt-0">
                <span>
                  <b>{x.codigo}</b> · {x.recurso} · {etiquetaDia(diaLima(x.inicio))} {formatearHora(x.inicio)}
                </span>
                <Insignia estado={x.estado} />
              </div>
            ))}
          </section>
        )}

        {d?.trabajos_mes?.length > 0 && (
          <section className="flex flex-col gap-2 rounded-xl border border-borde bg-superficie p-4">
            <h2 className="text-base font-semibold">Mantenimiento del mes</h2>
            {d.trabajos_mes.map((t) => (
              <div key={t.id} className="flex items-center justify-between gap-2 text-sm">
                <span className="min-w-0 truncate">
                  {t.codigo} · {t.titulo}
                </span>
                <Insignia estado={t.estado} />
              </div>
            ))}
          </section>
        )}

        <section className="flex items-center justify-between gap-3 rounded-xl border border-borde bg-superficie p-4">
          <span className="flex items-center gap-3">
            <Icono nombre="documento" className="text-acento" />
            <span className="flex flex-col">
              <b className="text-base">Normas del edificio</b>
              <span className="text-sm text-texto-apoyo">Manual de convivencia y reglamento de áreas</span>
            </span>
          </span>
          {d?.normas_url ? (
            <a href={d.normas_url} target="_blank" rel="noopener" className="text-sm font-semibold">
              Abrir
            </a>
          ) : (
            <span className="text-xs text-texto-apoyo">Pronto</span>
          )}
        </section>
        {r && <p className="text-center text-xs text-texto-apoyo">Último recibo: {r.numero} · emitido {formatearFecha(r.emitido)}</p>}
      </div>
    </div>
  );
}

function MiniKpi({ titulo, valor, tono }) {
  const cajas = { acento: 'bg-acento-suave border-acento-borde', alerta: 'bg-alerta-suave border-alerta-borde' };
  const titulos = { acento: 'text-acento-hover', alerta: 'text-alerta-texto' };
  const valores = { acento: 'text-acento', alerta: 'text-alerta' };
  return (
    <div className={`flex flex-col gap-1 rounded-xl border p-4 ${cajas[tono] || 'bg-superficie border-borde'}`}>
      <span className={`text-xs ${titulos[tono] || 'text-texto-apoyo'}`}>{titulo}</span>
      <span className={`font-titulo text-2xl font-semibold tabular-nums ${valores[tono] || ''}`}>{valor}</span>
    </div>
  );
}
