package app_test

import "testing"

// Bloque B4 · recibos e ingresos externos.
func TestExternos(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")

	st, d := e.pedir("POST", "/api/v1/edificios/1/recibos-externos", tok, map[string]any{
		"concepto": "Alquiler de salón a tercero", "tercero": "Academia XYZ", "monto_cts": 30000, "fecha_emision": "2026-09-05", "fecha_vencimiento": "2026-09-20",
	})
	if st != 201 {
		t.Fatalf("crear recibo externo: %d %v", st, d)
	}
	rid := int64(d["id"].(float64))

	st, l := e.pedir("GET", "/api/v1/edificios/1/recibos-externos?periodo=2026-09", tok, nil)
	if st != 200 || len(l["datos"].([]any)) < 1 {
		t.Fatalf("listar recibos externos: %d %v", st, l)
	}

	if st, _ := e.pedir("POST", "/api/v1/edificios/1/recibos-externos/"+itoa(rid)+"/pagar", tok, nil); st != 200 {
		t.Fatalf("pagar recibo externo: %d", st)
	}

	// Ingreso externo a un fondo: entra en la trazabilidad.
	_, fondos := e.pedir("GET", "/api/v1/edificios/1/fondos", tok, nil)
	fid := int64(fondos["datos"].([]any)[0].(map[string]any)["id"].(float64))
	_, tr0 := e.pedir("GET", "/api/v1/edificios/1/fondos/trazabilidad?desde=2026-09&hasta=2026-09", tok, nil)
	base := int64(tr0["ingreso_cts"].(float64))

	st, d = e.pedir("POST", "/api/v1/edificios/1/ingresos-externos", tok, map[string]any{
		"descripcion": "Venta de reciclaje", "monto_cts": 15000, "fecha": "2026-09-10", "fondo_id": fid,
	})
	if st != 201 {
		t.Fatalf("crear ingreso externo: %d %v", st, d)
	}
	_, tr := e.pedir("GET", "/api/v1/edificios/1/fondos/trazabilidad?desde=2026-09&hasta=2026-09", tok, nil)
	if got := int64(tr["ingreso_cts"].(float64)); got != base+15000 {
		t.Fatalf("el ingreso externo no entró al fondo: base %d, ahora %d", base, got)
	}
}

func TestPermisosExternos(t *testing.T) {
	e := nuevo(t)
	junta := e.login("junta@demo.pe")
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/recibos-externos?periodo=2026-09", junta, nil); st != 200 {
		t.Fatalf("junta ver externos: %d", st)
	}
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/recibos-externos", junta, map[string]any{"concepto": "x", "monto_cts": 100}); st != 403 {
		t.Fatalf("junta no debería registrar externos, dio %d", st)
	}
}
