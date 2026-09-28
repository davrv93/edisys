// Servidor simulado para VITE_MOCK=1. Responde con la misma forma que el contrato del API.
// Estado en memoria: se reinicia al recargar la página.
import {
  EDIFICIO, EID, PERIODO, UNIDADES, RECIBOS, USUARIOS_MOCK, TECNICOS, ARBOL, DOCUMENTOS, INCIDENCIAS,
  AREAS, RESERVAS, FRANJAS, MENSAJES, PLANTILLAS, ANALITICA, m3DeAgua,
} from './datos.js';
import { ApiError } from '../lib/api.js';
import { PERMISOS_POR_ROL, NOMBRE_ROL } from '../lib/permisos.js';
import { filtrarIncidencias, puedeTransicionar } from '../lib/kanban.js';
import { formatearSoles } from '../lib/dinero.js';
import { diaLima, formatearHora } from '../lib/fechas.js';

const db = {
  recibos: structuredClone(RECIBOS),
  incidencias: structuredClone(INCIDENCIAS),
  reservas: structuredClone(RESERVAS),
  mensajes: structuredClone(MENSAJES),
  wa: { modo: 'simulado', url: '', instancia: '' },
  lecturas: null,
  sigInc: 20,
  sigRes: 413,
  sigMsg: 8,
};

function rolActual() {
  try {
    return window.localStorage.getItem('edisys.mock.rol') || 'administrador';
  } catch {
    return 'administrador';
  }
}

const espera = (ms = 180) => new Promise((r) => setTimeout(r, ms));
const error = (status, codigo, mensaje, extra = {}) => new ApiError(status, { error: { codigo, mensaje, ...extra } });

function inicializarLecturas() {
  db.lecturas = UNIDADES.map((u, i) => {
    const anterior = 800 + i * 17 + (u.id % 7);
    const consumo = Math.round(m3DeAgua(u.agua_cts) * 1000) / 1000;
    const pendiente = u.codigo === '603' || u.codigo === '604';
    return {
      medidor_id: 500 + i, unidad: `Dpto ${u.codigo}`, unidad_codigo: u.codigo, medidor: `A-${u.codigo}`, orden_ronda: i + 1,
      participacion_pct: u.participacion_pct,
      lectura_anterior: anterior, lectura_actual: pendiente ? null : Math.round((anterior + consumo) * 1000) / 1000,
      consumo: pendiente ? null : consumo, foto_url: null,
      estado: pendiente ? 'pendiente' : u.codigo === '402' ? 'alerta' : 'leida',
      alerta: u.codigo === '402' ? 'PICO' : null,
      media_3m: 13.5,
    };
  });
}

const RUTAS = [];
function en(metodo, patron, fn) {
  const claves = [];
  const re = new RegExp('^' + patron.replace(/:(\w+)/g, (_, k) => (claves.push(k), '([^/]+)')) + '$');
  RUTAS.push({ metodo, re, claves, fn });
}

export async function responderMock(metodo, ruta, op = {}) {
  await espera();
  const [camino] = ruta.split('?');
  for (const r of RUTAS) {
    if (r.metodo !== metodo) continue;
    const m = camino.match(r.re);
    if (!m) continue;
    const params = Object.fromEntries(r.claves.map((k, i) => [k, decodeURIComponent(m[i + 1])]));
    return structuredClone(await r.fn({ params, query: op.query || {}, body: op.body || {}, form: op.form }));
  }
  throw error(404, 'NO_ENCONTRADO', `Ruta simulada no definida: ${metodo} ${camino}`);
}

const leerForm = (form, k) => (form && form.get ? form.get(k) : undefined);

// --- Sesión ---
en('GET', '/yo', () => {
  const rol = rolActual();
  const u = USUARIOS_MOCK[rol] || USUARIOS_MOCK.administrador;
  return {
    usuario: { ...u, rol },
    roles_por_edificio: [{ edificio_id: EID, edificio_nombre: EDIFICIO.nombre, rol, unidades_total: 24, periodo_abierto: PERIODO }],
    permisos: PERMISOS_POR_ROL[rol],
    edificio_actual: { id: EID, nombre: EDIFICIO.nombre },
    unidades: rol === 'propietario' ? [{ id: 201, codigo: '201' }] : rol === 'inquilino' ? [{ id: 103, codigo: '103' }] : [],
  };
});
en('POST', '/auth/logout', () => null);
en('POST', '/auth/refresh', () => null);

