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
