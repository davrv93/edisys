package app_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"edisys/api/internal/app"
)

// lineaCDPG arma un detalle «DD» del CDPG de referencia BCP (posiciones fijas, 250 de ancho).
func lineaCDPG(dep, ref, fecha string, cts int64, agencia, op string) string {
	s := "DD" + "1930" + "1234567" + fmt.Sprintf("%-14s%-30s", dep, ref) + fecha + fecha +
		fmt.Sprintf("%015d%015d%015d", cts, 0, cts) + fmt.Sprintf("%-6s%-6s", agencia, op)
	return s + strings.Repeat(" ", 250-len(s))
}

// Bloque A2 · contrato del layout: cabecera y cola exactas, todas las líneas de 250 con CRLF.
func TestCrepCabeceraYColaExactas(t *testing.T) {
	l, ok := app.LayoutDeBanco("bcp_ref")
	if !ok || !l.Provisional() {
		t.Fatal("el layout de referencia BCP debe existir y estar marcado como provisional")
	}
	f := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	dets := []app.CrepDetalle{
		{Depositante: "402", Nombre: "María Núñez", Referencia: "2026-09-402", Emision: f, Vence: f.AddDate(0, 0, 9), MontoCts: 10000},
		{Depositante: "201", Nombre: "José Pérez", Referencia: "2026-09-201", Emision: f, Vence: f.AddDate(0, 0, 9), MontoCts: 5000},
	}
	txt, err := app.GenerarCREP(l, app.CrepCabecera{Cuenta: "193-1234567-0-12", Moneda: "PEN", Empresa: "Edificio Demo Ñandú", Fecha: f}, dets)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(txt, "\r\n") {
		t.Fatal("el archivo debe terminar en CRLF")
	}
	lineas := strings.Split(strings.TrimSuffix(txt, "\r\n"), "\r\n")
	if len(lineas) != 4 {
		t.Fatalf("líneas = %d, quiero cabecera + 2 detalles + cola", len(lineas))
	}
	for i, ln := range lineas {
		if len(ln) != 250 {
			t.Errorf("línea %d mide %d, quiero 250", i+1, len(ln))
		}
	}
	empresa := "EDIFICIO DEMO NANDU" + strings.Repeat(" ", 40-len("EDIFICIO DEMO NANDU"))
	cab := "CC" + "193" + "0" + "1234567" + "C" + empresa + "20261001" + "000000002" + "000000000015000" + "R"
	if lineas[0] != cab+strings.Repeat(" ", 250-len(cab)) {
		t.Errorf("cabecera:\n%q\nquiero\n%q", strings.TrimRight(lineas[0], " "), cab)
	}
	cola := "TT" + "000000002" + "000000000015000"
	if lineas[3] != cola+strings.Repeat(" ", 250-len(cola)) {
		t.Errorf("cola: %q", strings.TrimRight(lineas[3], " "))
	}
	d := lineas[1]
	if d[13:27] != "402           " || strings.TrimSpace(d[67:97]) != "2026-09-402" || d[113:128] != "000000000010000" || d[152] != 'A' {
		t.Errorf("detalle con posiciones corridas: %q", strings.TrimRight(d, " "))
	}
	if _, err := app.GenerarCREP(l, app.CrepCabecera{Cuenta: "123", Fecha: f}, dets); err == nil {
		t.Error("una cuenta sin formato BCP debió fallar")
	}
}

// Bloque A2 · una fila inválida del CDPG no rompe el resto.
func TestCdpgFilaInvalidaNoRompe(t *testing.T) {
	l, _ := app.LayoutDeBanco("")
	txt := strings.Join([]string{
		"CC19301234567CEDIFICIO",
		lineaCDPG("402", "2026-09-402", "20260930", 12345, "001234", "000001"),
		"XX basura",
		"DD123",
		lineaCDPG("201", "2026-09-201", "2026AB30", 100, "001234", "000002"), // fecha mala
		lineaCDPG("201", "2026-09-201", "20260930", 5000, "001234", "000003"),
		"",
	}, "\r\n")
	filas, errores := app.LeerCDPG(l, []byte(txt))
	if len(filas) != 2 || len(errores) != 3 {
		t.Fatalf("filas %d errores %d (%v)", len(filas), len(errores), errores)
	}
	if filas[0].MontoCts != 12345 || filas[0].Referencia != "2026-09-402" || filas[0].Fecha != "2026-09-30" || filas[0].NumeroOperacion != "000001" || filas[0].Linea != 2 {
		t.Fatalf("fila mal leída: %+v", filas[0])
	}
}

