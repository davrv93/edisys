import Icono from './Icono.jsx';
import { Spinner } from './Boton.jsx';
import { formatearSoles, formatearPct } from '../lib/dinero.js';

/**
 * Fila de árbol del balance: nombre, documentos, total, % del padre y flecha.
 * Accesible por teclado: → abre, ← cierra, ↑/↓ mueven el foco, Enter abre o muestra el documento.
 * Los hijos los carga el padre (perezoso) al abrir; el error se muestra EN LA FILA.
 */
export default function NodoDesplegable({
  nodo, nivel = 0, abierto = false, cargando = false, error, onAlternar, onReintentar, onDocumento,
  seleccionado = false, raiz = false,
}) {
  const esDoc = nodo.tipo === 'documento';
  const desplegable = !esDoc && nodo.tiene_hijos;

  const onKeyDown = (e) => {
    const arbol = e.currentTarget.closest('[role="tree"]');
    const items = arbol ? [...arbol.querySelectorAll('[role="treeitem"]')] : [];
    const i = items.indexOf(e.currentTarget);
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      items[i + 1]?.focus();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      items[i - 1]?.focus();
    } else if (e.key === 'ArrowRight' && desplegable && !abierto) {
      e.preventDefault();
      onAlternar?.();
    } else if (e.key === 'ArrowLeft' && desplegable && abierto) {
      e.preventDefault();
      onAlternar?.();
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      if (esDoc) onDocumento?.(nodo);
      else if (desplegable) onAlternar?.();
    }
  };

  const onClick = () => {
    if (esDoc) onDocumento?.(nodo);
    else if (desplegable) onAlternar?.();
  };

  const sangria = { paddingLeft: `${12 + Math.min(nivel, 5) * 20}px` };
  const tipoDoc = (nodo.formato || nodo.tipo_documento || 'pdf').toUpperCase().slice(0, 3);

  return (
    <div
      role="treeitem"
      aria-level={nivel + 1}
      aria-expanded={desplegable ? abierto : undefined}
      aria-selected={seleccionado}
      tabIndex={nivel === 0 ? 0 : -1}
      onKeyDown={onKeyDown}
      onClick={onClick}
      className={[
        'grid cursor-pointer grid-cols-[1fr_auto] items-center gap-x-3 gap-y-1 border-t border-superficie-2 py-3 pr-4 text-sm',
        'sm:grid-cols-[1fr_96px_150px_72px]',
        'focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-acento',
        raiz ? 'bg-acento-suave' : seleccionado ? 'bg-acento-suave/60' : 'hover:bg-fondo',
      ].join(' ')}
      style={sangria}
    >
      <span className={`flex min-w-0 items-center gap-2 ${raiz ? 'font-titulo text-lg sm:text-xl font-semibold' : nivel === 1 ? 'text-base font-semibold' : ''} ${esDoc && seleccionado ? 'text-acento font-semibold' : ''}`}>
        {esDoc ? (
          <span className={`rounded px-1.5 py-0.5 text-[11px] font-bold ${seleccionado ? 'bg-acento text-white' : 'bg-borde text-egreso'}`}>{tipoDoc}</span>
        ) : desplegable ? (
          <span className="flex h-5 w-5 items-center justify-center text-texto-apoyo">
            {cargando ? <Spinner className="h-3.5 w-3.5" /> : <Icono nombre={abierto ? 'abajo' : 'der'} tam={16} />}
          </span>
        ) : (
          <span className="h-5 w-5" />
        )}
        <span className="min-w-0 truncate">{nodo.nombre}</span>
        {nodo.sin_sustento && (
          <span className="shrink-0 rounded-full border border-aviso-borde bg-aviso-suave px-2 py-0.5 text-[11px] font-semibold text-aviso">sin sustento</span>
        )}
      </span>
      <span className="hidden text-texto-suave tabular-nums sm:block">{esDoc ? '—' : nodo.documentos ?? ''}</span>
      <span className={`text-right tabular-nums ${raiz ? 'text-lg font-semibold text-acento' : nivel === 1 ? 'text-base font-semibold' : ''} ${nodo.es_sustento ? 'text-texto-apoyo' : ''}`}>
        {nodo.es_sustento ? 'sustento' : formatearSoles(nodo.total_cts)}
      </span>
      <span className="hidden text-right text-texto-suave tabular-nums sm:block">{nodo.pct_padre != null && !esDoc ? formatearPct(nodo.pct_padre) : ''}</span>
      {error && (
        <span className="col-span-full flex items-center gap-2 text-sm text-alerta" role="alert">
          <Icono nombre="alerta" tam={14} /> No se pudo abrir este nodo.
          <button
            type="button"
            className="font-semibold underline"
            onClick={(e) => {
              e.stopPropagation();
              onReintentar?.();
            }}
          >
            Reintentar
          </button>
        </span>
      )}
    </div>
  );
}
