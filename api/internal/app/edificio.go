package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"edisys/api/internal/auth"
	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// ---------- ficha del edificio ----------

func (s *Server) verEdificio(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	f, err := db.Fila(r.Context(), s.DB, `SELECT e.id, e.nombre, e.direccion, e.distrito, e.dia_corte, e.dias_vencimiento, e.dias_gracia,
			e.politica_reparto, e.modo_cobro_reservas, e.cobra_agua, e.umbral_aprobacion_cts, e.modo_aprobacion, e.yape_numero, e.normas_texto,
			e.manual_archivo_id, e.reglamento_archivo_id, a.nombre AS administradora,
			(SELECT count(*) FROM unidad u WHERE u.edificio_id=e.id AND u.activo) AS unidades,
			(SELECT COALESCE(sum(participacion_pct),0)::float8 FROM unidad u WHERE u.edificio_id=e.id AND u.activo) AS suma_participacion_pct,
			(SELECT max(periodo) FROM periodo p WHERE p.edificio_id=e.id) AS periodo_abierto
		FROM edificio e JOIN administradora a ON a.id=e.administradora_id WHERE e.id=$1`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, k := range []string{"manual_archivo_id", "reglamento_archivo_id"} {
		if id, ok := f[k].(int64); ok {
			f[strings.TrimSuffix(k, "_archivo_id")+"_url"] = s.Firma.URL(id)
		}
	}
	f["rol"] = e.Rol
	P.JSON(w, http.StatusOK, f)
}

func (s *Server) editarEdificio(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	permitidos := map[string]bool{"nombre": true, "direccion": true, "distrito": true, "dia_corte": true, "dias_vencimiento": true, "dias_gracia": true,
		"politica_reparto": true, "modo_cobro_reservas": true, "cobra_agua": true, "umbral_aprobacion_cts": true, "modo_aprobacion": true,
		"yape_numero": true, "normas_texto": true}
	e := edf(r)
	ctx := r.Context()
	antes, _ := db.Fila(ctx, s.DB, `SELECT * FROM edificio WHERE id=$1`, e.ID)
	sets := []string{}
	args := []any{e.ID}
	for k, v := range in {
		if !permitidos[k] {
			continue
		}
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d", k, len(args)))
	}
	if len(sets) == 0 {
		P.Fallo(w, r, P.Validacion("No hay campos para actualizar."))
		return
	}
	if _, err := s.DB.Exec(ctx, `UPDATE edificio SET `+strings.Join(sets, ", ")+` WHERE id=$1`, args...); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "edificio", "editar", "edificio", e.ID, antes, in)
	s.verEdificio(w, r)
}

// ---------- unidades ----------

const sqlPropietario = `COALESCE((SELECT json_build_object('persona_id', pe.id, 'nombre', pe.nombre, 'dni_ruc', pe.dni_ruc, 'correo', pe.correo, 'celular', pe.celular, 'desde', up.desde)
	FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id WHERE up.unidad_id=u.id AND up.rol='%s' AND up.hasta IS NULL LIMIT 1), 'null'::json)`

