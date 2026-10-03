package app

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Personal del edificio (bloques F1, F2 y F3). Este archivo: rutas comunes y F1 · colaboradores
// con sus documentos (CV, ficha, PLAME) en el cubo privado, abiertos con enlace firmado.

// rutasPersonal registra F1–F3. Se llama desde rutasEdificio y rutasModulosNuevos, como el resto.
func (s *Server) rutasPersonal(r chi.Router) {
	q := s.requiere

	// F1 · colaboradores y documentos
	r.With(q("personal.ver")).Get("/colaboradores", s.listarColaboradores)
	r.With(q("personal.administrar")).Post("/colaboradores", s.crearColaborador)
	r.With(q("personal.administrar")).Put("/colaboradores/{id}", s.editarColaborador)
	r.With(q("personal.administrar")).Post("/colaboradores/{id}/foto", s.fotoColaborador)
	r.With(q("personal.ver")).Get("/colaboradores/{id}/documentos", s.listarDocumentosColaborador)
	r.With(q("personal.administrar")).Post("/colaboradores/{id}/documentos", s.crearDocumentoColaborador)
	r.With(q("personal.administrar")).Delete("/colaboradores/{id}/documentos/{did}", s.borrarDocumentoColaborador)
	r.With(q("personal.administrar")).Get("/personal/cuentas", s.cuentasPersonal)

	// F2 · turnos, asistencia con foto, checklist y puntualidad
	r.With(q("personal.ver")).Get("/turnos", s.listarTurnos)
	r.With(q("personal.administrar")).Post("/turnos", s.crearTurno)
	r.With(q("personal.administrar")).Put("/turnos/{id}", s.editarTurno)
	r.With(q("personal.ver")).Get("/checklist", s.listarChecklist)
	r.With(q("personal.administrar")).Post("/checklist", s.crearChecklistItem)
	r.With(q("personal.administrar")).Delete("/checklist/{id}", s.desactivarChecklistItem)
	r.With(q("asistencia.marcar")).Get("/asistencia/hoy", s.miAsistenciaHoy)
	r.With(q("asistencia.marcar")).Post("/asistencia/marcar", s.marcarAsistencia)
	r.With(q("asistencia.marcar")).Post("/asistencia/{id}/checklist", s.marcarChecklist)
	r.With(q("personal.administrar")).Post("/asistencia/manual", s.asistenciaManual)
	r.With(q("asistencia.ver")).Get("/asistencia", s.listarAsistencia)
	r.With(q("asistencia.ver")).Get("/asistencia/puntualidad", s.puntualidad)

	// F3 · almacén sobre producto (0017_comercio)
	r.With(q("almacen.ver")).Get("/almacen/articulos", s.listarArticulosAlmacen)
	r.With(q("almacen.administrar")).Post("/almacen/articulos", s.crearArticuloAlmacen)
	r.With(q("almacen.administrar")).Put("/almacen/articulos/{pid}", s.configurarArticuloAlmacen)
	r.With(q("almacen.ver")).Get("/almacen/alertas", s.alertasAlmacen)
	r.With(q("almacen.ver")).Get("/almacen/movimientos", s.listarMovimientosAlmacen)
	r.With(q("almacen.registrar")).Post("/almacen/movimientos", s.registrarMovimientoAlmacen)
}

var tiposDocColaborador = map[string]bool{"1": true, "4": true, "7": true}
var tiposDocumentoColaborador = map[string]bool{"cv": true, "ficha": true, "plame": true, "contrato": true, "otro": true}

