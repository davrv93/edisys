package app_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"edisys/api/internal/app"
	"edisys/api/internal/archivo"
	"edisys/api/internal/config"
)

// Bloques B3, D1, D2 y D3 · gestión de deuda.

func fecha(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// ---------- reglas puras ----------

func TestDividirCuotasSumaExacta(t *testing.T) {
	// El caso del video: S/ 886 en 12 cuotas (no es divisible al céntimo).
	for _, c := range []struct {
		monto int64
		n     int
	}{{88600, 12}, {88600, 6}, {100, 3}, {1, 1}, {99999, 7}} {
		cs := app.DividirCuotas(c.monto, c.n, fecha("2026-10-15"))
		if len(cs) != c.n {
			t.Fatalf("%d cuotas, esperaba %d", len(cs), c.n)
		}
		var suma, min, max int64 = 0, cs[0].MontoCts, cs[0].MontoCts
		for _, q := range cs {
			suma += q.MontoCts
			if q.MontoCts < min {
				min = q.MontoCts
			}
			if q.MontoCts > max {
				max = q.MontoCts
			}
		}
		if suma != c.monto {
			t.Fatalf("Σ cuotas %d ≠ %d", suma, c.monto)
		}
		if max-min > 1 {
			t.Fatalf("cuotas desparejas: %d..%d", min, max)
		}
	}
	// El 31 de enero cae al último día de febrero y vuelve al 31 en marzo.
	cs := app.DividirCuotas(300, 3, fecha("2027-01-31"))
	if cs[1].Vence != "2027-02-28" || cs[2].Vence != "2027-03-31" {
		t.Fatalf("vencimientos: %+v", cs)
	}
}

func TestMoraYCuotas(t *testing.T) {
	// 1 % mensual sobre S/ 1.000 con 45 días de atraso = 2 meses iniciados = S/ 20.
	if m := app.MoraCts(100000, 100, fecha("2026-08-15"), fecha("2026-09-29")); m != 2000 {
		t.Fatalf("mora %d", m)
	}
	if m := app.MoraCts(100000, 0, fecha("2026-08-15"), fecha("2026-09-29")); m != 0 {
		t.Fatalf("sin tasa no hay mora: %d", m)
	}
	cs := []app.Cuota{{1, "2026-09-01", 1000}, {2, "2026-10-01", 1000}, {3, "2026-11-01", 1000}}
	est := app.EstadoCuotas(cs, 1500, 15, fecha("2026-10-20"))
	if est[0]["estado"] != "pagado" || est[1]["estado"] != "vencido" || est[1]["pagado_cts"] != int64(500) || est[2]["estado"] != "pendiente" {
		t.Fatalf("estados: %v", est)
	}
}

func TestEscalasDeAviso(t *testing.T) {
	h := func(v int64) *int64 { return &v }
	_, err := app.ValidarEscalas([]app.Escala{{Desde: 10000, Hasta: h(100000), Asunto: "a", Cuerpo: "b"}, {Desde: 100000, Hasta: nil, Asunto: "c", Cuerpo: "d"}})
	if err == nil {
		t.Fatal("tramos que comparten el borde deben rechazarse")
	}
	_, err = app.ValidarEscalas([]app.Escala{{Desde: 0, Hasta: nil, Asunto: "a", Cuerpo: "b"}, {Desde: 500, Hasta: h(900), Asunto: "c", Cuerpo: "d"}})
	if err == nil {
		t.Fatal("un tramo sin tope que no es el último debe rechazarse")
	}
	es, err := app.ValidarEscalas([]app.Escala{{Desde: 100001, Hasta: nil, Asunto: "Evite problemas judiciales", Cuerpo: "x"}, {Desde: 10000, Hasta: h(100000), Asunto: "Recordatorio", Cuerpo: "y"}})
	if err != nil {
		t.Fatal(err)
	}
	if x := app.ElegirEscala(es, 50000); x == nil || x.Asunto != "Recordatorio" {
		t.Fatalf("S/ 500 → suave: %v", x)
	}
	if x := app.ElegirEscala(es, 500000); x == nil || x.Asunto != "Evite problemas judiciales" {
		t.Fatalf("S/ 5.000 → fuerte: %v", x)
	}
	if x := app.ElegirEscala(es, 5000); x != nil {
		t.Fatalf("S/ 50 no tiene tramo: %v", x)
	}
	if !app.TocaHoy("mensual", 31, fecha("2026-02-28")) || app.TocaHoy("mensual", 15, fecha("2026-02-14")) || !app.TocaHoy("semanal", 7, fecha("2026-10-04")) {
		t.Fatal("calendario de avisos")
	}
	if got := app.RenderAviso("Hola {{nombre}}, debes {{deuda}}", map[string]string{"nombre": "Rina", "deuda": "S/ 886,00"}); got != "Hola Rina, debes S/ 886,00" {
		t.Fatal(got)
	}
}

func TestEstadoCeldaYEstadoCuenta(t *testing.T) {
	v := fecha("2026-09-10")
	hoy := fecha("2026-10-20")
	p := func(s string) *time.Time { x := fecha(s); return &x }
	casos := map[string]string{
		app.EstadoCelda(1000, &v, p("2026-09-10"), 15, hoy):     "puntual",
		app.EstadoCelda(1000, &v, p("2026-09-20"), 15, hoy):     "en_gracia",
		app.EstadoCelda(1000, &v, p("2026-10-05"), 15, hoy):     "tardio",
		app.EstadoCelda(1000, &v, nil, 15, hoy):                 "moroso",
		app.EstadoCelda(1000, &v, nil, 15, fecha("2026-09-20")): "pendiente",
	}
	for got, want := range casos {
		if got != want {
			t.Fatalf("celda %s, esperaba %s", got, want)
		}
	}
	movs := []app.Movimiento{
		{Fecha: "2026-09-01", Tipo: "cargo", CargoCts: 1000},
		{Fecha: "2026-08-01", Tipo: "cargo", CargoCts: 1000},
		{Fecha: "2026-08-05", Tipo: "abono", AbonoCts: 1000},
		{Fecha: "2026-10-01", Tipo: "cargo", CargoCts: 500},
	}
	ini, vis, fin := app.EstadoCuenta(movs, "2026-09-01", "2026-09-30")
	if ini != 0 || len(vis) != 1 || vis[0].SaldoCts != 1000 || fin != 1000 {
		t.Fatalf("estado de cuenta: ini %d vis %v fin %d", ini, vis, fin)
	}
}

// ---------- B3 · cuentas por cobrar y estado de cuenta ----------

func TestCuentasPorCobrarYEstadoCuenta(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	st, d := e.pedir("GET", "/api/v1/edificios/1/cuentas-por-cobrar", tok, nil)
	if st != 200 {
		t.Fatalf("cxc: %d %v", st, d)
	}
	var suma float64
	var vio402 bool
	for _, it := range d["datos"].([]any) {
		f := it.(map[string]any)
		parte := f["por_vencer_cts"].(float64) + f["d1_30_cts"].(float64) + f["d31_60_cts"].(float64) + f["d61_90_cts"].(float64) + f["d90_mas_cts"].(float64)
		if parte != f["deuda_cts"].(float64) {
			t.Fatalf("los tramos de antigüedad no suman la deuda de %v: %v ≠ %v", f["unidad"], parte, f["deuda_cts"])
		}
		suma += f["deuda_cts"].(float64)
		if f["unidad"] == "402" {
			vio402 = f["moroso"] == true
		}
	}
	if suma != d["totales"].(map[string]any)["deuda_cts"].(float64) || !vio402 {
		t.Fatalf("totales %v / 402 moroso %v", d["totales"], vio402)
	}
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/cuentas-por-cobrar", e.login("propietario201@demo.pe"), nil); st != 403 {
		t.Fatalf("el propietario no ve las cuentas por cobrar: %d", st)
	}

	// Estado de cuenta: el saldo final cuadra con la cuenta corriente.
	u := e.idUnidad("402")
	_, ec := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/unidades/%d/estado-cuenta", u), tok, nil)
	_, cc := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/unidades/%d/cuenta", u), tok, nil)
	if ec["saldo_final_cts"] != cc["deuda_cts"] || ec["saldo_final_cts"].(float64) <= 0 {
		t.Fatalf("saldo final %v ≠ deuda %v", ec["saldo_final_cts"], cc["deuda_cts"])
	}
	// Con rango: saldo inicial + cargos − abonos = saldo al cierre.
	_, r := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/unidades/%d/estado-cuenta?desde=2026-08-01&hasta=2026-09-30", u), tok, nil)
	if r["saldo_inicial_cts"].(float64)+r["cargos_cts"].(float64)-r["abonos_cts"].(float64) != r["saldo_final_cts"].(float64) {
		t.Fatalf("estado de cuenta con rango descuadrado: %v", r)
	}
	// El propietario ve su unidad y no la ajena.
	prop := e.login("propietario201@demo.pe")
	if st, _ := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/unidades/%d/estado-cuenta", e.idUnidad("201")), prop, nil); st != 200 {
		t.Fatalf("propietario, su unidad: %d", st)
	}
	if st, _ := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/unidades/%d/estado-cuenta", u), prop, nil); st != 404 {
		t.Fatalf("propietario, unidad ajena: %d", st)
	}
	if st, _, b := e.bajar(fmt.Sprintf("/api/v1/edificios/1/unidades/%d/estado-cuenta?formato=csv", u), tok); st != 200 || !strings.Contains(string(b), "Saldo inicial") {
		t.Fatalf("csv: %d", st)
	}
}

