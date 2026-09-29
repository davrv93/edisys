import { useEffect, useMemo, useState } from 'react';
import { api, subir } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { diaLima, DIAS_CORTOS, diaSemana, etiquetaDia, mesDePeriodo, sumarDias, sumarMeses, periodoActual } from '../../lib/fechas.js';
import { ruta, useQuery } from '../../lib/nav.jsx';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import { useModoTarea } from '../../layout/Armazon.jsx';
import { CabeceraTarea } from '../../layout/Encabezado.jsx';
import { Boton, Campo, ErrorCarga, Esqueleto, Icono, SubirArchivo, Vacio, useDialog } from '../../ui/index.js';

const DIAS_VISIBLES = 14;

/** 07 · Reservar desde el celular: área → recurso → día → turno → pago → código. */
export default function NuevaReserva() {
  useModoTarea();
  const eid = useEid();
  const s = useSesion();
  const [q] = useQuery();
  const { dialog, dialogEl } = useDialog();
  const esAdmin = s.tiene('reservas.administrar');
  const volver = esAdmin ? ruta('reservas') : ruta('portal');
  const hoy = diaLima(new Date());

  const areas = useCarga(() => api.get(`/edificios/${eid}/areas`), [eid]);
  const listaAreas = useMemo(() => (Array.isArray(areas.datos) ? areas.datos : areas.datos?.datos || []), [areas.datos]);
  const unidadesAdmin = useCarga(() => api.get(`/edificios/${eid}/unidades`, { por_pagina: 500 }), [eid], { activo: esAdmin });

  const recursoInicial = Number(q.get('recurso')) || null;
  const [areaId, setAreaId] = useState(null);
  const [recursoId, setRecursoId] = useState(recursoInicial || 'cualquiera');
  const [dia, setDia] = useState(/^\d{4}-\d{2}-\d{2}$/.test(q.get('dia') || '') ? q.get('dia') : hoy);
  const [franja, setFranja] = useState(null);
  const [medio, setMedio] = useState('yape');
  const [acepta, setAcepta] = useState(false);
  const [unidadId, setUnidadId] = useState(s.unidades?.[0]?.id ?? '');
  const [enviando, setEnviando] = useState(false);
  const [creada, setCreada] = useState(null);
  const [moroso, setMoroso] = useState(null);

  useEffect(() => {
    if (!listaAreas.length || areaId) return;
    const conRecurso = recursoInicial && listaAreas.find((a) => a.recursos?.some((r) => r.id === recursoInicial));
    setAreaId((conRecurso || listaAreas[0]).id);
  }, [listaAreas, areaId, recursoInicial]);

  const area = listaAreas.find((a) => a.id === areaId);
  const recursos = area?.recursos || [];
  const idsConsulta = recursoId === 'cualquiera' ? recursos.map((r) => r.id) : [recursoId];

  const disp = useCarga(
    async () => {
      const res = await Promise.all(idsConsulta.map((id) => api.get(`/edificios/${eid}/disponibilidad`, { recurso: id, desde: dia, hasta: dia })));
      // Forma del API: fecha, hora_inicio, hora_fin; la del mock: dia, desde, hasta.
      return res.flatMap((r) => r?.franjas || []).map((f) => ({ ...f, dia: f.dia || f.fecha, desde: f.desde || f.hora_inicio, hasta: f.hasta || f.hora_fin }));
    },
    [eid, dia, areaId, recursoId],
    { activo: !!area && idsConsulta.length > 0 },
  );

  // Franjas del día agrupadas por horario; con «cualquiera libre», libre si algún recurso lo está.
  const franjas = useMemo(() => {
    const m = new Map();
    for (const f of disp.datos || []) {
      const clave = `${f.desde || f.inicio}|${f.hasta || f.fin}`;
      const previo = m.get(clave);
      if (!previo || (previo.estado !== 'libre' && f.estado === 'libre')) m.set(clave, f);
    }
    return [...m.values()].sort((a, b) => String(a.inicio).localeCompare(String(b.inicio)));
  }, [disp.datos]);

  useEffect(() => setFranja(null), [dia, areaId, recursoId]);

  const nombreRecurso = recursoId === 'cualquiera' ? area?.nombre : recursos.find((r) => r.id === recursoId)?.nombre;
  const tarifa = area?.tarifa_cts || 0;
  // El modo de cobro lo fija el edificio (modo_cobro); si el API no lo dice, la persona elige.
  const modoEdificio = areas.datos?.modo_cobro || null;
  const medioEfectivo = modoEdificio === 'pago_inmediato' ? 'yape' : modoEdificio === 'cargo_recibo' ? 'recibo' : medio;
  const mesRecibo = mesDePeriodo(sumarMeses(periodoActual(), 1));
  const unidadNombre = esAdmin ? unidadesAdmin.datos?.datos?.find((u) => String(u.id) === String(unidadId))?.codigo : s.unidades?.[0]?.codigo;

  const reservar = async () => {
    if (!franja || !acepta) return;
    setEnviando(true);
    try {
      const r = await api.post(`/edificios/${eid}/reservas`, {
        recurso_id: franja.recurso_id,
        unidad_id: unidadId || undefined,
        inicio: franja.inicio,
        fin: franja.fin,
        acepta_normas: true,
        medio: tarifa ? medioEfectivo : 'recibo',
      });
      setCreada({ ...r, franja, recurso: nombreRecurso });
    } catch (err) {
      if (err.status === 409) {
        await dialog.alert({ title: 'Alguien acaba de reservar esa franja', text: 'Elige otro turno; ya actualizamos la disponibilidad.' });
        disp.recargar();
      } else if (err.status === 403 && err.codigo === 'MOROSO') {
        setMoroso(err.detalle);
      } else {
        await dialog.alert({ title: 'No se pudo reservar', text: err.message });
      }
    } finally {
      setEnviando(false);
    }
  };

  const subtitulo = `${unidadNombre ? `Dpto ${unidadNombre} · ` : ''}${s.edificio.nombre}`;
  const dias = Array.from({ length: DIAS_VISIBLES }, (_, i) => sumarDias(hoy, i));

  const nMeses = moroso ? (Array.isArray(moroso.meses) ? moroso.meses.length : moroso.meses) : 0;
  if (moroso) {
    return (
      <div className="flex min-h-screen flex-col bg-fondo">
        <CabeceraTarea titulo="No puedes reservar por ahora" subtitulo={subtitulo} volver={volver} />
        <div className="flex flex-1 flex-col gap-4 p-4">
          <div className="flex flex-col gap-2 rounded-xl border border-alerta-borde bg-alerta-suave p-5 text-alerta-texto">
            <span className="text-xs font-bold">RECIBO VENCIDO</span>
            <span className="font-titulo text-3xl font-semibold text-alerta">{formatearSoles(moroso.monto_cts)}</span>
            <span className="text-base">
              {nMeses ? `Tienes ${nMeses} ${nMeses === 1 ? 'mes' : 'meses'} pendiente${nMeses === 1 ? '' : 's'}. ` : ''}Las unidades con deuda vencida no pueden reservar áreas comunes. Cuando se valide tu pago, podrás reservar.
            </span>
          </div>
          <Boton href={ruta('recibos')} tamano="lg" bloque>
            Ver mi deuda
          </Boton>
        </div>
      </div>
    );
  }

  if (creada) return <Creada creada={creada} eid={eid} volver={volver} subtitulo={subtitulo} dialog={dialog} dialogEl={dialogEl} />;

  return (
    <div className="flex min-h-screen flex-col bg-fondo">
      {dialogEl}
      <CabeceraTarea titulo={`Reservar ${nombreRecurso || ''}`.trim()} subtitulo={subtitulo} volver={volver} />
      {areas.error ? (
        <ErrorCarga error={areas.error} onReintentar={areas.recargar} />
      ) : !areas.datos ? (
        <div className="flex flex-col gap-3 p-4">
          <Esqueleto className="h-12 w-full" />
          <Esqueleto className="h-24 w-full" />
        </div>
      ) : listaAreas.length === 0 ? (
        <Vacio titulo="Tu edificio aún no tiene áreas reservables" texto="Cuando la administración configure parrillas o salas, podrás reservarlas aquí." icono="calendario" />
      ) : (
        <>
          <div className="mx-auto flex w-full max-w-xl flex-1 flex-col gap-5 p-4 pb-48">
            {esAdmin && (
              <Campo
                etiqueta="A nombre de la unidad"
                tipo="select"
                valor={unidadId}
                onCambio={setUnidadId}
                opciones={[{ valor: '', etiqueta: 'Elige la unidad' }, ...(unidadesAdmin.datos?.datos || []).map((u) => ({ valor: u.id, etiqueta: `Dpto ${u.codigo} · ${u.propietario}` }))]}
              />
            )}
            <section className="flex flex-col gap-2">
              <span className="text-sm font-semibold">Área</span>
              <div className="flex flex-wrap gap-2">
                {listaAreas.map((a) => (
                  <Chip key={a.id} activo={a.id === areaId} onClick={() => (setAreaId(a.id), setRecursoId('cualquiera'))}>
                    {a.nombre}
                  </Chip>
                ))}
              </div>
              {recursos.length > 1 && (
                <div className="flex flex-wrap gap-2">
                  <Chip activo={recursoId === 'cualquiera'} onClick={() => setRecursoId('cualquiera')} suave>
                    Cualquiera libre
                  </Chip>
                  {recursos.map((r) => (
                    <Chip key={r.id} activo={recursoId === r.id} onClick={() => setRecursoId(r.id)} suave>
                      {r.nombre}
                    </Chip>
                  ))}
                </div>
              )}
            </section>

            <section className="flex flex-col gap-2">
              <span className="text-sm font-semibold">Día</span>
              <div className="-mx-4 flex gap-2 overflow-x-auto px-4 pb-1">
                {dias.map((d) => (
                  <button
                    key={d}
                    type="button"
                    aria-pressed={d === dia}
                    onClick={() => setDia(d)}
                    className={`flex h-16 w-13 shrink-0 flex-col items-center justify-center gap-0.5 rounded-lg border ${d === dia ? 'border-acento bg-acento text-white' : 'border-borde-fuerte bg-superficie'}`}
                  >
                    <span className={`text-xs ${d === dia ? '' : 'text-texto-apoyo'}`}>{DIAS_CORTOS[diaSemana(d)]}</span>
                    <b className="text-base">{Number(d.slice(8))}</b>
                  </button>
                ))}
              </div>
            </section>

            <section className="flex flex-col gap-2">
              <span className="text-sm font-semibold">Turno</span>
              {disp.error ? (
                <ErrorCarga error={disp.error} onReintentar={disp.recargar} compacto />
              ) : disp.cargando && !disp.datos ? (
                <Esqueleto className="h-12 w-full" />
              ) : franjas.length === 0 ? (
                <p className="text-base text-texto-suave">No hay turnos este día.</p>
              ) : (
                <>
                  <div className="grid grid-cols-3 gap-2">
                    {franjas.map((f) => {
                      const libre = f.estado === 'libre';
                      const sel = franja && franja.inicio === f.inicio;
                      const etiqueta = `${(f.desde || '').slice(0, 2)}–${(f.hasta || '').slice(0, 2)}`;
                      return (
                        <button
                          key={f.inicio}
                          type="button"
                          disabled={!libre}
                          aria-pressed={sel}
                          onClick={() => setFranja(f)}
                          className={`flex h-13 flex-col items-center justify-center rounded-lg border text-sm font-semibold ${
                            sel ? 'border-acento bg-acento-suave text-acento-hover ring-2 ring-acento' : libre ? 'border-borde-fuerte bg-superficie' : 'cursor-not-allowed border-borde bg-superficie-2 text-texto-tenue'
                          }`}
                        >
                          {etiqueta}
                          {!libre && <span className="text-[11px] font-normal">{f.estado === 'retenida' ? 'retenida' : f.estado === 'fuera_de_horario' ? 'no disponible' : 'ocupada'}</span>}
                        </button>
                      );
                    })}
                  </div>
                  {!franjas.some((f) => f.estado === 'libre') && <p className="text-sm text-aviso">Este día no quedan turnos libres. Prueba otro día.</p>}
                </>
              )}
              {area && (
                <span className="text-xs text-texto-apoyo">
                  {[area.aforo ? `Aforo ${area.aforo} personas` : null, area.incluye, area.normas].filter(Boolean).join(' · ')}
                </span>
              )}
            </section>

            {tarifa > 0 && modoEdificio && (
              <p className="rounded-xl border border-borde bg-superficie p-4 text-sm text-texto-suave">
                {modoEdificio === 'pago_inmediato' ? 'Se paga al reservar con Yape: guardamos tu turno 15 minutos mientras subes el voucher.' : `Se carga a tu recibo de ${mesRecibo}.`}
              </p>
            )}
            {tarifa > 0 && !modoEdificio && (
              <section className="flex flex-col gap-2">
                <span className="text-sm font-semibold">Forma de pago</span>
                <OpcionPago valor="yape" medio={medio} setMedio={setMedio} titulo="Yape" detalle="QR y código de operación · guardamos tu turno 15 min" />
                <OpcionPago valor="recibo" medio={medio} setMedio={setMedio} titulo={`Cargo al recibo de ${mesRecibo}`} detalle="Se suma a tu próximo recibo" />
                <div className="flex min-h-[56px] items-center gap-3 rounded-xl border border-borde bg-superficie-2 px-4 text-sm text-texto-apoyo">
                  <Icono nombre="candado" tam={18} /> Tarjeta: disponible más adelante
                </div>
              </section>
            )}

            <label className="flex min-h-[44px] items-start gap-3 text-sm text-egreso">
              <input type="checkbox" checked={acepta} onChange={(e) => setAcepta(e.target.checked)} className="mt-0.5 h-5 w-5 shrink-0 accent-[var(--color-acento)]" />
              Acepto las normas de uso de {area?.nombre?.toLowerCase() || 'el área'} y la penalidad por daños.
            </label>
          </div>

          <footer className="fixed inset-x-0 bottom-0 z-20 border-t border-borde bg-superficie p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
            <div className="mx-auto flex max-w-xl flex-col gap-2">
              <div className="flex items-baseline justify-between">
                <span className="text-sm text-texto-suave">{franja ? `${etiquetaDia(dia)} · ${franja.desde}–${franja.hasta}` : 'Elige un turno'}</span>
                <span className="font-titulo text-2xl font-semibold tabular-nums">{tarifa ? formatearSoles(tarifa) : 'Sin costo'}</span>
              </div>
              <Boton tamano="lg" bloque disabled={!franja || !acepta || (esAdmin && !unidadId)} cargando={enviando} onClick={reservar}>
                {!tarifa ? 'Reservar' : medioEfectivo === 'yape' ? 'Pagar con Yape' : `Reservar con cargo al recibo`}
              </Boton>
              {tarifa > 0 && medioEfectivo === 'yape' && <span className="text-center text-xs text-texto-apoyo">Guardamos tu turno 15 minutos mientras pagas.</span>}
            </div>
          </footer>
        </>
      )}
    </div>
  );
}

