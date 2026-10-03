package app

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Gestión de deuda, parte de consultas: cuentas por cobrar y estado de cuenta (B3), grillas de morosos
// y puntualidad (D3) y la configuración de deuda del edificio. Nada de esto escribe dinero.

// hoyLima: la fecha de hoy en Lima (el servidor corre en UTC).
func hoyLima() time.Time {
	n := time.Now().In(P.Lima)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// DeudaConfig: etiquetas visibles y tasa de mora del edificio (valores por defecto si no hay fila).
type DeudaConfig struct {
	EtiquetaMoroso  string `json:"etiqueta_moroso"`
	EtiquetaPuntual string `json:"etiqueta_puntual"`
	TasaMoraBP      int    `json:"tasa_mora_bp"`
}

func (s *Server) deudaConfig(ctx context.Context, q db.Q, eid int64) DeudaConfig {
	c := DeudaConfig{EtiquetaMoroso: "Moroso", EtiquetaPuntual: "Puntual"}
	_ = q.QueryRow(ctx, `SELECT etiqueta_moroso, etiqueta_puntual, tasa_mora_bp FROM deuda_config WHERE edificio_id=$1`, eid).
		Scan(&c.EtiquetaMoroso, &c.EtiquetaPuntual, &c.TasaMoraBP)
	return c
}

// verDeudaConfig: GET /deuda/config
func (s *Server) verDeudaConfig(w http.ResponseWriter, r *http.Request) {
	P.JSON(w, http.StatusOK, s.deudaConfig(r.Context(), s.DB, edf(r).ID))
}

// guardarDeudaConfig: PUT /deuda/config {etiqueta_moroso, etiqueta_puntual, tasa_mora_bp}
func (s *Server) guardarDeudaConfig(w http.ResponseWriter, r *http.Request) {
	var in DeudaConfig
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.EtiquetaMoroso, in.EtiquetaPuntual = strings.TrimSpace(in.EtiquetaMoroso), strings.TrimSpace(in.EtiquetaPuntual)
	ev := P.Validacion("Revisa la configuración.")
	if in.EtiquetaMoroso == "" {
		ev.Campo("etiqueta_moroso", "Obligatorio.")
	}
	if in.EtiquetaPuntual == "" {
		ev.Campo("etiqueta_puntual", "Obligatorio.")
	}
	if in.TasaMoraBP < 0 || in.TasaMoraBP > 10000 {
		ev.Campo("tasa_mora_bp", "Entre 0 y 10 000 puntos básicos (0 % a 100 % al mes).")
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	e := edf(r)
	ctx := r.Context()
	antes := s.deudaConfig(ctx, s.DB, e.ID)
	if _, err := s.DB.Exec(ctx, `INSERT INTO deuda_config (edificio_id, etiqueta_moroso, etiqueta_puntual, tasa_mora_bp) VALUES ($1,$2,$3,$4)
		ON CONFLICT (edificio_id) DO UPDATE SET etiqueta_moroso=$2, etiqueta_puntual=$3, tasa_mora_bp=$4, actualizado_en=now()`,
		e.ID, in.EtiquetaMoroso, in.EtiquetaPuntual, in.TasaMoraBP); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "deuda", "configurar", "deuda_config", e.ID, antes, in)
	P.JSON(w, http.StatusOK, in)
}

// ---------- B3 · cuentas por cobrar ----------

// cuentasPorCobrar: GET /cuentas-por-cobrar[?formato=csv] → por unidad: deuda, por vencer, vencido por antigüedad
// (días desde el vencimiento del recibo), deuda vencida según la morosidad (con gracia y acuerdos) y acuerdo activo.
func (s *Server) cuentasPorCobrar(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	filas, err := db.Filas(ctx, s.DB, `WITH hoy AS (SELECT (now() AT TIME ZONE 'America/Lima')::date AS d),
		pend AS (
			SELECT r.unidad_id, r.total_cts - r.pagado_cts AS saldo,
			       CASE WHEN r.vence IS NULL OR r.vence >= hoy.d THEN NULL ELSE hoy.d - r.vence END AS dias
			FROM recibo r, hoy
			WHERE r.edificio_id=$1 AND r.estado IN ('emitido','pagado_parcial') AND r.total_cts > r.pagado_cts)
		SELECT u.id AS unidad_id, u.codigo AS unidad,
			COALESCE((SELECT pe.nombre FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id WHERE up.unidad_id=u.id AND up.rol='propietario' AND up.hasta IS NULL LIMIT 1),'') AS propietario,
			COALESCE(sum(p.saldo),0)::bigint AS deuda_cts,
			COALESCE(sum(p.saldo) FILTER (WHERE p.dias IS NULL),0)::bigint AS por_vencer_cts,
			COALESCE(sum(p.saldo) FILTER (WHERE p.dias BETWEEN 1 AND 30),0)::bigint AS d1_30_cts,
			COALESCE(sum(p.saldo) FILTER (WHERE p.dias BETWEEN 31 AND 60),0)::bigint AS d31_60_cts,
			COALESCE(sum(p.saldo) FILTER (WHERE p.dias BETWEEN 61 AND 90),0)::bigint AS d61_90_cts,
			COALESCE(sum(p.saldo) FILTER (WHERE p.dias > 90),0)::bigint AS d90_mas_cts,
			COALESCE(max(p.dias),0)::int AS antiguedad_dias,
			deuda_vencida_cts(u.id) AS deuda_vencida_cts,
			es_moroso(u.id) AS moroso,
			(SELECT a.numero FROM acuerdo_pago a WHERE a.unidad_id=u.id AND a.estado='activo' ORDER BY a.id DESC LIMIT 1) AS acuerdo
		FROM unidad u JOIN pend p ON p.unidad_id=u.id
		WHERE u.edificio_id=$1
		GROUP BY u.id ORDER BY deuda_cts DESC, u.codigo`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	tot := map[string]int64{}
	claves := []string{"deuda_cts", "por_vencer_cts", "d1_30_cts", "d31_60_cts", "d61_90_cts", "d90_mas_cts", "deuda_vencida_cts"}
	for _, f := range filas {
		for _, k := range claves {
			tot[k] += f[k].(int64)
		}
	}
	if r.URL.Query().Get("formato") == "csv" {
		cab := []string{"Unidad", "Propietario", "Deuda", "Por vencer", "1-30 días", "31-60 días", "61-90 días", "Más de 90", "Deuda vencida", "Acuerdo"}
		datos := [][]string{}
		for _, f := range filas {
			fila := []string{fmt.Sprint(f["unidad"]), fmt.Sprint(f["propietario"])}
			for _, k := range claves[:6] {
				fila = append(fila, P.Soles(f[k].(int64)))
			}
			fila = append(fila, P.Soles(f["deuda_vencida_cts"].(int64)), val(f["acuerdo"]))
			datos = append(datos, fila)
		}
		escribirCSV(w, "cuentas-por-cobrar.csv", cab, datos)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "totales": tot, "fecha": hoyLima().Format("2006-01-02")})
}

// escribirCSV responde un CSV con BOM (Excel lo abre con tildes) y separador coma.
func escribirCSV(w http.ResponseWriter, nombre string, cab []string, filas [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+nombre+`"`)
	_, _ = w.Write([]byte("\xEF\xBB\xBF"))
	cw := csv.NewWriter(w)
	_ = cw.Write(cab)
	_ = cw.WriteAll(filas)
}

// ---------- B3 · estado de cuenta ----------

// Movimiento del estado de cuenta: un cargo (recibo, recargo/descuento de acuerdo) o un abono (pago validado).
type Movimiento struct {
	Fecha      string `json:"fecha"`
	Tipo       string `json:"tipo"` // cargo | abono
	Concepto   string `json:"concepto"`
	Referencia string `json:"referencia"`
	CargoCts   int64  `json:"cargo_cts"`
	AbonoCts   int64  `json:"abono_cts"`
	SaldoCts   int64  `json:"saldo_cts"`
	ReciboID   *int64 `json:"recibo_id,omitempty"`
	orden      int
	id         int64
}

// EstadoCuenta ordena los movimientos por fecha (cargos antes que abonos el mismo día), calcula el saldo
// corrido y separa el saldo inicial (lo anterior a «desde»). desde/hasta vacíos = sin límite.
func EstadoCuenta(movs []Movimiento, desde, hasta string) (inicial int64, visibles []Movimiento, final int64) {
	sort.SliceStable(movs, func(i, j int) bool {
		if movs[i].Fecha != movs[j].Fecha {
			return movs[i].Fecha < movs[j].Fecha
		}
		if movs[i].orden != movs[j].orden {
			return movs[i].orden < movs[j].orden
		}
		return movs[i].id < movs[j].id
	})
	saldo := int64(0)
	visibles = []Movimiento{}
	for _, m := range movs {
		if hasta != "" && m.Fecha > hasta {
			continue
		}
		saldo += m.CargoCts - m.AbonoCts
		if desde != "" && m.Fecha < desde {
			inicial = saldo
			continue
		}
		m.SaldoCts = saldo
		visibles = append(visibles, m)
	}
	return inicial, visibles, saldo
}

// estadoCuenta: GET /unidades/{uid}/estado-cuenta?desde=&hasta=[&formato=csv]
// El propietario solo ve sus unidades. El saldo final cuadra con la cuenta corriente (Σ total − pagado).
func (s *Server) estadoCuenta(w http.ResponseWriter, r *http.Request) {
	uid, err := idRuta(r, "uid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	if e.SoloLoSuyo() && !e.EsSuya(uid) {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	ctx := r.Context()
	desde, hasta := strings.TrimSpace(r.URL.Query().Get("desde")), strings.TrimSpace(r.URL.Query().Get("hasta"))
	ev := P.Validacion("Revisa el rango de fechas.")
	fechaOpc(desde, ev, "desde")
	fechaOpc(hasta, ev, "hasta")
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	cab, err := db.Fila(ctx, s.DB, `SELECT u.id AS unidad_id, u.codigo AS unidad,
			COALESCE((SELECT pe.nombre FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id WHERE up.unidad_id=u.id AND up.rol='propietario' AND up.hasta IS NULL LIMIT 1),'') AS propietario,
			deuda_vencida_cts(u.id) AS deuda_vencida_cts, es_moroso(u.id) AS moroso
		FROM unidad u WHERE u.id=$1 AND u.edificio_id=$2`, uid, e.ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	movs := []Movimiento{}
	// Cargos: el recibo por lo que se emitió (sin los ajustes de acuerdos, que van aparte con su fecha).
	fr, err := s.DB.Query(ctx, `SELECT r.id, COALESCE(r.numero,''), p.periodo, r.origen,
			to_char(COALESCE((r.emitido_en AT TIME ZONE 'America/Lima')::date, (p.periodo||'-01')::date),'YYYY-MM-DD'),
			r.total_cts - COALESCE((SELECT sum(l.monto_cts) FROM recibo_linea l WHERE l.recibo_id=r.id AND l.acuerdo_id IS NOT NULL),0)
		FROM recibo r JOIN periodo p ON p.id=r.periodo_id
		WHERE r.unidad_id=$1 AND r.edificio_id=$2 AND r.estado NOT IN ('borrador','anulado')`, uid, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for fr.Next() {
		var m Movimiento
		var rid int64
		var numero, per, origen string
		if err := fr.Scan(&rid, &numero, &per, &origen, &m.Fecha, &m.CargoCts); err != nil {
			fr.Close()
			P.Fallo(w, r, err)
			return
		}
		m.Tipo, m.Referencia, m.ReciboID, m.id = "cargo", numero, &rid, rid
		m.Concepto = "Recibo " + P.NombrePeriodo(per)
		if origen == "deuda_inicial" {
			m.Concepto = "Deuda anterior " + P.NombrePeriodo(per)
		}
		movs = append(movs, m)
	}
	fr.Close()
	// Recargos y descuentos de acuerdos de pago (líneas «ajuste» marcadas con el acuerdo).
	fa, err := s.DB.Query(ctx, `SELECT l.id, l.recibo_id, l.descripcion, l.monto_cts, a.numero, to_char(a.fecha,'YYYY-MM-DD')
		FROM recibo_linea l JOIN recibo r ON r.id=l.recibo_id JOIN acuerdo_pago a ON a.id=l.acuerdo_id
		WHERE r.unidad_id=$1 AND r.edificio_id=$2 AND r.estado NOT IN ('borrador','anulado')`, uid, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for fa.Next() {
		var m Movimiento
		var lid, rid, monto int64
		if err := fa.Scan(&lid, &rid, &m.Concepto, &monto, &m.Referencia, &m.Fecha); err != nil {
			fa.Close()
			P.Fallo(w, r, err)
			return
		}
		m.Tipo, m.ReciboID, m.id, m.orden = "cargo", &rid, lid, 1
		if monto >= 0 {
			m.CargoCts = monto
		} else {
			m.Tipo, m.AbonoCts = "abono", -monto
		}
		movs = append(movs, m)
	}
	fa.Close()
	// Abonos: pagos validados de recibos vigentes.
	fp, err := s.DB.Query(ctx, `SELECT pg.id, pg.recibo_id, to_char(pg.fecha,'YYYY-MM-DD'), pg.medio, COALESCE(pg.codigo_operacion,''), pg.monto_cts, COALESCE(r.numero,'')
		FROM pago pg JOIN recibo r ON r.id=pg.recibo_id
		WHERE r.unidad_id=$1 AND r.edificio_id=$2 AND pg.estado='validado' AND r.estado NOT IN ('borrador','anulado')`, uid, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for fp.Next() {
		var m Movimiento
		var pid, rid int64
		var medio, cod, numero string
		if err := fp.Scan(&pid, &rid, &m.Fecha, &medio, &cod, &m.AbonoCts, &numero); err != nil {
			fp.Close()
			P.Fallo(w, r, err)
			return
		}
		m.Tipo, m.ReciboID, m.id, m.orden = "abono", &rid, pid, 2
		m.Concepto = "Pago " + medio + " · recibo " + numero
		m.Referencia = cod
		movs = append(movs, m)
	}
	fp.Close()

	inicial, visibles, final := EstadoCuenta(movs, desde, hasta)
	var cargos, abonos int64
	for _, m := range visibles {
		cargos += m.CargoCts
		abonos += m.AbonoCts
	}
	acuerdos, err := db.Filas(ctx, s.DB, `SELECT a.id, a.numero, to_char(a.fecha,'YYYY-MM-DD') AS fecha, a.monto_acordado_cts, a.n_cuotas,
			acuerdo_avance_cts(a.id) AS avance_cts, acuerdo_vencido_cts(a.id) AS vencido_cts,
			(SELECT to_char(min(c.vence),'YYYY-MM-DD') FROM acuerdo_cuota c WHERE c.acuerdo_id=a.id
			   AND (SELECT COALESCE(sum(c2.monto_cts),0) FROM acuerdo_cuota c2 WHERE c2.acuerdo_id=a.id AND c2.numero<=c.numero) > acuerdo_avance_cts(a.id)) AS proxima_cuota
		FROM acuerdo_pago a WHERE a.unidad_id=$1 AND a.edificio_id=$2 AND a.estado='activo' ORDER BY a.id`, uid, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if r.URL.Query().Get("formato") == "csv" {
		datos := [][]string{{"", "Saldo inicial", "", "", "", P.Soles(inicial)}}
		for _, m := range visibles {
			datos = append(datos, []string{m.Fecha, m.Concepto, m.Referencia, P.Soles(m.CargoCts), P.Soles(m.AbonoCts), P.Soles(m.SaldoCts)})
		}
		escribirCSV(w, "estado-cuenta-"+fmt.Sprint(cab["unidad"])+".csv", []string{"Fecha", "Concepto", "Referencia", "Cargo", "Abono", "Saldo"}, datos)
		return
	}
	cab["desde"], cab["hasta"] = desde, hasta
	cab["saldo_inicial_cts"], cab["saldo_final_cts"] = inicial, final
	cab["cargos_cts"], cab["abonos_cts"] = cargos, abonos
	cab["movimientos"] = visibles
	cab["acuerdos"] = acuerdos
	P.JSON(w, http.StatusOK, cab)
}

// ---------- D3 · morosos, morosos detallado y puntualidad ----------

// Estados de una celda (unidad × mes):
//
//	puntual   → pagado completo hasta el vencimiento
//	en_gracia → pagado completo después del vencimiento pero dentro de los días de gracia
//	tardio    → pagado completo después de la gracia (fue moroso y regularizó; se ve la fecha)
//	moroso    → sin pagar completo y ya pasó la gracia
//	pendiente → sin pagar completo, todavía en plazo
func EstadoCelda(total int64, vence, completo *time.Time, gracia int, hoy time.Time) string {
	if total == 0 {
		return "puntual"
	}
	if vence == nil {
		if completo != nil {
			return "puntual"
		}
		return "pendiente"
	}
	limite := vence.AddDate(0, 0, gracia)
	if completo != nil {
		switch {
		case !completo.After(*vence):
			return "puntual"
		case !completo.After(limite):
			return "en_gracia"
		default:
			return "tardio"
		}
	}
	if hoy.After(limite) {
		return "moroso"
	}
	return "pendiente"
}

// grillaMorosos: GET /morosos?desde=AAAA-MM&hasta=AAAA-MM → grilla Departamento × mes con el estado de cada
// recibo del periodo, el monto, la fecha en que se completó el pago y los días de atraso. Una sola consulta
// alimenta las tres vistas (Morosos, Morosos detallado y Puntualidad).
func (s *Server) grillaMorosos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	desde, hasta := r.URL.Query().Get("desde"), r.URL.Query().Get("hasta")
	if (desde != "" && !P.PeriodoValido(desde)) || (hasta != "" && !P.PeriodoValido(hasta)) {
		P.Fallo(w, r, P.Validacion("Periodo inválido.").Campo("desde", "Formato AAAA-MM."))
		return
	}
	// Por defecto, los últimos 12 periodos no históricos del edificio.
	var meses []string
	fm, err := s.DB.Query(ctx, `SELECT periodo FROM (SELECT periodo FROM periodo WHERE edificio_id=$1 AND NOT historico
			AND ($2='' OR periodo >= $2) AND ($3='' OR periodo <= $3) ORDER BY periodo DESC LIMIT 24) x ORDER BY periodo`, e.ID, desde, hasta)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for fm.Next() {
		var m string
		if fm.Scan(&m) == nil {
			meses = append(meses, m)
		}
	}
	fm.Close()
	if desde == "" && hasta == "" && len(meses) > 12 {
		meses = meses[len(meses)-12:]
	}
	if meses == nil {
		meses = []string{}
	}
	var gracia int
	_ = s.DB.QueryRow(ctx, `SELECT dias_gracia FROM edificio WHERE id=$1`, e.ID).Scan(&gracia)
	cfg := s.deudaConfig(ctx, s.DB, e.ID)

	celdas, err := s.DB.Query(ctx, `WITH pagos AS (
			SELECT pg.recibo_id, pg.fecha, sum(pg.monto_cts) OVER (PARTITION BY pg.recibo_id ORDER BY pg.fecha, pg.id) AS acum
			FROM pago pg WHERE pg.edificio_id=$1 AND pg.estado='validado')
		SELECT r.unidad_id, p.periodo, r.id, r.total_cts, r.total_cts - r.pagado_cts, r.vence,
			(SELECT min(pa.fecha) FROM pagos pa WHERE pa.recibo_id=r.id AND pa.acum >= r.total_cts)
		FROM recibo r JOIN periodo p ON p.id=r.periodo_id
		WHERE r.edificio_id=$1 AND r.origen='periodo' AND r.estado NOT IN ('borrador','anulado') AND p.periodo = ANY($2)`, e.ID, meses)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	hoy := hoyLima()
	porUnidad := map[int64]map[string]any{}
	for celdas.Next() {
		var uid, rid, total, saldo int64
		var per string
		var vence, completo *time.Time
		if err := celdas.Scan(&uid, &per, &rid, &total, &saldo, &vence, &completo); err != nil {
			celdas.Close()
			P.Fallo(w, r, err)
			return
		}
		est := EstadoCelda(total, vence, completo, gracia, hoy)
		c := map[string]any{"recibo_id": rid, "estado": est, "total_cts": total, "saldo_cts": saldo, "vence": nil, "pagado_en": nil, "dias_atraso": 0}
		if vence != nil {
			c["vence"] = vence.Format("2006-01-02")
			fin := hoy
			if completo != nil {
				fin = *completo
			}
			if d := int(fin.Sub(*vence).Hours() / 24); d > 0 {
				c["dias_atraso"] = d
			}
		}
		if completo != nil {
			c["pagado_en"] = completo.Format("2006-01-02")
		}
		if porUnidad[uid] == nil {
			porUnidad[uid] = map[string]any{}
		}
		porUnidad[uid][per] = c
	}
	celdas.Close()

	unidades, err := db.Filas(ctx, s.DB, `SELECT u.id AS unidad_id, u.codigo AS unidad,
			COALESCE((SELECT pe.nombre FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id WHERE up.unidad_id=u.id AND up.rol='propietario' AND up.hasta IS NULL LIMIT 1),'') AS propietario
		FROM unidad u WHERE u.edificio_id=$1 ORDER BY u.codigo`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	resMes := map[string]map[string]int64{}
	for _, m := range meses {
		resMes[m] = map[string]int64{"morosos": 0, "moroso_cts": 0, "puntuales": 0, "recibos": 0}
	}
	for _, u := range unidades {
		cs := porUnidad[u["unidad_id"].(int64)]
		if cs == nil {
			cs = map[string]any{}
		}
		var morosos, puntuales, recibos int
		for per, v := range cs {
			c := v.(map[string]any)
			recibos++
			rm := resMes[per]
			rm["recibos"]++
			switch c["estado"] {
			case "moroso":
				morosos++
				rm["morosos"]++
				rm["moroso_cts"] += c["saldo_cts"].(int64)
			case "puntual":
				puntuales++
				rm["puntuales"]++
			}
		}
		u["celdas"] = cs
		u["meses_morosos"], u["meses_puntuales"], u["recibos"] = morosos, puntuales, recibos
		pct := 0.0
		if recibos > 0 {
			pct = float64(int(1000*float64(puntuales)/float64(recibos)+0.5)) / 10
		}
		u["puntualidad_pct"] = pct
	}
	P.JSON(w, http.StatusOK, map[string]any{"meses": meses, "unidades": unidades, "por_mes": resMes, "dias_gracia": gracia,
		"etiquetas": map[string]string{"moroso": cfg.EtiquetaMoroso, "puntual": cfg.EtiquetaPuntual}, "fecha": hoy.Format("2006-01-02")})
}
