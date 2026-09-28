-- 0006 · Mantenimiento: incidencias (= trabajos), línea de tiempo, evidencias, votos de la junta.

CREATE TABLE junta_miembro (
    edificio_id  bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    usuario_id   bigint NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    cargo        text NOT NULL DEFAULT 'miembro',
    presidente   boolean NOT NULL DEFAULT false,
    activo       boolean NOT NULL DEFAULT true,
    PRIMARY KEY (edificio_id, usuario_id)
);
CREATE UNIQUE INDEX junta_un_presidente_uq ON junta_miembro (edificio_id) WHERE presidente AND activo;

CREATE TABLE incidencia (
    id                      bigserial PRIMARY KEY,
    edificio_id             bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    numero                  int NOT NULL,
    codigo                  text NOT NULL,
    titulo                  text NOT NULL,
    descripcion             text NOT NULL DEFAULT '',
    ubicacion               text NOT NULL DEFAULT '',
    categoria               text NOT NULL DEFAULT 'otros'
                            CHECK (categoria IN ('gasfiteria','electricidad','ascensores','bombas','limpieza','seguridad','areas_comunes','estructura','jardineria','otros')),
    criticidad              text CHECK (criticidad IN ('critica','media','baja')),
    estado                  text NOT NULL DEFAULT 'reportado'
                            CHECK (estado IN ('reportado','validado','presupuestado','aprobado','en_ejecucion','terminado','rechazado','descartado')),
    origen                  text NOT NULL DEFAULT 'app' CHECK (origen IN ('app','whatsapp','admin')),
    reportado_por           bigint REFERENCES usuario(id),
    unidad_id               bigint REFERENCES unidad(id),
    responsable_id          bigint REFERENCES usuario(id),
    proveedor               text NOT NULL DEFAULT '',
    diagnostico             text NOT NULL DEFAULT '',
    monto_presupuesto_cts   bigint CHECK (monto_presupuesto_cts >= 0),
    presupuesto_archivo_id  bigint REFERENCES archivo(id),
    rubro_id                bigint REFERENCES rubro(id),
    costo_real_cts          bigint CHECK (costo_real_cts >= 0),
    comprobante_id          bigint REFERENCES archivo(id),
    revision_junta          boolean NOT NULL DEFAULT false,
    motivo                  text NOT NULL DEFAULT '',
    creado_en               timestamptz NOT NULL DEFAULT now(),
    actualizado_en          timestamptz NOT NULL DEFAULT now(),
    terminado_en            timestamptz,
    UNIQUE (edificio_id, numero),
    UNIQUE (edificio_id, codigo)
);
CREATE INDEX incidencia_estado_idx ON incidencia (edificio_id, estado);

CREATE TABLE incidencia_evento (
    id              bigserial PRIMARY KEY,
    incidencia_id   bigint NOT NULL REFERENCES incidencia(id) ON DELETE CASCADE,
    estado_desde    text,
    estado_hasta    text NOT NULL,
    usuario_id      bigint REFERENCES usuario(id),
    nota            text NOT NULL DEFAULT '',
    creado_en       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE incidencia_evidencia (
    id              bigserial PRIMARY KEY,
    incidencia_id   bigint NOT NULL REFERENCES incidencia(id) ON DELETE CASCADE,
    archivo_id      bigint NOT NULL REFERENCES archivo(id),
    tipo            text NOT NULL CHECK (tipo IN ('reporte','avance','cierre','presupuesto','comprobante')),
    usuario_id      bigint REFERENCES usuario(id),
    creado_en       timestamptz NOT NULL DEFAULT now()
);

-- Un voto por miembro y trabajo (409 YA_VOTASTE).
CREATE TABLE voto (
    id              bigserial PRIMARY KEY,
    incidencia_id   bigint NOT NULL REFERENCES incidencia(id) ON DELETE CASCADE,
    usuario_id      bigint NOT NULL REFERENCES usuario(id),
    voto            text NOT NULL CHECK (voto IN ('aprueba','rechaza')),
    comentario      text NOT NULL DEFAULT '',
    creado_en       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (incidencia_id, usuario_id)
);

-- Máquina de estados (misma tabla que internal/mantenimiento/estados.go). Regla dura en la base.
CREATE FUNCTION transicion_incidencia_valida(desde text, hasta text) RETURNS boolean AS $$
    SELECT (desde, hasta) IN (
        ('reportado','validado'), ('reportado','descartado'),
        ('validado','presupuestado'), ('validado','descartado'),
        ('presupuestado','aprobado'), ('presupuestado','rechazado'),
        ('rechazado','presupuestado'),
        ('aprobado','en_ejecucion'),
        ('en_ejecucion','terminado')
    );
$$ LANGUAGE sql IMMUTABLE;

CREATE FUNCTION incidencia_valida_estado() RETURNS trigger AS $$
BEGIN
    IF NEW.estado IS DISTINCT FROM OLD.estado THEN
        IF NOT transicion_incidencia_valida(OLD.estado, NEW.estado) THEN
            RAISE EXCEPTION 'TRANSICION_INVALIDA: de % a %', OLD.estado, NEW.estado USING ERRCODE = 'ED004';
        END IF;
        IF NEW.estado = 'terminado' THEN NEW.terminado_en := now(); END IF;
    END IF;
    NEW.actualizado_en := now();
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER incidencia_valida_estado_trg BEFORE UPDATE ON incidencia
    FOR EACH ROW EXECUTE FUNCTION incidencia_valida_estado();

ALTER TABLE egreso ADD CONSTRAINT egreso_incidencia_fk FOREIGN KEY (incidencia_id) REFERENCES incidencia(id) ON DELETE SET NULL;
