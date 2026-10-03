// Roles, permisos por defecto y menú por rol (§2.5).
// Las guardas de React solo esconden: el API valida todo (§0.3).

export const ROLES = ['superadmin', 'administrador', 'junta', 'propietario', 'inquilino', 'operario', 'tecnico'];

export const NOMBRE_ROL = {
  superadmin: 'Superadmin',
  administrador: 'Administrador',
  junta: 'Junta',
  propietario: 'Propietario',
  inquilino: 'Inquilino',
  operario: 'Operario',
  tecnico: 'Técnico',
};

// Espejo de la semilla del API (api/migrations/0001_base.sql). Solo se usa si /yo no trae permisos.
// El API es quien manda.
const TODOS = [
  'analitica.ver', 'areas.administrar', 'auditoria.ver', 'balance.conciliar', 'balance.ver', 'balance.ver_documentos', 'chatbot.probar',
  'comprobantes.emitir', 'facturacion.configurar',
  'dashboard.ver', 'edificio.editar', 'edificio.ver', 'egresos.registrar', 'incidencias.reportar', 'incidencias.validar',
  'incidencias.ver', 'lecturas.aprobar_reparto', 'lecturas.corregir',  'lecturas.registrar', 'lecturas.ver', 'motor.administrar', 'morosidad.ver',
  'pagos.informar', 'pagos.registrar', 'pagos.validar', 'periodos.administrar', 'portal.ver', 'recibos.emitir', 'recibos.ver',
  'reservas.administrar', 'reservas.crear', 'reservas.ver', 'roles.administrar', 'trabajos.aprobar', 'trabajos.ejecutar',
  'trabajos.presupuestar', 'trabajos.votar', 'unidades.editar', 'unidades.importar', 'unidades.ver', 'usuarios.ver',
  'whatsapp.configurar', 'whatsapp.enviar', 'whatsapp.ver',
  'marca.configurar', 'recibos.plantilla', 'configuracion.ver', 'configuracion.editar', // marca: I1/I2/I5
];

export const PERMISOS_POR_ROL = {
  superadmin: TODOS,
  administrador: TODOS.filter((p) => !['trabajos.votar', 'pagos.informar', 'portal.ver'].includes(p)),
  junta: ['dashboard.ver', 'balance.ver', 'balance.ver_documentos', 'edificio.ver', 'unidades.ver', 'recibos.ver', 'morosidad.ver', 'reservas.ver', 'lecturas.ver', 'incidencias.reportar', 'incidencias.ver', 'trabajos.aprobar', 'trabajos.votar', 'analitica.ver', 'usuarios.ver', 'whatsapp.ver'],
  propietario: ['balance.ver', 'balance.ver_documentos', 'edificio.ver', 'recibos.ver', 'pagos.informar', 'reservas.ver', 'reservas.crear', 'incidencias.reportar', 'incidencias.ver', 'portal.ver'],
  inquilino: ['edificio.ver', 'reservas.ver', 'reservas.crear', 'incidencias.reportar', 'incidencias.ver', 'portal.ver'],
  operario: ['edificio.ver', 'lecturas.ver', 'lecturas.registrar', 'reservas.ver', 'incidencias.reportar', 'incidencias.ver'],
  tecnico: ['edificio.ver', 'incidencias.reportar', 'incidencias.ver', 'trabajos.ejecutar'],
};

/** ¿Ve el calendario de reservas (staff) o el flujo para reservar (residente)? */
export function veCalendarioReservas(tiene) {
  return tiene('reservas.administrar') || !tiene('reservas.crear');
}

/** ¿Ve el tablero de mantenimiento? Solo quien gestiona trabajos; el resto, el formulario para reportar. */
export function veTableroMantenimiento(tiene) {
  return ['incidencias.validar', 'trabajos.presupuestar', 'trabajos.aprobar', 'trabajos.votar', 'trabajos.ejecutar'].some((p) => tiene(p));
}

