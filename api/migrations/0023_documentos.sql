-- 0023 · Documentos por categorías (bloque E1).
-- Actas, reglamentos, libro de reclamaciones y demás documentos que el edificio publica.

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('documentos.ver',         'documentos', 'Ver los documentos publicados', false),
 ('documentos.administrar', 'documentos', 'Publicar y organizar documentos', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','documentos.ver'), ('superadmin','documentos.administrar'),
 ('administrador','documentos.ver'), ('administrador','documentos.administrar'),
 ('junta','documentos.ver'), ('propietario','documentos.ver'), ('inquilino','documentos.ver');

CREATE TABLE documento_categoria (
    id          bigserial PRIMARY KEY,
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    nombre      text NOT NULL CHECK (length(btrim(nombre)) > 0),
    orden       int NOT NULL DEFAULT 0,
    activo      boolean NOT NULL DEFAULT true,
    creado_en   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX documento_categoria_uq ON documento_categoria (edificio_id, lower(btrim(nombre)));

CREATE TABLE documento_publicado (
    id           bigserial PRIMARY KEY,
    edificio_id  bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    categoria_id bigint NOT NULL REFERENCES documento_categoria(id) ON DELETE CASCADE,
    titulo       text NOT NULL CHECK (length(btrim(titulo)) > 0),
    numero       text NOT NULL DEFAULT '',
    resumen      text NOT NULL DEFAULT '',
    archivo_id   bigint NOT NULL REFERENCES archivo(id),
    publicado    boolean NOT NULL DEFAULT true,
    creado_por   bigint REFERENCES usuario(id),
    creado_en    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX documento_publicado_idx ON documento_publicado (edificio_id, categoria_id, publicado);
