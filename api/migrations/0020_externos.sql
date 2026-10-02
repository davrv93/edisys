-- 0020 · Recibos externos e ingresos externos (bloque B4).
-- Documentos que no son la cuota de mantenimiento: alquileres y servicios a terceros,
-- y los ingresos que no vienen de un recibo.

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('externos.ver',       'externos', 'Ver recibos e ingresos externos', false),
 ('externos.registrar', 'externos', 'Registrar y cobrar externos', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','externos.ver'), ('superadmin','externos.registrar'),
 ('administrador','externos.ver'), ('administrador','externos.registrar'),
 ('junta','externos.ver');

CREATE TABLE recibo_externo (
    id                bigserial PRIMARY KEY,
    edificio_id       bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    periodo           text NOT NULL CHECK (periodo ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    cliente_id        bigint REFERENCES cliente(id) ON DELETE SET NULL,
    tercero           text NOT NULL DEFAULT '',
    concepto          text NOT NULL CHECK (length(btrim(concepto)) > 0),
    monto_cts         bigint NOT NULL CHECK (monto_cts > 0),
    estado            text NOT NULL DEFAULT 'emitido' CHECK (estado IN ('emitido','pagado','anulado')),
    fecha_emision     date NOT NULL DEFAULT (now() AT TIME ZONE 'America/Lima')::date,
    fecha_vencimiento date,
    recurrente        boolean NOT NULL DEFAULT false,
    archivo_id        bigint REFERENCES archivo(id),
    creado_por        bigint REFERENCES usuario(id),
    creado_en         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX recibo_externo_idx ON recibo_externo (edificio_id, periodo, estado);

CREATE TABLE ingreso_externo (
    id          bigserial PRIMARY KEY,
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    periodo     text NOT NULL CHECK (periodo ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    descripcion text NOT NULL CHECK (length(btrim(descripcion)) > 0),
    monto_cts   bigint NOT NULL CHECK (monto_cts > 0),
    fecha       date NOT NULL,
    medio       text NOT NULL DEFAULT 'efectivo',
    fondo_id    bigint REFERENCES fondo(id),
    archivo_id  bigint REFERENCES archivo(id),
    creado_por  bigint REFERENCES usuario(id),
    creado_en   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ingreso_externo_idx ON ingreso_externo (edificio_id, periodo);
