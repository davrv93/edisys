package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// G2 · Tickets con SLA y semáforo. Las incidencias del tablero son los tickets: la base les pone
// objetivo (horas) y vencimiento según su criticidad (trigger incidencia_sla, 0027); aquí se pinta
// el semáforo y sale la respuesta automática al solicitante.

// Semáforos del SLA. «ambar» es cuando queda menos de la cuarta parte del plazo.
const (
	SemaforoVerde   = "verde"
	SemaforoAmbar   = "ambar"
	SemaforoRojo    = "rojo"
	SemaforoCerrado = "cerrado"
)

// estadosCerrados: el ticket ya no corre contra el reloj.
var estadosCerrados = map[string]bool{"terminado": true, "descartado": true, "rechazado": true}

// SemaforoSLA calcula el color de un ticket. cerradoEn != nil = ticket cerrado: el color es «cerrado»
// y cumplido dice si se cerró dentro del plazo. Abierto: rojo si venció, ámbar si queda < 25 %.
func SemaforoSLA(objetivoHoras int, vence, ahora time.Time, cerradoEn *time.Time) (color string, cumplido bool) {
	if cerradoEn != nil {
		return SemaforoCerrado, !cerradoEn.After(vence)
	}
	restante := vence.Sub(ahora)
	if restante < 0 {
		return SemaforoRojo, false
	}
	if objetivoHoras > 0 && restante*4 < time.Duration(objetivoHoras)*time.Hour {
		return SemaforoAmbar, true
	}
	return SemaforoVerde, true
}

// decorarSLA añade semáforo, cumplido y horas restantes a una fila de incidencia (sqlIncidencia).
func decorarSLA(f map[string]any, ahora time.Time) {
	vence, ok := f["sla_vencimiento"].(time.Time)
	if !ok {
		f["semaforo"] = nil
		return
	}
	obj := 0
	switch v := f["sla_objetivo"].(type) {
	case int32:
		obj = int(v)
	case int64:
		obj = int(v)
	}
	var cerrado *time.Time
	if estado, _ := f["estado"].(string); estadosCerrados[estado] {
		t, ok := f["terminado_en"].(time.Time)
		if !ok {
			t, _ = f["actualizado_en"].(time.Time)
		}
		cerrado = &t
	}
	color, cumplido := SemaforoSLA(obj, vence, ahora, cerrado)
	f["semaforo"] = color
	f["sla_cumplido"] = cumplido
	f["sla_restante_min"] = int64(vence.Sub(ahora) / time.Minute)
}

