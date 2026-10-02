package app_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/xuri/excelize/v2"
)

// Bloque A4 · cobranza sin identificar y su imputación.
func TestCobranzaSinIdentificar(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	ctx := context.Background()

	st, d := e.pedir("POST", "/api/v1/edificios/1/cobranzas-sin-identificar", tok, map[string]any{
		"monto_cts": 8000, "medio": "transferencia", "codigo_operacion": "SIN-1", "fecha": "2026-09-30", "descripcion": "Depósito sin código",
	})
	if st != 201 {
		t.Fatalf("crear cobranza: %d %v", st, d)
	}
	id := int64(d["id"].(float64))

	var uid int64
	if err := e.pool.QueryRow(ctx, `SELECT r.unidad_id FROM recibo r WHERE r.edificio_id=1 AND r.estado IN ('emitido','pagado_parcial') AND r.total_cts>r.pagado_cts ORDER BY r.unidad_id LIMIT 1`).Scan(&uid); err != nil {
		t.Fatalf("sin unidad con deuda: %v", err)
	}
	st, d = e.pedir("POST", "/api/v1/edificios/1/cobranzas-sin-identificar/"+itoa(id)+"/imputar", tok, map[string]any{"unidad_id": uid})
	if st != 200 {
		t.Fatalf("imputar cobranza: %d %v", st, d)
	}
	if d["estado"] != "imputada" {
		t.Fatalf("estado tras imputar = %v", d["estado"])
	}
	// Imputar dos veces → 409.
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/cobranzas-sin-identificar/"+itoa(id)+"/imputar", tok, map[string]any{"unidad_id": uid}); st != 409 {
		t.Fatalf("imputar dos veces debería dar 409, dio %d", st)
	}
}

func TestDevolverCobranza(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	st, d := e.pedir("POST", "/api/v1/edificios/1/cobranzas-sin-identificar", tok, map[string]any{
		"monto_cts": 3000, "medio": "efectivo", "fecha": "2026-09-30", "descripcion": "No corresponde",
	})
	if st != 201 {
		t.Fatalf("crear cobranza: %d", st)
	}
	id := int64(d["id"].(float64))
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/cobranzas-sin-identificar/"+itoa(id)+"/devolver", tok, map[string]any{"motivo": "Pago equivocado"}); st != 200 {
		t.Fatalf("devolver: %d", st)
	}
	if st, dl := e.pedir("GET", "/api/v1/edificios/1/devoluciones", tok, nil); st != 200 || len(dl["datos"].([]any)) < 1 {
		t.Fatalf("listar devoluciones: %d %v", st, dl)
	}
}

// Bloque A5 · importar presupuesto por Excel.
func TestImportarPresupuestoExcel(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")

	// Necesitamos un periodo abierto para 2026-09 (lo crea la semilla). Un rubro real del edificio.
	var rubro string
	if err := e.pool.QueryRow(context.Background(), `SELECT nombre FROM rubro WHERE edificio_id=1 ORDER BY orden LIMIT 1`).Scan(&rubro); err != nil {
		t.Fatalf("sin rubro: %v", err)
	}

	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "rubro")
	_ = f.SetCellValue("Sheet1", "B1", "monto")
	_ = f.SetCellValue("Sheet1", "A2", rubro)
	_ = f.SetCellValue("Sheet1", "B2", "1.234,56")
	var xbuf bytes.Buffer
	if err := f.Write(&xbuf); err != nil {
		t.Fatal(err)
	}
	f.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("archivo", "presupuesto.xlsx")
	_, _ = fw.Write(xbuf.Bytes())
	mw.Close()

	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/edificios/1/periodos/2026-09/presupuesto/importar", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	st, d := e.hacer(req)
	if st != 200 {
		t.Fatalf("importar presupuesto: %d %v", st, d)
	}
	if n := int(d["actualizados"].(float64)); n < 1 {
		t.Fatalf("sin filas actualizadas: %v", d)
	}
}
