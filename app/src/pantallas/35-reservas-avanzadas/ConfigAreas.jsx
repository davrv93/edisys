import { useEffect, useMemo, useState } from 'react';
import { api, lista, subir } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { ruta, useQuery } from '../../lib/nav.jsx';
import { useEid } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, BotonIcono, Campo, Chip, ErrorCarga, Esqueleto, Vacio, useDialog, useToast } from '../../ui/index.js';

// 1 = lunes … 7 = domingo (las claves de area.horarios).
const DIAS = [
  ['1', 'Lunes'],
  ['2', 'Martes'],
  ['3', 'Miércoles'],
  ['4', 'Jueves'],
  ['5', 'Viernes'],
  ['6', 'Sábado'],
  ['7', 'Domingo'],
];

const NUEVA = {
  nombre: '',
  descripcion: '',
  incluye: '',
  normas: '',
  aforo: '',
  activo: true,
  tarifa_cts: 0,
  garantia_cts: 0,
  limpieza_cts: 0,
  franjas: [{ inicio: '12:00', fin: '17:00' }],
  horarios: {},
  anticipacion_min_dias: 0,
  anticipacion_max_dias: 30,
  separacion_dias: 0,
  cupo_mensual_unidad: 0,
  checkin_tolerancia_min: 30,
  permite_parciales: false,
  deuda_tolerada_cts: 0,
  permite_financiados: false,
  recursos: [],
  nuevosRecursos: '',
};

/** Del área del API al formulario. */
function aFormulario(a) {
  return {
    ...NUEVA,
    ...a,
    aforo: a.aforo ?? '',
    cupo_mensual_unidad: a.cupo_mensual_unidad ?? 0,
    franjas: a.franjas || [],
    horarios: a.horarios || {},
    nuevosRecursos: '',
  };
}

const entero = (v) => {
  const n = parseInt(String(v ?? '').trim(), 10);
  return Number.isFinite(n) && n >= 0 ? n : 0;
};

/** Del formulario al cuerpo de POST/PUT /areas (H2 y H3 viajan en el mismo JSON). */
export function cuerpoArea(f) {
  const limpiaFranjas = (fs) =>
    (fs || []).map((x) => {
      const o = { inicio: String(x.inicio || '').trim(), fin: String(x.fin || '').trim() };
      if (x.tarifa_cts != null && x.tarifa_cts !== '') o.tarifa_cts = x.tarifa_cts;
      return o;
    });
  const horarios = {};
  for (const [k, fs] of Object.entries(f.horarios || {})) horarios[k] = limpiaFranjas(fs);
  return {
    nombre: f.nombre.trim(),
    descripcion: f.descripcion,
    incluye: f.incluye,
    normas: f.normas,
    aforo: f.aforo === '' ? null : entero(f.aforo),
    activo: f.activo,
    tarifa_cts: f.tarifa_cts || 0,
    garantia_cts: f.garantia_cts || 0,
    limpieza_cts: f.limpieza_cts || 0,
    franjas: limpiaFranjas(f.franjas),
    horarios,
    anticipacion_min_dias: entero(f.anticipacion_min_dias),
    anticipacion_max_dias: entero(f.anticipacion_max_dias) || 30,
    separacion_dias: entero(f.separacion_dias),
    cupo_mensual_unidad: entero(f.cupo_mensual_unidad),
    checkin_tolerancia_min: entero(f.checkin_tolerancia_min),
    permite_parciales: !!f.permite_parciales,
    deuda_tolerada_cts: f.deuda_tolerada_cts || 0,
    permite_financiados: !!f.permite_financiados,
    recursos: String(f.nuevosRecursos || '')
      .split(',')
      .map((x) => x.trim())
      .filter(Boolean),
  };
}