/** Pantalla de aterrizaje por rol (§3 · 01). */
export function destinoPorRol(rol) {
  switch (rol) {
    case 'propietario':
    case 'inquilino':
      return 'portal';
    case 'operario':
      return 'medidores';
    case 'tecnico':
      return 'mantenimiento';
    default:
      return 'inicio';
  }
}

// Menú lateral (escritorio) y pestañas (móvil). Cada ítem apunta a una página Astro (+ query opcional).
// Para añadir una sección: una entrada aquí (pagina, etiqueta, corta, icono, permiso) y su id en MENU_POR_ROL.
// Iconos previstos para lo que llega de la rama de pendientes: «conciliacion» (Landmark) y «facturacion» (FileCheck2).
export const ITEMS = {
  inicio: { pagina: 'inicio', etiqueta: 'Resumen', corta: 'Resumen', icono: 'inicio', permiso: 'dashboard.ver' },
  portal: { pagina: 'portal', etiqueta: 'Mi portal', corta: 'Inicio', icono: 'inicio', permiso: 'portal.ver' },
  balance: { pagina: 'balance', etiqueta: 'Balance', corta: 'Balance', icono: 'balance', permiso: 'balance.ver' },
  recibos: { pagina: 'recibos', etiqueta: 'Recibos y cobranza', corta: 'Recibos', icono: 'recibo', permiso: 'recibos.ver' },
  unidades: { pagina: 'unidades', etiqueta: 'Unidades y propietarios', corta: 'Unidades', icono: 'edificio', permiso: 'unidades.ver' },
  reservas: { pagina: 'reservas', etiqueta: 'Reservas', corta: 'Reservas', icono: 'calendario', permiso: 'reservas.ver' },
  reservar: { pagina: 'reservas', etiqueta: 'Reservar', corta: 'Reservas', icono: 'calendario', permiso: 'reservas.crear' },
  medidores: { pagina: 'medidores', etiqueta: 'Medidores', corta: 'Lecturas', icono: 'medidor', permiso: ['lecturas.registrar', 'lecturas.ver'] },
  mantenimiento: { pagina: 'mantenimiento', etiqueta: 'Mantenimiento', corta: 'Mantenim.', icono: 'herramienta', permiso: 'incidencias.ver' },
  aprobaciones: { pagina: 'mantenimiento', etiqueta: 'Aprobaciones', corta: 'Aprobar', icono: 'herramienta', permiso: 'trabajos.aprobar' },
  trabajos: { pagina: 'mantenimiento', etiqueta: 'Mis trabajos', corta: 'Trabajos', icono: 'herramienta', permiso: 'trabajos.ejecutar' },
  reportar: { pagina: 'mantenimiento', query: { reportar: 1 }, etiqueta: 'Reportar incidencia', corta: 'Reportar', icono: 'camara', permiso: 'incidencias.reportar' },
  whatsapp: { pagina: 'whatsapp', etiqueta: 'WhatsApp', corta: 'WhatsApp', icono: 'whatsapp', permiso: 'whatsapp.ver' },
  chatbot: { pagina: 'chatbot', etiqueta: 'Simulador del chatbot', corta: 'Chatbot', icono: 'robot', permiso: 'chatbot.probar' },
  motor: { pagina: 'motor', etiqueta: 'Motor conversacional', corta: 'Motor', icono: 'llave', permiso: 'motor.administrar' },
  analitica: { pagina: 'analitica', etiqueta: 'Analítica', corta: 'Analítica', icono: 'grafico', permiso: 'analitica.ver' },
  configuracion: { pagina: 'configuracion', etiqueta: 'Configuración', corta: 'Config.', icono: 'engranaje', permiso: 'facturacion.configurar' },
  conciliacion: { pagina: 'conciliacion', etiqueta: 'Conciliación bancaria', corta: 'Banco', icono: 'balance', permiso: 'balance.conciliar' },
  proveedores: { pagina: 'proveedores', etiqueta: 'Proveedores y cuentas por pagar', corta: 'Proveed.', icono: 'edificio', permiso: 'proveedores.ver' },
  fondos: { pagina: 'fondos', etiqueta: 'Trazabilidad de fondos', corta: 'Fondos', icono: 'balance', permiso: 'fondos.ver' },
  informes: { pagina: 'informes', etiqueta: 'Informes económicos', corta: 'Informes', icono: 'grafico', permiso: 'balance.ver' },
  externos: { pagina: 'externos', etiqueta: 'Recibos e ingresos externos', corta: 'Externos', icono: 'recibo', permiso: 'externos.ver' },
  vouchers: { pagina: 'vouchers', etiqueta: 'Vouchers y cuentas bancarias', corta: 'Vouchers', icono: 'recibo', permiso: ['pagos.registrar', 'pagos.informar'] },
  cobranzas: { pagina: 'cobranzas', etiqueta: 'Cobranzas sin identificar', corta: 'Cobranzas', icono: 'entrante', permiso: 'cobranzas.ver' },
  documentos: { pagina: 'documentos', etiqueta: 'Documentos', corta: 'Documentos', icono: 'recibo', permiso: 'documentos.ver' },
  // recaudacion: A1, A2
  recaudadora: { pagina: 'recaudadora', etiqueta: 'Recaudadora', corta: 'Recaudad.', icono: 'entrante', permiso: 'recaudacion.ver' },
  cobranzaMasiva: { pagina: 'cobranza-masiva', etiqueta: 'Cobranza masiva (CREP/CDPG)', corta: 'CREP', icono: 'conciliacion', permiso: 'recaudacion.ver' },
  // deuda: B3, D1, D2 y D3
  cuentasCobrar: { pagina: 'cuentas-cobrar', etiqueta: 'Cuentas por cobrar', corta: 'Por cobrar', icono: 'balance', permiso: 'morosidad.ver' },
  estadoCuenta: { pagina: 'cuentas-cobrar', etiqueta: 'Estado de cuenta', corta: 'Mi cuenta', icono: 'recibo', permiso: 'recibos.ver' },
  acuerdos: { pagina: 'acuerdos', etiqueta: 'Acuerdos de pago', corta: 'Acuerdos', icono: 'documento', permiso: 'acuerdos.ver' },
  morosos: { pagina: 'morosos', etiqueta: 'Morosos, puntualidad y avisos', corta: 'Morosos', icono: 'moroso', permiso: 'morosidad.ver' },
  // comunicacion: E2–E5
  anuncios: { pagina: 'anuncios', etiqueta: 'Anuncios', corta: 'Anuncios', icono: 'mensaje', permiso: 'anuncios.ver' },
  bandeja: { pagina: 'bandeja', etiqueta: 'Bandeja de salida', corta: 'Bandeja', icono: 'bandeja', permiso: ['recibos.emitir', 'telegram.configurar'] },
  ayuda: { pagina: 'ayuda', etiqueta: 'Ayuda y beneficios', corta: 'Ayuda', icono: 'info', permiso: 'contenido.ver' },
  roles: { pagina: 'roles', etiqueta: 'Roles y permisos', corta: 'Roles', icono: 'llave', permiso: 'roles.administrar' },
  // marca: I1/I2/I5
  marca: { pagina: 'marca', etiqueta: 'Marca y recibo', corta: 'Marca', icono: 'recibo', permiso: ['marca.configurar', 'recibos.plantilla'] },
  ajustes: { pagina: 'ajustes', etiqueta: 'Configuración del edificio', corta: 'Edificio', icono: 'engranaje', permiso: 'configuracion.ver' },
};

