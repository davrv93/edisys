import Icono from './Icono.jsx';
import { Esqueleto } from './EstadosPantalla.jsx';
import { TONO_TEXTO } from './estados.js';

/**
 * Variación contra el periodo anterior como flecha pequeña + texto (no solo color).
 * variacion: { texto: '+12 % vs. agosto', buena: true|false }
 */
export function Variacion({ variacion, className = '' }) {
  if (!variacion) return null;
  const baja = String(variacion.texto || '').trim().startsWith('-') || String(variacion.texto || '').trim().startsWith('−');
  return (
    <span className={`inline-flex items-center gap-0.5 text-xs font-semibold ${variacion.buena ? 'text-acento' : 'text-alerta'} ${className}`}>
      <Icono nombre={baja ? 'baja' : 'sube'} tam={14} grosor={2} />
      {variacion.texto}
      <span className="sr-only">{variacion.buena ? ', bien' : ', a vigilar'}</span>
    </span>
  );
}

function Cifra({ it, principal, cargando }) {
  const tono = it.tono && it.tono !== 'neutro' ? TONO_TEXTO[it.tono] : 'text-tinta';
  const cuerpo = (
    <>
      <span className={`flex items-center gap-1.5 text-xs ${principal && it.tono === 'alerta' ? 'font-semibold text-alerta-texto' : 'text-texto-apoyo'}`}>
        {it.icono && <Icono nombre={it.icono} tam={14} />}
        {it.titulo}
      </span>
      {cargando ? (
        <Esqueleto className={principal ? 'h-8 w-28' : 'h-6 w-24'} />
      ) : (
        <span
          className={`whitespace-nowrap font-titulo font-semibold tabular-nums leading-tight ${principal ? 'text-kpi' : 'text-lg'} ${tono}`}
          title={it.valorCompleto && it.valorCompleto !== it.valor ? it.valorCompleto : undefined}
        >
          {it.valor}
          {it.valorCompleto && it.valorCompleto !== it.valor && <span className="sr-only"> ({it.valorCompleto})</span>}
        </span>
      )}
      {(it.nota || it.variacion) && (
        <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-texto-apoyo">
          {it.nota && <span className="min-w-0 truncate">{it.nota}</span>}
          <Variacion variacion={it.variacion} />
        </span>
      )}
    </>
  );
  const clases = `group flex h-full min-w-0 flex-col justify-center gap-0.5 px-4 py-3 ${principal ? (it.tono === 'alerta' ? 'bg-alerta-suave' : it.tono === 'acento' ? 'bg-acento-suave' : '') : ''}`;
  if (it.to) {
    return (
      <a href={it.to} className={`${clases} text-tinta transition-colors duration-rapida hover:bg-fondo hover:text-tinta ${principal && it.tono === 'alerta' ? 'hover:bg-alerta-suave/70' : ''}`}>
        {cuerpo}
        {it.verDetalle && <span className="text-xs font-semibold text-acento">{it.verDetalle} →</span>}
      </a>
    );
  }
  return <div className={clases}>{cuerpo}</div>;
}

/**
 * Franja compacta de KPI (v2, resuelve «seis KPI iguales compitiendo»): la cifra principal
 * en grande a la izquierda, el resto en fila, separadas por líneas finas y sin cajas.
 * principal / items: { titulo, valor, valorCompleto?, nota?, variacion?, tono?, icono?, to?, verDetalle? }
 */
export default function FranjaKPI({ principal, items = [], cargando = false, etiqueta = 'Indicadores' }) {
  const n = items.length;
  // Móvil: 2 columnas (la principal ocupa la fila). Tablet: la principal arriba y el resto en fila. ≥ 1280: todo en una fila.
  const colsMd = { 1: 'md:grid-cols-1', 2: 'md:grid-cols-2', 3: 'md:grid-cols-3', 4: 'md:grid-cols-4', 5: 'md:grid-cols-5' }[n] || 'md:grid-cols-4';
  const spanMd = { 1: 'md:col-span-1', 2: 'md:col-span-2', 3: 'md:col-span-3', 4: 'md:col-span-4', 5: 'md:col-span-5' }[n] || 'md:col-span-4';
  const colsXl = { 1: 'xl:grid-cols-[1.3fr_1fr]', 2: 'xl:grid-cols-[1.3fr_repeat(2,1fr)]', 3: 'xl:grid-cols-[1.3fr_repeat(3,1fr)]', 4: 'xl:grid-cols-[1.3fr_repeat(4,1fr)]', 5: 'xl:grid-cols-[1.3fr_repeat(5,1fr)]' }[n] || 'xl:grid-cols-5';
  const impar = n % 2 === 1;
  return (
    <section aria-label={etiqueta} aria-busy={cargando || undefined} className={`grid grid-cols-2 gap-px overflow-hidden rounded-tarjeta border border-borde bg-borde ${colsMd} ${colsXl}`}>
      {principal && (
        <div className={`col-span-2 flex bg-superficie ${spanMd} xl:col-span-1`}>
          <div className="flex-1">
            <Cifra it={principal} principal cargando={cargando} />
          </div>
        </div>
      )}
      {items.map((it, i) => (
        <div key={it.titulo || i} className={`flex bg-superficie ${impar && i === n - 1 ? 'col-span-2 md:col-span-1' : ''}`}>
          <div className="min-w-0 flex-1">
            <Cifra it={it} cargando={cargando} />
          </div>
        </div>
      ))}
    </section>
  );
}
