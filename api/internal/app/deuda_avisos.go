package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Avisos de cobranza (bloque D2). Un aviso automático tiene frecuencia, día y tramos (escalas) por deuda
// vencida o por número de recibos vencidos; cada tramo lleva su asunto y su cuerpo. La tarea diaria elige el
// tramo de cada unidad morosa y deja el mensaje en la bandeja de correo o de WhatsApp (las mismas de 0007 y
// 0010: en modo simulado no sale nada). Cada envío queda como evidencia en aviso_envio.
// Telegram queda para el bloque E4 (todavía no hay canal).

// Escala: un tramo del aviso. Hasta nil = sin tope.
type Escala struct {
	ID     int64  `json:"id,omitempty"`
	Desde  int64  `json:"desde"`
	Hasta  *int64 `json:"hasta"`
	Asunto string `json:"asunto"`
	Cuerpo string `json:"cuerpo"`
}

// ValidarEscalas ordena los tramos por «desde» y exige que no se crucen: cada uno empieza después de donde
// termina el anterior, y solo el último puede quedar sin tope.
func ValidarEscalas(es []Escala) ([]Escala, *P.Error) {
	ev := P.Validacion("Revisa las escalas del aviso.")
	if len(es) == 0 {
		return nil, ev.Campo("escalas", "Agrega al menos un tramo.")
	}
	out := append([]Escala(nil), es...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Desde < out[j].Desde })
	for i := range out {
		out[i].Asunto, out[i].Cuerpo = strings.TrimSpace(out[i].Asunto), strings.TrimSpace(out[i].Cuerpo)
		c := fmt.Sprintf("escalas.%d", i)
		switch {
		case out[i].Desde < 0:
			ev.Campo(c, "«Desde» no puede ser negativo.")
		case out[i].Hasta != nil && *out[i].Hasta < out[i].Desde:
			ev.Campo(c, "«Hasta» debe ser mayor o igual que «desde».")
		case out[i].Asunto == "" || out[i].Cuerpo == "":
			ev.Campo(c, "Escribe el asunto y el mensaje del tramo.")
		}
		if i > 0 {
			prev := out[i-1]
			if prev.Hasta == nil || *prev.Hasta >= out[i].Desde {
				ev.Campo(c, "Este tramo se cruza con el anterior.")
			}
		}
	}
	if len(ev.Campos) > 0 {
		return nil, ev
	}
	return out, nil
}

// ElegirEscala devuelve el tramo que contiene el valor (o nil si ninguno).
func ElegirEscala(es []Escala, valor int64) *Escala {
	for i := range es {
		if valor >= es[i].Desde && (es[i].Hasta == nil || valor <= *es[i].Hasta) {
			return &es[i]
		}
	}
	return nil
}

