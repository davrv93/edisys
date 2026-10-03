package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Acuerdos de pago (bloque D1). El acuerdo toma recibos vencidos de una unidad, les aplica recargo o descuento
// y divide el monto acordado en cuotas mensuales. No mueve dinero: los recibos siguen siendo la deuda y el pago
// se aplica como siempre (del más antiguo al más nuevo); el acuerdo mide su avance por lo cobrado en ellos.
// Mientras está activo, la morosidad de la unidad solo cuenta las cuotas vencidas e impagas (0024), así que un
// acuerdo al día habilita reservas. La firma digital está fuera de alcance: se sube el documento firmado.

// Cuota de un acuerdo (calendario).
type Cuota struct {
	Numero   int    `json:"numero"`
	Vence    string `json:"vence"`
	MontoCts int64  `json:"monto_cts"`
}

// DividirCuotas reparte el monto en n cuotas mensuales que suman exacto al céntimo: el resto de la división
// va de a un céntimo en las primeras. La primera vence en «primera»; las siguientes, el mismo día de cada mes
// (o el último día si el mes es más corto).
func DividirCuotas(monto int64, n int, primera time.Time) []Cuota {
	if n <= 0 || monto <= 0 {
		return []Cuota{}
	}
	base, resto := monto/int64(n), monto%int64(n)
	out := make([]Cuota, n)
	for i := 0; i < n; i++ {
		m := base
		if int64(i) < resto {
			m++
		}
		out[i] = Cuota{Numero: i + 1, Vence: sumarMeses(primera, i).Format("2006-01-02"), MontoCts: m}
	}
	return out
}

// sumarMeses suma meses conservando el día, recortado al último día del mes destino.
func sumarMeses(t time.Time, n int) time.Time {
	primero := time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	ultimo := primero.AddDate(0, 1, -1).Day()
	d := t.Day()
	if d > ultimo {
		d = ultimo
	}
	return time.Date(primero.Year(), primero.Month(), d, 0, 0, 0, 0, time.UTC)
}

// MoraCts: mora sugerida de un saldo vencido = saldo × tasa mensual × meses de atraso (mes iniciado cuenta),
// redondeada al céntimo. tasaBP en puntos básicos (100 = 1 %).
func MoraCts(saldo int64, tasaBP int, vence, hoy time.Time) int64 {
	if tasaBP <= 0 || saldo <= 0 || !hoy.After(vence) {
		return 0
	}
	dias := int64(hoy.Sub(vence).Hours() / 24)
	meses := (dias + 29) / 30
	return (saldo*int64(tasaBP)*meses + 5000) / 10000
}

// EstadoCuotas marca cada cuota según el avance (lo cobrado desde la firma), en orden:
// pagada si el acumulado hasta ella está cubierto; parcial si el avance cae dentro; vencida si pasó su
// vencimiento más la gracia sin cubrirse; pendiente en otro caso.
func EstadoCuotas(cuotas []Cuota, avance int64, gracia int, hoy time.Time) []map[string]any {
	out := make([]map[string]any, 0, len(cuotas))
	acum := int64(0)
	for _, c := range cuotas {
		antes := acum
		acum += c.MontoCts
		pagado := avance - antes
		if pagado < 0 {
			pagado = 0
		}
		if pagado > c.MontoCts {
			pagado = c.MontoCts
		}
		estado := "pendiente"
		v, _ := time.Parse("2006-01-02", c.Vence)
		switch {
		case pagado == c.MontoCts:
			estado = "pagado"
		case hoy.After(v.AddDate(0, 0, gracia)):
			estado = "vencido"
		case pagado > 0:
			estado = "parcial"
		}
		out = append(out, map[string]any{"numero": c.Numero, "vence": c.Vence, "monto_cts": c.MontoCts, "pagado_cts": pagado, "estado": estado})
	}
	return out
}

