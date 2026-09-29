import { api } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearSolesCorto, formatearPct, pctDe } from '../../lib/dinero.js';
import { mesDePeriodo, nombrePeriodo } from '../../lib/fechas.js';
import { ruta } from '../../lib/nav.jsx';
import { useEid, Guarda } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, FranjaKPI, SelectorPeriodo, ErrorCarga, Vacio, Esqueleto, Insignia, Icono, TONO_TEXTO } from '../../ui/index.js';

function variacion(pct, periodoAnterior, subirEsBueno) {
  if (pct == null || !periodoAnterior) return undefined;
  const signo = pct > 0 ? '+' : '';
  return { texto: `${signo}${formatearPct(pct, 1)} vs. ${mesDePeriodo(periodoAnterior)}`, buena: subirEsBueno ? pct >= 0 : pct <= 0 };
}

/** Icono de cada tarea de hoy según a dónde lleva (y si espera a la junta). */
function iconoTarea(p) {
  if (p.ruta === 'mantenimiento') return p.tono === 'alerta' ? 'junta' : 'herramienta';
  if (p.ruta === 'recibos') return 'voucher';
  if (p.ruta === 'medidores') return 'medidor';
  return 'reloj';
}

/** 03 · Dashboard del administrador (la junta lo ve igual, sin acciones). */
export default function Dashboard() {
  const eid = useEid();
  const [periodo, setPeriodo] = usePeriodo();
  const { datos: d, error, recargar } = useCarga(() => api.get(`/edificios/${eid}/dashboard`, { periodo }), [eid, periodo]);

  const acciones = (
    <>
      <SelectorPeriodo periodo={periodo} onCambio={setPeriodo} />
      <Guarda permiso="recibos.emitir">
        <Boton href={ruta('recibos', { periodo })} icono="recibo">
          Ir a recibos
        </Boton>
      </Guarda>
    </>
  );
  const secundarias = [{ etiqueta: 'Imprimir', icono: 'imprimir', onClick: () => window.print() }];

  const k = d?.kpis;
  const vacio = d && (!k || d.vacio || d.hay_datos === false);
  const tareas = d?.tareas || {};
  const pendientes = d?.pendientes || construirPendientes(tareas);
  const rubros = d?.egresos_por_rubro || [];
  const maxRubro = Math.max(1, ...rubros.map((r) => r.total_cts));
  const cob = d?.cobranza;
  const pctCobrado = cob ? pctDe(cob.cobrado_cts ?? k?.ingresos_cts, cob.emitido_cts ?? k?.emitido_cts) : 0;
  const va0 = d?.variacion_vs_mes_anterior || {};
  const va = { ...va0, periodo_anterior: va0.periodo_anterior || va0.periodo };
  const morosos = d?.morosidad_unidades || null;
  const cifra = (cts) => ({ valor: formatearSolesCorto(cts), valorCompleto: formatearSoles(cts) });

  return (
    <>
      <Encabezado titulo="Resumen del edificio" acciones={acciones} secundarias={secundarias} />
      <Contenido>
        {error ? (
          <ErrorCarga error={error} onReintentar={recargar} />
        ) : vacio ? (
          <Vacio titulo={`Aún no hay movimientos en ${mesDePeriodo(periodo)}`} texto="Empieza importando tus unidades desde Excel; luego abre el periodo y genera los recibos." icono="edificio">
            <Guarda permiso="unidades.importar">
              <Boton href={ruta('unidades', { tab: 'importar' })} icono="excel">
                Importar unidades
              </Boton>
            </Guarda>
          </Vacio>
        ) : (
          <>
            <FranjaKPI
              etiqueta={`Indicadores de ${mesDePeriodo(periodo)}`}
              cargando={!k}
              principal={{
                titulo: 'Morosidad',
                icono: 'moroso',
                tono: 'alerta',
                valor: k ? formatearPct(k.morosidad?.pct) : '',
                nota: k ? `${k.morosidad?.unidades} unidades · ${formatearSoles(k.morosidad?.monto_cts)} · histórica ${formatearPct(k.morosidad?.historica_pct ?? k.morosidad?.pct)}` : '',
                to: ruta('recibos', { periodo, estado: 'vencido' }),
              }}
              items={[
                { titulo: 'Ingresos cobrados', ...cifra(k?.ingresos_cts), nota: k?.emitido_cts ? `de ${formatearSolesCorto(k.emitido_cts)} emitidos` : 'lo cobrado', variacion: variacion(va.ingresos_pct, va.periodo_anterior, true), to: ruta('balance', { periodo, abrir: 'ing' }) },
                { titulo: 'Egresos', ...cifra(k?.egresos_cts), nota: k?.egresos_rubros ? `${k.egresos_rubros} rubros · ${k.egresos_documentos} doc.` : undefined, variacion: variacion(va.egresos_pct, va.periodo_anterior, false), to: ruta('balance', { periodo, abrir: 'egr' }) },
                { titulo: 'Saldo del mes', tono: 'acento', ...cifra(k?.saldo_cts), nota: 'ingresos − egresos', to: ruta('balance', { periodo }) },
                ...(k?.banco_cts != null || !k ? [{ titulo: 'Saldo en banco', ...cifra(k?.banco_cts), nota: 'con fondo de contingencia' }] : []),
                { titulo: 'Por reservas', ...cifra(d?.ingresos_reservas_cts ?? 0), nota: d?.reservas_mes ? `${d.reservas_mes.cantidad} reservas` : 'parrillas y otras áreas', to: ruta('reservas') },
              ]}
            />

            <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 lg:gap-5">
              <Seccion titulo={rubros.length || !d ? 'Egresos por rubro' : 'Mantenimiento del mes'} className="lg:col-span-2" extra={<a href={ruta('balance', { periodo, abrir: 'egr' })} className="text-sm">Ver balance por nodos</a>}>
                {!d ? (
                  <Esqueleto className="h-40 w-full" />
                ) : rubros.length === 0 ? (
                  <TrabajosMes trabajos={d.trabajos_mes || []} />
                ) : (
                  <div className="flex flex-col gap-2.5">
                    {rubros.map((r) => (
                      <a key={r.id || r.nombre} href={ruta('balance', { periodo, abrir: r.id })} className="grid grid-cols-[1fr_auto] items-center gap-x-4 gap-y-1 text-sm text-tinta hover:text-tinta sm:grid-cols-[200px_1fr_120px]">
                        <span className="truncate">{r.nombre}</span>
                        <span className="order-3 col-span-2 h-2 rounded-chip bg-superficie-2 sm:order-none sm:col-span-1">
                          <span className="crece-x block h-2 rounded-chip bg-serie-1" style={{ width: `${(r.total_cts / maxRubro) * 100}%` }} />
                        </span>
                        <span className="text-right font-semibold tabular-nums">{formatearSoles(r.total_cts)}</span>
                      </a>
                    ))}
                  </div>
                )}
                {d?.ingresos_por_concepto && (
                  <div className="grid grid-cols-1 gap-3 border-t border-borde pt-3 sm:grid-cols-3">
                    {d.ingresos_por_concepto.map((c) => (
                      <div key={c.nombre} className="flex flex-col gap-0.5">
                        <span className="text-xs text-texto-apoyo">{c.nombre}</span>
                        <span className="text-base font-semibold tabular-nums">{formatearSoles(c.total_cts)}</span>
                      </div>
                    ))}
                  </div>
                )}
              </Seccion>

              <div className="flex min-w-0 flex-col gap-4 lg:gap-5">
                <Seccion titulo={`Cobranza de ${mesDePeriodo(periodo)}`} extra={<a href={ruta('recibos', { periodo, estado: 'vencido' })} className="text-sm">Ver recibos vencidos</a>}>
                  {!cob ? (
                    <Esqueleto className="h-24 w-full" />
                  ) : (
                    <>
                      <div className="flex items-baseline justify-between gap-2 text-sm">
                        <span>
                          {cob.pagados} de {cob.emitidos} recibos pagados{cob.parciales ? ` · ${cob.parciales} parciales` : ''}
                        </span>
                        <span className="font-titulo text-lg font-semibold tabular-nums text-acento">{formatearPct(pctCobrado)}</span>
                      </div>
                      <div className="h-2 rounded-chip bg-alerta-pista" role="progressbar" aria-valuenow={pctCobrado} aria-valuemin={0} aria-valuemax={100} aria-label="Porcentaje cobrado">
                        <div className="crece-x h-2 rounded-chip bg-acento" style={{ width: `${pctCobrado}%` }} />
                      </div>
                      <dl className="grid grid-cols-3 gap-2 text-xs">
                        <div className="flex flex-col gap-0.5">
                          <dt className="text-texto-apoyo">Emitido</dt>
                          <dd className="text-sm font-semibold tabular-nums">{formatearSoles(cob.emitido_cts ?? k?.emitido_cts)}</dd>
                        </div>
                        <div className="flex flex-col gap-0.5">
                          <dt className="text-texto-apoyo">Cobrado</dt>
                          <dd className="text-sm font-semibold tabular-nums text-acento">{formatearSoles(cob.cobrado_cts ?? k?.ingresos_cts)}</dd>
                        </div>
                        <div className="flex flex-col gap-0.5">
                          <dt className="text-texto-apoyo">Por cobrar</dt>
                          <dd className="text-sm font-semibold tabular-nums text-alerta">{formatearSoles(cob.por_cobrar_cts ?? k?.morosidad?.monto_cts)}</dd>
                        </div>
                      </dl>
                      {/* Morosidad por unidad, fusionada aquí (antes una tarjeta casi vacía). */}
                      <div className="flex flex-col gap-1 border-t border-borde pt-3">
                        <span className="text-xs font-semibold text-texto-apoyo">Morosos</span>
                        {morosos && morosos.length > 0 ? (
                          <ul className="flex flex-col">
                            {morosos.slice(0, 3).map((m) => (
                              <li key={m.unidad} className="flex items-center justify-between gap-2 py-1 text-sm">
                                <span className="flex min-w-0 items-center gap-2">
                                  <Icono nombre="moroso" tam={14} className="text-alerta" />
                                  <b className="truncate">{m.unidad}</b>
                                  <span className="text-xs text-texto-apoyo">{m.detalle || `${m.meses} ${m.meses === 1 ? 'mes' : 'meses'}`}{m.reservas_bloqueadas ? ' · reservas bloqueadas' : ''}</span>
                                </span>
                                <span className="font-semibold tabular-nums text-alerta">{formatearSoles(m.deuda_cts)}</span>
                              </li>
                            ))}
                            {morosos.length > 3 && <li className="text-xs text-texto-apoyo">y {morosos.length - 3} más</li>}
                          </ul>
                        ) : (
                          <p className="text-sm text-texto-suave">
                            {k?.morosidad?.unidades ? `${k.morosidad.unidades} unidades con deuda por ${formatearSoles(k.morosidad.monto_cts)}.` : 'Ninguna unidad morosa. ¡Todo al día!'}
                          </p>
                        )}
                      </div>
                      <p className="text-xs text-texto-apoyo">Los ingresos cuentan lo cobrado; lo emitido y no cobrado es morosidad. Periodo: {nombrePeriodo(periodo)}.</p>
                    </>
                  )}
                </Seccion>

                <Seccion titulo="Tareas de hoy" extra={<span className="text-sm text-texto-apoyo">{pendientes.length} abiertas</span>}>
                  {!d ? (
                    <Esqueleto className="h-24 w-full" />
                  ) : pendientes.length === 0 ? (
                    <p className="flex items-center gap-2 text-sm text-texto-suave">
                      <Icono nombre="hecho" tam={16} className="text-acento" /> Nada pendiente. Así da gusto.
                    </p>
                  ) : (
                    <ul className="-mx-2 flex flex-col">
                      {pendientes.map((p, i) => (
                        <li key={i}>
                          <a href={ruta(p.ruta || 'inicio')} className="flex items-start gap-3 rounded-control px-2 py-2 text-tinta transition-colors duration-rapida hover:bg-fondo hover:text-tinta">
                            <Icono nombre={iconoTarea(p)} tam={18} className={`mt-0.5 ${TONO_TEXTO[p.tono] || TONO_TEXTO.neutro}`} />
                            <span className="flex min-w-0 flex-1 flex-col">
                              <span className="text-sm font-semibold">{p.titulo}</span>
                              <span className="text-xs text-texto-apoyo">{p.etiqueta?.charAt(0) + (p.etiqueta || '').slice(1).toLowerCase()}{p.detalle ? ` · ${p.detalle}` : ''}</span>
                            </span>
                            <Icono nombre="der" tam={16} className="mt-0.5 text-texto-apoyo" />
                          </a>
                        </li>
                      ))}
                    </ul>
                  )}
                </Seccion>
              </div>
            </div>
          </>
        )}
      </Contenido>
    </>
  );
}

