// Dinero en céntimos enteros (§0.1). El formato «S/ 4.800,00» es solo de pantalla:
// punto de miles y coma decimal, como en el lienzo. No se usa Intl con es-PE
// porque según el navegador devuelve «4,800.00».

/** Agrupa los miles de un entero no negativo (en texto) con punto. */
function agruparMiles(digitos) {
  return digitos.replace(/\B(?=(\d{3})+(?!\d))/g, '.');
}

/**
 * Formatea un número con punto de miles y coma decimal.
 * formatearNumero(4800, 2) → «4.800,00»; formatearNumero(13.125, 1) → «13,1».
 */
export function formatearNumero(valor, decimales = 0) {
  if (valor === null || valor === undefined || valor === '' || Number.isNaN(Number(valor))) return '—';
  const n = Number(valor);
  const negativo = n < 0;
  const fijo = Math.abs(n).toFixed(decimales); // redondeo half-up de toFixed sobre el valor mostrado
  const [ent, dec] = fijo.split('.');
  const texto = agruparMiles(ent) + (dec ? ',' + dec : '');
  return (negativo && Number(fijo) !== 0 ? '-' : '') + texto;
}

/**
 * Céntimos enteros → «S/ 19.460,00». Con { sinDecimales: true } → «S/ 19.460».
 * Trabaja con enteros: nunca pasa por float para las cifras.
 */
export function formatearSoles(cts, { sinDecimales = false, conSigno = false } = {}) {
  if (cts === null || cts === undefined || cts === '') return '—';
  let n = typeof cts === 'bigint' ? cts : BigInt(Math.round(Number(cts)));
  const negativo = n < 0n;
  if (negativo) n = -n;
  const soles = n / 100n;
  const cent = n % 100n;
  let texto = 'S/ ' + agruparMiles(soles.toString());
  if (!sinDecimales) texto += ',' + cent.toString().padStart(2, '0');
  if (negativo) return '-' + texto;
  if (conSigno && n > 0n) return '+' + texto;
  return texto;
}

/**
 * Texto que escribe una persona → céntimos enteros, o null si no es válido.
 * Acepta «1.234,56», «1234,56», «1234.56», «S/ 80», «80».
 * Regla: si hay coma, la coma es el decimal y los puntos son miles.
 * Si solo hay puntos: un único punto seguido de 1 o 2 cifras es decimal; si no, son miles.
 */
export function parsearSoles(texto) {
  if (texto === null || texto === undefined) return null;
  let s = String(texto).trim().replace(/^S\/\s*/i, '').replace(/\s/g, '');
  if (s === '') return null;
  let negativo = false;
  if (s.startsWith('-')) {
    negativo = true;
    s = s.slice(1);
  }
  let entero;
  let decimal = '';
  if (s.includes(',')) {
    if ((s.match(/,/g) || []).length > 1) return null;
    const [a, b] = s.split(',');
    entero = a.replace(/\./g, '');
    decimal = b;
  } else if (s.includes('.')) {
    const partes = s.split('.');
    const ultima = partes[partes.length - 1];
    if (partes.length === 2 && ultima.length <= 2) {
      entero = partes[0];
      decimal = ultima;
    } else {
      if (!partes.slice(1).every((p) => p.length === 3)) return null;
      entero = partes.join('');
    }
  } else {
    entero = s;
  }
  if (entero === '') entero = '0';
  if (!/^\d+$/.test(entero) || !/^\d{0,2}$/.test(decimal)) return null;
  const cts = Number(entero) * 100 + Number((decimal + '00').slice(0, 2));
  if (!Number.isSafeInteger(cts)) return null;
  return negativo ? -cts : cts;
}

/** Céntimos → texto editable en un Campo de dinero: 99000 → «990,00». */
export function ctsATexto(cts) {
  if (cts === null || cts === undefined) return '';
  return formatearSoles(cts).replace(/^-?S\/ /, (m) => (m.startsWith('-') ? '-' : ''));
}

/** Porcentaje → «13,1 %». Acepta número o texto con punto decimal del API («100.0000»). */
export function formatearPct(valor, decimales = 1) {
  if (valor === null || valor === undefined || valor === '') return '—';
  return formatearNumero(Number(valor), decimales) + ' %';
}

/** Suma segura de céntimos (ignora nulos). */
export function sumarCts(lista) {
  return lista.reduce((acc, v) => acc + (Number.isFinite(Number(v)) ? Math.round(Number(v)) : 0), 0);
}

/** Porcentaje de parte sobre total, con un decimal, sin dividir por cero. */
export function pctDe(parte, total) {
  if (!total) return 0;
  return Math.round((Number(parte) * 1000) / Number(total)) / 10;
}

/**
 * Céntimos → cifra corta para la franja de KPI cuando no cabe: «S/ 19,5 mil», «S/ 1,2 mill.».
 * Por debajo de S/ 10.000 devuelve el formato completo. El valor exacto va en el tooltip.
 */
export function formatearSolesCorto(cts) {
  if (cts === null || cts === undefined || cts === '') return '—';
  const n = Number(cts);
  if (!Number.isFinite(n)) return '—';
  const soles = Math.abs(n) / 100;
  const signo = n < 0 ? '-' : '';
  const limpio = (x) => formatearNumero(x, 1).replace(/,0$/, '');
  if (soles < 10000) return formatearSoles(cts);
  if (soles < 1000000) return `${signo}S/ ${limpio(soles / 1000)} mil`;
  return `${signo}S/ ${limpio(soles / 1000000)} mill.`;
}
