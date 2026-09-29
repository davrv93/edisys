// Package motor es el puerto a Go de motor/app.py (EDISYS · motor conversacional).
// Cada función lleva el nombre de su par en Python para poder auditarlas lado a lado.
package motor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"edisys/motor-go/internal/pyjson"
)

// ---------- configuración (mismos valores que app.py) ----------

const (
	Version          = "1.1.0" // la que publica /v1/salud: el contrato no cambia
	Recuperados      = 2       // RECUPERADOS
	UmbralFAQ        = 0.55    // UMBRAL_FAQ
	UmbralMem        = 0.80    // UMBRAL_MEM
	UmbralAltoSim    = 0.93    // UMBRAL_ALTO_SIM
	UmbralAltoVeces  = 2       // UMBRAL_ALTO_VECES
	MaxCorrecciones  = 50      // MAX_CORRECCIONES
	PrefijoQ         = "query: "
	PrefijoP         = "passage: "
	MaxHistorialChar = 6000 // MAX_HISTORIAL_CHARS
	SugerenciasN     = 3    // SUGERENCIAS_N
	MaxLog           = 100
	MaxPropuestas    = 50
	GoldenK          = 3
	GoldenMin        = 0.45
	GoldenPesoJacc   = 0.03
	MaxTokensLlama   = 280
	Temperatura      = 0.3
)

// System es SYSTEM de app.py, literal.
const System = "Eres el asistente de EDISYS, software de administración de edificios en Perú. " +
	"Responde SIEMPRE en español de Perú, con tuteo, breve (máximo 6 líneas). " +
	"Dinero como «S/ 4.800,00»: punto de miles y coma decimal. " +
	"Usa SOLO los datos del contexto y la conversación; si algo no está, di " +
	"«No tengo ese dato, escríbele a la administración» y nada más. " +
	"Nunca inventes cifras, fechas ni nombres. Nunca muestres DNI ni teléfonos de terceros."

// BancoSugerencias = BANCO_SUGERENCIAS; SugerenciasInicio = SUGERENCIAS_INICIO.
var BancoSugerencias = []string{
	"¿Cuánto debo?",
	"¿Mi recibo de este mes ya está pagado?",
	"¿Cómo pago por Yape?",
	"Quiero reservar la parrilla",
	"¿Qué horarios tiene la piscina?",
	"¿Cuál es el aforo de la piscina?",
	"Hay una fuga de agua en mi piso",
	"¿Cuál es la morosidad del edificio?",
	"¿Cuánto hay en la cuenta del banco?",
	"Explícame el reparto de medidores",
}
var SugerenciasInicio = []string{"Hola", "¿Cuánto debo?", "Quiero reservar la parrilla"}

// Embebedor calcula vectores e5 normalizados. Recibe los lotes tal cual los
// armaba Python (importa: ver e5.Embeber).
type Embebedor interface {
	Embeber(textos []string) ([][]float32, error)
}

// ---------- estado ----------

// Interaccion = entrada de _log.
type Interaccion struct {
	Cuando, Pregunta, Respuesta, Intencion, Claves string
	Tokens                                         *pyjson.Value
}

// Propuesta = entrada de _propuestas.
type Propuesta struct{ Pregunta, Respuesta, Cuando string }

type votos struct{ mas, menos int }

type claveGolden struct {
	id    int64
	texto string
}

// Motor guarda el estado en memoria (registro, filtro, propuestas, cachés) y
// el índice en archivos, como app.py.
type Motor struct {
	Indice   string // directorio de faq.json y memoria.json
	LlamaURL string
	Cliente  *salidaHTTP

	embMu  sync.Mutex
	emb    Embebedor
	cargar func() (Embebedor, error)

	archivos sync.Mutex // serializa lectura-modificación-escritura del índice

	candado    sync.Mutex
	log        []Interaccion
	filtroOrd  []string
	filtro     map[string]*votos
	propuestas []Propuesta
	vecGolden  map[claveGolden][]float32
	vecBanco   [][]float32

	Ahora func() time.Time
}

// Nuevo crea el motor; cargar se llama la primera vez que hace falta e5 (y se
// reintenta si falla), igual que _cargar_e5.
func Nuevo(indice, llama string, cargar func() (Embebedor, error)) *Motor {
	return &Motor{
		Indice: indice, LlamaURL: strings.TrimRight(llama, "/"), cargar: cargar,
		Cliente:   nuevoCliente(),
		filtro:    map[string]*votos{},
		vecGolden: map[claveGolden][]float32{},
		Ahora:     time.Now,
	}
}

