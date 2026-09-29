package app

import (
	"bytes"
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"edisys/api/internal/correo"
	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Correo: bandeja de salida (correo_mensaje, 0010) igual que WhatsApp. CORREO_MODO=simulado no envía nada;
// CORREO_MODO=smtp entrega por SMTP (en local, Mailpit en http://localhost:4726).

type adjuntoCola struct {
	nombre, tipo string
	datos        []byte
}

// encolarCorreo registra un correo «pendiente» con sus adjuntos.
func (s *Server) encolarCorreo(ctx context.Context, q db.Q, eid int64, unidad *int64, para, nombre, asunto, html, texto, origen, ref string, adj []adjuntoCola, usuario *int64) (int64, error) {
	if !correo.ValidarDireccion(para) {
		return 0, P.Validacion("Correo inválido: "+para).Campo("para", "Correo válido.")
	}
	var id int64
	if err := q.QueryRow(ctx, `INSERT INTO correo_mensaje (edificio_id, unidad_id, para, nombre, asunto, html, texto, origen, referencia, enviado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, eid, unidad, strings.TrimSpace(para), nombre, asunto, html, texto, origen, ref, usuario).Scan(&id); err != nil {
		return 0, err
	}
	for _, a := range adj {
		if _, err := q.Exec(ctx, `INSERT INTO correo_adjunto (mensaje_id, nombre, tipo_mime, datos) VALUES ($1,$2,$3,$4)`, id, a.nombre, a.tipo, a.datos); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func (s *Server) smtp() correo.SMTP {
	return correo.SMTP{Host: s.Cfg.SMTPHost, Puerto: s.Cfg.SMTPPuerto, Usuario: s.Cfg.SMTPUsuario, Clave: s.Cfg.SMTPClave}
}

// despacharCorreos procesa la bandeja. En simulado marca «simulado»; en smtp envía (3 intentos).
func (s *Server) despacharCorreos(ctx context.Context) {
	filas, err := s.DB.Query(ctx, `SELECT id, para, nombre, asunto, html, texto FROM correo_mensaje WHERE estado='pendiente' ORDER BY id LIMIT 100`)
	if err != nil {
		slog.Warn("correo: leer bandeja", "err", err)
		return
	}
	type msg struct {
		id                              int64
		para, nombre, asunto, html, txt string
	}
	var ms []msg
	for filas.Next() {
		var m msg
		if err := filas.Scan(&m.id, &m.para, &m.nombre, &m.asunto, &m.html, &m.txt); err == nil {
			ms = append(ms, m)
		}
	}
	filas.Close()
	for _, m := range ms {
		var intentos int
		if err := s.DB.QueryRow(ctx, `UPDATE correo_mensaje SET intentos=intentos+1 WHERE id=$1 AND estado='pendiente' RETURNING intentos`, m.id).Scan(&intentos); err != nil {
			continue
		}
		if s.Cfg.CorreoModo != "smtp" {
			_, _ = s.DB.Exec(ctx, `UPDATE correo_mensaje SET estado='simulado', procesado_en=now() WHERE id=$1`, m.id)
			continue
		}
		cm := correo.Mensaje{De: s.Cfg.CorreoDe, Para: m.para, Asunto: m.asunto, HTML: m.html, Texto: m.txt}
		if m.nombre != "" {
			cm.Para = m.nombre + " <" + m.para + ">"
		}
		fa, err := s.DB.Query(ctx, `SELECT nombre, tipo_mime, datos FROM correo_adjunto WHERE mensaje_id=$1 ORDER BY id`, m.id)
		if err == nil {
			for fa.Next() {
				var a correo.Adjunto
				if fa.Scan(&a.Nombre, &a.Tipo, &a.Datos) == nil {
					cm.Adjuntos = append(cm.Adjuntos, a)
				}
			}
			fa.Close()
		}
		if err := s.smtp().Enviar(cm); err != nil {
			estado := "pendiente"
			if intentos >= 3 {
				estado = "error"
			}
			_, _ = s.DB.Exec(ctx, `UPDATE correo_mensaje SET estado=$2, error=$3, procesado_en=now() WHERE id=$1`, m.id, estado, recortar(err.Error(), 300))
			continue
		}
		_, _ = s.DB.Exec(ctx, `UPDATE correo_mensaje SET estado='enviado', error='', procesado_en=now() WHERE id=$1`, m.id)
	}
}

func (s *Server) conteoCorreos(ctx context.Context, ids []int64) map[string]int {
	c := map[string]int{"simulado": 0, "enviado": 0, "error": 0, "pendiente": 0}
	f, err := s.DB.Query(ctx, `SELECT estado, count(*) FROM correo_mensaje WHERE id = ANY($1) GROUP BY estado`, ids)
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

// ---------- plantillas (HTML sencillo, en español) ----------

var plantillaCorreo = template.Must(template.New("correo").Parse(`<!doctype html><html lang="es"><body style="margin:0;background:#F1F5F9;font-family:Arial,Helvetica,sans-serif;color:#0F172A">
<table width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:24px 12px">
<table width="560" cellpadding="0" cellspacing="0" style="background:#FFFFFF;border-radius:12px;overflow:hidden">
<tr><td style="background:#155E75;color:#FFFFFF;padding:18px 24px"><strong style="font-size:20px">EDISYS</strong><br><span style="font-size:13px">{{.Edificio}}</span></td></tr>
<tr><td style="padding:24px;font-size:15px;line-height:1.5">
<p>Hola, {{.Nombre}}:</p>
<p>{{.Intro}}</p>
<table width="100%" cellpadding="6" cellspacing="0" style="border-collapse:collapse;font-size:14px">
{{range .Filas}}<tr><td style="border-bottom:1px solid #E2E8F0">{{.Etiqueta}}</td><td align="right" style="border-bottom:1px solid #E2E8F0"><strong>{{.Valor}}</strong></td></tr>{{end}}
</table>
<p>{{.Cierre}}</p>
<p style="color:#64748B;font-size:12px">Adjuntamos el PDF. Este correo lo envía EDISYS en nombre de la administración; no respondas a esta dirección.</p>
</td></tr></table></td></tr></table></body></html>`))

type filaCorreo struct{ Etiqueta, Valor string }

type datosCorreo struct {
	Edificio, Nombre, Intro, Cierre string
	Filas                           []filaCorreo
}

func armarCorreo(d datosCorreo) (html, texto string) {
	var b bytes.Buffer
	_ = plantillaCorreo.Execute(&b, d)
	var t strings.Builder
	t.WriteString("Hola, " + d.Nombre + ":\n\n" + d.Intro + "\n\n")
	for _, f := range d.Filas {
		t.WriteString("- " + f.Etiqueta + ": " + f.Valor + "\n")
	}
	t.WriteString("\n" + d.Cierre + "\n\nAdjuntamos el PDF. Este correo lo envía EDISYS en nombre de la administración.\n")
	return b.String(), t.String()
}

// ---------- endpoints ----------

// enviarRecibosCorreo: POST /recibos/{rid}/enviar-correo donde {rid} es el periodo (AAAA-MM).
// Cada propietario con correo recibe su recibo en PDF.
func (s *Server) enviarRecibosCorreo(w http.ResponseWriter, r *http.Request) {
	periodo := chi.URLParam(r, "rid")
	if !P.PeriodoValido(periodo) {
		P.Fallo(w, r, P.Validacion("Periodo AAAA-MM.").Campo("periodo", "Formato AAAA-MM."))
		return
	}
	res, err := s.CorreoRecibos(r.Context(), edf(r), ses(r).UsuarioID, periodo, nil)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusAccepted, res)
}

// CorreoRecibos encola el recibo en PDF para el propietario de cada recibo emitido del periodo (o de los ids).
func (s *Server) CorreoRecibos(ctx context.Context, e *Edificio, uid int64, periodo string, ids []int64) (map[string]any, error) {
	filas, err := db.Filas(ctx, s.DB, `SELECT r.id, u.id AS unidad_id, u.codigo, COALESCE(pe.nombre,'') AS nombre, COALESCE(pe.correo,'') AS correo
		FROM recibo r JOIN unidad u ON u.id=r.unidad_id JOIN periodo p ON p.id=r.periodo_id
		LEFT JOIN LATERAL (SELECT pe.nombre, pe.correo FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id
			WHERE up.unidad_id=u.id AND up.rol='propietario' AND up.hasta IS NULL LIMIT 1) pe ON true
		WHERE r.edificio_id=$1 AND r.origen='periodo' AND r.estado NOT IN ('borrador','anulado') AND (($2 <> '' AND p.periodo=$2) OR r.id = ANY($3))
		ORDER BY u.codigo`, e.ID, periodo, ids)
	if err != nil {
		return nil, err
	}
	if len(filas) == 0 {
		return nil, P.NoEncontrado("recibos emitidos para enviar")
	}
	sinCorreo := []string{}
	enviados := []int64{}
	for _, f := range filas {
		cod := f["codigo"].(string)
		if !correo.ValidarDireccion(f["correo"].(string)) {
			sinCorreo = append(sinCorreo, cod)
			continue
		}
		rid := f["id"].(int64)
		rc, err := s.reciboVisible(ctx, e, rid)
		if err != nil {
			return nil, err
		}
		pdfB, err := s.reciboPDF(ctx, rc, rid)
		if err != nil {
			return nil, err
		}
		per := rc["periodo"].(string)
		saldo := rc["saldo_cts"].(int64)
		estado := "Pendiente de pago"
		if saldo == 0 {
			estado = "Pagado, ¡gracias!"
		}
		filasC := []filaCorreo{{"Recibo", val(rc["numero"])}, {"Total", P.Soles(rc["total_cts"].(int64))}, {"Pagado", P.Soles(rc["pagado_cts"].(int64))},
			{"Saldo", P.Soles(saldo)}, {"Vence", fechaCorta(rc["vence"])}, {"Estado", estado}}
		cierre := "Puedes verlo con la foto de tu medidor y subir tu voucher en " + s.Cfg.URLPublica + "/app/recibos/."
		if yp, _ := rc["yape_numero"].(string); yp != "" && saldo > 0 {
			cierre = "Paga por Yape al " + yp + " con el concepto «Dpto " + cod + " " + per + "» y sube tu voucher en " + s.Cfg.URLPublica + "/app/recibos/."
		}
		html, texto := armarCorreo(datosCorreo{Edificio: e.Nombre, Nombre: primerNombre(f["nombre"].(string)), Intro: "Te enviamos tu recibo de mantenimiento de " + P.NombrePeriodo(per) + " del Dpto " + cod + ".",
			Filas: filasC, Cierre: cierre})
		unidad := f["unidad_id"].(int64)
		id, err := s.encolarCorreo(ctx, s.DB, e.ID, &unidad, f["correo"].(string), f["nombre"].(string), "Tu recibo de "+P.NombrePeriodo(per)+" · Dpto "+cod,
			html, texto, "recibo", val(rc["numero"]), []adjuntoCola{{"recibo-" + val(rc["numero"]) + ".pdf", "application/pdf", pdfB}}, &uid)
		if err != nil {
			return nil, err
		}
		enviados = append(enviados, id)
		_, _ = s.DB.Exec(ctx, `UPDATE recibo SET enviado_en=now() WHERE id=$1`, rid)
	}
	s.despacharCorreos(ctx)
	c := s.conteoCorreos(ctx, enviados)
	return map[string]any{"periodo": periodo, "encolados": len(enviados), "en_cola": len(enviados), "enviados": c["enviado"], "simulados": c["simulado"],
		"errores": c["error"], "pendientes": c["pendiente"], "sin_correo": sinCorreo, "mensaje_ids": enviados, "modo": s.Cfg.CorreoModo, "canal": "correo"}, nil
}

// enviarBalanceCorreo: POST /balance/{periodo}/enviar-correo {destinatarios: todos|junta|propietarios}.
// La junta recibe el balance y el informe a la junta; los propietarios, el balance.
func (s *Server) enviarBalanceCorreo(w http.ResponseWriter, r *http.Request) {
	periodo, err := periodoPDF(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Destinatarios string `json:"destinatarios"`
	}
	if r.ContentLength > 0 {
		if err := P.Leer(r, &in); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	if in.Destinatarios == "" {
		in.Destinatarios = "todos"
	}
	e := edf(r)
	ctx := r.Context()
	a, err := s.ArbolBalance(ctx, s.DB, e.ID, periodo, vistaCompleta())
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	balance, err := s.BalancePDF(ctx, e.ID, periodo, vistaCompleta(), false)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	informe, err := s.BalancePDF(ctx, e.ID, periodo, vistaCompleta(), true)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	type dest struct {
		correo, nombre string
		junta          bool
		unidad         *int64
	}
	vistos := map[string]bool{}
	var ds []dest
	if in.Destinatarios != "propietarios" {
		fj, _ := db.Filas(ctx, s.DB, `SELECT us.correo, us.nombre FROM junta_miembro jm JOIN usuario us ON us.id=jm.usuario_id
			WHERE jm.edificio_id=$1 AND jm.activo AND us.activo AND us.correo IS NOT NULL ORDER BY jm.presidente DESC, us.id`, e.ID)
		for _, f := range fj {
			c := strings.ToLower(f["correo"].(string))
			if !vistos[c] && correo.ValidarDireccion(c) {
				vistos[c] = true
				ds = append(ds, dest{c, f["nombre"].(string), true, nil})
			}
		}
	}
	if in.Destinatarios != "junta" {
		fp, _ := db.Filas(ctx, s.DB, `SELECT pe.correo, pe.nombre, u.id AS unidad_id FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id JOIN unidad u ON u.id=up.unidad_id
			WHERE u.edificio_id=$1 AND up.rol='propietario' AND up.hasta IS NULL AND pe.correo <> '' ORDER BY u.codigo`, e.ID)
		for _, f := range fp {
			c := strings.ToLower(f["correo"].(string))
			if !vistos[c] && correo.ValidarDireccion(c) {
				vistos[c] = true
				u := f["unidad_id"].(int64)
				ds = append(ds, dest{c, f["nombre"].(string), false, &u})
			}
		}
	}
	k := a.KPIs
	filasC := []filaCorreo{{"Ingresos cobrados", P.Soles(k.IngresosCts)}, {"Egresos", P.Soles(k.EgresosCts)}, {"Saldo del mes", P.Soles(k.SaldoCts)},
		{"Morosidad del mes", pctES(k.Morosidad.Pct)}, {"Banco acumulado", P.Soles(k.BancoCts)}}
	uid := ses(r).UsuarioID
	ids := []int64{}
	for _, d := range ds {
		adj := []adjuntoCola{{"balance-" + periodo + ".pdf", "application/pdf", balance}}
		intro := "Te enviamos el balance de " + P.NombrePeriodo(periodo) + " del edificio. Los ingresos cuentan lo cobrado; lo emitido y no cobrado es la morosidad."
		if d.junta {
			adj = append(adj, adjuntoCola{"informe-junta-" + periodo + ".pdf", "application/pdf", informe})
			intro += " Como miembro de la junta, también va el informe con los pendientes por criticidad y las aprobaciones del mes."
		}
		html, texto := armarCorreo(datosCorreo{Edificio: e.Nombre, Nombre: primerNombre(d.nombre), Intro: intro, Filas: filasC,
			Cierre: "El detalle con cada sustento está en " + s.Cfg.URLPublica + "/app/balance/?periodo=" + periodo + "."})
		id, err := s.encolarCorreo(ctx, s.DB, e.ID, d.unidad, d.correo, d.nombre, "Balance de "+P.NombrePeriodo(periodo)+" · "+e.Nombre, html, texto, "balance", periodo, adj, &uid)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		ids = append(ids, id)
	}
	s.despacharCorreos(ctx)
	c := s.conteoCorreos(ctx, ids)
	P.JSON(w, http.StatusAccepted, map[string]any{"periodo": periodo, "encolados": len(ids), "enviados": c["enviado"], "simulados": c["simulado"],
		"errores": c["error"], "pendientes": c["pendiente"], "mensaje_ids": ids, "modo": s.Cfg.CorreoModo})
}

// listarCorreos: GET /correo/mensajes?estado=&origen=&pagina=
func (s *Server) listarCorreos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	q := r.URL.Query()
	pagina, por := paginacion(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT m.id, m.para, m.nombre, m.asunto, m.estado, m.origen, m.referencia, m.intentos, m.error, m.creado_en, m.procesado_en,
			u.codigo AS unidad, (SELECT count(*) FROM correo_adjunto a WHERE a.mensaje_id=m.id) AS adjuntos
		FROM correo_mensaje m LEFT JOIN unidad u ON u.id=m.unidad_id
		WHERE m.edificio_id=$1 AND ($2='' OR m.estado=$2) AND ($3='' OR m.origen=$3) ORDER BY m.id DESC LIMIT $4 OFFSET $5`,
		e.ID, q.Get("estado"), q.Get("origen"), por, (pagina-1)*por)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var total int64
	_ = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM correo_mensaje WHERE edificio_id=$1`, e.ID).Scan(&total)
	resp := paginado(filas, total, pagina)
	resp["modo"] = s.Cfg.CorreoModo
	P.JSON(w, http.StatusOK, resp)
}
