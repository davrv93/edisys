import { DIAS_CORTOS, diaSemana, etiquetaDia } from '../lib/fechas.js';
import Icono from './Icono.jsx';

const TONO_EVENTO = {
  confirmada: 'bg-acento text-white border-acento',
  pendiente_pago: 'bg-aviso-suave text-aviso-texto border-aviso-borde',
  bloqueo: 'bg-superficie-2 text-egreso border-borde',
};

/**
 * Calendario de semana (recursos × días) en escritorio y de DÍA con lista de franjas en móvil.
 * Recibe la ocupación del servidor; no calcula disponibilidad.
 * eventos: [{ id, recurso_id, dia: 'AAAA-MM-DD', desde, hasta, estado, titulo, sub }]
 */
export default function Calendario({ dias, recursos, eventos, hoy, onEvento, onCelda, diaMovil, onDiaMovil }) {
  const porCelda = new Map();
  for (const ev of eventos) {
    const k = `${ev.recurso_id}|${ev.dia}`;
    if (!porCelda.has(k)) porCelda.set(k, []);
    porCelda.get(k).push(ev);
  }
  const dm = diaMovil || dias[0];

  return (
    <>
      {/* Escritorio: semana */}
      <div className="hidden md:block" role="grid" aria-label="Calendario semanal de reservas">
        <div className="grid grid-cols-[140px_repeat(7,minmax(0,1fr))] border-b border-borde bg-fondo text-xs text-texto-apoyo" role="row">
          <div role="columnheader" />
          {dias.map((d) => (
            <div key={d} role="columnheader" className="flex flex-col items-center gap-0.5 py-3">
              <span>{DIAS_CORTOS[diaSemana(d)]}</span>
              <span className={`text-base font-semibold ${d === hoy ? 'text-acento' : 'text-tinta'}`}>{Number(d.slice(8))}</span>
            </div>
          ))}
        </div>
        {recursos.map((r) => (
          <div key={r.id} role="row" className="grid min-h-[92px] grid-cols-[140px_repeat(7,minmax(0,1fr))] border-b border-superficie-2 text-xs">
            <div role="rowheader" className="flex flex-col justify-center gap-0.5 px-4">
              <span className="text-sm font-semibold">{r.nombre}</span>
              {r.detalle && <span className="text-texto-apoyo">{r.detalle}</span>}
            </div>
            {dias.map((d) => {
              const evs = porCelda.get(`${r.id}|${d}`) || [];
              return (
                <div key={d} role="gridcell" className="flex flex-col gap-1 border-l border-superficie-2 p-1.5">
                  {evs.map((ev) => (
                    <button
                      key={ev.id}
                      type="button"
                      onClick={() => onEvento?.(ev)}
                      className={`flex flex-col gap-0.5 rounded-control border px-2 py-1.5 text-left leading-tight ${TONO_EVENTO[ev.estado] || TONO_EVENTO.bloqueo}`}
                    >
                      <b className="truncate">{ev.titulo}</b>
                      <span>{ev.desde}–{ev.hasta}</span>
                      {ev.sub && <span className="truncate opacity-90">{ev.sub}</span>}
                    </button>
                  ))}
                  {onCelda && evs.length === 0 && (
                    <button
                      type="button"
                      onClick={() => onCelda(r, d)}
                      className="group flex flex-1 items-center justify-center rounded-control text-transparent hover:bg-acento-suave hover:text-acento focus-visible:text-acento"
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
        <div className="flex gap-2 overflow-x-auto px-4 py-3" role="tablist" aria-label="Día">
          {dias.map((d) => (
            <button
              key={d}
              type="button"
              role="tab"
              aria-selected={d === dm}
              onClick={() => onDiaMovil?.(d)}
              className={`flex h-14 w-13 shrink-0 flex-col items-center justify-center rounded-control border ${d === dm ? 'border-acento bg-acento text-white' : 'border-borde-fuerte bg-superficie'}`}
            >
              <span className={`text-xs ${d === dm ? '' : 'text-texto-apoyo'}`}>{DIAS_CORTOS[diaSemana(d)]}</span>
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
                  <span className="text-base font-semibold">{r.nombre}</span>
                  <span className="text-xs text-texto-apoyo">{r.detalle}</span>
                </div>
                {evs.length === 0 ? (
                  <p className="mt-1 text-sm text-texto-apoyo">Libre todo el día</p>
                ) : (
                  <div className="mt-2 flex flex-col gap-1.5">
                    {evs.map((ev) => (
                      <button key={ev.id} type="button" onClick={() => onEvento?.(ev)} className={`flex min-h-[44px] items-center justify-between rounded-control border px-3 py-2 text-left text-sm ${TONO_EVENTO[ev.estado] || TONO_EVENTO.bloqueo}`}>
                        <b>{ev.titulo}</b>
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
  const item = (clase, texto) => (
    <span className="inline-flex items-center gap-2">
      <span className={`h-3 w-3 rounded border ${clase}`} />
      {texto}
    </span>
  );
  return (
    <div className="flex flex-wrap gap-x-6 gap-y-2 px-4 py-3 text-xs text-texto-suave">
      {item(TONO_EVENTO.confirmada, 'Confirmada')}
      {item(TONO_EVENTO.pendiente_pago, 'Retenida 15 min esperando pago')}
      {item(TONO_EVENTO.bloqueo, 'Bloqueo de la administración')}
    </div>
  );
}
