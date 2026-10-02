package app_test

import "testing"

// Bloque C · informes económicos y consumos.
func TestInformesEconomicoYConsumos(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")

	st, d := e.pedir("GET", "/api/v1/edificios/1/informes/economico?periodo=2026-09", tok, nil)
	if st != 200 {
		t.Fatalf("informe económico: %d %v", st, d)
	}
	if _, ok := d["resumen"].(map[string]any); !ok {
		t.Fatalf("sin resumen: %v", d)
	}
	if len(d["flujo"].([]any)) != 12 {
		t.Fatalf("el flujo debe tener 12 meses, tiene %d", len(d["flujo"].([]any)))
	}

	st, c := e.pedir("GET", "/api/v1/edificios/1/informes/consumos?hasta=2026-09", tok, nil)
	if st != 200 {
		t.Fatalf("consumos: %d %v", st, c)
	}
	if _, ok := c["unidades"].([]any); !ok {
		t.Fatalf("sin unidades: %v", c)
	}
}
