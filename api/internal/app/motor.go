// Motor conversacional (F1–F9): el chatbot por reglas sigue siendo la vía rápida;
// el motor (LLM local en español) responde lo que las reglas no entendieron y
// las consultas de datos vía GoldenSQL con guardas duras (solo SELECT, lista
// blanca, edificio_id obligatorio). Todo detrás del ajuste `motor_activado`.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// ---------- ajuste del motor ----------

// ajusteMotor devuelve motor_activado de edificio (off por defecto).
func (s *Server) ajusteMotor(ctx context.Context, eid int64) string {
	var modo string
	_ = s.DB.QueryRow(ctx, `SELECT COALESCE(config_json->>'motor_activado','off') FROM edificio WHERE id=$1`, eid).Scan(&modo)
	switch modo {
	case "off", "solo_admin", "todos":
		return modo
	}
	return "off"
}

// --------- evaluation de la respuesta del motor (F5: entendimiento) ---------

var sinDatos = regexp.MustCompile(`(?i)(no tengo ese dato|no cuento con|no dispongo)`)

// evaluaMotor califica la respuesta: mayor es mejor. Es la guía del admin para
// confirmar la corrección en memoria (F5) y del test de humo del motor.
func evaluaMotor(pregunta, respuesta, esperado string) int {
	puntaje := 0
	bajo := strings.ToLower(respuesta)
	if sinDatos.MatchString(bajo) {
		return -2
	}
	for _, p := range []string{"\ufffd", "ï¿½"} {
		if strings.Contains(respuesta, p) {
			return -3
		}
	}
	esperadoLimpio, preguntaLimpia := strings.ToLower(esperado), strings.ToLower(pregunta)
	for _, p := range []string{"de", "del", "la", "el", "cuanto", "cuánto", "esta", "está"} {
		preguntaLimpia = strings.ReplaceAll(preguntaLimpia, p, " ")
	}
	for _, w := range strings.Fields(preguntaLimpia) {
		if len([]rune(w)) >= 4 && strings.Contains(bajo, w) {
			puntaje += 2
		}
	}
	if esperadoLimpio != "" && strings.Contains(respuesta, esperadoLimpio) {
		puntaje += 3
	}
	if len([]rune(respuesta)) > 400 {
		puntaje--
	}
	return puntaje
}

// ---------- cliente del motor (llama-server vía app.py) ----------

const motorTimeout = 90 * time.Second

// clienteMotor aparte: el HTTP del Server corta a 15 s (para Evolution) y el
// LLM local tarda mucho más en una pregunta larga.
var clienteMotor = &http.Client{Timeout: motorTimeout + 5*time.Second}

