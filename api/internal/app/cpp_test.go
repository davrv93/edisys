package app_test

import (
	"context"
	"fmt"
	"testing"
)

// Bloque B · proveedores y cuentas por pagar.
func TestProveedoresYCuentasPorPagar(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	ctx := context.Background()

	var rubroID int64
	if err := e.pool.QueryRow(ctx, `SELECT id FROM rubro WHERE edificio_id=1 ORDER BY orden LIMIT 1`).Scan(&rubroID); err != nil {
		t.Fatalf("sin rubro de demo: %v", err)
	}

	// Crear proveedor.
	st, d := e.pedir("POST", "/api/v1/edificios/1/proveedores", tok, map[string]any{
		"razon_social": "Sedapal Demo S.A.", "ruc": "20100070970", "contacto": "Mesa de partes", "telefono": "987654321",
	})
	if st != 201 {
		t.Fatalf("crear proveedor: %d %v", st, d)
	}
	pid := int64(d["id"].(float64))

	// RUC duplicado → 409.
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/proveedores", tok, map[string]any{"razon_social": "Otro", "ruc": "20100070970"}); st != 409 {
		t.Fatalf("RUC duplicado debería dar 409, dio %d", st)
	}
	// Sin razón social → 422.
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/proveedores", tok, map[string]any{"razon_social": "  "}); st != 422 {
		t.Fatalf("razón social vacía debería dar 422, dio %d", st)
	}

	// Listar y buscar.
	st, d = e.pedir("GET", "/api/v1/edificios/1/proveedores?buscar=Sedapal", tok, nil)
	if st != 200 || len(d["datos"].([]any)) < 1 {
		t.Fatalf("buscar proveedor: %d %v", st, d)
	}

	// Crear cuenta por pagar (recibo de agua) con vencimiento.
	st, d = e.pedir("POST", "/api/v1/edificios/1/cuentas-por-pagar", tok, map[string]any{
		"proveedor_id": pid, "rubro_id": rubroID, "descripcion": "Recibo de agua agosto",
		"comprobante_tipo": "recibo", "comprobante_numero": "SUM-4586612",
		"fecha_emision": "2026-08-15", "fecha_vencimiento": "2026-09-15", "monto_cts": 500000,
	})
	if st != 201 {
		t.Fatalf("crear CxP: %d %v", st, d)
	}
	cid := int64(d["id"].(float64))

	// Pago parcial de S/ 2,000 → saldo S/ 3,000 y egreso en el balance (periodo 2026-08).
	st, d = e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/cuentas-por-pagar/%d/pagos", cid), tok, map[string]any{
		"monto_cts": 200000, "fecha": "2026-08-20", "modalidad": "transferencia", "numero_operacion": "OP-1",
	})
	if st != 201 {
		t.Fatalf("pago parcial: %d %v", st, d)
	}
	if saldo := int64(d["saldo_cts"].(float64)); saldo != 300000 {
		t.Fatalf("saldo tras pago parcial = %d, esperado 300000", saldo)
	}
	st, det := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/cuentas-por-pagar/%d", cid), tok, nil)
	if st != 200 || det["cuenta"].(map[string]any)["estado"] != "parcial" {
		t.Fatalf("detalle CxP: %d %v", st, det)
	}
	if eg, _ := det["cuenta"].(map[string]any)["pagado_cts"].(float64); int64(eg) != 200000 {
		t.Fatalf("pagado en la cuenta = %v, esperado 200000", det["cuenta"].(map[string]any)["pagado_cts"])
	}

	_, eg := e.pedir("GET", "/api/v1/edificios/1/egresos?periodo=2026-08", tok, nil)
	encontrado := false
	for _, f := range eg["datos"].([]any) {
		m := f.(map[string]any)
		if int64(m["monto_cts"].(float64)) == 200000 {
			encontrado = true
		}
	}
	if !encontrado {
		t.Fatalf("el pago no generó egreso de S/ 2,000 en 2026-08: %v", eg["datos"])
	}

	// Sobre-pago bloqueado (409).
	if st, _ := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/cuentas-por-pagar/%d/pagos", cid), tok, map[string]any{
		"monto_cts": 400000, "fecha": "2026-08-21", "modalidad": "efectivo",
	}); st != 409 {
		t.Fatalf("sobre-pago debería dar 409, dio %d", st)
	}

	// Pago del saldo → pagado.
	st, d = e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/cuentas-por-pagar/%d/pagos", cid), tok, map[string]any{
		"monto_cts": 300000, "fecha": "2026-08-25", "modalidad": "transferencia", "numero_operacion": "OP-2",
	})
	if st != 201 {
		t.Fatalf("pago final: %d %v", st, d)
	}
	_, det = e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/cuentas-por-pagar/%d", cid), tok, nil)
	if det["cuenta"].(map[string]any)["estado"] != "pagado" {
		t.Fatalf("estado final = %v, esperado pagado", det["cuenta"].(map[string]any)["estado"])
	}

	// Baja lógica del proveedor.
	if st, _ := e.pedir("DELETE", fmt.Sprintf("/api/v1/edificios/1/proveedores/%d", pid), tok, nil); st != 200 {
		t.Fatalf("borrar proveedor: %d", st)
	}
}

func TestPermisosProveedores(t *testing.T) {
	e := nuevo(t)
	operario := e.login("operario@demo.pe")
	junta := e.login("junta@demo.pe")

	// El operario ve proveedores pero no los administra.
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/proveedores", operario, nil); st != 200 {
		t.Fatalf("operario ver proveedores: %d", st)
	}
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/proveedores", operario, map[string]any{"razon_social": "X"}); st != 403 {
		t.Fatalf("operario no debería crear proveedores, dio %d", st)
	}
	// La junta ve cuentas por pagar pero no las registra.
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/cuentas-por-pagar", junta, nil); st != 200 {
		t.Fatalf("junta ver CxP: %d", st)
	}
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/cuentas-por-pagar", junta, map[string]any{}); st != 403 {
		t.Fatalf("junta no debería crear CxP, dio %d", st)
	}
}
