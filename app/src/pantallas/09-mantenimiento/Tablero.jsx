import { useEffect, useMemo, useState } from 'react';
import { api } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha, haceCuanto } from '../../lib/fechas.js';
import {
  ACCION_HACIA, CATEGORIAS, COLUMNAS, CRITICIDADES, agruparPorEstado, filtrarIncidencias, filtrosAURL, filtrosDesdeURL,
  hayFiltros, normalizarIncidencia, puedeTransicionar, transicionesPermitidas, PERMISO_HACIA, esSalida,
} from '../../lib/kanban.js';
import { lista as aLista } from '../../lib/api.js';
import { ruta, useQuery } from '../../lib/nav.jsx';
import { useSesion, Guarda } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton, Campo, ErrorCarga, Esqueleto, Icono, Insignia, Modal, useDialog, useToast } from '../../ui/index.js';

const CRIT = {
  critica: { texto: 'CRÍTICO', clase: 'text-alerta', caja: 'bg-alerta-suave border-alerta-borde' },
  media: { texto: 'MEDIO', clase: 'text-aviso', caja: 'bg-superficie border-borde' },
  baja: { texto: 'BAJO', clase: 'text-texto-suave', caja: 'bg-superficie border-borde' },
};

/** Tablero kanban por estado, con barra de filtros sincronizada con la URL. */
export default function Tablero() {
  const s = useSesion();
  const [q, setQuery] = useQuery();
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();
  const filtros = useMemo(() => filtrosDesdeURL(q), [q]);
  const claveFiltros = filtrosAURL(filtros).toString();
  const [texto, setTexto] = useState(filtros.q || '');
  const [verFiltros, setVerFiltros] = useState(false);
  const [arrastrando, setArrastrando] = useState(null);
  const [sobre, setSobre] = useState(null);
  const [detalle, setDetalle] = useState(null);
  const [moviendo, setMoviendo] = useState(null);
  const colMovil = COLUMNAS.some((c) => c.estado === q.get('col')) ? q.get('col') : 'reportado';

  const carga = useCarga(() => api.get('/mantenimiento/incidencias', filtros), [claveFiltros]);
  const incidencias = useMemo(() => aLista(carga.datos, 'incidencias').map(normalizarIncidencia), [carga.datos]);
  // Red de seguridad: si el API ignorara algún filtro, igual se aplica aquí.
  const visibles = useMemo(() => filtrarIncidencias(incidencias, filtros), [incidencias, filtros]);
  const grupos = useMemo(() => agruparPorEstado(visibles), [visibles]);
  const responsables = useMemo(() => {
    const desdeApi = carga.datos?.responsables;
    if (Array.isArray(desdeApi)) return desdeApi;
    const m = new Map();
    for (const i of incidencias) if (i.responsable_id != null) m.set(String(i.responsable_id), { id: i.responsable_id, nombre: i.responsable_nombre || `Responsable ${i.responsable_id}` });
    return [...m.values()];
  }, [carga.datos, incidencias]);

  useEffect(() => {
    const t = setTimeout(() => {
      if ((texto || '') !== (filtros.q || '')) setQuery(filtrosAURL({ ...filtros, q: texto }, window.location.search), { reemplazar: true });
    }, 350);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [texto]);

  const setFiltro = (k, v) => setQuery(filtrosAURL({ ...filtros, [k]: v }, window.location.search), { reemplazar: true });
  const limpiar = () => {
    setTexto('');
    setQuery(filtrosAURL({}, window.location.search), { reemplazar: true });
  };
  const nFiltros = Object.keys(filtros).length;
  const permitidas = (inc) => transicionesPermitidas(inc.estado, s.tiene, inc.transiciones);

  const mover = async (inc, a) => {
    if (!puedeTransicionar(inc.estado, a, inc.transiciones) || !s.tiene(PERMISO_HACIA[a])) {
      toast(`No se puede pasar de «${etiqueta(inc.estado)}» a «${etiqueta(a)}».`, { tipo: 'aviso' });
      return;
    }
    let cuerpo = { estado: a };
    if (a === 'rechazado' || a === 'descartado') {
      const rechazo = a === 'rechazado';
      const motivo = await dialog.prompt({
        title: `${rechazo ? 'Rechazar' : 'Descartar'} ${inc.codigo}`,
        text: rechazo ? 'Quedará como «pendiente no aprobado» en el informe del mes.' : 'Por ejemplo: «igual a INC-019» o «no corresponde a áreas comunes».',
        label: 'Motivo',
        required: true,
        okText: rechazo ? 'Rechazar' : 'Descartar',
        danger: true,
      });
      if (motivo === null) return;
      cuerpo = { ...cuerpo, motivo };
    } else if (a === 'terminado') {
      const ok = await dialog.confirm({ title: `¿Marcar ${inc.codigo} como terminado?`, text: 'El costo real entra al balance como egreso, con su comprobante.', okText: 'Marcar terminado' });
      if (!ok) return;
    }
    const previo = carga.datos;
    // Optimista: se mueve ya y se revierte si el API no acepta.
    carga.setDatos((d) => {
      const l = aLista(d, 'incidencias').map((x) => (x.id === inc.id ? { ...x, estado: a } : x));
      return Array.isArray(d) ? l : { ...d, datos: l };
    });
    setMoviendo(inc.id);
    try {
      await api.patch(`/mantenimiento/incidencias/${inc.id}/estado`, cuerpo);
      toast(`${inc.codigo} → ${etiqueta(a)}.`, { tipo: 'exito', duracion: 2500 });
      if (detalle?.id === inc.id) setDetalle({ ...inc, estado: a });
    } catch (err) {
      carga.setDatos(previo);
      await dialog.alert({ title: 'No se pudo mover', text: err.message });
    } finally {
      setMoviendo(null);
    }
  };

  const soltar = (estado) => {
    setSobre(null);
    const inc = arrastrando;
    setArrastrando(null);
    if (inc && inc.estado !== estado) mover(inc, estado);
  };

  const barraFiltros = (
    <div className={`flex-col gap-3 rounded-tarjeta border border-borde bg-superficie p-4 lg:flex lg:flex-row lg:flex-wrap lg:items-end ${verFiltros ? 'flex' : 'hidden'}`} role="search" aria-label="Filtros del tablero">
      <Campo className="lg:w-40" etiqueta="Criticidad" tipo="select" valor={filtros.criticidad || ''} onCambio={(v) => setFiltro('criticidad', v)} opciones={[{ valor: '', etiqueta: 'Todas' }, ...CRITICIDADES]} />
      <Campo className="lg:w-48" etiqueta="Categoría" tipo="select" valor={filtros.categoria || ''} onCambio={(v) => setFiltro('categoria', v)} opciones={[{ valor: '', etiqueta: 'Todas' }, ...CATEGORIAS]} />
      <Campo className="lg:w-48" etiqueta="Responsable" tipo="select" valor={filtros.responsable_id || ''} onCambio={(v) => setFiltro('responsable_id', v)} opciones={[{ valor: '', etiqueta: 'Todos' }, ...responsables.map((r) => ({ valor: String(r.id), etiqueta: r.nombre }))]} />
      <Campo className="lg:w-40" etiqueta="Desde" tipo="fecha" valor={filtros.desde || ''} onCambio={(v) => setFiltro('desde', v)} />
      <Campo className="lg:w-40" etiqueta="Hasta" tipo="fecha" valor={filtros.hasta || ''} onCambio={(v) => setFiltro('hasta', v)} />
      <Campo className="min-w-[200px] flex-1" etiqueta="Buscar" tipo="buscar" valor={texto} onCambio={setTexto} placeholder="Código, título, ubicación…" />
      {hayFiltros(filtros) && (
        <Boton variante="fantasma" onClick={limpiar} icono="cerrar">
          Limpiar filtros
        </Boton>
      )}
    </div>
  );

  const tarjeta = (inc) => {
    const c = CRIT[inc.criticidad];
    const acc = permitidas(inc);
    return (
      <article
        key={inc.id}
        draggable={acc.length > 0}
        onDragStart={(e) => {
          setArrastrando(inc);
          e.dataTransfer.effectAllowed = 'move';
          e.dataTransfer.setData('text/plain', String(inc.id));
        }}
        onDragEnd={() => (setArrastrando(null), setSobre(null))}
        className={`flex flex-col gap-1.5 rounded-control border p-3 text-xs ${c ? c.caja : 'bg-superficie border-borde'} ${acc.length ? 'cursor-grab active:cursor-grabbing' : ''} ${moviendo === inc.id ? 'opacity-60' : ''}`}
      >
        <button type="button" onClick={() => setDetalle(inc)} className="flex flex-col gap-1 text-left">
          <span className="flex items-center justify-between gap-2">
            <span className={c ? `font-bold ${c.clase}` : 'text-texto-apoyo'}>{c ? c.texto : `${inc.codigo}${inc.unidad ? ` · ${inc.unidad}` : inc.reportado_por ? ` · ${inc.reportado_por}` : ''}`}</span>
            {acc.length > 0 && <Icono nombre="arrastrar" tam={14} className="hidden text-texto-tenue lg:block" />}
          </span>
          <b className="text-sm leading-snug text-tinta">{c ? `${inc.codigo} ${inc.titulo}` : inc.titulo}</b>
          <span className="text-texto-apoyo">
            {inc.votos && inc.estado === 'presupuestado'
              ? `${formatearSoles(inc.monto_cts)} · ${inc.votos.a_favor} de ${inc.votos.necesarios ?? '?'} votos necesarios`
              : inc.estado === 'terminado' && inc.monto_cts != null
                ? `${formatearSoles(inc.costo_real_cts ?? inc.monto_cts)} · en balance`
                : inc.monto_cts != null
                  ? `${formatearSoles(inc.monto_cts)}${inc.responsable_nombre ? ` · ${inc.responsable_nombre}` : ''}`
                  : `${inc.n_fotos || 0} foto${inc.n_fotos === 1 ? '' : 's'} · ${haceCuanto(inc.reportado_en)}`}
          </span>
          {inc.avance_pct != null && (
            <span className="mt-1 h-1.5 rounded-full bg-borde" aria-label={`Avance ${inc.avance_pct} %`}>
              <span className="block h-1.5 rounded-full bg-acento" style={{ width: `${inc.avance_pct}%` }} />
            </span>
          )}
        </button>
        {acc.length > 0 && (
          <div className="mt-1 flex flex-wrap gap-1.5">
            {acc.map((a) => (
              <button
                key={a}
                type="button"
                onClick={() => mover(inc, a)}
                disabled={moviendo === inc.id}
                className={`min-h-[36px] rounded-control border px-2 text-xs font-semibold ${esSalida(a) ? 'border-alerta-borde text-alerta hover:bg-alerta-suave' : 'border-acento-borde bg-acento-suave text-acento hover:bg-acento hover:text-white'}`}
              >
                {ACCION_HACIA[a]}
                {!esSalida(a) && ' →'}
              </button>
            ))}
          </div>
        )}
      </article>
    );
  };

  const columna = (col, i, movil = false) => {
    const items = grupos[col.estado] || [];
    const valida = arrastrando && arrastrando.estado !== col.estado && puedeTransicionar(arrastrando.estado, col.estado, arrastrando.transiciones) && s.tiene(PERMISO_HACIA[col.estado]);
    return (
      <section
        key={col.estado}
        aria-label={`${col.etiqueta}: ${items.length}`}
        onDragOver={(e) => {
          if (valida) {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'move';
            setSobre(col.estado);
          }
        }}
        onDragLeave={() => setSobre((x) => (x === col.estado ? null : x))}
        onDrop={(e) => {
          e.preventDefault();
          if (valida) soltar(col.estado);
        }}
        className={`flex min-w-0 flex-col gap-2 rounded-tarjeta p-2 transition-colors ${movil ? '' : 'min-w-[176px] flex-1 basis-0'} ${sobre === col.estado ? 'bg-acento-suave ring-2 ring-acento' : valida ? 'bg-acento-suave/50 ring-1 ring-acento-borde' : arrastrando ? 'opacity-60' : ''}`}
      >
        {!movil && (
          <h2 className="px-1 text-xs font-semibold text-texto-suave">
            {i + 1} · {col.etiqueta.toUpperCase()} ({items.length})
          </h2>
        )}
        {items.length === 0 ? <p className="rounded-control border border-dashed border-borde px-3 py-6 text-center text-xs text-texto-apoyo">{valida ? 'Suelta aquí' : 'Sin trabajos'}</p> : items.map(tarjeta)}
      </section>
    );
  };

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Mantenimiento e incidencias"
        acciones={
          <>
            <Boton variante="secundario" icono="filtro" className="lg:hidden" onClick={() => setVerFiltros(!verFiltros)} aria-expanded={verFiltros}>
              Filtros{nFiltros ? ` (${nFiltros})` : ''}
            </Boton>
            <Guarda permiso="incidencias.reportar">
              <Boton icono="camara" href={ruta('mantenimiento', { reportar: 1 })}>
                Registrar incidencia
              </Boton>
            </Guarda>
          </>
        }
      />
      <Contenido className="lg:max-w-none">
        {barraFiltros}
        {carga.error ? (
          <ErrorCarga error={carga.error} onReintentar={carga.recargar} />
        ) : !carga.datos ? (
          <div className="flex gap-3 overflow-hidden">
            {Array.from({ length: 5 }, (_, i) => (
              <Esqueleto key={i} className="h-72 w-56 shrink-0" />
            ))}
          </div>
        ) : visibles.length === 0 && !hayFiltros(filtros) ? (
          <div className="rounded-tarjeta border border-borde bg-superficie p-10 text-center text-base text-texto-suave">Sin trabajos este mes. Así da gusto.</div>
        ) : (
          <>
            <p className="text-sm text-texto-apoyo" aria-live="polite">
              {visibles.length} {visibles.length === 1 ? 'trabajo' : 'trabajos'}
              {hayFiltros(filtros) ? ' con estos filtros' : ''}<span className="hidden lg:inline"> · arrastra una tarjeta o usa sus botones para cambiarla de columna</span>.
            </p>
            {/* Escritorio: columnas */}
            <div className="hidden gap-3 overflow-x-auto pb-2 lg:flex" role="list">
              {COLUMNAS.map((c, i) => columna(c, i))}
            </div>
            {/* Móvil: pestañas por estado */}
            <div className="flex flex-col gap-3 lg:hidden">
              <div role="tablist" aria-label="Estado" className="-mx-4 flex gap-2 overflow-x-auto px-4 pb-1">
                {COLUMNAS.map((c) => (
                  <button
                    key={c.estado}
                    type="button"
                    role="tab"
                    aria-selected={colMovil === c.estado}
                    onClick={() => setQuery({ col: c.estado }, { reemplazar: true })}
                    className={`h-11 shrink-0 rounded-full border px-4 text-sm font-semibold ${colMovil === c.estado ? 'border-tinta bg-tinta text-white' : 'border-borde-fuerte bg-superficie'}`}
                  >
                    {c.etiqueta} · {(grupos[c.estado] || []).length}
                  </button>
                ))}
              </div>
              {columna(
                COLUMNAS.find((c) => c.estado === colMovil),
                0,
                true,
              )}
            </div>
          </>
        )}
      </Contenido>
      <Detalle inc={detalle} onCerrar={() => setDetalle(null)} acciones={detalle ? permitidas(detalle) : []} onMover={mover} />
    </>
  );
}

