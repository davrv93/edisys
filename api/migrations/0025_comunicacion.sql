-- 0025 · Comunicación (bloques E2, E3, E4 y E5).
-- E2 · anuncios con canales y evidencia de envío (cada mensaje queda enlazado a su bandeja).
-- E3 · la bandeja de correos ya existe (0010); aquí no cambia nada de su tabla.
-- E4 · Telegram: destinos del edificio y su propia bandeja de salida (outbox), igual que WhatsApp.
-- E5 · preguntas frecuentes, academia y beneficios por edificio, visibles en el portal.

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('anuncios.ver',          'anuncios',  'Ver los anuncios publicados', false),
 ('anuncios.administrar',  'anuncios',  'Redactar, publicar y enviar anuncios', false),
 ('telegram.configurar',   'telegram',  'Dar de alta el bot, los destinos y ver la bandeja de Telegram', false),
 ('contenido.ver',         'contenido', 'Ver preguntas frecuentes, academia y beneficios', false),
 ('contenido.administrar', 'contenido', 'Editar preguntas frecuentes, academia y beneficios', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','anuncios.ver'), ('superadmin','anuncios.administrar'), ('superadmin','telegram.configurar'),
 ('superadmin','contenido.ver'), ('superadmin','contenido.administrar'),
 ('administrador','anuncios.ver'), ('administrador','anuncios.administrar'), ('administrador','telegram.configurar'),
 ('administrador','contenido.ver'), ('administrador','contenido.administrar'),
 ('junta','anuncios.ver'), ('junta','contenido.ver'),
 ('propietario','anuncios.ver'), ('propietario','contenido.ver'),
 ('inquilino','anuncios.ver'), ('inquilino','contenido.ver'),
 ('operario','anuncios.ver'), ('tecnico','anuncios.ver');

-- ---------- E4 · Telegram ----------

-- A quién le escribe el bot: un grupo del edificio o un vecino que habló con el bot.
CREATE TABLE telegram_destino (
    id          bigserial PRIMARY KEY,
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    nombre      text NOT NULL CHECK (length(btrim(nombre)) > 0),
    chat_id     text NOT NULL CHECK (chat_id ~ '^(-?[0-9]+|@[A-Za-z0-9_]{5,})$'),
    tipo        text NOT NULL DEFAULT 'grupo' CHECK (tipo IN ('grupo','usuario')),
    unidad_id   bigint REFERENCES unidad(id) ON DELETE SET NULL,
    activo      boolean NOT NULL DEFAULT true,
    creado_en   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX telegram_destino_uq ON telegram_destino (edificio_id, chat_id);

CREATE TABLE telegram_mensaje (
    id            bigserial PRIMARY KEY,
    edificio_id   bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    destino_id    bigint REFERENCES telegram_destino(id) ON DELETE SET NULL,
    chat_id       text NOT NULL,
    texto         text NOT NULL CHECK (length(texto) BETWEEN 1 AND 4096),
    estado        text NOT NULL DEFAULT 'pendiente' CHECK (estado IN ('pendiente','simulado','enviado','error')),
    origen        text NOT NULL DEFAULT 'manual' CHECK (origen IN ('manual','anuncio','sistema')),
    referencia    text NOT NULL DEFAULT '',
    intentos      int NOT NULL DEFAULT 0,
    error         text NOT NULL DEFAULT '',
    proveedor_id  text NOT NULL DEFAULT '',
    enviado_por   bigint REFERENCES usuario(id),
    creado_en     timestamptz NOT NULL DEFAULT now(),
    procesado_en  timestamptz
);
CREATE INDEX telegram_mensaje_idx ON telegram_mensaje (edificio_id, creado_en DESC);
CREATE INDEX telegram_mensaje_pendiente_idx ON telegram_mensaje (estado) WHERE estado = 'pendiente';

-- ---------- E2 · anuncios ----------

CREATE TABLE anuncio (
    id           bigserial PRIMARY KEY,
    edificio_id  bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    titulo       text NOT NULL CHECK (length(btrim(titulo)) > 0),
    cuerpo       text NOT NULL CHECK (length(btrim(cuerpo)) > 0),
    fecha        date NOT NULL DEFAULT (now() AT TIME ZONE 'America/Lima')::date,
    estado       text NOT NULL DEFAULT 'borrador' CHECK (estado IN ('borrador','publicado','archivado')),
    -- «portal» siempre: el anuncio se ve en el portal; los demás canales además lo envían.
    canales      text[] NOT NULL DEFAULT '{portal}'
                 CHECK (canales <@ ARRAY['portal','correo','whatsapp','telegram']::text[] AND 'portal' = ANY(canales)),
    archivo_id   bigint REFERENCES archivo(id),
    creado_por   bigint REFERENCES usuario(id),
    creado_en    timestamptz NOT NULL DEFAULT now(),
    publicado_en timestamptz,
    enviado_en   timestamptz,
    -- Un anuncio publicado (o archivado después) siempre sabe cuándo se publicó.
    CHECK (estado = 'borrador' OR publicado_en IS NOT NULL),
    CHECK (enviado_en IS NULL OR publicado_en IS NOT NULL)
);
CREATE INDEX anuncio_idx ON anuncio (edificio_id, estado, fecha DESC);

-- Evidencia de envío: una fila por destinatario y canal, enlazada al mensaje de su bandeja.
-- El estado no se copia: se lee en vivo del mensaje (pendiente → enviado / error / simulado).
CREATE TABLE anuncio_envio (
    id          bigserial PRIMARY KEY,
    anuncio_id  bigint NOT NULL REFERENCES anuncio(id) ON DELETE CASCADE,
    canal       text NOT NULL CHECK (canal IN ('correo','whatsapp','telegram')),
    destino     text NOT NULL,
    unidad_id   bigint REFERENCES unidad(id) ON DELETE SET NULL,
    correo_id   bigint REFERENCES correo_mensaje(id) ON DELETE SET NULL,
    whatsapp_id bigint REFERENCES whatsapp_mensaje(id) ON DELETE SET NULL,
    telegram_id bigint REFERENCES telegram_mensaje(id) ON DELETE SET NULL,
    creado_en   timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(correo_id, whatsapp_id, telegram_id) <= 1)
);
CREATE INDEX anuncio_envio_idx ON anuncio_envio (anuncio_id, canal);
-- Nadie recibe dos veces el mismo anuncio por el mismo canal.
CREATE UNIQUE INDEX anuncio_envio_uq ON anuncio_envio (anuncio_id, canal, destino);

-- ---------- E5 · preguntas frecuentes, academia y beneficios ----------

CREATE TABLE faq (
    id             bigserial PRIMARY KEY,
    edificio_id    bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    pregunta       text NOT NULL CHECK (length(btrim(pregunta)) > 0),
    respuesta      text NOT NULL CHECK (length(btrim(respuesta)) > 0),
    categoria      text NOT NULL DEFAULT 'General',
    orden          int NOT NULL DEFAULT 0,
    publicado      boolean NOT NULL DEFAULT true,
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX faq_idx ON faq (edificio_id, publicado, orden);

CREATE TABLE academia (
    id             bigserial PRIMARY KEY,
    edificio_id    bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    titulo         text NOT NULL CHECK (length(btrim(titulo)) > 0),
    resumen        text NOT NULL DEFAULT '',
    tipo           text NOT NULL DEFAULT 'articulo' CHECK (tipo IN ('articulo','video','guia')),
    url            text NOT NULL DEFAULT '' CHECK (url = '' OR url ~* '^https?://'),
    contenido      text NOT NULL DEFAULT '',
    orden          int NOT NULL DEFAULT 0,
    publicado      boolean NOT NULL DEFAULT true,
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    -- Una lección sin enlace ni texto no enseña nada.
    CHECK (url <> '' OR length(btrim(contenido)) > 0)
);
CREATE INDEX academia_idx ON academia (edificio_id, publicado, orden);

CREATE TABLE beneficio (
    id             bigserial PRIMARY KEY,
    edificio_id    bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    titulo         text NOT NULL CHECK (length(btrim(titulo)) > 0),
    descripcion    text NOT NULL DEFAULT '',
    proveedor      text NOT NULL DEFAULT '',
    descuento      text NOT NULL DEFAULT '',   -- texto libre: «15 %», «2x1», «delivery gratis»
    codigo         text NOT NULL DEFAULT '',
    url            text NOT NULL DEFAULT '' CHECK (url = '' OR url ~* '^https?://'),
    vigente_hasta  date,
    orden          int NOT NULL DEFAULT 0,
    publicado      boolean NOT NULL DEFAULT true,
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX beneficio_idx ON beneficio (edificio_id, publicado, orden);