func (s *Server) listarUnidades(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	pagina, por := paginacion(r)
	buscar := "%" + strings.TrimSpace(r.URL.Query().Get("buscar")) + "%"
	if r.URL.Query().Get("buscar") == "" {
		buscar = "%"
	}
	ctx := r.Context()
	var total int64
	cond := `u.edificio_id=$1 AND (u.codigo ILIKE $2 OR EXISTS (SELECT 1 FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id
		WHERE up.unidad_id=u.id AND up.hasta IS NULL AND (pe.nombre ILIKE $2 OR pe.dni_ruc ILIKE $2)))`
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM unidad u WHERE `+cond, e.ID, buscar).Scan(&total); err != nil {
		P.Fallo(w, r, err)
		return
	}
	filas, err := db.Filas(ctx, s.DB, `SELECT u.id, u.codigo, u.tipo, u.piso, u.participacion_pct::float8 AS participacion_pct, u.alquilado, u.activo,
			`+fmt.Sprintf(sqlPropietario, "propietario")+` AS propietario, `+fmt.Sprintf(sqlPropietario, "inquilino")+` AS inquilino,
			COALESCE((SELECT sum(r.total_cts - r.pagado_cts) FROM recibo r WHERE r.unidad_id=u.id AND r.estado IN ('emitido','pagado_parcial')),0)::bigint AS deuda_cts,
			es_moroso(u.id) AS moroso
		FROM unidad u WHERE `+cond+` ORDER BY u.codigo LIMIT $3 OFFSET $4`, e.ID, buscar, por, (pagina-1)*por)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, f := range filas {
		enmascarar(f["propietario"])
		enmascarar(f["inquilino"])
	}
	var suma float64
	_ = s.DB.QueryRow(ctx, `SELECT COALESCE(sum(participacion_pct),0)::float8 FROM unidad WHERE edificio_id=$1 AND activo`, e.ID).Scan(&suma)
	resp := paginado(filas, total, pagina)
	resp["suma_participacion_pct"] = math.Round(suma*10000) / 10000
	P.JSON(w, http.StatusOK, resp)
}

// enmascarar: los DNI no se muestran enteros en listados (Ley 29733).
func enmascarar(v any) {
	if m, ok := v.(map[string]any); ok {
		if d, ok := m["dni_ruc"].(string); ok {
			m["dni_ruc"] = P.EnmascararDNI(d)
		}
	}
}

func (s *Server) verUnidad(w http.ResponseWriter, r *http.Request) {
	uid, err := idRuta(r, "uid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	u, err := db.Fila(ctx, s.DB, `SELECT u.id, u.codigo, u.tipo, u.piso, u.participacion_pct::float8 AS participacion_pct, u.alquilado, u.activo, u.permisos_inquilino,
			deuda_vencida_cts(u.id) AS deuda_vencida_cts, es_moroso(u.id) AS moroso
		FROM unidad u WHERE u.id=$1 AND u.edificio_id=$2`, uid, e.ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	personas, err := db.Filas(ctx, s.DB, `SELECT up.id, up.rol, to_char(up.desde,'YYYY-MM-DD') AS desde, to_char(up.hasta,'YYYY-MM-DD') AS hasta, (up.hasta IS NULL) AS vigente,
			pe.id AS persona_id, pe.nombre, pe.dni_ruc, pe.correo, pe.celular, pe.usuario_id
		FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id WHERE up.unidad_id=$1 ORDER BY up.rol, up.desde DESC, up.id DESC`, uid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if e.Rol != "administrador" && e.Rol != "superadmin" {
		for _, p := range personas {
			p["dni_ruc"] = P.EnmascararDNI(p["dni_ruc"].(string))
		}
	}
	medidores, err := db.Filas(ctx, s.DB, `SELECT m.id, m.tipo, m.serie, m.orden_ronda, m.lectura_inicial::text AS lectura_inicial,
			(SELECT l.valor::text FROM lectura l JOIN periodo p ON p.id=l.periodo_id WHERE l.medidor_id=m.id ORDER BY p.periodo DESC LIMIT 1) AS ultima_lectura
		FROM medidor m WHERE m.unidad_id=$1 ORDER BY m.tipo`, uid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	deuda, err := db.Filas(ctx, s.DB, `SELECT d.id, d.periodo, d.monto_cts, d.recibo_id, COALESCE(r.total_cts - r.pagado_cts, 0) AS saldo_cts
		FROM deuda_inicial d LEFT JOIN recibo r ON r.id = d.recibo_id WHERE d.unidad_id=$1 ORDER BY d.periodo`, uid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	u["personas"] = personas
	u["historial"] = personas
	u["medidores"] = medidores
	u["deuda_inicial"] = deuda
	P.JSON(w, http.StatusOK, u)
}

type unidadIn struct {
	Codigo           string   `json:"codigo"`
	Tipo             string   `json:"tipo"`
	Piso             *int     `json:"piso"`
	ParticipacionPct *float64 `json:"participacion_pct"`
	Alquilado        *bool    `json:"alquilado"`
	Activo           *bool    `json:"activo"`
}

func (s *Server) crearUnidad(w http.ResponseWriter, r *http.Request) {
	var in unidadIn
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if strings.TrimSpace(in.Codigo) == "" {
		P.Fallo(w, r, P.Validacion("Falta el código.").Campo("codigo", "Obligatorio."))
		return
	}
	if in.Tipo == "" {
		in.Tipo = "departamento"
	}
	part := 0.0
	if in.ParticipacionPct != nil {
		part = *in.ParticipacionPct
	}
	f, err := db.Fila(r.Context(), s.DB, `INSERT INTO unidad (edificio_id, codigo, tipo, piso, participacion_pct, alquilado) VALUES ($1,$2,$3,$4,$5,COALESCE($6,false))
		RETURNING id, codigo, tipo, piso, participacion_pct::float8 AS participacion_pct, alquilado, activo`, edf(r).ID, strings.TrimSpace(in.Codigo), in.Tipo, in.Piso, part, in.Alquilado)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, f)
}

func (s *Server) editarUnidad(w http.ResponseWriter, r *http.Request) {
	uid, err := idRuta(r, "uid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in unidadIn
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ctx := r.Context()
	antes, err := db.Fila(ctx, s.DB, `SELECT codigo, tipo, piso, participacion_pct::float8 AS participacion_pct, alquilado, activo FROM unidad WHERE id=$1 AND edificio_id=$2`, uid, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	var cod, tipo *string
	if in.Codigo != "" {
		cod = &in.Codigo
	}
	if in.Tipo != "" {
		tipo = &in.Tipo
	}
	f, err := db.Fila(ctx, s.DB, `UPDATE unidad SET codigo=COALESCE($3,codigo), tipo=COALESCE($4,tipo), piso=COALESCE($5,piso),
			participacion_pct=COALESCE($6,participacion_pct), alquilado=COALESCE($7,alquilado), activo=COALESCE($8,activo)
		WHERE id=$1 AND edificio_id=$2 RETURNING id, codigo, tipo, piso, participacion_pct::float8 AS participacion_pct, alquilado, activo`,
		uid, edf(r).ID, cod, tipo, in.Piso, in.ParticipacionPct, in.Alquilado, in.Activo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "unidades", "editar", "unidad", uid, antes, f)
	P.JSON(w, http.StatusOK, f)
}

// borrarUnidad: solo si no tiene recibos; si tiene, se desactiva.
func (s *Server) borrarUnidad(w http.ResponseWriter, r *http.Request) {
	uid, err := idRuta(r, "uid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	ctx := r.Context()
	e := edf(r)
	var recibos int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM recibo WHERE unidad_id=$1`, uid).Scan(&recibos); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if recibos > 0 {
		if _, err := s.DB.Exec(ctx, `UPDATE unidad SET activo=false WHERE id=$1 AND edificio_id=$2`, uid, e.ID); err != nil {
			P.Fallo(w, r, err)
			return
		}
		P.JSON(w, http.StatusOK, map[string]any{"id": uid, "desactivada": true, "mensaje": "La unidad tiene recibos: se desactivó en vez de borrarse."})
		return
	}
	tag, err := s.DB.Exec(ctx, `DELETE FROM unidad WHERE id=$1 AND edificio_id=$2`, uid, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// asignarPersona: POST /unidades/{uid}/personas {persona|nombre…, rol, desde}. Cierra al anterior (historial, RF-02).
func (s *Server) asignarPersona(w http.ResponseWriter, r *http.Request) {
	uid, err := idRuta(r, "uid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		PersonaID int64  `json:"persona_id"`
		Nombre    string `json:"nombre"`
		DNIRUC    string `json:"dni_ruc"`
		Correo    string `json:"correo"`
		Celular   string `json:"celular"`
		Rol       string `json:"rol"`
		Desde     string `json:"desde"`
		Persona   *struct {
			Nombre  string `json:"nombre"`
			DNIRUC  string `json:"dni_ruc"`
			Correo  string `json:"correo"`
			Celular string `json:"celular"`
		} `json:"persona"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.Persona != nil {
		in.Nombre, in.DNIRUC, in.Correo, in.Celular = in.Persona.Nombre, in.Persona.DNIRUC, in.Persona.Correo, in.Persona.Celular
	}
	if in.Rol != "propietario" && in.Rol != "inquilino" {
		P.Fallo(w, r, P.Validacion("Rol inválido.").Campo("rol", "propietario o inquilino."))
		return
	}
	desde := time.Now().In(P.Lima).Format("2006-01-02")
	if in.Desde != "" {
		desde = in.Desde
	}
	e := edf(r)
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT true FROM unidad WHERE id=$1 AND edificio_id=$2`, uid, e.ID).Scan(&ok); err != nil {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	pid := in.PersonaID
	if pid == 0 {
		if msg := validarDNI(in.DNIRUC); msg != "" && in.DNIRUC != "" {
			P.Fallo(w, r, P.Validacion(msg).Campo("dni_ruc", msg))
			return
		}
		if strings.TrimSpace(in.Nombre) == "" {
			P.Fallo(w, r, P.Validacion("Falta el nombre.").Campo("nombre", "Obligatorio."))
			return
		}
		if err := tx.QueryRow(ctx, `INSERT INTO persona (edificio_id, nombre, dni_ruc, correo, celular) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
			e.ID, strings.TrimSpace(in.Nombre), in.DNIRUC, strings.TrimSpace(in.Correo), in.Celular).Scan(&pid); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE unidad_persona SET hasta=$3 WHERE unidad_id=$1 AND rol=$2 AND hasta IS NULL`, uid, in.Rol, desde); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `INSERT INTO unidad_persona (unidad_id, persona_id, rol, desde) VALUES ($1,$2,$3,$4)`, uid, pid, in.Rol, desde); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.Rol == "inquilino" {
		_, _ = tx.Exec(ctx, `UPDATE unidad SET alquilado=true WHERE id=$1`, uid)
	}
	enlace, err := s.usuarioParaPersona(ctx, tx, e.ID, pid, in.Rol)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"unidad_id": uid, "persona_id": pid, "rol": in.Rol, "desde": desde, "enlace_invitacion": enlace})
}

