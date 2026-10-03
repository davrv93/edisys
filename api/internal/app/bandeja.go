package app

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Bandeja de correos (bloque E3): detalle, adjuntos y reintento sobre correo_mensaje/correo_adjunto (0010).
// El listado es listarCorreos (correo.go).

// verCorreo: GET /correo/mensajes/{id} → el mensaje con su cuerpo y la lista de adjuntos (sin los bytes).
func (s *Server) verCorreo(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	m, err := db.Fila(ctx, s.DB, `SELECT m.id, m.para, m.nombre, m.asunto, m.html, m.texto, m.estado, m.origen, m.referencia, m.intentos, m.error,
			m.creado_en, m.procesado_en, u.codigo AS unidad, us.nombre AS enviado_por
		FROM correo_mensaje m LEFT JOIN unidad u ON u.id=m.unidad_id LEFT JOIN usuario us ON us.id=m.enviado_por
		WHERE m.id=$1 AND m.edificio_id=$2`, id, e.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		P.Fallo(w, r, P.NoEncontrado("el correo"))
		return
	}
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	adj, err := db.Filas(ctx, s.DB, `SELECT id, nombre, tipo_mime, octet_length(datos) AS bytes FROM correo_adjunto WHERE mensaje_id=$1 ORDER BY id`, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	m["adjuntos"] = adj
	P.JSON(w, http.StatusOK, m)
}

// descargarAdjuntoCorreo: GET /correo/mensajes/{id}/adjuntos/{aid} → el archivo tal como salió.
func (s *Server) descargarAdjuntoCorreo(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var nombre, tipo string
	var datos []byte
	err := s.DB.QueryRow(r.Context(), `SELECT a.nombre, a.tipo_mime, a.datos FROM correo_adjunto a JOIN correo_mensaje m ON m.id=a.mensaje_id
		WHERE a.id=$1 AND a.mensaje_id=$2 AND m.edificio_id=$3`, idURL(r, "aid"), idURL(r, "id"), e.ID).Scan(&nombre, &tipo, &datos)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("el adjunto"))
		return
	}
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("Content-Length", strconv.Itoa(len(datos)))
	w.Header().Set("Content-Disposition", `inline; filename="`+reNombre.ReplaceAllString(nombre, "_")+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(datos)
}

// reintentarCorreo: POST /correo/mensajes/{id}/reintentar. Solo un correo en «error» vuelve a la cola
// (intentos a cero); lo enviado o simulado no se repite, así nadie lo recibe dos veces.
func (s *Server) reintentarCorreo(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	ct, err := s.DB.Exec(ctx, `UPDATE correo_mensaje SET estado='pendiente', intentos=0, error='' WHERE id=$1 AND edificio_id=$2 AND estado='error'`, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		var estado string
		if err := s.DB.QueryRow(ctx, `SELECT estado FROM correo_mensaje WHERE id=$1 AND edificio_id=$2`, id, e.ID).Scan(&estado); err != nil {
			P.Fallo(w, r, P.NoEncontrado("el correo"))
			return
		}
		P.Fallo(w, r, P.Conflicto("NO_REINTENTABLE", "Solo se reintenta un correo con error; este está «"+estado+"»."))
		return
	}
	s.auditarCambio(ctx, s.DB, r, "correo", "reintentar", "correo_mensaje", id, nil, map[string]any{"estado": "pendiente"})
	s.despacharCorreos(ctx)
	var estado string
	_ = s.DB.QueryRow(ctx, `SELECT estado FROM correo_mensaje WHERE id=$1`, id).Scan(&estado)
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": estado, "modo": s.Cfg.CorreoModo})
}
