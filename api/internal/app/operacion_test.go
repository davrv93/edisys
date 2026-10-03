package app_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"edisys/api/internal/app"
)

// Bloque G · Operación: ocurrencias, tickets con SLA, visitas QR, parking y paquetes.

// ---------- reglas puras ----------

func TestSemaforoSLA(t *testing.T) {
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	vence := base.Add(72 * time.Hour)
	casos := []struct {
		ahora    time.Time
		cerrado  *time.Time
		color    string
		cumplido bool
	}{
		{base.Add(time.Hour), nil, app.SemaforoVerde, true},
		{base.Add(55 * time.Hour), nil, app.SemaforoAmbar, true}, // quedan 17 h < 18 h (25 %)
		{base.Add(53 * time.Hour), nil, app.SemaforoVerde, true}, // quedan 19 h
		{base.Add(73 * time.Hour), nil, app.SemaforoRojo, false},
		{base.Add(80 * time.Hour), ptrT(base.Add(70 * time.Hour)), app.SemaforoCerrado, true},
		{base.Add(80 * time.Hour), ptrT(base.Add(75 * time.Hour)), app.SemaforoCerrado, false},
	}
	for i, c := range casos {
		color, ok := app.SemaforoSLA(72, vence, c.ahora, c.cerrado)
		if color != c.color || ok != c.cumplido {
			t.Errorf("caso %d: quiero %s/%v, obtuve %s/%v", i, c.color, c.cumplido, color, ok)
		}
	}
}

func ptrT(t time.Time) *time.Time { return &t }

func TestMontoParking(t *testing.T) {
	casos := []struct {
		min, frac, tol int
		tarifa         int64
		quiero         int64
	}{
		{10, 60, 15, 500, 0},    // dentro de la tolerancia
		{16, 60, 15, 500, 500},  // pasó la tolerancia: una hora
		{61, 60, 0, 500, 1000},  // fracción empezada se cobra
		{135, 30, 0, 500, 1250}, // 5 fracciones de media hora
		{20, 15, 0, 333, 167},   // 2×15 min a S/3.33 la hora = 166.5 → 167
		{120, 60, 0, 0, 0},      // sin tarifa no se cobra
		{0, 60, 0, 500, 0},      // sin minutos
	}
	for _, c := range casos {
		if got := app.MontoParking(c.min, c.tarifa, c.frac, c.tol); got != c.quiero {
			t.Errorf("MontoParking(%d min, %d, %d, %d) = %d; quiero %d", c.min, c.tarifa, c.frac, c.tol, got, c.quiero)
		}
	}
}

func TestMotivoRechazoVisita(t *testing.T) {
	ahora := time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC)
	d, h := ahora.Add(-time.Hour), ahora.Add(time.Hour)
	casos := []struct {
		estado     string
		desde, has time.Time
		usos, max  int
		quiero     string
	}{
		{"autorizada", d, h, 0, 1, ""},
		{"autorizada", ahora.Add(time.Minute), h, 0, 1, app.RechazoAunNo},
		{"autorizada", d, ahora, 0, 1, app.RechazoVencida},
		{"autorizada", d, h, 1, 1, app.RechazoSinUsos},
		{"en_curso", d, h, 1, 3, app.RechazoYaDentro},
		{"anulada", d, h, 0, 1, app.RechazoAnulada},
		{"finalizada", d, h, 1, 1, app.RechazoFinalizada},
	}
	for i, c := range casos {
		if got := app.MotivoRechazoVisita(c.estado, c.desde, c.has, ahora, c.usos, c.max); got != c.quiero {
			t.Errorf("caso %d: quiero %q, obtuve %q", i, c.quiero, got)
		}
	}
	if app.NormalizarCodigoVisita(" edisys:v:abcd-efgh ") != "ABCDEFGH" {
		t.Error("el código del QR debe normalizarse")
	}
}

// ---------- G1 · cuaderno de ocurrencias ----------

