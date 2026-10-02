-- 0017 · Comercio (punto de venta): catálogo de productos y servicios, clientes.
-- Bloque 1 del módulo: lo que el edificio vende (alquileres, servicios, espacios, extras).
-- La caja y las ventas van en 0018; aquí solo lo que el punto de venta necesita para cobrar.

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
  ('productos.ver',      'comercio', 'Ver el catálogo de productos y servicios', false),
  ('productos.registrar','comercio', 'Crear y editar productos y servicios', false),
  ('clientes.ver',       'comercio', 'Ver clientes', false),
  ('clientes.registrar', 'comercio', 'Registrar y editar clientes', false);
INSERT INTO rol_permiso (rol, permiso) VALUES
  ('superadmin', 'productos.ver'), ('superadmin', 'productos.registrar'),
  ('superadmin', 'clientes.ver'),  ('superadmin', 'clientes.registrar'),
  ('administrador', 'productos.ver'), ('administrador', 'productos.registrar'),
  ('administrador', 'clientes.ver'),  ('administrador', 'clientes.registrar'),
  -- El operario atiende el mostrador de la casa: consulta el catálogo y registra clientes.
  ('operario', 'productos.ver'), ('operario', 'clientes.ver'), ('operario', 'clientes.registrar');

CREATE TABLE producto_categoria (
    id          bigserial PRIMARY KEY,
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    nombre      text NOT NULL CHECK (length(btrim(nombre)) > 0),
    orden       int NOT NULL DEFAULT 0,
    activo      boolean NOT NULL DEFAULT true
);
-- UNIQUE no admite expresiones: va con índice (como producto_nombre_uq).
CREATE UNIQUE INDEX producto_categoria_nombre_uq ON producto_categoria (edificio_id, lower(btrim(nombre)));

-- Producto = lo que se vende. En un edificio casi todo es un servicio (alquiler, limpieza,
-- sala de reuniones), por eso la unidad de venta es parte del dato y el stock es opcional:
-- solo lo controlan los productos físicos (llaves, placas de parqueo).
CREATE TABLE producto (
    id              bigserial PRIMARY KEY,
    edificio_id     bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    categoria_id    bigint REFERENCES producto_categoria(id) ON DELETE SET NULL,
    codigo          text NOT NULL DEFAULT '',
    nombre          text NOT NULL CHECK (length(btrim(nombre)) > 0),
    descripcion     text NOT NULL DEFAULT '',
    unidad          text NOT NULL DEFAULT 'unidad' CHECK (unidad IN ('unidad','mes','dia','hora','kg','m2')),
    precio_cts      bigint NOT NULL CHECK (precio_cts >= 0),
    costo_cts       bigint NOT NULL DEFAULT 0 CHECK (costo_cts >= 0),
    afectacion      text NOT NULL DEFAULT 'inafecto' CHECK (afectacion IN ('gravado','exonerado','inafecto')),
    controla_stock  boolean NOT NULL DEFAULT false,
    stock           numeric(12,2) NOT NULL DEFAULT 0,
    activo          boolean NOT NULL DEFAULT true,
    creado_por      bigint REFERENCES usuario(id),
    creado_en       timestamptz NOT NULL DEFAULT now(),
    actualizado_en  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX producto_nombre_uq ON producto (edificio_id, lower(btrim(nombre)));
CREATE UNIQUE INDEX producto_codigo_uq ON producto (edificio_id, upper(codigo)) WHERE codigo <> '';
CREATE INDEX producto_categoria_ix ON producto (categoria_id) WHERE categoria_id IS NOT NULL;

CREATE TABLE cliente (
    id               bigserial PRIMARY KEY,
    edificio_id      bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    nombre           text NOT NULL CHECK (length(btrim(nombre)) > 0),
    tipo_doc         text NOT NULL DEFAULT '' CHECK (tipo_doc IN ('','0','1','4','6','7')),
    num_doc          text NOT NULL DEFAULT '',
    direccion        text NOT NULL DEFAULT '',
    telefono         text NOT NULL DEFAULT '',
    correo           text NOT NULL DEFAULT '',
    unidad_id        bigint REFERENCES unidad(id) ON DELETE SET NULL,
    activo           boolean NOT NULL DEFAULT true,
    creado_por       bigint REFERENCES usuario(id),
    creado_en        timestamptz NOT NULL DEFAULT now(),
    actualizado_en   timestamptz NOT NULL DEFAULT now()
);
-- El documento identifica al cliente del comprobante: no puede repetirse en el edificio.
CREATE UNIQUE INDEX cliente_doc_uq ON cliente (edificio_id, tipo_doc, num_doc) WHERE num_doc <> '';
CREATE INDEX cliente_nombre_ix ON cliente (edificio_id, lower(nombre));
