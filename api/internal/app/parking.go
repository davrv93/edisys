package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// G4 · Parking: estacionamientos con tarifa por hora cobrada por fracción, y sesiones de entrada y
// salida. Al salir se cobra de dos maneras: al recibo (nota de cargo «ajuste» que entra en el
// siguiente recibo de la unidad) o inmediato (ingreso externo del periodo, con su medio de pago).

var tiposEstacionamiento = map[string]bool{"propio": true, "visitas": true, "alquiler": true}

// MontoParking calcula el cobro en céntimos: dentro de la tolerancia no se cobra; pasada, se cobra
// cada fracción empezada (redondeo al céntimo, mitad hacia arriba).
func MontoParking(minutos int, tarifaHoraCts int64, fraccionMin, toleranciaMin int) int64 {
	if minutos <= 0 || minutos <= toleranciaMin || tarifaHoraCts <= 0 {
		return 0
	}
	if fraccionMin <= 0 {
		fraccionMin = 60
	}
	fracciones := int64((minutos + fraccionMin - 1) / fraccionMin)
	return (fracciones*int64(fraccionMin)*tarifaHoraCts + 30) / 60
}

// duracionTexto: 135 → «2 h 15 min».
func duracionTexto(min int) string {
	if min < 60 {
		return fmt.Sprintf("%d min", min)
	}
	if min%60 == 0 {
		return fmt.Sprintf("%d h", min/60)
	}
	return fmt.Sprintf("%d h %d min", min/60, min%60)
}

// listarEstacionamientos: GET /parking/estacionamientos — cada espacio con su sesión abierta (si hay).
func (s *Server) listarEstacionamientos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT es.id, es.codigo, es.tipo, es.unidad_id, u.codigo AS unidad, es.tarifa_hora_cts,
			es.fraccion_min, es.tolerancia_min, es.activo,
			sp.id AS sesion_id, sp.placa, sp.entrada_en
		FROM estacionamiento es LEFT JOIN unidad u ON u.id=es.unidad_id
		LEFT JOIN sesion_parking sp ON sp.estacionamiento_id=es.id AND sp.salida_en IS NULL
		WHERE es.edificio_id=$1 ORDER BY es.activo DESC, es.codigo`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	libres, ocupados := 0, 0
	for _, f := range filas {
		f["ocupado"] = f["sesion_id"] != nil
		if f["activo"] == true {
			if f["sesion_id"] != nil {
				ocupados++
			} else {
				libres++
			}
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "libres": libres, "ocupados": ocupados})
}

type entradaEstacionamiento struct {
	Codigo        string `json:"codigo"`
	Tipo          string `json:"tipo"`
	UnidadID      *int64 `json:"unidad_id"`
	TarifaHoraCts int64  `json:"tarifa_hora_cts"`
	FraccionMin   int    `json:"fraccion_min"`
	ToleranciaMin int    `json:"tolerancia_min"`
	Activo        *bool  `json:"activo"`
}

func (s *Server) validarEstacionamiento(ctx context.Context, eid int64, in *entradaEstacionamiento) *P.Error {
	ev := P.Validacion("Revisa el estacionamiento.")
	in.Codigo = strings.ToUpper(strings.TrimSpace(in.Codigo))
	if in.Codigo == "" {
		ev.Campo("codigo", "Escribe el código (p. ej. E-03).")
	}
	if in.Tipo == "" {
		in.Tipo = "visitas"
	}
	if !tiposEstacionamiento[in.Tipo] {
		ev.Campo("tipo", "propio, visitas o alquiler.")
	}
	if in.TarifaHoraCts < 0 {
		ev.Campo("tarifa_hora_cts", "No puede ser negativa.")
	}
	if in.FraccionMin == 0 {
		in.FraccionMin = 60
	}
	if in.FraccionMin < 1 || in.FraccionMin > 1440 {
		ev.Campo("fraccion_min", "Entre 1 y 1440 minutos.")
	}
	if in.ToleranciaMin < 0 {
		ev.Campo("tolerancia_min", "No puede ser negativa.")
	}
	if in.UnidadID != nil && *in.UnidadID > 0 {
		var ok bool
		_ = s.DB.QueryRow(ctx, `SELECT true FROM unidad WHERE id=$1 AND edificio_id=$2`, *in.UnidadID, eid).Scan(&ok)
		if !ok {
			ev.Campo("unidad_id", "Elige una unidad del edificio.")
		}
	} else {
		in.UnidadID = nil
	}
	if in.Tipo == "propio" && in.UnidadID == nil {
		ev.Campo("unidad_id", "Un estacionamiento propio pertenece a una unidad.")
	}
	if len(ev.Campos) > 0 {
		return ev
	}
	return nil
}

// crearEstacionamiento: POST /parking/estacionamientos
func (s *Server) crearEstacionamiento(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in entradaEstacionamiento
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ev := s.validarEstacionamiento(r.Context(), e.ID, &in); ev != nil {
		P.Fallo(w, r, ev)
		return
	}
	var id int64
	if err := s.DB.QueryRow(r.Context(), `INSERT INTO estacionamiento (edificio_id, codigo, tipo, unidad_id, tarifa_hora_cts, fraccion_min, tolerancia_min)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, e.ID, in.Codigo, in.Tipo, in.UnidadID, in.TarifaHoraCts, in.FraccionMin, in.ToleranciaMin).Scan(&id); err != nil {
		P.Fallo(w, r, errOperacion(err))
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "codigo": in.Codigo})
}

