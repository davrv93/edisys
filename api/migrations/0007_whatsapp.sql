-- 0007 · WhatsApp: bandeja de salida (outbox), mensajes entrantes y configuración.
-- Modo por defecto: simulado (no envía nada, solo registra).

CREATE TABLE whatsapp_config (
    edificio_id  bigint PRIMARY KEY REFERENCES edificio(id) ON DELETE CASCADE,
    modo         text NOT NULL DEFAULT 'simulado' CHECK (modo IN ('simulado','evolution')),
    url          text NOT NULL DEFAULT '',
    instancia    text NOT NULL DEFAULT '',
    apikey       text NOT NULL DEFAULT '',          -- nunca se devuelve por el API
    actualizado_por bigint REFERENCES usuario(id),
    actualizado_en  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE whatsapp_mensaje (
    id            bigserial PRIMARY KEY,
    edificio_id   bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    direccion     text NOT NULL DEFAULT 'saliente' CHECK (direccion IN ('saliente','entrante')),
    unidad_id     bigint REFERENCES unidad(id) ON DELETE SET NULL,
    telefono      text NOT NULL,
    plantilla     text NOT NULL DEFAULT 'libre',
    variables     jsonb NOT NULL DEFAULT '{}',
    texto         text NOT NULL,
    estado        text NOT NULL DEFAULT 'pendiente'
                  CHECK (estado IN ('pendiente','simulado','enviado','error','recibido')),
    origen        text NOT NULL DEFAULT 'manual' CHECK (origen IN ('manual','recibo','chatbot','sistema','webhook')),
    intentos      int NOT NULL DEFAULT 0,
    error         text NOT NULL DEFAULT '',
    proveedor_id  text NOT NULL DEFAULT '',
    intencion     text NOT NULL DEFAULT '',
    enviado_por   bigint REFERENCES usuario(id),
    creado_en     timestamptz NOT NULL DEFAULT now(),
    procesado_en  timestamptz
);
CREATE INDEX whatsapp_mensaje_idx ON whatsapp_mensaje (edificio_id, creado_en DESC);
CREATE INDEX whatsapp_mensaje_pendiente_idx ON whatsapp_mensaje (estado) WHERE estado = 'pendiente';
