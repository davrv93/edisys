import { useState } from 'react';
import { api, lista, subir } from '../../lib/api.js';
import { PREFIJO } from '../../lib/base.js';
import { useCarga } from '../../lib/useCarga.js';
import { diaLima, sumarDias } from '../../lib/fechas.js';
import { formatearPct } from '../../lib/dinero.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, FranjaKPI, Icono, Insignia, Modal, SubirFoto, Tabla, Vacio, useDialog, useToast } from '../../ui/index.js';

/**
 * Bloque F2 · Asistencia con foto y turnos.
 * El personal (asistencia.marcar) marca entrada/salida desde el teléfono con foto obligatoria y su checklist.
 * La administración y la junta (asistencia.ver) ven las marcas del día con sus fotos y el panel de puntualidad.
 */

const enlace = (u) => (u ? PREFIJO + u : null);

/** Pastilla de puntualidad de una marca. */
function Puntualidad({ a }) {
  if (a.puntual == null) return <Insignia estado="pendiente" tono="neutro" texto="Sin turno" icono={false} />;
  return a.puntual ? <Insignia estado="al_dia" texto="Puntual" /> : <Insignia estado="vencido" tono="aviso" texto={`${a.minutos_tarde} min tarde`} />;
}

export default function Asistencia() {
  const s = useSesion();
  const marca = s.tiene?.('asistencia.marcar');
  const panel = s.tiene?.('asistencia.ver');
  return (
    <>
      <Encabezado
        titulo="Asistencia"
        subtitulo={panel ? 'Marcas del personal con foto y puntualidad' : 'Marca tu entrada y tu salida'}
        ayuda="La entrada y la salida se marcan con foto tomada en el momento; la hora la pone el servidor. La puntualidad se calcula contra el turno y su tolerancia."
      />
      <Contenido>
        {marca && <MiMarca />}
        {panel && <Panel />}
      </Contenido>
    </>
  );
}

/** Lo que ve el colaborador en el teléfono: su turno, la foto, el botón y el checklist. */
function MiMarca() {
  const eid = useEid();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const hoy = useCarga(() => api.get(`/edificios/${eid}/asistencia/hoy`), [eid]);
  const [foto, setFoto] = useState(null);
  const [progreso, setProgreso] = useState(null);
  const d = hoy.datos;

  if (hoy.error) return <ErrorCarga error={hoy.error} onReintentar={hoy.recargar} />;
  if (!d) return <Esqueleto className="h-64 w-full" />;
  if (!d.colaborador) {
    return <Vacio icono="usuario" titulo="Tu cuenta no está enlazada" texto="Pide a la administración que te registre como colaborador para poder marcar asistencia." />;
  }
  const a = d.asistencia;
  const tipo = !a ? 'entrada' : !a.salida ? 'salida' : null;
  const c = d.colaborador;

  const marcar = async () => {
    if (!foto) {
      await dialog.alert({ title: 'Falta la foto', text: 'Toma la foto para marcar tu ' + tipo + '.' });
      return;
    }
    const fd = new FormData();
    fd.set('tipo', tipo);
    fd.set('foto', foto.archivo, 'marca.jpg');
    try {
      setProgreso(0);
      const r = await subir(`/edificios/${eid}/asistencia/marcar`, fd, { onProgreso: setProgreso });
      setFoto(null);
      toast(tipo === 'entrada' ? `Entrada marcada a las ${r.entrada}.` : `Salida marcada a las ${r.salida}.`, { tipo: 'exito' });
      await hoy.recargar();
    } catch (err) {
      await dialog.alert({ icon: '⚠️', title: 'No se pudo marcar', text: err.message });
    } finally {
      setProgreso(null);
    }
  };

  const tarea = async (t) => {
    try {
      await api.post(`/edificios/${eid}/asistencia/${a.id}/checklist`, { item_id: t.id, hecho: !t.hecho });
      await hoy.recargar();
    } catch (err) {
      await dialog.alert({ icon: '⚠️', title: 'No se pudo marcar la tarea', text: err.message });
    }
  };

  return (
    <Seccion titulo={`Hola, ${c.nombre.split(' ')[0]}`} extra={c.turno && <span className="text-sm text-texto-apoyo">Turno {c.turno} · {c.hora_entrada}–{c.hora_salida}</span>}>
      {dialogEl}
      <div className="mx-auto flex max-w-xl flex-col gap-4">
        {a && (
          <div className="flex flex-wrap items-center gap-3 rounded-control border border-borde bg-superficie-2 p-3 text-sm">
            <Icono nombre="reloj" tam={16} />
            <span>Entrada <b className="tabular-nums">{a.entrada}</b></span>
            {a.salida && <span>· Salida <b className="tabular-nums">{a.salida}</b></span>}
            <Puntualidad a={a} />
          </div>
        )}
        {tipo ? (
          <>
            <SubirFoto foto={foto} onFoto={setFoto} obligatoria etiqueta={tipo === 'entrada' ? 'Foto de entrada' : 'Foto de salida'} progreso={progreso} />
            <Boton bloque tamano="lg" icono="camara" onClick={marcar} cargando={progreso != null} disabled={!foto}>
              {tipo === 'entrada' ? 'Marcar entrada' : 'Marcar salida'}
            </Boton>
          </>
        ) : (
          <p className="text-sm text-texto-suave">Ya marcaste entrada y salida hoy. ¡Buen descanso!</p>
        )}
        {a && lista(d.checklist).length > 0 && (
          <fieldset className="flex flex-col gap-1">
            <legend className="mb-1 text-sm font-semibold text-tinta">Checklist del turno</legend>
            {lista(d.checklist).map((t) => (
              <label key={t.id} className="flex min-h-[44px] items-center gap-3 text-sm">
                <input type="checkbox" checked={!!t.hecho} onChange={() => tarea(t)} className="h-5 w-5 shrink-0 accent-[var(--color-acento)]" />
                <span className={t.hecho ? 'text-texto-apoyo line-through' : 'text-tinta'}>{t.texto}</span>
              </label>
            ))}
          </fieldset>
        )}
      </div>
    </Seccion>
  );
}