// Bloque A2 · CREP → descarga 24 h → CDPG con vista previa → un pago no se concilia dos veces.
func TestCrepYCobranzaMasiva(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	ctx := context.Background()

	st, d := e.pedir("POST", "/api/v1/edificios/1/cuentas-bancarias", tok, map[string]any{"banco": "BCP", "numero": "193-1234567-0-12"})
	if st != 201 {
		t.Fatalf("cuenta bancaria: %d %v", st, d)
	}
	cuenta := int64(d["id"].(float64))

	var periodo string
	if err := e.pool.QueryRow(ctx, `SELECT p.periodo FROM recibo r JOIN periodo p ON p.id=r.periodo_id WHERE r.edificio_id=1
		AND r.estado IN ('emitido','pagado_parcial') AND r.total_cts>r.pagado_cts ORDER BY p.periodo DESC LIMIT 1`).Scan(&periodo); err != nil {
		t.Fatalf("sin periodo con deuda: %v", err)
	}
	st, d = e.pedir("POST", "/api/v1/edificios/1/crep", tok, map[string]any{"periodo": periodo, "cuenta_bancaria_id": cuenta, "layout": "bcp_ref"})
	if st != 201 || num(d["filas"]) < 1 || d["provisional"] != true {
		t.Fatalf("generar CREP: %d %v", st, d)
	}
	crep := int64(d["id"].(float64))
	filasCrep := num(d["filas"])

	st, txt := e.textoDe("/api/v1/edificios/1/crep/"+itoa(crep)+"/descargar", tok)
	if st != 200 || !strings.HasPrefix(txt, "CC1930") {
		t.Fatalf("descargar CREP: %d %q", st, txt[:min(len(txt), 40)])
	}
	lineas := strings.Split(strings.TrimSuffix(txt, "\r\n"), "\r\n")
	if int64(len(lineas)) != filasCrep+2 || !strings.HasPrefix(lineas[len(lineas)-1], "TT") {
		t.Fatalf("CREP con %d líneas para %d deudas", len(lineas), filasCrep)
	}

	// CDPG con un pago bueno, una línea ilegible y un pago sin recibo.
	var rid, saldo int64
	var numero, unidad string
	if err := e.pool.QueryRow(ctx, `SELECT r.id, r.numero, u.codigo, r.total_cts-r.pagado_cts FROM recibo r JOIN unidad u ON u.id=r.unidad_id
		JOIN periodo p ON p.id=r.periodo_id WHERE r.edificio_id=1 AND p.periodo=$1 AND r.estado IN ('emitido','pagado_parcial') AND r.total_cts>r.pagado_cts
		ORDER BY r.id LIMIT 1`, periodo).Scan(&rid, &numero, &unidad, &saldo); err != nil {
		t.Fatal(err)
	}
	monto := min(saldo, 3000)
	antes := e.pagadoDe(rid)
	cdpg := strings.Join([]string{
		lineas[0],
		lineaCDPG(unidad, numero, "20260930", monto, "001234", "778899"),
		"DD esto no es un pago",
		lineaCDPG("ZZZ999", "NO-EXISTE", "20260930", 1000, "001234", "778900"),
	}, "\r\n") + "\r\n"

	st, d = e.subirRecaudacion("/api/v1/edificios/1/cdpg", tok, "cdpg.txt", []byte(cdpg), map[string]string{"vista_previa": "1", "crep_archivo_id": itoa(crep)})
	if st != 200 || num(d["filas_ok"]) != 1 || num(d["filas_error"]) != 2 {
		t.Fatalf("vista previa CDPG: %d %v", st, d)
	}
	if n := e.pagosConCodigo("778899"); n != 0 {
		t.Fatalf("la vista previa guardó %d pagos", n)
	}

	st, d = e.subirRecaudacion("/api/v1/edificios/1/cdpg", tok, "cdpg.txt", []byte(cdpg), map[string]string{"crep_archivo_id": itoa(crep)})
	if st != 200 || num(d["filas_ok"]) != 1 || num(d["filas_error"]) != 2 || num(d["total_cts"]) != monto {
		t.Fatalf("cobranza masiva: %d %v", st, d)
	}
	if got := e.pagadoDe(rid) - antes; got != monto {
		t.Fatalf("el recibo subió %d, quiero %d", got, monto)
	}

	// El mismo CDPG otra vez: el pago no se concilia dos veces.
	st, d = e.subirRecaudacion("/api/v1/edificios/1/cdpg", tok, "cdpg.txt", []byte(cdpg), nil)
	if st != 200 || num(d["filas_ok"]) != 0 {
		t.Fatalf("segunda carga CDPG: %d %v", st, d)
	}
	if f := d["filas"].([]any)[0].(map[string]any); f["estado"] != "ya_conciliado" {
		t.Fatalf("estado de la fila repetida: %v", f)
	}
	if n := e.pagosConCodigo("778899"); n != 1 {
		t.Fatalf("pagos con 778899 = %d, quiero 1", n)
	}
	if got := e.pagadoDe(rid) - antes; got != monto {
		t.Fatalf("doble conciliación: el recibo subió %d", got)
	}
	if st, d := e.pedir("GET", "/api/v1/edificios/1/cdpg", tok, nil); st != 200 || len(d["datos"].([]any)) != 2 {
		t.Fatalf("listar cargas CDPG: %d %v", st, d)
	}

	// Pasadas 24 h el archivo expira: 410 y la lista lo muestra expirado.
	if _, err := e.pool.Exec(ctx, `UPDATE crep_archivo SET expira_en = now() - interval '1 hour' WHERE id=$1`, crep); err != nil {
		t.Fatal(err)
	}
	if st, _ := e.textoDe("/api/v1/edificios/1/crep/"+itoa(crep)+"/descargar", tok); st != 410 {
		t.Fatalf("descarga expirada: %d, quiero 410", st)
	}
	st, d = e.pedir("GET", "/api/v1/edificios/1/crep", tok, nil)
	if st != 200 || d["datos"].([]any)[0].(map[string]any)["estado"] != "expirado" {
		t.Fatalf("lista de descargas: %d %v", st, d)
	}
}
