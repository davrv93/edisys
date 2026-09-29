import { api } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearPct, formatearNumero } from '../../lib/dinero.js';
import { esPeriodo, nombrePeriodo, periodoActual, sumarMeses } from '../../lib/fechas.js';
import { COLUMNAS } from '../../lib/kanban.js';
import { useQuery } from '../../lib/nav.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, ErrorCarga, Esqueleto, TarjetaKPI, Vacio } from '../../ui/index.js';
import { BarrasAgrupadas, BarrasH, Leyenda, Linea, TablaDatos } from './Graficos.jsx';

const cortoSoles = (cts, eje) => (eje ? (cts === 0 ? 'S/ 0' : `S/ ${formatearNumero(cts / 100000, cts % 100000 ? 1 : 0)} mil` ) : formatearSoles(cts));
const etiquetaMes = (d, corto) => (corto ? nombrePeriodo(d.periodo, { corto: true }).slice(0, 3) : nombrePeriodo(d.periodo));
const ETIQUETA_ESTADO = Object.fromEntries(COLUMNAS.map((c) => [c.estado, c.etiqueta]));

/** 13 · Analítica: cobranza, morosidad, agua, reservas, incidencias y tiempo de resolución, con filtro de rango (en la URL). */
export default function Analitica() {
  const [q, setQuery] = useQuery();
  const hasta = esPeriodo(q.get('hasta')) ? q.get('hasta') : periodoActual();
  const desde = esPeriodo(q.get('desde')) ? q.get('desde') : sumarMeses(hasta, -11);
  const { datos: d, error, recargar } = useCarga(() => api.get('/analitica/resumen', { desde, hasta }), [desde, hasta]);

  const rango = (meses) => setQuery({ desde: sumarMeses(hasta, -(meses - 1)), hasta }, { reemplazar: true });
  const cob = d?.cobranza_mensual || [];
  const mor = d?.morosidad_mensual || [];
  const agua = [...(d?.consumo_agua || [])].sort((a, b) => b.m3 - a.m3);
  const mediaAgua = agua.length ? agua.reduce((a, x) => a + x.m3, 0) / agua.length : 0;
  const reservas = d?.reservas_por_area || [];
  const inc = d?.incidencias_por_estado || [];
  const totEmitido = cob.reduce((a, x) => a + x.emitido, 0);
  const totCobrado = cob.reduce((a, x) => a + x.cobrado, 0);

  const filtro = (
    <div className="flex flex-wrap items-end gap-2" role="group" aria-label="Rango de fechas">
      <label className="flex flex-col gap-1 text-xs font-semibold">
        Desde
        <input type="month" value={desde} max={hasta} onChange={(e) => esPeriodo(e.target.value) && setQuery({ desde: e.target.value }, { reemplazar: true })} className="h-11 rounded-control border border-borde-fuerte bg-superficie px-2 text-sm lg:h-10" />
      </label>
      <label className="flex flex-col gap-1 text-xs font-semibold">
        Hasta
        <input type="month" value={hasta} min={desde} onChange={(e) => esPeriodo(e.target.value) && setQuery({ hasta: e.target.value }, { reemplazar: true })} className="h-11 rounded-control border border-borde-fuerte bg-superficie px-2 text-sm lg:h-10" />
      </label>
      {[3, 6, 12].map((n) => (
        <Boton key={n} variante="secundario" tamano="sm" onClick={() => rango(n)}>
          {n} meses
        </Boton>
      ))}
    </div>
  );

  return (
    <>
      <Encabezado titulo="Analítica" subtitulo={`${nombrePeriodo(desde)} – ${nombrePeriodo(hasta)}`} acciones={filtro} />
      <Contenido>
        {error ? (
          <ErrorCarga error={error} onReintentar={recargar} />
        ) : !d ? (
          <div className="grid gap-4 lg:grid-cols-2">
            {Array.from({ length: 4 }, (_, i) => (
              <Esqueleto key={i} className="h-64 w-full" />
            ))}
          </div>
        ) : (
          <>
            <div className="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
              <TarjetaKPI titulo="Emitido en el rango" valor={formatearSoles(totEmitido, { sinDecimales: true })} />
              <TarjetaKPI tono="acento" titulo="Cobrado en el rango" valor={formatearSoles(totCobrado, { sinDecimales: true })} nota={totEmitido ? `${formatearPct((totCobrado / totEmitido) * 100)} de lo emitido` : undefined} />
              <TarjetaKPI tono="alerta" titulo="Morosidad del último mes" valor={mor.length ? formatearPct(mor[mor.length - 1].pct) : '—'} />
              <TarjetaKPI titulo="Tiempo de resolución" valor={d.tiempo_resolucion_dias != null ? `${formatearNumero(d.tiempo_resolucion_dias, 1)} días` : '—'} nota="promedio de reporte a terminado" />
            </div>

            <div className="grid gap-4 lg:grid-cols-2 lg:gap-6">
              <Seccion titulo="Cobranza: emitido vs. cobrado">
                {cob.length ? (
                  <>
                    <Leyenda items={[{ texto: 'Emitido', clase: 'bg-borde-fuerte' }, { texto: 'Cobrado', clase: 'bg-acento' }]} />
                    <BarrasAgrupadas
                      titulo="Emitido y cobrado por mes"
                      datos={cob}
                      series={[
                        { clave: 'emitido', nombre: 'Emitido', claseSvg: 'fill-borde-fuerte' },
                        { clave: 'cobrado', nombre: 'Cobrado', claseSvg: 'fill-acento' },
                      ]}
                      formato={cortoSoles}
                      etiquetaX={etiquetaMes}
                    />
                    <TablaDatos
                      columnas={[
                        { titulo: 'Mes', valor: (f) => nombrePeriodo(f.periodo) },
                        { titulo: 'Emitido', der: true, valor: (f) => formatearSoles(f.emitido) },
                        { titulo: 'Cobrado', der: true, valor: (f) => formatearSoles(f.cobrado) },
                      ]}
                      filas={cob}
                    />
                  </>
                ) : (
                  <Vacio titulo="Sin datos en el rango" compacto />
                )}
              </Seccion>

              <Seccion titulo="Morosidad mensual (%)">
                {mor.length ? (
                  <>
                    <Linea titulo="Morosidad por mes" datos={mor} clave="pct" formato={(v, eje) => formatearPct(v, eje ? 0 : 1)} etiquetaX={etiquetaMes} />
                    <TablaDatos
                      columnas={[
                        { titulo: 'Mes', valor: (f) => nombrePeriodo(f.periodo) },
                        { titulo: 'Morosidad', der: true, valor: (f) => formatearPct(f.pct) },
                      ]}
                      filas={mor}
                    />
                  </>
                ) : (
                  <Vacio titulo="Sin datos en el rango" compacto />
                )}
              </Seccion>

              <Seccion titulo="Consumo de agua por unidad (m³)" extra={<span className="text-xs text-texto-apoyo">media {formatearNumero(mediaAgua, 1)} m³ · ámbar = más del doble</span>}>
                {agua.length ? (
                  <div className="max-h-[420px] overflow-y-auto pr-1">
                    <BarrasH titulo="Consumo por unidad" datos={agua} etiqueta={(x) => x.unidad} valor={(x) => x.m3} formato={(v) => formatearNumero(v, 1)} resaltar={(x) => x.m3 > 2 * mediaAgua} />
                  </div>
                ) : (
                  <Vacio titulo="Sin lecturas" compacto />
                )}
              </Seccion>

              <div className="flex flex-col gap-4 lg:gap-6">
                <Seccion titulo="Reservas por área">
                  {reservas.length ? (
                    <>
                      <BarrasH titulo="Reservas por área" datos={reservas} etiqueta={(x) => x.area} valor={(x) => x.cantidad} formato={(v) => `${v}`} />
                      <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-texto-apoyo">
                        {reservas.map((r) => (
                          <li key={r.area}>
                            {r.area}: {r.ingreso ? formatearSoles(r.ingreso) : 'sin costo'}
                          </li>
                        ))}
                      </ul>
                    </>
                  ) : (
                    <Vacio titulo="Sin reservas" compacto />
                  )}
                </Seccion>
                <Seccion titulo="Incidencias por estado">
                  {inc.length ? <BarrasH titulo="Incidencias por estado" datos={inc} etiqueta={(x) => ETIQUETA_ESTADO[x.estado] || x.estado} valor={(x) => x.cantidad} formato={(v) => `${v}`} /> : <Vacio titulo="Sin incidencias" compacto />}
                </Seccion>
              </div>
            </div>
          </>
        )}
      </Contenido>
    </>
  );
}