// editarEstacionamiento: PUT /parking/estacionamientos/{id}
func (s *Server) editarEstacionamiento(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	var in entradaEstacionamiento
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ev := s.validarEstacionamiento(r.Context(), e.ID, &in); ev != nil {
		P.Fallo(w, r, ev)
		return
	}
	activo := true
	if in.Activo != nil {
		activo = *in.Activo
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE estacionamiento SET codigo=$3, tipo=$4, unidad_id=$5, tarifa_hora_cts=$6, fraccion_min=$7, tolerancia_min=$8, activo=$9
		WHERE id=$1 AND edificio_id=$2`, id, e.ID, in.Codigo, in.Tipo, in.UnidadID, in.TarifaHoraCts, in.FraccionMin, in.ToleranciaMin, activo)
	if err != nil {
		P.Fallo(w, r, errOperacion(err))
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("ese estacionamiento"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

const sqlSesionParking = `SELECT sp.id, sp.estacionamiento_id, es.codigo AS estacionamiento, sp.placa, sp.unidad_id, u.codigo AS unidad,
	sp.visita_id, v.visitante, sp.entrada_en, sp.salida_en, sp.minutos, sp.monto_cts, sp.cobro, sp.medio, sp.ajuste_id, sp.ingreso_id,
	es.tarifa_hora_cts, es.fraccion_min, es.tolerancia_min
	FROM sesion_parking sp JOIN estacionamiento es ON es.id=sp.estacionamiento_id
	LEFT JOIN unidad u ON u.id=sp.unidad_id LEFT JOIN visita v ON v.id=sp.visita_id`

// cotizacionEn agrega minutos y monto a la hora «ahora» de una sesión abierta.
func cotizacionEn(f map[string]any, ahora time.Time) (int, int64) {
	entrada, _ := f["entrada_en"].(time.Time)
	min := int(ahora.Sub(entrada) / time.Minute)
	if min < 0 {
		min = 0
	}
	tarifa, _ := f["tarifa_hora_cts"].(int64)
	return min, MontoParking(min, tarifa, aInt(f["fraccion_min"]), aInt(f["tolerancia_min"]))
}

func aInt(v any) int {
	switch x := v.(type) {
	case int32:
		return int(x)
	case int64:
		return int(x)
	case int:
		return x
	}
	return 0
}

// listarSesionesParking: GET /parking/sesiones?abiertas=1&desde=&hasta=
func (s *Server) listarSesionesParking(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	v := r.URL.Query()
	cond := []string{"sp.edificio_id=$1"}
	args := []any{e.ID}
	if v.Get("abiertas") == "1" {
		cond = append(cond, "sp.salida_en IS NULL")
	}
	if t, err := parseFecha(v.Get("desde")); err == nil {
		args = append(args, t)
		cond = append(cond, fmt.Sprintf("sp.entrada_en >= $%d", len(args)))
	}
	if t, err := parseFecha(v.Get("hasta")); err == nil {
		args = append(args, t.AddDate(0, 0, 1))
		cond = append(cond, fmt.Sprintf("sp.entrada_en < $%d", len(args)))
	}
	filas, err := db.Filas(r.Context(), s.DB, sqlSesionParking+` WHERE `+strings.Join(cond, " AND ")+` ORDER BY sp.salida_en IS NOT NULL, sp.entrada_en DESC LIMIT 300`, args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	ahora := time.Now()
	var cobrado int64
	for _, f := range filas {
		if f["salida_en"] == nil {
			m, monto := cotizacionEn(f, ahora)
			f["minutos_actuales"], f["monto_actual_cts"] = m, monto
		} else if x, ok := f["monto_cts"].(int64); ok {
			cobrado += x
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "cobrado_cts": cobrado})
}

// entradaParking: POST /parking/sesiones {estacionamiento_id, placa, unidad_id?, visita_id?}
// Sin unidad se toma la de la visita o, si el espacio es propio, la del dueño.
func (s *Server) entradaParking(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	var in struct {
		EstacionamientoID int64  `json:"estacionamiento_id"`
		Placa             string `json:"placa"`
		UnidadID          *int64 `json:"unidad_id"`
		VisitaID          *int64 `json:"visita_id"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa la entrada.")
	placa := strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(in.Placa))
	if placa == "" {
		ev.Campo("placa", "Escribe la placa.")
	}
	var activo bool
	var dueno *int64
	if err := s.DB.QueryRow(ctx, `SELECT activo, unidad_id FROM estacionamiento WHERE id=$1 AND edificio_id=$2`, in.EstacionamientoID, e.ID).Scan(&activo, &dueno); err != nil {
		ev.Campo("estacionamiento_id", "Elige un estacionamiento del edificio.")
	} else if !activo {
		ev.Campo("estacionamiento_id", "Ese estacionamiento está desactivado.")
	}
	unidad := in.UnidadID
	if unidad != nil && *unidad <= 0 {
		unidad = nil
	}
	if in.VisitaID != nil && *in.VisitaID > 0 {
		var vu int64
		if err := s.DB.QueryRow(ctx, `SELECT unidad_id FROM visita WHERE id=$1 AND edificio_id=$2`, *in.VisitaID, e.ID).Scan(&vu); err != nil {
			ev.Campo("visita_id", "Esa visita no es del edificio.")
		} else if unidad == nil {
			unidad = &vu
		}
	} else {
		in.VisitaID = nil
	}
	if unidad == nil {
		unidad = dueno
	}
	if unidad != nil {
		var ok bool
		_ = s.DB.QueryRow(ctx, `SELECT true FROM unidad WHERE id=$1 AND edificio_id=$2`, *unidad, e.ID).Scan(&ok)
		if !ok {
			ev.Campo("unidad_id", "Elige una unidad del edificio.")
		}
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	var id int64
	var entrada time.Time
	if err := s.DB.QueryRow(ctx, `INSERT INTO sesion_parking (edificio_id, estacionamiento_id, placa, unidad_id, visita_id, registrado_por)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, entrada_en`, e.ID, in.EstacionamientoID, placa, unidad, in.VisitaID, ses(r).UsuarioID).Scan(&id, &entrada); err != nil {
		P.Fallo(w, r, errOperacion(err))
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "placa": placa, "unidad_id": unidad, "entrada_en": entrada})
}

