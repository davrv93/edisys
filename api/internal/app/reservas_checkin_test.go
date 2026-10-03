package app

import (
	"testing"
	"time"

	P "edisys/api/internal/plataforma"
)

// Bloque H1/H3 · reglas puras sin base: ventana de ingreso, lectura del QR y horario por día.

func TestVentanaCheckin(t *testing.T) {
	ini := time.Date(2026, 10, 10, 18, 0, 0, 0, P.Lima)
	fin := ini.Add(5 * time.Hour)
	casos := []struct {
		ahora  time.Time
		quiero string
	}{
		{ini.Add(-31 * time.Minute), "ANTES_DE_FRANJA"},
		{ini.Add(-30 * time.Minute), ""}, // justo en la tolerancia
		{ini.Add(2 * time.Hour), ""},
		{fin.Add(-time.Second), ""},
		{fin, "FRANJA_TERMINADA"},
		{ini.AddDate(0, 0, -3), "ANTES_DE_FRANJA"},
	}
	for _, c := range casos {
		m := ventanaCheckin(ini, fin, 30, c.ahora)
		got := ""
		if m != nil {
			got = m.Codigo
		}
		if got != c.quiero {
			t.Errorf("ahora %s: %q, quiero %q", c.ahora.Format("02/01 15:04:05"), got, c.quiero)
		}
	}
}

func TestNormalizarCodigoQR(t *testing.T) {
	casos := map[string][2]string{
		"EDISYS-R:ABCDEF0123":    {"abcdef0123", ""},
		"  edisys-r:abc  ":       {"abc", ""},
		"r-0415":                 {"", "R-0415"},
		"EDISYS-R:R-0415":        {"", "R-0415"},
		"0f0f0f0f0f0f0f0f0f0f0f": {"0f0f0f0f0f0f0f0f0f0f0f", ""},
	}
	for in, q := range casos {
		tok, cod := normalizarCodigoQR(in)
		if tok != q[0] || cod != q[1] {
			t.Errorf("%q → (%q, %q), quiero %v", in, tok, cod, q)
		}
	}
}

func TestFranjasParaDia(t *testing.T) {
	base := []Franja{{Inicio: "12:00", Fin: "17:00"}, {Inicio: "18:00", Fin: "23:00"}}
	t5 := int64(5000)
	hs := map[string][]Franja{"6": {{Inicio: "10:00", Fin: "14:00", TarifaCts: &t5}}, "7": {}}
	vie := time.Date(2026, 10, 9, 0, 0, 0, 0, P.Lima) // viernes → franjas generales
	sab := vie.AddDate(0, 0, 1)
	dom := vie.AddDate(0, 0, 2)
	if fs := franjasParaDia(vie, base, hs); len(fs) != 2 || fs[0].TarifaCts != nil {
		t.Errorf("viernes: %v", fs)
	}
	if fs := franjasParaDia(sab, base, hs); len(fs) != 1 || fs[0].Ini.Hour() != 10 || *fs[0].TarifaCts != 5000 {
		t.Errorf("sábado: %v", fs)
	}
	if fs := franjasParaDia(dom, base, hs); len(fs) != 0 {
		t.Errorf("domingo cerrado: %v", fs)
	}
	if err := validarHorarios(map[string][]Franja{"0": {}}); err == nil {
		t.Error("el día 0 no existe")
	}
}
