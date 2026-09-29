package app_test

// Pruebas de los pendientes funcionales (docs/PLAN_PENDIENTES_FUNCIONALES.md), bloque por bloque.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"

	"edisys/api/internal/app"
	P "edisys/api/internal/plataforma"
	"edisys/api/internal/reparto"
	"edisys/api/internal/sunat"
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

// «Solo morosos» se filtra en el API (deuda vencida, la misma que bloquea reservas),
// no en la página cargada: el total y la paginación ya vienen filtrados.
func TestUnidadesFiltroMorosos(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	_, todas := e.pedir("GET", "/api/v1/edificios/1/unidades?por_pagina=200", tok, nil)
	if num(todas["total"]) != 24 {
		t.Fatalf("total sin filtro: %v", todas["total"])
	}
	st, m := e.pedir("GET", "/api/v1/edificios/1/unidades?morosos=1&por_pagina=200", tok, nil)
	if st != 200 {
		t.Fatalf("filtro morosos: %d %v", st, m)
	}
	if num(m["total"]) != 3 {
		t.Fatalf("morosos de la semilla: %v", m["total"])
	}
	cods := map[string]bool{}
	for _, x := range m["datos"].([]any) {
		u := x.(map[string]any)
		if u["moroso"] != true {
			t.Errorf("no moroso en el filtro: %v", u["codigo"])
		}
		cods[u["codigo"].(string)] = true
	}
	for _, c := range []string{"402", "503", "104"} {
		if !cods[c] {
			t.Errorf("falta el moroso %s: %v", c, cods)
		}
	}
	// Con deuda inicial, el 201 también sale.
	e.importarConDeuda(tok, [][3]any{{"201", "2023-05", 700}})
	_, m2 := e.pedir("GET", "/api/v1/edificios/1/unidades?morosos=1&por_pagina=200", tok, nil)
	if num(m2["total"]) != 4 {
		t.Errorf("con deuda inicial: %v", m2["total"])
	}
}

// ---------- utilidades multipart ----------

func pngChico() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	return b.Bytes()
}

