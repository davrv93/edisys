package app_test

import (
	"context"
	"testing"
)

// Bloque A3 · cuentas bancarias y voucher multicuenta.
func TestVoucherMulticuenta(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	ctx := context.Background()

	// Cuenta bancaria.
	st, d := e.pedir("POST", "/api/v1/edificios/1/cuentas-bancarias", tok, map[string]any{"banco": "BCP", "numero": "194-0001", "moneda": "PEN"})
	if st != 201 {
		t.Fatalf("crear cuenta bancaria: %d %v", st, d)
	}
	cuentaID := int64(d["id"].(float64))
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/cuentas-bancarias", tok, nil); st != 200 {
		t.Fatalf("listar cuentas: %d", st)
	}

	// Un recibo con saldo de la semilla.
	var uid, rid, saldo int64
	if err := e.pool.QueryRow(ctx, `SELECT r.unidad_id, r.id, r.total_cts - r.pagado_cts
		FROM recibo r WHERE r.edificio_id=1 AND r.estado IN ('emitido','pagado_parcial') AND r.total_cts > r.pagado_cts
		ORDER BY r.unidad_id, r.id LIMIT 1`).Scan(&uid, &rid, &saldo); err != nil {
		t.Fatalf("sin recibo con saldo: %v", err)
	}
	monto := int64(5000)
	if monto > saldo {
		monto = saldo
	}

	st, d = e.pedir("POST", "/api/v1/edificios/1/unidades/"+itoa(uid)+"/vouchers", tok, map[string]any{
		"cuenta_bancaria_id": cuentaID, "medio": "transferencia", "codigo_operacion": "VCH-001", "fecha": "2026-09-30",
		"aplicaciones": []map[string]any{{"recibo_id": rid, "monto_cts": monto}},
	})
	if st != 201 {
		t.Fatalf("registrar voucher: %d %v", st, d)
	}
	if d["estado"] != "validado" {
		t.Fatalf("el voucher del admin debe quedar validado, quedó %v", d["estado"])
	}

	var n int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM pago WHERE cuenta_bancaria_id=$1`, cuentaID).Scan(&n); err != nil || n < 1 {
		t.Fatalf("el pago no quedó ligado a la cuenta (%d, %v)", n, err)
	}
	// El pago validado asienta en fondos.
	var movs int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM fondo_movimiento WHERE origen='pago' AND edificio_id=1`).Scan(&movs); err != nil || movs < 1 {
		t.Fatalf("sin asiento en fondos (%d, %v)", movs, err)
	}

	// Código de operación repetido → 409.
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/unidades/"+itoa(uid)+"/vouchers", tok, map[string]any{
		"cuenta_bancaria_id": cuentaID, "medio": "transferencia", "codigo_operacion": "VCH-001", "fecha": "2026-09-30",
		"aplicaciones": []map[string]any{{"recibo_id": rid, "monto_cts": 1}},
	}); st != 409 {
		t.Fatalf("código de operación repetido debería dar 409, dio %d", st)
	}
}

func TestPermisosCuentasBancarias(t *testing.T) {
	e := nuevo(t)
	junta := e.login("junta@demo.pe")
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/cuentas-bancarias", junta, nil); st != 200 {
		t.Fatalf("junta ver cuentas: %d", st)
	}
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/cuentas-bancarias", junta, map[string]any{"banco": "X"}); st != 403 {
		t.Fatalf("junta no debería crear cuentas, dio %d", st)
	}
}