func (m *Motor) embebedor() (Embebedor, error) {
	m.embMu.Lock()
	defer m.embMu.Unlock()
	if m.emb == nil {
		e, err := m.cargar()
		if err != nil {
			return nil, err
		}
		m.emb = e
	}
	return m.emb, nil
}

// Precargar fuerza la carga de e5 (arranque en caliente).
func (m *Motor) Precargar() error { _, err := m.embebedor(); return err }

func (m *Motor) e5(textos []string) ([][]float32, error) {
	e, err := m.embebedor()
	if err != nil {
		return nil, err
	}
	return e.Embeber(textos)
}

func (m *Motor) e5uno(texto string) ([]float32, error) {
	v, err := m.e5([]string{texto})
	if err != nil {
		return nil, err
	}
	return v[0], nil
}

func (m *Motor) cuando() string {
	return m.Ahora().UTC().Format("2006-01-02T15:04:05") + "+00:00"
}

// dot = np.dot de dos float32 (se acumula en float64 y se redondea a float32).
func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		if i >= len(b) {
			break
		}
		s += float64(a[i]) * float64(b[i])
	}
	return float64(float32(s))
}

// round3 = round(x, 3) de Python (redondeo correcto sobre el decimal exacto).
func round3(x float64) float64 {
	f, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'f', 3, 64), 64)
	return f
}

// ---------- archivos del índice ----------

// Item es un fragmento de FAQ o de memoria con su vector ya decodificado.
type Item struct {
	V   *pyjson.Value
	Vec []float32
}

func (it *Item) Texto() (string, error) {
	t := it.V.Get("texto")
	if t == nil {
		return "", errors.New("KeyError: 'texto'")
	}
	return t.PyStr(), nil
}

func cargarJSON(p string) (*pyjson.Value, error) {
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return pyjson.NewArray(), nil
	}
	if err != nil {
		return nil, err
	}
	return pyjson.Parse(data)
}

// guardarJSON = _guardar_json: json.dumps(indent=1, ensure_ascii=False), sin
// salto final. Escribe a un temporal y renombra (Python truncaba en el sitio).
func guardarJSON(p string, v *pyjson.Value) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	datos := []byte(pyjson.DumpsIndent(v, 1))
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, datos, 0o644); err != nil {
		return os.WriteFile(p, datos, 0o644)
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return os.WriteFile(p, datos, 0o644)
	}
	return nil
}

func items(lista *pyjson.Value) ([]*Item, error) {
	if lista.Kind != pyjson.Array {
		return nil, errors.New("el índice no es una lista")
	}
	fuera := make([]*Item, 0, len(lista.Arr))
	for _, x := range lista.Arr {
		if x.Kind != pyjson.Object {
			return nil, errors.New("entrada del índice que no es objeto")
		}
		it := &Item{V: x}
		if v := x.Get("vector"); v != nil && v.Kind == pyjson.Array && len(v.Arr) > 0 {
			it.Vec = make([]float32, len(v.Arr))
			for i, n := range v.Arr {
				f, _ := n.Float()
				it.Vec[i] = float32(f)
			}
		}
		fuera = append(fuera, it)
	}
	return fuera, nil
}

// indexar = _indexar: completa los vectores que falten en FAQ y memoria (en UN
// lote, FAQ primero) y, si faltaba alguno, reescribe los dos archivos.
func (m *Motor) indexar() (faq, memoria []*Item, err error) {
	m.archivos.Lock()
	defer m.archivos.Unlock()
	return m.indexarSinCandado()
}

