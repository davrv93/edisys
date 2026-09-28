import Boton from './Boton.jsx';
import Icono from './Icono.jsx';
import { Esqueleto, ErrorCarga, Vacio } from './EstadosPantalla.jsx';

/**
 * Tabla con orden, paginación (en el servidor), fila seleccionable y los tres estados.
 * En móvil (< 640 px) se convierte en lista de tarjetas, nunca en scroll horizontal.
 *
 * columnas: [{ clave, titulo, render?(fila), alinear?: 'izq'|'der', ordenable?, ancho?, movil?: 'titulo'|'sub'|'valor'|'dato'|'oculto', className? }]
 * paginacion: { pagina, porPagina, total, onPagina }
 */
export default function Tabla({
  columnas, filas, claveFila = 'id', onFila, seleccionada, cargando, error, onReintentar,
  vacio, orden, onOrden, paginacion, etiqueta, densa = false, claseFila,
}) {
  const valor = (c, f) => (c.render ? c.render(f) : f[c.clave]);
  const pad = densa ? 'px-4 py-2.5' : 'px-4 py-3.5';

  if (error) return <ErrorCarga error={error} onReintentar={onReintentar} compacto />;
  if (cargando && !filas?.length) {
    return (
      <div className="flex flex-col gap-3 p-4" aria-busy="true">
        {Array.from({ length: 5 }, (_, i) => (
          <Esqueleto key={i} className="h-10 w-full" />
        ))}
      </div>
    );
  }
  if (!filas?.length) return vacio || <Vacio titulo="Sin resultados" compacto />;

  const titulo = columnas.find((c) => c.movil === 'titulo') || columnas[0];
  const sub = columnas.find((c) => c.movil === 'sub');
  const val = columnas.find((c) => c.movil === 'valor');
  const datos = columnas.filter((c) => c !== titulo && c !== sub && c !== val && c.movil !== 'oculto');

  return (
    <div className={cargando ? 'opacity-60 transition-opacity' : ''}>
      {/* Escritorio y tableta */}
      <table className="hidden w-full border-collapse text-sm sm:table" aria-label={etiqueta}>
        <thead>
          <tr className="bg-fondo text-left text-xs text-texto-apoyo">
            {columnas.map((c) => {
              const activo = orden?.clave === c.clave;
              return (
                <th
                  key={c.clave}
                  scope="col"
                  style={c.ancho ? { width: c.ancho } : undefined}
                  className={`${pad} py-3 font-semibold ${c.alinear === 'der' ? 'text-right' : ''}`}
                  aria-sort={activo ? (orden.dir === 'asc' ? 'ascending' : 'descending') : undefined}
                >
                  {c.ordenable && onOrden ? (
                    <button
                      type="button"
                      className="inline-flex items-center gap-1 hover:text-tinta"
                      onClick={() => onOrden({ clave: c.clave, dir: activo && orden.dir === 'asc' ? 'desc' : 'asc' })}
                    >
                      {c.titulo}
                      <Icono nombre={activo && orden.dir === 'desc' ? 'abajo' : 'arriba'} tam={12} className={activo ? '' : 'opacity-30'} />
                    </button>
                  ) : (
                    c.titulo
                  )}
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody>
          {filas.map((f) => {
            const id = f[claveFila];
            const sel = seleccionada != null && String(seleccionada) === String(id);
            return (
              <tr
                key={id}
                onClick={onFila ? () => onFila(f) : undefined}
                onKeyDown={onFila ? (e) => (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), onFila(f)) : undefined}
                tabIndex={onFila ? 0 : undefined}
                aria-selected={onFila ? sel : undefined}
                className={`border-t border-superficie-2 ${onFila ? 'cursor-pointer hover:bg-fondo focus:outline-none focus-visible:bg-acento-suave' : ''} ${sel ? 'bg-acento-suave' : ''} ${claseFila ? claseFila(f) : ''}`}
              >
                {columnas.map((c, i) => (
                  <td key={c.clave} className={`${pad} ${c.alinear === 'der' ? 'text-right tabular-nums' : ''} ${i === 0 ? 'font-semibold' : ''} ${c.className || ''} ${sel && i === 0 ? 'text-acento' : ''}`}>
                    {valor(c, f)}
                  </td>
                ))}
              </tr>
            );
          })}
        </tbody>
      </table>

      {/* Móvil: tarjetas */}
      <ul className="flex flex-col gap-2 p-3 sm:hidden" aria-label={etiqueta}>
        {filas.map((f) => {
          const id = f[claveFila];
          const sel = seleccionada != null && String(seleccionada) === String(id);
          const Tag = onFila ? 'button' : 'div';
          return (
            <li key={id}>
              <Tag
                type={onFila ? 'button' : undefined}
                onClick={onFila ? () => onFila(f) : undefined}
                className={`flex w-full flex-col gap-2 rounded-xl border bg-superficie p-4 text-left ${sel ? 'border-acento bg-acento-suave' : 'border-borde'} ${claseFila ? claseFila(f) : ''}`}
              >
                <div className="flex w-full items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="text-base font-semibold text-tinta">{valor(titulo, f)}</div>
                    {sub && <div className="text-sm text-texto-suave">{valor(sub, f)}</div>}
                  </div>
                  {val && <div className="shrink-0 text-base font-semibold tabular-nums">{valor(val, f)}</div>}
                </div>
                {datos.length > 0 && (
                  <dl className="grid w-full grid-cols-2 gap-x-3 gap-y-1 text-sm">
                    {datos.map((c) => (
                      <div key={c.clave} className="contents">
                        <dt className="text-texto-apoyo">{c.titulo}</dt>
                        <dd className={`text-right ${c.alinear === 'der' ? 'tabular-nums' : ''}`}>{valor(c, f)}</dd>
                      </div>
                    ))}
                  </dl>
                )}
              </Tag>
            </li>
          );
        })}
      </ul>

      {paginacion && <Paginacion {...paginacion} mostradas={filas.length} />}
    </div>
  );
}

export function Paginacion({ pagina = 1, porPagina = 25, total = 0, onPagina, mostradas, unidad = 'registros' }) {
  const paginas = Math.max(1, Math.ceil(total / porPagina));
  const desde = total ? (pagina - 1) * porPagina + 1 : 0;
  const hasta = Math.min(total, desde + (mostradas ?? porPagina) - 1);
  return (
    <div className="flex items-center justify-between gap-3 border-t border-borde px-4 py-3 text-sm text-texto-apoyo">
      <span>
        {desde}–{hasta} de {total} {unidad}
      </span>
      <span className="flex gap-2">
        <Boton variante="secundario" tamano="sm" disabled={pagina <= 1} onClick={() => onPagina(pagina - 1)}>
          Anterior
        </Boton>
        <Boton variante="secundario" tamano="sm" disabled={pagina >= paginas} onClick={() => onPagina(pagina + 1)}>
          Siguiente
        </Boton>
      </span>
    </div>
  );
}
