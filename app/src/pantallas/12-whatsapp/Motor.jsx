import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { useEid } from '../../layout/Sesion.jsx';
import Encabezado from '../../layout/Encabezado.jsx';
import { Boton, Campo, Insignia, Modal, Tabla, TarjetaKPI, Vacio, useToast, infoEstado } from '../../ui/index.js';

const estadoConsulta = (estado) => {
  if (estado === 'ok') return { tono: 'exito', texto: 'OK' };
  if (estado.startsWith('rechazado')) return { tono: 'error', texto: 'Rechazada' };
  if (estado.startsWith('error')) return { tono: 'alerta', texto: 'Error de base' };
  return infoEstado(estado);
};

/** Pantalla Motor (F8): salud del LLM local, golden SQL con guardas, consultas auditadas
 *  y confirmación de correcciones que el motor propone aprender (F5/F6). */
export default function Motor() {
  const eid = useEid();
  const { toast } = useToast();
  const datos = useCarga(() => api.get('/motor/admin'), [eid]);
  const d = datos.datos || {};

  const [editando, setEditando] = useState(null);   // {id?, pregunta, sql} | null
  const [viendo, setViendo] = useState(null);       // consulta o fila de golden (detalle SQL)
  const [confirmando, setConfirmando] = useState(null); // propuesta {pregunta, respuesta}
  const [probando, setProbando] = useState('');     // pregunta libre contra el motor
  const [respuesta, setRespuesta] = useState(null);
  const [pensando, setPensando] = useState(false);

  const modo = d.modo || 'off';
  const salud = d.salud || 'off';
  const golden = lista(d.golden);
  const consultas = lista(d.consultas);
  const propuestas = lista(d.propuestas);
  const interacciones = lista(d.interacciones);
  const ultimas = interacciones.slice(-5).reverse();
  const rechazadas = consultas.filter((c) => c.estado && c.estado !== 'ok').length;
  const okDeGolden = golden.filter((g) => !g.desactivada).length;

  const cambiarModo = async (m) => {
    try {
      await api.put('/motor/ajuste', { modo: m });
      toast(`Motor ${m === 'off' ? 'apagado' : m === 'todos' ? 'para todos' : 'solo administración'}.`, { tipo: 'exito' });
      datos.recargar();
    } catch (err) {
      toast(err.message, { tipo: 'error' });
    }
  };

  const guardarGolden = async (g) => {
    try {
      await api.post('/motor/golden', g);
      toast(g.id ? 'Golden actualizado.' : 'Golden creado.', { tipo: 'exito' });
      setEditando(null);
      datos.recargar();
    } catch (err) {
      toast(err.message, { tipo: 'error' });
    }
  };

  const alternarGolden = async (g) => {
    try {
      await api.post('/motor/golden', { id: g.id, desactivada: !g.desactivada });
      toast(g.desactivada ? 'Golden activado.' : 'Golden desactivado.', { tipo: 'exito' });
      datos.recargar();
    } catch (err) {
      toast(err.message, { tipo: 'error' });
    }
  };

  const ejecutarGolden = async (g) => {
    try {
      const r = await api.post(`/motor/golden/${g.id}/ejecutar`);
      setViendo({ titulo: g.pregunta, sql: g.sql, filas: r.filas });
      toast('Golden ejecutado y verificado.', { tipo: 'exito' });
      datos.recargar();
    } catch (err) {
      toast(err.message, { tipo: 'error' });
    }
  };

  const probar = async () => {
    const pregunta = probando.trim();
    if (!pregunta || pensando) return;
    setPensando(true);
    setRespuesta(null);
    try {
      const r = await api.post('/motor/consulta', { pregunta });
      setRespuesta(r);
    } catch (err) {
      setRespuesta({ error: err.message });
    } finally {
      setPensando(false);
    }
  };

  const confirmar = async (p) => {
    try {
      await api.post('/motor/confirma', { pregunta: p.pregunta, respuesta: p.respuesta });
      toast('Corrección confirmada: el motor la aprendió.', { tipo: 'exito' });
      setConfirmando(null);
      datos.recargar();
    } catch (err) {
      toast(err.message, { tipo: 'error' });
    }
  };

  return (
    <>
      <Encabezado
        titulo="Motor conversacional"
        subtitulo="LLM local en español: responde lo que las reglas del chatbot no entendieron. GoldenSQL con guardas para las consultas de datos."
        volver="/app/whatsapp/"
      />
      <div className="flex flex-col gap-5 p-4 lg:p-6">
        {/* Estado y modo */}
        <section className="grid gap-3 sm:grid-cols-3">
          <TarjetaKPI titulo="Estado del motor" valor={salud === 'ok' ? 'Operativo' : salud === 'caido' ? 'Caído' : 'Apagado'} icono="robot"
            tono={salud === 'ok' ? 'acento' : salud === 'caido' ? 'alerta' : 'neutro'}
            nota={salud === 'caido' ? 'Revisa: docker compose ps' : modo === 'off' ? 'El chatbot por reglas contesta todo' : 'LLM local sin salir a internet'} />
          <TarjetaKPI titulo="Golden activos" valor={String(okDeGolden)} icono="llave" nota="Preguntas→SQL verificados" />
          <TarjetaKPI titulo="Consultas rechazadas" valor={String(rechazadas)} icono="alerta" tono={rechazadas ? 'aviso' : 'neutro'} nota="Bloqueadas por las guardas" />
        </section>

        <section className="flex flex-wrap items-center gap-2 rounded-tarjeta border border-borde bg-superficie p-3">
          <span className="mr-1 text-sm font-semibold text-tinta">Motor activado:</span>
          {['off', 'solo_admin', 'todos'].map((m) => (
            <button key={m} type="button" onClick={() => cambiarModo(m)} disabled={modo === m}
              className={`h-8 rounded-chip border px-3 text-sm transition-colors disabled:cursor-default ${modo === m ? 'border-acento bg-acento text-white' : 'border-borde-fuerte bg-superficie text-tinta hover:bg-superficie-2'}`}>
              {m === 'off' ? 'Apagado' : m === 'solo_admin' ? 'Solo admin' : 'Todos'}
            </button>
          ))}
          <span className="ml-auto text-xs text-texto-apoyo">
            {salud === 'caido' ? 'El servicio motor no responde (¿modelos bajados? ¿docker compose up motor?)' : `Ajuste por edificio · hoy: ${modo}`}
          </span>
        </section>

        {/* Probar el motor */}
        <section className="flex flex-col gap-2 rounded-tarjeta border border-borde bg-superficie p-4">
          <h2 className="font-titulo text-base font-semibold text-tinta">Probar el motor</h2>
          <p className="text-xs text-texto-apoyo">Pregunta algo que las reglas no cubran (p. ej. «explícame el reparto de medidores»). El puntaje alto es buena señal; negativo, respuesta evasiva.</p>
          <div className="flex gap-2">
            <Campo ocultarEtiqueta etiqueta="Pregunta" valor={probando} onCambio={setProbando} placeholder="Tu pregunta para el motor…" className="flex-1"
              onKeyDown={(e) => e.key === 'Enter' && probar()} />
            <Boton onClick={probar} cargando={pensando} disabled={!probando.trim()}>Probar</Boton>
          </div>
          {respuesta && (
            <div className="flex flex-col gap-1 rounded-tarjeta border border-borde bg-fondo p-3">
              {respuesta.error ? (
                <span className="text-sm text-alerta">{respuesta.error}</span>
              ) : (
                <>
                  <p className="whitespace-pre-line text-sm text-tinta">{respuesta.respuesta}</p>
                  <span className="text-xs text-texto-apoyo">
                    puntaje {respuesta.puntaje}
                    {respuesta.golden ? ` · golden #${respuesta.golden}` : ''}
                    {respuesta.tokens_generados ? ` · ${respuesta.tokens_generados} tokens` : ''}
                  </span>
                </>
              )}
            </div>
          )}
        </section>

        {/* Golden */}
        <section className="flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <h2 className="font-titulo text-base font-semibold text-tinta">Golden SQL</h2>
            <Boton variante="secundario" onClick={() => setEditando({ pregunta: '', sql: '' })}>Nuevo golden</Boton>
          </div>
          <Tabla
            etiqueta="Golden SQL"
            claveFila="id"
            filas={golden}
            cargando={datos.cargando}
            error={datos.error}
            onReintentar={datos.recargar}
            vacio={<Vacio titulo="Sin golden SQL" texto="Crea el primero: una pregunta y su SELECT verificado." />}
            columnas={[
              { clave: 'pregunta', titulo: 'Pregunta', movil: 'titulo' },
              { clave: 'sql', titulo: 'SQL', render: (g) => (
                <button type="button" onClick={() => setViendo({ titulo: g.pregunta, sql: g.sql })} className="max-w-[220px] truncate text-xs text-acento underline decoration-dotted">ver SQL</button>) },
              { clave: 'tablas', titulo: 'Tablas', render: (g) => <span className="text-xs text-texto-apoyo">{(g.tablas || []).join(', ') || '—'}</span>, prioridad: 3 },
              { clave: 'veces_usada', titulo: 'Usos', alinear: 'der', prioridad: 3 },
              { clave: 'verifico', titulo: 'Verificado', prioridad: 2 },
              { clave: 'fuente', titulo: '', render: (g) => (g.fuente === 'generada_aprobada' ? <Insignia estado="validado" texto="aprendido" /> : null), movil: 'oculto' },
              { clave: 'estado', titulo: '', render: (g) => (g.desactivada ? <Insignia estado="anulado" texto="desactivado" /> : null), movil: 'oculto' },
              { clave: 'acciones', titulo: '', render: (g) => (
                <div className="flex justify-end gap-1">
                  <Boton variante="fantasma" onClick={() => ejecutarGolden(g)}>Ejecutar</Boton>
                  <Boton variante="fantasma" onClick={() => setEditando({ id: g.id, pregunta: g.pregunta, sql: g.sql })}>Editar</Boton>
                  <Boton variante="fantasma" onClick={() => alternarGolden(g)}>{g.desactivada ? 'Activar' : 'Apagar'}</Boton>
                </div>) },
            ]}
          />
        </section>

        {/* Consultas auditadas */}
        <section className="flex flex-col gap-2">
          <h2 className="font-titulo text-base font-semibold text-tinta">Últimas consultas</h2>
          <Tabla
            etiqueta="Consultas del motor"
            claveFila="id"
            filas={consultas}
            cargando={datos.cargando}
            densa
            vacio={<Vacio titulo="Sin consultas todavía" texto="Cuando el motor ejecute o rechace un golden, queda aquí." />}
            columnas={[
              { clave: 'cuando', titulo: 'Cuándo' },
              { clave: 'pregunta', titulo: 'Pregunta', movil: 'titulo' },
              { clave: 'estado', titulo: 'Estado', render: (c) => { const e = estadoConsulta(c.estado); return <Insignia tono={e.tono} texto={e.texto} icono={false} />; } },
              { clave: 'duracion_ms', titulo: 'Duración', alinear: 'der', render: (c) => <span className="tabular-nums">{c.duracion_ms} ms</span>, prioridad: 2 },
              { clave: 'filas', titulo: 'Filas', alinear: 'der', prioridad: 3 },
              { clave: 'sql', titulo: '', render: (c) => (c.sql ? <button type="button" onClick={() => setViendo({ titulo: c.pregunta, sql: c.sql })} className="text-xs text-acento underline decoration-dotted">SQL</button> : null), movil: 'oculto' },
            ]}
          />
        </section>

        {/* Correcciones propuestas por los usuarios */}
        {propuestas.length > 0 && (
          <section className="flex flex-col gap-2">
            <h2 className="font-titulo text-base font-semibold text-tinta">Correcciones propuestas ({propuestas.length})</h2>
            <div className="grid gap-2">
              {propuestas.map((p, i) => (
                <div key={i} className="flex flex-col gap-2 rounded-tarjeta border border-borde bg-superficie p-3 sm:flex-row sm:items-center">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-semibold text-tinta">{p.pregunta}</p>
                    <p className="truncate text-xs text-texto-apoyo">{p.respuesta}</p>
                  </div>
                  <div className="flex gap-1">
                    <Boton variante="secundario" onClick={() => setConfirmando(p)}>Revisar</Boton>
                  </div>
                </div>
              ))}
            </div>
          </section>
        )}

        {/* Conversación reciente del motor */}
        {ultimas.length > 0 && (
          <section className="flex flex-col gap-2">
            <h2 className="font-titulo text-base font-semibold text-tinta">Conversación reciente</h2>
            <div className="grid gap-1.5">
              {ultimas.map((x, i) => (
                <div key={i} className="flex items-center gap-3 rounded-tarjeta border border-borde bg-superficie px-3 py-2">
                  <span className="min-w-0 flex-1 truncate text-sm text-tinta">{x.pregunta}</span>
                  <Insignia estado="simulado" texto={x.intencion || '—'} icono={false} />
                </div>
              ))}
            </div>
          </section>
        )}
      </div>

      {/* Modal: crear/editar golden */}
      <Modal abierto={!!editando} onCerrar={() => setEditando(null)} titulo={editando?.id ? 'Editar golden' : 'Nuevo golden'} ancho="max-w-xl"
        pie={<>
          <Boton variante="fantasma" onClick={() => setEditando(null)}>Cancelar</Boton>
          <Boton onClick={() => guardarGolden(editando)} disabled={!editando?.pregunta?.trim() || !editando?.sql?.trim()}>Guardar</Boton>
        </>}>
        {editando && (
          <div className="flex flex-col gap-3">
            <Campo etiqueta="Pregunta" valor={editando.pregunta} onCambio={(v) => setEditando({ ...editando, pregunta: v })} ayuda="La frase que la gente pregunta y el chatbot por reglas no sabe responder." />
            <Campo etiqueta="SQL (solo SELECT, con edificio_id = :edificio_id)" tipo="textarea" valor={editando.sql} onCambio={(v) => setEditando({ ...editando, sql: v })}
              ayuda="Las guardas lo revisan al guardar: lista blanca de tablas, sin escritura, sin comentarios. :edificio_id se reemplaza por el edificio." />
            <p className="text-xs text-texto-apoyo">Tablas permitidas: recibo, recibo_linea, pago, periodo, unidad, area, recurso, reserva, incidencia, medidor, lectura, reparto_medidor, egreso, movimiento_banco, edificio, rubro, concepto, presupuesto, recibo_general.</p>
          </div>
        )}
      </Modal>

      {/* Modal: detalle SQL + filas */}
      <Modal abierto={!!viendo} onCerrar={() => setViendo(null)} titulo={viendo?.titulo || 'SQL'} ancho="max-w-xl">
        {viendo && (
          <div className="flex flex-col gap-3">
            <pre className="max-h-48 overflow-auto rounded-tarjeta bg-superficie-2 p-3 text-xs text-tinta">{viendo.sql}</pre>
            {viendo.filas && (
              <div className="max-h-64 overflow-auto rounded-tarjeta border border-borde">
                <table className="w-full text-xs">
                  <tbody>
                    {viendo.filas.map((f, i) => (
                      <tr key={i} className="border-b border-borde last:border-0">
                        {Object.entries(f).map(([k, v]) => (
                          <td key={k} className="px-3 py-1.5 align-top">
                            <span className="font-semibold text-texto-apoyo">{k}: </span>
                            <span className="tabular-nums">{String(v)}</span>
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </Modal>

      {/* Modal: confirmar corrección */}
      <Modal abierto={!!confirmando} onCerrar={() => setConfirmando(null)} titulo="Confirmar corrección"
        pie={<>
          <Boton variante="fantasma" onClick={() => setConfirmando(null)}>Descartar</Boton>
          <Boton onClick={() => confirmar(confirmando)}>Confirmar y enseñar</Boton>
        </>}>
        {confirmando && (
          <div className="flex flex-col gap-3">
            <div>
              <p className="text-xs font-semibold uppercase tracking-wide text-texto-apoyo">Pregunta</p>
              <p className="text-sm text-tinta">{confirmando.pregunta}</p>
            </div>
            <div>
              <p className="text-xs font-semibold uppercase tracking-wide text-texto-apoyo">Respuesta propuesta</p>
              <p className="whitespace-pre-line text-sm text-tinta">{confirmando.respuesta}</p>
            </div>
            <p className="text-xs text-texto-apoyo">Al confirmar, esta pareja entra a la memoria del motor y la usará cuando vuelvan a preguntar algo parecido. El «Descartar» solo cierra; para borrarla de la lista, confirma o espera a que caduque.</p>
          </div>
        )}
      </Modal>
    </>
  );
}
