// Fechas: UTC en la base, hora de Lima en pantalla (§0.6). Periodos «AAAA-MM».
export const ZONA = 'America/Lima';

export const MESES = [
  'enero', 'febrero', 'marzo', 'abril', 'mayo', 'junio',
  'julio', 'agosto', 'setiembre', 'octubre', 'noviembre', 'diciembre',
];
export const MESES_CORTOS = ['ene', 'feb', 'mar', 'abr', 'may', 'jun', 'jul', 'ago', 'set', 'oct', 'nov', 'dic'];
export const DIAS_CORTOS = ['Dom', 'Lun', 'Mar', 'Mié', 'Jue', 'Vie', 'Sáb'];

const capitalizar = (s) => s.charAt(0).toUpperCase() + s.slice(1);

export function esPeriodo(p) {
  return typeof p === 'string' && /^\d{4}-(0[1-9]|1[0-2])$/.test(p);
}

/** «2026-09» → «Setiembre 2026». */
export function nombrePeriodo(p, { corto = false } = {}) {
  if (!esPeriodo(p)) return p || '';
  const [a, m] = p.split('-').map(Number);
  if (corto) return `${MESES_CORTOS[m - 1]}-${a}`;
  return `${capitalizar(MESES[m - 1])} ${a}`;
}

/** Nombre del mes en minúsculas: «2026-09» → «setiembre». */
export function mesDePeriodo(p) {
  if (!esPeriodo(p)) return '';
  return MESES[Number(p.slice(5, 7)) - 1];
}

export function sumarMeses(p, n) {
  const [a, m] = p.split('-').map(Number);
  const total = a * 12 + (m - 1) + n;
  const na = Math.floor(total / 12);
  const nm = (total % 12) + 1;
  return `${na}-${String(nm).padStart(2, '0')}`;
}

/** Partes de una fecha en hora de Lima. */
export function partesLima(fecha) {
  const d = fecha instanceof Date ? fecha : new Date(fecha);
  if (Number.isNaN(d.getTime())) return null;
  const fmt = new Intl.DateTimeFormat('en-US', {
    timeZone: ZONA, year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hourCycle: 'h23', weekday: 'short',
  });
  const p = Object.fromEntries(fmt.formatToParts(d).map((x) => [x.type, x.value]));
  const dias = { Sun: 0, Mon: 1, Tue: 2, Wed: 3, Thu: 4, Fri: 5, Sat: 6 };
  return {
    anio: Number(p.year), mes: Number(p.month), dia: Number(p.day),
    hora: Number(p.hour), minuto: Number(p.minute), diaSemana: dias[p.weekday],
  };
}

/** Periodo abierto por defecto: el mes actual en Lima. */
export function periodoActual(ahora = new Date()) {
  const p = partesLima(ahora);
  return `${p.anio}-${String(p.mes).padStart(2, '0')}`;
}

const dos = (n) => String(n).padStart(2, '0');

/** ISO o fecha → «15-09-2026» (hora de Lima). Una fecha pura «2026-09-15» no se desplaza. */
export function formatearFecha(valor) {
  if (!valor) return '—';
  if (typeof valor === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(valor)) {
    const [a, m, d] = valor.split('-');
    return `${d}-${m}-${a}`;
  }
  const p = partesLima(valor);
  if (!p) return '—';
  return `${dos(p.dia)}-${dos(p.mes)}-${p.anio}`;
}

/** «15-09» (sin año), como en el lienzo. */
export function formatearDiaMes(valor) {
  const f = formatearFecha(valor);
  return f === '—' ? f : f.slice(0, 5);
}

/** ISO → «30-09-2026 08:14» (hora de Lima). */
export function formatearFechaHora(valor) {
  if (!valor) return '—';
  const p = partesLima(valor);
  if (!p) return '—';
  return `${dos(p.dia)}-${dos(p.mes)}-${p.anio} ${dos(p.hora)}:${dos(p.minuto)}`;
}

export function formatearHora(valor) {
  const p = partesLima(valor);
  if (!p) return '—';
  return `${dos(p.hora)}:${dos(p.minuto)}`;
}

/** «2026-09-30» del día en Lima. */
export function diaLima(valor) {
  const p = partesLima(valor);
  if (!p) return '';
  return `${p.anio}-${dos(p.mes)}-${dos(p.dia)}`;
}

/** «hace 5 min», «hace 1 h», «ayer», «hace 3 días» o la fecha. */
export function haceCuanto(valor, ahora = new Date()) {
  const d = new Date(valor);
  if (Number.isNaN(d.getTime())) return '';
  const seg = Math.round((ahora.getTime() - d.getTime()) / 1000);
  if (seg < 60) return 'hace un momento';
  const min = Math.floor(seg / 60);
  if (min < 60) return `hace ${min} min`;
  const h = Math.floor(min / 60);
  const hoy = diaLima(ahora);
  const ayer = diaLima(new Date(ahora.getTime() - 86400000));
  if (diaLima(d) === hoy) return `hace ${h} h`;
  if (diaLima(d) === ayer) return 'ayer';
  const dias = Math.floor(h / 24);
  if (dias < 7) return `hace ${Math.max(dias, 2)} días`;
  return formatearFecha(d);
}

// --- Días de calendario (cadenas «AAAA-MM-DD», sin zonas: se tratan como fechas civiles) ---

function civil(diaIso) {
  const [a, m, d] = diaIso.split('-').map(Number);
  return new Date(Date.UTC(a, m - 1, d));
}
function aIso(date) {
  return `${date.getUTCFullYear()}-${dos(date.getUTCMonth() + 1)}-${dos(date.getUTCDate())}`;
}

export function sumarDias(diaIso, n) {
  const d = civil(diaIso);
  d.setUTCDate(d.getUTCDate() + n);
  return aIso(d);
}

export function diaSemana(diaIso) {
  return civil(diaIso).getUTCDay();
}

/** Lunes de la semana del día dado. */
export function inicioSemana(diaIso) {
  const ds = diaSemana(diaIso);
  return sumarDias(diaIso, ds === 0 ? -6 : 1 - ds);
}

export function diasDeSemana(lunesIso) {
  return Array.from({ length: 7 }, (_, i) => sumarDias(lunesIso, i));
}

/** «28 set – 4 oct 2026». */
export function rangoSemana(lunesIso) {
  const fin = sumarDias(lunesIso, 6);
  const [a1, m1, d1] = lunesIso.split('-').map(Number);
  const [a2, m2, d2] = fin.split('-').map(Number);
  const izq = m1 === m2 && a1 === a2 ? `${d1}` : `${d1} ${MESES_CORTOS[m1 - 1]}${a1 !== a2 ? ' ' + a1 : ''}`;
  return `${izq} – ${d2} ${MESES_CORTOS[m2 - 1]} ${a2}`;
}

/** «Sáb 3 oct». */
export function etiquetaDia(diaIso) {
  const [, m, d] = diaIso.split('-').map(Number);
  return `${DIAS_CORTOS[diaSemana(diaIso)]} ${d} ${MESES_CORTOS[m - 1]}`;
}

/** Días entre dos «AAAA-MM-DD» (b − a). */
export function diasEntre(a, b) {
  return Math.round((civil(b) - civil(a)) / 86400000);
}
