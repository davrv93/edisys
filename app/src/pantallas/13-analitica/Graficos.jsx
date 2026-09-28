import { useState } from 'react';

// Gráficos SVG propios: marcas finas, extremos redondeados de 4 px anclados a la base,
// separación de 2 px, rejilla discreta, un solo eje, tooltip al pasar y tabla de datos.

function Tooltip({ t }) {
  if (!t) return null;
  return (
    <div className="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full rounded-lg bg-tinta px-3 py-2 text-xs text-white shadow-lg" style={{ left: t.x, top: t.y - 8 }} role="status">
      {t.lineas.map((l, i) => (
        <div key={i} className={i === 0 ? 'font-semibold' : ''}>
          {l}
        </div>
      ))}
    </div>
  );
}

function useTooltip() {
  const [t, setT] = useState(null);
  const mostrar = (e, lineas) => {
    const caja = e.currentTarget.closest('[data-grafico]').getBoundingClientRect();
    const p = e.touches?.[0] || e;
    setT({ x: p.clientX - caja.left, y: p.clientY - caja.top, lineas });
  };
  return [t, mostrar, () => setT(null)];
}

/** Rejilla y eje Y con 4 marcas «bonitas». */
function escala(max) {
  if (max <= 0) return { tope: 1, marcas: [0, 1] };
  const bruto = max / 4;
  const pot = 10 ** Math.floor(Math.log10(bruto));
  const paso = [1, 2, 2.5, 5, 10].map((m) => m * pot).find((p) => p >= bruto);
  const tope = paso * Math.ceil(max / paso);
  return { tope, marcas: Array.from({ length: Math.round(tope / paso) + 1 }, (_, i) => i * paso) };
}

export function Leyenda({ items }) {
  return (
    <div className="flex flex-wrap gap-4 text-xs text-texto-suave">
      {items.map((it) => (
        <span key={it.texto} className="flex items-center gap-2">
          <span className={`h-3 w-3 rounded-sm ${it.clase}`} />
          {it.texto}
        </span>
      ))}
    </div>
  );
}

/** Barras agrupadas por categoría (p. ej. emitido vs cobrado por mes). */
export function BarrasAgrupadas({ datos, series, formato, etiquetaX, alto = 220, titulo }) {
  const [t, mostrar, ocultar] = useTooltip();
  const W = 640;
  const H = alto;
  const m = { i: 56, d: 8, s: 12, b: 28 };
  const max = Math.max(0, ...datos.flatMap((d) => series.map((s) => d[s.clave] || 0)));
  const { tope, marcas } = escala(max);
  const ancho = (W - m.i - m.d) / Math.max(1, datos.length);
  const barra = Math.max(3, Math.min(18, (ancho - 8) / series.length - 2));
  const y = (v) => m.s + (H - m.s - m.b) * (1 - v / tope);
  return (
    <div className="relative" data-grafico onMouseLeave={ocultar}>
      <svg viewBox={`0 0 ${W} ${H}`} className="h-auto w-full" role="img" aria-label={titulo}>
        {marcas.map((v) => (
          <g key={v}>
            <line x1={m.i} x2={W - m.d} y1={y(v)} y2={y(v)} className="stroke-borde" strokeWidth="1" />
            <text x={m.i - 6} y={y(v) + 4} textAnchor="end" className="fill-texto-apoyo text-[11px]">
              {formato(v, true)}
            </text>
          </g>
        ))}
        {datos.map((d, i) => {
          const cx = m.i + ancho * i + ancho / 2;
          const x0 = cx - (series.length * (barra + 2)) / 2;
          return (
            <g key={i}>
              <rect x={m.i + ancho * i} y={m.s} width={ancho} height={H - m.s - m.b} fill="transparent" onMouseMove={(e) => mostrar(e, [etiquetaX(d), ...series.map((s) => `${s.nombre}: ${formato(d[s.clave])}`)])} onTouchStart={(e) => mostrar(e, [etiquetaX(d), ...series.map((s) => `${s.nombre}: ${formato(d[s.clave])}`)])} />
              {series.map((s, j) => {
                const v = d[s.clave] || 0;
                const h = Math.max(0, y(0) - y(v));
                const r = Math.min(4, barra / 2, h);
                const x = x0 + j * (barra + 2);
                return <path key={s.clave} className={`${s.claseSvg} pointer-events-none`} d={`M${x},${y(0)} v${-(h - r)} q0,${-r} ${r},${-r} h${barra - 2 * r} q${r},0 ${r},${r} v${h - r} z`} />;
              })}
              {(datos.length <= 12 || i % 2 === 0) && (
                <text x={cx} y={H - 8} textAnchor="middle" className="fill-texto-apoyo text-[11px]">
                  {etiquetaX(d, true)}
                </text>
              )}
            </g>
          );
        })}
        <line x1={m.i} x2={W - m.d} y1={y(0)} y2={y(0)} className="stroke-borde-fuerte" strokeWidth="1" />
      </svg>
      <Tooltip t={t} />
    </div>
  );
}

