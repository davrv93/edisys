-- 0004 · Áreas comunes, recursos y reservas. Doble reserva y moroso bloqueados en la base.

CREATE TABLE area (
    id                      bigserial PRIMARY KEY,
    edificio_id             bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    nombre                  text NOT NULL,
    slug                    text NOT NULL,
    tarifa_cts              bigint NOT NULL DEFAULT 0 CHECK (tarifa_cts >= 0),
    franjas                 jsonb NOT NULL DEFAULT '[]',   -- [{"inicio":"12:00","fin":"17:00"}]
    aforo                   int,
    incluye                 text NOT NULL DEFAULT '',
    normas                  text NOT NULL DEFAULT '',
    anticipacion_max_dias   int NOT NULL DEFAULT 30,
    activo                  boolean NOT NULL DEFAULT true,
    UNIQUE (edificio_id, slug)
);

CREATE TABLE recurso (
    id       bigserial PRIMARY KEY,
    area_id  bigint NOT NULL REFERENCES area(id) ON DELETE CASCADE,
    nombre   text NOT NULL,
    activo   boolean NOT NULL DEFAULT true
);

CREATE SEQUENCE reserva_codigo_seq START 400;

CREATE TABLE reserva (
    id                bigserial PRIMARY KEY,
    edificio_id       bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    recurso_id        bigint NOT NULL REFERENCES recurso(id),
    unidad_id         bigint NOT NULL REFERENCES unidad(id),
    usuario_id        bigint REFERENCES usuario(id),
    codigo            text NOT NULL UNIQUE,
    inicio            timestamptz NOT NULL,
    fin               timestamptz NOT NULL,
    estado            text NOT NULL CHECK (estado IN ('pendiente_pago','confirmada','cancelada','vencida','no_show')),
    total_cts         bigint NOT NULL DEFAULT 0,
    modo_cobro        text NOT NULL DEFAULT 'cargo_recibo' CHECK (modo_cobro IN ('cargo_recibo','pago_inmediato')),
    acepta_normas     boolean NOT NULL DEFAULT false,
    vence_retencion   timestamptz,
    forzado_motivo    text,
    codigo_operacion  text,
    voucher_id        bigint REFERENCES archivo(id),
    pago_validado     boolean NOT NULL DEFAULT false,
    motivo            text,
    creado_en         timestamptz NOT NULL DEFAULT now(),
    CHECK (fin > inicio)
);
CREATE INDEX reserva_rango_idx ON reserva (edificio_id, inicio);

-- Regla dura 1: doble reserva imposible. El API traduce 23P01 a 409 FRANJA_OCUPADA.
ALTER TABLE reserva ADD CONSTRAINT reserva_sin_cruce
    EXCLUDE USING gist (recurso_id WITH =, tstzrange(inicio, fin, '[)') WITH &&)
    WHERE (estado IN ('pendiente_pago', 'confirmada'));

-- Regla dura 2: el moroso no reserva (salvo reserva forzada por el administrador con motivo).
-- El API traduce ED002 a 403 MOROSO.
CREATE FUNCTION reserva_bloquea_moroso() RETURNS trigger AS $$
BEGIN
    IF (NEW.forzado_motivo IS NULL OR btrim(NEW.forzado_motivo) = '') AND es_moroso(NEW.unidad_id) THEN
        RAISE EXCEPTION 'MOROSO: la unidad tiene deuda vencida de % céntimos', deuda_vencida_cts(NEW.unidad_id)
            USING ERRCODE = 'ED002';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER reserva_bloquea_moroso_trg BEFORE INSERT ON reserva
    FOR EACH ROW EXECUTE FUNCTION reserva_bloquea_moroso();

ALTER TABLE recibo_linea ADD CONSTRAINT recibo_linea_reserva_fk FOREIGN KEY (reserva_id) REFERENCES reserva(id) ON DELETE SET NULL;