// multipartPedir envía un formulario con campos y, opcionalmente, un archivo.
func (e *entorno) multipartPedir(metodo, ruta, tok string, campos map[string]string, campoArchivo, nombre string, datos []byte) (int, map[string]any) {
	e.t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for k, v := range campos {
		_ = w.WriteField(k, v)
	}
	if campoArchivo != "" {
		fw, _ := w.CreateFormFile(campoArchivo, nombre)
		_, _ = fw.Write(datos)
	}
	_ = w.Close()
	req, _ := http.NewRequest(metodo, e.srv.URL+ruta, &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	return e.hacer(req)
}

// ---------- Bloque 2 · lecturas en cascada ----------

func (e *entorno) lectura(unidad, periodo string) (id int64, valor, anterior string) {
	e.t.Helper()
	if err := e.pool.QueryRow(context.Background(), `SELECT l.id, l.valor::text, l.anterior::text FROM lectura l JOIN medidor m ON m.id=l.medidor_id
		JOIN periodo p ON p.id=l.periodo_id WHERE m.serie=$1 AND p.periodo=$2`, "AG-"+unidad, periodo).Scan(&id, &valor, &anterior); err != nil {
		e.t.Fatalf("lectura %s %s: %v", unidad, periodo, err)
	}
	return
}

// abrirOctubre: abre 2026-10, lee los 24 medidores (mismo consumo que setiembre), registra el recibo general
// de S/ 5.000 y aprueba el reparto (5.000 / 4.800 / 200). genera=true deja los borradores de octubre.
func (e *entorno) abrirOctubre(tok string, genera bool) {
	e.t.Helper()
	if st, d := e.pedir("POST", "/api/v1/edificios/1/periodos", tok, map[string]any{"periodo": "2026-10"}); st != 201 {
		e.t.Fatalf("abrir octubre: %d %v", st, d)
	}
	filas, _ := e.pool.Query(context.Background(), `SELECT m.id, u.codigo, l.valor::text FROM medidor m JOIN unidad u ON u.id=m.unidad_id
		JOIN lectura l ON l.medidor_id=m.id JOIN periodo p ON p.id=l.periodo_id WHERE p.periodo='2026-09' ORDER BY u.codigo`)
	type med struct {
		id         int64
		cod, valor string
	}
	var ms []med
	for filas.Next() {
		var m med
		_ = filas.Scan(&m.id, &m.cod, &m.valor)
		ms = append(ms, m)
	}
	filas.Close()
	for _, m := range ms {
		v, _ := P.Milesimas(m.valor)
		nuevo := P.TextoMilesimas(v + reparto.ConsumoSetiembreDemo[m.cod])
		if st, d := e.multipartPedir("POST", fmt.Sprintf("/api/v1/edificios/1/medidores/%d/lecturas", m.id), tok,
			map[string]string{"periodo": "2026-10", "valor": nuevo}, "foto", "m.png", pngChico()); st != 201 {
			e.t.Fatalf("lectura octubre %s: %d %v", m.cod, st, d)
		}
	}
	if st, d := e.multipartPedir("POST", "/api/v1/edificios/1/periodos/2026-10/recibo-general", tok,
		map[string]string{"monto_cts": "500000", "consumo_total": "357.143"}, "foto_recibo", "sedapal.png", pngChico()); st != 201 {
		e.t.Fatalf("recibo general octubre: %d %v", st, d)
	}
	if st, d := e.pedir("POST", "/api/v1/edificios/1/periodos/2026-10/reparto-medidores/aprobar", tok, nil); st != 200 || num(d["diferencia_cts"]) != 20000 {
		e.t.Fatalf("reparto octubre: %d %v", st, d)
	}
	if genera {
		if st, d := e.pedir("POST", "/api/v1/edificios/1/periodos/2026-10/recibos/generar", tok, nil); st != 200 {
			e.t.Fatalf("generar octubre: %d %v", st, d)
		}
	}
}

func sumaLineas(ls []any) int64 {
	var s int64
	for _, l := range ls {
		s += num(l.(map[string]any)["total_cts"])
	}
	return s
}

// Setiembre emitido y octubre sin recibos: corregir setiembre del 201 recalcula octubre (anterior y consumo),
// rehace los dos repartos (siguen sumando S/ 5.000 al céntimo) y deja ajustes que entran al generar octubre.
func TestLecturaEnCascadaConAjustes(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	e.abrirOctubre(tok, false)
	lid, valor, _ := e.lectura("201", "2026-09")
	_, valorOct, _ := e.lectura("201", "2026-10")
	v, _ := P.Milesimas(valor)
	corregido := P.TextoMilesimas(v + 2000) // +2 m³
	// Sin motivo: 422.
	if st, _ := e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/lecturas/%d", lid), tok, map[string]any{"valor": corregido}); st != 422 {
		t.Errorf("sin motivo: %d", st)
	}
	st, d := e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/lecturas/%d", lid), tok, map[string]any{"valor": corregido, "motivo": "El operario leyó mal un dígito"})
	if st != 200 {
		t.Fatalf("corregir: %d %v", st, d)
	}
	// Octubre: la anterior es la corregida y su consumo baja 2 m³.
	_, vo, ao := e.lectura("201", "2026-10")
	if vo != valorOct || ao != corregido || d["siguiente"].(map[string]any)["consumo"] != "12.000" {
		t.Errorf("octubre no se recalculó: valor %s anterior %s (quiero %s) · %v", vo, ao, corregido, d["siguiente"])
	}
	reps := d["repartos"].([]any)
	if len(reps) != 2 {
		t.Fatalf("repartos rehechos: %v", reps)
	}
	for _, x := range reps {
		rp := x.(map[string]any)
		if num(rp["suma_lineas_cts"]) != 500000 || num(rp["total_unidades_cts"])+num(rp["diferencia_cts"]) != 500000 {
			t.Errorf("el reparto de %v no suma S/ 5.000: %v", rp["periodo"], rp)
		}
	}
	sept := reps[0].(map[string]any)
	if sept["periodo"] != "2026-09" || sept["emitido"] != true || num(sept["suma_ajustes_cts"]) != 0 || num(sept["ajustes"]) == 0 {
		t.Errorf("setiembre emitido debería dar ajustes que suman 0: %v", sept)
	}
	// Los recibos emitidos de setiembre no cambian.
	var total201 int64
	_ = e.pool.QueryRow(context.Background(), `SELECT r.total_cts FROM recibo r JOIN unidad u ON u.id=r.unidad_id JOIN periodo p ON p.id=r.periodo_id WHERE u.codigo='201' AND p.periodo='2026-09'`).Scan(&total201)
	if total201 != 99000 {
		t.Errorf("el recibo emitido de setiembre del 201 cambió: %d", total201)
	}
	var cargo201 int64
	for _, x := range d["ajustes"].([]any) {
		a := x.(map[string]any)
		if a["unidad"] == "201" {
			cargo201 = num(a["monto_cts"])
		}
	}
	if cargo201 <= 0 {
		t.Errorf("el 201 consumió más: debería tener nota de cargo, tiene %d", cargo201)
	}
	// Al generar octubre, cada ajuste entra en el recibo de su unidad.
	if st, g := e.pedir("POST", "/api/v1/edificios/1/periodos/2026-10/recibos/generar", tok, nil); st != 200 {
		t.Fatalf("generar octubre: %d %v", st, g)
	}
	var enRecibo int64
	_ = e.pool.QueryRow(context.Background(), `SELECT rl.monto_cts FROM recibo_linea rl JOIN recibo r ON r.id=rl.recibo_id JOIN unidad u ON u.id=r.unidad_id
		JOIN periodo p ON p.id=r.periodo_id WHERE u.codigo='201' AND p.periodo='2026-10' AND rl.tipo='ajuste'`).Scan(&enRecibo)
	if enRecibo != cargo201 {
		t.Errorf("ajuste en el recibo de octubre: %d, quiero %d", enRecibo, cargo201)
	}
	_, pend := e.pedir("GET", "/api/v1/edificios/1/ajustes?pendientes=1", tok, nil)
	if num(pend["total"]) != 0 {
		t.Errorf("quedaron ajustes pendientes: %v", pend)
	}
	// Queda en la auditoría.
	var aud int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM auditoria WHERE modulo='lecturas' AND accion='corregir' AND entidad_id=$1`, fmt.Sprint(lid)).Scan(&aud)
	if aud != 1 {
		t.Errorf("auditoría de la corrección: %d", aud)
	}
	// Menor que la anterior: exige confirmar (cambio de medidor).
	st, d = e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/lecturas/%d", lid), tok, map[string]any{"valor": "1.000", "motivo": "cambio"})
	if st != 422 || codigo(d) != "CONSUMO_NEGATIVO" {
		t.Errorf("negativo sin confirmar: %d %v", st, d)
	}
}

// Con los borradores de octubre ya generados: la corrección de setiembre (emitido) aplica los ajustes
// directo en esos borradores y reescribe el agua de octubre; todo sigue cuadrando al céntimo.
func TestLecturaEnCascadaConBorradores(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	e.abrirOctubre(tok, true)
	var antes int64
	_ = e.pool.QueryRow(context.Background(), `SELECT sum(rl.monto_cts) FROM recibo_linea rl JOIN recibo r ON r.id=rl.recibo_id JOIN periodo p ON p.id=r.periodo_id
		WHERE p.periodo='2026-10' AND rl.tipo IN ('agua','agua_comun')`).Scan(&antes)
	lid, valor, _ := e.lectura("201", "2026-09")
	v, _ := P.Milesimas(valor)
	st, d := e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/lecturas/%d", lid), tok, map[string]any{"valor": P.TextoMilesimas(v + 3000), "motivo": "Foto releída"})
	if st != 200 {
		t.Fatalf("corregir: %d %v", st, d)
	}
	oct := d["repartos"].([]any)[1].(map[string]any)
	if oct["emitido"] != false || num(oct["recibos_actualizados"]) != 24 {
		t.Errorf("octubre en borrador: %v", oct)
	}
	var agua, ajustes, sinAplicar int64
	_ = e.pool.QueryRow(context.Background(), `SELECT COALESCE(sum(rl.monto_cts) FILTER (WHERE rl.tipo IN ('agua','agua_comun')),0), COALESCE(sum(rl.monto_cts) FILTER (WHERE rl.tipo='ajuste'),0)
		FROM recibo_linea rl JOIN recibo r ON r.id=rl.recibo_id JOIN periodo p ON p.id=r.periodo_id WHERE p.periodo='2026-10'`).Scan(&agua, &ajustes)
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM ajuste WHERE recibo_id IS NULL`).Scan(&sinAplicar)
	if agua != 500000 || antes != 500000 || ajustes != 0 || sinAplicar != 0 {
		t.Errorf("octubre: agua %d (antes %d), ajustes suman %d, sin aplicar %d", agua, antes, ajustes, sinAplicar)
	}
	var consumo string
	_ = e.pool.QueryRow(context.Background(), `SELECT rl.descripcion FROM recibo_linea rl JOIN recibo r ON r.id=rl.recibo_id JOIN unidad u ON u.id=r.unidad_id
		JOIN periodo p ON p.id=r.periodo_id WHERE u.codigo='201' AND p.periodo='2026-10' AND rl.tipo='agua'`).Scan(&consumo)
	if !strings.Contains(consumo, "11,000") {
		t.Errorf("el agua de octubre del 201 debería tomar 11 m³: %s", consumo)
	}
	// Los totales de los borradores cuadran con sus líneas.
	var descuadre int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM recibo r WHERE r.total_cts <> (SELECT COALESCE(sum(monto_cts),0) FROM recibo_linea WHERE recibo_id=r.id)`).Scan(&descuadre)
	if descuadre != 0 {
		t.Errorf("%d recibos no cuadran con sus líneas", descuadre)
	}
}

// ---------- Bloque 3 · PDF del balance y correo ----------

func (e *entorno) bajar(ruta, tok string) (int, string, []byte) {
	e.t.Helper()
	req, _ := http.NewRequest("GET", e.srv.URL+ruta, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header.Get("Content-Type"), b
}

// textoPDF: el texto del PDF con pdftotext si está instalado; si no, los bytes crudos (los flujos van sin comprimir).
func textoPDF(t *testing.T, b []byte) string {
	if ruta, err := exec.LookPath("pdftotext"); err == nil {
		cmd := exec.Command(ruta, "-layout", "-", "-")
		cmd.Stdin = bytes.NewReader(b)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("pdftotext no pudo abrir el PDF: %v", err)
		}
		return string(out)
	}
	s, _ := charmap.Windows1252.NewDecoder().Bytes(b)
	return string(s)
}

func TestPDFBalanceEInformeJunta(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	st, tipo, b := e.bajar("/api/v1/edificios/1/balance/2026-09.pdf", tok)
	if st != 200 || tipo != "application/pdf" || !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatalf("balance PDF: %d %s", st, tipo)
	}
	txt := textoPDF(t, b)
	for _, quiero := range []string{"S/ 19.460,00", "S/ 18.950,00", "S/ 510,00", "13,1 %", "S/ 34.120,00", "Morosidad por unidad", "Dpto 402", "S/ 1.420,00", "Trabajos del mes", "INC-014", "Conserjería"} {
		if !strings.Contains(txt, quiero) {
			t.Errorf("el PDF del balance no dice %q", quiero)
		}
	}
	st, _, b = e.bajar("/api/v1/edificios/1/balance/2026-09/informe-junta.pdf", tok)
	txt = textoPDF(t, b)
	if st != 200 || !strings.Contains(txt, "Pendientes por criticidad") || !strings.Contains(txt, "Aprobaciones del mes") || !strings.Contains(txt, "2 a favor") {
		t.Errorf("informe a la junta: %d\n%s", st, txt)
	}
	// El propietario también baja el balance; el formato de periodo se valida.
	if st, _, _ := e.bajar("/api/v1/edificios/1/balance/2026-09.pdf", e.login("propietario201@demo.pe")); st != 200 {
		t.Errorf("propietario baja el balance: %d", st)
	}
	if st, _, _ := e.bajar("/api/v1/edificios/1/balance/setiembre.pdf", tok); st != 422 {
		t.Errorf("periodo inválido: %d", st)
	}
}

func TestCorreoRecibosYBalanceSimulado(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	st, d := e.pedir("POST", "/api/v1/edificios/1/recibos/2026-09/enviar-correo", tok, nil)
	if st != 202 || num(d["encolados"]) != 24 || num(d["simulados"]) != 24 || len(d["sin_correo"].([]any)) != 0 {
		t.Fatalf("recibos por correo: %d %v", st, d)
	}
	var conPDF int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(DISTINCT m.id) FROM correo_mensaje m JOIN correo_adjunto a ON a.mensaje_id=m.id
		WHERE m.origen='recibo' AND a.tipo_mime='application/pdf' AND substr(a.datos,1,5)='%PDF-'::bytea`).Scan(&conPDF)
	if conPDF != 24 {
		t.Errorf("correos con PDF: %d", conPDF)
	}
	// El PDF del 201 cuadra con el API (S/ 990,00) y el correo va a su propietaria.
	var pdf201 []byte
	var para, html string
	_ = e.pool.QueryRow(context.Background(), `SELECT a.datos, m.para, m.html FROM correo_mensaje m JOIN correo_adjunto a ON a.mensaje_id=m.id JOIN unidad u ON u.id=m.unidad_id
		WHERE m.origen='recibo' AND u.codigo='201'`).Scan(&pdf201, &para, &html)
	if para != "propietario201@demo.pe" || !strings.Contains(textoPDF(t, pdf201), "S/ 990,00") || !strings.Contains(html, "S/ 990,00") {
		t.Errorf("correo del 201: %s", para)
	}
	st, d = e.pedir("POST", "/api/v1/edificios/1/balance/2026-09/enviar-correo", tok, map[string]string{"destinatarios": "todos"})
	if st != 202 || num(d["encolados"]) != 29 {
		t.Fatalf("balance por correo (5 de la junta + 24 propietarios): %d %v", st, d)
	}
	var adjJunta int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM correo_adjunto a JOIN correo_mensaje m ON m.id=a.mensaje_id WHERE m.origen='balance' AND m.para='junta@demo.pe'`).Scan(&adjJunta)
	if adjJunta != 2 {
		t.Errorf("la junta recibe balance + informe: %d adjuntos", adjJunta)
	}
	_, l := e.pedir("GET", "/api/v1/edificios/1/correo/mensajes?origen=balance", tok, nil)
	if num(l["total"]) != 53 || l["modo"] != "simulado" {
		t.Errorf("bandeja de correo: %v %v", l["total"], l["modo"])
	}
	// El propietario no puede enviar.
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/recibos/2026-09/enviar-correo", e.login("propietario201@demo.pe"), nil); st != 403 {
		t.Errorf("propietario envía correos: %d", st)
	}
}

// ---------- Bloque 4 · conciliación bancaria ----------

func (e *entorno) conciliacion(tok string) map[string]any {
	e.t.Helper()
	st, d := e.pedir("GET", "/api/v1/edificios/1/conciliacion?periodo=2026-09", tok, nil)
	if st != 200 {
		e.t.Fatalf("conciliación: %d %v", st, d)
	}
	return d
}

func movPorDesc(d map[string]any, texto string) map[string]any {
	for _, x := range d["banco"].([]any) {
		if m := x.(map[string]any); strings.Contains(m["descripcion"].(string), texto) {
			return m
		}
	}
	return nil
}

// El extracto demo de setiembre cuadra con el banco del sistema salvo 2 movimientos sin pareja; al confirmar
// las sugerencias y crear el egreso y el ingreso que faltan, la diferencia queda en cero y el balance lo dice.
func TestConciliacionSetiembre(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	d := e.conciliacion(tok)
	est := d["estado"].(map[string]any)
	if num(est["sin_pareja"]) != 2 || num(est["saldo_sistema_cts"]) != 3412000 || num(est["diferencia_cts"]) != 74200 || num(est["conciliados"]) == 0 || num(est["sugeridos"]) == 0 {
		t.Fatalf("estado inicial: %v", est)
	}
	// Reglas: los pagos con código se concilian solos; los egresos (sin código) quedan sugeridos por monto y fecha.
	reglas := map[string]int{}
	for _, x := range d["banco"].([]any) {
		m := x.(map[string]any)
		reglas[fmt.Sprint(m["estado"], "/", m["regla"])]++
	}
	if reglas["conciliado/codigo"] == 0 || reglas["sugerido/monto_fecha"] == 0 {
		t.Errorf("reglas aplicadas: %v", reglas)
	}
	// No se puede conciliar dos veces el mismo pago.
	var conPago, otro map[string]any
	for _, x := range d["banco"].([]any) {
		m := x.(map[string]any)
		if m["estado"] == "conciliado" && m["pago_id"] != nil && conPago == nil {
			conPago = m
		}
	}
	otro = movPorDesc(d, "DEPOSITO VENTANILLA")
	if st, r := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/conciliacion/movimientos/%d/confirmar", num(otro["id"])), tok, map[string]any{"pago_id": num(conPago["pago_id"])}); st != 409 || codigo(r) != "YA_CONCILIADO" {
		t.Errorf("conciliar dos veces el mismo pago: %d %v", st, r)
	}
	// Deshacer y volver a confirmar una pareja.
	if st, r := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/conciliacion/movimientos/%d/deshacer", num(conPago["id"])), tok, nil); st != 200 || r["estado"] != "sin_pareja" {
		t.Errorf("deshacer: %d %v", st, r)
	}
	if st, r := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/conciliacion/movimientos/%d/confirmar", num(conPago["id"])), tok, map[string]any{"pago_id": num(conPago["pago_id"])}); st != 200 {
		t.Errorf("reconfirmar: %d %v", st, r)
	}
	if st, r := e.pedir("POST", "/api/v1/edificios/1/conciliacion/confirmar-sugeridos", tok, map[string]string{"periodo": "2026-09"}); st != 200 || num(r["confirmados"]) == 0 {
		t.Errorf("confirmar sugeridos: %d %v", st, r)
	}
	com := movPorDesc(d, "COMISION")
	if st, r := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/conciliacion/movimientos/%d/crear-egreso", num(com["id"])), tok, map[string]any{"rubro": "servicios", "concepto": "Comisiones bancarias"}); st != 200 {
		t.Fatalf("crear egreso: %d %v", st, r)
	}
	st, r := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/conciliacion/movimientos/%d/crear-ingreso", num(otro["id"])), tok, map[string]any{"unidad_id": e.idUnidad("104")})
	if st != 200 {
		t.Fatalf("crear ingreso: %d %v", st, r)
	}
	est = r["estado_conciliacion"].(map[string]any)
	if num(est["diferencia_cts"]) != 0 || est["conciliado"] != true || num(est["saldo_sistema_cts"]) != 3486200 {
		t.Errorf("tras resolver todo: %v", est)
	}
	_, b := e.pedir("GET", "/api/v1/edificios/1/balance?periodo=2026-09", tok, nil)
	if c := b["conciliacion"].(map[string]any); c["texto"] != "Conciliado con el banco al 30/09." {
		t.Errorf("el balance no dice que está conciliado: %v", c)
	}
	// El junta no concilia.
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/conciliacion", e.login("junta@demo.pe"), nil); st != 403 {
		t.Errorf("junta en conciliación: %d", st)
	}
}

// Subir el extracto (CSV) con columnas propias: se guarda el mapeo del banco y se empareja igual.
func TestSubirExtractoConMapeo(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	st, _, csv := e.bajar("/api/v1/edificios/1/conciliacion/extracto-demo.csv?periodo=2026-09", tok)
	if st != 200 {
		t.Fatalf("extracto demo: %d", st)
	}
	st, c := e.multipartPedir("POST", "/api/v1/edificios/1/conciliacion/columnas", tok, map[string]string{"banco": "SCOTIA"}, "archivo", "scotia.csv", csv)
	if st != 200 || c["mapeo"].(map[string]any)["cargo"] != "Cargo" {
		t.Fatalf("columnas: %d %v", st, c)
	}
	st, s := e.multipartPedir("POST", "/api/v1/edificios/1/conciliacion/extractos", tok, map[string]string{"banco": "BCP", "periodo": "2026-09",
		"col_fecha": "Fecha", "col_descripcion": "Descripción", "col_cargo": "Cargo", "col_abono": "Abono", "col_codigo": "Nro. operación", "col_saldo": "Saldo"}, "archivo", "bcp.csv", csv)
	if st != 201 {
		t.Fatalf("subir: %d %v", st, s)
	}
	est := s["estado"].(map[string]any)
	if num(est["sin_pareja"]) != 2 || num(est["diferencia_cts"]) != 74200 {
		t.Errorf("tras subir: %v", est)
	}
	var mapeo string
	_ = e.pool.QueryRow(context.Background(), `SELECT mapeo->>'codigo_operacion' FROM banco_mapeo WHERE banco='BCP'`).Scan(&mapeo)
	if mapeo != "Nro. operación" {
		t.Errorf("mapeo guardado: %q", mapeo)
	}
	// Un archivo que no es extracto: 422.
	if st, _ := e.multipartPedir("POST", "/api/v1/edificios/1/conciliacion/extractos", tok, map[string]string{"banco": "BCP", "periodo": "2026-09"}, "archivo", "x.csv", []byte("hola")); st != 422 {
		t.Errorf("archivo inválido: %d", st)
	}
}

// ---------- Bloque 5 · SUNAT (simulado) ----------

func (e *entorno) reciboDe(unidad, periodo string) int64 {
	e.t.Helper()
	var id int64
	if err := e.pool.QueryRow(context.Background(), `SELECT r.id FROM recibo r JOIN unidad u ON u.id=r.unidad_id JOIN periodo p ON p.id=r.periodo_id
		WHERE u.codigo=$1 AND p.periodo=$2 AND r.origen='periodo'`, unidad, periodo).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func TestSunatBoletaFacturaYAnulacion(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	// La configuración no devuelve secretos; producción está deshabilitada.
	_, cfg := e.pedir("GET", "/api/v1/edificios/1/facturacion/config", tok, nil)
	if cfg["modo"] != "simulado" || cfg["ose_clave"] != nil || cfg["certificado_clave"] != nil || cfg["produccion_habilitada"] != false {
		t.Errorf("config: %v", cfg)
	}
	if st, d := e.pedir("PUT", "/api/v1/edificios/1/facturacion/config", tok, map[string]any{"ruc": "20600000005", "razon_social": "X", "modo": "produccion"}); st != 422 || codigo(d) != "PRODUCCION_DESHABILITADA" {
		t.Errorf("producción: %d %v", st, d)
	}
	// Boleta al 201 (DNI).
	r201 := e.reciboDe("201", "2026-09")
	st, b := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/recibos/%d/comprobante", r201), tok, nil)
	if st != 201 || b["numero_completo"] != "B001-1" || b["tipo"] != "03" || b["estado"] != "aceptado" || num(b["total_cts"]) != 99000 || b["cliente_tipo_doc"] != "1" || b["cdr_codigo"] != "0" {
		t.Fatalf("boleta: %d %v", st, b)
	}
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/recibos/%d/comprobante", r201), tok, nil); st != 409 || codigo(d) != "YA_TIENE_COMPROBANTE" {
		t.Errorf("segundo comprobante: %d %v", st, d)
	}
	// El XML guardado verifica su firma; la reserva (gravada) lleva IGV.
	var doc string
	_ = e.pool.QueryRow(context.Background(), `SELECT xml FROM comprobante WHERE id=$1`, num(b["id"])).Scan(&doc)
	cert, err := app.CertDelXML(doc)
	if err != nil || sunat.Verificar(doc, cert) != nil {
		t.Errorf("la firma del XML guardado no verifica: %v", err)
	}
	if num(b["igv_cts"]) != 1220 || !strings.Contains(doc, "<cbc:ID>B001-1</cbc:ID>") {
		t.Errorf("IGV de la reserva: %v", b["igv_cts"])
	}
	// La propietaria baja su XML y su PDF (con QR); otro propietario no.
	tokM := e.login("propietario201@demo.pe")
	if st, tipo, x := e.bajar(fmt.Sprintf("/api/v1/edificios/1/comprobantes/%d/xml", num(b["id"])), tokM); st != 200 || !strings.Contains(tipo, "xml") || !bytes.Contains(x, []byte("<Invoice")) {
		t.Errorf("XML: %d %s", st, tipo)
	}
	st, _, p := e.bajar(fmt.Sprintf("/api/v1/edificios/1/comprobantes/%d/pdf", num(b["id"])), tokM)
	if txt := textoPDF(t, p); st != 200 || !strings.Contains(txt, "B001-1") || !strings.Contains(txt, "SIMULADO") || !strings.Contains(txt, "S/ 990,00") {
		t.Errorf("PDF: %d\n%s", st, txt)
	}
	// Factura a empresa (RUC).
	_, _ = e.pool.Exec(context.Background(), `UPDATE persona SET dni_ruc='20123456786', nombre='Inversiones Castillo SAC' WHERE nombre='Jorge Castillo Ramos'`)
	st, f := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/recibos/%d/comprobante", e.reciboDe("202", "2026-09")), tok, nil)
	if st != 201 || f["numero_completo"] != "F001-1" || f["tipo"] != "01" || f["cliente_tipo_doc"] != "6" {
		t.Fatalf("factura: %d %v", st, f)
	}
	// 20 emisiones a la vez: correlativo sin saltos (B001-2 … B001-21).
	var rids []int64
	for _, c := range []string{"101", "102", "103", "104", "203", "204", "301", "302", "303", "304", "401", "402", "403", "404", "501", "502", "503", "504", "601", "602"} {
		rids = append(rids, e.reciboDe(c, "2026-09"))
	}
	var wg sync.WaitGroup
	numeros := make(chan int64, len(rids))
	for _, rid := range rids {
		wg.Add(1)
		go func(rid int64) {
			defer wg.Done()
			st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/recibos/%d/comprobante", rid), tok, nil)
			if st == 201 {
				numeros <- num(d["numero"])
			} else {
				t.Errorf("emisión concurrente: %d %v", st, d)
			}
		}(rid)
	}
	wg.Wait()
	close(numeros)
	vistos := map[int64]bool{}
	for n := range numeros {
		vistos[n] = true
	}
	for n := int64(2); n <= 21; n++ {
		if !vistos[n] {
			t.Errorf("falta el B001-%d (vistos %v)", n, vistos)
		}
	}
	// Anulación: la boleta va por nota de crédito; la factura de hoy, por comunicación de baja.
	st, nc := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/comprobantes/%d/anular", num(b["id"])), tok, map[string]string{"motivo": "Se emitió al propietario equivocado"})
	if st != 200 || nc["via"] != "nota_credito" || nc["numero_completo"] != "BC01-1" || nc["tipo"] != "07" || num(nc["total_cts"]) != 99000 {
		t.Errorf("nota de crédito: %d %v", st, nc)
	}
	st, ba := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/comprobantes/%d/anular", num(f["id"])), tok, map[string]string{"motivo": "Error en el RUC"})
	if st != 200 || ba["via"] != "baja" || ba["estado"] != "anulado" || !strings.HasPrefix(fmt.Sprint(ba["baja_id"]), "RA-") {
		t.Errorf("baja: %d %v", st, ba)
	}
	// Anulada la boleta, el recibo puede volver a emitirse.
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/recibos/%d/comprobante", r201), tok, nil); st != 201 || d["numero_completo"] != "B001-22" {
		t.Errorf("reemisión: %d %v", st, d)
	}
	// Apagado: 409.
	_, _ = e.pedir("PUT", "/api/v1/edificios/1/facturacion/config", tok, map[string]any{"modo": "off"})
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/recibos/%d/comprobante", e.reciboDe("603", "2026-09")), tok, nil); st != 409 || codigo(d) != "FACTURACION_APAGADA" {
		t.Errorf("apagado: %d %v", st, d)
	}
	// Beta sin credenciales: 422.
	_, _ = e.pedir("PUT", "/api/v1/edificios/1/facturacion/config", tok, map[string]any{"ruc": "20600000005", "razon_social": "Junta", "modo": "beta"})
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/recibos/%d/comprobante", e.reciboDe("603", "2026-09")), tok, nil); st != 422 || codigo(d) != "SIN_CREDENCIALES_BETA" {
		t.Errorf("beta sin credenciales: %d %v", st, d)
	}
}

// Beta con las credenciales de prueba del servidor (SUNAT_BETA_*): el edificio sin
// usuario ni clave emite igual; las claves nunca vuelven por el API.
func TestSunatBetaConCredencialesDelServidor(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("sin openssl")
	}
	var sobre string
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		sobre = string(b)
		var zb bytes.Buffer
		z := zip.NewWriter(&zb)
		f, _ := z.Create("R-20600000005-03-B001-1.xml")
		_, _ = f.Write([]byte(sunat.CDRSimulado("20600000005", sunat.Boleta, "B001-1", "x", time.Now()).XML))
		_ = z.Close()
		_, _ = w.Write([]byte(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><br:sendBillResponse xmlns:br="http://service.sunat.gob.pe"><applicationResponse>` +
			base64.StdEncoding.EncodeToString(zb.Bytes()) + `</applicationResponse></br:sendBillResponse></soap:Body></soap:Envelope>`))
	}))
	defer falso.Close()
	t.Setenv("SUNAT_BETA_URL", falso.URL)
	t.Setenv("SUNAT_BETA_USUARIO", "20600000005MODDATOS")
	t.Setenv("SUNAT_BETA_CLAVE", "moddatos")
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	_, cfg := e.pedir("GET", "/api/v1/edificios/1/facturacion/config", tok, nil)
	if cfg["tiene_beta_servidor"] != true {
		t.Fatalf("sin respaldo del servidor: %v", cfg)
	}
	ifTodo, _ := json.Marshal(cfg)
	if strings.Contains(string(ifTodo), "moddatos") {
		t.Fatalf("la clave del servidor vuelve por el API: %v", cfg)
	}
	// Certificado del edificio (beta lo exige): se genera y se sube.
	dir := t.TempDir()
	cmd := exec.Command("sh", "-c", `openssl req -x509 -newkey rsa:2048 -nodes -keyout k.pem -out c.pem -days 30 -subj "/CN=Prueba EDISYS" 2>/dev/null &&
		openssl pkcs12 -export -legacy -inkey k.pem -in c.pem -out c.pfx -passout pass:clave123 2>/dev/null || openssl pkcs12 -export -inkey k.pem -in c.pem -out c.pfx -passout pass:clave123`)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("openssl: %v %s", err, out)
	}
	pfx, _ := os.ReadFile(dir + "/c.pfx")
	if st, d := e.multipartPedir("POST", "/api/v1/edificios/1/facturacion/certificado", tok, map[string]string{"clave": "clave123"}, "certificado", "c.pfx", pfx); st != 201 {
		t.Fatalf("subir pfx: %d %v", st, d)
	}
	// Sin usuario ni clave en el edificio: igual emite, con las del servidor.
	_, _ = e.pedir("PUT", "/api/v1/edificios/1/facturacion/config", tok, map[string]any{"ruc": "20600000005", "razon_social": "Junta", "modo": "beta"})
	st, b := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/recibos/%d/comprobante", e.reciboDe("201", "2026-09")), tok, nil)
	if st != 201 || b["modo"] != "beta" || b["estado"] != "aceptado" || b["numero_completo"] != "B001-1" {
		t.Fatalf("beta con respaldo: %d %v", st, b)
	}
	if !strings.Contains(sobre, "<wsse:Username>20600000005MODDATOS</wsse:Username>") {
		t.Errorf("no usó el usuario del servidor: %s", sobre)
	}
}

