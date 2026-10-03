package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"edisys/api/internal/app"
)

// Bloques I1, I2 e I5 · marca blanca, plantilla de recibo y configuración del edificio.

// pedirCrudo devuelve el cuerpo sin interpretar (PDF).
func (e *entorno) pedirCrudo(metodo, ruta, tok string, cuerpo any) (int, string, []byte) {
	e.t.Helper()
	var body io.Reader
	if cuerpo != nil {
		b, _ := json.Marshal(cuerpo)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(metodo, e.srv.URL+ruta, body)
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	datos, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header.Get("Content-Type"), datos
}

// reciboConCorreo: un recibo emitido cuyo propietario tiene correo (para el PDF y el correo).
func (e *entorno) reciboConCorreo() int64 {
	var rid int64
	err := e.pool.QueryRow(context.Background(), `SELECT r.id FROM recibo r JOIN unidad u ON u.id=r.unidad_id
		JOIN unidad_persona up ON up.unidad_id=u.id AND up.rol='propietario' AND up.hasta IS NULL JOIN persona pe ON pe.id=up.persona_id
		JOIN lectura l ON l.periodo_id=r.periodo_id JOIN medidor m ON m.id=l.medidor_id AND m.unidad_id=u.id AND m.tipo='agua'
		WHERE r.origen='periodo' AND r.estado NOT IN ('borrador','anulado') AND pe.correo <> '' ORDER BY r.id DESC LIMIT 1`).Scan(&rid)
	if err != nil {
		e.t.Fatal(err)
	}
	return rid
}

// ---------- reglas puras ----------

func TestContrasteYTokensDeMarca(t *testing.T) {
	if c := app.Contraste("#FFFFFF", "#000000"); c < 20.99 || c > 21.01 {
		t.Fatalf("blanco/negro debe dar 21:1, da %.2f", c)
	}
	if c := app.Contraste("#155E75", "#FFFFFF"); c < app.ContrasteMinimo {
		t.Fatalf("el acento de EDISYS debe pasar AA con blanco, da %.2f", c)
	}
	// Un amarillo con texto blanco no se lee: se rechaza.
	if ev := app.ValidarMarca(app.Marca{Nombre: "X", ColorPrimario: "#FACC15", ColorFondoLogin: "#000000"}); ev == nil || ev.Campos["color_primario"] == "" {
		t.Fatalf("el amarillo debió rechazarse por contraste: %v", ev)
	}
	if ev := app.ValidarMarca(app.Marca{Nombre: "X", ColorPrimario: "#7C2D12", ColorFondoLogin: "#1C1917", Slug: "Con Espacios"}); ev == nil || ev.Campos["slug"] == "" {
		t.Fatalf("slug inválido aceptado: %v", ev)
	}
	if ev := app.ValidarMarca(app.Marca{Nombre: "Altamira", ColorPrimario: "#7C2D12", ColorFondoLogin: "#1C1917", Slug: "altamira-sac"}); ev != nil {
		t.Fatalf("marca válida rechazada: %v", ev.Campos)
	}
	tk := app.Marca{ColorPrimario: "#7C2D12"}.Tokens()
	if tk["claro"]["--color-acento"] != "#7C2D12" {
		t.Fatalf("acento claro: %v", tk["claro"])
	}
	// El hover es más oscuro y el suave casi blanco.
	if app.Contraste(tk["claro"]["--color-acento-hover"], "#FFFFFF") <= app.Contraste("#7C2D12", "#FFFFFF") {
		t.Fatal("el hover debe oscurecer el acento")
	}
	if app.Contraste(tk["claro"]["--color-acento-suave"], "#FFFFFF") > 1.2 {
		t.Fatalf("el suave debe ser casi blanco: %s", tk["claro"]["--color-acento-suave"])
	}
	// Sin marca: los tokens de siempre.
	if (app.Marca{}).Tokens()["claro"]["--color-acento"] != "#155E75" {
		t.Fatal("sin marca debe quedar el petróleo de EDISYS")
	}
}

func TestPlantillaPorDefectoYValidacion(t *testing.T) {
	p := app.LeerPlantilla([]byte(`{"bloques":{"qr":true}}`))
	if p.Titulo != "Recibo de mantenimiento" || !p.MostrarLogo || !p.Bloques.QR || p.Bloques.Barras {
		t.Fatalf("lo que falta debe tomar el valor por defecto: %+v", p)
	}
	if ev := app.ValidarPlantilla(app.PlantillaRecibo{Titulo: "Recibo", Color: "#EEEEEE"}); ev == nil || ev.Campos["color"] == "" {
		t.Fatal("un color claro en la cabecera debió rechazarse")
	}
	if ev := app.ValidarPlantilla(app.PlantillaRecibo{Titulo: strings.Repeat("x", 61)}); ev == nil || ev.Campos["titulo"] == "" {
		t.Fatal("título de 61 caracteres aceptado")
	}
	if ev := app.ValidarPlantilla(app.PlantillaRecibo{Titulo: "Recibo"}); ev != nil {
		t.Fatalf("color vacío (= el de la marca) debe valer: %v", ev.Campos)
	}
}

func TestDiferenciasCambio(t *testing.T) {
	d := app.DiferenciasCambio(map[string]any{"activo": true, "motivo": "", "igual": 1}, map[string]any{"activo": false, "motivo": "Fin de contrato", "igual": 1.0})
	if len(d) != 2 || d[0].Campo != "activo" || d[1].Campo != "motivo" {
		t.Fatalf("diferencias: %+v", d)
	}
	if len(app.DiferenciasCambio(nil, map[string]any{"a": 1})) != 1 {
		t.Fatal("de nada a algo es un cambio")
	}
	if len(app.DiferenciasCambio("x", "x")) != 0 {
		t.Fatal("valores iguales no cambian")
	}
}

// ---------- I2 · marca blanca ----------

func TestMarcaBlanca(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	base := "/api/v1/edificios/1"

	st, d := e.pedir("GET", base+"/marca", tok, nil)
	if st != 200 || d["nombre"] != "EDISYS" || d["personalizada"] != false {
		t.Fatalf("marca inicial: %d %v", st, d)
	}
	// Invariante: el primario lleva texto blanco encima.
	st, d = e.pedir("PUT", base+"/marca", tok, map[string]any{"nombre": "Altamira", "color_primario": "#FDE68A", "color_fondo_login": "#1C1917"})
	if st != 422 {
		t.Fatalf("color sin contraste: %d %v", st, d)
	}
	st, d = e.pedir("PUT", base+"/marca", tok, map[string]any{"nombre": "Altamira Administraciones", "lema": "Edificios en orden", "slug": "altamira",
		"color_primario": "#7c2d12", "color_fondo_login": "#1C1917"})
	if st != 200 {
		t.Fatalf("guardar marca: %d %v", st, d)
	}
	if tk := d["tokens"].(map[string]any)["claro"].(map[string]any); tk["--color-acento"] != "#7C2D12" {
		t.Fatalf("tokens: %v", tk)
	}

	// /yo lleva la marca para la app.
	_, yo := e.pedir("GET", "/api/v1/yo", tok, nil)
	if m, _ := yo["marca"].(map[string]any); m == nil || m["nombre"] != "Altamira Administraciones" || m["slug"] != "altamira" {
		t.Fatalf("/yo sin la marca: %v", yo["marca"])
	}
	// El login la lee sin sesión por su slug; un slug inexistente es 404.
	if st, d := e.pedir("GET", "/api/v1/publico/marca/altamira", "", nil); st != 200 || d["lema"] != "Edificios en orden" {
		t.Fatalf("marca pública: %d %v", st, d)
	}
	if st, _ := e.pedir("GET", "/api/v1/publico/marca/no-existe", "", nil); st != 404 {
		t.Fatalf("slug inexistente: %d", st)
	}
	// El propietario no la configura.
	if st, _ := e.pedir("PUT", base+"/marca", e.login("propietario201@demo.pe"), map[string]any{"nombre": "X"}); st != 403 {
		t.Fatalf("propietario configurando la marca: %d", st)
	}

	// Logo: un PNG real; un archivo que no es imagen se rechaza.
	subirLogo := func(nombre string, datos []byte) (int, map[string]any) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("logo", nombre)
		_, _ = fw.Write(datos)
		mw.Close()
		req, _ := http.NewRequest("POST", e.srv.URL+base+"/marca/logo", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+tok)
		return e.hacer(req)
	}
	if st, _ := subirLogo("logo.pdf", []byte("%PDF-1.4\n%%EOF")); st != 422 {
		t.Fatalf("un PDF como logo: %d", st)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 300, 100))
	for x := 0; x < 300; x++ {
		for y := 0; y < 100; y++ {
			img.Set(x, y, color.NRGBA{124, 45, 18, 255})
		}
	}
	var pngB bytes.Buffer
	_ = png.Encode(&pngB, img)
	if st, d := subirLogo("logo.png", pngB.Bytes()); st != 201 || d["logo_url"] == nil {
		t.Fatalf("subir logo: %d %v", st, d)
	}

	// El recibo en PDF sale con la marca y el logo incrustado.
	rid := e.reciboConCorreo()
	st, tipo, pdfB := e.pedirCrudo("GET", fmt.Sprintf("%s/recibos/%d/pdf", base, rid), tok, nil)
	if st != 200 || tipo != "application/pdf" {
		t.Fatalf("pdf: %d %s", st, tipo)
	}
	if !bytes.Contains(pdfB, []byte("/Subtype /Image")) || !bytes.Contains(pdfB, []byte("Generado por Altamira Administraciones")) {
		t.Fatal("el recibo no lleva el logo ni el nombre de la marca")
	}
	// 7C2D12 = 124,45,18 → la cabecera del recibo.
	if !bytes.Contains(pdfB, []byte("0.486 0.176 0.071 rg")) {
		t.Fatal("la cabecera del recibo no usa el color de la marca")
	}

	// El correo (modo simulado: no sale nada) lleva el nombre y el color.
	if st, d := e.pedir("POST", base+"/recibos/enviar", tok, map[string]any{"recibo_ids": []int64{rid}}); st != 202 {
		t.Fatalf("enviar recibo: %d %v", st, d)
	}
	var html string
	if err := e.pool.QueryRow(context.Background(), `SELECT html FROM correo_mensaje WHERE origen='recibo' ORDER BY id DESC LIMIT 1`).Scan(&html); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Altamira Administraciones") || !strings.Contains(html, "background:#7C2D12") {
		t.Fatalf("el correo no lleva la marca: %s", html[:min(400, len(html))])
	}

	// Registro de cambios: el guardado de la marca quedó con antes y después.
	_, c := e.pedir("GET", base+"/configuracion/cambios?modulo=marca", tok, nil)
	if num(c["total"]) < 2 {
		t.Fatalf("registro de la marca: %v", c)
	}
}