export const MENU_POR_ROL = {
  administrador: {
    lateral: ['inicio', 'balance', 'conciliacion', 'proveedores', 'fondos', 'informes', 'externos', 'vouchers', 'cobranzas', 'recibos', 'unidades', 'reservas', 'medidores', 'mantenimiento', 'documentos', 'whatsapp', 'chatbot', 'motor', 'analitica', 'roles', 'configuracion', 'marca', 'ajustes'],
    movil: ['inicio', 'recibos', 'mantenimiento', 'whatsapp'],
  },
  junta: {
    lateral: ['inicio', 'balance', 'recibos', 'unidades', 'reservas', 'aprobaciones', 'analitica', 'proveedores', 'fondos', 'externos', 'ajustes'],
    lateral: ['inicio', 'balance', 'conciliacion', 'proveedores', 'fondos', 'informes', 'externos', 'vouchers', 'cobranzas', 'recibos', 'cuentasCobrar', 'acuerdos', 'morosos', 'unidades', 'reservas', 'medidores', 'mantenimiento', 'documentos', 'whatsapp', 'chatbot', 'motor', 'analitica', 'roles', 'configuracion'],
    movil: ['inicio', 'recibos', 'mantenimiento', 'whatsapp'],
  },
  junta: {
    lateral: ['inicio', 'balance', 'recibos', 'unidades', 'reservas', 'aprobaciones', 'analitica', 'proveedores', 'fondos', 'externos', 'cuentasCobrar', 'acuerdos', 'morosos'],
    movil: ['inicio', 'balance', 'aprobaciones'],
  },
  propietario: {
    lateral: ['portal', 'recibos', 'estadoCuenta', 'reservar', 'reportar', 'balance', 'documentos'],
    lateral: ['inicio', 'balance', 'conciliacion', 'proveedores', 'fondos', 'informes', 'externos', 'vouchers', 'cobranzas', 'recibos', 'unidades', 'reservas', 'medidores', 'mantenimiento', 'documentos', 'whatsapp', 'chatbot', 'motor', 'analitica', 'roles', 'configuracion', 'anuncios', 'bandeja', 'ayuda'], // comunicacion: E2–E5
    movil: ['inicio', 'recibos', 'mantenimiento', 'whatsapp'],
  },
  junta: {
    lateral: ['inicio', 'balance', 'recibos', 'unidades', 'reservas', 'aprobaciones', 'analitica', 'proveedores', 'fondos', 'externos', 'anuncios', 'ayuda'], // comunicacion: E2, E5
    movil: ['inicio', 'balance', 'aprobaciones'],
  },
  propietario: {
    lateral: ['portal', 'recibos', 'reservar', 'reportar', 'balance', 'documentos', 'anuncios', 'ayuda'], // comunicacion: E2, E5
    movil: ['portal', 'recibos', 'reservar', 'reportar'],
  },
  inquilino: {
    lateral: ['portal', 'reservar', 'reportar', 'documentos', 'anuncios', 'ayuda'], // comunicacion: E2, E5
    movil: ['portal', 'reservar', 'reportar'],
  },
  operario: {
    lateral: ['medidores', 'reportar'],
    movil: ['medidores', 'reportar'],
  },
  tecnico: {
    lateral: ['trabajos'],
    movil: ['trabajos'],
  },
};
MENU_POR_ROL.superadmin = MENU_POR_ROL.administrador;