// El certificado .pfx se sube, se guarda en el cubo privado y nunca vuelve por el API (necesita openssl).
func TestSunatCertificadoPFX(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("sin openssl")
	}
	dir := t.TempDir()
	cmd := exec.Command("sh", "-c", `openssl req -x509 -newkey rsa:2048 -nodes -keyout k.pem -out c.pem -days 30 -subj "/CN=Prueba EDISYS" 2>/dev/null &&
		openssl pkcs12 -export -legacy -inkey k.pem -in c.pem -out c.pfx -passout pass:clave123 2>/dev/null || openssl pkcs12 -export -inkey k.pem -in c.pem -out c.pfx -passout pass:clave123`)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("openssl: %v %s", err, out)
	}
	pfx, _ := os.ReadFile(dir + "/c.pfx")
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	if st, _ := e.multipartPedir("POST", "/api/v1/edificios/1/facturacion/certificado", tok, map[string]string{"clave": "otra"}, "certificado", "c.pfx", pfx); st != 422 {
		t.Errorf("clave equivocada: %d", st)
	}
	st, d := e.multipartPedir("POST", "/api/v1/edificios/1/facturacion/certificado", tok, map[string]string{"clave": "clave123"}, "certificado", "c.pfx", pfx)
	if st != 201 || d["titular"] != "Prueba EDISYS" {
		t.Fatalf("subir pfx: %d %v", st, d)
	}
	_, cfg := e.pedir("GET", "/api/v1/edificios/1/facturacion/config", tok, nil)
	if cfg["tiene_certificado"] != true || cfg["certificado_clave"] != nil {
		t.Errorf("config con certificado: %v", cfg)
	}
	st, b := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/recibos/%d/comprobante", e.reciboDe("201", "2026-09")), tok, nil)
	if st != 201 || b["firmado_prueba"] != false {
		t.Errorf("firmado con el pfx: %d %v", st, b)
	}
}