// ---------- I1 · plantilla de recibo ----------

func TestPlantillaRecibo(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	base := "/api/v1/edificios/1"

	st, d := e.pedir("GET", base+"/plantilla-recibo", tok, nil)
	if st != 200 || d["guardada"] != false || d["config"].(map[string]any)["titulo"] != "Recibo de mantenimiento" {
		t.Fatalf("plantilla por defecto: %d %v", st, d)
	}
	if st, _ := e.pedir("PUT", base+"/plantilla-recibo", tok, map[string]any{"config": map[string]any{"titulo": "Recibo", "color": "#EEEEEE"}}); st != 422 {
		t.Fatalf("color claro aceptado: %d", st)
	}
	conf := map[string]any{"titulo": "Recibo Torre Sur", "color": "#1e3a8a", "nota": "Horario de caja: lunes a viernes de 9 a 6.",
		"mostrar_logo": true, "bloques": map[string]any{"contometro": true, "fotos": true, "qr": true, "barras": true}}
	if st, d := e.pedir("PUT", base+"/plantilla-recibo", tok, map[string]any{"config": conf}); st != 200 {
		t.Fatalf("guardar plantilla: %d %v", st, d)
	}

	rid := e.reciboConCorreo()
	var numero string
	_ = e.pool.QueryRow(context.Background(), `SELECT COALESCE(numero,'') FROM recibo WHERE id=$1`, rid).Scan(&numero)
	_, _, pdfB := e.pedirCrudo("GET", fmt.Sprintf("%s/recibos/%d/pdf", base, rid), tok, nil)
	// 1E3A8A = 30,58,138; el PDF va en WinAnsi (ó = 0xF3).
	for _, quiere := range []string{"Recibo Torre Sur", "0.118 0.227 0.541 rg", "Cont\xf3metro de agua", "Foto del medidor", "Escanea para ver y pagar", "Horario de caja"} {
		if !bytes.Contains(pdfB, []byte(quiere)) {
			t.Errorf("el recibo no lleva %q", quiere)
		}
	}
	if numero != "" && !bytes.Contains(pdfB, []byte("("+numero+")")) {
		t.Errorf("el código de barras no rotula el número %s", numero)
	}
	if !bytes.Contains(pdfB, []byte("/Subtype /Image")) {
		t.Error("la foto del medidor no se incrustó")
	}

	// Vista previa con una plantilla sin guardar: sin bloques no hay foto.
	st, tipo, prev := e.pedirCrudo("POST", base+"/plantilla-recibo/vista-previa", tok, map[string]any{"config": map[string]any{"titulo": "Prueba", "mostrar_logo": false}})
	if st != 200 || tipo != "application/pdf" || !bytes.Contains(prev, []byte("Prueba")) || bytes.Contains(prev, []byte("/Subtype /Image")) {
		t.Fatalf("vista previa: %d %s", st, tipo)
	}
	// La vista previa no guarda nada.
	_, d = e.pedir("GET", base+"/plantilla-recibo", tok, nil)
	if d["config"].(map[string]any)["titulo"] != "Recibo Torre Sur" {
		t.Fatalf("la vista previa cambió la plantilla guardada: %v", d["config"])
	}
	// Solo la administración la toca.
	if st, _ := e.pedir("GET", base+"/plantilla-recibo", e.login("propietario201@demo.pe"), nil); st != 403 {
		t.Fatalf("propietario viendo la plantilla: %d", st)
	}
	// El propietario sigue bajando su recibo con la plantilla del edificio.
	if st, tipo, _ := e.pedirCrudo("GET", base+"/recibos", e.login("propietario201@demo.pe"), nil); st != 200 || !strings.HasPrefix(tipo, "application/json") {
		t.Fatalf("recibos del propietario: %d", st)
	}
}