// pideMotor hace el POST /v1/chat al motor y decodifica la respuesta.
func (s *Server) pideMotor(ctx context.Context, cuerpo map[string]any) (texto string, sugerencias []string, ok bool) {
	cuerpoJSON, _ := json.Marshal(cuerpo)
	cta, cancel := context.WithTimeout(ctx, motorTimeout)
	defer cancel()
	peticion, err := http.NewRequestWithContext(cta, http.MethodPost, s.Cfg.MotorURL+"/v1/chat", bytes.NewReader(cuerpoJSON))
	if err != nil {
		return "", nil, false
	}
	peticion.Header.Set("Content-Type", "application/json")
	if s.Cfg.MotorToken != "" {
		peticion.Header.Set("Authorization", "Bearer "+s.Cfg.MotorToken)
	}
	resp, err := clienteMotor.Do(peticion)
	if err != nil {
		return "", nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", nil, false
	}
	var fuera struct {
		Respuesta    string   `json:"respuesta"`
		Sugerencias  []string `json:"sugerencias"`
		GeneroTokens int      `json:"tokens_generados"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&fuera); err != nil {
		return "", nil, false
	}
	return fuera.Respuesta, fuera.Sugerencias, true
}

// preguntaMotor manda la conversación al motor tal cual (sin golden).
func (s *Server) preguntaMotor(ctx context.Context, eid int64, mensajes []map[string]string, sugerencias bool) (texto string, lista []string, ok bool) {
	if s.ajusteMotor(ctx, eid) == "off" || s.Cfg.MotorURL == "" {
		return "", nil, false
	}
	return s.pideMotor(ctx, map[string]any{"mensajes": mensajes, "pedir_sugerencias": sugerencias})
}

// preguntaMotorGolden: además recupera golden parecidos por embeddings (e5 en el
// motor). Si el mejor cuadra (sim ≥ 0,80), lo EJECUTA con las guardas y manda las
// filas reales al prompt; los parecidos (≥ 0,60) van como ejemplos few-shot. El
// modelo nunca inventa ni ejecuta SQL: solo explica las filas que le pasamos.
func (s *Server) preguntaMotorGolden(ctx context.Context, eid int64, mensajes []map[string]string, sugerencias bool) (texto string, lista []string, ok bool, goldenID int64) {
	if s.ajusteMotor(ctx, eid) == "off" || s.Cfg.MotorURL == "" {
		return "", nil, false, 0
	}
	consulta := ""
	if len(mensajes) > 0 {
		consulta = mensajes[len(mensajes)-1]["content"]
	}
	datos := map[string]any{}
	if consulta != "" {
		if dorados, err := s.goldenCandidatos(ctx, eid); err == nil && len(dorados) > 0 {
			candidatos := make([]map[string]any, 0, len(dorados))
			for _, g := range dorados {
				candidatos = append(candidatos, map[string]any{"id": g["id"], "pregunta": g["pregunta"], "sql": g["sql"]})
			}
			orden := s.recuperaGolden(ctx, consulta, candidatos)
			ejemplos := []map[string]any{}
			for i, g := range orden {
				sim, _ := g["sim"].(float64)
				if i == 0 && sim >= 0.80 {
					if filas, err := s.ejecutaGolden(ctx, eid, g["id"].(int64), g["pregunta"].(string), g["sql"].(string), ""); err == nil {
						if len(filas) > 8 {
							filas = filas[:8]
						}
						datos["filas"] = filas
						goldenID = g["id"].(int64)
					}
				}
				if sim >= 0.60 {
					ejemplos = append(ejemplos, map[string]any{"pregunta": g["pregunta"]})
				}
			}
			if len(ejemplos) > 0 {
				datos["golden"] = ejemplos
			}
		}
	}
	texto, lista, ok = s.pideMotor(ctx, map[string]any{"mensajes": mensajes, "pedir_sugerencias": sugerencias, "datos": datos})
	return texto, lista, ok, goldenID
}

// recuperaGolden pide al motor (POST /v1/recuperar) el orden por similaridad e5
// de los golden con la pregunta. Cualquier fallo → nil (el chat sigue sin golden).
func (s *Server) recuperaGolden(ctx context.Context, consulta string, candidatos []map[string]any) []map[string]any {
	cuerpo, _ := json.Marshal(map[string]any{"consulta": consulta, "candidatos": candidatos})
	cta, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	peticion, err := http.NewRequestWithContext(cta, http.MethodPost, s.Cfg.MotorURL+"/v1/recuperar", bytes.NewReader(cuerpo))
	if err != nil {
		return nil
	}
	peticion.Header.Set("Content-Type", "application/json")
	if s.Cfg.MotorToken != "" {
		peticion.Header.Set("Authorization", "Bearer "+s.Cfg.MotorToken)
	}
	resp, err := clienteMotor.Do(peticion)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var fuera struct {
		Golden []map[string]any `json:"golden"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&fuera); err != nil {
		return nil
	}
	// JSON decodifica los números como float64; los golden los usamos como int64.
	salida := make([]map[string]any, 0, len(fuera.Golden))
	for _, g := range fuera.Golden {
		id, _ := g["id"].(float64)
		pregunta, _ := g["pregunta"].(string)
		sql, _ := g["sql"].(string)
		salida = append(salida, map[string]any{"id": int64(id), "pregunta": pregunta, "sql": sql, "sim": g["sim"]})
	}
	return salida
}

// ---------- GoldenSQL (F8) ----------

