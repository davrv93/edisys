import { useEffect, useId, useMemo, useState } from 'react';
import { createPortal } from 'react-dom';
import Icono from './Icono.jsx';
import { useFlotante } from './Menu.jsx';
import { MESES, MESES_CORTOS, diaLima, esPeriodo, sumarMeses } from '../lib/fechas.js';

const DIAS = ['Lu', 'Ma', 'Mi', 'Ju', 'Vi', 'Sá', 'Do'];
const NOMBRE_DIA = ['lunes', 'martes', 'miércoles', 'jueves', 'viernes', 'sábado', 'domingo'];
const dos = (n) => String(n).padStart(2, '0');
const capital = (s) => s.charAt(0).toUpperCase() + s.slice(1);

/** «2026-09-15» → «15/09/2026». */
export function isoADmy(iso) {
  if (!iso || !/^\d{4}-\d{2}-\d{2}$/.test(iso)) return '';
  const [a, m, d] = iso.split('-');
  return `${d}/${m}/${a}`;
}

/** «15/09/2026», «15-9-26», «15.09.2026» → «2026-09-15»; null si no es una fecha válida. */
export function dmyAIso(texto) {
  const t = String(texto || '').trim();
  const m = t.match(/^(\d{1,2})[/.\-\s](\d{1,2})[/.\-\s](\d{2}|\d{4})$/);
  if (!m) return null;
  const d = Number(m[1]);
  const mes = Number(m[2]);
  let a = Number(m[3]);
  if (a < 100) a += 2000;
  if (mes < 1 || mes > 12 || d < 1) return null;
  const ultimo = new Date(Date.UTC(a, mes, 0)).getUTCDate();
  if (d > ultimo) return null;
  return `${a}-${dos(mes)}-${dos(d)}`;
}

/** Semanas del mes «AAAA-MM» con lunes primero: matriz de «AAAA-MM-DD» o null. */
export function semanasDelMes(periodo) {
  const [a, m] = periodo.split('-').map(Number);
  const primero = new Date(Date.UTC(a, m - 1, 1)).getUTCDay(); // 0 = domingo
  const vacios = (primero + 6) % 7;
  const dias = new Date(Date.UTC(a, m, 0)).getUTCDate();
  const celdas = [...Array(vacios).fill(null), ...Array.from({ length: dias }, (_, i) => `${a}-${dos(m)}-${dos(i + 1)}`)];
  while (celdas.length % 7) celdas.push(null);
  const out = [];
  for (let i = 0; i < celdas.length; i += 7) out.push(celdas.slice(i, i + 7));
  return out;
}

/** ¿El navegador ya muestra la fecha nativa en español y en táctil? Entonces se usa la nativa. */
function nativoEnEspanol() {
  try {
    return window.matchMedia('(pointer: coarse)').matches && /^es\b/i.test(navigator.language || '');
  } catch {
    return false;
  }
}

/**
 * Selector de fecha propio (plan, bloque G): muestra y acepta «dd/mm/aaaa», calendario desplegable
 * en español con el lunes primero. Entrega «AAAA-MM-DD» (o '' si se borra), igual que el input nativo.
 * En un celular cuyo navegador ya está en español se usa el <input type="date"> nativo.
 */