// --- 03 Dashboard ---
en('GET', '/edificios/:eid/dashboard', ({ query }) => {
  if (query.periodo && query.periodo !== PERIODO) {
    return { periodo: query.periodo, vacio: true, kpis: null };
  }
  const pendientes = (db.lecturas || []).filter((l) => l.estado === 'pendiente').length || 2;
  return {
    periodo: PERIODO,
    kpis: {
      ingresos_cts: 1946000, egresos_cts: 1895000, saldo_cts: 51000, banco_cts: 3412000, emitido_cts: 2240000,
      morosidad: { pct: 13.1, unidades: 3, monto_cts: 294000 },
      egresos_rubros: 4, egresos_documentos: 42,
    },
    variacion_vs_mes_anterior: { ingresos_pct: -5.5, egresos_pct: 2.1, periodo_anterior: '2026-08' },
    cobranza: { emitidos: 24, pagados: 21, parciales: 0, pendientes: 3, emitido_cts: 2240000, cobrado_cts: 1946000, por_cobrar_cts: 294000 },
    egresos_por_rubro: ARBOL.hijos.egr.map((r) => ({ id: r.id, nombre: r.nombre, total_cts: r.total_cts })),
    ingresos_por_concepto: [
      { nombre: 'Cuotas de mantenimiento', total_cts: 1680000 },
      { nombre: 'Agua: medidores + común', total_cts: 500000 },
      { nombre: 'Alquiler de áreas comunes', total_cts: 60000 },
    ],
    tareas: { lecturas_pendientes: pendientes, vouchers_por_validar: 1, incidencias_por_validar: db.incidencias.filter((i) => i.estado === 'reportado').length, aprobaciones_pendientes: 1 },
    pendientes: [
      { tono: 'alerta', etiqueta: 'CRÍTICO · ESPERA A LA JUNTA', titulo: 'INC-014 Bomba de agua N.º 2', detalle: 'Presupuesto S/ 1.850,00 · 2 de 3 votos necesarios', ruta: 'mantenimiento' },
      { tono: 'aviso', etiqueta: 'VOUCHER POR VALIDAR', titulo: 'Reserva R-0412 · Dpto 201', detalle: 'Yape S/ 80,00 · Parrilla 2, sáb 3 oct', ruta: 'reservas' },
      { tono: 'neutro', etiqueta: 'MEDIO', titulo: `Lecturas de agua: ${pendientes} de 24 sin foto`, detalle: 'Corte del periodo el 30-09', ruta: 'medidores' },
    ],
    morosidad_unidades: [
      { unidad: 'Dpto 402', meses: 1, detalle: '1 mes · consumo con pico', reservas_bloqueadas: true, deuda_cts: 142000 },
      { unidad: 'Dpto 503', meses: 1, detalle: '1 mes', reservas_bloqueadas: true, deuda_cts: 76000 },
      { unidad: 'Dpto 104', meses: 1, detalle: '1 mes', reservas_bloqueadas: true, deuda_cts: 76000 },
    ],
    ingresos_reservas_cts: 60000,
    reservas_mes: { cantidad: 6, detalle: 'Parrillas 4 · Sala común 2' },
  };
});

// --- 04 Balance ---
en('GET', '/edificios/:eid/balance', ({ query }) => {
  if (query.periodo && query.periodo !== PERIODO) return { kpis: null, raiz: null };
  return {
    kpis: { ingresos_cts: 1946000, egresos_cts: 1895000, saldo_cts: 51000, banco_cts: 3412000, emitido_cts: 2240000, morosidad: { pct: 13.1, unidades: 3, monto_cts: 294000 } },
    raiz: { ...ARBOL.raiz, hijos: ARBOL.hijos.raiz },
    conciliacion: 'Conciliado con el extracto bancario del 30-09-2026: 64 de 66 movimientos cuadran. 2 depósitos sin código de operación.',
  };
});
en('GET', '/edificios/:eid/balance/nodos/:nodo', ({ params }) => {
  const hijos = ARBOL.hijos[params.nodo];
  if (!hijos) throw error(404, 'NODO_NO_EXISTE', 'Ese nodo no existe.');
  const padre = Object.values(ARBOL.hijos).flat().find((n) => n.id === params.nodo);
  const rol = rolActual();
  return hijos.map((h) => {
    const pct = padre && padre.total_cts && h.tipo !== 'documento' ? Math.round((h.total_cts / padre.total_cts) * 1000) / 10 : h.pct_padre;
    const oculto = h.tipo === 'documento' && (rol === 'propietario' || rol === 'inquilino') && DOCUMENTOS[h.doc_id]?.solo_junta;
    return { ...h, pct_padre: pct, ...(oculto ? { nombre: 'Documento disponible para la junta', doc_id: null, bloqueado: true } : {}) };
  });
});
en('GET', '/edificios/:eid/balance/documentos/:doc', ({ params }) => {
  const d = DOCUMENTOS[params.doc];
  if (!d) throw error(404, 'NO_ENCONTRADO', 'No encontramos este documento.');
  return { ...d, url_firmada: null };
});

