import { BotonIcono } from './Tooltip.jsx';
import { nombrePeriodo, sumarMeses } from '../lib/fechas.js';

/** Mes y año («2026-09») con flechas. */
export function SelectorPeriodo({ periodo, onCambio, max, oscuro = false }) {
  const puedeSiguiente = !max || periodo < max;
  const variante = oscuro ? 'oscuro' : 'secundario';
  return (
    <div className="flex items-center gap-1" role="group" aria-label="Periodo">
      <BotonIcono variante={variante} icono="izq" etiqueta="Mes anterior" onClick={() => onCambio(sumarMeses(periodo, -1))} />
      <span className={`min-w-[120px] text-center text-sm font-semibold tabular-nums ${oscuro ? 'text-white' : 'text-tinta'}`} aria-live="polite">
        {nombrePeriodo(periodo)}
      </span>
      <BotonIcono variante={variante} icono="der" etiqueta="Mes siguiente" onClick={() => onCambio(sumarMeses(periodo, 1))} disabled={!puedeSiguiente} />
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
