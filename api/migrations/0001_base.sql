-- 0001 · Base: administradoras, edificios, usuarios, roles, permisos, sesiones y auditoría.
-- Dinero siempre en céntimos (bigint). Fechas en timestamptz (UTC).

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE administradora (
    id          bigserial PRIMARY KEY,
    nombre      text NOT NULL,
    ruc         text,
    creado_en   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE edificio (
    id                     bigserial PRIMARY KEY,
    administradora_id      bigint NOT NULL REFERENCES administradora(id),
    nombre                 text NOT NULL,
    direccion              text NOT NULL DEFAULT '',
    distrito               text NOT NULL DEFAULT '',
    dia_corte              int  NOT NULL DEFAULT 1 CHECK (dia_corte BETWEEN 1 AND 31),
    dias_vencimiento       int  NOT NULL DEFAULT 10 CHECK (dias_vencimiento >= 0),
    dias_gracia            int  NOT NULL DEFAULT 15 CHECK (dias_gracia >= 0),
    politica_reparto       text NOT NULL DEFAULT 'participacion'
                           CHECK (politica_reparto IN ('participacion','partes_iguales','consumo','mixto')),
    modo_cobro_reservas    text NOT NULL DEFAULT 'cargo_recibo'
                           CHECK (modo_cobro_reservas IN ('cargo_recibo','pago_inmediato')),
    cobra_agua             boolean NOT NULL DEFAULT true,
    umbral_aprobacion_cts  bigint NOT NULL DEFAULT 100000,
    modo_aprobacion        text NOT NULL DEFAULT 'mayoria' CHECK (modo_aprobacion IN ('presidente','mayoria')),
    saldo_inicial_cts      bigint NOT NULL DEFAULT 0,
    yape_numero            text NOT NULL DEFAULT '',
    normas_texto           text NOT NULL DEFAULT '',
    manual_archivo_id      bigint,
    reglamento_archivo_id  bigint,
    ver_permisos           int NOT NULL DEFAULT 1,
    participacion_estricta boolean NOT NULL DEFAULT true,
    creado_en              timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE usuario (
    id                bigserial PRIMARY KEY,
    administradora_id bigint REFERENCES administradora(id),
    correo            text UNIQUE,
    nombre            text NOT NULL,
    telefono          text NOT NULL DEFAULT '',
    clave_hash        text,
    activo            boolean NOT NULL DEFAULT true,
    es_superadmin     boolean NOT NULL DEFAULT false,
    ultimo_ingreso    timestamptz,
    creado_en         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rol (
    codigo       text PRIMARY KEY,
    nombre       text NOT NULL,
    descripcion  text NOT NULL DEFAULT '',
    orden        int NOT NULL DEFAULT 0
);

CREATE TABLE permiso (
    codigo      text PRIMARY KEY,
    modulo      text NOT NULL,
    descripcion text NOT NULL,
    ajustable   boolean NOT NULL DEFAULT false
);

CREATE TABLE rol_permiso (
    rol     text NOT NULL REFERENCES rol(codigo),
    permiso text NOT NULL REFERENCES permiso(codigo),
    PRIMARY KEY (rol, permiso)
);

-- Ajustes por edificio de los permisos «ajustables» (matriz de 11).
CREATE TABLE rol_permiso_edificio (
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    rol         text NOT NULL REFERENCES rol(codigo),
    permiso     text NOT NULL REFERENCES permiso(codigo),
    habilitado  boolean NOT NULL,
    PRIMARY KEY (edificio_id, rol, permiso)
);

CREATE TABLE usuario_edificio_rol (
    usuario_id  bigint NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    rol         text NOT NULL REFERENCES rol(codigo),
    creado_en   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (usuario_id, edificio_id)
);

CREATE TABLE sesion_refresh (
    id          bigserial PRIMARY KEY,
    usuario_id  bigint NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    token_hash  text NOT NULL UNIQUE,
    vence_en    timestamptz NOT NULL,
    revocado_en timestamptz,
    creado_en   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE invitacion (
    id          bigserial PRIMARY KEY,
    usuario_id  bigint NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    token_hash  text NOT NULL UNIQUE,
    vence_en    timestamptz NOT NULL,
    usada_en    timestamptz,
    creado_en   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE auditoria (
    id          bigserial PRIMARY KEY,
    edificio_id bigint,
    usuario_id  bigint,
    modulo      text NOT NULL,
    accion      text NOT NULL,
    entidad     text NOT NULL DEFAULT '',
    entidad_id  text NOT NULL DEFAULT '',
    antes       jsonb,
    despues     jsonb,
    ip          text NOT NULL DEFAULT '',
    creado_en   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auditoria_edificio_idx ON auditoria (edificio_id, creado_en DESC);

-- Archivos en S3 (fotos, vouchers, PDF). La clave apunta al objeto en el cubo privado.
CREATE TABLE archivo (
    id           bigserial PRIMARY KEY,
    edificio_id  bigint REFERENCES edificio(id) ON DELETE CASCADE,
    clave        text NOT NULL UNIQUE,
    nombre       text NOT NULL,
    tipo_mime    text NOT NULL,
    tamano       bigint NOT NULL DEFAULT 0,
    subido_por   bigint REFERENCES usuario(id),
    creado_en    timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE edificio ADD CONSTRAINT edificio_manual_fk FOREIGN KEY (manual_archivo_id) REFERENCES archivo(id);
ALTER TABLE edificio ADD CONSTRAINT edificio_reglamento_fk FOREIGN KEY (reglamento_archivo_id) REFERENCES archivo(id);

-- Contactos de la landing (02).
CREATE TABLE contacto (
    id               bigserial PRIMARY KEY,
    nombre           text NOT NULL,
    correo           text NOT NULL,
    telefono         text NOT NULL DEFAULT '',
    empresa          text NOT NULL DEFAULT '',
    edificios_aprox  int,
    mensaje          text NOT NULL DEFAULT '',
    ip               text NOT NULL DEFAULT '',
    creado_en        timestamptz NOT NULL DEFAULT now()
);

-- Roles (7) y permisos por defecto.
INSERT INTO rol (codigo, nombre, descripcion, orden) VALUES
 ('superadmin',   'Superadmin',     'Dueño de la plataforma: todas las administradoras', 1),
 ('administrador','Administrador',  'Administra el edificio: cobra, registra, valida', 2),
 ('junta',        'Junta',          'Junta de propietarios: ve todo y aprueba trabajos', 3),
 ('propietario',  'Propietario',    'Ve y paga lo suyo, reserva y reporta', 4),
 ('inquilino',    'Inquilino',      'Reserva y reporta si el propietario lo habilita', 5),
 ('operario',     'Operario',       'Conserje, limpieza o nocturno: lecturas y reportes', 6),
 ('tecnico',      'Técnico',        'Técnico o proveedor externo: sus trabajos asignados', 7);

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('dashboard.ver',            'dashboard',     'Ver el dashboard del edificio', false),
 ('balance.ver',              'balance',       'Ver el balance por nodos', true),
 ('balance.ver_documentos',   'balance',       'Ver documentos de egresos del balance', true),
 ('egresos.registrar',        'balance',       'Registrar egresos', false),
 ('edificio.ver',             'edificio',      'Ver la ficha del edificio', false),
 ('edificio.editar',          'edificio',      'Editar la ficha del edificio', false),
 ('unidades.ver',             'unidades',      'Ver unidades y personas', false),
 ('unidades.editar',          'unidades',      'Crear y editar unidades', false),
 ('unidades.importar',        'unidades',      'Importar el padrón desde Excel', false),
 ('periodos.administrar',     'recibos',       'Abrir periodos y presupuesto', false),
 ('recibos.ver',              'recibos',       'Ver recibos (propietario: los suyos)', true),
 ('recibos.emitir',           'recibos',       'Generar, emitir y anular recibos', false),
 ('pagos.registrar',          'recibos',       'Registrar pagos', false),
 ('pagos.informar',           'recibos',       'Informar un pago con voucher', false),
 ('pagos.validar',            'recibos',       'Validar o rechazar pagos', false),
 ('morosidad.ver',            'recibos',       'Ver la morosidad del edificio', false),
 ('areas.administrar',        'reservas',      'Configurar áreas y recursos', false),
 ('reservas.ver',             'reservas',      'Ver reservas y disponibilidad', false),
 ('reservas.crear',           'reservas',      'Crear reservas', true),
 ('reservas.administrar',     'reservas',      'Confirmar, cancelar y forzar reservas', false),
 ('lecturas.ver',             'medidores',     'Ver lecturas de medidores', false),
 ('lecturas.registrar',       'medidores',     'Registrar lecturas con foto', false),
 ('lecturas.corregir',        'medidores',     'Corregir lecturas con motivo', false),
 ('lecturas.aprobar_reparto', 'medidores',     'Registrar recibo general y aprobar el reparto', false),
 ('incidencias.reportar',     'mantenimiento', 'Reportar incidencias', true),
 ('incidencias.ver',          'mantenimiento', 'Ver incidencias y trabajos', false),
 ('incidencias.validar',      'mantenimiento', 'Validar o descartar incidencias', false),
 ('trabajos.presupuestar',    'mantenimiento', 'Publicar informe y presupuesto', false),
 ('trabajos.aprobar',         'mantenimiento', 'Aprobar trabajos (dentro del umbral o como junta)', false),
 ('trabajos.votar',           'mantenimiento', 'Votar trabajos como miembro de la junta', false),
 ('trabajos.ejecutar',        'mantenimiento', 'Registrar avance y cierre de trabajos', false),
 ('portal.ver',               'portal',        'Ver el portal del propietario', false),
 ('usuarios.ver',             'roles',         'Ver usuarios del edificio', false),
 ('roles.administrar',        'roles',         'Invitar, asignar roles, definir la junta', false),
 ('auditoria.ver',            'roles',         'Ver la bitácora', false),
 ('whatsapp.ver',             'whatsapp',      'Ver la bandeja de WhatsApp', false),
 ('whatsapp.enviar',          'whatsapp',      'Enviar mensajes y recibos por WhatsApp', false),
 ('whatsapp.configurar',      'whatsapp',      'Configurar la conexión de WhatsApp', false),
 ('chatbot.probar',           'whatsapp',      'Probar el chatbot desde la app', false),
 ('analitica.ver',            'analitica',     'Ver la analítica del edificio', false);

-- superadmin: todo.
INSERT INTO rol_permiso (rol, permiso) SELECT 'superadmin', codigo FROM permiso;

-- administrador: todo salvo votar (eso es de la junta).
INSERT INTO rol_permiso (rol, permiso) SELECT 'administrador', codigo FROM permiso
  WHERE codigo NOT IN ('trabajos.votar', 'pagos.informar', 'portal.ver');

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('junta','dashboard.ver'), ('junta','balance.ver'), ('junta','balance.ver_documentos'),
 ('junta','edificio.ver'), ('junta','unidades.ver'), ('junta','recibos.ver'), ('junta','morosidad.ver'),
 ('junta','reservas.ver'), ('junta','lecturas.ver'), ('junta','incidencias.reportar'), ('junta','incidencias.ver'),
 ('junta','trabajos.aprobar'), ('junta','trabajos.votar'), ('junta','analitica.ver'), ('junta','usuarios.ver'),
 ('junta','whatsapp.ver'),

 ('propietario','balance.ver'), ('propietario','balance.ver_documentos'), ('propietario','edificio.ver'),
 ('propietario','recibos.ver'), ('propietario','pagos.informar'), ('propietario','reservas.ver'),
 ('propietario','reservas.crear'), ('propietario','incidencias.reportar'), ('propietario','incidencias.ver'),
 ('propietario','portal.ver'),

 ('inquilino','edificio.ver'), ('inquilino','reservas.ver'), ('inquilino','reservas.crear'),
 ('inquilino','incidencias.reportar'), ('inquilino','incidencias.ver'), ('inquilino','portal.ver'),

 ('operario','edificio.ver'), ('operario','lecturas.ver'), ('operario','lecturas.registrar'),
 ('operario','reservas.ver'), ('operario','incidencias.reportar'), ('operario','incidencias.ver'),

 ('tecnico','edificio.ver'), ('tecnico','incidencias.reportar'), ('tecnico','incidencias.ver'),
 ('tecnico','trabajos.ejecutar');
