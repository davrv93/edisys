package app

// Aprendizaje del chatbot: reglas → golden del edificio → motor local → LLM
// (Gemini) → menú. Una sola pasada por mensaje, sin recursión ni reintentos.
// Lo que se aprende es a CLASIFICAR (pregunta → intención); las cifras siempre
// salen de la base en el switch de Responder. Sin clave LLM, ese paso se salta.

import (
	"context"
	"net/http"
	"time"

	"edisys/api/internal/chatbot"
	"edisys/api/internal/db"
	"edisys/api/internal/llm"
	P "edisys/api/internal/plataforma"
)

// goldenSemilla: preguntas del dominio con su intención, fundadas en la semilla
// (Yape 987 654 321, parrillas, SUM, piscina, morosidad 13,1 %). Se siembran por
// edificio la primera vez que habla (asegurarGolden); el resto lo aprende el
// feedback. La norma la calcula Normalizar al insertar.
var goldenSemilla = []struct {
	pregunta  string
	intencion string
}{
	{"hola buenos dias", chatbot.Saludo},
	{"buenas noches", chatbot.Saludo},
	{"alo hay alguien", chatbot.Saludo},
	{"ayuda por favor", chatbot.Menu},
	{"que puedes hacer", chatbot.Menu},
	{"cto debo pa el mes", chatbot.Saldo},
	{"cual es mi deuda actual", chatbot.Saldo},
	{"cuanto debo en total", chatbot.Saldo},
	{"mi recibo de este mes", chatbot.UltimoRecibo},
	{"cuanto me toca pagar", chatbot.UltimoRecibo},
	{"oia me pueden rekordar el numero de yape de la administracion", chatbot.Pagar},
	{"cual es el numero de yape", chatbot.Pagar},
	{"como hago para pagar", chatbot.Pagar},
	{"quiero apartar la parrilla para el sabado", chatbot.Reservar},
	{"el sum se puede reservar un domingo", chatbot.Reservar},
	{"se malogro el ascensor", chatbot.Reportar},
	{"hay un foco quemado en mi piso", chatbot.Reportar},
	{"la piscina hasta que hora abre", chatbot.Horarios},
	{"cuanta gente maximo caben en la piscina", chatbot.Horarios},
	{"cual es el aforo del sum", chatbot.Horarios},
	{"a que hora cierra la piscina", chatbot.Horarios},
	{"necesito hablar con el administrador", chatbot.HablarAdmin},
	{"quiero presentar un reclamo", chatbot.HablarAdmin},
}

