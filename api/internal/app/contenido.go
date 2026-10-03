package app

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Preguntas frecuentes, Academia y Beneficios (bloque E5). Son tres listas de contenido por edificio
// con la misma forma (crear, editar, ocultar, borrar), así que comparten un manejador descrito por
// tipoContenido. Las columnas salen SOLO de esa descripción: nada del cliente llega al SQL como nombre.
// El portal ve lo publicado (y, en beneficios, lo vigente); quien administra lo ve todo.

type campoContenido struct {
	nombre      string
	tipo        string // texto | entero | bool | fecha | opcion
	obligatorio bool
	max         int      // largo máximo de un texto (0 = libre)
	opciones    []string // para «opcion»
	url         bool     // texto que, si viene, debe ser http(s)://
}

type tipoContenido struct {
	tabla, que string // «faq», «la pregunta»
	campos     []campoContenido
	orden      string // ORDER BY
	vigencia   string // condición extra para el portal (vacía = siempre vigente)
	validar    func(v map[string]any, ev *P.Error)
}

var (
	contenidoFAQ = tipoContenido{
		tabla: "faq", que: "la pregunta",
		campos: []campoContenido{
			{nombre: "pregunta", tipo: "texto", obligatorio: true, max: 300},
			{nombre: "respuesta", tipo: "texto", obligatorio: true, max: 5000},
			{nombre: "categoria", tipo: "texto", max: 60},
			{nombre: "orden", tipo: "entero"},
			{nombre: "publicado", tipo: "bool"},
		},
		orden: "lower(categoria), orden, id",
	}
	contenidoAcademia = tipoContenido{
		tabla: "academia", que: "la lección",
		campos: []campoContenido{
			{nombre: "titulo", tipo: "texto", obligatorio: true, max: 160},
			{nombre: "resumen", tipo: "texto", max: 500},
			{nombre: "tipo", tipo: "opcion", opciones: []string{"articulo", "video", "guia"}},
			{nombre: "url", tipo: "texto", url: true, max: 500},
			{nombre: "contenido", tipo: "texto", max: 20000},
			{nombre: "orden", tipo: "entero"},
			{nombre: "publicado", tipo: "bool"},
		},
		orden: "orden, id",
		// Una lección sin enlace ni texto no enseña nada (la base lo exige también).
		validar: func(v map[string]any, ev *P.Error) {
			u, _ := v["url"].(string)
			c, _ := v["contenido"].(string)
			if u == "" && c == "" {
				ev.Campo("url", "Pon un enlace (video, PDF) o escribe el contenido.")
			}
		},
	}
	contenidoBeneficio = tipoContenido{
		tabla: "beneficio", que: "el beneficio",
		campos: []campoContenido{
			{nombre: "titulo", tipo: "texto", obligatorio: true, max: 160},
			{nombre: "descripcion", tipo: "texto", max: 2000},
			{nombre: "proveedor", tipo: "texto", max: 120},
			{nombre: "descuento", tipo: "texto", max: 60},
			{nombre: "codigo", tipo: "texto", max: 60},
			{nombre: "url", tipo: "texto", url: true, max: 500},
			{nombre: "vigente_hasta", tipo: "fecha"},
			{nombre: "orden", tipo: "entero"},
			{nombre: "publicado", tipo: "bool"},
		},
		orden:    "orden, id",
		vigencia: "(vigente_hasta IS NULL OR vigente_hasta >= (now() AT TIME ZONE 'America/Lima')::date)",
	}
)

// leerCampos valida el cuerpo contra la descripción. En creación exige los obligatorios; en edición
// solo valida lo que llega. Devuelve los valores listos para el SQL, en el orden de t.campos.
func (t tipoContenido) leerCampos(in map[string]any, crear bool) (cols []string, vals []any, limpio map[string]any, ev *P.Error) {
	ev = P.Validacion("Revisa " + t.que + ".")
	limpio = map[string]any{}
	for _, c := range t.campos {
		bruto, viene := in[c.nombre]
		if !viene || bruto == nil {
			if crear && c.obligatorio {
				ev.Campo(c.nombre, "Obligatorio.")
			}
			continue
		}
		var v any
		switch c.tipo {
		case "texto", "opcion":
			s, ok := bruto.(string)
			if !ok {
				ev.Campo(c.nombre, "Debe ser texto.")
				continue
			}
			s = strings.TrimSpace(s)
			switch {
			case c.obligatorio && s == "":
				ev.Campo(c.nombre, "Obligatorio.")
				continue
			case c.max > 0 && len([]rune(s)) > c.max:
				ev.Campo(c.nombre, fmt.Sprintf("Máximo %d caracteres.", c.max))
				continue
			case c.url && s != "" && !(strings.HasPrefix(strings.ToLower(s), "http://") || strings.HasPrefix(strings.ToLower(s), "https://")):
				ev.Campo(c.nombre, "El enlace debe empezar con http:// o https://.")
				continue
			case c.tipo == "opcion" && !contiene(c.opciones, s):
				ev.Campo(c.nombre, "Usa "+strings.Join(c.opciones, ", ")+".")
				continue
			}
			v = s
		case "entero":
			f, ok := bruto.(float64)
			if !ok || f != float64(int64(f)) || f < 0 || f > 1e6 {
				ev.Campo(c.nombre, "Número entero entre 0 y 1 000 000.")
				continue
			}
			v = int64(f)
		case "bool":
			b, ok := bruto.(bool)
			if !ok {
				ev.Campo(c.nombre, "Sí o no.")
				continue
			}
			v = b
		case "fecha":
			s, _ := bruto.(string)
			s = strings.TrimSpace(s)
			if s == "" {
				v = nil // vacío = sin fecha
			} else if f, err := time.Parse("2006-01-02", s); err != nil {
				ev.Campo(c.nombre, "Formato AAAA-MM-DD.")
				continue
			} else {
				v = f
			}
		}
		cols = append(cols, c.nombre)
		vals = append(vals, v)
		limpio[c.nombre] = v
	}
	if crear && t.validar != nil && len(ev.Campos) == 0 {
		t.validar(limpio, ev)
	}
	if len(ev.Campos) == 0 {
		ev = nil
	}
	return
}

