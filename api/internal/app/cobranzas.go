package app

import (
	"net/http"
	"strings"
	"time"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Cobranzas sin identificar y devoluciones (bloque A4): dinero que entra sin recibo y su imputación.

// listarCobranzas: GET /cobranzas-sin-identificar?estado=
func (s *Server) listarCobranzas(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	estado := strings.TrimSpace(r.URL.Query().Get("estado"))
	filas, err := db.Filas(r.Context(), s.DB, `SELECT c.id, c.periodo, to_char(c.fecha,'YYYY-MM-DD') AS fecha, c.monto_cts, c.medio,
			c.codigo_operacion, c.descripcion, c.estado, c.unidad_id, u.codigo AS unidad
		FROM cobranza_sin_identificar c LEFT JOIN unidad u ON u.id=c.unidad_id
		WHERE c.edificio_id=$1 AND ($2='' OR c.estado=$2) ORDER BY c.estado, c.fecha DESC, c.id DESC`, e.ID, estado)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearCobranza: POST /cobranzas-sin-identificar
func (s *Server) crearCobranza(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in struct {
		MontoCts         int64  `json:"monto_cts"`
		Medio            string `json:"medio"`
		CuentaBancariaID int64  `json:"cuenta_bancaria_id"`
		CodigoOperacion  string `json:"codigo_operacion"`
		Fecha            string `json:"fecha"`
		Descripcion      string `json:"descripcion"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa la cobranza.")
	if in.MontoCts <= 0 {
		ev.Campo("monto_cts", "El monto debe ser mayor que cero.")
	}
	fecha := time.Now().In(P.Lima)
	if in.Fecha != "" {
		t, err := time.Parse("2006-01-02", in.Fecha)
		if err != nil {
			ev.Campo("fecha", "Formato AAAA-MM-DD.")
		} else {
			fecha = t
		}
	}
	if in.Medio == "" {
		in.Medio = "transferencia"
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	var cuenta *int64
	if in.CuentaBancariaID > 0 {
		cuenta = &in.CuentaBancariaID
	}
	var id int64
	if err := s.DB.QueryRow(r.Context(), `INSERT INTO cobranza_sin_identificar (edificio_id, periodo, fecha, monto_cts, medio, cuenta_bancaria_id, codigo_operacion, descripcion, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		e.ID, fecha.Format("2006-01"), fecha, in.MontoCts, in.Medio, cuenta, strings.TrimSpace(in.CodigoOperacion), strings.TrimSpace(in.Descripcion), ses(r).UsuarioID).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "estado": "pendiente"})
}

// imputarCobranza: POST /cobranzas-sin-identificar/{id}/imputar {unidad_id}
// Reparte el monto entre los cargos de la unidad (del más antiguo al más nuevo) y asienta en fondos.
func (s *Server) imputarCobranza(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	var in struct {
		UnidadID int64 `json:"unidad_id"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.UnidadID <= 0 {
		P.Fallo(w, r, P.Validacion("Elige la unidad.").Campo("unidad_id", "Obligatorio."))
		return
	}
	var ok bool
	_ = s.DB.QueryRow(ctx, `SELECT true FROM unidad WHERE id=$1 AND edificio_id=$2`, in.UnidadID, e.ID).Scan(&ok)
	if !ok {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var c struct {
		Monto  int64
		Medio  string
		Codigo string
		Fecha  time.Time
		Estado string
	}
	if err := tx.QueryRow(ctx, `SELECT monto_cts, medio, codigo_operacion, fecha, estado FROM cobranza_sin_identificar WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, id, e.ID).
		Scan(&c.Monto, &c.Medio, &c.Codigo, &c.Fecha, &c.Estado); err != nil {
		P.Fallo(w, r, P.NoEncontrado("la cobranza"))
		return
	}
	if c.Estado != "pendiente" {
		P.Fallo(w, r, P.Conflicto("COBRANZA_YA_RESUELTA", "Esa cobranza ya fue imputada o devuelta."))
		return
	}
	res, err := s.AplicarPago(ctx, tx, e.ID, in.UnidadID, ses(r).UsuarioID, c.Monto, c.Medio, c.Codigo, c.Fecha.Format("2006-01-02"))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if apls, ok := res["aplicaciones"].([]Aplicacion); ok {
		for _, a := range apls {
			if err := s.asentarIngresoPago(ctx, tx, e.ID, a.PagoID, a.ReciboID, a.MontoCts, c.Fecha); err != nil {
				P.Fallo(w, r, err)
				return
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE cobranza_sin_identificar SET estado='imputada', unidad_id=$1 WHERE id=$2`, in.UnidadID, id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": "imputada", "imputado_cts": c.Monto})
}

// devolverCobranza: POST /cobranzas-sin-identificar/{id}/devolver {motivo}
func (s *Server) devolverCobranza(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var c struct {
		Monto  int64
		Fecha  time.Time
		Estado string
	}
	if err := tx.QueryRow(ctx, `SELECT monto_cts, fecha, estado FROM cobranza_sin_identificar WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, id, e.ID).Scan(&c.Monto, &c.Fecha, &c.Estado); err != nil {
		P.Fallo(w, r, P.NoEncontrado("la cobranza"))
		return
	}
	if c.Estado != "pendiente" {
		P.Fallo(w, r, P.Conflicto("COBRANZA_YA_RESUELTA", "Esa cobranza ya fue imputada o devuelta."))
		return
	}
	if _, err := tx.Exec(ctx, `INSERT INTO devolucion (edificio_id, cobranza_id, monto_cts, fecha, motivo, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6)`, e.ID, id, c.Monto, c.Fecha, strings.TrimSpace(in.Motivo), ses(r).UsuarioID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE cobranza_sin_identificar SET estado='devuelta' WHERE id=$1`, id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": "devuelta"})
}

// listarDevoluciones: GET /devoluciones
func (s *Server) listarDevoluciones(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT d.id, to_char(d.fecha,'YYYY-MM-DD') AS fecha, d.monto_cts, d.motivo, u.codigo AS unidad
		FROM devolucion d LEFT JOIN unidad u ON u.id=d.unidad_id WHERE d.edificio_id=$1 ORDER BY d.fecha DESC, d.id DESC LIMIT 200`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}
