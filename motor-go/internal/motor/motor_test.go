package motor

import (
	"bytes"
	"encoding/json"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"edisys/motor-go/internal/pyjson"
)

// ---------- dobles de prueba ----------

// falso: vectores fijos por texto (sin prefijo) y, si no, bolsa de palabras.
// Registra los lotes para comprobar que se agrupan como en Python.
type falso struct {
	mu    sync.Mutex
	fijo  map[string][]float32
	lotes [][]string
}

func vec(comp ...float64) []float32 {
	v := make([]float32, 384)
	var n float64
	for _, c := range comp {
		n += c * c
	}
	for i, c := range comp {
		v[i] = float32(c / math.Sqrt(n))
	}
	return v
}

var rePal = regexp.MustCompile(`[\p{L}\p{N}]+`)

func (f *falso) Embeber(ts []string) ([][]float32, error) {
	f.mu.Lock()
	f.lotes = append(f.lotes, append([]string(nil), ts...))
	f.mu.Unlock()
	fuera := make([][]float32, len(ts))
	for i, t := range ts {
		base := strings.TrimPrefix(strings.TrimPrefix(t, PrefijoQ), PrefijoP)
		if v, ok := f.fijo[base]; ok {
			fuera[i] = v
			continue
		}
		v := make([]float32, 384)
		for _, w := range rePal.FindAllString(strings.ToLower(base), -1) {
			h := fnv.New32a()
			h.Write([]byte(w))
			v[10+h.Sum32()%370] += 1 // lejos de los ejes 0–9 que usan los fijos
		}
		var n float64
		for _, x := range v {
			n += float64(x * x)
		}
		for j := range v {
			v[j] = float32(float64(v[j]) / math.Max(math.Sqrt(n), 1e-9))
		}
		fuera[i] = v
	}
	return fuera, nil
}

// llamaFalso: /health y /v1/chat/completions; guarda los cuerpos recibidos.
type llamaFalso struct {
	mu        sync.Mutex
	pedidos   []map[string]any
	respuesta string
	codigo    int
	sinUsage  bool
}