/** Grupos del lateral: accesos comunes juntos y plegables. Todo ítem de ITEMS vive en un grupo. */
export const GRUPOS = [
  { id: 'panel', etiqueta: 'Panel', items: ['inicio', 'portal'] },
  { id: 'finanzas', etiqueta: 'Finanzas', items: ['balance', 'conciliacion', 'proveedores', 'fondos', 'informes', 'externos', 'vouchers', 'cobranzas', 'recibos', 'cuentasCobrar', 'estadoCuenta', 'acuerdos', 'morosos', 'unidades', 'analitica'] },
  { id: 'operacion', etiqueta: 'Operación', items: ['reservas', 'reservar', 'medidores', 'mantenimiento', 'aprobaciones', 'trabajos', 'reportar'] },
  { id: 'comunicacion', etiqueta: 'Comunicación', items: ['whatsapp', 'chatbot', 'motor', 'documentos'] },
  { id: 'ajustes', etiqueta: 'Ajustes', items: ['roles', 'configuracion', 'marca', 'ajustes'] },
  { id: 'comunicacion', etiqueta: 'Comunicación', items: ['whatsapp', 'chatbot', 'motor', 'documentos', 'anuncios', 'bandeja', 'ayuda'] }, // comunicacion: E2–E5
  { id: 'ajustes', etiqueta: 'Ajustes', items: ['roles', 'configuracion'] },
];

