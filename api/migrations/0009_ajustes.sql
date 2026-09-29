-- 0009 · Ajustes (notas de cargo o de abono internas).
-- Cuando se corrige una lectura de un periodo cuyos recibos ya se emitieron, el recibo emitido no se toca:
-- la diferencia del reparto se guarda aquí y entra como línea «ajuste» en el siguiente recibo de la unidad.

CREATE TABLE ajuste (
    id              bigserial PRIMARY KEY,
    edificio_id     bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    unidad_id       bigint NOT NULL REFERENCES unidad(id) ON DELETE CASCADE,
    monto_cts       bigint NOT NULL CHECK (monto_cts <> 0),   -- > 0 nota de cargo · < 0 nota de abono
    tipo            text GENERATED ALWAYS AS (CASE WHEN monto_cts > 0 THEN 'cargo' ELSE 'abono' END) STORED,
    motivo          text NOT NULL CHECK (btrim(motivo) <> ''),
    periodo_origen  text NOT NULL CHECK (periodo_origen ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    lectura_id      bigint REFERENCES lectura(id) ON DELETE SET NULL,
    recibo_id       bigint REFERENCES recibo(id) ON DELETE SET NULL,  -- recibo donde se aplicó (NULL = pendiente)
    creado_por      bigint REFERENCES usuario(id),
    creado_en       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ajuste_pendiente_idx ON ajuste (edificio_id, unidad_id) WHERE recibo_id IS NULL;

ALTER TABLE recibo_linea ADD COLUMN ajuste_id bigint REFERENCES ajuste(id) ON DELETE SET NULL;
