-- 0029 · Marca blanca, plantilla de recibo y configuración del edificio (bloques I1, I2 e I5).

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('marca.configurar',     'marca',         'Logo, colores y nombre comercial de la administradora', false),
 ('recibos.plantilla',    'recibos',       'Personalizar la plantilla del recibo', false),
 ('configuracion.ver',    'configuracion', 'Ver el estado, los asistentes y el registro de cambios', false),
 ('configuracion.editar', 'configuracion', 'Activar o desactivar el edificio', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','marca.configurar'), ('superadmin','recibos.plantilla'),
 ('superadmin','configuracion.ver'), ('superadmin','configuracion.editar'),
 ('administrador','marca.configurar'), ('administrador','recibos.plantilla'),
 ('administrador','configuracion.ver'), ('administrador','configuracion.editar'),
 ('junta','configuracion.ver');

-- I2 · marca blanca. branding = {nombre, lema, color_primario, color_fondo_login, logo_archivo_id}.
-- slug: identificador público para que el login muestre la marca (/login/?marca=slug) antes de la sesión.
ALTER TABLE administradora
    ADD COLUMN branding jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN slug     text CHECK (slug ~ '^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$');
CREATE UNIQUE INDEX administradora_slug_uq ON administradora (slug) WHERE slug IS NOT NULL;

-- I1 · una plantilla por edificio. config_json = {color, titulo, nota, mostrar_logo,
-- bloques: {contometro, fotos, qr, barras}}; lo que falta toma el valor por defecto en Go.
CREATE TABLE plantilla_recibo (
    id              bigserial PRIMARY KEY,
    edificio_id     bigint NOT NULL UNIQUE REFERENCES edificio(id) ON DELETE CASCADE,
    config_json     jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config_json) = 'object'),
    actualizado_por bigint REFERENCES usuario(id),
    actualizado_en  timestamptz NOT NULL DEFAULT now()
);

-- I5 · edificio activo. Desactivado: se puede consultar, pero no registrar nada (salvo reactivarlo).
ALTER TABLE edificio
    ADD COLUMN activo             boolean NOT NULL DEFAULT true,
    ADD COLUMN desactivado_en     timestamptz,
    ADD COLUMN desactivado_motivo text NOT NULL DEFAULT '';
ALTER TABLE edificio ADD CONSTRAINT edificio_desactivado_motivo_ck
    CHECK (activo OR length(btrim(desactivado_motivo)) > 0);

-- Registro de cambios: las filas con antes/después de auditoría, por edificio y fecha.
CREATE INDEX auditoria_cambios_idx ON auditoria (edificio_id, id DESC) WHERE antes IS NOT NULL OR despues IS NOT NULL;
