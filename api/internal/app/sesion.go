package app

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/auth"
	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

var reCodigoDNI = regexp.MustCompile(`^\s*([A-Za-z0-9]+)\s*[-+ /]\s*(\d{8}|\d{11})\s*$`)

// login: POST /auth/login {correo | usuario, clave}. El usuario puede ser un correo o
// «CODIGO-DNI» (p. ej. «201-40000201»). Pone las cookies y devuelve también los tokens,
// para que el login de Qwik pueda reescribirlas del lado del servidor.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Correo  string `json:"correo"`
		Usuario string `json:"usuario"`
		Clave   string `json:"clave"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ident := strings.TrimSpace(in.Correo)
	if ident == "" {
		ident = strings.TrimSpace(in.Usuario)
	}
	if ident == "" || in.Clave == "" {
		e := P.Validacion("Escribe tu usuario y tu clave.")
		if ident == "" {
			e.Campo("correo", "Escribe tu correo o el código de tu unidad con tu DNI.")
		}
		if in.Clave == "" {
			e.Campo("clave", "Escribe tu clave.")
		}
		P.Fallo(w, r, e)
		return
	}
	claves := []string{"u:" + strings.ToLower(ident), "ip:" + ipDe(r)}
	if bloq, min := s.limLogin.Bloqueado(claves...); bloq {
		w.Header().Set("Retry-After", strconv.Itoa(min*60))
		P.Fallo(w, r, P.Err(http.StatusTooManyRequests, "DEMASIADOS_INTENTOS",
			"Demasiados intentos. Espera "+strconv.Itoa(min)+" minutos e intenta otra vez.").Con("minutos", min))
		return
	}
	ctx := r.Context()
	var uid int64
	var hash *string
	var activo bool
	var err error
	if strings.Contains(ident, "@") {
		err = s.DB.QueryRow(ctx, `SELECT id, clave_hash, activo FROM usuario WHERE lower(correo)=lower($1)`, ident).Scan(&uid, &hash, &activo)
	} else if m := reCodigoDNI.FindStringSubmatch(ident); m != nil {
		err = s.DB.QueryRow(ctx, `SELECT us.id, us.clave_hash, us.activo FROM persona p
			JOIN unidad_persona up ON up.persona_id = p.id AND up.hasta IS NULL
			JOIN unidad u ON u.id = up.unidad_id
			JOIN usuario us ON us.id = p.usuario_id
			WHERE upper(u.codigo) = upper($1) AND p.dni_ruc = $2 LIMIT 1`, m[1], m[2]).Scan(&uid, &hash, &activo)
	} else {
		err = pgx.ErrNoRows
	}
	if err != nil || hash == nil || !auth.ClaveCorrecta(*hash, in.Clave) {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			P.Fallo(w, r, err)
			return
		}
		s.limLogin.Fallo(claves...)
		P.Fallo(w, r, P.Err(http.StatusUnauthorized, "CREDENCIALES", "Usuario o clave incorrectos."))
		return
	}
	if !activo {
		P.Fallo(w, r, P.Err(http.StatusLocked, "CUENTA_BLOQUEADA", "Tu cuenta está desactivada. Escribe a la administración."))
		return
	}
	s.limLogin.Limpiar(claves...)
	resp, err := s.emitirSesion(ctx, w, uid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, resp)
}

// emitirSesion crea JWT + refresh, escribe las cookies y arma la respuesta.
func (s *Server) emitirSesion(ctx context.Context, w http.ResponseWriter, uid int64) (map[string]any, error) {
	se, err := s.cargarSesion(ctx, uid)
	if err != nil {
		return nil, err
	}
	jwt, vence, err := auth.FirmarJWT(s.Cfg.JWTSecret, uid, se.AdmID, se.Edificios(), 1)
	if err != nil {
		return nil, err
	}
	rt, rtHash := auth.TokenOpaco()
	if _, err := s.DB.Exec(ctx, `INSERT INTO sesion_refresh (usuario_id, token_hash, vence_en) VALUES ($1,$2,$3)`, uid, rtHash, time.Now().Add(auth.DuracionRT)); err != nil {
		return nil, err
	}
	_, _ = s.DB.Exec(ctx, `UPDATE usuario SET ultimo_ingreso = now() WHERE id=$1`, uid)
	s.ponerCookies(w, jwt, rt)
	u, err := s.datosUsuario(ctx, se)
	if err != nil {
		return nil, err
	}
	return map[string]any{"access_jwt": jwt, "refresh": rt, "vence_en": vence.UTC(), "usuario": u}, nil
}

func (s *Server) ponerCookies(w http.ResponseWriter, jwt, rt string) {
	http.SetCookie(w, &http.Cookie{Name: auth.CookieAcceso, Value: jwt, Path: "/", HttpOnly: true, Secure: s.Cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode, MaxAge: int(auth.DuracionJWT.Seconds())})
	http.SetCookie(w, &http.Cookie{Name: auth.CookieRefresh, Value: rt, Path: "/api/v1/auth", HttpOnly: true, Secure: s.Cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode, MaxAge: int(auth.DuracionRT.Seconds())})
}

func (s *Server) borrarCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: auth.CookieAcceso, Value: "", Path: "/", HttpOnly: true, Secure: s.Cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: auth.CookieRefresh, Value: "", Path: "/api/v1/auth", HttpOnly: true, Secure: s.Cfg.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

// datosUsuario: id, nombre, correo, roles_por_edificio y destino según el rol.
func (s *Server) datosUsuario(ctx context.Context, se *Sesion) (map[string]any, error) {
	roles, err := db.Filas(ctx, s.DB, `SELECT e.id AS edificio_id, e.nombre AS edificio, x.rol
		FROM edificio e JOIN unnest($1::bigint[], $2::text[]) AS x(eid, rol) ON x.eid = e.id ORDER BY e.id`, ids(se), rolesDe(se))
	if err != nil {
		return nil, err
	}
	destino := "/app/"
	if len(roles) > 0 {
		destino = destinoPorRol(roles[0]["edificio_id"].(int64), roles[0]["rol"].(string))
	}
	return map[string]any{"id": se.UsuarioID, "nombre": se.Nombre, "correo": se.Correo, "telefono": se.Telefono,
		"roles_por_edificio": roles, "destino": destino}, nil
}

func ids(se *Sesion) []int64 { return se.Edificios() }
func rolesDe(se *Sesion) []string {
	out := []string{}
	for _, id := range se.Edificios() {
		out = append(out, se.Roles[id])
	}
	return out
}

// destinoPorRol: admin y junta → 03; propietario e inquilino → 10; operario → 08; técnico → 09.
func destinoPorRol(eid int64, rol string) string {
	base := "/app/e/" + strconv.FormatInt(eid, 10)
	switch rol {
	case "propietario", "inquilino":
		return base + "/portal"
	case "operario":
		return base + "/lecturas"
	case "tecnico":
		return base + "/mantenimiento"
	}
	return base + "/inicio"
}

// refresh: rota el refresh (cookie edisys_rt o {refresh}) y reescribe ambas cookies.
func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	token := ""
	if c, err := r.Cookie(auth.CookieRefresh); err == nil {
		token = c.Value
	}
	if token == "" {
		var in struct {
			Refresh string `json:"refresh"`
		}
		_ = P.Leer(r, &in)
		token = in.Refresh
	}
	if token == "" {
		P.Fallo(w, r, P.Err(http.StatusUnauthorized, "SIN_SESION", "Vuelve a iniciar sesión."))
		return
	}
	ctx := r.Context()
	var uid int64
	err := s.DB.QueryRow(ctx, `UPDATE sesion_refresh SET revocado_en = now()
		WHERE token_hash=$1 AND revocado_en IS NULL AND vence_en > now()
		  AND usuario_id IN (SELECT id FROM usuario WHERE activo) RETURNING usuario_id`, auth.HashToken(token)).Scan(&uid)
	if err != nil {
		s.borrarCookies(w)
		P.Fallo(w, r, P.Err(http.StatusUnauthorized, "SIN_SESION", "Tu sesión terminó. Vuelve a entrar."))
		return
	}
	resp, err := s.emitirSesion(ctx, w, uid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, resp)
}

// logout: invalida el refresh y borra las cookies. 204.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieRefresh); err == nil && c.Value != "" {
		_, _ = s.DB.Exec(r.Context(), `UPDATE sesion_refresh SET revocado_en = now() WHERE token_hash=$1`, auth.HashToken(c.Value))
	}
	s.borrarCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

// aceptarInvitacion: POST /auth/aceptar-invitacion {token, clave} → fija la clave y entra.
func (s *Server) aceptarInvitacion(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
		Clave string `json:"clave"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if len(in.Clave) < 8 {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "CLAVE_DEBIL", "La clave debe tener al menos 8 caracteres.").Campo("clave", "Mínimo 8 caracteres."))
		return
	}
	ctx := r.Context()
	var uid int64
	err := s.DB.QueryRow(ctx, `UPDATE invitacion SET usada_en = now() WHERE token_hash=$1 AND usada_en IS NULL AND vence_en > now() RETURNING usuario_id`,
		auth.HashToken(in.Token)).Scan(&uid)
	if err != nil {
		P.Fallo(w, r, P.Err(http.StatusGone, "ENLACE_VENCIDO", "El enlace venció o ya se usó. Pide otro a la administración."))
		return
	}
	hash, err := auth.HashClave(in.Clave)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := s.DB.Exec(ctx, `UPDATE usuario SET clave_hash=$2, activo=true WHERE id=$1`, uid, hash); err != nil {
		P.Fallo(w, r, err)
		return
	}
	resp, err := s.emitirSesion(ctx, w, uid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, resp)
}

