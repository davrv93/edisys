package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"edisys/api/internal/app"
	"edisys/api/internal/archivo"
	"edisys/api/internal/config"
	"edisys/api/internal/seed"
)

// Bloques E2–E5 · comunicación: anuncios, bandeja de correos, Telegram y contenido del portal.

// nuevoCon es nuevo() con la configuración ajustada (p. ej. Telegram contra un bot falso).
func nuevoCon(t *testing.T, ajustar func(*config.Config)) *entorno {
	t.Helper()
	pool := abrirBase(t)
	ctx := context.Background()
	alm := archivo.NuevaMemoria()
	if _, err := seed.Sembrar(ctx, pool, alm, seed.Opciones{}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Cargar()
	cfg.JWTSecret = "clave-de-pruebas-de-32-bytes-o-mas-123456"
	cfg.WhatsAppModo = "simulado"
	cfg.CorreoModo = "simulado"
	cfg.TelegramModo = "simulado"
	cfg.TelegramToken = ""
	cfg.Tareas = false
	ajustar(&cfg)
	s, err := app.Nuevo(ctx, pool, alm, cfg)
	if err != nil {
		t.Fatal(err)
	}
	e := &entorno{t: t, pool: pool, srv: httptest.NewServer(s.Rutas())}
	t.Cleanup(e.srv.Close)
	return e
}

// botTelegram imita la Bot API y guarda cada sendMessage. Nunca se llama a Telegram de verdad.
type botTelegram struct {
	mu     sync.Mutex
	falla  bool
	envios []map[string]any
}

const tokenBot = "999:token-de-prueba-que-no-debe-salir"

func (b *botTelegram) servidor(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot"+tokenBot+"/sendMessage" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Not Found"}`))
			return
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.falla {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`))
			return
		}
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		b.envios = append(b.envios, c)
		_, _ = fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d}}`, len(b.envios))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (b *botTelegram) cantidad() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.envios)
}

func (e *entorno) escalar(sql string, args ...any) int64 {
	e.t.Helper()
	var n int64
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func idsDe(m map[string]any) map[int64]bool {
	out := map[int64]bool{}
	ds, _ := m["datos"].([]any)
	for _, it := range ds {
		out[num(it.(map[string]any)["id"])] = true
	}
	return out
}

// E2 · publicar encola una vez por destinatario y canal; la evidencia refleja la bandeja.
func TestAnunciosCanalesYEvidencia(t *testing.T) {
	e := nuevo(t)
	adm := e.login("admin@demo.pe")
	prop := e.login("propietario201@demo.pe")
	const base = "/api/v1/edificios/1"

	if st, d := e.pedir("POST", base+"/telegram/destinos", adm, map[string]any{"nombre": "Grupo de propietarios", "chat_id": "-100200300"}); st != 201 {
		t.Fatalf("destino telegram: %d %v", st, d)
	}
	if st, d := e.pedir("POST", base+"/telegram/destinos", adm, map[string]any{"nombre": "Repetido", "chat_id": "-100200300"}); st != 409 {
		t.Fatalf("chat duplicado debe ser 409: %d %v", st, d)
	}
	// Canal inválido y campos vacíos.
	if st, d := e.pedir("POST", base+"/anuncios", adm, map[string]any{"titulo": "x", "cuerpo": "y", "canales": []string{"fax"}}); st != 422 {
		t.Fatalf("canal inválido: %d %v", st, d)
	}
	if st, _ := e.pedir("POST", base+"/anuncios", adm, map[string]any{"titulo": " ", "cuerpo": ""}); st != 422 {
		t.Fatalf("anuncio vacío: %d", st)
	}
	// El propietario no administra anuncios.
	if st, _ := e.pedir("POST", base+"/anuncios", prop, map[string]any{"titulo": "x", "cuerpo": "y"}); st != 403 {
		t.Fatalf("propietario creando anuncio: %d", st)
	}

	st, d := e.pedir("POST", base+"/anuncios", adm, map[string]any{"titulo": "Corte de agua", "cuerpo": "El sábado de 8 a 12 no habrá agua.",
		"canales": []string{"correo", "whatsapp", "telegram", "correo"}})
	if st != 201 {
		t.Fatalf("crear anuncio: %d %v", st, d)
	}
	aid := num(d["id"])
	if c := d["canales"].([]any); len(c) != 4 || c[0] != "portal" {
		t.Fatalf("canales normalizados (portal siempre, sin repetir): %v", c)
	}
	// Un borrador no se ve en el portal.
	if _, l := e.pedir("GET", base+"/anuncios", prop, nil); idsDe(l)[aid] {
		t.Fatal("el propietario ve un borrador")
	}
	if st, _ := e.pedir("PUT", fmt.Sprintf("%s/anuncios/%d", base, aid), adm, map[string]any{"titulo": "Corte de agua programado", "cuerpo": "El sábado de 8 a 12 no habrá agua.", "canales": []string{"correo", "whatsapp", "telegram"}}); st != 200 {
		t.Fatalf("editar borrador: %d", st)
	}

	st, pub := e.pedir("POST", fmt.Sprintf("%s/anuncios/%d/publicar", base, aid), adm, nil)
	if st != 200 {
		t.Fatalf("publicar: %d %v", st, pub)
	}
	env := pub["envios"].(map[string]any)
	nCorreo, nWA, nTG := num(env["correo"]), num(env["whatsapp"]), num(env["telegram"])
	if nCorreo < 1 || nWA < 1 || nTG != 1 {
		t.Fatalf("envíos por canal: %v", env)
	}
	// Uno por correo distinto: la evidencia no repite destinatario.
	distintos := e.escalar(`SELECT count(DISTINCT lower(pe.correo)) FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id JOIN unidad u ON u.id=up.unidad_id
		WHERE u.edificio_id=1 AND up.hasta IS NULL AND up.rol IN ('propietario','inquilino') AND pe.correo LIKE '%@%'`)
	if nCorreo != distintos {
		t.Fatalf("correos encolados %d, destinatarios distintos %d", nCorreo, distintos)
	}
	// Cada envío está enlazado a su mensaje en la bandeja, con la referencia del anuncio.
	if n := e.escalar(`SELECT count(*) FROM anuncio_envio WHERE anuncio_id=$1`, aid); n != nCorreo+nWA+nTG {
		t.Fatalf("filas de evidencia %d, envíos %d", n, nCorreo+nWA+nTG)
	}
	if n := e.escalar(`SELECT count(*) FROM correo_mensaje WHERE referencia=$1`, fmt.Sprintf("anuncio:%d", aid)); n != nCorreo {
		t.Fatalf("correos en bandeja %d, esperados %d", n, nCorreo)
	}
	if n := e.escalar(`SELECT count(*) FROM anuncio WHERE id=$1 AND enviado_en IS NOT NULL AND publicado_en IS NOT NULL`, aid); n != 1 {
		t.Fatal("el anuncio debe registrar publicado_en y enviado_en")
	}

	// Evidencia con el estado vivo: en simulado, todo «simulado».
	st, ev := e.pedir("GET", fmt.Sprintf("%s/anuncios/%d/envios", base, aid), adm, nil)
	if st != 200 || num(ev["total"]) != nCorreo+nWA+nTG {
		t.Fatalf("evidencia: %d %v", st, ev)
	}
	for _, it := range ev["datos"].([]any) {
		if est := it.(map[string]any)["estado"]; est != "simulado" {
			t.Fatalf("estado del envío: %v", it)
		}
	}
	if st, _ := e.pedir("GET", fmt.Sprintf("%s/anuncios/%d/envios", base, aid), prop, nil); st != 403 {
		t.Fatalf("propietario viendo la evidencia: %d", st)
	}

	// Publicar dos veces no reenvía; lo publicado no se edita ni se borra.
	if st, d := e.pedir("POST", fmt.Sprintf("%s/anuncios/%d/publicar", base, aid), adm, nil); st != 409 {
		t.Fatalf("segunda publicación: %d %v", st, d)
	}
	if n := e.escalar(`SELECT count(*) FROM anuncio_envio WHERE anuncio_id=$1`, aid); n != nCorreo+nWA+nTG {
		t.Fatal("la segunda publicación no debe añadir envíos")
	}
	if st, _ := e.pedir("PUT", fmt.Sprintf("%s/anuncios/%d", base, aid), adm, map[string]any{"titulo": "x", "cuerpo": "y"}); st != 409 {
		t.Fatalf("editar publicado: %d", st)
	}
	if st, _ := e.pedir("DELETE", fmt.Sprintf("%s/anuncios/%d", base, aid), adm, nil); st != 409 {
		t.Fatalf("borrar publicado: %d", st)
	}

	// Visible en el portal; archivado, deja de verse.
	if _, l := e.pedir("GET", base+"/anuncios", prop, nil); !idsDe(l)[aid] {
		t.Fatal("el propietario no ve el anuncio publicado")
	}
	if st, _ := e.pedir("POST", fmt.Sprintf("%s/anuncios/%d/archivar", base, aid), adm, nil); st != 200 {
		t.Fatalf("archivar: %d", st)
	}
	if _, l := e.pedir("GET", base+"/anuncios", prop, nil); idsDe(l)[aid] {
		t.Fatal("el propietario ve un anuncio archivado")
	}

	// Un anuncio solo de portal no envía nada.
	_, d = e.pedir("POST", base+"/anuncios", adm, map[string]any{"titulo": "Asamblea", "cuerpo": "Jueves 20:00"})
	solo := num(d["id"])
	_, pub = e.pedir("POST", fmt.Sprintf("%s/anuncios/%d/publicar", base, solo), adm, nil)
	if n := e.escalar(`SELECT count(*) FROM anuncio_envio WHERE anuncio_id=$1`, solo); n != 0 {
		t.Fatalf("solo portal no debe enviar: %d", n)
	}
	if n := e.escalar(`SELECT count(*) FROM anuncio WHERE id=$1 AND enviado_en IS NULL AND estado='publicado'`, solo); n != 1 {
		t.Fatal("solo portal: publicado y sin enviado_en")
	}
	// Un borrador sí se borra.
	_, d = e.pedir("POST", base+"/anuncios", adm, map[string]any{"titulo": "Borrador", "cuerpo": "x"})
	if st, _ := e.pedir("DELETE", fmt.Sprintf("%s/anuncios/%d", base, num(d["id"])), adm, nil); st != 200 {
		t.Fatalf("borrar borrador: %d", st)
	}
}

// E3 · bandeja de correos: detalle, filtros y reintento solo de lo que falló.
func TestBandejaCorreos(t *testing.T) {
	e := nuevo(t)
	adm := e.login("admin@demo.pe")
	prop := e.login("propietario201@demo.pe")
	const base = "/api/v1/edificios/1"
	ctx := context.Background()

	var fallido, enviado int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO correo_mensaje (edificio_id, para, asunto, texto, estado, intentos, error)
		VALUES (1,'vecino@demo.pe','Recibo de setiembre','hola','error',3,'smtp: conexión rechazada') RETURNING id`).Scan(&fallido); err != nil {
		t.Fatal(err)
	}
	_, _ = e.pool.Exec(ctx, `INSERT INTO correo_adjunto (mensaje_id, nombre, tipo_mime, datos) VALUES ($1,'recibo.pdf','application/pdf','%PDF-1.4')`, fallido)
	if err := e.pool.QueryRow(ctx, `INSERT INTO correo_mensaje (edificio_id, para, asunto, estado) VALUES (1,'otro@demo.pe','Balance','enviado') RETURNING id`).Scan(&enviado); err != nil {
		t.Fatal(err)
	}

	st, l := e.pedir("GET", base+"/correo/mensajes?estado=error", adm, nil)
	if st != 200 || num(l["total"]) != 1 || !idsDe(l)[fallido] || l["conteos"] == nil {
		t.Fatalf("filtro por estado (el total respeta el filtro): %d %v", st, l)
	}
	if _, l := e.pedir("GET", base+"/correo/mensajes?q=vecino", adm, nil); num(l["total"]) != 1 {
		t.Fatalf("búsqueda: %v", l)
	}
	st, det := e.pedir("GET", fmt.Sprintf("%s/correo/mensajes/%d", base, fallido), adm, nil)
	if st != 200 || det["error"] != "smtp: conexión rechazada" || len(det["adjuntos"].([]any)) != 1 {
		t.Fatalf("detalle: %d %v", st, det)
	}
	aid := num(det["adjuntos"].([]any)[0].(map[string]any)["id"])
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s%s/correo/mensajes/%d/adjuntos/%d", e.srv.URL, base, fallido, aid), nil)
	req.Header.Set("Authorization", "Bearer "+adm)
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("descargar adjunto: %v %v", err, res)
	}

	if st, _ := e.pedir("POST", fmt.Sprintf("%s/correo/mensajes/%d/reintentar", base, fallido), prop, nil); st != 403 {
		t.Fatalf("propietario reintentando: %d", st)
	}
	st, r := e.pedir("POST", fmt.Sprintf("%s/correo/mensajes/%d/reintentar", base, fallido), adm, nil)
	if st != 200 || r["estado"] != "simulado" {
		t.Fatalf("reintento: %d %v", st, r)
	}
	if n := e.escalar(`SELECT count(*) FROM correo_mensaje WHERE id=$1 AND error='' AND intentos=1`, fallido); n != 1 {
		t.Fatal("el reintento debe limpiar el error y contar un intento nuevo")
	}
	// Lo ya enviado (o simulado) no se reintenta: nadie lo recibe dos veces.
	if st, d := e.pedir("POST", fmt.Sprintf("%s/correo/mensajes/%d/reintentar", base, enviado), adm, nil); st != 409 || codigo(d) != "NO_REINTENTABLE" {
		t.Fatalf("reintentar enviado: %d %v", st, d)
	}
	if st, _ := e.pedir("POST", fmt.Sprintf("%s/correo/mensajes/%d/reintentar", base, fallido), adm, nil); st != 409 {
		t.Fatalf("reintentar dos veces: %d", st)
	}
	if st, _ := e.pedir("GET", base+"/correo/mensajes/999999", adm, nil); st != 404 {
		t.Fatalf("correo inexistente: %d", st)
	}
}

