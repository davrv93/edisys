package app_test

import (
	"context"
	"strconv"
	"testing"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// Bloque C · fondos y trazabilidad.
func TestFondosTrazabilidad(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	ctx := context.Background()

	// Los fondos base se crean solos.
	st, d := e.pedir("GET", "/api/v1/edificios/1/fondos", tok, nil)
	if st != 200 || len(d["datos"].([]any)) < 8 {
		t.Fatalf("fondos base: %d %v", st, d)
	}
	fondos := d["datos"].([]any)
	idDe := func(cod string) int64 {
		for _, f := range fondos {
			m := f.(map[string]any)
			if m["codigo"] == cod {
				return int64(m["id"].(float64))
			}
		}
		return 0
	}
	if idDe("cuota") == 0 || idDe("general") == 0 {
		t.Fatal("faltan fondos cuota/general")
	}

	// Un recibo con saldo del periodo de la semilla.
	_, l := e.pedir("GET", "/api/v1/edificios/1/recibos?periodo=2026-09", tok, nil)
	var rid, saldo int64
	for _, it := range l["datos"].([]any) {
		m := it.(map[string]any)
		if s, ok := m["saldo_cts"].(float64); ok && int64(s) > 0 {
			rid, saldo = int64(m["id"].(float64)), int64(s)
			break
		}
	}
	if rid == 0 {
		t.Fatal("sin recibo con saldo en la semilla")
	}
	pago := int64(100000) // S/ 1.000
	if pago > saldo {
		pago = saldo
	}

	// Pago validado por el administrador → asiento de ingreso repartido entre fondos.
	st, _ = e.pedir("POST", "/api/v1/edificios/1/recibos/"+itoa(rid)+"/pagos", tok, map[string]any{
		"monto_cts": pago, "medio": "efectivo", "fecha": "2026-09-30",
	})
	if st != 201 {
		t.Fatalf("registrar pago: %d", st)
	}

	st, tr := e.pedir("GET", "/api/v1/edificios/1/fondos/trazabilidad?desde=2026-09&hasta=2026-09", tok, nil)
	if st != 200 {
		t.Fatalf("trazabilidad: %d %v", st, tr)
	}
	if ing := int64(tr["ingreso_cts"].(float64)); ing != pago {
		t.Fatalf("ingreso total por fondos = %d, esperado %d", ing, pago)
	}

	// Transferencia entre fondos: el saldo global no cambia.
	cuota, general := idDe("cuota"), idDe("general")
	st, _ = e.pedir("POST", "/api/v1/edificios/1/fondos/transferencia", tok, map[string]any{
		"origen_id": cuota, "destino_id": general, "monto_cts": 50000, "fecha": "2026-09-30", "motivo": "prueba",
	})
	if st != 201 {
		t.Fatalf("transferencia: %d", st)
	}
	st, tr2 := e.pedir("GET", "/api/v1/edificios/1/fondos/trazabilidad?desde=2026-09&hasta=2026-09", tok, nil)
	if st != 200 {
		t.Fatalf("trazabilidad 2: %d", st)
	}
	if int64(tr2["saldo_cts"].(float64)) != int64(tr["saldo_cts"].(float64)) {
		t.Fatalf("la transferencia cambió el saldo global: %v -> %v", tr["saldo_cts"], tr2["saldo_cts"])
	}

	// Movimiento manual egreso baja el saldo del fondo.
	st, _ = e.pedir("POST", "/api/v1/edificios/1/fondos/"+itoa(cuota)+"/movimientos", tok, map[string]any{
		"monto_cts": 10000, "tipo": "egreso", "fecha": "2026-09-30", "descripcion": "ajuste",
	})
	if st != 201 {
		t.Fatalf("movimiento manual: %d", st)
	}
	_ = ctx
}

// Un egreso aparece en el fondo de su rubro y sube el total de egresos.
func TestEgresoAsientaEnFondo(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	ctx := context.Background()
	var rubroID int64
	if err := e.pool.QueryRow(ctx, `SELECT id FROM rubro WHERE edificio_id=1 ORDER BY orden LIMIT 1`).Scan(&rubroID); err != nil {
		t.Fatal(err)
	}
	_, tr0 := e.pedir("GET", "/api/v1/edificios/1/fondos/trazabilidad?desde=2026-09&hasta=2026-09", tok, nil)
	base := int64(tr0["egreso_cts"].(float64))

	st, _ := e.pedir("POST", "/api/v1/edificios/1/egresos", tok, map[string]any{
		"rubro_id": rubroID, "descripcion": "Prueba de fondo", "monto_cts": 25000, "fecha": "2026-09-15",
	})
	if st != 201 {
		t.Fatalf("crear egreso: %d", st)
	}
	_, tr := e.pedir("GET", "/api/v1/edificios/1/fondos/trazabilidad?desde=2026-09&hasta=2026-09", tok, nil)
	if got := int64(tr["egreso_cts"].(float64)); got != base+25000 {
		t.Fatalf("egreso por fondos = %d, esperado %d", got, base+25000)
	}
}

func TestPermisosFondos(t *testing.T) {
	e := nuevo(t)
	junta := e.login("junta@demo.pe")
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/fondos", junta, nil); st != 200 {
		t.Fatalf("junta ver fondos: %d", st)
	}
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/fondos", junta, map[string]any{"nombre": "X"}); st != 403 {
		t.Fatalf("junta no debería crear fondos, dio %d", st)
	}
}
