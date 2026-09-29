import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { api, irAlLogin, MOCK } from '../lib/api.js';
import { normalizarYo, tienePermiso } from '../lib/sesion.js';
import { CargandoApp, ErrorCarga, SinPermiso } from '../ui/index.js';

const Ctx = createContext(null);
const CLAVE_EDIFICIO = 'edisys.edificio';

function leerLocal(k) {
  try {
    return window.localStorage.getItem(k);
  } catch {
    return null;
  }
}
function escribirLocal(k, v) {
  try {
    window.localStorage.setItem(k, v);
  } catch {
    /* modo privado: no pasa nada */
  }
}

/** Carga GET /yo al arrancar. Si 401, el cliente redirige a /login?next=… */
export function SesionProvider({ children }) {
  const [crudo, setCrudo] = useState(null);
  const [error, setError] = useState(null);
  const [eid, setEid] = useState(() => leerLocal(CLAVE_EDIFICIO));

  const cargar = useCallback(async () => {
    setError(null);
    try {
      // El API acepta ?edificio_id= para devolver permisos y unidades de ese edificio.
      setCrudo(await api.get('/yo', { edificio_id: eid || undefined }));
    } catch (e) {
      if (e.status !== 401) setError(e);
    }
  }, [eid]);

  useEffect(() => {
    cargar();
  }, [cargar]);
  // cargar depende de eid: cambiar de edificio vuelve a pedir /yo con sus permisos.

  const sesion = useMemo(() => (crudo ? normalizarYo(crudo, eid) : null), [crudo, eid]);

  const valor = useMemo(() => {
    if (!sesion) return null;
    return {
      ...sesion,
      mock: MOCK,
      tiene: (p) => tienePermiso(sesion, p),
      cambiarEdificio: (id) => {
        escribirLocal(CLAVE_EDIFICIO, String(id));
        setEid(String(id));
      },
      recargar: cargar,
      salir: async () => {
        try {
          await api.post('/auth/logout', {}, { sinRedirigir: true });
        } catch {
          /* aunque falle, se sale */
        }
        window.location.assign(MOCK ? '/app/' : '/login/');
      },
    };
  }, [sesion, cargar]);

  if (error) {
    return (
      <div className="min-h-screen bg-fondo">
        <ErrorCarga error={error} onReintentar={cargar} />
      </div>
    );
  }
  if (!valor) return <CargandoApp />;
  if (!valor.edificio) {
    return (
      <div className="min-h-screen bg-fondo">
        <SinPermiso permiso="acceso a un edificio" />
        <div className="text-center">
          <button type="button" className="text-acento underline" onClick={irAlLogin}>
            Entrar con otra cuenta
          </button>
        </div>
      </div>
    );
  }
  return <Ctx.Provider value={valor}>{children}</Ctx.Provider>;
}

export function useSesion() {
  const c = useContext(Ctx);
  if (!c) throw new Error('useSesion fuera de SesionProvider');
  return c;
}

/** Esconde botones sin el permiso. El API valida igual (§0.3). */
export function Guarda({ permiso, children, sino = null }) {
  const s = useSesion();
  return s.tiene(permiso) ? children : sino;
}

/** Pantalla protegida: sin permiso → SinPermiso, nunca pantalla en blanco. */
export function RutaProtegida({ permiso, children }) {
  const s = useSesion();
  if (!s.tiene(permiso)) return <SinPermiso permiso={Array.isArray(permiso) ? permiso.join(' o ') : permiso} />;
  return children;
}

/** Id del edificio activo (va en las rutas del API: /edificios/{eid}/…). */
export function useEid() {
  return useSesion().edificio.id;
}