function Chip({ activo, onClick, children, suave = false }) {
  const on = suave ? 'border-acento bg-acento-suave text-acento-hover' : 'border-acento bg-acento text-white';
  return (
    <button type="button" aria-pressed={activo} onClick={onClick} className={`h-11 rounded-full border px-4 text-sm font-semibold ${activo ? on : 'border-borde-fuerte bg-superficie text-tinta'}`}>
      {children}
    </button>
  );
}

function OpcionPago({ valor, medio, setMedio, titulo, detalle }) {
  return (
    <label className={`flex min-h-[56px] cursor-pointer items-center gap-3 rounded-xl border bg-superficie px-4 py-2 text-sm ${medio === valor ? 'border-acento ring-1 ring-acento' : 'border-borde'}`}>
      <input type="radio" name="medio" checked={medio === valor} onChange={() => setMedio(valor)} className="h-5 w-5 accent-[var(--color-acento)]" />
      <span className="flex flex-col">
        <b>{titulo}</b>
        <span className="text-xs text-texto-apoyo">{detalle}</span>
      </span>
    </label>
  );
}

/** Reserva creada: si es pago inmediato, sube el voucher mientras corre la retención de 15 min. */
function Creada({ creada, eid, volver, subtitulo, dialog, dialogEl }) {
  const s = useSesion();
  const [codigo, setCodigo] = useState('');
  const [voucher, setVoucher] = useState(null);
  const [progreso, setProgreso] = useState(null);
  const [enviado, setEnviado] = useState(false);
  const [ahora, setAhora] = useState(Date.now());
  const pendiente = creada.estado === 'pendiente_pago' && !enviado;
  useEffect(() => {
    if (!pendiente) return undefined;
    const t = setInterval(() => setAhora(Date.now()), 1000);
    return () => clearInterval(t);
  }, [pendiente]);
  const resta = creada.vence_retencion ? Math.max(0, new Date(creada.vence_retencion).getTime() - ahora) : null;
  const mmss = resta != null ? `${String(Math.floor(resta / 60000)).padStart(2, '0')}:${String(Math.floor((resta % 60000) / 1000)).padStart(2, '0')}` : null;

  const enviar = async () => {
    const form = new FormData();
    form.set('codigo_operacion', codigo.trim());
    form.set('voucher', voucher);
    setProgreso(0);
    try {
      await subir(`/edificios/${eid}/reservas/${creada.id}/pago`, form, { onProgreso: setProgreso });
      setEnviado(true);
    } catch (err) {
      await dialog.alert({ title: err.status === 409 ? 'Ese código ya se usó' : 'No se pudo enviar el voucher', text: err.message });
    } finally {
      setProgreso(null);
    }
  };

  return (
    <div className="flex min-h-screen flex-col bg-fondo">
      {dialogEl}
      <CabeceraTarea titulo={`Reserva ${creada.codigo}`} subtitulo={subtitulo} volver={volver} />
      <div className="mx-auto flex w-full max-w-xl flex-col gap-4 p-4">
        <div className={`flex flex-col gap-1 rounded-xl border p-5 ${pendiente ? 'border-aviso-borde bg-aviso-suave text-aviso-texto' : 'border-acento-borde bg-acento-suave text-acento-hover'}`}>
          <span className="text-xs font-bold">{pendiente ? 'RETENIDA MIENTRAS PAGAS' : enviado ? 'PAGO ENVIADO, EN REVISIÓN' : 'RESERVA CONFIRMADA'}</span>
          <span className="font-titulo text-3xl font-semibold">{creada.codigo}</span>
          <span className="text-base">
            {creada.recurso} · {etiquetaDia(creada.franja.dia || String(creada.franja.inicio).slice(0, 10))} · {creada.franja.desde}–{creada.franja.hasta}
          </span>
          <span className="text-base font-semibold tabular-nums">{formatearSoles(creada.total_cts)}</span>
          {pendiente && mmss && <span className="text-sm">Tiempo para pagar: {resta > 0 ? mmss : 'se liberó la franja porque no llegó el pago'}</span>}
        </div>
        {pendiente && resta !== 0 && (
          <div className="flex flex-col gap-4 rounded-xl border border-borde bg-superficie p-4">
            <div className="flex flex-col gap-1 text-sm">
              <span className="text-xs font-bold text-texto-apoyo">YAPE DEL EDIFICIO</span>
              <span className="font-titulo text-2xl font-semibold text-acento">{s.edificio.yape?.numero || 'Consulta el número con la administración'}</span>
              <span className="text-texto-suave">Monto exacto {formatearSoles(creada.total_cts)} · concepto «{creada.codigo}»</span>
            </div>
            <Campo etiqueta="Código de operación" valor={codigo} onCambio={setCodigo} inputMode="numeric" placeholder="Ej. 000184" />
            <SubirArchivo etiqueta="Captura del voucher" archivo={voucher} onArchivo={setVoucher} />
            <Boton tamano="lg" bloque disabled={!codigo.trim() || !voucher} cargando={progreso != null} onClick={enviar}>
              Enviar voucher
            </Boton>
          </div>
        )}
        <Boton variante="secundario" href={volver} bloque>
          Listo
        </Boton>
      </div>
    </div>
  );
}
