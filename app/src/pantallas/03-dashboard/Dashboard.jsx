import { api } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearPct, pctDe } from '../../lib/dinero.js';
import { mesDePeriodo, nombrePeriodo } from '../../lib/fechas.js';
import { ruta } from '../../lib/nav.jsx';
import { useEid, Guarda } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, TarjetaKPI, SelectorPeriodo, ErrorCarga, Vacio, Esqueleto, Insignia } from '../../ui/index.js';

function variacion(pct, periodoAnterior, subirEsBueno) {
  if (pct == null || !periodoAnterior) return undefined;
  const signo = pct > 0 ? '+' : '';
  return { texto: `${signo}${formatearPct(pct, 1)} vs. ${mesDePeriodo(periodoAnterior)}`, buena: subirEsBueno ? pct >= 0 : pct <= 0 };
}

const TONO_PENDIENTE = {
  alerta: 'bg-alerta-suave border-alerta-borde',
  aviso: 'bg-aviso-suave border-aviso-borde',
  neutro: 'bg-superficie border-borde',
};
const TONO_ETIQUETA = { alerta: 'text-alerta', aviso: 'text-aviso', neutro: 'text-texto-suave' };

/** 03 · Dashboard del administrador (la junta lo ve igual, sin acciones). */
export default function Dashboard() {
  const eid = useEid();
  const [periodo, setPeriodo] = usePeriodo();
  const { datos: d, error, cargando, recargar } = useCarga(() => api.get(`/edificios/${eid}/dashboard`, { periodo }), [eid, periodo]);

  const acciones = (
    <>
      <SelectorPeriodo periodo={periodo} onCambio={setPeriodo} />
      <Boton variante="secundario" icono="descargar" onClick={() => window.print()} className="hidden sm:inline-flex">
        Imprimir
      </Boton>
      <Guarda permiso="recibos.emitir">
        <Boton href={ruta('recibos', { periodo })} icono="recibo">
          Ir a recibos
        </Boton>
      </Guarda>
    </>
  );

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

  return (
    <>
      <Encabezado titulo="Resumen del edificio" acciones={acciones} />
      <Contenido>
        {error ? (
          <ErrorCarga error={error} onReintentar={recargar} />
        ) : vacio ? (
          <Vacio titulo={`Aún no hay movimientos en ${mesDePeriodo(periodo)}`} texto="Empieza importando tus unidades desde Excel; luego abre el periodo y genera los recibos." icono="edificio">
            <Guarda permiso="unidades.importar">
              <Boton href={ruta('unidades', { tab: 'importar' })} icono="subir">
                Importar unidades
              </Boton>
            </Guarda>
          </Vacio>
        ) : (
          <>
            <div className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:gap-4 xl:grid-cols-6" aria-busy={cargando}>
              <TarjetaKPI
                cargando={!k}
                titulo="Ingresos cobrados"
                valor={formatearSoles(k?.ingresos_cts)}
                nota={k?.emitido_cts ? `de ${formatearSoles(k.emitido_cts)} emitidos` : 'lo cobrado, no lo emitido'}
                variacion={variacion(va.ingresos_pct, va.periodo_anterior, true)}
                to={ruta('balance', { periodo, abrir: 'ing' })}
              />
              <TarjetaKPI
                cargando={!k}
                titulo="Egresos"
                valor={formatearSoles(k?.egresos_cts)}
                nota={k?.egresos_rubros ? `${k.egresos_rubros} rubros · ${k.egresos_documentos} documentos` : undefined}
                variacion={variacion(va.egresos_pct, va.periodo_anterior, false)}
                to={ruta('balance', { periodo, abrir: 'egr' })}
              />
              <TarjetaKPI cargando={!k} tono="acento" titulo="Saldo del mes" valor={formatearSoles(k?.saldo_cts)} nota="ingresos − egresos" to={ruta('balance', { periodo })} />
              {(k?.banco_cts != null || !k) && <TarjetaKPI cargando={!k} titulo="Saldo en banco" valor={formatearSoles(k?.banco_cts)} nota="incluye fondo de contingencia" />}
              <TarjetaKPI
                cargando={!k}
                tono="alerta"
                titulo="Morosidad del mes"
                valor={k ? formatearPct(k.morosidad?.pct) : ''}
                nota={k ? `${k.morosidad?.unidades} unidades · ${formatearSoles(k.morosidad?.monto_cts)} · histórica ${formatearPct(k.morosidad?.historica_pct ?? k.morosidad?.pct)}` : ''}
                to={ruta('recibos', { periodo, estado: 'vencido' })}
              />
              <TarjetaKPI cargando={!k} titulo="Ingresos por reservas" valor={formatearSoles(d?.ingresos_reservas_cts ?? 0)} nota={d?.reservas_mes ? `${d.reservas_mes.cantidad} reservas · ${d.reservas_mes.detalle}` : 'parrillas y otras áreas'} to={ruta('reservas')} />
            </div>

            <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 lg:gap-6">
              <Seccion titulo={rubros.length || !d ? 'Egresos por rubro' : 'Mantenimiento del mes'} className="lg:col-span-2" extra={<a href={ruta('balance', { periodo, abrir: 'egr' })} className="text-sm">Ver balance por nodos</a>}>
                {!d ? (
                  <Esqueleto className="h-40 w-full" />
                ) : rubros.length === 0 ? (
                  <TrabajosMes trabajos={d.trabajos_mes || []} />
                ) : (
                  <div className="flex flex-col gap-3">
                    {rubros.map((r) => (
                      <a key={r.id || r.nombre} href={ruta('balance', { periodo, abrir: r.id })} className="grid grid-cols-[1fr_auto] items-center gap-x-4 gap-y-1 text-sm text-tinta hover:text-tinta sm:grid-cols-[200px_1fr_120px]">
                        <span className="truncate">{r.nombre}</span>
                        <span className="order-3 col-span-2 h-2.5 rounded-full bg-superficie-2 sm:order-none sm:col-span-1">
                          <span className="block h-2.5 rounded-full bg-acento" style={{ width: `${(r.total_cts / maxRubro) * 100}%` }} />
                        </span>
                        <span className="text-right font-semibold tabular-nums">{formatearSoles(r.total_cts)}</span>
                      </a>
                    ))}
                  </div>
                )}
                {d?.ingresos_por_concepto && (
                  <div className="grid grid-cols-1 gap-3 border-t border-borde pt-4 sm:grid-cols-3">
                    {d.ingresos_por_concepto.map((c) => (
                      <div key={c.nombre} className="flex flex-col gap-0.5">
                        <span className="text-xs text-texto-apoyo">{c.nombre}</span>
                        <span className="text-base font-semibold tabular-nums">{formatearSoles(c.total_cts)}</span>
                      </div>
                    ))}
                  </div>
                )}
              </Seccion>

              <Seccion titulo="Tareas de hoy" extra={<span className="text-sm text-texto-apoyo">{pendientes.length} abiertos</span>}>
                {!d ? (
                  <Esqueleto className="h-40 w-full" />
                ) : pendientes.length === 0 ? (
                  <p className="text-base text-texto-suave">Nada pendiente. Así da gusto.</p>
                ) : (
                  <ul className="flex flex-col gap-3">
                    {pendientes.map((p, i) => (
                      <li key={i}>
                        <a href={ruta(p.ruta || 'inicio')} className={`flex flex-col gap-1 rounded-lg border p-3 text-tinta hover:text-tinta hover:shadow-sm ${TONO_PENDIENTE[p.tono] || TONO_PENDIENTE.neutro}`}>
                          <span className={`text-xs font-bold ${TONO_ETIQUETA[p.tono] || TONO_ETIQUETA.neutro}`}>{p.etiqueta}</span>
                          <span className="text-sm font-semibold">{p.titulo}</span>
                          {p.detalle && <span className="text-xs text-texto-suave">{p.detalle}</span>}
                        </a>
                      </li>
                    ))}
                  </ul>
                )}
              </Seccion>
            </div>

            <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:gap-6">
              <Seccion titulo="Morosidad por unidad" extra={<a href={ruta('recibos', { periodo, estado: 'vencido' })} className="text-sm">Ver recibos</a>}>
                {!d ? (
                  <Esqueleto className="h-32 w-full" />
                ) : !d.morosidad_unidades ? (
                  <p className="text-base text-texto-suave">
                    {k?.morosidad?.unidades ? `${k.morosidad.unidades} unidades con deuda por ${formatearSoles(k.morosidad.monto_cts)}. ` : 'Ninguna unidad morosa. '}
                    <a href={ruta('recibos', { periodo, estado: 'vencido' })}>Ver recibos vencidos</a>
                  </p>
                ) : !d.morosidad_unidades.length ? (
                  <p className="text-base text-texto-suave">Ninguna unidad morosa. ¡Todo al día!</p>
                ) : (
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="text-left text-xs text-texto-apoyo">
                        <th className="pb-2 font-semibold">Unidad</th>
                        <th className="pb-2 font-semibold">Meses</th>
                        <th className="hidden pb-2 font-semibold sm:table-cell">Reservas</th>
                        <th className="pb-2 text-right font-semibold">Deuda</th>
                      </tr>
                    </thead>
                    <tbody>
                      {d.morosidad_unidades.map((m) => (
                        <tr key={m.unidad} className="border-t border-superficie-2">
                          <td className="py-3 font-semibold">{m.unidad}</td>
                          <td className="py-3">
                            <Insignia estado={m.meses > 1 ? 'vencido' : 'pendiente'} texto={m.detalle || `${m.meses} ${m.meses === 1 ? 'mes' : 'meses'}`} />
                          </td>
                          <td className="hidden py-3 text-alerta sm:table-cell">{m.reservas_bloqueadas ? 'Bloqueadas' : '—'}</td>
                          <td className="py-3 text-right font-semibold tabular-nums">{formatearSoles(m.deuda_cts)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </Seccion>

              <Seccion titulo={`Cobranza de ${mesDePeriodo(periodo)}`}>
                {!cob ? (
                  <Esqueleto className="h-32 w-full" />
                ) : (
                  <>
                    <div className="flex justify-between text-sm">
                      <span>
                        {cob.pagados} de {cob.emitidos} recibos pagados{cob.parciales ? ` · ${cob.parciales} parciales` : ''}
                      </span>
                      <span className="font-semibold tabular-nums">{formatearPct(pctCobrado)}</span>
                    </div>
                    <div className="h-2.5 rounded-full bg-alerta-pista" role="progressbar" aria-valuenow={pctCobrado} aria-valuemin={0} aria-valuemax={100} aria-label="Porcentaje cobrado">
                      <div className="h-2.5 rounded-full bg-acento" style={{ width: `${pctCobrado}%` }} />
                    </div>
                    <div className="grid grid-cols-3 gap-3 text-sm">
                      <div className="flex flex-col gap-0.5">
                        <span className="text-texto-apoyo">Emitido</span>
                        <span className="font-semibold tabular-nums">{formatearSoles(cob.emitido_cts ?? k?.emitido_cts)}</span>
                      </div>
                      <div className="flex flex-col gap-0.5">
                        <span className="text-texto-apoyo">Cobrado</span>
                        <span className="font-semibold tabular-nums text-acento">{formatearSoles(cob.cobrado_cts ?? k?.ingresos_cts)}</span>
                      </div>
                      <div className="flex flex-col gap-0.5">
                        <span className="text-texto-apoyo">Por cobrar</span>
                        <span className="font-semibold tabular-nums text-alerta">{formatearSoles(cob.por_cobrar_cts ?? k?.morosidad?.monto_cts)}</span>
                      </div>
                    </div>
                    <p className="text-xs text-texto-apoyo">Los ingresos cuentan lo cobrado; lo emitido y no cobrado es morosidad. Periodo: {nombrePeriodo(periodo)}.</p>
                  </>
                )}
              </Seccion>
            </div>
          </>
        )}
      </Contenido>
    </>
  );
}

function TrabajosMes({ trabajos }) {
  if (!trabajos.length) return <p className="text-base text-texto-suave">Sin trabajos este mes. Así da gusto.</p>;
  return (
    <ul className="flex flex-col divide-y divide-superficie-2">
      {trabajos.slice(0, 8).map((t) => (
        <li key={t.id} className="flex items-center justify-between gap-3 py-2 text-sm">
          <span className="min-w-0 truncate">
            <b>{t.codigo}</b> {t.titulo}
          </span>
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