// recibosVencidosLibres: recibos de la unidad con saldo, vencidos (sin gracia) y fuera de un acuerdo activo.
func (s *Server) recibosVencidosLibres(ctx context.Context, q db.Q, eid, uid int64, forUpdate bool) ([]map[string]any, error) {
	sql := `SELECT r.id AS recibo_id, COALESCE(r.numero,'') AS numero, p.periodo, r.origen, r.total_cts, r.pagado_cts,
			r.total_cts - r.pagado_cts AS saldo_cts, r.vence
		FROM recibo r JOIN periodo p ON p.id=r.periodo_id
		WHERE r.unidad_id=$1 AND r.edificio_id=$2 AND r.estado IN ('emitido','pagado_parcial') AND r.total_cts > r.pagado_cts
		  AND r.vence IS NOT NULL AND r.vence < (now() AT TIME ZONE 'America/Lima')::date
		  AND NOT EXISTS (SELECT 1 FROM acuerdo_recibo ar JOIN acuerdo_pago a ON a.id=ar.acuerdo_id WHERE ar.recibo_id=r.id AND ar.activo AND a.estado='activo')
		ORDER BY r.vence, p.periodo, r.id`
	if forUpdate {
		sql += ` FOR UPDATE OF r`
	}
	return db.Filas(ctx, q, sql, uid, eid)
}

