package app

// Bloques H2 y H3 · restricciones avanzadas y configuración completa de las áreas reservables.
// H2: anticipación mínima, separación entre reservas, garantía, limpieza y morosos habilitados (parciales/financiados).
// H3: horario por día de la semana con tarifa por franja, descripción, reglamento PDF, fotos, cupo mensual y aforo.
// La morosidad la decide la base (reserva_unidad_habilitada, 0028); aquí solo se arma la respuesta clara.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// areaConfig: lo que H2/H3 añaden al área y que la reserva necesita para validarse.
type areaConfig struct {
	AnticipacionMin    int
	Separacion         int
	Garantia, Limpieza int64
	Horarios           map[string][]Franja
	Cupo               *int
	Aforo              *int
	Tolerancia         int
}

func (s *Server) configArea(ctx context.Context, q db.Q, areaID int64) (areaConfig, error) {
	var c areaConfig
	var hs []byte
	err := q.QueryRow(ctx, `SELECT anticipacion_min_dias, separacion_dias, garantia_cts, limpieza_cts, horarios, cupo_mensual_unidad, aforo, checkin_tolerancia_min
		FROM area WHERE id=$1`, areaID).Scan(&c.AnticipacionMin, &c.Separacion, &c.Garantia, &c.Limpieza, &hs, &c.Cupo, &c.Aforo, &c.Tolerancia)
	if err != nil {
		return c, err
	}
	_ = json.Unmarshal(hs, &c.Horarios)
	return c, nil
}

// franjaDia: una franja concreta de un día, con su tarifa propia si el horario la fija.
type franjaDia struct {
	Ini, Fin  time.Time
	TarifaCts *int64
}

// diaISO: 1 = lunes … 7 = domingo, la clave de area.horarios.
func diaISO(d time.Time) string {
	n := int(d.Weekday())
	if n == 0 {
		n = 7
	}
	return strconv.Itoa(n)
}

// franjasParaDia: si el área tiene horario para ese día de la semana manda él ([] = cerrado); si no, las franjas generales.
func franjasParaDia(dia time.Time, base []Franja, horarios map[string][]Franja) []franjaDia {
	fs := base
	if h, ok := horarios[diaISO(dia)]; ok {
		fs = h
	}
	out := make([]franjaDia, 0, len(fs))
	for i, f := range franjasDelDia(dia, fs) {
		out = append(out, franjaDia{Ini: f[0], Fin: f[1], TarifaCts: fs[i].TarifaCts})
	}
	return out
}

// validarHorarios: claves 1..7 y franjas HH:MM bien formadas, sin tarifas negativas.
func validarHorarios(hs map[string][]Franja) error {
	for k, fs := range hs {
		n, err := strconv.Atoi(k)
		if err != nil || n < 1 || n > 7 {
			return P.Validacion("Los horarios van por día de la semana: 1 (lunes) a 7 (domingo).").Campo("horarios", "Día "+k+" inválido.")
		}
		if err := validarFranjas(fs); err != nil {
			return err
		}
		for _, f := range fs {
			if f.TarifaCts != nil && *f.TarifaCts < 0 {
				return P.Validacion("La tarifa de una franja no puede ser negativa.").Campo("horarios", "Tarifa negativa.")
			}
		}
	}
	return nil
}

// configAreaIn: campos de H2/H3 que viajan en el mismo JSON del área (POST/PUT /areas).
type configAreaIn struct {
	AnticipacionMinDias  *int                `json:"anticipacion_min_dias"`
	SeparacionDias       *int                `json:"separacion_dias"`
	GarantiaCts          *int64              `json:"garantia_cts"`
	LimpiezaCts          *int64              `json:"limpieza_cts"`
	PermiteParciales     *bool               `json:"permite_parciales"`
	DeudaToleradaCts     *int64              `json:"deuda_tolerada_cts"`
	PermiteFinanciados   *bool               `json:"permite_financiados"`
	Descripcion          *string             `json:"descripcion"`
	Horarios             map[string][]Franja `json:"horarios"`
	CupoMensualUnidad    *int                `json:"cupo_mensual_unidad"` // 0 = sin tope
	CheckinToleranciaMin *int                `json:"checkin_tolerancia_min"`
}

func (c configAreaIn) validar() error {
	neg := func(campo string, v *int64) error {
		if v != nil && *v < 0 {
			return P.Validacion("Los montos no pueden ser negativos.").Campo(campo, "Mayor o igual a 0.")
		}
		return nil
	}
	for campo, v := range map[string]*int{"anticipacion_min_dias": c.AnticipacionMinDias, "separacion_dias": c.SeparacionDias, "cupo_mensual_unidad": c.CupoMensualUnidad} {
		if v != nil && *v < 0 {
			return P.Validacion("Los días y cupos no pueden ser negativos.").Campo(campo, "Mayor o igual a 0.")
		}
	}
	if err := neg("garantia_cts", c.GarantiaCts); err != nil {
		return err
	}
	if err := neg("limpieza_cts", c.LimpiezaCts); err != nil {
		return err
	}
	if err := neg("deuda_tolerada_cts", c.DeudaToleradaCts); err != nil {
		return err
	}
	if c.CheckinToleranciaMin != nil && (*c.CheckinToleranciaMin < 0 || *c.CheckinToleranciaMin > 240) {
		return P.Validacion("La tolerancia de ingreso va de 0 a 240 minutos.").Campo("checkin_tolerancia_min", "0 a 240.")
	}
	return validarHorarios(c.Horarios)
}