// E4 · Telegram por la interfaz común: simulado no sale; con bot (falso) envía y no filtra el token.
func TestTelegramSimuladoYBot(t *testing.T) {
	const base = "/api/v1/edificios/1"
	// Simulado: se registra y no sale nada.
	e := nuevo(t)
	adm := e.login("admin@demo.pe")
	_, d := e.pedir("POST", base+"/telegram/destinos", adm, map[string]any{"nombre": "Junta", "chat_id": "-555"})
	did := num(d["id"])
	st, r := e.pedir("POST", base+"/telegram/enviar", adm, map[string]any{"destino_id": did, "texto": "Hola junta"})
	if st != 202 || r["modo"] != "simulado" || num(r["conteo"].(map[string]any)["simulado"]) != 1 {
		t.Fatalf("envío simulado: %d %v", st, r)
	}
	if st, _ := e.pedir("POST", base+"/telegram/verificar", adm, nil); st != 409 {
		t.Fatalf("verificar en simulado: %d", st)
	}
	if st, _ := e.pedir("POST", base+"/telegram/destinos", adm, map[string]any{"nombre": "Malo", "chat_id": "abc"}); st != 422 {
		t.Fatalf("chat_id inválido: %d", st)
	}
	if st, _ := e.pedir("GET", base+"/telegram/mensajes", e.login("propietario201@demo.pe"), nil); st != 403 {
		t.Fatalf("propietario viendo la bandeja de Telegram: %d", st)
	}

	// Con bot: contra un httptest que imita la Bot API.
	bot := &botTelegram{}
	srv := bot.servidor(t)
	e = nuevoCon(t, func(c *config.Config) {
		c.TelegramModo, c.TelegramToken, c.TelegramURL = "bot", tokenBot, srv.URL
	})
	adm = e.login("admin@demo.pe")
	st, est := e.pedir("GET", base+"/telegram/estado", adm, nil)
	if st != 200 || est["modo"] != "bot" || est["bot_configurado"] != true {
		t.Fatalf("estado: %d %v", st, est)
	}
	raw, _ := json.Marshal(est)
	if strings.Contains(string(raw), tokenBot) {
		t.Fatal("el estado filtra el token")
	}
	_, d = e.pedir("POST", base+"/telegram/destinos", adm, map[string]any{"nombre": "Propietarios", "chat_id": "-777"})
	did = num(d["id"])
	_, _ = e.pedir("POST", base+"/telegram/destinos", adm, map[string]any{"nombre": "Apagado", "chat_id": "-888"})
	_, d = e.pedir("GET", base+"/telegram/destinos", adm, nil)
	for _, it := range d["datos"].([]any) {
		if m := it.(map[string]any); m["chat_id"] == "-888" {
			if st, _ := e.pedir("PUT", fmt.Sprintf("%s/telegram/destinos/%d", base, num(m["id"])), adm, map[string]any{"activo": false}); st != 200 {
				t.Fatalf("desactivar destino: %d", st)
			}
		}
	}
	st, r = e.pedir("POST", base+"/telegram/enviar", adm, map[string]any{"todos": true, "texto": "Mañana fumigación"})
	if st != 202 || num(r["encolados"]) != 1 || num(r["conteo"].(map[string]any)["enviado"]) != 1 {
		t.Fatalf("envío con bot (solo destinos activos): %d %v", st, r)
	}
	if bot.cantidad() != 1 || bot.envios[0]["chat_id"] != "-777" || bot.envios[0]["text"] != "Mañana fumigación" {
		t.Fatalf("lo que recibió el bot: %v", bot.envios)
	}

	// El bot falla: el mensaje queda en la cola con el error (sin el token) y no se marca enviado.
	bot.mu.Lock()
	bot.falla = true
	bot.mu.Unlock()
	_, r = e.pedir("POST", base+"/telegram/enviar", adm, map[string]any{"destino_id": did, "texto": "Segundo aviso"})
	mid := num(r["mensaje_ids"].([]any)[0])
	var estado, errTxt string
	_ = e.pool.QueryRow(context.Background(), `SELECT estado, error FROM telegram_mensaje WHERE id=$1`, mid).Scan(&estado, &errTxt)
	if estado != "pendiente" || !strings.Contains(errTxt, "chat not found") || strings.Contains(errTxt, tokenBot) {
		t.Fatalf("fallo del bot: estado=%s error=%q", estado, errTxt)
	}
	// Tras agotar los intentos queda en «error»; el reintento lo manda cuando el bot vuelve.
	_, _ = e.pool.Exec(context.Background(), `UPDATE telegram_mensaje SET estado='error' WHERE id=$1`, mid)
	bot.mu.Lock()
	bot.falla = false
	bot.mu.Unlock()
	st, r = e.pedir("POST", fmt.Sprintf("%s/telegram/mensajes/%d/reintentar", base, mid), adm, nil)
	if st != 200 || r["estado"] != "enviado" || bot.cantidad() != 2 {
		t.Fatalf("reintento con bot: %d %v (bot recibió %d)", st, r, bot.cantidad())
	}
	if st, _ := e.pedir("POST", fmt.Sprintf("%s/telegram/mensajes/%d/reintentar", base, mid), adm, nil); st != 409 {
		t.Fatalf("reintentar lo enviado: %d", st)
	}
	st, l := e.pedir("GET", base+"/telegram/mensajes?estado=enviado", adm, nil)
	if st != 200 || num(l["total"]) != 2 {
		t.Fatalf("bandeja de Telegram: %d %v", st, l)
	}
	raw, _ = json.Marshal(l)
	if strings.Contains(string(raw), tokenBot) {
		t.Fatal("la bandeja filtra el token")
	}
}

