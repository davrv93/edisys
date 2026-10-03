-- 0024 · Gestión de deuda (bloques B3, D1, D2 y D3).
-- B3: cuentas por cobrar y estado de cuenta (solo consultas, sin tablas).
-- D1: acuerdos de pago con cuotas. El acuerdo no mueve dinero: los recibos vencidos que refinancia siguen
--     siendo la deuda (no hay doble conteo en balance, morosidad ni analítica); el acuerdo pone un calendario
--     encima. Mientras está activo, lo vencido de esos recibos deja de contar y cuentan solo las cuotas
--     vencidas e impagas. Por eso un acuerdo al día habilita reservas sin tocar el disparador de 0004.
-- D2: avisos de cobranza manuales y automáticos por escalas, con la evidencia de cada envío.
-- D3: grillas de morosos y puntualidad (consultas) y la etiqueta configurable de «moroso».

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('acuerdos.ver',       'deuda', 'Ver los acuerdos de pago y sus cuotas', false),
 ('acuerdos.gestionar', 'deuda', 'Crear, anular y adjuntar el documento firmado de un acuerdo de pago', false),
 ('avisos.gestionar',   'deuda', 'Configurar y enviar avisos de cobranza', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','acuerdos.ver'), ('superadmin','acuerdos.gestionar'), ('superadmin','avisos.gestionar'),
 ('administrador','acuerdos.ver'), ('administrador','acuerdos.gestionar'), ('administrador','avisos.gestionar'),
 ('junta','acuerdos.ver');

-- Configuración de deuda por edificio: el nombre visible de «moroso»/«puntual» y la tasa de mora mensual
-- (en puntos básicos: 100 = 1 % al mes) con la que se sugiere el recargo de un acuerdo.
CREATE TABLE deuda_config (
    edificio_id       bigint PRIMARY KEY REFERENCES edificio(id) ON DELETE CASCADE,
    etiqueta_moroso   text NOT NULL DEFAULT 'Moroso' CHECK (length(btrim(etiqueta_moroso)) > 0),
    etiqueta_puntual  text NOT NULL DEFAULT 'Puntual' CHECK (length(btrim(etiqueta_puntual)) > 0),
    tasa_mora_bp      int  NOT NULL DEFAULT 0 CHECK (tasa_mora_bp BETWEEN 0 AND 10000),
    actualizado_en    timestamptz NOT NULL DEFAULT now()
);

-- ---------- D1 · acuerdos de pago ----------

CREATE TABLE acuerdo_pago (
    id                  bigserial PRIMARY KEY,
    edificio_id         bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    unidad_id           bigint NOT NULL REFERENCES unidad(id),
    numero              text NOT NULL,
    comentario          text NOT NULL DEFAULT '',
    aceptado_por        text NOT NULL CHECK (length(btrim(aceptado_por)) > 0),
    fecha               date NOT NULL,
    saldo_cts           bigint NOT NULL CHECK (saldo_cts > 0),        -- saldo de los recibos al firmar
    mora_cts            bigint NOT NULL DEFAULT 0 CHECK (mora_cts >= 0), -- mora calculada (referencia)
    recargo_cts         bigint NOT NULL DEFAULT 0 CHECK (recargo_cts >= 0),
    descuento_cts       bigint NOT NULL DEFAULT 0 CHECK (descuento_cts >= 0),
    monto_acordado_cts  bigint NOT NULL CHECK (monto_acordado_cts > 0),
    pagado_inicial_cts  bigint NOT NULL DEFAULT 0 CHECK (pagado_inicial_cts >= 0), -- pagado de esos recibos al firmar
    n_cuotas            int  NOT NULL CHECK (n_cuotas BETWEEN 1 AND 60),
    estado              text NOT NULL DEFAULT 'activo' CHECK (estado IN ('activo','anulado')),
    documento_id        bigint REFERENCES archivo(id),               -- documento firmado (presidente + deudor)
    anulado_motivo      text,
    creado_por          bigint REFERENCES usuario(id),
    creado_en           timestamptz NOT NULL DEFAULT now(),
    CHECK (monto_acordado_cts = saldo_cts + recargo_cts - descuento_cts),
    UNIQUE (edificio_id, numero)
);
CREATE INDEX acuerdo_pago_unidad_idx ON acuerdo_pago (unidad_id) WHERE estado = 'activo';

CREATE TABLE acuerdo_cuota (
    id          bigserial PRIMARY KEY,
    acuerdo_id  bigint NOT NULL REFERENCES acuerdo_pago(id) ON DELETE CASCADE,
    numero      int  NOT NULL CHECK (numero >= 1),
    vence       date NOT NULL,
    monto_cts   bigint NOT NULL CHECK (monto_cts > 0),
    UNIQUE (acuerdo_id, numero)
);

-- Recibos que refinancia el acuerdo. Un recibo está en un solo acuerdo activo a la vez.
CREATE TABLE acuerdo_recibo (
    acuerdo_id  bigint NOT NULL REFERENCES acuerdo_pago(id) ON DELETE CASCADE,
    recibo_id   bigint NOT NULL REFERENCES recibo(id) ON DELETE CASCADE,
    saldo_cts   bigint NOT NULL CHECK (saldo_cts >= 0),   -- saldo del recibo al firmar (antes de recargo/descuento)
    activo      boolean NOT NULL DEFAULT true,
    PRIMARY KEY (acuerdo_id, recibo_id)
);
CREATE UNIQUE INDEX acuerdo_recibo_activo_uq ON acuerdo_recibo (recibo_id) WHERE activo;

-- Recargo y descuento del acuerdo entran como líneas «ajuste» de sus recibos; se marcan para revertirlas al anular.
ALTER TABLE recibo_linea ADD COLUMN acuerdo_id bigint REFERENCES acuerdo_pago(id) ON DELETE SET NULL;

-- Avance del acuerdo: lo cobrado en sus recibos desde que se firmó.
CREATE FUNCTION acuerdo_avance_cts(p_acuerdo bigint) RETURNS bigint AS $$
    SELECT GREATEST(0, COALESCE(SUM(r.pagado_cts), 0) - a.pagado_inicial_cts)::bigint
    FROM acuerdo_pago a
    LEFT JOIN acuerdo_recibo ar ON ar.acuerdo_id = a.id
    LEFT JOIN recibo r ON r.id = ar.recibo_id
    WHERE a.id = p_acuerdo
    GROUP BY a.pagado_inicial_cts;
$$ LANGUAGE sql STABLE;

-- Cuotas vencidas (vencimiento + días de gracia) que el avance todavía no cubre.
CREATE FUNCTION acuerdo_vencido_cts(p_acuerdo bigint) RETURNS bigint AS $$
    SELECT GREATEST(0, COALESCE(SUM(c.monto_cts) FILTER (
               WHERE c.vence + e.dias_gracia < (now() AT TIME ZONE 'America/Lima')::date), 0)
           - acuerdo_avance_cts(a.id))::bigint
    FROM acuerdo_pago a JOIN edificio e ON e.id = a.edificio_id
    LEFT JOIN acuerdo_cuota c ON c.acuerdo_id = a.id
    WHERE a.id = p_acuerdo
    GROUP BY a.id;
$$ LANGUAGE sql STABLE;

-- Morosidad con acuerdos: misma firma, así 03, 05, 07, el portal y el disparador de reservas la heredan.
-- Lo vencido de recibos en un acuerdo activo no cuenta; cuentan las cuotas vencidas e impagas del acuerdo.
CREATE OR REPLACE FUNCTION deuda_vencida_cts(p_unidad bigint) RETURNS bigint AS $$
    SELECT (
        COALESCE((SELECT SUM(r.total_cts - r.pagado_cts)
            FROM recibo r JOIN edificio e ON e.id = r.edificio_id
            WHERE r.unidad_id = p_unidad
              AND r.estado IN ('emitido','pagado_parcial')
              AND r.vence IS NOT NULL
              AND r.vence + e.dias_gracia < (now() AT TIME ZONE 'America/Lima')::date
              AND NOT EXISTS (SELECT 1 FROM acuerdo_recibo ar JOIN acuerdo_pago a ON a.id = ar.acuerdo_id
                              WHERE ar.recibo_id = r.id AND ar.activo AND a.estado = 'activo')), 0)
      + COALESCE((SELECT SUM(acuerdo_vencido_cts(a.id)) FROM acuerdo_pago a
            WHERE a.unidad_id = p_unidad AND a.estado = 'activo'), 0)
    )::bigint;
$$ LANGUAGE sql STABLE;

-- ---------- D2 · avisos de cobranza ----------

CREATE TABLE aviso_cobranza (
    id                bigserial PRIMARY KEY,
    edificio_id       bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    nombre            text NOT NULL CHECK (length(btrim(nombre)) > 0),
    frecuencia        text NOT NULL DEFAULT 'mensual' CHECK (frecuencia IN ('diaria','semanal','mensual')),
    dia               int  NOT NULL DEFAULT 1 CHECK (dia BETWEEN 1 AND 31),   -- semanal: 1=lunes…7=domingo; mensual: día del mes
    tipo_escala       text NOT NULL DEFAULT 'monto' CHECK (tipo_escala IN ('monto','recibos')),
    canales           text[] NOT NULL DEFAULT '{correo}' CHECK (canales <@ ARRAY['correo','whatsapp']::text[] AND cardinality(canales) > 0),
    adjunta_pdf       boolean NOT NULL DEFAULT false,
    activo            boolean NOT NULL DEFAULT true,
    ultima_ejecucion  date,
    creado_por        bigint REFERENCES usuario(id),
    creado_en         timestamptz NOT NULL DEFAULT now()
);

-- Tramos: «desde» y «hasta» en céntimos de deuda vencida (tipo monto) o en número de recibos vencidos.
-- hasta NULL = sin tope. El cuerpo admite {{nombre}}, {{unidad}}, {{deuda}}, {{recibos}} y {{edificio}}.
CREATE TABLE aviso_escala (
    id         bigserial PRIMARY KEY,
    aviso_id   bigint NOT NULL REFERENCES aviso_cobranza(id) ON DELETE CASCADE,
    desde      bigint NOT NULL CHECK (desde >= 0),
    hasta      bigint CHECK (hasta IS NULL OR hasta >= desde),
    asunto     text NOT NULL CHECK (length(btrim(asunto)) > 0),
    cuerpo     text NOT NULL CHECK (length(btrim(cuerpo)) > 0),
    orden      int  NOT NULL DEFAULT 0
);
CREATE INDEX aviso_escala_idx ON aviso_escala (aviso_id, desde);

-- Evidencia: un registro por unidad y canal, con el mensaje de la bandeja (correo o WhatsApp) que lo lleva.
CREATE TABLE aviso_envio (
    id           bigserial PRIMARY KEY,
    edificio_id  bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    aviso_id     bigint REFERENCES aviso_cobranza(id) ON DELETE SET NULL,  -- NULL = aviso manual
    escala_id    bigint REFERENCES aviso_escala(id) ON DELETE SET NULL,
    unidad_id    bigint NOT NULL REFERENCES unidad(id) ON DELETE CASCADE,
    canal        text NOT NULL CHECK (canal IN ('correo','whatsapp')),
    fecha        date NOT NULL,
    deuda_cts    bigint NOT NULL DEFAULT 0,
    recibos      int NOT NULL DEFAULT 0,
    asunto       text NOT NULL DEFAULT '',
    mensaje_id   bigint,                                  -- correo_mensaje.id o whatsapp_mensaje.id según el canal
    estado       text NOT NULL CHECK (estado IN ('encolado','sin_contacto','error')),
    detalle      text NOT NULL DEFAULT '',
    enviado_por  bigint REFERENCES usuario(id),
    creado_en    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX aviso_envio_idx ON aviso_envio (edificio_id, creado_en DESC);
-- El automático no repite a la misma unidad por el mismo canal el mismo día.
CREATE UNIQUE INDEX aviso_envio_dia_uq ON aviso_envio (aviso_id, unidad_id, canal, fecha) WHERE aviso_id IS NOT NULL;
