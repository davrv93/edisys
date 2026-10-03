import { useState } from 'react';
import { api, lista, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFechaHora } from '../../lib/fechas.js';
import { useQuery } from '../../lib/nav.jsx';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, Insignia, Modal, Tabla, Vacio, useDialog, useToast } from '../../ui/index.js';

const ESTADOS = [
  ['', 'Todos'],
  ['pendiente', 'Pendientes'],
  ['enviado', 'Enviados'],
  ['simulado', 'Simulados'],
  ['error', 'Con error'],
];
const ORIGEN = { manual: 'Manual', recibo: 'Recibo', balance: 'Balance', sistema: 'Sistema', anuncio: 'Anuncio' };

/** Conteo por estado → { estado: cantidad } (los chips muestran el número). */
function porEstado(conteos) {
  const m = {};
  for (const c of conteos || []) m[c.estado] = c.cantidad;
  m[''] = Object.values(m).reduce((a, b) => a + b, 0);
  return m;
}

/**
 * Bloques E3 y E4 · Bandeja de salida: correos (con su detalle, adjuntos y reintento) y Telegram
 * (bandeja, destinos del bot y envío). Solo se reintenta lo que terminó en error: nada sale dos veces.
 */
export default function Bandeja() {
  const s = useSesion();
  const veCorreo = s.tiene?.('recibos.emitir');
  const veTelegram = s.tiene?.('telegram.configurar');
  const [sp, setQuery] = useQuery();
  const vista = sp.get('vista') || (veCorreo ? 'correo' : 'telegram');
  const { dialog, dialogEl } = useDialog();

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Bandeja de salida"
        subtitulo="Correos y Telegram con su estado"
        ayuda="Todo lo que EDISYS envía pasa por esta bandeja. En modo simulado se registra sin salir. Un mensaje con error puede reintentarse; lo enviado no se repite."
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          {veCorreo && <Chip activo={vista === 'correo'} icono="correo" onClick={() => setQuery({ vista: 'correo' })}>Correos</Chip>}
          {veTelegram && <Chip activo={vista === 'telegram'} icono="enviar" onClick={() => setQuery({ vista: 'telegram' })}>Telegram</Chip>}
          {veTelegram && <Chip activo={vista === 'destinos'} icono="usuario" onClick={() => setQuery({ vista: 'destinos' })}>Destinos de Telegram</Chip>}
        </div>
        {vista === 'correo' && veCorreo && <Correos dialog={dialog} />}
        {vista === 'telegram' && veTelegram && <MensajesTelegram dialog={dialog} />}
        {vista === 'destinos' && veTelegram && <DestinosTelegram dialog={dialog} />}
      </Contenido>
    </>
  );
}

function FiltroEstados({ estado, setEstado, conteos }) {
  const n = porEstado(conteos);
  return (
    <div className="flex flex-wrap items-center gap-2">
      {ESTADOS.map(([v, t]) => (
        <Chip key={v || 'todos'} activo={estado === v} onClick={() => setEstado(v)} contador={n[v] ?? 0}>{t}</Chip>
      ))}
    </div>
  );
}

// ---------- E3 · correos ----------

