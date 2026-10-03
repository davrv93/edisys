package app

import (
	"context"
	"encoding/json"
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

// Franja configurada de un área (hora de Lima).
type Franja struct {
	Inicio    string `json:"inicio"`
	Fin       string `json:"fin"`
	TarifaCts *int64 `json:"tarifa_cts,omitempty"` // H3: tarifa propia de la franja; si falta, la del área
}

const sqlArea = `SELECT a.id, a.nombre, a.slug, a.tarifa_cts, a.franjas, a.aforo, a.incluye, a.normas, a.anticipacion_max_dias, a.activo,
	a.anticipacion_min_dias, a.separacion_dias, a.garantia_cts, a.limpieza_cts, a.permite_parciales, a.deuda_tolerada_cts, a.permite_financiados,
	a.descripcion, a.horarios, a.reglamento_archivo_id, a.cupo_mensual_unidad, a.checkin_tolerancia_min,
	COALESCE((SELECT json_agg(json_build_object('id', f.id, 'archivo_id', f.archivo_id) ORDER BY f.orden, f.id) FROM area_foto f WHERE f.area_id=a.id), '[]') AS fotos,
	COALESCE((SELECT json_agg(json_build_object('id', r.id, 'nombre', r.nombre, 'activo', r.activo) ORDER BY r.id) FROM recurso r WHERE r.area_id=a.id), '[]') AS recursos
	FROM area a`

func (s *Server) listarAreas(w http.ResponseWriter, r *http.Request) {
	filas, err := db.Filas(r.Context(), s.DB, sqlArea+` WHERE a.edificio_id=$1 ORDER BY a.id`, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.decorarAreas(filas...)
	var modo string
	_ = s.DB.QueryRow(r.Context(), `SELECT modo_cobro_reservas FROM edificio WHERE id=$1`, edf(r).ID).Scan(&modo)
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "modo_cobro": modo})
}

func (s *Server) verArea(w http.ResponseWriter, r *http.Request) {
	aid, err := idRuta(r, "aid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	f, err := db.Fila(r.Context(), s.DB, sqlArea+` WHERE a.id=$1 AND a.edificio_id=$2`, aid, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("el área"))
		return
	}
	s.decorarAreas(f)
	P.JSON(w, http.StatusOK, f)
}

type areaIn struct {
	Nombre              string   `json:"nombre"`
	TarifaCts           *int64   `json:"tarifa_cts"`
	Franjas             []Franja `json:"franjas"`
	Aforo               *int     `json:"aforo"`
	Incluye             *string  `json:"incluye"`
	Normas              *string  `json:"normas"`
	AnticipacionMaxDias *int     `json:"anticipacion_max_dias"`
	Activo              *bool    `json:"activo"`
	Recursos            []string `json:"recursos"`
	configAreaIn                 // H2/H3
}

func validarFranjas(fs []Franja) error {
	for _, f := range fs {
		a, err1 := time.Parse("15:04", f.Inicio)
		b, err2 := time.Parse("15:04", f.Fin)
		if err1 != nil || err2 != nil || !b.After(a) {
			return P.Validacion("Cada franja necesita inicio y fin en formato HH:MM, con fin después del inicio.").Campo("franjas", "Formato HH:MM.")
		}
	}
	return nil
}

