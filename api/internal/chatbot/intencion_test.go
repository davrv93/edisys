package chatbot

import (
	"testing"
	"time"
)

var lima = time.FixedZone("Lima", -5*3600)
var hoy = time.Date(2026, 9, 28, 10, 0, 0, 0, lima) // lunes

func TestIntenciones(t *testing.T) {
	casos := []struct{ texto, quiero string }{
		{"Hola", Saludo},
		{"Buenas tardes!", Saludo},
		{"¿Cuánto debo?", Saldo},
		{"cuanto debo", Saldo},
		{"hola, cuánto debo?", Saldo},
		{"¿Tengo alguna deuda pendiente?", Saldo},
		{"¿Estoy al día?", Saldo},
		{"Mándame mi último recibo", UltimoRecibo},
		{"cuanto es mi recibo de setiembre", UltimoRecibo},
		{"¿Cómo pago? ¿tienen Yape?", Pagar},
		{"quiero pagar mi deuda", Pagar},
		{"Quiero reservar la parrilla el sábado", Reservar},
		{"¿Está libre el SUM mañana?", Reservar},
		{"parrilla el 5/10", Reservar},
		{"Hay una fuga de agua en el baño del 3er piso", Reportar},
		{"el ascensor no funciona", Reportar},
		{"la luz del pasadizo está malograda", Reportar},
		{"¿A qué hora cierra la piscina?", Horarios},
		{"cuáles son las normas de la parrilla", Horarios},
		{"Quiero hablar con la administración", HablarAdmin},
		{"necesito un asesor", HablarAdmin},
		{"menu", Menu},
		{"1", Saldo},
		{"4", Reservar},
		{"asdfgh", NoEntendi},
		{"", NoEntendi},
	}
	for _, c := range casos {
		if r := Clasificar(c.texto, hoy); r.Intencion != c.quiero {
			t.Errorf("%q → %s, quiero %s (normalizado %q)", c.texto, r.Intencion, c.quiero, r.Texto)
		}
	}
	// Preguntas del EDIFICIO caen al motor aunque «cuánto» las capture: el saldo de
	// la regla es el personal; estas son cifras agregadas que viven en los golden.
	for _, texto := range []string{
		"¿cuánto queda por cobrar en el edificio?",
		"cuánto ha entrado por el alquiler de las áreas comunes en lo que va del mes",
		"¿cuál es la morosidad del edificio?",
		"¿cuánto hay en la cuenta del edificio?",
		"gastos del mes del edificio",
		"¿cuántos departamentos están morosos en todo el edificio?",
	} {
		if r := Clasificar(texto, hoy); r.Intencion != NoEntendi {
			t.Errorf("%q debería caer al motor (no_entendi), dio %q", texto, r.Intencion)
		}
	}
}

func TestNormalizar(t *testing.T) {
	if n := Normalizar("  ¿Cuánto DEBO?  q tal "); n != "cuanto debo que tal" {
		t.Errorf("normalizado %q", n)
	}
	if n := Normalizar("mañana"); n != "manana" {
		t.Errorf("ñ: %q", n)
	}
}

func TestFechasYAreas(t *testing.T) {
	casos := []struct {
		texto string
		fecha string
		area  string
	}{
		{"reservar parrilla mañana", "2026-09-29", "parrillas"},
		{"el sum pasado mañana", "2026-09-30", "sum"},
		{"piscina el sábado", "2026-10-03", "piscina"},
		{"parrilla el 5/10", "2026-10-05", "parrillas"},
		{"salón 5 de octubre", "2026-10-05", "sum"},
		{"parrilla hoy", "2026-09-28", "parrillas"},
		{"reservar el 2", "2026-10-02", ""},
	}
	for _, c := range casos {
		r := Clasificar(c.texto, hoy)
		f := ""
		if r.Fecha != nil {
			f = r.Fecha.Format("2006-01-02")
		}
		if f != c.fecha || r.Area != c.area {
			t.Errorf("%q → fecha %s área %q; quiero %s %q", c.texto, f, r.Area, c.fecha, c.area)
		}
	}
}
