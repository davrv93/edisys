package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Proveedores y cuentas por pagar (bloque B). El pago de una cuenta por pagar genera el egreso
// en el balance: una sola fuente de verdad para el gasto.

type entProveedor struct {
	RazonSocial string `json:"razon_social"`
	RUC         string `json:"ruc"`
	Contacto    string `json:"contacto"`
	Telefono    string `json:"telefono"`
	Correo      string `json:"correo"`
	Banco       string `json:"banco"`
	Cuenta      string `json:"cuenta"`
}

func (in *entProveedor) validar() error {
	ev := P.Validacion("Revisa los datos del proveedor.")
	in.RazonSocial = strings.TrimSpace(in.RazonSocial)
	in.RUC = strings.TrimSpace(in.RUC)
	in.Contacto = strings.TrimSpace(in.Contacto)
	in.Telefono = strings.TrimSpace(in.Telefono)
	in.Correo = strings.TrimSpace(in.Correo)
	if in.RazonSocial == "" {
		ev.Campo("razon_social", "Escribe la razón social.")
	}
	if in.RUC != "" && len(in.RUC) != 11 {
		ev.Campo("ruc", "El RUC tiene 11 dígitos.")
	}
	if in.Correo != "" && !strings.Contains(in.Correo, "@") {
		ev.Campo("correo", "Correo no válido.")
	}
	if len(ev.Campos) > 0 {
		return ev
	}
	return nil
}

