// Mapa único de estados (§2.1). Ninguna pantalla decide colores de estado por su cuenta.
// Cada estado lleva SIEMPRE su texto: el color nunca es el único portador del significado.

export const TONO_CLASES = {
  acento: 'bg-acento-suave text-acento border-acento-borde',
  aviso: 'bg-aviso-suave text-aviso border-aviso-borde',
  alerta: 'bg-alerta-suave text-alerta border-alerta-borde',
  neutro: 'bg-superficie-2 text-texto-suave border-borde',
  solido: 'bg-acento text-white border-acento',
  oscuro: 'bg-tinta text-white border-tinta',
};

export const ESTADOS = {
  // Recibos y pagos
  borrador: { tono: 'neutro', texto: 'Borrador' },
  emitido: { tono: 'aviso', texto: 'Pendiente' },
  pendiente: { tono: 'aviso', texto: 'Pendiente' },
  pagado: { tono: 'acento', texto: 'Pagado' },
  pagado_parcial: { tono: 'aviso', texto: 'Pago parcial' },
  vencido: { tono: 'alerta', texto: 'Vencido' },
  anulado: { tono: 'neutro', texto: 'Anulado' },
  pendiente_validacion: { tono: 'aviso', texto: 'En revisión' },
  validado_pago: { tono: 'acento', texto: 'Validado' },
  moroso: { tono: 'alerta', texto: 'Moroso' },
  al_dia: { tono: 'acento', texto: 'Al día' },
  // Reservas
  confirmada: { tono: 'acento', texto: 'Confirmada' },
  pendiente_pago: { tono: 'aviso', texto: 'Retenida 15 min' },
  cancelada: { tono: 'neutro', texto: 'Cancelada' },
  vencida: { tono: 'neutro', texto: 'Liberada' },
  no_show: { tono: 'neutro', texto: 'No se presentó' },
  bloqueo: { tono: 'neutro', texto: 'Bloqueo' },
  // Lecturas
  leida: { tono: 'acento', texto: 'Leída' },
  alerta_lectura: { tono: 'aviso', texto: 'Con alerta' },
  PICO: { tono: 'aviso', texto: 'Pico de consumo' },
  NEGATIVO: { tono: 'alerta', texto: 'Consumo negativo' },
  // Mantenimiento
  reportado: { tono: 'neutro', texto: 'Reportado' },
  validado: { tono: 'neutro', texto: 'Validado' },
  presupuestado: { tono: 'aviso', texto: 'Informe y costos' },
  aprobado: { tono: 'acento', texto: 'Aprobado' },
  en_ejecucion: { tono: 'aviso', texto: 'En ejecución' },
  terminado: { tono: 'acento', texto: 'Terminado' },
  rechazado: { tono: 'alerta', texto: 'Rechazado' },
  descartado: { tono: 'neutro', texto: 'Descartado' },
  // Criticidad
  critica: { tono: 'alerta', texto: 'Crítico' },
  media: { tono: 'aviso', texto: 'Medio' },
  baja: { tono: 'neutro', texto: 'Bajo' },
  // WhatsApp
  en_cola: { tono: 'neutro', texto: 'En cola' },
  enviado: { tono: 'acento', texto: 'Enviado' },
  entregado: { tono: 'acento', texto: 'Entregado' },
  leido: { tono: 'acento', texto: 'Leído' },
  fallido: { tono: 'alerta', texto: 'Fallido' },
  error: { tono: 'alerta', texto: 'Error' },
  simulado: { tono: 'aviso', texto: 'Simulado' },
  recibido: { tono: 'neutro', texto: 'Recibido' },
  // Usuarios
  activo: { tono: 'acento', texto: 'Activo' },
  inactivo: { tono: 'neutro', texto: 'Inactivo' },
  invitado: { tono: 'aviso', texto: 'Invitado' },
};

export function infoEstado(estado) {
  return ESTADOS[estado] || { tono: 'neutro', texto: estado ? String(estado).replace(/_/g, ' ') : '—' };
}

/** Tono de un estado (acento | aviso | alerta | neutro). */
export function tonoDe(estado) {
  return infoEstado(estado).tono;
}
