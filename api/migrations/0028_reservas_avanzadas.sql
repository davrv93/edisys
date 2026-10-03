-- 0028 · Reservas avanzadas (bloques H1, H2 y H3).
-- H1 · check-in con QR: el conserje escanea la entrada y la base comprueba franja y deuda el día del evento.
-- H2 · restricciones: anticipación mínima, separación entre reservas, garantía, limpieza y excepciones de morosidad.
-- H3 · configuración completa: horario por día de la semana, tarifa por franja, descripción, reglamento, fotos y cupos.

-- ---------- permisos ----------

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('reservas.checkin', 'reservas', 'Registrar el ingreso con QR a las áreas comunes', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','reservas.checkin'), ('administrador','reservas.checkin'), ('operario','reservas.checkin');

-- ---------- H2 · restricciones avanzadas del área ----------

ALTER TABLE area
    ADD COLUMN anticipacion_min_dias int    NOT NULL DEFAULT 0 CHECK (anticipacion_min_dias >= 0),
    ADD COLUMN separacion_dias       int    NOT NULL DEFAULT 0 CHECK (separacion_dias >= 0),  -- días entre dos reservas de la misma unidad
    ADD COLUMN garantia_cts          bigint NOT NULL DEFAULT 0 CHECK (garantia_cts >= 0),
    ADD COLUMN limpieza_cts          bigint NOT NULL DEFAULT 0 CHECK (limpieza_cts >= 0),
    -- Morosos que igual reservan: con deuda parcial (hasta deuda_tolerada_cts) o con la deuda financiada (acuerdo de pago activo).
    ADD COLUMN permite_parciales     boolean NOT NULL DEFAULT false,
    ADD COLUMN deuda_tolerada_cts    bigint  NOT NULL DEFAULT 0 CHECK (deuda_tolerada_cts >= 0),
    ADD COLUMN permite_financiados   boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT area_anticipacion_ck CHECK (anticipacion_min_dias <= anticipacion_max_dias);

-- ---------- H3 · configuración completa del área ----------

ALTER TABLE area
    ADD COLUMN descripcion            text  NOT NULL DEFAULT '',
    -- {"1":[{"inicio":"10:00","fin":"14:00","tarifa_cts":5000}], …, "7":[]} (1 = lunes). Día sin clave usa franjas; [] = cerrado.
    ADD COLUMN horarios               jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(horarios) = 'object'),
    ADD COLUMN reglamento_archivo_id  bigint REFERENCES archivo(id) ON DELETE SET NULL,
    ADD COLUMN cupo_mensual_unidad    int CHECK (cupo_mensual_unidad IS NULL OR cupo_mensual_unidad > 0),
    ADD COLUMN checkin_tolerancia_min int NOT NULL DEFAULT 30 CHECK (checkin_tolerancia_min BETWEEN 0 AND 240);

