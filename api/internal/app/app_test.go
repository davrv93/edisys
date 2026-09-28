package app_test

// Pruebas de integración contra PostgreSQL real (las reglas duras viven en la base).
// Usan la base edisys_test del compose (puerto 4754); si no hay base, se saltan.
//   TEST_DATABASE_URL=postgres://edisys:edisys@localhost:4754/edisys_test?sslmode=disable go test ./...

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"

	"edisys/api/internal/app"
	"edisys/api/internal/archivo"
	"edisys/api/internal/config"
	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
	"edisys/api/internal/reparto"
	"edisys/api/internal/seed"
)

type entorno struct {
	t    *testing.T
	pool *pgxpool.Pool
	srv  *httptest.Server
}

var (
	unaVez   sync.Once
	poolComp *pgxpool.Pool
	errPool  error
)

func urlPrueba() string {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://edisys:edisys@localhost:4754/edisys_test?sslmode=disable"
}

// abrirBase crea edisys_test si no existe, la deja vacía y aplica las migraciones (una vez por corrida).
func abrirBase(t *testing.T) *pgxpool.Pool {
	unaVez.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		u := urlPrueba()
		admin := strings.Replace(u, "/edisys_test", "/edisys", 1)
		if c, err := pgx.Connect(ctx, admin); err == nil {
			_, _ = c.Exec(ctx, `CREATE DATABASE edisys_test`)
			c.Close(ctx)
		}
		cfg, err := pgxpool.ParseConfig(u)
		if err != nil {
			errPool = err
			return
		}
		cfg.MaxConns = 25
		poolComp, errPool = pgxpool.NewWithConfig(ctx, cfg)
		if errPool == nil {
			errPool = poolComp.Ping(ctx)
		}
		if errPool != nil {
			return
		}
		if _, errPool = poolComp.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); errPool != nil {
			return
		}
		_, errPool = db.Migrar(ctx, poolComp)
	})
	if errPool != nil {
		t.Skipf("sin PostgreSQL de pruebas (%v): levanta `docker compose up -d postgres`", errPool)
	}
	return poolComp
}

// nuevo siembra el Edificio Demo y arranca el API en un servidor de prueba.
func nuevo(t *testing.T) *entorno {
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
	cfg.Tareas = false
	s, err := app.Nuevo(ctx, pool, alm, cfg)
	if err != nil {
		t.Fatal(err)
	}
	e := &entorno{t: t, pool: pool, srv: httptest.NewServer(s.Rutas())}
	t.Cleanup(e.srv.Close)
	return e
}

func (e *entorno) login(correo string) string {
	e.t.Helper()
	st, r := e.pedir("POST", "/api/v1/auth/login", "", map[string]string{"correo": correo, "clave": seed.ClaveDemo})
	if st != 200 {
		e.t.Fatalf("login %s: %d %v", correo, st, r)
	}
	return r["access_jwt"].(string)
}

func (e *entorno) pedir(metodo, ruta, tok string, cuerpo any) (int, map[string]any) {
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
	return e.hacer(req)
}

func (e *entorno) hacer(req *http.Request) (int, map[string]any) {
	e.t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	datos, _ := io.ReadAll(res.Body)
	var m map[string]any
	if err := json.Unmarshal(datos, &m); err != nil {
		var lista []any
		if json.Unmarshal(datos, &lista) == nil {
			m = map[string]any{"lista": lista}
		}
	}
	return res.StatusCode, m
}

func codigo(m map[string]any) string {
	if er, ok := m["error"].(map[string]any); ok {
		return fmt.Sprint(er["codigo"])
	}
	return ""
}

func num(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	}
	return -1
}

