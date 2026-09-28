import { Icono } from '../ui/index.js';

/**
 * Cabecera de pantalla. Escritorio: barra blanca de 72 px con h1 en Fraunces y acciones.
 * Móvil: título y acciones debajo, a una columna.
 */
export default function Encabezado({ titulo, subtitulo, acciones, volver, children }) {
  return (
    <header className="border-b border-borde bg-superficie">
      <div className="flex flex-col gap-3 px-4 py-4 lg:min-h-topbar lg:flex-row lg:items-center lg:justify-between lg:px-8 lg:py-3">
        <div className="flex min-w-0 items-center gap-2">
          {volver && (
            <a href={volver} className="-ml-2 flex h-11 w-11 shrink-0 items-center justify-center rounded-lg hover:bg-fondo" aria-label="Volver">
              <Icono nombre="volver" />
            </a>
          )}
          <div className="min-w-0">
            <h1 className="font-titulo text-2xl font-semibold leading-tight lg:text-3xl">{titulo}</h1>
            {subtitulo && <p className="text-sm text-texto-apoyo">{subtitulo}</p>}
          </div>
        </div>
        {acciones && <div className="flex flex-wrap items-center gap-2 lg:gap-3">{acciones}</div>}
      </div>
      {children}
    </header>
  );
}

/** Cabecera blanca de las pantallas-tarea en el celular (07 y 09 del lienzo). */
export function CabeceraTarea({ titulo, subtitulo, volver }) {
  return (
    <header className="sticky top-0 z-30 flex h-16 items-center gap-1 border-b border-borde bg-superficie px-2">
      <a href={volver} className="flex h-11 w-11 items-center justify-center rounded-lg hover:bg-fondo" aria-label="Volver">
        <Icono nombre="volver" />
      </a>
      <div className="flex min-w-0 flex-col">
        <span className="truncate text-lg font-semibold">{titulo}</span>
        {subtitulo && <span className="truncate text-xs text-texto-apoyo">{subtitulo}</span>}
      </div>
    </header>
  );
}

/** Contenedor de contenido con ancho máximo de 1280 px. */
export function Contenido({ children, className = '' }) {
  return <div className={`mx-auto flex w-full max-w-contenido flex-col gap-4 p-4 lg:gap-6 lg:p-8 ${className}`}>{children}</div>;
}

/** Tarjeta de sección con título y enlace opcional, como en el lienzo. */
export function Seccion({ titulo, extra, children, className = '', padding = 'p-4 lg:p-6' }) {
  return (
    <section className={`flex min-w-0 flex-col gap-4 rounded-xl border border-borde bg-superficie ${padding} ${className}`}>
      {(titulo || extra) && (
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          {titulo && <h2 className="text-lg font-semibold">{titulo}</h2>}
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