func (s *Server) crearArea(w http.ResponseWriter, r *http.Request) {
	var in areaIn
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if strings.TrimSpace(in.Nombre) == "" {
		P.Fallo(w, r, P.Validacion("Falta el nombre.").Campo("nombre", "Obligatorio."))
		return
	}
	if err := validarFranjas(in.Franjas); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := in.configAreaIn.validar(); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if len(in.Recursos) == 0 {
		in.Recursos = []string{in.Nombre}
	}
	e := edf(r)
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	franjas, _ := json.Marshal(in.Franjas)
	if in.Franjas == nil {
		franjas = []byte("[]")
	}
	var aid int64
	if err := tx.QueryRow(ctx, `INSERT INTO area (edificio_id, nombre, slug, tarifa_cts, franjas, aforo, incluye, normas, anticipacion_max_dias)
		VALUES ($1,$2,$3,COALESCE($4,0),$5,$6,COALESCE($7,''),COALESCE($8,''),COALESCE($9,30)) RETURNING id`,
		e.ID, in.Nombre, slugify(in.Nombre), in.TarifaCts, franjas, in.Aforo, in.Incluye, in.Normas, in.AnticipacionMaxDias).Scan(&aid); err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, rc := range in.Recursos {
		if _, err := tx.Exec(ctx, `INSERT INTO recurso (area_id, nombre) VALUES ($1,$2)`, aid, rc); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	if err := guardarConfigArea(ctx, tx, aid, e.ID, in.configAreaIn); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	f, _ := db.Fila(ctx, s.DB, sqlArea+` WHERE a.id=$1`, aid)
	s.decorarAreas(f)
	P.JSON(w, http.StatusCreated, f)
}

func (s *Server) editarArea(w http.ResponseWriter, r *http.Request) {
	aid, err := idRuta(r, "aid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in areaIn
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := validarFranjas(in.Franjas); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := in.configAreaIn.validar(); err != nil {
		P.Fallo(w, r, err)
		return
	}
	var franjas []byte
	if in.Franjas != nil {
		franjas, _ = json.Marshal(in.Franjas)
	}
	var nombre *string
	if in.Nombre != "" {
		nombre = &in.Nombre
	}
	ctx := r.Context()
	tag, err := s.DB.Exec(ctx, `UPDATE area SET nombre=COALESCE($3,nombre), tarifa_cts=COALESCE($4,tarifa_cts), franjas=COALESCE($5::jsonb,franjas),
		aforo=COALESCE($6,aforo), incluye=COALESCE($7,incluye), normas=COALESCE($8,normas), anticipacion_max_dias=COALESCE($9,anticipacion_max_dias),
		activo=COALESCE($10,activo) WHERE id=$1 AND edificio_id=$2`, aid, edf(r).ID, nombre, in.TarifaCts, franjas, in.Aforo, in.Incluye, in.Normas, in.AnticipacionMaxDias, in.Activo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("el área"))
		return
	}
	if err := guardarConfigArea(ctx, s.DB, aid, edf(r).ID, in.configAreaIn); err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, rc := range in.Recursos {
		_, _ = s.DB.Exec(ctx, `INSERT INTO recurso (area_id, nombre) SELECT $1, $2 WHERE NOT EXISTS (SELECT 1 FROM recurso WHERE area_id=$1 AND nombre=$2)`, aid, rc)
	}
	s.verArea(w, r)
}

// recursoInfo: datos para validar una reserva.
type recursoInfo struct {
	ID, AreaID, Tarifa int64
	Nombre, Area       string
	Franjas            []Franja
	AnticipacionMax    int
	Normas             string
	Cfg                areaConfig // H2/H3
}

func (s *Server) recurso(ctx context.Context, q db.Q, eid, rid int64) (*recursoInfo, error) {
	ri := &recursoInfo{ID: rid}
	var fr []byte
	var activo bool
	err := q.QueryRow(ctx, `SELECT r.area_id, a.tarifa_cts, r.nombre, a.nombre, a.franjas, a.anticipacion_max_dias, a.normas, (a.activo AND r.activo)
		FROM recurso r JOIN area a ON a.id=r.area_id WHERE r.id=$1 AND a.edificio_id=$2`, rid, eid).Scan(&ri.AreaID, &ri.Tarifa, &ri.Nombre, &ri.Area, &fr, &ri.AnticipacionMax, &ri.Normas, &activo)
	if err != nil {
		return nil, P.NoEncontrado("ese recurso")
	}
	if !activo {
		return nil, P.Err(http.StatusUnprocessableEntity, "AREA_INACTIVA", "Esa área no está disponible.")
	}
	_ = json.Unmarshal(fr, &ri.Franjas)
	if ri.Cfg, err = s.configArea(ctx, q, ri.AreaID); err != nil {
		return nil, err
	}
	return ri, nil
}

// franjasDelDia arma las franjas de un día (Lima) para un recurso.
func franjasDelDia(dia time.Time, fs []Franja) [][2]time.Time {
	var out [][2]time.Time
	for _, f := range fs {
		a, _ := time.ParseInLocation("2006-01-02 15:04", dia.Format("2006-01-02")+" "+f.Inicio, P.Lima)
		b, _ := time.ParseInLocation("2006-01-02 15:04", dia.Format("2006-01-02")+" "+f.Fin, P.Lima)
		out = append(out, [2]time.Time{a, b})
	}
	return out
}

func parseFecha(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("vacía")
	}
	for _, f := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(f, s, P.Lima); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("formato")
}

