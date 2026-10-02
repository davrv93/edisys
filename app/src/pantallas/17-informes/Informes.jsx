import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Chip, ErrorCarga, Esqueleto, Vacio } from '../../ui/index.js';

function Kpi({ et, v, d }) {
  return (
    <div className="flex flex-col gap-0.5 rounded-tarjeta border border-borde bg-superficie p-3">
      <span className="text-xs font-semibold uppercase tracking-wide text-texto-apoyo">{et}</span>
      <span className="font-titulo text-xl font-semibold tabular-nums text-tinta">{v}</span>
      {d && <span className="text-xs text-texto-apoyo">{d}</span>}
    </div>
  );
}

/** Bloque C · Informes: económico del periodo y consumos por departamento. */
export default function Informes() {
  const eid = useEid();
  const [periodo] = usePeriodo();
  const [vista, setVista] = useState('economico');

  const eco = useCarga(() => api.get(`/edificios/${eid}/informes/economico`, { periodo }), [eid, periodo]);
  const con = useCarga(() => api.get(`/edificios/${eid}/informes/consumos`, { hasta: periodo }), [eid, periodo]);

  const r = eco.datos?.resumen;
  const flujo = lista(eco.datos?.flujo);
  const pagos = lista(eco.datos?.pagos);

  return (
    <>
      <Encabezado titulo="Informes" subtitulo={`Económico y consumos · ${periodo}`} ayuda="Resumen económico del mes, flujo de los últimos 12 meses y consumo de agua por departamento." />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'economico'} icono="balance" onClick={() => setVista('economico')}>
            Económico
          </Chip>
          <Chip activo={vista === 'consumos'} icono="medidor" onClick={() => setVista('consumos')}>
            Consumos por departamento
          </Chip>
        </div>

        {vista === 'economico' ? (
          eco.error ? (
            <ErrorCarga error={eco.error} onReintentar={eco.recargar} />
          ) : !eco.datos ? (
            <Esqueleto className="h-56 w-full" />
          ) : (
            <>
              <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
                <Kpi et="Ingresos" v={formatearSoles(r.ingresos_cts)} />
                <Kpi et="Egresos" v={formatearSoles(r.egresos_cts)} />
                <Kpi et="Saldo del mes" v={formatearSoles(r.saldo_cts)} />
                <Kpi et="Banco" v={formatearSoles(r.banco_cts)} />
                <Kpi et="Emitido" v={formatearSoles(r.emitido_cts)} />
              </div>
              <Seccion titulo="Flujo de los últimos 12 meses">
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-borde text-left text-xs text-texto-apoyo">
                        <th className="py-2 pr-2 font-semibold">Periodo</th>
                        <th className="py-2 px-2 text-right font-semibold">Ingreso</th>
                        <th className="py-2 px-2 text-right font-semibold">Egreso</th>
                        <th className="py-2 pl-2 text-right font-semibold">Saldo</th>
                      </tr>
                    </thead>
                    <tbody>
                      {flujo.map((f) => (
                        <tr key={f.periodo} className="border-b border-borde last:border-0">
                          <td className="py-1.5 pr-2">{f.periodo}</td>
                          <td className="py-1.5 px-2 text-right tabular-nums">{formatearSoles(f.ingreso_cts)}</td>
                          <td className="py-1.5 px-2 text-right tabular-nums">{formatearSoles(f.egreso_cts)}</td>
                          <td className="py-1.5 pl-2 text-right font-semibold tabular-nums">{formatearSoles(f.ingreso_cts - f.egreso_cts)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </Seccion>
              <Seccion titulo={`Pagos del periodo (${pagos.length})`}>
                {pagos.length === 0 ? (
                  <p className="text-sm text-texto-apoyo">Sin pagos registrados en el periodo.</p>
                ) : (
                  <ul className="divide-y divide-borde">
                    {pagos.map((p) => (
                      <li key={p.id} className="flex items-center justify-between gap-3 py-2 text-sm">
                        <span className="min-w-0 truncate">
                          Dpto {p.unidad} · {formatearFecha(p.fecha)} · {p.medio}
                          {p.codigo_operacion ? ` · Op. ${p.codigo_operacion}` : ''}
                        </span>
                        <span className="shrink-0 tabular-nums">{formatearSoles(p.monto_cts)}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </Seccion>
            </>
          )
        ) : con.error ? (
          <ErrorCarga error={con.error} onReintentar={con.recargar} />
        ) : !con.datos ? (
          <Esqueleto className="h-56 w-full" />
        ) : lista(con.datos.unidades).length === 0 ? (
          <Vacio icono="medidor" titulo="Sin consumos" texto="Cuando el operario registre lecturas de agua, aparecerán aquí por departamento y periodo." />
        ) : (
          <Seccion titulo="Consumo de agua (m³) por departamento">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-borde text-left text-xs text-texto-apoyo">
                    <th className="py-2 pr-2 font-semibold">Dpto</th>
                    {(con.datos.periodos || []).map((p) => (
                      <th key={p} className="py-2 px-2 text-right font-semibold">{p}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {lista(con.datos.unidades).map((u) => (
                    <tr key={u.unidad} className="border-b border-borde last:border-0">
                      <td className="py-1.5 pr-2 font-medium">{u.unidad}</td>
                      {u.consumos.map((c, i) => (
                        <td key={i} className="py-1.5 px-2 text-right tabular-nums">{c == null ? '—' : Number(c).toFixed(3)}</td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Seccion>
        )}
      </Contenido>
    </>
  );
}
