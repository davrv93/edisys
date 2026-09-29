-- 0011 · Conciliación bancaria: extractos, sus movimientos y el mapeo de columnas por banco.

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('balance.conciliar', 'balance', 'Conciliar el banco con pagos y egresos', false);
INSERT INTO rol_permiso (rol, permiso) VALUES ('superadmin', 'balance.conciliar'), ('administrador', 'balance.conciliar');

CREATE TABLE banco_mapeo (
    edificio_id  bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    banco        text NOT NULL,
    mapeo        jsonb NOT NULL,
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (edificio_id, banco)
);

CREATE TABLE extracto (
    id               bigserial PRIMARY KEY,
    edificio_id      bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    banco            text NOT NULL,
    periodo          text NOT NULL CHECK (periodo ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    archivo_nombre   text NOT NULL DEFAULT '',
    saldo_inicial_cts bigint NOT NULL DEFAULT 0,
    saldo_final_cts  bigint NOT NULL,
    subido_por       bigint REFERENCES usuario(id),
    creado_en        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (edificio_id, periodo, banco)
);

CREATE TABLE movimiento_banco (
    id               bigserial PRIMARY KEY,
    extracto_id      bigint NOT NULL REFERENCES extracto(id) ON DELETE CASCADE,
    edificio_id      bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    fecha            date NOT NULL,
    descripcion      text NOT NULL DEFAULT '',
    monto_cts        bigint NOT NULL CHECK (monto_cts <> 0),   -- > 0 abono · < 0 cargo
    codigo_operacion text NOT NULL DEFAULT '',
    estado           text NOT NULL DEFAULT 'sin_pareja' CHECK (estado IN ('sin_pareja','sugerido','conciliado')),
    regla            text NOT NULL DEFAULT '',
    pago_id          bigint REFERENCES pago(id) ON DELETE SET NULL,
    egreso_id        bigint REFERENCES egreso(id) ON DELETE SET NULL,
    confirmado_por   bigint REFERENCES usuario(id),
    confirmado_en    timestamptz,
    CHECK (pago_id IS NULL OR egreso_id IS NULL),
    CHECK (estado = 'sin_pareja' OR pago_id IS NOT NULL OR egreso_id IS NOT NULL)
);
CREATE INDEX movimiento_banco_extracto_idx ON movimiento_banco (extracto_id, fecha);
-- Regla dura: un pago o un egreso se concilia una sola vez.
CREATE UNIQUE INDEX movimiento_banco_pago_uq ON movimiento_banco (pago_id) WHERE pago_id IS NOT NULL AND estado = 'conciliado';
CREATE UNIQUE INDEX movimiento_banco_egreso_uq ON movimiento_banco (egreso_id) WHERE egreso_id IS NOT NULL AND estado = 'conciliado';