func (e *entorno) idUnidad(cod string) int64 {
	var id int64
	if err := e.pool.QueryRow(context.Background(), `SELECT id FROM unidad WHERE codigo=$1`, cod).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// ---------------------------------------------------------------------------------------------

// Setiembre cuadra con design/DISENO.md y el dashboard da lo mismo que el balance.
func TestKPIsSetiembreYDashboardIgualBalance(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	st, d := e.pedir("GET", "/api/v1/edificios/1/dashboard?periodo=2026-09", tok, nil)
	if st != 200 {
		t.Fatalf("dashboard %d %v", st, d)
	}
	k := d["kpis"].(map[string]any)
	quiero := map[string]int64{"ingresos_cts": 1946000, "egresos_cts": 1895000, "saldo_cts": 51000, "emitido_cts": 2240000, "banco_cts": 3412000}
	for c, v := range quiero {
		if num(k[c]) != v {
			t.Errorf("%s = %v, quiero %d", c, k[c], v)
		}
	}
	m := k["morosidad"].(map[string]any)
	if m["pct"].(float64) != 13.1 || num(m["unidades"]) != 3 || num(m["monto_cts"]) != 294000 {
		t.Errorf("morosidad %v", m)
	}
	_, b := e.pedir("GET", "/api/v1/edificios/1/balance?periodo=2026-09", tok, nil)
	if num(b["raiz"].(map[string]any)["total_cts"]) != num(k["saldo_cts"]) {
		t.Errorf("el saldo del balance (%v) no es el del dashboard (%v)", b["raiz"], k["saldo_cts"])
	}
	// Morosos: 402 (1.420), 503 (760), 104 (760).
	_, mo := e.pedir("GET", "/api/v1/edificios/1/morosidad?periodo=2026-09", tok, nil)
	deudas := map[string]int64{}
	for _, u := range mo["unidades"].([]any) {
		x := u.(map[string]any)
		deudas[x["unidad"].(string)] = num(x["deuda_cts"])
	}
	if deudas["402"] != 142000 || deudas["503"] != 76000 || deudas["104"] != 76000 || len(deudas) != 3 {
		t.Errorf("deudas %v", deudas)
	}
}

// La suma de los hijos es igual al padre, al céntimo, en todos los nodos.
func TestInvarianteBalance(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	var revisar func(id string, total int64)
	n := 0
	revisar = func(id string, total int64) {
		_, r := e.pedir("GET", "/api/v1/edificios/1/balance/nodos/"+id+"?periodo=2026-09", tok, nil)
		hijos, _ := r["lista"].([]any)
		var suma int64
		for _, h := range hijos {
			x := h.(map[string]any)
			suma += num(x["total_cts"])
			if x["tiene_hijos"] == true {
				revisar(x["id"].(string), num(x["total_cts"]))
			}
		}
		n++
		if suma != total {
			t.Errorf("nodo %s: hijos suman %d, el nodo dice %d", id, suma, total)
		}
	}
	_, b := e.pedir("GET", "/api/v1/edificios/1/balance?periodo=2026-09", tok, nil)
	for _, h := range b["raiz"].(map[string]any)["hijos"].([]any) {
		x := h.(map[string]any)
		revisar(x["id"].(string), num(x["total_cts"]))
	}
	if n < 10 {
		t.Errorf("solo revisé %d nodos", n)
	}
}

// El recibo del 201 es 705,60 + 196,00 + 8,40 + 80,00 = 990,00 y María no ve recibos ajenos (404).
func TestRecibo201YPropietarioNoVeAjeno(t *testing.T) {
	e := nuevo(t)
	tok := e.login("propietario201@demo.pe")
	_, l := e.pedir("GET", "/api/v1/edificios/1/recibos?periodo=2026-09", tok, nil)
	datos := l["datos"].([]any)
	if len(datos) != 1 {
		t.Fatalf("María debería ver 1 recibo de setiembre, ve %d", len(datos))
	}
	r := datos[0].(map[string]any)
	if num(r["total_cts"]) != 99000 || r["unidad"] != "201" {
		t.Errorf("recibo 201: %v", r)
	}
	var ajeno int64
	_ = e.pool.QueryRow(context.Background(), `SELECT r.id FROM recibo r JOIN unidad u ON u.id=r.unidad_id JOIN periodo p ON p.id=r.periodo_id WHERE u.codigo='402' AND p.periodo='2026-09'`).Scan(&ajeno)
	if st, _ := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/recibos/%d", ajeno), tok, nil); st != 404 {
		t.Errorf("recibo ajeno: %d, quiero 404", st)
	}
	// El inquilino no tiene permiso de recibos por defecto: 403.
	if st, d := e.pedir("GET", "/api/v1/edificios/1/recibos", e.login("inquilino@demo.pe"), nil); st != 403 || codigo(d) != "SIN_PERMISO" {
		t.Errorf("inquilino recibos: %d %v", st, d)
	}
}

