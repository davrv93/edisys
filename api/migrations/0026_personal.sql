-- 0026 · Personal y almacén (bloques F1, F2 y F3).
-- F1: colaboradores del edificio y sus documentos (CV, ficha, PLAME) con archivo privado.
-- F2: turnos, asistencia con foto (entrada/salida), checklist del turno y puntualidad.
-- F3: almacén sobre el producto de 0017_comercio: movimientos de entrada/salida/ajuste y stock mínimo.

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('personal.ver',         'personal', 'Ver colaboradores y sus documentos visibles', false),
 ('personal.administrar', 'personal', 'Registrar colaboradores, documentos, turnos y checklist', false),
 ('asistencia.marcar',    'personal', 'Marcar la propia entrada y salida con foto', false),
 ('asistencia.ver',       'personal', 'Ver la asistencia y el panel de puntualidad', false),
 ('almacen.ver',          'almacen',  'Ver el stock del almacén y sus movimientos', false),
 ('almacen.registrar',    'almacen',  'Registrar entradas y salidas del almacén', false),
 ('almacen.administrar',  'almacen',  'Ajustar inventario, artículos y stock mínimo', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','personal.ver'), ('superadmin','personal.administrar'), ('superadmin','asistencia.ver'),
 ('superadmin','almacen.ver'), ('superadmin','almacen.registrar'), ('superadmin','almacen.administrar'),
 ('administrador','personal.ver'), ('administrador','personal.administrar'), ('administrador','asistencia.ver'),
 ('administrador','almacen.ver'), ('administrador','almacen.registrar'), ('administrador','almacen.administrar'),
 ('junta','personal.ver'), ('junta','asistencia.ver'), ('junta','almacen.ver'),
 -- El propietario ve quién trabaja en su edificio (sin DNI completo ni contacto) y los documentos marcados como visibles.
 ('propietario','personal.ver'),
 -- El personal de planta marca su asistencia; el operario además saca y repone insumos.
 ('operario','asistencia.marcar'), ('operario','almacen.ver'), ('operario','almacen.registrar'),
 ('tecnico','asistencia.marcar');

-- ---------- F2 · turnos (antes que colaborador, que apunta a su turno) ----------

-- Horario de un puesto: hora de entrada/salida en hora de Lima, tolerancia y días (ISO: 1 = lunes … 7 = domingo).
CREATE TABLE turno (
    id             bigserial PRIMARY KEY,
    edificio_id    bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    nombre         text NOT NULL CHECK (length(btrim(nombre)) > 0),
    hora_entrada   time NOT NULL,
    hora_salida    time NOT NULL,
    tolerancia_min int NOT NULL DEFAULT 10 CHECK (tolerancia_min BETWEEN 0 AND 120),
    dias           smallint[] NOT NULL DEFAULT '{1,2,3,4,5,6}' CHECK (dias <@ '{1,2,3,4,5,6,7}'::smallint[] AND cardinality(dias) > 0),
    activo         boolean NOT NULL DEFAULT true,
    creado_en      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX turno_nombre_uq ON turno (edificio_id, lower(btrim(nombre)));

-- ---------- F1 · colaboradores ----------

CREATE TABLE colaborador (
    id             bigserial PRIMARY KEY,
    edificio_id    bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    nombre         text NOT NULL CHECK (length(btrim(nombre)) > 0),
    tipo_doc       text NOT NULL DEFAULT '1' CHECK (tipo_doc IN ('1','4','7')), -- DNI, carné de extranjería, pasaporte
    num_doc        text NOT NULL DEFAULT '',
    cargo          text NOT NULL DEFAULT '',
    telefono       text NOT NULL DEFAULT '',
    correo         text NOT NULL DEFAULT '',
    foto_id        bigint REFERENCES archivo(id),
    -- Cuenta con la que marca su asistencia (operario/técnico del edificio). Opcional.
    usuario_id     bigint REFERENCES usuario(id) ON DELETE SET NULL,
    turno_id       bigint REFERENCES turno(id) ON DELETE SET NULL,
    fecha_ingreso  date NOT NULL DEFAULT (now() AT TIME ZONE 'America/Lima')::date,
    activo         boolean NOT NULL DEFAULT true,
    creado_por     bigint REFERENCES usuario(id),
    creado_en      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX colaborador_doc_uq ON colaborador (edificio_id, tipo_doc, num_doc) WHERE num_doc <> '';
-- Una cuenta marca por un solo colaborador en cada edificio.
CREATE UNIQUE INDEX colaborador_usuario_uq ON colaborador (edificio_id, usuario_id) WHERE usuario_id IS NOT NULL;

CREATE TABLE colaborador_documento (
    id             bigserial PRIMARY KEY,
    edificio_id    bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    colaborador_id bigint NOT NULL REFERENCES colaborador(id) ON DELETE CASCADE,
    tipo           text NOT NULL CHECK (tipo IN ('cv','ficha','plame','contrato','otro')),
    titulo         text NOT NULL CHECK (length(btrim(titulo)) > 0),
    periodo        text NOT NULL DEFAULT '' CHECK (periodo = '' OR periodo ~ '^\d{4}-(0[1-9]|1[0-2])$'),
    archivo_id     bigint NOT NULL REFERENCES archivo(id),
    -- Lo visible lo abre el propietario con enlace firmado; lo demás solo la administración.
    visible_propietarios boolean NOT NULL DEFAULT true,
    creado_por     bigint REFERENCES usuario(id),
    creado_en      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX colaborador_documento_idx ON colaborador_documento (colaborador_id, creado_en DESC);

-- ---------- F2 · asistencia y checklist ----------

-- Una fila por colaborador y día (fecha de Lima). La puntualidad se calcula al marcar la entrada,
-- contra el turno vigente en ese momento, y se guarda: cambiar el turno después no reescribe la historia.
CREATE TABLE asistencia (
    id               bigserial PRIMARY KEY,
    edificio_id      bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    colaborador_id   bigint NOT NULL REFERENCES colaborador(id) ON DELETE CASCADE,
    fecha            date NOT NULL,
    turno_id         bigint REFERENCES turno(id) ON DELETE SET NULL,
    entrada_en       timestamptz NOT NULL,
    entrada_foto_id  bigint REFERENCES archivo(id),
    salida_en        timestamptz,
    salida_foto_id   bigint REFERENCES archivo(id),
    minutos_tarde    int CHECK (minutos_tarde IS NULL OR minutos_tarde >= 0),
    puntual          boolean, -- null: sin turno asignado
    manual           boolean NOT NULL DEFAULT false, -- registrada por la administración, no por el colaborador
    nota             text NOT NULL DEFAULT '',
    registrado_por   bigint REFERENCES usuario(id),
    creado_en        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (colaborador_id, fecha),
    CHECK (salida_en IS NULL OR salida_en > entrada_en),
    -- Marcada por el colaborador: la foto es obligatoria (es la prueba de que estuvo).
    CHECK (manual OR entrada_foto_id IS NOT NULL),
    CHECK (manual OR salida_en IS NULL OR salida_foto_id IS NOT NULL)
);
CREATE INDEX asistencia_fecha_idx ON asistencia (edificio_id, fecha);

-- Tareas del turno (abrir portón, revisar bombas…). turno_id null = para todos los turnos.
CREATE TABLE checklist_item (
    id          bigserial PRIMARY KEY,
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    turno_id    bigint REFERENCES turno(id) ON DELETE CASCADE,
    texto       text NOT NULL CHECK (length(btrim(texto)) > 0),
    orden       int NOT NULL DEFAULT 0,
    activo      boolean NOT NULL DEFAULT true,
    creado_en   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX checklist_item_idx ON checklist_item (edificio_id, activo, orden);

CREATE TABLE asistencia_checklist (
    asistencia_id bigint NOT NULL REFERENCES asistencia(id) ON DELETE CASCADE,
    item_id       bigint NOT NULL REFERENCES checklist_item(id) ON DELETE CASCADE,
    hecho_por     bigint REFERENCES usuario(id),
    hecho_en      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (asistencia_id, item_id)
);

-- ---------- F3 · almacén ----------

-- El stock mínimo dispara la alerta; 0 = sin alerta.
ALTER TABLE producto ADD COLUMN stock_minimo numeric(12,2) NOT NULL DEFAULT 0 CHECK (stock_minimo >= 0);
-- Regla dura: ningún movimiento deja el stock en negativo (el API lo avisa antes; esto es la red).
ALTER TABLE producto ADD CONSTRAINT producto_stock_no_negativo CHECK (stock >= 0);

-- Kárdex: cada fila guarda el cambio (delta con signo) y el saldo que dejó.
-- Invariante: saldo = saldo anterior + delta, y producto.stock = saldo del último movimiento.
CREATE TABLE almacen_movimiento (
    id              bigserial PRIMARY KEY,
    edificio_id     bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    producto_id     bigint NOT NULL REFERENCES producto(id) ON DELETE CASCADE,
    tipo            text NOT NULL CHECK (tipo IN ('entrada','salida','ajuste')),
    delta           numeric(12,2) NOT NULL CHECK (delta <> 0),
    saldo           numeric(12,2) NOT NULL CHECK (saldo >= 0),
    costo_unit_cts  bigint NOT NULL DEFAULT 0 CHECK (costo_unit_cts >= 0),
    motivo          text NOT NULL DEFAULT '',
    creado_por      bigint REFERENCES usuario(id),
    creado_en       timestamptz NOT NULL DEFAULT now(),
    CHECK ((tipo = 'entrada' AND delta > 0) OR (tipo = 'salida' AND delta < 0) OR tipo = 'ajuste')
);
CREATE INDEX almacen_movimiento_idx ON almacen_movimiento (edificio_id, producto_id, id DESC);