/** Panel de la administración: marcas del día con fotos, ausentes y puntualidad por rango. */
function Panel() {
  const eid = useEid();
  const s = useSesion();
  const admin = s.tiene?.('personal.administrar');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const hoy = diaLima(new Date());
  const [vista, setVista] = useState('dia');
  const [fecha, setFecha] = useState(hoy);
  const [desde, setDesde] = useState(hoy.slice(0, 8) + '01');
  const [hasta, setHasta] = useState(hoy);
  const [foto, setFoto] = useState(null); // foto ampliada
  const [manual, setManual] = useState(null);
  const [ocupado, setOcupado] = useState(false);

  const dia = useCarga(() => api.get(`/edificios/${eid}/asistencia`, { fecha }), [eid, fecha], { activo: vista === 'dia' });
  const punt = useCarga(() => api.get(`/edificios/${eid}/asistencia/puntualidad`, { desde, hasta }), [eid, desde, hasta], { activo: vista === 'puntualidad' });
  const cols = useCarga(() => api.get(`/edificios/${eid}/colaboradores`), [eid], { activo: !!admin });

  const guardarManual = async (e) => {
    e?.preventDefault?.();
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/asistencia/manual`, { colaborador_id: Number(manual.colaborador_id), tipo: manual.tipo, en: `${manual.fecha}T${manual.hora}`, nota: manual.nota });
      toast('Marca registrada.', { tipo: 'exito' });
      setManual(null);
      await dia.recargar();
    } catch (err) {
      await dialog.alert({ icon: '⚠️', title: 'No se pudo registrar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const columnasDia = [
    { clave: 'colaborador', titulo: 'Colaborador', movil: 'titulo', render: (a) => <span className="font-medium text-tinta">{a.colaborador}</span> },
    { clave: 'turno', titulo: 'Turno', movil: 'sub', render: (a) => (a.turno ? `${a.turno} (${a.hora_turno})` : '—') },
    { clave: 'entrada', titulo: 'Entrada', render: (a) => <HoraConFoto hora={a.entrada} url={a.entrada_foto_url} onVer={setFoto} /> },
    { clave: 'salida', titulo: 'Salida', render: (a) => (a.salida ? <HoraConFoto hora={a.salida} url={a.salida_foto_url} onVer={setFoto} /> : <span className="text-texto-apoyo">Sin salida</span>) },
    { clave: 'puntual', titulo: 'Puntualidad', movil: 'valor2', render: (a) => <Puntualidad a={a} /> },
    { clave: 'checklist_hechos', titulo: 'Checklist', alinear: 'der', prioridad: 2 },
    { clave: 'nota', titulo: 'Nota', prioridad: 3, render: (a) => (a.manual ? <span title={a.nota}>Manual · {a.nota}</span> : '') },
  ];

  const columnasPunt = [
    { clave: 'nombre', titulo: 'Colaborador', movil: 'titulo', render: (f) => <span className="font-medium text-tinta">{f.nombre}</span> },
    { clave: 'turno', titulo: 'Turno', movil: 'sub', render: (f) => f.turno || 'Sin turno' },
    { clave: 'asistencias', titulo: 'Marcas', alinear: 'der' },
    { clave: 'tardanzas', titulo: 'Tardanzas', alinear: 'der' },
    { clave: 'minutos_tarde', titulo: 'Min. tarde', alinear: 'der', prioridad: 2 },
    { clave: 'faltas', titulo: 'Faltas', alinear: 'der', render: (f) => <span className={f.faltas > 0 ? 'font-semibold text-alerta' : ''}>{f.faltas}</span> },
    { clave: 'sin_salida', titulo: 'Sin salida', alinear: 'der', prioridad: 3 },
    { clave: 'pct_puntualidad', titulo: 'Puntualidad', alinear: 'der', movil: 'valor', render: (f) => (f.pct_puntualidad == null ? '—' : formatearPct(f.pct_puntualidad)) },
  ];

  const r = punt.datos?.resumen;
  return (
    <>
      {dialogEl}
      <div className="flex flex-wrap items-center gap-2">
        <Chip activo={vista === 'dia'} icono="calendario" onClick={() => setVista('dia')}>Día</Chip>
        <Chip activo={vista === 'puntualidad'} icono="grafico" onClick={() => setVista('puntualidad')}>Puntualidad</Chip>
        {admin && (
          <Boton className="ml-auto" variante="secundario" icono="mas_signo" onClick={() => setManual({ colaborador_id: '', tipo: 'entrada', fecha: hoy, hora: '08:00', nota: '' })}>
            Registro manual
          </Boton>
        )}
      </div>

      {vista === 'dia' && (
        <>
          <div className="flex flex-wrap items-end gap-2">
            <Boton variante="secundario" icono="izq" onClick={() => setFecha(sumarDias(fecha, -1))} aria-label="Día anterior" />
            <Campo etiqueta="Fecha" ocultarEtiqueta tipo="fecha" valor={fecha} onCambio={(v) => v && setFecha(v)} className="w-44" />
            <Boton variante="secundario" icono="der" onClick={() => setFecha(sumarDias(fecha, 1))} disabled={fecha >= hoy} aria-label="Día siguiente" />
          </div>
          <div className="overflow-hidden rounded-tarjeta border border-borde bg-superficie">
            <Tabla etiqueta="Marcas del día" columnas={columnasDia} filas={lista(dia.datos)} cargando={dia.cargando} error={dia.error} onReintentar={dia.recargar}
              vacio={<Vacio icono="reloj" titulo="Nadie marcó ese día" compacto />} />
          </div>
          {lista(dia.datos, 'ausentes').length > 0 && (
            <Seccion titulo="Con turno y sin marca">
              <ul className="flex flex-wrap gap-2">
                {lista(dia.datos, 'ausentes').map((x) => (
                  <li key={x.id}><Insignia estado="vencido" tono="alerta" texto={`${x.nombre} · ${x.turno} ${x.hora_entrada}`} /></li>
                ))}
              </ul>
            </Seccion>
          )}
        </>
      )}

      {vista === 'puntualidad' && (
        <>
          <div className="flex flex-wrap items-end gap-3">
            <Campo etiqueta="Desde" tipo="fecha" valor={desde} onCambio={(v) => v && setDesde(v)} className="w-44" />
            <Campo etiqueta="Hasta" tipo="fecha" valor={hasta} onCambio={(v) => v && setHasta(v)} className="w-44" />
          </div>
          <FranjaKPI
            cargando={!punt.datos && !punt.error}
            principal={{ titulo: 'Puntualidad', valor: r?.pct_puntualidad == null ? '—' : formatearPct(r.pct_puntualidad), tono: r?.pct_puntualidad != null && r.pct_puntualidad < 90 ? 'alerta' : 'acento', icono: 'reloj' }}
            items={[
              { titulo: 'Marcas', valor: String(r?.asistencias ?? 0) },
              { titulo: 'Tardanzas', valor: String(r?.tardanzas ?? 0), tono: r?.tardanzas ? 'aviso' : undefined },
              { titulo: 'Faltas', valor: String(r?.faltas ?? 0), tono: r?.faltas ? 'alerta' : undefined, nota: 'Contadas hasta ayer' },
            ]}
          />
          <div className="overflow-hidden rounded-tarjeta border border-borde bg-superficie">
            <Tabla etiqueta="Puntualidad por colaborador" columnas={columnasPunt} filas={lista(punt.datos)} cargando={punt.cargando} error={punt.error} onReintentar={punt.recargar}
              vacio={<Vacio icono="usuario" titulo="Sin colaboradores activos" compacto />} />
          </div>
        </>
      )}

      <Modal abierto={!!foto} onCerrar={() => setFoto(null)} titulo="Foto de la marca" ancho="max-w-lg">
        {foto && <img src={foto} alt="Foto de la marca" className="w-full rounded-control" />}
      </Modal>

      <Modal abierto={!!manual} onCerrar={() => setManual(null)} titulo="Registro manual" ancho="max-w-md"
        pie={<><Boton variante="fantasma" onClick={() => setManual(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarManual}>Registrar</Boton></>}>
        {manual && (
          <form className="flex flex-col gap-3" onSubmit={guardarManual}>
            <Campo etiqueta="Colaborador" tipo="select" valor={manual.colaborador_id} onCambio={(v) => setManual({ ...manual, colaborador_id: v })}
              opciones={[{ valor: '', etiqueta: 'Elige…' }, ...lista(cols.datos).map((c) => ({ valor: String(c.id), etiqueta: c.nombre }))]} />
            <div className="grid grid-cols-3 gap-3">
              <Campo etiqueta="Tipo" tipo="select" valor={manual.tipo} onCambio={(v) => setManual({ ...manual, tipo: v })} opciones={[{ valor: 'entrada', etiqueta: 'Entrada' }, { valor: 'salida', etiqueta: 'Salida' }]} />
              <Campo etiqueta="Fecha" tipo="fecha" valor={manual.fecha} onCambio={(v) => setManual({ ...manual, fecha: v })} />
              <Campo etiqueta="Hora" valor={manual.hora} onCambio={(v) => setManual({ ...manual, hora: v })} placeholder="08:00" />
            </div>
            <Campo etiqueta="Motivo" tipo="textarea" valor={manual.nota} onCambio={(v) => setManual({ ...manual, nota: v })} ayuda="Queda en el registro como marca manual." />
          </form>
        )}
      </Modal>
    </>
  );
}

function HoraConFoto({ hora, url, onVer }) {
  return (
    <span className="inline-flex items-center gap-2">
      <span className="tabular-nums">{hora}</span>
      {url && (
        <button type="button" onClick={() => onVer(enlace(url))} className="text-acento hover:text-acento-hover" aria-label={`Ver foto de las ${hora}`}>
          <Icono nombre="camara" tam={16} />
        </button>
      )}
    </span>
  );
}