// E5 · FAQ, Academia y Beneficios: el portal ve lo publicado y vigente; solo la administración edita.
func TestContenidoPortal(t *testing.T) {
	e := nuevo(t)
	adm := e.login("admin@demo.pe")
	prop := e.login("propietario201@demo.pe")
	inq := e.login("inquilino@demo.pe")
	const base = "/api/v1/edificios/1"

	crear := func(ruta string, cuerpo map[string]any) int64 {
		t.Helper()
		st, d := e.pedir("POST", base+ruta, adm, cuerpo)
		if st != 201 {
			t.Fatalf("crear %s: %d %v", ruta, st, d)
		}
		return num(d["id"])
	}
	faqVis := crear("/faq", map[string]any{"pregunta": "¿Cuándo vence el recibo?", "respuesta": "El día 15 de cada mes.", "categoria": "Pagos"})
	faqOculta := crear("/faq", map[string]any{"pregunta": "Borrador", "respuesta": "Aún no", "publicado": false})
	acad := crear("/academia", map[string]any{"titulo": "Cómo subir tu voucher", "tipo": "video", "url": "https://example.com/video"})
	benVig := crear("/beneficios", map[string]any{"titulo": "Lavandería", "descuento": "15 %", "vigente_hasta": "2099-12-31"})
	benVencido := crear("/beneficios", map[string]any{"titulo": "Pizza", "descuento": "2x1", "vigente_hasta": "2020-01-01"})
	benSinFecha := crear("/beneficios", map[string]any{"titulo": "Gimnasio", "codigo": "EDISYS10"})

	// Validaciones.
	casos := []struct {
		ruta   string
		cuerpo map[string]any
	}{
		{"/faq", map[string]any{"pregunta": "Sin respuesta"}},
		{"/academia", map[string]any{"titulo": "Vacía"}},
		{"/academia", map[string]any{"titulo": "Mal enlace", "url": "javascript:alert(1)"}},
		{"/academia", map[string]any{"titulo": "Tipo raro", "tipo": "podcast", "contenido": "x"}},
		{"/beneficios", map[string]any{"titulo": "Fecha mala", "vigente_hasta": "31/12/2099"}},
		{"/faq", map[string]any{"pregunta": "p", "respuesta": "r", "orden": 1.5}},
	}
	for _, c := range casos {
		if st, d := e.pedir("POST", base+c.ruta, adm, c.cuerpo); st != 422 {
			t.Fatalf("%s %v debía ser 422: %d %v", c.ruta, c.cuerpo, st, d)
		}
	}
	// Solo la administración edita.
	if st, _ := e.pedir("POST", base+"/faq", prop, map[string]any{"pregunta": "p", "respuesta": "r"}); st != 403 {
		t.Fatalf("propietario creando FAQ: %d", st)
	}

	// Lo que ve cada uno.
	for _, tok := range []string{prop, inq} {
		_, f := e.pedir("GET", base+"/faq", tok, nil)
		if ids := idsDe(f); !ids[faqVis] || ids[faqOculta] {
			t.Fatalf("FAQ del portal: %v", f)
		}
		_, a := e.pedir("GET", base+"/academia", tok, nil)
		if !idsDe(a)[acad] {
			t.Fatalf("academia del portal: %v", a)
		}
		_, b := e.pedir("GET", base+"/beneficios", tok, nil)
		if ids := idsDe(b); !ids[benVig] || !ids[benSinFecha] || ids[benVencido] {
			t.Fatalf("beneficios del portal (sin vencidos): %v", b)
		}
	}
	_, f := e.pedir("GET", base+"/faq", adm, nil)
	if ids := idsDe(f); !ids[faqVis] || !ids[faqOculta] {
		t.Fatalf("la administración ve todo: %v", f)
	}
	_, b := e.pedir("GET", base+"/beneficios", adm, nil)
	for _, it := range b["datos"].([]any) {
		m := it.(map[string]any)
		if num(m["id"]) == benVencido && m["vigente"] != false {
			t.Fatalf("el vencido debe marcarse no vigente: %v", m)
		}
	}

	// Ocultar y borrar.
	if st, _ := e.pedir("PUT", fmt.Sprintf("%s/faq/%d", base, faqVis), adm, map[string]any{"publicado": false}); st != 200 {
		t.Fatalf("ocultar FAQ: %d", st)
	}
	if _, f := e.pedir("GET", base+"/faq", prop, nil); idsDe(f)[faqVis] {
		t.Fatal("una FAQ oculta sigue en el portal")
	}
	if st, _ := e.pedir("PUT", fmt.Sprintf("%s/faq/%d", base, faqVis), adm, map[string]any{"respuesta": "  "}); st != 422 {
		t.Fatalf("vaciar un obligatorio al editar: %d", st)
	}
	if st, _ := e.pedir("DELETE", fmt.Sprintf("%s/beneficios/%d", base, benVencido), adm, nil); st != 200 {
		t.Fatalf("borrar beneficio: %d", st)
	}
	if st, _ := e.pedir("DELETE", fmt.Sprintf("%s/beneficios/%d", base, benVencido), adm, nil); st != 404 {
		t.Fatalf("borrar dos veces: %d", st)
	}
}
