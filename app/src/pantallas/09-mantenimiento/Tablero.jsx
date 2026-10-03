import { useEffect, useMemo, useState } from 'react';
import { api, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearSolesCorto } from '../../lib/dinero.js';
import { formatearFecha, haceCuanto } from '../../lib/fechas.js';
import {
  ACCION_HACIA, CATEGORIAS, COLUMNAS, CRITICIDADES, agruparPorEstado, filtrarIncidencias, filtrosAURL, filtrosDesdeURL,
  hayFiltros, normalizarIncidencia, puedeTransicionar, transicionesPermitidas, PERMISO_HACIA, esSalida, alternarCriticidad, criticidadesDe,
  columnasDesdeConfig, tarjetaDesdeConfig,
} from '../../lib/kanban.js';
import { lista as aLista } from '../../lib/api.js';
import { ruta, useQuery } from '../../lib/nav.jsx';
import { useSesion, Guarda } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, Desplegable, ErrorCarga, Esqueleto, Icono, Insignia, MenuAcciones, Modal, PuntoEstado, Vacio, infoEstado, TONO_PUNTO, useDialog, useToast } from '../../ui/index.js';
import ConfigurarTablero from './ConfigurarTablero.jsx';
import { TONO_TEXTO } from '../../ui/estados.js'; // operacion: G2
import { infoSemaforo, restanteSLA } from '../../lib/operacion.js'; // operacion: G2

const TEXTO_CRIT = { critica: 'Crítico', media: 'Medio', baja: 'Bajo' };

/** Tablero kanban por estado (v2): barra de filtros de una línea, tarjetas compactas, arrastre con movimiento.
 *  Con sinMarco lo monta Mantenimiento (encabezado y pestañas comunes con el plan). */