// --- 05 Recibos ---
en('GET', '/edificios/:eid/recibos', ({ query }) => {
  let l = db.recibos;
  const rol = rolActual();
  if (query.mios || rol === 'propietario') l = l.filter((r) => r.unidad === 'Dpto 201');
  if (query.estado) l = l.filter((r) => (query.estado === 'vencido' ? r.estado === 'vencido' : r.estado === query.estado));
  if (query.buscar) {
    const q = String(query.buscar).toLowerCase();
    l = l.filter((r) => r.unidad.toLowerCase().includes(q) || r.propietario.toLowerCase().includes(q));
  }
  const pagina = Number(query.pagina || 1);
  const por = Number(query.por_pagina || 25);
  const conteos = { todos: db.recibos.length, pagado: db.recibos.filter((r) => r.estado === 'pagado').length, vencido: db.recibos.filter((r) => r.estado === 'vencido').length };
  return { datos: l.slice((pagina - 1) * por, pagina * por).map(({ lineas: _l, pagos: _p, ...r }) => r), total: l.length, pagina, conteos };
});
en('GET', '/edificios/:eid/recibos/:rid', ({ params }) => {
  const r = db.recibos.find((x) => String(x.id) === params.rid);
  if (!r) throw error(404, 'NO_ENCONTRADO', 'No encontramos este recibo.');
  return r;
});
en('POST', '/edificios/:eid/recibos/:rid/pagos', ({ params, form }) => {
  const r = db.recibos.find((x) => String(x.id) === params.rid);
  const monto = Number(leerForm(form, 'monto_cts'));
  const codigo = leerForm(form, 'codigo_operacion');
  if (db.recibos.some((x) => x.pagos.some((p) => p.codigo_operacion === codigo && codigo))) throw error(409, 'PAGO_DUPLICADO', 'Ese código de operación ya está registrado.');
  if (!monto || monto <= 0) throw error(422, 'VALIDACION', 'Revisa el monto.', { campos: { monto_cts: 'El monto debe ser mayor a cero.' } });
  const esAdmin = rolActual() === 'administrador';
  r.pagos.push({ id: Date.now(), fecha: leerForm(form, 'fecha'), medio: leerForm(form, 'medio'), codigo_operacion: codigo, monto_cts: monto, estado: esAdmin ? 'validado' : 'pendiente_validacion' });
  if (esAdmin) {
    r.pagado_cts += monto;
    r.saldo_cts = Math.max(0, r.total_cts - r.pagado_cts);
    r.estado = r.saldo_cts === 0 ? 'pagado' : 'pagado_parcial';
  }
  return { id: Date.now(), estado: esAdmin ? 'validado' : 'pendiente_validacion' };
});
en('POST', '/edificios/:eid/recibos/enviar', ({ body }) => ({ en_cola: (body.recibo_ids || []).length }));
en('POST', '/edificios/:eid/periodos', () => {
  throw error(409, 'PERIODO_EXISTE', 'El periodo ya está abierto.');
});
en('POST', '/edificios/:eid/periodos/:p/recibos/generar', ({ params }) => ({ periodo: params.p, recibos: [], total_cts: 2240000, advertencias: ['Faltan 2 lecturas de agua (Dpto 603 y 604).'] }));
en('POST', '/edificios/:eid/periodos/:p/recibos/emitir', () => {
  throw error(422, 'LECTURAS_PENDIENTES', 'Faltan 2 lecturas de agua: Dpto 603 y 604. Complétalas antes de emitir.');
});

