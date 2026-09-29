-- 0016 · Tablero configurable: columnas visibles en orden y campos de la tarjeta,
-- por edificio. NULL = lo de fábrica (las 8 etapas en orden, tarjeta completa).
-- El grafo de transiciones NO es configurable (regla dura en 0006 + estados.go).
CREATE TABLE mantenimiento_config (
  edificio_id     bigint PRIMARY KEY REFERENCES edificio(id) ON DELETE CASCADE,
  columnas        jsonb,
  tarjeta_campos  jsonb,
  actualizado_en  timestamptz NOT NULL DEFAULT now()
);
