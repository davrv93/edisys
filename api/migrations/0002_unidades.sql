-- 0002 · Unidades, personas con historial, importación Excel y deuda inicial.

CREATE TABLE unidad (
    id                  bigserial PRIMARY KEY,
    edificio_id         bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    codigo              text NOT NULL,
    tipo                text NOT NULL DEFAULT 'departamento'
                        CHECK (tipo IN ('departamento','estacionamiento','deposito','local')),
    piso                int,
    participacion_pct   numeric(9,4) NOT NULL DEFAULT 0 CHECK (participacion_pct >= 0 AND participacion_pct <= 100),
    alquilado           boolean NOT NULL DEFAULT false,
    activo              boolean NOT NULL DEFAULT true,
    permisos_inquilino  jsonb NOT NULL DEFAULT '{"reservar": true, "reportar": true, "ver_recibos": false}',
    creado_en           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (edificio_id, codigo)
);

CREATE TABLE persona (
    id          bigserial PRIMARY KEY,
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    nombre      text NOT NULL,
    dni_ruc     text NOT NULL DEFAULT '',
    correo      text NOT NULL DEFAULT '',
    celular     text NOT NULL DEFAULT '',
    usuario_id  bigint REFERENCES usuario(id) ON DELETE SET NULL,
    creado_en   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX persona_dni_idx ON persona (edificio_id, dni_ruc);
CREATE INDEX persona_celular_idx ON persona (celular);

-- Historial: cambiar de propietario cierra al anterior con «hasta»; nunca se sobrescribe (RF-02).
CREATE TABLE unidad_persona (
    id          bigserial PRIMARY KEY,
    unidad_id   bigint NOT NULL REFERENCES unidad(id) ON DELETE CASCADE,
    persona_id  bigint NOT NULL REFERENCES persona(id) ON DELETE CASCADE,
    rol         text NOT NULL CHECK (rol IN ('propietario','inquilino')),
    desde       date NOT NULL DEFAULT CURRENT_DATE,
    hasta       date,
    creado_en   timestamptz NOT NULL DEFAULT now()
);
-- Un solo propietario/inquilino vigente por unidad y rol.
CREATE UNIQUE INDEX unidad_persona_vigente_uq ON unidad_persona (unidad_id, rol) WHERE hasta IS NULL;

CREATE TABLE deuda_inicial (
    id          bigserial PRIMARY KEY,
    unidad_id   bigint NOT NULL REFERENCES unidad(id) ON DELETE CASCADE,
    periodo     text NOT NULL CHECK (periodo ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    monto_cts   bigint NOT NULL CHECK (monto_cts > 0),
    UNIQUE (unidad_id, periodo)
);

CREATE TABLE importacion (
    id            bigserial PRIMARY KEY,
    edificio_id   bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    usuario_id    bigint REFERENCES usuario(id),
    estado        text NOT NULL DEFAULT 'validada' CHECK (estado IN ('validada','confirmada','descartada')),
    archivo_nombre text NOT NULL DEFAULT '',
    resumen       jsonb NOT NULL DEFAULT '{}',
    filas         jsonb NOT NULL DEFAULT '[]',
    deudas        jsonb NOT NULL DEFAULT '[]',
    creado_en     timestamptz NOT NULL DEFAULT now(),
    confirmada_en timestamptz
);

-- Regla dura: la participación de las unidades activas de un edificio suma 100 % (tolerancia 0,0001).
-- Es un trigger de restricción DIFERIDO: se comprueba al COMMIT, así una importación puede
-- mover varias participaciones dentro de la misma transacción. Un edificio vacío (suma 0) pasa.
CREATE FUNCTION validar_participacion_edificio() RETURNS trigger AS $$
DECLARE
    v_edificio bigint;
    v_suma     numeric;
    v_estricta boolean;
BEGIN
    IF TG_OP = 'DELETE' THEN v_edificio := OLD.edificio_id; ELSE v_edificio := NEW.edificio_id; END IF;
    SELECT participacion_estricta INTO v_estricta FROM edificio WHERE id = v_edificio;
    IF NOT COALESCE(v_estricta, false) THEN RETURN NULL; END IF;
    SELECT COALESCE(SUM(participacion_pct), 0) INTO v_suma FROM unidad WHERE edificio_id = v_edificio AND activo;
    IF v_suma <> 0 AND abs(v_suma - 100) > 0.0001 THEN
        RAISE EXCEPTION 'PARTICIPACION_NO_SUMA_100: las participaciones suman % %%; deben sumar 100 %%.', round(v_suma, 4)
            USING ERRCODE = 'ED001';
    END IF;
    RETURN NULL;
END $$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER unidad_participacion_100
    AFTER INSERT OR UPDATE OF participacion_pct, activo OR DELETE ON unidad
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION validar_participacion_edificio();
