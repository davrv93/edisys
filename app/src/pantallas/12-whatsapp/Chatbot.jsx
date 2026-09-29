import { useEffect, useRef, useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearHora } from '../../lib/fechas.js';
import { ruta } from '../../lib/nav.jsx';
import { useEid } from '../../layout/Sesion.jsx';
import Encabezado from '../../layout/Encabezado.jsx';
import { Boton, Campo, Icono, Spinner } from '../../ui/index.js';

const SUGERENCIAS = ['Hola', '¿Cuánto debo?', 'Mi recibo', 'Quiero reservar la parrilla', 'Hay una fuga en el pasadizo'];

/** Simulador del chatbot: conversa como si fueras un propietario, llamando a /chatbot/mensaje. No envía WhatsApp. */
export default function Chatbot() {
  const eid = useEid();
  const unidades = useCarga(() => api.get(`/edificios/${eid}/unidades`, { por_pagina: 500 }), [eid]);
  const conCelular = lista(unidades.datos).filter((u) => u.celular);
  const [telefono, setTelefono] = useState('');
  const [texto, setTexto] = useState('');
  const [chat, setChat] = useState([]);
  const [pensando, setPensando] = useState(false);
  const fin = useRef(null);

  useEffect(() => {
    if (!telefono && conCelular.length) setTelefono(conCelular.find((u) => String(u.codigo) === '201')?.celular || conCelular[0].celular);
  }, [conCelular, telefono]);
  useEffect(() => {
    fin.current?.scrollIntoView({ behavior: 'smooth', block: 'end' });
  }, [chat, pensando]);

  const quien = conCelular.find((u) => u.celular === telefono);

  const mandar = async (t) => {
    const msg = (t ?? texto).trim();
    if (!msg || !telefono || pensando) return;
    setTexto('');
    setChat((c) => [...c, { de: 'yo', texto: msg, hora: new Date() }]);
    setPensando(true);
    try {
      const r = await api.post('/chatbot/mensaje', { telefono: telefono.replace(/\D/g, ''), texto: msg });
      setChat((c) => [...c, { de: 'bot', texto: r?.respuesta || '(sin respuesta)', intencion: r?.intencion, datos: r?.datos, hora: new Date() }]);
    } catch (err) {
      setChat((c) => [...c, { de: 'error', texto: err.message, hora: new Date() }]);
    } finally {
      setPensando(false);
    }
  };

  return (
    <>
      <Encabezado titulo="Simulador del chatbot" subtitulo="Prueba lo que respondería el asistente de WhatsApp. No se envía ningún mensaje." volver={ruta('whatsapp')} />
      <div className="mx-auto flex w-full max-w-2xl flex-1 flex-col gap-3 p-4 lg:p-8">
        <div className="grid gap-3 sm:grid-cols-2">
          <Campo
            etiqueta="Escribir como"
            tipo="select"
            valor={quien ? telefono : ''}
            onCambio={(v) => (setTelefono(v), setChat([]))}
            opciones={[{ valor: '', etiqueta: conCelular.length ? 'Otro teléfono' : 'Cargando propietarios…' }, ...conCelular.map((u) => ({ valor: u.celular, etiqueta: `Dpto ${u.codigo} · ${u.propietario}` }))]}
          />
          <Campo etiqueta="Teléfono" tipo="telefono" valor={telefono} onCambio={(v) => (setTelefono(v), setChat([]))} placeholder="900 000 201" />
        </div>

        <div className="flex min-h-[360px] flex-1 flex-col overflow-hidden rounded-tarjeta border border-borde bg-superficie">
          <div className="flex items-center gap-3 border-b border-borde bg-tinta px-4 py-3 text-white">
            <span className="flex h-9 w-9 items-center justify-center rounded-full bg-acento">
              <Icono nombre="robot" tam={18} />
            </span>
            <span className="flex flex-col">
              <b className="text-sm">Asistente del edificio</b>
              <span className="text-xs text-texto-tenue">{quien ? `Hablando con ${quien.propietario} · Dpto ${quien.codigo}` : telefono ? `Hablando con ${telefono}` : 'Elige un propietario'}</span>
            </span>
          </div>
          <div className="flex flex-1 flex-col gap-3 overflow-y-auto bg-fondo p-4" aria-live="polite" style={{ maxHeight: '55vh' }}>
            {chat.length === 0 && <p className="m-auto max-w-xs text-center text-sm text-texto-apoyo">Escribe un mensaje o toca una sugerencia para empezar.</p>}
            {chat.map((m, i) => (
              <div key={i} className={`flex max-w-[85%] flex-col gap-1 ${m.de === 'yo' ? 'self-end items-end' : 'self-start'}`}>
                <div className={`whitespace-pre-line rounded-tarjeta px-4 py-2 text-base ${m.de === 'yo' ? 'rounded-br-sm bg-acento text-white' : m.de === 'error' ? 'border border-alerta-borde bg-alerta-suave text-alerta-texto' : 'rounded-bl-sm border border-borde bg-superficie'}`}>{m.texto}</div>
                <span className="flex flex-wrap items-center gap-2 text-[11px] text-texto-apoyo">
                  {formatearHora(m.hora)}
                  {m.intencion && <span className="rounded-full bg-superficie-2 px-2 py-0.5 font-semibold text-egreso">intención: {m.intencion}</span>}
                </span>
                {m.datos && (
                  <details className="text-xs text-texto-apoyo">
                    <summary className="cursor-pointer">datos</summary>
                    <pre className="mt-1 max-w-full overflow-x-auto rounded bg-superficie-2 p-2">{JSON.stringify(m.datos, null, 2)}</pre>
                  </details>
                )}
              </div>
            ))}
            {pensando && (
              <div className="flex items-center gap-2 self-start rounded-tarjeta border border-borde bg-superficie px-4 py-2 text-sm text-texto-apoyo">
                <Spinner /> escribiendo…
              </div>
            )}
            <div ref={fin} />
          </div>
          <div className="flex gap-2 overflow-x-auto border-t border-borde px-3 py-2">
            {SUGERENCIAS.map((sug) => (
              <button key={sug} type="button" onClick={() => mandar(sug)} disabled={!telefono || pensando} className="h-9 shrink-0 rounded-full border border-acento-borde bg-acento-suave px-3 text-sm text-acento disabled:opacity-50">
                {sug}
              </button>
            ))}
          </div>
          <form
            className="flex gap-2 border-t border-borde p-3"
            onSubmit={(e) => {
              e.preventDefault();
              mandar();
            }}
          >
            <label className="sr-only" htmlFor="msg">
              Mensaje
            </label>
            <input id="msg" value={texto} onChange={(e) => setTexto(e.target.value)} placeholder={telefono ? 'Escribe un mensaje' : 'Primero elige un teléfono'} disabled={!telefono} className="h-11 min-w-0 flex-1 rounded-full border border-borde-fuerte bg-superficie px-4 text-base focus:outline-none focus:ring-2 focus:ring-acento" />
            <Boton type="submit" disabled={!texto.trim() || !telefono} cargando={pensando} icono="enviar" aria-label="Enviar">
              <span className="hidden sm:inline">Enviar</span>
            </Boton>
          </form>
        </div>
      </div>
    </>
  );
}