// ---------- I5 · configuración ----------

func TestConfiguracionEdificio(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	base := "/api/v1/edificios/1"

	st, d := e.pedir("GET", base+"/configuracion/asistentes", tok, nil)
	if st != 200 || num(d["total"]) != 9 {
		t.Fatalf("asistente: %d %v", st, d)
	}
	pasos := map[string]bool{}
	for _, p := range d["pasos"].([]any) {
		m := p.(map[string]any)
		pasos[m["clave"].(string)] = m["hecho"].(bool)
	}
	// La semilla trae unidades y periodos, pero ni marca ni plantilla.
	if !pasos["unidades"] || !pasos["periodo"] || pasos["marca"] || pasos["plantilla"] {
		t.Fatalf("pasos: %v", pasos)
	}

	// Desactivar exige motivo.
	if st, _ := e.pedir("PUT", base+"/configuracion/activo", tok, map[string]any{"activo": false}); st != 422 {
		t.Fatalf("desactivar sin motivo: %d", st)
	}
	if st, d := e.pedir("PUT", base+"/configuracion/activo", tok, map[string]any{"activo": false, "motivo": "Fin de contrato"}); st != 200 || d["activo"] != false {
		t.Fatalf("desactivar: %d %v", st, d)
	}
	// Desactivado: se consulta, pero no se registra nada.
	if st, _ := e.pedir("GET", base+"/recibos", tok, nil); st != 200 {
		t.Fatalf("leer con el edificio desactivado: %d", st)
	}
	st, d = e.pedir("POST", base+"/documentos/categorias", tok, map[string]any{"nombre": "Actas"})
	if st != 409 || codigo(d) != "EDIFICIO_INACTIVO" {
		t.Fatalf("escribir con el edificio desactivado: %d %v", st, d)
	}
	// También por la ruta sin edificio (módulos nuevos).
	if st, d := e.pedir("POST", "/api/v1/documentos/categorias?edificio_id=1", tok, map[string]any{"nombre": "Actas"}); st != 409 {
		t.Fatalf("escribir por la ruta corta: %d %v", st, d)
	}
	// La junta ve el estado pero no lo cambia.
	junta := e.login("junta@demo.pe")
	if st, _ := e.pedir("GET", base+"/configuracion/estado", junta, nil); st != 200 {
		t.Fatalf("junta ve el estado: %d", st)
	}
	if st, _ := e.pedir("PUT", base+"/configuracion/activo", junta, map[string]any{"activo": true}); st != 403 {
		t.Fatalf("junta reactivando: %d", st)
	}
	// Reactivar.
	if st, d := e.pedir("PUT", base+"/configuracion/activo", tok, map[string]any{"activo": true}); st != 200 || d["activo"] != true {
		t.Fatalf("reactivar: %d %v", st, d)
	}
	if st, _ := e.pedir("PUT", base+"/configuracion/activo", tok, map[string]any{"activo": true}); st != 409 {
		t.Fatalf("reactivar dos veces: %d", st)
	}
	if st, _ := e.pedir("POST", base+"/documentos/categorias", tok, map[string]any{"nombre": "Actas"}); st != 201 {
		t.Fatalf("escribir tras reactivar: %d", st)
	}

	// Registro de cambios: la desactivación con el campo que cambió.
	st, d = e.pedir("GET", base+"/configuracion/cambios?modulo=configuracion", tok, nil)
	if st != 200 || num(d["total"]) != 2 {
		t.Fatalf("registro: %d %v", st, d)
	}
	filas := d["datos"].([]any)
	ultima := filas[len(filas)-1].(map[string]any) // la más antigua: desactivar
	if ultima["accion"] != "desactivar" || ultima["usuario"] != "Ana Administradora" {
		t.Fatalf("fila: %v", ultima)
	}
	encontrado := false
	for _, c := range ultima["cambios"].([]any) {
		m := c.(map[string]any)
		if m["campo"] == "activo" && m["antes"] == true && m["despues"] == false {
			encontrado = true
		}
	}
	if !encontrado {
		t.Fatalf("el cambio de activo no aparece: %v", ultima["cambios"])
	}
}
