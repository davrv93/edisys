-- 0030 · Recaudación por archivo (bloques A1 y A2). Cero conexiones de red: todo entra y sale como archivo.
-- A1: recaudadora externa (banco, agentes, Yape por código de recibo). Llegan dos reportes:
--     transacciones (cada pago) y liquidaciones (lo que la recaudadora abona, neto de su comisión).
-- A2: cuenta recaudadora en banco. EDISYS genera el CREP (deudas del periodo) que el usuario sube al banco
--     a mano, y lee el CDPG (pagos) que el banco devuelve: «cobranza masiva».

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('recaudacion.ver',       'recaudacion', 'Ver recaudadora, descargas CREP y cobranza masiva', false),
 ('recaudacion.gestionar', 'recaudacion', 'Importar transacciones, liquidaciones y CDPG; generar CREP', false);

INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin','recaudacion.ver'), ('superadmin','recaudacion.gestionar'),
 ('administrador','recaudacion.ver'), ('administrador','recaudacion.gestionar'),
 ('junta','recaudacion.ver');

-- ---------- A1 · recaudadora externa ----------

CREATE TABLE cuenta_recaudadora (
    id                   bigserial PRIMARY KEY,
    edificio_id          bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    proveedor            text NOT NULL CHECK (length(btrim(proveedor)) > 0),
    codigo_convenio      text NOT NULL DEFAULT '',
    -- Medio con el que se registra el pago cuando el archivo no lo trae (o trae uno que no conocemos).
    medio                text NOT NULL DEFAULT 'deposito' CHECK (medio IN ('yape','plin','transferencia','efectivo','deposito','tarjeta')),
    -- Último mapeo de columnas usado en cada reporte: la próxima carga lo propone.
    mapeo_transacciones  jsonb NOT NULL DEFAULT '{}',
    mapeo_liquidaciones  jsonb NOT NULL DEFAULT '{}',
    activo               boolean NOT NULL DEFAULT true,
    creado_en            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (edificio_id, proveedor, codigo_convenio)
);

-- Comisión pactada: porcentaje en puntos básicos (150 = 1,50 %) más un fijo por liquidación.
-- Solo se usa cuando el reporte de liquidaciones no trae la comisión.
CREATE TABLE recaudadora_comision_config (
    cuenta_recaudadora_id bigint PRIMARY KEY REFERENCES cuenta_recaudadora(id) ON DELETE CASCADE,
    porcentaje_pbs        int NOT NULL DEFAULT 0 CHECK (porcentaje_pbs BETWEEN 0 AND 10000),
    fijo_cts              bigint NOT NULL DEFAULT 0 CHECK (fijo_cts >= 0),
    rubro_id              bigint REFERENCES rubro(id) ON DELETE SET NULL, -- rubro del egreso de comisión
    actualizado_en        timestamptz NOT NULL DEFAULT now()
);

-- Cada pago reportado por la recaudadora. «acreditada» ya creó su pago; «observada» no encontró recibo
-- o no cabía en la deuda: queda a la vista y se reintenta si el mismo código vuelve en otra carga.
CREATE TABLE recaudadora_transaccion (
    id                     bigserial PRIMARY KEY,
    edificio_id            bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    cuenta_recaudadora_id  bigint NOT NULL REFERENCES cuenta_recaudadora(id) ON DELETE CASCADE,
    codigo_recibo          text NOT NULL,
    recibo_id              bigint REFERENCES recibo(id) ON DELETE SET NULL,
    monto_cts              bigint NOT NULL CHECK (monto_cts > 0),
    medio                  text NOT NULL DEFAULT '',
    codigo_operacion       text NOT NULL CHECK (length(btrim(codigo_operacion)) > 0),
    fecha                  date NOT NULL,
    estado                 text NOT NULL CHECK (estado IN ('acreditada','observada')),
    motivo                 text NOT NULL DEFAULT '',
    pago_id                bigint REFERENCES pago(id) ON DELETE SET NULL,
    archivo_nombre         text NOT NULL DEFAULT '',
    creado_por             bigint REFERENCES usuario(id),
    creado_en              timestamptz NOT NULL DEFAULT now(),
    -- Idempotencia: el mismo código de operación no entra dos veces por la misma recaudadora.
    UNIQUE (cuenta_recaudadora_id, codigo_operacion)
);
CREATE INDEX recaudadora_transaccion_idx ON recaudadora_transaccion (edificio_id, estado, fecha);

