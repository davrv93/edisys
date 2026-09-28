// Package chatbot clasifica mensajes de WhatsApp por reglas (sin LLM): normaliza el texto,
// busca palabras clave por intención en un orden fijo y extrae área y fecha si las hay.
package chatbot

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Intenciones reconocidas.
const (
	Saludo       = "saludo"
	Saldo        = "saldo"
	UltimoRecibo = "ultimo_recibo"
	Pagar        = "pagar"
	Reservar     = "reservar"
	Reportar     = "reportar_incidencia"
	Horarios     = "horarios"
	HablarAdmin  = "hablar_admin"
	Menu         = "menu"
	NoEntendi    = "no_entendi"
)

// Resultado de clasificar un mensaje.
type Resultado struct {
	Intencion string
	Texto     string     // texto normalizado
	Area      string     // slug aproximado: parrilla, sum, piscina, gimnasio, terraza
	Fecha     *time.Time // fecha pedida (reservas), en Lima
	Opcion    int        // si respondió con un número del menú
}

var quitarTildes = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

var reNoAlfa = regexp.MustCompile(`[^a-z0-9/ ]+`)
var reEspacios = regexp.MustCompile(`\s+`)

// Normalizar pasa a minúsculas, quita tildes y signos, y corrige abreviaturas comunes de chat.
func Normalizar(s string) string {
	s = strings.ToLower(s)
	if t, _, err := transform.String(quitarTildes, s); err == nil {
		s = t
	}
	s = strings.ReplaceAll(s, "ñ", "n")
	s = reNoAlfa.ReplaceAllString(s, " ")
	s = reEspacios.ReplaceAllString(strings.TrimSpace(s), " ")
	reemplazos := map[string]string{"q": "que", "xq": "porque", "pq": "porque", "tb": "tambien", "tmb": "tambien", "dpto": "departamento", "depa": "departamento", "k": "que", "cuant": "cuanto", "grax": "gracias", "bn": "bien", "hla": "hola", "ola": "hola"}
	toks := strings.Fields(s)
	for i, t := range toks {
		if r, ok := reemplazos[t]; ok {
			toks[i] = r
		}
	}
	return strings.Join(toks, " ")
}

// regla: una intención y sus claves. Una clave con «*» al final es prefijo de palabra;
// con espacios es una frase; si no, palabra exacta.
type regla struct {
	intencion string
	claves    []string
}

// El orden importa: lo más específico primero.
var reglas = []regla{
	{HablarAdmin, []string{"hablar con", "administrador*", "administracion", "asesor*", "humano", "una persona", "comunicarme", "llamar*", "contactar*"}},
	{Reportar, []string{"reportar", "reporto", "reporte", "fuga*", "gotea*", "goteo", "averia*", "malogr*", "no funciona*", "no prende", "no enciende", "roto", "rota", "incidencia*", "atasc*", "inund*", "sin agua", "sin luz", "se cayo", "filtracion", "humedad", "atoro", "atorado", "desague"}},
	{Reservar, []string{"reserv*", "disponib*", "libre", "separar", "alquilar"}},
	{Horarios, []string{"horario*", "a que hora", "hasta que hora", "desde que hora", "hora", "norma*", "reglamento", "reglas", "permitido", "mascota*", "aforo"}},
	{Pagar, []string{"pagar", "pago", "pague", "yape*", "plin", "transferencia", "deposit*", "numero de cuenta", "cuenta bancaria", "como pago", "donde pago"}},
	{UltimoRecibo, []string{"recibo*", "boleta*", "cuanto me toca", "cuanto es mi", "detalle", "cuota del mes"}},
	{Saldo, []string{"debo", "deuda*", "saldo", "cuanto", "pendiente*", "estado de cuenta", "al dia", "mora", "moroso", "adeudo"}},
	{Menu, []string{"menu", "ayuda", "opciones", "ayudame"}},
	{Saludo, []string{"hola", "buenas", "buenos dias", "buen dia", "buenas tardes", "buenas noches", "hey", "que tal", "alo", "saludos"}},
}

// opcionesMenu: si el mensaje es solo un número, se toma como opción del menú.
var opcionesMenu = map[int]string{1: Saldo, 2: UltimoRecibo, 3: Pagar, 4: Reservar, 5: Reportar, 6: Horarios, 7: HablarAdmin}

// Clasificar devuelve la intención del mensaje. hoy se usa para resolver «mañana», «sábado», «5/10».
func Clasificar(texto string, hoy time.Time) Resultado {
	n := Normalizar(texto)
	res := Resultado{Intencion: NoEntendi, Texto: n, Area: DetectarArea(n), Fecha: DetectarFecha(n, hoy)}
	if v, err := strconv.Atoi(n); err == nil {
		if in, ok := opcionesMenu[v]; ok {
			res.Intencion = in
			res.Opcion = v
			return res
		}
	}
	if n == "" {
		return res
	}
	for _, rg := range reglas {
		if contiene(n, rg.claves) {
			res.Intencion = rg.intencion
			break
		}
	}
	// «¿Está libre la parrilla?» o «parrilla el sábado» sin verbo: si nombra un área y una fecha, es reserva.
	if (res.Intencion == NoEntendi || res.Intencion == Saludo) && res.Area != "" && res.Fecha != nil {
		res.Intencion = Reservar
	}
	return res
}

