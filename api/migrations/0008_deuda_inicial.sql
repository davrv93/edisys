-- 0008 · La deuda inicial (importada con el Excel) entra en la cuenta corriente de la unidad.
-- Cada fila de deuda_inicial se vuelve un «cargo por periodo»: un recibo con origen = 'deuda_inicial',
-- ya emitido y vencido al cierre de su mes. Así cuenta en la antigüedad, la morosidad, el bloqueo de
-- reservas, el portal y el chatbot sin tocar deuda_vencida_cts() ni es_moroso().
-- Los periodos anteriores a EDISYS se guardan como periodo «histórico» (no se listan ni se emiten).

ALTER TABLE periodo ADD COLUMN historico boolean NOT NULL DEFAULT false;

ALTER TABLE recibo ADD COLUMN origen text NOT NULL DEFAULT 'periodo'
    CHECK (origen IN ('periodo', 'deuda_inicial'));

-- Un recibo de cuotas por unidad y periodo, y aparte un cargo de deuda inicial por unidad y periodo.
DROP INDEX recibo_unidad_periodo_uq;
CREATE UNIQUE INDEX recibo_unidad_periodo_uq ON recibo (periodo_id, unidad_id) WHERE estado <> 'anulado' AND origen = 'periodo';
CREATE UNIQUE INDEX recibo_deuda_inicial_uq ON recibo (periodo_id, unidad_id) WHERE estado <> 'anulado' AND origen = 'deuda_inicial';
CREATE INDEX recibo_pendiente_idx ON recibo (unidad_id, vence) WHERE estado IN ('emitido', 'pagado_parcial');

-- Tipos de línea nuevos: la deuda inicial y los ajustes (notas de cargo/abono internas de 0009).
ALTER TABLE recibo_linea DROP CONSTRAINT recibo_linea_tipo_check;
ALTER TABLE recibo_linea ADD CONSTRAINT recibo_linea_tipo_check
    CHECK (tipo IN ('cuota','agua','agua_comun','energia_comun','reserva','concepto','multa','saldo_anterior','deuda_inicial','ajuste'));

ALTER TABLE deuda_inicial ADD COLUMN recibo_id bigint REFERENCES recibo(id) ON DELETE SET NULL;

-- Un pago que se reparte entre varios recibos (del más antiguo al más nuevo) se guarda como varias
-- partes con el mismo código de operación. La unicidad del código se mira solo en la parte 1.
ALTER TABLE pago ADD COLUMN parte int NOT NULL DEFAULT 1 CHECK (parte >= 1);
DROP INDEX pago_operacion_uq;
CREATE UNIQUE INDEX pago_operacion_uq ON pago (edificio_id, medio, fecha, codigo_operacion)
    WHERE codigo_operacion IS NOT NULL AND codigo_operacion <> '' AND estado <> 'rechazado' AND parte = 1;

-- cargar_deuda_inicial crea (o actualiza) el cargo de la unidad por ese periodo y devuelve el id del recibo.
-- Vence el último día de su mes: para la morosidad ya está vencida.
CREATE FUNCTION cargar_deuda_inicial(p_unidad bigint, p_periodo text, p_monto bigint) RETURNS bigint AS $$
DECLARE
    v_edificio bigint;
    v_codigo   text;
    v_periodo  bigint;
    v_recibo   bigint;
    v_pagado   bigint;
BEGIN
    SELECT edificio_id, codigo INTO v_edificio, v_codigo FROM unidad WHERE id = p_unidad;
    IF v_edificio IS NULL THEN
        RAISE EXCEPTION 'la unidad % no existe', p_unidad;
    END IF;
    INSERT INTO periodo (edificio_id, periodo, fecha_corte, estado, historico)
    VALUES (v_edificio, p_periodo, (p_periodo || '-01')::date, 'cerrado', true)
    ON CONFLICT (edificio_id, periodo) DO NOTHING;
    SELECT id INTO v_periodo FROM periodo WHERE edificio_id = v_edificio AND periodo = p_periodo;

    SELECT id, pagado_cts INTO v_recibo, v_pagado FROM recibo
     WHERE periodo_id = v_periodo AND unidad_id = p_unidad AND origen = 'deuda_inicial' AND estado <> 'anulado';
    IF v_recibo IS NULL THEN
        INSERT INTO recibo (edificio_id, periodo_id, unidad_id, numero, estado, total_cts, emitido_en, vence, origen)
        VALUES (v_edificio, v_periodo, p_unidad, 'DI-' || p_periodo || '-' || v_codigo, 'emitido', p_monto, now(),
                ((p_periodo || '-01')::date + interval '1 month' - interval '1 day')::date, 'deuda_inicial')
        RETURNING id INTO v_recibo;
        INSERT INTO recibo_linea (recibo_id, tipo, descripcion, monto_cts, orden)
        VALUES (v_recibo, 'deuda_inicial', 'Deuda anterior (' || p_periodo || ')', p_monto, 1);
    ELSE
        IF p_monto < v_pagado THEN
            RAISE EXCEPTION 'DEUDA_MENOR_A_LO_PAGADO: la deuda de % ya tiene % céntimos pagados', p_periodo, v_pagado USING ERRCODE = 'ED005';
        END IF;
        UPDATE recibo_linea SET monto_cts = p_monto WHERE recibo_id = v_recibo AND tipo = 'deuda_inicial';
        UPDATE recibo SET total_cts = p_monto WHERE id = v_recibo;
        PERFORM recalcular_recibo(v_recibo);
    END IF;
    UPDATE deuda_inicial SET recibo_id = v_recibo WHERE unidad_id = p_unidad AND periodo = p_periodo;
    RETURN v_recibo;
END $$ LANGUAGE plpgsql;

-- Lo que ya se importó antes de esta migración pasa a la cuenta corriente.
SELECT cargar_deuda_inicial(unidad_id, periodo, monto_cts) FROM deuda_inicial ORDER BY id;
