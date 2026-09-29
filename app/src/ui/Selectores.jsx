import Icono from './Icono.jsx';
import { nombrePeriodo, sumarMeses } from '../lib/fechas.js';

/** Mes y año («2026-09») con flechas. */
export function SelectorPeriodo({ periodo, onCambio, max, oscuro = false }) {
  const puedeSiguiente = !max || periodo < max;
  const btn = `flex h-11 w-11 lg:h-10 lg:w-10 items-center justify-center rounded-control border ${oscuro ? 'border-superficie-oscura-2 text-white hover:bg-superficie-oscura' : 'border-borde-fuerte bg-superficie hover:bg-fondo'} disabled:opacity-40`;
  return (
    <div className="flex items-center gap-2" role="group" aria-label="Periodo">
      <button type="button" className={btn} onClick={() => onCambio(sumarMeses(periodo, -1))} aria-label="Mes anterior">
        <Icono nombre="izq" tam={18} />
      </button>
      <span className={`min-w-[132px] text-center text-sm font-semibold tabular-nums ${oscuro ? 'text-white' : 'text-tinta'}`} aria-live="polite">
        {nombrePeriodo(periodo)}
      </span>
      <button type="button" className={btn} onClick={() => onCambio(sumarMeses(periodo, 1))} disabled={!puedeSiguiente} aria-label="Mes siguiente">
        <Icono nombre="der" tam={18} />
      </button>
    </div>
  );
}

/** Cambia de edificio. Solo aparece si el usuario tiene más de uno. */
export function SelectorEdificio({ edificios = [], actual, onCambio, oscuro = true }) {
  const edif = edificios.find((e) => String(e.id) === String(actual)) || edificios[0];
  if (!edif) return null;
  const unidades = edif.unidades ?? edif.n_unidades;
  if (edificios.length <= 1) {
    return (
      <div className={`flex flex-col gap-0.5 rounded-control border p-3 ${oscuro ? 'border-superficie-oscura-2 bg-superficie-oscura text-white' : 'border-borde bg-superficie'}`}>
        <span className="text-sm font-semibold">{edif.nombre}</span>
        {unidades != null && <span className={`text-xs ${oscuro ? 'text-texto-tenue' : 'text-texto-apoyo'}`}>{unidades} unidades</span>}
      </div>
    );
  }
  return (
    <label className={`relative flex flex-col gap-0.5 rounded-control border p-3 ${oscuro ? 'border-superficie-oscura-2 bg-superficie-oscura text-white' : 'border-borde bg-superficie'}`}>
      <span className="text-sm font-semibold">{edif.nombre}</span>
      <span className={`text-xs ${oscuro ? 'text-texto-tenue' : 'text-texto-apoyo'}`}>
        {unidades != null ? `${unidades} unidades · ` : ''}cambiar edificio
      </span>
      <select
        aria-label="Cambiar de edificio"
        className="absolute inset-0 cursor-pointer opacity-0"
        value={String(edif.id)}
        onChange={(e) => onCambio(e.target.value)}
      >
        {edificios.map((e) => (
          <option key={e.id} value={String(e.id)}>
            {e.nombre}
          </option>
        ))}
      </select>
    </label>
  );
}