-- Lo que la recaudadora abona: neto = bruto − comisión, al céntimo (regla dura en la base).
CREATE TABLE recaudadora_liquidacion (
    id                     bigserial PRIMARY KEY,
    edificio_id            bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    cuenta_recaudadora_id  bigint NOT NULL REFERENCES cuenta_recaudadora(id) ON DELETE CASCADE,
    codigo_liquidacion     text NOT NULL CHECK (length(btrim(codigo_liquidacion)) > 0),
    fecha                  date NOT NULL,
    monto_bruto_cts        bigint NOT NULL CHECK (monto_bruto_cts > 0),
    comision_cts           bigint NOT NULL CHECK (comision_cts >= 0 AND comision_cts <= monto_bruto_cts),
    monto_neto_cts         bigint NOT NULL,
    egreso_id              bigint REFERENCES egreso(id) ON DELETE SET NULL, -- la comisión entra como egreso
    archivo_nombre         text NOT NULL DEFAULT '',
    creado_por             bigint REFERENCES usuario(id),
    creado_en              timestamptz NOT NULL DEFAULT now(),
    CHECK (monto_neto_cts = monto_bruto_cts - comision_cts),
    UNIQUE (cuenta_recaudadora_id, codigo_liquidacion)
);
CREATE INDEX recaudadora_liquidacion_idx ON recaudadora_liquidacion (edificio_id, fecha);

-- Estado «Enviado a recaudadora» del recibo (lo marcan el envío a la recaudadora y el CREP).
ALTER TABLE recibo ADD COLUMN enviado_recaudadora_en timestamptz;

-- ---------- A2 · CREP / CDPG ----------

-- Descargas: el CREP generado se guarda 24 h; al expirar se borra el contenido y queda la fila como rastro.
CREATE TABLE crep_archivo (
    id                  bigserial PRIMARY KEY,
    edificio_id         bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    periodo             text NOT NULL CHECK (periodo ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    cuenta_bancaria_id  bigint REFERENCES cuenta_bancaria(id) ON DELETE SET NULL,
    layout              text NOT NULL,
    nombre_archivo      text NOT NULL,
    contenido           text,
    filas               int NOT NULL DEFAULT 0,
    total_cts           bigint NOT NULL DEFAULT 0,
    estado              text NOT NULL DEFAULT 'completado' CHECK (estado IN ('completado','expirado')),
    creado_por          bigint REFERENCES usuario(id),
    creado_en           timestamptz NOT NULL DEFAULT now(),
    expira_en           timestamptz NOT NULL DEFAULT now() + interval '24 hours'
);
CREATE INDEX crep_archivo_idx ON crep_archivo (edificio_id, creado_en DESC);

-- Cada CDPG subido («cobranza masiva») con su resumen.
CREATE TABLE cdpg_carga (
    id                bigserial PRIMARY KEY,
    edificio_id       bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    crep_archivo_id   bigint REFERENCES crep_archivo(id) ON DELETE SET NULL,
    layout            text NOT NULL,
    nombre_archivo    text NOT NULL DEFAULT '',
    filas_ok          int NOT NULL DEFAULT 0,
    filas_error       int NOT NULL DEFAULT 0,
    total_cts         bigint NOT NULL DEFAULT 0,
    errores           jsonb NOT NULL DEFAULT '[]',
    creado_por        bigint REFERENCES usuario(id),
    creado_en         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cdpg_carga_idx ON cdpg_carga (edificio_id, creado_en DESC);

-- Cada pago del CDPG ya aplicado. Regla dura: un pago del banco se concilia una sola vez.
CREATE TABLE cdpg_movimiento (
    id                  bigserial PRIMARY KEY,
    edificio_id         bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    carga_id            bigint NOT NULL REFERENCES cdpg_carga(id) ON DELETE CASCADE,
    fecha               date NOT NULL,
    agencia             text NOT NULL DEFAULT '',
    numero_operacion    text NOT NULL CHECK (length(btrim(numero_operacion)) > 0),
    codigo_depositante  text NOT NULL DEFAULT '',
    referencia          text NOT NULL DEFAULT '',
    monto_cts           bigint NOT NULL CHECK (monto_cts > 0),
    recibo_id           bigint REFERENCES recibo(id) ON DELETE SET NULL,
    pago_id             bigint REFERENCES pago(id) ON DELETE SET NULL,
    creado_en           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (edificio_id, fecha, agencia, numero_operacion)
);