func TestOcurrencias(t *testing.T) {
	e := nuevo(t)
	op := e.login("operario@demo.pe")
	ctx := context.Background()

	st, d := e.multipartPedir("POST", "/api/v1/edificios/1/ocurrencias", op, map[string]string{
		"titulo": "Puerta del sótano abierta", "descripcion": "Se encontró abierta a las 2 a. m.", "prioridad": "alta", "empleado": "Óscar (nocturno)"}, "foto", "puerta.png", pngChico())
	if st != 201 {
		t.Fatalf("anotar: %d %v", st, d)
	}
	oc1 := num(d["id"])
	if d["codigo"] != "OC-001" {
		t.Errorf("correlativo: %v", d["codigo"])
	}
	if st, d := e.pedir("POST", "/api/v1/edificios/1/ocurrencias", op, map[string]any{"titulo": "Ruido en el 4to piso", "prioridad": "urgente"}); st != 422 {
		t.Errorf("prioridad inválida debería ser 422: %d %v", st, d)
	}
	st, d = e.pedir("POST", "/api/v1/edificios/1/ocurrencias", op, map[string]any{"titulo": "Foco quemado en el hall"})
	if st != 201 {
		t.Fatalf("anotar sin foto: %d %v", st, d)
	}
	oc2 := num(d["id"])

	// Cerrar exige nota; cerrada no se reabre ni se escala.
	if st, _ := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/ocurrencias/%d/cerrar", oc1), op, map[string]any{"nota": ""}); st != 422 {
		t.Errorf("cerrar sin nota: %d", st)
	}
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/ocurrencias/%d/cerrar", oc1), op, map[string]any{"nota": "Se cerró con llave"}); st != 200 {
		t.Fatalf("cerrar: %d %v", st, d)
	}
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/ocurrencias/%d/cerrar", oc1), op, map[string]any{"nota": "otra vez"}); st != 409 || codigo(d) != "OCURRENCIA_CERRADA" {
		t.Errorf("recerrar: %d %v", st, d)
	}
	if st, _ := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/ocurrencias/%d/escalar", oc1), op, map[string]any{}); st != 409 {
		t.Errorf("escalar cerrada: %d", st)
	}

	// La bitácora no se reescribe: regla dura en la base.
	if _, err := e.pool.Exec(ctx, `UPDATE ocurrencia SET titulo='otra cosa' WHERE id=$1`, oc2); err == nil {
		t.Error("la base debería impedir reescribir una ocurrencia")
	}

	// Escalar crea la incidencia en el tablero (reportado) y deja el vínculo.
	st, d = e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/ocurrencias/%d/escalar", oc2), op, map[string]any{"categoria": "electricidad", "ubicacion": "Hall"})
	if st != 201 {
		t.Fatalf("escalar: %d %v", st, d)
	}
	var estado, cat string
	if err := e.pool.QueryRow(ctx, `SELECT estado, categoria FROM incidencia WHERE id=$1`, num(d["incidencia_id"])).Scan(&estado, &cat); err != nil || estado != "reportado" || cat != "electricidad" {
		t.Errorf("incidencia escalada: %v %s %s", err, estado, cat)
	}

	_, l := e.pedir("GET", "/api/v1/edificios/1/ocurrencias", op, nil)
	cont := l["conteos"].(map[string]any)
	if num(cont["cerrada"]) != 1 || num(cont["escalada"]) != 1 {
		t.Errorf("conteos: %v", cont)
	}
	// El propietario no ve el cuaderno.
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/ocurrencias", e.login("propietario201@demo.pe"), nil); st != 403 {
		t.Errorf("propietario no debería ver ocurrencias: %d", st)
	}
}

// ---------- G2 · tickets con SLA ----------