// 20 peticiones simultáneas por la misma franja: exactamente 1 201 y 19 409.
func TestDobleReservaConcurrente(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	var recurso int64
	_ = e.pool.QueryRow(context.Background(), `SELECT id FROM recurso WHERE nombre='Parrilla 2'`).Scan(&recurso)
	dia := time.Now().In(P.Lima).AddDate(0, 0, 9).Format("2006-01-02")
	ini, _ := time.ParseInLocation("2006-01-02 15:04", dia+" 12:00", P.Lima)
	fin := ini.Add(5 * time.Hour)
	cuerpo := map[string]any{"recurso_id": recurso, "unidad_id": e.idUnidad("201"), "inicio": ini.Format(time.RFC3339), "fin": fin.Format(time.RFC3339), "acepta_normas": true}
	var wg sync.WaitGroup
	estados := make(chan int, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st, _ := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpo)
			estados <- st
		}()
	}
	wg.Wait()
	close(estados)
	cuenta := map[int]int{}
	for s := range estados {
		cuenta[s]++
	}
	if cuenta[201] != 1 || cuenta[409] != 19 {
		t.Fatalf("quiero 1×201 y 19×409, obtuve %v", cuenta)
	}
	// La base lo impide aunque se salte el API.
	_, err := e.pool.Exec(context.Background(), `INSERT INTO reserva (edificio_id, recurso_id, unidad_id, codigo, inicio, fin, estado) VALUES (1,$1,$2,'R-TEST',$3,$4,'confirmada')`,
		recurso, e.idUnidad("202"), ini.Add(time.Hour), fin)
	if err == nil || !strings.Contains(err.Error(), "23P01") {
		t.Errorf("la restricción EXCLUDE no saltó: %v", err)
	}
}

// El moroso no reserva: 403 MOROSO con el monto; la base también lo impide; el admin puede forzar con motivo.
func TestMorosoNoReserva(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	var recurso int64
	_ = e.pool.QueryRow(context.Background(), `SELECT id FROM recurso WHERE nombre='Parrilla 1'`).Scan(&recurso)
	dia := time.Now().In(P.Lima).AddDate(0, 0, 11).Format("2006-01-02")
	ini, _ := time.ParseInLocation("2006-01-02 15:04", dia+" 18:00", P.Lima)
	cuerpo := map[string]any{"recurso_id": recurso, "unidad_id": e.idUnidad("402"), "inicio": ini.Format(time.RFC3339), "fin": ini.Add(5 * time.Hour).Format(time.RFC3339), "acepta_normas": true}
	st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpo)
	if st != 403 || codigo(d) != "MOROSO" || num(d["error"].(map[string]any)["monto_cts"]) != 142000 {
		t.Fatalf("moroso: %d %v", st, d)
	}
	_, err := e.pool.Exec(context.Background(), `INSERT INTO reserva (edificio_id, recurso_id, unidad_id, codigo, inicio, fin, estado) VALUES (1,$1,$2,'R-TEST2',$3,$4,'confirmada')`,
		recurso, e.idUnidad("402"), ini, ini.Add(5*time.Hour))
	if err == nil || !strings.Contains(err.Error(), "ED002") {
		t.Errorf("el disparador de morosos no saltó: %v", err)
	}
	cuerpo["forzar_motivo"] = "Autorizado por la junta: pagará el lunes"
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpo); st != 201 {
		t.Errorf("reserva forzada: %d %v", st, d)
	}
	// El chatbot le explica al 402 por qué no puede reservar.
	_, c := e.pedir("POST", "/api/v1/chatbot/mensaje", tok, map[string]string{"telefono": "51900000402", "texto": "quiero reservar la parrilla mañana"})
	if c["intencion"] != "reservar" || !strings.Contains(c["respuesta"].(string), "S/ 1.420,00") {
		t.Errorf("chatbot moroso: %v", c)
	}
}

