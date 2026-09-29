import { createContext, Fragment, useCallback, useContext, useEffect, useState } from 'react';
import { useSesion } from './Sesion.jsx';
import { menuPara, gruposPara, NOMBRE_ROL } from '../lib/permisos.js';
import { ruta } from '../lib/nav.jsx';
import { BotonIcono, Icono, Isotipo, Logo, Modal, SelectorEdificio, Tooltip } from '../ui/index.js';

const ArmazonCtx = createContext({ setModoTarea: () => {} });
const CLAVE_MENU = 'edisys.menu'; // 'abierto' | 'iconos' (elección del usuario en escritorio)
const CLAVE_GRUPOS = 'edisys.grupos'; // clave vieja (lista de cerrados): se limpia, ahora manda la página

function leerPreferencia() {
  try {
    const v = window.localStorage.getItem(CLAVE_MENU);
    return v === 'abierto' || v === 'iconos' ? v : null;
  } catch {
    return null;
  }
}
function guardarPreferencia(v) {
  try {
    window.localStorage.setItem(CLAVE_MENU, v);
  } catch {
    /* sin almacenamiento: la elección dura lo que la página */
  }
}

/**
 * Pantallas de «una tarea» en el celular (reservar, reportar, leer medidores):
 * ocultan la cabecera y la barra inferior del armazón y ponen las suyas, como en el lienzo.
 * Con modo 'cabecera' solo se oculta la cabecera (el portal trae la suya y conserva las pestañas).
 */
export function useModoTarea(activo = true, modo = 'tarea') {
  const { setModoTarea } = useContext(ArmazonCtx);
  useEffect(() => {
    if (!activo) return undefined;
    setModoTarea(modo);
    return () => setModoTarea(false);
  }, [activo, modo, setModoTarea]);
}

/** ¿El ítem del menú corresponde a la página actual? (con query opcional, p. ej. ?reportar=1) */
function esActivo(it, pagina, query) {
  if (it.pagina !== pagina) return false;
  if (!it.query) return true;
  return Object.entries(it.query).every(([k, v]) => query.get(k) === String(v));
}

/**
 * Armazón responsivo común a todas las páginas Astro (v2).
 * - ≥ 1280 px: menú lateral abierto (220 px).
 * - 1024–1279 px: solo iconos (64 px) con el nombre en un tooltip.
 *   En escritorio el usuario puede plegarlo o abrirlo; se recuerda (localStorage).
 * - < 1024 px: cabecera con botón de menú (cajón a la izquierda) y barra inferior del rol + «Más».
 * Los ítems salen de lib/permisos.js (ITEMS y MENU_POR_ROL): para una sección nueva basta una entrada ahí.
 */
