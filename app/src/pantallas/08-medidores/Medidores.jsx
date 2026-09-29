import { useState } from 'react';
import { api, subir } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearNumero, formatearPct } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { alertaConsumo, calcularConsumo, cargoAgua, parsearLectura, siguientePendiente } from '../../lib/medidor.js';
import { ruta, useQuery } from '../../lib/nav.jsx';
import { useEid, useSesion, Guarda } from '../../layout/Sesion.jsx';
import { useModoTarea } from '../../layout/Armazon.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import { Boton, ErrorCarga, Esqueleto, Icono, Insignia, SubirFoto, useDialog, useToast } from '../../ui/index.js';
import Reparto from './Reparto.jsx';
import { nombreUnidad } from '../../lib/unidad.js';

const num = (v) => (v === null || v === undefined || v === '' ? null : Number(v));

/** 08 · Medidores. Operario: ronda de lecturas con foto obligatoria. Administración: además, el reparto (?vista=reparto). */
export default function Medidores() {
  const s = useSesion();
  const [q] = useQuery();
  if (q.get('vista') === 'reparto' && s.tiene('lecturas.aprobar_reparto')) return <Reparto />;
  return <Ronda />;
}

function Ronda() {
  const eid = useEid();
  const { edificio } = useSesion();
  const [periodo] = usePeriodo();
  const [q, setQuery] = useQuery();
  const { datos, error, recargar, setDatos } = useCarga(() => api.get(`/edificios/${eid}/lecturas`, { periodo, tipo: 'agua' }), [eid, periodo]);
  const medidores = (datos?.medidores || [])
    .map((m) => ({ ...m, unidad: nombreUnidad(m.unidad), medidor: m.medidor || m.serie, lectura_anterior: num(m.lectura_anterior), lectura_actual: num(m.lectura_actual), consumo: num(m.consumo), media_3m: num(m.media_3m) }))
    .sort((a, b) => (a.orden_ronda ?? 0) - (b.orden_ronda ?? 0));
  const avance = datos?.avance || { leidas: medidores.filter((m) => m.estado !== 'pendiente').length, total: medidores.length };
  const pct = avance.total ? (avance.leidas / avance.total) * 100 : 0;
  const idSel = q.get('medidor');
  const actual = medidores.find((m) => String(m.medidor_id) === idSel);

  const cabecera = (
    <header className="flex flex-col gap-3 bg-tinta p-4 text-white lg:rounded-xl">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          {actual && (
            <button type="button" onClick={() => setQuery({ medidor: null })} className="-ml-2 flex h-11 w-11 items-center justify-center rounded-lg hover:bg-superficie-oscura" aria-label="Volver a la lista">
              <Icono nombre="volver" />
            </button>
          )}
          <div className="flex min-w-0 flex-col">
            <span className="text-lg font-semibold">Lectura de agua</span>
            <span className="text-xs text-texto-tenue">
              {edificio.nombre} · {datos?.corte ? `corte ${formatearFecha(datos.corte)}` : `periodo ${periodo}`}
            </span>
          </div>
        </div>
        <span className="shrink-0 rounded-full bg-superficie-oscura px-3 py-1 text-sm font-semibold tabular-nums">
          {avance.leidas} de {avance.total}
        </span>
      </div>
      <div className="h-1.5 rounded-full bg-superficie-oscura-2" role="progressbar" aria-valuenow={Math.round(pct)} aria-valuemin={0} aria-valuemax={100} aria-label="Avance de la ronda">
        <div className="h-1.5 rounded-full bg-acento-oscuro transition-all" style={{ width: `${pct}%` }} />
      </div>
    </header>
  );

  if (error) return <ErrorCarga error={error} onReintentar={recargar} />;
  if (!datos)
    return (
      <div className="flex flex-col gap-3 p-4">
        <Esqueleto className="h-20 w-full" />
        <Esqueleto className="h-64 w-full" />
      </div>
    );

  if (actual) {
    return (
      <Captura
        key={actual.medidor_id}
        eid={eid}
        periodo={periodo}
        datos={datos}
        medidor={actual}
        medidores={medidores}
        cabecera={cabecera}
        onGuardado={(res, valor) => {
          setDatos((d) => ({
            ...d,
            avance: { ...avance, leidas: avance.leidas + (actual.estado === 'pendiente' ? 1 : 0) },
            medidores: d.medidores.map((m) => (m.medidor_id === actual.medidor_id ? { ...m, lectura_actual: valor, lectura_id: res?.id ?? m.lectura_id, consumo: res?.consumo ?? calcularConsumo(m.lectura_anterior, valor), estado: res?.alerta ? 'alerta' : 'leida', alerta: res?.alerta || null } : m)),
          }));
          const sig = siguientePendiente(medidores, actual.medidor_id);
          setQuery({ medidor: sig ? sig.medidor_id : null });
        }}
      />
    );
  }

  const terminado = avance.total > 0 && avance.leidas >= avance.total;
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4 p-4 lg:p-8">
      {cabecera}
      <Guarda permiso="lecturas.aprobar_reparto">
        <div className="flex flex-wrap gap-2">
          <Boton variante="secundario" icono="grafico" href={ruta('medidores', { vista: 'reparto', periodo })}>
            Ver reparto del recibo general
          </Boton>
        </div>
      </Guarda>
      {terminado && (
        <div className="flex flex-col items-center gap-2 rounded-xl border border-acento-borde bg-acento-suave p-6 text-center text-acento-hover">
          <Icono nombre="check" tam={32} />
          <span className="font-titulo text-2xl font-semibold">¡Listo! {avance.leidas} de {avance.total} leídas</span>
          <span className="text-base">La ronda de este periodo está completa.</span>
        </div>
      )}
      <ul className="flex flex-col gap-2" aria-label="Unidades en el orden de la ronda">
        {medidores.map((m) => (
          <li key={m.medidor_id}>
            <button type="button" onClick={() => setQuery({ medidor: m.medidor_id })} className="flex min-h-[64px] w-full items-center justify-between gap-3 rounded-xl border border-borde bg-superficie px-4 py-3 text-left hover:border-acento">
              <span className="flex flex-col">
                <span className="text-base font-semibold">{m.unidad}</span>
                <span className="text-sm text-texto-apoyo">
                  {m.medidor || `Medidor ${m.medidor_id}`} · anterior {formatearNumero(m.lectura_anterior, 3)}
                  {m.consumo != null ? ` · ${formatearNumero(m.consumo, 3)} m³` : ''}
                </span>
              </span>
              <Insignia estado={m.estado === 'alerta' ? m.alerta || 'alerta_lectura' : m.estado} />
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

function Captura({ eid, periodo, datos, medidor: m, medidores, cabecera, onGuardado }) {
  useModoTarea();
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();
  const [foto, setFoto] = useState(null);
  const [texto, setTexto] = useState(m.lectura_actual != null ? String(m.lectura_actual).replace('.', ',') : '');
  const [observacion, setObservacion] = useState('');
  const [progreso, setProgreso] = useState(null);
  const [fallo, setFallo] = useState(null);
  const valor = parsearLectura(texto);
  const consumo = calcularConsumo(m.lectura_anterior, valor);
  const alerta = alertaConsumo(consumo, m.media_3m);
  const tarifa = datos.tarifa_cts;
  const sig = siguientePendiente(medidores, m.medidor_id);
  const puedeGuardar = !!foto && valor != null && progreso == null;

  const guardar = async () => {
    if (!puedeGuardar) return;
    let motivo = '';
    if (alerta === 'NEGATIVO') {
      const r = await dialog.prompt({ title: 'La lectura es menor que la anterior', text: `Consumo de ${formatearNumero(consumo, 3)} m³. Si cambiaron el medidor o dio la vuelta, explícalo.`, label: 'Motivo', required: true, okText: 'Guardar con motivo' });
      if (r === null) return;
      motivo = r;
    } else if (alerta === 'PICO') {
      const ok = await dialog.confirm({ title: '¿Confirmas este consumo?', text: `${formatearNumero(consumo, 3)} m³ es más del doble de lo habitual (${formatearNumero(m.media_3m, 1)} m³). Puede ser una fuga. Revisa la foto antes de guardar.`, okText: 'Sí, está bien' });
      if (!ok) return;
    }
    const form = new FormData();
    form.set('periodo', periodo);
    form.set('valor', String(valor));
    form.set('foto', foto.archivo);
    if (foto.tomadaEn) form.set('tomada_en', foto.tomadaEn);
    if (motivo) form.set('motivo', motivo);
    if (observacion) form.set('observacion', observacion);
    setFallo(null);
    setProgreso(0);
    try {
      const res = await subir(`/edificios/${eid}/medidores/${m.medidor_id}/lecturas`, form, { onProgreso: setProgreso });
      toast(`${m.unidad}: lectura guardada.`, { tipo: 'exito', duracion: 2000 });
      onGuardado(res, valor);
    } catch (err) {
      if (err.status === 0 || err.status >= 500) setFallo('No se pudo subir. Revisa la señal y toca «Reintentar»: la lectura aún no está guardada.');
      else await dialog.alert({ title: err.codigo === 'YA_LEIDO' ? 'Este medidor ya tiene lectura' : 'No se pudo guardar', text: err.codigo === 'YA_LEIDO' ? 'Para corregirla, pídeselo a la administración.' : err.message });
    } finally {
      setProgreso(null);
    }
  };

  const pedirObservacion = async () => {
    const r = await dialog.prompt({ title: 'Observación', label: 'Qué viste en el medidor', type: 'textarea', defaultValue: observacion, placeholder: 'Ej. vidrio empañado, tapa rota…' });
    if (r !== null) setObservacion(r);
  };

  return (
    <div className="flex min-h-screen flex-col bg-fondo">
      {dialogEl}
      <div className="mx-auto flex w-full max-w-xl flex-1 flex-col gap-4 pb-40 lg:p-8 lg:pb-8">
        {cabecera}
        <div className="flex flex-col gap-4 px-4 lg:px-0">
          <div className="flex items-baseline justify-between">
            <span className="font-titulo text-3xl font-semibold">{m.unidad}</span>
            <span className="text-sm text-texto-apoyo">{m.medidor || `Medidor ${m.medidor_id}`}</span>
          </div>
          <SubirFoto foto={foto} onFoto={setFoto} obligatoria etiqueta="Tomar foto del medidor" progreso={progreso} />
          <details className="text-sm text-texto-apoyo">
            <summary className="cursor-pointer">¿La cámara no abre?</summary>
            <p className="mt-2">Android: Ajustes › Apps › tu navegador › Permisos › Cámara › Permitir. iPhone: Ajustes › Safari › Cámara › Permitir. Luego vuelve a tocar «Tomar foto».</p>
          </details>

          <div className="flex items-end gap-3">
            <div className="flex flex-1 flex-col gap-1.5">
              <label htmlFor="lectura" className="text-sm font-semibold">
                Lectura actual (m³)
              </label>
              <input
                id="lectura"
                type="text"
                inputMode="decimal"
                autoComplete="off"
                value={texto}
                onChange={(e) => setTexto(e.target.value)}
                aria-invalid={texto && valor == null ? 'true' : undefined}
                className={`h-16 w-full rounded-lg border-2 bg-superficie px-4 font-titulo text-3xl tabular-nums focus:outline-none focus:ring-2 focus:ring-acento ${alerta ? 'border-aviso bg-aviso-suave' : texto && valor == null ? 'border-alerta' : 'border-borde-fuerte'}`}
              />
            </div>
            <div className="flex w-[120px] flex-col gap-1.5">
              <span className="text-sm text-texto-apoyo">Anterior</span>
              <span className="flex h-16 items-center rounded-lg bg-superficie-2 px-3 text-2xl tabular-nums text-texto-suave">{formatearNumero(m.lectura_anterior, m.lectura_anterior % 1 ? 3 : 0)}</span>
            </div>
          </div>
          {texto && valor == null && (
            <p className="text-sm text-alerta" role="alert">
              Escribe solo números (hasta 3 decimales).
            </p>
          )}
          {consumo != null && (
            <div className={`flex items-center justify-between rounded-lg border p-3 text-sm ${alerta ? 'border-aviso-borde bg-aviso-suave text-aviso-texto' : 'border-borde bg-superficie'}`} role={alerta ? 'status' : undefined}>
              <span>
                Consumo <b className="tabular-nums">{formatearNumero(consumo, consumo % 1 ? 3 : 0)} m³</b>
                {tarifa ? ` × ${formatearSoles(tarifa)}` : ''}
                {alerta === 'PICO' && ' · pico: posible fuga'}
                {alerta === 'NEGATIVO' && ' · negativo: pide motivo'}
              </span>
              {tarifa && consumo >= 0 ? <b className="tabular-nums">{formatearSoles(cargoAgua(consumo, tarifa))}</b> : null}
            </div>
          )}
          {observacion && (
            <p className="rounded-lg bg-superficie-2 p-3 text-sm">
              <b>Observación:</b> {observacion}
            </p>
          )}
          {datos.recibo_general_cts && datos.tarifa_cts ? <CajaReparto datos={datos} medidor={m} /> : null}
          {fallo && (
            <div className="flex flex-col gap-2 rounded-lg border border-alerta-borde bg-alerta-suave p-3 text-sm text-alerta-texto" role="alert">
              {fallo}
              <Boton variante="secundario" onClick={guardar} className="self-start">
                Reintentar
              </Boton>
            </div>
          )}
        </div>
      </div>
      <footer className="fixed inset-x-0 bottom-0 z-20 border-t border-borde bg-superficie p-4 pb-[max(1rem,env(safe-area-inset-bottom))] lg:static lg:mx-auto lg:w-full lg:max-w-xl lg:border-0 lg:bg-transparent lg:px-8">
        <div className="flex gap-2">
          <Boton variante="secundario" tamano="lg" onClick={pedirObservacion} className="shrink-0 px-4">
            Observación
          </Boton>
          <Boton tamano="lg" bloque disabled={!puedeGuardar} cargando={progreso != null} onClick={guardar} className="min-w-0 flex-1">
            {sig ? `Guardar · sigue ${sig.unidad}` : 'Guardar lectura'}
          </Boton>
        </div>
        {!foto && <p className="mt-2 text-center text-xs text-texto-apoyo">Sin foto no se puede guardar.</p>}
      </footer>
    </div>
  );
}

function CajaReparto({ datos, medidor }) {
  const deptos = datos.total_unidades_cts ?? datos.recibo_general_cts - 20000;
  const comun = datos.recibo_general_cts - deptos;
  const pct = medidor.participacion_pct;
  return (
    <section className="flex flex-col gap-2 rounded-xl border border-acento-borde bg-acento-suave p-4 text-sm text-acento-hover">
      <span className="text-xs font-bold">REPARTO AL CERRAR EL PERIODO</span>
      <div className="flex justify-between">
        <span>Recibo general del edificio</span>
        <b className="tabular-nums">{formatearSoles(datos.recibo_general_cts)}</b>
      </div>
      {datos.total_unidades_cts != null && (
        <>
          <div className="flex justify-between">
            <span>Suma de departamentos</span>
            <b className="tabular-nums">{formatearSoles(deptos)}</b>
          </div>
          <div className="flex justify-between">
            <span>Áreas comunes (riego, limpieza)</span>
            <b className="tabular-nums text-acento">{formatearSoles(comun)}</b>
          </div>
        </>
      )}
      {pct != null && datos.total_unidades_cts != null && (
        <span className="text-xs">
          Se reparte por participación: {medidor.unidad} ({formatearPct(pct, 2)}) paga {formatearSoles(Math.round((comun * pct) / 100))}.
        </span>
      )}
    </section>
  );
}
