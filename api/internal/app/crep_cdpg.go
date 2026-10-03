package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Cuenta recaudadora en banco (bloque A2), todo por archivo: EDISYS genera el CREP del periodo, el usuario
// lo descarga («Descargas», retenido 24 h) y lo sube al banco a mano; el banco devuelve el CDPG con los
// pagos y el usuario lo sube en «Cobranza masiva». Cada pago se empareja con su recibo (por la referencia,
// que es el número del recibo, o por el código de la unidad) y queda como pago validado con su código de
// operación: la conciliación bancaria general lo encuentra después en el extracto por ese mismo código.

// listarLayoutsCrep: GET /crep/layouts
func (s *Server) listarLayoutsCrep(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	for _, l := range layouts {
		out = append(out, map[string]any{"codigo": l.Codigo(), "nombre": l.Nombre(), "provisional": l.Provisional()})
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": out, "total": len(out), "pagina": 1})
}

// generarCrep: POST /crep {periodo, cuenta_bancaria_id, layout?} → archivo listo para descargar durante 24 h.
func (s *Server) generarCrep(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	var in struct {
		Periodo          string `json:"periodo"`
		CuentaBancariaID int64  `json:"cuenta_bancaria_id"`
		Layout           string `json:"layout"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa los datos del CREP.")
	if !P.PeriodoValido(in.Periodo) {
		ev.Campo("periodo", "Formato AAAA-MM.")
	}
	l, ok := LayoutDeBanco(in.Layout)
	if !ok {
		ev.Campo("layout", "Layout de banco desconocido.")
	}
	var numero, moneda string
	if err := s.DB.QueryRow(ctx, `SELECT numero, moneda FROM cuenta_bancaria WHERE id=$1 AND edificio_id=$2`, in.CuentaBancariaID, e.ID).Scan(&numero, &moneda); err != nil {
		ev.Campo("cuenta_bancaria_id", "Elige la cuenta recaudadora del edificio.")
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	var empresa string
	_ = s.DB.QueryRow(ctx, `SELECT nombre FROM edificio WHERE id=$1`, e.ID).Scan(&empresa)
	recibos, err := recibosConDeuda(ctx, s.DB, e.ID, in.Periodo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if len(recibos) == 0 {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "SIN_DEUDAS", "El periodo no tiene recibos con saldo."))
		return
	}
	hoy := time.Now().In(P.Lima)
	dets := make([]CrepDetalle, 0, len(recibos))
	for _, rc := range recibos {
		d := CrepDetalle{Depositante: rc["unidad"].(string), Nombre: rc["titular"].(string), Referencia: rc["numero"].(string),
			Emision: hoy, Vence: hoy, MontoCts: rc["saldo_cts"].(int64)}
		if s, _ := rc["emitido"].(string); s != "" {
			d.Emision, _ = time.Parse("2006-01-02", s)
		}
		if s, _ := rc["vence"].(string); s != "" {
			d.Vence, _ = time.Parse("2006-01-02", s)
		}
		dets = append(dets, d)
	}
	cab := CrepCabecera{Cuenta: numero, Moneda: moneda, Empresa: empresa, Fecha: hoy}
	contenido, err := GenerarCREP(l, cab, dets)
	if err != nil {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "CREP_INVALIDO", err.Error()).Campo("cuenta_bancaria_id", err.Error()))
		return
	}
	var total int64
	for _, d := range dets {
		total += d.MontoCts
	}
	nombre := "crep-" + in.Periodo + "-" + hoy.Format("20060102-1504") + ".txt"
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var id int64
	var expira time.Time
	if err := tx.QueryRow(ctx, `INSERT INTO crep_archivo (edificio_id, periodo, cuenta_bancaria_id, layout, nombre_archivo, contenido, filas, total_cts, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, expira_en`, e.ID, in.Periodo, in.CuentaBancariaID, l.Codigo(), nombre, contenido, len(dets), total, ses(r).UsuarioID).
		Scan(&id, &expira); err != nil {
		P.Fallo(w, r, err)
		return
	}
	marcados, err := marcarEnviados(ctx, tx, e.ID, in.Periodo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "nombre_archivo": nombre, "filas": len(dets), "total_cts": total, "estado": "completado",
		"expira_en": expira.In(P.Lima).Format("2006-01-02 15:04"), "provisional": l.Provisional(), "recibos_marcados": marcados})
}

// expirarCreps borra el contenido de los CREP vencidos (la fila queda como rastro).
func expirarCreps(r *http.Request, q db.Q, eid int64) error {
	_, err := q.Exec(r.Context(), `UPDATE crep_archivo SET estado='expirado', contenido=NULL WHERE edificio_id=$1 AND expira_en < now() AND estado <> 'expirado'`, eid)
	return err
}

// listarCrep: GET /crep → «Descargas»: archivos con estado y expiración.
func (s *Server) listarCrep(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	if err := expirarCreps(r, s.DB, e.ID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	filas, err := db.Filas(r.Context(), s.DB, `SELECT c.id, 'Recibos Mantenimiento CREP' AS tipo, c.periodo, c.layout, c.nombre_archivo, c.filas, c.total_cts, c.estado,
			COALESCE(b.banco,'') AS banco, COALESCE(b.numero,'') AS cuenta,
			to_char(c.creado_en AT TIME ZONE 'America/Lima','YYYY-MM-DD HH24:MI') AS creado,
			to_char(c.expira_en AT TIME ZONE 'America/Lima','YYYY-MM-DD HH24:MI') AS expira
		FROM crep_archivo c LEFT JOIN cuenta_bancaria b ON b.id=c.cuenta_bancaria_id
		WHERE c.edificio_id=$1 ORDER BY c.creado_en DESC, c.id DESC LIMIT 100`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// descargarCrep: GET /crep/{id}/descargar → el TXT; 410 CREP_EXPIRADO pasadas las 24 h.
func (s *Server) descargarCrep(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var contenido *string
	var nombre string
	var vencido bool
	if err := s.DB.QueryRow(r.Context(), `SELECT contenido, nombre_archivo, expira_en < now() FROM crep_archivo WHERE id=$1 AND edificio_id=$2`,
		idURL(r, "id"), e.ID).Scan(&contenido, &nombre, &vencido); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el archivo"))
		return
	}
	if vencido || contenido == nil {
		_ = expirarCreps(r, s.DB, e.ID)
		P.Fallo(w, r, P.Err(http.StatusGone, "CREP_EXPIRADO", "El archivo expiró (se guarda 24 h). Genera uno nuevo."))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=us-ascii")
	w.Header().Set("Content-Disposition", `attachment; filename="`+nombre+`"`)
	_, _ = w.Write([]byte(*contenido))
}

// subirCdpg: POST /cdpg (multipart {archivo, layout?, crep_archivo_id?, vista_previa?}) → «Cobranza masiva».
// Con vista_previa=1 se empareja todo dentro de una transacción que se deshace: lo que muestra es exacto.
func (s *Server) subirCdpg(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	nombre, datos, err := archivoSubido(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	l, ok := LayoutDeBanco(campo(r, "layout"))
	if !ok {
		P.Fallo(w, r, P.Validacion("Layout de banco desconocido.").Campo("layout", "Elige el banco."))
		return
	}
	var crepID *int64
	if v, _ := strconv.ParseInt(campo(r, "crep_archivo_id"), 10, 64); v > 0 {
		var ok bool
		_ = s.DB.QueryRow(ctx, `SELECT true FROM crep_archivo WHERE id=$1 AND edificio_id=$2`, v, e.ID).Scan(&ok)
		if ok {
			crepID = &v
		}
	}
	filas, errLectura := LeerCDPG(l, datos)
	if len(filas) == 0 && len(errLectura) == 0 {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "CDPG_VACIO", "El archivo no trae pagos.").Campo("archivo", "Sin registros de detalle."))
		return
	}
	previa := siVistaPrevia(r)
	uid := ses(r).UsuarioID

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var cargaID int64
	if err := tx.QueryRow(ctx, `INSERT INTO cdpg_carga (edificio_id, crep_archivo_id, layout, nombre_archivo, creado_por) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		e.ID, crepID, l.Codigo(), nombre, uid).Scan(&cargaID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	resultado := []map[string]any{}
	errores := append([]map[string]any{}, errLectura...)
	var nOK, malas int
	var total int64
	for _, f := range filas {
		fila := map[string]any{"linea": f.Linea, "codigo_depositante": f.Depositante, "referencia": f.Referencia, "fecha": f.Fecha,
			"monto_cts": f.MontoCts, "agencia": f.Agencia, "numero_operacion": f.NumeroOperacion}
		estado, motivo := func() (string, string) {
			var ya bool
			_ = tx.QueryRow(ctx, `SELECT true FROM cdpg_movimiento WHERE edificio_id=$1 AND fecha=$2 AND agencia=$3 AND numero_operacion=$4`,
				e.ID, f.Fecha, f.Agencia, f.NumeroOperacion).Scan(&ya)
			if ya {
				return "ya_conciliado", "Ese pago ya se concilió en una carga anterior."
			}
			sp, err := tx.Begin(ctx)
			if err != nil {
				return "error", err.Error()
			}
			defer sp.Rollback(ctx)
			obj, err := resolverRecibo(ctx, sp, e.ID, f.Referencia, f.Depositante)
			if err != nil {
				return "error", mensajeDe(err)
			}
			fila["recibo"], fila["unidad"] = obj.Numero, obj.Unidad
			apls, err := s.acreditarRecibo(ctx, sp, e.ID, obj, &uid, f.MontoCts, "deposito", f.NumeroOperacion, f.Fecha)
			if err != nil {
				return "error", mensajeDe(err)
			}
			var pagoID *int64
			if len(apls) > 0 {
				pagoID = &apls[0].PagoID
			}
			if _, err := sp.Exec(ctx, `INSERT INTO cdpg_movimiento (edificio_id, carga_id, fecha, agencia, numero_operacion, codigo_depositante, referencia, monto_cts, recibo_id, pago_id)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, e.ID, cargaID, f.Fecha, f.Agencia, f.NumeroOperacion, f.Depositante, f.Referencia, f.MontoCts, obj.ID, pagoID); err != nil {
				if esUnico(err) {
					return "ya_conciliado", "Ese pago viene repetido en el archivo."
				}
				return "error", err.Error()
			}
			if err := sp.Commit(ctx); err != nil {
				return "error", err.Error()
			}
			return "emparejado", ""
		}()
		fila["estado"], fila["motivo"] = estado, motivo
		if estado == "emparejado" {
			nOK++
			total += f.MontoCts
		} else {
			malas++
			errores = append(errores, map[string]any{"linea": f.Linea, "motivo": motivo})
		}
		resultado = append(resultado, fila)
	}
	malas += len(errLectura)
	if !previa {
		b, _ := json.Marshal(errores)
		if _, err := tx.Exec(ctx, `UPDATE cdpg_carga SET filas_ok=$1, filas_error=$2, total_cts=$3, errores=$4 WHERE id=$5`, nOK, malas, total, b, cargaID); err != nil {
			P.Fallo(w, r, err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	resp := map[string]any{"vista_previa": previa, "archivo": nombre, "layout": l.Codigo(), "provisional": l.Provisional(),
		"filas_ok": nOK, "filas_error": malas, "total_cts": total, "filas": resultado, "errores_lectura": errLectura}
	if !previa {
		resp["carga_id"] = cargaID
	}
	P.JSON(w, http.StatusOK, resp)
}

// listarCdpg: GET /cdpg → cargas de cobranza masiva.
func (s *Server) listarCdpg(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT c.id, c.layout, c.nombre_archivo, c.filas_ok, c.filas_error, c.total_cts, c.errores,
			to_char(c.creado_en AT TIME ZONE 'America/Lima','YYYY-MM-DD HH24:MI') AS creado, COALESCE(k.nombre_archivo,'') AS crep
		FROM cdpg_carga c LEFT JOIN crep_archivo k ON k.id=c.crep_archivo_id
		WHERE c.edificio_id=$1 ORDER BY c.creado_en DESC, c.id DESC LIMIT 100`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}
