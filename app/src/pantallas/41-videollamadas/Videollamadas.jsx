import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Modal, Vacio, useDialog, useToast } from '../../ui/index.js';

// Estado calculado por el API → insignia (tono y texto propios).
const ESTADO = {
  programada: { estado: 'pendiente', tono: 'aviso', texto: 'Programada' },
  en_curso: { estado: 'hoy', tono: 'curso', texto: 'En curso' },
  finalizada: { estado: 'terminado', texto: 'Finalizada' },
  cancelada: { estado: 'cancelada', texto: 'Cancelada' },
};

const DURACIONES = [30, 45, 60, 90, 120, 180].map((m) => ({ valor: String(m), etiqueta: `${m} min` }));

/**
 * Bloques J2 e I3 · Reuniones de la junta por videollamada (enlace Jitsi) y, para la
 * administración, el dominio propio de la administradora. Dos vistas en una pantalla.
 */
export default function Videollamadas() {
  const s = useSesion();
  const veSalas = s.tiene?.('videollamadas.ver');
  const veDominios = s.tiene?.('dominios.administrar');
  const [vista, setVista] = useState(veSalas ? 'salas' : 'dominio');
  const { dialog, dialogEl } = useDialog();

  return (
    <>
      {dialogEl}
      {vista === 'salas' && veSalas ? <Salas dialog={dialog} pestanas={<Pestanas vista={vista} setVista={setVista} veSalas={veSalas} veDominios={veDominios} />} /> : null}
      {vista === 'dominio' && veDominios ? <Dominios dialog={dialog} pestanas={<Pestanas vista={vista} setVista={setVista} veSalas={veSalas} veDominios={veDominios} />} /> : null}
    </>
  );
}

function Pestanas({ vista, setVista, veSalas, veDominios }) {
  if (!(veSalas && veDominios)) return null;
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Chip activo={vista === 'salas'} icono="junta" onClick={() => setVista('salas')}>Reuniones</Chip>
      <Chip activo={vista === 'dominio'} icono="engranaje" onClick={() => setVista('dominio')}>Dominio propio</Chip>
    </div>
  );
}

