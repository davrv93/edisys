package motor

// Verificación de cifras (29-09-2026). El LLM local inventa montos aunque el
// prompt lo prohíba («S/ 1.500,00» para el 402, que debe S/ 1.420,00). Aquí se
// extraen las cifras de la respuesta y se comparan, normalizadas, con las de
// las fuentes que se le entregaron (datos.filas, datos.golden, fragmentos del
// FAQ, memoria y los mensajes del usuario). Ninguna cifra sale del motor sin
// estar en ellas: si falla, se reintenta una vez con la lista de cifras
// permitidas y, si insiste, se responde una versión segura.

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"edisys/motor-go/internal/pyjson"
)

// Tipos de cifra.
const (
	TipoNumero     = "numero"
	TipoMonto      = "monto"
	TipoPorcentaje = "porcentaje"
	TipoHora       = "hora"
	TipoFecha      = "fecha"
)

// lectura es una forma de leer un número escrito: x con dec decimales,
// multiplicado por mult («4,8 mil» → x=4.8, dec=1, mult=1000).
type lectura struct {
	x    float64
	dec  int
	mult float64
}

// Cifra es un número con significado dentro de un texto.
type Cifra struct {
	Texto    string // tal cual aparece, con «S/», «%» o «mil»
	Ini, Fin int    // posición en bytes
	Tipo     string
	lecturas []lectura
	hora     [2]int // h, m
	fecha    [3]int // d, m, a (a=0 si no se escribió)
}

var reNumeroCrudo = regexp.MustCompile(`\d+(?:[.,:/-]\d+)*`)

func esLetra(r rune) bool { return unicode.IsLetter(r) }