func padronExcel(t *testing.T, cambiar map[string]string) []byte {
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "Padron")
	cab := []string{"codigo", "tipo", "piso", "participacion_pct", "propietario_nombre", "propietario_dni_ruc", "propietario_correo", "propietario_celular", "alquilado"}
	for i, c := range cab {
		celda, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue("Padron", celda, c)
	}
	for i, c := range reparto.CodigosDemo {
		pct := fmt.Sprintf("%.4f", float64(reparto.ParticipacionDemo[c])/10000)
		if v, ok := cambiar[c]; ok {
			pct = v
		}
		fila := []any{c, "departamento", int(c[0] - '0'), pct, "Propietario " + c, "40000" + c, "", "900000" + c, "no"}
		for j, v := range fila {
			celda, _ := excelize.CoordinatesToCellName(j+1, i+2)
			_ = f.SetCellValue("Padron", celda, v)
		}
	}
	var b bytes.Buffer
	if err := f.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func (e *entorno) subirExcel(tok string, datos []byte) (int, map[string]any) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, _ := w.CreateFormFile("archivo", "padron.xlsx")
	_, _ = fw.Write(datos)
	_ = w.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/edificios/1/importaciones", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	return e.hacer(req)
}

// La participación suma 100 %: el Excel que suma 99,5 % se rechaza y no crea nada; la base también lo exige.
func TestParticipacionSuma100(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	st, r := e.subirExcel(tok, padronExcel(t, map[string]string{"101": "3.7"}))
	if st != 200 || r["bloqueante"] != true || r["suma_participacion_pct"] != "99.5000" {
		t.Fatalf("vista previa: %d %v", st, r)
	}
	encontrado := false
	for _, er := range r["errores"].([]any) {
		if strings.Contains(er.(map[string]any)["mensaje"].(string), "Suman 99,5000 %, faltan 0,5000 %") {
			encontrado = true
		}
	}
	if !encontrado {
		t.Errorf("falta el mensaje «Suman 99,5000 %%, faltan 0,5000 %%»: %v", r["errores"])
	}
	iid := num(r["importacion_id"])
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/importaciones/%d/confirmar", iid), tok, nil); st != 422 {
		t.Errorf("confirmar con errores: %d %v", st, d)
	}
	// Un padrón correcto entra completo (actualiza las 24 unidades).
	st, r = e.subirExcel(tok, padronExcel(t, nil))
	if st != 200 || r["bloqueante"] != false || num(r["validas"]) != 24 {
		t.Fatalf("padrón correcto: %d %v", st, r)
	}
	if st, d := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/importaciones/%d/confirmar", num(r["importacion_id"])), tok, nil); st != 200 || num(d["unidades_actualizadas"]) != 24 {
		t.Errorf("confirmar: %d %v", st, d)
	}
	// Regla dura en la base: mover una sola participación rompe el 100 % y el COMMIT falla.
	ctx := context.Background()
	tx, _ := e.pool.Begin(ctx)
	_, _ = tx.Exec(ctx, `UPDATE unidad SET participacion_pct = 5 WHERE codigo='101'`)
	if err := tx.Commit(ctx); err == nil || !strings.Contains(err.Error(), "ED001") {
		t.Errorf("el disparador de participación no saltó: %v", err)
	}
	// Y por el API: 422 PARTICIPACION_NO_SUMA_100.
	if st, d := e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/unidades/%d", e.idUnidad("101")), tok, map[string]any{"participacion_pct": 5}); st != 422 || codigo(d) != "PARTICIPACION_NO_SUMA_100" {
		t.Errorf("PUT unidad: %d %v", st, d)
	}
}