// yo: usuario, roles por edificio, edificio actual, permisos efectivos, unidades y menú.
func (s *Server) yo(w http.ResponseWriter, r *http.Request) {
	se := ses(r)
	ctx := r.Context()
	u, err := s.datosUsuario(ctx, se)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	resp := map[string]any{"usuario": u, "roles_por_edificio": u["roles_por_edificio"], "edificio_actual": nil, "permisos": []string{}, "menu": []any{}, "unidades": []any{}}
	eid := se.Edificios()
	if v, err := strconv.ParseInt(r.URL.Query().Get("edificio_id"), 10, 64); err == nil {
		if _, ok := se.Roles[v]; ok {
			eid = []int64{v}
		}
	}
	if len(eid) > 0 {
		e, err := s.cargarEdificio(ctx, eid[0], se.Roles[eid[0]], se.UsuarioID)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		perms := []string{}
		for p := range e.Permisos {
			perms = append(perms, p)
		}
		sortStrings(perms)
		unidades, err := db.Filas(ctx, s.DB, `SELECT id, codigo FROM unidad WHERE id = ANY($1) ORDER BY codigo`, e.Unidades)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		resp["edificio_actual"] = map[string]any{"id": e.ID, "nombre": e.Nombre, "rol": e.Rol}
		resp["permisos"] = perms
		resp["unidades"] = unidades
		resp["menu"], resp["pestanas_movil"] = menuPara(e)
		resp["destino"] = destinoPorRol(e.ID, e.Rol)
	}
	P.JSON(w, http.StatusOK, resp)
}

