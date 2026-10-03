import { useState } from 'react';
import { api, lista, subir } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFechaHora } from '../../lib/fechas.js';
import { infoSemaforo, restanteSLA } from '../../lib/operacion.js';
import { ruta } from '../../lib/nav.jsx';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Modal, SubirFoto, Vacio, useDialog, useToast } from '../../ui/index.js';

const PRIORIDAD = { alta: { tono: 'alerta', texto: 'Alta' }, media: { tono: 'aviso', texto: 'Media' }, baja: { tono: 'neutro', texto: 'Baja' } };
const ESTADO_OC = { abierta: { tono: 'aviso', texto: 'Abierta' }, cerrada: { tono: 'hecho', texto: 'Cerrada' }, escalada: { tono: 'curso', texto: 'Escalada' } };
const CATEGORIAS = [
  ['otros', 'Otros'], ['seguridad', 'Seguridad'], ['limpieza', 'Limpieza'], ['electricidad', 'Electricidad'], ['gasfiteria', 'Gasfitería'],
  ['ascensores', 'Ascensores'], ['bombas', 'Bombas'], ['areas_comunes', 'Áreas comunes'], ['estructura', 'Estructura'], ['jardineria', 'Jardinería'],
];
const FORM = { titulo: '', descripcion: '', prioridad: 'media', empleado: '', foto: null };

/** Bloques G1 y G2 · Cuaderno de ocurrencias y tickets con SLA (semáforo sobre las incidencias del tablero). */
export default function Ocurrencias() {
  const s = useSesion();
  const veTickets = s.tiene?.('incidencias.ver');
  const [vista, setVista] = useState(s.tiene?.('ocurrencias.ver') ? 'cuaderno' : 'tickets');
  return (
    <>
      <Encabezado
        titulo="Ocurrencias y tickets"
        subtitulo="Cuaderno de portería y plazos de atención"
        ayuda="El cuaderno guarda lo que pasa en el edificio: lo anotado no se edita. Una ocurrencia se cierra con nota o se escala al tablero de mantenimiento. Los tickets muestran el semáforo del plazo (SLA) de cada incidencia."
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          {s.tiene?.('ocurrencias.ver') && (
            <Chip activo={vista === 'cuaderno'} icono="documento" onClick={() => setVista('cuaderno')}>Cuaderno</Chip>
          )}
          {veTickets && (
            <Chip activo={vista === 'tickets'} icono="temporizador" onClick={() => setVista('tickets')}>Tickets y SLA</Chip>
          )}
        </div>
        {vista === 'cuaderno' ? <Cuaderno /> : <Tickets />}
      </Contenido>
    </>
  );
}

// ---------- G1 · cuaderno ----------