// guardarConfigArea aplica los campos presentes; los ausentes no cambian.
func guardarConfigArea(ctx context.Context, q db.Q, aid, eid int64, c configAreaIn) error {
	var hs []byte
	if c.Horarios != nil {
		hs, _ = json.Marshal(c.Horarios)
	}
	cupoCambia := c.CupoMensualUnidad != nil
	var cupo *int
	if cupoCambia && *c.CupoMensualUnidad > 0 {
		cupo = c.CupoMensualUnidad
	}
	_, err := q.Exec(ctx, `UPDATE area SET
			anticipacion_min_dias=COALESCE($3, anticipacion_min_dias), separacion_dias=COALESCE($4, separacion_dias),
			garantia_cts=COALESCE($5, garantia_cts), limpieza_cts=COALESCE($6, limpieza_cts),
			permite_parciales=COALESCE($7, permite_parciales), deuda_tolerada_cts=COALESCE($8, deuda_tolerada_cts),
			permite_financiados=COALESCE($9, permite_financiados), descripcion=COALESCE($10, descripcion),
			horarios=COALESCE($11::jsonb, horarios),
			cupo_mensual_unidad=CASE WHEN $12 THEN $13 ELSE cupo_mensual_unidad END,
			checkin_tolerancia_min=COALESCE($14, checkin_tolerancia_min)
		WHERE id=$1 AND edificio_id=$2`, aid, eid, c.AnticipacionMinDias, c.SeparacionDias, c.GarantiaCts, c.LimpiezaCts,
		c.PermiteParciales, c.DeudaToleradaCts, c.PermiteFinanciados, c.Descripcion, hs, cupoCambia, cupo, c.CheckinToleranciaMin)
	return traducirArea(err)
}

// traducirArea: los CHECK de 0028 llegan como 422 legibles.
func traducirArea(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23514" {
		if pg.ConstraintName == "area_anticipacion_ck" {
			return P.Validacion("La anticipación mínima no puede pasar a la máxima.").Campo("anticipacion_min_dias", "Menor o igual a la máxima.")
		}
		return P.Validacion("Hay un valor fuera de rango en la configuración del área.")
	}
	return err
}

// decorarAreas añade las URL firmadas de fotos y reglamento (el archivo es privado).
func (s *Server) decorarAreas(filas ...map[string]any) {
	for _, f := range filas {
		if v, ok := f["reglamento_archivo_id"].(int64); ok {
			f["reglamento_url"] = s.Firma.URL(v)
		}
		fotos, _ := f["fotos"].([]any)
		for _, x := range fotos {
			if m, ok := x.(map[string]any); ok {
				if id, ok := m["archivo_id"].(float64); ok {
					m["url"] = s.Firma.URL(int64(id))
				}
			}
		}
	}
}

// validarRestricciones (H2/H3): anticipación mínima, separación entre reservas de la unidad, cupo del mes y aforo.
func (s *Server) validarRestricciones(ctx context.Context, ri *recursoInfo, unidadID int64, inicio time.Time, asistentes *int) error {
	c := ri.Cfg
	hoy := time.Now().In(P.Lima)
	hoy = time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, P.Lima)
	il := inicio.In(P.Lima)
	diaEvento := time.Date(il.Year(), il.Month(), il.Day(), 0, 0, 0, 0, P.Lima)
	if c.AnticipacionMin > 0 && diaEvento.Before(hoy.AddDate(0, 0, c.AnticipacionMin)) {
		return P.Err(http.StatusUnprocessableEntity, "FUERA_DE_PLAZO", fmt.Sprintf("Esta área se reserva con al menos %d días de anticipación.", c.AnticipacionMin))
	}
	if asistentes != nil && c.Aforo != nil && *asistentes > *c.Aforo {
		return P.Err(http.StatusUnprocessableEntity, "AFORO_EXCEDIDO", fmt.Sprintf("El aforo de %s es de %d personas.", ri.Area, *c.Aforo)).Campo("asistentes", "Máximo "+strconv.Itoa(*c.Aforo)+".")
	}
	if c.Separacion > 0 {
		var cerca *string
		_ = s.DB.QueryRow(ctx, `SELECT rv.codigo FROM reserva rv JOIN recurso rc ON rc.id=rv.recurso_id
			WHERE rv.unidad_id=$1 AND rc.area_id=$2 AND rv.estado IN ('confirmada','pendiente_pago')
			  AND abs((rv.inicio AT TIME ZONE 'America/Lima')::date - $3::date) < $4 LIMIT 1`,
			unidadID, ri.AreaID, diaEvento.Format("2006-01-02"), c.Separacion).Scan(&cerca)
		if cerca != nil {
			return P.Err(http.StatusUnprocessableEntity, "SEPARACION_MINIMA",
				fmt.Sprintf("Entre dos reservas de %s deben pasar al menos %d días (ya tienes la %s).", ri.Area, c.Separacion, *cerca)).Con("reserva", *cerca)
		}
	}
	if c.Cupo != nil {
		var n int
		_ = s.DB.QueryRow(ctx, `SELECT count(*) FROM reserva rv JOIN recurso rc ON rc.id=rv.recurso_id
			WHERE rv.unidad_id=$1 AND rc.area_id=$2 AND rv.estado IN ('confirmada','pendiente_pago')
			  AND date_trunc('month', rv.inicio AT TIME ZONE 'America/Lima') = date_trunc('month', $3::date)`,
			unidadID, ri.AreaID, diaEvento.Format("2006-01-02")).Scan(&n)
		if n >= *c.Cupo {
			return P.Err(http.StatusUnprocessableEntity, "CUPO_AGOTADO", fmt.Sprintf("Tu unidad ya usó su cupo de %d reserva(s) de %s este mes.", *c.Cupo, ri.Area))
		}
	}
	return nil
}

