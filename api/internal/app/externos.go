package app

import (
	"net/http"
	"strings"
	"time"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Recibos externos e ingresos externos (bloque B4): lo que no es la cuota de mantenimiento.
// El ingreso externo se asienta en su fondo (bloque C) para que entre en la trazabilidad.

// listarRecibosExternos: GET /recibos-externos?periodo=AAAA-MM
func (s *Server) listarRecibosExternos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	periodo, err := s.periodoDe(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	filas, err := db.Filas(r.Context(), s.DB, `SELECT re.id, re.periodo, re.tercero, COALESCE(c.nombre,'') AS cliente, re.concepto, re.monto_cts, re.estado,
			to_char(re.fecha_emision,'YYYY-MM-DD') AS fecha_emision, to_char(re.fecha_vencimiento,'YYYY-MM-DD') AS fecha_vencimiento, re.recurrente
		FROM recibo_externo re LEFT JOIN cliente c ON c.id=re.cliente_id
		WHERE re.edificio_id=$1 AND re.periodo=$2 ORDER BY re.estado, re.id DESC`, e.ID, periodo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "periodo": periodo})
}

// crearReciboExterno: POST /recibos-externos
func (s *Server) crearReciboExterno(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in struct {
		Periodo          string `json:"periodo"`
		ClienteID        int64  `json:"cliente_id"`
		Tercero          string `json:"tercero"`
		Concepto         string `json:"concepto"`
		MontoCts         int64  `json:"monto_cts"`
		FechaEmision     string `json:"fecha_emision"`
		FechaVencimiento string `json:"fecha_vencimiento"`
		Recurrente       bool   `json:"recurrente"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa los datos del recibo externo.")
	in.Concepto = strings.TrimSpace(in.Concepto)
	if in.Concepto == "" {
		ev.Campo("concepto", "Escribe el concepto.")
	}
	if in.MontoCts <= 0 {
		ev.Campo("monto_cts", "El monto debe ser mayor que cero.")
	}
	fe := fechaOpc(in.FechaEmision, ev, "fecha_emision")
	fv := fechaOpc(in.FechaVencimiento, ev, "fecha_vencimiento")
	periodo := in.Periodo
	if periodo == "" {
		if fe != nil {
			periodo = fe.Format("2006-01")
		} else {
			periodo = P.PeriodoActual()
		}
	}
	if !P.PeriodoValido(periodo) {
		ev.Campo("periodo", "Formato AAAA-MM.")
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	var cliente *int64
	if in.ClienteID > 0 {
		cliente = &in.ClienteID
	}
	var id int64
	err := s.DB.QueryRow(r.Context(), `INSERT INTO recibo_externo (edificio_id, periodo, cliente_id, tercero, concepto, monto_cts, fecha_emision, fecha_vencimiento, recurrente, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,COALESCE($7,(now() AT TIME ZONE 'America/Lima')::date),$8,$9,$10) RETURNING id`,
		e.ID, periodo, cliente, strings.TrimSpace(in.Tercero), in.Concepto, in.MontoCts, fe, fv, in.Recurrente, ses(r).UsuarioID).Scan(&id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "periodo": periodo, "estado": "emitido"})
}

// pagarReciboExterno: POST /recibos-externos/{id}/pagar → pasa a pagado.
func (s *Server) pagarReciboExterno(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	ct, err := s.DB.Exec(r.Context(), `UPDATE recibo_externo SET estado='pagado' WHERE id=$1 AND edificio_id=$2 AND estado='emitido'`, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.Conflicto("EXTERNO_NO_PAGABLE", "Ese recibo externo no existe o ya no está emitido."))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": "pagado"})
}

// listarIngresosExternos: GET /ingresos-externos?periodo=AAAA-MM
func (s *Server) listarIngresosExternos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	periodo, err := s.periodoDe(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	filas, err := db.Filas(r.Context(), s.DB, `SELECT ie.id, ie.periodo, ie.descripcion, ie.monto_cts, to_char(ie.fecha,'YYYY-MM-DD') AS fecha, ie.medio, ie.fondo_id, f.nombre AS fondo
		FROM ingreso_externo ie LEFT JOIN fondo f ON f.id=ie.fondo_id
		WHERE ie.edificio_id=$1 AND ie.periodo=$2 ORDER BY ie.fecha DESC, ie.id DESC`, e.ID, periodo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "periodo": periodo})
}

// crearIngresoExterno: POST /ingresos-externos {descripcion, monto_cts, fecha?, fondo_id?}
func (s *Server) crearIngresoExterno(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in struct {
		Descripcion string `json:"descripcion"`
		MontoCts    int64  `json:"monto_cts"`
		Fecha       string `json:"fecha"`
		Medio       string `json:"medio"`
		FondoID     int64  `json:"fondo_id"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa el ingreso.")
	in.Descripcion = strings.TrimSpace(in.Descripcion)
	if in.Descripcion == "" {
		ev.Campo("descripcion", "Escribe la descripción.")
	}
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
		in.Medio = "efectivo"
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var fondo *int64
	if in.FondoID > 0 {
		var ok bool
		_ = tx.QueryRow(ctx, `SELECT true FROM fondo WHERE id=$1 AND edificio_id=$2`, in.FondoID, e.ID).Scan(&ok)
		if !ok {
			P.Fallo(w, r, P.Validacion("Elige un fondo del edificio.").Campo("fondo_id", "Fondo no válido."))
			return
		}
		fondo = &in.FondoID
	}
	se := ses(r)
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO ingreso_externo (edificio_id, periodo, descripcion, monto_cts, fecha, medio, fondo_id, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		e.ID, fecha.Format("2006-01"), in.Descripcion, in.MontoCts, fecha, in.Medio, fondo, se.UsuarioID).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if fondo != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO fondo_movimiento (edificio_id, fondo_id, periodo, fecha, monto_cts, tipo, origen, ref_id, descripcion, creado_por)
			VALUES ($1,$2,$3,$4,$5,'ingreso','manual',$6,$7,$8)`,
			e.ID, *fondo, fecha.Format("2006-01"), fecha, in.MontoCts, id, in.Descripcion, se.UsuarioID); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "monto_cts": in.MontoCts, "periodo": fecha.Format("2006-01")})
}