// listarTickets: GET /tickets?semaforo=&abiertos=1 (+ los filtros del tablero) → tickets ordenados por
// vencimiento, con conteo por color.
func (s *Server) listarTickets(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	v := r.URL.Query()
	where, args := condIncidencias(e, v, ses(r).UsuarioID)
	if v.Get("abiertos") != "0" {
		where += ` AND i.estado NOT IN ('terminado','descartado','rechazado')`
	}
	filas, err := db.Filas(r.Context(), s.DB, sqlIncidencia+` WHERE `+where+` ORDER BY i.sla_vencimiento NULLS LAST, i.numero LIMIT 500`, args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	ahora := time.Now()
	filtro := v.Get("semaforo")
	conteos := map[string]int{SemaforoVerde: 0, SemaforoAmbar: 0, SemaforoRojo: 0, SemaforoCerrado: 0}
	out := make([]map[string]any, 0, len(filas))
	for _, f := range filas {
		decorarSLA(f, ahora)
		c, _ := f["semaforo"].(string)
		conteos[c]++
		if filtro != "" && c != filtro {
			continue
		}
		delete(f, "foto_id")
		out = append(out, f)
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": out, "total": len(out), "pagina": 1, "conteos": conteos})
}

// configSLA es la fila del edificio (o la de fábrica).
type configSLA struct {
	HorasCritica        int    `json:"horas_critica"`
	HorasMedia          int    `json:"horas_media"`
	HorasBaja           int    `json:"horas_baja"`
	HorasSinClasificar  int    `json:"horas_sin_clasificar"`
	RespuestaAutomatica bool   `json:"respuesta_automatica"`
	Mensaje             string `json:"mensaje"`
}

const mensajeSLADefecto = "Hola {{nombre}}, recibimos tu reporte {{codigo}} ({{titulo}}). Lo atenderemos en un plazo de {{plazo}}, hasta el {{vence}}. Te avisaremos cada avance."

func (s *Server) leerConfigSLA(ctx context.Context, q db.Q, eid int64) configSLA {
	c := configSLA{24, 72, 168, 72, true, mensajeSLADefecto}
	_ = q.QueryRow(ctx, `SELECT horas_critica, horas_media, horas_baja, horas_sin_clasificar, respuesta_automatica, mensaje
		FROM ticket_sla_config WHERE edificio_id=$1`, eid).Scan(&c.HorasCritica, &c.HorasMedia, &c.HorasBaja, &c.HorasSinClasificar, &c.RespuestaAutomatica, &c.Mensaje)
	return c
}

// verConfigSLA: GET /tickets/sla
func (s *Server) verConfigSLA(w http.ResponseWriter, r *http.Request) {
	c := s.leerConfigSLA(r.Context(), s.DB, edf(r).ID)
	P.JSON(w, http.StatusOK, map[string]any{"config": c, "variables": []string{"nombre", "codigo", "titulo", "plazo", "vence"}})
}

// guardarConfigSLA: PUT /tickets/sla {horas_*, respuesta_automatica, mensaje, recalcular_abiertos?}
// Con recalcular_abiertos, los tickets abiertos toman el plazo nuevo (desde que se reportaron).
func (s *Server) guardarConfigSLA(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	var in struct {
		configSLA
		Recalcular bool `json:"recalcular_abiertos"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa los plazos.")
	for campo, h := range map[string]int{"horas_critica": in.HorasCritica, "horas_media": in.HorasMedia, "horas_baja": in.HorasBaja, "horas_sin_clasificar": in.HorasSinClasificar} {
		if h <= 0 || h > 24*90 {
			ev.Campo(campo, "Entre 1 y 2160 horas.")
		}
	}
	in.Mensaje = strings.TrimSpace(in.Mensaje)
	if in.Mensaje == "" {
		in.Mensaje = mensajeSLADefecto
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO ticket_sla_config (edificio_id, horas_critica, horas_media, horas_baja, horas_sin_clasificar, respuesta_automatica, mensaje)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (edificio_id) DO UPDATE SET horas_critica=EXCLUDED.horas_critica, horas_media=EXCLUDED.horas_media, horas_baja=EXCLUDED.horas_baja,
			horas_sin_clasificar=EXCLUDED.horas_sin_clasificar, respuesta_automatica=EXCLUDED.respuesta_automatica, mensaje=EXCLUDED.mensaje, actualizado_en=now()`,
		e.ID, in.HorasCritica, in.HorasMedia, in.HorasBaja, in.HorasSinClasificar, in.RespuestaAutomatica, in.Mensaje); err != nil {
		P.Fallo(w, r, err)
		return
	}
	var recalculados int64
	if in.Recalcular {
		ct, err := tx.Exec(ctx, `UPDATE incidencia SET sla_objetivo=sla_horas(edificio_id, criticidad),
			sla_vencimiento=creado_en + make_interval(hours => sla_horas(edificio_id, criticidad))
			WHERE edificio_id=$1 AND estado NOT IN ('terminado','descartado','rechazado')`, e.ID)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		recalculados = ct.RowsAffected()
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"config": in.configSLA, "recalculados": recalculados})
}

// plazoTexto: 72 → «3 días», 30 → «30 horas», 24 → «1 día».
func plazoTexto(horas int) string {
	if horas >= 24 && horas%24 == 0 {
		if horas == 24 {
			return "1 día"
		}
		return fmt.Sprintf("%d días", horas/24)
	}
	if horas == 1 {
		return "1 hora"
	}
	return fmt.Sprintf("%d horas", horas)
}

// respuestaAutomaticaTicket encola el acuse al solicitante de un ticket recién creado (en la misma
// transacción). Solo para reportes desde la app: el chatbot ya contesta en la conversación y lo que
// registra la administración no tiene a quién avisar. Nunca hace fallar la creación del ticket.
func (s *Server) respuestaAutomaticaTicket(ctx context.Context, tx pgx.Tx, eid, incID int64, origen string) {
	if origen != "app" {
		return
	}
	cfg := s.leerConfigSLA(ctx, tx, eid)
	if !cfg.RespuestaAutomatica {
		return
	}
	var codigo, titulo, nombre, tel string
	var horas int
	var vence time.Time
	var unidad *int64
	if err := tx.QueryRow(ctx, `SELECT i.codigo, i.titulo, COALESCE(i.sla_objetivo,72), i.sla_vencimiento, i.unidad_id, COALESCE(u.nombre,''), COALESCE(u.telefono,'')
		FROM incidencia i LEFT JOIN usuario u ON u.id=i.reportado_por WHERE i.id=$1`, incID).Scan(&codigo, &titulo, &horas, &vence, &unidad, &nombre, &tel); err != nil {
		return
	}
	texto := strings.NewReplacer("{{nombre}}", primerNombre(nombre), "{{codigo}}", codigo, "{{titulo}}", titulo,
		"{{plazo}}", plazoTexto(horas), "{{vence}}", vence.In(P.Lima).Format("02/01/2006 15:04")).Replace(cfg.Mensaje)
	if id := s.avisoSistema(ctx, tx, eid, unidad, tel, texto); id > 0 {
		_, _ = tx.Exec(ctx, `UPDATE incidencia SET respuesta_auto_en=now() WHERE id=$1`, incID)
	}
}
