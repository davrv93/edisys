-- Datos de demostración para los módulos nuevos (idempotente).
-- Se aplica sobre el edificio demo (id 1). No borra nada: solo inserta si falta.
-- Sirve para que el Tour y el Modo demo tengan contenido en cada pantalla.

-- Periodo y rubro de referencia del edificio demo.
WITH ref AS (
  SELECT (SELECT id FROM edificio WHERE id=1) AS eid,
         (SELECT max(periodo) FROM periodo WHERE edificio_id=1) AS periodo,
         (SELECT id FROM rubro WHERE edificio_id=1 ORDER BY orden LIMIT 1) AS rubro,
         (SELECT id FROM archivo WHERE edificio_id=1 AND tipo_mime LIKE 'image/%' ORDER BY id LIMIT 1) AS archivo
)
-- 1. Cuentas bancarias
INSERT INTO cuenta_bancaria (edificio_id, banco, numero, moneda)
SELECT 1, b.banco, b.numero, 'PEN' FROM (VALUES ('Banco de Crédito', '194-0123456-0-01'), ('BBVA', '0011-0234-0100123456')) AS b(banco, numero)
WHERE NOT EXISTS (SELECT 1 FROM cuenta_bancaria WHERE edificio_id=1);

-- 2. Proveedores
INSERT INTO proveedor (edificio_id, razon_social, ruc, contacto, telefono, correo, banco, cuenta)
SELECT 1, p.razon, p.ruc, p.contacto, p.tel, p.correo, p.banco, p.cuenta FROM (VALUES
  ('Sedapal', '20100070970', 'Mesa de partes', '987654321', 'cobranzas@sedapal.pe', 'Banco de la Nación', '00-123-456789'),
  ('Luz del Sur', '20260545946', 'Atención', '981234567', 'empresas@luzdelsur.pe', 'Banco de Crédito', '193-1122334-0-55'),
  ('Ascensores Andinos', '20512345678', 'Ing. Ríos', '999888777', 'servicio@ascandinos.pe', 'Interbank', '898-3001234567')
) AS p(razon, ruc, contacto, tel, correo, banco, cuenta)
WHERE NOT EXISTS (SELECT 1 FROM proveedor WHERE edificio_id=1);

-- 3. Cuentas por pagar (una pagada, una parcial, una pendiente)
INSERT INTO cuenta_por_pagar (edificio_id, proveedor_id, rubro_id, descripcion, comprobante_tipo, comprobante_numero, fecha_emision, fecha_vencimiento, monto_cts, pagado_cts, estado, archivo_comprobante_id)
SELECT 1, p.id, (SELECT id FROM rubro WHERE edificio_id=1 ORDER BY orden LIMIT 1), c.descripcion, c.tipo, c.numero,
       (now() AT TIME ZONE 'America/Lima')::date - 20, (now() AT TIME ZONE 'America/Lima')::date + 5, c.monto, c.pagado,
       CASE WHEN c.pagado >= c.monto THEN 'pagado' WHEN c.pagado > 0 THEN 'parcial' ELSE 'pendiente' END,
       (SELECT id FROM archivo WHERE edificio_id=1 AND tipo_mime LIKE 'image/%' ORDER BY id LIMIT 1)
FROM (VALUES
  ('Sedapal', 'Recibo de agua del mes', 'recibo', 'SUM-4586612', 500000::bigint, 500000::bigint),
  ('Luz del Sur', 'Suministro de áreas comunes', 'recibo', 'LDS-993210', 98000::bigint, 40000::bigint),
  ('Ascensores Andinos', 'Mantenimiento preventivo mensual', 'factura', 'F001-1042', 160000::bigint, 0::bigint)
) AS c(prov, descripcion, tipo, numero, monto, pagado)
JOIN proveedor p ON p.razon_social = c.prov AND p.edificio_id = 1
WHERE NOT EXISTS (SELECT 1 FROM cuenta_por_pagar WHERE edificio_id=1);

-- 4. Cobranzas sin identificar (pendientes de imputar)
INSERT INTO cobranza_sin_identificar (edificio_id, periodo, fecha, monto_cts, medio, codigo_operacion, descripcion, estado)
SELECT 1, to_char((now() AT TIME ZONE 'America/Lima'), 'YYYY-MM'), (now() AT TIME ZONE 'America/Lima')::date - 1, m.monto, m.medio, m.op, m.detalle, 'pendiente'
FROM (VALUES (36800::bigint, 'transferencia', 'OP-99871', 'Depósito sin recibo identificado'), (12000::bigint, 'yape', 'YP-4451', 'Yape sin unidad')) AS m(monto, medio, op, detalle)
WHERE NOT EXISTS (SELECT 1 FROM cobranza_sin_identificar WHERE edificio_id=1);

-- 5. Documentos (categorías + documentos reutilizando una imagen del seed)
INSERT INTO documento_categoria (edificio_id, nombre, orden)
SELECT 1, d.nombre, d.orden FROM (VALUES ('Actas de junta', 1), ('Reglamentos', 2), ('Comunicados', 3)) AS d(nombre, orden)
WHERE NOT EXISTS (SELECT 1 FROM documento_categoria WHERE edificio_id=1);

INSERT INTO documento_publicado (edificio_id, categoria_id, titulo, numero, resumen, archivo_id, publicado)
SELECT 1, c.id, d.titulo, d.numero, d.resumen,
       (SELECT id FROM archivo WHERE edificio_id=1 AND tipo_mime LIKE 'image/%' ORDER BY id LIMIT 1), d.publicado
FROM (VALUES
  ('Actas de junta', 'Acta de sesión ordinaria', '2026-08', 'Acuerdos de la sesión de agosto', true),
  ('Reglamentos', 'Manual de convivencia', '', 'Normas del edificio', true),
  ('Comunicados', 'Corte de agua programado', '', 'Mantenimiento de cisterna el sábado', true)
) AS d(cat, titulo, numero, resumen, publicado)
JOIN documento_categoria c ON c.nombre = d.cat AND c.edificio_id = 1
WHERE NOT EXISTS (SELECT 1 FROM documento_publicado WHERE edificio_id=1);

-- 6. Movimientos de fondos de demostración (para que la trazabilidad tenga contenido)
INSERT INTO fondo_movimiento (edificio_id, fondo_id, periodo, fecha, monto_cts, tipo, origen, descripcion)
SELECT 1, f.id, to_char((now() AT TIME ZONE 'America/Lima'), 'YYYY-MM'), (now() AT TIME ZONE 'America/Lima')::date,
       m.monto, m.tipo, 'manual', m.detalle
FROM (VALUES
  ('cuota', 800000::bigint, 'ingreso', 'Cobranza del mes (demo)'),
  ('agua', 300000::bigint, 'ingreso', 'Cobranza de agua (demo)'),
  ('luz', 98000::bigint, 'ingreso', 'Cobranza de luz (demo)'),
  ('general', -500000::bigint, 'egreso', 'Pago a Sedapal (demo)'),
  ('general', -160000::bigint, 'egreso', 'Ascensores Andinos (demo)')
) AS m(cod, monto, tipo, detalle)
JOIN fondo f ON f.codigo = m.cod AND f.edificio_id = 1
WHERE NOT EXISTS (SELECT 1 FROM fondo_movimiento WHERE edificio_id=1 AND origen='manual');
