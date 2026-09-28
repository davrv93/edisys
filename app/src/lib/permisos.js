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
  // El administrador aprueba lo que está bajo el umbral; por encima decide la junta (lo valida el API).
  administrador: TODOS.filter((p) => p !== 'portal.ver'),
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
      return 'medidores';
    case 'tecnico':
      return 'mantenimiento';
    default:
      return 'inicio';
  }
}

// Menú lateral (escritorio) y pestañas (móvil). Cada ítem apunta a una página Astro (+ query opcional).
const ITEMS = {
  inicio: { pagina: 'inicio', etiqueta: 'Resumen', corta: 'Inicio', icono: 'inicio', permiso: 'dashboard.ver' },
  portal: { pagina: 'portal', etiqueta: 'Mi portal', corta: 'Inicio', icono: 'inicio', permiso: 'portal.ver' },
  balance: { pagina: 'balance', etiqueta: 'Balance', corta: 'Balance', icono: 'balance', permiso: 'balance.ver' },
  recibos: { pagina: 'recibos', etiqueta: 'Recibos y cobranza', corta: 'Recibos', icono: 'recibo', permiso: 'recibos.ver' },
  unidades: { pagina: 'unidades', etiqueta: 'Unidades y propietarios', corta: 'Unidades', icono: 'edificio', permiso: 'unidades.ver' },
  reservas: { pagina: 'reservas', etiqueta: 'Reservas', corta: 'Reservas', icono: 'calendario', permiso: 'reservas.ver' },
  reservar: { pagina: 'reservas', etiqueta: 'Reservar', corta: 'Reservas', icono: 'calendario', permiso: 'reservas.crear' },
  medidores: { pagina: 'medidores', etiqueta: 'Medidores', corta: 'Lecturas', icono: 'medidor', permiso: ['lecturas.registrar', 'lecturas.ver'] },
  mantenimiento: { pagina: 'mantenimiento', etiqueta: 'Mantenimiento', corta: 'Mantenim.', icono: 'herramienta', permiso: 'incidencias.ver' },
  aprobaciones: { pagina: 'mantenimiento', etiqueta: 'Aprobaciones', corta: 'Aprobar', icono: 'herramienta', permiso: 'trabajos.aprobar' },
  trabajos: { pagina: 'mantenimiento', etiqueta: 'Mis trabajos', corta: 'Trabajos', icono: 'herramienta', permiso: null },
  reportar: { pagina: 'mantenimiento', query: { reportar: 1 }, etiqueta: 'Reportar incidencia', corta: 'Reportar', icono: 'camara', permiso: 'incidencias.reportar' },
  whatsapp: { pagina: 'whatsapp', etiqueta: 'WhatsApp', corta: 'WhatsApp', icono: 'whatsapp', permiso: 'whatsapp.ver' },
  chatbot: { pagina: 'chatbot', etiqueta: 'Simulador del chatbot', corta: 'Chatbot', icono: 'robot', permiso: 'whatsapp.ver' },
  analitica: { pagina: 'analitica', etiqueta: 'Analítica', corta: 'Analítica', icono: 'grafico', permiso: 'analitica.ver' },
  roles: { pagina: 'roles', etiqueta: 'Roles y permisos', corta: 'Roles', icono: 'llave', permiso: 'roles.administrar' },
};

const MENU_POR_ROL = {
  administrador: {
    lateral: ['inicio', 'balance', 'recibos', 'unidades', 'reservas', 'medidores', 'mantenimiento', 'whatsapp', 'chatbot', 'analitica', 'roles'],
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
    lateral: ['medidores', 'reportar'],
    movil: ['medidores', 'reportar'],
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
