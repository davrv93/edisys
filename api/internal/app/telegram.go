package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"edisys/api/internal/db"
	"edisys/api/internal/mensajeria"
	P "edisys/api/internal/plataforma"
)

// Telegram (bloque E4): bandeja de salida (telegram_mensaje, 0025) igual que WhatsApp y correo.
// TELEGRAM_MODO=simulado (por defecto) no envía nada; «bot» con TELEGRAM_BOT_TOKEN envía por la
// Bot API a través del adaptador común de mensajería. El token nunca sale por el API.

const maxTextoTelegram = 4096

// telegramReal: solo hay envío real si el servidor lo enciende y tiene token.
func (s *Server) telegramReal() bool {
	return s.Cfg.TelegramModo == "bot" && strings.TrimSpace(s.Cfg.TelegramToken) != ""
}

// canalTelegram devuelve el canal efectivo detrás de la interfaz común.
func (s *Server) canalTelegram() mensajeria.Canal {
	if !s.telegramReal() {
		return mensajeria.Simulado{Canal: "telegram"}
	}
	return mensajeria.Telegram{Token: s.Cfg.TelegramToken, URL: s.Cfg.TelegramURL, HTTP: s.HTTP}
}

func (s *Server) modoTelegram() string {
	if s.telegramReal() {
		return "bot"
	}
	return "simulado"
}

// recortarRunas corta un texto a n caracteres sin partir una letra.
func recortarRunas(t string, n int) string {
	if utf8.RuneCountInString(t) <= n {
		return t
	}
	r := []rune(t)
	return string(r[:n-1]) + "…"
}

// encolarTelegram registra un mensaje «pendiente» para un chat.
func (s *Server) encolarTelegram(ctx context.Context, q db.Q, eid int64, destino *int64, chatID, texto, origen, ref string, usuario *int64) (int64, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return 0, P.Validacion("Escribe el mensaje.").Campo("texto", "Obligatorio.")
	}
	if !mensajeria.ChatIDValido(chatID) {
		return 0, P.Validacion("Chat de Telegram inválido.").Campo("chat_id", "Id numérico (los grupos empiezan con -) o @canal.")
	}
	var id int64
	err := q.QueryRow(ctx, `INSERT INTO telegram_mensaje (edificio_id, destino_id, chat_id, texto, origen, referencia, enviado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, eid, destino, strings.TrimSpace(chatID), recortarRunas(texto, maxTextoTelegram), origen, ref, usuario).Scan(&id)
	return id, err
}

// despacharTelegram procesa la bandeja. En simulado marca «simulado»; con bot envía (3 intentos).
func (s *Server) despacharTelegram(ctx context.Context) {
	filas, err := s.DB.Query(ctx, `SELECT id, chat_id, texto FROM telegram_mensaje WHERE estado='pendiente' ORDER BY id LIMIT 100`)
	if err != nil {
		slog.Warn("telegram: leer bandeja", "err", err)
		return
	}
	type msg struct {
		id          int64
		chat, texto string
	}
	var ms []msg
	for filas.Next() {
		var m msg
		if err := filas.Scan(&m.id, &m.chat, &m.texto); err == nil {
			ms = append(ms, m)
		}
	}
	filas.Close()
	canal := s.canalTelegram()
	for _, m := range ms {
		// Reclamo atómico: si otro proceso ya lo tomó, se salta.
		var intentos int
		if err := s.DB.QueryRow(ctx, `UPDATE telegram_mensaje SET intentos=intentos+1 WHERE id=$1 AND estado='pendiente' RETURNING intentos`, m.id).Scan(&intentos); err != nil {
			continue
		}
		pid, err := canal.Enviar(ctx, m.chat, m.texto)
		if errors.Is(err, mensajeria.ErrSimulado) {
			_, _ = s.DB.Exec(ctx, `UPDATE telegram_mensaje SET estado='simulado', procesado_en=now() WHERE id=$1`, m.id)
			continue
		}
		if err != nil {
			estado := "pendiente"
			if intentos >= 3 {
				estado = "error"
			}
			_, _ = s.DB.Exec(ctx, `UPDATE telegram_mensaje SET estado=$2, error=$3, procesado_en=now() WHERE id=$1`, m.id, estado, recortar(err.Error(), 300))
			continue
		}
		_, _ = s.DB.Exec(ctx, `UPDATE telegram_mensaje SET estado='enviado', error='', proveedor_id=$2, procesado_en=now() WHERE id=$1`, m.id, pid)
	}
}

// ---------- endpoints ----------

// estadoTelegram: GET /telegram/estado → modo y si el bot tiene token (nunca el token).
func (s *Server) estadoTelegram(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var destinos int64
	_ = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM telegram_destino WHERE edificio_id=$1 AND activo`, e.ID).Scan(&destinos)
	P.JSON(w, http.StatusOK, map[string]any{"modo": s.modoTelegram(), "bot_configurado": strings.TrimSpace(s.Cfg.TelegramToken) != "", "destinos_activos": destinos})
}

