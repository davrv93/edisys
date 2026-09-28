package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// portal: GET /edificios/{eid}/portal — la portada del propietario en una sola petición (10).
func (s *Server) portal(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	se := ses(r)
	ctx := r.Context()
	unidades, err := db.Filas(ctx, s.DB, `SELECT u.id, u.codigo, u.tipo, u.participacion_pct::float8 AS participacion_pct, u.alquilado, u.permisos_inquilino,
			deuda_vencida_cts(u.id) AS deuda_vencida_cts, es_moroso(u.id) AS moroso
		FROM unidad u WHERE u.id = ANY($1) ORDER BY u.codigo`, e.Unidades)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	resp := map[string]any{"saludo": "Hola, " + strings.Split(se.Nombre, " ")[0], "rol": e.Rol, "unidades": unidades, "recibo_actual": nil}
	verRecibos := e.Puede("recibos.ver")
	if verRecibos && len(e.Unidades) > 0 {
		rc, err := db.Fila(ctx, s.DB, `SELECT r.id, r.numero, p.periodo, r.estado, r.total_cts, r.pagado_cts, r.total_cts - r.pagado_cts AS saldo_cts,
				to_char(r.vence,'YYYY-MM-DD') AS vence, u.codigo AS unidad,
				(SELECT count(*) FROM pago pg WHERE pg.recibo_id=r.id AND pg.estado='pendiente_validacion') AS pagos_en_revision
			FROM recibo r JOIN periodo p ON p.id=r.periodo_id JOIN unidad u ON u.id=r.unidad_id
			WHERE r.unidad_id = ANY($1) AND r.estado NOT IN ('borrador','anulado') ORDER BY p.periodo DESC, r.id DESC LIMIT 1`, e.Unidades)
		if err == nil {
			lineas, _ := db.Filas(ctx, s.DB, `SELECT tipo, descripcion, monto_cts FROM recibo_linea WHERE recibo_id=$1 ORDER BY orden, id`, rc["id"])
			rc["lineas"] = lineas
			resp["recibo_actual"] = rc
		} else if !errors.Is(err, pgx.ErrNoRows) {
			P.Fallo(w, r, err)
			return
		}
		deuda, err := s.deudaPorUnidad(ctx, e.ID, e.Unidades)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		var total int64
		for _, d := range deuda {
			total += d["deuda_cts"].(int64)
		}
		resp["deuda"] = map[string]any{"total_cts": total, "al_dia": total == 0, "por_unidad": deuda}
	}
	// Transparencia: los 4 KPIs del edificio (misma función del balance).
	periodo := P.PeriodoActual()
	if e.Puede("balance.ver") {
		if a, err := s.ArbolBalance(ctx, s.DB, e.ID, periodo, vistaDe(e)); err == nil {
			if !a.HayDatos {
				if b, err := s.ArbolBalance(ctx, s.DB, e.ID, P.PeriodoAnterior(periodo), vistaDe(e)); err == nil && b.HayDatos {
					a, periodo = b, P.PeriodoAnterior(periodo)
				}
			}
			resp["kpis_edificio"] = a.KPIs
			resp["periodo_kpis"] = periodo
		}
	}
	trabajos, err := db.Filas(ctx, s.DB, `SELECT id, codigo, titulo, estado, criticidad, monto_presupuesto_cts, costo_real_cts,
			(SELECT count(*) FROM voto v WHERE v.incidencia_id=i.id AND v.voto='aprueba') AS votos_a_favor
		FROM incidencia i WHERE edificio_id=$1 AND estado IN ('presupuestado','aprobado','en_ejecucion','terminado','rechazado')
		  AND (estado <> 'terminado' OR terminado_en > now() - interval '45 days') ORDER BY numero DESC LIMIT 10`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	mias, err := db.Filas(ctx, s.DB, `SELECT id, codigo, titulo, estado, criticidad, creado_en, actualizado_en FROM incidencia
		WHERE edificio_id=$1 AND (reportado_por=$2 OR unidad_id = ANY($3)) ORDER BY numero DESC LIMIT 10`, e.ID, se.UsuarioID, e.Unidades)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	reservas, err := db.Filas(ctx, s.DB, `SELECT rv.id, rv.codigo, rv.estado, rv.inicio, rv.fin, rv.total_cts, rc.nombre AS recurso, ar.nombre AS area
		FROM reserva rv JOIN recurso rc ON rc.id=rv.recurso_id JOIN area ar ON ar.id=rc.area_id
		WHERE rv.unidad_id = ANY($1) AND rv.estado IN ('confirmada','pendiente_pago') AND rv.fin >= now() ORDER BY rv.inicio LIMIT 10`, e.Unidades)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var normas, yape string
	var reglamento *int64
	_ = s.DB.QueryRow(ctx, `SELECT normas_texto, yape_numero, COALESCE(reglamento_archivo_id, manual_archivo_id) FROM edificio WHERE id=$1`, e.ID).Scan(&normas, &yape, &reglamento)
	resp["trabajos_mes"] = trabajos
	resp["mis_incidencias"] = mias
	resp["proximas_reservas"] = reservas
	resp["normas_texto"] = normas
	resp["normas_url"] = s.url(reglamento)
	resp["yape_numero"] = yape
	resp["puede"] = map[string]bool{"ver_recibos": verRecibos, "reservar": e.Puede("reservas.crear"), "reportar": e.Puede("incidencias.reportar"),
		"ver_balance": e.Puede("balance.ver"), "pagar": e.Puede("pagos.informar")}
	P.JSON(w, http.StatusOK, resp)
}

// ---------- 11 · usuarios, roles, junta, auditoría ----------

func (s *Server) listarUsuarios(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	pagina, por := paginacion(r)
	ctx := r.Context()
	var total int64
	_ = s.DB.QueryRow(ctx, `SELECT count(*) FROM usuario_edificio_rol WHERE edificio_id=$1`, e.ID).Scan(&total)
	filas, err := db.Filas(ctx, s.DB, `SELECT us.id, us.nombre, us.correo, us.telefono, uer.rol, us.activo, us.ultimo_ingreso, (us.clave_hash IS NULL) AS invitacion_pendiente,
			COALESCE((SELECT string_agg(DISTINCT u.codigo, ', ') FROM persona pe JOIN unidad_persona up ON up.persona_id=pe.id AND up.hasta IS NULL
			          JOIN unidad u ON u.id=up.unidad_id WHERE pe.usuario_id=us.id AND u.edificio_id=$1), '') AS unidades,
			EXISTS (SELECT 1 FROM junta_miembro jm WHERE jm.usuario_id=us.id AND jm.edificio_id=$1 AND jm.activo) AS miembro_junta
		FROM usuario_edificio_rol uer JOIN usuario us ON us.id=uer.usuario_id WHERE uer.edificio_id=$1
		ORDER BY CASE uer.rol WHEN 'administrador' THEN 0 WHEN 'junta' THEN 1 ELSE 2 END, us.nombre LIMIT $2 OFFSET $3`, e.ID, por, (pagina-1)*por)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, paginado(filas, total, pagina))
}