/** Línea con marcadores (una serie). */
export function Linea({ datos, clave, formato, etiquetaX, alto = 200, titulo, claseTrazo = 'stroke-alerta', claseMarca = 'fill-alerta' }) {
  const [t, mostrar, ocultar] = useTooltip();
  const W = 640;
  const H = alto;
  const m = { i: 48, d: 12, s: 12, b: 28 };
  const max = Math.max(0, ...datos.map((d) => d[clave] || 0));
  const { tope, marcas } = escala(max);
  const paso = (W - m.i - m.d) / Math.max(1, datos.length - 1);
  const x = (i) => (datos.length === 1 ? (W + m.i - m.d) / 2 : m.i + paso * i);
  const y = (v) => m.s + (H - m.s - m.b) * (1 - v / tope);
  const puntos = datos.map((d, i) => `${x(i)},${y(d[clave] || 0)}`).join(' ');
  const ultimo = datos[datos.length - 1];
  return (
    <div className="relative" data-grafico onMouseLeave={ocultar}>
      <svg viewBox={`0 0 ${W} ${H}`} className="h-auto w-full" role="img" aria-label={titulo}>
        {marcas.map((v) => (
          <g key={v}>
            <line x1={m.i} x2={W - m.d} y1={y(v)} y2={y(v)} className="stroke-borde" strokeWidth="1" />
            <text x={m.i - 6} y={y(v) + 4} textAnchor="end" className="fill-texto-apoyo text-[11px]">
              {formato(v, true)}
            </text>
          </g>
        ))}
        <polyline points={puntos} fill="none" className={claseTrazo} strokeWidth="2" strokeLinejoin="round" />
        {datos.map((d, i) => (
          <g key={i}>
            <circle cx={x(i)} cy={y(d[clave] || 0)} r="4" className={`${claseMarca} stroke-superficie`} strokeWidth="2" />
            <rect x={x(i) - paso / 2} y={m.s} width={Math.max(paso, 20)} height={H - m.s - m.b} fill="transparent" onMouseMove={(e) => mostrar(e, [etiquetaX(d), formato(d[clave])])} onTouchStart={(e) => mostrar(e, [etiquetaX(d), formato(d[clave])])} />
            {(datos.length <= 12 || i % 2 === 0) && (
              <text x={x(i)} y={H - 8} textAnchor="middle" className="fill-texto-apoyo text-[11px]">
                {etiquetaX(d, true)}
              </text>
            )}
          </g>
        ))}
        {ultimo && (
          <text x={x(datos.length - 1) - 6} y={y(ultimo[clave] || 0) - 10} textAnchor="end" className="fill-tinta text-[12px] font-semibold">
            {formato(ultimo[clave])}
          </text>
        )}
      </svg>
      <Tooltip t={t} />
    </div>
  );
}

/** Barras horizontales con etiqueta y valor (magnitud de una sola serie). */
export function BarrasH({ datos, etiqueta, valor, formato, detalle, resaltar, titulo }) {
  const max = Math.max(1, ...datos.map(valor));
  return (
    <ul className="flex flex-col gap-2" aria-label={titulo}>
      {datos.map((d, i) => {
        const v = valor(d);
        const fuerte = resaltar?.(d);
        return (
          <li key={i} className="grid grid-cols-[88px_1fr_auto] items-center gap-3 text-sm sm:grid-cols-[120px_1fr_auto]" title={detalle ? detalle(d) : undefined}>
            <span className="truncate text-texto-suave">{etiqueta(d)}</span>
            <span className="h-3 rounded-r bg-superficie-2">
              <span className={`block h-3 rounded-r ${fuerte ? 'bg-aviso' : 'bg-acento'}`} style={{ width: `${(v / max) * 100}%` }} />
            </span>
            <span className="text-right font-semibold tabular-nums">
              {formato(v)}
              {fuerte && <span className="ml-1 text-xs font-normal text-aviso">pico</span>}
            </span>
          </li>
        );
      })}
    </ul>
  );
}

/** Tabla de datos accesible bajo cada gráfico. */
export function TablaDatos({ columnas, filas }) {
  return (
    <details className="text-sm">
      <summary className="cursor-pointer text-acento">Ver como tabla</summary>
      <div className="mt-2 max-h-64 overflow-auto">
        <table className="w-full">
          <thead>
            <tr className="text-left text-xs text-texto-apoyo">
              {columnas.map((c) => (
                <th key={c.titulo} className={`py-1 pr-3 font-semibold ${c.der ? 'text-right' : ''}`}>
                  {c.titulo}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {filas.map((f, i) => (
              <tr key={i} className="border-t border-superficie-2">
                {columnas.map((c) => (
                  <td key={c.titulo} className={`py-1 pr-3 ${c.der ? 'text-right tabular-nums' : ''}`}>
                    {c.valor(f)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </details>
  );
}