// ---------- D1 · acuerdos de pago ----------

func (e *entorno) moroso(u int64) (bool, int64) {
	var m bool
	var d int64
	if err := e.pool.QueryRow(context.Background(), `SELECT es_moroso($1), deuda_vencida_cts($1)`, u).Scan(&m, &d); err != nil {
		e.t.Fatal(err)
	}
	return m, d
}

func (e *entorno) totalRecibos(u int64) (total, pagado int64) {
	_ = e.pool.QueryRow(context.Background(), `SELECT COALESCE(sum(total_cts),0), COALESCE(sum(pagado_cts),0) FROM recibo WHERE unidad_id=$1 AND estado NOT IN ('borrador','anulado')`, u).Scan(&total, &pagado)
	return
}

func TestAcuerdoDePago(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	u := e.idUnidad("402")
	if m, _ := e.moroso(u); !m {
		t.Skip("la semilla ya no deja al 402 moroso")
	}
	totalAntes, _ := e.totalRecibos(u)

	st, pr := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/acuerdos/propuesta?unidad_id=%d", u), tok, nil)
	if st != 200 || len(pr["recibos"].([]any)) == 0 {
		t.Fatalf("propuesta: %d %v", st, pr)
	}
	saldo := int64(pr["saldo_cts"].(float64))

	// Recargo S/ 10, descuento S/ 5, 3 cuotas desde el mes que viene.
	primera := time.Now().AddDate(0, 1, 0).Format("2006-01-02")
	st, ac := e.pedir("POST", "/api/v1/edificios/1/acuerdos", tok, map[string]any{
		"unidad_id": u, "recargo_cts": 1000, "descuento_cts": 500, "n_cuotas": 3, "primera_cuota": primera,
		"aceptado_por": "Luis Alberto Campos", "comentario": "Acuerdo en junta"})
	if st != 201 {
		t.Fatalf("crear acuerdo: %d %v", st, ac)
	}
	monto := int64(ac["monto_acordado_cts"].(float64))
	if monto != saldo+500 {
		t.Fatalf("monto acordado %d ≠ saldo %d + recargo − descuento", monto, saldo)
	}
	var suma int64
	for _, c := range ac["cuotas"].([]any) {
		suma += int64(c.(map[string]any)["monto_cts"].(float64))
	}
	if suma != monto {
		t.Fatalf("Σ cuotas %d ≠ %d", suma, monto)
	}
	totalCon, pagadoCon := e.totalRecibos(u)
	if totalCon != totalAntes+500 {
		t.Fatalf("los recibos deben subir el neto recargo − descuento: %d → %d", totalAntes, totalCon)
	}
	// Con el acuerdo al día la unidad deja de ser morosa (habilita reservas).
	if m, d := e.moroso(u); m {
		t.Fatalf("con acuerdo al día no debería ser moroso (deuda vencida %d)", d)
	}
	// Los mismos recibos no entran en otro acuerdo.
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/acuerdos", tok, map[string]any{"unidad_id": u, "n_cuotas": 2, "aceptado_por": "x"}); st != 422 {
		t.Fatalf("segundo acuerdo con los mismos recibos: %d", st)
	}
	aid := int64(ac["id"].(float64))

	// Un pago avanza el acuerdo.
	st, pg := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/unidades/%d/pagos", u), tok, map[string]any{
		"monto_cts": 1000, "medio": "efectivo", "fecha": time.Now().Format("2006-01-02")})
	if st != 201 && st != 200 {
		t.Fatalf("pago: %d %v", st, pg)
	}
	_, ver := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/acuerdos/%d", aid), tok, nil)
	if ver["avance_cts"].(float64) != 1000 || ver["pendiente_cts"].(float64) != float64(monto-1000) {
		t.Fatalf("avance: %v pendiente %v", ver["avance_cts"], ver["pendiente_cts"])
	}
	if cs := ver["cuotas"].([]any); cs[0].(map[string]any)["estado"] != "parcial" && monto/3 > 1000 {
		t.Fatalf("primera cuota parcial: %v", cs[0])
	}

	// El documento firmado se sube al cubo privado.
	if st, d := e.multipartPedir("POST", fmt.Sprintf("/api/v1/edificios/1/acuerdos/%d/documento", aid), tok, map[string]string{}, "archivo", "acuerdo.pdf",
		[]byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF")); st != 200 || d["documento_url"] == nil {
		t.Fatalf("documento: %d %v", st, d)
	}

	// Anular: vuelve la morosidad y se revierten recargo y descuento.
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/acuerdos/%d/anular", aid), tok, map[string]any{"motivo": "Incumplió"}); st != 200 {
		t.Fatalf("anular: %d %v", st, d)
	}
	totalDesp, _ := e.totalRecibos(u)
	if totalDesp != totalAntes {
		t.Fatalf("anular debe devolver los totales: %d ≠ %d", totalDesp, totalAntes)
	}
	if m, _ := e.moroso(u); !m {
		t.Fatal("anulado el acuerdo, la unidad vuelve a ser morosa")
	}
	_ = pagadoCon

	// La junta ve los acuerdos pero no los crea.
	junta := e.login("junta@demo.pe")
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/acuerdos", junta, nil); st != 200 {
		t.Fatalf("junta ve: %d", st)
	}
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/acuerdos", junta, map[string]any{"unidad_id": u, "n_cuotas": 2, "aceptado_por": "x"}); st != 403 {
		t.Fatalf("junta no crea: %d", st)
	}
}