// asegurarGolden siembra el lote del dominio si el edificio aún no tiene golden.
func (s *Server) asegurarGolden(ctx context.Context, eid int64) {
	var n int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM chatbot_golden WHERE edificio_id=$1`, eid).Scan(&n); err != nil || n > 0 {
		return
	}
	for _, g := range goldenSemilla {
		_, _ = s.DB.Exec(ctx, `INSERT INTO chatbot_golden (edificio_id, pregunta, norma, intencion)
			VALUES ($1,$2,$3,$4) ON CONFLICT (edificio_id, norma) DO NOTHING`,
			eid, g.pregunta, chatbot.Normalizar(g.pregunta), g.intencion)
	}
}

// goldenPara: la pregunta tal cual (normalizada) ya se aprendió en este edificio.
func (s *Server) goldenPara(ctx context.Context, eid int64, texto string) (string, bool) {
	var in string
	err := s.DB.QueryRow(ctx, `SELECT intencion FROM chatbot_golden WHERE edificio_id=$1 AND norma=$2 AND activa`, eid, chatbot.Normalizar(texto)).Scan(&in)
	if err != nil || in == "" {
		return "", false
	}
	for _, v := range llm.Intenciones {
		if in == v {
			return in, true
		}
	}
	return "", false
}

// clasificarLLM: Gemini como último recurso antes del menú. Cuenta cada intento
// en chatbot_llm_uso (cuota a la vista en la UI). Nunca falla: "" = no supo.
func (s *Server) clasificarLLM(ctx context.Context, eid int64, texto string) string {
	if s.Cfg.LLMAPIKey == "" {
		return ""
	}
	cl := llm.Clasificador{
		Clave: s.Cfg.LLMAPIKey, ClaveRespaldo: s.Cfg.LLMAPIKeyRespaldo,
		Modelo: s.Cfg.LLMModelo, ModeloRespaldo: s.Cfg.LLMModeloRespaldo,
		HTTP: s.HTTP, Timeout: time.Duration(s.Cfg.LLMTimeoutSeg) * time.Second,
	}
	in, intentos := cl.Clasificar(ctx, texto)
	dia := time.Now().In(P.Lima).Format("2006-01-02")
	for _, it := range intentos {
		fallo := 0
		if it.Fallo != "" {
			fallo = 1
		}
		_, _ = s.DB.Exec(ctx, `INSERT INTO chatbot_llm_uso (dia, edificio_id, modelo, llamadas, fallos)
			VALUES ($1,$2,$3,1,$4) ON CONFLICT (dia, edificio_id, modelo)
			DO UPDATE SET llamadas=chatbot_llm_uso.llamadas+1, fallos=chatbot_llm_uso.fallos+$4`,
			dia, eid, it.Modelo, fallo)
	}
	if in == "" || in == "ninguna" {
		return ""
	}
	return in
}

// guardarMensaje persiste el intercambio (best-effort: nunca rompe la respuesta)
// y deja el id en datos para el feedback.
func (s *Server) guardarMensaje(ctx context.Context, res *RespuestaBot, tel, texto string) {
	if res == nil || res.EdificioID == 0 {
		return
	}
	origen, _ := res.Datos["origen"].(string)
	if origen == "" {
		origen = "reglas"
	}
	var id int64
	err := s.DB.QueryRow(ctx, `INSERT INTO chatbot_mensaje (edificio_id, telefono, texto, respuesta, intencion, origen)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		res.EdificioID, tel, texto, res.Respuesta, res.Intencion, origen).Scan(&id)
	if err != nil {
		return
	}
	res.Datos["mensaje_id"] = id
}

// feedbackMensaje: POST /chatbot/mensajes/{mid}/feedback {valor: util|mal}.
// Un «útil» sobre intención conocida confirma la golden (la crea o suma).
func (s *Server) feedbackMensaje(w http.ResponseWriter, r *http.Request) {
	mid, err := idRuta(r, "mid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Valor string `json:"valor"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.Valor != "util" && in.Valor != "mal" {
		P.Fallo(w, r, P.Validacion("Valor inválido.").Campo("valor", "util o mal."))
		return
	}
	e := edf(r)
	ctx := r.Context()
	var m struct {
		eid       int64
		texto     string
		intencion string
	}
	if err := s.DB.QueryRow(ctx, `SELECT edificio_id, texto, intencion FROM chatbot_mensaje WHERE id=$1 AND edificio_id=$2`,
		mid, e.ID).Scan(&m.eid, &m.texto, &m.intencion); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el mensaje"))
		return
	}
	if _, err := s.DB.Exec(ctx, `INSERT INTO chatbot_feedback (mensaje_id, edificio_id, valor) VALUES ($1,$2,$3)
		ON CONFLICT (mensaje_id) DO UPDATE SET valor=EXCLUDED.valor, creado_en=now()`, mid, e.ID, in.Valor); err != nil {
		P.Fallo(w, r, err)
		return
	}
	confirmada := false
	if in.Valor == "util" && m.intencion != "" && m.intencion != chatbot.NoEntendi && m.intencion != "motor" {
		var ok bool
		for _, v := range llm.Intenciones {
			if m.intencion == v {
				ok = true
			}
		}
		if ok {
			if _, err := s.DB.Exec(ctx, `INSERT INTO chatbot_golden (edificio_id, pregunta, norma, intencion, confirmaciones)
				VALUES ($1,$2,$3,$4,1) ON CONFLICT (edificio_id, norma)
				DO UPDATE SET confirmaciones=chatbot_golden.confirmaciones+1, intencion=EXCLUDED.intencion,
				  activa=true, actualizado_en=now()`, e.ID, m.texto, chatbot.Normalizar(m.texto), m.intencion); err == nil {
				confirmada = true
			}
		}
	}
	s.auditarCambio(ctx, s.DB, r, "chatbot", "feedback", "chatbot_mensaje", mid, nil, map[string]any{"valor": in.Valor, "golden": confirmada})
	P.JSON(w, http.StatusOK, map[string]any{"mensaje_id": mid, "valor": in.Valor, "golden_confirmada": confirmada})
}

