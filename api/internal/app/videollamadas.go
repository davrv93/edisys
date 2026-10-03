package app

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Videollamadas (bloque J2): salas junta ↔ administración sobre Jitsi. EDISYS no usa SDK ni
// llama a nadie: solo arma el enlace https://<JITSI_BASE_URL>/<código>. El código es aleatorio
// (80 bits), así que la sala no se adivina; quien no tiene videollamadas.ver no recibe el enlace.

// jitsiPorDefecto es la instancia pública de Jitsi; en producción se apunta a una propia con JITSI_BASE_URL.
const jitsiPorDefecto = "https://meet.jit.si"

// jitsiBase lee JITSI_BASE_URL (se lee en cada petición: cambiarla no exige reiniciar el API en pruebas).
// Solo se acepta http(s) con host; cualquier otra cosa cae al valor por defecto.
func jitsiBase() string {
	v := strings.TrimRight(strings.TrimSpace(os.Getenv("JITSI_BASE_URL")), "/")
	if u, err := url.Parse(v); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
		return v
	}
	return jitsiPorDefecto
}

// enlaceSala arma la URL de la sala; el código no lleva datos personales.
func enlaceSala(codigo string) string {
	return jitsiBase() + "/" + codigo
}

// codigoSala: «edisys-<eid>-<20 hex>»; minúsculas y guiones, como pide el CHECK de la tabla.
func codigoSala(eid int64) (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "edisys-" + strconv.FormatInt(eid, 10) + "-" + hex.EncodeToString(b), nil
}

// configVideollamadas: GET /videollamadas/config → a qué servidor Jitsi apuntan los enlaces.
func (s *Server) configVideollamadas(w http.ResponseWriter, r *http.Request) {
	base := jitsiBase()
	P.JSON(w, http.StatusOK, map[string]any{"base_url": base, "propia": base != jitsiPorDefecto})
}

// listarVideollamadas: GET /videollamadas. El estado se calcula con la hora: programada,
// en_curso (desde 10 min antes del inicio hasta el fin), finalizada o cancelada.
func (s *Server) listarVideollamadas(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT v.id, v.titulo, v.descripcion, v.duracion_min, v.codigo, v.grabacion_url,
			to_char(v.inicia_en AT TIME ZONE 'America/Lima','YYYY-MM-DD"T"HH24:MI') AS inicia_en,
			CASE WHEN v.cancelada THEN 'cancelada'
			     WHEN now() > v.inicia_en + make_interval(mins => v.duracion_min) THEN 'finalizada'
			     WHEN now() >= v.inicia_en - interval '10 minutes' THEN 'en_curso'
			     ELSE 'programada' END AS estado,
			COALESCE(u.nombre,'') AS convoca
		FROM videollamada v LEFT JOIN usuario u ON u.id=v.creado_por
		WHERE v.edificio_id=$1
		ORDER BY (NOT v.cancelada AND now() <= v.inicia_en + make_interval(mins => v.duracion_min)) DESC,
		         CASE WHEN now() <= v.inicia_en + make_interval(mins => v.duracion_min) THEN v.inicia_en END ASC,
		         v.inicia_en DESC`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, f := range filas {
		if c, ok := f["codigo"].(string); ok {
			f["enlace"] = enlaceSala(c)
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "base_url": jitsiBase()})
}

// crearVideollamada: POST /videollamadas {titulo, descripcion, inicia_en (AAAA-MM-DDTHH:MM, Lima), duracion_min}
func (s *Server) crearVideollamada(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in struct {
		Titulo      string `json:"titulo"`
		Descripcion string `json:"descripcion"`
		IniciaEn    string `json:"inicia_en"`
		DuracionMin int    `json:"duracion_min"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.Titulo = strings.TrimSpace(in.Titulo)
	in.Descripcion = strings.TrimSpace(in.Descripcion)
	if in.DuracionMin == 0 {
		in.DuracionMin = 60
	}
	ev := P.Validacion("Revisa la reunión.")
	if in.Titulo == "" {
		ev.Campo("titulo", "Escribe el asunto de la reunión.")
	}
	if in.DuracionMin < 15 || in.DuracionMin > 480 {
		ev.Campo("duracion_min", "La duración va de 15 a 480 minutos.")
	}
	inicio, err := time.ParseInLocation("2006-01-02T15:04", strings.TrimSpace(in.IniciaEn), P.Lima)
	if err != nil {
		if t, err2 := time.Parse(time.RFC3339, strings.TrimSpace(in.IniciaEn)); err2 == nil {
			inicio, err = t, nil
		}
	}
	if err != nil {
		ev.Campo("inicia_en", "Indica fecha y hora (AAAA-MM-DDTHH:MM).")
	} else if inicio.Before(time.Now().Add(-time.Hour)) {
		ev.Campo("inicia_en", "La reunión no puede empezar en el pasado.")
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	codigo, err := codigoSala(e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var id int64
	if err := s.DB.QueryRow(r.Context(), `INSERT INTO videollamada (edificio_id, titulo, descripcion, inicia_en, duracion_min, codigo, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, e.ID, in.Titulo, in.Descripcion, inicio, in.DuracionMin, codigo, ses(r).UsuarioID).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "codigo": codigo, "enlace": enlaceSala(codigo)})
}

// cancelarVideollamada: POST /videollamadas/{id}/cancelar
func (s *Server) cancelarVideollamada(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	ct, err := s.DB.Exec(r.Context(), `UPDATE videollamada SET cancelada=true
		WHERE id=$1 AND edificio_id=$2 AND NOT cancelada AND now() <= inicia_en + make_interval(mins => duracion_min)`, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.Conflicto("NO_CANCELABLE", "La reunión ya terminó, ya estaba cancelada o no existe."))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": "cancelada"})
}

// grabacionVideollamada: PUT /videollamadas/{id}/grabacion {url}. La grabación es opcional y vive
// fuera (Jitsi la deja en Dropbox o en el servidor propio): aquí solo se guarda el enlace https.
func (s *Server) grabacionVideollamada(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	var in struct {
		URL string `json:"url"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.URL = strings.TrimSpace(in.URL)
	if in.URL != "" {
		u, err := url.Parse(in.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			P.Fallo(w, r, P.Validacion("El enlace de la grabación debe empezar con https://.").Campo("url", "Enlace https inválido."))
			return
		}
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE videollamada SET grabacion_url=$1 WHERE id=$2 AND edificio_id=$3`, in.URL, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("la reunión"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "grabacion_url": in.URL})
}
