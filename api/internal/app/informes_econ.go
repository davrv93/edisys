package app

import (
	"net/http"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Informes económicos (C4) y consumos por departamento (C3). Salen de las mismas fuentes del
// balance y de las lecturas: no se recalcula nada por otra vía.

// informeEconomico: GET /informes/economico?periodo=AAAA-MM
// Devuelve el resumen del periodo, el flujo de los últimos 12 meses y los pagos del periodo.
func (s *Server) informeEconomico(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	periodo, err := s.periodoDe(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	ctx := r.Context()
	a, err := s.ArbolBalance(ctx, s.DB, e.ID, periodo, vistaCompleta())
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	flujo, err := db.Filas(ctx, s.DB, `WITH meses AS (
			SELECT to_char(m, 'YYYY-MM') AS periodo FROM generate_series(date_trunc('month', to_date($2,'YYYY-MM')) - interval '11 months', date_trunc('month', to_date($2,'YYYY-MM')), interval '1 month') m
		)
		SELECT me.periodo,
			COALESCE((SELECT SUM(p.monto_cts) FROM pago p WHERE p.edificio_id=$1 AND p.estado='validado' AND to_char(p.fecha,'YYYY-MM')=me.periodo),0)::bigint AS ingreso_cts,
			COALESCE((SELECT SUM(eg.monto_cts) FROM egreso eg WHERE eg.edificio_id=$1 AND eg.periodo=me.periodo),0)::bigint AS egreso_cts
		FROM meses me ORDER BY me.periodo`, e.ID, periodo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	pagos, err := db.Filas(ctx, s.DB, `SELECT pg.id, u.codigo AS unidad, pg.monto_cts, pg.medio, pg.estado, to_char(pg.fecha,'YYYY-MM-DD') AS fecha, pg.codigo_operacion
		FROM pago pg JOIN recibo rc ON rc.id=pg.recibo_id JOIN unidad u ON u.id=rc.unidad_id
		WHERE pg.edificio_id=$1 AND to_char(pg.fecha,'YYYY-MM')=$2 ORDER BY pg.fecha, pg.id`, e.ID, periodo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	k := a.KPIs
	P.JSON(w, http.StatusOK, map[string]any{
		"periodo": periodo,
		"resumen": map[string]any{
			"ingresos_cts": k.IngresosCts, "egresos_cts": k.EgresosCts, "saldo_cts": k.SaldoCts,
			"banco_cts": k.BancoCts, "emitido_cts": k.EmitidoCts,
		},
		"flujo": flujo, "pagos": pagos,
	})
}

// consumosPorDepartamento: GET /informes/consumos?hasta=AAAA-MM&meses=12
// Matriz departamento × periodo con el consumo de agua (m³) de cada lectura.
func (s *Server) consumosPorDepartamento(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	hasta := r.URL.Query().Get("hasta")
	if hasta == "" {
		hasta = P.PeriodoActual()
	}
	if !P.PeriodoValido(hasta) {
		P.Fallo(w, r, P.Validacion("El periodo debe ser AAAA-MM.").Campo("hasta", "Formato AAAA-MM."))
		return
	}
	filas, err := db.Filas(ctx, s.DB, `SELECT u.codigo AS unidad, p.periodo, l.consumo::float8 AS consumo
		FROM lectura l
		JOIN medidor m ON m.id=l.medidor_id
		JOIN unidad u ON u.id=m.unidad_id
		JOIN periodo p ON p.id=l.periodo_id
		WHERE m.edificio_id=$1 AND m.tipo='agua' AND p.periodo <= $2
		  AND p.periodo > to_char(date_trunc('month', to_date($2,'YYYY-MM')) - interval '12 months', 'YYYY-MM')
		ORDER BY u.codigo, p.periodo`, e.ID, hasta)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	// Pivote: periodos en orden y una fila por unidad.
	idxPeriodo := map[string]int{}
	var periodos []string
	porUnidad := map[string][]any{}
	var orden []string
	for _, f := range filas {
		per := f["periodo"].(string)
		if _, ok := idxPeriodo[per]; !ok {
			idxPeriodo[per] = len(periodos)
			periodos = append(periodos, per)
		}
		u := f["unidad"].(string)
		if _, ok := porUnidad[u]; !ok {
			porUnidad[u] = make([]any, len(periodos))
			orden = append(orden, u)
		}
	}
	// Completar filas al ancho final (los periodos pueden aparecer después de la primera unidad).
	for _, u := range orden {
		if len(porUnidad[u]) < len(periodos) {
			nueva := make([]any, len(periodos))
			copy(nueva, porUnidad[u])
			porUnidad[u] = nueva
		}
	}
	for _, f := range filas {
		u := f["unidad"].(string)
		porUnidad[u][idxPeriodo[f["periodo"].(string)]] = f["consumo"]
	}
	unidades := make([]map[string]any, 0, len(orden))
	for _, u := range orden {
		unidades = append(unidades, map[string]any{"unidad": u, "consumos": porUnidad[u]})
	}
	P.JSON(w, http.StatusOK, map[string]any{"hasta": hasta, "periodos": periodos, "unidades": unidades})
}