func (l *llamaFalso) servidor(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chat/completions":
			var p map[string]any
			_ = json.NewDecoder(r.Body).Decode(&p)
			l.mu.Lock()
			l.pedidos = append(l.pedidos, p)
			cod, resp, sin := l.codigo, l.respuesta, l.sinUsage
			l.mu.Unlock()
			if cod != 0 {
				w.WriteHeader(cod)
				return
			}
			out := map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": resp}}}}
			if !sin {
				out["usage"] = map[string]any{"completion_tokens": 7, "prompt_tokens": 100}
			}
			_ = json.NewEncoder(w).Encode(out)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

type entorno struct {
	m     *Motor
	srv   *httptest.Server
	emb   *falso
	llama *llamaFalso
	dir   string
}

const faqSemilla = `[
 {
  "id": "f1",
  "texto": "Piscina abre de 8 a 12."
 },
 {
  "id": "f2",
  "texto": "La parrilla se reserva con 48 h."
 },
 {
  "id": "f3",
  "texto": "Yape al 987 654 321."
 }
]`

func nuevoEntorno(t *testing.T, faq string) *entorno {
	t.Helper()
	dir := t.TempDir()
	if faq != "" {
		if err := os.WriteFile(filepath.Join(dir, "faq.json"), []byte(faq), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	emb := &falso{fijo: map[string][]float32{
		"Piscina abre de 8 a 12.":          vec(1),
		"La parrilla se reserva con 48 h.": vec(0.8, 0.6),
		"Yape al 987 654 321.":             vec(0, 0, 1),
		"horario piscina":                  vec(1),
		"¿cómo pago?":                      vec(0, 0, 1),
		"algo sin relación":                vec(0, 0, 0, 0, 0, 1),
	}}
	l := &llamaFalso{respuesta: "  La piscina abre temprano.  "}
	ls := l.servidor(t)
	m := Nuevo(dir, ls.URL, func() (Embebedor, error) { return emb, nil })
	m.Ahora = func() time.Time { return time.Date(2026, 9, 29, 15, 4, 5, 0, time.FixedZone("Lima", -5*3600)) }
	s := httptest.NewServer(m.Handler())
	t.Cleanup(s.Close)
	return &entorno{m, s, emb, l, dir}
}

func (e *entorno) post(t *testing.T, ruta, cuerpo string) (int, string) {
	t.Helper()
	resp, err := http.Post(e.srv.URL+ruta, "application/json", strings.NewReader(cuerpo))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (e *entorno) get(t *testing.T, ruta string) (int, string) {
	t.Helper()
	resp, err := http.Get(e.srv.URL + ruta)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func igual(t *testing.T, que, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s:\n got: %s\nwant: %s", que, got, want)
	}
}

// ---------- /v1/salud ----------

func TestSalud(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	c, b := e.get(t, "/v1/salud")
	igual(t, "salud", b, `{"ok":true,"motor":"1.1.0","llama":{"estado":"ok"}}`)
	if c != 200 {
		t.Errorf("código %d", c)
	}
	e.m.LlamaURL = "http://127.0.0.1:1" // nadie escucha
	c, b = e.get(t, "/v1/salud")
	if c != 200 || !strings.HasPrefix(b, `{"ok":true,"motor":"1.1.0","llama":{"estado":"caido","error":"`) {
		t.Errorf("salud con llama caído: %d %s", c, b)
	}
}

// ---------- /v1/chat ----------

func TestChatContratoEIndexado(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	c, b := e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"  horario piscina "}],"pedir_sugerencias":false}`)
	if c != 200 {
		t.Fatalf("%d %s", c, b)
	}
	// Orden de claves, respuesta sin espacios, sims redondeadas como round(x,3).
	igual(t, "chat", b, `{"respuesta":"La piscina abre temprano.","intencion":"falla","sugerencias":[],"contexto_usado":[{"id":"f1","sim":1.0},{"id":"f2","sim":0.8}],"tokens_generados":7}`)

	// El primer lote e5 es el de _indexar: los 3 fragmentos juntos, con «passage: ».
	if got := e.emb.lotes[0]; len(got) != 3 || got[0] != "passage: Piscina abre de 8 a 12." {
		t.Errorf("lote de indexado: %q", got)
	}
	// faq.json reescrito en el formato de Python con los vectores añadidos al final.
	data, _ := os.ReadFile(filepath.Join(e.dir, "faq.json"))
	v, err := pyjson.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if pyjson.DumpsIndent(v, 1) != string(data) {
		t.Error("faq.json no quedó en formato json.dumps(indent=1)")
	}
	f1 := v.Arr[0]
	if strings.Join(f1.Keys, ",") != "id,texto,vector" || len(f1.Get("vector").Arr) != 384 || f1.Get("vector").Arr[0].Num != "1.0" {
		t.Errorf("vector de f1 mal guardado: %v", f1.Keys)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "memoria.json")); err != nil {
		t.Error("memoria.json no se creó (Python lo escribe junto a faq.json)")
	}

	// Lo que llegó a llama-server.
	p := e.llama.pedidos[0]
	if p["temperature"] != 0.3 || p["max_tokens"] != float64(280) || p["cache_prompt"] != true {
		t.Errorf("parámetros a llama: %v", p)
	}
	ms := p["messages"].([]any)
	sys := ms[0].(map[string]any)["content"].(string)
	if !strings.HasPrefix(sys, System+"\n\nContexto del edificio (puede estar vacío o no servir):\nPiscina abre de 8 a 12.\n---\nLa parrilla") {
		t.Errorf("system: %q", sys)
	}
	if u := ms[1].(map[string]any); u["role"] != "user" || u["content"] != "  horario piscina " {
		t.Errorf("el último mensaje va sin recortar, como en Python: %v", u)
	}

	// Segunda llamada: no reindexa (el archivo no cambia).
	antes, _ := os.ReadFile(filepath.Join(e.dir, "faq.json"))
	e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"algo sin relación"}],"pedir_sugerencias":false}`)
	despues, _ := os.ReadFile(filepath.Join(e.dir, "faq.json"))
	if !bytes.Equal(antes, despues) {
		t.Error("faq.json cambió sin fragmentos nuevos")
	}
}