func TestTicketsSLA(t *testing.T) {
	e := nuevo(t)
	ctx := context.Background()
	admin := e.login("admin@demo.pe")
	prop := e.login("propietario201@demo.pe")

	// Plazo de fábrica sin clasificar: 72 h; el edificio lo baja a 48 h.
	if st, d := e.pedir("PUT", "/api/v1/edificios/1/tickets/sla", admin, map[string]any{"horas_critica": 4, "horas_media": 24, "horas_baja": 120,
		"horas_sin_clasificar": 48, "respuesta_automatica": true, "mensaje": "Hola {{nombre}}, tu ticket {{codigo}} vence en {{plazo}}."}); st != 200 {
		t.Fatalf("config SLA: %d %v", st, d)
	}
	if st, _ := e.pedir("PUT", "/api/v1/edificios/1/tickets/sla", prop, map[string]any{"horas_critica": 1}); st != 403 {
		t.Errorf("el propietario no configura el SLA: %d", st)
	}

	st, d := e.multipartPedir("POST", "/api/v1/edificios/1/incidencias", prop, map[string]string{"descripcion": "Gotea el caño del baño", "ubicacion": "Dpto 201"}, "fotos", "cano.png", pngChico())
	if st != 201 {
		t.Fatalf("reportar: %d %v", st, d)
	}
	inc := num(d["id"])
	var obj int
	var vence, creado time.Time
	var auto *time.Time
	if err := e.pool.QueryRow(ctx, `SELECT sla_objetivo, sla_vencimiento, creado_en, respuesta_auto_en FROM incidencia WHERE id=$1`, inc).Scan(&obj, &vence, &creado, &auto); err != nil {
		t.Fatal(err)
	}
	if obj != 48 || !vence.Equal(creado.Add(48*time.Hour)) {
		t.Errorf("SLA sin clasificar: %d h, vence %v", obj, vence)
	}
	// Respuesta automática al solicitante, en la bandeja (simulado: no sale nada a terceros).
	if auto == nil {
		t.Error("debería registrarse la respuesta automática")
	}
	var texto string
	_ = e.pool.QueryRow(ctx, `SELECT texto FROM whatsapp_mensaje WHERE origen='sistema' AND texto LIKE '%'||$1||'%' ORDER BY id DESC LIMIT 1`, d["codigo"]).Scan(&texto)
	if texto != fmt.Sprintf("Hola María, tu ticket %s vence en 2 días.", d["codigo"]) {
		t.Errorf("texto de la respuesta automática: %q", texto)
	}

	// Al clasificar como crítica, el vencimiento se recalcula desde el reporte (4 h).
	if st, d := e.pedir("PATCH", fmt.Sprintf("/api/v1/edificios/1/incidencias/%d/estado", inc), admin, map[string]any{"estado": "validado", "criticidad": "critica"}); st != 200 {
		t.Fatalf("validar: %d %v", st, d)
	}
	_ = e.pool.QueryRow(ctx, `SELECT sla_objetivo, sla_vencimiento FROM incidencia WHERE id=$1`, inc).Scan(&obj, &vence)
	if obj != 4 || !vence.Equal(creado.Add(4*time.Hour)) {
		t.Errorf("SLA crítico: %d h, vence %v", obj, vence)
	}

	// Lo vencido sale en rojo (se envejece el reporte en la base).
	if _, err := e.pool.Exec(ctx, `UPDATE incidencia SET sla_vencimiento=now()-interval '1 hour' WHERE id=$1`, inc); err != nil {
		t.Fatal(err)
	}
	_, l := e.pedir("GET", "/api/v1/edificios/1/tickets?semaforo=rojo", admin, nil)
	hallado := false
	for _, it := range l["datos"].([]any) {
		f := it.(map[string]any)
		if num(f["id"]) == inc {
			hallado = f["semaforo"] == "rojo" && f["sla_cumplido"] == false
		}
	}
	if !hallado {
		t.Errorf("el ticket vencido debería salir en rojo: %v", l["conteos"])
	}
	// El kanban también trae el semáforo.
	_, k := e.pedir("GET", "/api/v1/edificios/1/trabajos", admin, nil)
	for _, it := range k["datos"].([]any) {
		f := it.(map[string]any)
		if num(f["id"]) == inc && f["semaforo"] != "rojo" {
			t.Errorf("semáforo en el tablero: %v", f["semaforo"])
		}
	}
}

// ---------- G3 · visitas QR ----------