func (m *Motor) indexarSinCandado() (faq, memoria []*Item, err error) {
	pf, pm := filepath.Join(m.Indice, "faq.json"), filepath.Join(m.Indice, "memoria.json")
	vf, err := cargarJSON(pf)
	if err != nil {
		return nil, nil, err
	}
	vm, err := cargarJSON(pm)
	if err != nil {
		return nil, nil, err
	}
	if faq, err = items(vf); err != nil {
		return nil, nil, err
	}
	if memoria, err = items(vm); err != nil {
		return nil, nil, err
	}
	var faltan []*Item
	for _, lista := range [][]*Item{faq, memoria} {
		for _, x := range lista {
			if !x.V.Get("vector").Truthy() {
				faltan = append(faltan, x)
			}
		}
	}
	if len(faltan) == 0 {
		return faq, memoria, nil
	}
	textos := make([]string, len(faltan))
	for i, x := range faltan {
		t, err := x.Texto()
		if err != nil {
			return nil, nil, err
		}
		textos[i] = PrefijoP + t
	}
	vecs, err := m.e5(textos)
	if err != nil {
		return nil, nil, err
	}
	for i, x := range faltan {
		x.Vec = vecs[i]
		arr := make([]*pyjson.Value, len(vecs[i]))
		for j, f := range vecs[i] {
			arr[j] = pyjson.NewFloat(float64(f)) // v.tolist(): float32 → float de Python
		}
		x.V.Set("vector", pyjson.NewArray(arr...))
	}
	if err := guardarJSON(pf, vf); err != nil {
		return nil, nil, err
	}
	if err := guardarJSON(pm, vm); err != nil {
		return nil, nil, err
	}
	return faq, memoria, nil
}

type par struct {
	sim float64
	it  *Item
}

// buscar = _buscar con el vector de la consulta ya calculado (es determinista:
// Python lo recalculaba en cada llamada con el mismo resultado).
func buscar(q []float32, banco []*Item, k int, umbral float64) []par {
	var pares []par
	for _, x := range banco {
		if len(x.Vec) == 0 {
			continue
		}
		if s := dot(q, x.Vec); s >= umbral {
			pares = append(pares, par{s, x})
		}
	}
	sort.SliceStable(pares, func(i, j int) bool { return pares[i].sim > pares[j].sim })
	if len(pares) > k {
		pares = pares[:k]
	}
	return pares
}

// ---------- KTO-lite (F6) ----------

func claveFiltro(ids []string) string {
	s := append([]string(nil), ids...)
	sort.Strings(s)
	return strings.Join(s, "|")
}

func idsDe(frag []*Item) []string {
	ids := make([]string, len(frag))
	for i, f := range frag {
		if id := f.V.Get("id"); id != nil {
			ids[i] = id.PyStr()
		} // f.get("id", "") → "" si falta
	}
	return ids
}

// pyIsSpace = str.isspace() (y \s de re en modo Unicode).
func pyIsSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

func pyStrip(s string) string { return strings.TrimFunc(s, pyIsSpace) }

// pySplitlines = str.splitlines().
func pySplitlines(s string) []string {
	var fuera []string
	ini := 0
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '\n', '\r', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
			fuera = append(fuera, string(rs[ini:i]))
			if rs[i] == '\r' && i+1 < len(rs) && rs[i+1] == '\n' {
				i++
			}
			ini = i + 1
		}
	}
	if ini < len(rs) {
		fuera = append(fuera, string(rs[ini:]))
	}
	return fuera
}

// pedazos = _pedazos.
func pedazos(linea string) []string {
	baja := strings.ToLower(linea)
	partes := strings.FieldsFunc(baja, func(r rune) bool {
		return pyIsSpace(r) || r == ',' || r == ';' || r == ':'
	})
	var palabras []string
	for _, p := range partes {
		if utf8.RuneCountInString(p) >= 6 {
			palabras = append(palabras, p)
		}
	}
	if len(palabras) == 0 {
		return []string{pyStrip(baja)}
	}
	return palabras
}

// aplicarFiltro = _aplicar_filtro.
func (m *Motor) aplicarFiltro(texto string, frag []*Item) (string, error) {
	m.candado.Lock()
	var mas, menos int
	if v := m.filtro[claveFiltro(idsDe(frag))]; v != nil {
		mas, menos = v.mas, v.menos
	}
	m.candado.Unlock()
	if menos <= mas {
		return texto, nil
	}
	textos := make([]string, len(frag))
	for i, f := range frag {
		t, err := f.Texto()
		if err != nil {
			return "", err
		}
		textos[i] = t
	}
	fuente := strings.ToLower(strings.Join(textos, " "))
	var lineas []string
	for _, ln := range pySplitlines(texto) {
		coincide := false
		for _, p := range pedazos(ln) {
			if strings.Contains(fuente, p) {
				coincide = true
				break
			}
		}
		if !coincide || utf8.RuneCountInString(pyStrip(ln)) < 8 {
			lineas = append(lineas, ln)
		}
	}
	if s := pyStrip(strings.Join(lineas, "\n")); s != "" {
		return s, nil
	}
	return texto, nil
}

