package app_test

// Pruebas de los pendientes funcionales (docs/PLAN_PENDIENTES_FUNCIONALES.md), bloque por bloque.

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"edisys/api/internal/app"
	P "edisys/api/internal/plataforma"
)

// padronConDeuda: el padrón demo más la hoja «Deuda» con las filas dadas (codigo, periodo, monto en soles).
func padronConDeuda(t *testing.T, deudas [][3]any) []byte {
	f, err := excelize.OpenReader(bytes.NewReader(padronExcel(t, nil)))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.NewSheet("Deuda")
	for i, c := range []string{"codigo", "periodo", "monto"} {
		celda, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue("Deuda", celda, c)
	}
	for i, d := range deudas {
		for j, v := range d {
			celda, _ := excelize.CoordinatesToCellName(j+1, i+2)
			_ = f.SetCellValue("Deuda", celda, v)
		}
	}
	var b bytes.Buffer
	if err := f.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func (e *entorno) importarConDeuda(tok string, deudas [][3]any) {
	e.t.Helper()
	st, r := e.subirExcel(tok, padronConDeuda(e.t, deudas))
	if st != 200 || r["bloqueante"] != false {
		e.t.Fatalf("vista previa con deuda: %d %v", st, r)
	}
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/importaciones/%d/confirmar", num(r["importacion_id"])), tok, nil); st != 200 || num(d["deudas_cargadas"]) != int64(len(deudas)) {
		e.t.Fatalf("confirmar con deuda: %d %v", st, d)
	}
}

// ---------- Bloque 1 · deuda inicial en la morosidad ----------

func TestRepartirDelMasAntiguo(t *testing.T) {
	got := app.Repartir(50000, []int64{70000, 50000, 99000})
	if got[0] != 50000 || got[1] != 0 || got[2] != 0 {
		t.Errorf("500 sobre 700/500/990: %v", got)
	}
	got = app.Repartir(130000, []int64{70000, 50000, 99000})
	if got[0] != 70000 || got[1] != 50000 || got[2] != 10000 {
		t.Errorf("1300 sobre 700/500/990: %v", got)
	}
}