/**
 * Bloques H2 y H3 · configuración completa de las áreas reservables: datos, fotos y reglamento,
 * tarifa + garantía + limpieza, horario por día de la semana, restricciones y excepciones de morosidad.
 * La pantalla de reservas (07) y el calendario la usan tal cual.
 */
export default function ConfigAreas() {
  const eid = useEid();
  const [q, setQuery] = useQuery();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const areas = useCarga(() => api.get(`/edificios/${eid}/areas`), [eid]);
  const listaAreas = useMemo(() => lista(areas.datos), [areas.datos]);
  const sel = q.get('area') || '';
  const area = listaAreas.find((a) => String(a.id) === sel);
  const [form, setForm] = useState(null);
  const [guardando, setGuardando] = useState(false);
  const [subiendo, setSubiendo] = useState(false);

  useEffect(() => {
    if (sel === 'nueva') setForm({ ...NUEVA });
    else if (area) setForm(aFormulario(area));
    else if (!sel && listaAreas[0]) setQuery({ area: listaAreas[0].id }, { reemplazar: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sel, area?.id, listaAreas.length]);

  const cambiar = (campo) => (v) => setForm((f) => ({ ...f, [campo]: v }));

  const guardar = async () => {
    if (!form.nombre.trim()) {
      await dialog.alert({ title: 'Falta el nombre', text: 'Ponle un nombre al área (p. ej. «Parrillas»).' });
      return;
    }
    setGuardando(true);
    try {
      const cuerpo = cuerpoArea(form);
      const r = sel === 'nueva' ? await api.post(`/edificios/${eid}/areas`, cuerpo) : await api.put(`/edificios/${eid}/areas/${area.id}`, cuerpo);
      toast(`«${r.nombre}» guardada.`, { tipo: 'exito' });
      await areas.recargar();
      setQuery({ area: r.id }, { reemplazar: true });
    } catch (err) {
      await dialog.alert({ title: 'No se pudo guardar', text: err.message });
    } finally {
      setGuardando(false);
    }
  };

  const subirArchivos = async (tipo, archivos) => {
    if (!archivos?.length || !area) return;
    const fd = new FormData();
    for (const a of archivos) fd.append(tipo === 'fotos' ? 'fotos' : 'reglamento', a);
    setSubiendo(true);
    try {
      await subir(`/edificios/${eid}/areas/${area.id}/${tipo}`, fd);
      toast(tipo === 'fotos' ? 'Fotos subidas.' : 'Reglamento subido.', { tipo: 'exito' });
      await areas.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo subir', text: err.message });
    } finally {
      setSubiendo(false);
    }
  };

  const borrarFoto = async (foto) => {
    const ok = await dialog.confirm({ title: '¿Quitar esta foto?', text: 'Deja de mostrarse al reservar.', danger: true, okText: 'Quitar' });
    if (!ok) return;
    try {
      await api.del(`/edificios/${eid}/areas/${area.id}/fotos/${foto.id}`);
      await areas.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo quitar', text: err.message });
    }
  };

  const acciones = (
    <>
      <Boton variante="secundario" icono="calendario" href={ruta('reservas')}>
        Calendario
      </Boton>
      <Boton icono="mas_signo" onClick={() => setQuery({ area: 'nueva' })}>
        Nueva área
      </Boton>
    </>
  );

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Configuración de áreas comunes"
        subtitulo="Horarios, tarifas, reglamento, fotos y restricciones"
        ayuda="Todo lo que el vecino ve al reservar y lo que el sistema valida: anticipación, separación entre reservas, cupos, garantía, limpieza y quién puede reservar con deuda."
        acciones={acciones}
      />
      <Contenido>
        {areas.error ? (
          <ErrorCarga error={areas.error} onReintentar={areas.recargar} />
        ) : !areas.datos ? (
          <Esqueleto className="h-64 w-full" />
        ) : (
          <>
            <div className="carrusel flex gap-2 pb-1">
              {listaAreas.map((a) => (
                <Chip key={a.id} activo={String(a.id) === sel} onClick={() => setQuery({ area: a.id })} tono={a.activo ? 'acento' : 'neutro'}>
                  {a.nombre}
                </Chip>
              ))}
              {sel === 'nueva' && <Chip activo>Nueva área</Chip>}
            </div>
            {!form ? (
              <Vacio icono="calendario" titulo="Configura tu primera área" texto="Parrillas, SUM, piscina… con su tarifa, horario, aforo y normas." />
            ) : (
              <FormArea form={form} cambiar={cambiar} setForm={setForm} area={sel === 'nueva' ? null : area} subirArchivos={subirArchivos} subiendo={subiendo} borrarFoto={borrarFoto} guardar={guardar} guardando={guardando} />
            )}
          </>
        )}
      </Contenido>
    </>
  );
}

function FormArea({ form, cambiar, setForm, area, subirArchivos, subiendo, borrarFoto, guardar, guardando }) {
  const total = (form.tarifa_cts || 0) + (form.garantia_cts || 0) + (form.limpieza_cts || 0);
  const modoDia = (k) => (!(k in (form.horarios || {})) ? 'general' : form.horarios[k].length === 0 ? 'cerrado' : 'propio');
  const ponerModo = (k, modo) =>
    setForm((f) => {
      const hs = { ...(f.horarios || {}) };
      if (modo === 'general') delete hs[k];
      else if (modo === 'cerrado') hs[k] = [];
      else hs[k] = (f.franjas || []).map((x) => ({ ...x }));
      return { ...f, horarios: hs };
    });
  const ponerFranjasDia = (k) => (fs) => setForm((f) => ({ ...f, horarios: { ...f.horarios, [k]: fs } }));

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <Seccion titulo="Datos del área">
        <div className="flex flex-col gap-3">
          <Campo etiqueta="Nombre" valor={form.nombre} onCambio={cambiar('nombre')} />
          <Campo etiqueta="Descripción" tipo="textarea" valor={form.descripcion} onCambio={cambiar('descripcion')} ayuda="Lo que el vecino lee antes de reservar." />
          <div className="grid grid-cols-2 gap-3">
            <Campo etiqueta="Aforo (personas)" tipo="numero" valor={form.aforo} onCambio={cambiar('aforo')} />
            <Campo
              etiqueta="Estado"
              tipo="select"
              valor={form.activo ? '1' : '0'}
              onCambio={(v) => cambiar('activo')(v === '1')}
              opciones={[
                { valor: '1', etiqueta: 'Disponible' },
                { valor: '0', etiqueta: 'Cerrada' },
              ]}
            />
          </div>
          <Campo etiqueta="Incluye" valor={form.incluye} onCambio={cambiar('incluye')} placeholder="Parrilla, mesa para 10, lavadero…" />
          <Campo etiqueta="Normas de uso" tipo="textarea" valor={form.normas} onCambio={cambiar('normas')} ayuda="Se aceptan al reservar." />
          <div className="flex flex-col gap-1">
            <span className="text-sm font-semibold text-tinta">Recursos</span>
            <span className="text-sm text-texto-suave">{(form.recursos || []).map((r) => r.nombre).join(' · ') || 'Se crea uno con el nombre del área.'}</span>
            <Campo etiqueta="Añadir recursos" ocultarEtiqueta valor={form.nuevosRecursos} onCambio={cambiar('nuevosRecursos')} placeholder="Parrilla 3, Parrilla 4 (separados por coma)" />
          </div>
        </div>
      </Seccion>

      <Seccion titulo="Tarifas">
        <div className="flex flex-col gap-3">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Campo etiqueta="Tarifa" tipo="dinero" valor={form.tarifa_cts} onCambio={(v) => cambiar('tarifa_cts')(v || 0)} />
            <Campo etiqueta="Garantía" tipo="dinero" valor={form.garantia_cts} onCambio={(v) => cambiar('garantia_cts')(v || 0)} />
            <Campo etiqueta="Limpieza" tipo="dinero" valor={form.limpieza_cts} onCambio={(v) => cambiar('limpieza_cts')(v || 0)} />
          </div>
          <p className="text-sm text-texto-suave">
            Total por reserva: <b className="tabular-nums text-tinta">{formatearSoles(total)}</b>. La garantía y la limpieza se cobran junto con la tarifa (al recibo o al pagar). Una franja puede tener tarifa propia en el horario por día.
          </p>
        </div>
      </Seccion>

      <Seccion titulo="Horario">
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <span className="text-sm font-semibold text-tinta">Franjas generales</span>
            <EditorFranjas franjas={form.franjas} onCambio={cambiar('franjas')} />
          </div>
          <div className="flex flex-col gap-2">
            <span className="text-sm font-semibold text-tinta">Por día de la semana</span>
            {DIAS.map(([k, nombre]) => (
              <div key={k} className="flex flex-col gap-2 border-t border-borde pt-2">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="w-24 text-sm">{nombre}</span>
                  {[
                    ['general', 'Generales'],
                    ['propio', 'Propio'],
                    ['cerrado', 'Cerrado'],
                  ].map(([m, t]) => (
                    <Chip key={m} activo={modoDia(k) === m} onClick={() => ponerModo(k, m)}>
                      {t}
                    </Chip>
                  ))}
                </div>
                {modoDia(k) === 'propio' && <EditorFranjas franjas={form.horarios[k]} onCambio={ponerFranjasDia(k)} conTarifa />}
              </div>
            ))}
          </div>
        </div>
      </Seccion>

      <Seccion titulo="Restricciones">
        <div className="flex flex-col gap-3">
          <div className="grid grid-cols-2 gap-3">
            <Campo etiqueta="Anticipación mínima (días)" tipo="numero" valor={form.anticipacion_min_dias} onCambio={cambiar('anticipacion_min_dias')} />
            <Campo etiqueta="Anticipación máxima (días)" tipo="numero" valor={form.anticipacion_max_dias} onCambio={cambiar('anticipacion_max_dias')} />
            <Campo etiqueta="Separación entre reservas (días)" tipo="numero" valor={form.separacion_dias} onCambio={cambiar('separacion_dias')} ayuda="De la misma unidad en esta área." />
            <Campo etiqueta="Cupo por unidad al mes" tipo="numero" valor={form.cupo_mensual_unidad} onCambio={cambiar('cupo_mensual_unidad')} ayuda="0 = sin tope." />
            <Campo etiqueta="Ingreso antes del inicio (min)" tipo="numero" valor={form.checkin_tolerancia_min} onCambio={cambiar('checkin_tolerancia_min')} ayuda="Tolerancia del check-in." />
          </div>
          <div className="flex flex-col gap-2 rounded-tarjeta border border-borde p-3">
            <span className="text-sm font-semibold text-tinta">Unidades con deuda vencida</span>
            <span className="text-xs text-texto-apoyo">Por defecto no reservan ni entran. Estas excepciones valen para reservar y para el check-in.</span>
            <Interruptor activo={form.permite_parciales} onCambio={cambiar('permite_parciales')} texto="Permitir con deuda parcial (hasta un monto)" />
            {form.permite_parciales && <Campo etiqueta="Deuda tolerada" tipo="dinero" valor={form.deuda_tolerada_cts} onCambio={(v) => cambiar('deuda_tolerada_cts')(v || 0)} />}
            <Interruptor activo={form.permite_financiados} onCambio={cambiar('permite_financiados')} texto="Permitir con la deuda financiada (acuerdo de pago activo)" />
          </div>
        </div>
      </Seccion>

      {area && (
        <Seccion titulo="Fotos y reglamento" className="xl:col-span-2">
          <div className="flex flex-col gap-4">
            <div className="flex flex-wrap gap-3">
              {(area.fotos || []).map((f) => (
                <figure key={f.id} className="relative h-28 w-40 overflow-hidden rounded-control border border-borde">
                  <img src={f.url} alt={`Foto de ${area.nombre}`} className="h-full w-full object-cover" loading="lazy" />
                  <span className="absolute right-1 top-1">
                    <BotonIcono variante="secundario" icono="cerrar" etiqueta="Quitar foto" onClick={() => borrarFoto(f)} />
                  </span>
                </figure>
              ))}
              <label className="flex h-28 w-40 cursor-pointer flex-col items-center justify-center gap-1 rounded-control border border-dashed border-borde-fuerte text-sm text-texto-suave hover:bg-fondo">
                {subiendo ? 'Subiendo…' : '+ Añadir fotos'}
                <input type="file" accept="image/*" multiple className="sr-only" onChange={(e) => subirArchivos('fotos', [...(e.target.files || [])])} disabled={subiendo} />
              </label>
            </div>
            <div className="flex flex-wrap items-center gap-3 text-sm">
              <span className="font-semibold text-tinta">Reglamento (PDF):</span>
              {area.reglamento_url ? (
                <a href={area.reglamento_url} target="_blank" rel="noreferrer" className="text-acento hover:text-acento-hover">
                  Ver reglamento
                </a>
              ) : (
                <span className="text-texto-apoyo">Sin reglamento</span>
              )}
              <label className="cursor-pointer text-acento hover:text-acento-hover">
                {area.reglamento_url ? 'Reemplazar' : 'Subir'}
                <input type="file" accept="application/pdf,image/*" className="sr-only" onChange={(e) => subirArchivos('reglamento', [...(e.target.files || [])])} disabled={subiendo} />
              </label>
            </div>
          </div>
        </Seccion>
      )}
      {!area && <p className="text-sm text-texto-apoyo xl:col-span-2">Guarda el área para añadirle fotos y reglamento.</p>}

      <div className="flex justify-end xl:col-span-2">
        <Boton cargando={guardando} onClick={guardar}>
          Guardar área
        </Boton>
      </div>
    </div>
  );
}

