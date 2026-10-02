-- 0018 · Proveedores y cuentas por pagar.
-- Catálogo de proveedores y ciclo de cuentas por pagar: comprobante del proveedor,
-- pagos (totales o parciales) y el egreso que genera cada pago en el balance.

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('proveedores.ver',         'proveedores',   'Ver el catálogo de proveedores', false),
 ('proveedores.administrar', 'proveedores',   'Crear y editar proveedores', false),
 ('cuentas_pagar.ver',       'cuentas_pagar', 'Ver cuentas por pagar', false),
 ('cuentas_pagar.registrar', 'cuentas_pagar', 'Registrar y pagar cuentas por pagar', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','proveedores.ver'),        ('superadmin','proveedores.administrar'),
 ('superadmin','cuentas_pagar.ver'),      ('superadmin','cuentas_pagar.registrar'),
 ('administrador','proveedores.ver'),     ('administrador','proveedores.administrar'),
 ('administrador','cuentas_pagar.ver'),   ('administrador','cuentas_pagar.registrar'),
 ('junta','proveedores.ver'),             ('junta','cuentas_pagar.ver'),
 ('operario','proveedores.ver');

CREATE TABLE proveedor (
    id             bigserial PRIMARY KEY,
    edificio_id    bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    razon_social   text NOT NULL CHECK (length(btrim(razon_social)) > 0),
    ruc            text NOT NULL DEFAULT '',
    contacto       text NOT NULL DEFAULT '',
    telefono       text NOT NULL DEFAULT '',
    correo         text NOT NULL DEFAULT '',
    banco          text NOT NULL DEFAULT '',
    cuenta         text NOT NULL DEFAULT '',
    activo         boolean NOT NULL DEFAULT true,
    creado_por     bigint REFERENCES usuario(id),
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX proveedor_ruc_uq ON proveedor (edificio_id, ruc) WHERE ruc <> '';
CREATE INDEX proveedor_nombre_ix ON proveedor (edificio_id, lower(razon_social));

CREATE TABLE cuenta_por_pagar (
    id                     bigserial PRIMARY KEY,
    edificio_id            bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    proveedor_id           bigint NOT NULL REFERENCES proveedor(id),
    rubro_id               bigint REFERENCES rubro(id),
    concepto_id            bigint REFERENCES concepto(id),
    descripcion            text NOT NULL DEFAULT '',
    comprobante_tipo       text NOT NULL DEFAULT 'recibo' CHECK (comprobante_tipo IN ('recibo','factura','boleta','otro')),
    comprobante_numero     text NOT NULL DEFAULT '',
    fecha_emision          date,
    fecha_vencimiento      date,
    monto_cts              bigint NOT NULL CHECK (monto_cts > 0),
    pagado_cts             bigint NOT NULL DEFAULT 0 CHECK (pagado_cts >= 0),
    estado                 text NOT NULL DEFAULT 'pendiente' CHECK (estado IN ('pendiente','parcial','pagado','anulado')),
    archivo_comprobante_id bigint REFERENCES archivo(id),
    creado_por             bigint REFERENCES usuario(id),
    creado_en              timestamptz NOT NULL DEFAULT now(),
    actualizado_en         timestamptz NOT NULL DEFAULT now(),
    CHECK (pagado_cts <= monto_cts)
);
CREATE INDEX cpp_edificio_idx ON cuenta_por_pagar (edificio_id, estado, fecha_vencimiento);
CREATE INDEX cpp_proveedor_idx ON cuenta_por_pagar (proveedor_id);

CREATE TABLE cpp_pago (
    id                  bigserial PRIMARY KEY,
    cuenta_por_pagar_id bigint NOT NULL REFERENCES cuenta_por_pagar(id) ON DELETE CASCADE,
    fecha               date NOT NULL,
    monto_cts           bigint NOT NULL CHECK (monto_cts > 0),
    cuenta_cargo        text NOT NULL DEFAULT '',
    numero_operacion    text NOT NULL DEFAULT '',
    modalidad           text NOT NULL DEFAULT 'transferencia' CHECK (modalidad IN ('transferencia','efectivo','yape','plin','deposito','cheque')),
    comentario          text NOT NULL DEFAULT '',
    archivo_ticket_id   bigint REFERENCES archivo(id),
    egreso_id           bigint REFERENCES egreso(id),
    registrado_por      bigint REFERENCES usuario(id),
    creado_en           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cpp_pago_cpp_idx ON cpp_pago (cuenta_por_pagar_id);

-- El saldo de la cuenta se recalcula con cada pago (una sola fuente de verdad).
CREATE FUNCTION recalcular_cpp(p_cpp bigint) RETURNS void AS $$
    UPDATE cuenta_por_pagar c SET
        pagado_cts = s.pagado,
        estado = CASE
            WHEN c.estado = 'anulado' THEN 'anulado'
            WHEN s.pagado >= c.monto_cts THEN 'pagado'
            WHEN s.pagado > 0 THEN 'parcial'
            ELSE 'pendiente' END,
        actualizado_en = now()
    FROM (SELECT COALESCE(SUM(monto_cts),0) AS pagado FROM cpp_pago WHERE cuenta_por_pagar_id = p_cpp) s
    WHERE c.id = p_cpp;
$$ LANGUAGE sql;

CREATE FUNCTION cpp_pago_recalcula() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN PERFORM recalcular_cpp(OLD.cuenta_por_pagar_id); RETURN OLD; END IF;
    PERFORM recalcular_cpp(NEW.cuenta_por_pagar_id);
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER cpp_pago_recalcula_trg AFTER INSERT OR UPDATE OR DELETE ON cpp_pago
    FOR EACH ROW EXECUTE FUNCTION cpp_pago_recalcula();
