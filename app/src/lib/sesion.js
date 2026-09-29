// Normaliza la respuesta de GET /api/v1/yo a una forma única para la app.
// Contrato de la guía (§2.4): { usuario, roles_por_edificio, permisos, edificio_actual, menu }.
// Se toleran variantes razonables (arreglo u objeto por edificio) porque el API lo construye otro equipo.
import { PERMISOS_POR_ROL, ROLES } from './permisos.js';

const PRIORIDAD = ROLES; // superadmin > administrador > junta > propietario > inquilino > operario > tecnico

export function rolPrincipal(roles) {
  const lista = (Array.isArray(roles) ? roles : [roles]).filter(Boolean);
  return PRIORIDAD.find((r) => lista.includes(r)) || lista[0] || 'propietario';
}

function edificiosDe(r) {
  const out = new Map();
  const agregar = (e) => {
    if (!e) return;
    const id = e.edificio_id ?? e.id ?? e.eid;
    if (id == null) return;
    const previo = out.get(String(id)) || { id, roles: [] };
    const roles = [...previo.roles, ...(Array.isArray(e.roles) ? e.roles : e.rol ? [e.rol] : [])];
    out.set(String(id), {
      ...previo,
      id,
      nombre: e.edificio_nombre || (typeof e.edificio === 'string' ? e.edificio : null) || e.nombre || previo.nombre || `Edificio ${id}`,
      unidades: e.unidades_total ?? e.n_unidades ?? (typeof e.unidades === 'number' ? e.unidades : previo.unidades),
      periodo_abierto: e.periodo_abierto || previo.periodo_abierto,
      roles: [...new Set(roles)],
    });
  };
  const rpe = r.roles_por_edificio ?? r.usuario?.roles_por_edificio;
  if (Array.isArray(rpe)) rpe.forEach(agregar);
  else if (rpe && typeof rpe === 'object') {
    for (const [id, v] of Object.entries(rpe)) {
      if (typeof v === 'string' || Array.isArray(v)) agregar({ id, roles: Array.isArray(v) ? v : [v] });
      else agregar({ id, ...v });
    }
  }
  (r.edificios || []).forEach(agregar);
  if (r.edificio_actual) agregar(r.edificio_actual);
  return [...out.values()];
}

export function normalizarYo(r, eidPreferido) {
  const u = r?.usuario || r?.user || r || {};
  const edificios = edificiosDe(r || {});
  const actualId =
    (eidPreferido != null && edificios.some((e) => String(e.id) === String(eidPreferido)) && eidPreferido) ||
    r?.edificio_actual?.id ||
    edificios[0]?.id ||
    null;
  const edificio = edificios.find((e) => String(e.id) === String(actualId)) || null;
  const rolesGlobales = u.roles || (u.rol ? [u.rol] : []);
  const rol = rolPrincipal(edificio?.roles?.length ? edificio.roles : rolesGlobales);

  let permisos = r?.permisos;
  if (permisos && !Array.isArray(permisos) && typeof permisos === 'object') {
    permisos = permisos[String(actualId)] || permisos[actualId] || Object.values(permisos).flat();
  }
  if (!Array.isArray(permisos) || permisos.length === 0) permisos = PERMISOS_POR_ROL[rol] || [];

  const nombre = u.nombre || u.name || u.correo || 'Usuario';
  return {
    usuario: {
      id: u.id,
      nombre,
      correo: u.correo || u.email || '',
      telefono: u.telefono || u.celular || '',
      iniciales: iniciales(nombre),
    },
    rol,
    roles: edificio?.roles || rolesGlobales,
    permisos: new Set(permisos),
    edificios,
    edificio,
    unidades: r?.unidades || u.unidades || r?.edificio_actual?.unidades_propias || [],
  };
}

export function iniciales(nombre = '') {
  const p = String(nombre).trim().split(/\s+/).filter(Boolean);
  return ((p[0]?.[0] || '') + (p[1]?.[0] || '')).toUpperCase() || 'U';
}

export function tienePermiso(sesion, permiso) {
  if (!permiso) return true;
  if (!sesion) return false;
  if (sesion.rol === 'superadmin') return true;
  const lista = Array.isArray(permiso) ? permiso : [permiso];
  return lista.some((p) => sesion.permisos.has(p));
}
