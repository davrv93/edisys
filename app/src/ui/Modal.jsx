import { useEffect, useId, useRef } from 'react';
import { createPortal } from 'react-dom';
import { BotonIcono } from './Tooltip.jsx';

// Pila global: solo el modal de arriba responde a Escape y atrapa el foco.
const pila = [];
const ENFOCABLES = 'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';

/**
 * Modal accesible: foco atrapado, cierre con Escape, apilable, devuelve el foco al cerrar.
 * En móvil sale como hoja desde abajo; en escritorio, centrado.
 */
export default function Modal({ abierto, onCerrar, titulo, children, pie, ancho = 'max-w-md', cerrable = true, lateral = false }) {
  const ref = useRef(null);
  const idTitulo = useId();
  const clave = useRef(Symbol('modal'));
  const cerrarRef = useRef(onCerrar);
  cerrarRef.current = onCerrar;

  useEffect(() => {
    if (!abierto) return undefined;
    const previo = document.activeElement;
    const yo = clave.current;
    pila.push(yo);
    const cuerpo = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const t = setTimeout(() => {
      const el = ref.current;
      if (!el) return;
      const primero = el.querySelector('[data-autofoco]') || el.querySelector(ENFOCABLES);
      (primero || el).focus();
    }, 0);
    const onKey = (e) => {
      if (pila[pila.length - 1] !== yo) return;
      if (e.key === 'Escape' && cerrable) {
        e.stopPropagation();
        cerrarRef.current?.();
      } else if (e.key === 'Tab') {
        const el = ref.current;
        if (!el) return;
        const items = [...el.querySelectorAll(ENFOCABLES)].filter((x) => x.offsetParent !== null);
        if (!items.length) return;
        const [a, z] = [items[0], items[items.length - 1]];
        if (e.shiftKey && document.activeElement === a) {
          e.preventDefault();
          z.focus();
        } else if (!e.shiftKey && document.activeElement === z) {
          e.preventDefault();
          a.focus();
        }
      }
    };
    document.addEventListener('keydown', onKey, true);
    return () => {
      clearTimeout(t);
      document.removeEventListener('keydown', onKey, true);
      const i = pila.indexOf(yo);
      if (i >= 0) pila.splice(i, 1);
      if (!pila.length) document.body.style.overflow = cuerpo;
      if (previo && previo.focus) previo.focus();
    };
  }, [abierto, cerrable]);

  if (!abierto) return null;
  const nivel = 50 + pila.indexOf(clave.current) * 10;

  const posicion = lateral
    ? 'items-end sm:items-stretch sm:justify-end'
    : 'items-end sm:items-center justify-center';
  const caja = lateral
    ? `w-full sm:max-w-md sm:h-full rounded-t-tarjeta sm:rounded-none max-h-[92vh] sm:max-h-none`
    : `w-full ${ancho} rounded-t-tarjeta sm:rounded-tarjeta max-h-[92vh]`;

  return createPortal(
    <div className={`fixed inset-0 flex ${posicion} sm:p-4 ${lateral ? 'sm:p-0' : ''}`} style={{ zIndex: nivel }}>
      <div className="absolute inset-0 bg-tinta/50" aria-hidden="true" onClick={() => cerrable && onCerrar?.()} />
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titulo ? idTitulo : undefined}
        tabIndex={-1}
        className={`relative flex flex-col bg-superficie shadow-flotante focus:outline-none ${caja}`}
      >
        {titulo && (
          <div className="flex items-start justify-between gap-4 border-b border-borde px-5 py-4">
            <h2 id={idTitulo} className="text-lg font-semibold text-tinta">
              {titulo}
            </h2>
            {cerrable && (
              <BotonIcono etiqueta="Cerrar" icono="cerrar" variante="suave" lado="abajo" onClick={onCerrar} className="-mr-2 -mt-1" />
            )}
          </div>
        )}
        <div className="flex-1 overflow-y-auto px-5 py-4">{children}</div>
        {pie && <div className="flex flex-col-reverse gap-2 border-t border-borde px-5 py-4 sm:flex-row sm:justify-end">{pie}</div>}
      </div>
    </div>,
    document.body,
  );
}
