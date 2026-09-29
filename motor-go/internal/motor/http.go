package motor

import (
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"edisys/motor-go/internal/pyjson"
)

// ---------- respuestas como Starlette ----------

// escribir emite JSON como JSONResponse de Starlette: ensure_ascii=False y
// separadores (",", ":").
func escribir(w http.ResponseWriter, codigo int, v *pyjson.Value) {
	b, _ := v.MarshalJSON()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.WriteHeader(codigo)
	_, _ = w.Write(b)
}

func obj(kv ...any) *pyjson.Value {
	o := pyjson.NewObject()
	for i := 0; i+1 < len(kv); i += 2 {
		o.Set(kv[i].(string), valor(kv[i+1]))
	}
	return o
}

func valor(x any) *pyjson.Value {
	switch v := x.(type) {
	case nil:
		return pyjson.NewNull()
	case *pyjson.Value:
		if v == nil {
			return pyjson.NewNull()
		}
		return v
	case string:
		return pyjson.NewString(v)
	case bool:
		return pyjson.NewBool(v)
	case int:
		return pyjson.NewInt(int64(v))
	case int64:
		return pyjson.NewInt(v)
	case float64:
		return pyjson.NewFloat(v)
	case []string:
		a := pyjson.NewArray()
		a.Arr = []*pyjson.Value{}
		for _, s := range v {
			a.Arr = append(a.Arr, pyjson.NewString(s))
		}
		return a
	case []*pyjson.Value:
		a := pyjson.NewArray()
		a.Arr = append([]*pyjson.Value{}, v...)
		return a
	}
	panic("valor: tipo no soportado")
}

// detalle = HTTPException(código, detail).
func detalle(w http.ResponseWriter, codigo int, msg string) {
	escribir(w, codigo, obj("detail", msg))
}

// ---------- validación al estilo pydantic v2 (modo laxo) ----------

type errValidacion struct{ items []*pyjson.Value }

func (e *errValidacion) add(tipo string, loc []any, msg string, input *pyjson.Value) {
	l := pyjson.NewArray()
	for _, x := range loc {
		switch v := x.(type) {
		case string:
			l.Arr = append(l.Arr, pyjson.NewString(v))
		case int:
			l.Arr = append(l.Arr, pyjson.NewInt(int64(v)))
		}
	}
	if input == nil {
		input = pyjson.NewNull()
	}
	e.items = append(e.items, obj("type", tipo, "loc", l, "msg", msg, "input", input))
}

func (e *errValidacion) responder(w http.ResponseWriter) {
	escribir(w, http.StatusUnprocessableEntity, obj("detail", e.items))
}

func loc(base []any, mas ...any) []any { return append(append([]any{}, base...), mas...) }

func (e *errValidacion) str(o *pyjson.Value, k string, base []any, requerido bool, defecto string) string {
	v := o.Get(k)
	if v == nil {
		if requerido {
			e.add("missing", loc(base, k), "Field required", o)
		}
		return defecto
	}
	if v.Kind != pyjson.String {
		e.add("string_type", loc(base, k), "Input should be a valid string", v)
		return defecto
	}
	return v.S
}

func (e *errValidacion) boolean(o *pyjson.Value, k string, base []any, requerido bool, defecto bool) bool {
	v := o.Get(k)
	if v == nil {
		if requerido {
			e.add("missing", loc(base, k), "Field required", o)
		}
		return defecto
	}
	switch v.Kind {
	case pyjson.Bool:
		return v.B
	case pyjson.Number:
		if f, ok := v.Float(); ok && (f == 0 || f == 1) {
			return f == 1
		}
	case pyjson.String:
		switch strings.ToLower(v.S) {
		case "1", "true", "t", "yes", "y", "on":
			return true
		case "0", "false", "f", "no", "n", "off":
			return false
		}
	}
	e.add("bool_parsing", loc(base, k), "Input should be a valid boolean", v)
	return defecto
}

func (e *errValidacion) lista(o *pyjson.Value, k string, base []any, requerido bool) []*pyjson.Value {
	v := o.Get(k)
	if v == nil {
		if requerido {
			e.add("missing", loc(base, k), "Field required", o)
		}
		return nil
	}
	if v.Kind != pyjson.Array {
		e.add("list_type", loc(base, k), "Input should be a valid list", v)
		return nil
	}
	return v.Arr
}

