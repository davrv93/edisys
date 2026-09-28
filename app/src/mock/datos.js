// Datos de demostración (DISENO.md y guía §5): Edificio Demo, 24 dptos (101–604), setiembre 2026.
// Solo se cargan con VITE_MOCK=1. Las cifras cuadran al céntimo con el lienzo:
// emitido 22.400 = cuotas 16.800 + agua 4.800 + común 200 + reservas 600; cobrado 19.460; morosidad 2.940 (13,1 %).

export const EID = 1;
export const PERIODO = '2026-09';

export const EDIFICIO = {
  id: EID, nombre: 'Edificio Demo', direccion: 'Av. Demo 123', distrito: 'Miraflores, Lima', unidades: 24,
  periodo_abierto: PERIODO, fecha_corte: 30, dias_gracia: 15, reparto: 'participacion', cobro_reservas: 'mixto',
  yape: { numero: '900 000 000', titular: 'Junta de Propietarios Edificio Demo' },
};

// Participaciones de la semilla (guía §08): A 4,20 % (16), B 4,00 % (6), C 4,40 % (2).
const GRUPO_B = ['102', '202', '302', '402', '502', '602'];
const GRUPO_C = ['601', '604'];
const NOMBRES = {
  101: 'Ana Prueba', 102: 'Carlos Ejemplo', 103: 'Rosa Muestra', 104: 'Jorge Demo',
  201: 'María Demo', 202: 'Luis Ejemplo', 203: 'Carmen Prueba', 204: 'Raúl Ejemplo',
  301: 'Sofía Muestra', 302: 'Diego Demo', 303: 'Lucía Prueba', 304: 'Andrés Ejemplo',
  401: 'Patricia Muestra', 402: 'Pedro Prueba', 403: 'Julia Demo', 404: 'Miguel Ejemplo',
  501: 'Ana Prueba', 502: 'Teresa Muestra', 503: 'Elena Muestra', 504: 'Óscar Demo',
  601: 'Gloria Prueba', 602: 'Hugo Ejemplo', 603: 'Inés Muestra', 604: 'Víctor Demo',
};

// Agua propia en céntimos (tarifa S/ 14,00/m³). 201 = 14,000 m³ = S/ 196,00. Suma = S/ 4.800,00.
const AGUA_FIJA = { 201: 19600, 104: 4600, 503: 4600, 402: 74000 };
const VARIACION = [1400, -1400, 2800, -2800, 700, -700, 2100, -2100, 0, 0, 1400, -1400, 3500, -3500, 700, -700, 0, 1400, -1400, 0];
// Reservas cargadas al recibo (suman S/ 600,00).
const RESERVAS_RECIBO = { 201: 8000, 202: 8000, 103: 14000, 602: 8000, 304: 8000, 401: 14000 };
const MOROSOS = ['402', '503', '104'];

export const UNIDADES = [];
{
  let k = 0;
  for (let piso = 1; piso <= 6; piso++) {
    for (let n = 1; n <= 4; n++) {
      const codigo = `${piso}0${n}`;
      const pct = GRUPO_C.includes(codigo) ? 4.4 : GRUPO_B.includes(codigo) ? 4.0 : 4.2;
      const agua = AGUA_FIJA[codigo] ?? 18860 + VARIACION[k++];
      UNIDADES.push({
        id: Number(codigo), codigo, tipo: 'departamento', piso, participacion_pct: pct,
        propietario: NOMBRES[codigo], propietario_dni: `4000${codigo}`.slice(0, 8),
        celular: `900000${codigo}`, correo: `propietario${codigo}@demo.pe`,
        inquilino: codigo === '103' ? 'Inquilino Demo 103' : null,
        agua_cts: agua, moroso: MOROSOS.includes(codigo),
      });
    }
  }
}

const TARIFA_M3_CTS = 1400;
export const m3DeAgua = (cts) => cts / TARIFA_M3_CTS;

