-- 0022 · Cobranzas sin identificar, devoluciones (bloque A4).
-- Dinero que entra sin recibo identificado, y devoluciones de dinero.

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('cobranzas.ver',      'cobranzas', 'Ver cobranzas sin identificar y devoluciones', false),
 ('cobranzas.gestionar','cobranzas', 'Registrar, imputar y devolver cobranzas', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','cobranzas.ver'), ('superadmin','cobranzas.gestionar'),
 ('administrador','cobranzas.ver'), ('administrador','cobranzas.gestionar'),
 ('junta','cobranzas.ver');

CREATE TABLE cobranza_sin_identificar (
    id                  bigserial PRIMARY KEY,
    edificio_id         bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    periodo             text NOT NULL CHECK (periodo ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    fecha               date NOT NULL,
    monto_cts           bigint NOT NULL CHECK (monto_cts > 0),
    medio               text NOT NULL DEFAULT 'transferencia',
    cuenta_bancaria_id  bigint REFERENCES cuenta_bancaria(id),
    codigo_operacion    text NOT NULL DEFAULT '',
    descripcion         text NOT NULL DEFAULT '',
    estado              text NOT NULL DEFAULT 'pendiente' CHECK (estado IN ('pendiente','imputada','devuelta')),
    unidad_id           bigint REFERENCES unidad(id),
    creado_por          bigint REFERENCES usuario(id),
    creado_en           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cobranza_sin_idx ON cobranza_sin_identificar (edificio_id, estado, fecha);

CREATE TABLE devolucion (
    id                  bigserial PRIMARY KEY,
    edificio_id         bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    unidad_id           bigint REFERENCES unidad(id),
    cobranza_id         bigint REFERENCES cobranza_sin_identificar(id),
    monto_cts           bigint NOT NULL CHECK (monto_cts > 0),
    fecha               date NOT NULL,
    motivo              text NOT NULL DEFAULT '',
    creado_por          bigint REFERENCES usuario(id),
    creado_en           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX devolucion_idx ON devolucion (edificio_id, fecha);