// El 201 (al día en setiembre) con S/ 1.200 de deuda de 2023: moroso, no reserva, el chatbot y el portal
// le dicen cuánto debe; la morosidad «del mes» sigue en 13,1 % y la «histórica» la incluye.
// Un pago de S/ 500 reduce primero lo más antiguo.
func TestDeudaInicialEnMorosidad(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	e.importarConDeuda(tok, [][3]any{{"201", "2023-05", 700}, {"201", "2023-08", 500}})
	u201 := e.idUnidad("201")

	_, c := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/unidades/%d/cuenta", u201), tok, nil)
	if num(c["deuda_cts"]) != 120000 || num(c["deuda_vencida_cts"]) != 120000 || c["moroso"] != true {
		t.Fatalf("cuenta del 201: %v", c)
	}
	// Morosidad: del mes igual, histórica con la deuda inicial.
	_, b := e.pedir("GET", "/api/v1/edificios/1/balance?periodo=2026-09", tok, nil)
	m := b["kpis"].(map[string]any)["morosidad"].(map[string]any)
	if m["pct"].(float64) != 13.1 || num(m["monto_cts"]) != 294000 {
		t.Errorf("morosidad del mes cambió: %v", m)
	}
	if num(m["historica_monto_cts"]) != 414000 || num(m["deuda_inicial_cts"]) != 120000 || m["historica_pct"].(float64) != 18.5 || num(m["historica_unidades"]) != 4 {
		t.Errorf("morosidad histórica: %v", m)
	}
	// El listado de periodos no muestra los históricos.
	_, ps := e.pedir("GET", "/api/v1/edificios/1/periodos", tok, nil)
	for _, p := range ps["datos"].([]any) {
		if strings.HasPrefix(p.(map[string]any)["periodo"].(string), "2023") {
			t.Errorf("periodo histórico listado: %v", p)
		}
	}
	// No puede reservar (el propietario desde la app).
	tokM := e.login("propietario201@demo.pe")
	var recurso int64
	_ = e.pool.QueryRow(context.Background(), `SELECT id FROM recurso WHERE nombre='Parrilla 2'`).Scan(&recurso)
	dia := time.Now().In(P.Lima).AddDate(0, 0, 12).Format("2006-01-02")
	ini, _ := time.ParseInLocation("2006-01-02 15:04", dia+" 12:00", P.Lima)
	st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tokM, map[string]any{"recurso_id": recurso, "inicio": ini.Format(time.RFC3339),
		"fin": ini.Add(5 * time.Hour).Format(time.RFC3339), "acepta_normas": true})
	if st != 403 || codigo(d) != "MOROSO" || num(d["error"].(map[string]any)["monto_cts"]) != 120000 {
		t.Errorf("reserva del 201 con deuda inicial: %d %v", st, d)
	}
	// El chatbot le dice cuánto debe.
	_, c = e.pedir("POST", "/api/v1/chatbot/mensaje", tok, map[string]string{"telefono": "51900000201", "texto": "¿cuánto debo?"})
	if num(c["datos"].(map[string]any)["deuda_cts"]) != 120000 || !strings.Contains(c["respuesta"].(string), "S/ 1.200,00") ||
		!strings.Contains(c["respuesta"].(string), "deuda anterior") {
		t.Errorf("chatbot cuánto debo: %v", c)
	}
	// El portal del propietario.
	_, pt := e.pedir("GET", "/api/v1/edificios/1/portal", tokM, nil)
	if num(pt["deuda"].(map[string]any)["total_cts"]) != 120000 {
		t.Errorf("portal: %v", pt["deuda"])
	}
	// Antigüedad: más de 2 años.
	_, mo := e.pedir("GET", "/api/v1/edificios/1/morosidad?periodo=2026-09", tok, nil)
	hallado := false
	for _, x := range mo["unidades"].([]any) {
		u := x.(map[string]any)
		if u["unidad"] == "201" {
			hallado = true
			if num(u["antiguedad_dias"]) < 700 || num(u["deuda_inicial_cts"]) != 120000 {
				t.Errorf("201 en morosidad: %v", u)
			}
		}
	}
	if !hallado {
		t.Errorf("el 201 no sale en la morosidad: %v", mo["unidades"])
	}
	// Pago de S/ 500: primero lo más antiguo (mayo 2023).
	st, p := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/unidades/%d/pagos", u201), tok, map[string]any{"monto_cts": 50000, "medio": "yape", "codigo_operacion": "55501", "fecha": "2026-09-28"})
	if st != 201 {
		t.Fatalf("pago a cuenta: %d %v", st, p)
	}
	apl := p["aplicaciones"].([]any)
	if len(apl) != 1 || apl[0].(map[string]any)["periodo"] != "2023-05" || num(apl[0].(map[string]any)["saldo_cts"]) != 20000 {
		t.Errorf("aplicación: %v", apl)
	}
	_, c = e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/unidades/%d/cuenta", u201), tok, nil)
	if num(c["deuda_cts"]) != 70000 {
		t.Errorf("deuda tras pagar 500: %v", c)
	}
	// Un pago que cruza cargos se parte en dos con el mismo código.
	st, p = e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/unidades/%d/pagos", u201), tok, map[string]any{"monto_cts": 30000, "medio": "yape", "codigo_operacion": "55502", "fecha": "2026-09-28"})
	if st != 201 || len(p["aplicaciones"].([]any)) != 2 {
		t.Errorf("pago que cruza cargos: %d %v", st, p)
	}
	// Más que la deuda: 422. El mismo código otra vez: 409.
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/unidades/%d/pagos", u201), tok, map[string]any{"monto_cts": 99999, "medio": "efectivo"}); st != 422 || codigo(d) != "MONTO_MAYOR_AL_SALDO" {
		t.Errorf("pago mayor a la deuda: %d %v", st, d)
	}
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/unidades/%d/pagos", u201), tok, map[string]any{"monto_cts": 1000, "medio": "yape", "codigo_operacion": "55501", "fecha": "2026-09-28"}); st != 409 || codigo(d) != "PAGO_DUPLICADO" {
		t.Errorf("código repetido: %d %v", st, d)
	}
	// Lo cobrado entra en el balance del mes del pago como «Deuda anterior recuperada».
	_, b = e.pedir("GET", "/api/v1/edificios/1/balance/nodos/ing?periodo=2026-09", tok, nil)
	var recuperada int64
	for _, x := range b["lista"].([]any) {
		if n := x.(map[string]any); n["id"] == "ing.deuda" {
			recuperada = num(n["total_cts"])
		}
	}
	if recuperada != 80000 {
		t.Errorf("deuda recuperada en setiembre: %d", recuperada)
	}
}