export function lineasRecibo(u) {
  const cuota = Math.round(1680000 * u.participacion_pct / 100);
  const comun = Math.round(20000 * u.participacion_pct / 100);
  const m3 = m3DeAgua(u.agua_cts);
  const anterior = 1284 + (u.id % 37) * 3;
  const lineas = [
    { concepto: 'Cuota de mantenimiento', detalle: `${fmtPct(u.participacion_pct)} de S/ 16.800,00`, monto_cts: cuota },
    { concepto: 'Agua: consumo propio', detalle: `${fmtM3(m3)} m³ (${fmtMiles(anterior)} → ${fmtMiles(anterior + Math.round(m3))}) × S/ 14,00`, monto_cts: u.agua_cts },
    { concepto: 'Agua: áreas comunes', detalle: `${fmtPct(u.participacion_pct)} de S/ 200,00 (S/ 5.000 − S/ 4.800)`, monto_cts: comun },
  ];
  if (RESERVAS_RECIBO[u.codigo]) {
    lineas.push({ concepto: u.codigo === '201' ? 'Reserva Parrilla 1' : 'Reserva de área común', detalle: u.codigo === '201' ? '29-08-2026 · cargo al recibo' : 'Cargo al recibo', monto_cts: RESERVAS_RECIBO[u.codigo] });
  }
  lineas.push({ concepto: 'Saldo anterior', detalle: '', monto_cts: 0 });
  return lineas;
}

function fmtPct(p) {
  return p.toFixed(2).replace('.', ',') + ' %';
}
function fmtM3(m) {
  return m.toFixed(3).replace('.', ',');
}
function fmtMiles(n) {
  return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, '.');
}

export const RECIBOS = UNIDADES.map((u, i) => {
  const lineas = lineasRecibo(u);
  const total = lineas.reduce((a, l) => a + l.monto_cts, 0);
  const moroso = u.moroso;
  return {
    id: 9000 + i + 1,
    numero: `2026-09-${String(u.codigo).padStart(4, '0')}`,
    correlativo: `R-${String(101 + i).padStart(6, '0')}`,
    periodo: PERIODO,
    unidad_id: u.id,
    unidad: `Dpto ${u.codigo}`,
    propietario: u.propietario,
    participacion_pct: u.participacion_pct,
    total_cts: total,
    pagado_cts: moroso ? 0 : total,
    saldo_cts: moroso ? total : 0,
    estado: moroso ? 'vencido' : 'pagado',
    emitido: '2026-09-01',
    vence: '2026-09-15',
    lineas,
    pagos: moroso ? [] : [{ id: 7000 + i, fecha: '2026-09-12', medio: 'yape', codigo_operacion: `0000${i}`.slice(-6), monto_cts: total, estado: 'validado' }],
    foto_medidor: { url: null, lectura: 1298 - (u.codigo === '201' ? 0 : i), medidor: `A-${u.codigo}`, tomada_en: '2026-08-31T12:42:00Z', operario: 'Operario Demo' },
  };
});

export const USUARIOS_MOCK = {
  administrador: { id: 1, nombre: 'Administración Demo', correo: 'admin@demo.pe', telefono: '900000001' },
  junta: { id: 2, nombre: 'Presidente Demo', correo: 'presidente@demo.pe', telefono: '900000002' },
  propietario: { id: 201, nombre: 'María Demo', correo: 'propietario201@demo.pe', telefono: '900000201' },
  inquilino: { id: 1031, nombre: 'Inquilino Demo 103', correo: 'inquilino103@demo.pe', telefono: '900001031' },
  operario: { id: 5, nombre: 'Conserje Demo', correo: 'conserje@demo.pe', telefono: '900000005' },
  tecnico: { id: 31, nombre: 'Técnico Demo', correo: 'tecnico@demo.pe', telefono: '900000031' },
};

export const TECNICOS = [
  { id: 31, nombre: 'Técnico Demo' },
  { id: 32, nombre: 'Bombas Demo SAC' },
  { id: 33, nombre: 'Ascensores Demo SAC' },
];