// propuestaAcuerdo: GET /acuerdos/propuesta?unidad_id= → recibos vencidos con Total, Saldo y Mora calculada.
func (s *Server) propuestaAcuerdo(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	uid, _ := strconv.ParseInt(r.URL.Query().Get("unidad_id"), 10, 64)
	var codigo string
	if err := s.DB.QueryRow(ctx, `SELECT codigo FROM unidad WHERE id=$1 AND edificio_id=$2`, uid, e.ID).Scan(&codigo); err != nil {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	recibos, err := s.recibosVencidosLibres(ctx, s.DB, e.ID, uid, false)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	cfg := s.deudaConfig(ctx, s.DB, e.ID)
	hoy := hoyLima()
	var total, saldo, mora int64
	for _, rc := range recibos {
		v := rc["vence"].(time.Time)
		m := MoraCts(rc["saldo_cts"].(int64), cfg.TasaMoraBP, v, hoy)
		rc["mora_cts"] = m
		rc["vence"] = v.Format("2006-01-02")
		total += rc["total_cts"].(int64)
		saldo += rc["saldo_cts"].(int64)
		mora += m
	}
	P.JSON(w, http.StatusOK, map[string]any{"unidad_id": uid, "unidad": codigo, "recibos": recibos, "tasa_mora_bp": cfg.TasaMoraBP,
		"total_cts": total, "saldo_cts": saldo, "mora_cts": mora})
}

type entradaAcuerdo struct {
	UnidadID     int64   `json:"unidad_id"`
	ReciboIDs    []int64 `json:"recibo_ids"`
	RecargoCts   int64   `json:"recargo_cts"`
	DescuentoCts int64   `json:"descuento_cts"`
	NCuotas      int     `json:"n_cuotas"`
	PrimeraCuota string  `json:"primera_cuota"`
	Fecha        string  `json:"fecha"`
	Comentario   string  `json:"comentario"`
	AceptadoPor  string  `json:"aceptado_por"`
}

// crearAcuerdo: POST /acuerdos → 201 {id, numero, monto_acordado_cts, cuotas}.
// Sin recibo_ids toma todos los vencidos libres de la unidad. Invariante: Σ cuotas = monto acordado =
// saldo + recargo − descuento = Σ saldo de los recibos del acuerdo tras aplicar recargo y descuento.
func (s *Server) crearAcuerdo(w http.ResponseWriter, r *http.Request) {
	var in entradaAcuerdo
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	in.AceptadoPor, in.Comentario = strings.TrimSpace(in.AceptadoPor), strings.TrimSpace(in.Comentario)
	hoy := hoyLima()
	ev := P.Validacion("Revisa el acuerdo de pago.")
	if in.AceptadoPor == "" {
		ev.Campo("aceptado_por", "Escribe quién acepta el acuerdo (el deudor).")
	}
	if in.NCuotas < 1 || in.NCuotas > 60 {
		ev.Campo("n_cuotas", "Entre 1 y 60 cuotas.")
	}
	if in.RecargoCts < 0 {
		ev.Campo("recargo_cts", "No puede ser negativo.")
	}
	if in.DescuentoCts < 0 {
		ev.Campo("descuento_cts", "No puede ser negativo.")
	}
	fecha := hoy
	if f := fechaOpc(in.Fecha, ev, "fecha"); f != nil {
		fecha = *f
	}
	primera := sumarMeses(fecha, 1)
	if f := fechaOpc(in.PrimeraCuota, ev, "primera_cuota"); f != nil {
		primera = *f
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var codigo string
	// Bloquea la unidad: dos acuerdos simultáneos de la misma unidad se serializan aquí.
	if err := tx.QueryRow(ctx, `SELECT codigo FROM unidad WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, in.UnidadID, e.ID).Scan(&codigo); err != nil {
		P.Fallo(w, r, P.Validacion("Elige una unidad del edificio.").Campo("unidad_id", "Obligatorio."))
		return
	}
	libres, err := s.recibosVencidosLibres(ctx, tx, e.ID, in.UnidadID, true)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	elegidos := libres
	if len(in.ReciboIDs) > 0 {
		porID := map[int64]map[string]any{}
		for _, rc := range libres {
			porID[rc["recibo_id"].(int64)] = rc
		}
		elegidos = nil
		vistos := map[int64]bool{}
		for _, id := range in.ReciboIDs {
			rc, ok := porID[id]
			if !ok {
				P.Fallo(w, r, P.Validacion("Ese recibo no se puede incluir.").Campo("recibo_ids",
					fmt.Sprintf("El recibo %d no es un recibo vencido con saldo de la unidad, o ya está en otro acuerdo.", id)))
				return
			}
			if !vistos[id] {
				vistos[id] = true
				elegidos = append(elegidos, rc)
			}
		}
	}
	if len(elegidos) == 0 {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "SIN_DEUDA_VENCIDA", "La unidad no tiene recibos vencidos libres para un acuerdo."))
		return
	}
	var saldo, pagadoIni int64
	cfg := s.deudaConfig(ctx, tx, e.ID)
	var mora int64
	for _, rc := range elegidos {
		saldo += rc["saldo_cts"].(int64)
		pagadoIni += rc["pagado_cts"].(int64)
		mora += MoraCts(rc["saldo_cts"].(int64), cfg.TasaMoraBP, rc["vence"].(time.Time), hoy)
	}
	monto := saldo + in.RecargoCts - in.DescuentoCts
	if monto <= 0 {
		P.Fallo(w, r, P.Validacion("El descuento no puede dejar el acuerdo en cero.").Campo("descuento_cts", "Máximo "+P.Soles(saldo+in.RecargoCts-1)+"."))
		return
	}
	var n int
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM acuerdo_pago WHERE edificio_id=$1`, e.ID).Scan(&n)
	numero := fmt.Sprintf("AP-%04d", n+1)
	se := ses(r)
	var aid int64
	if err := tx.QueryRow(ctx, `INSERT INTO acuerdo_pago (edificio_id, unidad_id, numero, comentario, aceptado_por, fecha, saldo_cts, mora_cts,
			recargo_cts, descuento_cts, monto_acordado_cts, pagado_inicial_cts, n_cuotas, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`,
		e.ID, in.UnidadID, numero, in.Comentario, in.AceptadoPor, fecha, saldo, mora, in.RecargoCts, in.DescuentoCts, monto, pagadoIni, in.NCuotas, se.UsuarioID).Scan(&aid); err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("ACUERDO_DUPLICADO", "Otro acuerdo se creó al mismo tiempo. Intenta de nuevo."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	for _, rc := range elegidos {
		if _, err := tx.Exec(ctx, `INSERT INTO acuerdo_recibo (acuerdo_id, recibo_id, saldo_cts) VALUES ($1,$2,$3)`, aid, rc["recibo_id"], rc["saldo_cts"]); err != nil {
			if esUnico(err) {
				P.Fallo(w, r, P.Conflicto("RECIBO_EN_ACUERDO", "Uno de los recibos ya está en otro acuerdo activo."))
				return
			}
			P.Fallo(w, r, err)
			return
		}
	}
	// Recargo: una línea «ajuste» en el recibo más nuevo. Descuento: líneas negativas del más nuevo al más
	// antiguo, sin bajar ningún recibo de lo ya pagado.
	if err := ajustarRecibosAcuerdo(ctx, tx, aid, numero, elegidos, in.RecargoCts, in.DescuentoCts); err != nil {
		P.Fallo(w, r, err)
		return
	}
	cuotas := DividirCuotas(monto, in.NCuotas, primera)
	for _, c := range cuotas {
		if _, err := tx.Exec(ctx, `INSERT INTO acuerdo_cuota (acuerdo_id, numero, vence, monto_cts) VALUES ($1,$2,$3,$4)`, aid, c.Numero, c.Vence, c.MontoCts); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	// Comprobación del invariante dentro de la transacción: lo pendiente de los recibos = monto acordado.
	var pendiente int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(r.total_cts - r.pagado_cts),0) FROM acuerdo_recibo ar JOIN recibo r ON r.id=ar.recibo_id WHERE ar.acuerdo_id=$1`, aid).Scan(&pendiente); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if pendiente != monto {
		P.Fallo(w, r, fmt.Errorf("acuerdo %s descuadrado: pendiente %d ≠ acordado %d", numero, pendiente, monto))
		return
	}
	s.auditarCambio(ctx, tx, r, "deuda", "crear_acuerdo", "acuerdo_pago", aid, nil,
		map[string]any{"numero": numero, "unidad": codigo, "monto_acordado_cts": monto, "n_cuotas": in.NCuotas})
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": aid, "numero": numero, "unidad": codigo, "saldo_cts": saldo, "mora_cts": mora,
		"monto_acordado_cts": monto, "cuotas": cuotas})
}

// ajustarRecibosAcuerdo aplica el recargo y el descuento del acuerdo como líneas «ajuste» marcadas con él.
func ajustarRecibosAcuerdo(ctx context.Context, tx pgx.Tx, aid int64, numero string, recibos []map[string]any, recargo, descuento int64) error {
	linea := func(rid, monto int64, desc string) error {
		if _, err := tx.Exec(ctx, `INSERT INTO recibo_linea (recibo_id, tipo, descripcion, monto_cts, orden, acuerdo_id)
			VALUES ($1,'ajuste',$2,$3,(SELECT COALESCE(max(orden),0)+1 FROM recibo_linea WHERE recibo_id=$1),$4)`, rid, desc, monto, aid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE recibo SET total_cts = total_cts + $2 WHERE id=$1`, rid, monto); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `SELECT recalcular_recibo($1)`, rid)
		return err
	}
	saldos := make([]int64, len(recibos))
	for i, rc := range recibos {
		saldos[i] = rc["saldo_cts"].(int64)
	}
	ultimo := len(recibos) - 1
	if recargo > 0 {
		if err := linea(recibos[ultimo]["recibo_id"].(int64), recargo, "Recargo del acuerdo de pago "+numero); err != nil {
			return err
		}
		saldos[ultimo] += recargo
	}
	for i := ultimo; i >= 0 && descuento > 0; i-- {
		x := descuento
		if x > saldos[i] {
			x = saldos[i]
		}
		if x == 0 {
			continue
		}
		if err := linea(recibos[i]["recibo_id"].(int64), -x, "Descuento del acuerdo de pago "+numero); err != nil {
			return err
		}
		saldos[i] -= x
		descuento -= x
	}
	return nil
}

