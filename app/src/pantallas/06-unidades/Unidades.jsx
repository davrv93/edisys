import { useEffect, useState } from 'react';
import { api, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearPct } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { enmascararDni } from '../../lib/participacion.js';
import { useQuery } from '../../lib/nav.jsx';
import { useEid, Guarda } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton, Tabla, Insignia, Modal, ErrorCarga, Esqueleto, Vacio, Icono } from '../../ui/index.js';
import Importar from './Importar.jsx';

const POR_PAGINA = 25;

/** 06 · Unidades y propietarios, con la importación desde Excel en la pestaña «Importar Excel» (?tab=importar). */
export default function Unidades() {
  const eid = useEid();
  const [q, setQuery] = useQuery();
  const tab = q.get('tab') === 'importar' ? 'importar' : 'unidades';
  const pagina = Number(q.get('pagina') || 1);
  const buscar = q.get('buscar') || '';
  const [texto, setTexto] = useState(buscar);
  const [detalleId, setDetalleId] = useState(null);

  useEffect(() => {
    const t = setTimeout(() => texto !== buscar && setQuery({ buscar: texto, pagina: null }, { reemplazar: true }), 350);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [texto]);

  const lista = useCarga(() => api.get(`/edificios/${eid}/unidades`, { buscar, pagina, por_pagina: POR_PAGINA }), [eid, buscar, pagina], { activo: tab === 'unidades' });
  const filas = lista.datos?.datos || [];
  const total = lista.datos?.total ?? filas.length;

  const pestanas = (
    <div role="tablist" aria-label="Secciones de unidades" className="flex gap-1 overflow-x-auto px-4 lg:px-8">
      {[
        { id: 'unidades', etiqueta: `Unidades${total && tab === 'unidades' ? ` · ${total}` : ''}` },
        { id: 'importar', etiqueta: 'Importar Excel', permiso: 'unidades.importar' },
      ].map((t) => (
        <Guarda key={t.id} permiso={t.permiso}>
          <button
            type="button"
            role="tab"
            aria-selected={tab === t.id}
            onClick={() => setQuery({ tab: t.id === 'unidades' ? null : t.id })}
            className={`h-12 whitespace-nowrap border-b-2 px-3 text-sm font-semibold ${tab === t.id ? 'border-acento text-acento' : 'border-transparent text-texto-suave hover:text-tinta'}`}
          >
            {t.etiqueta}
          </button>
        </Guarda>
      ))}
    </div>
  );

  const acciones = (
    <Guarda permiso="unidades.importar">
      <Boton variante="secundario" icono="descargar" href={urlApi(`/edificios/${eid}/importaciones/plantilla.xlsx`)}>
        Descargar plantilla Excel
      </Boton>
    </Guarda>
  );

  const columnas = [
    { clave: 'codigo', titulo: 'Unidad', render: (u) => `Dpto ${u.codigo}`, movil: 'titulo' },
    { clave: 'propietario', titulo: 'Propietario', movil: 'sub' },
    { clave: 'tipo', titulo: 'Tipo', render: (u) => (u.tipo ? u.tipo.charAt(0).toUpperCase() + u.tipo.slice(1) : '—'), movil: 'oculto' },
    { clave: 'propietario_dni', titulo: 'DNI / RUC', render: (u) => enmascararDni(u.propietario_dni) },
    { clave: 'celular', titulo: 'Celular', render: (u) => u.celular || '—', movil: 'oculto' },
    { clave: 'inquilino', titulo: 'Inquilino', render: (u) => u.inquilino || <span className="text-texto-apoyo">—</span> },
    { clave: 'participacion_pct', titulo: 'Participación', alinear: 'der', render: (u) => formatearPct(u.participacion_pct, 2) },
    { clave: 'deuda_cts', titulo: 'Deuda', alinear: 'der', movil: 'valor', render: (u) => (u.deuda_cts > 0 ? <Insignia estado="moroso" texto={formatearSoles(u.deuda_cts)} /> : <Insignia estado="al_dia" />) },
  ];

  return (
    <>
      <Encabezado titulo="Unidades y propietarios" acciones={acciones}>
        {pestanas}
      </Encabezado>
      <Contenido>
        {tab === 'importar' ? (
          <Importar eid={eid} onVerUnidades={() => setQuery({ tab: null })} />
        ) : (
          <>
            <label className="relative max-w-md">
              <span className="sr-only">Buscar unidad o propietario</span>
              <Icono nombre="buscar" tam={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-texto-apoyo" />
              <input type="search" value={texto} onChange={(e) => setTexto(e.target.value)} placeholder="Buscar unidad o propietario" className="h-11 w-full rounded-lg border border-borde-fuerte bg-superficie pl-9 pr-3 text-base focus:outline-none focus:ring-2 focus:ring-acento sm:text-sm" />
            </label>
            <div className="overflow-hidden rounded-xl border border-borde bg-superficie">
              <Tabla
                etiqueta="Unidades"
                columnas={columnas}
                filas={filas}
                cargando={lista.cargando}
                error={lista.error}
                onReintentar={lista.recargar}
                onFila={(u) => setDetalleId(u.id)}
                vacio={
                  buscar ? (
                    <Vacio titulo="Ninguna unidad coincide" compacto />
                  ) : (
                    <Vacio titulo="Tu edificio aún no tiene unidades" texto="Cárgalas todas desde el Excel del padrón, sin registrar a nadie a mano." icono="edificio">
                      <Guarda permiso="unidades.importar">
                        <Boton icono="subir" onClick={() => setQuery({ tab: 'importar' })}>
                          Importar Excel
                        </Boton>
                      </Guarda>
                    </Vacio>
                  )
                }
                paginacion={total > POR_PAGINA ? { pagina, porPagina: POR_PAGINA, total, onPagina: (p) => setQuery({ pagina: p }), unidad: 'unidades' } : undefined}
              />
            </div>
            <p className="text-xs text-texto-apoyo">Los DNI se muestran enmascarados (Ley 29733). Toca una unidad para ver sus personas, su historial y sus medidores.</p>
          </>
        )}
      </Contenido>
      <DetalleUnidad eid={eid} id={detalleId} onCerrar={() => setDetalleId(null)} />
    </>
  );
}

function DetalleUnidad({ eid, id, onCerrar }) {
  const { datos: u, error, recargar } = useCarga(() => api.get(`/edificios/${eid}/unidades/${id}`), [eid, id], { activo: id != null });
  return (
    <Modal abierto={id != null} onCerrar={onCerrar} titulo={u ? `Dpto ${u.codigo}` : 'Unidad'} lateral>
      {error ? (
        <ErrorCarga error={error} onReintentar={recargar} compacto />
      ) : !u ? (
        <Esqueleto className="h-48 w-full" />
      ) : (
        <div className="flex flex-col gap-5 text-sm">
          <dl className="grid grid-cols-2 gap-2">
            <dt className="text-texto-apoyo">Tipo</dt>
            <dd className="capitalize">{u.tipo}</dd>
            <dt className="text-texto-apoyo">Piso</dt>
            <dd>{u.piso}</dd>
            <dt className="text-texto-apoyo">Participación</dt>
            <dd className="tabular-nums">{formatearPct(u.participacion_pct, 4)}</dd>
          </dl>
          <section className="flex flex-col gap-2">
            <h3 className="font-semibold">Personas</h3>
            {(u.personas || []).map((p, i) => (
              <div key={i} className="rounded-lg border border-borde p-3">
                <div className="flex items-center justify-between gap-2">
                  <b>{p.nombre}</b>
                  <Insignia estado="activo" texto={p.rol === 'inquilino' ? 'Inquilino' : 'Propietario'} />
                </div>
                <div className="text-texto-apoyo">
                  {[p.dni && `DNI ${enmascararDni(p.dni)}`, p.celular, p.correo].filter(Boolean).join(' · ')}
                </div>
                <div className="text-xs text-texto-apoyo">Desde {formatearFecha(p.desde)}</div>
              </div>
            ))}
          </section>
          {u.historial?.length > 0 && (
            <section className="flex flex-col gap-2">
              <h3 className="font-semibold">Historial</h3>
              {u.historial.map((p, i) => (
                <div key={i} className="flex justify-between gap-2 border-b border-superficie-2 py-2 text-texto-suave">
                  <span>
                    {p.nombre} · {p.rol}
                  </span>
                  <span className="tabular-nums">
                    {formatearFecha(p.desde)} → {formatearFecha(p.hasta)}
                  </span>
                </div>
              ))}
            </section>
          )}
          {u.medidores?.length > 0 && (
            <section className="flex flex-col gap-2">
              <h3 className="font-semibold">Medidores</h3>
              {u.medidores.map((m) => (
                <div key={m.serie} className="flex justify-between">
                  <span>
                    {m.serie} · {m.tipo}
                  </span>
                  <span className="text-texto-apoyo">lectura inicial {m.lectura_inicial}</span>
                </div>
              ))}
            </section>
          )}
        </div>
      )}
    </Modal>
  );
}
