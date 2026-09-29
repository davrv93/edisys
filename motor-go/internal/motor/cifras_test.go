package motor

import (
	"encoding/json"
	"strings"
	"testing"

	"edisys/motor-go/internal/pyjson"
)

func datosDe(t *testing.T, s string) *pyjson.Value {
	t.Helper()
	v, err := pyjson.Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// ---------- extractor y normalización ----------

func TestExtraerCifrasFormatosPeruanos(t *testing.T) {
	casos := []struct {
		texto, cifra, tipo string
		valores            []float64 // lecturas posibles (x·mult)
	}{
		{"Debes S/ 1.420,00 a la fecha.", "S/ 1.420,00", TipoMonto, []float64{1420}},
		{"Debes S/1420 en total", "S/1420", TipoMonto, []float64{1420}},
		{"Son 1,420.00 soles", "1,420.00", TipoMonto, []float64{1420}},
		{"La morosidad es 13,1 %.", "13,1 %", TipoPorcentaje, []float64{13.1}},
		{"La morosidad es 13,1%.", "13,1%", TipoPorcentaje, []float64{13.1}},
		{"Entraron 4,8 mil soles", "4,8 mil", TipoMonto, []float64{4800}},
		{"Hay S/. 3.500 pendientes", "S/. 3.500", TipoMonto, []float64{3500, 3.5}},
		{"Tarifa: S/ 80,00.", "S/ 80,00", TipoMonto, []float64{80}},
		{"Son 2.5 horas", "2.5", TipoNumero, []float64{2.5}},
		{"Tienes 3 recibos", "3", TipoNumero, []float64{3}},
	}
	for _, c := range casos {
		cs := ExtraerCifras(c.texto)
		if len(cs) != 1 {
			t.Errorf("%q: %d cifras %v", c.texto, len(cs), textos(cs))
			continue
		}
		g := cs[0]
		if g.Texto != c.cifra || g.Tipo != c.tipo {
			t.Errorf("%q: cifra %q (%s), quería %q (%s)", c.texto, g.Texto, g.Tipo, c.cifra, c.tipo)
		}
		if len(g.lecturas) != len(c.valores) {
			t.Errorf("%q: lecturas %v, quería %v", c.texto, g.lecturas, c.valores)
			continue
		}
		for i, l := range g.lecturas {
			if d := l.x*l.mult - c.valores[i]; d > 1e-9 || d < -1e-9 {
				t.Errorf("%q: lectura %d = %v, quería %v", c.texto, i, l.x*l.mult, c.valores[i])
			}
		}
	}
}

func TestExtraerHorasFechasYExcluidos(t *testing.T) {
	cs := ExtraerCifras("La piscina abre de 8:00 a 20:00 h; tu recibo vence el 29/09 y el 2026-10-05.")
	if got := strings.Join(textos(cs), "|"); got != "8:00|20:00|29/09|2026-10-05" {
		t.Fatalf("cifras: %s", got)
	}
	if cs[2].Tipo != TipoFecha || cs[2].fecha != [3]int{29, 9, 0} || cs[3].fecha != [3]int{5, 10, 2026} {
		t.Errorf("fechas: %+v %+v", cs[2], cs[3])
	}
	// Pegados a letras (e5, F5) y viñetas de lista no son cifras.
	if cs := ExtraerCifras("1. Entra a la app\n2) Paga con e5 en F5"); len(cs) != 0 {
		t.Errorf("no debía encontrar cifras: %v", textos(cs))
	}
	// Rangos y correlativos se parten.
	if got := strings.Join(textos(ExtraerCifras("de 8-12 y el R-2026-0012")), "|"); got != "8|12|2026|0012" {
		t.Errorf("rangos: %s", got)
	}
}

func TestFormatoSoles(t *testing.T) {
	for x, want := range map[float64]string{1420: "S/ 1.420,00", 0: "S/ 0,00", 80.5: "S/ 80,50", 1234567.891: "S/ 1.234.567,89", -12: "-S/ 12,00"} {
		if got := FormatoSoles(x); got != want {
			t.Errorf("FormatoSoles(%v) = %q, quería %q", x, got, want)
		}
	}
}

// ---------- verificación contra las fuentes ----------

// fuentes402: lo que manda el API para «¿cuánto debe el dpto 402?» (golden 3).
func fuentes402(t *testing.T) (*Fuentes, *pyjson.Value) {
	datos := datosDe(t, `{"filas":[{"codigo":"402","saldo_cts":142000}]}`)
	msgs := []Mensaje{{Role: "user", Content: "¿Cuánto debe el Dpto 402 al 29/09?"}}
	return ReunirFuentes(msgs, nil, nil, datos), datos
}

func TestVerificarMontoReal402(t *testing.T) {
	f, _ := fuentes402(t)
	for _, bien := range []string{
		"El Dpto 402 debe S/ 1.420,00.",
		"El 402 debe S/1420.",
		"Debe 1,420.00 soles.",
		"Debe S/ 1.420 al 29/09.",   // fecha de la pregunta
		"Debe 1,4 mil soles (402).", // redondeo a miles
	} {
		if _, malas := f.Verificar(bien); len(malas) != 0 {
			t.Errorf("%q: marcó %v como inventadas", bien, textos(malas))
		}
	}
	for _, mal := range []string{
		"El Dpto 402 debe S/ 1.500,00.", // el caso real
		"Debe S/ 142.000.",              // céntimos leídos como soles
		"Debe S/ 142,00.",
		"Debe S/ 4.800,00.", // el ejemplo del SYSTEM no es dato
		"Vence el 30/09.",
		"Son 3 meses.",
	} {
		if _, malas := f.Verificar(mal); len(malas) == 0 {
			t.Errorf("%q: debía detectar la cifra inventada", mal)
		}
	}
}

func TestVerificarFalsosPositivos(t *testing.T) {
	frag := itemsDe(t, []any{map[string]any{"id": "f1", "texto": "La piscina abre de 8:00 a 20:00. Aforo 25 personas. Morosidad de 2025: 13,1 %."}})
	msgs := []Mensaje{
		{Role: "user", Content: "hola, soy del 201"},
		{Role: "assistant", Content: "Tu saldo es S/ 999,00"}, // lo dicho por el asistente no es fuente
		{Role: "user", Content: "¿y el horario del 29/09?"},
	}
	f := ReunirFuentes(msgs, frag, nil, datosDe(t, `{}`))
	for _, bien := range []string{
		"El 201 puede ir de 8:00 a 20:00 el 29/09.",
		"Abre a las 8 y cierra a las 20:00.",
		"El aforo es de 25 personas.",
		"En 2025 la morosidad fue 13,1 %.",
		"La morosidad ronda el 13 %.",
	} {
		if _, malas := f.Verificar(bien); len(malas) != 0 {
			t.Errorf("%q: marcó %v", bien, textos(malas))
		}
	}
	for _, mal := range []string{"Tu saldo es S/ 999,00", "Abre a las 9:00.", "En 2024 fue igual.", "Aforo 30."} {
		if _, malas := f.Verificar(mal); len(malas) == 0 {
			t.Errorf("%q: debía detectarse", mal)
		}
	}
}

func TestReglasCifrasConvierteCentimos(t *testing.T) {
	f, datos := fuentes402(t)
	r := ReglasCifras(f, datos)
	if !strings.Contains(r, "- codigo 402: saldo = S/ 1.420,00") || !strings.Contains(r, "Cifras disponibles: 402 · S/ 1.420,00") {
		t.Errorf("reglas: %s", r)
	}
	if strings.Contains(r, "142000") {
		t.Error("los céntimos crudos no se ofrecen como cifra")
	}
}

// ---------- versión segura ----------

func TestVersionSegura(t *testing.T) {
	f, datos := fuentes402(t)
	// 1) una sola cifra mala y un único monto en datos → se sustituye.
	texto := "El Dpto 402 debe S/ 1.500,00 este mes."
	_, malas := f.Verificar(texto)
	if got := VersionSegura(texto, malas, f, datos); got != "El Dpto 402 debe S/ 1.420,00 este mes." {
		t.Errorf("sustitución: %q", got)
	}
	// 2) varias cifras malas con filas → se narran las filas.
	texto = "El 402 pagó S/ 3.500,00 de cuota y S/ 800,00 de saldo."
	_, malas = f.Verificar(texto)
	got := VersionSegura(texto, malas, f, datos)
	if got != "Esto es lo que tengo en los datos del edificio:\n- codigo: 402 · saldo: S/ 1.420,00" {
		t.Errorf("resumen: %q", got)
	}
	// 3) sin filas: se quitan las frases con cifras malas.
	frag := itemsDe(t, []any{map[string]any{"id": "f", "texto": "La parrilla se reserva con 48 h de anticipación."}})
	f2 := ReunirFuentes([]Mensaje{{Role: "user", Content: "¿cuánto cuesta la parrilla?"}}, frag, nil, datosDe(t, `{}`))
	texto = "La parrilla cuesta S/ 80,00. Resérvala con 48 h de anticipación."
	_, malas = f2.Verificar(texto)
	if got := VersionSegura(texto, malas, f2, datosDe(t, `{}`)); got != "Resérvala con 48 h de anticipación.\n"+SinMonto {
		t.Errorf("sin filas: %q", got)
	}
	texto = "Cuesta S/ 80,00."
	_, malas = f2.Verificar(texto)
	if got := VersionSegura(texto, malas, f2, datosDe(t, `{}`)); got != SinMonto {
		t.Errorf("todo inventado: %q", got)
	}
}

// ---------- chat de punta a punta con un llama falso ----------

type respChat struct {
	Respuesta    string `json:"respuesta"`
	Verificacion struct {
		Cifras      []string `json:"cifras"`
		OK          bool     `json:"ok"`
		Reintento   bool     `json:"reintento"`
		Seguro      bool     `json:"seguro"`
		Descartadas []string `json:"descartadas"`
	} `json:"verificacion"`
}

const cuerpo402 = `{"mensajes":[{"role":"user","content":"¿cuánto debe el dpto 402?"}],"pedir_sugerencias":false,"datos":{"filas":[{"codigo":"402","saldo_cts":142000}]}}`

func chat402(t *testing.T, e *entorno) respChat {
	t.Helper()
	cod, b := e.post(t, "/v1/chat", cuerpo402)
	if cod != 200 {
		t.Fatalf("%d %s", cod, b)
	}
	var r respChat
	if err := json.Unmarshal([]byte(b), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestChatCifraCorrectaSinReintento(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	e.llama.respuesta = "El Dpto 402 debe S/ 1.420,00."
	r := chat402(t, e)
	if r.Respuesta != "El Dpto 402 debe S/ 1.420,00." || !r.Verificacion.OK || r.Verificacion.Reintento ||
		strings.Join(r.Verificacion.Cifras, "|") != "402|S/ 1.420,00" {
		t.Errorf("%+v", r)
	}
	if len(e.llama.pedidos) != 1 {
		t.Errorf("%d llamadas a llama", len(e.llama.pedidos))
	}
}

func TestChatReintentoCorrige(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	e.llama.respuestas = []string{"El Dpto 402 debe S/ 1.500,00.", "El Dpto 402 debe S/ 1.420,00."}
	r := chat402(t, e)
	if r.Respuesta != "El Dpto 402 debe S/ 1.420,00." || !r.Verificacion.OK || !r.Verificacion.Reintento ||
		r.Verificacion.Seguro || strings.Join(r.Verificacion.Descartadas, "|") != "S/ 1.500,00" {
		t.Errorf("%+v", r)
	}
	if len(e.llama.pedidos) != 2 {
		t.Fatalf("%d llamadas a llama", len(e.llama.pedidos))
	}
	// El reintento lleva la lista de cifras permitidas y temperatura 0.
	p := e.llama.pedidos[1]
	sys := p["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "«S/ 1.500,00»") || !strings.Contains(sys, "son: 402 · S/ 1.420,00") || p["temperature"] != 0.0 {
		t.Errorf("reintento: temp %v, system %q", p["temperature"], sys[len(sys)-400:])
	}
	// No queda motivo en el registro: el LLM acabó acertando.
	_, reg := e.get(t, "/v1/registro")
	if strings.Contains(reg, MotivoCifra) {
		t.Errorf("registro: %s", reg)
	}
}

func TestChatInsisteEnInventarVersionSegura(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	e.llama.respuestas = []string{"El Dpto 402 debe S/ 1.500,00.", "Debe S/ 1.550,00."}
	r := chat402(t, e)
	if r.Respuesta != "Debe S/ 1.420,00." || r.Verificacion.OK || !r.Verificacion.Seguro ||
		strings.Join(r.Verificacion.Descartadas, "|") != "S/ 1.500,00|S/ 1.550,00" {
		t.Errorf("%+v", r)
	}
	_, reg := e.get(t, "/v1/registro")
	if !strings.Contains(reg, `"motivo":"cifra_no_verificada","descartadas":["S/ 1.500,00","S/ 1.550,00"]`) ||
		!strings.Contains(reg, `"respuesta":"Debe S/ 1.420,00."`) {
		t.Errorf("registro: %s", reg)
	}

	// Sin datos: se responde sin la cifra.
	e.llama.respuestas = []string{"La parrilla cuesta S/ 80,00.", "Cuesta S/ 90,00."}
	_, b := e.post(t, "/v1/chat", `{"mensajes":[{"role":"user","content":"¿cuánto cuesta la parrilla?"}],"pedir_sugerencias":false}`)
	var r2 respChat
	_ = json.Unmarshal([]byte(b), &r2)
	if r2.Respuesta != SinMonto || !r2.Verificacion.Seguro || len(r2.Verificacion.Cifras) != 0 {
		t.Errorf("%s", b)
	}
}

func TestChatLlamaCaeEnElReintento(t *testing.T) {
	e := nuevoEntorno(t, faqSemilla)
	e.llama.respuestas = []string{"El Dpto 402 debe S/ 1.500,00."}
	// Tras la primera respuesta, llama falla: no es un 502, es la versión segura.
	e.llama.mu.Lock()
	e.llama.codigoTras = 1
	e.llama.mu.Unlock()
	r := chat402(t, e)
	if r.Respuesta != "El Dpto 402 debe S/ 1.420,00." || !r.Verificacion.Seguro {
		t.Errorf("%+v", r)
	}
}

// Casos vistos en la prueba real del 29-09-2026.
func TestCasosReales29Sep(t *testing.T) {
	// Rangos de horas: «12:00-17:00» son dos horas, no «00-17».
	if got := strings.Join(textos(ExtraerCifras("de 12:00-17:00 y 18:00 - 23:00")), "|"); got != "12:00|17:00|18:00|23:00" {
		t.Errorf("rangos de horas: %s", got)
	}
	// Parrilla: tarifa_cts 8000 = S/ 80,00; «S/ 8,00» es inventada; las franjas cuentan.
	datos := datosDe(t, `{"filas":[{"nombre":"Parrillas","tarifa_cts":8000,"franjas":[{"fin":"17:00","inicio":"12:00"},{"fin":"23:00","inicio":"18:00"}]}]}`)
	f := ReunirFuentes([]Mensaje{{Role: "user", Content: "¿Cuánto cuesta la parrilla?"}}, nil, nil, datos)
	if _, m := f.Verificar("Cuesta S/ 80,00 y se usa de 12:00-17:00 o de 18:00 a 23:00."); len(m) != 0 {
		t.Errorf("parrilla bien: %v", textos(m))
	}
	texto := "La parrilla cuesta S/ 8,00 por franja de 12:00-17:00."
	_, m := f.Verificar(texto)
	if strings.Join(textos(m), "|") != "S/ 8,00" {
		t.Fatalf("parrilla mal: %v", textos(m))
	}
	if got := VersionSegura(texto, m, f, datos); got != "La parrilla cuesta S/ 80,00 por franja de 12:00-17:00." {
		t.Errorf("sustitución parrilla: %q", got)
	}
	// 402: ya había un monto correcto y otro inventado → NO se sustituye (diría
	// que la cuota es la deuda); se narran las filas.
	f402, d402 := fuentes402(t)
	texto = "El Dpto 402 tiene un saldo de S/ 1.420,00 y la próxima cuota es de S/ 385,00."
	_, m = f402.Verificar(texto)
	if got := VersionSegura(texto, m, f402, d402); !strings.HasPrefix(got, "Esto es lo que tengo") {
		t.Errorf("402 con dos montos: %q", got)
	}
	if got := ResumenFilas(datos); got != "Esto es lo que tengo en los datos del edificio:\n- nombre: Parrillas · tarifa: S/ 80,00 · franjas: 12:00 a 17:00, 18:00 a 23:00" {
		t.Errorf("resumen con franjas: %q", got)
	}
}