// listarColaboradores: GET /colaboradores?todos=1
// Quien no administra (junta, propietario) ve el documento enmascarado y sin teléfono ni correo.
func (s *Server) listarColaboradores(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	admin := e.Puede("personal.administrar")
	todos := admin && r.URL.Query().Get("todos") == "1"
	filas, err := db.Filas(r.Context(), s.DB, `SELECT c.id, c.nombre, c.tipo_doc, c.num_doc, c.cargo, c.telefono, c.correo,
			c.foto_id, c.usuario_id, u.nombre AS usuario, u.correo AS usuario_correo, c.turno_id, t.nombre AS turno,
			to_char(c.fecha_ingreso,'YYYY-MM-DD') AS fecha_ingreso, c.activo,
			(SELECT count(*) FROM colaborador_documento d WHERE d.colaborador_id=c.id AND ($3 OR d.visible_propietarios)) AS documentos
		FROM colaborador c
		LEFT JOIN usuario u ON u.id=c.usuario_id
		LEFT JOIN turno t ON t.id=c.turno_id
		WHERE c.edificio_id=$1 AND ($2 OR c.activo)
		ORDER BY c.activo DESC, lower(c.nombre)`, e.ID, todos, admin)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, f := range filas {
		if v, ok := f["foto_id"].(int64); ok {
			f["foto_url"] = s.Firma.URL(v)
		}
		if !admin {
			doc, _ := f["num_doc"].(string)
			f["num_doc"] = P.EnmascararDNI(doc)
			f["telefono"], f["correo"], f["usuario_correo"] = "", "", ""
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

type colaboradorIn struct {
	Nombre       *string `json:"nombre"`
	TipoDoc      *string `json:"tipo_doc"`
	NumDoc       *string `json:"num_doc"`
	Cargo        *string `json:"cargo"`
	Telefono     *string `json:"telefono"`
	Correo       *string `json:"correo"`
	UsuarioID    *int64  `json:"usuario_id"`
	TurnoID      *int64  `json:"turno_id"`
	FechaIngreso *string `json:"fecha_ingreso"`
	Activo       *bool   `json:"activo"`
}

// validar revisa forma y pertenencia (la cuenta debe ser del edificio; el turno, también).
func (s *Server) validarColaborador(r *http.Request, in *colaboradorIn, nuevo bool) error {
	e := edf(r)
	ev := P.Validacion("Revisa los datos del colaborador.")
	if in.Nombre != nil {
		v := strings.TrimSpace(*in.Nombre)
		in.Nombre = &v
	}
	if (nuevo && in.Nombre == nil) || (in.Nombre != nil && *in.Nombre == "") {
		ev.Campo("nombre", "Escribe el nombre.")
	}
	if in.TipoDoc != nil && !tiposDocColaborador[*in.TipoDoc] {
		ev.Campo("tipo_doc", "Usa DNI (1), carné de extranjería (4) o pasaporte (7).")
	}
	if in.NumDoc != nil {
		v := strings.ToUpper(strings.TrimSpace(*in.NumDoc))
		in.NumDoc = &v
		tipo := "1"
		if in.TipoDoc != nil {
			tipo = *in.TipoDoc
		}
		if tipo == "1" && v != "" && (len(v) != 8 || strings.Trim(v, "0123456789") != "") {
			ev.Campo("num_doc", "El DNI tiene 8 dígitos.")
		}
	}
	if in.FechaIngreso != nil && *in.FechaIngreso != "" {
		fechaOpc(*in.FechaIngreso, ev, "fecha_ingreso")
	}
	if in.UsuarioID != nil && *in.UsuarioID > 0 {
		var ok bool
		_ = s.DB.QueryRow(r.Context(), `SELECT true FROM usuario_edificio_rol WHERE usuario_id=$1 AND edificio_id=$2`, *in.UsuarioID, e.ID).Scan(&ok)
		if !ok {
			ev.Campo("usuario_id", "Esa cuenta no pertenece al edificio.")
		}
	}
	if in.TurnoID != nil && *in.TurnoID > 0 {
		var ok bool
		_ = s.DB.QueryRow(r.Context(), `SELECT true FROM turno WHERE id=$1 AND edificio_id=$2`, *in.TurnoID, e.ID).Scan(&ok)
		if !ok {
			ev.Campo("turno_id", "Elige un turno del edificio.")
		}
	}
	if len(ev.Campos) > 0 {
		return ev
	}
	return nil
}

// errColaborador traduce las unicidades propias a mensajes claros.
func errColaborador(err error) error {
	if err != nil && strings.Contains(err.Error(), "colaborador_doc_uq") {
		return P.Conflicto("COLABORADOR_DUPLICADO", "Ya hay un colaborador con ese documento.")
	}
	if err != nil && strings.Contains(err.Error(), "colaborador_usuario_uq") {
		return P.Conflicto("CUENTA_EN_USO", "Esa cuenta ya marca asistencia por otro colaborador.")
	}
	return err
}

// idONulo: 0 o negativo en un *int64 quiere decir «quitar».
func idONulo(v *int64) *int64 {
	if v == nil || *v <= 0 {
		return nil
	}
	return v
}

// crearColaborador: POST /colaboradores (JSON, o multipart con «foto» y los mismos campos)
func (s *Server) crearColaborador(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	var in colaboradorIn
	var fotos []Subido
	if esMultipart(r) {
		if err := leerMultipart(r); err != nil {
			P.Fallo(w, r, err)
			return
		}
		str := func(k string) *string {
			v := campo(r, k)
			return &v
		}
		in = colaboradorIn{Nombre: str("nombre"), TipoDoc: str("tipo_doc"), NumDoc: str("num_doc"), Cargo: str("cargo"),
			Telefono: str("telefono"), Correo: str("correo"), FechaIngreso: str("fecha_ingreso")}
		if *in.TipoDoc == "" {
			in.TipoDoc = nil
		}
		if v, err := strconv.ParseInt(campo(r, "usuario_id"), 10, 64); err == nil {
			in.UsuarioID = &v
		}
		if v, err := strconv.ParseInt(campo(r, "turno_id"), 10, 64); err == nil {
			in.TurnoID = &v
		}
		var err error
		if fotos, err = archivosDeForm(r, "foto"); err != nil {
			P.Fallo(w, r, err)
			return
		}
	} else if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := s.validarColaborador(r, &in, true); err != nil {
		P.Fallo(w, r, err)
		return
	}
	val := func(p *string, def string) string {
		if p == nil {
			return def
		}
		return strings.TrimSpace(*p)
	}
	fecha := val(in.FechaIngreso, "")
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	se := ses(r)
	var fotoID *int64
	if len(fotos) > 0 {
		id, err := s.guardarArchivo(ctx, tx, e.ID, &se.UsuarioID, fotos[0])
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		fotoID = &id
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO colaborador (edificio_id, nombre, tipo_doc, num_doc, cargo, telefono, correo, foto_id, usuario_id, turno_id, fecha_ingreso, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,COALESCE(NULLIF($11,'')::date,(now() AT TIME ZONE 'America/Lima')::date),$12) RETURNING id`,
		e.ID, val(in.Nombre, ""), val(in.TipoDoc, "1"), val(in.NumDoc, ""), val(in.Cargo, ""), val(in.Telefono, ""), val(in.Correo, ""),
		fotoID, idONulo(in.UsuarioID), idONulo(in.TurnoID), fecha, se.UsuarioID).Scan(&id)
	if err != nil {
		P.Fallo(w, r, errColaborador(err))
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// editarColaborador: PUT /colaboradores/{id} — campos parciales; usuario_id/turno_id = 0 los quita.
func (s *Server) editarColaborador(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	var in colaboradorIn
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := s.validarColaborador(r, &in, false); err != nil {
		P.Fallo(w, r, err)
		return
	}
	// Distinguir «no tocar» (nil) de «quitar» (0) en las referencias.
	tocaUsuario, tocaTurno := in.UsuarioID != nil, in.TurnoID != nil
	ct, err := s.DB.Exec(r.Context(), `UPDATE colaborador SET
			nombre=COALESCE($3,nombre), tipo_doc=COALESCE($4,tipo_doc), num_doc=COALESCE($5,num_doc), cargo=COALESCE($6,cargo),
			telefono=COALESCE($7,telefono), correo=COALESCE($8,correo),
			usuario_id=CASE WHEN $9 THEN $10 ELSE usuario_id END,
			turno_id=CASE WHEN $11 THEN $12 ELSE turno_id END,
			fecha_ingreso=COALESCE(NULLIF($13,'')::date,fecha_ingreso), activo=COALESCE($14,activo)
		WHERE id=$1 AND edificio_id=$2`,
		id, e.ID, in.Nombre, in.TipoDoc, in.NumDoc, in.Cargo, in.Telefono, in.Correo,
		tocaUsuario, idONulo(in.UsuarioID), tocaTurno, idONulo(in.TurnoID), in.FechaIngreso, in.Activo)
	if err != nil {
		P.Fallo(w, r, errColaborador(err))
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("el colaborador"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

// fotoColaborador: POST /colaboradores/{id}/foto (multipart «foto»)
func (s *Server) fotoColaborador(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	id := idURL(r, "id")
	fotos, err := archivosDeForm(r, "foto")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if len(fotos) == 0 {
		P.Fallo(w, r, P.Validacion("Adjunta la foto.").Campo("foto", "Obligatoria."))
		return
	}
	if !s.colaboradorDelEdificio(r, id) {
		P.Fallo(w, r, P.NoEncontrado("el colaborador"))
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	archID, err := s.guardarArchivo(ctx, tx, e.ID, &ses(r).UsuarioID, fotos[0])
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE colaborador SET foto_id=$1 WHERE id=$2 AND edificio_id=$3`, archID, id, e.ID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "foto_url": s.Firma.URL(archID)})
}

func (s *Server) colaboradorDelEdificio(r *http.Request, id int64) bool {
	var ok bool
	_ = s.DB.QueryRow(r.Context(), `SELECT true FROM colaborador WHERE id=$1 AND edificio_id=$2`, id, edf(r).ID).Scan(&ok)
	return ok
}

// listarDocumentosColaborador: GET /colaboradores/{id}/documentos
// Cada documento sale con su enlace firmado (10 min); el propietario solo recibe los visibles.
func (s *Server) listarDocumentosColaborador(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	if !s.colaboradorDelEdificio(r, id) {
		P.Fallo(w, r, P.NoEncontrado("el colaborador"))
		return
	}
	admin := e.Puede("personal.administrar")
	filas, err := db.Filas(r.Context(), s.DB, `SELECT d.id, d.tipo, d.titulo, d.periodo, d.archivo_id, d.visible_propietarios,
			a.nombre AS archivo_nombre, to_char(d.creado_en AT TIME ZONE 'America/Lima','YYYY-MM-DD') AS fecha
		FROM colaborador_documento d JOIN archivo a ON a.id=d.archivo_id
		WHERE d.colaborador_id=$1 AND d.edificio_id=$2 AND ($3 OR d.visible_propietarios)
		ORDER BY d.creado_en DESC, d.id DESC`, id, e.ID, admin)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, f := range filas {
		if v, ok := f["archivo_id"].(int64); ok {
			f["archivo_url"] = s.Firma.URL(v)
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearDocumentoColaborador: POST /colaboradores/{id}/documentos (multipart: tipo, titulo, periodo, visible, archivo)
func (s *Server) crearDocumentoColaborador(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	id := idURL(r, "id")
	if err := leerMultipart(r); err != nil {
		P.Fallo(w, r, err)
		return
	}
	tipo := campo(r, "tipo")
	titulo := campo(r, "titulo")
	periodo := campo(r, "periodo")
	visible := campo(r, "visible") != "0" && campo(r, "visible") != "false"
	docs, err := archivosDeForm(r, "archivo")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa el documento.")
	if !tiposDocumentoColaborador[tipo] {
		ev.Campo("tipo", "Usa CV, ficha, PLAME, contrato u otro.")
	}
	if titulo == "" {
		ev.Campo("titulo", "Escribe el título.")
	}
	if tipo == "plame" && !P.PeriodoValido(periodo) {
		ev.Campo("periodo", "La PLAME va con su periodo (AAAA-MM).")
	} else if periodo != "" && !P.PeriodoValido(periodo) {
		ev.Campo("periodo", "Formato AAAA-MM.")
	}
	if len(docs) == 0 {
		ev.Campo("archivo", "Adjunta el archivo.")
	}
	if !s.colaboradorDelEdificio(r, id) {
		P.Fallo(w, r, P.NoEncontrado("el colaborador"))
		return
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
	se := ses(r)
	archID, err := s.guardarArchivo(ctx, tx, e.ID, &se.UsuarioID, docs[0])
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var did int64
	if err := tx.QueryRow(ctx, `INSERT INTO colaborador_documento (edificio_id, colaborador_id, tipo, titulo, periodo, archivo_id, visible_propietarios, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, e.ID, id, tipo, titulo, periodo, archID, visible, se.UsuarioID).Scan(&did); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": did, "archivo_id": archID, "archivo_url": s.Firma.URL(archID)})
}

// borrarDocumentoColaborador: DELETE /colaboradores/{id}/documentos/{did}
func (s *Server) borrarDocumentoColaborador(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id, did := idURL(r, "id"), idURL(r, "did")
	ct, err := s.DB.Exec(r.Context(), `DELETE FROM colaborador_documento WHERE id=$1 AND colaborador_id=$2 AND edificio_id=$3`, did, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("el documento"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": did})
}

// cuentasPersonal: GET /personal/cuentas — cuentas del personal de planta (operario, técnico) para enlazar
// a un colaborador, con el colaborador que ya la usa si lo hay.
func (s *Server) cuentasPersonal(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT u.id, u.nombre, u.correo, uer.rol, c.id AS colaborador_id
		FROM usuario_edificio_rol uer JOIN usuario u ON u.id=uer.usuario_id
		LEFT JOIN colaborador c ON c.usuario_id=u.id AND c.edificio_id=uer.edificio_id
		WHERE uer.edificio_id=$1 AND uer.rol IN ('operario','tecnico') AND u.activo
		ORDER BY u.nombre`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}