function TrabajosMes({ trabajos }) {
  if (!trabajos.length)
    return (
      <p className="flex items-center gap-2 text-sm text-texto-suave">
        <Icono nombre="hecho" tam={16} className="text-acento" /> Sin trabajos este mes. Así da gusto.
      </p>
    );
  return (
    <ul className="flex flex-col divide-y divide-superficie-2">
      {trabajos.slice(0, 8).map((t) => (
        <li key={t.id} className="flex items-center justify-between gap-3 py-2 text-sm">
          <a href={ruta('mantenimiento', { q: t.codigo })} className="line-clamp-2 min-w-0 text-tinta hover:text-acento" title={`${t.codigo} ${t.titulo}`}>
            <b>{t.codigo}</b> {t.titulo}
          </a>
          <span className="flex shrink-0 items-center gap-2">
            {t.monto_presupuesto_cts ? <span className="tabular-nums text-texto-suave">{formatearSoles(t.monto_presupuesto_cts)}</span> : null}
            <Insignia estado={t.estado} />
          </span>
        </li>
      ))}
    </ul>
  );
}

/** Si el API solo manda contadores, se arma la lista de tareas con ellos. */
function construirPendientes(t) {
  const l = [];
  if (t.aprobaciones_pendientes) l.push({ tono: 'alerta', etiqueta: 'ESPERA A LA JUNTA', titulo: `${t.aprobaciones_pendientes} trabajo(s) esperando aprobación`, ruta: 'mantenimiento' });
  if (t.vouchers_por_validar) l.push({ tono: 'aviso', etiqueta: 'VOUCHERS', titulo: `${t.vouchers_por_validar} voucher(s) por validar`, ruta: 'recibos' });
  if (t.incidencias_por_validar) l.push({ tono: 'neutro', etiqueta: 'INCIDENCIAS', titulo: `${t.incidencias_por_validar} incidencia(s) por validar`, ruta: 'mantenimiento' });
  if (t.lecturas_pendientes) l.push({ tono: 'neutro', etiqueta: 'LECTURAS', titulo: `${t.lecturas_pendientes} lecturas de agua pendientes`, ruta: 'medidores' });
  return l;
}
