package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
	"edisys/api/internal/whatsapp"
)

// configWA es la configuración efectiva de un edificio.
type configWA struct {
	Modo, URL, Instancia, APIKey string
}

// configEfectiva: la fila del edificio o, si no hay, el entorno. El interruptor maestro es WHATSAPP_MODO
// del servidor: si no dice «evolution», NADA sale aunque el edificio diga evolution.
func (s *Server) configEfectiva(ctx context.Context, q db.Q, eid int64) configWA {
	c := configWA{Modo: "simulado", URL: s.Cfg.EvolutionURL, Instancia: s.Cfg.EvolutionInst, APIKey: s.Cfg.EvolutionKey}
	var modo, url, inst, key string
	if err := q.QueryRow(ctx, `SELECT modo, url, instancia, apikey FROM whatsapp_config WHERE edificio_id=$1`, eid).Scan(&modo, &url, &inst, &key); err == nil {
		c.Modo = modo
		if url != "" {
			c.URL = url
		}
		if inst != "" {
			c.Instancia = inst
		}
		if key != "" {
			c.APIKey = key
		}
	} else if s.Cfg.WhatsAppModo == "evolution" {
		c.Modo = "evolution"
	}
	return c
}

func (s *Server) envioReal(c configWA) bool {
	return s.Cfg.WhatsAppModo == "evolution" && c.Modo == "evolution"
}

