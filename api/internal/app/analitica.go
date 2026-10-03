package app

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"time"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// rangoPeriodos interpreta desde/hasta como AAAA-MM o AAAA-MM-DD. Por defecto, los últimos 6 meses.
func rangoPeriodos(desde, hasta string) (string, string, error) {
	norm := func(s string) string {
		if len(s) >= 7 {
			return s[:7]
		}
		return s
	}
	h := norm(hasta)
	if h == "" {
		h = P.PeriodoActual()
	}
	d := norm(desde)
	if d == "" {
		t, _ := time.Parse("2006-01", h)
		d = t.AddDate(0, -5, 0).Format("2006-01")
	}
	if !P.PeriodoValido(d) || !P.PeriodoValido(h) || d > h {
		return "", "", P.Validacion("Rango inválido: usa desde y hasta como AAAA-MM (desde ≤ hasta).").Campo("desde", "AAAA-MM.")
	}
	return d, h, nil
}

// analitica: GET /analitica/resumen?desde=&hasta=
func (s *Server) analitica(w http.ResponseWriter, r *http.Request) {
	d, h, err := rangoPeriodos(r.URL.Query().Get("desde"), r.URL.Query().Get("hasta"))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	cobranza, err := db.Filas(ctx, s.DB, `SELECT p.periodo,
			COALESCE(sum(r.total_cts),0)::bigint AS emitido,
			COALESCE(sum((SELECT COALESCE(sum(pg.monto_cts),0) FROM pago pg WHERE pg.recibo_id=r.id AND pg.estado='validado')),0)::bigint AS cobrado
		FROM periodo p LEFT JOIN recibo r ON r.periodo_id=p.id AND r.estado NOT IN ('borrador','anulado')
		WHERE p.edificio_id=$1 AND p.periodo BETWEEN $2 AND $3 GROUP BY p.periodo ORDER BY p.periodo`, e.ID, d, h)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	// Morosidad al cierre de cada mes: lo emitido que al último día del mes seguía sin pagar.
	moros, err := db.Filas(ctx, s.DB, `SELECT p.periodo,
			COALESCE(sum(r.total_cts),0)::bigint AS emitido,
			COALESCE(sum((SELECT COALESCE(sum(pg.monto_cts),0) FROM pago pg WHERE pg.recibo_id=r.id AND pg.estado='validado'
				AND pg.fecha <= LEAST((to_date(p.periodo,'YYYY-MM') + interval '1 month - 1 day')::date, (now() AT TIME ZONE 'America/Lima')::date))),0)::bigint AS cobrado_al_cierre
		FROM periodo p LEFT JOIN recibo r ON r.periodo_id=p.id AND r.estado NOT IN ('borrador','anulado')
		WHERE p.edificio_id=$1 AND p.periodo BETWEEN $2 AND $3 GROUP BY p.periodo ORDER BY p.periodo`, e.ID, d, h)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	morosidad := []map[string]any{}
	for _, m := range moros {
		em := m["emitido"].(int64)
		pend := em - m["cobrado_al_cierre"].(int64)
		morosidad = append(morosidad, map[string]any{"periodo": m["periodo"], "pct": pct(pend, em), "pendiente_cts": pend})
	}
	consumo, err := db.Filas(ctx, s.DB, `SELECT u.codigo AS unidad, round(sum(l.consumo)::numeric, 3)::float8 AS m3
		FROM lectura l JOIN periodo p ON p.id=l.periodo_id JOIN medidor m ON m.id=l.medidor_id JOIN unidad u ON u.id=m.unidad_id
		WHERE p.edificio_id=$1 AND p.periodo BETWEEN $2 AND $3 AND m.tipo='agua' GROUP BY u.codigo ORDER BY u.codigo`, e.ID, d, h)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	consumoMes, err := db.Filas(ctx, s.DB, `SELECT p.periodo, round(sum(l.consumo)::numeric, 3)::float8 AS m3,
			(SELECT rg.monto_cts FROM recibo_general rg WHERE rg.periodo_id=p.id AND rg.tipo='agua') AS recibo_general_cts
		FROM lectura l JOIN periodo p ON p.id=l.periodo_id JOIN medidor m ON m.id=l.medidor_id
		WHERE p.edificio_id=$1 AND p.periodo BETWEEN $2 AND $3 AND m.tipo='agua' GROUP BY p.id, p.periodo ORDER BY p.periodo`, e.ID, d, h)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	ini, _ := P.RangoPeriodo(d)
	_, fin := P.RangoPeriodo(h)
	reservas, err := db.Filas(ctx, s.DB, `SELECT a.nombre AS area, count(rv.id) AS cantidad, COALESCE(sum(rv.total_cts),0)::bigint AS ingreso
		FROM area a LEFT JOIN recurso rc ON rc.area_id=a.id
		LEFT JOIN reserva rv ON rv.recurso_id=rc.id AND rv.estado='confirmada' AND rv.inicio >= $2 AND rv.inicio < $3
		WHERE a.edificio_id=$1 GROUP BY a.id, a.nombre ORDER BY cantidad DESC, a.nombre`, e.ID, ini, fin)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	incid, err := db.Filas(ctx, s.DB, `SELECT estado, count(*) AS cantidad FROM incidencia WHERE edificio_id=$1 AND creado_en >= $2 AND creado_en < $3
		GROUP BY estado ORDER BY array_position(ARRAY['reportado','validado','presupuestado','aprobado','en_ejecucion','terminado','rechazado','descartado'], estado)`, e.ID, ini, fin)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	porCat, err := db.Filas(ctx, s.DB, `SELECT categoria, count(*) AS cantidad FROM incidencia WHERE edificio_id=$1 AND creado_en >= $2 AND creado_en < $3
		GROUP BY categoria ORDER BY cantidad DESC`, e.ID, ini, fin)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var tiempo *float64
	_ = s.DB.QueryRow(ctx, `SELECT round((avg(extract(epoch FROM terminado_en - creado_en))/86400)::numeric, 1)::float8 FROM incidencia
		WHERE edificio_id=$1 AND estado='terminado' AND terminado_en >= $2 AND terminado_en < $3`, e.ID, ini, fin).Scan(&tiempo)
	var emitido, cobrado int64
	for _, c := range cobranza {
		emitido += c["emitido"].(int64)
		cobrado += c["cobrado"].(int64)
	}
	var tr any
	if tiempo != nil {
		tr = *tiempo
	}
	P.JSON(w, http.StatusOK, map[string]any{
		"desde": d, "hasta": h,
		"cobranza_mensual":          cobranza,
		"morosidad_mensual":         morosidad,
		"consumo_agua":              consumo,
		"consumo_agua_mensual":      consumoMes,
		"reservas_por_area":         reservas,
		"incidencias_por_estado":    incid,
		"incidencias_por_categoria": porCat,
		"tiempo_resolucion_dias":    tr,
		"totales":                   map[string]any{"emitido_cts": emitido, "cobrado_cts": cobrado, "efectividad_cobranza_pct": pct(cobrado, emitido)},
	})
}

// Tareas corre las tareas programadas del API: liberar retenciones cada minuto y la bandeja de WhatsApp.
func (s *Server) Tareas(ctx context.Context) {
	go s.procesoBandeja(ctx)
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		if n, err := s.liberarRetenciones(ctx); err != nil {
			slog.Warn("liberar retenciones", "err", err)
		} else if n > 0 {
			slog.Info("retenciones liberadas", "cantidad", n)
		}
		s.tareaAvisosCobranza(ctx) // deuda: D2 · avisos de cobranza automáticos (una vez al día por aviso)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

var _ = math.Round