function etiqueta(estado) {
  return COLUMNAS.find((c) => c.estado === estado)?.etiqueta || estado;
}

const PASOS = ['reportado', 'validado', 'presupuestado', 'aprobado', 'en_ejecucion', 'terminado'];

function Detalle({ inc, onCerrar, acciones, onMover }) {
  if (!inc) return null;
  const idx = PASOS.indexOf(inc.estado);
  const rechazado = esSalida(inc.estado);
  return (
    <Modal abierto={!!inc} onCerrar={onCerrar} titulo={`${inc.codigo} · ${inc.titulo}`} lateral>
      <div className="flex flex-col gap-5 text-sm">
        <div className="flex flex-wrap gap-2">
          <Insignia estado={inc.estado} />
          {inc.criticidad && <Insignia estado={inc.criticidad} />}
        </div>
        <ol className="grid grid-cols-3 gap-2 sm:grid-cols-6" aria-label="Línea de tiempo">
          {PASOS.map((p, i) => {
            const hecho = !rechazado && i < idx;
            const actual = !rechazado && i === idx;
            return (
              <li key={p} className="flex flex-col gap-1 text-xs" aria-current={actual ? 'step' : undefined}>
                <span className={`h-1.5 rounded-full ${hecho ? 'bg-acento' : actual ? (inc.criticidad === 'critica' ? 'bg-alerta' : 'bg-acento') : 'bg-borde'}`} />
                <b className={actual ? 'text-tinta' : hecho ? '' : 'text-texto-apoyo'}>{etiqueta(p)}</b>
              </li>
            );
          })}
        </ol>
        {rechazado && <p className="rounded-control border border-alerta-borde bg-alerta-suave p-3 text-alerta-texto">{inc.estado === 'descartado' ? 'Descartado' : 'Rechazado: queda como «pendiente no aprobado» en el informe del mes'}{inc.motivo ? ` · ${inc.motivo}` : ''}.</p>}
        <dl className="grid grid-cols-[120px_1fr] gap-x-3 gap-y-2">
          {inc.descripcion && inc.descripcion !== inc.titulo && (
            <>
              <dt className="text-texto-apoyo">Descripción</dt>
              <dd>{inc.descripcion}</dd>
            </>
          )}
          <dt className="text-texto-apoyo">Ubicación</dt>
          <dd>{inc.ubicacion || '—'}</dd>
          <dt className="text-texto-apoyo">Categoría</dt>
          <dd>{CATEGORIAS.find((c) => c.valor === inc.categoria)?.etiqueta || '—'}</dd>
          <dt className="text-texto-apoyo">Reportado</dt>
          <dd>
            {inc.reportado_por || '—'} · {formatearFecha(inc.reportado_en)}
          </dd>
          <dt className="text-texto-apoyo">Responsable</dt>
          <dd>{inc.responsable_nombre || 'Sin asignar'}</dd>
          <dt className="text-texto-apoyo">Presupuesto</dt>
          <dd className="tabular-nums">{inc.monto_cts != null ? formatearSoles(inc.monto_cts) : '—'}</dd>
          {inc.votos && (
            <>
              <dt className="text-texto-apoyo">Junta</dt>
              <dd>
                {inc.votos.a_favor} de {inc.votos.necesarios} votos necesarios{inc.votos.miembros ? ` (junta de ${inc.votos.miembros})` : ''}
              </dd>
            </>
          )}
        </dl>
        {acciones.length > 0 && (
          <div className="flex flex-col gap-2 border-t border-borde pt-4 sm:flex-row sm:flex-wrap">
            {acciones.map((a) => (
              <Boton key={a} variante={esSalida(a) ? 'secundario' : 'primario'} onClick={() => onMover(inc, a)}>
                {ACCION_HACIA[a]}
              </Boton>
            ))}
          </div>
        )}
      </div>
    </Modal>
  );
}