function Correos({ dialog }) {
  const eid = useEid();
  const { toast } = useToast();
  const [estado, setEstado] = useState('');
  const [buscar, setBuscar] = useState('');
  const [pagina, setPagina] = useState(1);
  const [detalle, setDetalle] = useState(null); // id
  const correos = useCarga(() => api.get(`/edificios/${eid}/correo/mensajes`, { estado, q: buscar, pagina }), [eid, estado, buscar, pagina]);

  const reintentar = async (m) => {
    try {
      const r = await api.post(`/edificios/${eid}/correo/mensajes/${m.id}/reintentar`);
      toast(r.estado === 'error' || r.estado === 'pendiente' ? 'Vuelve a la cola: aún no sale.' : `Reintentado · ${r.estado}.`, { tipo: 'exito' });
      await correos.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo reintentar', text: err.message });
    }
  };

  const columnas = [
    { clave: 'para', titulo: 'Para', movil: 'titulo', render: (m) => (m.nombre ? `${m.nombre} <${m.para}>` : m.para) },
    { clave: 'asunto', titulo: 'Asunto', movil: 'sub' },
    { clave: 'origen', titulo: 'Origen', render: (m) => ORIGEN[m.origen] || m.origen, prioridad: 2 },
    { clave: 'unidad', titulo: 'Unidad', render: (m) => m.unidad || '—', prioridad: 3 },
    { clave: 'creado_en', titulo: 'Creado', render: (m) => formatearFechaHora(m.creado_en), prioridad: 2 },
    { clave: 'estado', titulo: 'Estado', render: (m) => <Insignia estado={m.estado} />, movil: 'valor2' },
    {
      clave: 'acciones', titulo: '', alinear: 'der',
      render: (m) => m.estado === 'error' && (
        <Boton tamano="sm" variante="secundario" onClick={(ev) => { ev.stopPropagation(); reintentar(m); }}>Reintentar</Boton>
      ),
    },
  ];

  return (
    <Seccion titulo="Correos" extra={correos.datos?.modo && <Insignia estado={correos.datos.modo === 'smtp' ? 'activo' : 'simulado'} texto={correos.datos.modo === 'smtp' ? 'SMTP' : 'Simulado'} />} padding="p-0">
      <div className="flex flex-col gap-3 px-4 pt-1">
        <FiltroEstados estado={estado} setEstado={(v) => { setEstado(v); setPagina(1); }} conteos={correos.datos?.conteos} />
        <Campo etiqueta="Buscar" ocultarEtiqueta tipo="buscar" placeholder="Destinatario, asunto o unidad" valor={buscar} onCambio={(v) => { setBuscar(v); setPagina(1); }} />
      </div>
      <Tabla
        columnas={columnas}
        filas={lista(correos.datos)}
        cargando={correos.cargando}
        error={correos.error}
        onReintentar={correos.recargar}
        onFila={(m) => setDetalle(m.id)}
        etiqueta="Correos de la bandeja"
        vacio={<Vacio icono="correo" titulo="Sin correos" texto="Aquí aparecen los recibos, balances y anuncios enviados por correo." compacto />}
        paginacion={{ pagina, porPagina: 25, total: correos.datos?.total || 0, onPagina: setPagina }}
      />
      {detalle && <DetalleCorreo id={detalle} onCerrar={() => setDetalle(null)} onReintentar={async (m) => { await reintentar(m); setDetalle(null); }} />}
    </Seccion>
  );
}

function DetalleCorreo({ id, onCerrar, onReintentar }) {
  const eid = useEid();
  const { datos: m, error } = useCarga(() => api.get(`/edificios/${eid}/correo/mensajes/${id}`), [eid, id]);
  return (
    <Modal
      abierto
      onCerrar={onCerrar}
      titulo={m?.asunto || 'Correo'}
      ancho="max-w-2xl"
      pie={m?.estado === 'error' ? <Boton onClick={() => onReintentar(m)}>Reintentar</Boton> : <Boton variante="secundario" onClick={onCerrar}>Cerrar</Boton>}
    >
      {error ? (
        <p className="text-sm text-alerta">{error.message}</p>
      ) : !m ? (
        <p className="text-sm text-texto-apoyo">Cargando…</p>
      ) : (
        <div className="flex flex-col gap-3 text-sm">
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
            <dt className="text-texto-apoyo">Para</dt><dd>{m.nombre ? `${m.nombre} <${m.para}>` : m.para}</dd>
            <dt className="text-texto-apoyo">Estado</dt><dd><Insignia estado={m.estado} /> <span className="text-xs text-texto-apoyo">{m.intentos} intento(s)</span></dd>
            <dt className="text-texto-apoyo">Origen</dt><dd>{ORIGEN[m.origen] || m.origen}{m.referencia ? ` · ${m.referencia}` : ''}</dd>
            <dt className="text-texto-apoyo">Creado</dt><dd>{formatearFechaHora(m.creado_en)}{m.enviado_por ? ` · ${m.enviado_por}` : ''}</dd>
            {m.procesado_en && (<><dt className="text-texto-apoyo">Procesado</dt><dd>{formatearFechaHora(m.procesado_en)}</dd></>)}
          </dl>
          {m.error && <p className="rounded-control border border-alerta-borde bg-alerta-suave px-3 py-2 text-alerta">{m.error}</p>}
          {m.adjuntos?.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {m.adjuntos.map((a) => (
                <a key={a.id} href={urlApi(`/edificios/${eid}/correo/mensajes/${m.id}/adjuntos/${a.id}`)} target="_blank" rel="noreferrer" className="text-acento hover:text-acento-hover">
                  {a.nombre} ({Math.max(1, Math.round(a.bytes / 1024))} KB)
                </a>
              ))}
            </div>
          )}
          {/* El HTML del correo va en un iframe sin scripts: se ve tal cual, sin tocar la app. */}
          {m.html ? (
            <iframe title="Vista del correo" sandbox="" srcDoc={m.html} className="h-80 w-full rounded-control border border-borde bg-white" />
          ) : (
            <pre className="whitespace-pre-wrap rounded-control border border-borde bg-fondo p-3 text-xs">{m.texto}</pre>
          )}
        </div>
      )}
    </Modal>
  );
}

// ---------- E4 · Telegram ----------

