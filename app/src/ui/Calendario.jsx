import { DIAS_CORTOS, diaSemana, etiquetaDia } from '../lib/fechas.js';
import Icono from './Icono.jsx';

const TONO_EVENTO = {
  confirmada: 'bg-acento-suave text-acento-hover border-acento-borde',
  pendiente_pago: 'bg-aviso-suave text-aviso-texto border-aviso-borde border-dashed',
  bloqueo: 'bg-superficie-2 text-egreso border-borde',
};
const ICONO_EVENTO = { confirmada: 'calendario', pendiente_pago: 'temporizador', bloqueo: 'candado' };
/** Color de área (paleta de series, en orden). `serie` va de 1 a 5 y se repite. */
const colorSerie = (n) => (n ? `var(--color-serie-${((n - 1) % 5) + 1})` : undefined);

/**
 * Calendario de semana (recursos × días) en escritorio y de DÍA con lista de franjas en móvil.
 * Recibe la ocupación del servidor; no calcula disponibilidad.
 * eventos: [{ id, recurso_id, dia: 'AAAA-MM-DD', desde, hasta, estado, titulo, sub }]
 * recursos: [{ id, nombre, detalle?, serie? }]  (v2: `serie` 1–5 pinta el filo del área con la paleta de series)
 */
export default function Calendario({ dias, recursos, eventos, hoy, onEvento, onCelda, diaMovil, onDiaMovil }) {
  const porCelda = new Map();
  for (const ev of eventos) {
    const k = `${ev.recurso_id}|${ev.dia}`;
    if (!porCelda.has(k)) porCelda.set(k, []);
    porCelda.get(k).push(ev);
  }
  const dm = diaMovil || dias[0];
  const serieDe = new Map(recursos.map((r) => [r.id, r.serie]));

  return (
    <>
      {/* Escritorio: semana */}
      <div className="hidden md:block" role="grid" aria-label="Calendario semanal de reservas">
        <div className="grid grid-cols-[140px_repeat(7,minmax(0,1fr))] border-b border-borde bg-fondo text-xs text-texto-apoyo" role="row">
          <div role="columnheader" />
          {dias.map((d) => (
            <div key={d} role="columnheader" aria-current={d === hoy ? 'date' : undefined} className={`relative flex flex-col items-center gap-0.5 py-2 ${d === hoy ? 'text-curso-texto' : ''}`}>
              <span>{d === hoy ? 'Hoy' : DIAS_CORTOS[diaSemana(d)]}</span>
              <span className={`text-base font-semibold ${d === hoy ? 'text-curso' : 'text-tinta'}`}>{Number(d.slice(8))}</span>
              {d === hoy && <span className="absolute inset-x-2 bottom-0 h-0.5 rounded-chip bg-curso" aria-hidden="true" />}
            </div>
          ))}
        </div>
        {recursos.map((r) => (
          <div key={r.id} role="row" className="grid min-h-[64px] grid-cols-[140px_repeat(7,minmax(0,1fr))] border-b border-superficie-2 text-xs">
            <div role="rowheader" className="flex flex-col justify-center gap-0.5 px-4">
              <span className="flex items-center gap-2 text-sm font-semibold">
                {r.serie && <span className="h-2.5 w-2.5 shrink-0 rounded-chip" style={{ background: colorSerie(r.serie) }} aria-hidden="true" />}
                {r.nombre}
              </span>
              {r.detalle && <span className="text-texto-apoyo">{r.detalle}</span>}
            </div>
            {dias.map((d) => {
              const evs = porCelda.get(`${r.id}|${d}`) || [];
              return (
                <div key={d} role="gridcell" className={`flex flex-col gap-1 border-l border-superficie-2 p-1 ${d === hoy ? 'bg-curso-suave/40' : ''}`}>
                  {evs.map((ev) => (
                    <button
                      key={ev.id}
                      type="button"
                      onClick={() => onEvento?.(ev)}
                      className={`flex flex-col gap-0.5 rounded-control border border-l-[3px] px-1.5 py-1 text-left leading-tight transition-[transform,box-shadow] duration-rapida animate-aparecer hover:-translate-y-px hover:shadow-flotante ${TONO_EVENTO[ev.estado] || TONO_EVENTO.bloqueo}`}
                      style={{ borderLeftColor: ev.estado === 'bloqueo' ? undefined : colorSerie(serieDe.get(ev.recurso_id)) }}
                    >
                      <span className="flex items-center gap-1">
                        <Icono nombre={ICONO_EVENTO[ev.estado] || 'calendario'} tam={12} />
                        <b className="truncate">{ev.titulo}</b>
                      </span>
                      <span className="tabular-nums">{ev.desde}–{ev.hasta}</span>
                      {ev.sub && <span className="truncate opacity-90">{ev.sub}</span>}
                    </button>
                  ))}
                  {onCelda && evs.length === 0 && (
                    <button
                      type="button"
                      onClick={() => onCelda(r, d)}
                      className="group flex min-h-[40px] flex-1 items-center justify-center rounded-control text-transparent transition-colors duration-rapida hover:bg-acento-suave hover:text-acento focus-visible:text-acento"
                      aria-label={`Reservar ${r.nombre} el ${etiquetaDia(d)}`}
                    >
                      <Icono nombre="mas_signo" tam={16} />
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        ))}
      </div>

      {/* Móvil: día con lista de franjas */}
      <div className="md:hidden">
        <div className="carrusel gap-2 px-4 py-3" role="tablist" aria-label="Día">
          {dias.map((d) => (
            <button
              key={d}
              type="button"
              role="tab"
              aria-selected={d === dm}
              onClick={() => onDiaMovil?.(d)}
              className={`relative flex h-14 w-13 shrink-0 flex-col items-center justify-center rounded-control border transition-colors duration-rapida ${d === dm ? 'border-acento bg-acento text-white' : d === hoy ? 'border-curso-borde bg-curso-suave text-curso-texto' : 'border-borde-fuerte bg-superficie'}`}
            >
              <span className={`text-xs ${d === dm ? '' : d === hoy ? '' : 'text-texto-apoyo'}`}>{d === hoy ? 'Hoy' : DIAS_CORTOS[diaSemana(d)]}</span>
              <b className="text-base">{Number(d.slice(8))}</b>
            </button>
          ))}
        </div>
        <ul className="flex flex-col gap-2 px-4 pb-4">
          {recursos.map((r) => {
            const evs = porCelda.get(`${r.id}|${dm}`) || [];
            return (
              <li key={r.id} className="rounded-tarjeta border border-borde bg-superficie p-3">
                <div className="flex items-baseline justify-between">
                  <span className="flex items-center gap-2 text-base font-semibold">
                    {r.serie && <span className="h-2.5 w-2.5 shrink-0 rounded-chip" style={{ background: colorSerie(r.serie) }} aria-hidden="true" />}
                    {r.nombre}
                  </span>
                  <span className="text-xs text-texto-apoyo">{r.detalle}</span>
                </div>
                {evs.length === 0 ? (
                  <p className="mt-1 text-sm text-texto-apoyo">Libre todo el día</p>
                ) : (
                  <div className="mt-2 flex flex-col gap-1.5">
                    {evs.map((ev) => (
                      <button key={ev.id} type="button" onClick={() => onEvento?.(ev)} className={`flex min-h-[48px] items-center justify-between gap-2 rounded-control border border-l-[3px] px-3 py-2 text-left text-sm ${TONO_EVENTO[ev.estado] || TONO_EVENTO.bloqueo}`} style={{ borderLeftColor: ev.estado === 'bloqueo' ? undefined : colorSerie(r.serie) }}>
                        <b className="flex items-center gap-1.5">
                          <Icono nombre={ICONO_EVENTO[ev.estado] || 'calendario'} tam={14} />
                          {ev.titulo}
                        </b>
                        <span>
                          {ev.desde}–{ev.hasta}
                          {ev.sub ? ` · ${ev.sub}` : ''}
                        </span>
                      </button>
                    ))}
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      </div>
    </>
  );
}

export function LeyendaCalendario() {
  const item = (clase, texto, icono) => (
    <span className="inline-flex items-center gap-2">
      <span className={`flex h-4 w-4 items-center justify-center rounded border ${clase}`}>
        <Icono nombre={icono} tam={10} />
      </span>
      {texto}
    </span>
  );
  return (
    <div className="flex flex-wrap gap-x-5 gap-y-2 border-t border-borde px-4 py-2.5 text-xs text-texto-suave">
      {item(TONO_EVENTO.confirmada, 'Confirmada', ICONO_EVENTO.confirmada)}
      {item(TONO_EVENTO.pendiente_pago, 'Retenida 15 min esperando pago', ICONO_EVENTO.pendiente_pago)}
      {item(TONO_EVENTO.bloqueo, 'Bloqueo de la administración', ICONO_EVENTO.bloqueo)}
      <span className="inline-flex items-center gap-2">
        <span className="h-3 w-0.5 rounded-chip bg-curso" aria-hidden="true" />
        Hoy
      </span>
      <span>El filo de color es el área.</span>
    </div>
  );
}