// votarFiltro = _votar_filtro.
func (m *Motor) votarFiltro(ids []string, valor int) {
	m.candado.Lock()
	defer m.candado.Unlock()
	k := claveFiltro(ids)
	v := m.filtro[k]
	if v == nil {
		v = &votos{}
		m.filtro[k] = v
		m.filtroOrd = append(m.filtroOrd, k)
	}
	if valor > 0 {
		v.mas++
	} else {
		v.menos++
	}
}

// ---------- sugerencias (F7) ----------

// sugerencias = _sugerencias. El banco se vectoriza en un lote (como Python) y
// se guarda: es fijo, así que el resultado es el mismo que recalcularlo.
func (m *Motor) sugerencias(consulta string, q []float32, usadas []string) ([]string, error) {
	if consulta == "" {
		return append([]string(nil), SugerenciasInicio...), nil
	}
	m.candado.Lock()
	banco := m.vecBanco
	m.candado.Unlock()
	if banco == nil {
		textos := make([]string, len(BancoSugerencias))
		for i, s := range BancoSugerencias {
			textos[i] = PrefijoP + s
		}
		v, err := m.e5(textos)
		if err != nil {
			return nil, err
		}
		m.candado.Lock()
		m.vecBanco, banco = v, v
		m.candado.Unlock()
	}
	if q == nil {
		var err error
		if q, err = m.e5uno(PrefijoQ + consulta); err != nil {
			return nil, err
		}
	}
	sims := make([]float64, len(banco))
	for i := range banco {
		sims[i] = dot(banco[i], q)
	}
	orden := make([]int, len(banco))
	for i := range orden {
		orden[i] = i
	}
	sort.SliceStable(orden, func(a, b int) bool { return sims[orden[a]] > sims[orden[b]] })
	fuera := map[string]bool{}
	for _, u := range usadas {
		fuera[strings.ToLower(pyStrip(u))] = true
	}
	salida := []string{}
	for _, i := range orden {
		t := BancoSugerencias[i]
		if !fuera[strings.ToLower(t)] {
			salida = append(salida, t)
		}
		if len(salida) >= SugerenciasN {
			break
		}
	}
	return salida, nil
}

// ---------- memoria de correcciones (F5) ----------

// memoriaBuena = _memoria_buena.
func (m *Motor) memoriaBuena(q []float32) ([]*Item, error) {
	_, memoria, err := m.indexar()
	if err != nil {
		return nil, err
	}
	var confirmadas []*Item
	for _, x := range memoria {
		if x.V.Get("confirmada").Truthy() {
			confirmadas = append(confirmadas, x)
		}
	}
	var fuera []*Item
	for _, p := range buscar(q, confirmadas, 1, UmbralMem) {
		fuera = append(fuera, p.it)
	}
	return fuera, nil
}

// guardarCorreccion = _guardar_correccion seguido del _indexar() que hacía
// /v1/feedback (completa el vector de la nueva corrección).
func (m *Motor) guardarCorreccion(pregunta, respuesta string, confirmada bool) error {
	m.archivos.Lock()
	defer m.archivos.Unlock()
	_, memoria, err := m.indexarSinCandado()
	if err != nil {
		return err
	}
	baja := strings.ToLower(pyStrip(pregunta))
	nueva := pyjson.NewArray()
	nueva.Arr = []*pyjson.Value{}
	for _, x := range memoria {
		t, err := x.Texto()
		if err != nil {
			return err
		}
		if strings.ToLower(pyStrip(t)) != baja {
			nueva.Arr = append(nueva.Arr, x.V)
		}
	}
	o := pyjson.NewObject()
	o.Set("texto", pyjson.NewString(pyStrip(pregunta)))
	o.Set("respuesta", pyjson.NewString(pyStrip(respuesta)))
	o.Set("confirmada", pyjson.NewBool(confirmada))
	o.Set("cuando", pyjson.NewString(m.cuando()))
	nueva.Arr = append(nueva.Arr, o)
	if len(nueva.Arr) > MaxCorrecciones {
		nueva.Arr = nueva.Arr[len(nueva.Arr)-MaxCorrecciones:]
	}
	if err := guardarJSON(filepath.Join(m.Indice, "memoria.json"), nueva); err != nil {
		return err
	}
	_, _, err = m.indexarSinCandado()
	return err
}

