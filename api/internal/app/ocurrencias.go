package app

import (
	"fmt"
	"net/http"
	"strings"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// G1 · Cuaderno de ocurrencias: la bitácora de portería y del personal. Lo anotado no se edita
// ni se borra (regla en la base); una ocurrencia se cierra con nota o se escala a incidencia.

var prioridadesOcurrencia = map[string]bool{"alta": true, "media": true, "baja": true}

const sqlOcurrencia = `SELECT o.id, o.numero, o.titulo, o.descripcion, o.prioridad, o.estado, o.empleado, o.foto_id,
	o.registrado_en, rp.nombre AS registrado_por, o.cierre_nota, o.cerrado_en, cp.nombre AS cerrado_por,
	o.incidencia_id, i.codigo AS incidencia_codigo, i.estado AS incidencia_estado
	FROM ocurrencia o LEFT JOIN usuario rp ON rp.id=o.registrado_por LEFT JOIN usuario cp ON cp.id=o.cerrado_por
	LEFT JOIN incidencia i ON i.id=o.incidencia_id`

func (s *Server) decorarOcurrencia(f map[string]any) {
	if id, ok := f["foto_id"].(int64); ok {
		f["foto_url"] = s.Firma.URL(id)
	} else {
		f["foto_url"] = nil
	}
	f["codigo"] = fmt.Sprintf("OC-%03d", f["numero"])
}

// listarOcurrencias: GET /ocurrencias?estado=&prioridad=&desde=&hasta=&q=
func (s *Server) listarOcurrencias(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	v := r.URL.Query()
	cond := []string{"o.edificio_id=$1"}
	args := []any{e.ID}
	add := func(c string, a any) {
		args = append(args, a)
		cond = append(cond, strings.ReplaceAll(c, "?", fmt.Sprintf("$%d", len(args))))
	}
	if x := v.Get("estado"); x != "" {
		add("o.estado = ?", x)
	}
	if x := v.Get("prioridad"); prioridadesOcurrencia[x] {
		add("o.prioridad = ?", x)
	}
	if t, err := parseFecha(v.Get("desde")); err == nil {
		add("o.registrado_en >= ?", t)
	}
	if t, err := parseFecha(v.Get("hasta")); err == nil {
		add("o.registrado_en < ?", t.AddDate(0, 0, 1))
	}
	if t := strings.TrimSpace(v.Get("q")); t != "" {
		add("(o.titulo ILIKE ? OR o.descripcion ILIKE ? OR o.empleado ILIKE ?)", "%"+t+"%")
	}
	filas, err := db.Filas(r.Context(), s.DB, sqlOcurrencia+` WHERE `+strings.Join(cond, " AND ")+`
		ORDER BY o.registrado_en DESC, o.id DESC LIMIT 500`, args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	conteos := map[string]int{"abierta": 0, "cerrada": 0, "escalada": 0}
	for _, f := range filas {
		s.decorarOcurrencia(f)
		conteos[f["estado"].(string)]++
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "conteos": conteos})
}

// crearOcurrencia: POST /ocurrencias (multipart o JSON {titulo, descripcion, prioridad, empleado}, foto opcional).
func (s *Server) crearOcurrencia(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	se := ses(r)
	ctx := r.Context()
	var in struct {
		Titulo      string `json:"titulo"`
		Descripcion string `json:"descripcion"`
		Prioridad   string `json:"prioridad"`
		Empleado    string `json:"empleado"`
	}
	var fotos []Subido
	if esMultipart(r) {
		var err error
		if fotos, err = archivosDeForm(r, "foto", "fotos", "fotos[]"); err != nil {
			P.Fallo(w, r, err)
			return
		}
		in.Titulo, in.Descripcion, in.Prioridad, in.Empleado = campo(r, "titulo"), campo(r, "descripcion"), campo(r, "prioridad"), campo(r, "empleado")
	} else if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.Titulo = strings.TrimSpace(in.Titulo)
	if in.Prioridad == "" {
		in.Prioridad = "media"
	}
	ev := P.Validacion("Revisa la ocurrencia.")
	if in.Titulo == "" {
		ev.Campo("titulo", "Escribe qué pasó.")
	}
	if !prioridadesOcurrencia[in.Prioridad] {
		ev.Campo("prioridad", "alta, media o baja.")
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
	// El número es correlativo por edificio: se bloquea el edificio para no repetirlo.
	if _, err := tx.Exec(ctx, `SELECT id FROM edificio WHERE id=$1 FOR UPDATE`, e.ID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	var fotoID *int64
	if len(fotos) > 0 {
		id, err := s.guardarArchivo(ctx, tx, e.ID, &se.UsuarioID, fotos[0])
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		fotoID = &id
	}
	empleado := strings.TrimSpace(in.Empleado)
	var id int64
	var num int
	if err := tx.QueryRow(ctx, `INSERT INTO ocurrencia (edificio_id, numero, titulo, descripcion, prioridad, empleado, foto_id, registrado_por)
		VALUES ($1,(SELECT COALESCE(max(numero),0)+1 FROM ocurrencia WHERE edificio_id=$1),$2,$3,$4,$5,$6,$7) RETURNING id, numero`,
		e.ID, in.Titulo, strings.TrimSpace(in.Descripcion), in.Prioridad, empleado, fotoID, se.UsuarioID).Scan(&id, &num); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "numero": num, "codigo": fmt.Sprintf("OC-%03d", num), "estado": "abierta"})
}

// cerrarOcurrencia: POST /ocurrencias/{id}/cerrar {nota}
func (s *Server) cerrarOcurrencia(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	var in struct {
		Nota string `json:"nota"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if strings.TrimSpace(in.Nota) == "" {
		P.Fallo(w, r, P.Validacion("Anota cómo se resolvió.").Campo("nota", "Obligatoria."))
		return
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE ocurrencia SET estado='cerrada', cierre_nota=$3, cerrado_por=$4, cerrado_en=now()
		WHERE id=$1 AND edificio_id=$2 AND estado='abierta'`, id, e.ID, strings.TrimSpace(in.Nota), ses(r).UsuarioID)
	if err != nil {
		P.Fallo(w, r, errOperacion(err))
		return
	}
	if ct.RowsAffected() == 0 {
		var existe bool
		_ = s.DB.QueryRow(r.Context(), `SELECT true FROM ocurrencia WHERE id=$1 AND edificio_id=$2`, id, e.ID).Scan(&existe)
		if existe {
			P.Fallo(w, r, P.Conflicto("OCURRENCIA_CERRADA", "Esa ocurrencia ya está cerrada; el cuaderno no se reabre."))
		} else {
			P.Fallo(w, r, P.NoEncontrado("esa ocurrencia"))
		}
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": "cerrada"})
}

// escalarOcurrencia: POST /ocurrencias/{id}/escalar {categoria?, ubicacion?} → crea la incidencia en el
// tablero de mantenimiento (estado «reportado», con la foto como evidencia) y deja el vínculo.
func (s *Server) escalarOcurrencia(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	se := ses(r)
	ctx := r.Context()
	id := idURL(r, "id")
	var in struct {
		Categoria string `json:"categoria"`
		Ubicacion string `json:"ubicacion"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var titulo, desc, estado string
	var foto *int64
	if err := tx.QueryRow(ctx, `SELECT titulo, descripcion, estado, foto_id FROM ocurrencia WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, id, e.ID).
		Scan(&titulo, &desc, &estado, &foto); err != nil {
		P.Fallo(w, r, P.NoEncontrado("esa ocurrencia"))
		return
	}
	if estado != "abierta" {
		P.Fallo(w, r, P.Conflicto("OCURRENCIA_CERRADA", "Solo se escala una ocurrencia abierta."))
		return
	}
	if desc == "" {
		desc = titulo
	}
	incID, codigo, err := s.crearIncidencia(ctx, tx, e.ID, &se.UsuarioID, nil, titulo, desc, strings.TrimSpace(in.Ubicacion), in.Categoria, "admin")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if foto != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO incidencia_evidencia (incidencia_id, archivo_id, tipo, usuario_id) VALUES ($1,$2,'reporte',$3)`, incID, *foto, se.UsuarioID); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE ocurrencia SET estado='escalada', incidencia_id=$2, cerrado_por=$3, cerrado_en=now(),
		cierre_nota='Escalada a '||$4::text WHERE id=$1`, id, incID, se.UsuarioID, codigo); err != nil {
		P.Fallo(w, r, errOperacion(err))
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "estado": "escalada", "incidencia_id": incID, "incidencia_codigo": codigo})
}