func TestVisitasQR(t *testing.T) {
	e := nuevo(t)
	ctx := context.Background()
	prop := e.login("propietario201@demo.pe")
	op := e.login("operario@demo.pe")

	// El residente solo autoriza visitas a su unidad.
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/visitas", prop, map[string]any{"unidad_id": e.idUnidad("402"), "visitante": "Intruso"}); st != 422 {
		t.Errorf("unidad ajena: %d", st)
	}
	st, d := e.pedir("POST", "/api/v1/edificios/1/visitas", prop, map[string]any{"visitante": "Carlos Pérez", "documento": "45678912", "usos_max": 2})
	if st != 201 {
		t.Fatalf("autorizar: %d %v", st, d)
	}
	vid := num(d["id"])
	contenido := d["contenido_qr"].(string)

	// El QR se genera en el API: matriz cuadrada.
	st, q := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/visitas/%d/qr", vid), prop, nil)
	if st != 200 || num(q["lado"]) < 21 || len(q["matriz"].([]any)) != int(num(q["lado"])) {
		t.Fatalf("qr: %d %v", st, q["lado"])
	}
	// Otro residente no ve el QR ajeno.
	if st, _ := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/visitas/%d/qr", vid), e.login("inquilino@demo.pe"), nil); st != 404 {
		t.Errorf("QR ajeno: %d", st)
	}
	// El residente no valida en portería.
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/visitas/validar", prop, map[string]any{"codigo": contenido}); st != 403 {
		t.Errorf("residente validando: %d", st)
	}

	validar := func(cod string) map[string]any {
		st, r := e.pedir("POST", "/api/v1/edificios/1/visitas/validar", op, map[string]any{"codigo": cod})
		if st != 200 {
			t.Fatalf("validar: %d %v", st, r)
		}
		return r
	}
	if r := validar("NOEXISTE123"); r["valido"] != false || r["motivo"] != app.RechazoNoExiste {
		t.Errorf("código inventado: %v", r)
	}
	if r := validar(contenido); r["valido"] != true {
		t.Fatalf("primera entrada: %v", r)
	}
	if r := validar(contenido); r["motivo"] != app.RechazoYaDentro {
		t.Errorf("doble entrada: %v", r)
	}
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/visitas/%d/salida", vid), op, nil); st != 200 || d["estado"] != "autorizada" {
		t.Errorf("salida con usos pendientes: %d %v", st, d)
	}
	if r := validar(contenido); r["valido"] != true {
		t.Errorf("segunda entrada: %v", r)
	}
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/visitas/%d/salida", vid), op, nil); st != 200 || d["estado"] != "finalizada" {
		t.Errorf("última salida: %d %v", st, d)
	}
	if r := validar(contenido); r["motivo"] != app.RechazoFinalizada {
		t.Errorf("tras finalizar: %v", r)
	}

	// Vencida: se rechaza aunque tenga usos.
	st, d = e.pedir("POST", "/api/v1/edificios/1/visitas", op, map[string]any{"unidad_id": e.idUnidad("201"), "visitante": "Delivery"})
	if st != 201 {
		t.Fatalf("visita de portería: %d %v", st, d)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE visita SET valido_desde=now()-interval '2 day', valido_hasta=now()-interval '1 day' WHERE id=$1`, num(d["id"])); err != nil {
		t.Fatal(err)
	}
	if r := validar(d["codigo_qr"].(string)); r["motivo"] != app.RechazoVencida {
		t.Errorf("vencida: %v", r)
	}

	// Anulada por el residente.
	_, d = e.pedir("POST", "/api/v1/edificios/1/visitas", prop, map[string]any{"visitante": "Técnico cable"})
	if st, _ := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/visitas/%d/anular", num(d["id"])), prop, nil); st != 200 {
		t.Errorf("anular: %d", st)
	}
	if r := validar(d["codigo_qr"].(string)); r["motivo"] != app.RechazoAnulada {
		t.Errorf("anulada: %v", r)
	}

	// Todo quedó en la bitácora: 4 entradas permitidas+rechazadas de la primera visita, etc.
	var permitidos, rechazados int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE resultado='permitido' AND tipo='entrada'), count(*) FILTER (WHERE resultado='rechazado') FROM acceso`).Scan(&permitidos, &rechazados)
	if permitidos != 2 || rechazados != 5 {
		t.Errorf("bitácora: %d permitidos, %d rechazados", permitidos, rechazados)
	}
	// Walk-in: la portería registra y deja entrar en el acto.
	st, d = e.pedir("POST", "/api/v1/edificios/1/visitas", op, map[string]any{"unidad_id": e.idUnidad("301"), "visitante": "Gasfitero", "ingresar_ahora": true})
	if st != 201 || d["estado"] != "en_curso" {
		t.Errorf("walk-in: %d %v", st, d)
	}
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/visitas", prop, map[string]any{"visitante": "X", "ingresar_ahora": true}); st != 422 {
		t.Errorf("el residente no registra ingresos: %d", st)
	}
}

// ---------- G4 · parking ----------