// Sin foto no hay lectura: 422 en el API y NOT NULL en la base.
func TestFotoObligatoriaEnLecturas(t *testing.T) {
	e := nuevo(t)
	tok := e.login("operario@demo.pe")
	var mid int64
	_ = e.pool.QueryRow(context.Background(), `SELECT id FROM medidor WHERE serie='AG-201'`).Scan(&mid)
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("periodo", "2026-09")
	_ = w.WriteField("valor", "1300.000")
	_ = w.Close()
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/edificios/1/medidores/%d/lecturas", e.srv.URL, mid), &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	if st, d := e.hacer(req); st != 422 || codigo(d) != "FOTO_OBLIGATORIA" {
		t.Errorf("sin foto: %d %v", st, d)
	}
	_, err := e.pool.Exec(context.Background(), `INSERT INTO lectura (medidor_id, periodo_id, valor, anterior, consumo, foto_id) SELECT $1, id, 1, 0, 1, NULL FROM periodo WHERE periodo='2026-10' LIMIT 1`, mid)
	_, err2 := e.pool.Exec(context.Background(), `INSERT INTO lectura (medidor_id, periodo_id, valor, anterior, consumo, foto_id) SELECT $1, id, 1, 0, 1, NULL FROM periodo WHERE periodo='2026-04'`, mid)
	if err2 == nil || !strings.Contains(err2.Error(), "23502") {
		t.Errorf("la base aceptó una lectura sin foto: %v %v", err, err2)
	}
}

// Transiciones del kanban y la votación de INC-014 (2 de 3 → aprobado con el tercer voto).
func TestKanbanYVotacion(t *testing.T) {
	e := nuevo(t)
	admin := e.login("admin@demo.pe")
	id := func(cod string) int64 {
		var v int64
		_ = e.pool.QueryRow(context.Background(), `SELECT id FROM incidencia WHERE codigo=$1`, cod).Scan(&v)
		return v
	}
	ruta := func(cod string) string { return fmt.Sprintf("/api/v1/mantenimiento/incidencias/%d/estado", id(cod)) }
	// INC-012 está «reportado»: no puede saltar a aprobado.
	if st, d := e.pedir("PATCH", ruta("INC-012"), admin, map[string]any{"estado": "aprobado"}); st != 409 || codigo(d) != "TRANSICION_INVALIDA" {
		t.Errorf("reportado→aprobado: %d %v", st, d)
	}
	if st, d := e.pedir("PATCH", ruta("INC-012"), admin, map[string]any{"estado": "validado"}); st != 422 || codigo(d) != "CRITICIDAD_OBLIGATORIA" {
		t.Errorf("validar sin criticidad: %d %v", st, d)
	}
	if st, d := e.pedir("PATCH", ruta("INC-012"), admin, map[string]any{"estado": "validado", "criticidad": "baja"}); st != 200 || d["estado"] != "validado" {
		t.Errorf("reportado→validado: %d %v", st, d)
	}
	// Filtros del kanban.
	_, l := e.pedir("GET", "/api/v1/mantenimiento/incidencias?estado=presupuestado&criticidad=critica", admin, nil)
	if num(l["total"]) != 1 || l["datos"].([]any)[0].(map[string]any)["codigo"] != "INC-014" {
		t.Errorf("filtro presupuestado+crítica: %v", l["total"])
	}
	// INC-014 (S/ 1.850) pasa el umbral de S/ 1.000: el administrador no puede aprobarla solo.
	if st, d := e.pedir("PATCH", ruta("INC-014"), admin, map[string]any{"estado": "aprobado"}); st != 409 || codigo(d) != "REQUIERE_VOTO_JUNTA" {
		t.Errorf("admin aprueba INC-014: %d %v", st, d)
	}
	votos := fmt.Sprintf("/api/v1/edificios/1/trabajos/%d/votos", id("INC-014"))
	if st, d := e.pedir("POST", votos, e.login("junta2@demo.pe"), map[string]any{"voto": "aprueba"}); st != 409 || codigo(d) != "YA_VOTASTE" {
		t.Errorf("voto repetido: %d %v", st, d)
	}
	st, d := e.pedir("POST", votos, e.login("junta@demo.pe"), map[string]any{"voto": "aprueba"})
	if st != 201 || d["resultado"] != "aprobado" || num(d["a_favor"]) != 3 {
		t.Fatalf("tercer voto: %d %v", st, d)
	}
	// aprobado → en_ejecucion (técnico) → terminado exige evidencia de cierre.
	tec := e.login("tecnico@demo.pe")
	if st, d := e.pedir("PATCH", ruta("INC-014"), tec, map[string]any{"estado": "en_ejecucion"}); st != 200 {
		t.Errorf("en_ejecucion: %d %v", st, d)
	}
	if st, d := e.pedir("PATCH", ruta("INC-014"), tec, map[string]any{"estado": "terminado"}); st != 422 || codigo(d) != "EVIDENCIA_CIERRE_OBLIGATORIA" {
		t.Errorf("terminado sin evidencia: %d %v", st, d)
	}
	// Un propietario no mueve el tablero.
	if st, _ := e.pedir("PATCH", ruta("INC-013"), e.login("propietario201@demo.pe"), map[string]any{"estado": "aprobado"}); st != 403 {
		t.Errorf("propietario aprueba: %d", st)
	}
	// La base también rechaza transiciones inválidas.
	if _, err := e.pool.Exec(context.Background(), `UPDATE incidencia SET estado='reportado' WHERE codigo='INC-001'`); err == nil || !strings.Contains(err.Error(), "ED004") {
		t.Errorf("el disparador de transiciones no saltó: %v", err)
	}
}

