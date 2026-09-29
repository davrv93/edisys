package app_test

// Pruebas de los pendientes funcionales (docs/PLAN_PENDIENTES_FUNCIONALES.md), bloque por bloque.

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"

	"edisys/api/internal/app"
	P "edisys/api/internal/plataforma"
	"edisys/api/internal/reparto"
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
