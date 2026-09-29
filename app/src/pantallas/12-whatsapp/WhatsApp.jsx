import { useEffect, useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFechaHora, mesDePeriodo } from '../../lib/fechas.js';
import { ruta, useQuery } from '../../lib/nav.jsx';
import { useEid, useSesion, Guarda } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { nombreUnidad } from '../../lib/unidad.js';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Icono, Insignia, SelectorPeriodo, Vacio, infoEstado, useDialog, useToast } from '../../ui/index.js';

// Estados del CHECK de whatsapp_mensaje (api/migrations/0007_whatsapp.sql).
const ESTADOS = ['', 'pendiente', 'simulado', 'enviado', 'error', 'recibido'];
const NOMBRE_PLANTILLA = {
  recibo: 'Recibo del mes',
  recordatorio_deuda: 'Recordatorio de deuda',
  reserva_confirmada: 'Reserva confirmada',
  incidencia_actualizada: 'Incidencia actualizada',
  aviso_general: 'Aviso general',
  libre: 'Texto libre',
};
const PLANTILLAS_BASE = [
  { id: 'aviso_general', nombre: 'Aviso general', variables: ['mensaje'] },
  { id: 'recordatorio_deuda', nombre: 'Recordatorio de deuda', variables: ['nombre', 'unidad', 'saldo', 'yape'] },
  { id: 'libre', nombre: 'Texto libre', variables: ['texto'] },
];

/** 12 · WhatsApp: bandeja con filtro por estado, envío de recibos del periodo y configuración (con aviso de modo SIMULADO). */
export default function WhatsApp() {
  const s = useSesion();
  const [q, setQuery] = useQuery();
  const tab = ['bandeja', 'enviar', 'config'].includes(q.get('tab')) ? q.get('tab') : 'bandeja';
  const config = useCarga(() => api.get('/whatsapp/config'), []);
  // Con el API real, «envio_real» dice si de verdad sale algo; si no viene, se deduce del modo.
  const simulado = config.datos ? (config.datos.envio_real !== undefined ? !config.datos.envio_real : config.datos.modo !== 'evolution') : false;

  return (
    <>
      <Encabezado
        titulo="WhatsApp"
        acciones={
          <Boton variante="secundario" icono="chatbot" href={ruta('chatbot')}>
            Simulador del chatbot
          </Boton>
        }
      >
        {config.datos && simulado && (
          <div className="flex items-center gap-2 border-y border-aviso-borde bg-aviso-suave px-4 py-1.5 text-xs text-aviso-texto lg:px-6" role="status">
            <Icono nombre="simulado" tam={14} className="shrink-0" />
            <span>
              <b>Modo SIMULADO:</b> los mensajes se registran en la bandeja pero NO salen a ningún teléfono.
              {s.tiene('whatsapp.configurar') ? ' Para enviar de verdad, conecta evolution-go en «Configuración».' : ''}
            </span>
          </div>
        )}
        <div role="tablist" className="flex gap-1 overflow-x-auto px-4 lg:px-6">
          {[
            ['bandeja', 'Bandeja'],
            ['enviar', 'Enviar'],
            ...(s.tiene('whatsapp.configurar') ? [['config', 'Configuración']] : []),
          ].map(([id, t]) => (
            <button key={id} type="button" role="tab" aria-selected={tab === id} onClick={() => setQuery({ tab: id === 'bandeja' ? null : id, estado: null, q: null })} className={`h-11 whitespace-nowrap border-b-2 px-3 text-sm font-semibold transition-colors duration-rapida ${tab === id ? 'border-acento text-acento' : 'border-transparent text-texto-suave hover:text-tinta'}`}>
              {t}
            </button>
          ))}
        </div>
      </Encabezado>
      <Contenido>
        {config.error && <ErrorCarga error={config.error} onReintentar={config.recargar} compacto />}
        {tab === 'bandeja' && <Bandeja />}
        {tab === 'enviar' && <Enviar simulado={simulado} />}
        {tab === 'config' && config.datos && <Configuracion inicial={config.datos} onGuardado={config.recargar} />}
      </Contenido>
    </>
  );
}

