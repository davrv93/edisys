// Mapa único de estados (§2.1 y plan de la segunda pasada, bloque B).
// Ninguna pantalla decide colores de estado por su cuenta: insignias, columnas del kanban
// y leyendas de gráficos leen de aquí. Cada estado lleva SIEMPRE color, icono y texto:
// el color nunca es el único portador del significado (el icono lo refuerza, también para daltónicos).
//
// Tonos (regla de color):
//   acento  → acción / cobrado (petróleo)       alerta → crítico / deuda (rojo)
//   aviso   → pendiente, esperando (ámbar)      curso  → en curso (índigo)
//   hecho   → terminado, archivado (slate)      neutro → sin juicio (gris)

export const TONO_CLASES = {
  acento: 'bg-acento-suave text-acento border-acento-borde',
  aviso: 'bg-aviso-suave text-aviso-texto border-aviso-borde',
  alerta: 'bg-alerta-suave text-alerta border-alerta-borde',
  curso: 'bg-curso-suave text-curso-texto border-curso-borde',
  hecho: 'bg-hecho-suave text-hecho-texto border-hecho-borde',
  neutro: 'bg-superficie-2 text-texto-suave border-borde',
  solido: 'bg-acento text-white border-acento',
  oscuro: 'bg-tinta text-white border-tinta',
};

/** Color sólido del tono: puntos, barras de gráfico y el filo de las columnas del kanban. */
export const TONO_PUNTO = {
  acento: 'bg-acento',
  aviso: 'bg-aviso',
  alerta: 'bg-alerta',
  curso: 'bg-curso',
  hecho: 'bg-hecho',
  neutro: 'bg-texto-apoyo',
  solido: 'bg-acento',
  oscuro: 'bg-tinta',
};

/** Color de texto del tono (iconos y cifras sobre fondo blanco). */
export const TONO_TEXTO = {
  acento: 'text-acento',
  aviso: 'text-aviso',
  alerta: 'text-alerta',
  curso: 'text-curso',
  hecho: 'text-hecho-texto',
  neutro: 'text-texto-suave',
  solido: 'text-acento',
  oscuro: 'text-tinta',
};

export const ESTADOS = {
  // Recibos y pagos
  borrador: { tono: 'neutro', icono: 'borrador', texto: 'Borrador' },
  emitido: { tono: 'aviso', icono: 'reloj', texto: 'Pendiente' },
  pendiente: { tono: 'aviso', icono: 'reloj', texto: 'Pendiente' },
  pagado: { tono: 'acento', icono: 'pagado', texto: 'Pagado' },
  pagado_parcial: { tono: 'aviso', icono: 'parcial', texto: 'Pago parcial' },
  parcial: { tono: 'aviso', icono: 'parcial', texto: 'Parcial' },
  vencido: { tono: 'alerta', icono: 'moroso', texto: 'Vencido' },
  anulado: { tono: 'hecho', icono: 'anulado', texto: 'Anulado' },
  pendiente_validacion: { tono: 'aviso', icono: 'reloj_arena', texto: 'En revisión' },
  validado_pago: { tono: 'acento', icono: 'voucher', texto: 'Validado' },
  moroso: { tono: 'alerta', icono: 'moroso', texto: 'Moroso' },
  al_dia: { tono: 'acento', icono: 'hecho', texto: 'Al día' },
  // Reservas
  confirmada: { tono: 'acento', icono: 'calendario', texto: 'Confirmada' },
  pendiente_pago: { tono: 'aviso', icono: 'temporizador', texto: 'Retenida 15 min' },
  hoy: { tono: 'curso', icono: 'agenda', texto: 'Hoy' },
  cancelada: { tono: 'hecho', icono: 'cancelado', texto: 'Cancelada' },
  vencida: { tono: 'hecho', icono: 'reloj', texto: 'Liberada' },
  no_show: { tono: 'hecho', icono: 'no_show', texto: 'No se presentó' },
  bloqueo: { tono: 'neutro', icono: 'candado', texto: 'Bloqueo' },
  // Lecturas
  pendiente_lectura: { tono: 'neutro', icono: 'circulo', texto: 'Pendiente' },
  leida: { tono: 'acento', icono: 'check', texto: 'Leída' },
  alerta_lectura: { tono: 'aviso', icono: 'alerta', texto: 'Con alerta' },
  PICO: { tono: 'aviso', icono: 'pico', texto: 'Pico de consumo' },
  NEGATIVO: { tono: 'alerta', icono: 'negativo', texto: 'Consumo negativo' },
  // Mantenimiento
  reportado: { tono: 'aviso', icono: 'reloj', texto: 'Reportado' },
  validado: { tono: 'curso', icono: 'check', texto: 'Validado' },
  presupuestado: { tono: 'aviso', icono: 'junta', texto: 'Informe y costos' },
  aprobado: { tono: 'acento', icono: 'voucher', texto: 'Aprobado' },
  en_ejecucion: { tono: 'curso', icono: 'cargando', texto: 'En ejecución' },
  terminado: { tono: 'hecho', icono: 'hecho', texto: 'Terminado' },
  rechazado: { tono: 'alerta', icono: 'cancelado', texto: 'Rechazado' },
  descartado: { tono: 'hecho', icono: 'archivado', texto: 'Descartado' },
  // Criticidad
  critica: { tono: 'alerta', icono: 'critico', texto: 'Crítico' },
  media: { tono: 'aviso', icono: 'punto', texto: 'Medio' },
  baja: { tono: 'neutro', icono: 'circulo', texto: 'Bajo' },
  // WhatsApp
  en_cola: { tono: 'neutro', icono: 'reloj', texto: 'En cola' },
  enviado: { tono: 'acento', icono: 'check', texto: 'Enviado' },
  entregado: { tono: 'acento', icono: 'doble_check', texto: 'Entregado' },
  leido: { tono: 'acento', icono: 'doble_check', texto: 'Leído' },
  fallido: { tono: 'alerta', icono: 'alerta', texto: 'Fallido' },
  error: { tono: 'alerta', icono: 'alerta', texto: 'Error' },
  simulado: { tono: 'aviso', icono: 'simulado', texto: 'Simulado' },
  recibido: { tono: 'neutro', icono: 'entrante', texto: 'Recibido' },
  // Usuarios
  activo: { tono: 'acento', icono: 'hecho', texto: 'Activo' },
  inactivo: { tono: 'hecho', icono: 'inactivo', texto: 'Inactivo' },
  invitado: { tono: 'aviso', icono: 'correo', texto: 'Invitado' },
};

/** Estados que el plan exige cubrir (prueba del mapa). */
export const ESTADOS_DEL_PLAN = [
  'reportado', 'validado', 'presupuestado', 'aprobado', 'en_ejecucion', 'terminado', 'rechazado', 'descartado',
  'pagado', 'vencido', 'parcial', 'moroso', 'simulado', 'enviado', 'error',
];

export function infoEstado(estado) {
  return ESTADOS[estado] || { tono: 'neutro', icono: 'info', texto: estado ? String(estado).replace(/_/g, ' ') : '—' };
}

/** Tono de un estado (acento | aviso | alerta | curso | hecho | neutro). */
export function tonoDe(estado) {
  return infoEstado(estado).tono;
}