func TestChatIntencionesYSinUsage(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	e.llama.sinUsage = true
	_, b := e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"algo sin relación"}],"pedir_sugerencias":false}`)
	igual(t, "conversa", b, `{"respuesta":"La piscina abre temprano.","intencion":"conversa","sugerencias":[],"contexto_usado":[],"tokens_generados":null}`)
	_, b = e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"algo sin relación"}],"pedir_sugerencias":false,"datos":{"filas":[{"x":1}]}}`)
	if !strings.Contains(b, `"intencion":"golden"`) {
		t.Errorf("con filas la intención es golden: %s", b)
	}
	_, b = e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"algo sin relación"}],"pedir_sugerencias":false,"datos":{"filas":[]}}`)
	if !strings.Contains(b, `"intencion":"conversa"`) {
		t.Errorf("filas vacías no cuentan: %s", b)
	}
}

func TestChatValidacion(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	casos := []struct{ cuerpo, want string }{
		{`{"mensajes":[]}`, `{"detail":"El último mensaje debe ser del usuario."}`},
		{`{"mensajes":[{"role":"assistant","content":"x"}]}`, `{"detail":"El último mensaje debe ser del usuario."}`},
		{`{"mensajes":[{"role":"user","content":"   "}]}`, `{"detail":"Mensaje vacío."}`},
		{`{"mensajes":[{"role":"user","content":"` + strings.Repeat("ñ", 2001) + `"}]}`, `{"detail":"Mensaje demasiado largo (máximo 2000 caracteres)."}`},
		{`{}`, `{"detail":[{"type":"missing","loc":["body","mensajes"],"msg":"Field required","input":{}}]}`},
		{`{"mensajes":[{"role":"user"}]}`, `{"detail":[{"type":"missing","loc":["body","mensajes",0,"content"],"msg":"Field required","input":{"role":"user"}}]}`},
		{`no es json`, `{"detail":[{"type":"json_invalid","loc":["body",0],"msg":"JSON decode error","input":{}}]}`},
	}
	for _, c := range casos {
		cod, b := e.post(t, "/v1/chat", c.cuerpo)
		if cod != 422 {
			t.Errorf("%s → %d", c.cuerpo[:min(len(c.cuerpo), 40)], cod)
		}
		igual(t, "422", b, c.want)
	}
	// 2000 exactos sí pasan (se cuentan caracteres, no bytes).
	if cod, _ := e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"`+strings.Repeat("ñ", 2000)+`"}],"pedir_sugerencias":false}`); cod != 200 {
		t.Errorf("2000 caracteres → %d", cod)
	}
}

func TestChatLlamaFalla502(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	e.llama.codigo = 500
	cod, b := e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"hola"}]}`)
	if cod != 502 || !strings.HasPrefix(b, `{"detail":"llama-server no respondió: HTTP Error 500`) {
		t.Errorf("%d %s", cod, b)
	}
}