function EditorFranjas({ franjas = [], onCambio, conTarifa = false }) {
  const poner = (i, campo, v) => onCambio(franjas.map((f, j) => (j === i ? { ...f, [campo]: v } : f)));
  return (
    <div className="flex flex-col gap-2">
      {franjas.map((f, i) => (
        <div key={i} className="flex flex-wrap items-end gap-2">
          <Campo className="w-24" etiqueta="Desde" valor={f.inicio} onCambio={(v) => poner(i, 'inicio', v)} placeholder="HH:MM" />
          <Campo className="w-24" etiqueta="Hasta" valor={f.fin} onCambio={(v) => poner(i, 'fin', v)} placeholder="HH:MM" />
          {conTarifa && <Campo className="w-32" etiqueta="Tarifa propia" tipo="dinero" valor={f.tarifa_cts ?? null} onCambio={(v) => poner(i, 'tarifa_cts', v)} />}
          <BotonIcono variante="secundario" icono="cerrar" etiqueta="Quitar franja" onClick={() => onCambio(franjas.filter((_, j) => j !== i))} />
        </div>
      ))}
      <div>
        <Boton variante="fantasma" tamano="sm" icono="mas_signo" onClick={() => onCambio([...franjas, { inicio: '', fin: '' }])}>
          Añadir franja
        </Boton>
      </div>
    </div>
  );
}

function Interruptor({ activo, onCambio, texto }) {
  return (
    <label className="flex min-h-[44px] cursor-pointer items-center gap-3 text-sm text-tinta">
      <input type="checkbox" checked={!!activo} onChange={(e) => onCambio(e.target.checked)} className="h-5 w-5 shrink-0 accent-[var(--color-acento)]" />
      {texto}
    </label>
  );
}