func TestParking(t *testing.T) {
	e := nuevo(t)
	ctx := context.Background()
	admin := e.login("admin@demo.pe")
	op := e.login("operario@demo.pe")

	if st, _ := e.pedir("POST", "/api/v1/edificios/1/parking/estacionamientos", op, map[string]any{"codigo": "V-01"}); st != 403 {
		t.Errorf("el operario no configura espacios: %d", st)
	}
	st, d := e.pedir("POST", "/api/v1/edificios/1/parking/estacionamientos", admin, map[string]any{"codigo": "v-01", "tipo": "visitas", "tarifa_hora_cts": 400, "fraccion_min": 30, "tolerancia_min": 10})
	if st != 201 {
		t.Fatalf("crear espacio: %d %v", st, d)
	}
	esp := num(d["id"])
	if st, d := e.pedir("POST", "/api/v1/edificios/1/parking/estacionamientos", admin, map[string]any{"codigo": "V-01"}); st != 409 || codigo(d) != "ESTACIONAMIENTO_EXISTE" {
		t.Errorf("código repetido: %d %v", st, d)
	}
	_, d = e.pedir("POST", "/api/v1/edificios/1/parking/estacionamientos", admin, map[string]any{"codigo": "V-02", "tarifa_hora_cts": 400})
	esp2 := num(d["id"])

	st, d = e.pedir("POST", "/api/v1/edificios/1/parking/sesiones", op, map[string]any{"estacionamiento_id": esp, "placa": "abc-123", "unidad_id": e.idUnidad("201")})
	if st != 201 {
		t.Fatalf("entrada: %d %v", st, d)
	}
	s1 := num(d["id"])
	// Un espacio, un vehículo; una placa no entra dos veces.
	if st, d := e.pedir("POST", "/api/v1/edificios/1/parking/sesiones", op, map[string]any{"estacionamiento_id": esp, "placa": "XYZ999"}); st != 409 || codigo(d) != "ESPACIO_OCUPADO" {
		t.Errorf("espacio ocupado: %d %v", st, d)
	}
	if st, d := e.pedir("POST", "/api/v1/edificios/1/parking/sesiones", op, map[string]any{"estacionamiento_id": esp2, "placa": "ABC 123"}); st != 409 || codigo(d) != "PLACA_DENTRO" {
		t.Errorf("placa dentro: %d %v", st, d)
	}

	// 2 h 15 min a S/4.00 la hora por fracción de 30 min → 5 fracciones = S/10.00, al recibo.
	if _, err := e.pool.Exec(ctx, `UPDATE sesion_parking SET entrada_en=now()-interval '135 minutes' WHERE id=$1`, s1); err != nil {
		t.Fatal(err)
	}
	st, d = e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/parking/sesiones/%d/salida", s1), op, map[string]any{"cobro": "recibo"})
	if st != 200 || num(d["monto_cts"]) != 1000 || d["cobro"] != "recibo" {
		t.Fatalf("salida al recibo: %d %v", st, d)
	}
	var montoAjuste, unidad int64
	var pendiente bool
	if err := e.pool.QueryRow(ctx, `SELECT monto_cts, unidad_id, recibo_id IS NULL FROM ajuste WHERE id=$1`, num(d["ajuste_id"])).Scan(&montoAjuste, &unidad, &pendiente); err != nil {
		t.Fatal(err)
	}
	if montoAjuste != 1000 || unidad != e.idUnidad("201") || !pendiente {
		t.Errorf("nota de cargo: %d unidad %d pendiente %v", montoAjuste, unidad, pendiente)
	}
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/parking/sesiones/%d/salida", s1), op, map[string]any{"cobro": "recibo"}); st != 409 || codigo(d) != "SESION_CERRADA" {
		t.Errorf("doble salida: %d %v", st, d)
	}

	// Sin unidad no se cobra al recibo; inmediato va a ingresos externos.
	_, d = e.pedir("POST", "/api/v1/edificios/1/parking/sesiones", op, map[string]any{"estacionamiento_id": esp, "placa": "TAX001"})
	s2 := num(d["id"])
	_, _ = e.pool.Exec(ctx, `UPDATE sesion_parking SET entrada_en=now()-interval '50 minutes' WHERE id=$1`, s2)
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/parking/sesiones/%d/salida", s2), op, map[string]any{"cobro": "recibo"}); st != 422 || codigo(d) != "UNIDAD_OBLIGATORIA" {
		t.Errorf("recibo sin unidad: %d %v", st, d)
	}
	st, d = e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/parking/sesiones/%d/salida", s2), op, map[string]any{"cobro": "inmediato", "medio": "yape"})
	if st != 200 || num(d["monto_cts"]) != 400 {
		t.Fatalf("salida inmediata: %d %v", st, d)
	}
	var montoIng int64
	var medio string
	_ = e.pool.QueryRow(ctx, `SELECT monto_cts, medio FROM ingreso_externo WHERE id=$1`, num(d["ingreso_id"])).Scan(&montoIng, &medio)
	if montoIng != 400 || medio != "yape" {
		t.Errorf("ingreso externo: %d %s", montoIng, medio)
	}

	// Dentro de la tolerancia no hay cobro.
	_, d = e.pedir("POST", "/api/v1/edificios/1/parking/sesiones", op, map[string]any{"estacionamiento_id": esp, "placa": "RAP001"})
	st, d = e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/parking/sesiones/%d/salida", num(d["id"])), op, map[string]any{"cobro": "inmediato"})
	if st != 200 || d["cobro"] != "sin_cobro" || num(d["monto_cts"]) != 0 {
		t.Errorf("tolerancia: %d %v", st, d)
	}
}

