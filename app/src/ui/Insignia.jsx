import Icono from './Icono.jsx';
import { infoEstado, TONO_CLASES, TONO_PUNTO } from './estados.js';

/**
 * Pastilla de estado. Usa el mapa de estados.js (color + icono + texto);
 * `texto` permite un rótulo más preciso («Pagado 12-09»). `icono={false}` la deja solo con texto.
 */
export default function Insignia({ estado, texto, tono, className = '', tam = 'sm', icono = true }) {
  const info = infoEstado(estado);
  const t = tono || info.tono;
  const tamano = tam === 'md' ? 'text-sm px-3 py-1 gap-1.5' : 'text-xs px-2 py-0.5 gap-1';
  return (
    <span className={`inline-flex items-center whitespace-nowrap rounded-chip border font-semibold ${tamano} ${TONO_CLASES[t] || TONO_CLASES.neutro} ${className}`}>
      {icono && <Icono nombre={info.icono} tam={tam === 'md' ? 14 : 12} grosor={2} />}
      {texto || info.texto}
    </span>
  );
}

/** Punto de color con su texto accesible (criticidad en la tarjeta del kanban, deuda en tablas). */
export function PuntoEstado({ estado, tono, texto, className = '' }) {
  const info = infoEstado(estado);
  const t = tono || info.tono;
  return (
    <span className={`inline-flex items-center gap-1.5 ${className}`}>
      <span className={`h-2 w-2 shrink-0 rounded-chip ${TONO_PUNTO[t] || TONO_PUNTO.neutro}`} aria-hidden="true" />
      <span>{texto ?? info.texto}</span>
    </span>
  );
}