// listaBlanca: tablas que el SQL generado puede tocar. Fuera quedan persona,
// usuario y correo_* (Ley 29733: el modelo nunca consulta datos personales).
var listaBlanca = map[string]bool{
	"recibo": true, "recibo_linea": true, "pago": true, "periodo": true,
	"unidad": true, "area": true, "recurso": true, "reserva": true,
	"incidencia": true, "medidor": true, "lectura": true, "reparto_medidor": true,
	"egreso": true, "movimiento_banco": true, "edificio": true,
	"rubro": true, "concepto": true, "presupuesto": true, "recibo_general": true,
}

var (
	rePeligro  = regexp.MustCompile(`(?is);|--|/\*|\b(insert|update|delete|drop|alter|create|truncate|grant|revoke|copy|vacuum|analyze|call|do)\b`)
	reTabla    = regexp.MustCompile(`(?is)\b(?:from|join)\s+([a-z_][a-z_0-9]*)`)
	reCTE      = regexp.MustCompile(`(?is)\bwith\s+([a-z_][a-z_0-9]*)\s+as\s*\(|,\s*([a-z_][a-z_0-9]*)\s+as\s*\(`)
	reEdificio = regexp.MustCompile(`(?i)edificio_id\s*=\s*(\$?\d+|:edificio_id|\$\{[^}]*\})`)
)

// validaSQL aplica las guardas de §4 del plan. Devuelve el SQL limpio.
func validaSQL(sql string) (string, error) {
	sql = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sql), ";"))
	if sql == "" || len(sql) > 4000 {
		return "", errors.New("sql vacío o demasiado largo")
	}
	if !strings.HasPrefix(strings.ToLower(sql), "select") && !strings.HasPrefix(strings.ToLower(sql), "with") {
		return "", errors.New("solo se permiten SELECT")
	}
	if rePeligro.MatchString(sql) {
		return "", errors.New("el SQL contiene sentencias o comentarios prohibidos")
	}
	// Los alias de CTE (WITH deudas AS …, top AS …) son nombres de la consulta, no tablas.
	permitidas := make(map[string]bool, len(listaBlanca)+2)
	for k, v := range listaBlanca {
		permitidas[k] = v
	}
	for _, m := range reCTE.FindAllStringSubmatch(sql, -1) {
		for _, nombre := range m[1:] {
			if nombre != "" {
				permitidas[strings.ToLower(nombre)] = true
			}
		}
	}
	for _, m := range reTabla.FindAllStringSubmatch(sql, -1) {
		if !permitidas[strings.ToLower(m[1])] {
			return "", fmt.Errorf("tabla no permitida: %s", m[1])
		}
	}
	if !reEdificio.MatchString(sql) && !strings.Contains(sql, ":edificio_id") {
		return "", errors.New("el SQL debe filtrar por edificio_id")
	}
	return sql, nil
}

// goldenCandidatos trae los golden activos del edificio (+ los generales).
func (s *Server) goldenCandidatos(ctx context.Context, eid int64) ([]map[string]any, error) {
	return db.Filas(ctx, s.DB, `SELECT id, pregunta, sql, tablas FROM motor_golden_sql
		WHERE desactivada=false AND (edificio_id IS NULL OR edificio_id=$1) ORDER BY veces_usada DESC LIMIT 12`, eid)
}

// ejecutaGolden corre un SQL con las guardas y lo audita en motor_consulta.
func (s *Server) ejecutaGolden(ctx context.Context, eid int64, gid int64, pregunta, sql string, esperado string) (filas []map[string]any, err error) {
	limpio, err := validaSQL(sql)
	if err != nil {
		_ = s.auditaConsulta(ctx, eid, gid, pregunta, sql, 0, "rechazado: "+err.Error())
		return nil, err
	}
	// Los golden se escriben con :edificio_id; pgx habla $1.
	limpio = strings.ReplaceAll(limpio, ":edificio_id", "$1")
	t0 := time.Now()
	filas, err = db.Filas(ctx, s.DB, limpio, eid)
	ms := time.Since(t0).Milliseconds()
	estado := "ok"
	if err != nil {
		estado = "error: " + err.Error()
	} else {
		// Ejecutó bien: queda verificado y suma un uso (lo muestra la pantalla Motor).
		_, _ = s.DB.Exec(ctx, `UPDATE motor_golden_sql SET verifico_en=now(), veces_usada=veces_usada+1 WHERE id=$1`, gid)
	}
	_ = s.auditaConsulta(ctx, eid, gid, pregunta, limpio, ms, estado)
	return filas, err
}