// Chatbot con cifras reales y WhatsApp en modo simulado (no sale nada).
func TestChatbotYWhatsAppSimulado(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	_, c := e.pedir("POST", "/api/v1/chatbot/mensaje", tok, map[string]string{"telefono": "51900000201", "texto": "¿cuánto debo?"})
	if c["intencion"] != "saldo" || num(c["datos"].(map[string]any)["deuda_cts"]) != 0 || !strings.Contains(c["respuesta"].(string), "al día") {
		t.Errorf("María cuánto debo: %v", c)
	}
	_, c = e.pedir("POST", "/api/v1/chatbot/mensaje", tok, map[string]string{"telefono": "900000402", "texto": "cuanto debo"})
	if num(c["datos"].(map[string]any)["deuda_cts"]) != 142000 {
		t.Errorf("402 cuánto debo: %v", c)
	}
	_, c = e.pedir("POST", "/api/v1/chatbot/mensaje", tok, map[string]string{"telefono": "51900000201", "texto": "hay una fuga de agua en el pasadizo del 2do piso"})
	if c["intencion"] != "reportar_incidencia" || c["datos"].(map[string]any)["codigo"] != "INC-015" {
		t.Errorf("reporte por chatbot: %v", c)
	}
	st, m := e.pedir("POST", "/api/v1/whatsapp/enviar", tok, map[string]any{"unidad_id": e.idUnidad("402"), "plantilla": "recordatorio_deuda", "variables": map[string]string{"saldo": "S/ 1.420,00"}})
	if st != 201 || m["estado"] != "simulado" || !strings.Contains(m["texto"].(string), "Luis") {
		t.Errorf("enviar: %d %v", st, m)
	}
	st, r := e.pedir("POST", "/api/v1/whatsapp/recibos/2026-09/enviar", tok, nil)
	if st != 202 || num(r["simulados"]) != 24 || num(r["enviados"]) != 0 {
		t.Errorf("recibos por WhatsApp: %d %v", st, r)
	}
	// Webhook entrante: responde por la bandeja, en simulado.
	st, w := e.pedir("POST", "/api/v1/whatsapp/webhook", "", map[string]any{"event": "messages.upsert", "data": map[string]any{
		"key": map[string]any{"remoteJid": "51900000201@s.whatsapp.net", "fromMe": false}, "message": map[string]any{"conversation": "hola"}}})
	if st != 200 || w["intencion"] != "saludo" {
		t.Errorf("webhook: %d %v", st, w)
	}
	// La clave de Evolution nunca se devuelve.
	_, _ = e.pedir("PUT", "/api/v1/whatsapp/config", tok, map[string]any{"modo": "evolution", "url": "http://evolution.invalid", "instancia": "demo", "apikey": "secreta"})
	_, cfg := e.pedir("GET", "/api/v1/whatsapp/config", tok, nil)
	b, _ := json.Marshal(cfg)
	if strings.Contains(string(b), "secreta") || cfg["envio_real"] != false || cfg["tiene_apikey"] != true {
		t.Errorf("config: %s", b)
	}
	// Aun con el edificio en «evolution», el servidor en simulado no envía nada.
	_, m = e.pedir("POST", "/api/v1/whatsapp/enviar", tok, map[string]any{"telefono": "51900000201", "plantilla": "libre", "variables": map[string]string{"texto": "prueba"}})
	if m["estado"] != "simulado" {
		t.Errorf("con el interruptor maestro en simulado salió: %v", m)
	}
}