// usuarioParaPersona crea el usuario (sin clave) de una persona con correo y devuelve el enlace de invitación.
func (s *Server) usuarioParaPersona(ctx context.Context, tx pgx.Tx, eid, pid int64, rol string) (string, error) {
	var correo string
	var usuario *int64
	if err := tx.QueryRow(ctx, `SELECT correo, usuario_id FROM persona WHERE id=$1`, pid).Scan(&correo, &usuario); err != nil {
		return "", err
	}
	if usuario != nil || correo == "" {
		if usuario != nil {
			_, err := tx.Exec(ctx, `INSERT INTO usuario_edificio_rol (usuario_id, edificio_id, rol) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, *usuario, eid, rol)
			return "", err
		}
		return "", nil
	}
	var uid int64
	err := tx.QueryRow(ctx, `SELECT id FROM usuario WHERE lower(correo)=lower($1)`, correo).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.QueryRow(ctx, `INSERT INTO usuario (administradora_id, correo, nombre, telefono)
			SELECT e.administradora_id, $2, p.nombre, p.celular FROM edificio e, persona p WHERE e.id=$1 AND p.id=$3 RETURNING id`, eid, correo, pid).Scan(&uid); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE persona SET usuario_id=$2 WHERE id=$1`, pid, uid); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO usuario_edificio_rol (usuario_id, edificio_id, rol) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, eid, rol); err != nil {
		return "", err
	}
	return s.crearInvitacion(ctx, tx, uid)
}

func (s *Server) crearInvitacion(ctx context.Context, q db.Q, uid int64) (string, error) {
	tok, hash := auth.TokenOpaco()
	if _, err := q.Exec(ctx, `INSERT INTO invitacion (usuario_id, token_hash, vence_en) VALUES ($1,$2, now() + interval '7 days')`, uid, hash); err != nil {
		return "", err
	}
	return s.Cfg.URLPublica + "/login/invitacion?token=" + tok, nil
}

func (s *Server) verPermisosInquilino(w http.ResponseWriter, r *http.Request) {
	uid, err := idRuta(r, "uid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	if e.Rol != "administrador" && e.Rol != "superadmin" && !(e.Rol == "propietario" && e.EsSuya(uid)) {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	var perms []byte
	if err := s.DB.QueryRow(r.Context(), `SELECT permisos_inquilino FROM unidad WHERE id=$1 AND edificio_id=$2`, uid, e.ID).Scan(&perms); err != nil {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(perms)
}

func (s *Server) editarPermisosInquilino(w http.ResponseWriter, r *http.Request) {
	uid, err := idRuta(r, "uid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	if !(e.Rol == "propietario" && e.EsSuya(uid)) && e.Rol != "administrador" && e.Rol != "superadmin" {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	var in struct {
		Reservar   bool `json:"reservar"`
		Reportar   bool `json:"reportar"`
		VerRecibos bool `json:"ver_recibos"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	b, _ := json.Marshal(in)
	if _, err := s.DB.Exec(r.Context(), `UPDATE unidad SET permisos_inquilino=$3 WHERE id=$1 AND edificio_id=$2`, uid, e.ID, b); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, in)
}

// ---------- importación Excel ----------

var columnasPadron = []string{"codigo", "tipo", "piso", "participacion_pct", "propietario_nombre", "propietario_dni_ruc", "propietario_correo",
	"propietario_celular", "alquilado", "inquilino_nombre", "inquilino_dni", "inquilino_celular", "medidor_agua_serie", "lectura_inicial_agua"}

// plantillaExcel: GET /importaciones/plantilla.xlsx
func (s *Server) plantillaExcel(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()
	_ = f.SetSheetName("Sheet1", "Padron")
	for i, c := range columnasPadron {
		celda, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue("Padron", celda, c)
	}
	ejemplos := [][]any{
		{"101", "departamento", 1, 4.2, "Juan Pérez", "40000101", "juan@correo.pe", "900000101", "no", "", "", "", "AG-101", 1203.5},
		{"102", "departamento", 1, 4.0, "Rosa Díaz", "40000102", "", "900000102", "si", "Luis Soto", "41000102", "911000102", "AG-102", 988.25},
	}
	for fi, fila := range ejemplos {
		for ci, v := range fila {
			celda, _ := excelize.CoordinatesToCellName(ci+1, fi+2)
			_ = f.SetCellValue("Padron", celda, v)
		}
	}
	estilo, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"155E75"}, Pattern: 1}})
	_ = f.SetRowStyle("Padron", 1, 1, estilo)
	_ = f.SetColWidth("Padron", "A", "N", 18)
	dv := excelize.NewDataValidation(true)
	dv.Sqref = "B2:B1000"
	_ = dv.SetDropList([]string{"departamento", "estacionamiento", "deposito", "local"})
	_ = f.AddDataValidation("Padron", dv)
	dv2 := excelize.NewDataValidation(true)
	dv2.Sqref = "I2:I1000"
	_ = dv2.SetDropList([]string{"si", "no"})
	_ = f.AddDataValidation("Padron", dv2)
	_, _ = f.NewSheet("Deuda")
	for i, c := range []string{"codigo", "periodo", "monto"} {
		celda, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue("Deuda", celda, c)
	}
	_ = f.SetCellValue("Deuda", "A2", "102")
	_ = f.SetCellValue("Deuda", "B2", "2026-07")
	_ = f.SetCellValue("Deuda", "C2", 650.5)
	_ = f.SetRowStyle("Deuda", 1, 1, estilo)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		P.Fallo(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="plantilla-padron-edisys.xlsx"`)
	_, _ = w.Write(buf.Bytes())
}

// FilaPadron es una fila validada del Excel.
type FilaPadron struct {
	Fila              int     `json:"fila"`
	Codigo            string  `json:"codigo"`
	Tipo              string  `json:"tipo"`
	Piso              *int    `json:"piso"`
	ParticipacionDiez int64   `json:"participacion_diezmilesimas"`
	ParticipacionPct  string  `json:"participacion_pct"`
	PropNombre        string  `json:"propietario_nombre"`
	PropDNI           string  `json:"propietario_dni_ruc"`
	PropCorreo        string  `json:"propietario_correo"`
	PropCelular       string  `json:"propietario_celular"`
	Alquilado         bool    `json:"alquilado"`
	InqNombre         string  `json:"inquilino_nombre"`
	InqDNI            string  `json:"inquilino_dni"`
	InqCelular        string  `json:"inquilino_celular"`
	MedidorSerie      string  `json:"medidor_agua_serie"`
	LecturaInicial    *string `json:"lectura_inicial_agua"`
	Accion            string  `json:"accion"`
	Valida            bool    `json:"valida"`
}

// DeudaImport es una fila de la hoja Deuda.
type DeudaImport struct {
	Fila     int    `json:"fila"`
	Codigo   string `json:"codigo"`
	Periodo  string `json:"periodo"`
	MontoCts int64  `json:"monto_cts"`
}

// ProblemaImport es un error o advertencia de una fila.
type ProblemaImport struct {
	Hoja    string `json:"hoja,omitempty"`
	Fila    int    `json:"fila"`
	Campo   string `json:"campo,omitempty"`
	Mensaje string `json:"mensaje"`
}

// ResultadoImport es la vista previa de una importación.
type ResultadoImport struct {
	Filas        int              `json:"filas"`
	Validas      int              `json:"validas"`
	Errores      []ProblemaImport `json:"errores"`
	Advertencias []ProblemaImport `json:"advertencias"`
	SumaPct      string           `json:"suma_participacion_pct"`
	Bloqueante   bool             `json:"bloqueante"`
	Padron       []FilaPadron     `json:"-"`
	Deudas       []DeudaImport    `json:"-"`
}

var reDNI = regexp.MustCompile(`^\d{8}$`)
var reRUC = regexp.MustCompile(`^(10|20)\d{9}$`)
var reCel = regexp.MustCompile(`^9\d{8}$`)

func validarDNI(d string) string {
	d = strings.TrimSpace(d)
	switch {
	case len(d) == 11:
		if !reRUC.MatchString(d) {
			return "RUC debe tener 11 dígitos y empezar con 10 o 20"
		}
	case !reDNI.MatchString(d):
		return "DNI debe tener 8 dígitos"
	}
	return ""
}

func celularLimpio(c string) string {
	c = strings.NewReplacer(" ", "", "-", "", "+", "").Replace(strings.TrimSpace(c))
	if len(c) == 11 && strings.HasPrefix(c, "51") {
		c = c[2:]
	}
	return c
}

// pctADiez convierte "4,20" o "4.2" a diezmilésimas (42000).
func pctADiez(s string) (int64, error) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	s = strings.Replace(s, ",", ".", 1)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || f > 100 {
		return 0, fmt.Errorf("participación inválida")
	}
	return int64(math.Round(f * 10000)), nil
}

func diezATexto(v int64) string { return fmt.Sprintf("%d.%04d", v/10000, v%10000) }
func diezAES(v int64) string    { return fmt.Sprintf("%d,%04d", v/10000, v%10000) }

// ValidarPadron lee el .xlsx (en streaming) y valida cada fila con las reglas de 06.
func ValidarPadron(datos []byte, hoy time.Time) (*ResultadoImport, error) {
	f, err := excelize.OpenReader(bytes.NewReader(datos))
	if err != nil {
		return nil, P.Err(http.StatusUnprocessableEntity, "EXCEL_INVALIDO", "No pude abrir el archivo. Sube un .xlsx como la plantilla.")
	}
	defer f.Close()
	res := &ResultadoImport{Errores: []ProblemaImport{}, Advertencias: []ProblemaImport{}}
	hoja := "Padron"
	if idx, _ := f.GetSheetIndex(hoja); idx < 0 {
		hoja = f.GetSheetName(0)
	}
	filas, err := f.Rows(hoja)
	if err != nil {
		return nil, P.Err(http.StatusUnprocessableEntity, "EXCEL_INVALIDO", "No encuentro la hoja Padron.")
	}
	var cabecera map[string]int
	num := 0
	codigos := map[string]int{}
	dnis := map[string]int{}
	var suma int64
	for filas.Next() {
		num++
		cols, err := filas.Columns()
		if err != nil {
			return nil, err
		}
		if cabecera == nil {
			cabecera = map[string]int{}
			for i, c := range cols {
				cabecera[strings.ToLower(strings.TrimSpace(strings.Split(c, " ")[0]))] = i
			}
			for _, req := range []string{"codigo", "participacion_pct", "propietario_nombre", "propietario_dni_ruc"} {
				if _, ok := cabecera[req]; !ok {
					return nil, P.Err(http.StatusUnprocessableEntity, "EXCEL_SIN_COLUMNAS", "Falta la columna «"+req+"». Usa la plantilla.")
				}
			}
			continue
		}
		get := func(k string) string {
			if i, ok := cabecera[k]; ok && i < len(cols) {
				return strings.TrimSpace(cols[i])
			}
			return ""
		}
		vacia := true
		for _, c := range cols {
			if strings.TrimSpace(c) != "" {
				vacia = false
			}
		}
		if vacia {
			continue
		}
		fp := FilaPadron{Fila: num, Codigo: get("codigo"), Tipo: strings.ToLower(get("tipo")), PropNombre: get("propietario_nombre"),
			PropDNI: get("propietario_dni_ruc"), PropCorreo: get("propietario_correo"), PropCelular: celularLimpio(get("propietario_celular")),
			InqNombre: get("inquilino_nombre"), InqDNI: get("inquilino_dni"), InqCelular: celularLimpio(get("inquilino_celular")), MedidorSerie: get("medidor_agua_serie")}
		errs := 0
		mal := func(campo, msg string) {
			res.Errores = append(res.Errores, ProblemaImport{Fila: num, Campo: campo, Mensaje: msg})
			errs++
		}
		if fp.Codigo == "" {
			mal("codigo", "Falta el código de la unidad")
		} else if prev, ok := codigos[fp.Codigo]; ok {
			mal("codigo", fmt.Sprintf("Código repetido con la fila %d", prev))
		} else {
			codigos[fp.Codigo] = num
		}
		if fp.Tipo == "" {
			fp.Tipo = "departamento"
		}
		switch fp.Tipo {
		case "departamento", "estacionamiento", "deposito", "local":
		default:
			mal("tipo", "Tipo debe ser departamento, estacionamiento, deposito o local")
		}
		if p := get("piso"); p != "" {
			if v, err := strconv.Atoi(strings.Split(p, ".")[0]); err == nil {
				fp.Piso = &v
			} else {
				mal("piso", "Piso debe ser un número")
			}
		}
		if v, err := pctADiez(get("participacion_pct")); err != nil {
			mal("participacion_pct", "Participación debe ser un número entre 0 y 100")
		} else {
			fp.ParticipacionDiez = v
			fp.ParticipacionPct = diezATexto(v)
			suma += v
		}
		if fp.PropNombre == "" {
			mal("propietario_nombre", "Falta el nombre del propietario")
		}
		if msg := validarDNI(fp.PropDNI); msg != "" {
			mal("propietario_dni_ruc", msg)
		} else if prev, ok := dnis[fp.PropDNI]; ok {
			res.Advertencias = append(res.Advertencias, ProblemaImport{Fila: num, Campo: "propietario_dni_ruc", Mensaje: fmt.Sprintf("DNI repetido con la fila %d", prev)})
		} else {
			dnis[fp.PropDNI] = num
		}
		if fp.PropCorreo != "" {
			if _, err := mail.ParseAddress(fp.PropCorreo); err != nil {
				mal("propietario_correo", "Correo con formato inválido")
			}
		}
		if fp.PropCelular != "" && !reCel.MatchString(fp.PropCelular) {
			mal("propietario_celular", "Celular debe tener 9 dígitos y empezar con 9")
		}
		alq := strings.ToLower(get("alquilado"))
		fp.Alquilado = alq == "si" || alq == "sí" || alq == "s" || alq == "true" || alq == "1"
		if fp.Alquilado && fp.InqNombre == "" {
			mal("inquilino_nombre", "Unidad alquilada sin nombre de inquilino")
		}
		if fp.InqDNI != "" && !reDNI.MatchString(fp.InqDNI) {
			mal("inquilino_dni", "DNI debe tener 8 dígitos")
		}
		if fp.InqCelular != "" && !reCel.MatchString(fp.InqCelular) {
			mal("inquilino_celular", "Celular debe tener 9 dígitos y empezar con 9")
		}
		if l := get("lectura_inicial_agua"); l != "" {
			if v, err := P.Milesimas(l); err != nil || v < 0 {
				mal("lectura_inicial_agua", "Lectura inicial debe ser un número mayor o igual a 0")
			} else {
				t := P.TextoMilesimas(v)
				fp.LecturaInicial = &t
			}
		}
		fp.Valida = errs == 0
		if fp.Valida {
			res.Validas++
		}
		res.Filas++
		res.Padron = append(res.Padron, fp)
	}
	_ = filas.Close()
	if res.Filas == 0 {
		return nil, P.Err(http.StatusUnprocessableEntity, "EXCEL_VACIO", "El Excel no tiene filas de unidades.")
	}
	res.SumaPct = diezATexto(suma)
	if suma != 1000000 {
		falta := int64(1000000) - suma
		msg := fmt.Sprintf("Suman %s %%, faltan %s %%", diezAES(suma), diezAES(falta))
		if falta < 0 {
			msg = fmt.Sprintf("Suman %s %%, sobran %s %%", diezAES(suma), diezAES(-falta))
		}
		res.Errores = append(res.Errores, ProblemaImport{Fila: 0, Campo: "participacion_pct", Mensaje: msg})
		res.Bloqueante = true
	}
	// Hoja opcional Deuda.
	if idx, _ := f.GetSheetIndex("Deuda"); idx >= 0 {
		dr, err := f.GetRows("Deuda")
		if err == nil {
			for i, cols := range dr {
				if i == 0 || len(cols) == 0 || strings.TrimSpace(strings.Join(cols, "")) == "" {
					continue
				}
				for len(cols) < 3 {
					cols = append(cols, "")
				}
				d := DeudaImport{Fila: i + 1, Codigo: strings.TrimSpace(cols[0]), Periodo: strings.TrimSpace(cols[1])}
				ok := true
				if _, existe := codigos[d.Codigo]; !existe {
					res.Errores = append(res.Errores, ProblemaImport{Hoja: "Deuda", Fila: i + 1, Campo: "codigo", Mensaje: "La unidad " + d.Codigo + " no está en el padrón"})
					ok = false
				}
				if !P.PeriodoValido(d.Periodo) || d.Periodo > hoy.Format("2006-01") {
					res.Errores = append(res.Errores, ProblemaImport{Hoja: "Deuda", Fila: i + 1, Campo: "periodo", Mensaje: "Periodo AAAA-MM válido y no futuro"})
					ok = false
				}
				m, err := strconv.ParseFloat(strings.Replace(strings.TrimSpace(cols[2]), ",", ".", 1), 64)
				if err != nil || m <= 0 {
					res.Errores = append(res.Errores, ProblemaImport{Hoja: "Deuda", Fila: i + 1, Campo: "monto", Mensaje: "El monto debe ser mayor que 0"})
					ok = false
				}
				d.MontoCts = int64(math.Round(m * 100))
				if ok {
					res.Deudas = append(res.Deudas, d)
				}
			}
		}
	}
	if len(res.Errores) > 0 {
		res.Bloqueante = true
	}
	return res, nil
}

// importarExcel: POST /importaciones (multipart «archivo») → vista previa con errores por fila.
func (s *Server) importarExcel(w http.ResponseWriter, r *http.Request) {
	if err := leerMultipart(r); err != nil {
		P.Fallo(w, r, err)
		return
	}
	var fh = firstFile(r, "archivo", "excel", "file")
	if fh == nil {
		P.Fallo(w, r, P.Validacion("Sube el Excel en el campo «archivo».").Campo("archivo", "Obligatorio."))
		return
	}
	fl, err := fh.Open()
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer fl.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(fl); err != nil {
		P.Fallo(w, r, err)
		return
	}
	res, err := ValidarPadron(buf.Bytes(), time.Now().In(P.Lima))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	existentes := map[string]bool{}
	filas, _ := db.Filas(ctx, s.DB, `SELECT codigo FROM unidad WHERE edificio_id=$1`, e.ID)
	for _, f := range filas {
		existentes[f["codigo"].(string)] = true
	}
	for i := range res.Padron {
		res.Padron[i].Accion = "crear"
		if existentes[res.Padron[i].Codigo] {
			res.Padron[i].Accion = "actualizar"
		}
	}
	resumen, _ := json.Marshal(res)
	padron, _ := json.Marshal(res.Padron)
	deudas, _ := json.Marshal(res.Deudas)
	var iid int64
	if err := s.DB.QueryRow(ctx, `INSERT INTO importacion (edificio_id, usuario_id, archivo_nombre, resumen, filas, deudas) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		e.ID, ses(r).UsuarioID, fh.Filename, resumen, padron, deudas).Scan(&iid); err != nil {
		P.Fallo(w, r, err)
		return
	}
	previa := res.Padron
	if len(previa) > 50 {
		previa = previa[:50]
	}
	P.JSON(w, http.StatusOK, map[string]any{"importacion_id": iid, "filas": res.Filas, "validas": res.Validas, "errores": res.Errores,
		"advertencias": res.Advertencias, "suma_participacion_pct": res.SumaPct, "bloqueante": res.Bloqueante,
		"deudas": len(res.Deudas), "vista_previa": previa})
}

// confirmarImportacion aplica todo en una sola transacción: entra el padrón completo o no entra nada.
func (s *Server) confirmarImportacion(w http.ResponseWriter, r *http.Request) {
	iid, err := idRuta(r, "iid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	var estado string
	var resumenB, filasB, deudasB []byte
	if err := s.DB.QueryRow(ctx, `SELECT estado, resumen, filas, deudas FROM importacion WHERE id=$1 AND edificio_id=$2`, iid, e.ID).Scan(&estado, &resumenB, &filasB, &deudasB); err != nil {
		P.Fallo(w, r, P.NoEncontrado("la importación"))
		return
	}
	if estado != "validada" {
		P.Fallo(w, r, P.Conflicto("IMPORTACION_CERRADA", "Esa importación ya se "+estado+"."))
		return
	}
	var res ResultadoImport
	_ = json.Unmarshal(resumenB, &res)
	if res.Bloqueante || len(res.Errores) > 0 {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "IMPORTACION_CON_ERRORES", "Corrige los errores del Excel antes de confirmar.").Con("errores", res.Errores))
		return
	}
	var padron []FilaPadron
	var deudas []DeudaImport
	_ = json.Unmarshal(filasB, &padron)
	_ = json.Unmarshal(deudasB, &deudas)
	out, err := s.aplicarPadron(ctx, e.ID, padron, deudas)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	_, _ = s.DB.Exec(ctx, `UPDATE importacion SET estado='confirmada', confirmada_en=now() WHERE id=$1`, iid)
	s.auditarCambio(ctx, s.DB, r, "unidades", "importar", "importacion", iid, nil, out)
	P.JSON(w, http.StatusOK, out)
}

func (s *Server) aplicarPadron(ctx context.Context, eid int64, padron []FilaPadron, deudas []DeudaImport) (map[string]any, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	creadas, actualizadas, personas, cargadas := 0, 0, 0, 0
	invitaciones := []map[string]any{}
	hoy := time.Now().In(P.Lima).Format("2006-01-02")
	ids := map[string]int64{}
	for _, f := range padron {
		var uid int64
		var nueva bool
		if err := tx.QueryRow(ctx, `INSERT INTO unidad (edificio_id, codigo, tipo, piso, participacion_pct, alquilado) VALUES ($1,$2,$3,$4,$5::numeric/10000,$6)
			ON CONFLICT (edificio_id, codigo) DO UPDATE SET tipo=EXCLUDED.tipo, piso=EXCLUDED.piso, participacion_pct=EXCLUDED.participacion_pct,
			  alquilado=EXCLUDED.alquilado, activo=true
			RETURNING id, (xmax = 0)`, eid, f.Codigo, f.Tipo, f.Piso, f.ParticipacionDiez, f.Alquilado).Scan(&uid, &nueva); err != nil {
			return nil, err
		}
		ids[f.Codigo] = uid
		if nueva {
			creadas++
		} else {
			actualizadas++
		}
		asignar := func(rol, nombre, dni, correo, celular string) error {
			if nombre == "" {
				_, err := tx.Exec(ctx, `UPDATE unidad_persona SET hasta=$3 WHERE unidad_id=$1 AND rol=$2 AND hasta IS NULL`, uid, rol, hoy)
				return err
			}
			var pid int64
			var dniAct string
			err := tx.QueryRow(ctx, `SELECT pe.id, pe.dni_ruc FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id
				WHERE up.unidad_id=$1 AND up.rol=$2 AND up.hasta IS NULL`, uid, rol).Scan(&pid, &dniAct)
			if err == nil && (dniAct == dni || dni == "") {
				_, err = tx.Exec(ctx, `UPDATE persona SET nombre=$2, correo=COALESCE(NULLIF($3,''),correo), celular=COALESCE(NULLIF($4,''),celular) WHERE id=$1`, pid, nombre, correo, celular)
				return err
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE unidad_persona SET hasta=$3 WHERE unidad_id=$1 AND rol=$2 AND hasta IS NULL`, uid, rol, hoy); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO persona (edificio_id, nombre, dni_ruc, correo, celular) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
				eid, nombre, dni, correo, celular).Scan(&pid); err != nil {
				return err
			}
			personas++
			if _, err := tx.Exec(ctx, `INSERT INTO unidad_persona (unidad_id, persona_id, rol, desde) VALUES ($1,$2,$3,$4)`, uid, pid, rol, hoy); err != nil {
				return err
			}
			enlace, err := s.usuarioParaPersona(ctx, tx, eid, pid, rol)
			if err != nil {
				return err
			}
			if enlace != "" {
				invitaciones = append(invitaciones, map[string]any{"unidad": f.Codigo, "correo": correo, "enlace": enlace})
			}
			return nil
		}
		if err := asignar("propietario", f.PropNombre, f.PropDNI, f.PropCorreo, f.PropCelular); err != nil {
			return nil, err
		}
		inq := ""
		if f.Alquilado {
			inq = f.InqNombre
		}
		if err := asignar("inquilino", inq, f.InqDNI, "", f.InqCelular); err != nil {
			return nil, err
		}
		if f.MedidorSerie != "" || f.LecturaInicial != nil {
			ini := "0"
			if f.LecturaInicial != nil {
				ini = *f.LecturaInicial
			}
			tag, err := tx.Exec(ctx, `UPDATE medidor SET serie=COALESCE(NULLIF($2,''),serie),
				lectura_inicial = CASE WHEN EXISTS (SELECT 1 FROM lectura l WHERE l.medidor_id=medidor.id) THEN lectura_inicial ELSE $3::numeric END
				WHERE unidad_id=$1 AND tipo='agua'`, uid, f.MedidorSerie, ini)
			if err != nil {
				return nil, err
			}
			if tag.RowsAffected() == 0 {
				if _, err := tx.Exec(ctx, `INSERT INTO medidor (edificio_id, unidad_id, tipo, serie, orden_ronda, lectura_inicial) VALUES ($1,$2,'agua',$3,
					(SELECT COALESCE(max(orden_ronda),0)+1 FROM medidor WHERE edificio_id=$1),$4)`, eid, uid, f.MedidorSerie, ini); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, d := range deudas {
		if _, err := tx.Exec(ctx, `INSERT INTO deuda_inicial (unidad_id, periodo, monto_cts) VALUES ($1,$2,$3) ON CONFLICT (unidad_id, periodo) DO UPDATE SET monto_cts=EXCLUDED.monto_cts`,
			ids[d.Codigo], d.Periodo, d.MontoCts); err != nil {
			return nil, err
		}
		// La deuda entra en la cuenta corriente de la unidad como cargo vencido (0008).
		if _, err := tx.Exec(ctx, `SELECT cargar_deuda_inicial($1,$2,$3)`, ids[d.Codigo], d.Periodo, d.MontoCts); err != nil {
			return nil, err
		}
		cargadas++
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"unidades_creadas": creadas, "unidades_actualizadas": actualizadas, "personas_creadas": personas, "deudas_cargadas": cargadas, "invitaciones": invitaciones}, nil
}

func firstFile(r *http.Request, campos ...string) *multipartFileHeader {
	if r.MultipartForm == nil {
		return nil
	}
	for _, c := range campos {
		if fs := r.MultipartForm.File[c]; len(fs) > 0 {
			return fs[0]
		}
	}
	return nil
}

var _ = chi.URLParam