func (e *errValidacion) entero(o *pyjson.Value, k string, base []any) int64 {
	v := o.Get(k)
	if v == nil {
		e.add("missing", loc(base, k), "Field required", o)
		return 0
	}
	switch v.Kind {
	case pyjson.Number:
		if v.IsInt() {
			if n, err := strconv.ParseInt(v.Num, 10, 64); err == nil {
				return n
			}
		} else if f, ok := v.Float(); ok && f == float64(int64(f)) {
			return int64(f)
		} else {
			e.add("int_from_float", loc(base, k), "Input should be a valid integer, got a number with a fractional part", v)
			return 0
		}
	case pyjson.String:
		if n, err := strconv.ParseInt(strings.TrimSpace(v.S), 10, 64); err == nil {
			return n
		}
		e.add("int_parsing", loc(base, k), "Input should be a valid integer, unable to parse string as an integer", v)
		return 0
	}
	e.add("int_type", loc(base, k), "Input should be a valid integer", v)
	return 0
}

// leerCuerpo decodifica el JSON del cuerpo; nil + respuesta 422 si no sirve.
func leerCuerpo(w http.ResponseWriter, r *http.Request) *pyjson.Value {
	datos, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	e := &errValidacion{}
	if err != nil || len(strings.TrimSpace(string(datos))) == 0 {
		e.add("missing", []any{"body"}, "Field required", nil)
		e.responder(w)
		return nil
	}
	v, err := pyjson.Parse(datos)
	if err != nil {
		e.add("json_invalid", []any{"body", 0}, "JSON decode error", pyjson.NewObject())
		e.responder(w)
		return nil
	}
	if v.Kind != pyjson.Object {
		e.add("model_attributes_type", []any{"body"}, "Input should be a valid dictionary or object to extract fields from", v)
		e.responder(w)
		return nil
	}
	return v
}

// ---------- rutas ----------