// ---------- golden (Go trae candidatos, el motor ordena por e5) ----------

// Candidato = GoldenCandidato.
type Candidato struct {
	ID       int64
	Pregunta string
	SQL      string
}

// Golden es una fila de la respuesta de /v1/recuperar.
type Golden struct {
	ID       int64
	Pregunta string
	SQL      string
	Sim      float64
}

func (m *Motor) vecGoldenDe(id int64, texto string) ([]float32, error) {
	k := claveGolden{id, texto}
	m.candado.Lock()
	v := m.vecGolden[k]
	m.candado.Unlock()
	if v != nil {
		return v, nil
	}
	v, err := m.e5uno(PrefijoP + texto)
	if err != nil {
		return nil, err
	}
	m.candado.Lock()
	m.vecGolden[k] = v
	m.candado.Unlock()
	return v, nil
}

var parar = map[string]bool{
	"que": true, "qué": true, "cuánto": true, "cuántos": true, "cuántas": true, "cuál": true,
	"cuáles": true, "los": true, "las": true, "del": true, "por": true, "para": true, "con": true,
	"este": true, "esta": true, "hay": true, "se": true, "de": true, "en": true, "el": true,
	"la": true, "un": true, "una": true, "y": true, "o": true, "es": true, "son": true, "va": true,
}

var reToken = regexp.MustCompile(`[a-z0-9]+`)

// jaccard = _jaccard (solo [a-z0-9]: las vocales con tilde parten la palabra,
// igual que en Python; por eso las stopwords con tilde nunca coinciden).
func jaccard(a, b string) float64 {
	norm := func(t string) map[string]bool {
		s := map[string]bool{}
		for _, w := range reToken.FindAllString(strings.ToLower(t), -1) {
			if !parar[w] {
				s[w] = true
			}
		}
		return s
	}
	A, B := norm(a), norm(b)
	if len(A) == 0 || len(B) == 0 {
		return 0
	}
	inter := 0
	for w := range A {
		if B[w] {
			inter++
		}
	}
	return float64(inter) / float64(len(A)+len(B)-inter)
}

// recuperarGolden = _recuperar_golden.
func (m *Motor) recuperarGolden(consulta string, cands []Candidato) ([]Golden, error) {
	if len(cands) == 0 {
		return []Golden{}, nil
	}
	q, err := m.e5uno(PrefijoQ + consulta)
	if err != nil {
		return nil, err
	}
	type trio struct {
		s, j float64
		g    Candidato
	}
	pares := make([]trio, len(cands))
	for i, g := range cands {
		v, err := m.vecGoldenDe(g.ID, g.Pregunta)
		if err != nil {
			return nil, err
		}
		pares[i] = trio{dot(q, v), jaccard(consulta, g.Pregunta), g}
	}
	sort.SliceStable(pares, func(a, b int) bool {
		return pares[a].s+GoldenPesoJacc*pares[a].j > pares[b].s+GoldenPesoJacc*pares[b].j
	})
	if len(pares) > GoldenK {
		pares = pares[:GoldenK]
	}
	fuera := []Golden{}
	for _, p := range pares {
		if p.s >= GoldenMin {
			fuera = append(fuera, Golden{p.g.ID, p.g.Pregunta, p.g.SQL, round3(p.s)})
		}
	}
	return fuera, nil
}

// ---------- ensamblado del prompt ----------

// Mensaje = Mensaje de app.py.
type Mensaje struct{ Role, Content string }

func runas(s string) int { return utf8.RuneCountInString(s) }

func cortarRunas(s string, n int) string {
	i := 0
	for p := range s {
		if i == n {
			return s[:p]
		}
		i++
	}
	return s
}

