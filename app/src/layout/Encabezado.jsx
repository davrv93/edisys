import { BotonIcono, MenuAcciones } from '../ui/index.js';

/**
 * Cabecera de pantalla (v2): 56 px en escritorio. Título de 22 px a la izquierda; a la derecha,
 * `acciones` (la principal y lo que deba verse) y el menú `⋯` con las `secundarias`
 * (Imprimir, Exportar…): ninguna acción se pierde, pasan al menú.
 * secundarias: [{ etiqueta, icono?, onClick?, href?, peligro?, oculto?, soloMovil? }] (ver ui/Menu.jsx).
 * `soloMovil`: la acción ya está a la vista en escritorio (en `acciones`) y en el celular pasa al menú.
 * En móvil: título y `⋯` en una fila; las acciones debajo, en una sola línea que se desliza si no cabe.
 */
export default function Encabezado({ titulo, subtitulo, acciones, secundarias, volver, children }) {
  const visibles = (secundarias || []).filter((x) => x && !x.oculto);
  const escritorio = visibles.filter((x) => !x.soloMovil);
  const menu = visibles.length ? <MenuAcciones items={visibles} lado="abajo" /> : null;
  const menuEscritorio = escritorio.length ? <MenuAcciones items={escritorio} lado="abajo" /> : null;
  return (
    <header className="border-b border-borde bg-superficie">
      <div className="flex flex-col gap-2 px-4 py-3 lg:min-h-topbar lg:flex-row lg:items-center lg:justify-between lg:gap-4 lg:px-6 lg:py-2">
        <div className="flex min-w-0 items-center gap-1">
          {volver && <BotonIcono href={volver} etiqueta="Volver" icono="volver" lado="abajo" className="-ml-2" />}
          <div className="min-w-0 flex-1">
            <h1 className="truncate font-titulo text-titulo-pantalla font-semibold">{titulo}</h1>
            {subtitulo && <p className="truncate text-xs text-texto-apoyo">{subtitulo}</p>}
          </div>
          {menu && <span className="lg:hidden">{menu}</span>}
        </div>
        {(acciones || menuEscritorio) && (
          <div className="-mx-4 flex items-center gap-2 overflow-x-auto px-4 pb-0.5 lg:mx-0 lg:shrink-0 lg:overflow-visible lg:px-0 lg:pb-0">
            {acciones}
            {menuEscritorio && <span className="hidden lg:inline-flex">{menuEscritorio}</span>}
          </div>
        )}
      </div>
      {children}
    </header>
  );
}

/** Cabecera blanca de las pantallas-tarea en el celular (07 y 09 del lienzo). */
export function CabeceraTarea({ titulo, subtitulo, volver }) {
  return (
    <header className="sticky top-0 z-30 flex h-14 items-center gap-1 border-b border-borde bg-superficie px-2">
      <BotonIcono href={volver} etiqueta="Volver" icono="volver" lado="abajo" />
      <div className="flex min-w-0 flex-col">
        <span className="truncate text-lg font-semibold leading-tight">{titulo}</span>
        {subtitulo && <span className="truncate text-xs text-texto-apoyo">{subtitulo}</span>}
      </div>
    </header>
  );
}

/** Contenedor de contenido con ancho máximo de 1280 px. Sus hijos entran escalonados (30 ms, máx. 6). */
export function Contenido({ children, className = '' }) {
  return <div className={`escalonado mx-auto flex w-full max-w-contenido flex-col gap-4 p-4 lg:gap-5 lg:p-6 ${className}`}>{children}</div>;
}

/** Sección con título y enlace opcional. Separación con borde y espacio, sin sombra. */
export function Seccion({ titulo, extra, children, className = '', padding = 'p-tarjeta' }) {
  return (
    <section className={`flex min-w-0 flex-col gap-3 rounded-tarjeta border border-borde bg-superficie ${padding} ${className}`}>
      {(titulo || extra) && (
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          {titulo && <h2 className="text-base font-semibold">{titulo}</h2>}
          {extra}
        </div>
      )}
      {children}
    </section>
  );
}

/** Barra de acción fija abajo en el celular (por encima de la barra de pestañas si la hay). */
export function BarraAccion({ children, sobreTabs = false }) {
  return (
    <div className={`fixed inset-x-0 z-20 border-t border-borde bg-superficie p-4 pb-[max(1rem,env(safe-area-inset-bottom))] ${sobreTabs ? 'bottom-16 lg:bottom-0' : 'bottom-0'} lg:static lg:border-0 lg:bg-transparent lg:p-0`}>
      {children}
    </div>
  );
}
