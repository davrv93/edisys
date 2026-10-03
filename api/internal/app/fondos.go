package app

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Fondos y trazabilidad (bloque C). Cada pago validado y cada egreso se asientan en un fondo;
// el saldo de un fondo es la suma de sus movimientos, sin recálculos paralelos.

type fondoBase struct {
	codigo, nombre, categoria string
	tipos                     []string
}

// Los fondos por defecto de un edificio nuevo y a qué tipo de línea del recibo reciben.
var fondosPorDefecto = []fondoBase{
	{"cuota", "Cuota ordinaria", "Administración", []string{"cuota", "concepto"}},
	{"agua", "Agua", "Servicios", []string{"agua"}},
	{"agua_comun", "Agua común", "Servicios", []string{"agua_comun"}},
	{"luz", "Luz", "Servicios", []string{"energia_comun"}},
	{"reservas", "Reservas", "Ingresos", []string{"reserva"}},
	{"multas", "Multas", "Ingresos", []string{"multa"}},
	{"saldo", "Saldo anterior", "Administración", []string{"saldo_anterior"}},
	{"general", "Gastos generales", "Egresos", nil},
}

// asegurarFondosBase crea los fondos y mapeos por defecto si el edificio no tiene ninguno.
func (s *Server) asegurarFondosBase(ctx context.Context, eid int64) error {
	var n int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM fondo WHERE edificio_id=$1`, eid).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for i, f := range fondosPorDefecto {
		var fid int64
		if err := tx.QueryRow(ctx, `INSERT INTO fondo (edificio_id, codigo, nombre, categoria, orden) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
			eid, f.codigo, f.nombre, f.categoria, i).Scan(&fid); err != nil {
			return err
		}
		for _, t := range f.tipos {
			if _, err := tx.Exec(ctx, `INSERT INTO fondo_tipo_linea (fondo_id, tipo) VALUES ($1,$2) ON CONFLICT DO NOTHING`, fid, t); err != nil {
				return err
			}
		}
	}
	// Todos los rubros de egreso existentes caen en «Gastos generales» hasta que se remapeen.
	if _, err := tx.Exec(ctx, `INSERT INTO fondo_rubro (fondo_id, rubro_id)
		SELECT (SELECT id FROM fondo WHERE edificio_id=$1 AND codigo='general'), r.id FROM rubro r WHERE r.edificio_id=$1
		ON CONFLICT DO NOTHING`, eid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Server) fondosDeTipo(ctx context.Context, q db.Q, eid int64) map[string]int64 {
	m := map[string]int64{}
	rows, err := q.Query(ctx, `SELECT ft.tipo, ft.fondo_id FROM fondo_tipo_linea ft JOIN fondo f ON f.id=ft.fondo_id WHERE f.edificio_id=$1`, eid)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var id int64
		if rows.Scan(&t, &id) == nil {
			m[t] = id
		}
	}
	return m
}

func (s *Server) fondoDeRubro(ctx context.Context, q db.Q, eid, rubroID int64) (int64, bool) {
	var id int64
	err := q.QueryRow(ctx, `SELECT fr.fondo_id FROM fondo_rubro fr JOIN fondo f ON f.id=fr.fondo_id WHERE fr.rubro_id=$1 AND f.edificio_id=$2`, rubroID, eid).Scan(&id)
	if err != nil {
		_ = s.DB.QueryRow(ctx, `SELECT id FROM fondo WHERE edificio_id=$1 AND codigo='general'`, eid).Scan(&id)
	}
	return id, id != 0
}