func (s *Server) auditaConsulta(ctx context.Context, eid, gid int64, pregunta, sql string, ms int64, estado string) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO motor_consulta (edificio_id, golden_id, pregunta, sql, estado, duracion_ms)
		VALUES ($1,$2,$3,$4,$5,$6)`, eid, gid, pregunta, sql, estado, ms)
	return err
}

// ---------- endpoint: POST /motor/consulta ----------

// motorConsulta deja probar el motor y el GoldenSQL desde la app (permiso
// motor.administrar): {pregunta, esperado?} → {respuesta, golden, filas}.
func (s *Server) motorConsulta(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Pregunta string `json:"pregunta"`
		Esperado string `json:"esperado"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	pregunta := strings.TrimSpace(in.Pregunta)
	if pregunta == "" {
		P.Fallo(w, r, P.Validacion("Escribe la pregunta de prueba.").Campo("pregunta", "Obligatorio."))
		return
	}
	eid := edf(r).ID

	// 1) GoldenSQL: ¿hay un golden parecido? (por ahora, igualdad por palabras clave)
	dorados, err := s.goldenCandidatos(r.Context(), eid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	para := func(pregunta string) string { // palabras clave normalizadas
		return strings.Join(strings.Fields(strings.ToLower(pregunta)), " ")
	}
	objetivo := para(pregunta)
	for _, g := range dorados {
		if para(g["pregunta"].(string)) == objetivo {
			filas, err := s.ejecutaGolden(r.Context(), eid, g["id"].(int64), pregunta, g["sql"].(string), in.Esperado)
			if err != nil {
				P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "SQL_RECHAZADO", err.Error()))
				return
			}
			P.JSON(w, http.StatusOK, map[string]any{"respuesta": "", "golden": g["id"], "filas": filas})
			return
		}
	}

	// 2) Motor conversacional (F1): pregunta y puntúa; la orquestación de golden
	// (recuperar → ejecutar si cuadra → few-shot) va dentro.
	resp, sug, ok, gid := s.preguntaMotorGolden(r.Context(), eid, []map[string]string{{"role": "user", "content": pregunta}}, false)
	if !ok {
		P.Fallo(w, r, P.Err(http.StatusServiceUnavailable, "MOTOR_NO_DISPONIBLE",
			"El motor no está disponible: revisa docker compose ps en el servicio motor."))
		return
	}
	fuera := map[string]any{
		"respuesta":   resp,
		"sugerencias": sug,
		"puntaje":     evaluaMotor(pregunta, resp, in.Esperado),
	}
	if gid > 0 {
		fuera["golden"] = gid
	}
	P.JSON(w, http.StatusOK, fuera)
}

// ---------- pantalla Motor (F8): administración ----------

// motorSalud pide /v1/salud al motor; «off» si no está configurado, «caido» si no responde.
func (s *Server) motorSalud(ctx context.Context) string {
	if s.Cfg.MotorURL == "" {
		return "off"
	}
	cta, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	peticion, err := http.NewRequestWithContext(cta, http.MethodGet, s.Cfg.MotorURL+"/v1/salud", nil)
	if err != nil {
		return "caido"
	}
	if s.Cfg.MotorToken != "" {
		peticion.Header.Set("Authorization", "Bearer "+s.Cfg.MotorToken)
	}
	resp, err := clienteMotor.Do(peticion)
	if err != nil {
		return "caido"
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return "ok"
	}
	return "caido"
}

