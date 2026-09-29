import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import Icono from './Icono.jsx';
import { BotonIcono } from './Tooltip.jsx';

/**
 * Abre/cierra con clic fuera y Escape; devuelve el foco al disparador.
 * El panel se pinta en un portal con posición fija (no lo recorta el scroll de una columna del kanban
 * ni lo tapa el contenido que entra animado). Se cierra al desplazar la página o cambiar el tamaño.
 */
export function useFlotante(alinear = 'der') {
  const [abierto, setAbierto] = useState(false);
  const [pos, setPos] = useState(null);
  const caja = useRef(null);
  const panel = useRef(null);
  const disparador = useRef(null);
  useLayoutEffect(() => {
    if (!abierto || !caja.current) return;
    const r = caja.current.getBoundingClientRect();
    const abajo = window.innerHeight - r.bottom;
    const arriba = abajo < 240 && r.top > abajo;
    const p = { maxHeight: Math.max(160, (arriba ? r.top : abajo) - 12) };
    if (arriba) p.bottom = window.innerHeight - r.top + 4;
    else p.top = r.bottom + 4;
    if (alinear === 'izq') p.left = Math.max(8, Math.min(r.left, window.innerWidth - 368));
    else p.right = Math.max(8, window.innerWidth - r.right);
    setPos(p);
  }, [abierto, alinear]);
  const cerrar = useCallback((devolverFoco = true) => {
    setAbierto(false);
    if (devolverFoco) disparador.current?.focus();
  }, []);
  useEffect(() => {
    if (!abierto) return undefined;
    const fuera = (e) => {
      // Dentro del panel, o de otro flotante abierto desde él (p. ej. el calendario de una fecha), no se cierra.
      if (caja.current?.contains(e.target) || panel.current?.contains(e.target) || e.target.closest?.('[data-flotante]')) return;
      setAbierto(false);
    };
    const mover = (e) => {
      if (panel.current?.contains(e.target) || e.target.closest?.('[data-flotante]')) return;
      setAbierto(false);
    };
    const tecla = (e) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        cerrar();
      }
    };
    document.addEventListener('pointerdown', fuera);
    document.addEventListener('keydown', tecla, true);
    window.addEventListener('scroll', mover, true);
    window.addEventListener('resize', mover);
    return () => {
      document.removeEventListener('pointerdown', fuera);
      document.removeEventListener('keydown', tecla, true);
      window.removeEventListener('scroll', mover, true);
      window.removeEventListener('resize', mover);
    };
  }, [abierto, cerrar]);
  return { abierto, setAbierto, cerrar, caja, panel, disparador, pos };
}

/**
 * Menú `⋯` de acciones secundarias (Imprimir, Anular…). Ninguna acción se pierde: pasan aquí.
 * items: [{ etiqueta, icono?, onClick?, href?, target?, peligro?, deshabilitado?, oculto? }]
 * Teclado: ↑/↓ recorren, Inicio/Fin, Escape cierra y devuelve el foco.
 */
export function MenuAcciones({ items = [], etiqueta = 'Más acciones', icono = 'mas', alinear = 'der', variante = 'secundario', lado = 'arriba', className = '' }) {
  const { abierto, setAbierto, cerrar, caja, panel, disparador, pos } = useFlotante(alinear);
  const id = useId();
  const visibles = items.filter((it) => it && !it.oculto);
  useEffect(() => {
    if (abierto && pos) panel.current?.querySelector('[role="menuitem"]:not([disabled])')?.focus();
  }, [abierto, pos, panel]);
  if (!visibles.length) return null;
  const mover = (e) => {
    if (!panel.current) return;
    const lista = [...panel.current.querySelectorAll('[role="menuitem"]:not([disabled])')];
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
    <div ref={caja} className={`relative inline-flex ${className}`}>
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
      {abierto && pos && createPortal(
        <div ref={panel} data-flotante id={id} role="menu" aria-label={etiqueta} onKeyDown={mover} style={pos} className="fixed z-[80] flex min-w-[200px] flex-col overflow-y-auto rounded-tarjeta border border-borde bg-superficie p-1 shadow-flotante animate-desplegar">
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
        </div>,
        document.body,
      )}
    </div>
  );
}

/**
 * Panel desplegable con contenido libre (p. ej. el menú «Filtros» del kanban).
 * `disparador` recibe { abierto, alternar, ref, props } y pinta el botón que lo abre.
 */
export function Desplegable({ disparador, children, alinear = 'izq', ancho = 'w-[min(92vw,360px)]', etiqueta = 'Opciones' }) {
  const { abierto, setAbierto, caja, panel, disparador: ref, pos } = useFlotante(alinear);
  const id = useId();
  return (
    <div ref={caja} className="relative inline-flex">
      {disparador({
        abierto,
        alternar: () => setAbierto(!abierto),
        ref,
        props: { 'aria-expanded': abierto, 'aria-controls': abierto ? id : undefined, 'aria-haspopup': 'dialog' },
      })}
      {abierto && pos && createPortal(
        <div ref={panel} data-flotante id={id} role="dialog" aria-label={etiqueta} style={pos} className={`fixed z-[80] flex flex-col gap-3 overflow-y-auto rounded-tarjeta border border-borde bg-superficie p-tarjeta shadow-flotante animate-desplegar ${ancho}`}>
          {typeof children === 'function' ? children({ cerrar: () => setAbierto(false) }) : children}
        </div>,
        document.body,
      )}
    </div>
  );
}