// asentarIngresoPago reparte lo cobrado entre los fondos de las líneas del recibo
// (proporcional al monto, con el método del mayor residuo: la suma cuadra al céntimo).
func (s *Server) asentarIngresoPago(ctx context.Context, tx db.Q, eid, pagoID, reciboID, monto int64, fecha time.Time) error {
	lineas, err := db.Filas(ctx, tx, `SELECT tipo, monto_cts FROM recibo_linea WHERE recibo_id=$1 ORDER BY orden, id`, reciboID)
	if err != nil {
		return err
	}
	if len(lineas) == 0 {
		return nil
	}
	mapa := s.fondosDeTipo(ctx, tx, eid)
	if len(mapa) == 0 {
		_ = s.asegurarFondosBase(ctx, eid)
		mapa = s.fondosDeTipo(ctx, tx, eid)
	}
	pesos := make([]int64, 0, len(lineas))
	fondos := make([]int64, 0, len(lineas))
	var total int64
	for _, l := range lineas {
		tipo := l["tipo"].(string)
		peso := l["monto_cts"].(int64)
		if peso < 0 {
			peso = -peso
		}
		fid, ok := mapa[tipo]
		if !ok {
			_ = s.DB.QueryRow(ctx, `SELECT id FROM fondo WHERE edificio_id=$1 AND codigo='general'`, eid).Scan(&fid)
		}
		pesos = append(pesos, peso)
		fondos = append(fondos, fid)
		total += peso
	}
	if total == 0 {
		return nil
	}
	partes := mayorResiduo(monto, pesos)
	acum := map[int64]int64{}
	for i, p := range partes {
		if fondos[i] != 0 {
			acum[fondos[i]] += p
		}
	}
	periodo := fecha.Format("2006-01")
	for fid, cts := range acum {
		if cts == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO fondo_movimiento (edificio_id, fondo_id, periodo, fecha, monto_cts, tipo, origen, ref_id, descripcion)
			VALUES ($1,$2,$3,$4,$5,'ingreso','pago',$6,'Cobro de recibo')`, eid, fid, periodo, fecha, cts, pagoID); err != nil {
			return err
		}
	}
	return nil
}

// revertirPago borra los asientos de un pago (rechazo o anulación).
func (s *Server) revertirPago(ctx context.Context, q db.Q, pagoID int64) error {
	_, err := q.Exec(ctx, `DELETE FROM fondo_movimiento WHERE origen='pago' AND ref_id=$1`, pagoID)
	return err
}

// asentarEgreso asienta un egreso en el fondo de su rubro (o en «Gastos generales»).
func (s *Server) asentarEgreso(ctx context.Context, q db.Q, eid, egresoID, rubroID, monto int64, fecha time.Time) error {
	fid, ok := s.fondoDeRubro(ctx, q, eid, rubroID)
	if !ok {
		return nil
	}
	_, err := q.Exec(ctx, `INSERT INTO fondo_movimiento (edificio_id, fondo_id, periodo, fecha, monto_cts, tipo, origen, ref_id, descripcion)
		VALUES ($1,$2,$3,$4,$5,'egreso','egreso',$6,'Pago de egreso')`, eid, fid, fecha.Format("2006-01"), fecha, -monto, egresoID)
	return err
}

// mayorResiduo reparte `monto` entre pesos de forma proporcional; la suma es exacta.
func mayorResiduo(monto int64, pesos []int64) []int64 {
	var total int64
	for _, p := range pesos {
		total += p
	}
	out := make([]int64, len(pesos))
	rem := make([]int64, len(pesos))
	if total <= 0 || monto <= 0 {
		return out
	}
	var asignado int64
	for i, p := range pesos {
		num := monto * p
		out[i] = num / total
		rem[i] = num % total
		asignado += out[i]
	}
	for falta := monto - asignado; falta > 0; falta-- {
		idx := 0
		for i := 1; i < len(rem); i++ {
			if rem[i] > rem[idx] {
				idx = i
			}
		}
		out[idx]++
		rem[idx] = -1
	}
	return out
}

// ---------- API ----------

// listarFondos: GET /fondos
func (s *Server) listarFondos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	if err := s.asegurarFondosBase(ctx, e.ID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	filas, err := db.Filas(ctx, s.DB, `SELECT f.id, f.codigo, f.nombre, f.categoria, f.orden, f.activo,
			COALESCE(SUM(m.monto_cts),0)::bigint AS saldo_cts
		FROM fondo f LEFT JOIN fondo_movimiento m ON m.fondo_id=f.id
		WHERE f.edificio_id=$1 GROUP BY f.id ORDER BY f.orden, f.nombre`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

type entFondo struct {
	Codigo    string `json:"codigo"`
	Nombre    string `json:"nombre"`
	Categoria string `json:"categoria"`
}

// crearFondo: POST /fondos
func (s *Server) crearFondo(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in entFondo
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.Nombre = strings.TrimSpace(in.Nombre)
	if in.Nombre == "" {
		P.Fallo(w, r, P.Validacion("Escribe el nombre del fondo."))
		return
	}
	if in.Codigo == "" {
		in.Codigo = slugify(in.Nombre)
	}
	var id int64
	err := s.DB.QueryRow(r.Context(), `INSERT INTO fondo (edificio_id, codigo, nombre, categoria, orden)
		VALUES ($1,$2,$3,$4,(SELECT COALESCE(max(orden),0)+1 FROM fondo WHERE edificio_id=$1)) RETURNING id`,
		e.ID, in.Codigo, in.Nombre, in.Categoria).Scan(&id)
	if err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("FONDO_DUPLICADO", "Ya existe un fondo con ese código."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "codigo": in.Codigo})
}

// editarFondo: PUT /fondos/{fid}
func (s *Server) editarFondo(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	fid := idURL(r, "fid")
	var in entFondo
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE fondo SET nombre=$1, categoria=$2 WHERE id=$3 AND edificio_id=$4`,
		strings.TrimSpace(in.Nombre), in.Categoria, fid, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("fondo"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": fid})
}

// desactivarFondo: DELETE /fondos/{fid} → baja lógica.
func (s *Server) desactivarFondo(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	fid := idURL(r, "fid")
	ct, err := s.DB.Exec(r.Context(), `UPDATE fondo SET activo=false WHERE id=$1 AND edificio_id=$2`, fid, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("fondo"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": fid, "activo": false})
}

// trazabilidad: GET /fondos/trazabilidad?desde=AAAA-MM&hasta=AAAA-MM
func (s *Server) trazabilidadFondos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	if err := s.asegurarFondosBase(ctx, e.ID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	desde := r.URL.Query().Get("desde")
	hasta := r.URL.Query().Get("hasta")
	if desde == "" {
		desde = P.PeriodoActual()
	}
	if hasta == "" {
		hasta = desde
	}
	if !P.PeriodoValido(desde) || !P.PeriodoValido(hasta) {
		P.Fallo(w, r, P.Validacion("Los periodos deben ser AAAA-MM.").Campo("desde", "Formato AAAA-MM."))
		return
	}
	filas, err := db.Filas(ctx, s.DB, `SELECT f.id, f.codigo, f.nombre, f.categoria,
			COALESCE(SUM(m.monto_cts) FILTER (WHERE m.periodo < $2),0)::bigint AS anterior_cts,
			COALESCE(SUM(m.monto_cts) FILTER (WHERE m.monto_cts > 0 AND m.periodo BETWEEN $2 AND $3),0)::bigint AS ingreso_cts,
			COALESCE(-SUM(m.monto_cts) FILTER (WHERE m.monto_cts < 0 AND m.periodo BETWEEN $2 AND $3),0)::bigint AS egreso_cts
		FROM fondo f LEFT JOIN fondo_movimiento m ON m.fondo_id=f.id AND m.edificio_id=f.edificio_id
		WHERE f.edificio_id=$1
		GROUP BY f.id ORDER BY f.orden, f.nombre`, e.ID, desde, hasta)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var totIng, totEgr, totSaldo int64
	for _, f := range filas {
		ant := f["anterior_cts"].(int64)
		ing := f["ingreso_cts"].(int64)
		egr := f["egreso_cts"].(int64)
		saldo := ant + ing - egr
		f["saldo_cts"] = saldo
		totIng += ing
		totEgr += egr
		totSaldo += saldo
	}
	P.JSON(w, http.StatusOK, map[string]any{
		"desde": desde, "hasta": hasta, "datos": filas, "total": len(filas),
		"ingreso_cts": totIng, "egreso_cts": totEgr, "saldo_cts": totSaldo,
	})
}

// movimientosFondo: GET /fondos/{fid}/movimientos?periodo=
func (s *Server) movimientosFondo(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	fid := idURL(r, "fid")
	filas, err := db.Filas(r.Context(), s.DB, `SELECT m.id, m.periodo, to_char(m.fecha,'YYYY-MM-DD') AS fecha, m.monto_cts, m.tipo, m.origen, m.descripcion
		FROM fondo_movimiento m WHERE m.fondo_id=$1 AND m.edificio_id=$2 ORDER BY m.fecha DESC, m.id DESC LIMIT 200`, fid, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// movimientoManual: POST /fondos/{fid}/movimientos {monto_cts, tipo ingreso|egreso, fecha?, descripcion?}
func (s *Server) movimientoManual(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	fid := idURL(r, "fid")
	var in struct {
		MontoCts    int64  `json:"monto_cts"`
		Tipo        string `json:"tipo"`
		Fecha       string `json:"fecha"`
		Descripcion string `json:"descripcion"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa el movimiento.")
	if in.MontoCts <= 0 {
		ev.Campo("monto_cts", "El monto debe ser mayor que cero.")
	}
	if in.Tipo != "ingreso" && in.Tipo != "egreso" {
		ev.Campo("tipo", "Elige ingreso o egreso.")
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
	var ok bool
	_ = s.DB.QueryRow(r.Context(), `SELECT true FROM fondo WHERE id=$1 AND edificio_id=$2`, fid, e.ID).Scan(&ok)
	if !ok {
		P.Fallo(w, r, P.NoEncontrado("fondo"))
		return
	}
	monto := in.MontoCts
	if in.Tipo == "egreso" {
		monto = -monto
	}
	var id int64
	if err := s.DB.QueryRow(r.Context(), `INSERT INTO fondo_movimiento (edificio_id, fondo_id, periodo, fecha, monto_cts, tipo, origen, descripcion, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,'manual',$7,$8) RETURNING id`,
		e.ID, fid, fecha.Format("2006-01"), fecha, monto, in.Tipo, strings.TrimSpace(in.Descripcion), ses(r).UsuarioID).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "monto_cts": monto})
}

// transferenciaFondos: POST /fondos/transferencia {origen_id, destino_id, monto_cts, fecha?, motivo?}
func (s *Server) transferenciaFondos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in struct {
		OrigenID  int64  `json:"origen_id"`
		DestinoID int64  `json:"destino_id"`
		MontoCts  int64  `json:"monto_cts"`
		Fecha     string `json:"fecha"`
		Motivo    string `json:"motivo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa la transferencia.")
	if in.MontoCts <= 0 {
		ev.Campo("monto_cts", "El monto debe ser mayor que cero.")
	}
	if in.OrigenID == in.DestinoID || in.OrigenID == 0 || in.DestinoID == 0 {
		ev.Campo("destino_id", "Elige dos fondos distintos.")
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
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	periodo := fecha.Format("2006-01")
	desc := "Transferencia entre fondos: " + strings.TrimSpace(in.Motivo)
	var ref int64
	if err := tx.QueryRow(r.Context(), `INSERT INTO fondo_movimiento (edificio_id, fondo_id, periodo, fecha, monto_cts, tipo, origen, descripcion, creado_por)
		VALUES ($1,$2,$3,$4,$5,'transferencia_salida','transferencia',$6,$7) RETURNING id`,
		e.ID, in.OrigenID, periodo, fecha, -in.MontoCts, desc, ses(r).UsuarioID).Scan(&ref); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO fondo_movimiento (edificio_id, fondo_id, periodo, fecha, monto_cts, tipo, origen, ref_id, descripcion, creado_por)
		VALUES ($1,$2,$3,$4,$5,'transferencia_entrada','transferencia',$6,$7,$8)`,
		e.ID, in.DestinoID, periodo, fecha, in.MontoCts, ref, desc, ses(r).UsuarioID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"ok": true, "monto_cts": in.MontoCts})
}

// ordenarFondos ordena por orden, nombre (usado por pruebas).
func ordenarFondos(f []map[string]any) {
	sort.SliceStable(f, func(i, j int) bool { return f[i]["orden"].(int) < f[j]["orden"].(int) })
}