// listarProveedores: GET /proveedores?buscar=&activo=
func (s *Server) listarProveedores(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	buscar := strings.TrimSpace(r.URL.Query().Get("buscar"))
	soloActivos := r.URL.Query().Get("activo") != "0"
	filas, err := db.Filas(r.Context(), s.DB, `SELECT p.id, p.razon_social, p.ruc, p.contacto, p.telefono, p.correo, p.banco, p.cuenta, p.activo,
			COALESCE(SUM(c.monto_cts - c.pagado_cts) FILTER (WHERE c.estado IN ('pendiente','parcial')), 0)::bigint AS por_pagar_cts
		FROM proveedor p LEFT JOIN cuenta_por_pagar c ON c.proveedor_id = p.id
		WHERE p.edificio_id=$1
		  AND ($2 = '' OR lower(p.razon_social) LIKE '%' || lower($2) || '%' OR p.ruc LIKE '%' || $2 || '%')
		  AND (NOT $3 OR p.activo)
		GROUP BY p.id ORDER BY lower(p.razon_social)`, e.ID, buscar, soloActivos)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearProveedor: POST /proveedores
func (s *Server) crearProveedor(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in entProveedor
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := in.validar(); err != nil {
		P.Fallo(w, r, err)
		return
	}
	var id int64
	err := s.DB.QueryRow(r.Context(), `INSERT INTO proveedor (edificio_id, razon_social, ruc, contacto, telefono, correo, banco, cuenta, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		e.ID, in.RazonSocial, in.RUC, in.Contacto, in.Telefono, in.Correo, in.Banco, in.Cuenta, ses(r).UsuarioID).Scan(&id)
	if err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("PROVEEDOR_DUPLICADO", "Ya existe un proveedor con ese RUC."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// verProveedor: GET /proveedores/{pid}
func (s *Server) verProveedor(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	pid := idURL(r, "pid")
	p, err := db.Fila(r.Context(), s.DB, `SELECT p.id, p.razon_social, p.ruc, p.contacto, p.telefono, p.correo, p.banco, p.cuenta, p.activo,
			COALESCE(SUM(c.monto_cts - c.pagado_cts) FILTER (WHERE c.estado IN ('pendiente','parcial')), 0)::bigint AS por_pagar_cts
		FROM proveedor p LEFT JOIN cuenta_por_pagar c ON c.proveedor_id = p.id
		WHERE p.id=$1 AND p.edificio_id=$2 GROUP BY p.id`, pid, e.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		P.Fallo(w, r, P.NoEncontrado("proveedor"))
		return
	}
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	cuentas, err := db.Filas(r.Context(), s.DB, `SELECT c.id, c.descripcion, c.comprobante_tipo, c.comprobante_numero, c.monto_cts, c.pagado_cts,
			(c.monto_cts - c.pagado_cts) AS saldo_cts, c.estado,
			to_char(c.fecha_vencimiento,'YYYY-MM-DD') AS fecha_vencimiento
		FROM cuenta_por_pagar c WHERE c.proveedor_id=$1 ORDER BY c.estado, c.fecha_vencimiento NULLS LAST, c.id DESC`, pid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"proveedor": p, "cuentas": cuentas})
}

// editarProveedor: PUT /proveedores/{pid}
func (s *Server) editarProveedor(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	pid := idURL(r, "pid")
	var in entProveedor
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := in.validar(); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE proveedor SET razon_social=$1, ruc=$2, contacto=$3, telefono=$4, correo=$5, banco=$6, cuenta=$7, actualizado_en=now()
		WHERE id=$8 AND edificio_id=$9`, in.RazonSocial, in.RUC, in.Contacto, in.Telefono, in.Correo, in.Banco, in.Cuenta, pid, e.ID)
	if err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("PROVEEDOR_DUPLICADO", "Ya existe un proveedor con ese RUC."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("proveedor"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": pid})
}

// borrarProveedor: DELETE /proveedores/{pid} → baja lógica (activo=false).
func (s *Server) borrarProveedor(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	pid := idURL(r, "pid")
	ct, err := s.DB.Exec(r.Context(), `UPDATE proveedor SET activo=false, actualizado_en=now() WHERE id=$1 AND edificio_id=$2`, pid, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("proveedor"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": pid, "activo": false})
}

type entCuentaPorPagar struct {
	ProveedorID       int64  `json:"proveedor_id"`
	RubroID           int64  `json:"rubro_id"`
	ConceptoID        int64  `json:"concepto_id"`
	Descripcion       string `json:"descripcion"`
	ComprobanteTipo   string `json:"comprobante_tipo"`
	ComprobanteNumero string `json:"comprobante_numero"`
	FechaEmision      string `json:"fecha_emision"`
	FechaVencimiento  string `json:"fecha_vencimiento"`
	MontoCts          int64  `json:"monto_cts"`
}

// listarCuentasPorPagar: GET /cuentas-por-pagar?estado=&proveedor_id=
func (s *Server) listarCuentasPorPagar(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	estado := strings.TrimSpace(r.URL.Query().Get("estado"))
	prov, _ := strconv.ParseInt(r.URL.Query().Get("proveedor_id"), 10, 64)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT c.id, c.proveedor_id, p.razon_social AS proveedor, c.descripcion,
			c.comprobante_tipo, c.comprobante_numero, c.monto_cts, c.pagado_cts, (c.monto_cts - c.pagado_cts) AS saldo_cts, c.estado,
			to_char(c.fecha_emision,'YYYY-MM-DD') AS fecha_emision, to_char(c.fecha_vencimiento,'YYYY-MM-DD') AS fecha_vencimiento,
			(c.fecha_vencimiento IS NOT NULL AND c.fecha_vencimiento < (now() AT TIME ZONE 'America/Lima')::date AND c.estado IN ('pendiente','parcial')) AS vencida,
			c.archivo_comprobante_id
		FROM cuenta_por_pagar c JOIN proveedor p ON p.id=c.proveedor_id
		WHERE c.edificio_id=$1
		  AND ($2 = '' OR c.estado=$2)
		  AND ($3 = 0 OR c.proveedor_id=$3)
		ORDER BY (c.estado='pagado'), c.fecha_vencimiento NULLS LAST, c.id DESC`, e.ID, estado, prov)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var totalPendiente int64
	for _, f := range filas {
		if v, ok := f["saldo_cts"].(int64); ok {
			f["saldo_cts"] = v
		}
	}
	_ = s.DB.QueryRow(r.Context(), `SELECT COALESCE(SUM(monto_cts - pagado_cts),0) FROM cuenta_por_pagar WHERE edificio_id=$1 AND estado IN ('pendiente','parcial')`, e.ID).Scan(&totalPendiente)
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "por_pagar_cts": totalPendiente})
}

// crearCuentaPorPagar: POST /cuentas-por-pagar (multipart con «comprobante» o JSON)
func (s *Server) crearCuentaPorPagar(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	var in entCuentaPorPagar
	var docs []Subido
	if esMultipart(r) {
		if err := leerMultipart(r); err != nil {
			P.Fallo(w, r, err)
			return
		}
		in.ProveedorID, _ = strconv.ParseInt(campo(r, "proveedor_id"), 10, 64)
		in.RubroID, _ = strconv.ParseInt(campo(r, "rubro_id"), 10, 64)
		in.ConceptoID, _ = strconv.ParseInt(campo(r, "concepto_id"), 10, 64)
		in.Descripcion = campo(r, "descripcion")
		in.ComprobanteTipo = campo(r, "comprobante_tipo")
		in.ComprobanteNumero = campo(r, "comprobante_numero")
		in.FechaEmision = campo(r, "fecha_emision")
		in.FechaVencimiento = campo(r, "fecha_vencimiento")
		in.MontoCts, _ = strconv.ParseInt(campo(r, "monto_cts"), 10, 64)
		var err error
		if docs, err = archivosDeForm(r, "comprobante", "archivo", "foto"); err != nil {
			P.Fallo(w, r, err)
			return
		}
	} else if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa los datos de la cuenta por pagar.")
	if in.MontoCts <= 0 {
		ev.Campo("monto_cts", "El monto debe ser mayor que cero.")
	}
	var ok bool
	_ = s.DB.QueryRow(ctx, `SELECT true FROM proveedor WHERE id=$1 AND edificio_id=$2`, in.ProveedorID, e.ID).Scan(&ok)
	if !ok {
		ev.Campo("proveedor_id", "Elige un proveedor del edificio.")
	}
	if in.RubroID == 0 {
		ev.Campo("rubro_id", "Elige el rubro del gasto (para el egreso).")
	} else {
		_ = s.DB.QueryRow(ctx, `SELECT true FROM rubro WHERE id=$1 AND edificio_id=$2`, in.RubroID, e.ID).Scan(&ok)
		if !ok {
			ev.Campo("rubro_id", "Rubro del edificio no válido.")
		}
	}
	if in.ComprobanteTipo == "" {
		in.ComprobanteTipo = "recibo"
	}
	if in.ComprobanteTipo != "recibo" && in.ComprobanteTipo != "factura" && in.ComprobanteTipo != "boleta" && in.ComprobanteTipo != "otro" {
		ev.Campo("comprobante_tipo", "Tipo no válido.")
	}
	fe := fechaOpc(in.FechaEmision, ev, "fecha_emision")
	fv := fechaOpc(in.FechaVencimiento, ev, "fecha_vencimiento")
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
	var concepto *int64
	if in.ConceptoID > 0 {
		concepto = &in.ConceptoID
	}
	se := ses(r)
	var docID *int64
	if len(docs) > 0 {
		id, err := s.guardarArchivo(ctx, tx, e.ID, &se.UsuarioID, docs[0])
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		docID = &id
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO cuenta_por_pagar
		(edificio_id, proveedor_id, rubro_id, concepto_id, descripcion, comprobante_tipo, comprobante_numero, fecha_emision, fecha_vencimiento, monto_cts, archivo_comprobante_id, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
		e.ID, in.ProveedorID, in.RubroID, concepto, strings.TrimSpace(in.Descripcion), in.ComprobanteTipo, strings.TrimSpace(in.ComprobanteNumero),
		fe, fv, in.MontoCts, docID, se.UsuarioID).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "monto_cts": in.MontoCts, "saldo_cts": in.MontoCts, "estado": "pendiente", "con_comprobante": docID != nil})
}

// verCuentaPorPagar: GET /cuentas-por-pagar/{cid}
func (s *Server) verCuentaPorPagar(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	cid := idURL(r, "cid")
	c, err := db.Fila(r.Context(), s.DB, `SELECT c.id, c.proveedor_id, p.razon_social AS proveedor, p.banco AS proveedor_banco, p.cuenta AS proveedor_cuenta,
			c.rubro_id, c.concepto_id, c.descripcion, c.comprobante_tipo, c.comprobante_numero,
			to_char(c.fecha_emision,'YYYY-MM-DD') AS fecha_emision, to_char(c.fecha_vencimiento,'YYYY-MM-DD') AS fecha_vencimiento,
			c.monto_cts, c.pagado_cts, (c.monto_cts - c.pagado_cts) AS saldo_cts, c.estado, c.archivo_comprobante_id, c.creado_en
		FROM cuenta_por_pagar c JOIN proveedor p ON p.id=c.proveedor_id
		WHERE c.id=$1 AND c.edificio_id=$2`, cid, e.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		P.Fallo(w, r, P.NoEncontrado("cuenta por pagar"))
		return
	}
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	pagos, err := db.Filas(r.Context(), s.DB, `SELECT id, to_char(fecha,'YYYY-MM-DD') AS fecha, monto_cts, cuenta_cargo, numero_operacion, modalidad, comentario, archivo_ticket_id, egreso_id
		FROM cpp_pago WHERE cuenta_por_pagar_id=$1 ORDER BY fecha, id`, cid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"cuenta": c, "pagos": pagos})
}

// pagarCuentaPorPagar: POST /cuentas-por-pagar/{cid}/pagos (multipart con «ticket» o JSON).
// Crea el cpp_pago y el egreso del balance en la misma transacción.
func (s *Server) pagarCuentaPorPagar(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	cid := idURL(r, "cid")
	var in struct {
		MontoCts        int64  `json:"monto_cts"`
		Fecha           string `json:"fecha"`
		CuentaCargo     string `json:"cuenta_cargo"`
		NumeroOperacion string `json:"numero_operacion"`
		Modalidad       string `json:"modalidad"`
		Comentario      string `json:"comentario"`
	}
	var docs []Subido
	if esMultipart(r) {
		if err := leerMultipart(r); err != nil {
			P.Fallo(w, r, err)
			return
		}
		in.MontoCts, _ = strconv.ParseInt(campo(r, "monto_cts"), 10, 64)
		in.Fecha = campo(r, "fecha")
		in.CuentaCargo = campo(r, "cuenta_cargo")
		in.NumeroOperacion = campo(r, "numero_operacion")
		in.Modalidad = campo(r, "modalidad")
		in.Comentario = campo(r, "comentario")
		var err error
		if docs, err = archivosDeForm(r, "ticket", "archivo", "foto"); err != nil {
			P.Fallo(w, r, err)
			return
		}
	} else if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa los datos del pago.")
	if in.MontoCts <= 0 {
		ev.Campo("monto_cts", "El monto debe ser mayor que cero.")
	}
	if in.Modalidad == "" {
		in.Modalidad = "transferencia"
	}
	seg := map[string]bool{"transferencia": true, "efectivo": true, "yape": true, "plin": true, "deposito": true, "cheque": true}
	if !seg[in.Modalidad] {
		ev.Campo("modalidad", "Modalidad no válida.")
	}
	fe := fechaOpc(in.Fecha, ev, "fecha")
	if fe == nil {
		hoy := time.Now().In(P.Lima)
		fe = &hoy
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
	var cpp struct {
		ID       int64
		RubroID  *int64
		Concepto *int64
		Desc     string
		Monto    int64
		Pagado   int64
		Estado   string
	}
	if err := tx.QueryRow(ctx, `SELECT id, rubro_id, concepto_id, descripcion, monto_cts, pagado_cts, estado
		FROM cuenta_por_pagar WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, cid, e.ID).
		Scan(&cpp.ID, &cpp.RubroID, &cpp.Concepto, &cpp.Desc, &cpp.Monto, &cpp.Pagado, &cpp.Estado); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			P.Fallo(w, r, P.NoEncontrado("cuenta por pagar"))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	if cpp.Estado == "anulado" {
		P.Fallo(w, r, P.Prohibido("CUENTA_ANULADA", "La cuenta está anulada."))
		return
	}
	saldo := cpp.Monto - cpp.Pagado
	if in.MontoCts > saldo {
		P.Fallo(w, r, P.Conflicto("PAGO_MAYOR_AL_SALDO", "El pago supera el saldo de "+P.Soles(saldo)+"."))
		return
	}
	se := ses(r)
	var ticketID *int64
	if len(docs) > 0 {
		id, err := s.guardarArchivo(ctx, tx, e.ID, &se.UsuarioID, docs[0])
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		ticketID = &id
	}
	periodo := fe.Format("2006-01")
	var rubroID int64
	if cpp.RubroID != nil {
		rubroID = *cpp.RubroID
	}
	desc := strings.TrimSpace(cpp.Desc)
	if desc == "" {
		desc = "Pago a proveedor"
	}
	var egresoID int64
	if err := tx.QueryRow(ctx, `INSERT INTO egreso (edificio_id, periodo, rubro_id, concepto_id, descripcion, monto_cts, fecha, documento_id, tipo_documento, origen, registrado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'voucher','manual',$9) RETURNING id`,
		e.ID, periodo, rubroID, cpp.Concepto, desc, in.MontoCts, *fe, ticketID, se.UsuarioID).Scan(&egresoID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	var pagoID int64
	if err := tx.QueryRow(ctx, `INSERT INTO cpp_pago (cuenta_por_pagar_id, fecha, monto_cts, cuenta_cargo, numero_operacion, modalidad, comentario, archivo_ticket_id, egreso_id, registrado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		cid, *fe, in.MontoCts, strings.TrimSpace(in.CuentaCargo), strings.TrimSpace(in.NumeroOperacion), in.Modalidad,
		strings.TrimSpace(in.Comentario), ticketID, egresoID, se.UsuarioID).Scan(&pagoID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"pago_id": pagoID, "egreso_id": egresoID, "saldo_cts": saldo - in.MontoCts, "periodo": periodo})
}

// idURL lee un id numérico de la ruta.
func idURL(r *http.Request, clave string) int64 {
	v, _ := strconv.ParseInt(chi.URLParam(r, clave), 10, 64)
	return v
}

// fechaOpc parsea AAAA-MM-DD; vacío → nil; error → campo.
func fechaOpc(s string, ev *P.Error, campo string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		ev.Campo(campo, "Formato AAAA-MM-DD.")
		return nil
	}
	return &t
}

// esUnico detecta la violación de unicidad de Postgres (23505).
func esUnico(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}