// extras: J1/J2/I3 · encuestas (todos los roles del edificio), videollamadas (junta y administración)
// y dominio propio (vive como segunda vista de videollamadas, solo administración).
ITEMS.encuestas = { pagina: 'encuestas', etiqueta: 'Encuestas', corta: 'Encuestas', icono: 'votos', permiso: 'encuestas.ver' };
ITEMS.videollamadas = { pagina: 'videollamadas', etiqueta: 'Videollamadas', corta: 'Reuniones', icono: 'junta', permiso: ['videollamadas.ver', 'dominios.administrar'] };
MENU_POR_ROL.administrador.lateral.push('encuestas', 'videollamadas'); // superadmin comparte este objeto
MENU_POR_ROL.junta.lateral.push('encuestas', 'videollamadas');
MENU_POR_ROL.propietario.lateral.push('encuestas');
MENU_POR_ROL.inquilino.lateral.push('encuestas');
GRUPOS.find((g) => g.id === 'comunicacion').items.push('encuestas', 'videollamadas');
// reservas: H1 · check-in QR del conserje · H3 · configuración de áreas comunes.
// Se añaden en bloque propio (sin tocar las listas de arriba) para no chocar con otros módulos.
ITEMS.checkin = { pagina: 'checkin', etiqueta: 'Ingreso con QR', corta: 'Ingreso', icono: 'camara', permiso: 'reservas.checkin' };
ITEMS.areascomunes = { pagina: 'areascomunes', etiqueta: 'Áreas comunes', corta: 'Áreas', icono: 'engranaje', permiso: 'areas.administrar' };
MENU_POR_ROL.administrador.lateral.splice(MENU_POR_ROL.administrador.lateral.indexOf('reservas') + 1, 0, 'checkin', 'areascomunes');
MENU_POR_ROL.operario.lateral.push('checkin'); // en el celular sale en «Más»: la barra móvil del operario queda igual
GRUPOS.find((g) => g.id === 'operacion').items.splice(1, 0, 'checkin', 'areascomunes');
for (const rol of ['superadmin', 'administrador', 'operario']) {
  if (!PERMISOS_POR_ROL[rol].includes('reservas.checkin')) PERMISOS_POR_ROL[rol].push('reservas.checkin');
}
// recaudacion: A1, A2 — se insertan junto a «cobranzas» sin reescribir las listas de arriba.
for (const ids of [MENU_POR_ROL.administrador.lateral, GRUPOS.find((g) => g.id === 'finanzas').items]) {
  ids.splice(ids.indexOf('cobranzas') + 1, 0, 'recaudadora', 'cobranzaMasiva');
}
MENU_POR_ROL.junta.lateral.push('recaudadora');

/** Ítems ya filtrados por rol, repartidos en sus grupos (solo grupos con algo visible). */
export function gruposPara(items) {
  const porId = new Map(items.map((i) => [i.id, i]));
  const out = [];
  for (const g of GRUPOS) {
    const sus = g.items.map((id) => porId.get(id)).filter(Boolean);
    if (sus.length) out.push({ id: g.id, etiqueta: g.etiqueta, items: sus });
  }
  return out;
}

/**
 * Menú del rol, filtrado por los permisos efectivos.
 * Devuelve { lateral, movil, mas } donde `mas` son los ítems que no caben en la barra móvil.
 */
export function menuPara(rol, tiene = () => true) {
  const def = MENU_POR_ROL[rol] || MENU_POR_ROL.propietario;
  const visible = (id) => {
    const it = ITEMS[id];
    if (!it) return false;
    if (!it.permiso) return true;
    return Array.isArray(it.permiso) ? it.permiso.some((p) => tiene(p)) : tiene(it.permiso);
  };
  const lateral = def.lateral.filter(visible).map((id) => ({ id, ...ITEMS[id] }));
  const movil = def.movil.filter(visible).map((id) => ({ id, ...ITEMS[id] }));
  const enMovil = new Set(movil.map((m) => m.id));
  const mas = lateral.filter((i) => !enMovil.has(i.id));
  return { lateral, movil, mas };
}