// El botón «Enviar por correo» del recibo (POST /recibos/enviar) manda ese recibo en PDF a su propietario.
func TestEnviarUnReciboPorCorreo(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	st, d := e.pedir("POST", "/api/v1/edificios/1/recibos/enviar", tok, map[string]any{"recibo_ids": []int64{e.reciboDe("201", "2026-09")}})
	if st != 202 || num(d["encolados"]) != 1 || d["canal"] != "correo" {
		t.Fatalf("enviar un recibo: %d %v", st, d)
	}
	var para string
	_ = e.pool.QueryRow(context.Background(), `SELECT para FROM correo_mensaje WHERE origen='recibo'`).Scan(&para)
	if para != "propietario201@demo.pe" {
		t.Errorf("destinatario %q", para)
	}
}

// ---------- Bloque 6 · reservar desde el chatbot ----------

func (e *entorno) bot(tok, tel, texto string) map[string]any {
	e.t.Helper()
	st, c := e.pedir("POST", "/api/v1/chatbot/mensaje", tok, map[string]string{"telefono": tel, "texto": texto})
	if st != 200 {
		e.t.Fatalf("chatbot %q: %d %v", texto, st, c)
	}
	return c
}

func paso(c map[string]any) any { return c["datos"].(map[string]any)["paso"] }