// Seguridad básica: CSRF con cookie, permisos por rol, credenciales y analítica.
func TestSesionPermisosYAnalitica(t *testing.T) {
	e := nuevo(t)
	// Login por código de unidad + DNI.
	if st, d := e.pedir("POST", "/api/v1/auth/login", "", map[string]string{"usuario": "201-40000201", "clave": seed.ClaveDemo}); st != 200 {
		t.Errorf("login 201+DNI: %d %v", st, d)
	}
	if st, d := e.pedir("POST", "/api/v1/auth/login", "", map[string]string{"correo": "admin@demo.pe", "clave": "mala"}); st != 401 || codigo(d) != "CREDENCIALES" {
		t.Errorf("clave mala: %d %v", st, d)
	}
	// Con cookie, una escritura sin X-EDISYS es 403 CSRF.
	tok := e.login("admin@demo.pe")
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/whatsapp/enviar", strings.NewReader(`{}`))
	req.AddCookie(&http.Cookie{Name: "edisys_at", Value: tok})
	if st, d := e.hacer(req); st != 403 || codigo(d) != "CSRF" {
		t.Errorf("CSRF: %d %v", st, d)
	}
	// El operario no ve el balance.
	if st, d := e.pedir("GET", "/api/v1/edificios/1/balance", e.login("operario@demo.pe"), nil); st != 403 || codigo(d) != "SIN_PERMISO" {
		t.Errorf("operario balance: %d %v", st, d)
	}
	// Edificio ajeno: 404.
	if st, _ := e.pedir("GET", "/api/v1/edificios/999/dashboard", tok, nil); st != 404 {
		t.Errorf("edificio ajeno: %d", st)
	}
	st, a := e.pedir("GET", "/api/v1/analitica/resumen?desde=2026-04&hasta=2026-09", tok, nil)
	if st != 200 || len(a["cobranza_mensual"].([]any)) != 6 || len(a["morosidad_mensual"].([]any)) != 6 || len(a["consumo_agua"].([]any)) != 24 {
		t.Fatalf("analítica: %d %v", st, a)
	}
	ult := a["morosidad_mensual"].([]any)[5].(map[string]any)
	if ult["pct"].(float64) != 13.1 {
		t.Errorf("morosidad de setiembre en analítica: %v", ult)
	}
	// /yo arma el menú desde los permisos.
	_, yo := e.pedir("GET", "/api/v1/yo", e.login("operario@demo.pe"), nil)
	if yo["destino"] != "/app/e/1/lecturas" {
		t.Errorf("destino del operario: %v", yo["destino"])
	}
}
