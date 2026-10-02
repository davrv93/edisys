package app

import (
	"net/http"
	"strings"
	"time"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Vouchers multicuenta (bloque A3): cuentas bancarias del edificio y registro de un voucher
// aplicado a uno o varios recibos de la unidad. El voucher del propietario queda por validar.

// listarCuentasBancarias: GET /cuentas-bancarias
func (s *Server) listarCuentasBancarias(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT id, banco, numero, moneda, activo
		FROM cuenta_bancaria WHERE edificio_id=$1 ORDER BY activo DESC, banco`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearCuentaBancaria: POST /cuentas-bancarias
func (s *Server) crearCuentaBancaria(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in struct {
		Banco  string `json:"banco"`
		Numero string `json:"numero"`
		Moneda string `json:"moneda"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.Banco = strings.TrimSpace(in.Banco)
	if in.Banco == "" {
		P.Fallo(w, r, P.Validacion("Escribe el banco.").Campo("banco", "Obligatorio."))
		return
	}
	if in.Moneda != "USD" {
		in.Moneda = "PEN"
	}
	var id int64
	err := s.DB.QueryRow(r.Context(), `INSERT INTO cuenta_bancaria (edificio_id, banco, numero, moneda) VALUES ($1,$2,$3,$4) RETURNING id`,
		e.ID, in.Banco, strings.TrimSpace(in.Numero), in.Moneda).Scan(&id)
	if err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("CUENTA_DUPLICADA", "Ya existe esa cuenta bancaria."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// editarCuentaBancaria: PUT /cuentas-bancarias/{id}
func (s *Server) editarCuentaBancaria(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	var in struct {
		Banco  string `json:"banco"`
		Numero string `json:"numero"`
		Activo *bool  `json:"activo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE cuenta_bancaria SET banco=$1, numero=$2, activo=COALESCE($3, activo) WHERE id=$4 AND edificio_id=$5`,
		strings.TrimSpace(in.Banco), strings.TrimSpace(in.Numero), in.Activo, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("cuenta bancaria"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

// registrarVoucher: POST /unidades/{uid}/vouchers
// Cuerpo: {cuenta_bancaria_id?, medio?, codigo_operacion, fecha?, aplicaciones?:[{recibo_id, monto_cts}]}.
// Sin aplicaciones, se reparte del cargo más antiguo al más nuevo (como el pago a cuenta).
func (s *Server) registrarVoucher(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	uid := idURL(r, "uid")
	admin := e.Puede("pagos.registrar")
	if !admin && !e.Puede("pagos.informar") {
		P.Fallo(w, r, P.Prohibido("SIN_PERMISO", "No puedes registrar vouchers."))
		return
	}
	if e.SoloLoSuyo() && !e.EsSuya(uid) {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	var in struct {
		CuentaBancariaID int64  `json:"cuenta_bancaria_id"`
		Medio            string `json:"medio"`
		CodigoOperacion  string `json:"codigo_operacion"`
		Fecha            string `json:"fecha"`
		MontoCts         int64  `json:"monto_cts"`
		Aplicaciones     []struct {
			ReciboID int64 `json:"recibo_id"`
			MontoCts int64 `json:"monto_cts"`
		} `json:"aplicaciones"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	var codigoU string
	if err := s.DB.QueryRow(ctx, `SELECT codigo FROM unidad WHERE id=$1 AND edificio_id=$2`, uid, e.ID).Scan(&codigoU); err != nil {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	if in.Medio == "" {
		in.Medio = "transferencia"
	}
	if in.MontoCts <= 0 && len(in.Aplicaciones) > 0 {
		for _, a := range in.Aplicaciones {
			in.MontoCts += a.MontoCts
		}
	}
	ev := P.Validacion("Revisa el voucher.")
	if in.MontoCts <= 0 {
		ev.Campo("monto_cts", "El monto debe ser mayor que cero.")
	}
	if strings.TrimSpace(in.CodigoOperacion) == "" {
		ev.Campo("codigo_operacion", "Escribe el código de operación.")
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
	if in.CuentaBancariaID > 0 {
		var ok bool
		_ = tx.QueryRow(ctx, `SELECT true FROM cuenta_bancaria WHERE id=$1 AND edificio_id=$2`, in.CuentaBancariaID, e.ID).Scan(&ok)
		if !ok {
			P.Fallo(w, r, P.Validacion("Elige una cuenta del edificio.").Campo("cuenta_bancaria_id", "No válida."))
			return
		}
	}
	cargos, err := cargosPendientes(ctx, tx, uid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	saldo := map[int64]int64{}
	var orden []int64
	for _, c := range cargos {
		saldo[c.id] = c.saldo
		orden = append(orden, c.id)
	}
	// Aplicaciones: explícitas o repartidas del más antiguo al más nuevo.
	type apl struct{ recibo, monto int64 }
	var aplicaciones []apl
	if len(in.Aplicaciones) > 0 {
		var suma int64
		for _, a := range in.Aplicaciones {
			if _, ok := saldo[a.ReciboID]; !ok {
				P.Fallo(w, r, P.Validacion("El recibo no es de la unidad o no tiene saldo.").Campo("aplicaciones", "Revisa los recibos."))
				return
			}
			if a.MontoCts > saldo[a.ReciboID] {
				P.Fallo(w, r, P.Validacion("Un monto pasa el saldo del recibo.").Campo("aplicaciones", "Revisa los montos."))
				return
			}
			aplicaciones = append(aplicaciones, apl{a.ReciboID, a.MontoCts})
			suma += a.MontoCts
		}
		if suma != in.MontoCts {
			P.Fallo(w, r, P.Validacion("La suma de las aplicaciones no coincide con el monto.").Campo("monto_cts", "Debe coincidir."))
			return
		}
	} else {
		saldos := make([]int64, len(orden))
		for i, id := range orden {
			saldos[i] = saldo[id]
		}
		partes := Repartir(in.MontoCts, saldos)
		for i, m := range partes {
			if m > 0 {
				aplicaciones = append(aplicaciones, apl{orden[i], m})
			}
		}
	}
	estado := "pendiente_validacion"
	var validadoPor *int64
	if admin {
		estado = "validado"
		validadoPor = &ses(r).UsuarioID
	}
	var cuenta *int64
	if in.CuentaBancariaID > 0 {
		cuenta = &in.CuentaBancariaID
	}
	codigo := strings.TrimSpace(in.CodigoOperacion)
	res := make([]map[string]any, 0, len(aplicaciones))
	for i, a := range aplicaciones {
		var cod *string
		if i == 0 {
			cod = &codigo // el código se guarda en la parte 1 (índice de unicidad)
		}
		var pid int64
		if err := tx.QueryRow(ctx, `INSERT INTO pago (edificio_id, recibo_id, monto_cts, medio, codigo_operacion, fecha, estado, registrado_por, validado_por, validado_en, parte, cuenta_bancaria_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, CASE WHEN $9::bigint IS NULL THEN NULL ELSE now() END, $10, $11) RETURNING id`,
			e.ID, a.recibo, a.monto, in.Medio, cod, fecha.Format("2006-01-02"), estado, ses(r).UsuarioID, validadoPor, i+1, cuenta).Scan(&pid); err != nil {
			P.Fallo(w, r, P.Traducir(err))
			return
		}
		if admin {
			if err := s.asentarIngresoPago(ctx, tx, e.ID, pid, a.recibo, a.monto, fecha); err != nil {
				P.Fallo(w, r, err)
				return
			}
		}
		res = append(res, map[string]any{"pago_id": pid, "recibo_id": a.recibo, "monto_cts": a.monto})
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"unidad": codigoU, "estado": estado, "monto_cts": in.MontoCts, "aplicaciones": res})
}