CREATE TABLE area_foto (
    id          bigserial PRIMARY KEY,
    area_id     bigint NOT NULL REFERENCES area(id) ON DELETE CASCADE,
    archivo_id  bigint NOT NULL REFERENCES archivo(id),
    orden       int NOT NULL DEFAULT 0,
    creado_en   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX area_foto_idx ON area_foto (area_id, orden);

-- ---------- reserva: desglose, datos del evento y check-in ----------

ALTER TABLE reserva
    ADD COLUMN garantia_cts           bigint NOT NULL DEFAULT 0 CHECK (garantia_cts >= 0),   -- incluido en total_cts
    ADD COLUMN limpieza_cts           bigint NOT NULL DEFAULT 0 CHECK (limpieza_cts >= 0),   -- incluido en total_cts
    ADD COLUMN titulo                 text   NOT NULL DEFAULT '',
    ADD COLUMN asistentes             int CHECK (asistentes IS NULL OR asistentes > 0),
    -- Token de la entrada (lo que lleva el QR). Aleatorio: el código R-0000 se adivina, el token no.
    ADD COLUMN qr_token               text NOT NULL DEFAULT md5(random()::text || clock_timestamp()::text),
    ADD COLUMN checkin_en             timestamptz,
    ADD COLUMN checkin_por            bigint REFERENCES usuario(id),
    ADD COLUMN checkin_valido         boolean,
    ADD COLUMN checkin_motivo         text,
    ADD COLUMN checkin_forzado_motivo text,
    ADD CONSTRAINT reserva_qr_token_uq UNIQUE (qr_token),
    ADD CONSTRAINT reserva_checkin_ck CHECK ((checkin_en IS NULL) = (checkin_valido IS NULL));

-- Bitácora de cada escaneo, válido o rechazado (la reserva solo guarda el último).
CREATE TABLE reserva_checkin (
    id              bigserial PRIMARY KEY,
    reserva_id      bigint NOT NULL REFERENCES reserva(id) ON DELETE CASCADE,
    usuario_id      bigint REFERENCES usuario(id),
    en              timestamptz NOT NULL DEFAULT now(),
    valido          boolean NOT NULL,
    motivos         jsonb NOT NULL DEFAULT '[]',
    deuda_cts       bigint NOT NULL DEFAULT 0,
    forzado_motivo  text
);
CREATE INDEX reserva_checkin_idx ON reserva_checkin (reserva_id, en DESC);

-- ---------- reglas en la base ----------

-- ¿La unidad tiene la deuda financiada? Lee acuerdo_pago (bloque D1) solo si esa tabla existe,
-- así esta migración no depende del orden en que lleguen los bloques.
CREATE FUNCTION reserva_unidad_financiada(p_unidad bigint) RETURNS boolean AS $$
DECLARE r boolean;
BEGIN
    IF to_regclass('acuerdo_pago') IS NULL THEN
        RETURN false;
    END IF;
    EXECUTE 'SELECT EXISTS (SELECT 1 FROM acuerdo_pago WHERE unidad_id = $1 AND estado IN (''activo'', ''vigente''))' INTO r USING p_unidad;
    RETURN COALESCE(r, false);
EXCEPTION WHEN undefined_column OR undefined_table THEN
    RETURN false;
END $$ LANGUAGE plpgsql STABLE;

-- ¿La unidad puede usar el área? Al día, o morosa dentro de las excepciones que el área permite (H2).
CREATE FUNCTION reserva_unidad_habilitada(p_unidad bigint, p_area bigint) RETURNS boolean AS $$
DECLARE
    deuda bigint := deuda_vencida_cts(p_unidad);
    a     area%ROWTYPE;
BEGIN
    IF deuda <= 0 THEN
        RETURN true;
    END IF;
    SELECT * INTO a FROM area WHERE id = p_area;
    IF NOT FOUND THEN
        RETURN false;
    END IF;
    IF a.permite_parciales AND deuda <= a.deuda_tolerada_cts THEN
        RETURN true;
    END IF;
    IF a.permite_financiados AND reserva_unidad_financiada(p_unidad) THEN
        RETURN true;
    END IF;
    RETURN false;
END $$ LANGUAGE plpgsql STABLE;

-- Regla dura 2 (0004), ahora con las excepciones del área. Mismo disparador, mismo ED002.
CREATE OR REPLACE FUNCTION reserva_bloquea_moroso() RETURNS trigger AS $$
BEGIN
    IF (NEW.forzado_motivo IS NULL OR btrim(NEW.forzado_motivo) = '')
       AND NOT reserva_unidad_habilitada(NEW.unidad_id, (SELECT area_id FROM recurso WHERE id = NEW.recurso_id)) THEN
        RAISE EXCEPTION 'MOROSO: la unidad tiene deuda vencida de % céntimos', deuda_vencida_cts(NEW.unidad_id)
            USING ERRCODE = 'ED002';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

-- Regla dura 3 (H1): un ingreso válido exige reserva confirmada, estar dentro de la franja
-- (con la tolerancia del área antes del inicio) y la unidad al día ESE día, salvo ingreso forzado con motivo.
-- Un ingreso válido no se reescribe. El API traduce EDR01..EDR04.
CREATE FUNCTION reserva_valida_checkin() RETURNS trigger AS $$
DECLARE tol int;
BEGIN
    IF OLD.checkin_valido IS TRUE AND (NEW.checkin_en IS DISTINCT FROM OLD.checkin_en OR NEW.checkin_valido IS DISTINCT FROM OLD.checkin_valido) THEN
        RAISE EXCEPTION 'CHECKIN_REPETIDO: la reserva % ya registró su ingreso', OLD.codigo USING ERRCODE = 'EDR04';
    END IF;
    IF NEW.checkin_valido IS TRUE AND OLD.checkin_valido IS DISTINCT FROM TRUE THEN
        IF NEW.estado <> 'confirmada' THEN
            RAISE EXCEPTION 'CHECKIN_NO_CONFIRMADA: la reserva está %', NEW.estado USING ERRCODE = 'EDR01';
        END IF;
        SELECT a.checkin_tolerancia_min INTO tol FROM recurso r JOIN area a ON a.id = r.area_id WHERE r.id = NEW.recurso_id;
        IF NEW.checkin_en < NEW.inicio - make_interval(mins => COALESCE(tol, 30)) OR NEW.checkin_en >= NEW.fin THEN
            RAISE EXCEPTION 'CHECKIN_FUERA_DE_FRANJA: el ingreso no cae en la franja reservada' USING ERRCODE = 'EDR02';
        END IF;
        IF (NEW.checkin_forzado_motivo IS NULL OR btrim(NEW.checkin_forzado_motivo) = '')
           AND NOT reserva_unidad_habilitada(NEW.unidad_id, (SELECT area_id FROM recurso WHERE id = NEW.recurso_id)) THEN
            RAISE EXCEPTION 'CHECKIN_MOROSO: la unidad tiene deuda vencida de % céntimos', deuda_vencida_cts(NEW.unidad_id)
                USING ERRCODE = 'EDR03';
        END IF;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER reserva_valida_checkin_trg BEFORE UPDATE ON reserva
    FOR EACH ROW EXECUTE FUNCTION reserva_valida_checkin();
