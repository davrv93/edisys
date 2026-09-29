import { useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { BotonIcono } from './Tooltip.jsx';

// Pila global: solo el modal de arriba responde a Escape y atrapa el foco.
const pila = [];
const ENFOCABLES = 'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';
const SALIDA_MS = 120; // --dur-rapida

/** ¿El usuario pidió menos movimiento? (entonces se cierra sin esperar la animación). */
function sinMovimiento() {
  try {
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  } catch {
    return false;
  }
}

/**
 * Modal accesible: foco atrapado, cierre con Escape, apilable, devuelve el foco al cerrar.
 * En móvil sale como hoja desde abajo; en escritorio, centrado.
 * `lateral`: panel a la derecha (detalle) que entra desde la derecha 16 px.
 * `cajon`: panel a la izquierda, a toda altura (menú del armazón en tablet y móvil). Nuevo en v2.
 * Movimiento: fondo que funde y panel scale(.97)→1 en 200 ms; al cerrar, 120 ms.
 */
export default function Modal({ abierto, onCerrar, titulo, children, pie, ancho = 'max-w-md', cerrable = true, lateral = false, cajon = false, etiqueta }) {
  const ref = useRef(null);
  const idTitulo = useId();
  const clave = useRef(Symbol('modal'));
  const cerrarRef = useRef(onCerrar);
  cerrarRef.current = onCerrar;
  const [montado, setMontado] = useState(abierto);
  const [saliendo, setSaliendo] = useState(false);
  // Mientras sale, se sigue pintando lo último que se vio (el padre ya puso su estado a null).
  const ultimo = useRef({ titulo, children, pie });
  if (abierto) ultimo.current = { titulo, children, pie };

  useEffect(() => {
    if (abierto) {
      setMontado(true);
      setSaliendo(false);
      return undefined;
    }
    if (!montado) return undefined;
    if (sinMovimiento()) {
      setMontado(false);
      return undefined;
    }
    setSaliendo(true);
    const t = setTimeout(() => {
      setMontado(false);
      setSaliendo(false);
    }, SALIDA_MS);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [abierto]);

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

  if (!montado) return null;
  const v = abierto ? { titulo, children, pie } : ultimo.current;
  const i = pila.indexOf(clave.current);
  const nivel = 50 + (i >= 0 ? i : pila.length) * 10;

  let posicion;
  let caja;
  let entrada;
  if (cajon) {
    posicion = 'items-stretch justify-start';
    caja = 'h-full w-[min(86vw,300px)] rounded-r-tarjeta';
    entrada = 'animate-entrar-izquierda';
  } else if (lateral) {
    posicion = 'items-end sm:items-stretch sm:justify-end';
    caja = 'w-full sm:max-w-md sm:h-full rounded-t-tarjeta sm:rounded-none max-h-[92vh] sm:max-h-none';
    entrada = 'animate-entrar-abajo sm:animate-entrar-derecha';
  } else {
    posicion = 'items-end sm:items-center justify-center sm:p-4';
    caja = `w-full ${ancho} rounded-t-tarjeta sm:rounded-tarjeta max-h-[92vh]`;
    entrada = 'animate-entrar-abajo sm:animate-escala-entrar';
  }

  return createPortal(
    <div className={`fixed inset-0 flex ${posicion}`} style={{ zIndex: nivel }}>
      <div className={`absolute inset-0 bg-tinta/50 ${saliendo ? 'animate-fundir-salida' : 'animate-fundir'}`} aria-hidden="true" onClick={() => cerrable && onCerrar?.()} />
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-labelledby={v.titulo ? idTitulo : undefined}
        aria-label={!v.titulo ? etiqueta : undefined}
        tabIndex={-1}
        className={`relative flex flex-col bg-superficie shadow-flotante focus:outline-none ${caja} ${saliendo ? 'pointer-events-none animate-escala-salir' : entrada}`}
      >
        {v.titulo && (
          <div className="flex items-start justify-between gap-4 border-b border-borde px-5 py-3">
            <h2 id={idTitulo} className="pt-1.5 text-lg font-semibold text-tinta">
              {v.titulo}
            </h2>
            {cerrable && <BotonIcono etiqueta="Cerrar" icono="cerrar" variante="suave" lado="izquierda" onClick={onCerrar} className="-mr-2" />}
          </div>
        )}
        <div className="flex-1 overflow-y-auto px-5 py-4">{v.children}</div>
        {v.pie && <div className="flex flex-col-reverse gap-2 border-t border-borde px-5 py-3 sm:flex-row sm:justify-end">{v.pie}</div>}
      </div>
    </div>,
    document.body,
  );
}