type itemMenu struct {
	Clave   string `json:"clave"`
	Titulo  string `json:"titulo"`
	Ruta    string `json:"ruta"`
	Permiso string `json:"permiso"`
}

// menuPara genera el menú desde los permisos (no hay menú escrito a mano por rol).
func menuPara(e *Edificio) ([]itemMenu, []string) {
	base := "/app/e/" + strconv.FormatInt(e.ID, 10)
	todos := []itemMenu{
		{"inicio", "Inicio", base + "/inicio", "dashboard.ver"},
		{"portal", "Mi portal", base + "/portal", "portal.ver"},
		{"balance", "Balance", base + "/balance", "balance.ver"},
		{"recibos", "Recibos", base + "/recibos", "recibos.ver"},
		{"unidades", "Unidades", base + "/unidades", "unidades.ver"},
		{"reservas", "Reservas", base + "/reservas", "reservas.ver"},
		{"lecturas", "Lecturas", base + "/lecturas", "lecturas.ver"},
		{"mantenimiento", "Mantenimiento", base + "/mantenimiento", "incidencias.ver"},
		{"reportar", "Reportar", base + "/mantenimiento/reportar", "incidencias.reportar"},
		{"whatsapp", "WhatsApp", base + "/whatsapp", "whatsapp.ver"},
		{"analitica", "Analítica", base + "/analitica", "analitica.ver"},
		{"usuarios", "Usuarios y roles", base + "/ajustes/usuarios", "usuarios.ver"},
	}
	menu := []itemMenu{}
	for _, m := range todos {
		if e.Puede(m.Permiso) {
			menu = append(menu, m)
		}
	}
	var pest []string
	switch e.Rol {
	case "administrador", "superadmin":
		pest = []string{"inicio", "balance", "recibos", "mantenimiento", "mas"}
	case "junta":
		pest = []string{"inicio", "balance", "mantenimiento", "mas"}
	case "propietario", "inquilino":
		pest = []string{"portal", "recibos", "reservas", "reportar"}
		if !e.Puede("recibos.ver") {
			pest = []string{"portal", "reservas", "reportar"}
		}
	case "operario":
		pest = []string{"lecturas", "reportar"}
	case "tecnico":
		pest = []string{"mantenimiento"}
	}
	return menu, pest
}