// listarGolden: GET /chatbot/golden → las aprendidas del edificio con sus conteos.
func (s *Server) listarGolden(w http.ResponseWriter, r *http.Request) {
	filas, err := db.Filas(r.Context(), s.DB, `SELECT g.id, g.pregunta, g.intencion, g.confirmaciones, g.activa,
			to_char(g.actualizado_en,'YYYY-MM-DD') AS actualizada,
			(SELECT count(*) FROM chatbot_feedback f JOIN chatbot_mensaje m ON m.id=f.mensaje_id
				WHERE m.edificio_id=g.edificio_id AND m.intencion=g.intencion AND f.valor='util') AS utiles,
			(SELECT count(*) FROM chatbot_feedback f JOIN chatbot_mensaje m ON m.id=f.mensaje_id
				WHERE m.edificio_id=g.edificio_id AND m.intencion=g.intencion AND f.valor='mal') AS mal
		FROM chatbot_golden g WHERE g.edificio_id=$1 ORDER BY g.confirmaciones DESC, g.id`, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas)})
}

// cambiarGolden: PATCH /chatbot/golden/{gid} {activa} — apagar sin borrar.
func (s *Server) cambiarGolden(w http.ResponseWriter, r *http.Request) {
	gid, err := idRuta(r, "gid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Activa *bool `json:"activa"`
	}
	if err := P.Leer(r, &in); err != nil || in.Activa == nil {
		P.Fallo(w, r, P.Validacion("Indica activa.").Campo("activa", "true o false."))
		return
	}
	e := edf(r)
	ctx := r.Context()
	tag, err := s.DB.Exec(ctx, `UPDATE chatbot_golden SET activa=$3, actualizado_en=now() WHERE id=$1 AND edificio_id=$2`,
		gid, e.ID, *in.Activa)
	if err != nil || tag.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("la golden"))
		return
	}
	s.auditarCambio(ctx, s.DB, r, "chatbot", "golden", "chatbot_golden", gid, nil, map[string]any{"activa": *in.Activa})
	P.JSON(w, http.StatusOK, map[string]any{"id": gid, "activa": *in.Activa})
}

// borrarGolden: DELETE /chatbot/golden/{gid}.
func (s *Server) borrarGolden(w http.ResponseWriter, r *http.Request) {
	gid, err := idRuta(r, "gid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM chatbot_golden WHERE id=$1 AND edificio_id=$2`, gid, e.ID)
	if err != nil || tag.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("la golden"))
		return
	}
	s.auditarCambio(r.Context(), s.DB, r, "chatbot", "golden_borrar", "chatbot_golden", gid, nil, nil)
	w.WriteHeader(http.StatusNoContent)
}

// resumenAprendizaje: GET /chatbot/aprendizaje → golden, feedback por intención y uso del LLM.
func (s *Server) resumenAprendizaje(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	var golden, goldenActivas int
	_ = s.DB.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE activa) FROM chatbot_golden WHERE edificio_id=$1`, e.ID).Scan(&golden, &goldenActivas)
	fb, _ := db.Filas(ctx, s.DB, `SELECT m.intencion, f.valor, count(*) AS n FROM chatbot_feedback f
		JOIN chatbot_mensaje m ON m.id=f.mensaje_id WHERE m.edificio_id=$1 GROUP BY m.intencion, f.valor ORDER BY n DESC LIMIT 50`, e.ID)
	hoy := time.Now().In(P.Lima).Format("2006-01-02")
	uso, _ := db.Filas(ctx, s.DB, `SELECT modelo, llamadas, fallos FROM chatbot_llm_uso WHERE dia=$1 AND edificio_id=$2 ORDER BY modelo`, hoy, e.ID)
	llmCfg := map[string]any{"activo": s.Cfg.LLMAPIKey != "", "modelo": s.Cfg.LLMModelo, "respaldo": s.Cfg.LLMModeloRespaldo}
	P.JSON(w, http.StatusOK, map[string]any{
		"golden":   map[string]any{"total": golden, "activas": goldenActivas},
		"feedback": fb, "llm": llmCfg, "llm_uso_hoy": uso,
	})
}