// Árbol del balance (guía §04): ingresos 14.630 + 4.230 + 600 = 19.460; egresos 10.500 + 6.150 + 1.600 + 700 = 18.950.
export const ARBOL = {
  raiz: { id: 'raiz', nombre: 'Edificio Demo · saldo del mes', total_cts: 51000, documentos: 66, tiene_hijos: true },
  hijos: {
    raiz: [
      { id: 'ing', tipo: 'ingresos', nombre: 'Ingresos cobrados', total_cts: 1946000, pct_padre: 100, documentos: 24, tiene_hijos: true },
      { id: 'egr', tipo: 'egresos', nombre: 'Egresos', total_cts: 1895000, pct_padre: 100, documentos: 42, tiene_hijos: true },
    ],
    ing: [
      { id: 'ing.cuotas', tipo: 'rubro', nombre: 'Cuotas de mantenimiento', total_cts: 1463000, documentos: 21, tiene_hijos: true },
      { id: 'ing.agua', tipo: 'rubro', nombre: 'Agua y áreas comunes (reparto de medidores)', total_cts: 423000, documentos: 21, tiene_hijos: false },
      { id: 'ing.reservas', tipo: 'rubro', nombre: 'Reservas de áreas', total_cts: 60000, documentos: 6, tiene_hijos: true },
    ],
    'ing.cuotas': [
      { id: 'doc.r201', tipo: 'documento', formato: 'img', nombre: 'Recibo 2026-09-201 · María Demo (pagado) · voucher', total_cts: 70560, doc_id: 'voucher-201' },
      { id: 'doc.r101', tipo: 'documento', formato: 'img', nombre: 'Recibo 2026-09-101 · Ana Prueba (pagado) · voucher', total_cts: 70560, doc_id: 'voucher-101' },
      { id: 'ing.cuotas.resto', tipo: 'concepto', nombre: '19 recibos más (pagados)', total_cts: 1321880, tiene_hijos: false },
    ],
    'ing.reservas': [
      { id: 'doc.R0412', tipo: 'documento', formato: 'img', nombre: 'R-0412 Parrilla 1 · Dpto 201 · cargada al recibo', total_cts: 8000, doc_id: 'reserva-0412' },
      { id: 'ing.reservas.resto', tipo: 'concepto', nombre: '5 reservas más', total_cts: 52000, tiene_hijos: false },
    ],
    egr: [
      { id: 'egr.administracion', tipo: 'rubro', nombre: 'Administración', total_cts: 1050000, documentos: 12, tiene_hijos: true },
      { id: 'egr.servicios', tipo: 'rubro', nombre: 'Servicios básicos', total_cts: 615000, documentos: 5, tiene_hijos: true },
      { id: 'egr.mantenimiento', tipo: 'rubro', nombre: 'Mantenimiento preventivo', total_cts: 160000, documentos: 7, tiene_hijos: true },
      { id: 'egr.fondo', tipo: 'rubro', nombre: 'Fondo de contingencia', total_cts: 70000, documentos: 1, tiene_hijos: false, sin_sustento: true },
    ],
    'egr.administracion': [
      { id: 'egr.administracion.conserjeria', tipo: 'concepto', nombre: 'Conserjería', total_cts: 630000, documentos: 4, tiene_hijos: true },
      { id: 'egr.administracion.limpieza', tipo: 'concepto', nombre: 'Limpieza', total_cts: 280000, documentos: 4, tiene_hijos: false },
      { id: 'egr.administracion.administrador', tipo: 'concepto', nombre: 'Administrador', total_cts: 140000, documentos: 4, tiene_hijos: false },
    ],
    'egr.administracion.conserjeria': [
      { id: 'doc.conserjeria', tipo: 'documento', formato: 'pdf', nombre: 'Factura E001-000245 · Servicios Demo SAC', total_cts: 630000, doc_id: 'factura-conserjeria' },
    ],
    'egr.servicios': [
      { id: 'egr.servicios.sedapal', tipo: 'concepto', nombre: 'Sedapal setiembre', total_cts: 500000, documentos: 1, tiene_hijos: true },
      { id: 'egr.servicios.luz', tipo: 'concepto', nombre: 'Luz de áreas comunes', total_cts: 98000, documentos: 1, tiene_hijos: false },
      { id: 'egr.servicios.internet', tipo: 'concepto', nombre: 'Internet de áreas sociales', total_cts: 17000, documentos: 1, tiene_hijos: false, sin_sustento: true },
    ],
    'egr.servicios.sedapal': [
      { id: 'doc.sedapal', tipo: 'documento', formato: 'img', nombre: 'Foto del recibo Sedapal · 357,143 m³', total_cts: 500000, doc_id: 'recibo-sedapal' },
    ],
    'egr.mantenimiento': [
      { id: 'egr.mantenimiento.ascensores', tipo: 'concepto', nombre: 'Mantenimiento de ascensores', total_cts: 120000, documentos: 3, tiene_hijos: true },
      { id: 'egr.mantenimiento.cisterna', tipo: 'concepto', nombre: 'Limpieza y desinfección de cisterna', total_cts: 40000, documentos: 2, tiene_hijos: false },
    ],
    'egr.mantenimiento.ascensores': [
      { id: 'doc.ascensores', tipo: 'documento', formato: 'pdf', nombre: 'Factura E001-000123 · Ascensores Demo SAC', total_cts: 120000, doc_id: 'factura-ascensores' },
      { id: 'doc.ascensores.informe', tipo: 'documento', formato: 'pdf', nombre: 'Informe técnico de visita · 12-09-2026', total_cts: 0, es_sustento: true, doc_id: 'informe-ascensores' },
      { id: 'doc.ascensores.voucher', tipo: 'documento', formato: 'img', nombre: 'Voucher de transferencia · Op. 000184', total_cts: 0, es_sustento: true, doc_id: 'voucher-ascensores' },
    ],
  },
};