// disponibilidad: GET /disponibilidad?recurso=&area=&desde=&hasta= → franjas libre|ocupada|retenida|fuera_de_horario.
func (s *Server) disponibilidad(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	q := r.URL.Query()
	hoy := time.Now().In(P.Lima)
	desde := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, P.Lima)
	if t, err := parseFecha(q.Get("desde")); err == nil {
		desde = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, P.Lima)
	}
	hasta := desde.AddDate(0, 0, 6)
	if t, err := parseFecha(q.Get("hasta")); err == nil {
		hasta = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, P.Lima)
	}
	if hasta.Before(desde) || hasta.Sub(desde) > 62*24*time.Hour {
		P.Fallo(w, r, P.Validacion("El rango de fechas debe ser de hasta 62 días."))
		return
	}
	franjas, err := s.calcularDisponibilidad(ctx, e.ID, q.Get("recurso"), q.Get("area"), desde, hasta, !e.SoloLoSuyo())
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"desde": desde.Format("2006-01-02"), "hasta": hasta.Format("2006-01-02"), "franjas": franjas})
}

func (s *Server) calcularDisponibilidad(ctx context.Context, eid int64, recurso, area string, desde, hasta time.Time, verCodigo bool) ([]map[string]any, error) {
	cond := "a.edificio_id=$1 AND a.activo AND r.activo"
	args := []any{eid}
	if id, err := strconv.ParseInt(recurso, 10, 64); err == nil {
		args = append(args, id)
		cond += " AND r.id=$2"
	} else if area != "" {
		args = append(args, area)
		cond += " AND (a.id::text=$2 OR a.slug=$2)"
	}
	filas, err := s.DB.Query(ctx, `SELECT r.id, r.nombre, a.id, a.nombre, a.franjas, a.tarifa_cts, a.horarios FROM recurso r JOIN area a ON a.id=r.area_id WHERE `+cond+` ORDER BY a.id, r.id`, args...)
	if err != nil {
		return nil, err
	}
	type rec struct {
		id, areaID, tarifa int64
		nombre, area       string
		franjas            []Franja
		horarios           map[string][]Franja
	}
	var recs []rec
	for filas.Next() {
		var x rec
		var fr, hs []byte
		if err := filas.Scan(&x.id, &x.nombre, &x.areaID, &x.area, &fr, &x.tarifa, &hs); err != nil {
			filas.Close()
			return nil, err
		}
		_ = json.Unmarshal(fr, &x.franjas)
		_ = json.Unmarshal(hs, &x.horarios)
		recs = append(recs, x)
	}
	filas.Close()
	type ocup struct {
		recurso  int64
		ini, fin time.Time
		estado   string
		codigo   string
		vence    *time.Time
	}
	fin := hasta.AddDate(0, 0, 1)
	fo, err := s.DB.Query(ctx, `SELECT recurso_id, inicio, fin, estado, codigo, vence_retencion FROM reserva
		WHERE edificio_id=$1 AND estado IN ('confirmada','pendiente_pago') AND inicio < $3 AND fin > $2`, eid, desde, fin)
	if err != nil {
		return nil, err
	}
	var ocs []ocup
	for fo.Next() {
		var o ocup
		if err := fo.Scan(&o.recurso, &o.ini, &o.fin, &o.estado, &o.codigo, &o.vence); err != nil {
			fo.Close()
			return nil, err
		}
		ocs = append(ocs, o)
	}
	fo.Close()
	ahora := time.Now()
	out := []map[string]any{}
	for d := desde; !d.After(hasta); d = d.AddDate(0, 0, 1) {
		for _, x := range recs {
			for _, fd := range franjasParaDia(d, x.franjas, x.horarios) {
				f := [2]time.Time{fd.Ini, fd.Fin}
				tarifa := x.tarifa
				if fd.TarifaCts != nil {
					tarifa = *fd.TarifaCts
				}
				estado := "libre"
				codigo := ""
				if f[1].Before(ahora) {
					estado = "fuera_de_horario"
				}
				for _, o := range ocs {
					if o.recurso == x.id && o.ini.Before(f[1]) && o.fin.After(f[0]) {
						estado = "ocupada"
						if o.estado == "pendiente_pago" {
							estado = "retenida"
						}
						codigo = o.codigo
					}
				}
				m := map[string]any{"recurso_id": x.id, "recurso": x.nombre, "area_id": x.areaID, "area": x.area, "tarifa_cts": tarifa,
					"inicio": f[0].UTC(), "fin": f[1].UTC(), "fecha": d.Format("2006-01-02"),
					"hora_inicio": f[0].Format("15:04"), "hora_fin": f[1].Format("15:04"), "estado": estado}
				if verCodigo && codigo != "" {
					m["codigo"] = codigo
				}
				out = append(out, m)
			}
		}
	}
	return out, nil
}