// Handler devuelve el http.Handler con las rutas de app.py. Las rutas
// desconocidas y los métodos no admitidos contestan como Starlette.
func (m *Motor) Handler() http.Handler {
	rutas := map[string]map[string]http.HandlerFunc{
		"/v1/salud":       {"GET": m.salud},
		"/v1/chat":        {"POST": m.chat},
		"/v1/feedback":    {"POST": m.feedback},
		"/v1/recuperar":   {"POST": m.recuperar},
		"/v1/sugerencias": {"POST": m.sugerenciasHTTP},
		"/v1/registro":    {"GET": m.registro},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Motor", "go")
		defer func() {
			if p := recover(); p != nil {
				log.Printf("pánico en %s: %v", r.URL.Path, p)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		metodos, ok := rutas[r.URL.Path]
		if !ok {
			// redirect_slashes de Starlette: /v1/salud/ → 307 /v1/salud
			if sin := strings.TrimRight(r.URL.Path, "/"); sin != r.URL.Path {
				if _, ok := rutas[sin]; ok {
					u := *r.URL
					u.Path = sin
					http.Redirect(w, r, u.String(), http.StatusTemporaryRedirect)
					return
				}
			}
			detalle(w, http.StatusNotFound, "Not Found")
			return
		}
		metodo := r.Method
		if metodo == http.MethodHead {
			metodo = http.MethodGet // Starlette añade HEAD a las rutas GET
		}
		h, ok := metodos[metodo]
		if !ok {
			for k := range metodos {
				w.Header().Set("Allow", k)
			}
			detalle(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		h(w, r)
	})
}

func fallo500(w http.ResponseWriter, err error) {
	log.Printf("error: %v", err)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = io.WriteString(w, "Internal Server Error")
}

func (m *Motor) salud(w http.ResponseWriter, r *http.Request) {
	escribir(w, 200, obj("ok", true, "motor", Version, "llama", m.SaludLlama()))
}

func (m *Motor) chat(w http.ResponseWriter, r *http.Request) {
	body := leerCuerpo(w, r)
	if body == nil {
		return
	}
	e := &errValidacion{}
	base := []any{"body"}
	crudos := e.lista(body, "mensajes", base, true)
	var mensajes []Mensaje
	for i, x := range crudos {
		b := loc(base, "mensajes", i)
		if x.Kind != pyjson.Object {
			e.add("model_attributes_type", b, "Input should be a valid dictionary or object to extract fields from", x)
			continue
		}
		mensajes = append(mensajes, Mensaje{Role: e.str(x, "role", b, true, ""), Content: e.str(x, "content", b, true, "")})
	}
	pedirSug := e.boolean(body, "pedir_sugerencias", base, false, true)
	memoria := e.boolean(body, "memoria", base, false, true)
	datos := body.Get("datos")
	if datos == nil {
		datos = pyjson.NewObject()
	} else if datos.Kind != pyjson.Object {
		e.add("dict_type", loc(base, "datos"), "Input should be a valid dictionary", datos)
	}
	if len(e.items) > 0 {
		e.responder(w)
		return
	}
	if len(mensajes) == 0 || mensajes[len(mensajes)-1].Role != "user" {
		detalle(w, 422, "El último mensaje debe ser del usuario.")
		return
	}
	consulta := pyStrip(mensajes[len(mensajes)-1].Content)
	if consulta == "" {
		detalle(w, 422, "Mensaje vacío.")
		return
	}
	if runas(consulta) > 2000 {
		detalle(w, 422, "Mensaje demasiado largo (máximo 2000 caracteres).")
		return
	}
	res, err := m.Chat(mensajes, pedirSug, memoria, datos)
	if err != nil {
		if el, ok := err.(ErrLlama); ok {
			detalle(w, http.StatusBadGateway, el.Error())
			return
		}
		fallo500(w, err)
		return
	}
	ctx := make([]*pyjson.Value, len(res.Contexto))
	for i, c := range res.Contexto {
		ctx[i] = obj("id", c.ID, "sim", c.Sim)
	}
	escribir(w, 200, obj("respuesta", res.Respuesta, "intencion", res.Intencion,
		"sugerencias", res.Sugerencias, "contexto_usado", ctx, "tokens_generados", res.Tokens))
}

func (m *Motor) feedback(w http.ResponseWriter, r *http.Request) {
	body := leerCuerpo(w, r)
	if body == nil {
		return
	}
	e := &errValidacion{}
	base := []any{"body"}
	pregunta := e.str(body, "pregunta", base, true, "")
	respuesta := e.str(body, "respuesta", base, true, "")
	bien := e.boolean(body, "respondio_bien", base, true, false)
	admin := e.boolean(body, "admin", base, false, false)
	if len(e.items) > 0 {
		e.responder(w)
		return
	}
	estado, err := m.Feedback(pregunta, respuesta, bien, admin)
	if err != nil {
		fallo500(w, err)
		return
	}
	escribir(w, 200, obj("ok", true, "memoria", estado))
}

func (m *Motor) recuperar(w http.ResponseWriter, r *http.Request) {
	body := leerCuerpo(w, r)
	if body == nil {
		return
	}
	e := &errValidacion{}
	base := []any{"body"}
	consulta := e.str(body, "consulta", base, true, "")
	var cands []Candidato
	for i, x := range e.lista(body, "candidatos", base, true) {
		b := loc(base, "candidatos", i)
		if x.Kind != pyjson.Object {
			e.add("model_attributes_type", b, "Input should be a valid dictionary or object to extract fields from", x)
			continue
		}
		cands = append(cands, Candidato{ID: e.entero(x, "id", b), Pregunta: e.str(x, "pregunta", b, true, ""), SQL: e.str(x, "sql", b, false, "")})
	}
	if len(e.items) > 0 {
		e.responder(w)
		return
	}
	golden, err := m.Recuperar(consulta, cands)
	if err != nil {
		fallo500(w, err)
		return
	}
	lista := make([]*pyjson.Value, len(golden))
	for i, g := range golden {
		lista[i] = obj("id", g.ID, "pregunta", g.Pregunta, "sql", g.SQL, "sim", g.Sim)
	}
	escribir(w, 200, obj("golden", lista))
}

func (m *Motor) sugerenciasHTTP(w http.ResponseWriter, r *http.Request) {
	body := leerCuerpo(w, r)
	if body == nil {
		return
	}
	e := &errValidacion{}
	base := []any{"body"}
	consulta := e.str(body, "consulta", base, false, "")
	var usadas []string
	for i, x := range e.lista(body, "usadas", base, false) {
		if x.Kind != pyjson.String {
			e.add("string_type", loc(base, "usadas", i), "Input should be a valid string", x)
			continue
		}
		usadas = append(usadas, x.S)
	}
	if len(e.items) > 0 {
		e.responder(w)
		return
	}
	sug, err := m.Sugerencias(consulta, usadas)
	if err != nil {
		fallo500(w, err)
		return
	}
	escribir(w, 200, obj("sugerencias", sug))
}

func (m *Motor) registro(w http.ResponseWriter, r *http.Request) {
	inter, props, orden, filtro := m.Registro()
	li := make([]*pyjson.Value, len(inter))
	for i, x := range inter {
		li[i] = obj("cuando", x.Cuando, "pregunta", x.Pregunta, "respuesta", x.Respuesta,
			"intencion", x.Intencion, "claves", x.Claves, "tokens", x.Tokens)
	}
	lp := make([]*pyjson.Value, len(props))
	for i, p := range props {
		lp[i] = obj("pregunta", p.Pregunta, "respuesta", p.Respuesta, "cuando", p.Cuando)
	}
	f := pyjson.NewObject()
	for _, k := range orden {
		f.Set(k, obj("+1", filtro[k].mas, "-1", filtro[k].menos))
	}
	escribir(w, 200, obj("interacciones", li, "propuestas", lp, "filtro", f))
}