export default function Tablero({ sinMarco = false }) {
  const s = useSesion();
  const [q, setQuery] = useQuery();
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();
  const filtros = useMemo(() => filtrosDesdeURL(q), [q]);
  const claveFiltros = filtrosAURL(filtros).toString();
  const [texto, setTexto] = useState(filtros.q || '');
  const [arrastrando, setArrastrando] = useState(null);
  const [sobre, setSobre] = useState(null);
  const [detalle, setDetalle] = useState(null);
  const [moviendo, setMoviendo] = useState(null);
  const [asentada, setAsentada] = useState(null); // tarjeta que acaba de caer en su columna
  const [rechazada, setRechazada] = useState(null); // tarjeta que el API devolvió (tiembla)
  const [configura, setConfigura] = useState(false);
  // Configuración central del tablero (etapas visibles en orden + campos de tarjeta).
  const conf = useCarga(() => api.get('/mantenimiento/tablero/config'), []);
  const columnas = useMemo(() => columnasDesdeConfig(conf.datos), [conf.datos]);
  const campos = useMemo(() => tarjetaDesdeConfig(conf.datos), [conf.datos]);
  const flujo = useMemo(() => columnas.filter((c) => !esSalida(c.estado)), [columnas]);
  const salidas = useMemo(() => columnas.filter((c) => esSalida(c.estado)), [columnas]);
  const colMovil = columnas.some((c) => c.estado === q.get('col')) ? q.get('col') : (columnas[0]?.estado || 'reportado');

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
  const permitidas = (inc) => transicionesPermitidas(inc.estado, s.tiene, inc.transiciones);
  const crits = criticidadesDe(filtros);
  // Filtros del menú «Filtros» (los que no son buscador ni criticidad).
  const activosMenu = [
    filtros.categoria && { k: 'categoria', texto: CATEGORIAS.find((c) => c.valor === filtros.categoria)?.etiqueta || filtros.categoria },
    filtros.responsable_id && { k: 'responsable_id', texto: responsables.find((r) => String(r.id) === String(filtros.responsable_id))?.nombre || `Responsable ${filtros.responsable_id}` },
    filtros.desde && { k: 'desde', texto: `Desde ${formatearFecha(filtros.desde)}` },
    filtros.hasta && { k: 'hasta', texto: `Hasta ${formatearFecha(filtros.hasta)}` },
  ].filter(Boolean);

  const mover = async (inc, a) => {
    if (!puedeTransicionar(inc.estado, a, inc.transiciones) || !s.tiene(PERMISO_HACIA[a])) {
      toast(`No se puede pasar de «${etiqueta(inc.estado)}» a «${etiqueta(a)}».`, { tipo: 'aviso' });
      setRechazada(inc.id);
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
    setAsentada(inc.id);
    try {
      await api.patch(`/mantenimiento/incidencias/${inc.id}/estado`, cuerpo);
      toast(`${inc.codigo} → ${etiqueta(a)}.`, { tipo: 'exito', duracion: 2500 });
      if (detalle?.id === inc.id) setDetalle({ ...inc, estado: a });
    } catch (err) {
      carga.setDatos(previo);
      setAsentada(null);
      setRechazada(inc.id);
      toast(`No se pudo mover ${inc.codigo}: ${err.message}`, { tipo: 'error' });
    } finally {
      setMoviendo(null);
    }
  };

  // El temblor y el asentado se ven una vez y se limpian.
  useEffect(() => {
    if (rechazada == null) return undefined;
    const t = setTimeout(() => setRechazada(null), 400);
    return () => clearTimeout(t);
  }, [rechazada]);
  useEffect(() => {
    if (asentada == null) return undefined;
    const t = setTimeout(() => setAsentada(null), 400);
    return () => clearTimeout(t);
  }, [asentada]);

  const soltar = (estado) => {
    setSobre(null);
    const inc = arrastrando;
    setArrastrando(null);
    if (inc && inc.estado !== estado) mover(inc, estado);
  };

  const barraFiltros = (
    <div className="flex flex-col gap-2" role="search" aria-label="Filtros del tablero">
      <div className="flex flex-wrap items-center gap-2">
        <label className="relative min-w-[180px] flex-1 sm:max-w-xs">
          <span className="sr-only">Buscar por código, título o ubicación</span>
          <Icono nombre="buscar" tam={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-texto-apoyo" />
          <input type="search" value={texto} onChange={(e) => setTexto(e.target.value)} placeholder="Código, título, ubicación…" className="h-11 w-full rounded-control border border-borde-fuerte bg-superficie pl-9 pr-3 text-base transition-colors duration-rapida focus:border-acento focus:outline-none focus:ring-2 focus:ring-acento lg:h-8 lg:text-sm" />
        </label>
        <div className="flex gap-1.5 overflow-x-auto" role="group" aria-label="Criticidad (puedes elegir varias)">
          {CRITICIDADES.map((c) => (
            <Chip key={c.valor} activo={crits.includes(c.valor)} tono={infoEstado(c.valor).tono} onClick={() => setFiltro('criticidad', alternarCriticidad(filtros, c.valor))}>
              {TEXTO_CRIT[c.valor]}
            </Chip>
          ))}
        </div>
        <Desplegable
          etiqueta="Más filtros"
          alinear="izq"
          disparador={({ alternar, ref, props, abierto }) => (
            <Boton ref={ref} variante="secundario" tamano="sm" icono="ajustes" onClick={alternar} {...props} className={abierto ? 'bg-fondo' : ''}>
              Filtros{activosMenu.length ? ` · ${activosMenu.length}` : ''}
            </Boton>
          )}
        >
          <Campo etiqueta="Categoría" tipo="select" valor={filtros.categoria || ''} onCambio={(v) => setFiltro('categoria', v)} opciones={[{ valor: '', etiqueta: 'Todas' }, ...CATEGORIAS]} />
          <Campo etiqueta="Responsable" tipo="select" valor={filtros.responsable_id || ''} onCambio={(v) => setFiltro('responsable_id', v)} opciones={[{ valor: '', etiqueta: 'Todos' }, ...responsables.map((r) => ({ valor: String(r.id), etiqueta: r.nombre }))]} />
          <div className="grid grid-cols-2 gap-2">
            <Campo etiqueta="Desde" tipo="fecha" valor={filtros.desde || ''} onCambio={(v) => setFiltro('desde', v)} />
            <Campo etiqueta="Hasta" tipo="fecha" valor={filtros.hasta || ''} onCambio={(v) => setFiltro('hasta', v)} />
          </div>
        </Desplegable>
        {hayFiltros(filtros) && (
          <Boton variante="fantasma" tamano="sm" onClick={limpiar} icono="cerrar">
            Limpiar
          </Boton>
        )}
        <Boton variante="secundario" tamano="sm" icono="descargar" href={urlApi(`/mantenimiento/incidencias/exportar?${filtrosAURL(filtros).toString()}`)}>
          Exportar
        </Boton>
        {s.tiene('roles.administrar') && (
          <Boton variante="secundario" tamano="sm" icono="ajustes" onClick={() => setConfigura(true)}>
            Configurar
          </Boton>
        )}
        <p className="ml-auto hidden text-xs text-texto-apoyo lg:block" aria-live="polite">
          {visibles.length} {visibles.length === 1 ? 'trabajo' : 'trabajos'}
          {hayFiltros(filtros) ? ' con estos filtros' : ''}
        </p>
      </div>
      {activosMenu.length > 0 && (
        <div className="flex flex-wrap gap-1.5" aria-label="Filtros activos">
          {activosMenu.map((f) => (
            <Chip key={f.k} onQuitar={() => setFiltro(f.k, null)} etiquetaQuitar={`Quitar filtro: ${f.texto}`}>
              {f.texto}
            </Chip>
          ))}
        </div>
      )}
    </div>
  );

  const tarjeta = (inc) => {
    const acc = permitidas(inc);
    const siguiente = acc.find((a) => !esSalida(a));
    const otras = acc.filter((a) => a !== siguiente);
    const tono = inc.criticidad ? infoEstado(inc.criticidad).tono : null;
    const monto = inc.estado === 'terminado' ? inc.costo_real_cts ?? inc.monto_cts : inc.monto_cts;
    const levantada = arrastrando?.id === inc.id;
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
        className={[
          'group relative flex flex-col gap-1 rounded-control border bg-superficie p-2.5 text-xs transition-[transform,box-shadow,opacity] duration-media',
          tono === 'alerta' ? 'border-alerta-borde' : 'border-borde',
          acc.length ? 'cursor-grab active:cursor-grabbing' : '',
          levantada ? 'rotate-2 opacity-80 shadow-flotante' : 'hover:border-borde-fuerte',
          moviendo === inc.id ? 'opacity-60' : '',
          asentada === inc.id ? 'animate-asentar' : '',
          rechazada === inc.id ? 'animate-temblor' : '',
        ].join(' ')}
      >
        <div className="flex items-center justify-between gap-2">
          <span className="flex min-w-0 items-center gap-1.5 text-texto-apoyo">
            {inc.criticidad ? <PuntoEstado estado={inc.criticidad} texto={<span className="sr-only">{TEXTO_CRIT[inc.criticidad]}</span>} className="-mr-1" /> : null}
            <span className="font-semibold text-texto-suave">{inc.codigo}</span>
            {tono === 'alerta' && <span className="font-semibold text-alerta">· Crítico</span>}
            {/* operacion: G2 · semáforo del SLA (solo tickets abiertos) */}
            {inc.semaforo && inc.semaforo !== 'cerrado' && (
              <span title={restanteSLA(inc.sla_restante_min)} className={`ml-1 inline-flex items-center gap-1 ${TONO_TEXTO[infoSemaforo(inc.semaforo).tono]}`}>
                <span className={`h-1.5 w-1.5 rounded-chip ${TONO_PUNTO[infoSemaforo(inc.semaforo).tono]}`} aria-hidden="true" />
                {infoSemaforo(inc.semaforo).texto}
              </span>
            )}
          </span>
          <span className="flex items-center">
            {acc.length > 0 && <Icono nombre="arrastrar" tam={14} className="hidden text-texto-apoyo lg:block" />}
            <MenuAcciones
              etiqueta={`Acciones de ${inc.codigo}`}
              variante="fantasma"
              className="-my-2 -mr-2 scale-90"
              items={[
                { etiqueta: 'Ver detalle', icono: 'ver', onClick: () => setDetalle(inc) },
                ...acc.map((a) => ({ etiqueta: ACCION_HACIA[a], icono: esSalida(a) ? (a === 'descartado' ? 'archivado' : 'cancelado') : infoEstado(a).icono, peligro: esSalida(a), onClick: () => mover(inc, a), deshabilitado: moviendo === inc.id })),
              ]}
            />
          </span>
        </div>
        <button type="button" onClick={() => setDetalle(inc)} className="text-left">
          <b className="line-clamp-2 text-sm leading-snug text-tinta" title={inc.titulo}>
            {inc.titulo}
          </b>
        </button>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-texto-apoyo">
          {campos.monto && monto != null && <span className="font-semibold tabular-nums text-texto-suave">{formatearSoles(monto)}</span>}
          {inc.estado === 'terminado' && campos.monto && monto != null && <span>en balance</span>}
          {campos.responsable && inc.responsable_nombre && <span className="truncate">{inc.responsable_nombre}</span>}
          {campos.votos && inc.votos && inc.estado === 'presupuestado' ? (
            <span className="inline-flex items-center gap-1 font-semibold text-aviso-texto" title="Espera a la junta">
              <Icono nombre="junta" tam={13} /> {inc.votos.a_favor}/{inc.votos.necesarios ?? '?'} votos
            </span>
          ) : monto == null && (campos.fotos || campos.antiguedad) ? (
            <span className="inline-flex items-center gap-1">
              {campos.fotos && (<><Icono nombre="camara" tam={13} /> {inc.n_fotos || 0}</>)}
              {campos.fotos && campos.antiguedad && <span aria-hidden="true">·</span>}
              {campos.antiguedad && haceCuanto(inc.reportado_en)}
            </span>
          ) : null}
        </div>
        {inc.avance_pct != null && (
          <span className="mt-0.5 h-1 overflow-hidden rounded-chip bg-borde" role="progressbar" aria-valuenow={inc.avance_pct} aria-valuemin={0} aria-valuemax={100} aria-label={`Avance ${inc.avance_pct} %`}>
            <span className="block h-1 rounded-chip bg-curso transition-[width] duration-lenta" style={{ width: `${inc.avance_pct}%` }} />
          </span>
        )}
        {siguiente && (
          <button
            type="button"
            onClick={() => mover(inc, siguiente)}
            disabled={moviendo === inc.id}
            className="mt-0.5 inline-flex min-h-[32px] items-center justify-center gap-1 self-start rounded-control border border-acento-borde bg-acento-suave px-2 text-xs font-semibold text-acento transition-[opacity,background-color,color] duration-rapida hover:bg-acento hover:text-white disabled:opacity-50 shadow-flotante [@media(hover:hover)]:absolute [@media(hover:hover)]:bottom-2 [@media(hover:hover)]:right-2 [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-focus-within:opacity-100 [@media(hover:hover)]:group-hover:opacity-100 [@media(hover:none)]:shadow-none"
          >
            {ACCION_HACIA[siguiente]} <Icono nombre="der" tam={12} grosor={2.25} />
          </button>
        )}
        {otras.length > 0 && !siguiente && <span className="sr-only">Más acciones en el menú de la tarjeta.</span>}
      </article>
    );
  };

  // movil: pestaña del celular (sin caja). fijo: carrusel de la tablet (248 px con imán).
  // En escritorio la columna es fluida: la rejilla la estira y las 6 del flujo caben a 1440 sin scroll.
  const columna = (col, i, movil = false, fijo = true) => {
    const items = grupos[col.estado] || [];
    const valida = arrastrando && arrastrando.estado !== col.estado && puedeTransicionar(arrastrando.estado, col.estado, arrastrando.transiciones) && s.tiene(PERMISO_HACIA[col.estado]);
    const invalida = arrastrando && arrastrando.estado !== col.estado && !valida;
    const suma = items.reduce((a, x) => a + (Number(x.estado === 'terminado' ? x.costo_real_cts ?? x.monto_cts : x.monto_cts) || 0), 0);
    const info = infoEstado(col.estado);
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
        className={[
          'flex min-w-0 flex-col rounded-tarjeta transition-[background-color,box-shadow,opacity] duration-media',
          movil ? '' : fijo ? 'max-h-[calc(100dvh-230px)] w-[248px] shrink-0 overflow-y-auto bg-superficie-2/60' : 'max-h-[calc(100dvh-230px)] overflow-y-auto bg-superficie-2/60',
          sobre === col.estado ? 'bg-acento-suave ring-2 ring-acento' : valida ? 'bg-acento-suave/60 ring-1 ring-acento-borde' : invalida ? 'opacity-50' : '',
        ].join(' ')}
      >
        {!movil && (
          <h2 className="sticky top-0 z-10 flex items-center justify-between gap-2 rounded-t-tarjeta bg-superficie-2 px-3 py-2 text-xs font-semibold text-texto-suave backdrop-blur">
            <span className="flex items-center gap-1.5">
              <span className={`h-2 w-2 rounded-chip ${TONO_PUNTO[info.tono]}`} aria-hidden="true" />
              {col.etiqueta}
            </span>
            <span className="rounded-chip bg-superficie px-1.5 tabular-nums">{items.length}</span>
          </h2>
        )}
        <div className={`flex flex-col gap-1.5 ${movil ? '' : 'px-1.5 pb-1.5'}`}>
          {items.length === 0 ? <p className="rounded-control border border-dashed border-borde px-3 py-5 text-center text-xs text-texto-apoyo">{valida ? 'Suelta aquí' : 'Sin trabajos'}</p> : items.map(tarjeta)}
        </div>
        {suma > 0 && (
          <p className={`mt-auto flex justify-between px-3 py-2 text-xs text-texto-apoyo ${movil ? '' : 'sticky bottom-0 rounded-b-tarjeta bg-superficie-2'}`}>
            <span>Suma</span>
            <b className="tabular-nums text-texto-suave" title={formatearSoles(suma)}>
              {formatearSolesCorto(suma)}
            </b>
          </p>
        )}
      </section>
    );
  };

  const acciones = (
    <Guarda permiso="incidencias.reportar">
      <Boton icono="camara" href={ruta('mantenimiento', { reportar: 1 })}>
        Registrar incidencia
      </Boton>
    </Guarda>
  );
  const cuerpo = (
    <>
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
          <div className="rounded-tarjeta border border-borde bg-superficie">
            <Vacio titulo="Sin trabajos este mes" texto="Así da gusto. Cuando alguien reporte una incidencia, aparecerá aquí." icono="herramienta" compacto>
              <Guarda permiso="incidencias.reportar">
                <Boton icono="camara" href={ruta('mantenimiento', { reportar: 1 })}>
                  Registrar incidencia
                </Boton>
              </Guarda>
            </Vacio>
          </div>
        ) : (
          <>
            {/* Tablet: columnas con desplazamiento horizontal e imán por columna */}
            <div className="carrusel -mx-4 hidden items-start gap-2 px-4 pb-2 md:flex lg:hidden" role="list" aria-label="Columnas del tablero">
              {columnas.map((c, i) => columna(c, i))}
            </div>
            {/* Escritorio: el flujo en rejilla fluida y las salidas debajo (según la configuración) */}
            <div className="hidden items-start gap-2 lg:grid lg:grid-cols-6" role="list" aria-label="Columnas del tablero">
              {flujo.map((c, i) => columna(c, i, false, false))}
            </div>
            {salidas.length > 0 && (
              <div className="hidden items-start gap-2 lg:grid lg:grid-cols-2" role="list" aria-label="Salidas del tablero">
                {salidas.map((c, i) => columna(c, i, false, false))}
              </div>
            )}
            <p className="hidden text-xs text-texto-apoyo lg:block">Arrastra una tarjeta, usa «siguiente paso» o su menú ⋯ para cambiarla de columna.</p>
            {/* Móvil: pestañas por estado, con contador */}
            <div className="flex flex-col gap-3 md:hidden">
              <p className="text-xs text-texto-apoyo" aria-live="polite">
                {visibles.length} {visibles.length === 1 ? 'trabajo' : 'trabajos'}
                {hayFiltros(filtros) ? ' con estos filtros' : ''}
              </p>
              <div role="tablist" aria-label="Estado" className="carrusel -mx-4 gap-2 px-4 pb-1">
                {columnas.map((c) => {
                  const n = (grupos[c.estado] || []).length;
                  const act = colMovil === c.estado;
                  return (
                    <button
                      key={c.estado}
                      type="button"
                      role="tab"
                      aria-selected={act}
                      onClick={() => setQuery({ col: c.estado }, { reemplazar: true })}
                      className={`inline-flex h-11 shrink-0 items-center gap-1.5 rounded-chip border px-3.5 text-sm font-semibold transition-colors duration-rapida ${act ? 'border-tinta bg-tinta text-white' : 'border-borde-fuerte bg-superficie'}`}
                    >
                      <span className={`h-2 w-2 rounded-chip ${TONO_PUNTO[infoEstado(c.estado).tono]}`} aria-hidden="true" />
                      {c.etiqueta}
                      <span className={`rounded-chip px-1.5 text-xs tabular-nums ${act ? 'bg-superficie-oscura-2' : 'bg-superficie-2'}`}>{n}</span>
                    </button>
                  );
                })}
              </div>
              {columna(
                columnas.find((c) => c.estado === colMovil) || columnas[0],
                0,
                true,
              )}
            </div>
          </>
        )}
    </>
  );
  return (
    <>
      {dialogEl}
      {sinMarco ? cuerpo : (
        <>
          <Encabezado titulo="Mantenimiento e incidencias" acciones={acciones} />
          <Contenido className="lg:max-w-none">{cuerpo}</Contenido>
        </>
      )}
      <Detalle inc={detalle} onCerrar={() => setDetalle(null)} acciones={detalle ? permitidas(detalle) : []} onMover={mover} />
      {configura && (
        <ConfigurarTablero
          columnas={columnas.map((c) => c.estado)}
          tarjeta={campos}
          onCerrar={() => setConfigura(false)}
          onGuardar={async (cuerpo) => {
            await api.put('/mantenimiento/tablero/config', cuerpo);
            conf.recargar();
            setConfigura(false);
          }}
        />
      )}
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
                <span className={`h-1.5 rounded-full transition-colors duration-media ${hecho ? 'bg-acento' : actual ? (inc.criticidad === 'critica' ? 'bg-alerta' : 'bg-curso') : 'bg-borde'}`} />
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
              <dd className="flex items-center gap-1.5">
                <Icono nombre="junta" tam={14} className="text-aviso" />
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