func TestAcuerdoConCuotaVencidaSigueMoroso(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	u := e.idUnidad("402")
	if m, _ := e.moroso(u); !m {
		t.Skip("la semilla ya no deja al 402 moroso")
	}
	// Primera cuota hace 60 días: ya pasó su gracia y nadie la pagó.
	primera := time.Now().AddDate(0, 0, -60).Format("2006-01-02")
	st, ac := e.pedir("POST", "/api/v1/edificios/1/acuerdos", tok, map[string]any{
		"unidad_id": u, "n_cuotas": 4, "primera_cuota": primera, "aceptado_por": "Luis Alberto Campos"})
	if st != 201 {
		t.Fatalf("crear: %d %v", st, ac)
	}
	m, d := e.moroso(u)
	primeraCuota := int64(ac["cuotas"].([]any)[0].(map[string]any)["monto_cts"].(float64))
	if !m || d < primeraCuota {
		t.Fatalf("con la cuota 1 vencida debe seguir moroso por al menos esa cuota: %v %d < %d", m, d, primeraCuota)
	}
	_, l := e.pedir("GET", "/api/v1/edificios/1/acuerdos", tok, nil)
	if l["datos"].([]any)[0].(map[string]any)["estado_visible"] != "vencido" {
		t.Fatalf("estado visible: %v", l["datos"])
	}
}

