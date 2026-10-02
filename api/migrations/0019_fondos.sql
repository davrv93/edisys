-- 0019 · Fondos y trazabilidad de fondos.
-- Cada sol recaudado y cada egreso se asientan en un fondo (el «servicio» del cliente).
-- El saldo de un fondo es la suma de sus movimientos: no se recalcula por otra vía.

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('fondos.ver',        'fondos', 'Ver fondos y su trazabilidad', false),
 ('fondos.administrar','fondos', 'Crear, editar y mover fondos', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','fondos.ver'), ('superadmin','fondos.administrar'),
 ('administrador','fondos.ver'), ('administrador','fondos.administrar'),
 ('junta','fondos.ver');

CREATE TABLE fondo (
    id          bigserial PRIMARY KEY,
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    codigo      text NOT NULL,
    nombre      text NOT NULL CHECK (length(btrim(nombre)) > 0),
    categoria   text NOT NULL DEFAULT '',
    orden       int NOT NULL DEFAULT 0,
    activo      boolean NOT NULL DEFAULT true,
    creado_en   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (edificio_id, codigo)
);

-- Qué fondo recibe el cobro de cada tipo de línea del recibo.
CREATE TABLE fondo_tipo_linea (
    id          bigserial PRIMARY KEY,
    fondo_id    bigint NOT NULL REFERENCES fondo(id) ON DELETE CASCADE,
    tipo        text NOT NULL,
    UNIQUE (fondo_id, tipo)
);
CREATE INDEX fondo_tipo_tipo_ix ON fondo_tipo_linea (tipo);

-- Qué fondo paga cada rubro de egreso.
CREATE TABLE fondo_rubro (
    id        bigserial PRIMARY KEY,
    fondo_id  bigint NOT NULL REFERENCES fondo(id) ON DELETE CASCADE,
    rubro_id  bigint NOT NULL REFERENCES rubro(id) ON DELETE CASCADE,
    UNIQUE (rubro_id)
);

-- Mayor contable por fondo. monto_cts con signo: ingreso (+), egreso o salida (−).
CREATE TABLE fondo_movimiento (
    id          bigserial PRIMARY KEY,
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    fondo_id    bigint NOT NULL REFERENCES fondo(id) ON DELETE CASCADE,
    periodo     text NOT NULL CHECK (periodo ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    fecha       date NOT NULL,
    monto_cts   bigint NOT NULL,
    tipo        text NOT NULL CHECK (tipo IN ('ingreso','egreso','transferencia_entrada','transferencia_salida','ajuste')),
    origen      text NOT NULL DEFAULT 'manual' CHECK (origen IN ('pago','egreso','manual','transferencia')),
    ref_id      bigint,
    descripcion text NOT NULL DEFAULT '',
    creado_por  bigint REFERENCES usuario(id),
    creado_en   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX fondo_mov_fondo_idx ON fondo_movimiento (edificio_id, fondo_id, periodo);
CREATE INDEX fondo_mov_origen_idx ON fondo_movimiento (origen, ref_id);

ALTER TABLE egreso ADD COLUMN fondo_id bigint REFERENCES fondo(id);
