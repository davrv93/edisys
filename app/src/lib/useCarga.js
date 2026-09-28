import { useCallback, useEffect, useRef, useState } from 'react';

/**
 * Carga de datos con estados: { datos, error, cargando, recargar, setDatos }.
 * Ignora respuestas viejas si las dependencias cambian antes de que lleguen.
 */
export function useCarga(fn, deps = [], { activo = true } = {}) {
  const [estado, setEstado] = useState({ datos: null, error: null, cargando: activo });
  const turno = useRef(0);
  const fnRef = useRef(fn);
  fnRef.current = fn;

  const recargar = useCallback(async () => {
    const mio = ++turno.current;
    setEstado((e) => ({ ...e, cargando: true, error: null }));
    try {
      const datos = await fnRef.current();
      if (mio === turno.current) setEstado({ datos, error: null, cargando: false });
      return datos;
    } catch (error) {
      if (mio === turno.current) setEstado((e) => ({ ...e, error, cargando: false }));
      return null;
    }
  }, []);

  useEffect(() => {
    if (activo) recargar();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activo, ...deps]);

  const setDatos = useCallback((f) => setEstado((e) => ({ ...e, datos: typeof f === 'function' ? f(e.datos) : f })), []);
  return { ...estado, recargar, setDatos };
}