func TestChatbotReservaCompleta(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	c := e.bot(tok, "51900000201", "quiero reservar la parrilla el sábado")
	if c["intencion"] != "reservar" || paso(c) != "elegir_franja" || !strings.Contains(c["respuesta"].(string), "1. Parrilla") {
		t.Fatalf("franjas: %v", c)
	}
	c = e.bot(tok, "51900000201", "7") // fuera de rango
	if !strings.Contains(c["respuesta"].(string), "Elige un número del 1 al") {
		t.Errorf("fuera de rango: %v", c["respuesta"])
	}
	c = e.bot(tok, "51900000201", "1")
	if paso(c) != "confirmar" || !strings.Contains(c["respuesta"].(string), "¿Confirmo?") || !strings.Contains(c["respuesta"].(string), "S/ 80,00") {
		t.Fatalf("resumen: %v", c)
	}
	c = e.bot(tok, "51900000201", "sí")
	r, ok := c["datos"].(map[string]any)["reserva"].(map[string]any)
	if !ok || r["estado"] != "confirmada" || !strings.Contains(c["respuesta"].(string), "¡Reservado!") || !strings.Contains(c["respuesta"].(string), "recibo") {
		t.Fatalf("reserva: %v", c)
	}
	var unidad string
	_ = e.pool.QueryRow(context.Background(), `SELECT u.codigo FROM reserva rv JOIN unidad u ON u.id=rv.unidad_id WHERE rv.codigo=$1`, r["codigo"]).Scan(&unidad)
	if unidad != "201" {
		t.Errorf("la reserva quedó en %q", unidad)
	}
	// Pasos con «cancelar»: vuelve al menú en cualquier paso.
	e.bot(tok, "51900000201", "quiero reservar")
	c = e.bot(tok, "51900000201", "piscina")
	if paso(c) != "elegir_fecha" {
		t.Errorf("pide la fecha: %v", c)
	}
	c = e.bot(tok, "51900000201", "cancelar")
	if !strings.Contains(c["respuesta"].(string), "cancelé") || !strings.Contains(c["respuesta"].(string), "1. Cuánto debo") {
		t.Errorf("cancelar: %v", c)
	}
	var n int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM chatbot_sesion`).Scan(&n)
	if n != 0 {
		t.Errorf("quedó una sesión abierta")
	}
}

// Con pago inmediato la reserva queda retenida 15 minutos y el bot explica cómo pagar.
func TestChatbotReservaPagoInmediato(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	_, _ = e.pool.Exec(context.Background(), `UPDATE edificio SET modo_cobro_reservas='pago_inmediato'`)
	e.bot(tok, "51900000201", "reservar el sum mañana")
	e.bot(tok, "51900000201", "1")
	c := e.bot(tok, "51900000201", "si")
	r := c["datos"].(map[string]any)["reserva"].(map[string]any)
	if r["estado"] != "pendiente_pago" || !strings.Contains(c["respuesta"].(string), "15 minutos") || !strings.Contains(c["respuesta"].(string), "Yape") {
		t.Errorf("retención: %v", c)
	}
}

func TestChatbotReservaMorosoYSesionCaducada(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	c := e.bot(tok, "51900000402", "quiero reservar la parrilla el sábado")
	if c["datos"].(map[string]any)["moroso"] != true || !strings.Contains(c["respuesta"].(string), "S/ 1.420,00") {
		t.Errorf("moroso: %v", c)
	}
	// Sesión caducada: el «1» ya no reserva nada.
	e.bot(tok, "51900000201", "quiero reservar la parrilla el sábado")
	_, _ = e.pool.Exec(context.Background(), `UPDATE chatbot_sesion SET vence_en = now() - interval '1 minute'`)
	c = e.bot(tok, "51900000201", "1")
	if c["datos"].(map[string]any)["sesion_caducada"] != true || !strings.Contains(c["respuesta"].(string), "caducó") {
		t.Errorf("caducada: %v", c)
	}
	var n int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM reserva WHERE creado_en > now() - interval '1 minute'`).Scan(&n)
	if n != 0 {
		t.Errorf("reservó con la sesión caducada")
	}
}