const sqlAcuerdo = `SELECT a.id, a.numero, a.unidad_id, u.codigo AS unidad, a.comentario, a.aceptado_por, to_char(a.fecha,'YYYY-MM-DD') AS fecha,
		a.saldo_cts, a.mora_cts, a.recargo_cts, a.descuento_cts, a.monto_acordado_cts, a.n_cuotas, a.estado, a.documento_id, a.anulado_motivo,
		acuerdo_avance_cts(a.id) AS avance_cts, acuerdo_vencido_cts(a.id) AS vencido_cts,
		(SELECT COALESCE(sum(r.total_cts - r.pagado_cts),0) FROM acuerdo_recibo ar JOIN recibo r ON r.id=ar.recibo_id WHERE ar.acuerdo_id=a.id)::bigint AS pendiente_cts
	FROM acuerdo_pago a JOIN unidad u ON u.id=a.unidad_id`

// estadoAcuerdo: el estado guardado es activo/anulado; un activo sin pendiente se muestra como «cumplido» y uno
// con cuotas vencidas impagas, como «vencido».
func estadoAcuerdo(f map[string]any) string {
	if f["estado"] == "anulado" {
		return "anulado"
	}
	if f["pendiente_cts"].(int64) <= 0 {
		return "cumplido"
	}
	if f["vencido_cts"].(int64) > 0 {
		return "vencido"
	}
	return "activo"
}