// TocaHoy: ¿el aviso corre hoy? diaria siempre; semanal el día ISO (1 lunes … 7 domingo); mensual el día del
// mes, y si el mes es más corto, su último día.
func TocaHoy(frecuencia string, dia int, hoy time.Time) bool {
	switch frecuencia {
	case "diaria":
		return true
	case "semanal":
		wd := int(hoy.Weekday())
		if wd == 0 {
			wd = 7
		}
		return wd == dia
	default:
		ultimo := time.Date(hoy.Year(), hoy.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		return hoy.Day() == dia || (dia > ultimo && hoy.Day() == ultimo)
	}
}

// RenderAviso reemplaza {{nombre}}, {{unidad}}, {{deuda}}, {{recibos}} y {{edificio}}.
func RenderAviso(t string, vars map[string]string) string {
	for k, v := range vars {
		t = strings.ReplaceAll(t, "{{"+k+"}}", v)
	}
	return t
}

// ---------- configuración ----------

type entradaAviso struct {
	Nombre     string   `json:"nombre"`
	Frecuencia string   `json:"frecuencia"`
	Dia        int      `json:"dia"`
	TipoEscala string   `json:"tipo_escala"`
	Canales    []string `json:"canales"`
	AdjuntaPDF bool     `json:"adjunta_pdf"`
	Activo     *bool    `json:"activo"`
	Escalas    []Escala `json:"escalas"`
}

func (in *entradaAviso) validar() ([]Escala, error) {
	in.Nombre = strings.TrimSpace(in.Nombre)
	ev := P.Validacion("Revisa el aviso.")
	if in.Nombre == "" {
		ev.Campo("nombre", "Obligatorio.")
	}
	if in.Frecuencia == "" {
		in.Frecuencia = "mensual"
	}
	if in.Frecuencia != "diaria" && in.Frecuencia != "semanal" && in.Frecuencia != "mensual" {
		ev.Campo("frecuencia", "diaria, semanal o mensual.")
	}
	if in.Dia == 0 {
		in.Dia = 1
	}
	if (in.Frecuencia == "semanal" && (in.Dia < 1 || in.Dia > 7)) || in.Dia < 1 || in.Dia > 31 {
		ev.Campo("dia", "Semanal: 1 (lunes) a 7 (domingo). Mensual: 1 a 31.")
	}
	if in.TipoEscala == "" {
		in.TipoEscala = "monto"
	}
	if in.TipoEscala != "monto" && in.TipoEscala != "recibos" {
		ev.Campo("tipo_escala", "monto o recibos.")
	}
	if err := validarCanales(in.Canales); err != "" {
		ev.Campo("canales", err)
	}
	if in.Activo == nil {
		t := true
		in.Activo = &t
	}
	if len(ev.Campos) > 0 {
		return nil, ev
	}
	es, e2 := ValidarEscalas(in.Escalas)
	if e2 != nil {
		return nil, e2
	}
	return es, nil
}

func validarCanales(cs []string) string {
	if len(cs) == 0 {
		return "Elige al menos un canal."
	}
	for _, c := range cs {
		if c != "correo" && c != "whatsapp" {
			return "Canales: correo o whatsapp (Telegram llega con su bloque)."
		}
	}
	return ""
}

func guardarEscalas(ctx context.Context, q db.Q, aid int64, es []Escala) error {
	if _, err := q.Exec(ctx, `DELETE FROM aviso_escala WHERE aviso_id=$1`, aid); err != nil {
		return err
	}
	for i, x := range es {
		if _, err := q.Exec(ctx, `INSERT INTO aviso_escala (aviso_id, desde, hasta, asunto, cuerpo, orden) VALUES ($1,$2,$3,$4,$5,$6)`,
			aid, x.Desde, x.Hasta, x.Asunto, x.Cuerpo, i); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) escalasDe(ctx context.Context, aid int64) ([]Escala, error) {
	f, err := s.DB.Query(ctx, `SELECT id, desde, hasta, asunto, cuerpo FROM aviso_escala WHERE aviso_id=$1 ORDER BY desde, orden`, aid)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []Escala{}
	for f.Next() {
		var x Escala
		if err := f.Scan(&x.ID, &x.Desde, &x.Hasta, &x.Asunto, &x.Cuerpo); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, f.Err()
}

// listarAvisos: GET /avisos-cobranza → avisos con sus escalas.
func (s *Server) listarAvisos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	filas, err := db.Filas(ctx, s.DB, `SELECT id, nombre, frecuencia, dia, tipo_escala, canales, adjunta_pdf, activo,
			to_char(ultima_ejecucion,'YYYY-MM-DD') AS ultima_ejecucion,
			(SELECT count(*) FROM aviso_envio v WHERE v.aviso_id=a.id) AS envios
		FROM aviso_cobranza a WHERE edificio_id=$1 ORDER BY activo DESC, id`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, f := range filas {
		es, err := s.escalasDe(ctx, f["id"].(int64))
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		f["escalas"] = es
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearAviso: POST /avisos-cobranza
func (s *Server) crearAviso(w http.ResponseWriter, r *http.Request) {
	var in entradaAviso
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	es, err := in.validar()
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO aviso_cobranza (edificio_id, nombre, frecuencia, dia, tipo_escala, canales, adjunta_pdf, activo, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, e.ID, in.Nombre, in.Frecuencia, in.Dia, in.TipoEscala, in.Canales, in.AdjuntaPDF, *in.Activo, ses(r).UsuarioID).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := guardarEscalas(ctx, tx, id, es); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// editarAviso: PUT /avisos-cobranza/{id} (reemplaza las escalas)
func (s *Server) editarAviso(w http.ResponseWriter, r *http.Request) {
	var in entradaAviso
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	es, err := in.validar()
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	ct, err := tx.Exec(ctx, `UPDATE aviso_cobranza SET nombre=$3, frecuencia=$4, dia=$5, tipo_escala=$6, canales=$7, adjunta_pdf=$8, activo=$9
		WHERE id=$1 AND edificio_id=$2`, id, e.ID, in.Nombre, in.Frecuencia, in.Dia, in.TipoEscala, in.Canales, in.AdjuntaPDF, *in.Activo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("el aviso"))
		return
	}
	if err := guardarEscalas(ctx, tx, id, es); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

// borrarAviso: DELETE /avisos-cobranza/{id} (la evidencia de envíos se conserva).
func (s *Server) borrarAviso(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	ct, err := s.DB.Exec(r.Context(), `DELETE FROM aviso_cobranza WHERE id=$1 AND edificio_id=$2`, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("el aviso"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

// listarEnviosAviso: GET /avisos-cobranza/envios?aviso_id=&pagina= → evidencia (50 por página).
func (s *Server) listarEnviosAviso(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	aid, _ := strconv.ParseInt(r.URL.Query().Get("aviso_id"), 10, 64)
	pag, _ := strconv.Atoi(r.URL.Query().Get("pagina"))
	if pag < 1 {
		pag = 1
	}
	filas, err := db.Filas(r.Context(), s.DB, `SELECT v.id, v.aviso_id, COALESCE(a.nombre,'Aviso manual') AS aviso, v.unidad_id, u.codigo AS unidad, v.canal,
			to_char(v.fecha,'YYYY-MM-DD') AS fecha, v.deuda_cts, v.recibos, v.asunto, v.mensaje_id, v.estado, v.detalle,
			CASE v.canal WHEN 'correo' THEN (SELECT m.estado FROM correo_mensaje m WHERE m.id=v.mensaje_id)
			             ELSE (SELECT m.estado FROM whatsapp_mensaje m WHERE m.id=v.mensaje_id) END AS estado_bandeja,
			v.creado_en, count(*) OVER () AS total
		FROM aviso_envio v JOIN unidad u ON u.id=v.unidad_id LEFT JOIN aviso_cobranza a ON a.id=v.aviso_id
		WHERE v.edificio_id=$1 AND ($2=0 OR v.aviso_id=$2)
		ORDER BY v.creado_en DESC, v.id DESC LIMIT 50 OFFSET $3`, e.ID, aid, (pag-1)*50)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	total := int64(0)
	if len(filas) > 0 {
		total = filas[0]["total"].(int64)
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": total, "pagina": pag})
}

// ---------- ejecución ----------

// deudorAviso: una unidad con deuda vencida y sus datos de contacto.
type deudorAviso struct {
	unidadID                    int64
	codigo, nombre, correo, cel string
	deuda                       int64
	recibos                     int
	recibosIDs                  []int64
}

// deudores: unidades del edificio con deuda vencida (la misma regla de la morosidad, con acuerdos y gracia).
// «recibos» cuenta los recibos vencidos fuera de acuerdos (lo vencido de un acuerdo cuenta como uno).
func (s *Server) deudores(ctx context.Context, eid int64, soloUnidades []int64) ([]deudorAviso, error) {
	filas, err := s.DB.Query(ctx, `SELECT u.id, u.codigo, COALESCE(pe.nombre,''), COALESCE(pe.correo,''), COALESCE(pe.celular,''), deuda_vencida_cts(u.id),
			COALESCE((SELECT array_agg(r.id ORDER BY r.vence) FROM recibo r JOIN edificio e ON e.id=r.edificio_id
				WHERE r.unidad_id=u.id AND r.estado IN ('emitido','pagado_parcial') AND r.total_cts > r.pagado_cts
				  AND r.vence + e.dias_gracia < (now() AT TIME ZONE 'America/Lima')::date
				  AND NOT EXISTS (SELECT 1 FROM acuerdo_recibo ar JOIN acuerdo_pago a ON a.id=ar.acuerdo_id WHERE ar.recibo_id=r.id AND ar.activo AND a.estado='activo')), '{}'),
			EXISTS (SELECT 1 FROM acuerdo_pago a WHERE a.unidad_id=u.id AND a.estado='activo' AND acuerdo_vencido_cts(a.id) > 0)
		FROM unidad u
		LEFT JOIN LATERAL (SELECT pe.nombre, pe.correo, pe.celular FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id
			WHERE up.unidad_id=u.id AND up.hasta IS NULL ORDER BY (up.rol='propietario') DESC LIMIT 1) pe ON true
		WHERE u.edificio_id=$1 AND ($2::bigint[] IS NULL OR u.id = ANY($2)) AND deuda_vencida_cts(u.id) > 0
		ORDER BY u.codigo`, eid, soloUnidades)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	out := []deudorAviso{}
	for filas.Next() {
		var d deudorAviso
		var acuerdoVencido bool
		if err := filas.Scan(&d.unidadID, &d.codigo, &d.nombre, &d.correo, &d.cel, &d.deuda, &d.recibosIDs, &acuerdoVencido); err != nil {
			return nil, err
		}
		d.recibos = len(d.recibosIDs)
		if acuerdoVencido {
			d.recibos++
		}
		out = append(out, d)
	}
	return out, filas.Err()
}

// plantillaAviso: lo que se envía a una unidad (ya con el tramo elegido).
type plantillaAviso struct {
	avisoID, escalaID *int64
	asunto, cuerpo    string
	canales           []string
	adjuntaPDF        bool
}

// enviarAviso deja el aviso de una unidad en la bandeja de cada canal y registra la evidencia, en una
// transacción por canal. En el automático, el índice único (aviso, unidad, canal, día) impide repetir.
// Devuelve cuántos quedaron encolados.
func (s *Server) enviarAviso(ctx context.Context, eid int64, edificio string, d deudorAviso, pl plantillaAviso, usuario *int64, hoy time.Time) (int, error) {
	vars := map[string]string{"nombre": primerNombre(d.nombre), "unidad": d.codigo, "deuda": P.Soles(d.deuda),
		"recibos": strconv.Itoa(d.recibos), "edificio": edificio}
	asunto, cuerpo := RenderAviso(pl.asunto, vars), RenderAviso(pl.cuerpo, vars)
	encolados := 0
	for _, canal := range pl.canales {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			return encolados, err
		}
		var vid int64
		err = tx.QueryRow(ctx, `INSERT INTO aviso_envio (edificio_id, aviso_id, escala_id, unidad_id, canal, fecha, deuda_cts, recibos, asunto, estado, enviado_por)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'encolado',$10) ON CONFLICT DO NOTHING RETURNING id`,
			eid, pl.avisoID, pl.escalaID, d.unidadID, canal, hoy, d.deuda, d.recibos, asunto, usuario).Scan(&vid)
		if errors.Is(err, pgx.ErrNoRows) { // ya se avisó hoy por este canal: no se repite
			tx.Rollback(ctx)
			continue
		}
		if err != nil {
			tx.Rollback(ctx)
			return encolados, err
		}
		var mid int64
		estado, detalle := "encolado", ""
		uid := d.unidadID
		switch canal {
		case "correo":
			if d.correo == "" {
				estado, detalle = "sin_contacto", "La unidad no tiene correo registrado."
				break
			}
			html, texto := armarCorreo(datosCorreo{Edificio: edificio, Nombre: primerNombre(d.nombre), Intro: cuerpo,
				Filas:  []filaCorreo{{"Unidad", "Dpto " + d.codigo}, {"Deuda vencida", P.Soles(d.deuda)}, {"Recibos vencidos", strconv.Itoa(d.recibos)}},
				Cierre: "Si ya pagaste, no tomes en cuenta este mensaje."})
			var adj []adjuntoCola
			if pl.adjuntaPDF {
				adj = s.pdfsVencidos(ctx, eid, d.recibosIDs)
			}
			mid, err = s.encolarCorreo(ctx, tx, eid, &uid, d.correo, d.nombre, asunto, html, texto, "sistema", fmt.Sprintf("aviso:%d", vid), adj, usuario)
		case "whatsapp":
			if d.cel == "" {
				estado, detalle = "sin_contacto", "La unidad no tiene celular registrado."
				break
			}
			mid, err = s.encolar(ctx, tx, eid, &uid, d.cel, "libre", map[string]string{"texto": asunto + "\n\n" + cuerpo}, "sistema", usuario)
		}
		if err != nil {
			estado, detalle, mid = "error", recortar(err.Error(), 300), 0
		}
		var midPtr *int64
		if mid > 0 {
			midPtr = &mid
		}
		if _, err := tx.Exec(ctx, `UPDATE aviso_envio SET estado=$2, detalle=$3, mensaje_id=$4 WHERE id=$1`, vid, estado, detalle, midPtr); err != nil {
			tx.Rollback(ctx)
			return encolados, err
		}
		if err := tx.Commit(ctx); err != nil {
			return encolados, err
		}
		if estado == "encolado" {
			encolados++
		}
	}
	return encolados, nil
}

// pdfsVencidos: el PDF de hasta tres recibos vencidos (los más antiguos) para adjuntar al correo.
func (s *Server) pdfsVencidos(ctx context.Context, eid int64, ids []int64) []adjuntoCola {
	out := []adjuntoCola{}
	e := &Edificio{ID: eid}
	for i, rid := range ids {
		if i >= 3 {
			break
		}
		rc, err := s.reciboVisible(ctx, e, rid)
		if err != nil {
			continue
		}
		b, err := s.reciboPDF(ctx, rc, rid)
		if err != nil {
			continue
		}
		out = append(out, adjuntoCola{nombre: "recibo-" + val(rc["numero"]) + ".pdf", tipo: "application/pdf", datos: b})
	}
	return out
}

// EjecutarAviso corre un aviso automático para todas las unidades morosas del edificio. Devuelve el resumen.
func (s *Server) EjecutarAviso(ctx context.Context, aid int64, usuario *int64, hoy time.Time) (map[string]any, error) {
	var eid int64
	var tipo, edificio string
	var canales []string
	var adjunta bool
	if err := s.DB.QueryRow(ctx, `SELECT a.edificio_id, a.tipo_escala, a.canales, a.adjunta_pdf, e.nombre FROM aviso_cobranza a JOIN edificio e ON e.id=a.edificio_id WHERE a.id=$1`, aid).
		Scan(&eid, &tipo, &canales, &adjunta, &edificio); err != nil {
		return nil, P.NoEncontrado("el aviso")
	}
	es, err := s.escalasDe(ctx, aid)
	if err != nil {
		return nil, err
	}
	ds, err := s.deudores(ctx, eid, nil)
	if err != nil {
		return nil, err
	}
	res := map[string]any{"aviso_id": aid, "deudores": len(ds), "encolados": 0, "sin_tramo": 0}
	enc, sin := 0, 0
	for _, d := range ds {
		valor := d.deuda
		if tipo == "recibos" {
			valor = int64(d.recibos)
		}
		x := ElegirEscala(es, valor)
		if x == nil {
			sin++
			continue
		}
		eidEsc := x.ID
		n, err := s.enviarAviso(ctx, eid, edificio, d, plantillaAviso{avisoID: &aid, escalaID: &eidEsc, asunto: x.Asunto, cuerpo: x.Cuerpo, canales: canales, adjuntaPDF: adjunta}, usuario, hoy)
		if err != nil {
			return nil, err
		}
		enc += n
	}
	res["encolados"], res["sin_tramo"] = enc, sin
	return res, nil
}

// ejecutarAvisoAhora: POST /avisos-cobranza/{id}/ejecutar → corre el aviso ya (sin esperar su día).
func (s *Server) ejecutarAvisoAhora(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	var ok bool
	if err := s.DB.QueryRow(ctx, `SELECT true FROM aviso_cobranza WHERE id=$1 AND edificio_id=$2`, id, e.ID).Scan(&ok); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el aviso"))
		return
	}
	uid := ses(r).UsuarioID
	res, err := s.EjecutarAviso(ctx, id, &uid, hoyLima())
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusAccepted, res)
}

// avisoManual: POST /avisos-cobranza/manual {unidad_ids?, canales, asunto, cuerpo, adjunta_pdf} → enviar ahora.
// Sin unidad_ids va a todas las unidades con deuda vencida. Una unidad sin deuda vencida no recibe aviso.
func (s *Server) avisoManual(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UnidadIDs  []int64  `json:"unidad_ids"`
		Canales    []string `json:"canales"`
		Asunto     string   `json:"asunto"`
		Cuerpo     string   `json:"cuerpo"`
		AdjuntaPDF bool     `json:"adjunta_pdf"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.Asunto, in.Cuerpo = strings.TrimSpace(in.Asunto), strings.TrimSpace(in.Cuerpo)
	ev := P.Validacion("Revisa el aviso.")
	if in.Asunto == "" {
		ev.Campo("asunto", "Obligatorio.")
	}
	if in.Cuerpo == "" {
		ev.Campo("cuerpo", "Obligatorio.")
	}
	if m := validarCanales(in.Canales); m != "" {
		ev.Campo("canales", m)
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	e := edf(r)
	ctx := r.Context()
	var solo []int64
	if len(in.UnidadIDs) > 0 {
		solo = in.UnidadIDs
	}
	ds, err := s.deudores(ctx, e.ID, solo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	uid := ses(r).UsuarioID
	hoy := hoyLima()
	enc := 0
	for _, d := range ds {
		n, err := s.enviarAviso(ctx, e.ID, e.Nombre, d, plantillaAviso{asunto: in.Asunto, cuerpo: in.Cuerpo, canales: in.Canales, adjuntaPDF: in.AdjuntaPDF}, &uid, hoy)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		enc += n
	}
	P.JSON(w, http.StatusAccepted, map[string]any{"deudores": len(ds), "encolados": enc})
}

// AvisosDelDia: la tarea diaria. Toma cada aviso activo al que le toca hoy y aún no corrió hoy (reclamo
// atómico sobre ultima_ejecucion, así dos réplicas no lo duplican) y lo ejecuta.
func (s *Server) AvisosDelDia(ctx context.Context, hoy time.Time) (int, error) {
	filas, err := s.DB.Query(ctx, `SELECT id, frecuencia, dia FROM aviso_cobranza WHERE activo AND (ultima_ejecucion IS NULL OR ultima_ejecucion < $1)`, hoy)
	if err != nil {
		return 0, err
	}
	type av struct {
		id   int64
		frec string
		dia  int
	}
	var avs []av
	for filas.Next() {
		var a av
		if filas.Scan(&a.id, &a.frec, &a.dia) == nil {
			avs = append(avs, a)
		}
	}
	filas.Close()
	n := 0
	for _, a := range avs {
		if !TocaHoy(a.frec, a.dia, hoy) {
			continue
		}
		ct, err := s.DB.Exec(ctx, `UPDATE aviso_cobranza SET ultima_ejecucion=$2 WHERE id=$1 AND (ultima_ejecucion IS NULL OR ultima_ejecucion < $2)`, a.id, hoy)
		if err != nil || ct.RowsAffected() == 0 {
			continue
		}
		res, err := s.EjecutarAviso(ctx, a.id, nil, hoy)
		if err != nil {
			slog.Warn("avisos de cobranza", "aviso", a.id, "err", err)
			continue
		}
		slog.Info("avisos de cobranza", "aviso", a.id, "deudores", res["deudores"], "encolados", res["encolados"])
		n++
	}
	return n, nil
}

// tareaAvisosCobranza: se llama cada minuto desde Tareas; corre los avisos del día a partir de las 8:00 de Lima.
func (s *Server) tareaAvisosCobranza(ctx context.Context) {
	if time.Now().In(P.Lima).Hour() < 8 {
		return
	}
	if _, err := s.AvisosDelDia(ctx, hoyLima()); err != nil {
		slog.Warn("avisos de cobranza", "err", err)
	}
}