// --- 06 Unidades ---
en('GET', '/edificios/:eid', () => EDIFICIO);
en('GET', '/edificios/:eid/unidades', ({ query }) => {
  let l = UNIDADES;
  if (query.buscar) {
    const q = String(query.buscar).toLowerCase();
    l = l.filter((u) => u.codigo.includes(q) || u.propietario.toLowerCase().includes(q));
  }
  const pagina = Number(query.pagina || 1);
  const por = Number(query.por_pagina || 25);
  return {
    datos: l.slice((pagina - 1) * por, pagina * por).map((u) => ({
      id: u.id, codigo: u.codigo, tipo: u.tipo, piso: u.piso, participacion_pct: u.participacion_pct,
      propietario: u.propietario, propietario_dni: u.propietario_dni.slice(0, 4) + '****', celular: u.celular, inquilino: u.inquilino,
      deuda_cts: db.recibos.find((r) => r.unidad_id === u.id)?.saldo_cts || 0,
    })),
    total: l.length,
    pagina,
  };
});
en('POST', '/edificios/:eid/importaciones', ({ form }) => {
  const f = leerForm(form, 'archivo');
  const vista = UNIDADES.map((u, i) => ({
    fila: i + 2, codigo: u.codigo, tipo: 'Departamento', propietario_nombre: u.codigo === '501' ? 'Ana Prueba' : u.propietario,
    propietario_dni_ruc: u.codigo === '501' ? '40000101' : u.propietario_dni, propietario_celular: u.celular,
    inquilino_nombre: u.inquilino, participacion_pct: u.participacion_pct.toFixed(4),
  }));
  return {
    importacion_id: 77, archivo: f?.name || 'padron-edificio-demo.xlsx', hoja: 'Padron', filas: 24, validas: 24,
    errores: [], advertencias: [{ fila: 2, mensaje: 'DNI repetido (fila 17)' }, { fila: 17, mensaje: '¿Misma persona que fila 2?' }],
    suma_participacion_pct: '100.0000', vista_previa: vista,
  };
});
en('POST', '/edificios/:eid/importaciones/:iid/confirmar', () => ({ unidades_creadas: 0, unidades_actualizadas: 24, personas_creadas: 23, deudas_cargadas: 0 }));

// --- 07 Reservas ---
en('GET', '/edificios/:eid/areas', () => AREAS);
en('GET', '/edificios/:eid/reservas', ({ query }) => {
  let l = db.reservas;
  if (query.desde) l = l.filter((r) => diaLima(r.inicio) >= query.desde);
  if (query.hasta) l = l.filter((r) => diaLima(r.inicio) <= query.hasta);
  if (rolActual() === 'propietario') l = l.filter((r) => r.unidad === 'Dpto 201');
  return l;
});
en('GET', '/edificios/:eid/disponibilidad', ({ query }) => {
  const recurso = Number(query.recurso);
  const dias = [];
  for (let d = query.desde; d <= query.hasta; ) {
    dias.push(d);
    const t = new Date(d + 'T12:00:00Z');
    t.setUTCDate(t.getUTCDate() + 1);
    d = t.toISOString().slice(0, 10);
  }
  const franjas = [];
  for (const dia of dias) {
    for (const f of FRANJAS) {
      const ocupada = db.reservas.find((r) => r.recurso_id === recurso && diaLima(r.inicio) === dia && formatearHora(r.inicio) === f.desde && ['confirmada', 'pendiente_pago', 'bloqueo'].includes(r.estado));
      const pasado = dia === '2026-10-01' && f.desde === '08:00';
      franjas.push({ recurso_id: recurso, dia, desde: f.desde, hasta: f.hasta, inicio: `${dia}T${f.desde}:00-05:00`, fin: `${dia}T${f.hasta}:00-05:00`, estado: ocupada ? (ocupada.estado === 'pendiente_pago' ? 'retenida' : 'ocupada') : pasado ? 'fuera_de_horario' : 'libre' });
    }
  }
  return { franjas };
});
en('POST', '/edificios/:eid/reservas', ({ body }) => {
  const rol = rolActual();
  if (rol === 'propietario' && body.unidad_moroso) throw error(403, 'MOROSO', 'Tienes un recibo vencido.', { monto_cts: 142000, meses: 1 });
  const choque = db.reservas.find((r) => r.recurso_id === body.recurso_id && r.inicio === new Date(body.inicio).toISOString() && ['confirmada', 'pendiente_pago'].includes(r.estado));
  if (choque) throw error(409, 'FRANJA_OCUPADA', 'Alguien acaba de reservar esa franja.');
  const area = AREAS.find((a) => a.recursos.some((x) => x.id === body.recurso_id));
  const id = db.sigRes++;
  const inmediato = body.medio && body.medio !== 'recibo';
  const r = {
    id, codigo: `R-0${id}`, recurso_id: body.recurso_id, unidad: rol === 'propietario' ? 'Dpto 201' : body.unidad || 'Dpto 201',
    inicio: new Date(body.inicio).toISOString(), fin: new Date(body.fin).toISOString(),
    estado: inmediato && area.tarifa_cts ? 'pendiente_pago' : 'confirmada', medio: inmediato ? 'Esperando pago' : 'Cargo al recibo', total_cts: area.tarifa_cts,
    vence_retencion: new Date(Date.now() + 15 * 60000).toISOString(),
  };
  db.reservas.push(r);
  return { id, codigo: r.codigo, estado: r.estado, total_cts: r.total_cts, vence_retencion: r.vence_retencion };
});
en('POST', '/edificios/:eid/reservas/:rid/pago', () => ({ estado: 'pendiente_validacion' }));
en('PATCH', '/edificios/:eid/reservas/:rid', ({ params, body }) => {
  const r = db.reservas.find((x) => String(x.id) === params.rid);
  if (r) {
    r.estado = body.estado;
    if (body.estado === 'confirmada') r.medio = 'Pago validado';
  }
  return r;
});