function MensajesTelegram({ dialog }) {
  const eid = useEid();
  const { toast } = useToast();
  const [estado, setEstado] = useState('');
  const [pagina, setPagina] = useState(1);
  const msgs = useCarga(() => api.get(`/edificios/${eid}/telegram/mensajes`, { estado, pagina }), [eid, estado, pagina]);

  const reintentar = async (m) => {
    try {
      const r = await api.post(`/edificios/${eid}/telegram/mensajes/${m.id}/reintentar`);
      toast(`Reintentado · ${r.estado}.`, { tipo: 'exito' });
      await msgs.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo reintentar', text: err.message });
    }
  };

  const columnas = [
    { clave: 'destino', titulo: 'Destino', movil: 'titulo', render: (m) => m.destino || m.chat_id },
    { clave: 'texto', titulo: 'Mensaje', movil: 'sub', render: (m) => <span className="line-clamp-2">{m.texto}</span> },
    { clave: 'origen', titulo: 'Origen', render: (m) => ORIGEN[m.origen] || m.origen, prioridad: 2 },
    { clave: 'creado_en', titulo: 'Creado', render: (m) => formatearFechaHora(m.creado_en), prioridad: 2 },
    { clave: 'estado', titulo: 'Estado', movil: 'valor2', render: (m) => <span title={m.error || undefined}><Insignia estado={m.estado} /></span> },
    {
      clave: 'acciones', titulo: '', alinear: 'der',
      render: (m) => m.estado === 'error' && <Boton tamano="sm" variante="secundario" onClick={() => reintentar(m)}>Reintentar</Boton>,
    },
  ];
  return (
    <Seccion titulo="Mensajes de Telegram" extra={msgs.datos?.modo && <Insignia estado={msgs.datos.modo === 'bot' ? 'activo' : 'simulado'} texto={msgs.datos.modo === 'bot' ? 'Bot activo' : 'Simulado'} />} padding="p-0">
      <div className="px-4 pt-1">
        <FiltroEstados estado={estado} setEstado={(v) => { setEstado(v); setPagina(1); }} conteos={msgs.datos?.conteos} />
      </div>
      <Tabla
        columnas={columnas}
        filas={lista(msgs.datos)}
        cargando={msgs.cargando}
        error={msgs.error}
        onReintentar={msgs.recargar}
        etiqueta="Mensajes de Telegram"
        vacio={<Vacio icono="enviar" titulo="Sin mensajes" texto="Envía desde «Destinos de Telegram» o publica un anuncio con el canal Telegram." compacto />}
        paginacion={{ pagina, porPagina: 25, total: msgs.datos?.total || 0, onPagina: setPagina }}
      />
    </Seccion>
  );
}