// cotizarParking: GET /parking/sesiones/{id}/cotizar — cuánto se cobraría si sale ahora.
func (s *Server) cotizarParking(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	f, err := db.Fila(r.Context(), s.DB, sqlSesionParking+` WHERE sp.id=$1 AND sp.edificio_id=$2`, idURL(r, "id"), e.ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("esa sesión"))
		return
	}
	if f["salida_en"] != nil {
		P.JSON(w, http.StatusOK, map[string]any{"sesion": f, "minutos": f["minutos"], "monto_cts": f["monto_cts"], "cerrada": true})
		return
	}
	m, monto := cotizacionEn(f, time.Now())
	P.JSON(w, http.StatusOK, map[string]any{"sesion": f, "minutos": m, "duracion": duracionTexto(m), "monto_cts": monto, "cerrada": false})
}

// salidaParking: POST /parking/sesiones/{id}/salida {cobro: recibo|inmediato, medio?}
// El monto lo calcula el API con la tarifa del espacio; si sale en cero, no hay cobro.
func (s *Server) salidaParking(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	se := ses(r)
	ctx := r.Context()
	id := idURL(r, "id")
	var in struct {
		Cobro string `json:"cobro"`
		Medio string `json:"medio"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.Cobro != "recibo" && in.Cobro != "inmediato" {
		P.Fallo(w, r, P.Validacion("Elige cómo se cobra.").Campo("cobro", "recibo o inmediato."))
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM sesion_parking WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, id, e.ID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	f, err := db.Fila(ctx, tx, sqlSesionParking+` WHERE sp.id=$1 AND sp.edificio_id=$2`, id, e.ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("esa sesión"))
		return
	}
	if f["salida_en"] != nil {
		P.Fallo(w, r, P.Conflicto("SESION_CERRADA", "Esa salida ya se registró."))
		return
	}
	ahora := time.Now()
	minutos, monto := cotizacionEn(f, ahora)
	cobro := in.Cobro
	if monto == 0 {
		cobro = "sin_cobro"
	}
	unidad, _ := f["unidad_id"].(int64)
	motivo := fmt.Sprintf("Parking %s · placa %s · %s", f["estacionamiento"], f["placa"], duracionTexto(minutos))
	periodo := ahora.In(P.Lima).Format("2006-01")
	var ajusteID, ingresoID *int64
	switch cobro {
	case "recibo":
		if unidad == 0 {
			P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "UNIDAD_OBLIGATORIA", "Para cobrar al recibo la sesión debe tener una unidad; cobra inmediato.").Campo("cobro", "Sin unidad."))
			return
		}
		var aid int64
		if err := tx.QueryRow(ctx, `INSERT INTO ajuste (edificio_id, unidad_id, monto_cts, motivo, periodo_origen, creado_por)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, e.ID, unidad, monto, motivo, periodo, se.UsuarioID).Scan(&aid); err != nil {
			P.Fallo(w, r, err)
			return
		}
		ajusteID = &aid
	case "inmediato":
		if in.Medio == "" {
			in.Medio = "efectivo"
		}
		var iid int64
		if err := tx.QueryRow(ctx, `INSERT INTO ingreso_externo (edificio_id, periodo, descripcion, monto_cts, fecha, medio, creado_por)
			VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, e.ID, periodo, motivo, monto, ahora.In(P.Lima).Format("2006-01-02"), in.Medio, se.UsuarioID).Scan(&iid); err != nil {
			P.Fallo(w, r, err)
			return
		}
		ingresoID = &iid
	}
	if _, err := tx.Exec(ctx, `UPDATE sesion_parking SET salida_en=$2, minutos=$3, monto_cts=$4, cobro=$5, medio=$6, ajuste_id=$7, ingreso_id=$8, cerrado_por=$9
		WHERE id=$1`, id, ahora, minutos, monto, cobro, in.Medio, ajusteID, ingresoID, se.UsuarioID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "minutos": minutos, "duracion": duracionTexto(minutos), "monto_cts": monto, "cobro": cobro,
		"ajuste_id": ajusteID, "ingreso_id": ingresoID})
}