/** J2 · salas junta ↔ administración. */
function Salas({ dialog, pestanas }) {
  const eid = useEid();
  const s = useSesion();
  const admin = s.tiene?.('videollamadas.administrar');
  const { toast } = useToast();
  const salas = useCarga(() => api.get(`/edificios/${eid}/videollamadas`), [eid]);
  const filas = lista(salas.datos);
  const [form, setForm] = useState(null);
  const [ocupado, setOcupado] = useState(false);

  const nueva = () => {
    const man = new Date(Date.now() + 24 * 3600 * 1000);
    const iso = new Date(man.getTime() - man.getTimezoneOffset() * 60000).toISOString().slice(0, 10);
    setForm({ titulo: '', descripcion: '', fecha: iso, hora: '19:00', duracion_min: '60' });
  };

  const guardar = async (e) => {
    e?.preventDefault?.();
    if (!/^\d{2}:\d{2}$/.test(form.hora || '')) {
      await dialog.alert({ title: 'Revisa la hora', text: 'Escribe la hora como HH:MM (por ejemplo 19:30).' });
      return;
    }
    setOcupado(true);
    try {
      const r = await api.post(`/edificios/${eid}/videollamadas`, {
        titulo: form.titulo,
        descripcion: form.descripcion,
        inicia_en: `${form.fecha}T${form.hora}`,
        duracion_min: Number(form.duracion_min),
      });
      toast('Reunión convocada. Comparte el enlace con la junta.', { tipo: 'exito' });
      setForm(null);
      await salas.recargar();
      await copiar(r.enlace, true);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo convocar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const copiar = async (enlace, silencioso = false) => {
    try {
      await navigator.clipboard.writeText(enlace);
      toast('Enlace copiado.', { tipo: 'exito' });
    } catch {
      if (!silencioso) await dialog.alert({ title: 'Enlace de la sala', text: enlace });
    }
  };

  const cancelar = async (v) => {
    if (!(await dialog.confirm({ title: '¿Cancelar la reunión?', text: `«${v.titulo}» quedará cancelada; el enlace deja de mostrarse.`, danger: true }))) return;
    try {
      await api.post(`/edificios/${eid}/videollamadas/${v.id}/cancelar`);
      await salas.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo cancelar', text: err.message });
    }
  };

  const grabacion = async (v) => {
    const url = await dialog.prompt({ title: 'Enlace de la grabación', label: 'URL (https://…)', defaultValue: v.grabacion_url || '', placeholder: 'https://' });
    if (url === null) return;
    try {
      await api.put(`/edificios/${eid}/videollamadas/${v.id}/grabacion`, { url });
      toast(url ? 'Grabación enlazada.' : 'Enlace de grabación quitado.', { tipo: 'exito' });
      await salas.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo guardar', text: err.message });
    }
  };

  return (
    <>
      <Encabezado
        titulo="Videollamadas"
        subtitulo="Reuniones de la junta con la administración"
        ayuda="Cada reunión tiene su sala propia con un enlace difícil de adivinar. Solo la junta y la administración ven el enlace. La grabación es opcional: pega aquí el enlace cuando la tengas."
        acciones={admin && <Boton icono="mas_signo" onClick={nueva}>Convocar reunión</Boton>}
      />
      <Contenido>
        {pestanas}
        <Seccion titulo="Reuniones" extra={salas.datos?.base_url && <span className="text-xs text-texto-apoyo">Servidor: {salas.datos.base_url}</span>}>
          {salas.error ? (
            <ErrorCarga error={salas.error} onReintentar={salas.recargar} />
          ) : !salas.datos ? (
            <Esqueleto className="h-40 w-full" />
          ) : filas.length === 0 ? (
            <Vacio icono="junta" titulo="Sin reuniones" texto="Convoca la primera sesión de la junta por videollamada." />
          ) : (
            <ul className="divide-y divide-borde">
              {filas.map((v) => {
                const est = ESTADO[v.estado] || ESTADO.programada;
                const [dia, hora] = String(v.inicia_en || '').split('T');
                const vigente = v.estado === 'programada' || v.estado === 'en_curso';
                return (
                  <li key={v.id} className="flex flex-wrap items-center gap-3 py-3">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-tinta">{v.titulo}</p>
                      <p className="truncate text-xs text-texto-apoyo">
                        {[`${formatearFecha(dia)} ${hora || ''}`.trim(), `${v.duracion_min} min`, v.convoca && `convoca ${v.convoca}`].filter(Boolean).join(' · ')}
                      </p>
                      {v.descripcion && <p className="mt-0.5 line-clamp-2 text-xs text-texto-suave">{v.descripcion}</p>}
                    </div>
                    <Insignia estado={est.estado} tono={est.tono} texto={est.texto} />
                    {vigente && (
                      <>
                        <Boton tamano="sm" href={v.enlace} target="_blank" rel="noreferrer">Entrar</Boton>
                        <Boton tamano="sm" variante="fantasma" onClick={() => copiar(v.enlace)}>Copiar enlace</Boton>
                      </>
                    )}
                    {v.grabacion_url && (
                      <a href={v.grabacion_url} target="_blank" rel="noreferrer" className="text-sm text-acento hover:text-acento-hover">Grabación</a>
                    )}
                    {admin && v.estado !== 'cancelada' && (
                      <Boton tamano="sm" variante="fantasma" onClick={() => grabacion(v)}>{v.grabacion_url ? 'Cambiar grabación' : 'Enlazar grabación'}</Boton>
                    )}
                    {admin && vigente && (
                      <Boton tamano="sm" variante="fantasma" onClick={() => cancelar(v)}>Cancelar</Boton>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </Seccion>
      </Contenido>

      <Modal
        abierto={!!form}
        onCerrar={() => setForm(null)}
        titulo="Convocar reunión"
        ancho="max-w-lg"
        pie={<><Boton variante="fantasma" onClick={() => setForm(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardar}>Convocar</Boton></>}
      >
        {form && (
          <form className="flex flex-col gap-3" onSubmit={guardar}>
            <Campo etiqueta="Asunto" valor={form.titulo} onCambio={(v) => setForm({ ...form, titulo: v })} />
            <Campo etiqueta="Agenda" tipo="textarea" valor={form.descripcion} onCambio={(v) => setForm({ ...form, descripcion: v })} />
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <Campo etiqueta="Fecha" tipo="fecha" valor={form.fecha} onCambio={(v) => setForm({ ...form, fecha: v })} />
              <Campo etiqueta="Hora (Lima)" valor={form.hora} onCambio={(v) => setForm({ ...form, hora: v })} placeholder="19:00" />
              <Campo etiqueta="Duración" tipo="select" valor={form.duracion_min} onCambio={(v) => setForm({ ...form, duracion_min: v })} opciones={DURACIONES} />
            </div>
          </form>
        )}
      </Modal>
    </>
  );
}

/** I3 · dominios propios de la administradora. EDISYS no toca DNS: aquí solo se registra el host. */
function Dominios({ dialog, pestanas }) {
  const eid = useEid();
  const { toast } = useToast();
  const doms = useCarga(() => api.get(`/edificios/${eid}/dominios`), [eid]);
  const filas = lista(doms.datos);

  const agregar = async () => {
    const host = await dialog.prompt({ title: 'Registrar dominio', label: 'Dominio', placeholder: 'intranet.tuadministradora.pe', required: true });
    if (!host) return;
    try {
      await api.post(`/edificios/${eid}/dominios`, { host });
      toast('Dominio registrado. Falta apuntar el DNS al servidor.', { tipo: 'exito' });
      await doms.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar', text: err.message });
    }
  };

  const alternar = async (d) => {
    try {
      await api.patch(`/edificios/${eid}/dominios/${d.id}`, { activo: !d.activo });
      await doms.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo cambiar', text: err.message });
    }
  };

  const borrar = async (d) => {
    if (!(await dialog.confirm({ title: '¿Quitar el dominio?', text: `${d.host} dejará de servir EDISYS y no se renovará su certificado.`, danger: true }))) return;
    try {
      await api.del(`/edificios/${eid}/dominios/${d.id}`);
      await doms.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo quitar', text: err.message });
    }
  };

  return (
    <>
      <Encabezado
        titulo="Dominio propio"
        subtitulo="Sirve EDISYS con el dominio de tu administradora"
        ayuda="Registra el dominio aquí y luego apunta su DNS (registro A o CNAME) al servidor de EDISYS. El certificado HTTPS lo saca el servidor solo, y únicamente para dominios registrados y activos. En un dominio propio solo inician sesión los usuarios de tu administradora."
        acciones={<Boton icono="mas_signo" onClick={agregar}>Registrar dominio</Boton>}
      />
      <Contenido>
        {pestanas}
        <Seccion titulo="Dominios" extra={doms.datos?.host_actual && <span className="text-xs text-texto-apoyo">Estás en: {doms.datos.host_actual}</span>}>
          {doms.error ? (
            <ErrorCarga error={doms.error} onReintentar={doms.recargar} />
          ) : !doms.datos ? (
            <Esqueleto className="h-32 w-full" />
          ) : filas.length === 0 ? (
            <Vacio icono="edificio" titulo="Sin dominio propio" texto="Mientras no registres uno, EDISYS se usa en el dominio de la plataforma." />
          ) : (
            <ul className="divide-y divide-borde">
              {filas.map((d) => (
                <li key={d.id} className="flex flex-wrap items-center gap-3 py-2.5">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-tinta">{d.host}</p>
                    <p className="text-xs text-texto-apoyo">Registrado el {formatearFecha(d.fecha)}</p>
                  </div>
                  <Insignia estado={d.activo ? 'activo' : 'inactivo'} />
                  <Boton tamano="sm" variante="fantasma" onClick={() => alternar(d)}>{d.activo ? 'Desactivar' : 'Activar'}</Boton>
                  <Boton tamano="sm" variante="fantasma" onClick={() => borrar(d)}>Quitar</Boton>
                </li>
              ))}
            </ul>
          )}
        </Seccion>
      </Contenido>
    </>
  );
}
