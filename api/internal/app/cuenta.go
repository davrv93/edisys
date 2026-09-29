package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Cuenta corriente de la unidad: los recibos de cada periodo y los cargos de deuda inicial (0008)
// forman una sola lista de cargos. Los pagos a cuenta se aplican del más antiguo al más nuevo.

// Aplicacion es la parte de un pago que cae en un cargo.
type Aplicacion struct {
	ReciboID int64  `json:"recibo_id"`
	Numero   string `json:"numero"`
	Periodo  string `json:"periodo"`
	Origen   string `json:"origen"`
	MontoCts int64  `json:"monto_cts"`
	SaldoCts int64  `json:"saldo_cts"` // saldo del cargo después de aplicar
	PagoID   int64  `json:"pago_id"`
}

// cargoPendiente es un recibo con saldo, en el orden en que se cobra.
type cargoPendiente struct {
	id          int64
	numero, per string
	origen      string
	saldo       int64
}

// cargosPendientes bloquea (FOR UPDATE) y devuelve los cargos con saldo de la unidad, del más antiguo al más nuevo.
func cargosPendientes(ctx context.Context, tx pgx.Tx, uid int64) ([]cargoPendiente, error) {
	filas, err := tx.Query(ctx, `SELECT r.id, COALESCE(r.numero,''), p.periodo, r.origen, r.total_cts - r.pagado_cts
		FROM recibo r JOIN periodo p ON p.id = r.periodo_id
		WHERE r.unidad_id=$1 AND r.estado IN ('emitido','pagado_parcial') AND r.total_cts > r.pagado_cts
		ORDER BY r.vence NULLS LAST, p.periodo, r.id FOR UPDATE OF r`, uid)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []cargoPendiente
	for filas.Next() {
		var c cargoPendiente
		if err := filas.Scan(&c.id, &c.numero, &c.per, &c.origen, &c.saldo); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, filas.Err()
}

// Repartir distribuye un monto entre saldos en orden (del más antiguo al más nuevo). Pura: se prueba sola.
func Repartir(monto int64, saldos []int64) []int64 {
	out := make([]int64, len(saldos))
	for i, s := range saldos {
		if monto <= 0 {
			break
		}
		a := s
		if a > monto {
			a = monto
		}
		out[i] = a
		monto -= a
	}
	return out
}

// pagoACuenta: POST /unidades/{uid}/pagos {monto_cts, medio, codigo_operacion, fecha} (administración).
// El pago se reparte entre los cargos pendientes empezando por el más antiguo (la deuda inicial primero).
func (s *Server) pagoACuenta(w http.ResponseWriter, r *http.Request) {
	uid, err := idRuta(r, "uid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		MontoCts        int64  `json:"monto_cts"`
		Medio           string `json:"medio"`
		CodigoOperacion string `json:"codigo_operacion"`
		Fecha           string `json:"fecha"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	var codigoU string
	if err := s.DB.QueryRow(ctx, `SELECT codigo FROM unidad WHERE id=$1 AND edificio_id=$2`, uid, e.ID).Scan(&codigoU); err != nil {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	ev := P.Validacion("Revisa los datos del pago.")
	if in.MontoCts <= 0 {
		ev.Campo("monto_cts", "El monto debe ser mayor que cero.")
	}
	switch in.Medio {
	case "yape", "plin", "transferencia", "deposito", "tarjeta":
		if strings.TrimSpace(in.CodigoOperacion) == "" {
			ev.Campo("codigo_operacion", "Escribe el código de operación.")
		}
	case "efectivo":
	default:
		ev.Campo("medio", "Elige yape, plin, transferencia, deposito, tarjeta o efectivo.")
	}
	fecha := time.Now().In(P.Lima).Format("2006-01-02")
	if in.Fecha != "" {
		if _, err := time.Parse("2006-01-02", in.Fecha); err != nil {
			ev.Campo("fecha", "Formato AAAA-MM-DD.")
		}
		fecha = in.Fecha
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
	res, err := s.AplicarPago(ctx, tx, e.ID, uid, ses(r).UsuarioID, in.MontoCts, in.Medio, in.CodigoOperacion, fecha)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "recibos", "pago_a_cuenta", "unidad", uid, nil, res)
	res["unidad"] = codigoU
	P.JSON(w, http.StatusCreated, res)
}

// AplicarPago registra un pago validado de la unidad repartido del cargo más antiguo al más nuevo.
// Lo usan el pago a cuenta y la conciliación bancaria. 422 MONTO_MAYOR_AL_SALDO si pasa la deuda total.
func (s *Server) AplicarPago(ctx context.Context, tx pgx.Tx, eid, uid, usuario, monto int64, medio, codigoOp, fecha string) (map[string]any, error) {
	cargos, err := cargosPendientes(ctx, tx, uid)
	if err != nil {
		return nil, err
	}
	saldos := make([]int64, len(cargos))
	var deuda int64
	for i, c := range cargos {
		saldos[i] = c.saldo
		deuda += c.saldo
	}
	if monto > deuda {
		return nil, P.Err(http.StatusUnprocessableEntity, "MONTO_MAYOR_AL_SALDO", "El pago pasa la deuda de la unidad ("+P.Soles(deuda)+").").
			Campo("monto_cts", "Máximo "+P.Soles(deuda)+".")
	}
	var codigo *string
	if c := strings.TrimSpace(codigoOp); c != "" {
		codigo = &c
	}
	partes := Repartir(monto, saldos)
	apls := []Aplicacion{}
	parte := 0
	for i, a := range partes {
		if a == 0 {
			continue
		}
		parte++
		var pid int64
		if err := tx.QueryRow(ctx, `INSERT INTO pago (edificio_id, recibo_id, monto_cts, medio, codigo_operacion, fecha, estado, registrado_por, validado_por, validado_en, parte)
			VALUES ($1,$2,$3,$4,$5,$6,'validado',$7,$7,now(),$8) RETURNING id`, eid, cargos[i].id, a, medio, codigo, fecha, usuario, parte).Scan(&pid); err != nil {
			return nil, P.Traducir(err)
		}
		apls = append(apls, Aplicacion{ReciboID: cargos[i].id, Numero: cargos[i].numero, Periodo: cargos[i].per, Origen: cargos[i].origen,
			MontoCts: a, SaldoCts: cargos[i].saldo - a, PagoID: pid})
	}
	return map[string]any{"monto_cts": monto, "deuda_antes_cts": deuda, "deuda_despues_cts": deuda - monto, "aplicaciones": apls}, nil
}

// cuentaCorriente: GET /unidades/{uid}/cuenta → cargos (recibos y deuda inicial) y pagos, del más antiguo al más nuevo.
func (s *Server) cuentaCorriente(w http.ResponseWriter, r *http.Request) {
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
	cargos, err := db.Filas(ctx, s.DB, `SELECT r.id AS recibo_id, COALESCE(r.numero,'') AS numero, p.periodo, r.origen, r.estado, r.total_cts, r.pagado_cts,
			r.total_cts - r.pagado_cts AS saldo_cts, to_char(r.vence,'YYYY-MM-DD') AS vence
		FROM recibo r JOIN periodo p ON p.id=r.periodo_id JOIN unidad u ON u.id=r.unidad_id
		WHERE r.unidad_id=$1 AND u.edificio_id=$2 AND r.estado NOT IN ('borrador','anulado') ORDER BY r.vence NULLS LAST, p.periodo, r.id`, uid, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var deuda, vencida int64
	_ = s.DB.QueryRow(ctx, `SELECT COALESCE(sum(total_cts - pagado_cts) FILTER (WHERE estado IN ('emitido','pagado_parcial')),0), deuda_vencida_cts($1)
		FROM recibo WHERE unidad_id=$1`, uid).Scan(&deuda, &vencida)
	P.JSON(w, http.StatusOK, map[string]any{"unidad_id": uid, "cargos": cargos, "deuda_cts": deuda, "deuda_vencida_cts": vencida, "moroso": vencida > 0})
}
