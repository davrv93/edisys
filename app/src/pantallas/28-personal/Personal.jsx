import { useState } from 'react';
import { api, lista, subir } from '../../lib/api.js';
import { PREFIJO } from '../../lib/base.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Icono, Insignia, Modal, Tabla, Vacio, useDialog, useToast } from '../../ui/index.js';

/** Bloque F1 · Colaboradores del edificio y sus documentos (CV, ficha, PLAME). Turnos y checklist del bloque F2. */

const TIPOS_DOC = [
  { valor: 'cv', etiqueta: 'CV' },
  { valor: 'ficha', etiqueta: 'Ficha de datos' },
  { valor: 'plame', etiqueta: 'PLAME' },
  { valor: 'contrato', etiqueta: 'Contrato' },
  { valor: 'otro', etiqueta: 'Otro' },
];
const NOMBRE_TIPO = Object.fromEntries(TIPOS_DOC.map((t) => [t.valor, t.etiqueta]));
const DIAS = ['L', 'M', 'X', 'J', 'V', 'S', 'D'];

// Los enlaces firmados vienen relativos al dominio («/api/v1/archivos/…»): se les antepone el prefijo de la app.
const enlace = (u) => (u ? PREFIJO + u : null);

export default function Personal() {
  const eid = useEid();
  const s = useSesion();
  const admin = s.tiene?.('personal.administrar');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [vista, setVista] = useState('colaboradores');
  const [verInactivos, setVerInactivos] = useState(false);

  const cols = useCarga(() => api.get(`/edificios/${eid}/colaboradores`, { todos: verInactivos ? 1 : '' }), [eid, verInactivos]);
  const turnos = useCarga(() => api.get(`/edificios/${eid}/turnos`), [eid]);
  const checklist = useCarga(() => api.get(`/edificios/${eid}/checklist`), [eid], { activo: !!admin });
  const cuentas = useCarga(() => api.get(`/edificios/${eid}/personal/cuentas`), [eid], { activo: !!admin });
  const listaCols = lista(cols.datos);
  const listaTurnos = lista(turnos.datos);

  const [detalle, setDetalle] = useState(null); // colaborador abierto
  const [form, setForm] = useState(null); // alta/edición de colaborador
  const [formTurno, setFormTurno] = useState(null);
  const [ocupado, setOcupado] = useState(false);

  const fallo = (titulo) => (err) => dialog.alert({ icon: '⚠️', title: titulo, text: err.message });

  const guardarColaborador = async (e) => {
    e?.preventDefault?.();
    setOcupado(true);
    try {
      const cuerpo = {
        nombre: form.nombre, tipo_doc: form.tipo_doc, num_doc: form.num_doc, cargo: form.cargo, telefono: form.telefono, correo: form.correo,
        turno_id: Number(form.turno_id) || 0, usuario_id: Number(form.usuario_id) || 0, fecha_ingreso: form.fecha_ingreso || '',
      };
      let id = form.id;
      if (id) await api.put(`/edificios/${eid}/colaboradores/${id}`, cuerpo);
      else id = (await api.post(`/edificios/${eid}/colaboradores`, cuerpo)).id;
      if (form.foto) {
        const fd = new FormData();
        fd.set('foto', form.foto);
        await subir(`/edificios/${eid}/colaboradores/${id}/foto`, fd);
      }
      toast(form.id ? 'Colaborador actualizado.' : 'Colaborador registrado.', { tipo: 'exito' });
      setForm(null);
      await cols.recargar();
    } catch (err) {
      await fallo('No se pudo guardar')(err);
    } finally {
      setOcupado(false);
    }
  };

  const alternarActivo = async (c) => {
    const ok = await dialog.confirm({
      title: c.activo ? `¿Dar de baja a ${c.nombre}?` : `¿Reactivar a ${c.nombre}?`,
      text: c.activo ? 'Deja de aparecer en la asistencia y no puede marcar. Su historial se conserva.' : 'Vuelve a la lista y puede marcar asistencia.',
      danger: c.activo,
    });
    if (!ok) return;
    try {
      await api.put(`/edificios/${eid}/colaboradores/${c.id}`, { activo: !c.activo });
      await cols.recargar();
    } catch (err) {
      await fallo('No se pudo cambiar')(err);
    }
  };

  const guardarTurno = async (e) => {
    e?.preventDefault?.();
    setOcupado(true);
    try {
      const cuerpo = { nombre: formTurno.nombre, hora_entrada: formTurno.hora_entrada, hora_salida: formTurno.hora_salida, tolerancia_min: Number(formTurno.tolerancia_min) || 0, dias: formTurno.dias };
      if (formTurno.id) await api.put(`/edificios/${eid}/turnos/${formTurno.id}`, cuerpo);
      else await api.post(`/edificios/${eid}/turnos`, cuerpo);
      toast('Turno guardado.', { tipo: 'exito' });
      setFormTurno(null);
      await turnos.recargar();
    } catch (err) {
      await fallo('No se pudo guardar el turno')(err);
    } finally {
      setOcupado(false);
    }
  };

  const nuevaTarea = async () => {
    const texto = await dialog.prompt({ title: 'Nueva tarea del checklist', label: 'Qué debe hacer el turno', placeholder: 'Ej. revisar bombas de agua' });
    if (!texto) return;
    try {
      await api.post(`/edificios/${eid}/checklist`, { texto });
      await checklist.recargar();
    } catch (err) {
      await fallo('No se pudo crear la tarea')(err);
    }
  };

  const quitarTarea = async (t) => {
    if (!(await dialog.confirm({ title: '¿Quitar la tarea?', text: `«${t.texto}» deja de pedirse. Lo ya marcado se conserva.`, danger: true }))) return;
    try {
      await api.del(`/edificios/${eid}/checklist/${t.id}`);
      await checklist.recargar();
    } catch (err) {
      await fallo('No se pudo quitar')(err);
    }
  };

  const columnas = [
    {
      clave: 'nombre', titulo: 'Colaborador', movil: 'titulo',
      render: (c) => (
        <span className="flex items-center gap-2.5">
          {c.foto_url ? (
            <img src={enlace(c.foto_url)} alt="" className="h-8 w-8 shrink-0 rounded-full object-cover" />
          ) : (
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-superficie-2 text-texto-apoyo"><Icono nombre="usuario" tam={16} /></span>
          )}
          <span className="min-w-0">
            <span className="block truncate font-medium text-tinta">{c.nombre}</span>
            {!c.activo && <Insignia estado="inactivo" texto="De baja" />}
          </span>
        </span>
      ),
    },
    { clave: 'cargo', titulo: 'Cargo', movil: 'sub' },
    { clave: 'num_doc', titulo: 'Documento', prioridad: 2 },
    { clave: 'turno', titulo: 'Turno', render: (c) => c.turno || <span className="text-texto-apoyo">Sin turno</span> },
    ...(admin ? [{ clave: 'usuario', titulo: 'Marca con', prioridad: 3, render: (c) => c.usuario || <span className="text-texto-apoyo">—</span> }] : []),
    { clave: 'documentos', titulo: 'Documentos', alinear: 'der', movil: 'valor' },
  ];

  const nuevoColaborador = () => setForm({ nombre: '', tipo_doc: '1', num_doc: '', cargo: '', telefono: '', correo: '', turno_id: '', usuario_id: '', fecha_ingreso: '', foto: null });
  const editar = (c) => setForm({ ...c, turno_id: c.turno_id ? String(c.turno_id) : '', usuario_id: c.usuario_id ? String(c.usuario_id) : '', foto: null });

  const acciones = admin && (
    vista === 'turnos' ? <Boton icono="mas_signo" onClick={() => setFormTurno({ nombre: '', hora_entrada: '08:00', hora_salida: '16:00', tolerancia_min: '10', dias: [1, 2, 3, 4, 5, 6] })}>Nuevo turno</Boton>
      : vista === 'checklist' ? <Boton icono="mas_signo" onClick={nuevaTarea}>Nueva tarea</Boton>
        : <Boton icono="mas_signo" onClick={nuevoColaborador}>Nuevo colaborador</Boton>
  );

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Colaboradores"
        subtitulo="Quién trabaja en el edificio, con su documentación"
        ayuda="Registra al personal, su turno y sus documentos. Los propietarios ven la lista y los documentos marcados como visibles, con enlace firmado de 10 minutos."
        acciones={acciones}
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'colaboradores'} icono="usuario" onClick={() => setVista('colaboradores')} contador={listaCols.length}>Colaboradores</Chip>
          {admin && <Chip activo={vista === 'turnos'} icono="reloj" onClick={() => setVista('turnos')} contador={listaTurnos.length}>Turnos</Chip>}
          {admin && <Chip activo={vista === 'checklist'} icono="check" onClick={() => setVista('checklist')} contador={lista(checklist.datos).length}>Checklist</Chip>}
          {admin && vista === 'colaboradores' && (
            <label className="ml-auto flex items-center gap-2 text-sm text-texto-suave">
              <input type="checkbox" checked={verInactivos} onChange={(e) => setVerInactivos(e.target.checked)} className="h-4 w-4 accent-[var(--color-acento)]" />
              Ver dados de baja
            </label>
          )}
        </div>

        {vista === 'colaboradores' && (
          <div className="overflow-hidden rounded-tarjeta border border-borde bg-superficie">
            <Tabla
              etiqueta="Colaboradores"
              columnas={columnas}
              filas={listaCols}
              cargando={cols.cargando}
              error={cols.error}
              onReintentar={cols.recargar}
              onFila={(c) => setDetalle(c)}
              vacio={<Vacio icono="usuario" titulo="Sin colaboradores" texto={admin ? 'Registra al conserje, al personal de limpieza y de seguridad.' : 'La administración aún no publica al personal.'} />}
            />
          </div>
        )}

        {vista === 'turnos' && (
          <Seccion titulo="Turnos">
            {turnos.error ? <ErrorCarga error={turnos.error} onReintentar={turnos.recargar} /> : !turnos.datos ? <Esqueleto className="h-32 w-full" /> : listaTurnos.length === 0 ? (
              <Vacio icono="reloj" titulo="Sin turnos" texto="Crea el turno de mañana, tarde o noche con su hora de entrada y tolerancia." />
            ) : (
              <ul className="divide-y divide-borde">
                {listaTurnos.map((t) => (
                  <li key={t.id} className="flex flex-wrap items-center gap-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-tinta">{t.nombre}</p>
                      <p className="text-xs text-texto-apoyo">
                        {t.hora_entrada}–{t.hora_salida} · tolerancia {t.tolerancia_min} min · {(t.dias || []).map((d) => DIAS[d - 1]).join(' ')} · {t.colaboradores} colaborador(es)
                      </p>
                    </div>
                    <Boton tamano="sm" variante="fantasma" onClick={() => setFormTurno({ ...t, tolerancia_min: String(t.tolerancia_min) })}>Editar</Boton>
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        )}

        {vista === 'checklist' && (
          <Seccion titulo="Checklist del turno">
            {checklist.error ? <ErrorCarga error={checklist.error} onReintentar={checklist.recargar} /> : !checklist.datos ? <Esqueleto className="h-32 w-full" /> : lista(checklist.datos).length === 0 ? (
              <Vacio icono="check" titulo="Sin tareas" texto="Agrega lo que el personal debe revisar en cada turno; lo marcan desde el teléfono al llegar." />
            ) : (
              <ul className="divide-y divide-borde">
                {lista(checklist.datos).map((t) => (
                  <li key={t.id} className="flex items-center gap-3 py-2.5">
                    <Icono nombre="check" tam={16} className="text-texto-apoyo" />
                    <span className="min-w-0 flex-1 text-sm text-tinta">{t.texto}</span>
                    <span className="text-xs text-texto-apoyo">{t.turno || 'Todos los turnos'}</span>
                    <Boton tamano="sm" variante="fantasma" onClick={() => quitarTarea(t)}>Quitar</Boton>
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        )}
      </Contenido>

      {detalle && (
        <DetalleColaborador
          eid={eid}
          c={detalle}
          admin={admin}
          onCerrar={() => setDetalle(null)}
          onEditar={() => { editar(detalle); setDetalle(null); }}
          onBaja={() => { alternarActivo(detalle); setDetalle(null); }}
          onCambio={cols.recargar}
          dialog={dialog}
          toast={toast}
        />
      )}

      <Modal abierto={!!form} onCerrar={() => setForm(null)} titulo={form?.id ? 'Editar colaborador' : 'Nuevo colaborador'} ancho="max-w-lg"
        pie={<><Boton variante="fantasma" onClick={() => setForm(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarColaborador}>Guardar</Boton></>}>
        {form && (
          <form className="flex flex-col gap-3" onSubmit={guardarColaborador}>
            <Campo etiqueta="Nombre completo" valor={form.nombre} onCambio={(v) => setForm({ ...form, nombre: v })} />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Documento" tipo="select" valor={form.tipo_doc} onCambio={(v) => setForm({ ...form, tipo_doc: v })}
                opciones={[{ valor: '1', etiqueta: 'DNI' }, { valor: '4', etiqueta: 'Carné de extranjería' }, { valor: '7', etiqueta: 'Pasaporte' }]} />
              <Campo etiqueta="Número" valor={form.num_doc} onCambio={(v) => setForm({ ...form, num_doc: v })} />
              <Campo etiqueta="Cargo" valor={form.cargo} onCambio={(v) => setForm({ ...form, cargo: v })} placeholder="Conserje, limpieza…" />
              <Campo etiqueta="Ingreso" tipo="fecha" valor={form.fecha_ingreso} onCambio={(v) => setForm({ ...form, fecha_ingreso: v })} />
              <Campo etiqueta="Teléfono" tipo="telefono" valor={form.telefono} onCambio={(v) => setForm({ ...form, telefono: v })} />
              <Campo etiqueta="Correo" tipo="correo" valor={form.correo} onCambio={(v) => setForm({ ...form, correo: v })} />
              <Campo etiqueta="Turno" tipo="select" valor={form.turno_id} onCambio={(v) => setForm({ ...form, turno_id: v })}
                opciones={[{ valor: '', etiqueta: 'Sin turno' }, ...listaTurnos.filter((t) => t.activo).map((t) => ({ valor: String(t.id), etiqueta: `${t.nombre} (${t.hora_entrada})` }))]} />
              <Campo etiqueta="Cuenta para marcar" tipo="select" valor={form.usuario_id} onCambio={(v) => setForm({ ...form, usuario_id: v })}
                ayuda="Operario o técnico del edificio."
                opciones={[{ valor: '', etiqueta: 'Ninguna' }, ...lista(cuentas.datos).map((u) => ({ valor: String(u.id), etiqueta: `${u.nombre}${u.colaborador_id && u.colaborador_id !== form.id ? ' (en uso)' : ''}`, deshabilitado: !!u.colaborador_id && u.colaborador_id !== form.id }))]} />
            </div>
            <div className="flex flex-col gap-1.5">
              <label htmlFor="col-foto" className="text-sm font-semibold text-tinta">Foto {form.id ? '(reemplaza la actual)' : ''}</label>
              <input id="col-foto" type="file" accept="image/*" onChange={(ev) => setForm({ ...form, foto: ev.target.files?.[0] || null })} className="text-sm" />
            </div>
          </form>
        )}
      </Modal>

      <Modal abierto={!!formTurno} onCerrar={() => setFormTurno(null)} titulo={formTurno?.id ? 'Editar turno' : 'Nuevo turno'} ancho="max-w-md"
        pie={<><Boton variante="fantasma" onClick={() => setFormTurno(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarTurno}>Guardar</Boton></>}>
        {formTurno && (
          <form className="flex flex-col gap-3" onSubmit={guardarTurno}>
            <Campo etiqueta="Nombre" valor={formTurno.nombre} onCambio={(v) => setFormTurno({ ...formTurno, nombre: v })} placeholder="Mañana, tarde, noche…" />
            <div className="grid grid-cols-3 gap-3">
              <Campo etiqueta="Entrada" valor={formTurno.hora_entrada} onCambio={(v) => setFormTurno({ ...formTurno, hora_entrada: v })} placeholder="08:00" />
              <Campo etiqueta="Salida" valor={formTurno.hora_salida} onCambio={(v) => setFormTurno({ ...formTurno, hora_salida: v })} placeholder="16:00" />
              <Campo etiqueta="Tolerancia (min)" tipo="numero" valor={formTurno.tolerancia_min} onCambio={(v) => setFormTurno({ ...formTurno, tolerancia_min: v })} />
            </div>
            <fieldset className="flex flex-col gap-1.5">
              <legend className="text-sm font-semibold text-tinta">Días</legend>
              <div className="flex flex-wrap gap-2">
                {DIAS.map((d, i) => {
                  const n = i + 1;
                  const on = formTurno.dias?.includes(n);
                  return (
                    <Chip key={d} activo={on} onClick={() => setFormTurno({ ...formTurno, dias: on ? formTurno.dias.filter((x) => x !== n) : [...(formTurno.dias || []), n].sort() })}>{d}</Chip>
                  );
                })}
              </div>
            </fieldset>
          </form>
        )}
      </Modal>
    </>
  );
}

/** Ficha del colaborador: datos, documentos con enlace firmado y (administración) subir o quitar documentos. */
function DetalleColaborador({ eid, c, admin, onCerrar, onEditar, onBaja, onCambio, dialog, toast }) {
  const docs = useCarga(() => api.get(`/edificios/${eid}/colaboradores/${c.id}/documentos`), [eid, c.id]);
  const [form, setForm] = useState(null);
  const [ocupado, setOcupado] = useState(false);

  const subirDoc = async (e) => {
    e?.preventDefault?.();
    if (!form.archivo) {
      await dialog.alert({ title: 'Falta el archivo', text: 'Adjunta el documento (PDF o imagen).' });
      return;
    }
    setOcupado(true);
    try {
      const fd = new FormData();
      fd.set('tipo', form.tipo);
      fd.set('titulo', form.titulo);
      fd.set('periodo', form.periodo || '');
      fd.set('visible', form.visible ? '1' : '0');
      fd.set('archivo', form.archivo);
      await subir(`/edificios/${eid}/colaboradores/${c.id}/documentos`, fd);
      toast('Documento subido.', { tipo: 'exito' });
      setForm(null);
      await Promise.all([docs.recargar(), onCambio()]);
    } catch (err) {
      await dialog.alert({ icon: '⚠️', title: 'No se pudo subir', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const quitar = async (d) => {
    if (!(await dialog.confirm({ title: '¿Quitar el documento?', text: `«${d.titulo}» deja de estar disponible.`, danger: true }))) return;
    try {
      await api.del(`/edificios/${eid}/colaboradores/${c.id}/documentos/${d.id}`);
      await Promise.all([docs.recargar(), onCambio()]);
    } catch (err) {
      await dialog.alert({ icon: '⚠️', title: 'No se pudo quitar', text: err.message });
    }
  };

  const pie = admin ? (
    <>
      <Boton variante="fantasma" onClick={onBaja}>{c.activo ? 'Dar de baja' : 'Reactivar'}</Boton>
      <Boton variante="secundario" onClick={onEditar}>Editar</Boton>
      <Boton icono="subir" onClick={() => setForm({ tipo: 'cv', titulo: '', periodo: '', visible: true, archivo: null })}>Subir documento</Boton>
    </>
  ) : null;

  return (
    <Modal abierto onCerrar={onCerrar} titulo={c.nombre} ancho="max-w-lg" lateral pie={pie}>
      <div className="flex flex-col gap-4">
        <div className="flex items-center gap-3">
          {c.foto_url ? <img src={enlace(c.foto_url)} alt={`Foto de ${c.nombre}`} className="h-16 w-16 rounded-full object-cover" /> : (
            <span className="flex h-16 w-16 items-center justify-center rounded-full bg-superficie-2 text-texto-apoyo"><Icono nombre="usuario" tam={28} /></span>
          )}
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-sm">
            <dt className="text-texto-apoyo">Cargo</dt><dd>{c.cargo || '—'}</dd>
            <dt className="text-texto-apoyo">Documento</dt><dd>{c.num_doc || '—'}</dd>
            <dt className="text-texto-apoyo">Turno</dt><dd>{c.turno || 'Sin turno'}</dd>
            <dt className="text-texto-apoyo">Ingreso</dt><dd>{formatearFecha(c.fecha_ingreso)}</dd>
            {admin && c.telefono && <><dt className="text-texto-apoyo">Teléfono</dt><dd>{c.telefono}</dd></>}
          </dl>
        </div>

        <section className="flex flex-col gap-2">
          <h3 className="text-sm font-semibold text-tinta">Documentos</h3>
          {docs.error ? <ErrorCarga error={docs.error} onReintentar={docs.recargar} compacto /> : !docs.datos ? <Esqueleto className="h-20 w-full" /> : lista(docs.datos).length === 0 ? (
            <Vacio icono="documento" titulo="Sin documentos" compacto />
          ) : (
            <ul className="divide-y divide-borde">
              {lista(docs.datos).map((d) => (
                <li key={d.id} className="flex items-center gap-3 py-2">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-tinta">{d.titulo}</p>
                    <p className="truncate text-xs text-texto-apoyo">{[NOMBRE_TIPO[d.tipo], d.periodo, formatearFecha(d.fecha)].filter(Boolean).join(' · ')}</p>
                  </div>
                  {admin && !d.visible_propietarios && <Insignia estado="borrador" texto="Solo administración" />}
                  <a href={enlace(d.archivo_url)} target="_blank" rel="noreferrer" className="text-sm text-acento hover:text-acento-hover">Abrir</a>
                  {admin && <Boton tamano="sm" variante="fantasma" onClick={() => quitar(d)}>Quitar</Boton>}
                </li>
              ))}
            </ul>
          )}
        </section>

        {form && (
          <form className="flex flex-col gap-3 rounded-control border border-borde p-3" onSubmit={subirDoc}>
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Tipo" tipo="select" valor={form.tipo} onCambio={(v) => setForm({ ...form, tipo: v })} opciones={TIPOS_DOC} />
              <Campo etiqueta="Periodo (AAAA-MM)" valor={form.periodo} onCambio={(v) => setForm({ ...form, periodo: v })} placeholder="2026-09" ayuda={form.tipo === 'plame' ? 'Obligatorio en la PLAME.' : undefined} />
            </div>
            <Campo etiqueta="Título" valor={form.titulo} onCambio={(v) => setForm({ ...form, titulo: v })} />
            <input type="file" accept="application/pdf,image/*" aria-label="Archivo" onChange={(ev) => setForm({ ...form, archivo: ev.target.files?.[0] || null })} className="text-sm" />
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.visible} onChange={(e) => setForm({ ...form, visible: e.target.checked })} className="h-4 w-4 accent-[var(--color-acento)]" />
              Visible para los propietarios
            </label>
            <div className="flex justify-end gap-2">
              <Boton variante="fantasma" onClick={() => setForm(null)}>Cancelar</Boton>
              <Boton cargando={ocupado} onClick={subirDoc}>Subir</Boton>
            </div>
          </form>
        )}
      </div>
    </Modal>
  );
}
