package app

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// F2 · turnos, asistencia con foto y checklist del turno.
// La puntualidad se calcula en la base al marcar la entrada, contra el turno vigente, y queda guardada.

var reHora = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// ventanaSalida: la salida cierra la última entrada abierta de las 20 h previas (cubre el turno noche,
// que entra un día y sale al siguiente).
const ventanaSalida = "20 hours"

// ---------- turnos ----------

// listarTurnos: GET /turnos
func (s *Server) listarTurnos(w http.ResponseWriter, r *http.Request) {
	filas, err := db.Filas(r.Context(), s.DB, `SELECT t.id, t.nombre, to_char(t.hora_entrada,'HH24:MI') AS hora_entrada,
			to_char(t.hora_salida,'HH24:MI') AS hora_salida, t.tolerancia_min, t.dias::int[] AS dias, t.activo,
			(SELECT count(*) FROM colaborador c WHERE c.turno_id=t.id AND c.activo) AS colaboradores
		FROM turno t WHERE t.edificio_id=$1 ORDER BY t.activo DESC, t.hora_entrada, lower(t.nombre)`, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

type turnoIn struct {
	Nombre        *string `json:"nombre"`
	HoraEntrada   *string `json:"hora_entrada"`
	HoraSalida    *string `json:"hora_salida"`
	ToleranciaMin *int    `json:"tolerancia_min"`
	Dias          []int   `json:"dias"`
	Activo        *bool   `json:"activo"`
}

func validarTurno(in *turnoIn, nuevo bool) error {
	ev := P.Validacion("Revisa el turno.")
	if in.Nombre != nil {
		v := strings.TrimSpace(*in.Nombre)
		in.Nombre = &v
	}
	if (nuevo && in.Nombre == nil) || (in.Nombre != nil && *in.Nombre == "") {
		ev.Campo("nombre", "Escribe el nombre del turno.")
	}
	if (nuevo && in.HoraEntrada == nil) || (in.HoraEntrada != nil && !reHora.MatchString(*in.HoraEntrada)) {
		ev.Campo("hora_entrada", "Hora en formato HH:MM.")
	}
	if (nuevo && in.HoraSalida == nil) || (in.HoraSalida != nil && !reHora.MatchString(*in.HoraSalida)) {
		ev.Campo("hora_salida", "Hora en formato HH:MM.")
	}
	if in.HoraEntrada != nil && in.HoraSalida != nil && *in.HoraEntrada == *in.HoraSalida {
		ev.Campo("hora_salida", "La salida no puede ser a la misma hora que la entrada.")
	}
	if in.ToleranciaMin != nil && (*in.ToleranciaMin < 0 || *in.ToleranciaMin > 120) {
		ev.Campo("tolerancia_min", "Entre 0 y 120 minutos.")
	}
	if in.Dias != nil {
		if len(in.Dias) == 0 {
			ev.Campo("dias", "Elige al menos un día.")
		}
		for _, d := range in.Dias {
			if d < 1 || d > 7 {
				ev.Campo("dias", "Los días van de 1 (lunes) a 7 (domingo).")
			}
		}
	}
	if len(ev.Campos) > 0 {
		return ev
	}
	return nil
}

func errTurno(err error) error {
	if err != nil && strings.Contains(err.Error(), "turno_nombre_uq") {
		return P.Conflicto("TURNO_DUPLICADO", "Ya hay un turno con ese nombre.")
	}
	return err
}

// crearTurno: POST /turnos {nombre, hora_entrada, hora_salida, tolerancia_min, dias}
func (s *Server) crearTurno(w http.ResponseWriter, r *http.Request) {
	var in turnoIn
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := validarTurno(&in, true); err != nil {
		P.Fallo(w, r, err)
		return
	}
	tol := 10
	if in.ToleranciaMin != nil {
		tol = *in.ToleranciaMin
	}
	dias := in.Dias
	if dias == nil {
		dias = []int{1, 2, 3, 4, 5, 6}
	}
	var id int64
	err := s.DB.QueryRow(r.Context(), `INSERT INTO turno (edificio_id, nombre, hora_entrada, hora_salida, tolerancia_min, dias)
		VALUES ($1,$2,$3::time,$4::time,$5,$6::smallint[]) RETURNING id`, edf(r).ID, *in.Nombre, *in.HoraEntrada, *in.HoraSalida, tol, dias).Scan(&id)
	if err != nil {
		P.Fallo(w, r, errTurno(err))
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// editarTurno: PUT /turnos/{id} — campos parciales. No toca las asistencias ya marcadas.
func (s *Server) editarTurno(w http.ResponseWriter, r *http.Request) {
	id := idURL(r, "id")
	var in turnoIn
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := validarTurno(&in, false); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE turno SET nombre=COALESCE($3,nombre), hora_entrada=COALESCE($4::time,hora_entrada),
			hora_salida=COALESCE($5::time,hora_salida), tolerancia_min=COALESCE($6,tolerancia_min),
			dias=COALESCE($7::smallint[],dias), activo=COALESCE($8,activo)
		WHERE id=$1 AND edificio_id=$2`, id, edf(r).ID, in.Nombre, in.HoraEntrada, in.HoraSalida, in.ToleranciaMin, in.Dias, in.Activo)
	if err != nil {
		P.Fallo(w, r, errTurno(err))
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("el turno"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

// ---------- checklist ----------

// listarChecklist: GET /checklist
func (s *Server) listarChecklist(w http.ResponseWriter, r *http.Request) {
	filas, err := db.Filas(r.Context(), s.DB, `SELECT i.id, i.texto, i.turno_id, t.nombre AS turno, i.orden
		FROM checklist_item i LEFT JOIN turno t ON t.id=i.turno_id
		WHERE i.edificio_id=$1 AND i.activo ORDER BY i.orden, i.id`, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearChecklistItem: POST /checklist {texto, turno_id?}
func (s *Server) crearChecklistItem(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in struct {
		Texto   string `json:"texto"`
		TurnoID *int64 `json:"turno_id"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.Texto = strings.TrimSpace(in.Texto)
	if in.Texto == "" {
		P.Fallo(w, r, P.Validacion("Escribe la tarea.").Campo("texto", "Obligatorio."))
		return
	}
	turno := idONulo(in.TurnoID)
	if turno != nil {
		var ok bool
		_ = s.DB.QueryRow(r.Context(), `SELECT true FROM turno WHERE id=$1 AND edificio_id=$2`, *turno, e.ID).Scan(&ok)
		if !ok {
			P.Fallo(w, r, P.Validacion("Elige un turno del edificio.").Campo("turno_id", "No existe."))
			return
		}
	}
	var id int64
	if err := s.DB.QueryRow(r.Context(), `INSERT INTO checklist_item (edificio_id, turno_id, texto, orden)
		VALUES ($1,$2,$3,(SELECT COALESCE(max(orden),0)+1 FROM checklist_item WHERE edificio_id=$1)) RETURNING id`, e.ID, turno, in.Texto).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// desactivarChecklistItem: DELETE /checklist/{id} — se desactiva para no perder lo ya marcado.
func (s *Server) desactivarChecklistItem(w http.ResponseWriter, r *http.Request) {
	id := idURL(r, "id")
	ct, err := s.DB.Exec(r.Context(), `UPDATE checklist_item SET activo=false WHERE id=$1 AND edificio_id=$2`, id, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("la tarea"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

// ---------- marcado ----------

type marca struct {
	colaborador int64
	tipo        string // entrada | salida
	en          time.Time
	foto        *int64
	manual      bool
	nota        string
	usuario     int64
}

// registrarMarca aplica la entrada o la salida dentro de la transacción y devuelve la fila resultante.
func registrarMarca(ctx context.Context, tx pgx.Tx, eid int64, m marca) (map[string]any, error) {
	switch m.tipo {
	case "entrada":
		var id int64
		err := tx.QueryRow(ctx, `WITH x AS (SELECT ($3::timestamptz AT TIME ZONE 'America/Lima') AS loc)
			INSERT INTO asistencia (edificio_id, colaborador_id, fecha, turno_id, entrada_en, entrada_foto_id, minutos_tarde, puntual, manual, nota, registrado_por)
			SELECT $1::bigint, c.id, x.loc::date, t.id, $3::timestamptz, $4::bigint, m.min,
				CASE WHEN t.id IS NULL THEN NULL ELSE m.min <= t.tolerancia_min END, $5::boolean, $6::text, $7::bigint
			FROM colaborador c CROSS JOIN x
			LEFT JOIN turno t ON t.id=c.turno_id AND t.activo
			-- Minutos después de la hora de entrada del día (llegar antes cuenta como 0).
			CROSS JOIN LATERAL (SELECT CASE WHEN t.id IS NULL THEN NULL
				ELSE GREATEST(0, floor(extract(epoch FROM x.loc - (x.loc::date + t.hora_entrada)) / 60))::int END AS min) m
			WHERE c.id=$2::bigint AND c.edificio_id=$1::bigint
			RETURNING id`, eid, m.colaborador, m.en, m.foto, m.manual, m.nota, m.usuario).Scan(&id)
		if err != nil {
			if esUnico(err) {
				return nil, P.Conflicto("YA_MARCO_ENTRADA", "La entrada de ese día ya está marcada.")
			}
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, P.NoEncontrado("el colaborador")
			}
			return nil, err
		}
		return filaAsistencia(ctx, tx, id)
	case "salida":
		var id int64
		err := tx.QueryRow(ctx, `SELECT id FROM asistencia
			WHERE colaborador_id=$1 AND edificio_id=$2 AND salida_en IS NULL AND entrada_en < $3 AND entrada_en > $3 - $4::interval
			ORDER BY entrada_en DESC LIMIT 1 FOR UPDATE`, m.colaborador, eid, m.en, ventanaSalida).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, P.Conflicto("SIN_ENTRADA", "No hay una entrada abierta que cerrar: marca primero la entrada.")
		}
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE asistencia SET salida_en=$2, salida_foto_id=$3,
				manual = manual OR $4, nota = CASE WHEN $5 = '' THEN nota ELSE btrim(nota || ' ' || $5) END
			WHERE id=$1`, id, m.en, m.foto, m.manual, m.nota); err != nil {
			return nil, err
		}
		return filaAsistencia(ctx, tx, id)
	}
	return nil, P.Validacion("Indica si es entrada o salida.").Campo("tipo", "entrada o salida.")
}

const sqlAsistencia = `SELECT a.id, a.colaborador_id, c.nombre AS colaborador, c.cargo, to_char(a.fecha,'YYYY-MM-DD') AS fecha,
		a.turno_id, t.nombre AS turno, to_char(t.hora_entrada,'HH24:MI') AS hora_turno,
		to_char(a.entrada_en AT TIME ZONE 'America/Lima','HH24:MI') AS entrada, a.entrada_foto_id,
		to_char(a.salida_en AT TIME ZONE 'America/Lima','HH24:MI') AS salida, a.salida_foto_id,
		a.minutos_tarde, a.puntual, a.manual, a.nota,
		(SELECT count(*) FROM asistencia_checklist k WHERE k.asistencia_id=a.id) AS checklist_hechos
	FROM asistencia a JOIN colaborador c ON c.id=a.colaborador_id LEFT JOIN turno t ON t.id=a.turno_id`

func filaAsistencia(ctx context.Context, q db.Q, id int64) (map[string]any, error) {
	return db.Fila(ctx, q, sqlAsistencia+` WHERE a.id=$1`, id)
}

// conFotos añade los enlaces firmados de las fotos de entrada y salida.
func (s *Server) conFotos(f map[string]any) {
	if v, ok := f["entrada_foto_id"].(int64); ok {
		f["entrada_foto_url"] = s.Firma.URL(v)
	}
	if v, ok := f["salida_foto_id"].(int64); ok {
		f["salida_foto_url"] = s.Firma.URL(v)
	}
}

// miColaborador: el colaborador activo enlazado a la cuenta de la sesión (0 si no hay).
func (s *Server) miColaborador(r *http.Request) int64 {
	var id int64
	_ = s.DB.QueryRow(r.Context(), `SELECT id FROM colaborador WHERE usuario_id=$1 AND edificio_id=$2 AND activo`, ses(r).UsuarioID, edf(r).ID).Scan(&id)
	return id
}

// marcarAsistencia: POST /asistencia/marcar (multipart: tipo, foto) — el colaborador marca con la hora del servidor.
// La foto es obligatoria: es la prueba de que estuvo en el edificio.
func (s *Server) marcarAsistencia(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	cid := s.miColaborador(r)
	if cid == 0 {
		P.Fallo(w, r, P.Conflicto("SIN_COLABORADOR", "Tu cuenta no está enlazada a un colaborador del edificio. Pídeselo a la administración."))
		return
	}
	fotos, err := archivosDeForm(r, "foto")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	tipo := campo(r, "tipo")
	if tipo != "entrada" && tipo != "salida" {
		P.Fallo(w, r, P.Validacion("Indica si es entrada o salida.").Campo("tipo", "entrada o salida."))
		return
	}
	if len(fotos) == 0 {
		P.Fallo(w, r, P.Validacion("Toma la foto para marcar.").Campo("foto", "Obligatoria."))
		return
	}
	if !strings.HasPrefix(fotos[0].Mime, "image/") {
		P.Fallo(w, r, P.Validacion("La marca va con una foto, no con un PDF.").Campo("foto", "Solo imagen."))
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	uid := ses(r).UsuarioID
	fotoID, err := s.guardarArchivo(ctx, tx, e.ID, &uid, fotos[0])
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	fila, err := registrarMarca(ctx, tx, e.ID, marca{colaborador: cid, tipo: tipo, en: time.Now(), foto: &fotoID, usuario: uid})
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.conFotos(fila)
	P.JSON(w, http.StatusCreated, fila)
}

// asistenciaManual: POST /asistencia/manual {colaborador_id, tipo, en: "AAAA-MM-DDTHH:MM" (Lima), nota}
// La administración corrige o registra por el colaborador (olvidó el teléfono). Queda marcada como manual.
func (s *Server) asistenciaManual(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	var in struct {
		ColaboradorID int64  `json:"colaborador_id"`
		Tipo          string `json:"tipo"`
		En            string `json:"en"`
		Nota          string `json:"nota"`
	}
	var fotos []Subido
	if esMultipart(r) {
		var err error
		if fotos, err = archivosDeForm(r, "foto"); err != nil {
			P.Fallo(w, r, err)
			return
		}
		in.ColaboradorID, _ = strconv.ParseInt(campo(r, "colaborador_id"), 10, 64)
		in.Tipo, in.En, in.Nota = campo(r, "tipo"), campo(r, "en"), campo(r, "nota")
	} else if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa la marca.")
	en, err := time.ParseInLocation("2006-01-02T15:04", strings.TrimSpace(in.En), P.Lima)
	if err != nil {
		ev.Campo("en", "Fecha y hora en formato AAAA-MM-DDTHH:MM.")
	} else if en.After(time.Now().Add(5 * time.Minute)) {
		ev.Campo("en", "No se marca a futuro.")
	}
	if in.Tipo != "entrada" && in.Tipo != "salida" {
		ev.Campo("tipo", "entrada o salida.")
	}
	if strings.TrimSpace(in.Nota) == "" {
		ev.Campo("nota", "Explica por qué se registra a mano.")
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	if !s.colaboradorDelEdificio(r, in.ColaboradorID) {
		P.Fallo(w, r, P.NoEncontrado("el colaborador"))
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	uid := ses(r).UsuarioID
	var fotoID *int64
	if len(fotos) > 0 {
		id, err := s.guardarArchivo(ctx, tx, e.ID, &uid, fotos[0])
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		fotoID = &id
	}
	fila, err := registrarMarca(ctx, tx, e.ID, marca{colaborador: in.ColaboradorID, tipo: in.Tipo, en: en, foto: fotoID, manual: true, nota: strings.TrimSpace(in.Nota), usuario: uid})
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.conFotos(fila)
	P.JSON(w, http.StatusCreated, fila)
}

// miAsistenciaHoy: GET /asistencia/hoy — lo que ve el colaborador en el teléfono: su turno,
// la marca abierta (o la de hoy) y el checklist con lo ya hecho.
func (s *Server) miAsistenciaHoy(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	cid := s.miColaborador(r)
	if cid == 0 {
		P.JSON(w, http.StatusOK, map[string]any{"colaborador": nil, "asistencia": nil, "checklist": []any{}})
		return
	}
	col, err := db.Fila(ctx, s.DB, `SELECT c.id, c.nombre, c.cargo, c.turno_id, t.nombre AS turno,
			to_char(t.hora_entrada,'HH24:MI') AS hora_entrada, to_char(t.hora_salida,'HH24:MI') AS hora_salida, t.tolerancia_min
		FROM colaborador c LEFT JOIN turno t ON t.id=c.turno_id WHERE c.id=$1`, cid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	// La abierta de las últimas horas (turno noche) o, si no, la de hoy.
	asis, err := db.Fila(ctx, s.DB, sqlAsistencia+` WHERE a.colaborador_id=$1
			AND ((a.salida_en IS NULL AND a.entrada_en > now() - $2::interval) OR a.fecha=(now() AT TIME ZONE 'America/Lima')::date)
		ORDER BY a.entrada_en DESC LIMIT 1`, cid, ventanaSalida)
	var asistencia any
	var asisID int64
	if err == nil {
		s.conFotos(asis)
		asistencia = asis
		asisID, _ = asis["id"].(int64)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		P.Fallo(w, r, err)
		return
	}
	items, err := db.Filas(ctx, s.DB, `SELECT i.id, i.texto,
			EXISTS(SELECT 1 FROM asistencia_checklist k WHERE k.asistencia_id=$3 AND k.item_id=i.id) AS hecho
		FROM checklist_item i
		WHERE i.edificio_id=$1 AND i.activo AND (i.turno_id IS NULL OR i.turno_id=$2)
		ORDER BY i.orden, i.id`, e.ID, col["turno_id"], asisID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"colaborador": col, "asistencia": asistencia, "checklist": items})
}

// marcarChecklist: POST /asistencia/{id}/checklist {item_id, hecho}
// Solo sobre la propia asistencia (o cualquiera, si administra el personal).
func (s *Server) marcarChecklist(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	aid := idURL(r, "id")
	var in struct {
		ItemID int64 `json:"item_id"`
		Hecho  bool  `json:"hecho"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	var dueno int64
	var turno *int64
	if err := s.DB.QueryRow(ctx, `SELECT colaborador_id, turno_id FROM asistencia WHERE id=$1 AND edificio_id=$2`, aid, e.ID).Scan(&dueno, &turno); err != nil {
		P.Fallo(w, r, P.NoEncontrado("la asistencia"))
		return
	}
	if dueno != s.miColaborador(r) && !e.Puede("personal.administrar") {
		P.Fallo(w, r, P.Prohibido("NO_ES_TUYA", "Solo puedes marcar el checklist de tu propia asistencia."))
		return
	}
	var ok bool
	_ = s.DB.QueryRow(ctx, `SELECT true FROM checklist_item WHERE id=$1 AND edificio_id=$2 AND activo
		AND (turno_id IS NULL OR turno_id IS NOT DISTINCT FROM $3)`, in.ItemID, e.ID, turno).Scan(&ok)
	if !ok {
		P.Fallo(w, r, P.Validacion("Esa tarea no es de este turno.").Campo("item_id", "No corresponde."))
		return
	}
	var err error
	if in.Hecho {
		_, err = s.DB.Exec(ctx, `INSERT INTO asistencia_checklist (asistencia_id, item_id, hecho_por) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, aid, in.ItemID, ses(r).UsuarioID)
	} else {
		_, err = s.DB.Exec(ctx, `DELETE FROM asistencia_checklist WHERE asistencia_id=$1 AND item_id=$2`, aid, in.ItemID)
	}
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"asistencia_id": aid, "item_id": in.ItemID, "hecho": in.Hecho})
}

// ---------- consulta y puntualidad ----------

// fechaLima lee AAAA-MM-DD de la query o devuelve def.
func fechaLima(r *http.Request, clave string, def time.Time) (time.Time, error) {
	v := strings.TrimSpace(r.URL.Query().Get(clave))
	if v == "" {
		return def, nil
	}
	t, err := time.ParseInLocation("2006-01-02", v, P.Lima)
	if err != nil {
		return t, P.Validacion("Fecha inválida.").Campo(clave, "Formato AAAA-MM-DD.")
	}
	return t, nil
}

// listarAsistencia: GET /asistencia?fecha= — marcas del día con fotos, y quién tenía turno y no marcó.
func (s *Server) listarAsistencia(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	hoy := time.Now().In(P.Lima)
	f, err := fechaLima(r, "fecha", hoy)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	fecha := f.Format("2006-01-02")
	filas, err := db.Filas(ctx, s.DB, sqlAsistencia+` WHERE a.edificio_id=$1 AND a.fecha=$2::date ORDER BY a.entrada_en`, e.ID, fecha)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, x := range filas {
		s.conFotos(x)
	}
	ausentes, err := db.Filas(ctx, s.DB, `SELECT c.id, c.nombre, c.cargo, t.nombre AS turno, to_char(t.hora_entrada,'HH24:MI') AS hora_entrada
		FROM colaborador c JOIN turno t ON t.id=c.turno_id AND t.activo
		WHERE c.edificio_id=$1 AND c.activo AND c.fecha_ingreso <= $2::date
			AND extract(isodow FROM $2::date)::smallint = ANY(t.dias)
			AND NOT EXISTS (SELECT 1 FROM asistencia a WHERE a.colaborador_id=c.id AND a.fecha=$2::date)
		ORDER BY t.hora_entrada, c.nombre`, e.ID, fecha)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"fecha": fecha, "datos": filas, "total": len(filas), "pagina": 1, "ausentes": ausentes})
}

// puntualidad: GET /asistencia/puntualidad?desde=&hasta= (por defecto, el mes en curso).
// Faltas = días del turno sin marca, contados hasta ayer (hoy aún puede llegar).
func (s *Server) puntualidad(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	hoy := time.Now().In(P.Lima)
	inicio := time.Date(hoy.Year(), hoy.Month(), 1, 0, 0, 0, 0, P.Lima)
	desde, err := fechaLima(r, "desde", inicio)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	hasta, err := fechaLima(r, "hasta", hoy)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if hasta.Before(desde) {
		P.Fallo(w, r, P.Validacion("El rango está al revés.").Campo("hasta", "Debe ser igual o posterior a «desde»."))
		return
	}
	if hasta.Sub(desde) > 370*24*time.Hour {
		P.Fallo(w, r, P.Validacion("El rango es muy largo.").Campo("hasta", "Máximo un año."))
		return
	}
	d, h := desde.Format("2006-01-02"), hasta.Format("2006-01-02")
	filas, err := db.Filas(ctx, s.DB, `WITH dias AS (
			SELECT g::date AS fecha FROM generate_series($2::date,
				LEAST($3::date, (now() AT TIME ZONE 'America/Lima')::date - 1), '1 day') g)
		SELECT c.id, c.nombre, c.cargo, t.nombre AS turno,
			count(a.id) AS asistencias,
			count(a.id) FILTER (WHERE a.puntual) AS puntuales,
			count(a.id) FILTER (WHERE a.puntual = false) AS tardanzas,
			COALESCE(sum(a.minutos_tarde), 0) AS minutos_tarde,
			count(a.id) FILTER (WHERE a.salida_en IS NULL AND a.fecha < (now() AT TIME ZONE 'America/Lima')::date) AS sin_salida,
			count(a.id) FILTER (WHERE a.manual) AS manuales,
			(SELECT count(*) FROM dias WHERE t.id IS NOT NULL AND dias.fecha >= c.fecha_ingreso
				AND extract(isodow FROM dias.fecha)::smallint = ANY(t.dias)
				AND NOT EXISTS (SELECT 1 FROM asistencia x WHERE x.colaborador_id=c.id AND x.fecha=dias.fecha)) AS faltas
		FROM colaborador c
		LEFT JOIN turno t ON t.id=c.turno_id
		LEFT JOIN asistencia a ON a.colaborador_id=c.id AND a.fecha BETWEEN $2::date AND $3::date
		WHERE c.edificio_id=$1 AND c.activo
		GROUP BY c.id, t.id
		ORDER BY lower(c.nombre)`, e.ID, d, h)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var tot struct{ asis, punt, tard, faltas int64 }
	for _, f := range filas {
		a, p, t, fa := f["asistencias"].(int64), f["puntuales"].(int64), f["tardanzas"].(int64), f["faltas"].(int64)
		f["pct_puntualidad"] = pctPuntual(p, p+t)
		tot.asis += a
		tot.punt += p
		tot.tard += t
		tot.faltas += fa
	}
	P.JSON(w, http.StatusOK, map[string]any{
		"desde": d, "hasta": h, "datos": filas, "total": len(filas), "pagina": 1,
		"resumen": map[string]any{"asistencias": tot.asis, "puntuales": tot.punt, "tardanzas": tot.tard, "faltas": tot.faltas,
			"pct_puntualidad": pctPuntual(tot.punt, tot.punt+tot.tard)},
	})
}

// pctPuntual: porcentaje con un decimal sobre las marcas con turno; nil si no hay ninguna.
func pctPuntual(puntuales, conTurno int64) any {
	if conTurno == 0 {
		return nil
	}
	return float64(puntuales*1000/conTurno) / 10
}