func TestChatEvasionAprendida(t *testing.T) {
	e := nuevoEntorno(t, strings.Replace(faqSemilla, "La parrilla se reserva con 48 h.", "Piscina: horario de mañana.", 1))
	e.emb.fijo["Piscina: horario de mañana."] = vec(0.95, 0.312)
	e.emb.fijo["respuesta literal"] = vec(1)
	e.llama.respuesta = "respuesta literal"
	_, b := e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"horario piscina"}],"pedir_sugerencias":false}`)
	if !strings.Contains(b, `"respuesta":"Piscina abre de 8 a 12."`) {
		t.Errorf("≥2 fragmentos con sim>0,93 → responde el primero: %s", b)
	}
}

// ---------- /v1/feedback + memoria ----------

func TestFeedbackMemoriaYFiltro(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	e.emb.fijo["¿Cuánto debo?"] = vec(0, 0, 0, 1)
	e.emb.fijo["cuanto debo"] = vec(0, 0, 0, 1)
	e.emb.fijo["¿cuánto DEBO?"] = vec(0, 0, 0, 1)

	// Admin confirma → memoria.json con la corrección y su vector.
	_, b := e.post(t, "/v1/feedback", `{"pregunta":" ¿Cuánto debo? ","respuesta":"Mira la app.","respondio_bien":true,"admin":true}`)
	igual(t, "confirmada", b, `{"ok":true,"memoria":"confirmada"}`)
	data, _ := os.ReadFile(filepath.Join(e.dir, "memoria.json"))
	mem, _ := pyjson.Parse(data)
	if len(mem.Arr) != 1 || strings.Join(mem.Arr[0].Keys, ",") != "texto,respuesta,confirmada,cuando,vector" ||
		mem.Arr[0].Get("texto").S != "¿Cuánto debo?" || mem.Arr[0].Get("cuando").S != "2026-09-29T20:04:05+00:00" {
		t.Errorf("memoria.json: %s", data)
	}
	// Repetir la misma pregunta (otra caja/espacios) reemplaza, no duplica.
	e.post(t, "/v1/feedback", `{"pregunta":"¿cuánto DEBO?","respuesta":"Otra.","respondio_bien":true,"admin":true}`)
	data, _ = os.ReadFile(filepath.Join(e.dir, "memoria.json"))
	mem, _ = pyjson.Parse(data)
	if len(mem.Arr) != 1 || mem.Arr[0].Get("respuesta").S != "Otra." {
		t.Errorf("reemplazo: %s", data)
	}

	// El chat trae la corrección al prompt (sim ≥ 0,80).
	e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"cuanto debo"}],"pedir_sugerencias":false}`)
	sys := e.llama.pedidos[len(e.llama.pedidos)-1]["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "Correcciones aprendidas de la administración:\nP: ¿cuánto DEBO?\nR: Otra.") {
		t.Errorf("sin corrección en el prompt: %q", sys)
	}
	// memoria=false la omite.
	e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"cuanto debo"}],"pedir_sugerencias":false,"memoria":false}`)
	sys = e.llama.pedidos[len(e.llama.pedidos)-1]["messages"].([]any)[0].(map[string]any)["content"].(string)
	if strings.Contains(sys, "Correcciones") {
		t.Error("memoria=false no debe usar correcciones")
	}

	// 👎 tras un chat con contexto → propuesta + voto -1 a la clave de fragmentos.
	e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"horario piscina"}],"pedir_sugerencias":false}`)
	_, b = e.post(t, "/v1/feedback", `{"pregunta":"horario piscina","respuesta":"mal","respondio_bien":false}`)
	igual(t, "👎", b, `{"ok":true,"memoria":"propuesta_pendiente"}`)
	// 👎 sin chat previo: no hay propuesta.
	e.post(t, "/v1/feedback", `{"pregunta":"nunca preguntado","respuesta":"x","respondio_bien":false}`)
	// 👍 → refuerzo genérico con clave "".
	_, b = e.post(t, "/v1/feedback", `{"pregunta":"x","respuesta":"y","respondio_bien":true}`)
	igual(t, "👍", b, `{"ok":true,"memoria":"reforzado"}`)
	// admin sin respuesta: cae en refuerzo, como en Python.
	_, b = e.post(t, "/v1/feedback", `{"pregunta":"x","respuesta":"","respondio_bien":true,"admin":true}`)
	igual(t, "admin vacío", b, `{"ok":true,"memoria":"reforzado"}`)

	_, reg := e.get(t, "/v1/registro")
	var r struct {
		Interacciones []map[string]any `json:"interacciones"`
		Propuestas    []map[string]any `json:"propuestas"`
		Filtro        map[string]map[string]int
	}
	if err := json.Unmarshal([]byte(reg), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Propuestas) != 1 || r.Propuestas[0]["pregunta"] != "horario piscina" {
		t.Errorf("propuestas: %v", r.Propuestas)
	}
	if r.Filtro["f1|f2"]["-1"] != 1 || r.Filtro[""]["+1"] != 2 {
		t.Errorf("filtro: %v", r.Filtro)
	}
	if !strings.Contains(reg, `"filtro":{"f1|f2":{"+1":0,"-1":1},"":{"+1":2,"-1":0}}`) {
		t.Errorf("orden del filtro: %s", reg)
	}
	ult := r.Interacciones[len(r.Interacciones)-1]
	if ult["claves"] != "f1|f2" || ult["intencion"] != "falla" || ult["tokens"] != float64(7) || ult["cuando"] != "2026-09-29T20:04:05+00:00" {
		t.Errorf("interacción: %v", ult)
	}
	if !strings.HasPrefix(reg, `{"interacciones":[{"cuando":`) {
		t.Errorf("orden de claves del registro: %.80s", reg)
	}

	// Con el 👎, las líneas de la respuesta que citan el contexto se filtran.
	e.llama.respuesta = "Piscina abre de 8 a 12 según reglamento.\nTe ayudo en algo más, vecino"
	_, b = e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"horario piscina"}],"pedir_sugerencias":false}`)
	if !strings.Contains(b, `"respuesta":"Te ayudo en algo más, vecino"`) {
		t.Errorf("filtro KTO-lite: %s", b)
	}
}

func TestFeedbackTopeMemoria(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	for i := 0; i < MaxCorrecciones+5; i++ {
		e.post(t, "/v1/feedback", `{"pregunta":"p`+strings.Repeat("x", i)+`","respuesta":"r","respondio_bien":true,"admin":true}`)
	}
	data, _ := os.ReadFile(filepath.Join(e.dir, "memoria.json"))
	mem, _ := pyjson.Parse(data)
	if len(mem.Arr) != MaxCorrecciones || mem.Arr[0].Get("texto").S != "p"+strings.Repeat("x", 5) {
		t.Errorf("tope de %d: hay %d", MaxCorrecciones, len(mem.Arr))
	}
}

// ---------- /v1/recuperar ----------

func TestRecuperarGolden(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	q := "cuánto entró por alquiler este mes"
	e.emb.fijo[q] = vec(1)
	e.emb.fijo["recaudado por alquiler este mes"] = vec(0.9, 0.436)
	e.emb.fijo["recaudado por alquiler este año"] = vec(0.9, 0.436) // misma sim: decide Jaccard
	e.emb.fijo["deuda por unidad"] = vec(0.5, 0.866)
	e.emb.fijo["morosidad"] = vec(0.6, 0.8)
	e.emb.fijo["nada que ver"] = vec(0.3, 0.954)
	cuerpo := `{"consulta":"  ` + q + ` ","candidatos":[
		{"id":1,"pregunta":"recaudado por alquiler este año","sql":"SELECT 1"},
		{"id":2,"pregunta":"recaudado por alquiler este mes"},
		{"id":"3","pregunta":"deuda por unidad","sql":"s3"},
		{"id":4.0,"pregunta":"morosidad","sql":"s4"},
		{"id":5,"pregunta":"nada que ver","sql":"s5"}]}`
	cod, b := e.post(t, "/v1/recuperar", cuerpo)
	if cod != 200 {
		t.Fatalf("%d %s", cod, b)
	}
	// «mes» empata en e5 con «año» pero gana por Jaccard; top 3; sql por defecto "".
	igual(t, "recuperar", b, `{"golden":[{"id":2,"pregunta":"recaudado por alquiler este mes","sql":"","sim":0.9},{"id":1,"pregunta":"recaudado por alquiler este año","sql":"SELECT 1","sim":0.9},{"id":4,"pregunta":"morosidad","sql":"s4","sim":0.6}]}`)

	// El top 3 se corta ANTES del umbral 0,45: lo que quede por debajo desaparece.
	e.emb.fijo["bajo"] = vec(0.4, 0.917)
	_, b = e.post(t, "/v1/recuperar", `{"consulta":"`+q+`","candidatos":[{"id":9,"pregunta":"bajo"}]}`)
	igual(t, "umbral", b, `{"golden":[]}`)
	_, b = e.post(t, "/v1/recuperar", `{"consulta":"x","candidatos":[]}`)
	igual(t, "vacío", b, `{"golden":[]}`)
	cod, _ = e.post(t, "/v1/recuperar", `{"consulta":"x"}`)
	if cod != 422 {
		t.Errorf("sin candidatos → %d", cod)
	}
	cod, _ = e.post(t, "/v1/recuperar", `{"consulta":"x","candidatos":[{"id":1.5,"pregunta":"a"}]}`)
	if cod != 422 {
		t.Errorf("id 1.5 → %d", cod)
	}

	// Caché por (id, texto): editar el texto del golden lo re-vectoriza.
	n := len(e.emb.lotes)
	e.post(t, "/v1/recuperar", `{"consulta":"`+q+`","candidatos":[{"id":2,"pregunta":"recaudado por alquiler este mes"}]}`)
	if len(e.emb.lotes) != n+1 { // solo la consulta
		t.Errorf("golden ya vectorizado se recalculó")
	}
	e.post(t, "/v1/recuperar", `{"consulta":"`+q+`","candidatos":[{"id":2,"pregunta":"morosidad"}]}`)
	if len(e.emb.lotes) != n+3 {
		t.Errorf("golden editado no se re-vectorizó")
	}
}

// ---------- /v1/sugerencias ----------

func TestSugerencias(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	_, b := e.post(t, "/v1/sugerencias", `{}`)
	igual(t, "inicio", b, `{"sugerencias":["Hola","¿Cuánto debo?","Quiero reservar la parrilla"]}`)
	_, b = e.post(t, "/v1/sugerencias", `{"consulta":"   ","usadas":["x"]}`)
	igual(t, "inicio con espacios", b, `{"sugerencias":["Hola","¿Cuánto debo?","Quiero reservar la parrilla"]}`)

	for i, s := range BancoSugerencias {
		comp := make([]float64, 10)
		comp[i] = 1
		e.emb.fijo[s] = vec(comp...)
	}
	e.emb.fijo["piscina"] = vec(0, 0, 0, 0, 0.8, 0.6, 0.1)
	_, b = e.post(t, "/v1/sugerencias", `{"consulta":"piscina","usadas":["  ¿QUÉ HORARIOS TIENE LA PISCINA? "]}`)
	// Excluye la usada (strip + lower) y completa por orden de similaridad.
	igual(t, "orden", b, `{"sugerencias":["¿Cuál es el aforo de la piscina?","Hay una fuga de agua en mi piso","¿Cuánto debo?"]}`)
	// El banco se vectoriza en UN lote de 10 (como Python) y una sola vez.
	lotes := 0
	for _, l := range e.emb.lotes {
		if len(l) == len(BancoSugerencias) {
			lotes++
		}
	}
	e.post(t, "/v1/sugerencias", `{"consulta":"piscina"}`)
	for _, l := range e.emb.lotes[len(e.emb.lotes)-1:] {
		if len(l) == len(BancoSugerencias) {
			lotes++
		}
	}
	if lotes != 1 {
		t.Errorf("banco vectorizado %d veces", lotes)
	}
	cod, _ := e.post(t, "/v1/sugerencias", `{"usadas":[1]}`)
	if cod != 422 {
		t.Errorf("usadas no-string → %d", cod)
	}
}

func TestChatConSugerencias(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	_, b := e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"¿Cuánto debo?"},{"role":"assistant","content":"x"},{"role":"user","content":"horario piscina"}]}`)
	var r struct{ Sugerencias []string }
	_ = json.Unmarshal([]byte(b), &r)
	if len(r.Sugerencias) != 3 {
		t.Fatalf("sugerencias: %s", b)
	}
	for _, s := range r.Sugerencias {
		if s == "¿Cuánto debo?" {
			t.Error("sugirió una pregunta ya hecha por el usuario")
		}
	}
}