// motorAdmin: GET /motor/admin → modo, salud del motor, golden, últimas consultas y propuestas.
func (s *Server) motorAdmin(w http.ResponseWriter, r *http.Request) {
	eid := edf(r).ID
	ctx := r.Context()
	modo := s.ajusteMotor(ctx, eid)
	salud := "off"
	if modo != "off" {
		salud = s.motorSalud(ctx)
	}
	golden, err := db.Filas(ctx, s.DB, `SELECT g.id, g.pregunta, g.sql, g.tablas, g.fuente, g.veces_usada, g.desactivada,
			to_char(g.verifico_en, 'DD/MM/YY HH24:MI') AS verifico, to_char(g.creado_en, 'DD/MM/YY') AS creado
		FROM motor_golden_sql g WHERE g.edificio_id IS NULL OR g.edificio_id=$1
		ORDER BY g.desactivada, g.veces_usada DESC, g.id`, eid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	consultas, err := db.Filas(ctx, s.DB, `SELECT c.id, c.pregunta, c.sql, c.estado, c.duracion_ms, c.filas,
			to_char(c.creado_en, 'DD/MM HH24:MI') AS cuando
		FROM motor_consulta c WHERE c.edificio_id=$1 ORDER BY c.id DESC LIMIT 25`, eid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	fuera := map[string]any{"modo": modo, "salud": salud, "golden": golden, "consultas": consultas}
	// Las propuestas de corrección viven en el motor (indice/memoria.json), no en la base.
	if s.Cfg.MotorURL != "" {
		cta, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		peticion, _ := http.NewRequestWithContext(cta, http.MethodGet, s.Cfg.MotorURL+"/v1/registro", nil)
		if s.Cfg.MotorToken != "" {
			peticion.Header.Set("Authorization", "Bearer "+s.Cfg.MotorToken)
		}
		if resp, err := clienteMotor.Do(peticion); err == nil {
			var registro struct {
				Propuestas    []map[string]any `json:"propuestas"`
				Interacciones []map[string]any `json:"interacciones"`
			}
			if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&registro) == nil {
				fuera["propuestas"] = registro.Propuestas
				fuera["interacciones"] = registro.Interacciones
			}
			_ = resp.Body.Close()
		}
	}
	P.JSON(w, http.StatusOK, fuera)
}

// tablasDe extrae las tablas reales (de la lista blanca) que usa un SQL; los CTE no cuentan.
func tablasDe(sql string) []string {
	vistas := map[string]bool{}
	var out []string
	for _, m := range reTabla.FindAllStringSubmatch(sql, -1) {
		t := strings.ToLower(m[1])
		if listaBlanca[t] && !vistas[t] {
			vistas[t] = true
			out = append(out, t)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// motorGolden: POST /motor/golden — crea o edita (con guardas antes de guardar),
// o solo marca desactivada=true/false si viene desactivada.
func (s *Server) motorGolden(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID          int64  `json:"id"`
		Pregunta    string `json:"pregunta"`
		SQL         string `json:"sql"`
		Desactivada *bool  `json:"desactivada"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	eid := edf(r).ID
	ctx := r.Context()
	if in.Desactivada != nil { // solo encender/apagar
		if in.ID == 0 {
			P.Fallo(w, r, P.Validacion("Falta el golden a desactivar.").Campo("id", "Obligatorio."))
			return
		}
		_, err := s.DB.Exec(ctx, `UPDATE motor_golden_sql SET desactivada=$2 WHERE id=$1 AND (edificio_id=$3 OR edificio_id IS NULL)`, in.ID, *in.Desactivada, eid)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		P.JSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	pregunta := strings.TrimSpace(in.Pregunta)
	if pregunta == "" {
		P.Fallo(w, r, P.Validacion("Escribe la pregunta que responde este golden.").Campo("pregunta", "Obligatorio."))
		return
	}
	if _, err := validaSQL(in.SQL); err != nil { // guardas §4 antes de guardar
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "SQL_RECHAZADO", err.Error()))
		return
	}
	if in.ID > 0 {
		_, err := s.DB.Exec(ctx, `UPDATE motor_golden_sql SET pregunta=$2, sql=$3, tablas=$4 WHERE id=$1 AND (edificio_id=$5 OR edificio_id IS NULL)`,
			in.ID, pregunta, in.SQL, tablasDe(in.SQL), eid)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		P.JSON(w, http.StatusOK, map[string]any{"ok": true, "id": in.ID})
		return
	}
	var gid int64
	err := s.DB.QueryRow(ctx, `INSERT INTO motor_golden_sql (edificio_id, pregunta, sql, tablas, fuente, creado_por)
		VALUES ($1,$2,$3,$4,'manual',$5) RETURNING id`, eid, pregunta, in.SQL, tablasDe(in.SQL), ses(r).UsuarioID).Scan(&gid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"ok": true, "id": gid})
}

// motorGoldenEjecutar: POST /motor/golden/{gid}/ejecutar — corre el golden ahora y deja la marca de verificación.
func (s *Server) motorGoldenEjecutar(w http.ResponseWriter, r *http.Request) {
	gid, err := strconv.ParseInt(chi.URLParam(r, "gid"), 10, 64)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	eid := edf(r).ID
	var pregunta, sql string
	err = s.DB.QueryRow(r.Context(), `SELECT pregunta, sql FROM motor_golden_sql WHERE id=$1 AND (edificio_id=$2 OR edificio_id IS NULL)`, gid, eid).Scan(&pregunta, &sql)
	if err != nil {
		P.Fallo(w, r, P.Err(http.StatusNotFound, "GOLDEN_NO_EXISTE", "Ese golden no existe en este edificio."))
		return
	}
	filas, err := s.ejecutaGolden(r.Context(), eid, gid, pregunta, sql, "")
	if err != nil {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "SQL_RECHAZADO", err.Error()))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"filas": filas})
}

// motorConfirma: POST /motor/confirma {pregunta, respuesta} — el admin confirma una
// corrección propuesta; el motor la guarda en su memoria (F5) y responde mejor la próxima vez.
func (s *Server) motorConfirma(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Pregunta  string `json:"pregunta"`
		Respuesta string `json:"respuesta"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if strings.TrimSpace(in.Pregunta) == "" || strings.TrimSpace(in.Respuesta) == "" {
		P.Fallo(w, r, P.Validacion("Faltan la pregunta o la respuesta.").Campo("pregunta", "Obligatorio."))
		return
	}
	cta, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cuerpo, _ := json.Marshal(map[string]any{
		"pregunta":       in.Pregunta,
		"respuesta":      in.Respuesta,
		"respondio_bien": true,
		"admin":          true,
	})
	peticion, _ := http.NewRequestWithContext(cta, http.MethodPost, s.Cfg.MotorURL+"/v1/feedback", bytes.NewReader(cuerpo))
	peticion.Header.Set("Content-Type", "application/json")
	if s.Cfg.MotorToken != "" {
		peticion.Header.Set("Authorization", "Bearer "+s.Cfg.MotorToken)
	}
	resp, err := clienteMotor.Do(peticion)
	if err != nil {
		P.Fallo(w, r, P.Err(http.StatusServiceUnavailable, "MOTOR_NO_DISPONIBLE", "El motor no está disponible."))
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	P.JSON(w, http.StatusOK, map[string]any{"ok": resp.StatusCode == http.StatusOK})
}

// motorAjuste: PUT /motor/ajuste {modo: off|solo_admin|todos}.
func (s *Server) motorAjuste(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Modo string `json:"modo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	switch in.Modo {
	case "off", "solo_admin", "todos":
	default:
		P.Fallo(w, r, P.Validacion("Modo inválido: off, solo_admin o todos.").Campo("modo", "Uno de: off, solo_admin, todos."))
		return
	}
	_, err := s.DB.Exec(r.Context(), `UPDATE edificio SET config_json = COALESCE(config_json,'{}'::jsonb) || $2::jsonb WHERE id=$1`,
		edf(r).ID, fmt.Sprintf(`{"motor_activado":%q}`, in.Modo))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"ok": true, "modo": in.Modo})
}