// invitarUsuario: POST /usuarios/invitar {correo|unidad_id, rol, nombre?} → 201 {enlace_invitacion}.
func (s *Server) invitarUsuario(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Correo   string `json:"correo"`
		Nombre   string `json:"nombre"`
		UnidadID int64  `json:"unidad_id"`
		Rol      string `json:"rol"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	if _, ok := s.permBase[in.Rol]; !ok || in.Rol == "superadmin" {
		P.Fallo(w, r, P.Validacion("Rol inválido.").Campo("rol", "administrador, junta, propietario, inquilino, operario o tecnico."))
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	if in.UnidadID > 0 && in.Correo == "" {
		var pid int64
		var correo, nombre string
		err := tx.QueryRow(ctx, `SELECT pe.id, pe.correo, pe.nombre FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id JOIN unidad u ON u.id=up.unidad_id
			WHERE up.unidad_id=$1 AND u.edificio_id=$2 AND up.hasta IS NULL AND up.rol=$3`, in.UnidadID, e.ID, in.Rol).Scan(&pid, &correo, &nombre)
		if err != nil || correo == "" {
			P.Fallo(w, r, P.Validacion("La unidad no tiene "+in.Rol+" con correo. Escribe el correo.").Campo("correo", "Obligatorio."))
			return
		}
		in.Correo, in.Nombre = correo, nombre
	}
	if !strings.Contains(in.Correo, "@") {
		P.Fallo(w, r, P.Validacion("Escribe un correo válido.").Campo("correo", "Obligatorio."))
		return
	}
	if in.Nombre == "" {
		in.Nombre = strings.Split(in.Correo, "@")[0]
	}
	var uid int64
	err = tx.QueryRow(ctx, `SELECT id FROM usuario WHERE lower(correo)=lower($1)`, in.Correo).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.QueryRow(ctx, `INSERT INTO usuario (administradora_id, correo, nombre) SELECT administradora_id, $2, $3 FROM edificio WHERE id=$1 RETURNING id`,
			e.ID, in.Correo, in.Nombre).Scan(&uid); err != nil {
			P.Fallo(w, r, err)
			return
		}
	} else if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `INSERT INTO usuario_edificio_rol (usuario_id, edificio_id, rol) VALUES ($1,$2,$3) ON CONFLICT (usuario_id, edificio_id) DO UPDATE SET rol=EXCLUDED.rol`, uid, e.ID, in.Rol); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.UnidadID > 0 {
		_, _ = tx.Exec(ctx, `UPDATE persona SET usuario_id=$1 WHERE id IN (SELECT persona_id FROM unidad_persona WHERE unidad_id=$2 AND hasta IS NULL AND rol=$3) AND usuario_id IS NULL`, uid, in.UnidadID, in.Rol)
	}
	enlace, err := s.crearInvitacion(ctx, tx, uid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"usuario_id": uid, "correo": in.Correo, "rol": in.Rol, "enlace_invitacion": enlace})
}

// editarUsuario: PATCH /usuarios/{uid} {rol?, activo?} · 409 ULTIMO_ADMIN · nadie se sube a sí mismo.
func (s *Server) editarUsuario(w http.ResponseWriter, r *http.Request) {
	uid, err := strconv.ParseInt(chi.URLParam(r, "uid"), 10, 64)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("el usuario"))
		return
	}
	var in struct {
		Rol    *string `json:"rol"`
		Activo *bool   `json:"activo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	se := ses(r)
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var rolActual string
	var activo bool
	if err := tx.QueryRow(ctx, `SELECT uer.rol, us.activo FROM usuario_edificio_rol uer JOIN usuario us ON us.id=uer.usuario_id WHERE uer.usuario_id=$1 AND uer.edificio_id=$2 FOR UPDATE`,
		uid, e.ID).Scan(&rolActual, &activo); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el usuario"))
		return
	}
	if in.Rol != nil {
		if _, ok := s.permBase[*in.Rol]; !ok || *in.Rol == "superadmin" {
			P.Fallo(w, r, P.Validacion("Rol inválido.").Campo("rol", "Rol inválido."))
			return
		}
		if uid == se.UsuarioID && *in.Rol != rolActual {
			P.Fallo(w, r, P.Prohibido("NO_AUTOASIGNAR", "No puedes cambiar tu propio rol."))
			return
		}
	}
	quitaAdmin := rolActual == "administrador" && ((in.Rol != nil && *in.Rol != "administrador") || (in.Activo != nil && !*in.Activo))
	if quitaAdmin {
		var admins int
		_ = tx.QueryRow(ctx, `SELECT count(*) FROM usuario_edificio_rol uer JOIN usuario us ON us.id=uer.usuario_id WHERE uer.edificio_id=$1 AND uer.rol='administrador' AND us.activo`, e.ID).Scan(&admins)
		if admins <= 1 {
			P.Fallo(w, r, P.Conflicto("ULTIMO_ADMIN", "El edificio no puede quedarse sin administrador."))
			return
		}
	}
	if in.Rol != nil {
		if _, err := tx.Exec(ctx, `UPDATE usuario_edificio_rol SET rol=$3 WHERE usuario_id=$1 AND edificio_id=$2`, uid, e.ID, *in.Rol); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	if in.Activo != nil {
		if _, err := tx.Exec(ctx, `UPDATE usuario SET activo=$2 WHERE id=$1`, uid, *in.Activo); err != nil {
			P.Fallo(w, r, err)
			return
		}
		if !*in.Activo {
			_, _ = tx.Exec(ctx, `UPDATE sesion_refresh SET revocado_en=now() WHERE usuario_id=$1 AND revocado_en IS NULL`, uid)
		}
	}
	s.auditarCambio(ctx, tx, r, "roles", "editar_usuario", "usuario", uid, map[string]any{"rol": rolActual, "activo": activo}, in)
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	f, _ := db.Fila(ctx, s.DB, `SELECT us.id, us.nombre, us.correo, uer.rol, us.activo FROM usuario us JOIN usuario_edificio_rol uer ON uer.usuario_id=us.id AND uer.edificio_id=$2 WHERE us.id=$1`, uid, e.ID)
	P.JSON(w, http.StatusOK, f)
}

