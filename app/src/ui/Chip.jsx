import Icono from './Icono.jsx';
import { TONO_PUNTO } from './estados.js';

/**
 * Chip de filtro (pastilla pulsable). Activo en tinta; `tono` pinta un punto de color
 * (criticidad, estado). `contador` va a la derecha. `onQuitar` lo vuelve un chip removible
 * (filtro activo) con su botón «×». Aparece con escala (movimiento del bloque F).
 */
export default function Chip({ activo = false, onClick, children, contador, tono, icono, onQuitar, etiquetaQuitar, className = '', ...resto }) {
  if (onQuitar) {
    return (
      <span className={`inline-flex h-8 shrink-0 items-center gap-1 rounded-chip border border-acento-borde bg-acento-suave pl-3 pr-1 text-xs font-semibold text-acento-hover animate-escala-entrar ${className}`}>
        {children}
        <button type="button" onClick={onQuitar} aria-label={etiquetaQuitar || `Quitar filtro ${typeof children === 'string' ? children : ''}`.trim()} className="flex h-6 w-6 items-center justify-center rounded-chip transition-colors duration-rapida hover:bg-acento-borde">
          <Icono nombre="cerrar" tam={12} grosor={2.25} />
        </button>
      </span>
    );
  }
  return (
    <button
      type="button"
      aria-pressed={activo}
      onClick={onClick}
      className={`inline-flex h-11 shrink-0 items-center gap-1.5 rounded-chip border px-3 text-sm font-semibold transition-[color,background-color,border-color,transform] duration-rapida active:scale-97 animate-escala-entrar lg:h-8 ${activo ? 'border-tinta bg-tinta text-white' : 'border-borde-fuerte bg-superficie text-tinta hover:bg-fondo'} ${className}`}
      {...resto}
    >
      {tono && <span className={`h-2 w-2 rounded-chip ${TONO_PUNTO[tono] || TONO_PUNTO.neutro}`} aria-hidden="true" />}
      {icono && <Icono nombre={icono} tam={14} />}
      {children}
      {contador != null && <span className={`tabular-nums ${activo ? 'text-texto-claro' : 'text-texto-apoyo'}`}>{contador}</span>}
    </button>
  );
}