export const DOCUMENTOS = {
  'factura-ascensores': { tipo: 'pdf', nombre: 'Factura E001-000123', emisor: 'Ascensores Demo SAC', ruc: '20000000001 (demo)', fecha: '2026-09-15', monto_cts: 120000, origen: 'egreso', banco: 'Conciliado · 22-09' },
  'recibo-sedapal': { tipo: 'foto', nombre: 'Recibo Sedapal setiembre', emisor: 'Sedapal', ruc: '20100152356', fecha: '2026-09-20', monto_cts: 500000, origen: 'egreso', nota: '357,143 m³ en el medidor general' },
  'factura-conserjeria': { tipo: 'pdf', nombre: 'Factura E001-000245', emisor: 'Servicios Demo SAC', ruc: '20000000002 (demo)', fecha: '2026-09-30', monto_cts: 630000, origen: 'egreso' },
  'voucher-201': { tipo: 'voucher', nombre: 'Voucher Yape · Op. 000000', emisor: 'María Demo · Dpto 201', fecha: '2026-09-12', monto_cts: 99000, origen: 'recibo', solo_junta: true },
  'voucher-101': { tipo: 'voucher', nombre: 'Voucher Yape · Op. 000001', emisor: 'Ana Prueba · Dpto 101', fecha: '2026-09-10', monto_cts: 81240, origen: 'recibo', solo_junta: true },
  'reserva-0412': { tipo: 'voucher', nombre: 'Reserva R-0412', emisor: 'Dpto 201', fecha: '2026-08-29', monto_cts: 8000, origen: 'reserva' },
  'informe-ascensores': { tipo: 'pdf', nombre: 'Informe técnico de visita', emisor: 'Ascensores Demo SAC', fecha: '2026-09-12', monto_cts: 0, origen: 'trabajo' },
  'voucher-ascensores': { tipo: 'voucher', nombre: 'Voucher de transferencia · Op. 000184', emisor: 'Banco Demo', fecha: '2026-09-22', monto_cts: 120000, origen: 'egreso' },
};