// encolar registra un mensaje saliente «pendiente» en la bandeja (outbox).
func (s *Server) encolar(ctx context.Context, q db.Q, eid int64, unidad *int64, telefono, plantilla string, vars map[string]string, origen string, usuario *int64) (int64, error) {
	texto, faltan := whatsapp.Renderizar(plantilla, vars)
	if _, ok := whatsapp.Plantillas[plantilla]; !ok {
		return 0, P.Validacion("Plantilla desconocida: "+plantilla).Campo("plantilla", "Usa GET /whatsapp/plantillas.")
	}
	if len(faltan) > 0 {
		return 0, P.Err(http.StatusUnprocessableEntity, "VARIABLES_FALTANTES", "Faltan variables de la plantilla: "+strings.Join(faltan, ", ")).Con("faltan", faltan)
	}
	tel := whatsapp.NormalizarTelefono(telefono)
	if len(tel) < 9 {
		return 0, P.Validacion("Teléfono inválido.").Campo("telefono", "Celular con código de país, p. ej. 51987654321.")
	}
	vb, _ := json.Marshal(vars)
	var id int64
	err := q.QueryRow(ctx, `INSERT INTO whatsapp_mensaje (edificio_id, unidad_id, telefono, plantilla, variables, texto, origen, enviado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, eid, unidad, tel, plantilla, vb, texto, origen, usuario).Scan(&id)
	return id, err
}

// despacharPendientes procesa la bandeja: en simulado marca «simulado»; en evolution envía.
func (s *Server) despacharPendientes(ctx context.Context) {
	filas, err := s.DB.Query(ctx, `SELECT id, edificio_id, telefono, texto FROM whatsapp_mensaje WHERE estado='pendiente' AND direccion='saliente' ORDER BY id LIMIT 100`)
	if err != nil {
		slog.Warn("whatsapp: leer bandeja", "err", err)
		return
	}
	type msg struct {
		id, eid     int64
		tel, texto string
	}
	var ms []msg
	for filas.Next() {
		var m msg
		if err := filas.Scan(&m.id, &m.eid, &m.tel, &m.texto); err == nil {
			ms = append(ms, m)
		}
	}
	filas.Close()
	cfgs := map[int64]configWA{}
	for _, m := range ms {
		c, ok := cfgs[m.eid]
		if !ok {
			c = s.configEfectiva(ctx, s.DB, m.eid)
			cfgs[m.eid] = c
		}
		// Reclamo atómico: si otro proceso ya lo tomó, se salta.
		tag, err := s.DB.Exec(ctx, `UPDATE whatsapp_mensaje SET intentos=intentos+1 WHERE id=$1 AND estado='pendiente'`, m.id)
		if err != nil || tag.RowsAffected() == 0 {
			continue
		}
		if !s.envioReal(c) {
			_, _ = s.DB.Exec(ctx, `UPDATE whatsapp_mensaje SET estado='simulado', procesado_en=now() WHERE id=$1`, m.id)
			continue
		}
		ev := whatsapp.Evolution{URL: c.URL, Instancia: c.Instancia, APIKey: c.APIKey, HTTP: s.HTTP}
		pid, err := ev.Enviar(ctx, m.tel, m.texto)
		if err != nil {
			_, _ = s.DB.Exec(ctx, `UPDATE whatsapp_mensaje SET estado='error', error=$2, procesado_en=now() WHERE id=$1`, m.id, err.Error())
			continue
		}
		_, _ = s.DB.Exec(ctx, `UPDATE whatsapp_mensaje SET estado='enviado', proveedor_id=$2, procesado_en=now() WHERE id=$1`, m.id, pid)
	}
}

const sqlMensaje = `SELECT m.id, m.direccion, m.unidad_id, u.codigo AS unidad, m.telefono, m.plantilla, m.variables, m.texto, m.estado, m.origen,
	m.intentos, m.error, m.intencion, m.creado_en, m.procesado_en, us.nombre AS enviado_por
	FROM whatsapp_mensaje m LEFT JOIN unidad u ON u.id=m.unidad_id LEFT JOIN usuario us ON us.id=m.enviado_por`

// listarMensajes: GET /whatsapp/mensajes?estado=&q=&direccion=&pagina=
func (s *Server) listarMensajes(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	q := r.URL.Query()
	pagina, por := paginacion(r)
	cond := []string{"m.edificio_id=$1"}
	args := []any{e.ID}
	if v := q.Get("estado"); v != "" {
		args = append(args, strings.Split(v, ","))
		cond = append(cond, "m.estado = ANY($"+strconv.Itoa(len(args))+")")
	}
	if v := q.Get("direccion"); v != "" {
		args = append(args, v)
		cond = append(cond, "m.direccion = $"+strconv.Itoa(len(args)))
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		args = append(args, "%"+v+"%")
		n := strconv.Itoa(len(args))
		cond = append(cond, "(m.texto ILIKE $"+n+" OR m.telefono ILIKE $"+n+" OR u.codigo ILIKE $"+n+" OR m.plantilla ILIKE $"+n+")")
	}
	where := strings.Join(cond, " AND ")
	ctx := r.Context()
	var total int64
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM whatsapp_mensaje m LEFT JOIN unidad u ON u.id=m.unidad_id WHERE `+where, args...).Scan(&total); err != nil {
		P.Fallo(w, r, err)
		return
	}
	args = append(args, por, (pagina-1)*por)
	filas, err := db.Filas(ctx, s.DB, sqlMensaje+` WHERE `+where+fmt.Sprintf(` ORDER BY m.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var conteo []map[string]any
	conteo, _ = db.Filas(ctx, s.DB, `SELECT estado, count(*) AS cantidad FROM whatsapp_mensaje WHERE edificio_id=$1 GROUP BY estado ORDER BY estado`, e.ID)
	resp := paginado(filas, total, pagina)
	resp["conteos"] = conteo
	resp["modo"] = s.modoVisible(ctx, e.ID)
	P.JSON(w, http.StatusOK, resp)
}

func (s *Server) modoVisible(ctx context.Context, eid int64) string {
	if s.envioReal(s.configEfectiva(ctx, s.DB, eid)) {
		return "evolution"
	}
	return "simulado"
}

// datosUnidadWA: nombre y celular del propietario (o inquilino si no hay) de una unidad.
func (s *Server) datosUnidadWA(ctx context.Context, eid, uid int64) (codigo, nombre, celular string, err error) {
	err = s.DB.QueryRow(ctx, `SELECT u.codigo, COALESCE(pe.nombre,''), COALESCE(pe.celular,'') FROM unidad u
		LEFT JOIN LATERAL (SELECT pe.nombre, pe.celular FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id
			WHERE up.unidad_id=u.id AND up.hasta IS NULL ORDER BY (up.rol='propietario') DESC LIMIT 1) pe ON true
		WHERE u.id=$1 AND u.edificio_id=$2`, uid, eid).Scan(&codigo, &nombre, &celular)
	if errors.Is(err, pgx.ErrNoRows) {
		err = P.NoEncontrado("la unidad")
	}
	return
}

// enviarWhatsApp: POST /whatsapp/enviar {unidad_id?, telefono?, plantilla, variables} → 201 mensaje.
func (s *Server) enviarWhatsApp(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UnidadID  *int64            `json:"unidad_id"`
		Telefono  string            `json:"telefono"`
		Plantilla string            `json:"plantilla"`
		Variables map[string]string `json:"variables"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	if in.Plantilla == "" {
		in.Plantilla = "libre"
	}
	if in.Variables == nil {
		in.Variables = map[string]string{}
	}
	if in.UnidadID != nil {
		cod, nombre, cel, err := s.datosUnidadWA(ctx, e.ID, *in.UnidadID)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		if in.Telefono == "" {
			in.Telefono = cel
		}
		if _, ok := in.Variables["nombre"]; !ok && nombre != "" {
			in.Variables["nombre"] = strings.Split(nombre, " ")[0]
		}
		if _, ok := in.Variables["unidad"]; !ok {
			in.Variables["unidad"] = cod
		}
	}
	if _, ok := in.Variables["yape"]; !ok {
		var yape string
		_ = s.DB.QueryRow(ctx, `SELECT yape_numero FROM edificio WHERE id=$1`, e.ID).Scan(&yape)
		if yape != "" {
			in.Variables["yape"] = yape
		}
	}
	if in.Telefono == "" {
		P.Fallo(w, r, P.Validacion("Indica el teléfono o una unidad con celular registrado.").Campo("telefono", "Obligatorio."))
		return
	}
	uid := ses(r).UsuarioID
	id, err := s.encolar(ctx, s.DB, e.ID, in.UnidadID, in.Telefono, in.Plantilla, in.Variables, "manual", &uid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.despacharPendientes(ctx)
	f, err := db.Fila(ctx, s.DB, sqlMensaje+` WHERE m.id=$1`, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, f)
}

// encolarRecibos envía por WhatsApp los recibos emitidos del periodo (o los ids dados) a cada propietario.
func (s *Server) encolarRecibos(ctx context.Context, eid, uid int64, periodo string, ids []int64) (map[string]any, error) {
	if periodo == "" && len(ids) == 0 {
		return nil, P.Validacion("Indica el periodo o los recibos.").Campo("recibo_ids", "Obligatorio.")
	}
	filas, err := s.DB.Query(ctx, `SELECT r.id, r.unidad_id, u.codigo, p.periodo, r.total_cts, r.total_cts - r.pagado_cts, r.estado, to_char(r.vence,'DD/MM/YYYY'),
			COALESCE(pe.nombre,''), COALESCE(pe.celular,'')
		FROM recibo r JOIN unidad u ON u.id=r.unidad_id JOIN periodo p ON p.id=r.periodo_id
		LEFT JOIN LATERAL (SELECT pe.nombre, pe.celular FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id
			WHERE up.unidad_id=u.id AND up.rol='propietario' AND up.hasta IS NULL LIMIT 1) pe ON true
		WHERE r.edificio_id=$1 AND r.estado NOT IN ('borrador','anulado') AND (($2 <> '' AND p.periodo=$2) OR r.id = ANY($3))
		ORDER BY u.codigo`, eid, periodo, ids)
	if err != nil {
		return nil, err
	}
	type rec struct {
		id, unidad, total, saldo           int64
		codigo, per, estado, vence, nombre string
		cel                                string
	}
	var rs []rec
	for filas.Next() {
		var x rec
		if err := filas.Scan(&x.id, &x.unidad, &x.codigo, &x.per, &x.total, &x.saldo, &x.estado, &x.vence, &x.nombre, &x.cel); err != nil {
			filas.Close()
			return nil, err
		}
		rs = append(rs, x)
	}
	filas.Close()
	if len(rs) == 0 {
		return nil, P.NoEncontrado("recibos emitidos para enviar")
	}
	var mensajes []int64
	sinTel := []string{}
	for _, x := range rs {
		if x.cel == "" {
			sinTel = append(sinTel, x.codigo)
			continue
		}
		estado := "Saldo pendiente: " + P.Soles(x.saldo) + "."
		if x.saldo == 0 {
			estado = "Ya está pagado, ¡gracias!"
		}
		vars := map[string]string{"nombre": strings.Split(x.nombre, " ")[0], "periodo": P.NombrePeriodo(x.per), "unidad": x.codigo,
			"total": P.Soles(x.total), "vence": x.vence, "estado": estado, "enlace": fmt.Sprintf("%s/app/e/%d/recibos/%d", s.Cfg.URLPublica, eid, x.id)}
		unidad := x.unidad
		id, err := s.encolar(ctx, s.DB, eid, &unidad, x.cel, "recibo", vars, "recibo", &uid)
		if err != nil {
			return nil, err
		}
		mensajes = append(mensajes, id)
		_, _ = s.DB.Exec(ctx, `UPDATE recibo SET enviado_en=now() WHERE id=$1`, x.id)
	}
	s.despacharPendientes(ctx)
	conteo := map[string]int{"simulado": 0, "enviado": 0, "error": 0, "pendiente": 0}
	fe, err := s.DB.Query(ctx, `SELECT estado, count(*) FROM whatsapp_mensaje WHERE id = ANY($1) GROUP BY estado`, mensajes)
	if err == nil {
		for fe.Next() {
			var es string
			var n int
			_ = fe.Scan(&es, &n)
			conteo[es] = n
		}
		fe.Close()
	}
	return map[string]any{"periodo": periodo, "encolados": len(mensajes), "simulados": conteo["simulado"], "enviados": conteo["enviado"],
		"errores": conteo["error"], "pendientes": conteo["pendiente"], "sin_telefono": sinTel, "mensaje_ids": mensajes, "modo": s.modoVisible(ctx, eid)}, nil
}

// enviarRecibosWhatsApp: POST /whatsapp/recibos/{periodo}/enviar.
func (s *Server) enviarRecibosWhatsApp(w http.ResponseWriter, r *http.Request) {
	periodo := chi.URLParam(r, "periodo")
	if !P.PeriodoValido(periodo) {
		P.Fallo(w, r, P.Validacion("Periodo AAAA-MM.").Campo("periodo", "Formato AAAA-MM."))
		return
	}
	res, err := s.encolarRecibos(r.Context(), edf(r).ID, ses(r).UsuarioID, periodo, nil)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusAccepted, res)
}

// verConfigWhatsApp: GET /whatsapp/config → {modo, url, instancia, tiene_apikey, …}. La clave nunca se devuelve.
func (s *Server) verConfigWhatsApp(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	c := s.configEfectiva(r.Context(), s.DB, e.ID)
	P.JSON(w, http.StatusOK, map[string]any{"modo": c.Modo, "url": c.URL, "instancia": c.Instancia, "tiene_apikey": c.APIKey != "",
		"modo_servidor": s.Cfg.WhatsAppModo, "envio_real": s.envioReal(c),
		"webhook_url": s.Cfg.URLPublica + "/api/v1/whatsapp/webhook", "webhook_protegido": s.Cfg.WebhookToken != ""})
}

// guardarConfigWhatsApp: PUT /whatsapp/config {modo, url, instancia, apikey?}.
func (s *Server) guardarConfigWhatsApp(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Modo      string  `json:"modo"`
		URL       string  `json:"url"`
		Instancia string  `json:"instancia"`
		APIKey    *string `json:"apikey"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.Modo != "simulado" && in.Modo != "evolution" {
		P.Fallo(w, r, P.Validacion("Modo inválido.").Campo("modo", "simulado o evolution."))
		return
	}
	if in.Modo == "evolution" && (in.URL == "" || in.Instancia == "") {
		P.Fallo(w, r, P.Validacion("Para el modo evolution indica la URL y la instancia.").Campo("url", "Obligatoria.").Campo("instancia", "Obligatoria."))
		return
	}
	e := edf(r)
	ctx := r.Context()
	key := ""
	cambiaKey := in.APIKey != nil && *in.APIKey != ""
	if cambiaKey {
		key = *in.APIKey
	}
	if _, err := s.DB.Exec(ctx, `INSERT INTO whatsapp_config (edificio_id, modo, url, instancia, apikey, actualizado_por) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (edificio_id) DO UPDATE SET modo=EXCLUDED.modo, url=EXCLUDED.url, instancia=EXCLUDED.instancia,
		  apikey = CASE WHEN $7 THEN EXCLUDED.apikey ELSE whatsapp_config.apikey END, actualizado_por=EXCLUDED.actualizado_por, actualizado_en=now()`,
		e.ID, in.Modo, strings.TrimRight(in.URL, "/"), in.Instancia, key, ses(r).UsuarioID, cambiaKey); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "whatsapp", "config", "whatsapp_config", e.ID, nil, map[string]any{"modo": in.Modo, "url": in.URL, "instancia": in.Instancia, "apikey_cambiada": cambiaKey})
	s.verConfigWhatsApp(w, r)
}

func (s *Server) listarPlantillas(w http.ResponseWriter, r *http.Request) {
	nombres := make([]string, 0, len(whatsapp.Plantillas))
	for k := range whatsapp.Plantillas {
		nombres = append(nombres, k)
	}
	sort.Strings(nombres)
	out := []map[string]any{}
	for _, n := range nombres {
		out = append(out, map[string]any{"codigo": n, "texto": whatsapp.Plantillas[n], "variables": whatsapp.Variables(n)})
	}
	P.JSON(w, http.StatusOK, map[string]any{"plantillas": out})
}

// webhookWhatsApp: POST /whatsapp/webhook — mensaje entrante de Evolution (messages.upsert).
// Identifica al propietario por teléfono, registra el entrante y responde por la bandeja.
func (s *Server) webhookWhatsApp(w http.ResponseWriter, r *http.Request) {
	if s.Cfg.WebhookToken != "" {
		tok := r.URL.Query().Get("token")
		if tok == "" {
			tok = r.Header.Get("X-Webhook-Token")
		}
		if tok != s.Cfg.WebhookToken {
			P.Fallo(w, r, P.Err(http.StatusUnauthorized, "TOKEN_WEBHOOK", "Token del webhook inválido."))
			return
		}
	}
	cuerpo, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		P.Fallo(w, r, P.Validacion("No pude leer el cuerpo."))
		return
	}
	ent, ok := whatsapp.LeerWebhook(cuerpo)
	if !ok || ent.DeMi {
		P.JSON(w, http.StatusOK, map[string]any{"ok": true, "ignorado": true})
		return
	}
	ctx := r.Context()
	res, err := s.Responder(ctx, 0, ent.Telefono, ent.Texto)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if res.EdificioID == 0 {
		// Número desconocido: se registra en el primer edificio para que la administración lo vea.
		_ = s.DB.QueryRow(ctx, `SELECT min(id) FROM edificio`).Scan(&res.EdificioID)
	}
	if res.EdificioID == 0 {
		P.JSON(w, http.StatusOK, map[string]any{"ok": true, "ignorado": true})
		return
	}
	vb, _ := json.Marshal(map[string]string{"nombre": ent.Nombre})
	if _, err := s.DB.Exec(ctx, `INSERT INTO whatsapp_mensaje (edificio_id, direccion, unidad_id, telefono, plantilla, variables, texto, estado, origen, intencion, procesado_en)
		VALUES ($1,'entrante',$2,$3,'entrante',$4,$5,'recibido','webhook',$6, now())`, res.EdificioID, res.UnidadID, ent.Telefono, vb, ent.Texto, res.Intencion); err != nil {
		P.Fallo(w, r, err)
		return
	}
	mid, err := s.encolar(ctx, s.DB, res.EdificioID, res.UnidadID, ent.Telefono, "chatbot", map[string]string{"texto": res.Respuesta}, "chatbot", nil)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	_, _ = s.DB.Exec(ctx, `UPDATE whatsapp_mensaje SET intencion=$2 WHERE id=$1`, mid, res.Intencion)
	s.despacharPendientes(ctx)
	P.JSON(w, http.StatusOK, map[string]any{"ok": true, "intencion": res.Intencion, "respuesta": res.Respuesta, "mensaje_id": mid})
}

// chatbotMensaje: POST /chatbot/mensaje {telefono, texto} → {respuesta, intencion, datos}. No envía nada por WhatsApp.
func (s *Server) chatbotMensaje(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Telefono string `json:"telefono"`
		Texto    string `json:"texto"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if strings.TrimSpace(in.Telefono) == "" {
		P.Fallo(w, r, P.Validacion("Indica el teléfono que escribe.").Campo("telefono", "Obligatorio."))
		return
	}
	res, err := s.Responder(r.Context(), edf(r).ID, whatsapp.NormalizarTelefono(in.Telefono), in.Texto)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"respuesta": res.Respuesta, "intencion": res.Intencion, "datos": res.Datos})
}

// procesoBandeja reintenta cada 30 s lo que haya quedado pendiente.
func (s *Server) procesoBandeja(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.despacharPendientes(ctx)
		}
	}
}