// Dos vecinos piden la misma franja a la vez por el bot: uno gana y al otro le ofrece las que quedan.
func TestChatbotReservaConcurrente(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	tels := []string{"51900000101", "51900000102"}
	for _, tel := range tels {
		e.bot(tok, tel, "quiero reservar la parrilla el sábado")
		if c := e.bot(tok, tel, "1"); paso(c) != "confirmar" {
			t.Fatalf("%s: %v", tel, c)
		}
	}
	var wg sync.WaitGroup
	resp := make([]map[string]any, 2)
	for i, tel := range tels {
		wg.Add(1)
		go func(i int, tel string) {
			defer wg.Done()
			resp[i] = e.bot(tok, tel, "sí")
		}(i, tel)
	}
	wg.Wait()
	ganan, otras := 0, 0
	for _, c := range resp {
		d := c["datos"].(map[string]any)
		if _, ok := d["reserva"]; ok {
			ganan++
		}
		if d["franja_ocupada"] == true && d["paso"] == "elegir_franja" && strings.Contains(c["respuesta"].(string), "Alguien acaba de reservar") {
			otras++
		}
	}
	if ganan != 1 || otras != 1 {
		t.Errorf("ganan %d, reciben otras franjas %d: %v", ganan, otras, resp)
	}
}