// ensamblar = _ensamblar.
func ensamblar(mensajes []Mensaje, frag, mem []*Item, datos *pyjson.Value) ([]Mensaje, error) {
	partes := []string{System}
	if len(frag) > 0 {
		t := make([]string, len(frag))
		for i, f := range frag {
			x, err := f.Texto()
			if err != nil {
				return nil, err
			}
			t[i] = x
		}
		partes = append(partes, "Contexto del edificio (puede estar vacío o no servir):\n"+strings.Join(t, "\n---\n"))
	}
	if len(mem) > 0 {
		t := make([]string, len(mem))
		for i, x := range mem {
			texto, err := x.Texto()
			if err != nil {
				return nil, err
			}
			r := x.V.Get("respuesta")
			if r == nil {
				return nil, errors.New("KeyError: 'respuesta'")
			}
			t[i] = fmt.Sprintf("P: %s\nR: %s", texto, r.PyStr())
		}
		partes = append(partes, "Correcciones aprendidas de la administración:\n"+strings.Join(t, "\n"))
	}
	if filas := datos.Get("filas"); filas.Truthy() {
		partes = append(partes, "Datos reales ya calculados para esta pregunta (usa SOLO estos números, en soles con coma decimal; no menciones unidades ni montos que no estén aquí):\n"+
			cortarRunas(pyjson.Dumps(filas), 1800))
	} else if ej := datos.Get("golden"); ej.Truthy() {
		if ej.Kind != pyjson.Array {
			return nil, errors.New("TypeError: datos.golden no es una lista")
		}
		t := make([]string, len(ej.Arr))
		for i, g := range ej.Arr {
			p := g.Get("pregunta")
			if p == nil {
				return nil, errors.New("KeyError: 'pregunta'")
			}
			t[i] = "P: " + p.PyStr()
		}
		partes = append(partes, "Ejemplos de preguntas que SÍ sabes responder (con su consulta interna):\n"+
			strings.Join(t, "\n")+
			"\nSi la pregunta del usuario se parece a alguna, respóndela con datos del contexto o di que la administración la ve en la app.")
	}
	fuera := 0
	for _, p := range partes { // no cuenta los "\n\n" del join, igual que Python
		fuera += runas(p)
	}
	var historial []Mensaje
	for i := len(mensajes) - 2; i >= 0; i-- {
		n := runas(mensajes[i].Content)
		if fuera+n > MaxHistorialChar {
			break
		}
		historial = append([]Mensaje{mensajes[i]}, historial...)
		fuera += n
	}
	salida := []Mensaje{{Role: "system", Content: strings.Join(partes, "\n\n")}}
	salida = append(salida, historial...)
	salida = append(salida, Mensaje{Role: "user", Content: mensajes[len(mensajes)-1].Content})
	return salida, nil
}

// ---------- chat ----------

// RespuestaChat = cuerpo de /v1/chat.
type RespuestaChat struct {
	Respuesta, Intencion string
	Sugerencias          []string
	Contexto             []ContextoUsado
	Tokens               *pyjson.Value
}

// ContextoUsado = elemento de contexto_usado.
type ContextoUsado struct {
	ID  *pyjson.Value
	Sim float64
}

// ErrLlama marca el fallo de llama-server (→ 502).
type ErrLlama struct{ Causa error }

func (e ErrLlama) Error() string { return "llama-server no respondió: " + e.Causa.Error() }