// --- 08 Lecturas ---
en('GET', '/edificios/:eid/lecturas', () => {
  if (!db.lecturas) inicializarLecturas();
  const leidas = db.lecturas.filter((l) => l.estado !== 'pendiente').length;
  return { periodo: PERIODO, tipo: 'agua', corte: '2026-09-30', tarifa_cts: 1400, recibo_general_cts: 500000, avance: { leidas, total: db.lecturas.length }, medidores: db.lecturas };
});
en('POST', '/edificios/:eid/medidores/:mid/lecturas', ({ params, form }) => {
  if (!db.lecturas) inicializarLecturas();
  const foto = leerForm(form, 'foto');
  if (!foto) throw error(422, 'FOTO_OBLIGATORIA', 'La foto del medidor es obligatoria.');
  const l = db.lecturas.find((x) => String(x.medidor_id) === params.mid);
  const valor = Number(leerForm(form, 'valor'));
  l.lectura_actual = valor;
  l.consumo = Math.round((valor - l.lectura_anterior) * 1000) / 1000;
  l.alerta = l.consumo < 0 ? 'NEGATIVO' : l.consumo > 2 * l.media_3m ? 'PICO' : null;
  l.estado = l.alerta ? 'alerta' : 'leida';
  return { consumo: l.consumo, alerta: l.alerta };
});
en('POST', '/edificios/:eid/periodos/:p/reparto-medidores/calcular', () => {
  if (!db.lecturas) inicializarLecturas();
  const lineas = UNIDADES.map((u) => {
    const comun = Math.round(20000 * u.participacion_pct / 100);
    return { unidad: `Dpto ${u.codigo}`, consumo: Math.round(m3DeAgua(u.agua_cts) * 1000) / 1000, propio_cts: u.agua_cts, comun_cts: comun, total_cts: u.agua_cts + comun };
  });
  return { tarifa_cts_x_1000: 1400000, recibo_general_cts: 500000, consumo_general: 357.143, total_unidades_cts: 480000, diferencia_cts: 20000, lineas, alertas: [{ unidad: 'Dpto 402', alerta: 'PICO' }] };
});
en('POST', '/edificios/:eid/periodos/:p/reparto-medidores/aprobar', () => {
  throw error(422, 'LECTURAS_PENDIENTES', 'Faltan lecturas: termina la ronda antes de aprobar el reparto.');
});

// --- 09 Mantenimiento ---
en('GET', '/mantenimiento/incidencias', ({ query }) => {
  let l = db.incidencias.map((i) => ({ ...i, responsable_nombre: TECNICOS.find((t) => t.id === i.responsable_id)?.nombre || '' }));
  if (query.estado) l = l.filter((i) => i.estado === query.estado);
  if (rolActual() === 'tecnico') l = l.filter((i) => i.responsable_id === 31);
  return { datos: filtrarIncidencias(l, query), responsables: TECNICOS };
});
en('PATCH', '/mantenimiento/incidencias/:id/estado', ({ params, body }) => {
  const i = db.incidencias.find((x) => String(x.id) === params.id);
  if (!i) throw error(404, 'NO_ENCONTRADO', 'No encontramos esa incidencia.');
  if (!puedeTransicionar(i.estado, body.estado)) throw error(409, 'TRANSICION_INVALIDA', `No se puede pasar de «${i.estado}» a «${body.estado}».`);
  if (body.estado === 'aprobado' && i.votos && i.votos.a_favor < i.votos.necesarios && rolActual() !== 'junta') {
    throw error(403, 'ESPERA_A_LA_JUNTA', `Supera el umbral de S/ 1.000,00: decide la junta (${i.votos.a_favor} de ${i.votos.necesarios} votos).`);
  }
  i.estado = body.estado;
  if (body.estado === 'validado' && !i.criticidad) i.criticidad = 'media';
  return i;
});
en('POST', '/edificios/:eid/incidencias', ({ form }) => {
  const id = db.sigInc++;
  const inc = {
    id, codigo: `INC-0${id}`, titulo: (leerForm(form, 'descripcion') || '').slice(0, 60), descripcion: leerForm(form, 'descripcion'),
    ubicacion: leerForm(form, 'ubicacion'), categoria: leerForm(form, 'categoria'), estado: 'reportado', criticidad: null,
    reportado_por: USUARIOS_MOCK[rolActual()]?.nombre, reportado_en: new Date().toISOString(), n_fotos: form?.getAll ? form.getAll('fotos[]').length : 1,
  };
  db.incidencias.unshift(inc);
  return { id, codigo: inc.codigo };
});