// unidadHabilitada: al día o dentro de las excepciones de morosidad del área (misma función que usa la base).
func (s *Server) unidadHabilitada(ctx context.Context, q db.Q, unidadID, areaID int64) bool {
	var ok bool
	if err := q.QueryRow(ctx, `SELECT reserva_unidad_habilitada($1, $2)`, unidadID, areaID).Scan(&ok); err != nil {
		return false
	}
	return ok
}

// ---------- H3 · fotos y reglamento ----------

// subirFotosArea: POST /areas/{aid}/fotos (multipart «fotos») → el área con sus fotos.
func (s *Server) subirFotosArea(w http.ResponseWriter, r *http.Request) {
	aid, err := idRuta(r, "aid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	var ok bool
	if err := s.DB.QueryRow(ctx, `SELECT true FROM area WHERE id=$1 AND edificio_id=$2`, aid, e.ID).Scan(&ok); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el área"))
		return
	}
	fotos, err := archivosDeForm(r, "fotos", "fotos[]", "foto")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if len(fotos) == 0 {
		P.Fallo(w, r, P.Validacion("Adjunta al menos una foto.").Campo("fotos", "Obligatorio."))
		return
	}
	var n int
	_ = s.DB.QueryRow(ctx, `SELECT count(*) FROM area_foto WHERE area_id=$1`, aid).Scan(&n)
	if n+len(fotos) > 8 {
		P.Fallo(w, r, P.Validacion("Cada área admite hasta 8 fotos.").Campo("fotos", "Máximo 8."))
		return
	}
	uid := ses(r).UsuarioID
	for i, f := range fotos {
		if f.Mime == "application/pdf" {
			P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "TIPO_NO_PERMITIDO", "Las fotos del área deben ser imágenes."))
			return
		}
		arch, err := s.guardarArchivo(ctx, s.DB, e.ID, &uid, f)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		if _, err := s.DB.Exec(ctx, `INSERT INTO area_foto (area_id, archivo_id, orden) VALUES ($1,$2,$3)`, aid, arch, n+i); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	s.verArea(w, r)
}

// borrarFotoArea: DELETE /areas/{aid}/fotos/{fid}.
func (s *Server) borrarFotoArea(w http.ResponseWriter, r *http.Request) {
	aid, err := idRuta(r, "aid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	fid, err := idRuta(r, "fid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM area_foto f USING area a WHERE f.id=$1 AND f.area_id=$2 AND a.id=f.area_id AND a.edificio_id=$3`, fid, aid, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("la foto"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// subirReglamentoArea: POST /areas/{aid}/reglamento (multipart «reglamento», PDF o imagen).
func (s *Server) subirReglamentoArea(w http.ResponseWriter, r *http.Request) {
	aid, err := idRuta(r, "aid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	arch, err := archivosDeForm(r, "reglamento", "archivo")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if len(arch) == 0 {
		P.Fallo(w, r, P.Validacion("Adjunta el reglamento (PDF).").Campo("reglamento", "Obligatorio."))
		return
	}
	var ok bool
	if err := s.DB.QueryRow(ctx, `SELECT true FROM area WHERE id=$1 AND edificio_id=$2`, aid, e.ID).Scan(&ok); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el área"))
		return
	}
	uid := ses(r).UsuarioID
	id, err := s.guardarArchivo(ctx, s.DB, e.ID, &uid, arch[0])
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := s.DB.Exec(ctx, `UPDATE area SET reglamento_archivo_id=$2 WHERE id=$1`, aid, id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.verArea(w, r)
}
