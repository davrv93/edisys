import { createContext, useContext, useEffect, useState } from 'react';
import { useSesion } from './Sesion.jsx';
import { menuPara, NOMBRE_ROL, ROLES } from '../lib/permisos.js';
import { ruta } from '../lib/nav.jsx';
import { Icono, Logo, Modal, SelectorEdificio } from '../ui/index.js';

const ArmazonCtx = createContext({ setModoTarea: () => {} });

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
 * Armazón responsivo común a todas las páginas Astro.
 * Escritorio: menú lateral slate-900 de 248 px. Móvil: cabecera + barra inferior de pestañas según el rol.
 */
export default function Armazon({ pagina, children }) {
  const s = useSesion();
  const [modoTarea, setModoTarea] = useState(false);
  const [masAbierto, setMasAbierto] = useState(false);
  const menu = menuPara(s.rol, s.tiene);
  const query = typeof window !== 'undefined' ? new URLSearchParams(window.location.search) : new URLSearchParams();
  // Si hay dos ítems de la misma página (p. ej. mantenimiento y reportar), gana el que coincide con la query.
  const activoId = (lista) => {
    const exactos = lista.filter((it) => esActivo(it, pagina, query));
    return (exactos.find((it) => it.query) || exactos[0])?.id;
  };
  const idLateral = activoId(menu.lateral);
  const idMovil = activoId([...menu.movil, ...menu.mas]);

  return (
    <ArmazonCtx.Provider value={{ setModoTarea }}>
      <div className="min-h-screen bg-fondo text-tinta lg:flex">
        <a href="#contenido" className="sr-only focus:not-sr-only focus:absolute focus:left-2 focus:top-2 focus:z-[100] focus:rounded focus:bg-superficie focus:p-2">
          Saltar al contenido
        </a>
        {/* Escritorio: menú lateral */}
        <aside className="hidden w-sidebar shrink-0 flex-col gap-6 bg-tinta px-4 py-6 lg:sticky lg:top-0 lg:flex lg:h-screen lg:overflow-y-auto">
          <a href={ruta('inicio')} className="px-2" aria-label="EDISYS, ir al inicio">
            <Logo claro />
          </a>
          <SelectorEdificio edificios={s.edificios} actual={s.edificio.id} onCambio={s.cambiarEdificio} />
          <nav className="flex flex-col gap-1" aria-label="Menú principal">
            {menu.lateral.map((it) => {
              const activo = it.id === idLateral;
              return (
                <a
                  key={it.id}
                  href={ruta(it.pagina, it.query)}
                  aria-current={activo ? 'page' : undefined}
                  className={`flex h-10 items-center gap-3 rounded-lg px-3 text-sm ${activo ? 'bg-acento font-semibold text-white hover:text-white' : 'text-texto-claro hover:bg-superficie-oscura hover:text-white'}`}
                >
                  <Icono nombre={it.icono} tam={18} />
                  {it.etiqueta}
                </a>
              );
            })}
          </nav>
          <div className="mt-auto flex flex-col gap-3">
            {s.mock && <CambiarRolMock />}
            <div className="flex items-center gap-3 px-2">
              <span className="flex h-9 w-9 items-center justify-center rounded-full bg-superficie-oscura-2 text-sm font-semibold text-white">{s.usuario.iniciales}</span>
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate text-sm font-semibold text-white">{s.usuario.nombre}</span>
                <span className="text-xs text-texto-tenue">{NOMBRE_ROL[s.rol]}</span>
              </span>
              <button type="button" onClick={s.salir} className="flex h-9 w-9 items-center justify-center rounded-lg text-texto-claro hover:bg-superficie-oscura hover:text-white" aria-label="Cerrar sesión" title="Cerrar sesión">
                <Icono nombre="salir" tam={18} />
              </button>
            </div>
          </div>
        </aside>

        <div className="flex min-w-0 flex-1 flex-col">
          {/* Móvil: cabecera con el edificio y el usuario */}
          {!modoTarea && (
            <header className="sticky top-0 z-30 flex h-14 items-center justify-between gap-3 border-b border-borde bg-superficie px-4 lg:hidden">
              <a href={ruta('inicio')} className="flex min-w-0 items-center gap-2" aria-label="Inicio">
                <Logo tam={28} />
              </a>
              <button type="button" onClick={() => setMasAbierto(true)} className="flex min-w-0 items-center gap-2 rounded-lg px-2 py-1 hover:bg-fondo" aria-label="Menú de usuario y edificio">
                <span className="flex min-w-0 flex-col items-end">
                  <span className="max-w-[160px] truncate text-sm font-semibold">{s.edificio.nombre}</span>
                  <span className="text-xs text-texto-apoyo">{NOMBRE_ROL[s.rol]}</span>
                </span>
                <span className="flex h-9 w-9 items-center justify-center rounded-full bg-tinta text-sm font-semibold text-white">{s.usuario.iniciales}</span>
              </button>
            </header>
          )}

          <main id="contenido" className={`flex min-w-0 flex-1 flex-col ${modoTarea === 'tarea' ? '' : 'pb-20 lg:pb-0'}`}>
            {children}
          </main>

          {/* Móvil: barra inferior según el rol */}
          {modoTarea !== 'tarea' && (
            <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-borde bg-superficie pb-[env(safe-area-inset-bottom)] lg:hidden" aria-label="Navegación">
              <div className="grid h-16" style={{ gridTemplateColumns: `repeat(${menu.movil.length + (menu.mas.length ? 1 : 0)}, minmax(0, 1fr))` }}>
                {menu.movil.map((it) => {
                  const activo = it.id === idMovil;
                  return (
                    <a
                      key={it.id}
                      href={ruta(it.pagina, it.query)}
                      aria-current={activo ? 'page' : undefined}
                      className={`flex flex-col items-center justify-center gap-1 text-xs ${activo ? 'font-bold text-acento' : 'text-texto-suave hover:text-tinta'}`}
                    >
                      <Icono nombre={it.icono} tam={20} />
                      <span className="relative">
                        {it.corta}
                        {activo && <span className="absolute -bottom-1.5 left-1/2 h-0.5 w-5 -translate-x-1/2 rounded-full bg-acento" />}
                      </span>
                    </a>
                  );
                })}
                {menu.mas.length > 0 && (
                  <button type="button" onClick={() => setMasAbierto(true)} className={`flex flex-col items-center justify-center gap-1 text-xs ${menu.mas.some((i) => i.id === idMovil) ? 'font-bold text-acento' : 'text-texto-suave'}`}>
                    <Icono nombre="menu" tam={20} />
                    Más
                  </button>
                )}
              </div>
            </nav>
          )}
        </div>
      </div>

      <Modal abierto={masAbierto} onCerrar={() => setMasAbierto(false)} titulo="Más opciones">
        <div className="flex flex-col gap-4">
          <SelectorEdificio edificios={s.edificios} actual={s.edificio.id} onCambio={s.cambiarEdificio} oscuro={false} />
          {menu.mas.length > 0 && (
            <nav className="flex flex-col" aria-label="Más secciones">
              {menu.mas.map((it) => (
                <a
                  key={it.id}
                  href={ruta(it.pagina, it.query)}
                  className={`flex min-h-[48px] items-center gap-3 rounded-lg px-3 text-base ${it.id === idMovil ? 'bg-acento-suave font-semibold text-acento' : 'text-tinta hover:bg-fondo'}`}
                >
                  <Icono nombre={it.icono} tam={20} />
                  {it.etiqueta}
                </a>
              ))}
            </nav>
          )}
          {s.mock && <CambiarRolMock claro />}
          <div className="flex items-center gap-3 border-t border-borde pt-4">
            <span className="flex h-10 w-10 items-center justify-center rounded-full bg-tinta text-sm font-semibold text-white">{s.usuario.iniciales}</span>
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="truncate font-semibold">{s.usuario.nombre}</span>
              <span className="text-sm text-texto-apoyo">{s.usuario.correo || NOMBRE_ROL[s.rol]}</span>
            </span>
          </div>
          <button type="button" onClick={s.salir} className="flex min-h-[48px] items-center gap-3 rounded-lg px-3 text-base text-alerta hover:bg-alerta-suave">
            <Icono nombre="salir" tam={20} /> Cerrar sesión
          </button>
        </div>
      </Modal>
    </ArmazonCtx.Provider>
  );
}

/** Solo con VITE_MOCK=1: cambia el rol de la sesión de prueba para revisar cada menú. */
function CambiarRolMock({ claro = false }) {
  const s = useSesion();
  const cambiar = (rol) => {
    try {
      window.localStorage.setItem('edisys.mock.rol', rol);
    } catch {
      /* sin almacenamiento */
    }
    window.location.assign(ruta('inicio'));
  };
  return (
    <label className={`flex flex-col gap-1 rounded-lg border border-dashed p-2 text-xs ${claro ? 'border-aviso-borde bg-aviso-suave text-aviso-texto' : 'border-superficie-oscura-2 text-texto-tenue'}`}>
      Modo demostración (mock): ver como
      <select className="h-9 rounded bg-superficie px-2 text-sm text-tinta" value={s.rol} onChange={(e) => cambiar(e.target.value)}>
        {ROLES.filter((r) => r !== 'superadmin').map((r) => (
          <option key={r} value={r}>
            {NOMBRE_ROL[r]}
          </option>
        ))}
      </select>
    </label>
  );
}
