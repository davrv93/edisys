package app_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"edisys/api/internal/app"
)

// subirRecaudacion manda un multipart con «archivo» y campos sueltos.
func (e *entorno) subirRecaudacion(ruta, tok, nombre string, datos []byte, campos map[string]string) (int, map[string]any) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("archivo", nombre)
	_, _ = fw.Write(datos)
	for k, v := range campos {
		_ = mw.WriteField(k, v)
	}
	mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+ruta, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	return e.hacer(req)
}

// textoDe pide una ruta y devuelve el cuerpo crudo (descargas de texto).
func (e *entorno) textoDe(ruta, tok string) (int, string) {
	e.t.Helper()
	req, _ := http.NewRequest("GET", e.srv.URL+ruta, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// reciboConSaldo devuelve un recibo emitido con saldo del edificio demo.
func (e *entorno) reciboConSaldo() (id int64, numero, unidad string, saldo int64) {
	e.t.Helper()
	if err := e.pool.QueryRow(context.Background(), `SELECT r.id, r.numero, u.codigo, r.total_cts - r.pagado_cts FROM recibo r JOIN unidad u ON u.id=r.unidad_id
		WHERE r.edificio_id=1 AND r.estado IN ('emitido','pagado_parcial') AND r.total_cts > r.pagado_cts AND r.numero IS NOT NULL
		ORDER BY r.id LIMIT 1`).Scan(&id, &numero, &unidad, &saldo); err != nil {
		e.t.Fatalf("sin recibo con saldo: %v", err)
	}
	return
}

func (e *entorno) pagadoDe(reciboID int64) int64 {
	var p int64
	_ = e.pool.QueryRow(context.Background(), `SELECT pagado_cts FROM recibo WHERE id=$1`, reciboID).Scan(&p)
	return p
}

func (e *entorno) pagosConCodigo(cod string) int {
	var n int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM pago WHERE codigo_operacion=$1 AND estado='validado'`, cod).Scan(&n)
	return n
}

// Bloque A1 · la comisión pactada y el neto, al céntimo.
func TestComisionYNetoRecaudadora(t *testing.T) {
	casos := []struct {
		bruto int64
		pbs   int
		fijo  int64
		com   int64
	}{
		{10000, 150, 100, 250}, // 1,50 % de S/ 100 + S/ 1 fijo
		{333, 150, 0, 5},       // 4,995 → 5 (redondeo al céntimo)
		{50, 0, 100, 50},       // la comisión nunca pasa el bruto
		{10000, 0, 0, 0},
	}
	for _, c := range casos {
		got := app.ComisionCts(c.bruto, c.pbs, c.fijo)
		if got != c.com {
			t.Errorf("ComisionCts(%d,%d,%d) = %d, quiero %d", c.bruto, c.pbs, c.fijo, got, c.com)
		}
		if n := app.NetoLiquidacion(c.bruto, got); n != c.bruto-c.com {
			t.Errorf("neto %d ≠ bruto − comisión", n)
		}
	}
}

func crearCuentaRecaudadora(t *testing.T, e *entorno, tok string, pbs int, fijo int64) int64 {
	t.Helper()
	st, d := e.pedir("POST", "/api/v1/edificios/1/recaudadora/cuentas", tok, map[string]any{
		"proveedor": "Agentes Kasnet", "codigo_convenio": fmt.Sprintf("CONV-%d-%d", pbs, fijo), "medio": "deposito", "porcentaje_pbs": pbs, "fijo_cts": fijo,
	})
	if st != 201 {
		t.Fatalf("crear cuenta recaudadora: %d %v", st, d)
	}
	return int64(d["id"].(float64))
}

// Bloque A1 · doble carga no duplica; la vista previa no guarda; lo que no casa queda observado.
func TestRecaudadoraTransaccionesIdempotentes(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	cid := crearCuentaRecaudadora(t, e, tok, 0, 0)
	rid, numero, _, saldo := e.reciboConSaldo()
	monto := saldo
	if monto > 5000 {
		monto = 5000
	}
	antes := e.pagadoDe(rid)
	csv := fmt.Sprintf("Codigo Pago;Importe;Nro Operacion;Fecha;Canal\n%s;%d,%02d;REC-OP-1;30/09/2026;Agente\nNO-EXISTE;10,00;REC-OP-2;30/09/2026;Yape\n;;;;\n",
		numero, monto/100, monto%100)
	ruta := "/api/v1/edificios/1/recaudadora/cuentas/" + itoa(cid) + "/transacciones/importar"

	st, d := e.subirRecaudacion(ruta, tok, "transacciones.csv", []byte(csv), map[string]string{"vista_previa": "1"})
	if st != 200 || int(d["acreditadas"].(float64)) != 1 || int(d["observadas"].(float64)) != 1 {
		t.Fatalf("vista previa: %d %v", st, d)
	}
	if n := e.pagosConCodigo("REC-OP-1"); n != 0 {
		t.Fatalf("la vista previa guardó %d pagos", n)
	}

	st, d = e.subirRecaudacion(ruta, tok, "transacciones.csv", []byte(csv), nil)
	if st != 200 || int(d["acreditadas"].(float64)) != 1 || int(d["observadas"].(float64)) != 1 {
		t.Fatalf("primera carga: %d %v", st, d)
	}
	if got := e.pagadoDe(rid) - antes; got != monto {
		t.Fatalf("el recibo subió %d, quiero %d", got, monto)
	}

	// Segunda carga del mismo archivo: nada nuevo.
	st, d = e.subirRecaudacion(ruta, tok, "transacciones.csv", []byte(csv), nil)
	if st != 200 || int(d["acreditadas"].(float64)) != 0 || int(d["duplicadas"].(float64)) != 1 {
		t.Fatalf("segunda carga: %d %v", st, d)
	}
	if got := e.pagadoDe(rid) - antes; got != monto {
		t.Fatalf("doble carga duplicó: el recibo subió %d, quiero %d", got, monto)
	}
	if n := e.pagosConCodigo("REC-OP-1"); n != 1 {
		t.Fatalf("pagos con REC-OP-1 = %d, quiero 1", n)
	}

	st, d = e.pedir("GET", "/api/v1/edificios/1/recaudadora/transacciones", tok, nil)
	if st != 200 || len(d["datos"].([]any)) != 2 || num(d["acreditado_cts"]) != monto {
		t.Fatalf("reporte de transacciones: %d %v", st, d)
	}
	// El recibo pagado por la recaudadora queda marcado como enviado a ella.
	var enviado bool
	_ = e.pool.QueryRow(context.Background(), `SELECT enviado_recaudadora_en IS NOT NULL FROM recibo WHERE id=$1`, rid).Scan(&enviado)
	if !enviado {
		t.Fatal("el recibo no quedó «Enviado a recaudadora»")
	}
}

// Bloque A1 · neto = bruto − comisión; la comisión entra como egreso una sola vez.
func TestRecaudadoraLiquidaciones(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	cid := crearCuentaRecaudadora(t, e, tok, 150, 100)
	csv := "Liquidacion,Fecha,Bruto,Neto\nL-1,2026-09-30,100.00,97.50\nL-2,2026-09-30,50.00,10.00\n"
	ruta := "/api/v1/edificios/1/recaudadora/cuentas/" + itoa(cid) + "/liquidaciones/importar"

	st, d := e.subirRecaudacion(ruta, tok, "liquidaciones.csv", []byte(csv), nil)
	if st != 200 || int(d["registradas"].(float64)) != 1 || int(d["errores"].(float64)) != 1 {
		t.Fatalf("liquidaciones: %d %v", st, d)
	}
	if num(d["bruto_cts"]) != 10000 || num(d["comision_cts"]) != 250 || num(d["neto_cts"]) != 9750 {
		t.Fatalf("totales: %v", d)
	}
	ctx := context.Background()
	var bruto, com, neto, egreso int64
	if err := e.pool.QueryRow(ctx, `SELECT l.monto_bruto_cts, l.comision_cts, l.monto_neto_cts, eg.monto_cts FROM recaudadora_liquidacion l
		JOIN egreso eg ON eg.id=l.egreso_id WHERE l.codigo_liquidacion='L-1'`).Scan(&bruto, &com, &neto, &egreso); err != nil {
		t.Fatalf("liquidación sin egreso: %v", err)
	}
	if neto != bruto-com || egreso != com {
		t.Fatalf("bruto %d comisión %d neto %d egreso %d", bruto, com, neto, egreso)
	}

	st, d = e.subirRecaudacion(ruta, tok, "liquidaciones.csv", []byte(csv), nil)
	if st != 200 || int(d["duplicadas"].(float64)) != 1 || int(d["registradas"].(float64)) != 0 {
		t.Fatalf("segunda carga de liquidaciones: %d %v", st, d)
	}
	var egresos int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM egreso WHERE descripcion LIKE 'Comisión % · liquidación L-1'`).Scan(&egresos)
	if egresos != 1 {
		t.Fatalf("egresos de comisión = %d, quiero 1", egresos)
	}
	// La base no deja un neto que no cuadre.
	if _, err := e.pool.Exec(ctx, `INSERT INTO recaudadora_liquidacion (edificio_id, cuenta_recaudadora_id, codigo_liquidacion, fecha, monto_bruto_cts, comision_cts, monto_neto_cts)
		VALUES (1,$1,'L-X','2026-09-30',1000,10,1000)`, cid); err == nil {
		t.Fatal("la base aceptó neto ≠ bruto − comisión")
	}
}

// Bloque A1 · permisos: el propietario no ve la recaudadora.
func TestRecaudadoraPermisos(t *testing.T) {
	e := nuevo(t)
	tok := e.login("propietario201@demo.pe")
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/recaudadora/cuentas", tok, nil); st != 403 && st != 404 {
		t.Fatalf("propietario en recaudadora: %d", st)
	}
}