// --- 10 Portal ---
en('GET', '/edificios/:eid/portal', () => {
  const r = db.recibos.find((x) => x.unidad === 'Dpto 201');
  return {
    unidades: [{ id: 201, codigo: '201', nombre: 'Dpto 201' }],
    recibo_actual: r,
    deuda: { total_cts: r.saldo_cts, meses: r.saldo_cts ? [{ periodo: PERIODO, saldo_cts: r.saldo_cts }] : [] },
    pago_en_revision: r.pagos.some((p) => p.estado === 'pendiente_validacion'),
    kpis_edificio: { ingresos_cts: 1946000, egresos_cts: 1895000, saldo_cts: 51000, morosidad: { pct: 13.1, unidades: 3, monto_cts: 294000 } },
    mis_incidencias: db.incidencias.filter((i) => i.unidad === 'Dpto 201' || i.reportado_por === 'María Demo').slice(0, 3),
    proximas_reservas: db.reservas.filter((x) => x.unidad === 'Dpto 201').map((x) => ({ ...x, recurso: AREAS.flatMap((a) => a.recursos).find((y) => y.id === x.recurso_id)?.nombre })),
    trabajos_mes: db.incidencias.filter((i) => ['en_ejecucion', 'terminado', 'presupuestado'].includes(i.estado)).slice(0, 4),
    normas_url: null,
    yape: EDIFICIO.yape,
  };
});

