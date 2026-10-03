package app_test

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

// Bloques F1–F3 · personal (colaboradores, asistencia con foto) y almacén.

// pngFalso: lo mínimo para que DetectContentType diga image/png.
var pngFalso = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x02\x00\x00\x00")

// multipartPersonal manda un formulario con campos y, si nombreArchivo no es "", un archivo.
func (e *entorno) multipartPersonal(ruta, tok string, campos map[string]string, campoArchivo, nombreArchivo string, datos []byte) (int, map[string]any) {
	e.t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for k, v := range campos {
		_ = mw.WriteField(k, v)
	}
	if nombreArchivo != "" {
		fw, _ := mw.CreateFormFile(campoArchivo, nombreArchivo)
		_, _ = fw.Write(datos)
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+ruta, &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	return e.hacer(req)
}

func (e *entorno) usuarioID(correo string) int64 {
	e.t.Helper()
	var id int64
	if err := e.pool.QueryRow(context.Background(), `SELECT id FROM usuario WHERE correo=$1`, correo).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

const pdfFalso = "%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF"

// F1 · colaboradores, documentos con URL firmada y lo que ve el propietario.
func TestPersonalColaboradores(t *testing.T) {
	e := nuevo(t)
	adm := e.login("admin@demo.pe")
	prop := e.login("propietario201@demo.pe")
	base := "/api/v1/edificios/1"

	st, d := e.pedir("POST", base+"/colaboradores", adm, map[string]any{"nombre": "Rosa Portera", "num_doc": "45120987", "cargo": "Conserje", "telefono": "999111222"})
	if st != 201 {
		t.Fatalf("crear colaborador: %d %v", st, d)
	}
	cid := num(d["id"])
	if st, d := e.pedir("POST", base+"/colaboradores", adm, map[string]any{"nombre": "Otra", "num_doc": "45120987"}); st != 409 || codigo(d) != "COLABORADOR_DUPLICADO" {
		t.Errorf("documento repetido: %d %v", st, d)
	}
	if st, d := e.pedir("POST", base+"/colaboradores", adm, map[string]any{"nombre": "Corto", "num_doc": "123"}); st != 422 {
		t.Errorf("DNI de 3 dígitos: %d %v", st, d)
	}
	if st, _ := e.pedir("POST", base+"/colaboradores", prop, map[string]any{"nombre": "Intruso"}); st != 403 {
		t.Errorf("el propietario no registra colaboradores: %d", st)
	}

	docs := fmt.Sprintf("%s/colaboradores/%d/documentos", base, cid)
	if st, d := e.multipartPersonal(docs, adm, map[string]string{"tipo": "cv", "titulo": "CV 2026"}, "archivo", "cv.pdf", []byte(pdfFalso)); st != 201 {
		t.Fatalf("subir CV: %d %v", st, d)
	}
	if st, d := e.multipartPersonal(docs, adm, map[string]string{"tipo": "plame", "titulo": "PLAME"}, "archivo", "plame.pdf", []byte(pdfFalso)); st != 422 {
		t.Errorf("PLAME sin periodo: %d %v", st, d)
	}
	if st, d := e.multipartPersonal(docs, adm, map[string]string{"tipo": "plame", "titulo": "PLAME setiembre", "periodo": "2026-09", "visible": "0"}, "archivo", "plame.pdf", []byte(pdfFalso)); st != 201 {
		t.Fatalf("subir PLAME oculta: %d %v", st, d)
	}

	if _, l := e.pedir("GET", docs, adm, nil); len(l["datos"].([]any)) != 2 {
		t.Errorf("la administración ve los 2 documentos: %v", l)
	}
	_, l := e.pedir("GET", docs, prop, nil)
	lista := l["datos"].([]any)
	if len(lista) != 1 || lista[0].(map[string]any)["tipo"] != "cv" {
		t.Fatalf("el propietario solo ve lo visible: %v", l)
	}
	// La URL firmada abre el archivo; alterada, no.
	url := lista[0].(map[string]any)["archivo_url"].(string)
	if res, err := http.Get(e.srv.URL + url); err != nil || res.StatusCode != 200 {
		t.Errorf("abrir con URL firmada: %v %v", res, err)
	}
	if res, _ := http.Get(e.srv.URL + strings.Replace(url, "firma=", "firma=00", 1)); res.StatusCode != 403 {
		t.Errorf("firma alterada debería dar 403: %d", res.StatusCode)
	}

	// El propietario ve al colaborador, pero sin DNI completo ni contacto.
	_, lc := e.pedir("GET", base+"/colaboradores", prop, nil)
	var visto map[string]any
	for _, it := range lc["datos"].([]any) {
		if num(it.(map[string]any)["id"]) == cid {
			visto = it.(map[string]any)
		}
	}
	if visto == nil || visto["num_doc"] != "4512****" || visto["telefono"] != "" || num(visto["documentos"]) != 1 {
		t.Errorf("vista del propietario: %v", visto)
	}
}

// F2 · turnos, marcado con foto, puntualidad, checklist y reglas duras de la asistencia.
func TestAsistenciaConFoto(t *testing.T) {
	e := nuevo(t)
	ctx := context.Background()
	adm := e.login("admin@demo.pe")
	ope := e.login("operario@demo.pe")
	base := "/api/v1/edificios/1"
	// Parte de cero: la semilla trae colaboradores (uno con la cuenta del operario), su asistencia y el checklist.
	if _, err := e.pool.Exec(ctx, `DELETE FROM colaborador WHERE edificio_id=1; DELETE FROM checklist_item WHERE edificio_id=1`); err != nil {
		t.Fatal(err)
	}

	st, d := e.pedir("POST", base+"/turnos", adm, map[string]any{"nombre": "Prueba mañana", "hora_entrada": "08:00", "hora_salida": "16:00", "tolerancia_min": 10, "dias": []int{1, 2, 3, 4, 5, 6, 7}})
	if st != 201 {
		t.Fatalf("crear turno: %d %v", st, d)
	}
	tid := num(d["id"])
	if st, _ := e.pedir("POST", base+"/turnos", adm, map[string]any{"nombre": "Malo", "hora_entrada": "8h", "hora_salida": "16:00"}); st != 422 {
		t.Errorf("hora mal escrita: %d", st)
	}
	st, d = e.pedir("POST", base+"/colaboradores", adm, map[string]any{"nombre": "Óscar Operario", "cargo": "Conserje",
		"usuario_id": e.usuarioID("operario@demo.pe"), "turno_id": tid, "fecha_ingreso": "2026-09-01"})
	if st != 201 {
		t.Fatalf("crear colaborador enlazado: %d %v", st, d)
	}
	cid := num(d["id"])
	_, d = e.pedir("POST", base+"/colaboradores", adm, map[string]any{"nombre": "Nocturno sin cuenta", "turno_id": tid, "fecha_ingreso": "2026-09-01"})
	otro := num(d["id"])
	if st, d := e.pedir("POST", base+"/colaboradores", adm, map[string]any{"nombre": "Duplica cuenta", "usuario_id": e.usuarioID("operario@demo.pe")}); st != 409 || codigo(d) != "CUENTA_EN_USO" {
		t.Errorf("una cuenta, un colaborador: %d %v", st, d)
	}

	marcar := base + "/asistencia/marcar"
	if st, d := e.multipartPersonal(marcar, ope, map[string]string{"tipo": "entrada"}, "", "", nil); st != 422 {
		t.Errorf("sin foto no se marca: %d %v", st, d)
	}
	if st, d := e.multipartPersonal(marcar, ope, map[string]string{"tipo": "salida"}, "foto", "s.png", pngFalso); st != 409 || codigo(d) != "SIN_ENTRADA" {
		t.Errorf("salida sin entrada: %d %v", st, d)
	}
	st, d = e.multipartPersonal(marcar, ope, map[string]string{"tipo": "entrada"}, "foto", "e.png", pngFalso)
	if st != 201 || d["entrada_foto_url"] == nil || d["minutos_tarde"] == nil {
		t.Fatalf("marcar entrada: %d %v", st, d)
	}
	hoyID := num(d["id"])
	if st, d := e.multipartPersonal(marcar, ope, map[string]string{"tipo": "entrada"}, "foto", "e.png", pngFalso); st != 409 || codigo(d) != "YA_MARCO_ENTRADA" {
		t.Errorf("doble entrada: %d %v", st, d)
	}
	if st, d := e.multipartPersonal(marcar, ope, map[string]string{"tipo": "salida"}, "foto", "s.png", pngFalso); st != 201 || d["salida"] == nil {
		t.Errorf("marcar salida: %d %v", st, d)
	}
	if st, d := e.multipartPersonal(marcar, e.login("tecnico@demo.pe"), map[string]string{"tipo": "entrada"}, "foto", "e.png", pngFalso); st != 409 || codigo(d) != "SIN_COLABORADOR" {
		t.Errorf("cuenta sin colaborador: %d %v", st, d)
	}
	if st, _ := e.multipartPersonal(marcar, e.login("propietario201@demo.pe"), map[string]string{"tipo": "entrada"}, "foto", "e.png", pngFalso); st != 403 {
		t.Errorf("el propietario no marca asistencia: %d", st)
	}

	// Registro manual con hora: 08:05 entra en la tolerancia; 08:25 llega 25 min tarde.
	manual := base + "/asistencia/manual"
	st, d = e.pedir("POST", manual, adm, map[string]any{"colaborador_id": cid, "tipo": "entrada", "en": "2026-09-01T08:05", "nota": "Olvidó el teléfono"})
	if st != 201 || d["puntual"] != true || num(d["minutos_tarde"]) != 5 || d["manual"] != true {
		t.Fatalf("manual 08:05: %d %v", st, d)
	}
	st, d = e.pedir("POST", manual, adm, map[string]any{"colaborador_id": cid, "tipo": "entrada", "en": "2026-09-02T08:25", "nota": "Corte de luz"})
	if st != 201 || d["puntual"] != false || num(d["minutos_tarde"]) != 25 {
		t.Fatalf("manual 08:25: %d %v", st, d)
	}
	if st, d := e.pedir("POST", manual, adm, map[string]any{"colaborador_id": cid, "tipo": "salida", "en": "2026-09-02T07:00", "nota": "Antes de entrar"}); st != 409 || codigo(d) != "SIN_ENTRADA" {
		t.Errorf("salida anterior a la entrada: %d %v", st, d)
	}
	if st, d := e.pedir("POST", manual, adm, map[string]any{"colaborador_id": cid, "tipo": "salida", "en": "2026-09-02T16:10", "nota": "Cierre"}); st != 201 {
		t.Errorf("salida manual: %d %v", st, d)
	}
	if st, _ := e.pedir("POST", manual, adm, map[string]any{"colaborador_id": cid, "tipo": "entrada", "en": "2026-09-03T08:00"}); st != 422 {
		t.Errorf("manual sin nota: %d", st)
	}
	if st, _ := e.pedir("POST", manual, adm, map[string]any{"colaborador_id": cid, "tipo": "entrada", "en": "2099-01-01T08:00", "nota": "x"}); st != 422 {
		t.Errorf("manual a futuro: %d", st)
	}

	// Panel de puntualidad del 1 al 7 de setiembre: 2 marcas (1 puntual, 1 tarde), 5 faltas, 1 sin salida.
	_, p := e.pedir("GET", base+"/asistencia/puntualidad?desde=2026-09-01&hasta=2026-09-07", adm, nil)
	var fila map[string]any
	for _, it := range p["datos"].([]any) {
		if num(it.(map[string]any)["id"]) == cid {
			fila = it.(map[string]any)
		}
	}
	if fila == nil || num(fila["asistencias"]) != 2 || num(fila["puntuales"]) != 1 || num(fila["tardanzas"]) != 1 ||
		num(fila["minutos_tarde"]) != 30 || num(fila["faltas"]) != 5 || num(fila["sin_salida"]) != 1 || fila["pct_puntualidad"] != 50.0 {
		t.Errorf("puntualidad: %v", fila)
	}
	if _, dia := e.pedir("GET", base+"/asistencia?fecha=2026-09-03", adm, nil); len(dia["ausentes"].([]any)) != 2 {
		t.Errorf("el 3/9 faltan los dos colaboradores con turno: %v", dia["ausentes"])
	}
	if st, _ := e.pedir("GET", base+"/asistencia/puntualidad", ope, nil); st != 403 {
		t.Errorf("el operario no ve el panel: %d", st)
	}

	// Checklist: el operario marca lo de su propia asistencia, no la ajena.
	st, d = e.pedir("POST", base+"/checklist", adm, map[string]any{"texto": "Revisar bombas de agua"})
	if st != 201 {
		t.Fatalf("crear tarea: %d %v", st, d)
	}
	item := num(d["id"])
	if st, d := e.pedir("POST", fmt.Sprintf("%s/asistencia/%d/checklist", base, hoyID), ope, map[string]any{"item_id": item, "hecho": true}); st != 200 {
		t.Errorf("marcar tarea propia: %d %v", st, d)
	}
	_, hoy := e.pedir("GET", base+"/asistencia/hoy", ope, nil)
	its := hoy["checklist"].([]any)
	if len(its) != 1 || its[0].(map[string]any)["hecho"] != true || hoy["asistencia"] == nil {
		t.Errorf("mi asistencia de hoy: %v", hoy)
	}
	_, d = e.pedir("POST", manual, adm, map[string]any{"colaborador_id": otro, "tipo": "entrada", "en": "2026-09-01T22:00", "nota": "Registro"})
	if st, _ := e.pedir("POST", fmt.Sprintf("%s/asistencia/%d/checklist", base, num(d["id"])), ope, map[string]any{"item_id": item, "hecho": true}); st != 403 {
		t.Errorf("checklist de otro colaborador: %d", st)
	}

	// Regla dura en la base: una marca del colaborador (no manual) sin foto no entra.
	_, err := e.pool.Exec(ctx, `INSERT INTO asistencia (edificio_id, colaborador_id, fecha, entrada_en) VALUES (1,$1,'2026-08-01','2026-08-01 08:00-05')`, cid)
	if err == nil || !strings.Contains(err.Error(), "23514") {
		t.Errorf("la base aceptó una marca sin foto: %v", err)
	}
}

// F3 · almacén: kárdex, stock mínimo con alerta, sin stock negativo, ajuste solo de la administración.
func TestAlmacen(t *testing.T) {
	e := nuevo(t)
	ctx := context.Background()
	adm := e.login("admin@demo.pe")
	ope := e.login("operario@demo.pe")
	base := "/api/v1/edificios/1"
	movs := base + "/almacen/movimientos"

	st, d := e.pedir("POST", base+"/almacen/articulos", adm, map[string]any{"nombre": "Lejía 1 L", "codigo": "LEJ-1", "costo_cts": 450, "stock_minimo": 5, "stock_inicial": 10})
	if st != 201 {
		t.Fatalf("crear artículo: %d %v", st, d)
	}
	pid := num(d["id"])

	st, d = e.pedir("POST", movs, ope, map[string]any{"producto_id": pid, "tipo": "salida", "cantidad": 4, "motivo": "Limpieza de cocheras"})
	if st != 201 || d["saldo"] != 6.0 || d["alerta"] != nil {
		t.Fatalf("salida de 4: %d %v", st, d)
	}
	st, d = e.pedir("POST", movs, ope, map[string]any{"producto_id": pid, "tipo": "salida", "cantidad": 2, "motivo": "Limpieza de escaleras"})
	if st != 201 || d["saldo"] != 4.0 || d["alerta"] == nil {
		t.Errorf("bajo el mínimo debe avisar: %d %v", st, d)
	}
	if st, d := e.pedir("POST", movs, ope, map[string]any{"producto_id": pid, "tipo": "salida", "cantidad": 10, "motivo": "Todo"}); st != 409 || codigo(d) != "STOCK_INSUFICIENTE" {
		t.Errorf("salida mayor al stock: %d %v", st, d)
	}
	if st, _ := e.pedir("POST", movs, ope, map[string]any{"producto_id": pid, "tipo": "salida", "cantidad": 1}); st != 422 {
		t.Errorf("salida sin motivo: %d", st)
	}
	if st, _ := e.pedir("POST", movs, ope, map[string]any{"producto_id": pid, "tipo": "ajuste", "cantidad": 3, "motivo": "Conteo"}); st != 403 {
		t.Errorf("el operario no ajusta inventario: %d", st)
	}
	if st, d := e.pedir("POST", movs, adm, map[string]any{"producto_id": pid, "tipo": "ajuste", "cantidad": 3, "motivo": "Conteo de fin de mes"}); st != 201 || d["delta"] != -1.0 || d["saldo"] != 3.0 {
		t.Errorf("ajuste a 3: %d %v", st, d)
	}
	if _, a := e.pedir("GET", base+"/almacen/alertas", ope, nil); len(a["datos"].([]any)) < 1 {
		t.Errorf("con 3 de 5 debe estar en alertas: %v", a)
	}
	if st, d := e.pedir("POST", movs, adm, map[string]any{"producto_id": pid, "tipo": "entrada", "cantidad": 2.5, "costo_unit_cts": 500}); st != 201 || d["saldo"] != 5.5 {
		t.Errorf("entrada de 2,5: %d %v", st, d)
	}
	_, a := e.pedir("GET", base+"/almacen/alertas", adm, nil)
	for _, it := range a["datos"].([]any) {
		if num(it.(map[string]any)["id"]) == pid {
			t.Errorf("con 5,5 sobre 5 ya no hay alerta")
		}
	}

	// Invariante del kárdex: Σ deltas = stock = saldo del último movimiento; la entrada actualizó el costo.
	var suma, stock, ultimo float64
	var costo int64
	_ = e.pool.QueryRow(ctx, `SELECT (SELECT sum(delta) FROM almacen_movimiento WHERE producto_id=$1)::float8, p.stock::float8,
		(SELECT saldo FROM almacen_movimiento WHERE producto_id=$1 ORDER BY id DESC LIMIT 1)::float8, p.costo_cts FROM producto p WHERE p.id=$1`, pid).Scan(&suma, &stock, &ultimo, &costo)
	if suma != 5.5 || stock != 5.5 || ultimo != 5.5 || costo != 500 {
		t.Errorf("kárdex descuadrado: Σ=%v stock=%v último=%v costo=%d", suma, stock, ultimo, costo)
	}
	if _, l := e.pedir("GET", fmt.Sprintf("%s?producto_id=%d", movs, pid), ope, nil); num(l["total"]) != 5 {
		t.Errorf("5 movimientos (inicial, 2 salidas, ajuste, entrada): %v", l["total"])
	}

	// Un servicio sin control de stock no se mueve hasta activarlo.
	var serv int64
	_ = e.pool.QueryRow(ctx, `SELECT id FROM producto WHERE edificio_id=1 AND NOT controla_stock ORDER BY id LIMIT 1`).Scan(&serv)
	if st, _ := e.pedir("POST", movs, adm, map[string]any{"producto_id": serv, "tipo": "entrada", "cantidad": 1}); st != 422 {
		t.Errorf("producto sin control de stock: %d", st)
	}
	if st, d := e.pedir("PUT", fmt.Sprintf("%s/almacen/articulos/%d", base, serv), adm, map[string]any{"controla_stock": true, "stock_minimo": 1}); st != 200 || d["controla_stock"] != true {
		t.Errorf("activar control: %d %v", st, d)
	}
	if st, _ := e.pedir("POST", movs, adm, map[string]any{"producto_id": serv, "tipo": "entrada", "cantidad": 1}); st != 201 {
		t.Errorf("entrada tras activar: %d", st)
	}

	// Regla dura en la base: el stock no baja de cero.
	if _, err := e.pool.Exec(ctx, `UPDATE producto SET stock=-1 WHERE id=$1`, pid); err == nil || !strings.Contains(err.Error(), "23514") {
		t.Errorf("la base aceptó stock negativo: %v", err)
	}
}

// La semilla de F1–F3 es coherente: la quincena del conserje cuadra en el panel y el kárdex con el stock.
func TestSemillaPersonal(t *testing.T) {
	e := nuevo(t)
	adm := e.login("admin@demo.pe")
	// 16–30/9 sin los domingos 20 y 27 ni la falta del 23: 12 marcas, 4 fuera de tolerancia (12, 25, 15 y 30 min)
	// y 97 minutos tarde en total (3+12+25+1+15+9+2+30).
	_, p := e.pedir("GET", "/api/v1/edificios/1/asistencia/puntualidad?desde=2026-09-16&hasta=2026-09-30", adm, nil)
	var oscar map[string]any
	for _, it := range p["datos"].([]any) {
		if it.(map[string]any)["nombre"] == "Óscar Operario" {
			oscar = it.(map[string]any)
		}
	}
	if oscar == nil || num(oscar["asistencias"]) != 12 || num(oscar["tardanzas"]) != 4 || num(oscar["faltas"]) != 1 || num(oscar["minutos_tarde"]) != 97 {
		t.Errorf("quincena del conserje: %v", oscar)
	}
	var descuadres int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM producto p
		WHERE p.controla_stock AND p.stock <> COALESCE((SELECT sum(delta) FROM almacen_movimiento m WHERE m.producto_id=p.id),0)`).Scan(&descuadres)
	if descuadres != 0 {
		t.Errorf("%d productos con stock distinto a Σ movimientos", descuadres)
	}
	if _, a := e.pedir("GET", "/api/v1/edificios/1/almacen/alertas", adm, nil); len(a["datos"].([]any)) != 1 {
		t.Errorf("la placa de parqueo debe estar en alerta: %v", a)
	}
	// El propietario ve a los tres y solo los CV (las PLAME son de la administración).
	prop := e.login("propietario201@demo.pe")
	_, lc := e.pedir("GET", "/api/v1/edificios/1/colaboradores", prop, nil)
	if len(lc["datos"].([]any)) != 3 {
		t.Errorf("tres colaboradores en la semilla: %v", lc)
	}
	for _, it := range lc["datos"].([]any) {
		if num(it.(map[string]any)["documentos"]) != 1 {
			t.Errorf("el propietario solo ve el CV: %v", it)
		}
	}
}
