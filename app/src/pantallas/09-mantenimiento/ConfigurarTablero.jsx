import { useState } from 'react';
import { COLUMNAS, CAMPOS_TARJETA } from '../../lib/kanban.js';
import { Boton, Icono, Modal, useToast } from '../../ui/index.js';

const etiquetaDe = (estado) => COLUMNAS.find((c) => c.estado === estado)?.etiqueta || estado;

/** Configurar tablero: etapas visibles en orden + campos de la tarjeta. El grafo no se toca. */
export default function ConfigurarTablero({ columnas, tarjeta, onGuardar, onCerrar }) {
  const { toast } = useToast();
  const [lista, setLista] = useState(columnas.length ? columnas : COLUMNAS.map((c) => c.estado));
  const [campos, setCampos] = useState({ monto: true, responsable: true, fotos: true, votos: true, antiguedad: true, ...tarjeta });
  const [guardando, setGuardando] = useState(false);

  const alternar = (estado) => {
    setLista((l) => {
      if (l.includes(estado)) {
        if (l.length === 1) {
          toast('El tablero necesita al menos una etapa.', { tipo: 'aviso' });
          return l;
        }
        return l.filter((e) => e !== estado);
      }
      const orden = COLUMNAS.map((c) => c.estado);
      return [...l, estado].sort((a, b) => orden.indexOf(a) - orden.indexOf(b));
    });
  };
  const mover = (i, d) => {
    setLista((l) => {
      const j = i + d;
      if (j < 0 || j >= l.length) return l;
      const n = [...l];
      [n[i], n[j]] = [n[j], n[i]];
      return n;
    });
  };
  const guardar = async () => {
    setGuardando(true);
    try {
      await onGuardar({ columnas: lista, tarjeta: campos });
      toast('Tablero configurado.', { tipo: 'exito' });
    } catch (err) {
      toast(err.message, { tipo: 'error' });
    } finally {
      setGuardando(false);
    }
  };
  const restablecer = async () => {
    setGuardando(true);
    try {
      await onGuardar({ columnas: COLUMNAS.map((c) => c.estado), tarjeta: { monto: true, responsable: true, fotos: true, votos: true, antiguedad: true } });
      toast('Tablero restablecido.', { tipo: 'exito' });
    } catch (err) {
      toast(err.message, { tipo: 'error' });
    } finally {
      setGuardando(false);
    }
  };

  return (
    <Modal abierto onCerrar={onCerrar} titulo="Configurar tablero"
      pie={<>
        <Boton variante="fantasma" onClick={restablecer} disabled={guardando}>Restablecer</Boton>
        <Boton onClick={guardar} cargando={guardando}>Guardar</Boton>
      </>}>
      <div className="flex flex-col gap-5">
        <section className="flex flex-col gap-2">
          <h3 className="text-sm font-semibold">Etapas visibles, en orden</h3>
          <ul className="flex flex-col gap-1">
            {lista.map((estado, i) => (
              <li key={estado} className="flex items-center gap-2 rounded-control border border-borde px-3 py-2">
                <span className="min-w-0 flex-1 truncate text-sm font-semibold">{etiquetaDe(estado)}</span>
                <button type="button" onClick={() => mover(i, -1)} disabled={i === 0} aria-label={`Subir ${etiquetaDe(estado)}`} className="rounded p-1 text-texto-apoyo hover:text-tinta disabled:opacity-30">
                  <Icono nombre="arriba" tam={16} />
                </button>
                <button type="button" onClick={() => mover(i, 1)} disabled={i === lista.length - 1} aria-label={`Bajar ${etiquetaDe(estado)}`} className="rounded p-1 text-texto-apoyo hover:text-tinta disabled:opacity-30">
                  <Icono nombre="abajo" tam={16} />
                </button>
                <button type="button" onClick={() => alternar(estado)} aria-label={`Ocultar ${etiquetaDe(estado)}`} className="rounded p-1 text-texto-apoyo hover:text-alerta">
                  <Icono nombre="cerrar" tam={16} />
                </button>
              </li>
            ))}
          </ul>
          <div className="flex flex-wrap gap-1.5" aria-label="Etapas ocultas">
            {COLUMNAS.filter((c) => !lista.includes(c.estado)).map((c) => (
              <button key={c.estado} type="button" onClick={() => alternar(c.estado)} className="rounded-chip border border-dashed border-borde-fuerte px-2.5 py-1 text-xs text-texto-apoyo hover:text-tinta">
                + {c.etiqueta}
              </button>
            ))}
          </div>
          <p className="text-xs text-texto-apoyo">Lo que se oculta no se borra: sigue en filtros, exportar y plan. Las transiciones no cambian.</p>
        </section>
        <section className="flex flex-col gap-2">
          <h3 className="text-sm font-semibold">Campos de la tarjeta</h3>
          {CAMPOS_TARJETA.map((c) => (
            <label key={c.valor} className="flex min-h-[44px] cursor-pointer items-center gap-3 rounded-control border border-borde px-3">
              <input type="checkbox" checked={!!campos[c.valor]} onChange={() => setCampos((x) => ({ ...x, [c.valor]: !x[c.valor] }))} className="h-5 w-5 accent-[var(--color-acento)]" />
              <span className="text-sm">{c.etiqueta}</span>
            </label>
          ))}
        </section>
      </div>
    </Modal>
  );
}
