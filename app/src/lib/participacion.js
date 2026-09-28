// Participaciones con 4 decimales (§06). Se trabaja en diezmilésimas de punto porcentual (enteros).

/** «4.2000» | 4.2 | «4,20» → 42000 (diezmilésimas), o null. */
export function aDiezmilesimas(valor) {
  if (valor === null || valor === undefined || valor === '') return null;
  const s = String(valor).trim().replace(',', '.');
  const m = s.match(/^(\d+)(?:\.(\d{1,4})\d*)?$/);
  if (!m) return null;
  return Number(m[1]) * 10000 + Number((m[2] || '').padEnd(4, '0'));
}

/** «Suman 99,5000 %, faltan 0,5000 %» o null si cuadra (tolerancia 0,0001 %). */
export function revisarSuma(valores) {
  const total = valores.reduce((a, v) => a + (aDiezmilesimas(v) ?? 0), 0);
  const diff = 1000000 - total;
  const fmt = (n) => `${Math.floor(Math.abs(n) / 10000)},${String(Math.abs(n) % 10000).padStart(4, '0')} %`;
  if (Math.abs(diff) <= 1) return { ok: true, suma: fmt(total), mensaje: null };
  return {
    ok: false,
    suma: fmt(total),
    mensaje: diff > 0 ? `Suman ${fmt(total)}, faltan ${fmt(diff)}` : `Suman ${fmt(total)}, sobran ${fmt(diff)}`,
  };
}

/** DNI 8 dígitos; RUC 11 que empieza con 10 o 20; celular peruano 9 dígitos que empieza con 9. */
export const esDni = (s) => /^\d{8}$/.test(String(s || ''));
export const esRuc = (s) => /^(10|20)\d{9}$/.test(String(s || ''));
export const esCelular = (s) => /^9\d{8}$/.test(String(s || '').replace(/\s/g, ''));

/** DNI enmascarado para listados (Ley 29733): «4512****». */
export function enmascararDni(dni) {
  const s = String(dni || '');
  if (s.includes('*')) return s;
  if (s.length < 5) return s;
  return s.slice(0, 4) + '*'.repeat(s.length - 4);
}
