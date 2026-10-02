-- 0021 · Vouchers multicuenta (bloque A3).
-- Cuentas bancarias del edificio y el vínculo del pago con su cuenta de origen.

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('cuentas_bancarias.ver',         'cuentas_bancarias', 'Ver las cuentas bancarias del edificio', false),
 ('cuentas_bancarias.administrar', 'cuentas_bancarias', 'Crear y editar cuentas bancarias', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','cuentas_bancarias.ver'), ('superadmin','cuentas_bancarias.administrar'),
 ('administrador','cuentas_bancarias.ver'), ('administrador','cuentas_bancarias.administrar'),
 ('junta','cuentas_bancarias.ver');

CREATE TABLE cuenta_bancaria (
    id          bigserial PRIMARY KEY,
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    banco       text NOT NULL CHECK (length(btrim(banco)) > 0),
    numero      text NOT NULL DEFAULT '',
    moneda      text NOT NULL DEFAULT 'PEN' CHECK (moneda IN ('PEN','USD')),
    activo      boolean NOT NULL DEFAULT true,
    creado_en   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (edificio_id, banco, numero)
);

ALTER TABLE pago ADD COLUMN cuenta_bancaria_id bigint REFERENCES cuenta_bancaria(id);
