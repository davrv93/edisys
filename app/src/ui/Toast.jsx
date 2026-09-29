import { createContext, useCallback, useContext, useMemo, useRef, useState } from 'react';
import Icono from './Icono.jsx';

const Ctx = createContext(null);

const TONOS = {
  info: 'bg-tinta text-white',
  exito: 'bg-acento text-white',
  aviso: 'bg-aviso-suave text-aviso-texto border border-aviso-borde',
  error: 'bg-alerta-suave text-alerta-texto border border-alerta-borde',
};
const ICONOS = { info: 'info', exito: 'check', aviso: 'alerta', error: 'alerta' };

/** Avisos que no piden respuesta. Se van solos a los 4 s; los errores se quedan hasta cerrarlos. */
export function ToastProvider({ children }) {
  const [lista, setLista] = useState([]);
  const n = useRef(0);
  const quitar = useCallback((id) => setLista((l) => l.filter((t) => t.id !== id)), []);
  const toast = useCallback(
    (mensaje, op = {}) => {
      const id = ++n.current;
      const tipo = op.tipo || 'info';
      setLista((l) => [...l.slice(-3), { id, mensaje, tipo, accion: op.accion }]);
      if (tipo !== 'error' && !op.fijo) setTimeout(() => quitar(id), op.duracion || 4000);
      return id;
    },
    [quitar],
  );
  const valor = useMemo(() => ({ toast, quitar }), [toast, quitar]);
  return (
    <Ctx.Provider value={valor}>
      {children}
      <div className="pointer-events-none fixed inset-x-0 bottom-20 z-[90] flex flex-col items-center gap-2 px-4 lg:bottom-6 lg:items-end lg:pr-6" aria-live="polite">
        {lista.map((t) => (
          <div key={t.id} role={t.tipo === 'error' ? 'alert' : 'status'} className={`pointer-events-auto flex w-full max-w-sm items-start gap-3 rounded-tarjeta px-4 py-3 text-sm shadow-flotante ${TONOS[t.tipo]}`}>
            <Icono nombre={ICONOS[t.tipo]} tam={18} className="mt-0.5" />
            <span className="flex-1">{t.mensaje}</span>
            {t.accion && (
              <button type="button" className="font-semibold underline" onClick={() => { t.accion.onClick(); quitar(t.id); }}>
                {t.accion.texto}
              </button>
            )}
            <button type="button" aria-label="Cerrar aviso" onClick={() => quitar(t.id)} className="-m-1 p-1 opacity-80 hover:opacity-100">
              <Icono nombre="cerrar" tam={16} />
            </button>
          </div>
        ))}
      </div>
    </Ctx.Provider>
  );
}

export function useToast() {
  const c = useContext(Ctx);
  if (!c) throw new Error('useToast fuera de ToastProvider');
  return c;
}
