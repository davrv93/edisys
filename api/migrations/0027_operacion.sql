-- 0027 · Operación del edificio (bloque G): cuaderno de ocurrencias, tickets con SLA,
-- registro de visitas con QR, parking y paquetes. Todo interno: el QR se genera y se valida
-- en el propio API; los avisos salen por la bandeja de WhatsApp (simulado si no hay Evolution).

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('ocurrencias.ver',       'ocurrencias', 'Ver el cuaderno de ocurrencias', false),
 ('ocurrencias.registrar', 'ocurrencias', 'Anotar, cerrar y escalar ocurrencias', false),
 ('tickets.configurar',    'tickets',     'Configurar el SLA y la respuesta automática de los tickets', false),
 ('visitas.ver',           'visitas',     'Ver visitas y accesos', false),
 ('visitas.autorizar',     'visitas',     'Autorizar visitas (genera el QR)', false),
 ('visitas.validar',       'visitas',     'Validar el QR en portería y registrar entradas y salidas', false),
 ('parking.ver',           'parking',     'Ver estacionamientos y sesiones', false),
 ('parking.operar',        'parking',     'Registrar entradas y salidas del parking', false),
 ('parking.administrar',   'parking',     'Configurar estacionamientos y tarifas', false),
 ('paquetes.ver',          'paquetes',    'Ver los paquetes recibidos', false),
 ('paquetes.registrar',    'paquetes',    'Recibir, entregar y devolver paquetes', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','ocurrencias.ver'), ('superadmin','ocurrencias.registrar'), ('superadmin','tickets.configurar'),
 ('superadmin','visitas.ver'), ('superadmin','visitas.autorizar'), ('superadmin','visitas.validar'),
 ('superadmin','parking.ver'), ('superadmin','parking.operar'), ('superadmin','parking.administrar'),
 ('superadmin','paquetes.ver'), ('superadmin','paquetes.registrar'),
 ('administrador','ocurrencias.ver'), ('administrador','ocurrencias.registrar'), ('administrador','tickets.configurar'),
 ('administrador','visitas.ver'), ('administrador','visitas.autorizar'), ('administrador','visitas.validar'),
 ('administrador','parking.ver'), ('administrador','parking.operar'), ('administrador','parking.administrar'),
 ('administrador','paquetes.ver'), ('administrador','paquetes.registrar'),
 ('junta','ocurrencias.ver'), ('junta','visitas.ver'), ('junta','parking.ver'),
 -- El conserje (operario) lleva la portería: cuaderno, QR, parking y paquetes.
 ('operario','ocurrencias.ver'), ('operario','ocurrencias.registrar'),
 ('operario','visitas.ver'), ('operario','visitas.validar'), ('operario','visitas.autorizar'),
 ('operario','parking.ver'), ('operario','parking.operar'),
 ('operario','paquetes.ver'), ('operario','paquetes.registrar'),
 -- Residentes: autorizan las visitas de su unidad y ven sus paquetes (filtro por unidad en Go).
 ('propietario','visitas.ver'), ('propietario','visitas.autorizar'), ('propietario','paquetes.ver'),
 ('inquilino','visitas.ver'), ('inquilino','visitas.autorizar'), ('inquilino','paquetes.ver');

-- ---------------------------------------------------------------------------------------------
-- G1 · Cuaderno de ocurrencias. Es una bitácora: lo anotado no se edita ni se borra; solo se
-- cierra con nota o se escala a una incidencia del tablero de mantenimiento.
CREATE TABLE ocurrencia (
    id              bigserial PRIMARY KEY,
    edificio_id     bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    numero          int NOT NULL,
    titulo          text NOT NULL CHECK (length(btrim(titulo)) > 0),
    descripcion     text NOT NULL DEFAULT '',
    prioridad       text NOT NULL DEFAULT 'media' CHECK (prioridad IN ('alta','media','baja')),
    estado          text NOT NULL DEFAULT 'abierta' CHECK (estado IN ('abierta','cerrada','escalada')),
    foto_id         bigint REFERENCES archivo(id),
    empleado        text NOT NULL DEFAULT '',            -- quién estaba de turno (texto libre)
    registrado_por  bigint REFERENCES usuario(id),
    registrado_en   timestamptz NOT NULL DEFAULT now(),
    cierre_nota     text NOT NULL DEFAULT '',
    cerrado_por     bigint REFERENCES usuario(id),
    cerrado_en      timestamptz,
    incidencia_id   bigint REFERENCES incidencia(id) ON DELETE SET NULL,
    UNIQUE (edificio_id, numero),
    CHECK (estado = 'abierta' OR cerrado_en IS NOT NULL)
);
CREATE INDEX ocurrencia_idx ON ocurrencia (edificio_id, estado, registrado_en DESC);

-- Regla dura: lo escrito no se reescribe y una ocurrencia cerrada o escalada no se reabre.
CREATE FUNCTION ocurrencia_inmutable() RETURNS trigger AS $$
BEGIN
    IF NEW.titulo IS DISTINCT FROM OLD.titulo OR NEW.descripcion IS DISTINCT FROM OLD.descripcion
       OR NEW.registrado_en IS DISTINCT FROM OLD.registrado_en OR NEW.registrado_por IS DISTINCT FROM OLD.registrado_por
       OR NEW.numero IS DISTINCT FROM OLD.numero THEN
        RAISE EXCEPTION 'OCURRENCIA_INMUTABLE: el cuaderno no se reescribe' USING ERRCODE = 'ED010';
    END IF;
    IF OLD.estado <> 'abierta' AND (NEW.estado IS DISTINCT FROM OLD.estado OR NEW.cierre_nota IS DISTINCT FROM OLD.cierre_nota
       OR NEW.cerrado_en IS DISTINCT FROM OLD.cerrado_en) THEN
        RAISE EXCEPTION 'OCURRENCIA_CERRADA: de % a %', OLD.estado, NEW.estado USING ERRCODE = 'ED010';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER ocurrencia_inmutable_trg BEFORE UPDATE ON ocurrencia
    FOR EACH ROW EXECUTE FUNCTION ocurrencia_inmutable();

-- ---------------------------------------------------------------------------------------------
-- G2 · Tickets con SLA. Las incidencias del tablero son los tickets: cada una lleva su objetivo
-- (horas) y su vencimiento, calculados por la criticidad con la tabla del edificio.
CREATE TABLE ticket_sla_config (
    edificio_id           bigint PRIMARY KEY REFERENCES edificio(id) ON DELETE CASCADE,
    horas_critica         int NOT NULL DEFAULT 24  CHECK (horas_critica > 0),
    horas_media           int NOT NULL DEFAULT 72  CHECK (horas_media > 0),
    horas_baja            int NOT NULL DEFAULT 168 CHECK (horas_baja > 0),
    horas_sin_clasificar  int NOT NULL DEFAULT 72  CHECK (horas_sin_clasificar > 0),
    respuesta_automatica  boolean NOT NULL DEFAULT true,
    mensaje               text NOT NULL DEFAULT 'Hola {{nombre}}, recibimos tu reporte {{codigo}} ({{titulo}}). Lo atenderemos en un plazo de {{plazo}}, hasta el {{vence}}. Te avisaremos cada avance.',
    actualizado_en        timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE incidencia
    ADD COLUMN sla_objetivo       int,           -- horas para resolver
    ADD COLUMN sla_vencimiento    timestamptz,
    ADD COLUMN respuesta_auto_en  timestamptz;   -- cuándo salió la respuesta automática al solicitante

-- Horas del SLA para una criticidad (sin fila del edificio = valores de fábrica).
CREATE FUNCTION sla_horas(eid bigint, crit text) RETURNS int AS $$
    SELECT COALESCE((SELECT CASE crit WHEN 'critica' THEN c.horas_critica WHEN 'media' THEN c.horas_media
                                      WHEN 'baja' THEN c.horas_baja ELSE c.horas_sin_clasificar END
                     FROM ticket_sla_config c WHERE c.edificio_id = eid),
                    CASE crit WHEN 'critica' THEN 24 WHEN 'media' THEN 72 WHEN 'baja' THEN 168 ELSE 72 END);
$$ LANGUAGE sql STABLE;

-- Al crear el ticket y cada vez que cambia su criticidad, el vencimiento se recalcula desde que se reportó.
CREATE FUNCTION incidencia_sla() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'INSERT' OR NEW.criticidad IS DISTINCT FROM OLD.criticidad THEN
        NEW.sla_objetivo := sla_horas(NEW.edificio_id, NEW.criticidad);
        NEW.sla_vencimiento := NEW.creado_en + make_interval(hours => NEW.sla_objetivo);
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER incidencia_sla_trg BEFORE INSERT OR UPDATE OF criticidad ON incidencia
    FOR EACH ROW EXECUTE FUNCTION incidencia_sla();

-- Tickets que ya existían: se les pone su SLA sin tocar actualizado_en.
ALTER TABLE incidencia DISABLE TRIGGER incidencia_valida_estado_trg;
UPDATE incidencia SET sla_objetivo = sla_horas(edificio_id, criticidad),
    sla_vencimiento = creado_en + make_interval(hours => sla_horas(edificio_id, criticidad));
ALTER TABLE incidencia ENABLE TRIGGER incidencia_valida_estado_trg;

-- ---------------------------------------------------------------------------------------------
-- G3 · Visitas e identificación QR. El QR lleva un código aleatorio que solo este API reconoce.
CREATE TABLE visita (
    id              bigserial PRIMARY KEY,
    edificio_id     bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    unidad_id       bigint NOT NULL REFERENCES unidad(id) ON DELETE CASCADE,
    visitante       text NOT NULL CHECK (length(btrim(visitante)) > 0),
    documento       text NOT NULL DEFAULT '',
    vehiculo_placa  text NOT NULL DEFAULT '',
    motivo          text NOT NULL DEFAULT '',
    autorizado_por  bigint REFERENCES usuario(id),
    valido_desde    timestamptz NOT NULL,
    valido_hasta    timestamptz NOT NULL,
    usos_max        int NOT NULL DEFAULT 1 CHECK (usos_max BETWEEN 1 AND 100),
    usos            int NOT NULL DEFAULT 0 CHECK (usos >= 0),
    codigo_qr       text NOT NULL UNIQUE,
    estado          text NOT NULL DEFAULT 'autorizada' CHECK (estado IN ('autorizada','en_curso','finalizada','anulada')),
    creado_en       timestamptz NOT NULL DEFAULT now(),
    CHECK (valido_hasta > valido_desde),
    CHECK (usos <= usos_max)
);
CREATE INDEX visita_idx ON visita (edificio_id, valido_desde DESC);

-- Bitácora de portería: cada lectura de QR (permitida o rechazada) y cada salida.
CREATE TABLE acceso (
    id              bigserial PRIMARY KEY,
    edificio_id     bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    visita_id       bigint REFERENCES visita(id) ON DELETE SET NULL,
    tipo            text NOT NULL CHECK (tipo IN ('entrada','salida')),
    resultado       text NOT NULL CHECK (resultado IN ('permitido','rechazado')),
    motivo          text NOT NULL DEFAULT '',
    codigo_leido    text NOT NULL DEFAULT '',
    registrado_por  bigint REFERENCES usuario(id),
    creado_en       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX acceso_idx ON acceso (edificio_id, creado_en DESC);

-- ---------------------------------------------------------------------------------------------
-- G4 · Parking: estacionamientos con tarifa por fracción y sesiones de entrada/salida.
CREATE TABLE estacionamiento (
    id               bigserial PRIMARY KEY,
    edificio_id      bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    codigo           text NOT NULL CHECK (length(btrim(codigo)) > 0),
    tipo             text NOT NULL DEFAULT 'visitas' CHECK (tipo IN ('propio','visitas','alquiler')),
    unidad_id        bigint REFERENCES unidad(id) ON DELETE SET NULL,  -- dueño si es «propio»
    tarifa_hora_cts  bigint NOT NULL DEFAULT 0 CHECK (tarifa_hora_cts >= 0),
    fraccion_min     int NOT NULL DEFAULT 60 CHECK (fraccion_min BETWEEN 1 AND 1440),
    tolerancia_min   int NOT NULL DEFAULT 0 CHECK (tolerancia_min >= 0),
    activo           boolean NOT NULL DEFAULT true,
    creado_en        timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX estacionamiento_uq ON estacionamiento (edificio_id, lower(btrim(codigo)));

CREATE TABLE sesion_parking (
    id                  bigserial PRIMARY KEY,
    edificio_id         bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    estacionamiento_id  bigint NOT NULL REFERENCES estacionamiento(id),
    placa               text NOT NULL CHECK (length(btrim(placa)) > 0),
    unidad_id           bigint REFERENCES unidad(id) ON DELETE SET NULL,   -- a quién se cobra
    visita_id           bigint REFERENCES visita(id) ON DELETE SET NULL,
    entrada_en          timestamptz NOT NULL DEFAULT now(),
    salida_en           timestamptz,
    minutos             int CHECK (minutos >= 0),
    monto_cts           bigint CHECK (monto_cts >= 0),
    cobro               text CHECK (cobro IN ('recibo','inmediato','sin_cobro')),
    medio               text NOT NULL DEFAULT '',
    ajuste_id           bigint REFERENCES ajuste(id) ON DELETE SET NULL,           -- cobro al recibo
    ingreso_id          bigint REFERENCES ingreso_externo(id) ON DELETE SET NULL,  -- cobro inmediato
    registrado_por      bigint REFERENCES usuario(id),
    cerrado_por         bigint REFERENCES usuario(id),
    CHECK (salida_en IS NULL OR salida_en >= entrada_en),
    CHECK ((salida_en IS NULL) = (cobro IS NULL)),
    CHECK (cobro IS DISTINCT FROM 'recibo' OR unidad_id IS NOT NULL)
);
-- Un espacio, una sesión abierta; una placa no puede estar dentro dos veces.
CREATE UNIQUE INDEX sesion_parking_espacio_uq ON sesion_parking (estacionamiento_id) WHERE salida_en IS NULL;
CREATE UNIQUE INDEX sesion_parking_placa_uq ON sesion_parking (edificio_id, upper(btrim(placa))) WHERE salida_en IS NULL;
CREATE INDEX sesion_parking_idx ON sesion_parking (edificio_id, entrada_en DESC);

-- ---------------------------------------------------------------------------------------------
-- G5 · Paquetes: se reciben en portería, se avisa a la unidad y se entregan con firma o foto.
CREATE TABLE paquete (
    id                bigserial PRIMARY KEY,
    edificio_id       bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    unidad_id         bigint NOT NULL REFERENCES unidad(id) ON DELETE CASCADE,
    remitente         text NOT NULL DEFAULT '',
    descripcion       text NOT NULL CHECK (length(btrim(descripcion)) > 0),
    foto_id           bigint REFERENCES archivo(id),
    recibido_por      bigint REFERENCES usuario(id),
    recibido_en       timestamptz NOT NULL DEFAULT now(),
    estado            text NOT NULL DEFAULT 'recibido' CHECK (estado IN ('recibido','entregado','devuelto')),
    aviso_mensaje_id  bigint REFERENCES whatsapp_mensaje(id) ON DELETE SET NULL,
    aviso_en          timestamptz,
    entregado_a       text NOT NULL DEFAULT '',
    entrega_firma_id  bigint REFERENCES archivo(id),
    entregado_por     bigint REFERENCES usuario(id),
    entregado_en      timestamptz,
    motivo_devolucion text NOT NULL DEFAULT '',
    -- Sin nombre de quien recoge y su firma o foto, no hay entrega.
    CHECK (estado <> 'entregado' OR (entregado_en IS NOT NULL AND length(btrim(entregado_a)) > 0 AND entrega_firma_id IS NOT NULL)),
    CHECK (estado <> 'devuelto' OR length(btrim(motivo_devolucion)) > 0)
);
CREATE INDEX paquete_idx ON paquete (edificio_id, estado, recibido_en DESC);
