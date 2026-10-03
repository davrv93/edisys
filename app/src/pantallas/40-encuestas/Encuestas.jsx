import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Modal, Vacio, useDialog, useToast } from '../../ui/index.js';
import { TIPOS_PREGUNTA, alternarOpcion, cuerpoEncuesta, cuerpoRespuestas, formularioDeEncuesta, formularioVacio, preguntaVacia } from './encuesta.js';

// Estado de la encuesta → insignia (tono + texto propios: estos estados no están en el mapa común).
const ESTADO = {
  borrador: { estado: 'borrador', texto: 'Borrador' },
  abierta: { estado: 'pendiente', tono: 'curso', texto: 'Abierta' },
  cerrada: { estado: 'terminado', texto: 'Cerrada' },
};

/** Bloque J1 · Encuestas: la administración pregunta, residentes y junta responden una vez, todos ven el resultado al cierre. */
export default function Encuestas() {
  const eid = useEid();
  const s = useSesion();
  const admin = s.tiene?.('encuestas.administrar');
  const puedeResponder = s.tiene?.('encuestas.responder');
  const veParciales = s.tiene?.('encuestas.resultados');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [filtro, setFiltro] = useState('todas');

  const encs = useCarga(() => api.get(`/edificios/${eid}/encuestas`), [eid]);
  const todas = lista(encs.datos);
  const visibles = filtro === 'todas' ? todas : todas.filter((x) => x.estado === filtro);

  const [form, setForm] = useState(null); // { id?, ...formulario }
  const [responder, setResponder] = useState(null); // { enc, marcadas }
  const [resultados, setResultados] = useState(null);
  const [ocupado, setOcupado] = useState(false);

  const fallo = (titulo) => (err) => dialog.alert({ title: titulo, text: err.message });

  const editar = async (enc) => {
    try {
      const det = await api.get(`/edificios/${eid}/encuestas/${enc.id}`);
      setForm({ id: enc.id, ...formularioDeEncuesta(det) });
    } catch (err) {
      await fallo('No se pudo abrir la encuesta')(err);
    }
  };

  const guardar = async (e) => {
    e?.preventDefault?.();
    const { cuerpo, error } = cuerpoEncuesta(form);
    if (error) {
      await dialog.alert({ title: 'Revisa la encuesta', text: error });
      return;
    }
    setOcupado(true);
    try {
      if (form.id) await api.put(`/edificios/${eid}/encuestas/${form.id}`, cuerpo);
      else await api.post(`/edificios/${eid}/encuestas`, cuerpo);
      toast(form.id ? 'Encuesta actualizada.' : 'Encuesta guardada en borrador.', { tipo: 'exito' });
      setForm(null);
      await encs.recargar();
    } catch (err) {
      await fallo('No se pudo guardar')(err);
    } finally {
      setOcupado(false);
    }
  };

  const transicion = async (enc, accion) => {
    const textos = {
      abrir: { title: '¿Abrir la encuesta?', text: 'Desde ahora residentes y junta podrán responder y ya no podrás editar las preguntas.' },
      cerrar: { title: '¿Cerrar la encuesta?', text: 'No se aceptarán más respuestas y los resultados quedarán visibles para todos.', danger: true },
    };
    if (!(await dialog.confirm(textos[accion]))) return;
    try {
      await api.post(`/edificios/${eid}/encuestas/${enc.id}/${accion}`);
      toast(accion === 'abrir' ? 'Encuesta abierta.' : 'Encuesta cerrada.', { tipo: 'exito' });
      await encs.recargar();
    } catch (err) {
      await fallo('No se pudo cambiar el estado')(err);
    }
  };

  const borrar = async (enc) => {
    if (!(await dialog.confirm({ title: '¿Borrar el borrador?', text: `«${enc.titulo}» se elimina con sus preguntas.`, danger: true }))) return;
    try {
      await api.del(`/edificios/${eid}/encuestas/${enc.id}`);
      await encs.recargar();
    } catch (err) {
      await fallo('No se pudo borrar')(err);
    }
  };

  const abrirResponder = async (enc) => {
    try {
      const det = await api.get(`/edificios/${eid}/encuestas/${enc.id}`);
      setResponder({ enc: det, marcadas: {} });
    } catch (err) {
      await fallo('No se pudo abrir la encuesta')(err);
    }
  };

  const enviarRespuestas = async () => {
    const { cuerpo, faltan } = cuerpoRespuestas(responder.enc.preguntas, responder.marcadas);
    if (faltan.length) {
      await dialog.alert({ title: 'Faltan respuestas', text: `Responde: ${faltan.join(' · ')}` });
      return;
    }
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/encuestas/${responder.enc.id}/respuestas`, cuerpo);
      toast('¡Gracias! Tu respuesta quedó registrada.', { tipo: 'exito' });
      setResponder(null);
      await encs.recargar();
    } catch (err) {
      await fallo('No se pudo enviar')(err);
    } finally {
      setOcupado(false);
    }
  };

  const verResultados = async (enc) => {
    try {
      setResultados(await api.get(`/edificios/${eid}/encuestas/${enc.id}/resultados`));
    } catch (err) {
      await fallo('No se pudieron cargar los resultados')(err);
    }
  };

  const setPregunta = (i, cambio) => setForm({ ...form, preguntas: form.preguntas.map((p, j) => (j === i ? { ...p, ...cambio } : p)) });

  const conteo = (estado) => todas.filter((x) => x.estado === estado).length;

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Encuestas"
        subtitulo="Consultas a propietarios, inquilinos y junta"
        ayuda="Arma la encuesta en borrador, ábrela cuando esté lista y ciérrala para publicar los resultados. Cada persona responde una sola vez."
        acciones={admin && <Boton icono="mas_signo" onClick={() => setForm(formularioVacio())}>Nueva encuesta</Boton>}
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={filtro === 'todas'} onClick={() => setFiltro('todas')} contador={todas.length}>Todas</Chip>
          <Chip activo={filtro === 'abierta'} tono="curso" onClick={() => setFiltro('abierta')} contador={conteo('abierta')}>Abiertas</Chip>
          <Chip activo={filtro === 'cerrada'} tono="hecho" onClick={() => setFiltro('cerrada')} contador={conteo('cerrada')}>Cerradas</Chip>
          {admin && <Chip activo={filtro === 'borrador'} tono="neutro" onClick={() => setFiltro('borrador')} contador={conteo('borrador')}>Borradores</Chip>}
        </div>

        <Seccion titulo="Encuestas del edificio">
          {encs.error ? (
            <ErrorCarga error={encs.error} onReintentar={encs.recargar} />
          ) : !encs.datos ? (
            <Esqueleto className="h-40 w-full" />
          ) : visibles.length === 0 ? (
            <Vacio icono="votos" titulo="Sin encuestas" texto={admin ? 'Crea la primera: por ejemplo, el color de la fachada o el horario del gimnasio.' : 'Cuando la administración publique una encuesta aparecerá aquí.'} />
          ) : (
            <ul className="divide-y divide-borde">
              {visibles.map((x) => {
                const est = ESTADO[x.estado] || ESTADO.borrador;
                const verRes = x.estado === 'cerrada' || veParciales;
                return (
                  <li key={x.id} className="flex flex-wrap items-center gap-3 py-3">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-tinta">{x.titulo}</p>
                      <p className="truncate text-xs text-texto-apoyo">
                        {[`${x.preguntas} pregunta(s)`, `${x.respuestas} respuesta(s)`, x.cierra_en && `cierra ${formatearFecha(x.cierra_en)}`, x.anonima ? 'anónima' : 'con nombre']
                          .filter(Boolean)
                          .join(' · ')}
                      </p>
                    </div>
                    <Insignia estado={est.estado} tono={est.tono} texto={est.texto} />
                    {x.respondida && <Insignia estado="hecho" tono="acento" texto="Respondiste" />}
                    {puedeResponder && x.acepta_respuestas && !x.respondida && (
                      <Boton tamano="sm" onClick={() => abrirResponder(x)}>Responder</Boton>
                    )}
                    {verRes && x.estado !== 'borrador' && (
                      <Boton tamano="sm" variante="fantasma" onClick={() => verResultados(x)}>Resultados</Boton>
                    )}
                    {admin && x.estado === 'borrador' && (
                      <>
                        <Boton tamano="sm" variante="fantasma" onClick={() => editar(x)}>Editar</Boton>
                        <Boton tamano="sm" variante="fantasma" onClick={() => borrar(x)}>Borrar</Boton>
                        <Boton tamano="sm" onClick={() => transicion(x, 'abrir')}>Abrir</Boton>
                      </>
                    )}
                    {admin && x.estado === 'abierta' && (
                      <Boton tamano="sm" variante="fantasma" onClick={() => transicion(x, 'cerrar')}>Cerrar</Boton>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </Seccion>
      </Contenido>

      {/* Editor (solo borradores) */}
      <Modal
        abierto={!!form}
        onCerrar={() => setForm(null)}
        titulo={form?.id ? 'Editar encuesta' : 'Nueva encuesta'}
        ancho="max-w-2xl"
        pie={<><Boton variante="fantasma" onClick={() => setForm(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardar}>Guardar borrador</Boton></>}
      >
        {form && (
          <form className="flex flex-col gap-3" onSubmit={guardar}>
            <Campo etiqueta="Título" valor={form.titulo} onCambio={(v) => setForm({ ...form, titulo: v })} />
            <Campo etiqueta="Descripción" tipo="textarea" valor={form.descripcion} onCambio={(v) => setForm({ ...form, descripcion: v })} />
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <Campo etiqueta="Cierra el (opcional)" tipo="fecha" valor={form.cierra_en} onCambio={(v) => setForm({ ...form, cierra_en: v || '' })} />
              <label className="flex items-center gap-2 self-end pb-2 text-sm text-tinta">
                <input type="checkbox" className="h-4 w-4" checked={form.anonima} onChange={(e) => setForm({ ...form, anonima: e.target.checked })} />
                Anónima (los resultados no muestran quién respondió)
              </label>
            </div>
            {form.preguntas.map((p, i) => (
              <fieldset key={i} className="flex flex-col gap-2 rounded-control border border-borde p-3">
                <legend className="px-1 text-xs font-semibold text-texto-apoyo">Pregunta {i + 1}</legend>
                <Campo etiqueta="Pregunta" valor={p.texto} onCambio={(v) => setPregunta(i, { texto: v })} />
                <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                  <Campo etiqueta="Tipo" tipo="select" valor={p.tipo} onCambio={(v) => setPregunta(i, { tipo: v })} opciones={TIPOS_PREGUNTA} />
                  <label className="flex items-center gap-2 self-end pb-2 text-sm text-tinta">
                    <input type="checkbox" className="h-4 w-4" checked={p.obligatoria} onChange={(e) => setPregunta(i, { obligatoria: e.target.checked })} />
                    Obligatoria
                  </label>
                </div>
                {p.tipo !== 'texto' && (
                  <Campo etiqueta="Opciones (una por línea)" tipo="textarea" valor={p.opciones} onCambio={(v) => setPregunta(i, { opciones: v })} />
                )}
                {form.preguntas.length > 1 && (
                  <div>
                    <Boton tamano="sm" variante="fantasma" onClick={() => setForm({ ...form, preguntas: form.preguntas.filter((_, j) => j !== i) })}>Quitar pregunta</Boton>
                  </div>
                )}
              </fieldset>
            ))}
            <div>
              <Boton tamano="sm" variante="secundario" icono="mas_signo" onClick={() => setForm({ ...form, preguntas: [...form.preguntas, preguntaVacia()] })}>Agregar pregunta</Boton>
            </div>
          </form>
        )}
      </Modal>

      {/* Responder */}
      <Modal
        abierto={!!responder}
        onCerrar={() => setResponder(null)}
        titulo={responder?.enc?.titulo || 'Responder'}
        ancho="max-w-lg"
        pie={<><Boton variante="fantasma" onClick={() => setResponder(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={enviarRespuestas}>Enviar respuesta</Boton></>}
      >
        {responder && (
          <div className="flex flex-col gap-4">
            {responder.enc.descripcion && <p className="text-sm text-texto-suave">{responder.enc.descripcion}</p>}
            {responder.enc.preguntas.map((p) => (
              <fieldset key={p.id} className="flex flex-col gap-2">
                <legend className="mb-1 text-sm font-semibold text-tinta">
                  {p.texto} {p.obligatoria && <span className="text-alerta" aria-label="obligatoria">*</span>}
                </legend>
                {p.tipo === 'texto' ? (
                  <Campo etiqueta={p.texto} ocultarEtiqueta tipo="textarea" valor={responder.marcadas[p.id] || ''} onCambio={(v) => setResponder({ ...responder, marcadas: { ...responder.marcadas, [p.id]: v } })} />
                ) : (
                  p.opciones.map((o) => (
                    <label key={o.id} className="flex items-center gap-2 text-sm text-tinta">
                      <input
                        type={p.tipo === 'unica' ? 'radio' : 'checkbox'}
                        name={`p-${p.id}`}
                        className="h-4 w-4"
                        checked={(responder.marcadas[p.id] || []).includes(o.id)}
                        onChange={() => setResponder({ ...responder, marcadas: alternarOpcion(responder.marcadas, p, o.id) })}
                      />
                      {o.texto}
                    </label>
                  ))
                )}
              </fieldset>
            ))}
          </div>
        )}
      </Modal>

      {/* Resultados agregados */}
      <Modal abierto={!!resultados} onCerrar={() => setResultados(null)} titulo={resultados ? `Resultados · ${resultados.titulo}` : 'Resultados'} ancho="max-w-2xl">
        {resultados && (
          <div className="flex flex-col gap-5">
            <p className="text-sm text-texto-suave">
              {resultados.respuestas} respuesta(s){resultados.estado === 'abierta' ? ' · resultados parciales, la encuesta sigue abierta' : ''}
            </p>
            {resultados.preguntas.map((p) => (
              <div key={p.id} className="flex flex-col gap-2">
                <p className="text-sm font-semibold text-tinta">{p.texto}</p>
                {p.tipo === 'texto' ? (
                  p.textos.length === 0 ? (
                    <p className="text-xs text-texto-apoyo">Sin comentarios.</p>
                  ) : (
                    <ul className="flex flex-col gap-1.5">
                      {p.textos.map((t, i) => (
                        <li key={i} className="rounded-control bg-superficie-2 px-3 py-2 text-sm text-tinta">
                          {t.texto}
                          {t.autor && <span className="ml-2 text-xs text-texto-apoyo">— {t.autor}</span>}
                        </li>
                      ))}
                    </ul>
                  )
                ) : (
                  <ul className="flex flex-col gap-2">
                    {p.opciones.map((o) => (
                      <li key={o.id} className="flex flex-col gap-1">
                        <div className="flex items-baseline justify-between gap-2 text-sm">
                          <span className="text-tinta">{o.texto}</span>
                          <span className="tabular-nums text-texto-suave">{o.votos} · {o.porcentaje} %</span>
                        </div>
                        <div className="h-2 w-full overflow-hidden rounded-chip bg-superficie-2" role="img" aria-label={`${o.texto}: ${o.porcentaje} %`}>
                          <div className="h-full rounded-chip bg-acento" style={{ width: `${o.porcentaje}%` }} />
                        </div>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            ))}
          </div>
        )}
      </Modal>
    </>
  );
}
