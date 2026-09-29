-- 0012 · Facturación electrónica SUNAT: configuración por edificio, series con correlativo sin huecos
-- y comprobantes (boleta 03, factura 01, nota de crédito 07) con su XML firmado y el CDR.
-- Modos: off | simulado | beta | produccion (producción deshabilitada en esta entrega).

INSERT INTO permiso (codigo, modulo, descripcion, ajustable) VALUES
 ('facturacion.configurar', 'recibos', 'Configurar la facturación electrónica (SUNAT)', false),
 ('comprobantes.emitir',    'recibos', 'Emitir y anular boletas y facturas electrónicas', false);
INSERT INTO rol_permiso (rol, permiso) VALUES
 ('superadmin', 'facturacion.configurar'), ('superadmin', 'comprobantes.emitir'),
 ('administrador', 'facturacion.configurar'), ('administrador', 'comprobantes.emitir');

CREATE TABLE facturacion_config (
    edificio_id            bigint PRIMARY KEY REFERENCES edificio(id) ON DELETE CASCADE,
    ruc                    text NOT NULL DEFAULT '',
    razon_social           text NOT NULL DEFAULT '',
    direccion              text NOT NULL DEFAULT '',
    ubigeo                 text NOT NULL DEFAULT '',
    serie_boleta           text NOT NULL DEFAULT 'B001' CHECK (serie_boleta ~ '^B[A-Z0-9]{3}$'),
    serie_factura          text NOT NULL DEFAULT 'F001' CHECK (serie_factura ~ '^F[A-Z0-9]{3}$'),
    modo                   text NOT NULL DEFAULT 'off' CHECK (modo IN ('off','simulado','beta','produccion')),
    ose_url                text NOT NULL DEFAULT '',
    ose_usuario            text NOT NULL DEFAULT '',
    ose_clave              text NOT NULL DEFAULT '',            -- nunca se devuelve por el API
    certificado_archivo_id bigint REFERENCES archivo(id),
    certificado_clave      text NOT NULL DEFAULT '',            -- nunca se devuelve por el API
    certificado_vence      date,
    afectacion             jsonb NOT NULL DEFAULT '{"cuota":"inafecto","agua":"inafecto","agua_comun":"inafecto","energia_comun":"inafecto","reserva":"gravado","concepto":"inafecto","multa":"inafecto","ajuste":"inafecto","saldo_anterior":"inafecto","deuda_inicial":"inafecto"}',
    actualizado_por        bigint REFERENCES usuario(id),
    actualizado_en         timestamptz NOT NULL DEFAULT now()
);

-- Correlativo por serie: se toma con UPDATE … RETURNING dentro de la transacción de la emisión,
-- así dos emisiones a la vez esperan su turno y un rollback no deja huecos.
CREATE TABLE comprobante_serie (
    edificio_id bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    serie       text NOT NULL,
    ultimo      bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (edificio_id, serie)
);

CREATE TABLE comprobante (
    id                bigserial PRIMARY KEY,
    edificio_id       bigint NOT NULL REFERENCES edificio(id) ON DELETE CASCADE,
    recibo_id         bigint REFERENCES recibo(id),
    tipo              text NOT NULL CHECK (tipo IN ('01','03','07')),
    serie             text NOT NULL,
    numero            bigint NOT NULL CHECK (numero > 0),
    fecha             date NOT NULL,
    cliente_tipo_doc  text NOT NULL,
    cliente_doc       text NOT NULL,
    cliente_nombre    text NOT NULL,
    gravado_cts       bigint NOT NULL DEFAULT 0,
    exonerado_cts     bigint NOT NULL DEFAULT 0,
    inafecto_cts      bigint NOT NULL DEFAULT 0,
    igv_cts           bigint NOT NULL DEFAULT 0,
    total_cts         bigint NOT NULL,
    modo              text NOT NULL,
    estado            text NOT NULL CHECK (estado IN ('aceptado','rechazado','pendiente','anulado')),
    xml               text NOT NULL,
    hash              text NOT NULL,
    cdr_codigo        text NOT NULL DEFAULT '',
    cdr_descripcion   text NOT NULL DEFAULT '',
    cdr_xml           text NOT NULL DEFAULT '',
    firmado_prueba    boolean NOT NULL DEFAULT false,
    referencia_id     bigint REFERENCES comprobante(id),    -- nota de crédito → comprobante anulado
    anulacion         text CHECK (anulacion IN ('baja','nota_credito')),
    anulacion_motivo  text,
    baja_id           text,                                  -- RA-AAAAMMDD-n
    baja_ticket       text,
    creado_por        bigint REFERENCES usuario(id),
    creado_en         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (edificio_id, serie, numero)
);
-- Un comprobante vivo por recibo.
CREATE UNIQUE INDEX comprobante_recibo_uq ON comprobante (recibo_id) WHERE tipo IN ('01','03') AND estado <> 'anulado' AND estado <> 'rechazado';
