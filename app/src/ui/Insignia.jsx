import { infoEstado, TONO_CLASES } from './estados.js';

/** Pastilla de estado. Usa el mapa de estados.js; `texto` permite un rótulo más preciso («Pagado 12-09»). */
export default function Insignia({ estado, texto, tono, className = '', tam = 'sm' }) {
  const info = infoEstado(estado);
  const t = tono || info.tono;
  const tamano = tam === 'md' ? 'text-sm px-3 py-1' : 'text-xs px-2.5 py-0.5';
  return (
    <span className={`inline-flex items-center gap-1 whitespace-nowrap rounded-full border font-semibold ${tamano} ${TONO_CLASES[t] || TONO_CLASES.neutro} ${className}`}>
      {texto || info.texto}
    </span>
  );
}
