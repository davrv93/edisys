-- 0003 · Rubros, periodos, presupuesto, recibos, pagos, egresos y morosidad.

CREATE TABLE rubro (
    id          bigserial PRIMARY KEY,
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    slug        text NOT NULL,
    nombre      text NOT NULL,
    orden       int NOT NULL DEFAULT 0,
    UNIQUE (edificio_id, slug)
);

CREATE TABLE concepto (
    id        bigserial PRIMARY KEY,
    rubro_id  bigint NOT NULL REFERENCES rubro(id) ON DELETE CASCADE,
    slug      text NOT NULL,
    nombre    text NOT NULL,
    orden     int NOT NULL DEFAULT 0,
    UNIQUE (rubro_id, slug)
);

CREATE TABLE periodo (
    id           bigserial PRIMARY KEY,
    edificio_id  bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    periodo      text NOT NULL CHECK (periodo ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    fecha_corte  date NOT NULL,
    estado       text NOT NULL DEFAULT 'abierto' CHECK (estado IN ('abierto','emitido','cerrado')),
    creado_en    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (edificio_id, periodo)
);

CREATE TABLE presupuesto (
    periodo_id  bigint NOT NULL REFERENCES periodo(id) ON DELETE CASCADE,
    rubro_id    bigint NOT NULL REFERENCES rubro(id),
    monto_cts   bigint NOT NULL CHECK (monto_cts >= 0),
    PRIMARY KEY (periodo_id, rubro_id)
);

CREATE SEQUENCE recibo_correlativo_seq START 100;

CREATE TABLE recibo (
    id           bigserial PRIMARY KEY,
    edificio_id  bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    periodo_id   bigint NOT NULL REFERENCES periodo(id) ON DELETE CASCADE,
    unidad_id    bigint NOT NULL REFERENCES unidad(id),
    numero       text,
    correlativo  text,
    estado       text NOT NULL DEFAULT 'borrador'
                 CHECK (estado IN ('borrador','emitido','pagado_parcial','pagado','anulado')),
    total_cts    bigint NOT NULL DEFAULT 0 CHECK (total_cts >= 0),
    pagado_cts   bigint NOT NULL DEFAULT 0 CHECK (pagado_cts >= 0),
    emitido_en   timestamptz,
    vence        date,
    enviado_en   timestamptz,
    anulado_motivo text,
    creado_en    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX recibo_unidad_periodo_uq ON recibo (periodo_id, unidad_id) WHERE estado <> 'anulado';
CREATE INDEX recibo_unidad_idx ON recibo (unidad_id);

CREATE TABLE recibo_linea (
    id           bigserial PRIMARY KEY,
    recibo_id    bigint NOT NULL REFERENCES recibo(id) ON DELETE CASCADE,
    tipo         text NOT NULL CHECK (tipo IN ('cuota','agua','agua_comun','energia_comun','reserva','concepto','multa','saldo_anterior')),
    descripcion  text NOT NULL,
    monto_cts    bigint NOT NULL,
    orden        int NOT NULL DEFAULT 0,
    reserva_id   bigint,
    lectura_id   bigint
);
CREATE INDEX recibo_linea_recibo_idx ON recibo_linea (recibo_id);

CREATE TABLE pago (
    id                bigserial PRIMARY KEY,
    edificio_id       bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    recibo_id         bigint NOT NULL REFERENCES recibo(id) ON DELETE CASCADE,
    monto_cts         bigint NOT NULL CHECK (monto_cts > 0),
    medio             text NOT NULL CHECK (medio IN ('yape','plin','transferencia','efectivo','deposito','tarjeta')),
    codigo_operacion  text,
    fecha             date NOT NULL,
    voucher_id        bigint REFERENCES archivo(id),
    estado            text NOT NULL DEFAULT 'validado' CHECK (estado IN ('pendiente_validacion','validado','rechazado')),
    motivo            text,
    registrado_por    bigint REFERENCES usuario(id),
    validado_por      bigint REFERENCES usuario(id),
    validado_en       timestamptz,
    creado_en         timestamptz NOT NULL DEFAULT now()
);
-- El mismo código de operación por medio y fecha no entra dos veces (409 PAGO_DUPLICADO).
CREATE UNIQUE INDEX pago_operacion_uq ON pago (edificio_id, medio, fecha, codigo_operacion)
    WHERE codigo_operacion IS NOT NULL AND codigo_operacion <> '' AND estado <> 'rechazado';

-- El recibo se recalcula solo con cada pago: pagado = suma de pagos validados.
CREATE FUNCTION recalcular_recibo(p_recibo bigint) RETURNS void AS $$
    UPDATE recibo r SET
        pagado_cts = s.pagado,
        estado = CASE
            WHEN r.estado IN ('borrador','anulado') THEN r.estado
            WHEN s.pagado >= r.total_cts THEN 'pagado'
            WHEN s.pagado > 0 THEN 'pagado_parcial'
            ELSE 'emitido' END
    FROM (SELECT COALESCE(SUM(monto_cts) FILTER (WHERE estado = 'validado'), 0) AS pagado
          FROM pago WHERE recibo_id = p_recibo) s
    WHERE r.id = p_recibo;
$$ LANGUAGE sql;

CREATE FUNCTION pago_recalcula() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN PERFORM recalcular_recibo(OLD.recibo_id); RETURN OLD; END IF;
    PERFORM recalcular_recibo(NEW.recibo_id);
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER pago_recalcula_trg AFTER INSERT OR UPDATE OR DELETE ON pago
    FOR EACH ROW EXECUTE FUNCTION pago_recalcula();

CREATE TABLE egreso (
    id                   bigserial PRIMARY KEY,
    edificio_id          bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    periodo              text NOT NULL CHECK (periodo ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    rubro_id             bigint NOT NULL REFERENCES rubro(id),
    concepto_id          bigint REFERENCES concepto(id),
    descripcion          text NOT NULL,
    monto_cts            bigint NOT NULL CHECK (monto_cts > 0),
    fecha                date NOT NULL,
    documento_id         bigint REFERENCES archivo(id),
    tipo_documento       text NOT NULL DEFAULT 'foto' CHECK (tipo_documento IN ('foto','pdf','voucher')),
    origen               text NOT NULL DEFAULT 'manual' CHECK (origen IN ('manual','trabajo')),
    incidencia_id        bigint,
    movimiento_banco_id  bigint,
    registrado_por       bigint REFERENCES usuario(id),
    creado_en            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX egreso_periodo_idx ON egreso (edificio_id, periodo);

-- Morosidad (una sola función para 03, 05 y 07):
-- deuda vencida = saldo de recibos emitidos cuyo vencimiento + días de gracia ya pasó (hora de Lima).
CREATE FUNCTION deuda_vencida_cts(p_unidad bigint) RETURNS bigint AS $$
    SELECT COALESCE(SUM(r.total_cts - r.pagado_cts), 0)::bigint
    FROM recibo r JOIN edificio e ON e.id = r.edificio_id
    WHERE r.unidad_id = p_unidad
      AND r.estado IN ('emitido','pagado_parcial')
      AND r.vence IS NOT NULL
      AND r.vence + e.dias_gracia < (now() AT TIME ZONE 'America/Lima')::date;
$$ LANGUAGE sql STABLE;

CREATE FUNCTION es_moroso(p_unidad bigint) RETURNS boolean AS $$
    SELECT deuda_vencida_cts(p_unidad) > 0;
$$ LANGUAGE sql STABLE;