func (s *Server) misEdificios(w http.ResponseWriter, r *http.Request) {
	se := ses(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT e.id, e.nombre, e.direccion, e.distrito, x.rol,
		(SELECT count(*) FROM unidad u WHERE u.edificio_id = e.id AND u.activo) AS unidades
		FROM edificio e JOIN unnest($1::bigint[], $2::text[]) AS x(eid, rol) ON x.eid = e.id ORDER BY e.id`, ids(se), rolesDe(se))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas})
}

// listarRoles: roles con sus permisos y cuáles son ajustables.
func (s *Server) listarRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	roles, err := db.Filas(ctx, s.DB, `SELECT r.codigo, r.nombre, r.descripcion,
		COALESCE(array_agg(rp.permiso ORDER BY rp.permiso) FILTER (WHERE rp.permiso IS NOT NULL), '{}') AS permisos
		FROM rol r LEFT JOIN rol_permiso rp ON rp.rol = r.codigo GROUP BY r.codigo ORDER BY r.orden`)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	perms, err := db.Filas(ctx, s.DB, `SELECT codigo, modulo, descripcion, ajustable FROM permiso ORDER BY modulo, codigo`)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"roles": roles, "permisos": perms})
}

// contacto: POST /publico/contacto (landing). 202 siempre que pase la validación;
// si el campo trampa sitio_web viene lleno, 202 y se descarta. 3 envíos por IP y hora.
func (s *Server) contacto(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nombre         string `json:"nombre"`
		Correo         string `json:"correo"`
		Telefono       string `json:"telefono"`
		Celular        string `json:"celular"`
		Empresa        string `json:"empresa"`
		EdificiosAprox *int   `json:"edificios_aprox"`
		Mensaje        string `json:"mensaje"`
		SitioWeb       string `json:"sitio_web"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if strings.TrimSpace(in.SitioWeb) != "" {
		P.JSON(w, http.StatusAccepted, map[string]any{"ok": true})
		return
	}
	ip := ipDe(r)
	if bloq, min := s.limLead.Bloqueado("ip:" + ip); bloq {
		w.Header().Set("Retry-After", strconv.Itoa(min*60))
		P.Fallo(w, r, P.Err(http.StatusTooManyRequests, "DEMASIADOS_ENVIOS", "Ya recibimos tus mensajes. Intenta otra vez en "+strconv.Itoa(min)+" minutos.").Con("minutos", min))
		return
	}
	e := P.Validacion("Revisa los campos marcados.")
	if len(strings.TrimSpace(in.Nombre)) < 2 {
		e.Campo("nombre", "Escribe tu nombre.")
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(in.Correo)); err != nil {
		e.Campo("correo", "Escribe un correo válido.")
	}
	if len(strings.TrimSpace(in.Mensaje)) < 5 {
		e.Campo("mensaje", "Cuéntanos un poco más (mínimo 5 caracteres).")
	}
	tel := in.Telefono
	if tel == "" {
		tel = in.Celular
	}
	if len(e.Campos) > 0 {
		P.Fallo(w, r, e)
		return
	}
	s.limLead.Fallo("ip:" + ip)
	if _, err := s.DB.Exec(r.Context(), `INSERT INTO contacto (nombre, correo, telefono, empresa, edificios_aprox, mensaje, ip) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		strings.TrimSpace(in.Nombre), strings.TrimSpace(in.Correo), strings.TrimSpace(tel), in.Empresa, in.EdificiosAprox, strings.TrimSpace(in.Mensaje), ip); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusAccepted, map[string]any{"ok": true, "mensaje": "Te escribimos en menos de 24 horas."})
}

func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
