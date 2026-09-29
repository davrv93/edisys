-- 0013 · Conversación con estado del chatbot (reservar paso a paso). Una sesión por teléfono; caduca a los 15 min.
CREATE TABLE chatbot_sesion (
    telefono       text PRIMARY KEY,
    edificio_id    bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    paso           text NOT NULL CHECK (paso IN ('elegir_area','elegir_fecha','elegir_franja','confirmar')),
    datos          jsonb NOT NULL DEFAULT '{}',
    vence_en       timestamptz NOT NULL,
    actualizado_en timestamptz NOT NULL DEFAULT now()
);
