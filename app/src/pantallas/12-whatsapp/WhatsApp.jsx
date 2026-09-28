import { useEffect, useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFechaHora, mesDePeriodo } from '../../lib/fechas.js';
import { ruta, useQuery } from '../../lib/nav.jsx';
import { useEid, useSesion, Guarda } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, ErrorCarga, Esqueleto, Icono, Insignia, SelectorPeriodo, Vacio, useDialog, useToast } from '../../ui/index.js';

const ESTADOS = ['', 'en_cola', 'simulado', 'enviado', 'entregado', 'leido', 'fallido', 'recibido'];
const PLANTILLAS_BASE = [
  { id: 'recibo_emitido', nombre: 'Recibo emitido', variables: ['nombre', 'periodo', 'monto', 'vence'] },
  { id: 'recordatorio_deuda', nombre: 'Recordatorio de deuda', variables: ['nombre', 'periodo', 'monto'] },
  { id: 'reserva_confirmada', nombre: 'Reserva confirmada', variables: ['nombre', 'area', 'fecha', 'codigo'] },
  { id: 'aviso_general', nombre: 'Aviso general', variables: ['texto'] },
];

/** 12 · WhatsApp: bandeja con filtro por estado, envío de recibos del periodo y configuración (con aviso de modo SIMULADO). */
export default function WhatsApp() {
  const s = useSesion();
  const [q, setQuery] = useQuery();
  const tab = ['bandeja', 'enviar', 'config'].includes(q.get('tab')) ? q.get('tab') : 'bandeja';
  const config = useCarga(() => api.get('/whatsapp/config'), []);
  const simulado = config.datos?.modo !== 'evolution';

  return (
    <>
      <Encabezado
        titulo="WhatsApp"
        acciones={
          <Boton variante="secundario" icono="robot" href={ruta('chatbot')}>
            Simulador del chatbot
          </Boton>
        }
      >
        <div role="tablist" className="flex gap-1 overflow-x-auto px-4 lg:px-8">
          {[
            ['bandeja', 'Bandeja'],
            ['enviar', 'Enviar'],
            ...(s.tiene('whatsapp.configurar') ? [['config', 'Configuración']] : []),
          ].map(([id, t]) => (
            <button key={id} type="button" role="tab" aria-selected={tab === id} onClick={() => setQuery({ tab: id === 'bandeja' ? null : id, estado: null, q: null })} className={`h-12 whitespace-nowrap border-b-2 px-3 text-sm font-semibold ${tab === id ? 'border-acento text-acento' : 'border-transparent text-texto-suave hover:text-tinta'}`}>
              {t}
            </button>
          ))}
        </div>
      </Encabezado>
      <Contenido>
        {config.datos && simulado && (
          <div className="flex items-start gap-3 rounded-xl border-2 border-aviso-borde bg-aviso-suave p-4 text-aviso-texto" role="status">
            <Icono nombre="alerta" tam={22} className="mt-0.5 shrink-0 text-aviso" />
            <div className="flex flex-col gap-1">
              <b className="text-base">Modo SIMULADO</b>
              <span className="text-sm">Los mensajes se registran en la bandeja pero NO salen a ningún teléfono. Para enviar de verdad, conecta evolution-go en «Configuración».</span>
            </div>
          </div>
        )}
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
            <button key={e || 'todos'} type="button" aria-pressed={estado === e} onClick={() => setQuery({ estado: e || null }, { reemplazar: true })} className={`h-10 shrink-0 rounded-full border px-4 text-sm font-semibold ${estado === e ? 'border-tinta bg-tinta text-white' : 'border-borde-fuerte bg-superficie'}`}>
              {e ? <Insignia estado={e} className="border-0 bg-transparent p-0 text-inherit" /> : 'Todos'}
            </button>
          ))}
        </div>
        <label className="relative lg:ml-auto lg:w-72">
          <span className="sr-only">Buscar por teléfono, unidad o texto</span>
          <Icono nombre="buscar" tam={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-texto-apoyo" />
          <input type="search" value={texto} onChange={(e) => setTexto(e.target.value)} placeholder="Teléfono, unidad o texto" className="h-11 w-full rounded-lg border border-borde-fuerte bg-superficie pl-9 pr-3 text-base focus:outline-none focus:ring-2 focus:ring-acento sm:text-sm" />
        </label>
      </div>
      {error ? (
        <ErrorCarga error={error} onReintentar={recargar} />
      ) : !datos && cargando ? (
        <Esqueleto className="h-64 w-full" />
      ) : mensajes.length === 0 ? (
        <Vacio titulo="No hay mensajes" texto={estado || busca ? 'Prueba con otro filtro.' : 'Los recibos y avisos que envíes aparecerán aquí.'} icono="mensaje" compacto />
      ) : (
        <ul className="flex flex-col gap-2">
          {mensajes.map((m) => (
            <li key={m.id} className={`flex flex-col gap-2 rounded-xl border p-4 ${m.direccion === 'entrante' ? 'border-acento-borde bg-acento-suave' : 'border-borde bg-superficie'}`}>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="flex items-center gap-2 text-sm">
                  <Icono nombre={m.direccion === 'entrante' ? 'volver' : 'enviar'} tam={16} className={m.direccion === 'entrante' ? 'text-acento' : 'text-texto-apoyo'} />
                  <b>{m.destinatario || m.telefono}</b>
                  <span className="text-texto-apoyo">
                    {[m.unidad, m.telefono].filter(Boolean).join(' · ')}
                  </span>
                </span>
                <span className="flex items-center gap-2">
                  {m.plantilla && <span className="text-xs text-texto-apoyo">{m.plantilla}</span>}
                  <Insignia estado={m.estado} />
                </span>
              </div>
              <p className="whitespace-pre-line text-base">{m.texto}</p>
              <div className="flex flex-wrap justify-between gap-2 text-xs text-texto-apoyo">
                <span>{m.direccion === 'entrante' ? 'Recibido' : 'Enviado'} {formatearFechaHora(m.fecha)}</span>
                {m.error && <span className="font-semibold text-alerta">{m.error}</span>}
              </div>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

function Enviar({ simulado }) {
  const eid = useEid();
  const [periodo, setPeriodo] = usePeriodo();
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();
  const [enviandoRecibos, setEnviandoRecibos] = useState(false);
  const unidades = useCarga(() => api.get(`/edificios/${eid}/unidades`, { por_pagina: 500 }), [eid]);
  // El contrato no expone un endpoint de plantillas: se usan las conocidas por el API.
  const plantillas = PLANTILLAS_BASE;
  const [f, setF] = useState({ unidad_id: '', telefono: '', plantilla: 'aviso_general', variables: {} });
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
      toast(`${r?.en_cola ?? ''} recibos en cola${r?.simulado || simulado ? ' (SIMULADO: no salió ninguno)' : ''}.`, { tipo: r?.simulado || simulado ? 'aviso' : 'exito' });
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
    for (const v of plantilla.variables || []) if (!String(f.variables[v] || '').trim()) e[v] = 'Obligatorio.';
    setErrores(e);
    if (Object.keys(e).length) return;
    setEnviando(true);
    try {
      const r = await api.post('/whatsapp/enviar', { unidad_id: f.unidad_id ? Number(f.unidad_id) : undefined, telefono: f.telefono.trim() || undefined, plantilla: f.plantilla, variables: f.variables });
      toast(r?.simulado || simulado ? 'Mensaje registrado en modo SIMULADO (no salió).' : 'Mensaje en cola de envío.', { tipo: r?.simulado || simulado ? 'aviso' : 'exito' });
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
    <div className="grid gap-4 lg:grid-cols-2 lg:gap-6">
      {dialogEl}
      <Guarda permiso="whatsapp.enviar" sino={<Vacio titulo="Solo lectura" texto="Tu rol puede ver la bandeja, pero no enviar." compacto />}>
        <Seccion titulo="Recibos del periodo">
          <p className="text-base text-texto-suave">Envía a cada unidad su recibo del periodo con el monto y la fecha de vencimiento.</p>
          <SelectorPeriodo periodo={periodo} onCambio={setPeriodo} />
          <Boton icono="whatsapp" onClick={enviarRecibos} cargando={enviandoRecibos} className="self-start">
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
            <Campo key={v} etiqueta={v.charAt(0).toUpperCase() + v.slice(1)} tipo={v === 'texto' ? 'textarea' : 'texto'} valor={f.variables[v] || ''} onCambio={(x) => setF({ ...f, variables: { ...f.variables, [v]: x } })} error={errores[v]} placeholder={v === 'monto' ? formatearSoles(99000) : undefined} />
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
  const [f, setF] = useState({ modo: inicial.modo || 'simulado', url: inicial.url || '', instancia: inicial.instancia || '' });
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
      await api.put('/whatsapp/config', { modo: f.modo, url: f.url.trim(), instancia: f.instancia.trim() });
      toast(f.modo === 'simulado' ? 'Guardado: modo SIMULADO, no sale ningún mensaje.' : 'Guardado: los mensajes saldrán por evolution-go.', { tipo: f.modo === 'simulado' ? 'aviso' : 'exito' });
      onGuardado();
    } catch (err) {
      setErrores(Object.keys(err.campos || {}).length ? err.campos : { general: err.message });
    } finally {
      setGuardando(false);
    }
  };
  const opcion = (valor, titulo, detalle) => (
    <label className={`flex min-h-[56px] cursor-pointer items-start gap-3 rounded-xl border bg-superficie p-4 ${f.modo === valor ? 'border-acento ring-1 ring-acento' : 'border-borde'}`}>
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
          <p className="text-xs text-texto-apoyo">La clave de la API se configura en el servidor, no aquí.</p>
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