function Cuaderno() {
  const eid = useEid();
  const s = useSesion();
  const puede = s.tiene?.('ocurrencias.registrar');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [estado, setEstado] = useState('abierta');
  const [form, setForm] = useState(null);
  const [escalar, setEscalar] = useState(null); // {oc, categoria, ubicacion}
  const [ocupado, setOcupado] = useState(false);
  const ocs = useCarga(() => api.get(`/edificios/${eid}/ocurrencias`, { estado }), [eid, estado]);
  const filas = lista(ocs.datos);
  const conteos = ocs.datos?.conteos || {};

  const guardar = async (e) => {
    e?.preventDefault?.();
    if (!form.titulo.trim()) {
      await dialog.alert({ title: 'Falta el título', text: 'Escribe en una línea qué pasó.' });
      return;
    }
    setOcupado(true);
    try {
      const fd = new FormData();
      fd.set('titulo', form.titulo);
      fd.set('descripcion', form.descripcion);
      fd.set('prioridad', form.prioridad);
      fd.set('empleado', form.empleado);
      if (form.foto?.archivo) fd.set('foto', form.foto.archivo, 'ocurrencia.jpg');
      const r = await subir(`/edificios/${eid}/ocurrencias`, fd);
      toast(`${r.codigo} anotada en el cuaderno.`, { tipo: 'exito' });
      setForm(null);
      setEstado('abierta');
      await ocs.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo anotar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const cerrar = async (oc) => {
    const nota = await dialog.prompt({ title: `Cerrar ${oc.codigo}`, label: 'Cómo se resolvió', type: 'textarea', required: true, okText: 'Cerrar ocurrencia' });
    if (nota == null) return;
    try {
      await api.post(`/edificios/${eid}/ocurrencias/${oc.id}/cerrar`, { nota });
      toast(`${oc.codigo} cerrada.`, { tipo: 'exito' });
      await ocs.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo cerrar', text: err.message });
    }
  };

  const escalarAhora = async () => {
    setOcupado(true);
    try {
      const r = await api.post(`/edificios/${eid}/ocurrencias/${escalar.oc.id}/escalar`, { categoria: escalar.categoria, ubicacion: escalar.ubicacion });
      toast(`${escalar.oc.codigo} escalada como ${r.incidencia_codigo}.`, { tipo: 'exito' });
      setEscalar(null);
      await ocs.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo escalar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  return (
    <>
      {dialogEl}
      <Seccion
        titulo="Cuaderno de ocurrencias"
        extra={puede && <Boton tamano="sm" icono="mas_signo" onClick={() => setForm({ ...FORM })}>Anotar</Boton>}
      >
        <div className="mb-3 flex flex-wrap gap-2">
          {['abierta', 'escalada', 'cerrada', ''].map((x) => (
            <Chip key={x || 'todas'} activo={estado === x} onClick={() => setEstado(x)} contador={x ? conteos[x] : undefined}>
              {x ? ESTADO_OC[x].texto + 's' : 'Todas'}
            </Chip>
          ))}
        </div>
        {ocs.error ? (
          <ErrorCarga error={ocs.error} onReintentar={ocs.recargar} />
        ) : !ocs.datos ? (
          <Esqueleto className="h-40 w-full" />
        ) : filas.length === 0 ? (
          <Vacio icono="documento" titulo="Sin ocurrencias" texto="Anota aquí lo que pase en el turno: puertas abiertas, ruidos, daños, visitas sospechosas." />
        ) : (
          <ul className="divide-y divide-borde">
            {filas.map((oc) => (
              <li key={oc.id} className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start">
                {oc.foto_url && (
                  <a href={oc.foto_url} target="_blank" rel="noreferrer" className="shrink-0">
                    <img src={oc.foto_url} alt={`Foto de ${oc.codigo}`} className="h-14 w-14 rounded-control border border-borde object-cover" />
                  </a>
                )}
                <div className="min-w-0 flex-1">
                  <p className="flex flex-wrap items-center gap-2 text-sm">
                    <span className="font-semibold text-texto-suave">{oc.codigo}</span>
                    <b className="text-tinta">{oc.titulo}</b>
                  </p>
                  {oc.descripcion && <p className="mt-0.5 text-sm text-texto-suave">{oc.descripcion}</p>}
                  <p className="mt-1 text-xs text-texto-apoyo">
                    {[formatearFechaHora(oc.registrado_en), oc.empleado && `Turno: ${oc.empleado}`, oc.registrado_por && `Anotó ${oc.registrado_por}`].filter(Boolean).join(' · ')}
                  </p>
                  {oc.estado !== 'abierta' && (
                    <p className="mt-1 text-xs text-texto-suave">
                      {oc.estado === 'escalada' && oc.incidencia_codigo ? (
                        <a className="text-acento hover:text-acento-hover" href={ruta('mantenimiento')}>{oc.cierre_nota}</a>
                      ) : (
                        oc.cierre_nota
                      )}
                      {oc.cerrado_por && ` · ${oc.cerrado_por}`}
                    </p>
                  )}
                </div>
                <div className="flex shrink-0 flex-wrap items-center gap-2">
                  <Insignia estado="pendiente" tono={PRIORIDAD[oc.prioridad]?.tono} texto={PRIORIDAD[oc.prioridad]?.texto} icono={false} />
                  <Insignia estado={oc.estado === 'cerrada' ? 'terminado' : oc.estado === 'escalada' ? 'en_ejecucion' : 'pendiente'} tono={ESTADO_OC[oc.estado]?.tono} texto={ESTADO_OC[oc.estado]?.texto} />
                  {puede && oc.estado === 'abierta' && (
                    <>
                      <Boton tamano="sm" variante="secundario" onClick={() => cerrar(oc)}>Cerrar</Boton>
                      <Boton tamano="sm" variante="fantasma" onClick={() => setEscalar({ oc, categoria: 'otros', ubicacion: '' })}>Escalar</Boton>
                    </>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </Seccion>

      <Modal abierto={!!form} onCerrar={() => setForm(null)} titulo="Anotar ocurrencia" ancho="max-w-lg" pie={<><Boton variante="fantasma" onClick={() => setForm(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardar}>Anotar</Boton></>}>
        {form && (
          <form className="flex flex-col gap-3" onSubmit={guardar}>
            <Campo etiqueta="Qué pasó" valor={form.titulo} onCambio={(v) => setForm({ ...form, titulo: v })} placeholder="Puerta del sótano abierta" />
            <Campo etiqueta="Detalle" tipo="textarea" valor={form.descripcion} onCambio={(v) => setForm({ ...form, descripcion: v })} />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Prioridad" tipo="select" valor={form.prioridad} onCambio={(v) => setForm({ ...form, prioridad: v })} opciones={Object.entries(PRIORIDAD).map(([valor, p]) => ({ valor, etiqueta: p.texto }))} />
              <Campo etiqueta="Empleado de turno" valor={form.empleado} onCambio={(v) => setForm({ ...form, empleado: v })} />
            </div>
            <SubirFoto foto={form.foto} onFoto={(foto) => setForm((f) => ({ ...f, foto }))} etiqueta="Foto (opcional)" alto="h-40" oscuro={false} />
          </form>
        )}
      </Modal>

      <Modal abierto={!!escalar} onCerrar={() => setEscalar(null)} titulo={escalar ? `Escalar ${escalar.oc.codigo} a mantenimiento` : ''} ancho="max-w-md" pie={<><Boton variante="fantasma" onClick={() => setEscalar(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={escalarAhora}>Crear incidencia</Boton></>}>
        {escalar && (
          <div className="flex flex-col gap-3">
            <p className="text-sm text-texto-suave">Se crea una incidencia «reportada» en el tablero, con la foto como evidencia. La ocurrencia queda cerrada con el vínculo.</p>
            <Campo etiqueta="Categoría" tipo="select" valor={escalar.categoria} onCambio={(v) => setEscalar({ ...escalar, categoria: v })} opciones={CATEGORIAS.map(([valor, etiqueta]) => ({ valor, etiqueta }))} />
            <Campo etiqueta="Ubicación" valor={escalar.ubicacion} onCambio={(v) => setEscalar({ ...escalar, ubicacion: v })} />
          </div>
        )}
      </Modal>
    </>
  );
}

// ---------- G2 · tickets con SLA ----------

function Tickets() {
  const eid = useEid();
  const s = useSesion();
  const config = s.tiene?.('tickets.configurar');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [color, setColor] = useState('');
  const [sla, setSla] = useState(null);
  const [ocupado, setOcupado] = useState(false);
  const t = useCarga(() => api.get(`/edificios/${eid}/tickets`, { semaforo: color }), [eid, color]);
  const filas = lista(t.datos);
  const conteos = t.datos?.conteos || {};

  const abrirConfig = async () => {
    try {
      const r = await api.get(`/edificios/${eid}/tickets/sla`);
      setSla({ ...r.config, recalcular_abiertos: false });
    } catch (err) {
      await dialog.alert({ title: 'No se pudo leer el SLA', text: err.message });
    }
  };

  const guardarConfig = async () => {
    setOcupado(true);
    try {
      const cuerpo = { ...sla };
      for (const k of ['horas_critica', 'horas_media', 'horas_baja', 'horas_sin_clasificar']) cuerpo[k] = parseInt(cuerpo[k], 10) || 0;
      const r = await api.put(`/edificios/${eid}/tickets/sla`, cuerpo);
      toast(r.recalculados ? `Plazos guardados; ${r.recalculados} ticket(s) recalculados.` : 'Plazos guardados.', { tipo: 'exito' });
      setSla(null);
      await t.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo guardar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  return (
    <>
      {dialogEl}
      <Seccion titulo="Tickets abiertos por plazo" extra={config && <Boton tamano="sm" variante="secundario" icono="ajustes" onClick={abrirConfig}>Plazos y respuesta</Boton>}>
        <div className="mb-3 flex flex-wrap gap-2">
          <Chip activo={color === ''} onClick={() => setColor('')}>Todos</Chip>
          {['rojo', 'ambar', 'verde'].map((c) => (
            <Chip key={c} activo={color === c} tono={infoSemaforo(c).tono} onClick={() => setColor(c)} contador={conteos[c]}>
              {infoSemaforo(c).texto}
            </Chip>
          ))}
        </div>
        {t.error ? (
          <ErrorCarga error={t.error} onReintentar={t.recargar} />
        ) : !t.datos ? (
          <Esqueleto className="h-40 w-full" />
        ) : filas.length === 0 ? (
          <Vacio icono="hecho" titulo="Nada en este color" texto="Los tickets son las incidencias abiertas del tablero de mantenimiento." />
        ) : (
          <ul className="divide-y divide-borde">
            {filas.map((x) => {
              const sem = infoSemaforo(x.semaforo, x.sla_cumplido);
              return (
                <li key={x.id} className="flex flex-col gap-1.5 py-2.5 sm:flex-row sm:items-center sm:gap-3">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm">
                      <span className="font-semibold text-texto-suave">{x.codigo}</span> <b className="text-tinta">{x.titulo}</b>
                    </p>
                    <p className="truncate text-xs text-texto-apoyo">
                      {[x.unidad && `Dpto ${x.unidad}`, x.reportado_por_nombre, `Plazo ${x.sla_objetivo} h`, x.sla_vencimiento && `vence ${formatearFechaHora(x.sla_vencimiento)}`].filter(Boolean).join(' · ')}
                    </p>
                  </div>
                  <span className="text-xs tabular-nums text-texto-suave">{restanteSLA(x.sla_restante_min)}</span>
                  {x.criticidad && <Insignia estado={x.criticidad} />}
                  <Insignia estado="pendiente" tono={sem.tono} texto={sem.texto} />
                </li>
              );
            })}
          </ul>
        )}
        <p className="mt-3 text-xs text-texto-apoyo">
          Para mover un ticket de etapa usa el <a className="text-acento hover:text-acento-hover" href={ruta('mantenimiento')}>tablero de mantenimiento</a>; ahí también se ve el semáforo.
        </p>
      </Seccion>

      <Modal abierto={!!sla} onCerrar={() => setSla(null)} titulo="Plazos de atención (SLA)" ancho="max-w-lg" pie={<><Boton variante="fantasma" onClick={() => setSla(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarConfig}>Guardar</Boton></>}>
        {sla && (
          <div className="flex flex-col gap-3">
            <p className="text-sm text-texto-suave">Horas para resolver según la criticidad, contadas desde el reporte. Al clasificar un ticket, su vencimiento se recalcula.</p>
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Crítica (h)" tipo="numero" valor={String(sla.horas_critica)} onCambio={(v) => setSla({ ...sla, horas_critica: v })} />
              <Campo etiqueta="Media (h)" tipo="numero" valor={String(sla.horas_media)} onCambio={(v) => setSla({ ...sla, horas_media: v })} />
              <Campo etiqueta="Baja (h)" tipo="numero" valor={String(sla.horas_baja)} onCambio={(v) => setSla({ ...sla, horas_baja: v })} />
              <Campo etiqueta="Sin clasificar (h)" tipo="numero" valor={String(sla.horas_sin_clasificar)} onCambio={(v) => setSla({ ...sla, horas_sin_clasificar: v })} />
            </div>
            <label className="flex items-center gap-2 text-sm text-tinta">
              <input type="checkbox" checked={!!sla.respuesta_automatica} onChange={(e) => setSla({ ...sla, respuesta_automatica: e.target.checked })} />
              Respuesta automática al solicitante por WhatsApp
            </label>
            <Campo etiqueta="Mensaje" tipo="textarea" valor={sla.mensaje} onCambio={(v) => setSla({ ...sla, mensaje: v })} ayuda="Variables: {{nombre}}, {{codigo}}, {{titulo}}, {{plazo}}, {{vence}}" />
            <label className="flex items-center gap-2 text-sm text-tinta">
              <input type="checkbox" checked={!!sla.recalcular_abiertos} onChange={(e) => setSla({ ...sla, recalcular_abiertos: e.target.checked })} />
              Aplicar los plazos nuevos a los tickets abiertos
            </label>
          </div>
        )}
      </Modal>
    </>
  );
}
