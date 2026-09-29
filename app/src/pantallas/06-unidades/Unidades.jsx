import { useEffect, useState } from 'react';
import { api, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearPct } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { enmascararDni } from '../../lib/participacion.js';
import { useQuery } from '../../lib/nav.jsx';
import { useEid, Guarda } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton, Tabla, Insignia, Modal, ErrorCarga, Esqueleto, Vacio, Icono, Chip, PuntoEstado } from '../../ui/index.js';
import Importar from './Importar.jsx';

const POR_PAGINA = 25;

const nombreDe = (p) => (p && typeof p === 'object' ? p.nombre : p) || '';

/** 06 · Unidades y propietarios, con la importación desde Excel en la pestaña «Importar Excel» (?tab=importar). */
export default function Unidades() {
  const eid = useEid();
  const [q, setQuery] = useQuery();
  const tab = q.get('tab') === 'importar' ? 'importar' : 'unidades';
  const pagina = Number(q.get('pagina') || 1);
  const buscar = q.get('buscar') || '';
  const [texto, setTexto] = useState(buscar);
  const [detalleId, setDetalleId] = useState(null);
  const soloMorosos = q.get('morosos') === '1';

  useEffect(() => {
    const t = setTimeout(() => texto !== buscar && setQuery({ buscar: texto, pagina: null }, { reemplazar: true }), 350);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [texto]);

  const lista = useCarga(() => api.get(`/edificios/${eid}/unidades`, { buscar, pagina, por_pagina: POR_PAGINA, morosos: soloMorosos ? 1 : undefined }), [eid, buscar, pagina, soloMorosos], { activo: tab === 'unidades' });
  // «Solo morosos» lo filtra el API (deuda vencida, igual que la morosidad); aquí ya viene filtrado.
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
            className={`h-11 whitespace-nowrap border-b-2 px-3 text-sm font-semibold transition-colors duration-rapida ${tab === t.id ? 'border-acento text-acento' : 'border-transparent text-texto-suave hover:text-tinta'}`}
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
    { clave: 'codigo', titulo: 'Unidad', render: (u) => `Dpto ${u.codigo}`, movil: 'titulo', className: 'whitespace-nowrap' },
    // El API manda propietario/inquilino como objeto { nombre, dni_ruc, celular, … }; el mock, como texto.
    { clave: 'propietario', titulo: 'Propietario', movil: 'sub', render: (u) => nombreDe(u.propietario) },
    { clave: 'tipo', titulo: 'Tipo', render: (u) => (u.tipo ? u.tipo.charAt(0).toUpperCase() + u.tipo.slice(1) : '—'), movil: 'oculto', prioridad: 3 },
    { clave: 'propietario_dni', titulo: 'DNI / RUC', render: (u) => enmascararDni(u.propietario?.dni_ruc ?? u.propietario_dni), prioridad: 2, className: 'whitespace-nowrap' },
    { clave: 'celular', titulo: 'Celular', render: (u) => u.propietario?.celular || u.celular || '—', movil: 'oculto', prioridad: 3, className: 'whitespace-nowrap' },
    { clave: 'inquilino', titulo: 'Inquilino', render: (u) => nombreDe(u.inquilino) || <span className="text-texto-apoyo">—</span>, prioridad: 2 },
    { clave: 'participacion_pct', titulo: 'Particip.', alinear: 'der', render: (u) => formatearPct(u.participacion_pct, 2), className: 'whitespace-nowrap' },
    {
      clave: 'deuda_cts',
      titulo: 'Deuda',
      alinear: 'der',
      movil: 'valor',
      className: 'whitespace-nowrap',
      render: (u) =>
        u.deuda_cts > 0 ? (
          <PuntoEstado estado="moroso" texto={<b className="tabular-nums text-alerta">{formatearSoles(u.deuda_cts)}</b>} className="justify-end" />
        ) : (
          <span className="text-texto-apoyo">Al día</span>
        ),
    },
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
            <div className="flex flex-wrap items-center gap-2" role="search" aria-label="Filtrar unidades">
              <label className="relative min-w-[200px] max-w-md flex-1">
                <span className="sr-only">Buscar unidad o propietario</span>
                <Icono nombre="buscar" tam={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-texto-apoyo" />
                <input type="search" value={texto} onChange={(e) => setTexto(e.target.value)} placeholder="Buscar unidad o propietario" className="h-11 w-full rounded-control border border-borde-fuerte bg-superficie pl-9 pr-3 text-base transition-colors duration-rapida focus:border-acento focus:outline-none focus:ring-2 focus:ring-acento lg:h-8 lg:text-sm" />
              </label>
              <Chip activo={soloMorosos} tono="alerta" onClick={() => setQuery({ morosos: soloMorosos ? null : 1, pagina: null }, { reemplazar: true })}>
                Solo morosos
              </Chip>
            </div>
            <div className="overflow-hidden rounded-tarjeta border border-borde bg-superficie">
              <Tabla
                etiqueta="Unidades"
                columnas={columnas}
                filas={filas}
                cargando={lista.cargando}
                error={lista.error}
                onReintentar={lista.recargar}
                onFila={(u) => setDetalleId(u.id)}
                vacio={
                  buscar || soloMorosos ? (
                    <Vacio titulo={soloMorosos && !buscar ? 'Ninguna unidad morosa' : 'Ninguna unidad coincide'} texto="Prueba con otro filtro." icono="buscar" compacto />
                  ) : (
                    <Vacio titulo="Tu edificio aún no tiene unidades" texto="Cárgalas todas desde el Excel del padrón, sin registrar a nadie a mano." icono="edificio">
                      <Guarda permiso="unidades.importar">
                        <Boton icono="excel" onClick={() => setQuery({ tab: 'importar' })}>
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
              <div key={i} className="rounded-control border border-borde p-3">
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
