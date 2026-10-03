import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFechaHora } from '../../lib/fechas.js';
import { Enlace } from '../../lib/nav.jsx';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Icono, Insignia, Paginacion, Vacio, useDialog, useToast } from '../../ui/index.js';

/**
 * Bloque I5 · Configuración del edificio: puesta en marcha (qué falta configurar), estado activo
 * y registro de cambios (auditoría con antes y después).
 */
export default function Ajustes() {
  const [vista, setVista] = useState('asistente');
  return (
    <>
      <Encabezado
        titulo="Configuración del edificio"
        subtitulo="Puesta en marcha, estado y registro de cambios"
        ayuda="El asistente se calcula con los datos del edificio: cada paso se marca solo cuando está hecho."
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'asistente'} icono="hecho" onClick={() => setVista('asistente')}>Puesta en marcha</Chip>
          <Chip activo={vista === 'estado'} icono="edificio" onClick={() => setVista('estado')}>Estado</Chip>
          <Chip activo={vista === 'cambios'} icono="llave" onClick={() => setVista('cambios')}>Registro de cambios</Chip>
        </div>
        {vista === 'asistente' && <Asistente />}
        {vista === 'estado' && <Estado />}
        {vista === 'cambios' && <Cambios />}
      </Contenido>
    </>
  );
}

// ---------- asistente ----------