export default function SelectorFecha({ valor, onCambio, id, className = '', disabled, min, max, ...resto }) {
  const [nativo] = useState(nativoEnEspanol);
  const [texto, setTexto] = useState(isoADmy(valor));
  const [invalido, setInvalido] = useState(false);
  const { abierto, setAbierto, cerrar, caja, panel, disparador, pos } = useFlotante('izq');
  const [mes, setMes] = useState(() => (valor || diaLima(new Date())).slice(0, 7));
  const idCal = useId();
  const hoy = diaLima(new Date());

  useEffect(() => {
    setTexto(isoADmy(valor));
    setInvalido(false);
    if (valor) setMes(valor.slice(0, 7));
  }, [valor]);

  const semanas = useMemo(() => semanasDelMes(mes), [mes]);

  if (nativo) {
    return <input type="date" id={id} value={valor || ''} min={min} max={max} disabled={disabled} onChange={(e) => onCambio?.(e.target.value)} className={className} {...resto} />;
  }

  const confirmarTexto = () => {
    if (!texto.trim()) {
      setInvalido(false);
      if (valor) onCambio?.('');
      return;
    }
    const iso = dmyAIso(texto);
    if (!iso || (min && iso < min) || (max && iso > max)) {
      setInvalido(true);
      return;
    }
    setInvalido(false);
    setTexto(isoADmy(iso));
    if (iso !== valor) onCambio?.(iso);
  };
  const elegir = (iso) => {
    onCambio?.(iso);
    cerrar();
  };
  const [a, m] = mes.split('-').map(Number);

  const teclaDia = (e, iso) => {
    const saltos = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7 };
    if (!(e.key in saltos)) return;
    e.preventDefault();
    const d = new Date(`${iso}T12:00:00Z`);
    d.setUTCDate(d.getUTCDate() + saltos[e.key]);
    const nuevo = d.toISOString().slice(0, 10);
    if (nuevo.slice(0, 7) !== mes) setMes(nuevo.slice(0, 7));
    setTimeout(() => panel.current?.querySelector(`[data-dia="${nuevo}"]`)?.focus(), 0);
  };

  return (
    <div ref={caja} className="relative">
      <input
        type="text"
        inputMode="numeric"
        autoComplete="off"
        id={id}
        placeholder="dd/mm/aaaa"
        value={texto}
        disabled={disabled}
        onChange={(e) => setTexto(e.target.value)}
        onBlur={confirmarTexto}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            confirmarTexto();
          } else if (e.key === 'ArrowDown' && e.altKey) {
            e.preventDefault();
            setAbierto(true);
          }
        }}
        aria-invalid={invalido || resto['aria-invalid'] ? 'true' : undefined}
        className={`${className} pr-10 tabular-nums ${invalido ? '!border-alerta' : ''}`}
        {...resto}
      />
      <button
        ref={disparador}
        type="button"
        disabled={disabled}
        onClick={() => setAbierto(!abierto)}
        aria-label="Abrir calendario"
        aria-haspopup="dialog"
        aria-expanded={abierto}
        aria-controls={abierto ? idCal : undefined}
        className="absolute inset-y-0 right-0 flex w-10 items-center justify-center rounded-r-control text-texto-apoyo transition-colors duration-rapida hover:text-acento disabled:opacity-40"
      >
        <Icono nombre="calendario_dias" tam={16} />
      </button>
      {invalido && (
        <p className="mt-1 text-xs text-alerta" role="alert">
          Escribe la fecha como dd/mm/aaaa.
        </p>
      )}
      {abierto &&
        pos &&
        createPortal(
          <div ref={panel} data-flotante id={idCal} role="dialog" aria-label="Elegir fecha" style={pos} className="fixed z-[90] w-[292px] rounded-tarjeta border border-borde bg-superficie p-3 shadow-flotante animate-desplegar">
            <div className="mb-2 flex items-center justify-between">
              <button type="button" onClick={() => setMes(sumarMeses(mes, -1))} aria-label="Mes anterior" className="flex h-9 w-9 items-center justify-center rounded-control hover:bg-fondo">
                <Icono nombre="izq" tam={16} />
              </button>
              <span className="text-sm font-semibold" aria-live="polite">
                {capital(MESES[m - 1])} {a}
              </span>
              <button type="button" onClick={() => setMes(sumarMeses(mes, 1))} aria-label="Mes siguiente" className="flex h-9 w-9 items-center justify-center rounded-control hover:bg-fondo">
                <Icono nombre="der" tam={16} />
              </button>
            </div>
            <table className="w-full table-fixed text-center text-sm" role="grid" aria-label={`${capital(MESES[m - 1])} ${a}`}>
              <thead>
                <tr>
                  {DIAS.map((d, i) => (
                    <th key={d} scope="col" abbr={NOMBRE_DIA[i]} className="pb-1 text-xs font-semibold text-texto-apoyo">
                      {d}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {semanas.map((sem, i) => (
                  <tr key={i}>
                    {sem.map((iso, j) => {
                      if (!iso) return <td key={j} />;
                      const sel = iso === valor;
                      const fuera = (min && iso < min) || (max && iso > max);
                      const esHoy = iso === hoy;
                      const enfocable = sel || (!valor && esHoy) || (!valor && !semanas.flat().includes(hoy) && iso.endsWith('-01'));
                      return (
                        <td key={j} className="p-0.5">
                          <button
                            type="button"
                            data-dia={iso}
                            disabled={fuera}
                            tabIndex={enfocable ? 0 : -1}
                            aria-pressed={sel}
                            aria-current={esHoy ? 'date' : undefined}
                            aria-label={`${Number(iso.slice(8))} de ${MESES[m - 1]} de ${a}`}
                            onClick={() => elegir(iso)}
                            onKeyDown={(e) => teclaDia(e, iso)}
                            className={`h-9 w-full rounded-control tabular-nums transition-colors duration-rapida disabled:opacity-30 ${sel ? 'bg-acento font-semibold text-white' : esHoy ? 'font-semibold text-curso ring-1 ring-inset ring-curso-borde hover:bg-fondo' : 'hover:bg-fondo'}`}
                          >
                            {Number(iso.slice(8))}
                          </button>
                        </td>
                      );
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
            <div className="mt-2 flex justify-between border-t border-borde pt-2 text-sm">
              <button type="button" className="rounded-control px-2 py-1 font-semibold text-acento hover:bg-acento-suave" onClick={() => elegir(hoy)}>
                Hoy
              </button>
              {valor && (
                <button type="button" className="rounded-control px-2 py-1 text-texto-suave hover:bg-fondo" onClick={() => elegir('')}>
                  Borrar
                </button>
              )}
            </div>
          </div>,
          document.body,
        )}
    </div>
  );
}

/**
 * Selector de mes («AAAA-MM») en español: botón con «Setiembre 2026» y una rejilla de 12 meses.
 * Reemplaza al <input type="month"> nativo, que sale en el idioma del navegador.
 */
export function SelectorMes({ valor, onCambio, min, max, etiqueta = 'Mes', className = '' }) {
  const { abierto, setAbierto, cerrar, caja, panel, disparador, pos } = useFlotante('izq');
  const [anio, setAnio] = useState(() => Number((valor || '').slice(0, 4)) || new Date().getFullYear());
  const idPanel = useId();
  useEffect(() => {
    if (esPeriodo(valor)) setAnio(Number(valor.slice(0, 4)));
  }, [valor]);
  useEffect(() => {
    if (abierto && pos) panel.current?.querySelector('[aria-pressed="true"],button:not([disabled])')?.focus();
  }, [abierto, pos, panel]);
  const nombre = esPeriodo(valor) ? `${capital(MESES[Number(valor.slice(5)) - 1])} ${valor.slice(0, 4)}` : 'Elegir';
  return (
    <div ref={caja} className={`relative inline-flex ${className}`}>
      <button
        ref={disparador}
        type="button"
        onClick={() => setAbierto(!abierto)}
        aria-haspopup="dialog"
        aria-expanded={abierto}
        aria-controls={abierto ? idPanel : undefined}
        aria-label={`${etiqueta}: ${nombre}`}
        className="inline-flex h-11 items-center gap-2 rounded-control border border-borde-fuerte bg-superficie px-3 text-sm font-semibold transition-colors duration-rapida hover:bg-fondo lg:h-9"
      >
        <Icono nombre="calendario_dias" tam={16} className="text-texto-apoyo" />
        {nombre}
      </button>
      {abierto &&
        pos &&
        createPortal(
          <div ref={panel} data-flotante id={idPanel} role="dialog" aria-label={etiqueta} style={pos} className="fixed z-[90] w-[260px] rounded-tarjeta border border-borde bg-superficie p-3 shadow-flotante animate-desplegar">
            <div className="mb-2 flex items-center justify-between">
              <button type="button" onClick={() => setAnio(anio - 1)} aria-label="Año anterior" className="flex h-9 w-9 items-center justify-center rounded-control hover:bg-fondo">
                <Icono nombre="izq" tam={16} />
              </button>
              <span className="text-sm font-semibold tabular-nums" aria-live="polite">
                {anio}
              </span>
              <button type="button" onClick={() => setAnio(anio + 1)} aria-label="Año siguiente" className="flex h-9 w-9 items-center justify-center rounded-control hover:bg-fondo">
                <Icono nombre="der" tam={16} />
              </button>
            </div>
            <div className="grid grid-cols-3 gap-1">
              {MESES_CORTOS.map((c, i) => {
                const p = `${anio}-${dos(i + 1)}`;
                const fuera = (min && p < min) || (max && p > max);
                const sel = p === valor;
                return (
                  <button
                    key={c}
                    type="button"
                    disabled={fuera}
                    aria-pressed={sel}
                    aria-label={`${MESES[i]} ${anio}`}
                    onClick={() => {
                      onCambio?.(p);
                      cerrar();
                    }}
                    className={`h-9 rounded-control text-sm capitalize transition-colors duration-rapida disabled:opacity-30 ${sel ? 'bg-acento font-semibold text-white' : 'hover:bg-fondo'}`}
                  >
                    {c}
                  </button>
                );
              })}
            </div>
          </div>,
          document.body,
        )}
    </div>
  );
}
