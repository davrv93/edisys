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

const TODOS = [
  'dashboard.ver', 'balance.ver', 'balance.ver_documentos', 'egresos.registrar',
  'recibos.ver', 'recibos.emitir', 'pagos.registrar',
  'unidades.ver', 'unidades.editar', 'unidades.importar',
  'reservas.ver', 'reservas.crear', 'reservas.administrar',
  'lecturas.ver', 'lecturas.registrar', 'lecturas.aprobar_reparto',
  'incidencias.ver', 'incidencias.reportar', 'incidencias.validar', 'trabajos.presupuestar', 'trabajos.aprobar',
  'roles.administrar', 'whatsapp.ver', 'whatsapp.enviar', 'whatsapp.configurar', 'analitica.ver', 'portal.ver',
];

/** Permisos por defecto si /yo no los trae (semilla de la guía). */
export const PERMISOS_POR_ROL = {
  superadmin: TODOS,
  administrador: TODOS.filter((p) => p !== 'trabajos.aprobar' && p !== 'portal.ver'),
  junta: ['dashboard.ver', 'balance.ver', 'balance.ver_documentos', 'recibos.ver', 'unidades.ver', 'reservas.ver', 'lecturas.ver', 'incidencias.ver', 'trabajos.aprobar', 'analitica.ver'],
  propietario: ['portal.ver', 'balance.ver', 'recibos.ver', 'reservas.crear', 'incidencias.reportar'],
  inquilino: ['portal.ver', 'reservas.crear', 'incidencias.reportar'],
  operario: ['lecturas.registrar', 'incidencias.reportar', 'reservas.ver'],
  tecnico: ['incidencias.ver', 'trabajos.avance'],
};

/** Pantalla de aterrizaje por rol (§3 · 01). */
export function destinoPorRol(rol) {
  switch (rol) {
    case 'propietario':
    case 'inquilino':
      return 'portal';
    case 'operario':
      return 'lecturas';
    case 'tecnico':
      return 'mantenimiento';
    default:
      return 'inicio';
  }
}

// Menú lateral (escritorio) y pestañas (móvil). `movil` fija el orden de la barra inferior.
const ITEMS = {
  inicio: { ruta: 'inicio', etiqueta: 'Resumen', corta: 'Inicio', icono: 'inicio', permiso: 'dashboard.ver' },
  portal: { ruta: 'portal', etiqueta: 'Mi portal', corta: 'Inicio', icono: 'inicio', permiso: 'portal.ver' },
  balance: { ruta: 'balance', etiqueta: 'Balance', corta: 'Balance', icono: 'balance', permiso: 'balance.ver' },
  recibos: { ruta: 'recibos', etiqueta: 'Recibos y cobranza', corta: 'Recibos', icono: 'recibo', permiso: 'recibos.ver' },
  unidades: { ruta: 'unidades', etiqueta: 'Unidades y propietarios', corta: 'Unidades', icono: 'edificio', permiso: 'unidades.ver' },
  reservas: { ruta: 'reservas', etiqueta: 'Reservas', corta: 'Reservas', icono: 'calendario', permiso: 'reservas.ver' },
  reservar: { ruta: 'reservas/nueva', etiqueta: 'Reservar', corta: 'Reservas', icono: 'calendario', permiso: 'reservas.crear' },
  lecturas: { ruta: 'lecturas', etiqueta: 'Medidores', corta: 'Lecturas', icono: 'medidor', permiso: null },
  mantenimiento: { ruta: 'mantenimiento', etiqueta: 'Mantenimiento', corta: 'Mantenim.', icono: 'herramienta', permiso: 'incidencias.ver' },
  aprobaciones: { ruta: 'mantenimiento', etiqueta: 'Aprobaciones', corta: 'Aprobar', icono: 'herramienta', permiso: 'trabajos.aprobar' },
  trabajos: { ruta: 'mantenimiento', etiqueta: 'Mis trabajos', corta: 'Trabajos', icono: 'herramienta', permiso: null },
  reportar: { ruta: 'mantenimiento/reportar', etiqueta: 'Reportar incidencia', corta: 'Reportar', icono: 'camara', permiso: 'incidencias.reportar' },
  whatsapp: { ruta: 'whatsapp', etiqueta: 'WhatsApp', corta: 'WhatsApp', icono: 'mensaje', permiso: 'whatsapp.ver' },
  analitica: { ruta: 'analitica', etiqueta: 'Analítica', corta: 'Analítica', icono: 'grafico', permiso: 'analitica.ver' },
  roles: { ruta: 'ajustes/usuarios', etiqueta: 'Roles y permisos', corta: 'Roles', icono: 'llave', permiso: 'roles.administrar' },
};

const MENU_POR_ROL = {
  administrador: {
    lateral: ['inicio', 'balance', 'recibos', 'unidades', 'reservas', 'lecturas', 'mantenimiento', 'whatsapp', 'analitica', 'roles'],
    movil: ['inicio', 'balance', 'recibos', 'mantenimiento'],
  },
  junta: {
    lateral: ['inicio', 'balance', 'recibos', 'unidades', 'reservas', 'aprobaciones', 'analitica'],
    movil: ['inicio', 'balance', 'aprobaciones'],
  },
  propietario: {
    lateral: ['portal', 'recibos', 'reservar', 'reportar', 'balance'],
    movil: ['portal', 'recibos', 'reservar', 'reportar'],
  },
  inquilino: {
    lateral: ['portal', 'reservar', 'reportar'],
    movil: ['portal', 'reservar', 'reportar'],
  },
  operario: {
    lateral: ['lecturas', 'reportar'],
    movil: ['lecturas', 'reportar'],
  },
  tecnico: {
    lateral: ['trabajos'],
    movil: ['trabajos'],
  },
};
MENU_POR_ROL.superadmin = MENU_POR_ROL.administrador;

/**
 * Menú del rol, filtrado por los permisos efectivos.
 * Devuelve { lateral, movil, mas } donde `mas` son los ítems que no caben en la barra móvil.
 */
export function menuPara(rol, tiene = () => true) {
  const def = MENU_POR_ROL[rol] || MENU_POR_ROL.propietario;
  const visible = (id) => {
    const it = ITEMS[id];
    return it && (!it.permiso || tiene(it.permiso));
  };
  const lateral = def.lateral.filter(visible).map((id) => ({ id, ...ITEMS[id] }));
  const movil = def.movil.filter(visible).map((id) => ({ id, ...ITEMS[id] }));
  const enMovil = new Set(movil.map((m) => m.ruta));
  const mas = lateral.filter((i) => !enMovil.has(i.ruta));
  return { lateral, movil, mas };
}