export const INCIDENCIAS = [
  { id: 19, codigo: 'INC-019', titulo: 'Humedad en muro de escalera, piso 2', descripcion: 'Mancha de humedad que creció esta semana; la pintura se está soltando.', estado: 'reportado', criticidad: null, categoria: 'humedad', ubicacion: 'Escalera, piso 2', unidad: 'Dpto 201', reportado_por: 'María Demo', reportado_en: '2026-09-28T16:00:00Z', n_fotos: 2 },
  { id: 18, codigo: 'INC-018', titulo: 'Luz quemada en pasillo, piso 3', estado: 'reportado', criticidad: null, categoria: 'electricidad', ubicacion: 'Piso 3 – pasadizo', reportado_por: 'Conserje Demo', reportado_en: '2026-09-27T14:00:00Z', n_fotos: 1 },
  { id: 17, codigo: 'INC-017', titulo: 'Filtración en techo', estado: 'reportado', criticidad: null, categoria: 'humedad', ubicacion: 'Dpto 602, sala', unidad: 'Dpto 602', reportado_por: 'Hugo Ejemplo', reportado_en: '2026-09-27T10:00:00Z', n_fotos: 1 },
  { id: 16, codigo: 'INC-016', titulo: 'Puerta de cochera con ruido', estado: 'validado', criticidad: 'media', categoria: 'seguridad', ubicacion: 'Cochera', reportado_por: 'Conserje Demo', reportado_en: '2026-09-24T13:00:00Z', n_fotos: 1, nota: 'Falta asignar técnico' },
  { id: 15, codigo: 'INC-015', titulo: 'Cámara 4 sin imagen', estado: 'presupuestado', criticidad: 'media', categoria: 'seguridad', ubicacion: 'Hall de ingreso', reportado_por: 'Conserje Demo', reportado_en: '2026-09-22T13:00:00Z', responsable_id: 31, monto_cts: 32000, n_fotos: 1, nota: 'Bajo el umbral, aprueba la administración' },
  { id: 14, codigo: 'INC-014', titulo: 'Bomba de agua N.º 2', descripcion: 'Bomba de agua N.º 2 con ruido y goteo', estado: 'presupuestado', criticidad: 'critica', categoria: 'gasfiteria', ubicacion: 'Cuarto de bombas, sótano', reportado_por: 'Conserje Demo', reportado_en: '2026-09-20T13:00:00Z', responsable_id: 32, monto_cts: 185000, n_fotos: 2, votos: { a_favor: 2, necesarios: 3, miembros: 5 }, diagnostico: 'Rodamiento dañado; sin cambio, falla en semanas.' },
  { id: 13, codigo: 'INC-013', titulo: 'Cambio de mayólica del hall', estado: 'rechazado', criticidad: 'baja', categoria: 'otro', ubicacion: 'Hall', reportado_por: 'Julia Demo', reportado_en: '2026-09-08T13:00:00Z', responsable_id: 31, monto_cts: 240000, n_fotos: 1, nota: 'Pendiente no aprobado por la junta' },
  { id: 12, codigo: 'INC-012', titulo: 'Pintura de escalera', estado: 'en_ejecucion', criticidad: 'baja', categoria: 'otro', ubicacion: 'Escalera', reportado_por: 'Administración Demo', reportado_en: '2026-09-05T13:00:00Z', responsable_id: 31, monto_cts: 90000, avance_pct: 60, n_fotos: 3 },
  { id: 11, codigo: 'INC-011', titulo: 'Chapa de puerta principal', estado: 'terminado', criticidad: 'media', categoria: 'seguridad', ubicacion: 'Puerta principal', reportado_por: 'Conserje Demo', reportado_en: '2026-09-03T13:00:00Z', responsable_id: 31, monto_cts: 18000, costo_real_cts: 18000, n_fotos: 2 },
  { id: 10, codigo: 'INC-010', titulo: 'Destape de desagüe', estado: 'terminado', criticidad: 'critica', categoria: 'gasfiteria', ubicacion: 'Sótano', reportado_por: 'Conserje Demo', reportado_en: '2026-09-02T13:00:00Z', responsable_id: 32, monto_cts: 15000, costo_real_cts: 15000, n_fotos: 2 },
  { id: 9, codigo: 'INC-009', titulo: 'Luminarias del hall', estado: 'terminado', criticidad: 'baja', categoria: 'electricidad', ubicacion: 'Hall', reportado_por: 'Administración Demo', reportado_en: '2026-09-01T13:00:00Z', responsable_id: 31, monto_cts: 24000, costo_real_cts: 24000, n_fotos: 1 },
];