func contiene(n string, claves []string) bool {
	relleno := " " + n + " "
	toks := strings.Fields(n)
	for _, c := range claves {
		switch {
		case strings.HasSuffix(c, "*"):
			pre := strings.TrimSuffix(c, "*")
			if strings.Contains(pre, " ") {
				if strings.Contains(relleno, " "+pre) {
					return true
				}
				continue
			}
			for _, t := range toks {
				if strings.HasPrefix(t, pre) {
					return true
				}
			}
		default:
			if strings.Contains(relleno, " "+c+" ") {
				return true
			}
		}
	}
	return false
}

// DetectarArea reconoce el área común nombrada en el mensaje.
func DetectarArea(n string) string {
	areas := []struct{ slug, clave string }{
		{"parrillas", "parrill*"}, {"parrillas", "bbq"}, {"parrillas", "barbacoa"},
		{"sum", "sum"}, {"sum", "salon*"}, {"sum", "usos multiples"},
		{"piscina", "piscina"}, {"piscina", "alberca"},
		{"gimnasio", "gimnasio"}, {"gimnasio", "gym"},
		{"terraza", "terraza"},
	}
	for _, a := range areas {
		if contiene(n, []string{a.clave}) {
			return a.slug
		}
	}
	return ""
}

var diasSemana = map[string]time.Weekday{"domingo": time.Sunday, "lunes": time.Monday, "martes": time.Tuesday, "miercoles": time.Wednesday, "jueves": time.Thursday, "viernes": time.Friday, "sabado": time.Saturday}
var meses = map[string]time.Month{"enero": 1, "febrero": 2, "marzo": 3, "abril": 4, "mayo": 5, "junio": 6, "julio": 7, "agosto": 8, "setiembre": 9, "septiembre": 9, "octubre": 10, "noviembre": 11, "diciembre": 12}

var reDiaMes = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})(?:/(\d{2,4}))?\b`)
var reDiaDeMes = regexp.MustCompile(`\b(\d{1,2}) de ([a-z]+)\b`)
var reElDia = regexp.MustCompile(`\b(?:el|dia) (\d{1,2})\b`)

// DetectarFecha entiende hoy, mañana, pasado mañana, días de la semana, 5/10, «5 de octubre», «el 5».
func DetectarFecha(n string, hoy time.Time) *time.Time {
	base := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, hoy.Location())
	d := func(t time.Time) *time.Time { return &t }
	relleno := " " + n + " "
	switch {
	case strings.Contains(relleno, " pasado manana "):
		return d(base.AddDate(0, 0, 2))
	case strings.Contains(relleno, " manana "):
		return d(base.AddDate(0, 0, 1))
	case strings.Contains(relleno, " hoy ") || strings.Contains(relleno, " esta noche "):
		return d(base)
	}
	if m := reDiaMes.FindStringSubmatch(n); m != nil {
		dia, _ := strconv.Atoi(m[1])
		mes, _ := strconv.Atoi(m[2])
		anio := base.Year()
		if m[3] != "" {
			anio, _ = strconv.Atoi(m[3])
			if anio < 100 {
				anio += 2000
			}
		}
		if f, ok := fechaValida(anio, time.Month(mes), dia, base, m[3] == ""); ok {
			return d(f)
		}
	}
	if m := reDiaDeMes.FindStringSubmatch(n); m != nil {
		if mes, ok := meses[m[2]]; ok {
			dia, _ := strconv.Atoi(m[1])
			if f, ok := fechaValida(base.Year(), mes, dia, base, true); ok {
				return d(f)
			}
		}
	}
	for nombre, wd := range diasSemana {
		if strings.Contains(relleno, " "+nombre+" ") {
			delta := (int(wd) - int(base.Weekday()) + 7) % 7
			return d(base.AddDate(0, 0, delta))
		}
	}
	if m := reElDia.FindStringSubmatch(n); m != nil {
		dia, _ := strconv.Atoi(m[1])
		if f, ok := fechaValida(base.Year(), base.Month(), dia, base, false); ok {
			if f.Before(base) {
				f = f.AddDate(0, 1, 0)
			}
			return d(f)
		}
	}
	return nil
}

func fechaValida(anio int, mes time.Month, dia int, base time.Time, pasarAlProximoAnio bool) (time.Time, bool) {
	if mes < 1 || mes > 12 || dia < 1 || dia > 31 {
		return time.Time{}, false
	}
	f := time.Date(anio, mes, dia, 0, 0, 0, 0, base.Location())
	if f.Day() != dia {
		return time.Time{}, false
	}
	if pasarAlProximoAnio && f.Before(base) {
		f = f.AddDate(1, 0, 0)
	}
	return f, true
}