func (s *Server) decorarAcuerdo(f map[string]any) {
	f["estado_visible"] = estadoAcuerdo(f)
	if v, ok := f["documento_id"].(int64); ok {
		f["documento_url"] = s.Firma.URL(v)
	}
}

// listarAcuerdos: GET /acuerdos?estado=activo|anulado&unidad_id=
func (s *Server) listarAcuerdos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	uid, _ := strconv.ParseInt(r.URL.Query().Get("unidad_id"), 10, 64)
	estado := r.URL.Query().Get("estado")
	filas, err := db.Filas(r.Context(), s.DB, sqlAcuerdo+` WHERE a.edificio_id=$1 AND ($2=0 OR a.unidad_id=$2) AND ($3='' OR a.estado=$3)
		ORDER BY (a.estado='activo') DESC, a.id DESC`, e.ID, uid, estado)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, f := range filas {
		s.decorarAcuerdo(f)
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// verAcuerdo: GET /acuerdos/{id} → cabecera, recibos refinanciados y cuotas con su estado.
func (s *Server) verAcuerdo(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	f, err := db.Fila(ctx, s.DB, sqlAcuerdo+` WHERE a.id=$1 AND a.edificio_id=$2`, id, e.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			P.Fallo(w, r, P.NoEncontrado("el acuerdo"))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	s.decorarAcuerdo(f)
	recibos, err := db.Filas(ctx, s.DB, `SELECT r.id AS recibo_id, COALESCE(r.numero,'') AS numero, p.periodo, ar.saldo_cts AS saldo_firma_cts,
			r.total_cts, r.pagado_cts, r.total_cts - r.pagado_cts AS saldo_cts, r.estado
		FROM acuerdo_recibo ar JOIN recibo r ON r.id=ar.recibo_id JOIN periodo p ON p.id=r.periodo_id
		WHERE ar.acuerdo_id=$1 ORDER BY r.vence, p.periodo`, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	cuotas, err := s.cuotasDe(ctx, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var gracia int
	_ = s.DB.QueryRow(ctx, `SELECT dias_gracia FROM edificio WHERE id=$1`, e.ID).Scan(&gracia)
	f["recibos"] = recibos
	f["cuotas"] = EstadoCuotas(cuotas, f["avance_cts"].(int64), gracia, hoyLima())
	P.JSON(w, http.StatusOK, f)
}

func (s *Server) cuotasDe(ctx context.Context, aid int64) ([]Cuota, error) {
	filas, err := s.DB.Query(ctx, `SELECT numero, to_char(vence,'YYYY-MM-DD'), monto_cts FROM acuerdo_cuota WHERE acuerdo_id=$1 ORDER BY numero`, aid)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	out := []Cuota{}
	for filas.Next() {
		var c Cuota
		if err := filas.Scan(&c.Numero, &c.Vence, &c.MontoCts); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, filas.Err()
}

// anularAcuerdo: POST /acuerdos/{id}/anular {motivo}. Revierte el recargo y el descuento (si lo pagado lo permite)
// y libera los recibos: su deuda vuelve a contar como vencida.
func (s *Server) anularAcuerdo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.Motivo = strings.TrimSpace(in.Motivo)
	if in.Motivo == "" {
		P.Fallo(w, r, P.Validacion("Escribe el motivo.").Campo("motivo", "Obligatorio."))
		return
	}
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var estado, numero string
	if err := tx.QueryRow(ctx, `SELECT estado, numero FROM acuerdo_pago WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, id, e.ID).Scan(&estado, &numero); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el acuerdo"))
		return
	}
	if estado != "activo" {
		P.Fallo(w, r, P.Conflicto("ACUERDO_NO_ACTIVO", "El acuerdo ya está anulado."))
		return
	}
	// Revertir las líneas del acuerdo: sale el recargo y vuelve lo descontado.
	lineas, err := db.Filas(ctx, tx, `SELECT l.id, l.recibo_id, l.monto_cts FROM recibo_linea l WHERE l.acuerdo_id=$1`, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, l := range lineas {
		rid, monto := l["recibo_id"].(int64), l["monto_cts"].(int64)
		var total, pagado int64
		if err := tx.QueryRow(ctx, `SELECT total_cts, pagado_cts FROM recibo WHERE id=$1 FOR UPDATE`, rid).Scan(&total, &pagado); err != nil {
			P.Fallo(w, r, err)
			return
		}
		if total-monto < pagado {
			P.Fallo(w, r, P.Conflicto("ACUERDO_CON_PAGOS", "No se puede quitar el recargo: el recibo ya tiene pagado más de lo que quedaría. Registra una nota de abono en su lugar."))
			return
		}
		if _, err := tx.Exec(ctx, `DELETE FROM recibo_linea WHERE id=$1`, l["id"]); err != nil {
			P.Fallo(w, r, err)
			return
		}
		if _, err := tx.Exec(ctx, `UPDATE recibo SET total_cts = total_cts - $2 WHERE id=$1`, rid, monto); err != nil {
			P.Fallo(w, r, err)
			return
		}
		if _, err := tx.Exec(ctx, `SELECT recalcular_recibo($1)`, rid); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE acuerdo_pago SET estado='anulado', anulado_motivo=$2 WHERE id=$1`, id, in.Motivo); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE acuerdo_recibo SET activo=false WHERE acuerdo_id=$1`, id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, tx, r, "deuda", "anular_acuerdo", "acuerdo_pago", id, map[string]any{"estado": "activo"}, map[string]any{"estado": "anulado", "motivo": in.Motivo})
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "numero": numero, "estado": "anulado"})
}

// documentoAcuerdo: POST /acuerdos/{id}/documento (multipart con «archivo») → el documento firmado por el
// presidente de la junta y el deudor, en el cubo privado. Reemplaza al anterior.
func (s *Server) documentoAcuerdo(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	if err := leerMultipart(r); err != nil {
		P.Fallo(w, r, err)
		return
	}
	docs, err := archivosDeForm(r, "archivo", "documento")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if len(docs) == 0 {
		P.Fallo(w, r, P.Validacion("Adjunta el documento firmado.").Campo("archivo", "Obligatorio."))
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT true FROM acuerdo_pago WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, id, e.ID).Scan(&ok); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el acuerdo"))
		return
	}
	se := ses(r)
	archID, err := s.guardarArchivo(ctx, tx, e.ID, &se.UsuarioID, docs[0])
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE acuerdo_pago SET documento_id=$2 WHERE id=$1`, id, archID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "documento_id": archID, "documento_url": s.Firma.URL(archID)})
}