// --- 11 Roles ---
const USUARIOS = [
  { id: 1, nombre: 'Administración Demo', correo: 'admin@demo.pe', rol: 'administrador', ultimo_ingreso: '2026-09-28T15:00:00Z', estado: 'activo' },
  { id: 3, nombre: 'Asistente Demo', correo: 'asistente@demo.pe', rol: 'administrador', ultimo_ingreso: '2026-09-25T15:00:00Z', estado: 'activo' },
  { id: 2, nombre: 'Presidente Demo', correo: 'presidente@demo.pe', rol: 'junta', ultimo_ingreso: '2026-09-27T01:00:00Z', estado: 'activo', presidente: true },
  { id: 6, nombre: 'Tesorera Demo', correo: 'tesorera@demo.pe', rol: 'junta', ultimo_ingreso: '2026-09-26T01:00:00Z', estado: 'activo' },
  { id: 7, nombre: 'Secretario Demo', correo: 'secretario@demo.pe', rol: 'junta', ultimo_ingreso: null, estado: 'invitado' },
  { id: 201, nombre: 'María Demo', correo: 'propietario201@demo.pe', unidad: 'Dpto 201', rol: 'propietario', ultimo_ingreso: '2026-09-27T23:00:00Z', estado: 'activo' },
  { id: 202, nombre: 'Luis Ejemplo', correo: 'propietario202@demo.pe', unidad: 'Dpto 202', rol: 'propietario', ultimo_ingreso: '2026-09-20T23:00:00Z', estado: 'activo' },
  { id: 1031, nombre: 'Inquilino Demo 103', correo: 'inquilino103@demo.pe', unidad: 'Dpto 103', rol: 'inquilino', ultimo_ingreso: null, estado: 'invitado' },
  { id: 5, nombre: 'Conserje Demo', correo: 'conserje@demo.pe', rol: 'operario', ultimo_ingreso: '2026-09-28T13:00:00Z', estado: 'activo' },
  { id: 31, nombre: 'Técnico Demo', correo: 'tecnico@demo.pe', rol: 'tecnico', ultimo_ingreso: '2026-09-22T13:00:00Z', estado: 'activo' },
  { id: 40, nombre: 'Ex conserje Demo', correo: 'exconserje@demo.pe', rol: 'operario', ultimo_ingreso: '2026-06-01T13:00:00Z', estado: 'inactivo' },
];
en('GET', '/edificios/:eid/usuarios', () => ({ datos: USUARIOS, total: USUARIOS.length, pagina: 1 }));
en('POST', '/edificios/:eid/usuarios/invitar', ({ body }) => ({ enlace_invitacion: `https://edisys.pe/login/invitacion/demo-${(body.correo || 'x').replace(/\W/g, '')}` }));
en('PATCH', '/edificios/:eid/usuarios/:uid', ({ params, body }) => {
  const u = USUARIOS.find((x) => String(x.id) === params.uid);
  if (u.rol === 'administrador' && (body.activo === false || (body.rol && body.rol !== 'administrador')) && USUARIOS.filter((x) => x.rol === 'administrador' && x.estado === 'activo').length <= 1) {
    throw error(409, 'ULTIMO_ADMIN', 'No se puede dejar el edificio sin administrador.');
  }
  if (body.rol) u.rol = body.rol;
  if (body.activo !== undefined) u.estado = body.activo ? 'activo' : 'inactivo';
  return u;
});
en('GET', '/roles', () => ({
  roles: ['administrador', 'junta', 'propietario', 'inquilino', 'operario', 'tecnico'].map((r) => ({ id: r, nombre: NOMBRE_ROL[r], personas: USUARIOS.filter((u) => u.rol === r).length })),
  modulos: [
    { modulo: 'Configuración del edificio', niveles: { administrador: 'total', junta: 'ver' } },
    { modulo: 'Unidades e importación', niveles: { administrador: 'total', junta: 'ver', propietario: 'propio' } },
    { modulo: 'Recibos y cobranza', niveles: { administrador: 'total', junta: 'ver', propietario: 'propio', inquilino: 'si_habilita' } },
    { modulo: 'Balance por nodos', niveles: { administrador: 'total', junta: 'ver', propietario: 'ver' } },
    { modulo: 'Documentos de egresos', niveles: { administrador: 'total', junta: 'ver', propietario: 'ajustable' }, ajustable: true, permiso: 'balance.ver_documentos' },
    { modulo: 'Reservas', niveles: { administrador: 'total', junta: 'ver', propietario: 'accion:Reservar', inquilino: 'accion:Reservar', operario: 'ver' } },
    { modulo: 'Lectura de medidores', niveles: { administrador: 'total', junta: 'ver', propietario: 'propio', operario: 'accion:Registrar' } },
    { modulo: 'Incidencias y mantenimiento', niveles: { administrador: 'total', junta: 'ver', propietario: 'accion:Reportar', inquilino: 'accion:Reportar', operario: 'accion:Reportar', tecnico: 'accion:Actualizar' } },
    { modulo: 'Aprobación de gastos', niveles: { administrador: 'accion:Proponer', junta: 'aprobar', propietario: 'ver' } },
    { modulo: 'WhatsApp y avisos', niveles: { administrador: 'total' } },
    { modulo: 'Analítica', niveles: { administrador: 'total', junta: 'ver' } },
    { modulo: 'Roles y permisos', niveles: { administrador: 'total' }, bloqueado: true },
  ],
}));
en('GET', '/edificios/:eid/junta', () => ({ modo: 'mayoria', umbral_cts: 100000, miembros: USUARIOS.filter((u) => u.rol === 'junta').map((u) => ({ id: u.id, nombre: u.nombre, presidente: !!u.presidente })).concat([{ id: 8, nombre: 'Vocal 1 Demo' }, { id: 9, nombre: 'Vocal 2 Demo' }]) }));

