import { forwardRef } from 'react';
import Icono from './Icono.jsx';

const VARIANTES = {
  primario: 'bg-acento text-white hover:bg-acento-hover border border-transparent',
  secundario: 'bg-superficie text-tinta border border-borde-fuerte hover:bg-fondo',
  fantasma: 'bg-transparent text-acento border border-transparent hover:bg-acento-suave',
  peligro: 'bg-alerta text-white border border-transparent hover:brightness-90',
  oscuro: 'bg-tinta text-white border border-transparent hover:bg-superficie-oscura',
};

// En móvil el área táctil mínima es de 44 px (h-11); en escritorio, 36 px (tokens v2: --alto-control).
const TAMANOS = {
  sm: 'h-11 lg:h-8 px-3 text-sm',
  md: 'h-11 lg:h-9 px-3.5 text-sm',
  lg: 'h-13 lg:h-11 px-5 text-base',
};

export function Spinner({ className = '' }) {
  return (
    <span
      className={`inline-block h-4 w-4 animate-spin rounded-full border-2 border-current border-r-transparent ${className}`}
      role="status"
      aria-label="Cargando"
    />
  );
}

/**
 * Botón del sistema. `cargando` lo desactiva (evita doble envío de pagos y lecturas).
 * Con `href` (o `to`, sinónimo) se pinta como enlace <a> a otra página.
 */
const Boton = forwardRef(function Boton(
  { variante = 'primario', tamano = 'md', cargando = false, icono, iconoDer, bloque = false, to, href, className = '', children, disabled, type = 'button', ...resto },
  ref,
) {
  const clases = [
    'inline-flex max-w-full items-center justify-center gap-2 rounded-control font-semibold whitespace-nowrap transition-colors select-none',
    'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-acento',
    'disabled:opacity-50 disabled:cursor-not-allowed aria-disabled:opacity-50',
    VARIANTES[variante] || VARIANTES.primario,
    TAMANOS[tamano] || TAMANOS.md,
    bloque ? 'w-full' : '',
    className,
  ].join(' ');
  const contenido = (
    <>
      {cargando ? <Spinner /> : icono ? <Icono nombre={icono} tam={18} /> : null}
      {children != null && children !== false && <span className="min-w-0 truncate">{children}</span>}
      {iconoDer && !cargando ? <Icono nombre={iconoDer} tam={18} /> : null}
    </>
  );
  if (href || to) {
    return (
      <a ref={ref} href={href || to} className={clases} {...resto}>
        {contenido}
      </a>
    );
  }
  return (
    <button ref={ref} type={type} className={clases} disabled={disabled || cargando} aria-busy={cargando || undefined} {...resto}>
      {contenido}
    </button>
  );
});

export default Boton;
