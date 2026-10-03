package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Dominio propio (bloque I3): cada administradora puede servir EDISYS en su dominio
// (p. ej. intranet.miadministradora.pe). El borde (Caddy) enruta por Host al mismo API; el API
// resuelve la administradora a partir del Host. EDISYS no toca DNS ni certificados: el cliente
// apunta su registro DNS al servidor y Caddy saca el certificado bajo demanda, preguntando antes
// a /publico/dominio-permitido si el host está registrado (así nadie emite certificados ajenos).
//
// Sobre X-Forwarded-Host: se confía en él porque detrás del borde es Caddy quien lo pone. Falsificarlo
// solo puede RESTRINGIR (un login rechazado) o revelar el nombre comercial de una administradora:
// nunca da acceso a datos.

var reHost = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// normalizarHost: minúsculas, sin puerto, sin punto final ni esquema. Vacío si no es un host válido.
func normalizarHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://")
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	h = strings.TrimSuffix(h, ".")
	if len(h) > 253 || !reHost.MatchString(h) {
		return ""
	}
	return h
}

// hostDePeticion: el host público por el que llegó la petición.
func hostDePeticion(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-Host"); xf != "" {
		return normalizarHost(strings.Split(xf, ",")[0])
	}
	return normalizarHost(r.Host)
}

// administradoraPorHost devuelve la administradora dueña del host (0 si el host no es un dominio propio).
func (s *Server) administradoraPorHost(ctx context.Context, host string) (int64, string, error) {
	if host == "" {
		return 0, "", nil
	}
	var id int64
	var nombre string
	err := s.DB.QueryRow(ctx, `SELECT a.id, a.nombre FROM administradora_dominio d JOIN administradora a ON a.id=d.administradora_id
		WHERE d.host=$1 AND d.activo`, host).Scan(&id, &nombre)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", nil
	}
	return id, nombre, err
}

// loginPermitidoEnHost: en un dominio propio solo entran los usuarios de esa administradora
// (y el superadmin). En el dominio de la plataforma no cambia nada.
func (s *Server) loginPermitidoEnHost(ctx context.Context, r *http.Request, uid int64) error {
	adm, _, err := s.administradoraPorHost(ctx, hostDePeticion(r))
	if err != nil || adm == 0 {
		return err
	}
	var suya *int64
	var super bool
	if err := s.DB.QueryRow(ctx, `SELECT administradora_id, es_superadmin FROM usuario WHERE id=$1`, uid).Scan(&suya, &super); err != nil {
		return err
	}
	if super || (suya != nil && *suya == adm) {
		return nil
	}
	// Mismo mensaje que una clave errada: no revela que la cuenta existe en otra administradora.
	return P.Err(http.StatusUnauthorized, "CREDENCIALES", "Usuario o clave incorrectos.")
}

// dominioPublico: GET /publico/dominio → la administradora del Host (para que login y app pongan su nombre).
func (s *Server) dominioPublico(w http.ResponseWriter, r *http.Request) {
	host := hostDePeticion(r)
	adm, nombre, err := s.administradoraPorHost(r.Context(), host)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if adm == 0 {
		P.JSON(w, http.StatusOK, map[string]any{"host": host, "propio": false})
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"host": host, "propio": true, "administradora_id": adm, "administradora": nombre})
}

// dominioPermitido: GET /publico/dominio-permitido?domain= → 200 si el host está registrado y activo, 404 si no.
// Es el «ask» del TLS bajo demanda de Caddy (edge/dominios.ejemplo.caddy).
func (s *Server) dominioPermitido(w http.ResponseWriter, r *http.Request) {
	host := normalizarHost(r.URL.Query().Get("domain"))
	adm, _, err := s.administradoraPorHost(r.Context(), host)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if adm == 0 {
		P.Fallo(w, r, P.NoEncontrado("el dominio"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"host": host, "permitido": true})
}

// admDelEdificio: la administradora dueña del edificio activo.
func (s *Server) admDelEdificio(ctx context.Context, eid int64) (int64, error) {
	var adm int64
	err := s.DB.QueryRow(ctx, `SELECT administradora_id FROM edificio WHERE id=$1`, eid).Scan(&adm)
	return adm, err
}

// listarDominios: GET /dominios (los de la administradora del edificio activo).
func (s *Server) listarDominios(w http.ResponseWriter, r *http.Request) {
	adm, err := s.admDelEdificio(r.Context(), edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	filas, err := db.Filas(r.Context(), s.DB, `SELECT id, host, activo,
			to_char(creado_en AT TIME ZONE 'America/Lima','YYYY-MM-DD') AS fecha
		FROM administradora_dominio WHERE administradora_id=$1 ORDER BY host`, adm)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "host_actual": hostDePeticion(r)})
}

// crearDominio: POST /dominios {host}
func (s *Server) crearDominio(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Host string `json:"host"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	host := normalizarHost(in.Host)
	if host == "" {
		P.Fallo(w, r, P.Validacion("Escribe un dominio válido, p. ej. intranet.tuadministradora.pe.").Campo("host", "Dominio inválido."))
		return
	}
	adm, err := s.admDelEdificio(ctx, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var id int64
	err = s.DB.QueryRow(ctx, `INSERT INTO administradora_dominio (administradora_id, host, creado_por) VALUES ($1,$2,$3) RETURNING id`,
		adm, host, ses(r).UsuarioID).Scan(&id)
	if err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("DOMINIO_EXISTE", "Ese dominio ya está registrado."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "host": host})
}

// cambiarDominio: PATCH /dominios/{id} {activo}
func (s *Server) cambiarDominio(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := idURL(r, "id")
	var in struct {
		Activo bool `json:"activo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	adm, err := s.admDelEdificio(ctx, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	ct, err := s.DB.Exec(ctx, `UPDATE administradora_dominio SET activo=$1 WHERE id=$2 AND administradora_id=$3`, in.Activo, id, adm)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("el dominio"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "activo": in.Activo})
}

// borrarDominio: DELETE /dominios/{id}
func (s *Server) borrarDominio(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := idURL(r, "id")
	adm, err := s.admDelEdificio(ctx, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	ct, err := s.DB.Exec(ctx, `DELETE FROM administradora_dominio WHERE id=$1 AND administradora_id=$2`, id, adm)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("el dominio"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}
