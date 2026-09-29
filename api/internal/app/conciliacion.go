package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/conciliacion"
	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Conciliación bancaria (bloque 4): el extracto del banco (CSV o XLSX) contra los pagos y egresos del sistema.

// EstadoCon resume la conciliación de un periodo.
type EstadoCon struct {
	ExtractoID   int64  `json:"extracto_id"`
	Banco        string `json:"banco"`
	SaldoBanco   int64  `json:"saldo_banco_cts"`
	SaldoSistema int64  `json:"saldo_sistema_cts"`
	Diferencia   int64  `json:"diferencia_cts"`
	Conciliados  int    `json:"conciliados"`
	Sugeridos    int    `json:"sugeridos"`
	SinPareja    int    `json:"sin_pareja"`
	Fecha        string `json:"fecha"` // último día cubierto por el extracto
	Conciliado   bool   `json:"conciliado"`
	Texto        string `json:"texto"`
}

// EstadoConciliacion: nil si el periodo no tiene extracto.
func (s *Server) EstadoConciliacion(ctx context.Context, eid int64, periodo string) (*EstadoCon, error) {
	c := &EstadoCon{}
	var hasta time.Time
	err := s.DB.QueryRow(ctx, `SELECT x.id, x.banco, x.saldo_final_cts,
			count(mb.id) FILTER (WHERE mb.estado='conciliado'), count(mb.id) FILTER (WHERE mb.estado='sugerido'), count(mb.id) FILTER (WHERE mb.estado='sin_pareja'),
			COALESCE(max(mb.fecha), (x.periodo || '-01')::date)
		FROM extracto x LEFT JOIN movimiento_banco mb ON mb.extracto_id=x.id
		WHERE x.edificio_id=$1 AND x.periodo=$2 GROUP BY x.id ORDER BY x.creado_en DESC LIMIT 1`, eid, periodo).
		Scan(&c.ExtractoID, &c.Banco, &c.SaldoBanco, &c.Conciliados, &c.Sugeridos, &c.SinPareja, &hasta)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a, err := s.ArbolBalance(ctx, s.DB, eid, periodo, vistaCompleta())
	if err != nil {
		return nil, err
	}
	c.SaldoSistema = a.KPIs.BancoCts
	c.Diferencia = c.SaldoBanco - c.SaldoSistema
	c.Fecha = hasta.Format("2006-01-02")
	c.Conciliado = c.Diferencia == 0 && c.Sugeridos == 0 && c.SinPareja == 0
	if c.Conciliado {
		c.Texto = "Conciliado con el banco al " + hasta.Format("02/01") + "."
	} else {
		c.Texto = fmt.Sprintf("Conciliación con el banco al %s: diferencia pendiente de %s (%d sin pareja, %d por confirmar).",
			hasta.Format("02/01"), P.Soles(c.Diferencia), c.SinPareja, c.Sugeridos)
	}
	return c, nil
}

func (s *Server) textoConciliacion(ctx context.Context, eid int64, periodo string) string {
	c, err := s.EstadoConciliacion(ctx, eid, periodo)
	if err != nil || c == nil {
		return ""
	}
	return c.Texto
}

