package app

import (
	"net/http"
	"strconv"
	"strings"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Documentos por categorías (bloque E1): actas, reglamentos y demás, con su archivo en el cubo privado.

// listarDocumentoCategorias: GET /documentos/categorias
func (s *Server) listarDocumentoCategorias(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT c.id, c.nombre, c.orden, c.activo,
			count(d.id) FILTER (WHERE d.publicado) AS documentos
		FROM documento_categoria c LEFT JOIN documento_publicado d ON d.categoria_id=c.id
		WHERE c.edificio_id=$1 GROUP BY c.id ORDER BY c.orden, lower(c.nombre)`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearDocumentoCategoria: POST /documentos/categorias
func (s *Server) crearDocumentoCategoria(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in struct {
		Nombre string `json:"nombre"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.Nombre = strings.TrimSpace(in.Nombre)
	if in.Nombre == "" {
		P.Fallo(w, r, P.Validacion("Escribe el nombre de la categoría.").Campo("nombre", "Obligatorio."))
		return
	}
	var id int64
	err := s.DB.QueryRow(r.Context(), `INSERT INTO documento_categoria (edificio_id, nombre, orden)
		VALUES ($1,$2,(SELECT COALESCE(max(orden),0)+1 FROM documento_categoria WHERE edificio_id=$1)) RETURNING id`, e.ID, in.Nombre).Scan(&id)
	if err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("CATEGORIA_DUPLICADA", "Ya existe una categoría con ese nombre."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// borrarDocumentoCategoria: DELETE /documentos/categorias/{id}
func (s *Server) borrarDocumentoCategoria(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	if _, err := s.DB.Exec(r.Context(), `DELETE FROM documento_categoria WHERE id=$1 AND edificio_id=$2`, id, e.ID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

// listarDocumentos: GET /documentos?categoria_id=
func (s *Server) listarDocumentos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	cat, _ := strconv.ParseInt(r.URL.Query().Get("categoria_id"), 10, 64)
	soloPropios := e.Puede("documentos.administrar")
	filas, err := db.Filas(r.Context(), s.DB, `SELECT d.id, d.categoria_id, c.nombre AS categoria, d.titulo, d.numero, d.resumen, d.archivo_id, d.publicado,
			to_char(d.creado_en AT TIME ZONE 'America/Lima','YYYY-MM-DD') AS fecha
		FROM documento_publicado d JOIN documento_categoria c ON c.id=d.categoria_id
		WHERE d.edificio_id=$1 AND ($2=0 OR d.categoria_id=$2) AND ($3 OR d.publicado)
		ORDER BY d.publicado DESC, d.creado_en DESC`, e.ID, cat, soloPropios)
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

// crearDocumento: POST /documentos (multipart con «archivo»)
func (s *Server) crearDocumento(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	if err := leerMultipart(r); err != nil {
		P.Fallo(w, r, err)
		return
	}
	categoriaID, _ := strconv.ParseInt(campo(r, "categoria_id"), 10, 64)
	titulo := strings.TrimSpace(campo(r, "titulo"))
	numero := strings.TrimSpace(campo(r, "numero"))
	resumen := strings.TrimSpace(campo(r, "resumen"))
	docs, err := archivosDeForm(r, "archivo", "documento")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa el documento.")
	if titulo == "" {
		ev.Campo("titulo", "Escribe el título.")
	}
	var ok bool
	_ = s.DB.QueryRow(ctx, `SELECT true FROM documento_categoria WHERE id=$1 AND edificio_id=$2`, categoriaID, e.ID).Scan(&ok)
	if !ok {
		ev.Campo("categoria_id", "Elige una categoría del edificio.")
	}
	if len(docs) == 0 {
		ev.Campo("archivo", "Adjunta el archivo.")
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
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO documento_publicado (edificio_id, categoria_id, titulo, numero, resumen, archivo_id, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, e.ID, categoriaID, titulo, numero, resumen, archID, se.UsuarioID).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "archivo_id": archID})
}

// publicarDocumento: POST /documentos/{id}/publicar {publicado}
func (s *Server) publicarDocumento(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	var in struct {
		Publicado bool `json:"publicado"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE documento_publicado SET publicado=$1 WHERE id=$2 AND edificio_id=$3`, in.Publicado, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("el documento"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "publicado": in.Publicado})
}
