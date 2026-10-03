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
  // personal: F1–F3 · colaboradores, asistencia con foto y almacén
  personal: { pagina: 'personal', etiqueta: 'Colaboradores', corta: 'Personal', icono: 'usuario', permiso: 'personal.ver' },
  asistencia: { pagina: 'asistencia', etiqueta: 'Asistencia y turnos', corta: 'Asistencia', icono: 'reloj', permiso: ['asistencia.marcar', 'asistencia.ver'] },
  almacen: { pagina: 'almacen', etiqueta: 'Almacén', corta: 'Almacén', icono: 'bandeja', permiso: 'almacen.ver' },
  roles: { pagina: 'roles', etiqueta: 'Roles y permisos', corta: 'Roles', icono: 'llave', permiso: 'roles.administrar' },
};

export const MENU_POR_ROL = {
  administrador: {
    lateral: ['inicio', 'balance', 'conciliacion', 'proveedores', 'fondos', 'informes', 'externos', 'vouchers', 'cobranzas', 'recibos', 'unidades', 'reservas', 'medidores', 'mantenimiento', 'documentos', 'whatsapp', 'chatbot', 'motor', 'analitica', 'roles', 'configuracion'],
    movil: ['inicio', 'recibos', 'mantenimiento', 'whatsapp'],
  },
  junta: {
    lateral: ['inicio', 'balance', 'recibos', 'unidades', 'reservas', 'aprobaciones', 'analitica', 'proveedores', 'fondos', 'externos'],
    movil: ['inicio', 'balance', 'aprobaciones'],
  },
  propietario: {
    lateral: ['portal', 'recibos', 'reservar', 'reportar', 'balance', 'documentos'],
    movil: ['portal', 'recibos', 'reservar', 'reportar'],
  },
  inquilino: {
    lateral: ['portal', 'reservar', 'reportar', 'documentos'],
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
// personal: F1–F3 (al final del lateral; la barra móvil no cambia)
MENU_POR_ROL.administrador.lateral.push('personal', 'asistencia', 'almacen');
MENU_POR_ROL.junta.lateral.push('personal', 'asistencia', 'almacen');
MENU_POR_ROL.propietario.lateral.push('personal');
MENU_POR_ROL.operario.lateral.push('asistencia', 'almacen');
MENU_POR_ROL.tecnico.lateral.push('asistencia');

/** Grupos del lateral: accesos comunes juntos y plegables. Todo ítem de ITEMS vive en un grupo. */
export const GRUPOS = [
  { id: 'panel', etiqueta: 'Panel', items: ['inicio', 'portal'] },
  { id: 'finanzas', etiqueta: 'Finanzas', items: ['balance', 'conciliacion', 'proveedores', 'fondos', 'informes', 'externos', 'vouchers', 'cobranzas', 'recibos', 'unidades', 'analitica'] },
  { id: 'operacion', etiqueta: 'Operación', items: ['reservas', 'reservar', 'medidores', 'mantenimiento', 'aprobaciones', 'trabajos', 'reportar'] },
  { id: 'comunicacion', etiqueta: 'Comunicación', items: ['whatsapp', 'chatbot', 'motor', 'documentos'] },
  { id: 'equipo', etiqueta: 'Personal', items: ['personal', 'asistencia', 'almacen'] }, // personal: F1–F3
  { id: 'ajustes', etiqueta: 'Ajustes', items: ['roles', 'configuracion'] },
];

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