// verificarTelegram: POST /telegram/verificar → pregunta a Telegram quién es el bot (getMe).
// En simulado no llama a nadie.
func (s *Server) verificarTelegram(w http.ResponseWriter, r *http.Request) {
	if !s.telegramReal() {
		P.Fallo(w, r, P.Conflicto("TELEGRAM_SIMULADO", "Telegram está en modo simulado: define TELEGRAM_MODO=bot y TELEGRAM_BOT_TOKEN en el servidor."))
		return
	}
	tg := s.canalTelegram().(mensajeria.Telegram)
	u, err := tg.Yo(r.Context())
	if err != nil {
		P.Fallo(w, r, P.Err(http.StatusBadGateway, "TELEGRAM_NO_RESPONDE", "Telegram no aceptó el bot: "+recortar(err.Error(), 200)))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"ok": true, "bot": u})
}

// listarDestinosTelegram: GET /telegram/destinos
func (s *Server) listarDestinosTelegram(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT d.id, d.nombre, d.chat_id, d.tipo, d.activo, d.unidad_id, u.codigo AS unidad,
			(SELECT count(*) FROM telegram_mensaje m WHERE m.destino_id=d.id) AS mensajes
		FROM telegram_destino d LEFT JOIN unidad u ON u.id=d.unidad_id
		WHERE d.edificio_id=$1 ORDER BY d.activo DESC, lower(d.nombre)`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearDestinoTelegram: POST /telegram/destinos {nombre, chat_id, tipo, unidad_id?}
func (s *Server) crearDestinoTelegram(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in struct {
		Nombre   string `json:"nombre"`
		ChatID   string `json:"chat_id"`
		Tipo     string `json:"tipo"`
		UnidadID *int64 `json:"unidad_id"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.Nombre, in.ChatID = strings.TrimSpace(in.Nombre), strings.TrimSpace(in.ChatID)
	if in.Tipo == "" {
		in.Tipo = "grupo"
	}
	ev := P.Validacion("Revisa el destino.")
	if in.Nombre == "" {
		ev.Campo("nombre", "Escribe un nombre (p. ej. «Grupo de propietarios»).")
	}
	if !mensajeria.ChatIDValido(in.ChatID) {
		ev.Campo("chat_id", "Id numérico (los grupos empiezan con -) o @canal.")
	}
	if in.Tipo != "grupo" && in.Tipo != "usuario" {
		ev.Campo("tipo", "grupo o usuario.")
	}
	if in.UnidadID != nil {
		var ok bool
		_ = s.DB.QueryRow(r.Context(), `SELECT true FROM unidad WHERE id=$1 AND edificio_id=$2`, *in.UnidadID, e.ID).Scan(&ok)
		if !ok {
			ev.Campo("unidad_id", "La unidad no es de este edificio.")
		}
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	var id int64
	err := s.DB.QueryRow(r.Context(), `INSERT INTO telegram_destino (edificio_id, nombre, chat_id, tipo, unidad_id) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		e.ID, in.Nombre, in.ChatID, in.Tipo, in.UnidadID).Scan(&id)
	if err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("DESTINO_DUPLICADO", "Ese chat ya está registrado en el edificio."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// activarDestinoTelegram: PUT /telegram/destinos/{id} {activo}
func (s *Server) activarDestinoTelegram(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	var in struct {
		Activo bool `json:"activo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE telegram_destino SET activo=$1 WHERE id=$2 AND edificio_id=$3`, in.Activo, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("el destino"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "activo": in.Activo})
}

// borrarDestinoTelegram: DELETE /telegram/destinos/{id}. Los mensajes ya enviados se quedan (destino_id → NULL).
func (s *Server) borrarDestinoTelegram(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	ct, err := s.DB.Exec(r.Context(), `DELETE FROM telegram_destino WHERE id=$1 AND edificio_id=$2`, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("el destino"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

// enviarTelegram: POST /telegram/enviar {destino_id? | todos, texto} → encola y despacha.
func (s *Server) enviarTelegram(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	var in struct {
		DestinoID *int64 `json:"destino_id"`
		Todos     bool   `json:"todos"`
		Texto     string `json:"texto"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if strings.TrimSpace(in.Texto) == "" {
		P.Fallo(w, r, P.Validacion("Escribe el mensaje.").Campo("texto", "Obligatorio."))
		return
	}
	if in.DestinoID == nil && !in.Todos {
		P.Fallo(w, r, P.Validacion("Elige un destino o «todos».").Campo("destino_id", "Obligatorio."))
		return
	}
	ds, err := s.destinosTelegram(ctx, e.ID, in.DestinoID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	uid := ses(r).UsuarioID
	ids := []int64{}
	for _, d := range ds {
		id, err := s.encolarTelegram(ctx, s.DB, e.ID, &d.id, d.chat, in.Texto, "manual", "", &uid)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		ids = append(ids, id)
	}
	s.despacharTelegram(ctx)
	P.JSON(w, http.StatusAccepted, map[string]any{"encolados": len(ids), "mensaje_ids": ids, "modo": s.modoTelegram(), "conteo": s.conteoTelegram(ctx, ids)})
}

type destinoTG struct {
	id   int64
	chat string
}

// destinosTelegram: uno (si viene id) o todos los activos del edificio.
func (s *Server) destinosTelegram(ctx context.Context, eid int64, uno *int64) ([]destinoTG, error) {
	filas, err := s.DB.Query(ctx, `SELECT id, chat_id FROM telegram_destino WHERE edificio_id=$1 AND activo AND ($2::bigint IS NULL OR id=$2) ORDER BY id`, eid, uno)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []destinoTG
	for filas.Next() {
		var d destinoTG
		if err := filas.Scan(&d.id, &d.chat); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if uno != nil && len(out) == 0 {
		return nil, P.NoEncontrado("el destino activo")
	}
	return out, filas.Err()
}

func (s *Server) conteoTelegram(ctx context.Context, ids []int64) map[string]int {
	c := map[string]int{"simulado": 0, "enviado": 0, "error": 0, "pendiente": 0}
	f, err := s.DB.Query(ctx, `SELECT estado, count(*) FROM telegram_mensaje WHERE id = ANY($1) GROUP BY estado`, ids)
	if err != nil {
		return c
	}
	defer f.Close()
	for f.Next() {
		var e string
		var n int
		if f.Scan(&e, &n) == nil {
			c[e] = n
		}
	}
	return c
}

// listarTelegram: GET /telegram/mensajes?estado=&q=&pagina=
func (s *Server) listarTelegram(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	q := r.URL.Query()
	pagina, por := paginacion(r)
	buscar := strings.TrimSpace(q.Get("q"))
	const where = `m.edificio_id=$1 AND ($2='' OR m.estado=$2) AND ($3='' OR m.texto ILIKE '%'||$3||'%' OR m.chat_id ILIKE '%'||$3||'%' OR d.nombre ILIKE '%'||$3||'%')`
	var total int64
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM telegram_mensaje m LEFT JOIN telegram_destino d ON d.id=m.destino_id WHERE `+where, e.ID, q.Get("estado"), buscar).Scan(&total); err != nil {
		P.Fallo(w, r, err)
		return
	}
	filas, err := db.Filas(ctx, s.DB, `SELECT m.id, m.chat_id, COALESCE(d.nombre,'') AS destino, m.texto, m.estado, m.origen, m.referencia, m.intentos, m.error,
			m.creado_en, m.procesado_en, us.nombre AS enviado_por
		FROM telegram_mensaje m LEFT JOIN telegram_destino d ON d.id=m.destino_id LEFT JOIN usuario us ON us.id=m.enviado_por
		WHERE `+where+` ORDER BY m.id DESC LIMIT $4 OFFSET $5`, e.ID, q.Get("estado"), buscar, por, (pagina-1)*por)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	conteos, _ := db.Filas(ctx, s.DB, `SELECT estado, count(*) AS cantidad FROM telegram_mensaje WHERE edificio_id=$1 GROUP BY estado ORDER BY estado`, e.ID)
	resp := paginado(filas, total, pagina)
	resp["conteos"] = conteos
	resp["modo"] = s.modoTelegram()
	P.JSON(w, http.StatusOK, resp)
}

// reintentarTelegram: POST /telegram/mensajes/{id}/reintentar. Solo un mensaje en «error» vuelve a la
// cola: lo enviado o simulado no se repite (no hay doble envío).
func (s *Server) reintentarTelegram(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	ct, err := s.DB.Exec(ctx, `UPDATE telegram_mensaje SET estado='pendiente', intentos=0, error='' WHERE id=$1 AND edificio_id=$2 AND estado='error'`, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		var estado string
		if err := s.DB.QueryRow(ctx, `SELECT estado FROM telegram_mensaje WHERE id=$1 AND edificio_id=$2`, id, e.ID).Scan(&estado); err != nil {
			P.Fallo(w, r, P.NoEncontrado("el mensaje"))
			return
		}
		P.Fallo(w, r, P.Conflicto("NO_REINTENTABLE", "Solo se reintenta un mensaje con error; este está «"+estado+"»."))
		return
	}
	s.despacharTelegram(ctx)
	var estado string
	_ = s.DB.QueryRow(ctx, `SELECT estado FROM telegram_mensaje WHERE id=$1`, id).Scan(&estado)
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": estado})
}
