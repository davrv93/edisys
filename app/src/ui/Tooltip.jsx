import { forwardRef } from 'react';
import Icono from './Icono.jsx';

const LADOS = {
  arriba: 'bottom-full left-1/2 mb-1.5 -translate-x-1/2',
  abajo: 'top-full left-1/2 mt-1.5 -translate-x-1/2',
  derecha: 'left-full top-1/2 ml-2 -translate-y-1/2',
  izquierda: 'right-full top-1/2 mr-2 -translate-y-1/2',
};

/**
 * Tooltip propio (sin `title` nativo): aparece al pasar el ratón y con el foco de teclado.
 * Es solo visual (aria-hidden): el nombre accesible lo lleva el control (aria-label).
 * `className` permite limitarlo a un tamaño (p. ej. «xl:hidden» en el menú de iconos).
 */
export function Tooltip({ texto, lado = 'arriba', children, className = '' }) {
  return (
    <span className="group/tt relative inline-flex">
      {children}
      {texto && (
        <span
          aria-hidden="true"
          className={`pointer-events-none absolute z-50 whitespace-nowrap rounded-control bg-tinta px-2 py-1 text-xs font-semibold text-white opacity-0 shadow-flotante transition-opacity duration-rapida group-focus-within/tt:opacity-100 group-hover/tt:opacity-100 ${LADOS[lado] || LADOS.arriba} ${className}`}
        >
          {texto}
        </span>
      )}
    </span>
  );
}

const VARIANTES = {
  fantasma: 'text-texto-suave hover:bg-superficie-2 hover:text-tinta',
  secundario: 'border border-borde-fuerte bg-superficie text-tinta hover:bg-fondo',
  oscuro: 'text-texto-claro hover:bg-superficie-oscura hover:text-white',
  suave: 'bg-superficie-2 text-egreso hover:bg-borde',
};

/**
 * Botón solo de icono (cerrar, siguiente mes, menú…). `etiqueta` es obligatoria: va en aria-label
 * y en el tooltip. 44 px en táctil, 36 px en escritorio.
 */
export const BotonIcono = forwardRef(function BotonIcono(
  { etiqueta, icono, tam = 18, variante = 'fantasma', href, lado = 'arriba', tooltip = true, className = '', type = 'button', ...resto },
  ref,
) {
  const clases = `inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-control transition-colors duration-rapida active:scale-97 disabled:opacity-40 lg:h-9 lg:w-9 ${VARIANTES[variante] || VARIANTES.fantasma} ${className}`;
  const el = href ? (
    <a ref={ref} href={href} aria-label={etiqueta} className={clases} {...resto}>
      <Icono nombre={icono} tam={tam} />
    </a>
  ) : (
    <button ref={ref} type={type} aria-label={etiqueta} className={clases} {...resto}>
      <Icono nombre={icono} tam={tam} />
    </button>
  );
  return tooltip ? (
    <Tooltip texto={etiqueta} lado={lado}>
      {el}
    </Tooltip>
  ) : (
    el
  );
});