export const AREAS = [
  { id: 1, nombre: 'Parrillas', tarifa_cts: 8000, duracion_h: 4, horario: '10:00–22:00', aforo: 12, incluye: 'incluye limpieza posterior', normas: 'Uso hasta las 22:00. Penalidad por daños.', cobro: 'pago inmediato o cargo al recibo', recursos: [{ id: 11, nombre: 'Parrilla 1' }, { id: 12, nombre: 'Parrilla 2' }, { id: 13, nombre: 'Parrilla 3' }] },
  { id: 2, nombre: 'Sala común', tarifa_cts: 14000, duracion_h: 4, horario: '09:00–23:00', aforo: 30, incluye: 'sillas y mesas', normas: 'Música hasta las 22:00.', cobro: 'cargo al recibo', recursos: [{ id: 21, nombre: 'Sala común' }] },
  { id: 3, nombre: 'Terraza', tarifa_cts: 0, duracion_h: 3, horario: '08:00–20:00', aforo: 15, incluye: 'sin costo', normas: 'Sin vidrio.', cobro: 'sin costo', recursos: [{ id: 31, nombre: 'Terraza' }] },
];

export const RESERVAS = [
  { id: 405, codigo: 'R-0405', recurso_id: 11, unidad: 'Dpto 304', inicio: '2026-10-03T17:00:00Z', fin: '2026-10-03T21:00:00Z', estado: 'confirmada', medio: 'Pagado · Yape', total_cts: 8000 },
  { id: 406, codigo: 'R-0406', recurso_id: 11, unidad: 'Dpto 202', inicio: '2026-10-04T17:00:00Z', fin: '2026-10-04T21:00:00Z', estado: 'confirmada', medio: 'Cargo al recibo', total_cts: 8000 },
  { id: 412, codigo: 'R-0412', recurso_id: 12, unidad: 'Dpto 201', inicio: '2026-10-03T17:00:00Z', fin: '2026-10-03T21:00:00Z', estado: 'pendiente_pago', medio: 'Esperando pago', total_cts: 8000, vence_retencion: '2026-10-03T17:15:00Z' },
  { id: 407, codigo: 'R-0407', recurso_id: 13, unidad: 'Dpto 602', inicio: '2026-10-03T22:00:00Z', fin: '2026-10-04T02:00:00Z', estado: 'confirmada', medio: 'Pagado · Yape', total_cts: 8000 },
  { id: 408, codigo: 'R-0408', recurso_id: 21, unidad: 'Dpto 103', inicio: '2026-10-03T00:00:00Z', fin: '2026-10-03T04:00:00Z', estado: 'confirmada', medio: 'Cargo al recibo', total_cts: 14000 },
  { id: 409, codigo: 'B-0012', recurso_id: 31, unidad: null, inicio: '2026-10-01T13:00:00Z', fin: '2026-10-02T03:00:00Z', estado: 'bloqueo', medio: 'Limpieza (PAM)', total_cts: 0 },
];

export const FRANJAS = [
  { desde: '08:00', hasta: '12:00' },
  { desde: '12:00', hasta: '16:00' },
  { desde: '17:00', hasta: '21:00' },
];