// ---------- rutas ----------

func TestRutas(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	c, b := e.get(t, "/no/existe")
	if c != 404 || b != `{"detail":"Not Found"}` {
		t.Errorf("404: %d %s", c, b)
	}
	c, b = e.get(t, "/v1/chat")
	if c != 405 || b != `{"detail":"Method Not Allowed"}` {
		t.Errorf("405: %d %s", c, b)
	}
	cli := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := cli.Get(e.srv.URL + "/v1/salud/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 307 || resp.Header.Get("Location") != "/v1/salud" {
		t.Errorf("redirect_slashes: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	_, b = e.get(t, "/v1/registro")
	igual(t, "registro vacío", b, `{"interacciones":[],"propuestas":[],"filtro":{}}`)
}

// ---------- índice real ----------

// El faq.json real del volumen (con vectores) se lee tal cual y no se reescribe.
func TestFaqRealCompatible(t *testing.T) {
	real, err := os.ReadFile("../../testdata/faq.json")
	if err != nil {
		t.Skip("sin testdata/faq.json")
	}
	e := nuevoEntorno(t, string(real))
	faq, mem, err := e.m.indexar()
	if err != nil {
		t.Fatal(err)
	}
	if len(faq) != 15 || len(mem) != 0 {
		t.Fatalf("faq %d, memoria %d", len(faq), len(mem))
	}
	for _, f := range faq {
		if len(f.Vec) != 384 {
			t.Errorf("%s sin vector de 384", f.V.Get("id").S)
		}
	}
	if len(e.emb.lotes) != 0 {
		t.Error("reindexó un faq.json que ya tenía todos los vectores")
	}
	// Buscar con el vector guardado de un fragmento lo trae primero con sim 1.
	q := faq[4].Vec
	p := buscar(q, faq, Recuperados, UmbralFAQ)
	if len(p) == 0 || p[0].it != faq[4] || round3(p[0].sim) != 1 {
		t.Errorf("búsqueda sobre el índice real: %+v", p)
	}
	despues, _ := os.ReadFile(filepath.Join(e.dir, "faq.json"))
	if !bytes.Equal(real, despues) {
		t.Error("faq.json real modificado")
	}
}