// editarPermisosRol: PUT /roles/{rol}/permisos {permisos: [...]} — solo los ajustables; sube ver_permisos.
func (s *Server) editarPermisosRol(w http.ResponseWriter, r *http.Request) {
	rol := chi.URLParam(r, "rol")
	if _, ok := s.permBase[rol]; !ok || rol == "superadmin" || rol == "administrador" {
		P.Fallo(w, r, P.Validacion("Ese rol no se puede ajustar."))
		return
	}
	var in struct {
		Permisos []string `json:"permisos"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	ajustables, err := db.Filas(ctx, s.DB, `SELECT codigo FROM permiso WHERE ajustable`)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	quiero := map[string]bool{}
	for _, p := range in.Permisos {
		quiero[p] = true
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	for _, a := range ajustables {
		p := a["codigo"].(string)
		if _, err := tx.Exec(ctx, `INSERT INTO rol_permiso_edificio (edificio_id, rol, permiso, habilitado) VALUES ($1,$2,$3,$4)
			ON CONFLICT (edificio_id, rol, permiso) DO UPDATE SET habilitado=EXCLUDED.habilitado`, e.ID, rol, p, quiero[p]); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	_, _ = tx.Exec(ctx, `UPDATE edificio SET ver_permisos = ver_permisos + 1 WHERE id=$1`, e.ID)
	s.auditarCambio(ctx, tx, r, "roles", "permisos", "rol", rol, nil, in)
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"rol": rol, "permisos_ajustables": in.Permisos})
}

func (s *Server) verJunta(w http.ResponseWriter, r *http.Request) {
	filas, err := db.Filas(r.Context(), s.DB, `SELECT jm.usuario_id, us.nombre, us.correo, jm.cargo, jm.presidente, jm.activo
		FROM junta_miembro jm JOIN usuario us ON us.id=jm.usuario_id WHERE jm.edificio_id=$1 ORDER BY jm.presidente DESC, us.nombre`, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"miembros": filas})
}

// editarJunta: PUT /junta {miembros: [{usuario_id, cargo, presidente}]} — reemplaza la lista.
func (s *Server) editarJunta(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Miembros []struct {
			UsuarioID  int64  `json:"usuario_id"`
			Cargo      string `json:"cargo"`
			Presidente bool   `json:"presidente"`
		} `json:"miembros"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	pres := 0
	for _, m := range in.Miembros {
		if m.Presidente {
			pres++
		}
	}
	if pres > 1 {
		P.Fallo(w, r, P.Validacion("Solo puede haber un presidente."))
		return
	}
	e := edf(r)
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE junta_miembro SET activo=false, presidente=false WHERE edificio_id=$1`, e.ID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, m := range in.Miembros {
		cargo := m.Cargo
		if cargo == "" {
			cargo = "miembro"
		}
		tag, err := tx.Exec(ctx, `INSERT INTO junta_miembro (edificio_id, usuario_id, cargo, presidente, activo)
			SELECT $1, $2, $3, $4, true WHERE EXISTS (SELECT 1 FROM usuario_edificio_rol WHERE edificio_id=$1 AND usuario_id=$2)
			ON CONFLICT (edificio_id, usuario_id) DO UPDATE SET cargo=EXCLUDED.cargo, presidente=EXCLUDED.presidente, activo=true`, e.ID, m.UsuarioID, cargo, m.Presidente)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		if tag.RowsAffected() == 0 {
			P.Fallo(w, r, P.Validacion("El usuario "+strconv.FormatInt(m.UsuarioID, 10)+" no pertenece al edificio."))
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.verJunta(w, r)
}

func (s *Server) listarAuditoria(w http.ResponseWriter, r *http.Request) {
	pagina, por := paginacion(r)
	modulo := r.URL.Query().Get("modulo")
	e := edf(r)
	ctx := r.Context()
	var total int64
	_ = s.DB.QueryRow(ctx, `SELECT count(*) FROM auditoria WHERE edificio_id=$1 AND ($2='' OR modulo=$2)`, e.ID, modulo).Scan(&total)
	filas, err := db.Filas(ctx, s.DB, `SELECT a.id, a.modulo, a.accion, a.entidad, a.entidad_id, a.antes, a.despues, a.ip, a.creado_en, us.nombre AS usuario
		FROM auditoria a LEFT JOIN usuario us ON us.id=a.usuario_id WHERE a.edificio_id=$1 AND ($2='' OR a.modulo=$2)
		ORDER BY a.id DESC LIMIT $3 OFFSET $4`, e.ID, modulo, por, (pagina-1)*por)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, paginado(filas, total, pagina))
}