export const MENSAJES = [
  { id: 1, direccion: 'saliente', telefono: '900000402', unidad: 'Dpto 402', destinatario: 'Pedro Prueba', plantilla: 'recordatorio_deuda', texto: 'Hola Pedro, tu recibo de setiembre (S/ 1.420,00) venció el 15-09. Puedes pagar por Yape al 900 000 000.', estado: 'entregado', fecha: '2026-09-26T14:10:00Z' },
  { id: 2, direccion: 'saliente', telefono: '900000201', unidad: 'Dpto 201', destinatario: 'María Demo', plantilla: 'recibo_emitido', texto: 'Hola María, tu recibo de setiembre es S/ 990,00 y vence el 15-09. Detalle: https://edisys.pe/app/', estado: 'leido', fecha: '2026-09-01T13:00:00Z' },
  { id: 3, direccion: 'entrante', telefono: '900000201', unidad: 'Dpto 201', destinatario: 'María Demo', plantilla: null, texto: '¿Cuánto debo?', estado: 'recibido', fecha: '2026-09-27T23:02:00Z' },
  { id: 4, direccion: 'saliente', telefono: '900000201', unidad: 'Dpto 201', destinatario: 'María Demo', plantilla: 'chatbot', texto: 'Estás al día, María. Tu recibo de setiembre (S/ 990,00) figura pagado el 12-09.', estado: 'entregado', fecha: '2026-09-27T23:02:05Z' },
  { id: 5, direccion: 'saliente', telefono: '900000503', unidad: 'Dpto 503', destinatario: 'Elena Muestra', plantilla: 'recordatorio_deuda', texto: 'Hola Elena, tu recibo de setiembre (S/ 760,00) venció el 15-09.', estado: 'fallido', error: 'Número sin WhatsApp', fecha: '2026-09-26T14:10:00Z' },
  { id: 6, direccion: 'saliente', telefono: '900000104', unidad: 'Dpto 104', destinatario: 'Jorge Demo', plantilla: 'recordatorio_deuda', texto: 'Hola Jorge, tu recibo de setiembre (S/ 760,00) venció el 15-09.', estado: 'enviado', fecha: '2026-09-26T14:10:00Z' },
  { id: 7, direccion: 'saliente', telefono: '900000002', unidad: null, destinatario: 'Presidente Demo', plantilla: 'voto_pendiente', texto: 'INC-014 Bomba de agua N.º 2 (S/ 1.850,00) espera tu voto.', estado: 'en_cola', fecha: '2026-09-28T15:00:00Z' },
];

export const PLANTILLAS = [
  { id: 'recibo_emitido', nombre: 'Recibo emitido', variables: ['nombre', 'periodo', 'monto', 'vence'] },
  { id: 'recordatorio_deuda', nombre: 'Recordatorio de deuda', variables: ['nombre', 'periodo', 'monto'] },
  { id: 'reserva_confirmada', nombre: 'Reserva confirmada', variables: ['nombre', 'area', 'fecha', 'codigo'] },
  { id: 'aviso_general', nombre: 'Aviso general', variables: ['texto'] },
  { id: 'voto_pendiente', nombre: 'Voto pendiente de la junta', variables: ['trabajo', 'monto'] },
];

const MESES = ['2025-10', '2025-11', '2025-12', '2026-01', '2026-02', '2026-03', '2026-04', '2026-05', '2026-06', '2026-07', '2026-08', '2026-09'];
const COBRADO = [2098000, 2105000, 2010000, 2150000, 2040000, 2080000, 2120000, 2065000, 2090000, 2030000, 2060000, 1946000];
const EMITIDO = [2210000, 2215000, 2230000, 2225000, 2218000, 2226000, 2231000, 2229000, 2236000, 2232000, 2238000, 2240000];
export const ANALITICA = {
  cobranza_mensual: MESES.map((p, i) => ({ periodo: p, emitido: EMITIDO[i], cobrado: COBRADO[i] })),
  morosidad_mensual: MESES.map((p, i) => ({ periodo: p, pct: Math.round(((EMITIDO[i] - COBRADO[i]) / EMITIDO[i]) * 1000) / 10 })),
  consumo_agua: UNIDADES.map((u) => ({ unidad: `Dpto ${u.codigo}`, m3: Math.round(m3DeAgua(u.agua_cts) * 1000) / 1000 })),
  reservas_por_area: [
    { area: 'Parrillas', cantidad: 34, ingreso: 272000 },
    { area: 'Sala común', cantidad: 11, ingreso: 154000 },
    { area: 'Terraza', cantidad: 19, ingreso: 0 },
  ],
  incidencias_por_estado: [
    { estado: 'reportado', cantidad: 3 }, { estado: 'validado', cantidad: 1 }, { estado: 'presupuestado', cantidad: 2 },
    { estado: 'aprobado', cantidad: 0 }, { estado: 'en_ejecucion', cantidad: 1 }, { estado: 'terminado', cantidad: 3 }, { estado: 'rechazado', cantidad: 1 },
  ],
  tiempo_resolucion_dias: 4.6,
};
