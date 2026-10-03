// Bloque G · Operación: utilidades de las pantallas de ocurrencias, tickets SLA, visitas QR,
// parking y paquetes. El API decide (semáforo, cobro, validez del QR); aquí solo se presenta.

/** Semáforo del SLA → tono del mapa de estados y texto. El color nunca va solo: siempre con texto. */
export const SEMAFORO = {
  verde: { tono: 'acento', texto: 'En plazo' },
  ambar: { tono: 'aviso', texto: 'Por vencer' },
  rojo: { tono: 'alerta', texto: 'Vencido' },
  cerrado: { tono: 'hecho', texto: 'Cerrado' },
};

/** Información del semáforo; cerrado distingue si se cumplió el plazo. */
export function infoSemaforo(color, cumplido) {
  if (color === 'cerrado') return { tono: cumplido ? 'hecho' : 'alerta', texto: cumplido ? 'Cerrado en plazo' : 'Cerrado fuera de plazo' };
  return SEMAFORO[color] || { tono: 'neutro', texto: 'Sin SLA' };
}

/** Minutos → «2 h 15 min», «3 d 4 h», «45 min». */
export function duracion(min) {
  const m = Math.abs(Math.round(Number(min) || 0));
  if (m < 60) return `${m} min`;
  const h = Math.floor(m / 60);
  if (h < 24) return m % 60 ? `${h} h ${m % 60} min` : `${h} h`;
  const d = Math.floor(h / 24);
  return h % 24 ? `${d} d ${h % 24} h` : `${d} d`;
}

/** Minutos restantes del SLA → «vence en 3 h» / «venció hace 2 d». */
export function restanteSLA(min) {
  if (min == null) return '';
  return min >= 0 ? `vence en ${duracion(min)}` : `venció hace ${duracion(min)}`;
}

/**
 * Matriz del QR (filas de «0»/«1», como la entrega el API) → trazo SVG de un cuadrado por módulo.
 * Junta los módulos negros consecutivos de cada fila en un solo rectángulo para aligerar el trazo.
 */
export function trazoQR(matriz = []) {
  const partes = [];
  matriz.forEach((fila, y) => {
    let x = 0;
    while (x < fila.length) {
      if (fila[x] !== '1') {
        x++;
        continue;
      }
      let fin = x;
      while (fin < fila.length && fila[fin] === '1') fin++;
      partes.push(`M${x} ${y}h${fin - x}v1h-${fin - x}z`);
      x = fin;
    }
  });
  return partes.join('');
}

/** Placa normalizada como la guarda el API: mayúsculas, sin espacios ni guiones. */
export function normalizarPlaca(p) {
  return String(p || '').toUpperCase().replace(/[\s-]/g, '');
}

/** Espejo de MontoParking (Go) para la vista previa: fracción empezada se cobra, tolerancia gratis. */
export function montoParking(minutos, tarifaHoraCts, fraccionMin = 60, toleranciaMin = 0) {
  if (minutos <= 0 || minutos <= toleranciaMin || tarifaHoraCts <= 0) return 0;
  const f = fraccionMin > 0 ? fraccionMin : 60;
  const fracciones = Math.ceil(minutos / f);
  return Math.floor((fracciones * f * tarifaHoraCts + 30) / 60);
}

/** Texto del motivo de rechazo del QR (el API ya lo manda; esto cubre el caso sin conexión de datos). */
export const MOTIVO_RECHAZO = {
  NO_EXISTE: 'Código desconocido',
  ANULADA: 'Visita anulada',
  FINALIZADA: 'Visita terminada',
  YA_DENTRO: 'Ya está dentro',
  AUN_NO_VIGENTE: 'Aún no vigente',
  VENCIDA: 'Autorización vencida',
  SIN_USOS: 'Sin ingresos disponibles',
};