// runaAntes/runaDespues: la runa vecina (0 si no hay).
func runaAntes(s string, i int) rune {
	if i <= 0 {
		return 0
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return r
}

func runaDespues(s string, i int) rune {
	if i >= len(s) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return r
}

// leerNumero interpreta «1.420,00», «1,420.00», «1420», «3.500», «13,1»… y
// devuelve todas las lecturas posibles (los casos ambiguos dan dos).
func leerNumero(s string) []lectura {
	puntos, comas := strings.Count(s, "."), strings.Count(s, ",")
	limpio := func(t string) float64 {
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	switch {
	case puntos == 0 && comas == 0:
		return []lectura{{limpio(s), 0, 1}}
	case puntos > 0 && comas > 0:
		// El último separador es el decimal; los otros, de miles.
		i := strings.LastIndexAny(s, ".,")
		ent := strings.NewReplacer(".", "", ",", "").Replace(s[:i])
		dec := s[i+1:]
		return []lectura{{limpio(ent + "." + dec), len(dec), 1}}
	}
	sep := "."
	if comas > 0 {
		sep = ","
	}
	partes := strings.Split(s, sep)
	if len(partes) > 2 { // varios del mismo tipo: miles
		return []lectura{{limpio(strings.Join(partes, "")), 0, 1}}
	}
	dec := partes[1]
	decimal := lectura{limpio(partes[0] + "." + dec), len(dec), 1}
	if len(dec) == 3 && partes[0] != "0" { // «1.500» / «1,420»: miles o decimal
		return []lectura{{limpio(partes[0] + dec), 0, 1}, decimal}
	}
	return []lectura{decimal}
}

// saltarEspacios avanza sobre espacios (incluido el no separable).
func saltarEspacios(s string, i int) int {
	for i < len(s) {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r != ' ' && r != ' ' && r != ' ' && r != '\t' {
			break
		}
		i += n
	}
	return i
}

func retrocederEspacios(s string, i int) int {
	for i > 0 {
		r, n := utf8.DecodeLastRuneInString(s[:i])
		if r != ' ' && r != ' ' && r != ' ' && r != '\t' {
			break
		}
		i -= n
	}
	return i
}

// palabraEn devuelve la palabra (solo letras) que empieza en i.
func palabraEn(s string, i int) string {
	j := i
	for j < len(s) {
		r, n := utf8.DecodeRuneInString(s[j:])
		if !esLetra(r) {
			break
		}
		j += n
	}
	return s[i:j]
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// ExtraerCifras devuelve las cifras con significado de un texto: montos
// (S/ 1.420,00 · S/1420 · 1,420.00 · 4,8 mil), porcentajes, horas (8:00),
// fechas (29/09, 2026-09-29) y números sueltos. No cuenta los números pegados
// a letras («e5», «F5») ni las viñetas de lista («1. », «2) »). Los rangos
// («12:00-17:00», «8-12») y correlativos («R-2026-0012») se parten por «-».
func ExtraerCifras(s string) []Cifra {
	var fuera []Cifra
	for _, loc := range reNumeroCrudo.FindAllStringIndex(s, -1) {
		ini, fin := loc[0], loc[1]
		if esLetra(runaAntes(s, ini)) {
			continue
		}
		crudo := s[ini:fin]
		if p := strings.Split(crudo, "-"); len(p) == 3 && len(p[0]) == 4 && len(p[1]) == 2 && len(p[2]) == 2 &&
			!strings.ContainsAny(crudo, ".,:/") {
			fuera = append(fuera, Cifra{Texto: crudo, Ini: ini, Fin: fin, Tipo: TipoFecha,
				fecha: [3]int{atoi(p[2]), atoi(p[1]), atoi(p[0])}})
			continue
		}
		off := 0
		for _, p := range strings.Split(crudo, "-") {
			if p != "" {
				fuera = append(fuera, pieza(s, ini+off, ini+off+len(p))...)
			}
			off += len(p) + 1
		}
	}
	return fuera
}

// pieza clasifica un trozo sin «-»: hora, fecha dd/mm[/aa] o número.
func pieza(s string, ini, fin int) []Cifra {
	crudo := s[ini:fin]
	if strings.Contains(crudo, ":") {
		p := strings.Split(crudo, ":")
		if (len(p) == 2 || len(p) == 3) && len(p[0]) <= 2 && len(p[1]) == 2 && !strings.ContainsAny(crudo, "/.,") {
			return []Cifra{{Texto: crudo, Ini: ini, Fin: fin, Tipo: TipoHora, hora: [2]int{atoi(p[0]), atoi(p[1])}}}
		}
		return partir(s, ini, crudo, ":")
	}
	if strings.Contains(crudo, "/") {
		p := strings.Split(crudo, "/")
		if (len(p) == 2 || len(p) == 3) && len(p[0]) <= 2 && len(p[1]) <= 2 && !strings.ContainsAny(crudo, ".,") {
			c := Cifra{Texto: crudo, Ini: ini, Fin: fin, Tipo: TipoFecha, fecha: [3]int{atoi(p[0]), atoi(p[1]), 0}}
			if len(p) == 3 {
				c.fecha[2] = normAnio(atoi(p[2]))
			}
			return []Cifra{c}
		}
		return partir(s, ini, crudo, "/")
	}
	return numeroEn(s, ini, fin)
}

func partir(s string, base int, crudo, sep string) []Cifra {
	var fuera []Cifra
	off := 0
	for _, p := range strings.Split(crudo, sep) {
		if p != "" {
			fuera = append(fuera, numeroEn(s, base+off, base+off+len(p))...)
		}
		off += len(p) + len(sep)
	}
	return fuera
}

// numeroEn lee el número s[ini:fin] con su contexto: «S/» delante, «%»,
// «mil»/«millones» o «soles» detrás. Devuelve 0 o 1 cifras.
func numeroEn(s string, ini, fin int) []Cifra {
	crudo := s[ini:fin]
	// Viñeta de lista: «1. » / «2) » al principio de línea.
	if !strings.ContainsAny(crudo, ".,") {
		antes := retrocederEspacios(s, ini)
		if (antes == 0 || s[antes-1] == '\n') && fin < len(s) && (s[fin] == '.' || s[fin] == ')') &&
			(fin+1 == len(s) || s[fin+1] == ' ') && len(crudo) <= 2 {
			return nil
		}
	}
	c := Cifra{Ini: ini, Fin: fin, Tipo: TipoNumero, lecturas: leerNumero(crudo)}
	// Prefijo «S/», «S/.», «s/» (con o sin espacio).
	a := retrocederEspacios(s, ini)
	switch {
	case a >= 3 && strings.EqualFold(s[a-3:a], "S/."):
		c.Ini, c.Tipo = a-3, TipoMonto
	case a >= 2 && strings.EqualFold(s[a-2:a], "S/"):
		c.Ini, c.Tipo = a-2, TipoMonto
	}
	d := saltarEspacios(s, fin)
	if r := runaDespues(s, d); r == '%' {
		c.Fin, c.Tipo = d+1, TipoPorcentaje
	} else if esLetra(r) {
		p := strings.ToLower(palabraEn(s, d))
		switch p {
		case "mil", "k":
			c.Fin = d + len(p)
			for i := range c.lecturas {
				c.lecturas[i].mult = 1000
			}
		case "millones", "millón", "millon", "mm":
			c.Fin = d + len(p)
			for i := range c.lecturas {
				c.lecturas[i].mult = 1e6
			}
		case "soles", "sol":
			c.Tipo = TipoMonto
		}
		if c.Tipo != TipoMonto && c.lecturas[0].mult > 1 {
			// «4,8 mil soles»
			if e := saltarEspacios(s, c.Fin); strings.HasPrefix(strings.ToLower(palabraEn(s, e)), "sol") {
				c.Tipo = TipoMonto
			}
		}
	}
	c.Texto = s[c.Ini:c.Fin]
	return []Cifra{c}
}

func normAnio(a int) int {
	if a > 0 && a < 100 {
		return 2000 + a
	}
	return a
}

// Valor devuelve la primera lectura (para el JSON de verificación).
func (c Cifra) Valor() float64 {
	if len(c.lecturas) == 0 {
		return 0
	}
	return c.lecturas[0].x * c.lecturas[0].mult
}

// ---------- fuentes ----------

// Fuentes reúne las cifras que el LLM tuvo delante.
type Fuentes struct {
	valores []float64
	horas   map[[2]int]bool
	fechas  [][3]int
	montos  []float64 // montos de datos.filas (campos *_cts ya en soles)
	mostrar []string  // cifras permitidas tal como se le enseñan al LLM
	vistos  map[string]bool
}

func nuevasFuentes() *Fuentes {
	return &Fuentes{horas: map[[2]int]bool{}, vistos: map[string]bool{}}
}

func (f *Fuentes) mostrarUna(s string) {
	if s != "" && !f.vistos[s] {
		f.vistos[s] = true
		f.mostrar = append(f.mostrar, s)
	}
}

// AgregarTexto añade las cifras de un texto libre (FAQ, memoria, pregunta).
func (f *Fuentes) AgregarTexto(s string, mostrar bool) {
	for _, c := range ExtraerCifras(s) {
		switch c.Tipo {
		case TipoHora:
			f.horas[c.hora] = true
			f.valores = append(f.valores, float64(c.hora[0]))
		case TipoFecha:
			f.fechas = append(f.fechas, c.fecha)
			f.valores = append(f.valores, float64(c.fecha[0]))
			if c.fecha[2] != 0 {
				f.valores = append(f.valores, float64(c.fecha[2]))
			}
		default:
			for _, l := range c.lecturas {
				f.valores = append(f.valores, l.x*l.mult)
			}
		}
		if mostrar {
			f.mostrarUna(c.Texto)
		}
	}
}

// esCampoCts: las columnas en céntimos del GoldenSQL (saldo_cts, tarifa_cts_x_1000…).
func esCampoCts(k string) bool {
	return strings.HasSuffix(k, "_cts") || strings.Contains(k, "_cts_")
}

// AgregarDatos recorre datos.filas: los *_cts se pasan a soles (y el valor en
// céntimos NO cuenta como cifra: «S/ 142.000» sería mostrar céntimos como soles).
func (f *Fuentes) AgregarDatos(v *pyjson.Value, clave string) {
	if v == nil {
		return
	}
	switch v.Kind {
	case pyjson.Number:
		x, ok := v.Float()
		if !ok {
			return
		}
		if esCampoCts(clave) {
			soles := x / 100
			f.valores = append(f.valores, soles)
			f.montos = append(f.montos, soles)
			f.mostrarUna(FormatoSoles(soles))
			return
		}
		f.valores = append(f.valores, x)
		f.mostrarUna(formatoNumero(x))
	case pyjson.String:
		f.AgregarTexto(v.S, true)
	case pyjson.Array:
		for _, x := range v.Arr {
			f.AgregarDatos(x, clave)
		}
	case pyjson.Object:
		for _, k := range v.Keys {
			f.AgregarDatos(v.Props[k], k)
		}
	}
}

// FormatoSoles: 1420 → «S/ 1.420,00».
func FormatoSoles(x float64) string {
	cts := int64(math.Round(x * 100))
	signo := ""
	if cts < 0 {
		signo, cts = "-", -cts
	}
	ent := strconv.FormatInt(cts/100, 10)
	var b strings.Builder
	for i, r := range ent {
		if i > 0 && (len(ent)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return signo + "S/ " + b.String() + "," + strconv.FormatInt(100+cts%100, 10)[1:]
}

// formatoNumero: 13.1 → «13,1»; 3 → «3».
func formatoNumero(x float64) string {
	return strings.Replace(strconv.FormatFloat(x, 'f', -1, 64), ".", ",", 1)
}

func redondear(x float64, dec int) float64 {
	p := math.Pow(10, float64(dec))
	return math.Round(x*p) / p
}

// Verificada dice si una cifra está en las fuentes. Un número escrito con d
// decimales vale si alguna fuente, redondeada a d decimales, da lo mismo
// («S/ 1.420» ← 1420,00; «4,8 mil» ← 4 812,50; «13 %» ← 13,1).
func (f *Fuentes) Verificada(c Cifra) bool {
	switch c.Tipo {
	case TipoHora:
		if f.horas[c.hora] {
			return true
		}
		if c.hora[1] == 0 {
			for _, v := range f.valores {
				if v == float64(c.hora[0]) {
					return true
				}
			}
		}
		return false
	case TipoFecha:
		for _, x := range f.fechas {
			if x[0] == c.fecha[0] && x[1] == c.fecha[1] && (c.fecha[2] == 0 || x[2] == 0 || x[2] == c.fecha[2]) {
				return true
			}
		}
		return false
	}
	for _, l := range c.lecturas {
		for _, v := range f.valores {
			if math.Abs(redondear(v/l.mult, l.dec)-l.x) < 1e-9 {
				return true
			}
		}
	}
	return false
}

// Verificar separa las cifras del texto en buenas y malas.
func (f *Fuentes) Verificar(texto string) (todas, malas []Cifra) {
	todas = ExtraerCifras(texto)
	for _, c := range todas {
		if !f.Verificada(c) {
			malas = append(malas, c)
		}
	}
	return todas, malas
}

// MontosUnicos: montos distintos de datos.filas.
func (f *Fuentes) MontosUnicos() []float64 {
	var u []float64
	for _, m := range f.montos {
		dup := false
		for _, x := range u {
			if math.Abs(x-m) < 1e-9 {
				dup = true
				break
			}
		}
		if !dup {
			u = append(u, m)
		}
	}
	return u
}

// ReunirFuentes junta las cifras de todo lo que ve el LLM, salvo el SYSTEM
// (su «S/ 4.800,00» de ejemplo no es un dato) y las respuestas anteriores
// del asistente.
func ReunirFuentes(mensajes []Mensaje, frag, mem []*Item, datos *pyjson.Value) *Fuentes {
	f := nuevasFuentes()
	if filas := datos.Get("filas"); filas.Truthy() {
		f.AgregarDatos(filas, "")
	} else if ej := datos.Get("golden"); ej.Truthy() && ej.Kind == pyjson.Array {
		for _, g := range ej.Arr {
			if p := g.Get("pregunta"); p != nil {
				f.AgregarTexto(p.PyStr(), false)
			}
		}
	}
	for _, x := range frag {
		if t, err := x.Texto(); err == nil {
			f.AgregarTexto(t, true)
		}
	}
	for _, x := range mem {
		if t, err := x.Texto(); err == nil {
			f.AgregarTexto(t, false)
		}
		if r := x.V.Get("respuesta"); r != nil {
			f.AgregarTexto(r.PyStr(), true)
		}
	}
	for _, x := range mensajes {
		if x.Role == "user" {
			f.AgregarTexto(x.Content, false)
		}
	}
	return f
}

// ---------- prompt ----------

const maxCifrasPrompt = 40

func (f *Fuentes) listaMostrar() string {
	l := f.mostrar
	if len(l) > maxCifrasPrompt {
		l = l[:maxCifrasPrompt]
	}
	return strings.Join(l, " · ")
}

// ReglasCifras es la parte del system que se añade tras _ensamblar.
func ReglasCifras(f *Fuentes, datos *pyjson.Value) string {
	var b strings.Builder
	b.WriteString("Reglas de cifras: escribe SOLO cifras que aparezcan textualmente en el contexto, los datos o la pregunta, copiadas tal cual. " +
		"No calcules sumas, restas, promedios ni porcentajes que no vengan ya escritos. " +
		"El «S/ 4.800,00» de arriba es solo un ejemplo de formato, no un dato. " +
		"Si no tienes la cifra que te piden, dilo sin poner ningún número.")
	if resumen := montosPorFila(datos); resumen != "" {
		b.WriteString("\nLos campos *_cts de los datos están en céntimos; estos son sus montos ya en soles (cópialos así):\n")
		b.WriteString(resumen)
	}
	if lista := f.listaMostrar(); lista != "" {
		b.WriteString("\nCifras disponibles: " + lista)
	}
	return b.String()
}

// Estricto es el añadido del reintento.
func Estricto(f *Fuentes, malas []Cifra) string {
	var inv []string
	for _, c := range malas {
		inv = append(inv, "«"+c.Texto+"»")
	}
	s := "ATENCIÓN: tu respuesta anterior tenía cifras que NO están en los datos: " + strings.Join(inv, ", ") + ". Están prohibidas. "
	if lista := f.listaMostrar(); lista != "" {
		s += "Las ÚNICAS cifras que puedes escribir, copiadas tal cual, son: " + lista + ". "
	} else {
		s += "No tienes ninguna cifra disponible: responde sin números. "
	}
	return s + "Si la que te piden no está en esa lista, responde «No tengo ese monto a mano; revísalo en Recibos» sin ninguna cifra."
}

// etiquetaFila: la primera columna de texto (código, rubro, nombre…).
func etiquetaFila(fila *pyjson.Value) string {
	for _, k := range fila.Keys {
		if x := fila.Props[k]; x.Kind == pyjson.String && x.S != "" {
			return k + " " + x.S
		}
	}
	return ""
}

func nombreCampo(k string) string {
	k = strings.Replace(k, "_cts", "", 1)
	return strings.ReplaceAll(k, "_", " ")
}

// montosPorFila: «- codigo 402: saldo = S/ 1.420,00» por cada *_cts.
func montosPorFila(datos *pyjson.Value) string {
	filas := datos.Get("filas")
	if filas == nil || filas.Kind != pyjson.Array {
		return ""
	}
	var b strings.Builder
	for i, fila := range filas.Arr {
		if i >= 8 || fila.Kind != pyjson.Object {
			break
		}
		var partes []string
		for _, k := range fila.Keys {
			if x, ok := fila.Props[k].Float(); ok && esCampoCts(k) {
				partes = append(partes, nombreCampo(k)+" = "+FormatoSoles(x/100))
			}
		}
		if len(partes) == 0 {
			continue
		}
		if et := etiquetaFila(fila); et != "" {
			b.WriteString("- " + et + ": ")
		} else {
			b.WriteString("- ")
		}
		b.WriteString(strings.Join(partes, "; ") + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---------- versión segura ----------

const (
	SinMonto = "No tengo ese monto a mano; revísalo en Recibos."
	SinDato  = "No tengo ese dato a mano; escríbele a la administración."
)

// ResumenFilas narra datos.filas sin LLM (≤8 filas de valores simples).
func ResumenFilas(datos *pyjson.Value) string {
	filas := datos.Get("filas")
	if filas == nil || filas.Kind != pyjson.Array || len(filas.Arr) == 0 || len(filas.Arr) > 8 {
		return ""
	}
	var lineas []string
	for _, fila := range filas.Arr {
		if fila.Kind != pyjson.Object {
			return ""
		}
		var partes []string
		for _, k := range fila.Keys {
			x := fila.Props[k]
			switch x.Kind {
			case pyjson.Number:
				n, _ := x.Float()
				if esCampoCts(k) {
					partes = append(partes, nombreCampo(k)+": "+FormatoSoles(n/100))
				} else {
					partes = append(partes, nombreCampo(k)+": "+formatoNumero(n))
				}
			case pyjson.String:
				partes = append(partes, nombreCampo(k)+": "+x.S)
			case pyjson.Bool:
				partes = append(partes, nombreCampo(k)+": "+map[bool]string{true: "sí", false: "no"}[x.B])
			case pyjson.Array: // franjas, listas de valores simples
				var vs []string
				for _, e := range x.Arr {
					switch e.Kind {
					case pyjson.String, pyjson.Number:
						vs = append(vs, e.PyStr())
					case pyjson.Object:
						if a, b := e.Get("inicio"), e.Get("fin"); a != nil && b != nil {
							vs = append(vs, a.PyStr()+" a "+b.PyStr())
							continue
						}
						var kv []string
						for _, k2 := range e.Keys {
							if e.Props[k2].Kind == pyjson.String || e.Props[k2].Kind == pyjson.Number {
								kv = append(kv, e.Props[k2].PyStr())
							}
						}
						vs = append(vs, strings.Join(kv, " "))
					}
				}
				if len(vs) > 0 {
					partes = append(partes, nombreCampo(k)+": "+strings.Join(vs, ", "))
				}
			}
		}
		lineas = append(lineas, "- "+strings.Join(partes, " · "))
	}
	return "Esto es lo que tengo en los datos del edificio:\n" + strings.Join(lineas, "\n")
}

// oraciones parte en frases (fin de línea o «. », «? », «! »), sin cortar
// «S/. » ni los números.
func oraciones(s string) []string {
	var fuera []string
	ini := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		corta := c == '\n'
		if (c == '.' || c == '?' || c == '!') && i+1 < len(s) && (s[i+1] == ' ' || s[i+1] == '\n') {
			corta = !(c == '.' && i >= 2 && strings.EqualFold(s[i-2:i], "S/"))
		}
		if corta {
			fuera = append(fuera, s[ini:i+1])
			ini = i + 1
		}
	}
	if ini < len(s) {
		fuera = append(fuera, s[ini:])
	}
	return fuera
}

func hayMonto(cs []Cifra) bool {
	for _, c := range cs {
		if c.Tipo == TipoMonto {
			return true
		}
	}
	return false
}

// VersionSegura quita del texto las cifras no verificadas:
//  1. una sola cifra mala, es el único monto del texto y los datos traen un
//     único monto → se cambia por ese monto (la intención es inequívoca);
//  2. si hay filas → se narran sin LLM;
//  3. si no → se quitan las frases con cifras malas y se avisa.
func VersionSegura(texto string, malas []Cifra, f *Fuentes, datos *pyjson.Value) string {
	montos := 0
	for _, c := range ExtraerCifras(texto) {
		if c.Tipo == TipoMonto {
			montos++
		}
	}
	if u := f.MontosUnicos(); len(malas) == 1 && malas[0].Tipo == TipoMonto && montos == 1 && len(u) == 1 {
		c := malas[0]
		return texto[:c.Ini] + FormatoSoles(u[0]) + texto[c.Fin:]
	}
	if r := ResumenFilas(datos); r != "" {
		return r
	}
	aviso := SinDato
	if hayMonto(malas) {
		aviso = SinMonto
	}
	var quedan []string
	for _, o := range oraciones(texto) {
		_, m := f.Verificar(o)
		if len(m) == 0 {
			quedan = append(quedan, o)
		}
	}
	resto := pyStrip(strings.Join(quedan, ""))
	if resto == "" {
		return aviso
	}
	return resto + "\n" + aviso
}

// textos devuelve el texto de cada cifra sin repetir, en orden.
func textos(cs []Cifra) []string {
	fuera := []string{}
	visto := map[string]bool{}
	for _, c := range cs {
		if !visto[c.Texto] {
			visto[c.Texto] = true
			fuera = append(fuera, c.Texto)
		}
	}
	return fuera
}