function DestinosTelegram({ dialog }) {
  const eid = useEid();
  const { toast } = useToast();
  const estado = useCarga(() => api.get(`/edificios/${eid}/telegram/estado`), [eid]);
  const destinos = useCarga(() => api.get(`/edificios/${eid}/telegram/destinos`), [eid]);
  const [form, setForm] = useState(null); // {nombre, chat_id, tipo}
  const [envio, setEnvio] = useState(null); // {destino_id|'todos', texto}
  const [ocupado, setOcupado] = useState(false);
  const est = estado.datos;

  const guardar = async (e) => {
    e?.preventDefault();
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/telegram/destinos`, form);
      toast('Destino agregado.', { tipo: 'exito' });
      setForm(null);
      await Promise.all([destinos.recargar(), estado.recargar()]);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo agregar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const alternar = async (d) => {
    try {
      await api.put(`/edificios/${eid}/telegram/destinos/${d.id}`, { activo: !d.activo });
      await Promise.all([destinos.recargar(), estado.recargar()]);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo cambiar', text: err.message });
    }
  };

  const borrar = async (d) => {
    if (!(await dialog.confirm({ title: '¿Quitar el destino?', text: `«${d.nombre}» deja de recibir mensajes. Lo ya enviado se conserva en la bandeja.`, danger: true }))) return;
    try {
      await api.del(`/edificios/${eid}/telegram/destinos/${d.id}`);
      await Promise.all([destinos.recargar(), estado.recargar()]);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo quitar', text: err.message });
    }
  };

  const verificar = async () => {
    try {
      const r = await api.post(`/edificios/${eid}/telegram/verificar`);
      await dialog.alert({ title: 'Bot conectado', text: `Telegram reconoce al bot @${r.bot}.` });
    } catch (err) {
      await dialog.alert({ title: 'No se pudo verificar', text: err.message });
    }
  };

  const enviar = async (e) => {
    e?.preventDefault();
    setOcupado(true);
    try {
      const cuerpo = envio.destino === 'todos' ? { todos: true, texto: envio.texto } : { destino_id: Number(envio.destino), texto: envio.texto };
      const r = await api.post(`/edificios/${eid}/telegram/enviar`, cuerpo);
      toast(`${r.encolados} mensaje(s) · ${r.modo === 'bot' ? 'enviados por el bot' : 'simulados'}.`, { tipo: 'exito' });
      setEnvio(null);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo enviar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const filas = lista(destinos.datos);
  const activos = filas.filter((d) => d.activo);
  return (
    <>
      <Seccion titulo="Bot de Telegram">
        <p className="text-sm text-tinta">
          {est ? (
            est.modo === 'bot' ? 'El bot está activo: los mensajes salen por Telegram.' : est.bot_configurado ? 'Hay token, pero el servidor está en modo simulado (TELEGRAM_MODO).' : 'Modo simulado: los mensajes se registran sin salir. El token del bot se configura en el servidor (TELEGRAM_BOT_TOKEN).'
          ) : '…'}
        </p>
        <p className="text-xs text-texto-apoyo">Para un grupo: agrega el bot al grupo y registra su chat_id (empieza con «-»). Para un vecino: que le escriba al bot y registra su id.</p>
        <div className="flex flex-wrap gap-2">
          <Boton tamano="sm" variante="secundario" onClick={verificar} disabled={est?.modo !== 'bot'}>Verificar bot</Boton>
          <Boton tamano="sm" icono="mas_signo" onClick={() => setForm({ nombre: '', chat_id: '', tipo: 'grupo' })}>Agregar destino</Boton>
          <Boton tamano="sm" icono="enviar" variante="secundario" disabled={!activos.length} onClick={() => setEnvio({ destino: 'todos', texto: '' })}>Enviar mensaje</Boton>
        </div>
      </Seccion>
      <Seccion titulo="Destinos">
        {destinos.error ? (
          <p className="text-sm text-alerta">{destinos.error.message}</p>
        ) : filas.length === 0 ? (
          <Vacio icono="usuario" titulo="Sin destinos" texto="Agrega el grupo de propietarios o de la junta." compacto />
        ) : (
          <ul className="divide-y divide-borde">
            {filas.map((d) => (
              <li key={d.id} className="flex flex-wrap items-center gap-3 py-2.5">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium text-tinta">{d.nombre}</p>
                  <p className="truncate text-xs text-texto-apoyo">{d.tipo === 'grupo' ? 'Grupo' : 'Usuario'} · {d.chat_id}{d.unidad ? ` · Dpto ${d.unidad}` : ''} · {d.mensajes} mensaje(s)</p>
                </div>
                {!d.activo && <Insignia estado="inactivo" />}
                <Boton tamano="sm" variante="fantasma" onClick={() => alternar(d)}>{d.activo ? 'Pausar' : 'Activar'}</Boton>
                <Boton tamano="sm" variante="fantasma" onClick={() => borrar(d)}>Quitar</Boton>
              </li>
            ))}
          </ul>
        )}
      </Seccion>

      <Modal abierto={!!form} onCerrar={() => setForm(null)} titulo="Agregar destino" ancho="max-w-sm" pie={<><Boton variante="fantasma" onClick={() => setForm(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardar}>Agregar</Boton></>}>
        {form && (
          <form className="flex flex-col gap-3" onSubmit={guardar}>
            <Campo etiqueta="Nombre" valor={form.nombre} onCambio={(v) => setForm({ ...form, nombre: v })} placeholder="Grupo de propietarios" />
            <Campo etiqueta="Chat id" valor={form.chat_id} onCambio={(v) => setForm({ ...form, chat_id: v })} placeholder="-1001234567890 o @canal" />
            <Campo etiqueta="Tipo" tipo="select" valor={form.tipo} onCambio={(v) => setForm({ ...form, tipo: v })} opciones={[{ valor: 'grupo', etiqueta: 'Grupo' }, { valor: 'usuario', etiqueta: 'Usuario' }]} />
          </form>
        )}
      </Modal>

      <Modal abierto={!!envio} onCerrar={() => setEnvio(null)} titulo="Enviar por Telegram" ancho="max-w-md" pie={<><Boton variante="fantasma" onClick={() => setEnvio(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={enviar}>Enviar</Boton></>}>
        {envio && (
          <form className="flex flex-col gap-3" onSubmit={enviar}>
            <Campo etiqueta="A" tipo="select" valor={envio.destino} onCambio={(v) => setEnvio({ ...envio, destino: v })} opciones={[{ valor: 'todos', etiqueta: `Todos los activos (${activos.length})` }, ...activos.map((d) => ({ valor: String(d.id), etiqueta: d.nombre }))]} />
            <Campo etiqueta="Mensaje" tipo="textarea" valor={envio.texto} onCambio={(v) => setEnvio({ ...envio, texto: v })} maxLength={4096} />
          </form>
        )}
      </Modal>
    </>
  );
}
