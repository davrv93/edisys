import { useCallback, useEffect, useId, useRef, useState } from 'react';
import Icono from './Icono.jsx';
import { BotonIcono } from './Tooltip.jsx';

/** Abre/cierra con clic fuera y Escape; devuelve el foco al disparador. */
function useFlotante() {
  const [abierto, setAbierto] = useState(false);
  const caja = useRef(null);
  const disparador = useRef(null);
  const cerrar = useCallback((devolverFoco = true) => {
    setAbierto(false);
    if (devolverFoco) disparador.current?.focus();
  }, []);
  useEffect(() => {
    if (!abierto) return undefined;
    const fuera = (e) => {
      if (caja.current && !caja.current.contains(e.target)) setAbierto(false);
    };
    const tecla = (e) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        cerrar();
      }
    };
    document.addEventListener('pointerdown', fuera);
    document.addEventListener('keydown', tecla, true);
    return () => {
      document.removeEventListener('pointerdown', fuera);
      document.removeEventListener('keydown', tecla, true);
    };
  }, [abierto, cerrar]);
  return { abierto, setAbierto, cerrar, caja, disparador };
}

const ALINEAR = { der: 'right-0', izq: 'left-0' };

/**
 * Menú `⋯` de acciones secundarias (Imprimir, Anular…). Ninguna acción se pierde: pasan aquí.
 * items: [{ etiqueta, icono?, onClick?, href?, target?, peligro?, deshabilitado?, oculto? }]
 * Teclado: ↑/↓ recorren, Inicio/Fin, Escape cierra y devuelve el foco.
 */
export function MenuAcciones({ items = [], etiqueta = 'Más acciones', icono = 'mas', alinear = 'der', variante = 'secundario', lado = 'arriba', className = '' }) {
  const { abierto, setAbierto, cerrar, caja, disparador } = useFlotante();
  const id = useId();
  const visibles = items.filter((it) => it && !it.oculto);
  useEffect(() => {
    if (abierto) caja.current?.querySelector('[role="menuitem"]:not([disabled])')?.focus();
  }, [abierto, caja]);
  if (!visibles.length) return null;
  const mover = (e) => {
    const lista = [...caja.current.querySelectorAll('[role="menuitem"]:not([disabled])')];
    const i = lista.indexOf(document.activeElement);
    let j = null;
    if (e.key === 'ArrowDown') j = (i + 1) % lista.length;
    else if (e.key === 'ArrowUp') j = (i - 1 + lista.length) % lista.length;
    else if (e.key === 'Home') j = 0;
    else if (e.key === 'End') j = lista.length - 1;
    else if (e.key === 'Tab') setAbierto(false);
    if (j != null) {
      e.preventDefault();
      lista[j]?.focus();
    }
  };
  return (
    <div ref={caja} className={`relative inline-flex ${className}`} onKeyDown={abierto ? mover : undefined}>
      <BotonIcono
        ref={disparador}
        etiqueta={etiqueta}
        icono={icono}
        variante={variante}
        lado={lado}
        tooltip={!abierto}
        aria-haspopup="menu"
        aria-expanded={abierto}
        aria-controls={abierto ? id : undefined}
        onClick={() => setAbierto(!abierto)}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown' && !abierto) {
            e.preventDefault();
            setAbierto(true);
          }
        }}
      />
      {abierto && (
        <div id={id} role="menu" aria-label={etiqueta} className={`absolute top-full z-40 mt-1 flex min-w-[200px] flex-col rounded-tarjeta border border-borde bg-superficie p-1 shadow-flotante animate-desplegar ${ALINEAR[alinear] || ALINEAR.der}`}>
          {visibles.map((it, i) => {
            const clases = `flex min-h-[44px] w-full items-center gap-3 whitespace-nowrap rounded-control px-3 text-left text-sm transition-colors duration-rapida lg:min-h-9 ${it.peligro ? 'text-alerta hover:bg-alerta-suave focus-visible:bg-alerta-suave' : 'text-tinta hover:bg-fondo focus-visible:bg-fondo'} disabled:cursor-not-allowed disabled:opacity-50`;
            const contenido = (
              <>
                {it.icono ? <Icono nombre={it.icono} tam={16} className={it.peligro ? '' : 'text-texto-apoyo'} /> : <span className="w-4" />}
                {it.etiqueta}
              </>
            );
            return it.href ? (
              <a key={i} role="menuitem" href={it.href} target={it.target} rel={it.target ? 'noopener' : undefined} className={`${clases} hover:text-tinta`} onClick={() => cerrar(false)}>
                {contenido}
              </a>
            ) : (
              <button
                key={i}
                type="button"
                role="menuitem"
                disabled={it.deshabilitado}
                className={clases}
                onClick={() => {
                  cerrar();
                  it.onClick?.();
                }}
              >
                {contenido}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

/**
 * Panel desplegable con contenido libre (p. ej. el menú «Filtros» del kanban).
 * `disparador` recibe { abierto, alternar, ref, props } y pinta el botón que lo abre.
 */
export function Desplegable({ disparador, children, alinear = 'izq', ancho = 'w-[min(92vw,360px)]', etiqueta = 'Opciones' }) {
  const { abierto, setAbierto, caja, disparador: ref } = useFlotante();
  const id = useId();
  return (
    <div ref={caja} className="relative inline-flex">
      {disparador({
        abierto,
        alternar: () => setAbierto(!abierto),
        ref,
        props: { 'aria-expanded': abierto, 'aria-controls': abierto ? id : undefined, 'aria-haspopup': 'dialog' },
      })}
      {abierto && (
        <div id={id} role="dialog" aria-label={etiqueta} className={`absolute top-full z-40 mt-1 flex flex-col gap-3 rounded-tarjeta border border-borde bg-superficie p-tarjeta shadow-flotante animate-desplegar ${ancho} ${ALINEAR[alinear] || ALINEAR.izq}`}>
          {typeof children === 'function' ? children({ cerrar: () => setAbierto(false) }) : children}
        </div>
      )}
    </div>
  );
}