// verConciliacion: GET /conciliacion?periodo= → {estado, banco: movimientos, sistema: pagos y egresos}.
func (s *Server) verConciliacion(w http.ResponseWriter, r *http.Request) {
	periodo, err := s.periodoDe(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	est, err := s.EstadoConciliacion(ctx, e.ID, periodo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	resp := map[string]any{"periodo": periodo, "estado": est, "banco": []any{}, "sistema": []any{}}
	if est == nil {
		var bancos []string
		_ = s.DB.QueryRow(ctx, `SELECT COALESCE(array_agg(banco ORDER BY banco), '{}') FROM banco_mapeo WHERE edificio_id=$1`, e.ID).Scan(&bancos)
		resp["bancos"] = bancos
		P.JSON(w, http.StatusOK, resp)
		return
	}
	movs, err := db.Filas(ctx, s.DB, `SELECT mb.id, to_char(mb.fecha,'YYYY-MM-DD') AS fecha, mb.descripcion, mb.monto_cts, mb.codigo_operacion, mb.estado, mb.regla,
			mb.pago_id, mb.egreso_id,
			CASE WHEN mb.pago_id IS NOT NULL THEN 'Pago ' || pg.medio || ' · Dpto ' || u.codigo || ' · ' || to_char(pg.fecha,'DD/MM')
			     WHEN mb.egreso_id IS NOT NULL THEN eg.descripcion || ' · ' || to_char(eg.fecha,'DD/MM') END AS pareja
		FROM movimiento_banco mb LEFT JOIN pago pg ON pg.id=mb.pago_id LEFT JOIN recibo rc ON rc.id=pg.recibo_id LEFT JOIN unidad u ON u.id=rc.unidad_id
		LEFT JOIN egreso eg ON eg.id=mb.egreso_id
		WHERE mb.extracto_id=$1 ORDER BY mb.fecha, mb.id`, est.ExtractoID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	items, err := conciliacion.ItemsSistema(ctx, s.DB, e.ID, periodo, est.ExtractoID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	usado := map[string]string{}
	for _, m := range movs {
		if v, ok := m["pago_id"].(int64); ok {
			usado[fmt.Sprintf("pago%d", v)] = m["estado"].(string)
		}
		if v, ok := m["egreso_id"].(int64); ok {
			usado[fmt.Sprintf("egreso%d", v)] = m["estado"].(string)
		}
	}
	sistema := make([]map[string]any, 0, len(items))
	for _, it := range items {
		estado := usado[fmt.Sprintf("%s%d", it.Tipo, it.ID)]
		if estado == "" {
			estado = conciliacion.SinPareja
		}
		sistema = append(sistema, map[string]any{"tipo": it.Tipo, "id": it.ID, "fecha": it.Fecha, "monto_cts": it.MontoCts, "codigo_operacion": it.Codigo,
			"descripcion": it.Descripcion, "estado": estado})
	}
	resp["banco"] = movs
	resp["sistema"] = sistema
	P.JSON(w, http.StatusOK, resp)
}

// leerExtracto: el archivo subido y sus cabeceras.
func leerExtracto(r *http.Request) (string, []string, [][]string, error) {
	if err := leerMultipart(r); err != nil {
		return "", nil, nil, err
	}
	fh := firstFile(r, "archivo", "extracto")
	if fh == nil {
		return "", nil, nil, P.Validacion("Sube el extracto (CSV o XLSX).").Campo("archivo", "Obligatorio.")
	}
	if fh.Size > MaxArchivo {
		return "", nil, nil, P.Err(http.StatusRequestEntityTooLarge, "ARCHIVO_GRANDE", "El archivo pasa de 10 MB.")
	}
	f, err := fh.Open()
	if err != nil {
		return "", nil, nil, err
	}
	datos, err := io.ReadAll(io.LimitReader(f, MaxArchivo+1))
	f.Close()
	if err != nil {
		return "", nil, nil, err
	}
	cab, filas, err := conciliacion.Leer(fh.Filename, datos)
	if err != nil {
		return "", nil, nil, P.Err(http.StatusUnprocessableEntity, "EXTRACTO_INVALIDO", err.Error()).Campo("archivo", err.Error())
	}
	return fh.Filename, cab, filas, nil
}

func (s *Server) mapeoGuardado(ctx context.Context, eid int64, banco string) (conciliacion.Mapeo, bool) {
	var m conciliacion.Mapeo
	var b []byte
	if err := s.DB.QueryRow(ctx, `SELECT mapeo FROM banco_mapeo WHERE edificio_id=$1 AND banco=$2`, eid, banco).Scan(&b); err != nil {
		return m, false
	}
	return m, json.Unmarshal(b, &m) == nil
}

// columnasExtracto: POST /conciliacion/columnas (multipart {archivo, banco?}) → cabeceras, primeras filas y mapeo sugerido.
func (s *Server) columnasExtracto(w http.ResponseWriter, r *http.Request) {
	_, cab, filas, err := leerExtracto(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	banco := strings.TrimSpace(campo(r, "banco"))
	m, guardado := s.mapeoGuardado(r.Context(), edf(r).ID, banco)
	if !guardado {
		m = conciliacion.AutoMapeo(cab)
	}
	if len(filas) > 5 {
		filas = filas[:5]
	}
	P.JSON(w, http.StatusOK, map[string]any{"cabeceras": cab, "muestra": filas, "mapeo": m, "mapeo_guardado": guardado})
}

// subirExtracto: POST /conciliacion/extractos (multipart {archivo, banco, periodo, col_fecha, col_descripcion, col_monto |
// col_cargo + col_abono, col_codigo, col_saldo, saldo_final_cts?}). Guarda el mapeo por banco y empareja.
func (s *Server) subirExtracto(w http.ResponseWriter, r *http.Request) {
	nombre, cab, filas, err := leerExtracto(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	banco := strings.ToUpper(strings.TrimSpace(campo(r, "banco")))
	periodo := campo(r, "periodo")
	ev := P.Validacion("Revisa los datos del extracto.")
	if banco == "" {
		ev.Campo("banco", "Indica el banco (p. ej. BCP).")
	}
	if !P.PeriodoValido(periodo) {
		ev.Campo("periodo", "Formato AAAA-MM.")
	}
	m, guardado := s.mapeoGuardado(ctx, e.ID, banco)
	if !guardado {
		m = conciliacion.AutoMapeo(cab)
	}
	for campoF, dst := range map[string]*string{"col_fecha": &m.Fecha, "col_descripcion": &m.Descripcion, "col_monto": &m.Monto, "col_cargo": &m.Cargo,
		"col_abono": &m.Abono, "col_codigo": &m.Codigo, "col_saldo": &m.Saldo} {
		if v := campo(r, campoF); v != "" || r.MultipartForm.Value[campoF] != nil {
			*dst = v
		}
	}
	if !m.Completo() {
		ev.Campo("col_fecha", "Elige la columna de fecha.").Campo("col_monto", "Elige la columna del monto (o cargo y abono).")
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev.Con("cabeceras", cab).Con("mapeo", m))
		return
	}
	movs, errs := conciliacion.Aplicar(cab, filas, m)
	if len(movs) == 0 {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "EXTRACTO_VACIO", "No encontré movimientos con ese mapeo.").Con("errores", errs).Con("cabeceras", cab))
		return
	}
	saldo, ok := conciliacion.SaldoFinal(cab, filas, m)
	if v := campo(r, "saldo_final_cts"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			saldo, ok = n, true
		}
	}
	if !ok {
		P.Fallo(w, r, P.Validacion("Falta el saldo final del banco: mapea la columna Saldo o escribe el saldo final.").Campo("saldo_final_cts", "Saldo al cierre en céntimos."))
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	mb, _ := json.Marshal(m)
	if _, err := tx.Exec(ctx, `INSERT INTO banco_mapeo (edificio_id, banco, mapeo) VALUES ($1,$2,$3)
		ON CONFLICT (edificio_id, banco) DO UPDATE SET mapeo=EXCLUDED.mapeo, actualizado_en=now()`, e.ID, banco, mb); err != nil {
		P.Fallo(w, r, err)
		return
	}
	uid := ses(r).UsuarioID
	if _, err := conciliacion.Guardar(ctx, tx, e.ID, banco, periodo, nombre, saldo, movs, &uid); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	est, _ := s.EstadoConciliacion(ctx, e.ID, periodo)
	P.JSON(w, http.StatusCreated, map[string]any{"periodo": periodo, "movimientos": len(movs), "filas_con_error": errs, "mapeo": m, "estado": est})
}

// movimiento del edificio, bloqueado para cambiarlo.
func movimiento(ctx context.Context, tx pgx.Tx, eid, mid int64) (map[string]any, error) {
	m, err := db.Fila(ctx, tx, `SELECT mb.id, mb.extracto_id, mb.estado, mb.monto_cts, to_char(mb.fecha,'YYYY-MM-DD') AS fecha, mb.descripcion, mb.codigo_operacion,
			mb.pago_id, mb.egreso_id, x.periodo
		FROM movimiento_banco mb JOIN extracto x ON x.id=mb.extracto_id WHERE mb.id=$1 AND mb.edificio_id=$2 FOR UPDATE OF mb`, mid, eid)
	if err != nil {
		return nil, P.NoEncontrado("el movimiento del banco")
	}
	return m, nil
}

func (s *Server) conTx(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, tx pgx.Tx, e *Edificio, mid int64) (map[string]any, error)) {
	mid, err := idRuta(r, "mid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	out, err := fn(ctx, tx, edf(r), mid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, P.Traducir(err))
		return
	}
	if p, ok := out["periodo"].(string); ok {
		out["estado_conciliacion"], _ = s.EstadoConciliacion(ctx, edf(r).ID, p)
	}
	P.JSON(w, http.StatusOK, out)
}

// confirmarMovimiento: POST /conciliacion/movimientos/{mid}/confirmar {pago_id?|egreso_id?}. Sin cuerpo confirma la sugerencia.
func (s *Server) confirmarMovimiento(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PagoID   *int64 `json:"pago_id"`
		EgresoID *int64 `json:"egreso_id"`
	}
	if r.ContentLength > 0 {
		if err := P.Leer(r, &in); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	uid := ses(r).UsuarioID
	s.conTx(w, r, func(ctx context.Context, tx pgx.Tx, e *Edificio, mid int64) (map[string]any, error) {
		m, err := movimiento(ctx, tx, e.ID, mid)
		if err != nil {
			return nil, err
		}
		if m["estado"] == conciliacion.Conciliado {
			return nil, P.Conflicto("YA_CONCILIADO", "Ese movimiento ya está conciliado.")
		}
		pago, egreso := in.PagoID, in.EgresoID
		regla := conciliacion.ReglaManual
		if pago == nil && egreso == nil {
			if m["estado"] != conciliacion.Sugerido {
				return nil, P.Validacion("Elige el pago o el egreso que corresponde.").Campo("pago_id", "O egreso_id.")
			}
			if v, ok := m["pago_id"].(int64); ok {
				pago = &v
			}
			if v, ok := m["egreso_id"].(int64); ok {
				egreso = &v
			}
			regla = ""
		}
		if pago != nil {
			var ok bool
			_ = tx.QueryRow(ctx, `SELECT true FROM pago WHERE id=$1 AND edificio_id=$2 AND estado='validado'`, *pago, e.ID).Scan(&ok)
			if !ok {
				return nil, P.NoEncontrado("el pago validado")
			}
			egreso = nil
		} else {
			var ok bool
			_ = tx.QueryRow(ctx, `SELECT true FROM egreso WHERE id=$1 AND edificio_id=$2`, *egreso, e.ID).Scan(&ok)
			if !ok {
				return nil, P.NoEncontrado("el egreso")
			}
		}
		// Si otro movimiento lo tenía como sugerencia, esa sugerencia se libera.
		if _, err := tx.Exec(ctx, `UPDATE movimiento_banco SET estado='sin_pareja', regla='', pago_id=NULL, egreso_id=NULL
			WHERE id <> $1 AND estado='sugerido' AND ((pago_id=$2 AND $2 IS NOT NULL) OR (egreso_id=$3 AND $3 IS NOT NULL))`, mid, pago, egreso); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE movimiento_banco SET estado='conciliado', pago_id=$2, egreso_id=$3, regla=COALESCE(NULLIF($4,''), regla),
			confirmado_por=$5, confirmado_en=now() WHERE id=$1`, mid, pago, egreso, regla, uid); err != nil {
			return nil, P.Traducir(err)
		}
		if egreso != nil {
			_, _ = tx.Exec(ctx, `UPDATE egreso SET movimiento_banco_id=$2 WHERE id=$1`, *egreso, mid)
		}
		return map[string]any{"id": mid, "estado": conciliacion.Conciliado, "pago_id": pago, "egreso_id": egreso, "periodo": m["periodo"]}, nil
	})
}

// deshacerMovimiento: POST /conciliacion/movimientos/{mid}/deshacer — vuelve a «sin pareja».
func (s *Server) deshacerMovimiento(w http.ResponseWriter, r *http.Request) {
	s.conTx(w, r, func(ctx context.Context, tx pgx.Tx, e *Edificio, mid int64) (map[string]any, error) {
		m, err := movimiento(ctx, tx, e.ID, mid)
		if err != nil {
			return nil, err
		}
		if v, ok := m["egreso_id"].(int64); ok {
			_, _ = tx.Exec(ctx, `UPDATE egreso SET movimiento_banco_id=NULL WHERE id=$1 AND movimiento_banco_id=$2`, v, mid)
		}
		if _, err := tx.Exec(ctx, `UPDATE movimiento_banco SET estado='sin_pareja', regla='', pago_id=NULL, egreso_id=NULL, confirmado_por=NULL, confirmado_en=NULL WHERE id=$1`, mid); err != nil {
			return nil, err
		}
		return map[string]any{"id": mid, "estado": conciliacion.SinPareja, "periodo": m["periodo"]}, nil
	})
}

// confirmarSugeridos: POST /conciliacion/confirmar-sugeridos {periodo}.
func (s *Server) confirmarSugeridos(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Periodo string `json:"periodo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	tag, err := s.DB.Exec(ctx, `UPDATE movimiento_banco mb SET estado='conciliado', confirmado_por=$3, confirmado_en=now()
		FROM extracto x WHERE x.id=mb.extracto_id AND x.edificio_id=$1 AND x.periodo=$2 AND mb.estado='sugerido'`, e.ID, in.Periodo, ses(r).UsuarioID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	_, _ = s.DB.Exec(ctx, `UPDATE egreso eg SET movimiento_banco_id=mb.id FROM movimiento_banco mb JOIN extracto x ON x.id=mb.extracto_id
		WHERE mb.egreso_id=eg.id AND mb.estado='conciliado' AND x.edificio_id=$1 AND x.periodo=$2`, e.ID, in.Periodo)
	est, _ := s.EstadoConciliacion(ctx, e.ID, in.Periodo)
	P.JSON(w, http.StatusOK, map[string]any{"confirmados": tag.RowsAffected(), "estado": est})
}

// crearEgresoDesdeMovimiento: POST /conciliacion/movimientos/{mid}/crear-egreso {rubro_id, concepto?, descripcion?}.
func (s *Server) crearEgresoDesdeMovimiento(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RubroID     int64  `json:"rubro_id"`
		Rubro       string `json:"rubro"`
		Concepto    string `json:"concepto"`
		Descripcion string `json:"descripcion"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	uid := ses(r).UsuarioID
	s.conTx(w, r, func(ctx context.Context, tx pgx.Tx, e *Edificio, mid int64) (map[string]any, error) {
		m, err := movimiento(ctx, tx, e.ID, mid)
		if err != nil {
			return nil, err
		}
		if m["estado"] != conciliacion.SinPareja {
			return nil, P.Conflicto("MOVIMIENTO_CON_PAREJA", "Ese movimiento ya tiene pareja: deshazla primero.")
		}
		monto := m["monto_cts"].(int64)
		if monto >= 0 {
			return nil, P.Validacion("Es un abono: crea un ingreso, no un egreso.")
		}
		if in.RubroID == 0 && in.Rubro != "" {
			_ = tx.QueryRow(ctx, `SELECT id FROM rubro WHERE edificio_id=$1 AND (slug=$2 OR lower(nombre)=lower($2))`, e.ID, in.Rubro).Scan(&in.RubroID)
		}
		var ok bool
		_ = tx.QueryRow(ctx, `SELECT true FROM rubro WHERE id=$1 AND edificio_id=$2`, in.RubroID, e.ID).Scan(&ok)
		if !ok {
			return nil, P.Validacion("Elige un rubro del edificio.").Campo("rubro_id", "Obligatorio.")
		}
		desc := strings.TrimSpace(in.Descripcion)
		if desc == "" {
			desc = mayus(strings.ToLower(m["descripcion"].(string)))
		}
		var concepto *int64
		if c := strings.TrimSpace(in.Concepto); c != "" {
			var cid int64
			if err := tx.QueryRow(ctx, `INSERT INTO concepto (rubro_id, slug, nombre, orden) VALUES ($1,$2,$3,99)
				ON CONFLICT (rubro_id, slug) DO UPDATE SET nombre = concepto.nombre RETURNING id`, in.RubroID, slugify(c), c).Scan(&cid); err != nil {
				return nil, err
			}
			concepto = &cid
		}
		fecha := m["fecha"].(string)
		var eg int64
		if err := tx.QueryRow(ctx, `INSERT INTO egreso (edificio_id, periodo, rubro_id, concepto_id, descripcion, monto_cts, fecha, tipo_documento, movimiento_banco_id, registrado_por)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'voucher',$8,$9) RETURNING id`, e.ID, fecha[:7], in.RubroID, concepto, desc, -monto, fecha, mid, uid).Scan(&eg); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE movimiento_banco SET estado='conciliado', regla='creado', egreso_id=$2, confirmado_por=$3, confirmado_en=now() WHERE id=$1`, mid, eg, uid); err != nil {
			return nil, err
		}
		return map[string]any{"id": mid, "estado": conciliacion.Conciliado, "egreso_id": eg, "periodo": m["periodo"]}, nil
	})
}

// crearIngresoDesdeMovimiento: POST /conciliacion/movimientos/{mid}/crear-ingreso {unidad_id, medio?}.
// Registra el abono como pago de la unidad (del cargo más antiguo al más nuevo) y lo concilia.
func (s *Server) crearIngresoDesdeMovimiento(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UnidadID int64  `json:"unidad_id"`
		Medio    string `json:"medio"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.Medio == "" {
		in.Medio = "deposito"
	}
	uid := ses(r).UsuarioID
	s.conTx(w, r, func(ctx context.Context, tx pgx.Tx, e *Edificio, mid int64) (map[string]any, error) {
		m, err := movimiento(ctx, tx, e.ID, mid)
		if err != nil {
			return nil, err
		}
		if m["estado"] != conciliacion.SinPareja {
			return nil, P.Conflicto("MOVIMIENTO_CON_PAREJA", "Ese movimiento ya tiene pareja: deshazla primero.")
		}
		monto := m["monto_cts"].(int64)
		if monto <= 0 {
			return nil, P.Validacion("Es un cargo: crea un egreso, no un ingreso.")
		}
		var ok bool
		_ = tx.QueryRow(ctx, `SELECT true FROM unidad WHERE id=$1 AND edificio_id=$2`, in.UnidadID, e.ID).Scan(&ok)
		if !ok {
			return nil, P.Validacion("Elige la unidad que pagó.").Campo("unidad_id", "Obligatorio.")
		}
		codigo := m["codigo_operacion"].(string)
		if codigo == "" {
			codigo = fmt.Sprintf("BANCO-%d", mid)
		}
		res, err := s.AplicarPago(ctx, tx, e.ID, in.UnidadID, uid, monto, in.Medio, codigo, m["fecha"].(string))
		if err != nil {
			return nil, err
		}
		apl := res["aplicaciones"].([]Aplicacion)
		if _, err := tx.Exec(ctx, `UPDATE movimiento_banco SET estado='conciliado', regla='creado', pago_id=$2, confirmado_por=$3, confirmado_en=now() WHERE id=$1`, mid, apl[0].PagoID, uid); err != nil {
			return nil, err
		}
		return map[string]any{"id": mid, "estado": conciliacion.Conciliado, "pago_id": apl[0].PagoID, "aplicaciones": apl, "periodo": m["periodo"]}, nil
	})
}

// extractoDemo: GET /conciliacion/extracto-demo.csv?periodo= — un extracto de ejemplo para probar la subida.
func (s *Server) extractoDemo(w http.ResponseWriter, r *http.Request) {
	periodo, err := s.periodoDe(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	a, err := s.ArbolBalance(r.Context(), s.DB, e.ID, periodo, vistaCompleta())
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	datos, _, err := conciliacion.ExtractoDemo(r.Context(), s.DB, e.ID, periodo, a.KPIs.BancoCts)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="extracto-demo-`+periodo+`.csv"`)
	_, _ = w.Write(datos)
}