// ---------- D2 · avisos de cobranza ----------

func TestAvisosDeCobranza(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	escalas := []map[string]any{
		{"desde": 1, "hasta": 100000, "asunto": "Recordatorio de pago", "cuerpo": "Hola {{nombre}}, el Dpto {{unidad}} debe {{deuda}}."},
		{"desde": 100001, "hasta": nil, "asunto": "Evite problemas judiciales", "cuerpo": "Hola {{nombre}}, su deuda vencida es {{deuda}} ({{recibos}} recibos)."},
	}
	cruzadas := []map[string]any{escalas[0], {"desde": 50000, "hasta": nil, "asunto": "x", "cuerpo": "y"}}
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/avisos-cobranza", tok, map[string]any{"nombre": "Mal", "canales": []string{"correo"}, "escalas": cruzadas}); st != 422 {
		t.Fatalf("escalas cruzadas: %d", st)
	}
	st, d := e.pedir("POST", "/api/v1/edificios/1/avisos-cobranza", tok, map[string]any{
		"nombre": "Aviso del 15", "frecuencia": "diaria", "dia": 15, "canales": []string{"correo", "whatsapp"}, "adjunta_pdf": true, "escalas": escalas})
	if st != 201 {
		t.Fatalf("crear aviso: %d %v", st, d)
	}
	aid := int64(d["id"].(float64))
	st, res := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/avisos-cobranza/%d/ejecutar", aid), tok, nil)
	if st != 202 || res["deudores"].(float64) < 1 || res["encolados"].(float64) < 1 {
		t.Fatalf("ejecutar: %d %v", st, res)
	}
	// Evidencia: un envío por deudor y canal, con su mensaje en la bandeja (el correo lleva el PDF).
	var envios, conMensaje, adjuntos int
	ctx := context.Background()
	_ = e.pool.QueryRow(ctx, `SELECT count(*), count(mensaje_id) FROM aviso_envio WHERE aviso_id=$1`, aid).Scan(&envios, &conMensaje)
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM correo_adjunto a JOIN aviso_envio v ON v.mensaje_id=a.mensaje_id AND v.canal='correo' WHERE v.aviso_id=$1`, aid).Scan(&adjuntos)
	if envios != 2*int(res["deudores"].(float64)) || conMensaje == 0 || adjuntos == 0 {
		t.Fatalf("evidencia: envíos %d, con mensaje %d, adjuntos %d", envios, conMensaje, adjuntos)
	}
	// El tramo fuerte lleva el asunto fuerte: el 402 debe más de S/ 1.000.
	var asunto string
	_ = e.pool.QueryRow(ctx, `SELECT asunto FROM aviso_envio WHERE aviso_id=$1 AND unidad_id=$2 AND canal='correo'`, aid, e.idUnidad("402")).Scan(&asunto)
	if asunto != "Evite problemas judiciales" {
		t.Fatalf("tramo del 402: %q", asunto)
	}
	// Repetir el mismo día no duplica.
	_, res2 := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/avisos-cobranza/%d/ejecutar", aid), tok, nil)
	if res2["encolados"].(float64) != 0 {
		t.Fatalf("el mismo día no se repite: %v", res2)
	}
	// Ninguna evidencia sale a terceros: todo queda en la bandeja (pendiente o simulado).
	var fuera int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM whatsapp_mensaje WHERE origen='sistema' AND estado='enviado'`).Scan(&fuera)
	if fuera != 0 {
		t.Fatalf("no debería salir nada: %d enviados", fuera)
	}

	// Aviso manual a una unidad sin deuda vencida: no se envía.
	_, man := e.pedir("POST", "/api/v1/edificios/1/avisos-cobranza/manual", tok, map[string]any{
		"unidad_ids": []int64{e.idUnidad("201")}, "canales": []string{"correo"}, "asunto": "Aviso", "cuerpo": "Hola {{nombre}}"})
	if man["deudores"].(float64) != 0 {
		t.Fatalf("201 está al día: %v", man)
	}
	_, man = e.pedir("POST", "/api/v1/edificios/1/avisos-cobranza/manual", tok, map[string]any{
		"unidad_ids": []int64{e.idUnidad("402")}, "canales": []string{"whatsapp"}, "asunto": "Aviso", "cuerpo": "Hola {{nombre}}, debe {{deuda}}"})
	if man["encolados"].(float64) != 1 {
		t.Fatalf("manual al 402: %v", man)
	}

	// La tarea diaria: corre el aviso diario una vez y al segundo intento no hace nada.
	if _, err := e.pool.Exec(ctx, `UPDATE aviso_cobranza SET ultima_ejecucion=NULL WHERE id=$1`, aid); err != nil {
		t.Fatal(err)
	}
	cfg := config.Cargar()
	cfg.JWTSecret = "clave-de-pruebas-de-32-bytes-o-mas-123456"
	cfg.WhatsAppModo = "simulado"
	cfg.Tareas = false
	srv, err := app.Nuevo(ctx, e.pool, archivo.NuevaMemoria(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	manana := time.Now().AddDate(0, 0, 1)
	hoyT := time.Date(manana.Year(), manana.Month(), manana.Day(), 0, 0, 0, 0, time.UTC)
	if n, err := srv.AvisosDelDia(ctx, hoyT); err != nil || n != 1 {
		t.Fatalf("tarea diaria: %d %v", n, err)
	}
	if n, _ := srv.AvisosDelDia(ctx, hoyT); n != 0 {
		t.Fatalf("la tarea no repite el mismo día: %d", n)
	}
	if st, l := e.pedir("GET", "/api/v1/edificios/1/avisos-cobranza/envios", tok, nil); st != 200 || l["total"].(float64) < 1 {
		t.Fatalf("envíos: %d %v", st, l)
	}
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/avisos-cobranza", e.login("junta@demo.pe"), nil); st != 403 {
		t.Fatalf("la junta no configura avisos: %d", st)
	}
}