export default function Armazon({ pagina, children }) {
  const s = useSesion();
  const [modoTarea, setModoTarea] = useState(false);
  const [cajon, setCajon] = useState(false);
  const [pref, setPref] = useState(leerPreferencia);
  // Acordeón: solo el grupo de la página actual, el resto cerrado.
  const [manual, setManual] = useState(null); // grupo fijado a mano; false = todo cerrado
  const menu = menuPara(s.rol, s.tiene);
  const grupos = gruposPara(menu.lateral);
  const query = typeof window !== 'undefined' ? new URLSearchParams(window.location.search) : new URLSearchParams();
  // Si hay dos ítems de la misma página (p. ej. mantenimiento y reportar), gana el que coincide con la query.
  const activoId = (lista) => {
    const exactos = lista.filter((it) => esActivo(it, pagina, query));
    return (exactos.find((it) => it.query) || exactos[0])?.id;
  };
  const idLateral = activoId(menu.lateral);
  const idMovil = activoId([...menu.movil, ...menu.mas]);
  // Acordeón: abierto solo el grupo de la página actual. Al navegar se olvida el fijado manual.
  const grupoActivo = grupos.find((g) => g.items.some((i) => i.id === idLateral))?.id || grupos[0]?.id || null;
  const abiertoId = manual === false ? null : manual || grupoActivo;
  const alternarGrupo = (id) => {
    setManual(abiertoId === id ? false : id);
  };
  useEffect(() => {
    setManual(null);
  }, [idLateral]);
  // Limpieza de la preferencia anterior (lista de cerrados): ahora manda la página.
  useEffect(() => {
    try {
      window.localStorage.removeItem(CLAVE_GRUPOS);
    } catch {
      /* sin almacenamiento */
    }
  }, []);

  const alternarLateral = useCallback(() => {
    let ahoraAbierto;
    if (pref) ahoraAbierto = pref === 'abierto';
    else {
      try {
        ahoraAbierto = window.matchMedia('(min-width: 1280px)').matches;
      } catch {
        ahoraAbierto = true;
      }
    }
    const nueva = ahoraAbierto ? 'iconos' : 'abierto';
    guardarPreferencia(nueva);
    setPref(nueva);
  }, [pref]);

  // Clases según el estado del lateral (auto = por ancho de pantalla).
  const L = {
    auto: { ancho: 'lg:w-sidebar-iconos xl:w-sidebar', texto: 'hidden xl:inline', tip: 'xl:hidden', abierto: 'hidden xl:flex', iconos: 'flex xl:hidden' },
    abierto: { ancho: 'lg:w-sidebar', texto: 'inline', tip: 'hidden', abierto: 'flex', iconos: 'hidden' },
    iconos: { ancho: 'lg:w-sidebar-iconos', texto: 'hidden', tip: '', abierto: 'hidden', iconos: 'flex' },
  }[pref || 'auto'];

  const enlaceLateral = (it) => {
    const activo = it.id === idLateral;
    return (
      <Tooltip key={it.id} texto={it.etiqueta} lado="derecha" className={L.tip}>
        <a
          href={ruta(it.pagina, it.query)}
          aria-current={activo ? 'page' : undefined}
          aria-label={it.etiqueta}
          className={`flex h-10 w-full items-center gap-3 rounded-control px-3 text-sm transition-colors duration-rapida ${activo ? 'bg-acento font-semibold text-white hover:text-white' : 'text-texto-claro hover:bg-superficie-oscura hover:text-white'}`}
        >
          <Icono nombre={it.icono} tam={18} />
          <span className={`truncate ${L.texto}`}>{it.etiqueta}</span>
        </a>
      </Tooltip>
    );
  };

  return (
    <ArmazonCtx.Provider value={{ setModoTarea }}>
      <div className="min-h-screen bg-fondo text-tinta lg:flex">
        <a href="#contenido" className="sr-only focus:not-sr-only focus:absolute focus:left-2 focus:top-2 focus:z-[100] focus:rounded-control focus:bg-superficie focus:p-2">
          Saltar al contenido
        </a>

        {/* Escritorio: menú lateral en dos anchos */}
        <aside className={`hidden shrink-0 flex-col gap-5 bg-tinta px-3 py-4 transition-[width] duration-media lg:sticky lg:top-0 lg:flex lg:h-screen ${L.ancho}`}>
          <div className="flex items-center justify-between gap-2">
            <a href={ruta('inicio')} className={`${L.abierto} px-1`} aria-label="EDISYS, ir al inicio">
              <Logo claro tam={28} />
            </a>
            <a href={ruta('inicio')} className={`${L.iconos} mx-auto`} aria-label="EDISYS, ir al inicio">
              <Isotipo tam={28} />
            </a>
            <span className={L.abierto}>
              <BotonIcono etiqueta="Plegar el menú" icono="plegar" variante="oscuro" lado="derecha" onClick={alternarLateral} />
            </span>
          </div>
          <span className={`${L.iconos} justify-center`}>
            <BotonIcono etiqueta="Abrir el menú completo" icono="desplegar" variante="oscuro" lado="derecha" onClick={alternarLateral} />
          </span>
          <div className={L.abierto}>
            <div className="w-full">
              <SelectorEdificio edificios={s.edificios} actual={s.edificio.id} onCambio={s.cambiarEdificio} />
            </div>
          </div>
          <nav className="flex flex-1 flex-col gap-0.5 overflow-y-auto" aria-label="Menú principal">
            {/* Abierto: grupos plegables con los accesos comunes juntos */}
            <div className={`${L.abierto} flex-col gap-4`}>
              {grupos.map((g) => {
                const plegado = abiertoId !== g.id;
                return (
                  <section key={g.id} aria-label={g.etiqueta} className="flex flex-col gap-0.5">
                    <button
                      type="button"
                      onClick={() => alternarGrupo(g.id)}
                      aria-expanded={!plegado}
                      className="flex items-center justify-between px-3 pb-0.5 text-[11px] font-semibold uppercase tracking-wider text-texto-tenue transition-colors duration-rapida hover:text-white"
                    >
                      {g.etiqueta}
                      <Icono nombre={plegado ? 'abajo' : 'arriba'} tam={12} />
                    </button>
                    {!plegado && g.items.map((it) => enlaceLateral(it))}
                  </section>
                );
              })}
            </div>
            {/* Solo iconos: plano con separadores por grupo */}
            <div className={`${L.iconos} flex-col gap-0.5`}>
              {grupos.map((g, gi) => (
                <Fragment key={g.id}>
                  {gi > 0 && <div className="mx-3 my-1 border-t border-superficie-oscura" aria-hidden="true" />}
                  {g.items.map((it) => enlaceLateral(it))}
                </Fragment>
              ))}
            </div>
          </nav>
          <div className="flex flex-col gap-3">
            <div className={`${L.abierto} items-center gap-3 px-1`}>
              <Avatar iniciales={s.usuario.iniciales} oscuro />
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate text-sm font-semibold text-white">{s.usuario.nombre}</span>
                <span className="text-xs text-texto-tenue">{NOMBRE_ROL[s.rol]}</span>
              </span>
              <BotonIcono etiqueta="Cerrar sesión" icono="salir" variante="oscuro" lado="derecha" onClick={s.salir} />
            </div>
            <div className={`${L.iconos} flex-col items-center gap-2`}>
              <Tooltip texto={`${s.usuario.nombre} · ${NOMBRE_ROL[s.rol]}`} lado="derecha">
                <span tabIndex={0} aria-label={`${s.usuario.nombre}, ${NOMBRE_ROL[s.rol]}`} className="rounded-chip">
                  <Avatar iniciales={s.usuario.iniciales} oscuro />
                </span>
              </Tooltip>
              <BotonIcono etiqueta="Cerrar sesión" icono="salir" variante="oscuro" lado="derecha" onClick={s.salir} />
            </div>
          </div>
        </aside>

        <div className="flex min-w-0 flex-1 flex-col">
          {/* Tablet y móvil: cabecera con menú, logo, edificio y usuario */}
          {!modoTarea && (
            <header className="sticky top-0 z-30 flex h-14 items-center justify-between gap-2 border-b border-borde bg-superficie px-2 sm:px-4 lg:hidden">
              <div className="flex min-w-0 items-center gap-1">
                <BotonIcono etiqueta="Abrir el menú" icono="menu" lado="abajo" onClick={() => setCajon(true)} aria-expanded={cajon} />
                <a href={ruta('inicio')} className="flex min-w-0 items-center" aria-label="EDISYS, ir al inicio">
                  <Logo tam={26} />
                </a>
              </div>
              <button type="button" onClick={() => setCajon(true)} className="flex min-w-0 items-center gap-2 rounded-control px-2 py-1 transition-colors duration-rapida hover:bg-fondo" aria-label="Menú de usuario y edificio">
                <span className="hidden min-w-0 flex-col items-end min-[380px]:flex">
                  <span className="max-w-[150px] truncate text-sm font-semibold">{s.edificio.nombre}</span>
                  <span className="text-xs text-texto-apoyo">{NOMBRE_ROL[s.rol]}</span>
                </span>
                <Avatar iniciales={s.usuario.iniciales} />
              </button>
            </header>
          )}

          <main id="contenido" className={`flex min-w-0 flex-1 flex-col ${modoTarea === 'tarea' ? '' : 'pb-20 lg:pb-0'}`}>
            {children}
          </main>

          {/* Móvil y tablet: barra inferior según el rol + «Más» */}
          {modoTarea !== 'tarea' && (
            <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-borde bg-superficie/95 pb-[env(safe-area-inset-bottom)] backdrop-blur lg:hidden" aria-label="Navegación">
              <div className="mx-auto grid h-16 max-w-xl" style={{ gridTemplateColumns: `repeat(${menu.movil.length + 1}, minmax(0, 1fr))` }}>
                {menu.movil.map((it) => {
                  const activo = it.id === idMovil;
                  return (
                    <a
                      key={it.id}
                      href={ruta(it.pagina, it.query)}
                      aria-current={activo ? 'page' : undefined}
                      className={`flex flex-col items-center justify-center gap-1 text-xs transition-colors duration-rapida ${activo ? 'font-bold text-acento' : 'text-texto-suave hover:text-tinta'}`}
                    >
                      <Icono nombre={it.icono} tam={20} grosor={activo ? 2.25 : 1.75} />
                      <span className="relative max-w-full truncate px-1">
                        {it.corta}
                        {activo && <span className="absolute -bottom-1.5 left-1/2 h-0.5 w-5 -translate-x-1/2 rounded-chip bg-acento animate-escala-entrar" />}
                      </span>
                    </a>
                  );
                })}
                <button type="button" onClick={() => setCajon(true)} aria-expanded={cajon} className={`flex flex-col items-center justify-center gap-1 text-xs ${menu.mas.some((i) => i.id === idMovil) ? 'font-bold text-acento' : 'text-texto-suave'}`}>
                  <Icono nombre="mas" tam={20} />
                  Más
                </button>
              </div>
            </nav>
          )}
        </div>
      </div>

      {/* Cajón (tablet y móvil): todas las secciones, el edificio y la cuenta */}
      <Modal abierto={cajon} onCerrar={() => setCajon(false)} cajon etiqueta="Menú">
        <div className="flex h-full flex-col gap-4">
          <div className="flex items-center justify-between">
            <Logo tam={28} />
            <BotonIcono etiqueta="Cerrar el menú" icono="cerrar" variante="suave" lado="izquierda" onClick={() => setCajon(false)} />
          </div>
          <SelectorEdificio edificios={s.edificios} actual={s.edificio.id} onCambio={s.cambiarEdificio} oscuro={false} />
          <nav className="flex flex-col gap-3" aria-label="Secciones">
            {grupos.map((g) => (
              <section key={g.id} aria-label={g.etiqueta} className="flex flex-col gap-0.5">
                <p className="px-3 text-[11px] font-semibold uppercase tracking-wider text-texto-apoyo">{g.etiqueta}</p>
                {g.items.map((it) => {
                  const activo = it.id === idLateral || it.id === idMovil;
                  return (
                    <a
                      key={it.id}
                      href={ruta(it.pagina, it.query)}
                      aria-current={activo ? 'page' : undefined}
                      className={`flex min-h-[44px] items-center gap-3 rounded-control px-3 text-base transition-colors duration-rapida ${activo ? 'bg-acento-suave font-semibold text-acento' : 'text-tinta hover:bg-fondo hover:text-tinta'}`}
                    >
                      <Icono nombre={it.icono} tam={20} />
                      {it.etiqueta}
                    </a>
                  );
                })}
              </section>
            ))}
          </nav>
          <div className="mt-auto flex flex-col gap-2 border-t border-borde pt-4">
            <div className="flex items-center gap-3">
              <Avatar iniciales={s.usuario.iniciales} />
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate font-semibold">{s.usuario.nombre}</span>
                <span className="truncate text-sm text-texto-apoyo">{s.usuario.correo || NOMBRE_ROL[s.rol]}</span>
              </span>
            </div>
            <button type="button" onClick={s.salir} className="flex min-h-[44px] items-center gap-3 rounded-control px-3 text-base text-alerta transition-colors duration-rapida hover:bg-alerta-suave">
              <Icono nombre="salir" tam={20} /> Cerrar sesión
            </button>
          </div>
        </div>
      </Modal>
    </ArmazonCtx.Provider>
  );
}

function Avatar({ iniciales, oscuro = false }) {
  return (
    <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-chip text-sm font-semibold text-white ${oscuro ? 'bg-superficie-oscura-2' : 'bg-tinta'}`} aria-hidden="true">
      {iniciales}
    </span>
  );
}
