import { useEffect, useMemo, useState } from 'react';
import { api } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { COLUMNAS, CRITICIDADES, filtrosDesdeURL, filtrosAURL, esSalida } from '../../lib/kanban.js';
import { useQuery } from '../../lib/nav.jsx';
import { Boton, Chip, ErrorCarga, Esqueleto, Icono, Insignia, Vacio, infoEstado } from '../../ui/index.js';

const PASOS = ['reportado', 'validado', 'presupuestado', 'aprobado', 'en_ejecucion', 'terminado'];
const PCT = { reportado: 5, validado: 20, presupuestado: 40, aprobado: 55, en_ejecucion: 75, terminado: 100 };
const etiquetaDe = (estado) => COLUMNAS.find((c) => c.estado === estado)?.etiqueta || estado;
const TEXTO_CRIT = { critica: 'Crítico', media: 'Medio', baja: 'Bajo' };

/** Hitos del flujo: reportado → validado → informe y costos → aprobado → en ejecución → terminado. */
function Hitos({ estado }) {
  const salida = esSalida(estado);
  const idx = PASOS.indexOf(estado);
  const pasos = salida ? [...PASOS.slice(0, 3), estado] : PASOS;
  return (
    <ol className="grid grid-cols-6 gap-1 sm:grid-cols-7" aria-label={`Avance: ${etiquetaDe(estado)}`}>
      {pasos.map((p, i) => {
        const esSalidaPaso = esSalida(p);
        const hecho = !salida ? i < idx : i < 3;
        const actual = salida ? esSalidaPaso : i === idx;
        return (
          <li key={p} className="flex min-w-0 flex-col gap-1" aria-current={actual ? 'step' : undefined}>
            <span title={etiquetaDe(p)}
              className={`h-1.5 rounded-full transition-colors duration-media ${hecho ? 'bg-acento' : actual ? (esSalidaPaso ? 'bg-alerta' : 'bg-curso') : 'bg-borde'}`} />
            <span className={`truncate text-[10px] leading-tight ${actual ? 'font-semibold text-tinta' : 'text-texto-apoyo'}`}>{etiquetaDe(p)}</span>
          </li>
        );
      })}
    </ol>
  );
}

/** Plan de trabajo: cada actividad con sus cajitas de progreso, informe técnico (sí/no) y estado. */
export default function Plan() {
  const [q, setQuery] = useQuery();
  const filtros = useMemo(() => filtrosDesdeURL(q), [q]);
  const clave = filtrosAURL(filtros).toString();
  const [texto, setTexto] = useState(filtros.q || '');
  const carga = useCarga(() => api.get('/mantenimiento/plan', filtros), [clave]);
  const filas = useMemo(() => (Array.isArray(carga.datos?.datos) ? carga.datos.datos : []), [carga.datos]);

  useEffect(() => {
    const t = setTimeout(() => texto !== (filtros.q || '') && setQuery(filtrosAURL({ ...filtros, q: texto }), { reemplazar: true }), 350);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [texto]);

  const crits = filtros.criticidad ? String(filtros.criticidad).split(',').filter(Boolean) : [];
  const setCrit = (v) => {
    const s = new Set(crits);
    if (s.has(v)) s.delete(v);
    else s.add(v);
    const orden = CRITICIDADES.map((c) => c.valor).filter((c) => s.has(c)).join(',') || null;
    setQuery(filtrosAURL({ ...filtros, criticidad: orden }), { reemplazar: true });
  };

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2" role="search" aria-label="Filtrar plan">
        <label className="relative min-w-[180px] flex-1 sm:max-w-xs">
          <span className="sr-only">Buscar trabajo</span>
          <Icono nombre="buscar" tam={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-texto-apoyo" />
          <input type="search" value={texto} onChange={(e) => setTexto(e.target.value)} placeholder="Código, título, ubicación…" className="h-11 w-full rounded-control border border-borde-fuerte bg-superficie pl-9 pr-3 text-base transition-colors duration-rapida focus:border-acento focus:outline-none focus:ring-2 focus:ring-acento lg:h-8 lg:text-sm" />
        </label>
        <div className="flex gap-1.5" role="group" aria-label="Criticidad">
          {CRITICIDADES.map((c) => (
            <Chip key={c.valor} activo={crits.includes(c.valor)} tono={infoEstado(c.valor).tono} onClick={() => setCrit(c.valor)}>
              {TEXTO_CRIT[c.valor]}
            </Chip>
          ))}
        </div>
        {(filtros.q || filtros.criticidad) && (
          <Boton variante="fantasma" tamano="sm" onClick={() => (setTexto(''), setQuery(filtrosAURL({}), { reemplazar: true }))} icono="cerrar">
            Limpiar
          </Boton>
        )}
        <p className="ml-auto text-xs text-texto-apoyo" aria-live="polite">{filas.length} en plan</p>
      </div>

      {carga.error ? (
        <ErrorCarga error={carga.error} onReintentar={carga.recargar} />
      ) : !carga.datos ? (
        <div className="flex flex-col gap-2">
          {Array.from({ length: 4 }, (_, i) => <Esqueleto key={i} className="h-28 w-full" />)}
        </div>
      ) : filas.length === 0 ? (
        <Vacio titulo="Plan al día" texto="Sin trabajos por hacer con estos filtros. Lo terminado sale del plan." icono="hecho" compacto />
      ) : (
        <ul className="flex flex-col gap-2">
          {filas.map((t) => (
            <li key={t.id} className="flex flex-col gap-2 rounded-tarjeta border border-borde bg-superficie p-3">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-semibold text-texto-suave">{t.codigo}</span>
                <b className="min-w-0 flex-1 truncate text-sm text-tinta" title={t.titulo}>{t.titulo}</b>
                <Insignia estado={t.estado} />
                {t.tiene_informe ? (
                  <span className="inline-flex items-center gap-1 rounded-chip bg-acento-suave px-2 py-0.5 text-xs font-semibold text-acento" title="Diagnóstico y costos publicados">
                    <Icono nombre="documento" tam={12} /> Informe: sí
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1 rounded-chip bg-superficie-2 px-2 py-0.5 text-xs font-semibold text-texto-apoyo" title="Falta publicar diagnóstico y costos">
                    Informe: no
                  </span>
                )}
              </div>
              <Hitos estado={t.estado} />
              <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-texto-apoyo">
                <span className="tabular-nums font-semibold text-texto-suave">{PCT[t.estado] ?? 0} %</span>
                {t.responsable && <span className="truncate">{t.responsable}</span>}
                {t.monto_cts > 0 && <span className="tabular-nums">Ppto. {formatearSoles(t.monto_cts)}</span>}
                {t.costo_cts > 0 && <span className="tabular-nums">Real {formatearSoles(t.costo_cts)}</span>}
                {t.avances > 0 && <span>{t.avances} {t.avances === 1 ? 'avance' : 'avances'}</span>}
                {t.criticidad && <span className="font-semibold text-alerta">{TEXTO_CRIT[t.criticidad] || t.criticidad}</span>}
                <span className="ml-auto">Act. {t.actualizado}</span>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
