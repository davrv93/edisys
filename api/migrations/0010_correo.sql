-- 0010 · Correo: bandeja de salida (outbox) igual que WhatsApp. CORREO_MODO=simulado no envía nada;
-- CORREO_MODO=smtp entrega por SMTP (en local, Mailpit). Los adjuntos (PDF) van en su propia tabla.

CREATE TABLE correo_mensaje (
    id            bigserial PRIMARY KEY,
    edificio_id   bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    unidad_id     bigint REFERENCES unidad(id) ON DELETE SET NULL,
    para          text NOT NULL,
    nombre        text NOT NULL DEFAULT '',
    asunto        text NOT NULL,
    html          text NOT NULL DEFAULT '',
    texto         text NOT NULL DEFAULT '',
    estado        text NOT NULL DEFAULT 'pendiente' CHECK (estado IN ('pendiente','simulado','enviado','error')),
    origen        text NOT NULL DEFAULT 'manual' CHECK (origen IN ('manual','recibo','balance','sistema')),
    referencia    text NOT NULL DEFAULT '',
    intentos      int NOT NULL DEFAULT 0,
    error         text NOT NULL DEFAULT '',
    enviado_por   bigint REFERENCES usuario(id),
    creado_en     timestamptz NOT NULL DEFAULT now(),
    procesado_en  timestamptz
);
CREATE INDEX correo_mensaje_idx ON correo_mensaje (edificio_id, creado_en DESC);
CREATE INDEX correo_mensaje_pendiente_idx ON correo_mensaje (estado) WHERE estado = 'pendiente';

CREATE TABLE correo_adjunto (
    id          bigserial PRIMARY KEY,
    mensaje_id  bigint NOT NULL REFERENCES correo_mensaje(id) ON DELETE CASCADE,
    nombre      text NOT NULL,
    tipo_mime   text NOT NULL DEFAULT 'application/pdf',
    datos       bytea NOT NULL
);
CREATE INDEX correo_adjunto_mensaje_idx ON correo_adjunto (mensaje_id);