func contiene(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// columnasSelect: id, los campos (las fechas como AAAA-MM-DD) y las marcas de tiempo.
func (t tipoContenido) columnasSelect() string {
	cols := []string{"id"}
	for _, c := range t.campos {
		if c.tipo == "fecha" {
			cols = append(cols, "to_char("+c.nombre+",'YYYY-MM-DD') AS "+c.nombre)
		} else {
			cols = append(cols, c.nombre)
		}
	}
	vig := "true"
	if t.vigencia != "" {
		vig = t.vigencia
	}
	return strings.Join(cols, ", ") + ", " + vig + " AS vigente, creado_en, actualizado_en"
}

// listarContenido: GET /<tipo>. Portal: lo publicado y vigente. Administración: todo.
func (s *Server) listarContenido(t tipoContenido) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e := edf(r)
		admin := e.Puede("contenido.administrar")
		cond := "edificio_id=$1 AND ($2 OR publicado)"
		if t.vigencia != "" {
			cond += " AND ($2 OR " + t.vigencia + ")"
		}
		filas, err := db.Filas(r.Context(), s.DB, `SELECT `+t.columnasSelect()+` FROM `+t.tabla+` WHERE `+cond+` ORDER BY `+t.orden, e.ID, admin)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
	}
}

// crearContenido: POST /<tipo>
func (s *Server) crearContenido(t tipoContenido) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e := edf(r)
		var in map[string]any
		if err := P.Leer(r, &in); err != nil {
			P.Fallo(w, r, err)
			return
		}
		cols, vals, _, ev := t.leerCampos(in, true)
		if ev != nil {
			P.Fallo(w, r, ev)
			return
		}
		cols = append([]string{"edificio_id"}, cols...)
		vals = append([]any{e.ID}, vals...)
		marcas := make([]string, len(cols))
		for i := range cols {
			marcas[i] = "$" + strconv.Itoa(i+1)
		}
		var id int64
		if err := s.DB.QueryRow(r.Context(), `INSERT INTO `+t.tabla+` (`+strings.Join(cols, ", ")+`) VALUES (`+strings.Join(marcas, ", ")+`) RETURNING id`, vals...).Scan(&id); err != nil {
			P.Fallo(w, r, err)
			return
		}
		P.JSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// editarContenido: PUT /<tipo>/{id}. Cambia solo los campos que llegan (p. ej. {publicado:false}).
func (s *Server) editarContenido(t tipoContenido) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e := edf(r)
		id := idURL(r, "id")
		var in map[string]any
		if err := P.Leer(r, &in); err != nil {
			P.Fallo(w, r, err)
			return
		}
		cols, vals, _, ev := t.leerCampos(in, false)
		if ev != nil {
			P.Fallo(w, r, ev)
			return
		}
		if len(cols) == 0 {
			P.Fallo(w, r, P.Validacion("No llegó ningún campo para cambiar."))
			return
		}
		sets := make([]string, len(cols))
		for i, c := range cols {
			sets[i] = c + "=$" + strconv.Itoa(i+1)
		}
		vals = append(vals, id, e.ID)
		n := len(vals)
		ct, err := s.DB.Exec(r.Context(), `UPDATE `+t.tabla+` SET `+strings.Join(sets, ", ")+`, actualizado_en=now() WHERE id=$`+strconv.Itoa(n-1)+` AND edificio_id=$`+strconv.Itoa(n), vals...)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		if ct.RowsAffected() == 0 {
			P.Fallo(w, r, P.NoEncontrado(t.que))
			return
		}
		P.JSON(w, http.StatusOK, map[string]any{"id": id})
	}
}

// borrarContenido: DELETE /<tipo>/{id}
func (s *Server) borrarContenido(t tipoContenido) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e := edf(r)
		id := idURL(r, "id")
		ct, err := s.DB.Exec(r.Context(), `DELETE FROM `+t.tabla+` WHERE id=$1 AND edificio_id=$2`, id, e.ID)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		if ct.RowsAffected() == 0 {
			P.Fallo(w, r, P.NoEncontrado(t.que))
			return
		}
		P.JSON(w, http.StatusOK, map[string]any{"id": id})
	}
}