// ---------- G5 · paquetes ----------

func TestPaquetes(t *testing.T) {
	e := nuevo(t)
	ctx := context.Background()
	op := e.login("operario@demo.pe")
	prop := e.login("propietario201@demo.pe")

	st, d := e.multipartPedir("POST", "/api/v1/edificios/1/paquetes", op, map[string]string{"unidad_id": fmt.Sprint(e.idUnidad("201")), "remitente": "Saga", "descripcion": "Caja mediana"}, "foto", "caja.png", pngChico())
	if st != 201 || d["avisado"] != true {
		t.Fatalf("recibir: %d %v", st, d)
	}
	pid := num(d["id"])
	// El aviso quedó en la bandeja para la unidad (simulado en pruebas).
	var texto, tel string
	if err := e.pool.QueryRow(ctx, `SELECT texto, telefono FROM whatsapp_mensaje WHERE id=$1`, num(d["aviso_mensaje_id"])).Scan(&texto, &tel); err != nil {
		t.Fatal(err)
	}
	if texto != "Hola María, llegó un paquete de Saga para el Dpto 201 (Caja mediana). Puedes recogerlo en portería." {
		t.Errorf("aviso: %q a %s", texto, tel)
	}

	// La propietaria ve su paquete; otra unidad no.
	_, l := e.pedir("GET", "/api/v1/edificios/1/paquetes", prop, nil)
	if num(l["pendientes"]) != 1 {
		t.Errorf("propietaria: %v", l["pendientes"])
	}
	_, l = e.pedir("GET", "/api/v1/edificios/1/paquetes", e.login("inquilino@demo.pe"), nil)
	if len(l["datos"].([]any)) != 0 {
		t.Error("el inquilino del 302 no debe ver el paquete del 201")
	}
	if st, _ := e.multipartPedir("POST", fmt.Sprintf("/api/v1/edificios/1/paquetes/%d/entregar", pid), prop, map[string]string{"entregado_a": "María"}, "firma", "f.png", pngChico()); st != 403 {
		t.Errorf("el residente no registra entregas: %d", st)
	}

	// Sin firma no hay entrega; con firma, una sola vez.
	if st, d := e.multipartPedir("POST", fmt.Sprintf("/api/v1/edificios/1/paquetes/%d/entregar", pid), op, map[string]string{"entregado_a": "María Demo"}, "", "", nil); st != 422 {
		t.Errorf("entrega sin firma: %d %v", st, d)
	}
	if st, d := e.multipartPedir("POST", fmt.Sprintf("/api/v1/edificios/1/paquetes/%d/entregar", pid), op, map[string]string{"entregado_a": "María Demo"}, "firma", "firma.png", pngChico()); st != 200 {
		t.Fatalf("entregar: %d %v", st, d)
	}
	if st, d := e.multipartPedir("POST", fmt.Sprintf("/api/v1/edificios/1/paquetes/%d/entregar", pid), op, map[string]string{"entregado_a": "Otro"}, "firma", "firma.png", pngChico()); st != 409 {
		t.Errorf("doble entrega: %d %v", st, d)
	}
	// La regla también vive en la base.
	if _, err := e.pool.Exec(ctx, `INSERT INTO paquete (edificio_id, unidad_id, descripcion, estado, entregado_en, entregado_a) VALUES (1,$1,'x','entregado',now(),'Juan')`, e.idUnidad("201")); err == nil {
		t.Error("la base debería exigir la firma en la entrega")
	}

	// Devolución con motivo.
	_, d = e.pedir("POST", "/api/v1/edificios/1/paquetes", op, map[string]any{"unidad_id": e.idUnidad("402"), "descripcion": "Sobre"})
	if st, _ := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/paquetes/%d/devolver", num(d["id"])), op, map[string]any{"motivo": ""}); st != 422 {
		t.Errorf("devolver sin motivo: %d", st)
	}
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/paquetes/%d/devolver", num(d["id"])), op, map[string]any{"motivo": "Dirección equivocada"}); st != 200 {
		t.Errorf("devolver: %d %v", st, d)
	}
}