function Asistente() {
  const eid = useEid();
  const a = useCarga(() => api.get(`/edificios/${eid}/configuracion/asistentes`), [eid]);
  if (a.error) return <ErrorCarga error={a.error} onReintentar={a.recargar} />;
  if (!a.datos) return <Esqueleto className="h-64 w-full" />;
  const { pasos = [], hechos = 0, total = 0, porcentaje = 0 } = a.datos;
  return (
    <Seccion titulo="Puesta en marcha" extra={<span className="text-sm text-texto-apoyo">{hechos} de {total} listos</span>}>
      <div className="h-2 w-full overflow-hidden rounded-chip bg-superficie-2" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={porcentaje} aria-label="Avance de la configuración">
        <div className="h-full bg-acento transition-[width] duration-media" style={{ width: `${porcentaje}%` }} />
      </div>
      <ol className="divide-y divide-borde">
        {pasos.map((p, i) => (
          <li key={p.clave} className="flex items-center gap-3 py-2.5">
            <span className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-chip text-xs font-semibold ${p.hecho ? 'bg-acento text-white' : 'border border-borde-fuerte text-texto-apoyo'}`}>
              {p.hecho ? <Icono nombre="hecho" tam={14} grosor={2.5} /> : i + 1}
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium text-tinta">{p.titulo}</p>
              <p className="truncate text-xs text-texto-apoyo">{p.detalle}</p>
            </div>
            {!p.hecho && p.pagina && (
              <Enlace pagina={p.pagina} className="text-sm text-acento hover:text-acento-hover">Completar</Enlace>
            )}
          </li>
        ))}
      </ol>
    </Seccion>
  );
}

// ---------- estado ----------

function Estado() {
  const eid = useEid();
  const s = useSesion();
  const puedeEditar = s.tiene('configuracion.editar');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const e = useCarga(() => api.get(`/edificios/${eid}/configuracion/estado`), [eid]);
  const [ocupado, setOcupado] = useState(false);

  if (e.error) return <ErrorCarga error={e.error} onReintentar={e.recargar} />;
  if (!e.datos) return <Esqueleto className="h-40 w-full" />;
  const activo = e.datos.activo;

  const cambiar = async () => {
    let cuerpo;
    if (activo) {
      const motivo = await dialog.prompt({
        title: 'Desactivar el edificio',
        label: 'Motivo',
        text: 'Quedará en solo lectura: nadie podrá registrar pagos, recibos ni reservas hasta reactivarlo.',
        required: true,
        okText: 'Desactivar',
        danger: true,
      });
      if (motivo == null) return;
      cuerpo = { activo: false, motivo };
    } else {
      const ok = await dialog.confirm({ title: '¿Reactivar el edificio?', text: 'Se podrá volver a registrar todo.', okText: 'Reactivar' });
      if (!ok) return;
      cuerpo = { activo: true };
    }
    setOcupado(true);
    try {
      await api.put(`/edificios/${eid}/configuracion/activo`, cuerpo);
      toast(cuerpo.activo ? 'Edificio reactivado.' : 'Edificio desactivado.', { tipo: 'exito' });
      await e.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo cambiar', text: err.campos?.motivo || err.message });
    } finally {
      setOcupado(false);
    }
  };

  return (
    <>
      {dialogEl}
      <Seccion titulo="Estado del edificio">
        <div className="flex flex-wrap items-center gap-3">
          <Insignia estado={activo ? 'activo' : 'inactivo'} tam="md" />
          <span className="text-sm text-texto-suave">
            {activo
              ? 'El edificio opera con normalidad.'
              : `Desactivado el ${formatearFechaHora(e.datos.desactivado_en)}: ${e.datos.desactivado_motivo}. Solo se puede consultar.`}
          </span>
        </div>
        {puedeEditar && (
          <div>
            <Boton variante={activo ? 'peligro' : 'primario'} cargando={ocupado} onClick={cambiar}>
              {activo ? 'Desactivar edificio' : 'Reactivar edificio'}
            </Boton>
          </div>
        )}
      </Seccion>
    </>
  );
}

// ---------- registro de cambios ----------

const NOMBRE_MODULO = {
  configuracion: 'Configuración',
  edificio: 'Ficha del edificio',
  marca: 'Marca',
  recibos: 'Recibos',
  roles: 'Roles',
  facturacion: 'Facturación',
  whatsapp: 'WhatsApp',
  mantenimiento: 'Mantenimiento',
};

function valorLegible(v) {
  if (v === null || v === undefined || v === '') return '—';
  if (typeof v === 'boolean') return v ? 'sí' : 'no';
  if (typeof v === 'object') return JSON.stringify(v);
  return String(v);
}

function Cambios() {
  const eid = useEid();
  const [modulo, setModulo] = useState('');
  const [pagina, setPagina] = useState(1);
  const c = useCarga(() => api.get(`/edificios/${eid}/configuracion/cambios`, { modulo, pagina, por_pagina: 25 }), [eid, modulo, pagina]);
  const filas = lista(c.datos);
  const modulos = c.datos?.modulos || Object.keys(NOMBRE_MODULO);

  return (
    <Seccion
      titulo="Registro de cambios"
      extra={
        <Campo
          tipo="select"
          etiqueta="Módulo"
          ocultarEtiqueta
          className="w-48"
          valor={modulo}
          onCambio={(v) => {
            setModulo(v);
            setPagina(1);
          }}
          opciones={[{ valor: '', etiqueta: 'Todos los módulos' }, ...modulos.map((m) => ({ valor: m, etiqueta: NOMBRE_MODULO[m] || m }))]}
        />
      }
      padding="p-0"
    >
      {c.error ? (
        <ErrorCarga error={c.error} onReintentar={c.recargar} />
      ) : !c.datos ? (
        <Esqueleto className="m-4 h-40" />
      ) : filas.length === 0 ? (
        <Vacio icono="info" titulo="Sin cambios registrados" texto="Aquí aparece quién cambió la configuración, cuándo y qué valores tenía antes." compacto />
      ) : (
        <>
          <ul className="divide-y divide-borde">
            {filas.map((f) => (
              <li key={f.id} className="flex flex-col gap-1.5 px-4 py-3">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
                  <span className="font-semibold text-tinta">{NOMBRE_MODULO[f.modulo] || f.modulo}</span>
                  <span className="text-texto-suave">· {f.accion}</span>
                  <span className="ml-auto text-xs text-texto-apoyo">{f.usuario} · {formatearFechaHora(f.creado_en)}</span>
                </div>
                {f.cambios?.length > 0 ? (
                  <dl className="grid gap-x-3 gap-y-0.5 text-xs sm:grid-cols-[minmax(120px,auto)_1fr]">
                    {f.cambios.map((x) => (
                      <div key={x.campo} className="contents">
                        <dt className="font-mono text-texto-apoyo">{x.campo}</dt>
                        <dd className="min-w-0 break-words text-tinta">
                          <span className="text-texto-apoyo line-through">{valorLegible(x.antes)}</span> → {valorLegible(x.despues)}
                        </dd>
                      </div>
                    ))}
                  </dl>
                ) : (
                  <p className="text-xs text-texto-apoyo">Guardado sin cambios de valores.</p>
                )}
              </li>
            ))}
          </ul>
          <Paginacion pagina={pagina} porPagina={25} total={c.datos.total || 0} mostradas={filas.length} onPagina={setPagina} unidad="cambios" />
        </>
      )}
    </Seccion>
  );
}