func (s *Server) listarReservas(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	q := r.URL.Query()
	cond := []string{"rv.edificio_id=$1"}
	args := []any{e.ID}
	add := func(c string, v any) {
		args = append(args, v)
		cond = append(cond, strings.ReplaceAll(c, "?", "$"+strconv.Itoa(len(args))))
	}
	if t, err := parseFecha(q.Get("desde")); err == nil {
		add("rv.fin > ?", t)
	}
	if t, err := parseFecha(q.Get("hasta")); err == nil {
		add("rv.inicio < ?", t.AddDate(0, 0, 1))
	}
	if v := q.Get("recurso"); v != "" {
		add("rv.recurso_id::text = ?", v)
	}
	if v := q.Get("estado"); v != "" {
		add("rv.estado = ?", v)
	}
	if v := q.Get("unidad"); v != "" {
		add("(u.codigo = ? OR u.id::text = ?)", v)
	}
	if e.SoloLoSuyo() || q.Get("mias") == "1" {
		add("rv.unidad_id = ANY(?)", e.Unidades)
	}
	filas, err := db.Filas(r.Context(), s.DB, `SELECT rv.id, rv.codigo, rv.estado, rv.inicio, rv.fin, rv.total_cts, rv.modo_cobro, rv.vence_retencion,
			rv.recurso_id, rc.nombre AS recurso, ar.id AS area_id, ar.nombre AS area, u.id AS unidad_id, u.codigo AS unidad,
			rv.codigo_operacion, rv.voucher_id, rv.pago_validado, rv.forzado_motivo, rv.motivo,
			rv.garantia_cts, rv.limpieza_cts, rv.titulo, rv.asistentes, rv.checkin_en, rv.checkin_valido, rv.checkin_motivo
		FROM reserva rv JOIN recurso rc ON rc.id=rv.recurso_id JOIN area ar ON ar.id=rc.area_id JOIN unidad u ON u.id=rv.unidad_id
		WHERE `+strings.Join(cond, " AND ")+` ORDER BY rv.inicio DESC LIMIT 500`, args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, f := range filas {
		if v, ok := f["voucher_id"].(int64); ok {
			f["voucher_url"] = s.Firma.URL(v)
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearReserva: POST /reservas {recurso_id, unidad_id, inicio, fin, acepta_normas, forzar_motivo?}.
// 201 · 409 FRANJA_OCUPADA · 403 MOROSO · 422 FUERA_DE_HORARIO / FUERA_DE_PLAZO / NORMAS_NO_ACEPTADAS.
func (s *Server) crearReserva(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RecursoID    int64  `json:"recurso_id"`
		UnidadID     int64  `json:"unidad_id"`
		Inicio       string `json:"inicio"`
		Fin          string `json:"fin"`
		AceptaNormas bool   `json:"acepta_normas"`
		ForzarMotivo string `json:"forzar_motivo"`
		Titulo       string `json:"titulo"`     // H3
		Asistentes   *int   `json:"asistentes"` // H3
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.Asistentes != nil && *in.Asistentes <= 0 {
		in.Asistentes = nil
	}
	res, err := s.crearReservaCon(r.Context(), edf(r), ses(r).UsuarioID, in.RecursoID, in.UnidadID, in.Inicio, in.Fin, in.AceptaNormas, in.ForzarMotivo,
		extraReserva{Titulo: strings.TrimSpace(in.Titulo), Asistentes: in.Asistentes})
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.ForzarMotivo != "" {
		s.auditarCambio(r.Context(), s.DB, r, "reservas", "forzar_moroso", "reserva", res["id"], nil, map[string]any{"motivo": in.ForzarMotivo})
	}
	P.JSON(w, http.StatusCreated, res)
}

// extraReserva: datos del evento que pide la configuración completa (H3).
type extraReserva struct {
	Titulo     string
	Asistentes *int
}

// CrearReserva aplica las reglas de 07; la base impide igual la doble reserva y el moroso.
func (s *Server) CrearReserva(ctx context.Context, e *Edificio, usuarioID, recursoID, unidadID int64, inicioTxt, finTxt string, acepta bool, forzar string) (map[string]any, error) {
	return s.crearReservaCon(ctx, e, usuarioID, recursoID, unidadID, inicioTxt, finTxt, acepta, forzar, extraReserva{})
}

func (s *Server) crearReservaCon(ctx context.Context, e *Edificio, usuarioID, recursoID, unidadID int64, inicioTxt, finTxt string, acepta bool, forzar string, extra extraReserva) (map[string]any, error) {
	ri, err := s.recurso(ctx, s.DB, e.ID, recursoID)
	if err != nil {
		return nil, err
	}
	if e.SoloLoSuyo() {
		if unidadID == 0 && len(e.Unidades) > 0 {
			unidadID = e.Unidades[0]
		}
		if !e.EsSuya(unidadID) {
			return nil, P.NoEncontrado("la unidad")
		}
		forzar = ""
		if e.Rol == "inquilino" {
			var puede bool
			_ = s.DB.QueryRow(ctx, `SELECT COALESCE((permisos_inquilino->>'reservar')::boolean, false) FROM unidad WHERE id=$1`, unidadID).Scan(&puede)
			if !puede {
				return nil, P.Prohibido("INQUILINO_SIN_PERMISO", "El propietario no habilitó las reservas para el inquilino.")
			}
		}
	} else {
		var ok bool
		if err := s.DB.QueryRow(ctx, `SELECT true FROM unidad WHERE id=$1 AND edificio_id=$2`, unidadID, e.ID).Scan(&ok); err != nil {
			return nil, P.Validacion("Elige la unidad que reserva.").Campo("unidad_id", "Obligatorio.")
		}
		if forzar != "" && !e.Puede("reservas.administrar") {
			forzar = ""
		}
	}
	if !acepta {
		return nil, P.Err(http.StatusUnprocessableEntity, "NORMAS_NO_ACEPTADAS", "Marca «Acepto las normas» para reservar.").Campo("acepta_normas", "Obligatorio.")
	}
	inicio, err1 := parseFecha(inicioTxt)
	fin, err2 := parseFecha(finTxt)
	if err1 != nil || err2 != nil || !fin.After(inicio) {
		return nil, P.Validacion("Inicio y fin inválidos (usa ISO 8601, p. ej. 2026-10-03T12:00:00-05:00).").Campo("inicio", "Fecha y hora.")
	}
	enFranja := false
	tarifa := ri.Tarifa
	for _, f := range franjasParaDia(inicio.In(P.Lima), ri.Franjas, ri.Cfg.Horarios) {
		if f.Ini.Equal(inicio) && f.Fin.Equal(fin) {
			enFranja = true
			if f.TarifaCts != nil {
				tarifa = *f.TarifaCts
			}
		}
	}
	if !enFranja {
		return nil, P.Err(http.StatusUnprocessableEntity, "FUERA_DE_HORARIO", "Esa franja no está dentro del horario del área.").Con("franjas", ri.Franjas)
	}
	ahora := time.Now()
	if inicio.Before(ahora) {
		return nil, P.Err(http.StatusUnprocessableEntity, "FUERA_DE_PLAZO", "Esa franja ya pasó.")
	}
	if inicio.After(ahora.AddDate(0, 0, ri.AnticipacionMax)) {
		return nil, P.Err(http.StatusUnprocessableEntity, "FUERA_DE_PLAZO", fmt.Sprintf("Solo se reserva con hasta %d días de anticipación.", ri.AnticipacionMax))
	}
	if err := s.validarRestricciones(ctx, ri, unidadID, inicio, extra.Asistentes); err != nil {
		return nil, err
	}
	if forzar == "" {
		var deuda int64
		if err := s.DB.QueryRow(ctx, `SELECT deuda_vencida_cts($1)`, unidadID).Scan(&deuda); err != nil {
			return nil, err
		}
		if deuda > 0 && !s.unidadHabilitada(ctx, s.DB, unidadID, ri.AreaID) { // H2: parciales y financiados pasan
			meses, _ := s.deudaPorUnidad(ctx, e.ID, []int64{unidadID})
			var detalle any = []any{}
			if len(meses) > 0 {
				detalle = meses[0]["meses"]
			}
			return nil, P.Prohibido("MOROSO", "Tu unidad tiene una deuda vencida de "+P.Soles(deuda)+". Cuando la regularices podrás reservar.").
				Con("monto_cts", deuda).Con("meses", detalle)
		}
	}
	var modo string
	if err := s.DB.QueryRow(ctx, `SELECT modo_cobro_reservas FROM edificio WHERE id=$1`, e.ID).Scan(&modo); err != nil {
		return nil, err
	}
	// H2: la garantía y la limpieza se cobran junto con la tarifa (total_cts las incluye; el desglose queda en la reserva).
	total := tarifa + ri.Cfg.Garantia + ri.Cfg.Limpieza
	estado := "confirmada"
	var vence *time.Time
	if modo == "pago_inmediato" && total > 0 {
		estado = "pendiente_pago"
		v := ahora.Add(15 * time.Minute)
		vence = &v
	}
	var forz *string
	if forzar != "" {
		forz = &forzar
	}
	var usuario *int64 // el chatbot puede reservar por una persona sin usuario en la app
	if usuarioID > 0 {
		usuario = &usuarioID
	}
	res, err := db.Fila(ctx, s.DB, `INSERT INTO reserva (edificio_id, recurso_id, unidad_id, usuario_id, codigo, inicio, fin, estado, total_cts, modo_cobro, acepta_normas, vence_retencion, forzado_motivo,
			garantia_cts, limpieza_cts, titulo, asistentes)
		VALUES ($1,$2,$3,$4,'R-' || lpad(nextval('reserva_codigo_seq')::text, 4, '0'),$5,$6,$7,$8,$9,true,$10,$11,$12,$13,$14,$15)
		RETURNING id, codigo, estado, total_cts, garantia_cts, limpieza_cts, titulo, asistentes, modo_cobro, vence_retencion, inicio, fin, unidad_id, recurso_id`,
		e.ID, recursoID, unidadID, usuario, inicio, fin, estado, total, modo, vence, forz, ri.Cfg.Garantia, ri.Cfg.Limpieza, extra.Titulo, extra.Asistentes)
	if err != nil {
		return nil, P.Traducir(err)
	}
	res["recurso"] = ri.Nombre
	res["area"] = ri.Area
	if estado == "pendiente_pago" {
		res["mensaje"] = "Te guardamos la franja 15 minutos. Paga por Yape y sube el voucher para confirmarla."
	} else {
		res["mensaje"] = "Reserva confirmada. Se cargará a tu recibo del mes."
	}
	return res, nil
}

// pagarReserva: POST /reservas/{rid}/pago (multipart {codigo_operacion, voucher}) → 201.
func (s *Server) pagarReserva(w http.ResponseWriter, r *http.Request) {
	rid, err := idRuta(r, "rid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	var unidad int64
	var estado string
	if err := s.DB.QueryRow(ctx, `SELECT unidad_id, estado FROM reserva WHERE id=$1 AND edificio_id=$2`, rid, e.ID).Scan(&unidad, &estado); err != nil ||
		(e.SoloLoSuyo() && !e.EsSuya(unidad)) {
		P.Fallo(w, r, P.NoEncontrado("la reserva"))
		return
	}
	if estado != "pendiente_pago" {
		P.Fallo(w, r, P.Conflicto("RESERVA_NO_PENDIENTE", "La reserva está "+estado+": no espera pago."))
		return
	}
	vouchers, err := archivosDeForm(r, "voucher", "foto")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	codigo := campo(r, "codigo_operacion")
	if len(vouchers) == 0 || codigo == "" {
		P.Fallo(w, r, P.Validacion("Sube el voucher y el código de operación.").Campo("voucher", "Obligatorio.").Campo("codigo_operacion", "Obligatorio."))
		return
	}
	uid := ses(r).UsuarioID
	vid, err := s.guardarArchivo(ctx, s.DB, e.ID, &uid, vouchers[0])
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	f, err := db.Fila(ctx, s.DB, `UPDATE reserva SET codigo_operacion=$2, voucher_id=$3 WHERE id=$1 RETURNING id, codigo, estado, voucher_id, codigo_operacion`, rid, codigo, vid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	f["mensaje"] = "Pago enviado, en revisión."
	P.JSON(w, http.StatusCreated, f)
}

// cambiarReserva (admin): PATCH /reservas/{rid} {estado: confirmada|cancelada|no_show, motivo}.
func (s *Server) cambiarReserva(w http.ResponseWriter, r *http.Request) {
	rid, err := idRuta(r, "rid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Estado string `json:"estado"`
		Motivo string `json:"motivo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	switch in.Estado {
	case "confirmada", "cancelada", "no_show":
	default:
		P.Fallo(w, r, P.Validacion("Estado inválido.").Campo("estado", "confirmada, cancelada o no_show."))
		return
	}
	ctx := r.Context()
	e := edf(r)
	antes, err := db.Fila(ctx, s.DB, `SELECT estado, voucher_id FROM reserva WHERE id=$1 AND edificio_id=$2`, rid, e.ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("la reserva"))
		return
	}
	f, err := db.Fila(ctx, s.DB, `UPDATE reserva SET estado=$3, motivo=NULLIF($4,''),
		pago_validado = CASE WHEN $3='confirmada' AND voucher_id IS NOT NULL THEN true ELSE pago_validado END
		WHERE id=$1 AND edificio_id=$2 RETURNING id, codigo, estado, pago_validado`, rid, e.ID, in.Estado, in.Motivo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "reservas", "cambiar_estado", "reserva", rid, antes, f)
	P.JSON(w, http.StatusOK, f)
}

// cancelarReserva: DELETE /reservas/{rid}. El propietario, solo las suyas y con 24 h de anticipación.
func (s *Server) cancelarReserva(w http.ResponseWriter, r *http.Request) {
	rid, err := idRuta(r, "rid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	var unidad int64
	var inicio time.Time
	var estado string
	if err := s.DB.QueryRow(ctx, `SELECT unidad_id, inicio, estado FROM reserva WHERE id=$1 AND edificio_id=$2`, rid, e.ID).Scan(&unidad, &inicio, &estado); err != nil {
		P.Fallo(w, r, P.NoEncontrado("la reserva"))
		return
	}
	if !e.Puede("reservas.administrar") {
		if !e.EsSuya(unidad) {
			P.Fallo(w, r, P.NoEncontrado("la reserva"))
			return
		}
		if time.Until(inicio) < 24*time.Hour {
			P.Fallo(w, r, P.Conflicto("CANCELACION_TARDIA", "Solo puedes cancelar con 24 horas de anticipación. Escribe a la administración."))
			return
		}
	}
	if estado != "confirmada" && estado != "pendiente_pago" {
		P.Fallo(w, r, P.Conflicto("RESERVA_NO_ACTIVA", "La reserva ya está "+estado+"."))
		return
	}
	var cargada bool
	_ = s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM recibo_linea rl JOIN recibo rc ON rc.id=rl.recibo_id WHERE rl.reserva_id=$1 AND rc.estado NOT IN ('anulado','borrador'))`, rid).Scan(&cargada)
	if cargada {
		P.Fallo(w, r, P.Conflicto("RESERVA_EN_RECIBO", "La reserva ya está en un recibo emitido."))
		return
	}
	if _, err := s.DB.Exec(ctx, `UPDATE reserva SET estado='cancelada' WHERE id=$1`, rid); err != nil {
		P.Fallo(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// liberarRetenciones: las retenciones pendiente_pago vencidas y sin voucher se liberan.
func (s *Server) liberarRetenciones(ctx context.Context) (int64, error) {
	tag, err := s.DB.Exec(ctx, `UPDATE reserva SET estado='vencida', motivo='Se liberó la franja porque no llegó el pago'
		WHERE estado='pendiente_pago' AND vence_retencion < now() AND voucher_id IS NULL`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

var _ = pgx.ErrNoRows