// Chat = chat() de app.py sin la validación (la hace el handler).
func (m *Motor) Chat(mensajes []Mensaje, pedirSug, usarMemoria bool, datos *pyjson.Value) (*RespuestaChat, error) {
	consulta := pyStrip(mensajes[len(mensajes)-1].Content)
	faq, _, err := m.indexar()
	if err != nil {
		return nil, err
	}
	q, err := m.e5uno(PrefijoQ + consulta)
	if err != nil {
		return nil, err
	}
	var frag []*Item
	fragSim := []float64{}
	for _, p := range buscar(q, faq, Recuperados, UmbralFAQ) {
		frag = append(frag, p.it)
		fragSim = append(fragSim, p.sim)
	}
	var mem []*Item
	if usarMemoria {
		if mem, err = m.memoriaBuena(q); err != nil {
			return nil, err
		}
	}
	prompt, err := ensamblar(mensajes, frag, mem, datos)
	if err != nil {
		return nil, err
	}
	texto, tokens, err := m.llama(prompt, MaxTokensLlama)
	if err != nil {
		return nil, ErrLlama{err}
	}
	if texto, err = m.aplicarFiltro(texto, frag); err != nil {
		return nil, err
	}

	// F5: evasión aprendida — respuesta casi literal de ≥2 fragmentos → el primero.
	if len(frag) > 0 {
		vr, err := m.e5uno(PrefijoP + texto)
		if err != nil {
			return nil, err
		}
		similares := 0
		for _, f := range frag {
			if dot(vr, f.Vec) > UmbralAltoSim {
				similares++
			}
		}
		if similares >= UmbralAltoVeces {
			if texto, err = frag[0].Texto(); err != nil {
				return nil, err
			}
		}
	}

	sug := []string{}
	if pedirSug {
		usadas := []string{}
		for _, x := range mensajes {
			if x.Role == "user" {
				usadas = append(usadas, x.Content)
			}
		}
		if sug, err = m.sugerencias(consulta, q, usadas); err != nil {
			return nil, err
		}
	}
	intencion := "conversa"
	if datos.Get("filas").Truthy() {
		intencion = "golden"
	} else if len(frag) > 0 {
		intencion = "falla"
	}

	m.candado.Lock()
	m.log = append(m.log, Interaccion{
		Cuando: m.cuando(), Pregunta: consulta, Respuesta: cortarRunas(texto, 400),
		Intencion: intencion, Claves: claveFiltro(idsDe(frag)), Tokens: tokens,
	})
	if len(m.log) > MaxLog {
		m.log = append([]Interaccion(nil), m.log[len(m.log)-MaxLog:]...)
	}
	m.candado.Unlock()

	ctx := []ContextoUsado{}
	for i, f := range frag {
		id := f.V.Get("id")
		if id == nil {
			id = pyjson.NewNull()
		}
		ctx = append(ctx, ContextoUsado{ID: id, Sim: round3(fragSim[i])})
	}
	return &RespuestaChat{Respuesta: texto, Intencion: intencion, Sugerencias: sug, Contexto: ctx, Tokens: tokens}, nil
}

// ---------- feedback ----------

// Feedback = feedback() de app.py; devuelve el valor de "memoria".
func (m *Motor) Feedback(pregunta, respuesta string, bien, admin bool) (string, error) {
	if admin && bien && pregunta != "" && respuesta != "" {
		if err := m.guardarCorreccion(pregunta, respuesta, true); err != nil {
			return "", err
		}
		return "confirmada", nil
	}
	if !bien {
		m.candado.Lock()
		var reciente *Interaccion
		for i := len(m.log) - 1; i >= 0; i-- {
			if m.log[i].Pregunta == pregunta {
				reciente = &m.log[i]
				break
			}
		}
		var claves string
		if reciente != nil {
			claves = reciente.Claves
		}
		m.candado.Unlock()
		if claves != "" {
			var ids []string
			for _, i := range strings.Split(claves, "|") {
				if i != "" {
					ids = append(ids, i)
				}
			}
			m.votarFiltro(ids, -1)
			m.candado.Lock()
			m.propuestas = append(m.propuestas, Propuesta{pregunta, respuesta, m.cuando()})
			if len(m.propuestas) > MaxPropuestas {
				m.propuestas = append([]Propuesta(nil), m.propuestas[len(m.propuestas)-MaxPropuestas:]...)
			}
			m.candado.Unlock()
		}
		return "propuesta_pendiente", nil
	}
	m.votarFiltro([]string{""}, +1) // 👍 sin contexto: refuerzo genérico
	return "reforzado", nil
}

// Sugerencias = /v1/sugerencias.
func (m *Motor) Sugerencias(consulta string, usadas []string) ([]string, error) {
	return m.sugerencias(pyStrip(consulta), nil, usadas)
}

// Recuperar = /v1/recuperar.
func (m *Motor) Recuperar(consulta string, cands []Candidato) ([]Golden, error) {
	return m.recuperarGolden(pyStrip(consulta), cands)
}

// Registro devuelve copias del estado en memoria.
func (m *Motor) Registro() ([]Interaccion, []Propuesta, []string, map[string]votos) {
	m.candado.Lock()
	defer m.candado.Unlock()
	f := make(map[string]votos, len(m.filtro))
	for k, v := range m.filtro {
		f[k] = *v
	}
	return append([]Interaccion(nil), m.log...), append([]Propuesta(nil), m.propuestas...),
		append([]string(nil), m.filtroOrd...), f
}
