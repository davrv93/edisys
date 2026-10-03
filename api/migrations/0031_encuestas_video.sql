-- 0031 · Encuestas (J1), videollamadas junta ↔ administración (J2) y dominio propio por administradora (I3).
-- Las reglas duras viven aquí: una respuesta por usuario y encuesta, la opción elegida pertenece
-- a su pregunta y la pregunta a su encuesta (claves compuestas), un dominio pertenece a una sola administradora.

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('encuestas.ver',              'encuestas',     'Ver las encuestas del edificio', false),
 ('encuestas.responder',        'encuestas',     'Responder encuestas abiertas', false),
 ('encuestas.administrar',      'encuestas',     'Crear, abrir y cerrar encuestas', false),
 ('encuestas.resultados',       'encuestas',     'Ver resultados antes del cierre', false),
 ('videollamadas.ver',          'videollamadas', 'Ver y entrar a las salas de la junta', false),
 ('videollamadas.administrar',  'videollamadas', 'Convocar, cancelar y registrar grabaciones', false),
 ('dominios.administrar',       'dominios',      'Registrar el dominio propio de la administradora', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','encuestas.ver'), ('superadmin','encuestas.administrar'), ('superadmin','encuestas.resultados'),
 ('administrador','encuestas.ver'), ('administrador','encuestas.administrar'), ('administrador','encuestas.resultados'),
 ('junta','encuestas.ver'), ('junta','encuestas.responder'), ('junta','encuestas.resultados'),
 ('propietario','encuestas.ver'), ('propietario','encuestas.responder'),
 ('inquilino','encuestas.ver'), ('inquilino','encuestas.responder'),
 ('superadmin','videollamadas.ver'), ('superadmin','videollamadas.administrar'),
 ('administrador','videollamadas.ver'), ('administrador','videollamadas.administrar'),
 ('junta','videollamadas.ver'), ('junta','videollamadas.administrar'),
 ('superadmin','dominios.administrar'), ('administrador','dominios.administrar');

-- ---------- J1 · encuestas ----------

CREATE TABLE encuesta (
    id           bigserial PRIMARY KEY,
    edificio_id  bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    titulo       text NOT NULL CHECK (length(btrim(titulo)) > 0),
    descripcion  text NOT NULL DEFAULT '',
    estado       text NOT NULL DEFAULT 'borrador' CHECK (estado IN ('borrador','abierta','cerrada')),
    anonima      boolean NOT NULL DEFAULT true,
    cierra_en    timestamptz,
    abierta_en   timestamptz,
    cerrada_en   timestamptz,
    creado_por   bigint REFERENCES usuario(id),
    creado_en    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX encuesta_edificio_idx ON encuesta (edificio_id, estado);

CREATE TABLE encuesta_pregunta (
    id           bigserial PRIMARY KEY,
    encuesta_id  bigint NOT NULL REFERENCES encuesta(id) ON DELETE CASCADE,
    orden        int NOT NULL DEFAULT 0,
    texto        text NOT NULL CHECK (length(btrim(texto)) > 0),
    tipo         text NOT NULL CHECK (tipo IN ('unica','multiple','texto')),
    obligatoria  boolean NOT NULL DEFAULT true,
    UNIQUE (id, encuesta_id)
);

CREATE TABLE encuesta_opcion (
    id           bigserial PRIMARY KEY,
    pregunta_id  bigint NOT NULL REFERENCES encuesta_pregunta(id) ON DELETE CASCADE,
    orden        int NOT NULL DEFAULT 0,
    texto        text NOT NULL CHECK (length(btrim(texto)) > 0),
    UNIQUE (id, pregunta_id)
);

-- Una sola respuesta por usuario: el usuario se guarda siempre (para impedir el doble voto)
-- aunque la encuesta sea anónima; el anonimato es no mostrarlo en los resultados.
CREATE TABLE encuesta_respuesta (
    id           bigserial PRIMARY KEY,
    encuesta_id  bigint NOT NULL REFERENCES encuesta(id) ON DELETE CASCADE,
    usuario_id   bigint NOT NULL REFERENCES usuario(id),
    unidad_id    bigint REFERENCES unidad(id),
    creado_en    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT encuesta_respuesta_unica UNIQUE (encuesta_id, usuario_id),
    UNIQUE (id, encuesta_id)
);

CREATE TABLE encuesta_respuesta_item (
    id           bigserial PRIMARY KEY,
    respuesta_id bigint NOT NULL,
    encuesta_id  bigint NOT NULL,
    pregunta_id  bigint NOT NULL,
    opcion_id    bigint,
    texto        text NOT NULL DEFAULT '',
    FOREIGN KEY (respuesta_id, encuesta_id) REFERENCES encuesta_respuesta(id, encuesta_id) ON DELETE CASCADE,
    FOREIGN KEY (pregunta_id, encuesta_id) REFERENCES encuesta_pregunta(id, encuesta_id) ON DELETE CASCADE,
    FOREIGN KEY (opcion_id, pregunta_id) REFERENCES encuesta_opcion(id, pregunta_id) ON DELETE CASCADE,
    CHECK (opcion_id IS NOT NULL OR length(btrim(texto)) > 0)
);
CREATE INDEX encuesta_respuesta_item_idx ON encuesta_respuesta_item (encuesta_id, pregunta_id);
CREATE UNIQUE INDEX encuesta_respuesta_item_opcion_uq ON encuesta_respuesta_item (respuesta_id, opcion_id) WHERE opcion_id IS NOT NULL;

-- ---------- J2 · videollamadas ----------

-- El enlace NO se guarda: se arma con JITSI_BASE_URL + codigo, así cambiar de servidor Jitsi
-- (meet.jit.si → uno propio) no deja enlaces viejos en la base.
CREATE TABLE videollamada (
    id             bigserial PRIMARY KEY,
    edificio_id    bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    titulo         text NOT NULL CHECK (length(btrim(titulo)) > 0),
    descripcion    text NOT NULL DEFAULT '',
    inicia_en      timestamptz NOT NULL,
    duracion_min   int NOT NULL DEFAULT 60 CHECK (duracion_min BETWEEN 15 AND 480),
    codigo         text NOT NULL UNIQUE CHECK (codigo ~ '^[a-z0-9-]{16,64}$'),
    cancelada      boolean NOT NULL DEFAULT false,
    grabacion_url  text NOT NULL DEFAULT '' CHECK (grabacion_url = '' OR grabacion_url ~ '^https://'),
    creado_por     bigint REFERENCES usuario(id),
    creado_en      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX videollamada_edificio_idx ON videollamada (edificio_id, inicia_en DESC);

-- ---------- I3 · dominio propio ----------

-- Host sin puerto, en minúsculas. Un dominio apunta a una sola administradora (UNIQUE global).
CREATE TABLE administradora_dominio (
    id                 bigserial PRIMARY KEY,
    administradora_id  bigint NOT NULL REFERENCES administradora(id) ON DELETE CASCADE,
    host               text NOT NULL CHECK (host = lower(host) AND host ~ '^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$'),
    activo             boolean NOT NULL DEFAULT true,
    creado_por         bigint REFERENCES usuario(id),
    creado_en          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT administradora_dominio_host_key UNIQUE (host)
);
