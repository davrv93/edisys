import { api } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearPct, formatearNumero } from '../../lib/dinero.js';
import { esPeriodo, nombrePeriodo, periodoActual, sumarMeses } from '../../lib/fechas.js';
import { COLUMNAS } from '../../lib/kanban.js';
import { useQuery } from '../../lib/nav.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Chip, ErrorCarga, Esqueleto, FranjaKPI, SelectorMes, TONO_PUNTO, Vacio, tonoDe } from '../../ui/index.js';
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

  const meses = (() => {
    const [a1, m1] = desde.split('-').map(Number);
    const [a2, m2] = hasta.split('-').map(Number);
    return (a2 - a1) * 12 + (m2 - m1) + 1;
  })();
  const filtro = (
    <div className="flex items-center gap-1.5" role="group" aria-label="Rango de fechas">
      <SelectorMes etiqueta="Desde" valor={desde} max={hasta} onCambio={(v) => setQuery({ desde: v }, { reemplazar: true })} />
      <span className="text-texto-apoyo" aria-hidden="true">
        –
      </span>
      <SelectorMes etiqueta="Hasta" valor={hasta} min={desde} onCambio={(v) => setQuery({ hasta: v }, { reemplazar: true })} />
      {[3, 6, 12].map((n) => (
        <Chip key={n} activo={meses === n} onClick={() => rango(n)}>
          {n} m
        </Chip>
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
            <FranjaKPI
              etiqueta="Indicadores del rango"
              principal={{ titulo: 'Cobrado en el rango', tono: 'acento', valor: formatearSoles(totCobrado, { sinDecimales: true }), nota: totEmitido ? `${formatearPct((totCobrado / totEmitido) * 100)} de lo emitido` : undefined }}
              items={[
                { titulo: 'Emitido en el rango', valor: formatearSoles(totEmitido, { sinDecimales: true }) },
                { titulo: 'Morosidad del último mes', tono: 'alerta', valor: mor.length ? formatearPct(mor[mor.length - 1].pct) : '—' },
                { titulo: 'Tiempo de resolución', valor: d.tiempo_resolucion_dias != null ? `${formatearNumero(d.tiempo_resolucion_dias, 1)} días` : '—', nota: 'de reporte a terminado' },
              ]}
            />

            <div className="grid gap-4 lg:grid-cols-2 lg:gap-5">
              <Seccion titulo="Cobranza: emitido vs. cobrado" extra={cob.length ? <Leyenda items={[{ texto: 'Emitido', clase: 'bg-serie-2' }, { texto: 'Cobrado', clase: 'bg-serie-1' }]} /> : null}>
                {cob.length ? (
                  <>
                    <BarrasAgrupadas
                      titulo="Emitido y cobrado por mes"
                      datos={cob}
                      series={[
                        { clave: 'emitido', nombre: 'Emitido', claseSvg: 'fill-serie-2' },
                        { clave: 'cobrado', nombre: 'Cobrado', claseSvg: 'fill-serie-1' },
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
                  <Vacio titulo="Sin datos en el rango" texto="Prueba con los últimos 12 meses." icono="grafico" compacto />
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
                  <Vacio titulo="Sin datos en el rango" texto="Prueba con los últimos 12 meses." icono="grafico" compacto />
                )}
              </Seccion>

              <Seccion titulo="Consumo de agua por unidad (m³)" extra={<span className="text-xs text-texto-apoyo">media {formatearNumero(mediaAgua, 1)} m³ · «pico» = más del doble</span>}>
                {agua.length ? (
                  <div className="max-h-[420px] overflow-y-auto pr-1">
                    <BarrasH titulo="Consumo por unidad" datos={agua} etiqueta={(x) => x.unidad} valor={(x) => x.m3} formato={(v) => formatearNumero(v, 1)} resaltar={(x) => x.m3 > 2 * mediaAgua} />
                  </div>
                ) : (
                  <Vacio titulo="Sin lecturas" texto="Aparecen cuando el operario registra la ronda." icono="medidor" compacto />
                )}
              </Seccion>

              <div className="flex flex-col gap-4 lg:gap-5">
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
                    <Vacio titulo="Sin reservas" texto="Aún no hay reservas en este rango." icono="calendario" compacto />
                  )}
                </Seccion>
                <Seccion titulo="Incidencias por estado">
                  {inc.length ? <BarrasH titulo="Incidencias por estado" datos={inc} etiqueta={(x) => ETIQUETA_ESTADO[x.estado] || x.estado} valor={(x) => x.cantidad} formato={(v) => `${v}`} claseBarra={(x) => TONO_PUNTO[tonoDe(x.estado)]} /> : <Vacio titulo="Sin incidencias" texto="Ningún reporte en este rango." icono="herramienta" compacto />}
                </Seccion>
              </div>
            </div>
          </>
        )}
      </Contenido>
    </>
  );
}
