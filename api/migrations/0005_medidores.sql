-- 0005 · Medidores, lecturas con foto obligatoria, recibo general y reparto.

CREATE TABLE medidor (
    id                bigserial PRIMARY KEY,
    edificio_id       bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    unidad_id         bigint REFERENCES unidad(id) ON DELETE CASCADE,   -- NULL = medidor general
    tipo              text NOT NULL DEFAULT 'agua' CHECK (tipo IN ('agua','energia')),
    serie             text NOT NULL DEFAULT '',
    orden_ronda       int NOT NULL DEFAULT 0,
    lectura_inicial   numeric(14,3) NOT NULL DEFAULT 0,
    activo            boolean NOT NULL DEFAULT true
);
CREATE INDEX medidor_edificio_idx ON medidor (edificio_id, tipo);

CREATE TABLE lectura (
    id           bigserial PRIMARY KEY,
    medidor_id   bigint NOT NULL REFERENCES medidor(id) ON DELETE CASCADE,
    periodo_id   bigint NOT NULL REFERENCES periodo(id) ON DELETE CASCADE,
    valor        numeric(14,3) NOT NULL CHECK (valor >= 0),
    anterior     numeric(14,3) NOT NULL,
    consumo      numeric(14,3) NOT NULL,
    -- Regla dura 3: sin foto no hay lectura.
    foto_id      bigint NOT NULL REFERENCES archivo(id),
    tomada_en    timestamptz,
    subida_en    timestamptz NOT NULL DEFAULT now(),
    operario_id  bigint REFERENCES usuario(id),
    alerta       text CHECK (alerta IN ('NEGATIVO','PICO')),
    motivo       text,
    UNIQUE (medidor_id, periodo_id)
);

CREATE TABLE recibo_general (
    id             bigserial PRIMARY KEY,
    periodo_id     bigint NOT NULL REFERENCES periodo(id) ON DELETE CASCADE,
    tipo           text NOT NULL DEFAULT 'agua' CHECK (tipo IN ('agua','energia')),
    monto_cts      bigint NOT NULL CHECK (monto_cts > 0),
    consumo_total  numeric(14,3) NOT NULL CHECK (consumo_total > 0),
    foto_id        bigint NOT NULL REFERENCES archivo(id),
    registrado_por bigint REFERENCES usuario(id),
    creado_en      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (periodo_id, tipo)
);

CREATE TABLE reparto_medidor (
    id              bigserial PRIMARY KEY,
    periodo_id      bigint NOT NULL REFERENCES periodo(id) ON DELETE CASCADE,
    tipo            text NOT NULL,
    tarifa_cts_x_1000 bigint NOT NULL,
    total_unidades_cts bigint NOT NULL,
    diferencia_cts  bigint NOT NULL,
    lineas          jsonb NOT NULL,
    aprobado_por    bigint REFERENCES usuario(id),
    aprobado_en     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (periodo_id, tipo)
);

ALTER TABLE recibo_linea ADD CONSTRAINT recibo_linea_lectura_fk FOREIGN KEY (lectura_id) REFERENCES lectura(id) ON DELETE SET NULL;
