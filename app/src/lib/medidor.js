// Lecturas de medidores con 3 decimales (litros). Consumo = actual − anterior.
// Negativo → alerta que exige motivo; pico (> 2 × media de 3 meses) → alerta ámbar que no bloquea.

/** «876», «876,5», «876.500», «1.298,125» → 876.5 (número con ≤ 3 decimales), o null. */
export function parsearLectura(texto) {
  if (texto === null || texto === undefined) return null;
  let s = String(texto).trim().replace(/\s/g, '');
  if (!s) return null;
  if (s.includes(',')) s = s.replace(/\./g, '').replace(',', '.');
  else if ((s.match(/\./g) || []).length > 1) s = s.replace(/\./g, '');
  if (!/^\d+(\.\d{1,3})?$/.test(s)) return null;
  return Number(s);
}

/** Resta exacta en milésimas para no arrastrar errores de float. */
export function calcularConsumo(anterior, actual) {
  if (anterior == null || actual == null) return null;
  return (Math.round(actual * 1000) - Math.round(anterior * 1000)) / 1000;
}

export function alertaConsumo(consumo, media3m) {
  if (consumo == null) return null;
  if (consumo < 0) return 'NEGATIVO';
  if (media3m && consumo > 2 * media3m) return 'PICO';
  return null;
}

/** Cargo por consumo en céntimos: m³ × tarifa (céntimos por m³), redondeado al céntimo. */
export function cargoAgua(consumoM3, tarifaCts) {
  if (consumoM3 == null || !tarifaCts) return null;
  return Math.round((Math.round(consumoM3 * 1000) * tarifaCts) / 1000);
}

/** Siguiente medidor pendiente en el orden de la ronda (después del actual; si no hay, desde el inicio). */
export function siguientePendiente(medidores, actualId) {
  const orden = [...medidores].sort((a, b) => (a.orden_ronda ?? 0) - (b.orden_ronda ?? 0));
  const i = orden.findIndex((m) => m.medidor_id === actualId);
  const despues = orden.slice(i + 1).find((m) => m.estado === 'pendiente' && m.medidor_id !== actualId);
  return despues || orden.find((m) => m.estado === 'pendiente' && m.medidor_id !== actualId) || null;
}