// ---------- D3 · morosos y puntualidad ----------

func TestGrillaMorosos(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	if st, d := e.pedir("PUT", "/api/v1/edificios/1/deuda/config", tok, map[string]any{"etiqueta_moroso": "Deudor", "etiqueta_puntual": "Al día", "tasa_mora_bp": 100}); st != 200 {
		t.Fatalf("config: %d %v", st, d)
	}
	st, g := e.pedir("GET", "/api/v1/edificios/1/morosos?desde=2026-04&hasta=2026-09", tok, nil)
	if st != 200 {
		t.Fatalf("grilla: %d %v", st, g)
	}
	if len(g["meses"].([]any)) != 6 || g["etiquetas"].(map[string]any)["moroso"] != "Deudor" {
		t.Fatalf("meses/etiquetas: %v %v", g["meses"], g["etiquetas"])
	}
	porMes := g["por_mes"].(map[string]any)
	var morososSet float64
	for _, it := range g["unidades"].([]any) {
		u := it.(map[string]any)
		c, _ := u["celdas"].(map[string]any)["2026-09"].(map[string]any)
		switch u["unidad"] {
		case "402", "503", "104":
			if c["estado"] != "moroso" {
				t.Fatalf("%v en setiembre: %v", u["unidad"], c)
			}
			morososSet++
		case "201":
			if c["estado"] == "moroso" {
				t.Fatalf("201 no es moroso en setiembre: %v", c)
			}
		}
	}
	if porMes["2026-09"].(map[string]any)["morosos"].(float64) != morososSet {
		t.Fatalf("resumen de setiembre: %v", porMes["2026-09"])
	}
	// El 402 pagó tarde abril: la celda lo dice con la fecha de pago.
	for _, it := range g["unidades"].([]any) {
		u := it.(map[string]any)
		if u["unidad"] == "402" {
			c := u["celdas"].(map[string]any)["2026-04"].(map[string]any)
			if c["estado"] == "puntual" || c["pagado_en"] == nil {
				t.Fatalf("402 abril (pagó tarde): %v", c)
			}
		}
	}
	// La tasa de mora configurada aparece en la propuesta del acuerdo.
	_, pr := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/acuerdos/propuesta?unidad_id=%d", e.idUnidad("402")), tok, nil)
	if pr["tasa_mora_bp"].(float64) != 100 || pr["mora_cts"].(float64) <= 0 {
		t.Fatalf("mora en la propuesta: %v", pr)
	}
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/morosos", e.login("propietario201@demo.pe"), nil); st != 403 {
		t.Fatalf("propietario no ve la grilla de morosos: %d", st)
	}
}