function Bandeja() {
  const [q, setQuery] = useQuery();
  const estado = q.get('estado') || '';
  const busca = q.get('q') || '';
  const [texto, setTexto] = useState(busca);
  useEffect(() => {
    const t = setTimeout(() => texto !== busca && setQuery({ q: texto }, { reemplazar: true }), 350);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [texto]);
  const { datos, error, cargando, recargar } = useCarga(() => api.get('/whatsapp/mensajes', { estado, q: busca }), [estado, busca]);
  const mensajes = lista(datos, 'mensajes');

  return (
    <>
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
        <div className="-mx-4 flex gap-2 overflow-x-auto px-4 lg:mx-0 lg:flex-wrap lg:px-0" role="group" aria-label="Filtrar por estado">
          {ESTADOS.map((e) => (
            <Chip key={e || 'todos'} activo={estado === e} icono={e ? infoEstado(e).icono : undefined} onClick={() => setQuery({ estado: e || null }, { reemplazar: true })}>
              {e ? infoEstado(e).texto : 'Todos'}
            </Chip>
          ))}
        </div>
        <label className="relative lg:ml-auto lg:w-72">
          <span className="sr-only">Buscar por teléfono, unidad o texto</span>
          <Icono nombre="buscar" tam={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-texto-apoyo" />
          <input type="search" value={texto} onChange={(e) => setTexto(e.target.value)} placeholder="Teléfono, unidad o texto" className="h-11 w-full rounded-control border border-borde-fuerte bg-superficie pl-9 pr-3 text-base transition-colors duration-rapida focus:border-acento focus:outline-none focus:ring-2 focus:ring-acento lg:h-8 lg:text-sm" />
        </label>
      </div>
      {error ? (
        <ErrorCarga error={error} onReintentar={recargar} />
      ) : !datos && cargando ? (
        <Esqueleto className="h-64 w-full" />
      ) : mensajes.length === 0 ? (
        <Vacio titulo="No hay mensajes" texto={estado || busca ? 'Prueba con otro filtro.' : 'Los recibos y avisos que envíes aparecerán aquí.'} icono="bandeja" compacto>
          {estado || busca ? (
            <Boton variante="secundario" icono="cerrar" onClick={() => (setTexto(''), setQuery({ estado: null, q: null }, { reemplazar: true }))}>
              Quitar filtros
            </Boton>
          ) : (
            <Boton icono="enviar" onClick={() => setQuery({ tab: 'enviar' })}>
              Enviar un mensaje
            </Boton>
          )}
        </Vacio>
      ) : (
        <ul className="flex flex-col divide-y divide-superficie-2 overflow-hidden rounded-tarjeta border border-borde bg-superficie" aria-label="Mensajes">
          {mensajes.map((m) => (
            <Mensaje key={m.id} m={m} />
          ))}
        </ul>
      )}
    </>
  );
}

/** Iniciales para el avatar: «María Demo» → «MD»; «Dpto 201» → «201». */
function iniciales(nombre) {
  const t = String(nombre || '').trim();
  if (!t) return '?';
  const num = t.match(/\d+[A-Za-z]?$/);
  if (/^dpto/i.test(t) && num) return num[0].slice(0, 3);
  if (/^\+?\d[\d\s]+$/.test(t)) return t.replace(/\D/g, '').slice(-2);
  return t.split(/\s+/).slice(0, 2).map((p) => p[0]).join('').toUpperCase();
}

/** Un mensaje de la bandeja como fila de conversación; el texto completo se despliega al tocarlo. */
function Mensaje({ m }) {
  const [abierto, setAbierto] = useState(false);
  const entrante = m.direccion === 'entrante';
  const quien = m.destinatario || nombreUnidad(m.unidad) || m.telefono;
  const largo = String(m.texto || '').length > 140 || String(m.texto || '').includes('\n');
  return (
    <li className={`flex gap-3 px-4 py-3 transition-colors duration-rapida ${entrante ? 'bg-acento-suave/60' : ''}`}>
      <span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-chip text-sm font-semibold ${entrante ? 'bg-acento text-white' : 'bg-superficie-2 text-texto-suave'}`} aria-hidden="true">
        {iniciales(quien)}
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-1">
          <span className="flex min-w-0 items-center gap-1.5 text-sm">
            <Icono nombre={entrante ? 'entrante' : 'enviar'} tam={14} className={entrante ? 'text-acento' : 'text-texto-apoyo'} />
            <b className="truncate">{quien}</b>
            <span className="truncate text-xs text-texto-apoyo">{[m.destinatario ? nombreUnidad(m.unidad) : null, m.telefono].filter((x) => x && x !== quien).join(' · ')}</span>
          </span>
          <span className="flex items-center gap-2">
            <span className="text-xs tabular-nums text-texto-apoyo">{formatearFechaHora(m.fecha || m.creado_en)}</span>
            <Insignia estado={m.estado} />
          </span>
        </div>
        <p className={`whitespace-pre-line text-sm text-tinta ${abierto ? '' : 'line-clamp-2'}`}>{m.texto}</p>
        {largo && (
          <button type="button" onClick={() => setAbierto(!abierto)} aria-expanded={abierto} className="self-start text-xs font-semibold text-acento">
            {abierto ? 'Ver menos' : 'Ver mensaje completo'}
          </button>
        )}
        <div className="flex flex-wrap justify-between gap-2 text-xs text-texto-apoyo">
          <span>
            {entrante ? 'Recibido' : 'Enviado'}
            {m.plantilla ? ` · ${NOMBRE_PLANTILLA[m.plantilla] || m.plantilla}` : ''}
            {m.enviado_por ? ` · ${m.enviado_por}` : ''}
            {m.intencion ? ` · intención: ${m.intencion}` : ''}
          </span>
          {m.error && (
            <span className="flex items-center gap-1 font-semibold text-alerta">
              <Icono nombre="alerta" tam={12} /> {m.error}
            </span>
          )}
        </div>
      </div>
    </li>
  );
}

function Enviar({ simulado }) {
  const eid = useEid();
  const [periodo, setPeriodo] = usePeriodo();
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();
  const [enviandoRecibos, setEnviandoRecibos] = useState(false);
  const unidades = useCarga(() => api.get(`/edificios/${eid}/unidades`, { por_pagina: 500 }), [eid]);
  const plantillasApi = useCarga(() => api.get('/whatsapp/plantillas'), []);
  const plantillas = (plantillasApi.datos?.plantillas || [])
    .filter((p) => p.codigo !== 'chatbot')
    .map((p) => ({ id: p.codigo, nombre: NOMBRE_PLANTILLA[p.codigo] || p.codigo, variables: p.variables || [], texto: p.texto }));
  if (!plantillas.length) plantillas.push(...PLANTILLAS_BASE);
  const [f, setF] = useState({ unidad_id: '', telefono: '', plantilla: 'aviso_general', variables: {} });
  const opcional = (v) => ['unidad', 'nombre', 'yape', 'enlace', 'estado'].includes(v) && f.unidad_id;
  const [enviando, setEnviando] = useState(false);
  const [errores, setErrores] = useState({});
  const plantilla = plantillas.find((p) => p.id === f.plantilla) || plantillas[0];

  const enviarRecibos = async () => {
    const ok = await dialog.confirm({
      title: `¿Enviar por WhatsApp los recibos de ${mesDePeriodo(periodo)}?`,
      text: simulado ? 'Estás en modo SIMULADO: se registran en la bandeja pero no salen.' : 'Cada propietario recibe su recibo en su celular.',
      okText: 'Enviar recibos',
    });
    if (!ok) return;
    setEnviandoRecibos(true);
    try {
      const r = await api.post(`/whatsapp/recibos/${periodo}/enviar`, {});
      const sim = r?.simulado || r?.modo === 'simulado' || (r?.simulados > 0 && !r?.enviados) || simulado;
      const n = r?.encolados ?? r?.en_cola ?? '';
      toast(`${n} recibos en cola${r?.sin_telefono ? ` · ${Array.isArray(r.sin_telefono) ? r.sin_telefono.length : r.sin_telefono} sin teléfono` : ''}${sim ? ' (SIMULADO: no salió ninguno)' : ''}.`, { tipo: sim ? 'aviso' : 'exito' });
    } catch (err) {
      dialog.alert({ title: 'No se pudieron enviar', text: err.message });
    } finally {
      setEnviandoRecibos(false);
    }
  };

  const enviar = async () => {
    const e = {};
    if (!f.unidad_id && !f.telefono.trim()) e.destino = 'Elige una unidad o escribe un teléfono.';
    if (f.telefono && !/^9\d{8}$/.test(f.telefono.replace(/\D/g, '').slice(-9))) e.telefono = 'Celular de 9 dígitos que empieza con 9.';
    for (const v of plantilla.variables || []) if (!opcional(v) && !String(f.variables[v] || '').trim()) e[v] = 'Obligatorio.';
    setErrores(e);
    if (Object.keys(e).length) return;
    setEnviando(true);
    try {
      const r = await api.post('/whatsapp/enviar', { unidad_id: f.unidad_id ? Number(f.unidad_id) : undefined, telefono: f.telefono.trim() || undefined, plantilla: f.plantilla, variables: f.variables });
      const sim = r?.simulado || r?.estado === 'simulado' || simulado;
      toast(sim ? 'Mensaje registrado en modo SIMULADO (no salió).' : 'Mensaje en cola de envío.', { tipo: sim ? 'aviso' : 'exito' });
      setF({ ...f, variables: {} });
    } catch (err) {
      if (Object.keys(err.campos || {}).length) setErrores(err.campos);
      else dialog.alert({ title: 'No se pudo enviar', text: err.message });
    } finally {
      setEnviando(false);
    }
  };

  const listaUnidades = lista(unidades.datos);
  return (
    <div className="grid gap-4 lg:grid-cols-2 lg:gap-5">
      {dialogEl}
      <Guarda permiso="whatsapp.enviar" sino={<Vacio titulo="Solo lectura" texto="Tu rol puede ver la bandeja, pero no enviar." compacto />}>
        <Seccion titulo="Recibos del periodo">
          <p className="text-base text-texto-suave">Envía a cada unidad su recibo del periodo con el monto y la fecha de vencimiento.</p>
          <SelectorPeriodo periodo={periodo} onCambio={setPeriodo} />
          <Boton icono="enviar" onClick={enviarRecibos} cargando={enviandoRecibos} className="self-start">
            Enviar recibos de {mesDePeriodo(periodo)}
          </Boton>
        </Seccion>
        <Seccion titulo="Mensaje individual">
          <Campo
            etiqueta="Unidad"
            tipo="select"
            valor={f.unidad_id}
            onCambio={(v) => setF({ ...f, unidad_id: v })}
            error={errores.destino}
            opciones={[{ valor: '', etiqueta: 'Elige una unidad (o escribe un teléfono)' }, ...listaUnidades.map((u) => ({ valor: String(u.id), etiqueta: `Dpto ${u.codigo} · ${u.propietario}` }))]}
          />
          <Campo etiqueta="Teléfono (opcional)" tipo="telefono" valor={f.telefono} onCambio={(v) => setF({ ...f, telefono: v })} placeholder="900 000 000" error={errores.telefono} />
          <Campo etiqueta="Plantilla" tipo="select" valor={f.plantilla} onCambio={(v) => setF({ ...f, plantilla: v, variables: {} })} opciones={plantillas.map((p) => ({ valor: p.id, etiqueta: p.nombre }))} />
          {(plantilla.variables || []).map((v) => (
            <Campo key={v} etiqueta={`${v.charAt(0).toUpperCase() + v.slice(1)}${opcional(v) ? ' (opcional: sale de la unidad)' : ''}`} tipo={v === 'texto' || v === 'mensaje' ? 'textarea' : 'texto'} valor={f.variables[v] || ''} onCambio={(x) => setF({ ...f, variables: { ...f.variables, [v]: x } })} error={errores[v]} placeholder={v === 'monto' ? formatearSoles(99000) : undefined} />
          ))}
          <Boton icono="enviar" onClick={enviar} cargando={enviando} className="self-start">
            Enviar mensaje
          </Boton>
        </Seccion>
      </Guarda>
    </div>
  );
}

function Configuracion({ inicial, onGuardado }) {
  const { toast } = useToast();
  const [f, setF] = useState({ modo: inicial.modo || 'simulado', url: inicial.url || '', instancia: inicial.instancia || '', apikey: '' });
  const [errores, setErrores] = useState({});
  const [guardando, setGuardando] = useState(false);
  const guardar = async () => {
    const e = {};
    if (f.modo === 'evolution') {
      if (!/^https?:\/\/\S+$/.test(f.url.trim())) e.url = 'Escribe la URL de evolution-go (http:// o https://).';
      if (!f.instancia.trim()) e.instancia = 'Escribe el nombre de la instancia.';
    }
    setErrores(e);
    if (Object.keys(e).length) return;
    setGuardando(true);
    try {
      const cuerpo = { modo: f.modo, url: f.url.trim(), instancia: f.instancia.trim() };
      if (f.apikey.trim()) cuerpo.apikey = f.apikey.trim();
      await api.put('/whatsapp/config', cuerpo);
      setF((x) => ({ ...x, apikey: '' }));
      toast(f.modo === 'simulado' ? 'Guardado: modo SIMULADO, no sale ningún mensaje.' : 'Guardado: los mensajes saldrán por evolution-go.', { tipo: f.modo === 'simulado' ? 'aviso' : 'exito' });
      onGuardado();
    } catch (err) {
      setErrores(Object.keys(err.campos || {}).length ? err.campos : { general: err.message });
    } finally {
      setGuardando(false);
    }
  };
  const opcion = (valor, titulo, detalle) => (
    <label className={`flex min-h-[56px] cursor-pointer items-start gap-3 rounded-tarjeta border bg-superficie p-3 transition-colors duration-rapida ${f.modo === valor ? 'border-acento ring-1 ring-acento' : 'border-borde'}`}>
      <input type="radio" name="modo" checked={f.modo === valor} onChange={() => setF({ ...f, modo: valor })} className="mt-0.5 h-5 w-5 accent-[var(--color-acento)]" />
      <span className="flex flex-col">
        <b>{titulo}</b>
        <span className="text-sm text-texto-apoyo">{detalle}</span>
      </span>
    </label>
  );
  return (
    <Seccion titulo="Conexión de WhatsApp" className="max-w-xl">
      {opcion('simulado', 'Simulado', 'Registra los mensajes en la bandeja sin enviarlos. Para pruebas y demos.')}
      {opcion('evolution', 'evolution-go', 'Envía de verdad por la instancia de WhatsApp conectada.')}
      {f.modo === 'evolution' && (
        <>
          <Campo etiqueta="URL de evolution-go" valor={f.url} onCambio={(v) => setF({ ...f, url: v })} placeholder="http://evolution-go:8080" error={errores.url} />
          <Campo etiqueta="Instancia" valor={f.instancia} onCambio={(v) => setF({ ...f, instancia: v })} placeholder="edificio-demo" error={errores.instancia} />
          <Campo etiqueta="API key de evolution-go" tipo="clave" valor={f.apikey} onCambio={(v) => setF({ ...f, apikey: v })} autoComplete="new-password" ayuda={inicial.tiene_apikey ? 'Ya hay una clave guardada. Déjalo vacío para conservarla.' : 'Se guarda en el servidor y no se vuelve a mostrar.'} />
        </>
      )}
      {errores.general && (
        <p className="text-sm text-alerta" role="alert">
          {errores.general}
        </p>
      )}
      <Boton onClick={guardar} cargando={guardando} className="self-start">
        Guardar configuración
      </Boton>
    </Seccion>
  );
}