// --- 12 WhatsApp y chatbot ---
en('GET', '/whatsapp/mensajes', ({ query }) => {
  let l = [...db.mensajes].sort((a, b) => b.fecha.localeCompare(a.fecha));
  if (query.estado) l = l.filter((m) => m.estado === query.estado);
  if (query.q) {
    const q = String(query.q).toLowerCase();
    l = l.filter((m) => [m.telefono, m.unidad, m.destinatario, m.texto].join(' ').toLowerCase().includes(q));
  }
  return { datos: l, total: l.length, plantillas: PLANTILLAS };
});
en('POST', '/whatsapp/enviar', ({ body }) => {
  const u = body.unidad_id ? UNIDADES.find((x) => x.id === Number(body.unidad_id)) : null;
  const texto = body.plantilla === 'aviso_general' ? body.variables?.texto : `[${body.plantilla}] ${Object.values(body.variables || {}).join(' · ')}`;
  const m = { id: db.sigMsg++, direccion: 'saliente', telefono: body.telefono || u?.celular, unidad: u ? `Dpto ${u.codigo}` : null, destinatario: u?.propietario || body.telefono, plantilla: body.plantilla, texto, estado: db.wa.modo === 'simulado' ? 'simulado' : 'en_cola', fecha: new Date().toISOString() };
  db.mensajes.push(m);
  return { id: m.id, estado: m.estado, simulado: db.wa.modo === 'simulado' };
});
en('POST', '/whatsapp/recibos/:periodo/enviar', ({ params }) => {
  const n = db.recibos.filter((r) => r.periodo === params.periodo).length;
  for (const r of db.recibos.slice(0, 3)) {
    db.mensajes.push({ id: db.sigMsg++, direccion: 'saliente', telefono: `900000${r.unidad.slice(5)}`, unidad: r.unidad, destinatario: r.propietario, plantilla: 'recibo_emitido', texto: `Hola, tu recibo de setiembre es ${formatearSoles(r.total_cts)}.`, estado: db.wa.modo === 'simulado' ? 'simulado' : 'en_cola', fecha: new Date().toISOString() });
  }
  return { en_cola: n, simulado: db.wa.modo === 'simulado' };
});
en('GET', '/whatsapp/config', () => db.wa);
en('PUT', '/whatsapp/config', ({ body }) => {
  if (body.modo === 'evolution' && !body.url) throw error(422, 'VALIDACION', 'Falta la URL de evolution-go.', { campos: { url: 'Obligatoria en modo evolution.' } });
  db.wa = { modo: body.modo, url: body.url || '', instancia: body.instancia || '' };
  return db.wa;
});
en('POST', '/chatbot/mensaje', ({ body }) => {
  const t = String(body.texto || '').toLowerCase();
  const u = UNIDADES.find((x) => x.celular === String(body.telefono).replace(/\D/g, '').slice(-9));
  if (!u) return { respuesta: 'Hola. No encuentro tu número en el padrón del edificio. Escribe a la administración para registrarlo.', intencion: 'desconocido', datos: null };
  const r = db.recibos.find((x) => x.unidad_id === u.id);
  const nombre = u.propietario.split(' ')[0];
  if (/deb|deuda|saldo|pagar|cu[aá]nto/.test(t)) {
    return r.saldo_cts
      ? { respuesta: `${nombre}, tienes ${formatearSoles(r.saldo_cts)} pendientes del recibo de setiembre (venció el 15-09). Puedes pagar por Yape al ${EDIFICIO.yape.numero}.`, intencion: 'consultar_deuda', datos: { saldo_cts: r.saldo_cts, periodo: PERIODO } }
      : { respuesta: `Estás al día, ${nombre}. Tu recibo de setiembre (${formatearSoles(r.total_cts)}) figura pagado.`, intencion: 'consultar_deuda', datos: { saldo_cts: 0 } };
  }
  if (/recibo/.test(t)) return { respuesta: `Tu recibo de setiembre es ${formatearSoles(r.total_cts)}: ${r.lineas.filter((l) => l.monto_cts).map((l) => `${l.concepto} ${formatearSoles(l.monto_cts)}`).join('; ')}.`, intencion: 'ver_recibo', datos: { recibo_id: r.id, total_cts: r.total_cts } };
  if (/reserv|parrilla|sala/.test(t)) return u.moroso ? { respuesta: 'Por ahora no puedes reservar: tienes un recibo vencido. Cuando lo pagues, se habilita.', intencion: 'reservar', datos: { bloqueado: true } } : { respuesta: 'Para reservar una parrilla entra a https://edisys.pe/app/ → Reservas. Hay turnos libres el sábado 3 de 12:00 a 16:00 en Parrilla 1.', intencion: 'reservar', datos: { libres: 2 } };
  if (/fuga|malogr|roto|report|incidencia|humedad/.test(t)) return { respuesta: 'Gracias por avisar. Registré tu reporte como INC-020; la administración lo revisará hoy. Si puedes, envía una foto.', intencion: 'reportar_incidencia', datos: { codigo: 'INC-020' } };
  if (/hola|buen/.test(t)) return { respuesta: `Hola, ${nombre}. Soy el asistente del Edificio Demo. Puedo decirte tu deuda, tu recibo, ayudarte a reservar o registrar un reporte.`, intencion: 'saludo', datos: null };
  return { respuesta: 'No te entendí. Prueba con: «¿cuánto debo?», «mi recibo», «reservar parrilla» o «reportar una fuga».', intencion: 'ayuda', datos: null };
});

// --- 13 Analítica ---
en('GET', '/analitica/resumen', ({ query }) => {
  const dentro = (p) => (!query.desde || p >= query.desde) && (!query.hasta || p <= query.hasta);
  return { ...ANALITICA, cobranza_mensual: ANALITICA.cobranza_mensual.filter((x) => dentro(x.periodo)), morosidad_mensual: ANALITICA.morosidad_mensual.filter((x) => dentro(x.periodo)) };
});
en('POST', '/edificios/:eid/egresos', ({ form }) => ({ id: Date.now(), sin_sustento: !leerForm(form, 'documento') }));
